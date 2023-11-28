package router

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/openshift/openshift-tests-private/test/extended/util/architecture"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

var _ = g.Describe("[sig-network-edge] Network_Edge should", func() {
	defer g.GinkgoRecover()

	var (
		oc                = exutil.NewCLI("externaldns", exutil.KubeConfigPath())
		operatorNamespace = "external-dns-operator"
		operatorLabel     = "name=external-dns-operator"
		operandLabelKey   = "app.kubernetes.io/instance="
		addLabel          = "external-dns.mydomain.org/publish=yes"
		delLabel          = "external-dns.mydomain.org/publish-"
		recordsReadyLog   = "All records are already up to date"
	)

	g.BeforeEach(func() {
		// skip ARM64 arch
		architecture.SkipNonAmd64SingleArch(oc)

		// CredentialReqeust needs to be provioned by Cloud automatically
		modeInCloudCredential, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("cloudcredential", "cluster", "-o=jsonpath={.spec.credentialsMode}").Output()
		if modeInCloudCredential == "Manual" {
			g.Skip("Skip since CCO mode is Manual")
		}
	})

	// author: hongli@redhat.com
	g.It("ConnectedOnly-ROSA-OSD_CCS-Author:hongli-High-48138-Support External DNS on AWS platform", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router", "extdns")
			sampleAWS           = filepath.Join(buildPruningBaseDir, "sample-aws-rt.yaml")
			crName              = "sample-aws-rt"
			operandLabel        = operandLabelKey + crName
			routeNamespace      = "openshift-ingress-canary"
			routeName           = "canary"
		)

		exutil.By("Ensure the case is runnable on the cluster")
		exutil.SkipIfPlatformTypeNot(oc, "AWS")
		baseDomain, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("dns.config", "cluster", "-o=jsonpath={.spec.baseDomain}").Output()
		createExternalDNSOperator(oc)

		exutil.By("Create CR ExternalDNS sample-aws-rt and ensure operand pod is ready")
		waitErr := waitForPodWithLabelReady(oc, operatorNamespace, operatorLabel)
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("the external dns operator pod is not ready"))
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("externaldns", crName).Output()
		time.Sleep(3 * time.Second)
		sedCmd := fmt.Sprintf(`sed -i 's/basedomain/%s/g' %s`, baseDomain, sampleAWS)
		_, err := exec.Command("bash", "-c", sedCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", sampleAWS).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		waitErr = waitForPodWithLabelReady(oc, operatorNamespace, operandLabel)
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("the external dns operand pod is not ready"))
		ensureLogsContainString(oc, operatorNamespace, operandLabel, recordsReadyLog)

		exutil.By("Add label to canary route, ensure ExternalDNS added the record")
		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("-n", routeNamespace, "route", routeName, delLabel, "--overwrite").Output()
		_, err = oc.AsAdmin().WithoutNamespace().Run("label").Args("-n", routeNamespace, "route", routeName, addLabel).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		ensureLogsContainString(oc, operatorNamespace, operandLabel, "Desired change: CREATE external-dns-canary-openshift-ingress-canary")

		exutil.By("Remove label from the canary route, ensure ExternalDNS deleted the record")
		_, err = oc.AsAdmin().WithoutNamespace().Run("label").Args("-n", routeNamespace, "route", routeName, delLabel, "--overwrite").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		ensureLogsContainString(oc, operatorNamespace, operandLabel, "Desired change: DELETE external-dns-canary-openshift-ingress-canary")
	})

	// author: hongli@redhat.com
	g.It("ConnectedOnly-ARO-Author:hongli-High-48139-Support External DNS on Azure DNS provider", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router", "extdns")
			sampleAzure         = filepath.Join(buildPruningBaseDir, "sample-azure-rt.yaml")
			crName              = "sample-azure-rt"
			operandLabel        = operandLabelKey + crName
			routeNamespace      = "openshift-ingress-canary"
			routeName           = "canary"
		)

		exutil.By("Ensure the case is runnable on the cluster")
		exutil.SkipIfPlatformTypeNot(oc, "Azure")
		zoneID, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("dns.config", "cluster", "-o=jsonpath={.spec.privateZone.id}").Output()
		if !strings.Contains(zoneID, "openshift") {
			g.Skip("Skip since no valid DNS privateZone is configured in this cluster")
		}
		createExternalDNSOperator(oc)

		exutil.By("Create CR ExternalDNS sample-azure-svc with invalid zone ID")
		waitErr := waitForPodWithLabelReady(oc, operatorNamespace, operatorLabel)
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("the external dns operator pod is not ready"))
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("externaldns", crName).Output()
		time.Sleep(3 * time.Second)
		_, err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", sampleAzure).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		waitErr = waitForPodWithLabelReady(oc, operatorNamespace, operandLabel)
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("the external dns operand pod is not ready"))
		ensureLogsContainString(oc, operatorNamespace, operandLabel, "Found 0 Azure DNS zone")
		operandPod := getPodName(oc, operatorNamespace, operandLabel)

		exutil.By("Patch externaldns with valid privateZone ID and wait until new operand pod ready")
		patchStr := "[{\"op\":\"replace\",\"path\":\"/spec/zones/0\",\"value\":" + zoneID + "}]"
		patchGlobalResourceAsAdmin(oc, "externaldnses.externaldns.olm.openshift.io/"+crName, patchStr)
		err = waitForResourceToDisappear(oc, operatorNamespace, "pod/"+operandPod[0])
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("resource %v does not disapper", "pod/"+operandPod[0]))
		waitErr = waitForPodWithLabelReady(oc, operatorNamespace, operandLabel)
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("the external dns operand pod is not ready"))
		ensureLogsContainString(oc, operatorNamespace, operandLabel, "Found 1 Azure Private DNS zone")

		exutil.By("Add label to canary route, ensure ExternalDNS added the record")
		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("-n", routeNamespace, "route", routeName, delLabel, "--overwrite").Output()
		_, err = oc.AsAdmin().WithoutNamespace().Run("label").Args("-n", routeNamespace, "route", routeName, addLabel).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		ensureLogsContainString(oc, operatorNamespace, operandLabel, "Updating TXT record named 'external-dns-canary")

		exutil.By("Remove label from the canary route, ensure ExternalDNS deleted the record")
		_, err = oc.AsAdmin().WithoutNamespace().Run("label").Args("-n", routeNamespace, "route", routeName, delLabel, "--overwrite").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		ensureLogsContainString(oc, operatorNamespace, operandLabel, "Deleting TXT record named 'external-dns-canary")
	})

	// author: hongli@redhat.com
	g.It("ConnectedOnly-Author:hongli-High-48140-Support External DNS on GCP DNS provider", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router", "extdns")
			sampleGCP           = filepath.Join(buildPruningBaseDir, "sample-gcp-svc.yaml")
			crName              = "sample-gcp-svc"
			operandLabel        = operandLabelKey + crName
			serviceNamespace    = "openshift-ingress-canary"
			serviceName         = "ingress-canary"
		)

		exutil.By("Ensure the case is runnable on the cluster")
		exutil.SkipIfPlatformTypeNot(oc, "GCP")
		zoneID, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("dns.config", "cluster", "-o=jsonpath={.spec.privateZone.id}").Output()
		if !strings.Contains(zoneID, "private") {
			g.Skip("Skip since no valid DNS privateZone is configured in this cluster")
		}
		createExternalDNSOperator(oc)
		baseDomain := getBaseDomain(oc)

		exutil.By("Create CR ExternalDNS sample-gcp-svc and ensure operand pod is ready")
		waitErr := waitForPodWithLabelReady(oc, operatorNamespace, operatorLabel)
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("the external dns operator pod is not ready"))
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("externaldns", crName).Output()
		time.Sleep(3 * time.Second)
		_, err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", sampleGCP).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		waitErr = waitForPodWithLabelReady(oc, operatorNamespace, operandLabel)
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("the external dns operand pod is not ready"))
		ensureLogsContainString(oc, operatorNamespace, operandLabel, "No zones found in the project")
		operandPod := getPodName(oc, operatorNamespace, operandLabel)

		exutil.By("Patch externaldns with valid privateZone ID and wait until new operand pod ready")
		patchStr := "[{\"op\":\"replace\",\"path\":\"/spec/source/fqdnTemplate/0\",\"value\":'{{.Name}}." + baseDomain + "'},{\"op\":\"replace\",\"path\":\"/spec/zones/0\",\"value\":" + zoneID + "}]"
		patchGlobalResourceAsAdmin(oc, "externaldnses.externaldns.olm.openshift.io/"+crName, patchStr)
		waitErr = waitForResourceToDisappear(oc, operatorNamespace, "pod/"+operandPod[0])
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("resource %v does not disapper", "pod/"+operandPod[0]))
		waitErr = waitForPodWithLabelReady(oc, operatorNamespace, operandLabel)
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("the external dns operand pod is not ready"))
		ensureLogsContainString(oc, operatorNamespace, operandLabel, recordsReadyLog)

		exutil.By("Add label to canary service, ensure ExternalDNS added the record")
		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("-n", serviceNamespace, "service", serviceName, delLabel, "--overwrite").Output()
		_, err = oc.AsAdmin().WithoutNamespace().Run("label").Args("-n", serviceNamespace, "service", serviceName, addLabel).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		ensureLogsContainString(oc, operatorNamespace, operandLabel, "Add records: external-dns-ingress-canary")

		exutil.By("Remove label from the canary service, ensure ExternalDNS deleted the record")
		_, err = oc.AsAdmin().WithoutNamespace().Run("label").Args("-n", serviceNamespace, "service", serviceName, delLabel, "--overwrite").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		ensureLogsContainString(oc, operatorNamespace, operandLabel, "Del records: external-dns-ingress-canary")
	})
})
