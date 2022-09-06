package cvo

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-updates] OTA cvo should", func() {
	defer g.GinkgoRecover()

	projectName := "openshift-cluster-version"

	oc := exutil.NewCLIWithoutNamespace(projectName)

	//author: yanyang@redhat.com
	g.It("Author:yanyang-High-49196-Install cluster without capabilities setting [Flaky]", func() {
		vCurrent := []string{"Console", "Insights", "Storage", "baremetal", "marketplace", "openshift-samples"}
		orgCap, err := getCVObyJP(oc, ".spec.capabilities")
		o.Expect(err).NotTo(o.HaveOccurred())

		if orgCap != "" {
			g.Skip("The test requires no capabilities in spec")
		}

		g.By("Check caps in vCurrent are installed")
		for _, op := range vCurrent {
			_, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("co", strings.ToLower(op)).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		g.By("Check cv status: enabled caps")
		enabledCap, err := getCVObyJP(oc, ".status.capabilities.enabledCapabilities[*]")
		o.Expect(err).NotTo(o.HaveOccurred())

		enabledCapSlice := strings.Split(enabledCap, " ")

		o.Expect(len(vCurrent)).To(o.Equal(len(enabledCapSlice)))
		for i, v := range enabledCapSlice {
			o.Expect(v).To(o.Equal(vCurrent[i]))
		}

		g.By("Check cv status: known caps")
		expKnown, err := getCapsManifest(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		knownCap, err := getCVObyJP(oc, ".status.capabilities.knownCapabilities[*]")
		o.Expect(err).NotTo(o.HaveOccurred())

		knownCapSlice := strings.Split(knownCap, " ")

		o.Expect(len(knownCapSlice)).To(o.Equal(len(expKnown)))
		for i, v := range knownCapSlice {
			o.Expect(v).To(o.Equal(expKnown[i]))
		}
	})

	//author: yanyang@redhat.com
	g.It("Author:yanyang-Medium-49508-disable capabilities by modifying the cv.spec.capabilities.baselineCapabilitySet [Serial]", func() {
		orgBaseCap, err := getCVObyJP(oc, ".spec.capabilities.baselineCapabilitySet")
		o.Expect(err).NotTo(o.HaveOccurred())

		if orgBaseCap != "vCurrent" {
			g.Skip("The test requires baselineCapabilitySet=vCurrent, rather than " + orgBaseCap)
		}

		defer changeCap(oc, true, orgBaseCap)

		g.By("Check cap status and condition prior to change")
		enabledCap, err := getCVObyJP(oc, ".status.capabilities.enabledCapabilities[*]")
		o.Expect(err).NotTo(o.HaveOccurred())

		capSet := strings.Split(enabledCap, " ")
		for _, op := range capSet {
			_, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("co", strings.ToLower(op)).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		status, err := getCVObyJP(oc, ".status.conditions[?(.type=='ImplicitlyEnabledCapabilities')].status")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(status).To(o.Equal("False"))

		g.By("Disable capabilities by modifying the baselineCapabilitySet")
		_, err = changeCap(oc, true, "None")
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check cap status and condition after change")
		enabledCapPost, err := getCVObyJP(oc, ".status.capabilities.enabledCapabilities[*]")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(enabledCapPost).To(o.Equal(enabledCap))

		for _, op := range capSet {
			_, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("co", strings.ToLower(op)).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		for _, k := range []string{"status", "reason", "message"} {
			jsonpath := ".status.conditions[?(.type=='ImplicitlyEnabledCapabilities')]." + k
			out, err := getCVObyJP(oc, jsonpath)
			o.Expect(err).NotTo(o.HaveOccurred())
			if k == "status" {
				o.Expect(out).To(o.Equal("True"))
			} else if k == "reason" {
				o.Expect(out).To(o.Equal("CapabilitiesImplicitlyEnabled"))
			} else {
				msg := append(capSet, "The following capabilities could not be disabled")
				for _, m := range msg {
					o.Expect(out).To(o.ContainSubstring(m))
				}
			}
		}
	})

	//author: yanyang@redhat.com
	g.It("Author:yanyang-Low-49670-change spec.capabilities to invalid value", func() {
		orgCap, err := getCVObyJP(oc, ".spec.capabilities")
		o.Expect(err).NotTo(o.HaveOccurred())
		if orgCap == "" {
			defer ocJSONPatch(oc, "", "clusterversion/version", []JSONp{{"remove", "/spec/capabilities", nil}})
		} else {
			orgBaseCap, err := getCVObyJP(oc, ".spec.capabilities.baselineCapabilitySet")
			o.Expect(err).NotTo(o.HaveOccurred())
			orgAddCapstr, err := getCVObyJP(oc, ".spec.capabilities.additionalEnabledCapabilities[*]")
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf(orgBaseCap, orgAddCapstr)

			orgAddCap := strings.Split(orgAddCapstr, " ")

			defer changeCap(oc, true, orgBaseCap)
			defer changeCap(oc, false, orgAddCap)
		}

		g.By("Set invalid baselineCapabilitySet")
		cmdOut, err := changeCap(oc, true, "Invalid")
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(cmdOut).To(o.ContainSubstring("Unsupported value: \"Invalid\": supported values: \"None\", \"v4.11\", \"v4.12\", \"vCurrent\""))

		g.By("Set invalid additionalEnabledCapabilities")
		cmdOut, err = changeCap(oc, false, []string{"Invalid"})
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(cmdOut).To(o.ContainSubstring("Unsupported value: \"Invalid\": supported values: \"openshift-samples\", \"baremetal\", \"marketplace\", \"Console\", \"Insights\", \"Storage\""))
	})

	//author: yanyang@redhat.com
	g.It("Longduration-NonPreRelease-ConnectedOnly-Author:yanyang-Medium-45879-check update info with oc adm upgrade --include-not-recommended [Serial][Slow]", func() {
		g.By("Check if it's a GCP cluster")
		platformType, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.type}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.ToLower(platformType) != "gcp" {
			g.Skip("Skip for non-gcp cluster!")
		}

		orgUpstream, err := getCVObyJP(oc, ".spec.upstream")
		o.Expect(err).NotTo(o.HaveOccurred())
		orgChannel, err := getCVObyJP(oc, ".spec.channel")
		o.Expect(err).NotTo(o.HaveOccurred())

		e2e.Logf("Original upstream:%s, original channel:%s", orgUpstream, orgChannel)

		g.By("Patch upstream")
		projectID := "openshift-qe"
		ctx := context.Background()
		client, err := storage.NewClient(ctx)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer client.Close()

		graphURL, bucket, object, _, _, err := buildGraph(client, oc, projectID, "cincy-conditional-edge.json")
		defer DeleteBucket(client, bucket)
		defer DeleteObject(client, bucket, object)
		o.Expect(err).NotTo(o.HaveOccurred())

		_, err = ocJSONPatch(oc, "", "clusterversion/version", []JSONp{{"add", "/spec/upstream", graphURL}})
		o.Expect(err).NotTo(o.HaveOccurred())

		defer restoreCVSpec(orgUpstream, orgChannel, oc)

		g.By("Check oc adm upgrade when there are not-recommended updates")
		expUpdate := "Additional updates which are not recommended based on your cluster " +
			"configuration are available, to view those re-run the command with " +
			"--include-not-recommended"
		found := checkUpdates(oc, false, 5, 15, "No updates available", expUpdate)
		o.Expect(found).To(o.BeTrue())

		g.By("Check risk type=Always updates present")
		expUpdate = "Version: 4.88.888888\n  " +
			"Image: registry.ci.openshift.org/ocp/release@sha256:" +
			"8888888888888888888888888888888888888888888888888888888888888888\n  " +
			"Recommended: False\n  " +
			"Reason: ReleaseIsRejected\n  " +
			"Message: Too many CI failures on this release, so do not update to it"
		found = checkUpdates(oc, true, 5, 15, "No updates available", "Supported but not recommended updates", expUpdate)
		o.Expect(found).To(o.BeTrue())

		g.By("Check 2 risks updates present")
		expUpdate = "Version: 4.77.777777\n  " +
			"Image: registry.ci.openshift.org/ocp/release@sha256:" +
			"7777777777777777777777777777777777777777777777777777777777777777\n  " +
			"Recommended: False\n  " +
			"Reason: SomeInvokerThing\n  " +
			"Message: On clusters on default invoker user, this imaginary bug can happen. https://bug.example.com/a"
		found = checkUpdates(oc, true, 60, 15*60, "No updates available", "Supported but not recommended updates", expUpdate)
		o.Expect(found).To(o.BeTrue())

		g.By("Check recommended update present")
		expUpdate = "Recommended updates:\n\n  " +
			"VERSION     IMAGE\n  " +
			"4.99.999999 registry.ci.openshift.org/ocp/release@sha256:" +
			"9999999999999999999999999999999999999999999999999999999999999999"
		found = checkUpdates(oc, true, 60, 15*60, expUpdate)
		o.Expect(found).To(o.BeTrue())

		g.By("Check multiple reason conditional update present")
		_, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade", "channel", "buggy").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		expUpdate = "Version: 4.77.777777\n  " +
			"Image: registry.ci.openshift.org/ocp/release@sha256:" +
			"7777777777777777777777777777777777777777777777777777777777777777\n  " +
			"Recommended: False\n  " +
			"Reason: MultipleReasons\n  " +
			"Message: On clusters on default invoker user, this imaginary bug can happen. " +
			"https://bug.example.com/a\n  \n  " +
			"On clusters with the channel set to 'buggy', this imaginary bug can happen. " +
			"https://bug.example.com/b"
		found = checkUpdates(oc, true, 300, 65*60, expUpdate)
		o.Expect(found).To(o.BeTrue())
	})

	//author: yanyang@redhat.com
	g.It("ConnectedOnly-Author:yanyang-Low-46422-cvo drops invalid conditional edges [Serial]", func() {
		g.By("Check if it's a GCP cluster")
		platformType, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.type}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.ToLower(platformType) != "gcp" {
			g.Skip("Skip for non-gcp cluster!")
		}

		orgUpstream, _ := getCVObyJP(oc, ".spec.upstream")
		e2e.Logf("Original upstream:%s", orgUpstream)

		g.By("Patch upstream")
		projectID := "openshift-qe"
		ctx := context.Background()
		client, err := storage.NewClient(ctx)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer client.Close()

		graphURL, bucket, object, _, _, err := buildGraph(client, oc, projectID, "cincy-conditional-edge-invalid-null-node.json")
		defer DeleteBucket(client, bucket)
		defer DeleteObject(client, bucket, object)
		o.Expect(err).NotTo(o.HaveOccurred())

		_, err = ocJSONPatch(oc, "", "clusterversion/version", []JSONp{{"add", "/spec/upstream", graphURL}})
		o.Expect(err).NotTo(o.HaveOccurred())

		defer restoreCVSpec(orgUpstream, "nochange", oc)

		g.By("Check CVO prompts correct reason and message")
		expString := "warning: Cannot display available updates:\n" +
			"  Reason: ResponseInvalid\n" +
			"  Message: Unable to retrieve available updates: no node for conditional update"
		err = wait.Poll(5*time.Second, 15*time.Second, func() (bool, error) {
			cmdOut, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf(cmdOut)
			if strings.Contains(cmdOut, expString) {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "Test on empty target node failed")

		graphURL, bucket, object, _, _, err = buildGraph(client, oc, projectID, "cincy-conditional-edge-invalid-multi-risks.json")
		defer DeleteBucket(client, bucket)
		defer DeleteObject(client, bucket, object)
		o.Expect(err).NotTo(o.HaveOccurred())

		_, err = ocJSONPatch(oc, "", "clusterversion/version", []JSONp{{"add", "/spec/upstream", graphURL}})
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check no updates")
		err = wait.Poll(5*time.Second, 15*time.Second, func() (bool, error) {
			cmdOut, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf(cmdOut)
			if strings.Contains(cmdOut, "No updates available") {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "Test on multiple invalid risks failed")
	})

	//author: yanyang@redhat.com
	g.It("ConnectedOnly-Author:yanyang-Low-47175-upgrade cluster when current version is in the upstream but there are not update paths [Serial]", func() {
		g.By("Check if it's a GCP cluster")
		platformType, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.type}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.ToLower(platformType) != "gcp" {
			g.Skip("Skip for non-gcp cluster!")
		}

		orgUpstream, _ := getCVObyJP(oc, ".spec.upstream")
		e2e.Logf("Original upstream:%s", orgUpstream)

		g.By("Patch upstream")
		projectID := "openshift-qe"
		ctx := context.Background()
		client, err := storage.NewClient(ctx)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer client.Close()

		graphURL, bucket, object, _, _, err := buildGraph(client, oc, projectID, "cincy-conditional-edge-invalid-multi-risks.json")
		defer DeleteBucket(client, bucket)
		defer DeleteObject(client, bucket, object)
		o.Expect(err).NotTo(o.HaveOccurred())

		_, err = ocJSONPatch(oc, "", "clusterversion/version", []JSONp{{"add", "/spec/upstream", graphURL}})
		o.Expect(err).NotTo(o.HaveOccurred())

		defer restoreCVSpec(orgUpstream, "nochange", oc)

		g.By("Check no updates but RetrievedUpdates=True")
		err = wait.Poll(5*time.Second, 15*time.Second, func() (bool, error) {
			cmdOut, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(cmdOut, "No updates available") {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "Failed to check updates")

		status, err := getCVObyJP(oc, ".status.conditions[?(.type=='RetrievedUpdates')].status")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(status).To(o.Equal("True"))

		target := GenerateReleaseVersion(oc)
		o.Expect(target).NotTo(o.BeEmpty())

		g.By("Upgrade with oc adm upgrade --to")
		cmdOut, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade", "--to", target).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(cmdOut).To(o.ContainSubstring(
			"no recommended updates, specify --to-image to conti" +
				"nue with the update or wait for new updates to be available"))

		g.By("Upgrade with oc adm upgrade --to --allow-not-recommended")
		cmdOut, err = oc.AsAdmin().WithoutNamespace().Run("adm").
			Args("upgrade", "--allow-not-recommended", "--to", target).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(cmdOut).To(o.ContainSubstring(
			"no recommended or conditional updates, specify --to-image to conti" +
				"nue with the update or wait for new updates to be available"))

		targetPullspec := GenerateReleasePayload(oc)
		o.Expect(targetPullspec).NotTo(o.BeEmpty())

		g.By("Upgrade with oc adm upgrade --to-image")
		cmdOut, err = oc.AsAdmin().WithoutNamespace().Run("adm").
			Args("upgrade", "--to-image", targetPullspec).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(cmdOut).To(o.ContainSubstring(
			"no recommended updates, specify --allow-explicit-upgrade to conti" +
				"nue with the update or wait for new updates to be available"))

		g.By("Upgrade with oc adm upgrade --to-image --allow-not-recommended")
		cmdOut, err = oc.AsAdmin().WithoutNamespace().Run("adm").
			Args("upgrade", "--allow-not-recommended", "--to-image", targetPullspec).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(cmdOut).To(o.ContainSubstring(
			"no recommended or conditional updates, specify --allow-explicit-upgrade to conti" +
				"nue with the update or wait for new updates to be available"))
	})

	//author: jialiu@redhat.com
	g.It("Author:jialiu-Medium-41391-cvo serves metrics over only https not http", func() {
		g.By("Check cvo delopyment config file...")
		cvoDeploymentYaml, err := GetDeploymentsYaml(oc, "cluster-version-operator", projectName)
		o.Expect(err).NotTo(o.HaveOccurred())
		var keywords = []string{"--listen=0.0.0.0:9099",
			"--serving-cert-file=/etc/tls/serving-cert/tls.crt",
			"--serving-key-file=/etc/tls/serving-cert/tls.key"}
		for _, v := range keywords {
			o.Expect(cvoDeploymentYaml).Should(o.ContainSubstring(v))
		}

		g.By("Check cluster-version-operator binary help")
		cvoPodsList, err := exutil.WaitForPods(
			oc.AdminKubeClient().CoreV1().Pods(projectName),
			exutil.ParseLabelsOrDie("k8s-app=cluster-version-operator"),
			exutil.CheckPodIsReady, 1, 3*time.Minute)
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Get cvo pods: %v", cvoPodsList)
		output, err := PodExec(oc, "/usr/bin/cluster-version-operator start --help", projectName, cvoPodsList[0])
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf(
			"/usr/bin/cluster-version-operator start --help executs error on %v", cvoPodsList[0]))
		e2e.Logf(output)
		keywords = []string{"You must set both --serving-cert-file and --serving-key-file unless you set --listen empty"}
		for _, v := range keywords {
			o.Expect(output).Should(o.ContainSubstring(v))
		}

		g.By("Verify cvo metrics is only exported via https")
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").
			Args("servicemonitor", "cluster-version-operator",
				"-n", projectName, "-o=json").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		var result map[string]interface{}
		json.Unmarshal([]byte(output), &result)
		endpoints := result["spec"].(map[string]interface{})["endpoints"]
		e2e.Logf("Get cvo's spec.endpoints: %v", endpoints)
		o.Expect(endpoints).Should(o.HaveLen(1))

		output, err = oc.AsAdmin().WithoutNamespace().Run("get").
			Args("servicemonitor", "cluster-version-operator",
				"-n", projectName, "-o=jsonpath={.spec.endpoints[].scheme}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Get cvo's spec.endpoints scheme: %v", output)
		o.Expect(output).Should(o.Equal("https"))

		g.By("Get cvo endpoint URI")
		//output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("endpoints", "cluster-version-operator", "-n", projectName, "-o=jsonpath='{.subsets[0].addresses[0].ip}:{.subsets[0].ports[0].port}'").Output()
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").
			Args("endpoints", "cluster-version-operator",
				"-n", projectName, "--no-headers").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		re := regexp.MustCompile(`cluster-version-operator\s+([^\s]*)`)
		matchedResult := re.FindStringSubmatch(output)
		e2e.Logf("Regex mached result: %v", matchedResult)
		o.Expect(matchedResult).Should(o.HaveLen(2))
		endpointURI := matchedResult[1]
		e2e.Logf("Get cvo endpoint URI: %v", endpointURI)
		o.Expect(endpointURI).ShouldNot(o.BeEmpty())

		g.By("Check metric server is providing service https, but not http")
		cmd := fmt.Sprintf("curl http://%s/metrics", endpointURI)
		output, err = PodExec(oc, cmd, projectName, cvoPodsList[0])
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("cmd %s executs error on %v", cmd, cvoPodsList[0]))
		e2e.Logf(output)
		keywords = []string{"Client sent an HTTP request to an HTTPS server"}
		for _, v := range keywords {
			o.Expect(output).Should(o.ContainSubstring(v))
		}

		g.By("Check metric server is providing service via https correctly.")
		cmd = fmt.Sprintf("curl -k -I https://%s/metrics", endpointURI)
		output, err = PodExec(oc, cmd, projectName, cvoPodsList[0])
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("cmd %s executs error on %v", cmd, cvoPodsList[0]))
		e2e.Logf(output)
		keywords = []string{"HTTP/1.1 200 OK"}
		for _, v := range keywords {
			o.Expect(output).Should(o.ContainSubstring(v))
		}
	})

	//author: yanyang@redhat.com
	g.It("Longduration-NonPreRelease-Author:yanyang-Medium-32138-cvo alert should not be fired when RetrievedUpdates failed due to nochannel [Serial][Slow]", func() {
		orgChannel, _ := getCVObyJP(oc, ".spec.channel")

		defer oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade", "channel", orgChannel).Execute()

		g.By("Enable alert by clearing channel")
		err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade", "channel").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check RetrievedUpdates condition")
		reason, err := getCVObyJP(oc, ".status.conditions[?(.type=='RetrievedUpdates')].reason")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(reason).To(o.Equal("NoChannel"))

		g.By("Alert CannotRetrieveUpdates does not appear within 60m")
		appeared, _, err := waitForAlert(oc, "CannotRetrieveUpdates", 600, 3600, "")
		o.Expect(appeared).NotTo(o.BeTrue())
		o.Expect(err.Error()).To(o.ContainSubstring("timed out waiting for the condition"))

		g.By("Alert CannotRetrieveUpdates does not appear after 60m")
		appeared, _, err = waitForAlert(oc, "CannotRetrieveUpdates", 300, 600, "")
		o.Expect(appeared).NotTo(o.BeTrue())
		o.Expect(err.Error()).To(o.ContainSubstring("timed out waiting for the condition"))
	})

	//author: yanyang@redhat.com
	g.It("ConnectedOnly-Author:yanyang-Medium-43178-manage channel by using oc adm upgrade channel [Serial]", func() {
		g.By("Check if it's a GCP cluster")
		platformType, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.type}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.ToLower(platformType) != "gcp" {
			g.Skip("Skip for non-gcp cluster!")
		}

		orgUpstream, _ := getCVObyJP(oc, ".spec.upstream")
		orgChannel, _ := getCVObyJP(oc, ".spec.channel")

		e2e.Logf("Original upstream:%s, original channel:%s", orgUpstream, orgChannel)

		projectID := "openshift-qe"
		ctx := context.Background()
		client, err := storage.NewClient(ctx)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer client.Close()

		graphURL, bucket, object, _, _, err := buildGraph(client, oc, projectID, "cincy.json")
		defer DeleteBucket(client, bucket)
		defer DeleteObject(client, bucket, object)
		o.Expect(err).NotTo(o.HaveOccurred())

		defer restoreCVSpec(orgUpstream, orgChannel, oc)

		// Prerequisite: the available channels are not present
		g.By("The test requires the available channels are not present as a prerequisite")
		cmdOut, _ := oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade").Output()
		o.Expect(cmdOut).NotTo(o.ContainSubstring("available channels:"))

		version, _ := getCVObyJP(oc, ".status.desired.version")

		g.By("Set to an unknown channel when available channels are not present")
		cmdOut, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade", "channel", "unknown-channel").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(cmdOut).To(o.ContainSubstring(
			fmt.Sprintf("warning: No channels known to be compatible with the current version \"%s\"; unable to vali"+
				"date \"unknown-channel\". Setting the update channel to \"unknown-channel\" anyway.", version)))
		cmdOut, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(cmdOut).To(o.ContainSubstring("Channel: unknown-channel"))

		g.By("Clear an unknown channel when available channels are not present")
		cmdOut, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade", "channel").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(cmdOut).To(o.ContainSubstring(
			"warning: Clearing channel \"unknown-channel\"; cluster will no longer request available update recommendations."))
		cmdOut, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(cmdOut).To(o.ContainSubstring("NoChannel"))

		// Prerequisite: a dummy update server is ready and the available channels is present
		g.By("Change to a dummy update server")
		_, err = ocJSONPatch(oc, "", "clusterversion/version", []JSONp{
			{"add", "/spec/upstream", graphURL},
			{"add", "/spec/channel", "channel-a"},
		})
		o.Expect(err).NotTo(o.HaveOccurred())
		exec.Command("bash", "-c", "sleep 5").Output()
		cmdOut, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(cmdOut).To(o.ContainSubstring("Channel: channel-a (available channels: channel-a, channel-b)"))

		g.By("Specify multiple channels")
		cmdOut, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade", "channel", "channel-a", "channel-b").Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(cmdOut).To(o.ContainSubstring(
			"error: multiple positional arguments given\nSee 'oc adm upgrade channel -h' for help and examples"))

		g.By("Set a channel which is same as the current channel")
		cmdOut, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade", "channel", "channel-a").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(cmdOut).To(o.ContainSubstring("info: Cluster is already in channel-a (no change)"))

		g.By("Clear a known channel which is in the available channels without --allow-explicit-channel")
		cmdOut, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade", "channel").Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(cmdOut).To(o.ContainSubstring(
			"error: You are requesting to clear the update channel. The current channel \"channel-a\" is " +
				"one of the available channels, you must pass --allow-explicit-channel to continue"))

		g.By("Clear a known channel which is in the available channels with --allow-explicit-channel")
		cmdOut, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade", "channel", "--allow-explicit-channel").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(cmdOut).To(o.ContainSubstring(
			"warning: Clearing channel \"channel-a\"; cluster will no longer request available update recommendations."))
		cmdOut, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(cmdOut).To(o.ContainSubstring("NoChannel"))

		g.By("Re-clear the channel")
		cmdOut, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade", "channel").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(cmdOut).To(o.ContainSubstring("info: Cluster channel is already clear (no change)"))

		g.By("Set to an unknown channel when the available channels are not present without --allow-explicit-channel")
		cmdOut, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade", "channel", "channel-d").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		exec.Command("bash", "-c", "sleep 5").Output()
		o.Expect(cmdOut).To(o.ContainSubstring(
			fmt.Sprintf("warning: No channels known to be compatible with the current version \"%s\"; unable to vali"+
				"date \"channel-d\". Setting the update channel to \"channel-d\" anyway.", version)))
		cmdOut, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(cmdOut).To(o.ContainSubstring("Channel: channel-d (available channels: channel-a, channel-b)"))

		g.By("Set to an unknown channel which is not in the available channels without --allow-explicit-channel")
		cmdOut, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade", "channel", "channel-f").Output()
		o.Expect(err).To(o.HaveOccurred())
		exec.Command("bash", "-c", "sleep 5").Output()
		o.Expect(cmdOut).To(o.ContainSubstring(
			"error: the requested channel \"channel-f\" is not one of the avail" +
				"able channels (channel-a, channel-b), you must pass --allow-explicit-channel to continue"))

		g.By("Set to an unknown channel which is not in the available channels with --allow-explicit-channel")
		cmdOut, err = oc.AsAdmin().WithoutNamespace().Run("adm").
			Args("upgrade", "channel", "channel-f", "--allow-explicit-channel").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		exec.Command("bash", "-c", "sleep 5").Output()
		o.Expect(cmdOut).To(o.ContainSubstring(
			"warning: The requested channel \"channel-f\" is not one of the avail" +
				"able channels (channel-a, channel-b). You have used --allow-explicit-cha" +
				"nnel to proceed anyway. Setting the update channel to \"channel-f\"."))
		cmdOut, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(cmdOut).To(o.ContainSubstring("Channel: channel-f (available channels: channel-a, channel-b)"))

		g.By("Clear an unknown channel which is not in the available channels without --allow-explicit-channel")
		cmdOut, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade", "channel").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(cmdOut).To(o.ContainSubstring(
			"warning: Clearing channel \"channel-f\"; cluster will no longer request available update recommendations."))
		cmdOut, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(cmdOut).To(o.ContainSubstring("NoChannel"))

		g.By("Set to a known channel when the available channels are not present")
		cmdOut, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade", "channel", "channel-a").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		exec.Command("bash", "-c", "sleep 5").Output()
		o.Expect(cmdOut).To(o.ContainSubstring(
			fmt.Sprintf("warning: No channels known to be compatible with the current version \"%s\"; un"+
				"able to validate \"channel-a\". Setting the update channel to \"channel-a\" anyway.", version)))
		cmdOut, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(cmdOut).To(o.ContainSubstring("Channel: channel-a (available channels: channel-a, channel-b)"))

		g.By("Set to a known channel without --allow-explicit-channel")
		_, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade", "channel", "channel-b").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		exec.Command("bash", "-c", "sleep 5").Output()
		cmdOut, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(cmdOut).To(o.ContainSubstring("Channel: channel-b (available channels: channel-a, channel-b)"))
	})

	//author: yanyang@redhat.com
	g.It("Author:yanyang-High-42543-the removed resources are not created in a fresh installed cluster", func() {
		g.By("Check the annotation delete:true for imagestream/hello-openshift is set in manifest")
		tempDataDir, err := extractManifest(oc)
		defer os.RemoveAll(tempDataDir)
		o.Expect(err).NotTo(o.HaveOccurred())
		manifestDir := filepath.Join(tempDataDir, "manifest")
		out, _ := exec.Command("bash", "-c", fmt.Sprintf("grep -rl \"name: hello-openshift\" %s", manifestDir)).Output()
		o.Expect(string(out)).NotTo(o.BeEmpty())
		file := strings.TrimSpace(string(out))
		cmd := fmt.Sprintf("grep -A5 'name: hello-openshift' %s | grep 'release.openshift.io/delete: \"true\"'", file)
		result, _ := exec.Command("bash", "-c", cmd).Output()
		o.Expect(string(result)).NotTo(o.BeEmpty())

		g.By("Check imagestream hello-openshift not present in a fresh installed cluster")
		cmdOut, err := oc.AsAdmin().WithoutNamespace().Run("get").
			Args("imagestream", "hello-openshift", "-n", "openshift").Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(cmdOut).To(o.ContainSubstring(
			"Error from server (NotFound): imagestreams.image.openshift.io \"hello-openshift\" not found"))
	})

	//author: yanyang@redhat.com
	g.It("ConnectedOnly-Author:yanyang-Medium-43172-get the upstream and channel info by using oc adm upgrade [Serial]", func() {
		g.By("Check if it's a GCP cluster")
		platformType, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.type}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.ToLower(platformType) != "gcp" {
			g.Skip("Skip for non-gcp cluster!")
		}

		orgUpstream, _ := getCVObyJP(oc, ".spec.upstream")
		orgChannel, _ := getCVObyJP(oc, ".spec.channel")

		e2e.Logf("Original upstream:%s, original channel:%s", orgUpstream, orgChannel)

		defer restoreCVSpec(orgUpstream, orgChannel, oc)

		g.By("Check when upstream is unset")
		if orgUpstream != "" {
			_, err := ocJSONPatch(oc, "", "clusterversion/version", []JSONp{{"remove", "/spec/upstream", nil}})
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		cmdOut, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(cmdOut).To(o.ContainSubstring("Upstream is unset, so the cluster will use an appropriate default."))
		o.Expect(cmdOut).To(o.ContainSubstring(fmt.Sprintf("Channel: %s", orgChannel)))

		desiredChannel, err := getCVObyJP(oc, ".status.desired.channels")

		o.Expect(err).NotTo(o.HaveOccurred())
		if desiredChannel == "" {
			o.Expect(cmdOut).NotTo(o.ContainSubstring("available channels:"))
		} else {
			msg := "available channels: "
			desiredChannel = desiredChannel[1 : len(desiredChannel)-1]
			splits := strings.Split(desiredChannel, ",")
			for _, split := range splits {
				split = strings.Trim(split, "\"")
				msg = msg + split + ", "
			}
			msg = msg[:len(msg)-2]

			o.Expect(cmdOut).To(o.ContainSubstring(msg))
		}

		g.By("Check when upstream is set")
		projectID := "openshift-qe"
		ctx := context.Background()
		client, err := storage.NewClient(ctx)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer client.Close()

		graphURL, bucket, object, targetVersion, targetPayload, err := buildGraph(client, oc, projectID, "cincy.json")
		defer DeleteBucket(client, bucket)
		defer DeleteObject(client, bucket, object)
		o.Expect(err).NotTo(o.HaveOccurred())

		_, err = ocJSONPatch(oc, "", "clusterversion/version", []JSONp{
			{"add", "/spec/upstream", graphURL},
			{"add", "/spec/channel", "channel-a"},
		})
		o.Expect(err).NotTo(o.HaveOccurred())
		exec.Command("bash", "-c", "sleep 5").Output()
		cmdOut, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		expStr := []string{
			fmt.Sprintf("Upstream: %s", graphURL),
			"Channel: channel-a (available channels: channel-a, channel-b)",
			"Recommended updates:",
			targetVersion,
			targetPayload}

		for _, v := range expStr {
			o.Expect(cmdOut).To(o.ContainSubstring(v))
		}

		cmdOut, err = oc.AsAdmin().WithoutNamespace().Run("adm").
			Args("upgrade", "--include-not-recommended").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(cmdOut).To(o.ContainSubstring(
			"No updates which are not recommended based on your cluster configuration are available"))

		g.By("Check when channel is unset")
		_, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade", "channel", "--allow-explicit-channel").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		cmdOut, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		expStr = []string{
			"Upstream:",
			"Channel:",
			"Reason: NoChannel",
			"Message: The update channel has not been configured"}

		for _, v := range expStr[:2] {
			o.Expect(cmdOut).NotTo(o.ContainSubstring(v))
		}

		for _, v := range expStr[2:] {
			o.Expect(cmdOut).To(o.ContainSubstring(v))
		}
	})

	//author: jiajliu@redhat.com
	g.It("Longduration-NonPreRelease-Author:jiajliu-Medium-41728-cvo alert ClusterOperatorDegraded on degraded operators [Disruptive][Slow]", func() {

		testDataDir := exutil.FixturePath("testdata", "ota/cvo")
		badOauthFile := filepath.Join(testDataDir, "bad-oauth.yaml")

		g.By("Get goodOauthFile from the initial oauth yaml file to oauth-41728.yaml")
		goodOauthFile, err := oc.AsAdmin().WithoutNamespace().Run("get").
			Args("oauth", "cluster", "-o", "yaml").OutputToFile("oauth-41728.yaml")
		defer os.RemoveAll(goodOauthFile)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Prune goodOauthFile")
		oauthfile, err := exec.Command("bash", "-c",
			fmt.Sprintf("sed -i \"/resourceVersion/d\" %s && cat %s", goodOauthFile, goodOauthFile)).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(oauthfile).NotTo(o.ContainSubstring("resourceVersion"))

		g.By("Enable ClusterOperatorDegraded alert")
		err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", badOauthFile).Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", goodOauthFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check ClusterOperatorDegraded condition...")
		err = waitForCondition(60, 300, "True",
			"oc get co authentication -ojson|jq -r '.status.conditions[]|select(.type==\"Degraded\").status'")
		exutil.AssertWaitPollNoErr(err, "authentication operator is not degraded in 5m")

		g.By("Check ClusterOperatorDown alert is not firing and ClusterOperatorDegraded alert is fired correctly.")
		err = wait.Poll(5*time.Minute, 30*time.Minute, func() (bool, error) {
			alertDown := getAlertByName(oc, "ClusterOperatorDown")
			alertDegraded := getAlertByName(oc, "ClusterOperatorDegraded")
			o.Expect(alertDown).To(o.BeNil())
			if alertDegraded == nil || alertDegraded["state"] != "firing" {
				e2e.Logf("Waiting for alert ClusterOperatorDegraded to be triggered and fired...")
				return false, nil
			}
			o.Expect(alertDegraded["labels"].(map[string]interface{})["severity"].(string)).To(o.Equal("warning"))
			o.Expect(alertDegraded["labels"].(map[string]interface{})["namespace"].(string)).To(o.Equal("openshift-cluster-version"))
			o.Expect(alertDegraded["annotations"].(map[string]interface{})["summary"].(string)).
				To(o.ContainSubstring("Cluster operator has been degraded for 30 minutes."))
			o.Expect(alertDegraded["annotations"].(map[string]interface{})["description"].(string)).
				To(o.ContainSubstring("The authentication operator is degraded"))
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "ClusterOperatorDegraded alert is not fired in 30m")

		g.By("Disable ClusterOperatorDegraded alert")
		err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", goodOauthFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check alert is disabled")
		err = wait.Poll(10*time.Second, 60*time.Second, func() (bool, error) {
			alertDegraded := getAlertByName(oc, "ClusterOperatorDegraded")
			if alertDegraded != nil {
				e2e.Logf("Waiting for alert being disabled...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "alert is not disabled.")
	})

	//author: jiajliu@redhat.com
	g.It("Longduration-NonPreRelease-Author:jiajliu-Medium-41778-ClusterOperatorDown and ClusterOperatorDegradedon alerts when unset conditions [Slow]", func() {

		testDataDir := exutil.FixturePath("testdata", "ota/cvo")
		badOauthFile := filepath.Join(testDataDir, "co-test.yaml")

		g.By("Enable alerts")
		err := oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", badOauthFile).Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("co", "test").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check operator's condition...")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co", "test", "-o=jsonpath={.status}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.Equal(""))

		g.By("Waiting for alerts triggered...")
		err = wait.Poll(30*time.Second, 180*time.Second, func() (bool, error) {
			alertDown := getAlertByName(oc, "ClusterOperatorDown")
			alertDegraded := getAlertByName(oc, "ClusterOperatorDegraded")
			if alertDown == nil || alertDegraded == nil {
				e2e.Logf("Waiting for alerts to be triggered...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "No alert triggerred!")

		g.By("Check alert ClusterOperatorDown fired.")
		err = wait.Poll(5*time.Minute, 10*time.Minute, func() (bool, error) {
			alertDown := getAlertByName(oc, "ClusterOperatorDown")
			if alertDown["state"] != "firing" {
				e2e.Logf("Waiting for alert ClusterOperatorDown to be triggered and fired...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "ClusterOperatorDown alert is not fired in 10m")

		g.By("Check alert ClusterOperatorDegraded fired.")
		err = wait.Poll(5*time.Minute, 20*time.Minute, func() (bool, error) {
			alertDegraded := getAlertByName(oc, "ClusterOperatorDegraded")
			if alertDegraded["state"] != "firing" {
				e2e.Logf("Waiting for alert ClusterOperatorDegraded to be triggered and fired...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "ClusterOperatorDegraded alert is not fired in 30m")

		g.By("Disable alerts")
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("co", "test").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check alerts are disabled...")
		err = wait.Poll(10*time.Second, 60*time.Second, func() (bool, error) {
			alertDown := getAlertByName(oc, "ClusterOperatorDown")
			alertDegraded := getAlertByName(oc, "ClusterOperatorDegraded")
			if alertDown != nil || alertDegraded != nil {
				e2e.Logf("Waiting for alerts being disabled...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "alerts are not disabled.")
	})

	//author: jiajliu@redhat.com
	g.It("Longduration-NonPreRelease-Author:jiajliu-Medium-41736-cvo alert ClusterOperatorDown on unavailable operators [Disruptive][Slow]", func() {
		g.By("Check trustedCA in a live cluster.")
		valueProxyTrustCA, err := oc.AsAdmin().WithoutNamespace().Run("get").
			Args("proxy", "cluster", "-o=jsonpath={.spec.trustedCA.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Enable ClusterOperatorDown alert")
		_, err = ocJSONPatch(oc, "", "proxy/cluster", []JSONp{{"replace", "/spec/trustedCA/name", "osus-ca"}})
		o.Expect(err).NotTo(o.HaveOccurred())
		defer ocJSONPatch(oc, "", "proxy/cluster", []JSONp{{"replace", "/spec/trustedCA/name", valueProxyTrustCA}})

		g.By("Check ClusterOperatorDown condition...")
		err = waitForCondition(60, 300, "False", "oc get co machine-config -ojson|jq -r '.status.conditions[]|select(.type==\"Available\").status'")
		exutil.AssertWaitPollNoErr(err, "machine-config operator is not down in 5m")

		g.By("Check ClusterOperatorDown alert is fired correctly")
		err = wait.Poll(100*time.Second, 600*time.Second, func() (bool, error) {
			alertDown := getAlertByName(oc, "ClusterOperatorDown")
			if alertDown == nil || alertDown["state"] != "firing" {
				e2e.Logf("Waiting for alert ClusterOperatorDown to be triggered and fired...")
				return false, nil
			}
			o.Expect(alertDown["labels"].(map[string]interface{})["severity"].(string)).To(o.Equal("critical"))
			o.Expect(alertDown["labels"].(map[string]interface{})["namespace"].(string)).To(o.Equal("openshift-cluster-version"))
			o.Expect(alertDown["annotations"].(map[string]interface{})["summary"].(string)).
				To(o.ContainSubstring("Cluster operator has not been available for 10 minutes."))
			o.Expect(alertDown["annotations"].(map[string]interface{})["description"].(string)).
				To(o.ContainSubstring("The machine-config operator may be down or disabled"))
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "ClusterOperatorDown alert is not fired in 10m")

		g.By("Disable ClusterOperatorDown alert")
		_, err = ocJSONPatch(oc, "", "proxy/cluster", []JSONp{{"replace", "/spec/trustedCA/name", valueProxyTrustCA}})
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check alert is disabled")
		err = wait.Poll(30*time.Second, 180*time.Second, func() (bool, error) {
			alertDown := getAlertByName(oc, "ClusterOperatorDown")
			if alertDown != nil {
				e2e.Logf("Waiting for alert being disabled...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "alert is not disabled.")
	})

	//author: jiajliu@redhat.com
	g.It("Author:jiajliu-Low-46922-check runlevel in cvo ns", func() {
		g.By("Check runlevel in cvo namespace.")
		runLevel, err := oc.AsAdmin().WithoutNamespace().Run("get").
			Args("ns", "openshift-cluster-version",
				"-o=jsonpath={.metadata.labels.openshift\\.io/run-level}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(runLevel).To(o.Equal(""))

		g.By("Check scc of cvo pod.")
		podName, err := oc.AsAdmin().WithoutNamespace().Run("get").
			Args("pod", "-n", "openshift-cluster-version", "-oname").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		scc, err := oc.AsAdmin().WithoutNamespace().Run("get").
			Args("-n", "openshift-cluster-version", podName,
				"-o=jsonpath={.metadata.annotations.openshift\\.io/scc}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(scc).To(o.Equal("hostaccess"))
	})

	//author: yanyang@redhat.com
	g.It("Author:yanyang-Medium-46724-cvo defaults deployment replicas to one if it's unset in manifest [Flaky]", func() {
		g.By("Check the replicas for openshift-insights/insights-operator is unset in manifest")
		tempDataDir, err := extractManifest(oc)
		defer os.RemoveAll(tempDataDir)
		o.Expect(err).NotTo(o.HaveOccurred())
		manifestDir := filepath.Join(tempDataDir, "manifest")
		namespace, name := "openshift-insights", "insights-operator"
		cmd := fmt.Sprintf(
			"grep -rlZ 'kind: Deployment' %s | xargs -0 grep -l 'name: %s\\|namespace: %s' | xargs grep replicas",
			manifestDir, name, namespace)
		e2e.Logf(cmd)
		out, _ := exec.Command("bash", "-c", cmd).Output()
		o.Expect(out).To(o.BeEmpty())

		g.By("Check only one insights-operator pod in a fresh installed cluster")
		num, err := oc.AsAdmin().WithoutNamespace().Run("get").
			Args("deployment", name,
				"-o=jsonpath={.spec.replicas}", "-n", namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(num).To(o.Equal("1"))

		defer oc.AsAdmin().WithoutNamespace().Run("scale").
			Args("--replicas", "1",
				fmt.Sprintf("deployment/%s", name),
				"-n", namespace).Output()

		g.By("Scale down insights-operator replica to 0")
		_, err = oc.AsAdmin().WithoutNamespace().Run("scale").
			Args("--replicas", "0",
				fmt.Sprintf("deployment/%s", name),
				"-n", namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check the insights-operator replica recovers to one")
		err = wait.Poll(30*time.Second, 5*time.Minute, func() (bool, error) {
			num, err = oc.AsAdmin().WithoutNamespace().Run("get").
				Args("deployment", name,
					"-o=jsonpath={.spec.replicas}",
					"-n", namespace).Output()
			if num != "1" {
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "insights-operator replicas is not 1")

		g.By("Scale up insights-operator replica to 2")
		_, err = oc.AsAdmin().WithoutNamespace().Run("scale").
			Args("--replicas", "2",
				fmt.Sprintf("deployment/%s", name),
				"-n", namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check the insights-operator replica recovers to one")
		err = wait.Poll(30*time.Second, 5*time.Minute, func() (bool, error) {
			num, err = oc.AsAdmin().WithoutNamespace().Run("get").
				Args("deployment", name,
					"-o=jsonpath={.spec.replicas}",
					"-n", namespace).Output()
			if num != "1" {
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "insights-operator replicas is not 1")
	})

	//author: jiajliu@redhat.com
	g.It("Author:jiajliu-Medium-47198-Techpreview operator will not be installed on a fresh installed", func() {
		tpOperatorNamespace := "openshift-cluster-api"
		tpOperatorName := "cluster-api"

		featuregate, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("featuregate", "cluster", "-o=jsonpath={.spec}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Featuregate:%s", featuregate)
		if featuregate != "{}" && strings.Contains(featuregate, "TechPreviewNoUpgrade") {
			g.Skip("This case is only suitable for non-techpreview cluster!")
		}
		g.By("Check annotation release.openshift.io/feature-gate=TechPreviewNoUpgrade in manifests are correct.")
		tempDataDir, err := extractManifest(oc)
		defer os.RemoveAll(tempDataDir)
		o.Expect(err).NotTo(o.HaveOccurred())
		manifestDir := filepath.Join(tempDataDir, "manifest")
		featuregateTotalNum, _ := exec.Command("bash", "-c", fmt.Sprintf(
			"grep -r 'release.openshift.io/feature-gate' %s|wc -l", manifestDir)).Output()
		featuregateNoUpgradeNum, _ := exec.Command("bash", "-c", fmt.Sprintf(
			"grep -r 'release.openshift.io/feature-gate: .*TechPreviewNoUpgrade.*' %s|wc -l", manifestDir)).Output()
		o.Expect(featuregateNoUpgradeNum).To(o.Equal(featuregateTotalNum))

		g.By("Check no TP operator cluster-api installed by default.")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ns", tpOperatorNamespace).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("NotFound"))
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("co", tpOperatorName).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("NotFound"))
	})

	//author: yanyang@redhat.com
	g.It("Author:yanyang-Medium-47757-cvo respects the deployment strategy in manifests [Serial]", func() {
		g.By("Get the strategy for openshift-insights/insights-operator in manifest")
		tempDataDir, err := extractManifest(oc)
		defer os.RemoveAll(tempDataDir)
		o.Expect(err).NotTo(o.HaveOccurred())
		manifestDir := filepath.Join(tempDataDir, "manifest")
		namespace, name := "openshift-insights", "insights-operator"
		cmd := fmt.Sprintf(
			"grep -rlZ 'kind: Deployment' %s | xargs -0 grep -l 'name: %s' | xargs grep strategy -A1 | sed -n 2p | cut -f2 -d ':'",
			manifestDir, name)
		e2e.Logf(cmd)
		out, _ := exec.Command("bash", "-c", cmd).Output()
		o.Expect(out).NotTo(o.BeEmpty())
		expectStrategy := strings.TrimSpace(string(out))
		e2e.Logf(expectStrategy)

		g.By("Check in-cluster insights-operator has the same strategy with manifest")
		existStrategy, err := oc.AsAdmin().WithoutNamespace().Run("get").
			Args("deployment", name,
				"-o=jsonpath={.spec.strategy}",
				"-n", namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(existStrategy).To(o.ContainSubstring(expectStrategy))

		g.By("Change the strategy")
		var patch []JSONp
		if expectStrategy == "Recreate" {
			patch = []JSONp{{"replace", "/spec/strategy/type", "RollingUpdate"}}
		} else {
			patch = []JSONp{
				{"remove", "/spec/strategy/rollingUpdate", nil},
				{"replace", "/spec/strategy/type", "Recreate"},
			}
		}
		_, err = ocJSONPatch(oc, namespace, fmt.Sprintf("deployment/%s", name), patch)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check the strategy reverted after 5 minutes")
		if pollErr := wait.Poll(30*time.Second, 5*time.Minute, func() (bool, error) {
			curStrategy, _ := oc.AsAdmin().WithoutNamespace().Run("get").
				Args("deployment", name, "-o=jsonpath={.spec.strategy}", "-n", namespace).Output()
			if strings.Contains(string(curStrategy), expectStrategy) {
				return true, nil
			}
			return false, nil
		}); pollErr != nil {
			//If the strategy is not reverted, manually change it back
			if expectStrategy == "Recreate" {
				patch = []JSONp{
					{"remove", "/spec/strategy/rollingUpdate", nil},
					{"replace", "/spec/strategy/type", "Recreate"},
				}
			} else {
				patch = []JSONp{{"replace", "/spec/strategy/type", "RollingUpdate"}}
			}
			_, err = ocJSONPatch(oc, namespace, fmt.Sprintf("deployment/%s", name), patch)
			o.Expect(err).NotTo(o.HaveOccurred())
			exutil.AssertWaitPollNoErr(pollErr, "Strategy is not reverted back after 5 minutes")
		}
	})

	//author: evakhoni@redhat.com
	g.It("Longduration-NonPreRelease-Author:evakhoni-Medium-48247-Prometheus is able to scrape metrics from the CVO after rotation of the signer ca in openshift-service-ca [Disruptive]", func() {

		g.By("Check for alerts Before signer ca rotation.")
		alertCVODown := getAlertByName(oc, "ClusterVersionOperatorDown")
		alertTargetDown := getAlert(oc, ".labels.alertname == \"TargetDown\" and .labels.service == \"cluster-version-operator\"")
		o.Expect(alertCVODown).To(o.BeNil())
		o.Expect(alertTargetDown).To(o.BeNil())

		g.By("Force signer ca rotation by deleting signing-key.")
		result, err := oc.AsAdmin().WithoutNamespace().Run("delete").
			Args("secret/signing-key", "-n", "openshift-service-ca").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf(result)
		o.Expect(result).To(o.ContainSubstring("deleted"))

		g.By("Check new signing-key is recreated")
		// supposed to fail until available so suppressing stderr and return code
		err = waitForCondition(3, 30, "signing-key",
			"oc -n openshift-service-ca get secret/signing-key -ojsonpath='{.metadata.name}' 2>/dev/null; :")
		exutil.AssertWaitPollNoErr(err, "signing-key not recreated within 30s")

		g.By("Wait for Prometheus route to be available")
		// firstly wait until route is unavailable
		err = wait.Poll(3*time.Second, 30*time.Second, func() (bool, error) {
			_, cmderr := exec.Command("bash", "-c", "oc get route prometheus-k8s -n openshift-monitoring").Output()
			if cmderr != nil {
				// oc get route returns "exit status 1" once unavailable
				o.Expect(cmderr.Error()).To(o.ContainSubstring("exit status 1"))
				return true, nil
			}
			return false, nil
		})
		if err != nil {
			// sometimes route stays available, won't impact rest of the test
			o.Expect(err.Error()).To(o.ContainSubstring("timed out waiting for the condition"))
		}
		// wait until available again
		// supposed to fail until available so suppressing stderr and return code
		err = waitForCondition(10, 600, "True",
			"oc get route prometheus-k8s -n openshift-monitoring -o"+
				"jsonpath='{.status.ingress[].conditions[].status}' 2>/dev/null; :")
		exutil.AssertWaitPollNoErr(err, "Prometheus route is unavailable for 10m")

		g.By("Check CVO accessable by Prometheus - After signer ca rotation.")
		seenAlertCVOd, seenAlertTD := false, false
		// alerts may appear within first 5 minutes, and fire after 10 more mins
		err = wait.Poll(1*time.Minute, 15*time.Minute, func() (bool, error) {
			alertCVODown = getAlertByName(oc, "ClusterVersionOperatorDown")
			alertTargetDown = getAlert(oc, ".labels.alertname == \"TargetDown\" and .labels.service == \"cluster-version-operator\"")
			if alertCVODown != nil {
				e2e.Logf("alert ClusterVersionOperatorDown found - checking state..")
				o.Expect(alertCVODown["state"]).NotTo(o.Equal("firing"))
				seenAlertCVOd = true
			}
			if alertTargetDown != nil {
				e2e.Logf("alert TargetDown for CVO found - checking state..")
				o.Expect(alertTargetDown["state"]).NotTo(o.Equal("firing"))
				seenAlertTD = true
			}
			if alertCVODown == nil && alertTargetDown == nil {
				if seenAlertCVOd && seenAlertTD {
					e2e.Logf("alerts pended and disappeared. success.")
					return true, nil
				}
			}
			return false, nil
		})
		if err != nil {
			o.Expect(err.Error()).To(o.ContainSubstring("timed out waiting for the condition"))
		}
	})

	//author: evakhoni@redhat.com
	g.It("Author:evakhoni-Low-21771-Upgrade cluster when current version is not in the graph from upstream [Serial]", func() {
		var graphURL, bucket, object, targetVersion, targetPayload string
		origVersion, err := getCVObyJP(oc, ".status.desired.version")
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check if upstream patch required")
		jsonpath := ".status.conditions[?(.type=='RetrievedUpdates')].reason"
		reason, err := getCVObyJP(oc, jsonpath)
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(reason, "VersionNotFound") {
			e2e.Logf("no patch required. skipping upstream creation")
			targetVersion = GenerateReleaseVersion(oc)
			targetPayload = GenerateReleasePayload(oc)
		} else {
			g.By("Check if it's a GCP cluster")
			platformType, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.type}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.ToLower(platformType) != "gcp" {
				g.Skip("Skip for non-gcp cluster!")
			}
			origUpstream, _ := getCVObyJP(oc, ".spec.upstream")
			origChannel, _ := getCVObyJP(oc, ".spec.channel")
			e2e.Logf("Original upstream:%s, original channel:%s", origUpstream, origChannel)
			defer restoreCVSpec(origUpstream, origChannel, oc)

			g.By("Patch upstream")
			projectID := "openshift-qe"
			ctx := context.Background()
			client, err := storage.NewClient(ctx)
			o.Expect(err).NotTo(o.HaveOccurred())
			defer client.Close()

			graphURL, bucket, object, targetVersion, targetPayload, err = buildGraph(
				client, oc, projectID, "cincy-source-not-in-graph.json")
			defer DeleteBucket(client, bucket)
			defer DeleteObject(client, bucket, object)
			o.Expect(err).NotTo(o.HaveOccurred())

			_, err = ocJSONPatch(oc, "", "clusterversion/version", []JSONp{
				{"add", "/spec/upstream", graphURL},
				{"add", "/spec/channel", "channel-a"},
			})
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Check RetrievedUpdates reason VersionNotFound after patching upstream")
			jsonpath = ".status.conditions[?(.type=='RetrievedUpdates')].reason"
			err = wait.Poll(5*time.Second, 15*time.Second, func() (bool, error) {
				reason, err := getCVObyJP(oc, jsonpath)
				o.Expect(err).NotTo(o.HaveOccurred())
				e2e.Logf("received reason: '%s'", reason)
				if strings.Contains(reason, "VersionNotFound") {
					return true, nil
				}
				return false, nil
			})
			exutil.AssertWaitPollNoErr(err, "Failed to check RetrievedUpdates!=True")
		}

		g.By("Give appropriate error on oc adm upgrade --to")
		toOutput, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade", "--to", targetVersion).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(toOutput).To(o.ContainSubstring("Unable to retrieve available updates"))
		o.Expect(toOutput).To(o.ContainSubstring("specify --to-image to continue with the update"))

		g.By("Give appropriate error on oc adm upgrade --to-image")
		toImageOutput, err := oc.AsAdmin().WithoutNamespace().Run("adm").
			Args("upgrade", "--to-image", targetPayload).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(toImageOutput).To(o.ContainSubstring("Unable to retrieve available updates"))
		o.Expect(toImageOutput).To(o.ContainSubstring("specify --allow-explicit-upgrade to continue with the update"))

		g.By("Find enable-auto-update index in deployment")
		origAutoState, autoUpdIndex, err := getCVOcontArg(oc, "enable-auto-update")
		o.Expect(err).NotTo(o.HaveOccurred())
		defer patchCVOcontArg(oc, autoUpdIndex, fmt.Sprintf("--enable-auto-update=%s", origAutoState))
		_, err = patchCVOcontArg(oc, autoUpdIndex, "--enable-auto-update=true")
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Wait for enable-auto-update")
		err = wait.PollImmediate(2*time.Second, 10*time.Second, func() (bool, error) {
			depArgs, _, err := getCVOcontArg(oc, "enable-auto-update")
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(depArgs, "true") {
				//e2e.Logf(depArgs)
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "Failed waiting for enable-auto-update=true")

		g.By("Check cvo can not get available update after setting enable-auto-update")
		jsonpath = ".status.conditions[?(.type=='RetrievedUpdates')].reason"
		err = wait.Poll(5*time.Second, 15*time.Second, func() (bool, error) {
			reason, err := getCVObyJP(oc, jsonpath)
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(reason, "VersionNotFound") {
				e2e.Logf("success - found reason: %s", reason)
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "Failed to check cvo can not get available update")

		g.By("Check availableUpdates is null")
		availableUpdates, err := getCVObyJP(oc, ".status.availableUpdates")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(availableUpdates).To(o.Equal("<nil>"))

		g.By("Check desired version haven't changed")
		desiredVersion, err := getCVObyJP(oc, ".status.desired.version")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(desiredVersion).To(o.Equal(origVersion))

	})

	//author: jiajliu@redhat.com
	g.It("Longduration-NonPreRelease-Author:jiajliu-High-46017-CVO should keep reconcile manifests when update failed on precondition check [Disruptive]", func() {
		//Take openshift-marketplace/deployment as an example, it can be any resource which included in manifest files
		resourceKindName := "deployment/marketplace-operator"
		resourceNamespace := "openshift-marketplace"
		g.By("Check default rollingUpdate strategy in a fresh installed cluster.")
		defaultValueMaxUnavailable, err := oc.AsAdmin().WithoutNamespace().Run("get").
			Args(resourceKindName, "-o=jsonpath={.spec.strategy.rollingUpdate.maxUnavailable}",
				"-n", resourceNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(defaultValueMaxUnavailable).To(o.Equal("25%"))

		g.By("Ensure upgradeable=false.")
		upgStatusOutput, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(upgStatusOutput, "Upgradeable=False") {
			e2e.Logf("Enable upgradeable=false explicitly...")
			//set overrides in cv to trigger upgradeable=false condition if it is not enabled by default
			err = setCVOverrides(oc, "deployment", "network-operator", "openshift-network-operator")
			defer unsetCVOverrides(oc)
			exutil.AssertWaitPollNoErr(err, "timeout to set overrides!")
		}

		g.By("Trigger update when upgradeable=false and precondition check fail.")
		//Choose a fixed old release payload to trigger a fake upgrade when upgradeable=false
		oldReleasePayload := "quay.io/openshift-release-dev/ocp-release@sha256:fd96300600f9585e5847f5855ca14e2b3cafbce12aefe3b3f52c5da10c4476eb"
		err = oc.AsAdmin().WithoutNamespace().Run("adm").
			Args("upgrade", "--allow-explicit-upgrade", "--to-image", oldReleasePayload).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade", "--clear").Execute()

		err = waitForCondition(30, 120, "False",
			"oc get clusterversion version -ojson|jq -r '.status.conditions[]|select(.type==\"ReleaseAccepted\").status'")
		exutil.AssertWaitPollNoErr(err, "ReleaseAccepted condition is not false in 3m")

		g.By("Change strategy.rollingUpdate.maxUnavailable to be 50%.")
		_, err = ocJSONPatch(oc, resourceNamespace, resourceKindName, []JSONp{
			{"replace", "/spec/strategy/rollingUpdate/maxUnavailable", "50%"},
		})
		o.Expect(err).NotTo(o.HaveOccurred())
		defer ocJSONPatch(oc, resourceNamespace, resourceKindName, []JSONp{
			{"replace", "/spec/strategy/rollingUpdate/maxUnavailable", "25%"},
		})

		g.By("Check the deployment was reconciled back.")
		err = wait.Poll(30*time.Second, 5*time.Minute, func() (bool, error) {
			valueMaxUnavailable, _ := oc.AsAdmin().WithoutNamespace().Run("get").
				Args(resourceKindName, "-o=jsonpath={.spec.strategy.rollingUpdate.maxUnavailable}", "-n", resourceNamespace).Output()
			if strings.Compare(valueMaxUnavailable, defaultValueMaxUnavailable) != 0 {
				e2e.Logf("valueMaxUnavailable is %v. Waiting for deployment being reconciled...", valueMaxUnavailable)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "the deployment was not reconciled back in 5min.")
	})

	//author: yanyang@redhat.com
	g.It("Author:yanyang-Medium-49507-disable capability by removing cap from cv.spec.capabilities.additionalEnabledCapabilities [Serial]", func() {
		orgBaseCap, err := getCVObyJP(oc, ".spec.capabilities.baselineCapabilitySet")
		o.Expect(err).NotTo(o.HaveOccurred())
		orgAddCapstr, err := getCVObyJP(oc, ".spec.capabilities.additionalEnabledCapabilities[*]")
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf(orgBaseCap, orgAddCapstr)

		orgAddCap := strings.Split(orgAddCapstr, " ")

		if orgBaseCap != "None" || len(orgAddCap) < 1 {
			g.Skip("The test requires baselineCapabilitySet=None and at least 1 additional enabled caps")
		}

		defer changeCap(oc, false, orgAddCap)

		g.By("Check cap status and condition prior to change")
		enabledCap, err := getCVObyJP(oc, ".status.capabilities.enabledCapabilities[*]")
		o.Expect(err).NotTo(o.HaveOccurred())

		for _, op := range orgAddCap {
			_, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("co", strings.ToLower(op)).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		status, err := getCVObyJP(oc, ".status.conditions[?(.type=='ImplicitlyEnabledCapabilities')].status")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(status).To(o.Equal("False"))

		capSet := make([]string, len(orgAddCap))
		copy(capSet, orgAddCap)
		loop := 1
		if len(orgAddCap) > 1 {
			loop = 2
		}
		r := rand.New(rand.NewSource(time.Now().Unix()))
		for i := 0; i < loop; i++ {
			g.By("Disable capabilities by modifying the additionalEnabledCapabilities")
			randIndex := r.Intn(len(capSet))
			delCap := capSet[randIndex]
			e2e.Logf("Disabling cap " + delCap)
			capSet = append(capSet[:randIndex], capSet[randIndex+1:]...)
			cmdOut, err := changeCap(oc, false, capSet)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(cmdOut).NotTo(o.ContainSubstring("no change"))

			g.By("Check cap status and condition after change")
			enabledCapPost, err := getCVObyJP(oc, ".status.capabilities.enabledCapabilities[*]")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(enabledCapPost).To(o.Equal(enabledCap))

			for _, op := range orgAddCap {
				_, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("co", strings.ToLower(op)).Output()
				o.Expect(err).NotTo(o.HaveOccurred())
			}

			for _, k := range []string{"status", "reason", "message"} {
				jsonpath := ".status.conditions[?(.type=='ImplicitlyEnabledCapabilities')]." + k
				out, err := getCVObyJP(oc, jsonpath)
				o.Expect(err).NotTo(o.HaveOccurred())
				if k == "status" {
					o.Expect(out).To(o.Equal("True"))
				} else if k == "reason" {
					o.Expect(out).To(o.Equal("CapabilitiesImplicitlyEnabled"))
				} else {
					msg := []string{"The following capabilities could not be disabled", delCap}
					for _, m := range msg {
						o.Expect(out).To(o.ContainSubstring(m))
					}
				}
			}
		}
	})

	//author: jiajliu@redhat.com
	g.It("Longduration-NonPreRelease-Author:jiajliu-Medium-51973-setting cv.overrides should work while ReleaseAccepted=False [Disruptive]", func() {
		resourceKind := "deployment"
		resourceName := "network-operator"
		resourceNamespace := "openshift-network-operator"

		g.By("Trigger ReleaseAccepted=False condition.")
		fakeReleasePayload := "quay.io/openshift-release-dev-test/ocp-release@sha256:39efe13ef67cb4449f5e6cdd8a26c83c07c6a2ce5d235dfbc3ba58c64418fcf3"
		err := oc.AsAdmin().WithoutNamespace().Run("adm").
			Args("upgrade", "--allow-explicit-upgrade", "--to-image", fakeReleasePayload).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade", "--clear").Execute()

		err = waitForCondition(30, 300, "False",
			"oc get clusterversion version -ojson|jq -r '.status.conditions[]|select(.type==\"ReleaseAccepted\").status'")
		exutil.AssertWaitPollNoErr(err, "ReleaseAccepted condition is not false in 5m")

		g.By("Disable deployment/network-operator's management through setting cv.overrides.")
		err = setCVOverrides(oc, resourceKind, resourceName, resourceNamespace)
		defer unsetCVOverrides(oc)
		exutil.AssertWaitPollNoErr(err, "timeout to set overrides!")

		g.By("Check default rollingUpdate strategy.")
		defaultValueMaxUnavailable, err := oc.AsAdmin().WithoutNamespace().Run("get").
			Args(resourceKind, resourceName, "-o=jsonpath={.spec.strategy.rollingUpdate.maxUnavailable}",
				"-n", resourceNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(defaultValueMaxUnavailable).To(o.Equal("1"))

		g.By("Change strategy.rollingUpdate.maxUnavailable to be 50%.")
		_, err = ocJSONPatch(oc, resourceNamespace, fmt.Sprintf("%s/%s", resourceKind, resourceName), []JSONp{
			{"replace", "/spec/strategy/rollingUpdate/maxUnavailable", "50%"},
		})
		o.Expect(err).NotTo(o.HaveOccurred())
		defer ocJSONPatch(oc, resourceNamespace, fmt.Sprintf("%s/%s", resourceKind, resourceName), []JSONp{
			{"replace", "/spec/strategy/rollingUpdate/maxUnavailable", "1"},
		})

		g.By("Check the deployment will not be reconciled back.")
		err = wait.Poll(30*time.Second, 8*time.Minute, func() (bool, error) {
			valueMaxUnavailable, _ := oc.AsAdmin().WithoutNamespace().Run("get").
				Args(resourceKind, resourceName, "-o=jsonpath={.spec.strategy.rollingUpdate.maxUnavailable}", "-n", resourceNamespace).Output()
			if strings.Compare(valueMaxUnavailable, defaultValueMaxUnavailable) == 0 {
				e2e.Logf("valueMaxUnavailable is %v. Waiting for deployment being reconciled...", valueMaxUnavailable)
				return false, nil
			}
			return true, nil
		})
		if err != nil {
			o.Expect(err.Error()).To(o.ContainSubstring("timed out waiting for the condition"))
		}
	})
})
