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
	"github.com/openshift/openshift-tests-private/test/extended/util/architecture"
	"github.com/tidwall/gjson"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
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

		// MinMultiArchSupportedVersion is the minimum version to support multi-arch
		MinMultiArchSupportedVersion = "1.13.0"
	)

	// switch to an available catalogsource
	catalogSourceName, err := getAvailableCatalogSourceName(oc, catalogSourceNamespace)
	if len(catalogSourceName) == 0 || err != nil {
		g.Skip("skip since no available catalogsource was found")
	}
	e2e.Logf("will use catalogsource: %s", catalogSourceName)

	// skip non-amd64 arch for unsupported version
	currentVersion, _ := semver.Parse(getCurrentCSVDescVersion(oc, catalogSourceNamespace, catalogSourceName, subscriptionName, channelName))
	e2e.Logf("current csv desc version: %s", currentVersion)
	minVersion, _ := semver.Parse(MinMultiArchSupportedVersion)
	if currentVersion.Compare(minVersion) == -1 {
		e2e.Logf("currentVersion(%s) < minVersion(%s), skip non-amd64 arch for unsupported version", currentVersion, minVersion)
		architecture.SkipNonAmd64SingleArch(oc)
	}

	e2e.Logf("Prepare cert manager operator.\n")
	buildPruningBaseDir := exutil.FixturePath("testdata", "apiserverauth/certmanager")

	// create namspace
	namespaceFile := filepath.Join(buildPruningBaseDir, "namespace.yaml")
	msg, err := oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", namespaceFile).Output()

	// skip the install process to mitigate the namespace deletion terminating issue caused by case 62006
	// the full message is 'Detected changes to resource cert-manager-operator which is currently being deleted'
	if strings.Contains(msg, "being deleted") {
		g.Skip("skipping the install process as the cert-manager-operator namespace is being terminated due to other env issue e.g. we ever hit such failures caused by OCPBUGS-31443")
	}
	e2e.Logf("err %v, msg %v", err, msg)

	// create operatorgroup
	operatorGroupFile := filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
	msg, err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", operatorGroupFile).Output()
	e2e.Logf("err %v, msg %v", err, msg)

	// create subscription
	subscriptionTemplate := filepath.Join(buildPruningBaseDir, "subscription.yaml")
	params := []string{"-f", subscriptionTemplate, "-p", "NAME=" + subscriptionName, "SOURCE=" + catalogSourceName, "SOURCE_NAMESPACE=" + catalogSourceNamespace, "CHANNEL=" + channelName}
	exutil.ApplyNsResourceFromTemplate(oc, subscriptionNamespace, params...)

	// checking subscription status
	errCheck := wait.Poll(10*time.Second, 180*time.Second, func() (bool, error) {
		subState, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", subscriptionName, "-n", subscriptionNamespace, "-o=jsonpath={.status.state}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Compare(subState, "AtLatestKnown") == 0 {
			return true, nil
		}
		return false, nil
	})
	if errCheck != nil {
		e2e.Logf("Dumping the status of subscription %s:", subscriptionName)
		err := oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", subscriptionName, "-n", subscriptionNamespace, "-o=jsonpath={.status}").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Failf("The subscription %v status is not correct", subscriptionName)
	}

	// checking csv status
	csvName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", subscriptionName, "-n", subscriptionNamespace, "-o=jsonpath={.status.installedCSV}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(csvName).NotTo(o.BeEmpty())
	errCheck = wait.Poll(10*time.Second, 180*time.Second, func() (bool, error) {
		csvState, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", csvName, "-n", subscriptionNamespace, "-o=jsonpath={.status.phase}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Compare(csvState, "Succeeded") == 0 {
			e2e.Logf("CSV check complete!!!")
			return true, nil
		}
		return false, nil

	})
	if errCheck != nil {
		tmpCsvState, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", csvName, "-n", subscriptionNamespace, "-o=jsonpath={.status}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("csv %s is:%s", csvName, tmpCsvState)
		exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("csv %v is not correct status", csvName))
	}

	e2e.Logf("Check cert manager pods.\n")
	mStatusErr := wait.Poll(10*time.Second, 300*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "cert-manager", "pod", "-o=jsonpath={.items[*].status.phase}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		var certManagerPodList []string = strings.Fields(output)
		e2e.Logf("certManagerPodList=%v", certManagerPodList)
		if len(certManagerPodList) == 3 {
			if strings.Contains(certManagerPodList[2], "Running") {
				e2e.Logf("operator pods created successfully!!!")
				return true, nil
			}
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(mStatusErr, "operator pods created failed.")
}

// create selfsigned issuer
func createIssuer(oc *exutil.CLI) {
	e2e.Logf("Create selfsigned issuer in ns scope created in last step.")
	buildPruningBaseDir := exutil.FixturePath("testdata", "apiserverauth/certmanager")
	issuerFile := filepath.Join(buildPruningBaseDir, "issuer-selfsigned.yaml")
	err := oc.Run("create").Args("-f", issuerFile).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())

	statusErr := wait.Poll(10*time.Second, 300*time.Second, func() (bool, error) {
		output, err := oc.Run("get").Args("issuer", "default-selfsigned").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(output, "True") {
			e2e.Logf("Get issuer output is: %v", output)
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(statusErr, "get issuer is wrong")
}

// create certificate using selfsigned issuer
func createCertificate(oc *exutil.CLI) {
	e2e.Logf("As the normal user, create certificate.")
	buildPruningBaseDir := exutil.FixturePath("testdata", "apiserverauth/certmanager")
	certFile := filepath.Join(buildPruningBaseDir, "cert-selfsigned.yaml")
	err := oc.Run("create").Args("-f", certFile).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())

	statusErr := wait.Poll(10*time.Second, 300*time.Second, func() (bool, error) {
		output, err := oc.Run("get").Args("certificate", "default-selfsigned-cert").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("certificate status is: %v ", output)
		if strings.Contains(output, "True") {
			e2e.Logf("certificate status is normal.")
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(statusErr, "certificate is wrong.")
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

// Get the available CatalogSource's name from specific namespace
func getAvailableCatalogSourceName(oc *exutil.CLI, namespace string) (string, error) {
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", namespace, "catalogsource", `-o=jsonpath={.items[?(@.status.connectionState.lastObservedState=="READY")].metadata.name}`).Output()
	if err != nil {
		return "", fmt.Errorf("failed to get catalogsource from namespace: %s", namespace)
	}

	// will first check if output contains "qe-app-registry", then "redhat-operators"
	targetCatalogSources := []string{"qe-app-registry", "redhat-operators"}
	for _, name := range targetCatalogSources {
		if strings.Contains(output, name) {
			return name, nil
		}
	}

	// if no target CatalogSource was found, print existing CatalogSource and return ""
	output, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", namespace, "catalogsource").Output()
	e2e.Logf("get existing catalogsource: %s", output)
	return "", nil
}

// Get current CSV described version from PackageManifest before creating Subscription
func getCurrentCSVDescVersion(oc *exutil.CLI, sourceNamespace string, source string, subscriptionName string, channelName string) string {
	e2e.Logf("Check PackageManifest before getting currentCSVDesc.\n")
	waitErr := wait.Poll(10*time.Second, 90*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("packagemanifest", "-n", sourceNamespace, "-l", "catalog="+source, "--field-selector", "metadata.name="+subscriptionName).Output()
		if strings.Contains(output, "No resources found") || err != nil {
			e2e.Logf("get packagemanifest output:\n%v", output)
			return false, nil
		}
		return true, nil
	})
	if waitErr != nil {
		catalogStatus, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("catalogsource", "-n", sourceNamespace, source, "-o=jsonpath={.status}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("catalogsource \"%v\"'s status:\n%v", source, catalogStatus)
		pod, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", sourceNamespace, "-l", "olm.catalogSource="+source).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("catalogsource \"%v\"'s pod:\n%v", source, pod)
		e2e.Failf("The packagemanifest \"%v\" or catalogsource \"%v\" is unhealthy", subscriptionName, source)
	}

	ver, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("packagemanifest", "-n", sourceNamespace, "-l", "catalog="+source, "--field-selector", "metadata.name="+subscriptionName, "-o=jsonpath={.items[0].status.channels[?(@.name=='"+channelName+"')].currentCSVDesc.version}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return ver
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
