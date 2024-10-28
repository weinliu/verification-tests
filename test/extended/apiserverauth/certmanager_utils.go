package apiserverauth

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
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

// Get the cloud provider type of the test environment
func getCloudProvider(oc *exutil.CLI) string {
	var (
		errMsg error
		output string
	)
	err := wait.Poll(5*time.Second, 30*time.Second, func() (bool, error) {
		output, errMsg = oc.WithoutNamespace().AsAdmin().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.type}").Output()
		if errMsg != nil {
			e2e.Logf("Get cloudProvider *failed with* :\"%v\",wait 5 seconds retry.", errMsg)
			return false, errMsg
		}
		e2e.Logf("The test cluster cloudProvider is :\"%s\".", strings.ToLower(output))
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, "Waiting for get cloudProvider timeout")
	return strings.ToLower(output)
}

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
	policy, err := crmService.Projects.GetIamPolicy(resource, &gcpcrm.GetIamPolicyRequest{}).Do()
	o.Expect(err).NotTo(o.HaveOccurred())

	if add {
		policy.Bindings = append(policy.Bindings, &gcpcrm.Binding{
			Role:    role,
			Members: []string{member},
		})
	} else {
		removeMember(policy, role, member)
	}
	_, err = crmService.Projects.SetIamPolicy(resource, &gcpcrm.SetIamPolicyRequest{Policy: policy}).Do()
	o.Expect(err).NotTo(o.HaveOccurred())
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
		e2e.Logf("Role %v not found. Member not removed.", role)
		return
	}
	if memberIndex == -1 {
		e2e.Logf("Role %v found. Member not found.", role)
		return
	}

	members := removeIdx(bindings[bindingIndex].Members, memberIndex)
	bindings[bindingIndex].Members = members
	if len(members) == 0 {
		bindings = removeIdx(bindings, bindingIndex)
		policy.Bindings = bindings
	}
	e2e.Logf("Role '%v' found. Member '%v' removed.", role, member)
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

// Get cluster's DNS Public Zone. Returning "" generally means cluster is private, except azure-stack env
func getDNSPublicZone(oc *exutil.CLI) string {
	publicZone, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("dns", "cluster", "-n", "openshift-dns", "-o=jsonpath={.spec.publicZone}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return publicZone
}

// Get AzureCloud name. Returning "" means non-Azure env, "AzurePublicCloud" means public Azure env; "AzureStackCloud" means AzureStack env
func getAzureCloudName(oc *exutil.CLI) string {
	azureCloudName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.azure.cloudName}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return azureCloudName
}

// create cert manager
func createCertManagerOperator(oc *exutil.CLI) {
	const (
		subscriptionName       = "openshift-cert-manager-operator"
		subscriptionNamespace  = "cert-manager-operator"
		catalogSourceNamespace = "openshift-marketplace"
		channelName            = "stable-v1"
		operandNamespace       = "cert-manager"
		operandPodLabel        = "app.kubernetes.io/instance=cert-manager"
		operandPodNum          = 3
	)

	// switch to an available catalogsource
	catalogSourceName, err := getAvailableCatalogSourceName(oc, catalogSourceNamespace, subscriptionName)
	if len(catalogSourceName) == 0 || err != nil {
		g.Skip("skip since no available catalogsource was found")
	}
	e2e.Logf("=> using catalogsource: '%s'", catalogSourceName)

	e2e.Logf("=> create the operator namespace")
	buildPruningBaseDir := exutil.FixturePath("testdata", "apiserverauth/certmanager")
	namespaceFile := filepath.Join(buildPruningBaseDir, "namespace.yaml")
	output, err := oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", namespaceFile).Output()
	// skip the install process to mitigate the namespace deletion terminating issue caused by case 62006
	// the full message is 'Detected changes to resource cert-manager-operator which is currently being deleted'
	if strings.Contains(output, "being deleted") {
		g.Skip("skip the install process as the cert-manager-operator namespace is being terminated due to other env issue e.g. we ever hit such failures caused by OCPBUGS-31443")
	}
	o.Expect(err).NotTo(o.HaveOccurred())

	e2e.Logf("=> create the operatorgroup")
	operatorGroupFile := filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
	err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", operatorGroupFile).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())

	e2e.Logf("=> create the subscription")
	subscriptionTemplate := filepath.Join(buildPruningBaseDir, "subscription.yaml")
	params := []string{"-f", subscriptionTemplate, "-p", "NAME=" + subscriptionName, "SOURCE=" + catalogSourceName, "SOURCE_NAMESPACE=" + catalogSourceNamespace, "CHANNEL=" + channelName}
	exutil.ApplyNsResourceFromTemplate(oc, subscriptionNamespace, params...)
	// wait for subscription state to become AtLatestKnown
	err = wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 180*time.Second, true, func(context.Context) (bool, error) {
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", subscriptionName, "-n", subscriptionNamespace, "-o=jsonpath={.status.state}").Output()
		if strings.Contains(output, "AtLatestKnown") {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		dumpResource(oc, subscriptionNamespace, "sub", subscriptionName, "-o=jsonpath={.status}")
	}
	exutil.AssertWaitPollNoErr(err, "timeout waiting for subscription state to become AtLatestKnown")

	e2e.Logf("=> retrieve the installed CSV name")
	csvName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", subscriptionName, "-n", subscriptionNamespace, "-o=jsonpath={.status.installedCSV}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(csvName).NotTo(o.BeEmpty())
	// wait for csv phase to become Succeeded
	err = wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 180*time.Second, true, func(context.Context) (bool, error) {
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", csvName, "-n", subscriptionNamespace, "-o=jsonpath={.status.phase}").Output()
		if strings.Contains(output, "Succeeded") {
			e2e.Logf("csv '%s' installed successfully", csvName)
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		dumpResource(oc, subscriptionNamespace, "csv", csvName, "-o=jsonpath={.status}")
	}
	exutil.AssertWaitPollNoErr(err, "timeout waiting for csv phase to become Succeeded")

	e2e.Logf("=> checking the cert-manager operand pods readiness")
	err = wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 300*time.Second, true, func(context.Context) (bool, error) {
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", operandNamespace, "-l", operandPodLabel, "--field-selector=status.phase=Running", "-o=jsonpath={.items[*].metadata.name}").Output()
		if len(strings.Fields(output)) == operandPodNum {
			e2e.Logf("all operand pods are up and running!")
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", operandNamespace, "-l", operandPodLabel).Execute()
	}
	exutil.AssertWaitPollNoErr(err, "timeout waiting for all operand pods phase to become Running")
}

// create selfsigned issuer
func createIssuer(oc *exutil.CLI, ns string) {
	e2e.Logf("create a selfsigned issuer")
	buildPruningBaseDir := exutil.FixturePath("testdata", "apiserverauth/certmanager")
	issuerFile := filepath.Join(buildPruningBaseDir, "issuer-selfsigned.yaml")
	err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", ns, "-f", issuerFile).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	err = waitForResourceReadiness(oc, ns, "issuer", "default-selfsigned", 10*time.Second, 120*time.Second)
	if err != nil {
		dumpResource(oc, ns, "issuer", "default-selfsigned", "-o=yaml")
	}
	exutil.AssertWaitPollNoErr(err, "timeout waiting for issuer to become Ready")
}

// create certificate using selfsigned issuer
func createCertificate(oc *exutil.CLI, ns string) {
	e2e.Logf("create a certificate using the selfsigned issuer")
	buildPruningBaseDir := exutil.FixturePath("testdata", "apiserverauth/certmanager")
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
		if parsedCert.Subject.CommonName != commonName {
			e2e.Failf("Incorrect subject CN: %v found in issued certificate", parsedCert.Subject.CommonName)
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

// Skip case if HTTP and HTTPS route are unreachable.
func skipIfRouteUnreachable(oc *exutil.CLI) {
	e2e.Logf("Detect if using a private cluster.")
	if os.Getenv("HTTP_PROXY") != "" || os.Getenv("HTTPS_PROXY") != "" || os.Getenv("http_proxy") != "" || os.Getenv("https_proxy") != "" {
		g.Skip("skip private clusters that are behind some proxy")
	} else if getDNSPublicZone(oc) == "" || getAzureCloudName(oc) == "AzureStackCloud" {
		g.Skip("skip private clusters that can't be directly accessed from external")
	}

	var (
		httpReachable  bool
		httpsReachable bool
	)

	e2e.Logf("Get route host from ingress canary.")
	host, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("route", "canary", "-n", "openshift-ingress-canary", "--template", `{{range .status.ingress}}{{if eq .routerName "default"}}{{.host}}{{end}}{{end}}`).Output()
	o.Expect(err).NotTo(o.HaveOccurred())

	e2e.Logf("New the client to detect HTTP, HTTPS connection.")
	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	httpResponse, httpErr := httpClient.Get("http://" + host)
	if httpErr == nil && httpResponse.StatusCode == http.StatusFound { // 302 -> port 80 is opened
		httpReachable = true
		defer httpResponse.Body.Close()
	}
	httpsResponse, httpsErr := httpClient.Get("https://" + host)
	if httpsErr == nil {
		httpsReachable = true
		defer httpsResponse.Body.Close()
	}

	if !httpReachable && !httpsReachable {
		g.Skip("HTTP and HTTPS are both unreachable, skipped.")
	} else if !httpReachable && httpsReachable {
		e2e.Failf("HTTPS reachable but HTTP unreachable. Marking case failed to signal router or installer problem. HTTP response error: %s", httpErr)
	}
}

// Get the available CatalogSource's name for specific subscription from given namespace
func getAvailableCatalogSourceName(oc *exutil.CLI, namespace, subscription string) (string, error) {
	// check if there are any catalogsources that not READY, otherwise it will block the subscription's readiness
	// ref: https://docs.openshift.com/container-platform/4.16/operators/understanding/olm/olm-understanding-olm.html#olm-cs-health_olm-understanding-olm
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", namespace, "catalogsource", `-o=jsonpath={.items[?(@.status.connectionState.lastObservedState!="READY")].metadata.name}`).Output()
	if err != nil {
		return "", fmt.Errorf("failed to get catalogsource from namespace: %s", namespace)
	}
	if len(output) > 0 {
		e2e.Logf("at least one catalogsource is not READY: '%s'", output)
		return "", nil
	}

	list, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", namespace, "catalogsource", `-o=jsonpath={.items[*].metadata.name}`).Output()
	if err != nil {
		return "", fmt.Errorf("failed to get catalogsource from namespace: %s", namespace)
	}

	// the default order is to use "qe-app-registry" first, if it is unavailable then switch to use "redhat-operators"
	targetCatalogSources := []string{"qe-app-registry", "redhat-operators"}
	for _, name := range targetCatalogSources {
		if strings.Contains(list, name) {
			// check if the specific subscription's packagemanifest exists in the catalogsource
			output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("packagemanifest", "-n", namespace, "-l", "catalog="+name, "--field-selector", "metadata.name="+subscription).Output()
			if strings.Contains(output, subscription) && err == nil {
				return name, nil
			}
		}
	}

	// if no target CatalogSource was found, print existing CatalogSource and return ""
	output, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", namespace, "catalogsource").Output()
	e2e.Logf("get existing catalogsource: %s", output)
	return "", nil
}

// Get installed cert-manager Operator version. The return value format is semantic 'x.y.z'.
func getCertManagerOperatorVersion(oc *exutil.CLI) string {
	version, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", "openshift-cert-manager-operator", "-n", "cert-manager-operator", "-o=jsonpath={.status.installedCSV}").Output()
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

// rapidastScan performs RapiDAST scan for apiGroupName using configFile and policyFile
func rapidastScan(oc *exutil.CLI, ns, componentName, apiGroupName, configFile, policyFile string) {
	const (
		serviceAccountName = "rapidast-privileged-sa"
		configMapName      = "rapidast-configmap"
		pvcName            = "rapidast-pvc"
		jobName            = "rapidast-job"
	)

	buildPruningBaseDir := exutil.FixturePath("testdata", "apiserverauth/certmanager")
	rbacTemplate := filepath.Join(buildPruningBaseDir, "rapidast-privileged-sa.yaml")
	jobTemplate := filepath.Join(buildPruningBaseDir, "rapidast-job.yaml")

	// explicitly skip non-amd64 arch since RapiDAST image only supports amd64
	architecture.SkipNonAmd64SingleArch(oc)

	// TODO(yuewu): once RapiDAST new image is released with hot-fix PR (https://github.com/RedHatProductSecurity/rapidast/pull/155), this section might be removed
	e2e.Logf("Set privilege for RapiDAST.")
	err := exutil.SetNamespacePrivileged(oc, oc.Namespace())
	o.Expect(err).NotTo(o.HaveOccurred())
	params := []string{"-f", rbacTemplate, "-p", "NAME=" + serviceAccountName, "NAMESPACE=" + ns}
	exutil.ApplyNsResourceFromTemplate(oc, ns, params...)

	e2e.Logf("Update the AUTH_TOKEN and APP_SHORT_NAME in RapiDAST config file.")
	configFileContent, err := os.ReadFile(configFile)
	o.Expect(err).NotTo(o.HaveOccurred())
	token, err := getSAToken(oc, serviceAccountName, ns)
	o.Expect(err).NotTo(o.HaveOccurred())
	configFileContentNew := strings.ReplaceAll(string(configFileContent), "AUTH_TOKEN", token)
	configFileContentNew = strings.ReplaceAll(configFileContentNew, "APP_SHORT_NAME", apiGroupName)
	err = os.WriteFile(configFile, []byte(configFileContentNew), 0644)
	o.Expect(err).NotTo(o.HaveOccurred())

	e2e.Logf("Create ConfigMap using RapiDAST config and policy file.")
	err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", ns, "configmap", configMapName, "--from-file=rapidastconfig.yaml="+configFile, "--from-file=customscan.policy="+policyFile).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", ns, "configmap", configMapName).Execute()

	e2e.Logf("Create Job to deploy RapiDAST and perform scan.")
	params = []string{"-f", jobTemplate, "-p", "JOB_NAME=" + jobName, "SA_NAME=" + serviceAccountName, "CONFIGMAP_NAME=" + configMapName, "PVC_NAME=" + pvcName, "APP_SHORT_NAME=" + apiGroupName}
	exutil.ApplyNsResourceFromTemplate(oc, ns, params...)
	defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", ns, "job", jobName).Execute()
	waitErr := wait.PollUntilContextTimeout(context.Background(), 20*time.Second, 10*time.Minute, true, func(context.Context) (bool, error) {
		jobStatus, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ns, "job", jobName, `-ojsonpath={.status}`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(jobStatus, "Complete") {
			e2e.Logf("RapiDAST Job completed successfully, status: %s.", jobStatus)
			return true, nil
		}
		return false, nil
	})

	podList, err := exutil.GetAllPodsWithLabel(oc, ns, "name="+jobName)
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(podList).ShouldNot(o.BeEmpty())
	podName := podList[0]
	podLogs, err := exutil.GetSpecificPodLogs(oc, ns, "", podName, "")
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("RapiDAST Job's Pod logs: %v", podLogs)
	exutil.AssertWaitPollNoErr(waitErr, "timeout after 10 minutes waiting for RapiDAST Job completed")

	riskHigh, riskMedium := getRapidastRiskNumberFromLogs(podLogs)
	e2e.Logf("RapiDAST scan summary: [High risk alerts=%v] [Medium risk alerts=%v]", riskHigh, riskMedium)
	syncRapidastResultsToArtifactDir(oc, ns, componentName, pvcName)
	if riskHigh > 0 || riskMedium > 0 {
		e2e.Failf("High/Medium risk alerts found! Please check the report and connect ProdSec Team if necessary!")
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
func syncRapidastResultsToArtifactDir(oc *exutil.CLI, ns, componentName, pvcName string) {
	const (
		resultsPodName           = "results-sync-helper"
		resultsVolumeMountPath   = "/zap/results"
		artifactDirSubFolderName = "rapiddastresultscfe"
	)

	e2e.Logf("Create temporary Pod to mount PVC")
	buildPruningBaseDir := exutil.FixturePath("testdata", "apiserverauth/certmanager")
	resultsSyncHelperTemplate := filepath.Join(buildPruningBaseDir, "rapidast-results-sync-helper.yaml")
	params := []string{"-f", resultsSyncHelperTemplate, "-p", "POD_NAME=" + resultsPodName, "VOLUME_MOUNT_PATH=" + resultsVolumeMountPath, "PVC_NAME=" + pvcName}
	exutil.ApplyNsResourceFromTemplate(oc, ns, params...)
	defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", ns, "pod", resultsPodName).Execute()
	exutil.AssertPodToBeReady(oc, resultsPodName, ns)

	e2e.Logf("Copy generated results directory from mounted PVC to local ArtifactDir")
	artifactDirPath := exutil.ArtifactDirPath()
	resultsDirPath := filepath.Join(artifactDirPath, artifactDirSubFolderName, componentName)
	err := os.MkdirAll(resultsDirPath, os.ModePerm)
	o.Expect(err).NotTo(o.HaveOccurred())
	err = oc.AsAdmin().WithoutNamespace().Run("cp").Args("-n", ns, resultsPodName+":"+resultsVolumeMountPath, resultsDirPath).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("RapiDAST results report can be found in: %s", resultsDirPath)
}

// Poll the pods in the given namespace with specific label, and check if all are redeployed from the oldPodList within the duration
func waitForPodsToBeRedeployed(oc *exutil.CLI, namespace, label string, oldPodList []string, interval, timeout time.Duration) {
	e2e.Logf("Poll the pods with label '%s' in namespace '%s'", label, namespace)
	statusErr := wait.Poll(interval, timeout, func() (bool, error) {
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

// setupVaultServer setups a single pod in-cluster Vault server and enable PKI secrets engine, returns server pod name and root token
func setupVaultServer(oc *exutil.CLI, ns string, release string) (string, string) {
	var (
		issuerName           = "default-ca"
		certName             = "vault-server-cert"
		configMapName        = "helm-vault-tls-config"
		installerSA          = "vault-installer-sa"
		installerRolebinding = "vault-installer-binding-" + ns
		installerPodName     = "vault-installer"
		vaultPodLabel        = "app.kubernetes.io/name=vault"
		httpProxy            = ""
		httpsProxy           = ""
		noProxy              = ""
	)

	// The Vault server requires Unix commands like 'chmod' to initialize operator, but it's not supported natively by Azure Files SMB protocol.
	// xref: https://learn.microsoft.com/en-us/troubleshoot/azure/azure-kubernetes/storage/could-not-change-permissions-azure-files
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("storageclass", `-o=jsonpath={.items[?(@.metadata.annotations.storageclass\.kubernetes\.io/is-default-class=="true")].metadata.name}`).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if strings.Contains(output, "azurefile-csi") || len(output) == 0 {
		g.Skip("Skipping as the default storage class is not applicable for the vault server to consume.")
	}

	e2e.Logf("=> perpare TLS certs to secure HTTPS traffic of Vault server")
	createIssuer(oc, ns)
	createCertificate(oc, ns)

	e2e.Logf("create a CA issuer")
	buildPruningBaseDir := exutil.FixturePath("testdata", "apiserverauth/certmanager")
	issuerFile := filepath.Join(buildPruningBaseDir, "issuer-ca.yaml")
	err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", ns, "-f", issuerFile).Execute()
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

	// set proxy envs for the Vault installer pod to access the Helm chart repository when running on a proxy-enabled cluster
	output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("proxy", "cluster", "-o=jsonpath={.status}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if strings.Contains(output, "httpProxy") {
		e2e.Logf("=> retrieve the proxy configurations for Vault installer pod to inject")
		httpProxy = gjson.Get(output, "httpProxy").String()
		httpsProxy = gjson.Get(output, "httpsProxy").String()
		noProxy = gjson.Get(output, "noProxy").String()
	}

	e2e.Logf("=> create a pod to install Vault through Helm charts")
	// store the Helm values regarding Vault TLS config into a configmap
	helmConfigFile := filepath.Join(buildPruningBaseDir, "helm-vault-tls-config.yaml")
	err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", ns, "configmap", configMapName, "--from-file=custom-values.yaml="+helmConfigFile).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	// install a standalone TLS-enabled Vault with agent injector service disabled
	cmd := fmt.Sprintf(`helm repo add hashicorp https://helm.releases.hashicorp.com && helm install %s hashicorp/vault -n %s --set injector.enabled=false --set global.openshift=true --values /helm/custom-values.yaml`, release, ns)
	helmHelperFile := filepath.Join(buildPruningBaseDir, "exec-helm-helper.yaml")
	params = []string{"-f", helmHelperFile, "-p", "SA_NAME=" + installerSA, "ROLEBINDING_NAME=" + installerRolebinding, "POD_NAME=" + installerPodName, "HELM_CMD=" + cmd, "CONFIGMAP_NAME=" + configMapName, "NAMESPACE=" + ns, "HTTP_PROXY=" + httpProxy, "HTTPS_PROXY=" + httpsProxy, "NO_PROXY=" + noProxy}
	exutil.ApplyClusterResourceFromTemplate(oc, params...)
	defer func() {
		e2e.Logf("cleanup created clusterrolebinding resource")
		oc.AsAdmin().WithoutNamespace().Run("delete").Args("clusterrolebinding", installerRolebinding).Execute()
	}()
	// wait for Vault installer pod completed
	// TODO(yuewu): need to address potential error caused by Docker's image pull rate limit: https://docs.docker.com/docker-hub/download-rate-limit/
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
	cmd = fmt.Sprintf(`vault operator init -key-shares=1 -key-threshold=1 -format=json`)
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

	e2e.Logf("=> configure Vault as a PKI secrets engine")
	// login to Vault with the VAULT_ROOT_TOKEN
	cmd = fmt.Sprintf(`vault login %s`, vaultRootToken)
	oc.NotShowInfo()
	_, err = exutil.RemoteShPod(oc, ns, vaultPodName, "sh", "-c", cmd)
	oc.SetShowInfo()
	o.Expect(err).NotTo(o.HaveOccurred())
	// enable the PKI secrets engine and create a root CA
	cmd = fmt.Sprintf(`vault secrets enable pki && vault secrets tune -max-lease-ttl=8760h pki && vault write -field=certificate pki/root/generate/internal common_name="cert-manager-issuer-root" issuer_name="vault-root" ttl=8760h`)
	_, err = exutil.RemoteShPod(oc, ns, vaultPodName, "sh", "-c", cmd)
	o.Expect(err).NotTo(o.HaveOccurred())
	// configure the CRL and CA endpoints
	cmd = fmt.Sprintf(`vault write pki/config/urls issuing_certificates="https://%s.%s:8200/v1/pki/ca" crl_distribution_points="https://%s.%s:8200/v1/pki/crl"`, release, ns, release, ns)
	_, err = exutil.RemoteShPod(oc, ns, vaultPodName, "sh", "-c", cmd)
	o.Expect(err).NotTo(o.HaveOccurred())
	// create an intermediate CA
	cmd = fmt.Sprintf(`vault secrets enable -path=pki_int pki && vault secrets tune -max-lease-ttl=2160h pki_int`)
	_, err = exutil.RemoteShPod(oc, ns, vaultPodName, "sh", "-c", cmd)
	o.Expect(err).NotTo(o.HaveOccurred())
	cmd = fmt.Sprintf(`vault write -format=json pki_int/intermediate/generate/internal common_name="cert-manager-issuer-int" issuer_name="vault-int" ttl=2160h`)
	output, err = exutil.RemoteShPod(oc, ns, vaultPodName, "sh", "-c", cmd)
	o.Expect(err).NotTo(o.HaveOccurred())
	pkiIntermediateCSR := gjson.Get(output, "data.csr").String()
	cmd = fmt.Sprintf(`echo "%s" > /tmp/pki_intermediate.csr`, pkiIntermediateCSR)
	oc.NotShowInfo()
	_, err = exutil.RemoteShPod(oc, ns, vaultPodName, "sh", "-c", cmd)
	oc.SetShowInfo()
	o.Expect(err).NotTo(o.HaveOccurred())
	// sign the intermediate certificate with the root CA private key
	cmd = fmt.Sprintf(`vault write -format=json pki/root/sign-intermediate issuer_ref="vault-root" csr=@/tmp/pki_intermediate.csr format=pem_bundle ttl=2160h`)
	output, err = exutil.RemoteShPod(oc, ns, vaultPodName, "sh", "-c", cmd)
	o.Expect(err).NotTo(o.HaveOccurred())
	intCertPemData := gjson.Get(output, "data.certificate").String()
	cmd = fmt.Sprintf(`echo "%s" > /tmp/intermediate.cert.pem`, intCertPemData)
	oc.NotShowInfo()
	_, err = exutil.RemoteShPod(oc, ns, vaultPodName, "sh", "-c", cmd)
	oc.SetShowInfo()
	o.Expect(err).NotTo(o.HaveOccurred())
	cmd = fmt.Sprintf(`vault write pki_int/intermediate/set-signed certificate=@/tmp/intermediate.cert.pem`)
	_, err = exutil.RemoteShPod(oc, ns, vaultPodName, "sh", "-c", cmd)
	o.Expect(err).NotTo(o.HaveOccurred())
	// create a role that enables to create certificates with any commonName or dnsNames for test simplicity
	cmd = fmt.Sprintf(`vault write pki_int/roles/cluster-dot-local require_cn=false allow_any_name=true max_ttl=720h`)
	_, err = exutil.RemoteShPod(oc, ns, vaultPodName, "sh", "-c", cmd)
	o.Expect(err).NotTo(o.HaveOccurred())
	// create a policy that enables needed permission to the PKI secrets engine paths
	cmd = fmt.Sprintf(`vault policy write cert-manager - <<EOF
path "pki_int/sign/cluster-dot-local"    { capabilities = ["update"] }
EOF`)
	_, err = exutil.RemoteShPod(oc, ns, vaultPodName, "sh", "-c", cmd)
	o.Expect(err).NotTo(o.HaveOccurred())

	e2e.Logf("Vault PKI engine configured successfully!")
	return vaultPodName, vaultRootToken
}
