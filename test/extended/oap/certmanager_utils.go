package oap

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/blang/semver"
	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"github.com/openshift/openshift-tests-private/test/extended/util/architecture"
	"github.com/tidwall/gjson"
	gcpcrm "google.golang.org/api/cloudresourcemanager/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

const (
	subscriptionName       = "openshift-cert-manager-operator"
	operatorNamespace      = "cert-manager-operator"
	operatorLabel          = "name=cert-manager-operator"
	operatorDeploymentName = "cert-manager-operator-controller-manager"
	controllerNamespace    = "cert-manager"
	controllerLabel        = "app.kubernetes.io/name=cert-manager"
	operandNamespace       = "cert-manager"
	operandLabel           = "app.kubernetes.io/instance=cert-manager"
	defaultOperandPodNum   = 3
)

// Get the credential from cluster
func getCredentialFromCluster(oc *exutil.CLI, cloudProvider string) (string, string) {
	var accessKeyID, secureKey string
	switch cloudProvider {
	case "aws":
		credential, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret/aws-creds", "-n", "kube-system", "-o", "json").Output()
		accessKeyIDBase64, secureKeyBase64 := gjson.Get(credential, `data.aws_access_key_id`).String(), gjson.Get(credential, `data.aws_secret_access_key`).String()
		awsAccessKeyID, err := base64.StdEncoding.DecodeString(accessKeyIDBase64)
		o.Expect(err).NotTo(o.HaveOccurred())
		awsSecureKey, err := base64.StdEncoding.DecodeString(secureKeyBase64)
		o.Expect(err).NotTo(o.HaveOccurred())
		accessKeyID = string(awsAccessKeyID)
		secureKey = string(awsSecureKey)
		os.Setenv("AWS_ACCESS_KEY_ID", accessKeyID)
		os.Setenv("AWS_SECRET_ACCESS_KEY", secureKey)
	case "vsphere":
		e2e.Logf("Get %s backend credential is under development", cloudProvider)
	case "gcp":
		e2e.Logf("Get %s backend credential is under development", cloudProvider)
	case "azure":
		e2e.Logf("Get %s backend credential is under development", cloudProvider)
	case "openstack":
		e2e.Logf("Get %s backend credential is under development", cloudProvider)
	default:
		e2e.Logf("unknown cloud provider")
	}
	return accessKeyID, secureKey
}

// Generate a random string with given number of digits
func getRandomString(digit int) string {
	chars := "abcdefghijklmnopqrstuvwxyz0123456789"
	seed := rand.New(rand.NewSource(time.Now().UnixNano()))
	buffer := make([]byte, digit)
	for index := range buffer {
		buffer[index] = chars[seed.Intn(len(chars))]
	}
	return string(buffer)
}

// parse base domain from dns config. format is like $clustername.$basedomain
func getBaseDomain(oc *exutil.CLI) string {
	str, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("dns/cluster", `-ojsonpath={.spec.baseDomain}`).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return str
}

func getSAToken(oc *exutil.CLI, sa, ns string) (string, error) {
	e2e.Logf("Getting a token assgined to specific serviceaccount from %s namespace...", ns)
	token, err := oc.AsAdmin().WithoutNamespace().Run("create").Args("token", sa, "-n", ns).Output()
	if err != nil {
		if strings.Contains(token, "unknown command") { // oc client is old version, create token is not supported
			e2e.Logf("oc create token is not supported by current client, use oc sa get-token instead")
			token, err = oc.AsAdmin().WithoutNamespace().Run("sa").Args("get-token", sa, "-n", ns).Output()
		} else {
			return "", err
		}
	}

	return token, err
}

// Get AWS Route53's hosted zone ID. Returning "" means retreiving Route53 hosted zone ID for current env returns none
// If there are multiple HostedZones sharing the same name (which is relatively rare), it will return the first one matched by AWS SDK.
func getRoute53HostedZoneID(awsConfig aws.Config, hostedZoneName string) string {
	// Equals: `aws route53 list-hosted-zones-by-name --dns-name qe.devcluster.openshift.com`
	route53Client := route53.NewFromConfig(awsConfig)
	list, err := route53Client.ListHostedZonesByName(
		context.Background(),
		&route53.ListHostedZonesByNameInput{
			DNSName: aws.String(hostedZoneName),
		},
	)
	o.Expect(err).NotTo(o.HaveOccurred())

	hostedZoneID := ""
	for _, hostedZone := range list.HostedZones {
		if strings.TrimSuffix(aws.ToString(hostedZone.Name), ".") == hostedZoneName {
			hostedZoneID = aws.ToString(hostedZone.Id)
			break
		}
	}
	return strings.TrimPrefix(hostedZoneID, "/hostedzone/")
}

// Add or remove the IAM Policy Binding from GCP resource. Set the argument 'add' to true to add the binding, or false to remove it.
func updateIamPolicyBinding(crmService *gcpcrm.Service, resource, role, member string, add bool) {
	statusErr := wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 60*time.Second, true, func(context.Context) (bool, error) {
		policy, err := crmService.Projects.GetIamPolicy(resource, &gcpcrm.GetIamPolicyRequest{}).Do()
		if err != nil {
			e2e.Logf("got error from 'GetIamPolicy':%v", err.Error())
			return false, nil
		}

		if add {
			policy.Bindings = append(policy.Bindings, &gcpcrm.Binding{
				Role:    role,
				Members: []string{member},
			})
		} else {
			removeMember(policy, role, member)
		}
		_, err = crmService.Projects.SetIamPolicy(resource, &gcpcrm.SetIamPolicyRequest{Policy: policy}).Do()
		if err != nil {
			e2e.Logf("got error from 'SetIamPolicy':%v", err.Error())
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(statusErr, "timeout waiting for updating IAM Policy successfully")
}

// removeMember removes a member from a role binding in a GCP IAM policy
// xref: https://cloud.google.com/iam/docs/samples/iam-modify-policy-remove-member#iam_modify_policy_remove_member-go
func removeMember(policy *gcpcrm.Policy, role, member string) {
	bindings := policy.Bindings
	bindingIndex, memberIndex := -1, -1
	for bIdx := range bindings {
		if bindings[bIdx].Role != role {
			continue
		}
		bindingIndex = bIdx
		for mIdx := range bindings[bindingIndex].Members {
			if bindings[bindingIndex].Members[mIdx] != member {
				continue
			}
			memberIndex = mIdx
			break
		}
	}
	if bindingIndex == -1 {
		e2e.Logf("Role '%v' not found.", role)
		return
	}
	if memberIndex == -1 {
		e2e.Logf("Role '%v' found. Member '%v' not found.", role, member)
		return
	}

	members := removeIdx(bindings[bindingIndex].Members, memberIndex)
	bindings[bindingIndex].Members = members
	if len(members) == 0 {
		bindings = removeIdx(bindings, bindingIndex)
		policy.Bindings = bindings
	}
	e2e.Logf("Role '%v' found. Member '%v' will be removed.", role, member)
}

// removeIdx removes arr[idx] from an array
// xref: https://cloud.google.com/iam/docs/samples/iam-modify-policy-remove-member#iam_modify_policy_remove_member-go
func removeIdx[T any](arr []T, idx int) []T {
	return append(arr[:idx], arr[idx+1:]...)
}

// Get the parent domain. "a.b.c"'s parent domain is "b.c"
func getParentDomain(domain string) (string, error) {
	parts := strings.Split(domain, ".")
	if len(parts) <= 1 {
		return "", fmt.Errorf("no parent domain for invalid input: %s", domain)
	}
	parentDomain := strings.Join(parts[1:], ".")
	return parentDomain, nil
}

func isDeploymentReady(oc *exutil.CLI, namespace string, deploymentName string) bool {
	e2e.Logf("Checking readiness of deployment '%s' in namespace '%s'...", deploymentName, namespace)
	status, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deploy", deploymentName, "-n", namespace, `-o=jsonpath={.status.conditions[?(@.type=="Available")].status}`).Output()
	if err != nil {
		e2e.Logf("Failed to check deployment readiness: %v", err.Error())
		return false
	}
	if strings.TrimSpace(status) == "True" {
		e2e.Logf("Deployment '%s' is ready and available.", deploymentName)
		return true
	}
	e2e.Logf("Deployment '%s' is not ready. Status: '%s'", deploymentName, status)
	return false
}

// Create Cert Manager Operator
func createCertManagerOperator(oc *exutil.CLI) {
	var (
		testMode    = "PROD"
		channelName = "stable-v1"
	)
	e2e.Logf("=> create the operator namespace")
	buildPruningBaseDir := exutil.FixturePath("testdata", "oap/certmanager")
	namespaceFile := filepath.Join(buildPruningBaseDir, "namespace.yaml")
	output, err := oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", namespaceFile).Output()
	// skip the install process to mitigate the namespace deletion terminating issue caused by case 62006
	// the full message is 'Detected changes to resource cert-manager-operator which is currently being deleted'
	if strings.Contains(output, "being deleted") {
		g.Skip("skip the install process as the cert-manager-operator namespace is being terminated due to other env issue e.g. we ever hit such failures caused by OCPBUGS-31443")
	}
	if err != nil && !strings.Contains(output, "AlreadyExists") {
		e2e.Failf("Failed to apply namespace cert-manager-operator: %v", err)
	}

	// use the default catalogsource if 'qe-app-registry' doesn't exists
	output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-marketplace", "catalogsource", "qe-app-registry").Output()
	if strings.Contains(output, "NotFound") || err != nil {
		testMode = "PROD"
	}

	var catalogSourceName, catalogSourceNamespace string
	if testMode == "PROD" {
		catalogSourceName = "redhat-operators"
		catalogSourceNamespace = "openshift-marketplace"

		// skip if the default catalogsource doesn't contain subscription's packagemanifest
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("packagemanifest", "-n", catalogSourceNamespace, "-l", "catalog="+catalogSourceName, "--field-selector", "metadata.name="+subscriptionName).Output()
		if !strings.Contains(output, subscriptionName) || err != nil {
			g.Skip("skip since no available packagemanifest was found")
		}
	} else if testMode == "STAGE" {
		// TODO: until the Konflux pipeline is configured to add a deterministic tag, update this pull spec whenever a new stage build is out
		fbcImage := "quay.io/redhat-user-workloads/cert-manager-oape-tenant/cert-manager-operator-1-15/cert-manager-operator-fbc-1-15:edb257f20c2c0261200e2f0f7bf8118099567745"
		catalogSourceName = "konflux-fbc-cert-manager"
		catalogSourceNamespace = operatorNamespace
		e2e.Logf("=> create the file-based catalog")
		createFBC(oc, catalogSourceName, catalogSourceNamespace, fbcImage)
	} else {
		e2e.Failf("Invalid Test Mode, it should be 'PROD' or 'STAGE'")
	}
	e2e.Logf("=> using catalogsource '%s' from namespace '%s'", catalogSourceName, catalogSourceNamespace)

	e2e.Logf("=> create the operatorgroup")
	operatorGroupFile := filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
	err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", operatorGroupFile).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())

	e2e.Logf("=> create the subscription")
	subscriptionTemplate := filepath.Join(buildPruningBaseDir, "subscription.yaml")
	params := []string{"-f", subscriptionTemplate, "-p", "NAME=" + subscriptionName, "SOURCE=" + catalogSourceName, "SOURCE_NAMESPACE=" + catalogSourceNamespace, "CHANNEL=" + channelName}
	exutil.ApplyNsResourceFromTemplate(oc, operatorNamespace, params...)
	// Wait for subscription state to become AtLatestKnown
	err = wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 180*time.Second, true, func(context.Context) (bool, error) {
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", subscriptionName, "-n", operatorNamespace, "-o=jsonpath={.status.state}").Output()
		if strings.Contains(output, "AtLatestKnown") {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		dumpResource(oc, operatorNamespace, "sub", subscriptionName, "-o=jsonpath={.status}")
	}
	exutil.AssertWaitPollNoErr(err, "timeout waiting for subscription state to become AtLatestKnown")

	e2e.Logf("=> retrieve the installed CSV name")
	csvName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", subscriptionName, "-n", operatorNamespace, "-o=jsonpath={.status.installedCSV}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(csvName).NotTo(o.BeEmpty())
	// Wait for csv phase to become Succeeded
	err = wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 180*time.Second, true, func(context.Context) (bool, error) {
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", csvName, "-n", operatorNamespace, "-o=jsonpath={.status.phase}").Output()
		if strings.Contains(output, "Succeeded") {
			e2e.Logf("csv '%s' installed successfully", csvName)
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		dumpResource(oc, operatorNamespace, "csv", csvName, "-o=jsonpath={.status}")
	}
	exutil.AssertWaitPollNoErr(err, "timeout waiting for csv phase to become Succeeded")

	e2e.Logf("=> checking the cert-manager operand pods readiness")
	err = wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 300*time.Second, true, func(context.Context) (bool, error) {
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", operandNamespace, "-l", operandLabel, "--field-selector=status.phase=Running", "-o=jsonpath={.items[*].metadata.name}").Output()
		if len(strings.Fields(output)) == defaultOperandPodNum {
			e2e.Logf("all operand pods are up and running!")
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", operandNamespace, "-l", operandLabel).Execute()
	}
	exutil.AssertWaitPollNoErr(err, "timeout waiting for all operand pods phase to become Running")
}

// uninstall cert-manager operator and cleanup its operand resources
func cleanupCertManagerOperator(oc *exutil.CLI) {
	e2e.Logf("=> delete the subscription and installed CSV")
	csvName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", subscriptionName, "-n", operatorNamespace, "-o=jsonpath={.status.installedCSV}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("sub", subscriptionName, "-n", operatorNamespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("csv", csvName, "-n", operatorNamespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())

	e2e.Logf("=> checking the cert-manager operator pod, it should be gone")
	err = wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 60*time.Second, true, func(context.Context) (bool, error) {
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", operatorNamespace, "-l", operatorLabel).Output()
		if strings.Contains(output, "No resources found") || err != nil {
			e2e.Logf("operator pod is deleted")
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "timeout waiting for operator pod to be deleted")

	e2e.Logf("=> delete the operator namespace")
	// To mitigate the known issue: https://issues.redhat.com/browse/OCPBUGS-31443
	err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("all", "--all", "-n", operatorNamespace, "--force", "--grace-period=0", "--wait=false").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("ns", operatorNamespace, "--force", "--grace-period=0", "--wait=false", "-v=6").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())

	e2e.Logf("=> delete the operand namespace")
	// To mitigate the known issue: https://issues.redhat.com/browse/OCPBUGS-31443
	err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("all", "--all", "-n", operandNamespace, "--force", "--grace-period=0", "--wait=false").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("ns", operandNamespace, "--force", "--grace-period=0", "--wait=false", "-v=6").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())

	e2e.Logf("=> delete the 'certmanagers' cluster object")
	// remove the finalizers from that object first, otherwise the deletion would be stuck
	err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("certmanagers", "cluster", "--type=merge", `-p={"metadata":{"finalizers":null}}`).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("certmanagers", "cluster").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())

	e2e.Logf("=> delete the cert-manager operator CRD")
	err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("crd", "certmanagers.operator.openshift.io").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("=> delete the cert-manager CRD")
	err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("crd", "-l", operandLabel).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())

	e2e.Logf("=> checking any of the resource types of cert-manager, it should be gone")
	statusErr := wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 60*time.Second, false, func(ctx context.Context) (bool, error) {
		err = oc.AsAdmin().Run("get").Args("issuer").Execute()
		if err != nil {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(statusErr, "timeout waiting for the cert-manager CRDs deletion to take effect")

	e2e.Logf("=> delete the clusterrolebindings and clusterroles of the the cert-manager")
	err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("clusterrolebindings", "-l", operandLabel).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("clusterrole", "-l", operandLabel).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

// create selfsigned issuer
func createIssuer(oc *exutil.CLI, ns string) {
	e2e.Logf("create a selfsigned issuer")
	buildPruningBaseDir := exutil.FixturePath("testdata", "oap/certmanager")
	issuerFile := filepath.Join(buildPruningBaseDir, "issuer-selfsigned.yaml")
	err := wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 120*time.Second, true, func(context.Context) (bool, error) {
		retryErr := oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", ns, "-f", issuerFile).Execute()
		if retryErr != nil {
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, "timeout waiting for issuer to be created successfully")
	err = waitForResourceReadiness(oc, ns, "issuer", "default-selfsigned", 10*time.Second, 120*time.Second)
	if err != nil {
		dumpResource(oc, ns, "issuer", "default-selfsigned", "-o=yaml")
	}
	exutil.AssertWaitPollNoErr(err, "timeout waiting for issuer to become Ready")
}

// create certificate using selfsigned issuer
func createCertificate(oc *exutil.CLI, ns string) {
	e2e.Logf("create a certificate using the selfsigned issuer")
	buildPruningBaseDir := exutil.FixturePath("testdata", "oap/certmanager")
	certFile := filepath.Join(buildPruningBaseDir, "cert-selfsigned.yaml")
	err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", ns, "-f", certFile).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	err = waitForResourceReadiness(oc, ns, "certificate", "default-selfsigned-cert", 10*time.Second, 300*time.Second)
	if err != nil {
		dumpResource(oc, ns, "certificate", "default-selfsigned-cert", "-o=yaml")
	}
	exutil.AssertWaitPollNoErr(err, "timeout waiting for certificate to become Ready")
}

// waitForResourceReadiness polls the status of the object and returns no error once Ready.
// 'namespace' could be an empty string to indicate not to specify the namespace.
// 'resourceType' is applicable to 'Certificate', 'Issuer' and 'ClusterIssuer'.
func waitForResourceReadiness(oc *exutil.CLI, namespace, resourceType, resourceName string, interval, timeout time.Duration) error {
	args := []string{resourceType, resourceName}
	if len(namespace) > 0 {
		args = append(args, "-n="+namespace)
	}

	statusErr := wait.PollUntilContextTimeout(context.TODO(), interval, timeout, false, func(ctx context.Context) (bool, error) {
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args(args...).Output()
		if strings.Contains(output, "True") {
			return true, nil
		}
		return false, nil
	})
	return statusErr
}

// dumpResource dumps the resource for debugging.
// 'parameter' could be any additional args you want to specify, like "-o=yaml" or "-o=jsonpath={.status}".
func dumpResource(oc *exutil.CLI, namespace, resourceType, resourceName, parameter string) {
	args := []string{resourceType, resourceName, parameter}
	if len(namespace) > 0 {
		args = append(args, "-n="+namespace)
	}

	output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args(args...).Output()
	e2e.Logf("Dumping the %s '%s' with parameter '%s':\n%s", resourceType, resourceName, parameter, output)
}

// getCertificateExpireTime returns the TLS secret's cert expire time.
func getCertificateExpireTime(oc *exutil.CLI, namespace string, secretName string) (time.Time, error) {
	// get certificate data from secret
	tlsCrtData, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret", secretName, "-n", namespace, "-o=jsonpath={.data.tls\\.crt}").Output()
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to get secret '%s'", secretName)
	}
	if len(tlsCrtData) == 0 {
		return time.Time{}, fmt.Errorf("empty TLS data in secret '%s'", secretName)
	}

	// parse certificate using x509 lib
	tlsCrtBytes, _ := base64.StdEncoding.DecodeString(tlsCrtData)
	block, _ := pem.Decode(tlsCrtBytes)
	parsedCert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to parse certificate: %v", err)
	}

	expireTime := parsedCert.NotAfter
	return expireTime, nil
}

// verifyCertificateRenewal verifies if the TLS secret's certificate got renewed after the specific interval
// 'interval' should be no less than 1 min, it can be calculated from 'spec.duration - spec.renewBefore' of the certificate object
func verifyCertificateRenewal(oc *exutil.CLI, namespace, secretName string, interval time.Duration) {
	// store the initial cert expire time for verifying renewal in the end
	initialExpireTime, _ := getCertificateExpireTime(oc, namespace, secretName)
	e2e.Logf("certificate initial expire time: %v ", initialExpireTime)

	// poll the value of the currentExpireTime and returns no error once it's bigger than initialExpireTime
	statusErr := wait.PollUntilContextTimeout(context.TODO(), 30*time.Second, interval, false, func(ctx context.Context) (bool, error) {
		currentExpireTime, err := getCertificateExpireTime(oc, namespace, secretName)
		if err != nil {
			e2e.Logf("got error in func 'getCertificateExpireTime': %v", err)
			return false, nil
		}

		// return Ture if currentExpireTime > initialExpireTime, indicates cert got renewed
		if currentExpireTime.After(initialExpireTime) {
			e2e.Logf("certificate renewed successfully, current expire time: %v ", currentExpireTime)
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(statusErr, "timeout waiting for certificate to get renewed")
}

// Check and verify issued certificate's subject CN (Common Name) content.
func verifyCertificate(oc *exutil.CLI, certName string, namespace string) {
	e2e.Logf("Check if certificate secret is non-null.")
	secretName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("certificate", certName, "-n", namespace, "-o=jsonpath={.spec.secretName}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	tlsCrtData, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret", secretName, "-n", namespace, "-o=jsonpath={.data.tls\\.crt}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(tlsCrtData).NotTo(o.BeEmpty(), fmt.Sprintf("secret \"%v\"'s \"tls.crt\" field is empty.", secretName))

	commonName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("certificate", certName, "-n", namespace, "-o=jsonpath={.spec.commonName}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if len(commonName) != 0 {
		e2e.Logf("Verify if certificate's subject CN is correct.")
		tlsCrtBytes, _ := base64.StdEncoding.DecodeString(tlsCrtData)
		block, _ := pem.Decode(tlsCrtBytes)
		parsedCert, _ := x509.ParseCertificate(block.Bytes)
		if parsedCert.Subject.CommonName != commonName && !slices.Contains(parsedCert.DNSNames, commonName) {
			e2e.Failf("Incorrect subject CN: '%v' and '%v' found in issued certificate", parsedCert.Subject.CommonName, parsedCert.DNSNames)
		}
	} else {
		e2e.Logf("Skip content verification because subject CN isn't specificed.")
	}
}

// constructDNSName constructs a DNS name from the given base name with a random prefix for testing usage
func constructDNSName(base string) string {
	dnsName := getRandomString(4) + "." + base
	if len(dnsName) > 63 {
		g.Skip("Skip for the DNS name has more than 63 bytes, otherwise the admission webhook would deny the request")
	}
	return dnsName
}

// isNetworkRestricted returns true if the cluster cannot be directly accessed from external, or is unable to access the public Internet from within the cluster
func isNetworkRestricted(oc *exutil.CLI) bool {
	if os.Getenv("HTTP_PROXY") != "" || os.Getenv("HTTPS_PROXY") != "" || os.Getenv("http_proxy") != "" || os.Getenv("https_proxy") != "" {
		e2e.Logf("Cluster cannot be directly accessed from external (Behind proxy)")
		return true
	}

	dnsPublicZone, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("dns", "cluster", "-n", "openshift-dns", "-o=jsonpath={.spec.publicZone}").Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
	if dnsPublicZone == "" {
		e2e.Logf("Cluster cannot be directly accessed from external (No public DNS record)")
		return true
	}

	azureCloudName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.azure.cloudName}").Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
	if azureCloudName == "AzureStackCloud" {
		e2e.Logf("Cluster cannot be directly accessed from external (AzureStack env)")
		return true
	}

	workNode, err := exutil.GetFirstWorkerNode(oc)
	o.Expect(err).ShouldNot(o.HaveOccurred())
	output, err := exutil.DebugNode(oc, workNode, "bash", "-c", "curl -I letsencrypt.org --connect-timeout 5")
	if !strings.Contains(output, "HTTP") || err != nil {
		e2e.Logf("Unable to access the public Internet from within the cluster")
		return true
	}
	return false
}

// createFBC creates the dedicated file-based catalog for cert-manager operator to subscribe
func createFBC(oc *exutil.CLI, name, namespace, image string) {
	exutil.SkipNoOLMCore(oc)
	if exutil.IsHypershiftHostedCluster(oc) {
		g.Skip("skip since ImageDigestMirrorSet resource cannot be modified in the Hypershift guest (hosted cluster)")
	}
	buildPruningBaseDir := exutil.FixturePath("testdata", "oap/certmanager")
	fbcTemplate := filepath.Join(buildPruningBaseDir, "konflux-fbc.yaml")
	params := []string{"-f", fbcTemplate, "-p", "NAME=" + name, "NAMESPACE=" + namespace, "IMAGE_INDEX=" + image}
	exutil.ApplyClusterResourceFromTemplate(oc, params...)
	// wait for catalogsource lastObservedState to become READY
	err := wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 120*time.Second, false, func(context.Context) (bool, error) {
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("catalogsource", name, "-n", namespace, "-o=jsonpath={.status.connectionState.lastObservedState}").Output()
		if strings.Contains(output, "READY") {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		dumpResource(oc, namespace, "catalogsource", name, "-o=jsonpath={.status}")
		oc.AsAdmin().WithoutNamespace().Run("get").Args("event", "-n", namespace).Execute()
		oc.AsAdmin().WithoutNamespace().Run("delete").Args("catalogsource", name, "-n", namespace).Execute()
	}
	exutil.AssertWaitPollNoErr(err, "timeout waiting for catalogsource state to become READY")
}

// Get installed cert-manager Operator version. The return value format is semantic 'x.y.z'.
func getCertManagerOperatorVersion(oc *exutil.CLI) string {
	version, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", subscriptionName, "-n", operatorNamespace, "-o=jsonpath={.status.installedCSV}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(version).NotTo(o.BeEmpty())
	return strings.TrimPrefix(version, "cert-manager-operator.v")
}

// Skip case if current version is below the minimum version requirement
func skipUnsupportedVersion(oc *exutil.CLI, version string) {
	currentVersion, _ := semver.Parse(getCertManagerOperatorVersion(oc))
	minSupportedVersion, _ := semver.Parse(version)

	// semverA.Compare(semverB) == -1 means 'semverA' < 'semverB', see: https://pkg.go.dev/github.com/blang/semver#Version.Compare
	if currentVersion.Compare(minSupportedVersion) == -1 {
		e2e.Logf("currentVersion=%s , minSupportedVersion=%s", currentVersion, minSupportedVersion)
		g.Skip("Skipping the test case since the operator's current version is below the minimum version required")
	}
}

// rapidastScan performs the RapiDAST scan based on the provided configuration file
// config examples: https://github.com/RedHatProductSecurity/rapidast/tree/development/config
func rapidastScan(oc *exutil.CLI, ns, configFile string) {
	var (
		serviceAccountName = "rapidast-privileged-sa"
		configMapName      = "rapidast-configmap"
		pvcName            = "rapidast-pvc"
		podName            = "rapidast-pod"
	)

	buildPruningBaseDir := exutil.FixturePath("testdata", "oap/certmanager")
	rbacTemplate := filepath.Join(buildPruningBaseDir, "rapidast-privileged-sa.yaml")
	podTemplate := filepath.Join(buildPruningBaseDir, "rapidast-pod.yaml")

	// explicitly skip non-amd64 arch since RapiDAST image only supports amd64
	architecture.SkipNonAmd64SingleArch(oc)

	e2e.Logf("=> configure the authentication token for RapiDAST scan")
	params := []string{"-f", rbacTemplate, "-p", "NAME=" + serviceAccountName, "NAMESPACE=" + ns}
	exutil.ApplyNsResourceFromTemplate(oc, ns, params...)
	defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", ns, "sa", serviceAccountName).Execute()
	configFileContent, err := os.ReadFile(configFile)
	o.Expect(err).NotTo(o.HaveOccurred())
	token, err := getSAToken(oc, serviceAccountName, ns)
	o.Expect(err).NotTo(o.HaveOccurred())
	configFileContentNew := strings.ReplaceAll(string(configFileContent), "AUTH_TOKEN", token)
	err = os.WriteFile(configFile, []byte(configFileContentNew), 0644)
	o.Expect(err).NotTo(o.HaveOccurred())

	e2e.Logf("=> store the RapiDAST config and policy file into a ConfigMap")
	err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", ns, "configmap", configMapName, "--from-file=rapidastconfig.yaml="+configFile).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", ns, "configmap", configMapName).Execute()

	e2e.Logf("=> set privileged labels for RapiDAST namespace")
	err = exutil.SetNamespacePrivileged(oc, oc.Namespace())
	o.Expect(err).NotTo(o.HaveOccurred())
	defer exutil.RecoverNamespaceRestricted(oc, ns)

	e2e.Logf("=> create a Pod to deploy RapiDAST image and perform scan")
	params = []string{"-f", podTemplate, "-p", "POD_NAME=" + podName, "SA_NAME=" + serviceAccountName, "CONFIGMAP_NAME=" + configMapName, "PVC_NAME=" + pvcName}
	exutil.ApplyNsResourceFromTemplate(oc, ns, params...)
	defer func() {
		oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", ns, "pod", podName).Execute()
		oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", ns, "pvc", pvcName).Execute()
	}()

	// wait for the RapiDAST Pod completed
	waitErr := wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 5*time.Minute, false, func(context.Context) (bool, error) {
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ns, "pod", podName).Output()
		if strings.Contains(output, "Completed") {
			e2e.Logf("RapiDAST Pod completed successfully")
			return true, nil
		}
		return false, nil
	})
	podLogs, err := exutil.GetSpecificPodLogs(oc, ns, "", podName, "")
	o.Expect(err).NotTo(o.HaveOccurred())
	exutil.AssertWaitPollNoErr(waitErr, "timeout after 5 minutes waiting for RapiDAST Pod to become Completed")

	e2e.Logf("=> scan 'High' and 'Medium' risk alerts in RapiDAST Pod logs")
	riskHigh, riskMedium := getRapidastRiskNumberFromLogs(podLogs)
	e2e.Logf("RapiDAST scan summary: [High risk alerts=%v] [Medium risk alerts=%v]", riskHigh, riskMedium)

	e2e.Logf("=> sync RapiDAST result artifacts from PVC to local directory")
	syncRapidastResultsToArtifactDir(oc, ns, pvcName)

	if riskHigh > 0 || riskMedium > 0 {
		e2e.Failf("High/Medium risk alerts found! Please check the report and contact ProdSec Team if necessary!")
	}
}

// getRapidastRiskNumberFromLogs returns RapiDAST High and Medium risk number in the given logs
func getRapidastRiskNumberFromLogs(podLogs string) (riskHigh, riskMedium int) {
	podLogLines := strings.Split(podLogs, "\n")
	riskHigh = 0
	riskMedium = 0

	riskHighPattern := regexp.MustCompile(`"riskdesc": .*High`)
	riskMediumPattern := regexp.MustCompile(`"riskdesc": .*Medium`)

	for _, line := range podLogLines {
		if riskHighPattern.MatchString(line) {
			riskHigh++
		}
		if riskMediumPattern.MatchString(line) {
			riskMedium++
		}
	}
	return riskHigh, riskMedium
}

// syncRapidastResultsToArtifactDir copies RapiDAST generated results directory from the given PVC to ArtifactDir
func syncRapidastResultsToArtifactDir(oc *exutil.CLI, ns, pvcName string) {
	var (
		podName                  = "rapidast-results-sync-helper"
		volumeMountPath          = "/opt/rapidast/results"
		artifactDirSubFolderName = "rapidast-results-oap"
	)

	e2e.Logf("create the temporary Pod to mount RapiDAST PVC")
	buildPruningBaseDir := exutil.FixturePath("testdata", "oap/certmanager")
	resultsSyncHelperTemplate := filepath.Join(buildPruningBaseDir, "rapidast-results-sync-helper.yaml")
	params := []string{"-f", resultsSyncHelperTemplate, "-p", "POD_NAME=" + podName, "VOLUME_MOUNT_PATH=" + volumeMountPath, "PVC_NAME=" + pvcName}
	exutil.ApplyNsResourceFromTemplate(oc, ns, params...)
	defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", ns, "pod", podName).Execute()
	exutil.AssertPodToBeReady(oc, podName, ns)

	e2e.Logf("copy generated results directory from mounted PVC to local ARTIFACT_DIR")
	artifactDirPath := exutil.ArtifactDirPath()
	resultsDirPath := filepath.Join(artifactDirPath, artifactDirSubFolderName)
	err := os.MkdirAll(resultsDirPath, os.ModePerm)
	o.Expect(err).NotTo(o.HaveOccurred())
	err = oc.AsAdmin().WithoutNamespace().Run("cp").Args(ns+"/"+podName+":"+volumeMountPath, resultsDirPath, "--retries=5").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("RapiDAST results report can be found in: %s", resultsDirPath)
}

// Poll the pods in the given namespace with specific label, and check if all are redeployed from the oldPodList within the duration
func waitForPodsToBeRedeployed(oc *exutil.CLI, namespace, label string, oldPodList []string, interval, timeout time.Duration) {
	e2e.Logf("Poll the pods with label '%s' in namespace '%s'", label, namespace)
	statusErr := wait.PollUntilContextTimeout(context.Background(), interval, timeout, false, func(context.Context) (bool, error) {
		newPodList, err := exutil.GetAllPodsWithLabel(oc, namespace, label)
		if err != nil {
			e2e.Logf("Error to get pods: %v", err)
			return false, nil
		}

		// Check if each pod in "oldPodList" is not contained in the "newPodList"
		// To avoid nested range loop, convert the slice (newPodList) to a plain string (newPodListString)
		newPodListString := strings.Join(newPodList, ",")
		for _, item := range oldPodList {
			if strings.Contains(newPodListString, item) {
				return false, nil
			}
		}
		e2e.Logf("All pods are redeployed successfully: %v", newPodList)
		return true, nil
	})
	exutil.AssertWaitPollNoErr(statusErr, fmt.Sprintf("timed out after %v waiting all pods to be redeployed", timeout))
}

// setupVaultServer setups a containerized Vault server in the given namespace, returns server pod name and root token
// 'release' is the name of the Vault instance that is going to be installed
// 'helmValueFilePath' is the path of the custom Helm values file
// 'enableTLS' is a boolean to indicate enable TLS traffic nor not
func setupVaultServer(oc *exutil.CLI, ns, release, helmValueFilePath string, enableTLS bool) (string, string) {
	var (
		configMapName        = "helm-vault-tls-config"
		installerSA          = "vault-installer-sa"
		installerRolebinding = "vault-installer-binding-" + ns
		installerPodName     = "vault-installer"
		vaultPodLabel        = "app.kubernetes.io/name=vault"
	)

	// explicitly skip since image 'hashicorp/vault' doesn't support ppc64le and s390x arches
	architecture.SkipArchitectures(oc, architecture.PPC64LE, architecture.S390X)

	// explicitly skip not applicable storage classes for Vault statefulset
	// 1. The Vault server requires Unix commands like 'chmod' to initialize operator, but it's not supported natively by Azure Files SMB protocol.
	//    xref: https://learn.microsoft.com/en-us/troubleshoot/azure/azure-kubernetes/storage/could-not-change-permissions-azure-files
	// 2. The Vault requests 1G PVC for data storage, but the 'Hyperdisk' type has a minimum disk size requirement of 4G which is expensive and unnecessary for our scenarios.
	//    xref: https://cloud.google.com/compute/docs/disks/hyperdisks#limits-disk
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("storageclass", `-o=jsonpath={.items[?(@.metadata.annotations.storageclass\.kubernetes\.io/is-default-class=="true")].metadata.name}`).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if strings.Contains(output, "azurefile-csi") || strings.Contains(output, "hyperdisk") || len(output) == 0 {
		g.Skip("Skipping as the default storage class is not applicable for the vault server to consume.")
	}

	if enableTLS {
		e2e.Logf("=> perpare TLS certs to secure HTTPS traffic of Vault server")
		createCertificateForVaultServer(oc, ns, release)
	}

	e2e.Logf("=> create a pod to install Vault through Helm charts")
	// set privileged labels for the Vault installer namespace as containerized Helm requires root permission to write config files
	exutil.SetNamespacePrivileged(oc, ns)
	defer func() {
		e2e.Logf("remove added privileged labels from the namespace")
		exutil.RecoverNamespaceRestricted(oc, ns)
	}()
	// store the Helm values config into a configmap
	err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", ns, "configmap", configMapName, "--from-file=custom-values.yaml="+helmValueFilePath).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	// install the Vault with custom Helm values
	cmd := fmt.Sprintf(`helm install %s ./vault -n %s --values /helm/custom-values.yaml`, release, ns)
	buildPruningBaseDir := exutil.FixturePath("testdata", "oap/certmanager")
	helmHelperFile := filepath.Join(buildPruningBaseDir, "exec-helm-helper.yaml")
	params := []string{"-f", helmHelperFile, "-p", "SA_NAME=" + installerSA, "ROLEBINDING_NAME=" + installerRolebinding, "POD_NAME=" + installerPodName, "HELM_CMD=" + cmd, "CONFIGMAP_NAME=" + configMapName, "NAMESPACE=" + ns}
	exutil.ApplyClusterResourceFromTemplate(oc, params...)
	defer func() {
		e2e.Logf("cleanup created clusterrolebinding resource")
		oc.AsAdmin().WithoutNamespace().Run("delete").Args("clusterrolebinding", installerRolebinding).Execute()
	}()
	// wait for Vault installer pod completed
	err = wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 180*time.Second, false, func(context.Context) (bool, error) {
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ns, "pod", installerPodName).Output()
		if strings.Contains(output, "Completed") {
			e2e.Logf("Vault installer pod completed successfully")
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		e2e.Logf("Dumping Vault installer pod...")
		oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ns, "pod", installerPodName, "-o=jsonpath={.status}").Execute()
		oc.AsAdmin().WithoutNamespace().Run("logs").Args("-n", ns, installerPodName, "--tail=10").Execute()
	}
	exutil.AssertWaitPollNoErr(err, "timeout waiting for Vault installer pod completed")
	// wait for Vault server pod to show up
	err = wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 180*time.Second, false, func(context.Context) (bool, error) {
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ns, "pod", "-l", vaultPodLabel).Output()
		if strings.Contains(output, "Running") && strings.Contains(output, "0/1") {
			e2e.Logf("Vault server pod is up and running (waiting for unseal to become ready)")
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "timeout waiting for Vault server pod to show up")

	e2e.Logf("=> retrieve vault server pod name")
	vaultPodName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ns, "pod", "-l", vaultPodLabel, "-o=jsonpath={.items[0].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())

	e2e.Logf("=> init and unseal Vault")
	// init Vault with one key share and one key threshold
	cmd = `vault operator init -key-shares=1 -key-threshold=1 -format=json`
	output, err = exutil.RemoteShPod(oc, ns, vaultPodName, "sh", "-c", cmd)
	o.Expect(err).NotTo(o.HaveOccurred())
	vaultUnsealKey := gjson.Get(output, "unseal_keys_b64.0").String()
	vaultRootToken := gjson.Get(output, "root_token").String()
	// unseal Vault with the VAULT_UNSEAL_KEY
	cmd = fmt.Sprintf(`vault operator unseal -format=json %s`, vaultUnsealKey)
	oc.NotShowInfo()
	output, err = exutil.RemoteShPod(oc, ns, vaultPodName, "sh", "-c", cmd)
	oc.SetShowInfo()
	o.Expect(err).NotTo(o.HaveOccurred())
	version := gjson.Get(output, "version").String()

	// wait for Vault server pod to become ready
	err = wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 120*time.Second, false, func(context.Context) (bool, error) {
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ns, "pod", vaultPodName).Output()
		if strings.Contains(output, "Running") && strings.Contains(output, "1/1") {
			e2e.Logf("Vault server pod is ready")
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "timeout waiting for Vault server pod to become ready")
	e2e.Logf("Vault server setup successfully! (version %s)", version)
	return vaultPodName, vaultRootToken
}

// createCertificateForVaultServer creates TLS certificates for Vault server using cert-manager
func createCertificateForVaultServer(oc *exutil.CLI, ns, release string) {
	var (
		issuerName = "default-ca"
		certName   = "vault-server-cert"
	)

	createIssuer(oc, ns)
	createCertificate(oc, ns)

	e2e.Logf("create a CA issuer")
	buildPruningBaseDir := exutil.FixturePath("testdata", "oap/certmanager")
	issuerFile := filepath.Join(buildPruningBaseDir, "issuer-ca.yaml")
	err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", ns, "-f", issuerFile).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	err = waitForResourceReadiness(oc, ns, "issuer", issuerName, 10*time.Second, 120*time.Second)
	if err != nil {
		dumpResource(oc, ns, "issuer", issuerName, "-o=yaml")
	}
	exutil.AssertWaitPollNoErr(err, "timeout waiting for issuer to become Ready")

	e2e.Logf("create a certificate using the CA issuer")
	certFile := filepath.Join(buildPruningBaseDir, "cert-selfsigned-vault.yaml")
	params := []string{"-f", certFile, "-p", "CERT_NAME=" + certName, "VAULT_SERVICE=" + release, "VAULT_NAMESPACE=" + ns, "ISSUER_NAME=" + issuerName}
	exutil.ApplyNsResourceFromTemplate(oc, ns, params...)
	err = waitForResourceReadiness(oc, ns, "certificate", certName, 10*time.Second, 300*time.Second)
	if err != nil {
		dumpResource(oc, ns, "certificate", certName, "-o=yaml")
	}
	exutil.AssertWaitPollNoErr(err, "timeout waiting for certificate to become Ready")
}

// configVaultPKI configs Vault server to enable PKI secrets engine
func configVaultPKI(oc *exutil.CLI, ns, release, vaultPodName, vaultRootToken string) {
	e2e.Logf("=> configure Vault as a PKI secrets engine")
	// login to Vault with the VAULT_ROOT_TOKEN
	cmd := fmt.Sprintf(`vault login %s`, vaultRootToken)
	oc.NotShowInfo()
	_, err := exutil.RemoteShPod(oc, ns, vaultPodName, "sh", "-c", cmd)
	oc.SetShowInfo()
	o.Expect(err).NotTo(o.HaveOccurred())
	// enable the PKI secrets engine and create a root CA
	cmd = `vault secrets enable pki && vault secrets tune -max-lease-ttl=8760h pki && vault write -field=certificate pki/root/generate/internal common_name="cert-manager-issuer-root" issuer_name="vault-root" ttl=8760h`
	_, err = exutil.RemoteShPod(oc, ns, vaultPodName, "sh", "-c", cmd)
	o.Expect(err).NotTo(o.HaveOccurred())
	// configure the CRL and CA endpoints
	cmd = fmt.Sprintf(`vault write pki/config/urls issuing_certificates="https://%s.%s:8200/v1/pki/ca" crl_distribution_points="https://%s.%s:8200/v1/pki/crl"`, release, ns, release, ns)
	_, err = exutil.RemoteShPod(oc, ns, vaultPodName, "sh", "-c", cmd)
	o.Expect(err).NotTo(o.HaveOccurred())
	// create an intermediate CA
	cmd = `vault secrets enable -path=pki_int pki && vault secrets tune -max-lease-ttl=2160h pki_int`
	_, err = exutil.RemoteShPod(oc, ns, vaultPodName, "sh", "-c", cmd)
	o.Expect(err).NotTo(o.HaveOccurred())
	cmd = `vault write -format=json pki_int/intermediate/generate/internal common_name="cert-manager-issuer-int" issuer_name="vault-int" ttl=2160h`
	output, err := exutil.RemoteShPod(oc, ns, vaultPodName, "sh", "-c", cmd)
	o.Expect(err).NotTo(o.HaveOccurred())
	pkiIntermediateCSR := gjson.Get(output, "data.csr").String()
	cmd = fmt.Sprintf(`echo "%s" > /tmp/pki_intermediate.csr`, pkiIntermediateCSR)
	oc.NotShowInfo()
	_, err = exutil.RemoteShPod(oc, ns, vaultPodName, "sh", "-c", cmd)
	oc.SetShowInfo()
	o.Expect(err).NotTo(o.HaveOccurred())
	// sign the intermediate certificate with the root CA private key
	cmd = `vault write -format=json pki/root/sign-intermediate issuer_ref="vault-root" csr=@/tmp/pki_intermediate.csr format=pem_bundle ttl=2160h`
	output, err = exutil.RemoteShPod(oc, ns, vaultPodName, "sh", "-c", cmd)
	o.Expect(err).NotTo(o.HaveOccurred())
	intCertPemData := gjson.Get(output, "data.certificate").String()
	cmd = fmt.Sprintf(`echo "%s" > /tmp/intermediate.cert.pem`, intCertPemData)
	oc.NotShowInfo()
	_, err = exutil.RemoteShPod(oc, ns, vaultPodName, "sh", "-c", cmd)
	oc.SetShowInfo()
	o.Expect(err).NotTo(o.HaveOccurred())
	cmd = `vault write pki_int/intermediate/set-signed certificate=@/tmp/intermediate.cert.pem`
	_, err = exutil.RemoteShPod(oc, ns, vaultPodName, "sh", "-c", cmd)
	o.Expect(err).NotTo(o.HaveOccurred())
	// create a role that enables to create certificates with any commonName or dnsNames for test simplicity
	cmd = `vault write pki_int/roles/cluster-dot-local require_cn=false allow_any_name=true max_ttl=720h`
	_, err = exutil.RemoteShPod(oc, ns, vaultPodName, "sh", "-c", cmd)
	o.Expect(err).NotTo(o.HaveOccurred())
	// create a policy that enables needed permission to the PKI secrets engine paths
	cmd = `vault policy write cert-manager - <<EOF
path "pki_int/sign/cluster-dot-local"    { capabilities = ["update"] }
EOF`
	_, err = exutil.RemoteShPod(oc, ns, vaultPodName, "sh", "-c", cmd)
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("Vault PKI engine configured successfully!")
}

// installGoogleCASIssuer installs the Google CAS Issuer into cert-manager namespace
func installGoogleCASIssuer(oc *exutil.CLI, ns string) {
	var (
		configMapName        = "dummy-cm"
		installerSA          = "cas-issuer-installer-sa"
		installerRolebinding = "cas-issuer-installer-binding"
		installerPodName     = "cas-issuer-installer"
		casPodLabel          = "app=cert-manager-google-cas-issuer"
		casNamespace         = "cert-manager"
	)

	e2e.Logf("=> create a pod to install Google CAS Issuer through Helm charts")
	// create a dummy configmap as a placeholder to reuse 'exec-helm-helper.yaml' template
	buildPruningBaseDir := exutil.FixturePath("testdata", "oap/certmanager")
	err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", ns, "configmap", configMapName).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	// install the external issuer through Helm charts
	cmd := fmt.Sprintf(`helm install cas ./cert-manager-google-cas-issuer -n %s`, casNamespace)
	helmHelperFile := filepath.Join(buildPruningBaseDir, "exec-helm-helper.yaml")
	params := []string{"-f", helmHelperFile, "-p", "SA_NAME=" + installerSA, "ROLEBINDING_NAME=" + installerRolebinding, "POD_NAME=" + installerPodName, "HELM_CMD=" + cmd, "CONFIGMAP_NAME=" + configMapName, "NAMESPACE=" + ns}
	exutil.ApplyClusterResourceFromTemplate(oc, params...)
	defer func() {
		e2e.Logf("cleanup created clusterrolebinding resource")
		oc.AsAdmin().WithoutNamespace().Run("delete").Args("clusterrolebinding", installerRolebinding).Execute()
	}()
	// wait for Helm installer pod completed
	err = wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 180*time.Second, false, func(context.Context) (bool, error) {
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ns, "pod", installerPodName).Output()
		if strings.Contains(output, "Completed") {
			e2e.Logf("Helm installer pod completed successfully")
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		e2e.Logf("Dumping Helm installer pod...")
		oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ns, "pod", installerPodName, "-o=jsonpath={.status}").Execute()
		oc.AsAdmin().WithoutNamespace().Run("logs").Args("-n", ns, installerPodName, "--tail=10").Execute()
	}
	exutil.AssertWaitPollNoErr(err, "timeout waiting for Helm installer pod completed")
	// wait for CAS Issuer controller pod to be up and running
	err = wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 180*time.Second, false, func(context.Context) (bool, error) {
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", casNamespace, "pod", "-l", casPodLabel).Output()
		if strings.Contains(output, "Running") {
			e2e.Logf("CAS Issuer controller pod is up and running")
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "timeout waiting for CAS Issuer controller pod to be up and running")
}

// setupPebbleServer setups a containerized Pebble ACME server in the given namespace
func setupPebbleServer(oc *exutil.CLI, ns string) string {
	var (
		deploymentName = "pebble"
	)

	// explicitly skip since image 'letsencrypt/pebble' doesn't support ppc64le and s390x arches
	architecture.SkipArchitectures(oc, architecture.PPC64LE, architecture.S390X, architecture.MULTI)

	e2e.Logf("create a deployment and expose service for Pebble")
	buildPruningBaseDir := exutil.FixturePath("testdata", "oap/certmanager")
	issuerFile := filepath.Join(buildPruningBaseDir, "deploy-pebble-server.yaml")
	err := oc.AsAdmin().WithoutNamespace().Run("apply").Args("-n", ns, "-f", issuerFile).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	err = wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 120*time.Second, false, func(context.Context) (bool, error) {
		if isDeploymentReady(oc, ns, deploymentName) {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "timeout waiting for Pebble server deployment to become ready")

	endpoint := fmt.Sprintf("https://%s.%s.svc.cluster.local:14000/dir", deploymentName, ns)
	e2e.Logf("Pebble server setup successfully! (endpoint %s)", endpoint)
	return endpoint
}
