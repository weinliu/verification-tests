package cvo

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"github.com/tidwall/gjson"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
	"sigs.k8s.io/yaml"
)

var _ = g.Describe("[sig-updates] OTA cvo should", func() {
	defer g.GinkgoRecover()

	projectName := "openshift-cluster-version"

	oc := exutil.NewCLIWithoutNamespace(projectName)

	//author: dis@redhat.com
	g.It("NonHyperShiftHOST-Author:dis-High-56072-CVO pod should not crash", func() {
		exutil.By("Get CVO container status")
		CVOStatus, err := getCVOPod(oc, ".status.containerStatuses[]")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(CVOStatus).NotTo(o.BeNil())

		exutil.By("Check ready is true")
		o.Expect(CVOStatus["ready"]).To(o.BeTrue(), "CVO is not ready: %v", CVOStatus)

		exutil.By("Check started is true")
		o.Expect(CVOStatus["started"]).To(o.BeTrue(), "CVO is not started: %v", CVOStatus)

		exutil.By("Check state is running")
		o.Expect(CVOStatus["state"]).NotTo(o.BeNil(), "CVO have no state: %v", CVOStatus)
		o.Expect(CVOStatus["state"].(map[string]interface{})["running"]).NotTo(o.BeNil(), "CVO state have no running: %v", CVOStatus)

		exutil.By("Check exitCode of lastState is 0 if lastState is not empty")
		lastState := CVOStatus["lastState"]
		o.Expect(lastState).NotTo(o.BeNil(), "CVO have no lastState: %v", CVOStatus)
		if reflect.ValueOf(lastState).Len() == 0 {
			e2e.Logf("lastState is empty which is expected")
		} else {
			o.Expect(lastState.(map[string]interface{})["terminated"]).NotTo(o.BeNil(), "no terminated for non-empty CVO lastState: %v", CVOStatus)
			exitCode := lastState.(map[string]interface{})["terminated"].(map[string]interface{})["exitCode"].(float64)
			if exitCode == 255 && strings.Contains(
				lastState.(map[string]interface{})["terminated"].(map[string]interface{})["message"].(string),
				"Failed to get FeatureGate from cluster") {
				e2e.Logf("detected a known issue OCPBUGS-13873, skipping lastState check")
			} else {
				o.Expect(exitCode).To(o.BeZero(), "CVO terminated with non-zero code: %v", CVOStatus)
				reason := lastState.(map[string]interface{})["terminated"].(map[string]interface{})["reason"]
				o.Expect(reason.(string)).To(o.Equal("Completed"), "CVO terminated with unexpected reason: %v", CVOStatus)
			}
		}
	})

	//author: dis@redhat.com
	g.It("NonHyperShiftHOST-Author:dis-Medium-49508-disable capabilities by modifying the cv.spec.capabilities.baselineCapabilitySet [Serial]", func() {
		orgBaseCap, err := getCVObyJP(oc, ".spec.capabilities.baselineCapabilitySet")
		o.Expect(err).NotTo(o.HaveOccurred())
		if orgBaseCap == "None" {
			g.Skip("The test cannot run on baselineCapabilitySet None")
		}

		defer func() {
			if newBaseCap, _ := getCVObyJP(oc, ".spec.capabilities.baselineCapabilitySet"); orgBaseCap != newBaseCap {
				var out string
				var err error
				e2e.Logf("restoring original base caps to '%s'", orgBaseCap)
				if orgBaseCap == "" {
					out, err = changeCap(oc, true, nil)
				} else {
					out, err = changeCap(oc, true, orgBaseCap)
				}
				o.Expect(err).NotTo(o.HaveOccurred(), out)
			} else {
				e2e.Logf("defer baselineCapabilitySet skipped for original value already matching '%v'", newBaseCap)
			}
		}()

		exutil.By("Check cap status and condition prior to change")
		enabledCap, err := getCVObyJP(oc, ".status.capabilities.enabledCapabilities[*]")
		o.Expect(err).NotTo(o.HaveOccurred())
		capSet := strings.Split(enabledCap, " ")
		o.Expect(verifyCaps(oc, capSet)).NotTo(o.HaveOccurred())

		out, err := getCVObyJP(oc, ".status.conditions[?(.type=='ImplicitlyEnabledCapabilities')]")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(gjson.Get(out, "status").Bool()).To(o.Equal(false),
			"unexpected status dumping implicit %s", out)
		o.Expect(gjson.Get(out, "reason").String()).To(o.Equal("AsExpected"),
			"unexpected reason dumping implicit %s", out)
		o.Expect(gjson.Get(out, "message").String()).To(o.ContainSubstring("Capabilities match configured spec"),
			"unexpected message dumping implicit %s", out)

		exutil.By("Disable capabilities by modifying the baselineCapabilitySet")
		_, err = changeCap(oc, true, "None")
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Check cap status and condition after change")
		enabledCapPost, err := getCVObyJP(oc, ".status.capabilities.enabledCapabilities[*]")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(enabledCapPost).To(o.Equal(enabledCap))
		o.Expect(verifyCaps(oc, capSet)).NotTo(o.HaveOccurred(), "verifyCaps for enabled %v failed", capSet)

		exutil.By("Check implicitly enabled caps are correct")
		out, err = getCVObyJP(oc, ".status.conditions[?(.type=='ImplicitlyEnabledCapabilities')]")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(gjson.Get(out, "status").Bool()).To(o.Equal(true),
			"unexpected status dumping implicit %s", out)
		o.Expect(gjson.Get(out, "reason").String()).To(o.Equal("CapabilitiesImplicitlyEnabled"),
			"unexpected reason dumping implicit %s", out)
		o.Expect(gjson.Get(out, "message").String()).To(o.ContainSubstring("The following capabilities could not be disabled"),
			"unexpected message dumping implicit %s", out)
		o.Expect(gjson.Get(out, "message").String()).To(o.ContainSubstring(strings.Join(capSet, ", ")),
			"unexpected message dumping implicit %s", out)
	})

	//author: dis@redhat.com
	g.It("NonHyperShiftHOST-Author:dis-Low-49670-change spec.capabilities to invalid value", func() {
		orgCap, err := getCVObyJP(oc, ".spec.capabilities")
		o.Expect(err).NotTo(o.HaveOccurred())
		if orgCap == "" {
			defer func() {
				out, err := ocJSONPatch(oc, "", "clusterversion/version", []JSONp{{"remove", "/spec/capabilities", nil}})
				o.Expect(err).NotTo(o.HaveOccurred(), out)
			}()
		} else {
			orgBaseCap, err := getCVObyJP(oc, ".spec.capabilities.baselineCapabilitySet")
			o.Expect(err).NotTo(o.HaveOccurred())
			orgAddCapstr, err := getCVObyJP(oc, ".spec.capabilities.additionalEnabledCapabilities[*]")
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("original baseline: '%s', original additional: '%s'", orgBaseCap, orgAddCapstr)

			orgAddCap := strings.Split(orgAddCapstr, " ")

			defer func() {
				if newBaseCap, _ := getCVObyJP(oc, ".spec.capabilities.baselineCapabilitySet"); orgBaseCap != newBaseCap {
					var out string
					var err error
					if orgBaseCap == "" {
						out, err = changeCap(oc, true, nil)
					} else {
						out, err = changeCap(oc, true, orgBaseCap)
					}
					o.Expect(err).NotTo(o.HaveOccurred(), out)
				} else {
					e2e.Logf("defer baselineCapabilitySet skipped for original value already matching '%v'", newBaseCap)
				}
			}()
			defer func() {
				if newAddCap, _ := getCVObyJP(oc, ".spec.capabilities.additionalEnabledCapabilities[*]"); !reflect.DeepEqual(orgAddCap, strings.Split(newAddCap, " ")) {
					var out string
					var err error
					if reflect.DeepEqual(orgAddCap, make([]string, 1)) {
						// need this cause strings.Split of an empty string creates len(1) slice which isn't nil
						out, err = changeCap(oc, false, nil)
					} else {
						out, err = changeCap(oc, false, orgAddCap)
					}
					o.Expect(err).NotTo(o.HaveOccurred(), out)
				} else {
					e2e.Logf("defer additionalEnabledCapabilities skipped for original value already matching '%v'", strings.Split(newAddCap, " "))
				}
			}()
		}

		exutil.By("Set invalid baselineCapabilitySet")
		cmdOut, err := changeCap(oc, true, "Invalid")
		o.Expect(err).To(o.HaveOccurred())
		clusterVersion, _, err := exutil.GetClusterVersion(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		version := strings.Split(clusterVersion, ".")
		minor_version := version[1]
		latest_version, err := strconv.Atoi(minor_version)
		o.Expect(err).NotTo(o.HaveOccurred())
		var versions []string
		for i := 11; i <= latest_version; i++ {
			versions = append(versions, "\"v4."+strconv.Itoa(i)+"\"")
		}
		versions = append(versions, "\"vCurrent\"")
		result := "Unsupported value: \"Invalid\": supported values: \"None\", " + strings.Join(versions, ", ")
		o.Expect(cmdOut).To(o.ContainSubstring(result))

		// Important! this one should be updated each version with new capabilities, as they added to openshift.
		exutil.By("Set invalid additionalEnabledCapabilities")
		cmdOut, err = changeCap(oc, false, []string{"Invalid"})
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(cmdOut).To(o.ContainSubstring("Unsupported value: \"Invalid\": supported values: \"openshift-samples\", \"baremetal\", \"marketplace\", \"Console\", \"Insights\", \"Storage\", \"CSISnapshot\", \"NodeTuning\", \"MachineAPI\", \"Build\", \"DeploymentConfig\", \"ImageRegistry\", \"OperatorLifecycleManager\", \"CloudCredential\", \"Ingress\", \"CloudControllerManager\", \"OperatorLifecycleManagerV1\""))
	})

	//author: jianl@redhat.com
	g.It("Longduration-NonPreRelease-ConnectedOnly-Author:jianl-Medium-45879-check update info with oc adm upgrade --include-not-recommended [Serial][Slow]", func() {
		exutil.By("Check if it's a GCP cluster")
		exutil.SkipIfPlatformTypeNot(oc, "gcp")

		orgUpstream, err := getCVObyJP(oc, ".spec.upstream")
		o.Expect(err).NotTo(o.HaveOccurred())
		orgChannel, err := getCVObyJP(oc, ".spec.channel")
		o.Expect(err).NotTo(o.HaveOccurred())

		e2e.Logf("Original upstream: %s, original channel: %s", orgUpstream, orgChannel)

		exutil.By("Patch upstream and channel")
		projectID := "openshift-qe"
		ctx := context.Background()
		client, err := storage.NewClient(ctx)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() { o.Expect(client.Close()).NotTo(o.HaveOccurred()) }()

		graphURL, bucket, object, _, _, err := buildGraph(client, oc, projectID, "cincy-conditional-edge.json")
		defer func() { o.Expect(DeleteBucket(client, bucket)).NotTo(o.HaveOccurred()) }()
		defer func() { o.Expect(DeleteObject(client, bucket, object)).NotTo(o.HaveOccurred()) }()
		o.Expect(err).NotTo(o.HaveOccurred())

		_, err = ocJSONPatch(oc, "", "clusterversion/version", []JSONp{
			{"add", "/spec/upstream", graphURL},
			{"add", "/spec/channel", "stable-a"},
		})
		o.Expect(err).NotTo(o.HaveOccurred())

		defer restoreCVSpec(orgUpstream, orgChannel, oc)

		exutil.By("Check recommended update and notes about additional updates present on the output of oc adm upgrade")
		o.Expect(checkUpdates(oc, false, 1, 10,
			"Additional updates which are not recommended",
			//"based on your cluster configuration are available",
			//"or where the recommended status is \"Unknown\"",
			"for your cluster configuration are available",
			"to view those re-run the command with --include-not-recommended",
			"Recommended updates:",
			"4.99.999999 registry.ci.openshift.org/ocp/release@sha256:"+
				"9999999999999999999999999999999999999999999999999999999999999999",
		)).To(o.BeTrue(), "recommended update and notes about additional updates")

		exutil.By("Check risk type=Always updates and 2 risks update present")
		o.Expect(checkUpdates(oc, true, 1, 3,
			"Updates with known issues", "Version: 4.88.888888",
			"Image: registry.ci.openshift.org/ocp/release@sha256:"+
				"8888888888888888888888888888888888888888888888888888888888888888",
			"Reason: ExposedToRisks",
			"Message: Too many CI failures on this release, so do not update to it",
			"Version: 4.77.777777",
			"Image: registry.ci.openshift.org/ocp/release@sha256:"+
				"7777777777777777777777777777777777777777777777777777777777777777",
			"Reason: MultipleReasons",
			"Message: On clusters on default invoker user, this imaginary bug can happen. "+
				"https://bug.example.com/a",
		)).To(o.BeTrue(), "risk type=Always updates and 2 risks update")

		exutil.By("Check The reason for the multiple risks is changed to SomeInvokerThing")
		o.Expect(checkUpdates(oc, true, 60, 6*60,
			"Updates with known issues",
			"Version: 4.77.777777",
			"Image: registry.ci.openshift.org/ocp/release@sha256:"+
				"7777777777777777777777777777777777777777777777777777777777777777",
			"Reason: SomeInvokerThing",
			"Message: On clusters on default invoker user, this imaginary bug can happen. "+
				"https://bug.example.com/a",
		)).To(o.BeTrue(), "reason for the multiple risks is changed to SomeInvokerThing")

		exutil.By("Check multiple reason conditional update present")
		_, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade", "channel", "buggy").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		o.Expect(checkUpdates(oc, true, 300, 75*60,
			"Version: 4.77.777777",
			"Image: registry.ci.openshift.org/ocp/release@sha256:"+
				"7777777777777777777777777777777777777777777777777777777777777777",
			"Reason: MultipleReasons",
			"Message: On clusters on default invoker user, this imaginary bug can happen. "+
				"https://bug.example.com/a",
			"On clusters with the channel set to 'buggy', this imaginary bug can happen. "+
				"https://bug.example.com/b",
		)).To(o.BeTrue(), "multiple reason conditional update present")
	})

	//author: jianl@redhat.com
	g.It("ConnectedOnly-Author:jianl-Low-46422-cvo drops invalid conditional edges [Serial]", func() {
		exutil.By("Check if it's a GCP cluster")
		exutil.SkipIfPlatformTypeNot(oc, "gcp")

		orgUpstream, err := getCVObyJP(oc, ".spec.upstream")
		o.Expect(err).NotTo(o.HaveOccurred())
		orgChannel, err := getCVObyJP(oc, ".spec.channel")
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Original upstream: %s, original channel: %s", orgUpstream, orgChannel)

		exutil.By("Patch upstream")
		projectID := "openshift-qe"
		ctx := context.Background()
		client, err := storage.NewClient(ctx)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() { o.Expect(client.Close()).NotTo(o.HaveOccurred()) }()

		graphURL, bucket1, object1, _, _, err := buildGraph(client, oc, projectID, "cincy-conditional-edge-invalid-null-node.json")
		defer func() { o.Expect(DeleteBucket(client, bucket1)).NotTo(o.HaveOccurred()) }()
		defer func() { o.Expect(DeleteObject(client, bucket1, object1)).NotTo(o.HaveOccurred()) }()
		o.Expect(err).NotTo(o.HaveOccurred())

		_, err = ocJSONPatch(oc, "", "clusterversion/version", []JSONp{
			{"add", "/spec/upstream", graphURL},
			{"add", "/spec/channel", "stable-a"},
		})
		o.Expect(err).NotTo(o.HaveOccurred())

		defer restoreCVSpec(orgUpstream, orgChannel, oc)

		exutil.By("Check CVO prompts correct reason and message")
		expString := "warning: Cannot display available updates:\n" +
			"  Reason: ResponseInvalid\n" +
			"  Message: Unable to retrieve available updates: no node for conditional update"
		exutil.AssertWaitPollNoErr(wait.Poll(5*time.Second, 15*time.Second, func() (bool, error) {
			cmdOut, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade").Output()
			e2e.Logf("oc adm upgrade returned:\n%s", cmdOut)
			if err != nil {
				return false, fmt.Errorf("oc adm upgrade returned error: %v", err)
			}
			return strings.Contains(cmdOut, expString), nil
		}), "Test on empty target node failed")

		graphURL, bucket2, object2, _, _, err := buildGraph(client, oc, projectID, "cincy-conditional-edge-invalid-multi-risks.json")
		defer func() { o.Expect(DeleteBucket(client, bucket2)).NotTo(o.HaveOccurred()) }()
		defer func() { o.Expect(DeleteObject(client, bucket2, object2)).NotTo(o.HaveOccurred()) }()
		o.Expect(err).NotTo(o.HaveOccurred())

		_, err = ocJSONPatch(oc, "", "clusterversion/version", []JSONp{{"add", "/spec/upstream", graphURL}})
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Check no updates")
		exutil.AssertWaitPollNoErr(wait.Poll(5*time.Second, 15*time.Second, func() (bool, error) {
			cmdOut, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade").Output()
			e2e.Logf("oc adm upgrade returned:\n%s", cmdOut)
			if err != nil {
				return false, fmt.Errorf("oc adm upgrade returned error: %v", err)
			}
			return strings.Contains(cmdOut, "No updates available"), nil
		}), "Test on multiple invalid risks failed")
	})

	//author: jianl@redhat.com
	g.It("ConnectedOnly-Author:jianl-Low-47175-upgrade cluster when current version is in the upstream but there are not update paths [Serial]", func() {
		exutil.By("Check if it's a GCP cluster")
		exutil.SkipIfPlatformTypeNot(oc, "gcp")

		orgUpstream, err := getCVObyJP(oc, ".spec.upstream")
		o.Expect(err).NotTo(o.HaveOccurred())
		orgChannel, err := getCVObyJP(oc, ".spec.channel")
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Original upstream: %s, original channel: %s", orgUpstream, orgChannel)

		exutil.By("Patch upstream")
		projectID := "openshift-qe"
		ctx := context.Background()
		client, err := storage.NewClient(ctx)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() { o.Expect(client.Close()).NotTo(o.HaveOccurred()) }()

		graphURL, bucket, object, _, _, err := buildGraph(client, oc, projectID, "cincy-conditional-edge-invalid-multi-risks.json")
		defer func() { o.Expect(DeleteBucket(client, bucket)).NotTo(o.HaveOccurred()) }()
		defer func() { o.Expect(DeleteObject(client, bucket, object)).NotTo(o.HaveOccurred()) }()
		o.Expect(err).NotTo(o.HaveOccurred())

		_, err = ocJSONPatch(oc, "", "clusterversion/version", []JSONp{
			{"add", "/spec/upstream", graphURL},
			{"add", "/spec/channel", "stable-a"},
		})
		o.Expect(err).NotTo(o.HaveOccurred())

		defer restoreCVSpec(orgUpstream, orgChannel, oc)

		var cmdOut string
		exutil.By("Check no updates but RetrievedUpdates=True")
		exutil.AssertWaitPollNoErr(wait.Poll(5*time.Second, 15*time.Second, func() (bool, error) {
			cmdOut, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade").Output()
			e2e.Logf("oc adm upgrade returned:\n%s", cmdOut)
			if err != nil {
				return false, fmt.Errorf("oc adm upgrade returned error: %v", err)
			}
			return strings.Contains(cmdOut, "No updates available"), nil
		}), "failure: missing expected 'No updates available'")

		status, err := getCVObyJP(oc, ".status.conditions[?(.type=='RetrievedUpdates')].status")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(status).To(o.Equal("True"))

		target := GenerateReleaseVersion(oc)
		o.Expect(target).NotTo(o.BeEmpty())

		exutil.By("Upgrade with oc adm upgrade --to")
		cmdOut, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade", "--to", target).Output()
		o.Expect(cmdOut).To(o.ContainSubstring(
			"no recommended updates, specify --to-image to conti" +
				"nue with the update or wait for new updates to be available"))
		o.Expect(err).To(o.HaveOccurred())

		exutil.By("Upgrade with oc adm upgrade --to --allow-not-recommended")
		cmdOut, err = oc.AsAdmin().WithoutNamespace().Run("adm").
			Args("upgrade", "--allow-not-recommended", "--to", target).Output()
		o.Expect(cmdOut).To(o.ContainSubstring(
			"no recommended or conditional updates, specify --to-image to conti" +
				"nue with the update or wait for new updates to be available"))
		o.Expect(err).To(o.HaveOccurred())

		targetPullspec := GenerateReleasePayload(oc)
		o.Expect(targetPullspec).NotTo(o.BeEmpty())

		exutil.By("Upgrade with oc adm upgrade --to-image")
		cmdOut, err = oc.AsAdmin().WithoutNamespace().Run("adm").
			Args("upgrade", "--to-image", targetPullspec).Output()
		o.Expect(cmdOut).To(o.ContainSubstring(
			"no recommended updates, specify --allow-explicit-upgrade to conti" +
				"nue with the update or wait for new updates to be available"))
		o.Expect(err).To(o.HaveOccurred())

		exutil.By("Upgrade with oc adm upgrade --to-image --allow-not-recommended")
		cmdOut, err = oc.AsAdmin().WithoutNamespace().Run("adm").
			Args("upgrade", "--allow-not-recommended", "--to-image", targetPullspec).Output()
		o.Expect(cmdOut).To(o.ContainSubstring(
			"no recommended or conditional updates, specify --allow-explicit-upgrade to conti" +
				"nue with the update or wait for new updates to be available"))
		o.Expect(err).To(o.HaveOccurred())
	})

	//author: dis@redhat.com
	g.It("NonHyperShiftHOST-Author:dis-Medium-41391-cvo serves metrics over only https not http", func() {
		exutil.By("Check cvo delopyment config file...")
		cvoDeploymentYaml, err := GetDeploymentsYaml(oc, "cluster-version-operator", projectName)
		o.Expect(err).NotTo(o.HaveOccurred())
		var keywords = []string{"--listen=0.0.0.0:9099",
			"--serving-cert-file=/etc/tls/serving-cert/tls.crt",
			"--serving-key-file=/etc/tls/serving-cert/tls.key"}
		for _, v := range keywords {
			o.Expect(cvoDeploymentYaml).Should(o.ContainSubstring(v))
		}

		exutil.By("Check cluster-version-operator binary help")
		cvoPodsList, err := exutil.WaitForPods(
			oc.AdminKubeClient().CoreV1().Pods(projectName),
			exutil.ParseLabelsOrDie("k8s-app=cluster-version-operator"),
			exutil.CheckPodIsReady, 1, 3*time.Minute)
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Get cvo pods: %v", cvoPodsList)
		output, err := PodExec(oc, "/usr/bin/cluster-version-operator start --help", projectName, cvoPodsList[0])
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf(
			"/usr/bin/cluster-version-operator start --help executs error on %v", cvoPodsList[0]))
		e2e.Logf("CVO help returned: %s", output)
		keywords = []string{"You must set both --serving-cert-file and --serving-key-file unless you set --listen empty"}
		for _, v := range keywords {
			o.Expect(output).Should(o.ContainSubstring(v))
		}

		exutil.By("Verify cvo metrics is only exported via https")
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").
			Args("servicemonitor", "cluster-version-operator",
				"-n", projectName, "-o=json").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		var result map[string]interface{}
		err = json.Unmarshal([]byte(output), &result)
		o.Expect(err).NotTo(o.HaveOccurred())
		endpoints := result["spec"].(map[string]interface{})["endpoints"]
		e2e.Logf("Get cvo's spec.endpoints: %v", endpoints)
		o.Expect(endpoints).Should(o.HaveLen(1))

		output, err = oc.AsAdmin().WithoutNamespace().Run("get").
			Args("servicemonitor", "cluster-version-operator",
				"-n", projectName, "-o=jsonpath={.spec.endpoints[].scheme}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Get cvo's spec.endpoints scheme: %v", output)
		o.Expect(output).Should(o.Equal("https"))

		exutil.By("Get cvo endpoint URI")
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

		exutil.By("Check metric server is providing service https, but not http")
		cmd := fmt.Sprintf("curl http://%s/metrics", endpointURI)
		output, err = PodExec(oc, cmd, projectName, cvoPodsList[0])
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("cmd %s executs error on %v", cmd, cvoPodsList[0]))
		keywords = []string{"Client sent an HTTP request to an HTTPS server"}
		for _, v := range keywords {
			o.Expect(output).Should(o.ContainSubstring(v))
		}

		exutil.By("Check metric server is providing service via https correctly.")
		cmd = fmt.Sprintf("curl -k -I https://%s/metrics", endpointURI)
		output, err = PodExec(oc, cmd, projectName, cvoPodsList[0])
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("cmd %s executs error on %v", cmd, cvoPodsList[0]))
		keywords = []string{"HTTP/1.1 200 OK"}
		for _, v := range keywords {
			o.Expect(output).Should(o.ContainSubstring(v))
		}
	})

	//author: dis@redhat.com
	g.It("Longduration-NonPreRelease-Author:dis-Medium-32138-cvo alert should not be fired when RetrievedUpdates failed due to nochannel [Serial][Slow]", func() {
		orgChannel, err := getCVObyJP(oc, ".spec.channel")
		o.Expect(err).NotTo(o.HaveOccurred())

		defer func() {
			o.Expect(oc.AsAdmin().WithoutNamespace().Run("adm").
				Args("upgrade", "channel", orgChannel).Execute()).NotTo(o.HaveOccurred())
		}()

		exutil.By("Enable alert by clearing channel")
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade", "channel").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Check RetrievedUpdates condition")
		reason, err := getCVObyJP(oc, ".status.conditions[?(.type=='RetrievedUpdates')].reason")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(reason).To(o.Equal("NoChannel"))

		exutil.By("Alert CannotRetrieveUpdates does not appear within 60m")
		appeared, _, err := waitForAlert(oc, "CannotRetrieveUpdates", 600, 3600, "")
		o.Expect(appeared).NotTo(o.BeTrue(), "no CannotRetrieveUpdates within 60m")
		o.Expect(err.Error()).To(o.ContainSubstring("timed out waiting for the condition"))

		exutil.By("Alert CannotRetrieveUpdates does not appear after 60m")
		appeared, _, err = waitForAlert(oc, "CannotRetrieveUpdates", 300, 600, "")
		o.Expect(appeared).NotTo(o.BeTrue(), "no CannotRetrieveUpdates after 60m")
		o.Expect(err.Error()).To(o.ContainSubstring("timed out waiting for the condition"))
	})

	//author: jianl@redhat.com
	g.It("ConnectedOnly-Author:jianl-Medium-43178-manage channel by using oc adm upgrade channel [Serial]", func() {
		exutil.By("Check if it's a GCP cluster")
		exutil.SkipIfPlatformTypeNot(oc, "gcp")

		orgUpstream, err := getCVObyJP(oc, ".spec.upstream")
		o.Expect(err).NotTo(o.HaveOccurred())
		orgChannel, err := getCVObyJP(oc, ".spec.channel")
		o.Expect(err).NotTo(o.HaveOccurred())

		e2e.Logf("Original upstream: %s, original channel: %s", orgUpstream, orgChannel)

		projectID := "openshift-qe"
		ctx := context.Background()
		client, err := storage.NewClient(ctx)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() { o.Expect(client.Close()).NotTo(o.HaveOccurred()) }()

		graphURL, bucket, object, _, _, err := buildGraph(client, oc, projectID, "cincy.json")
		defer func() { o.Expect(DeleteBucket(client, bucket)).NotTo(o.HaveOccurred()) }()
		defer func() { o.Expect(DeleteObject(client, bucket, object)).NotTo(o.HaveOccurred()) }()
		o.Expect(err).NotTo(o.HaveOccurred())

		defer restoreCVSpec(orgUpstream, orgChannel, oc)

		// Prerequisite: the available channels are not present
		exutil.By("The test requires the available channels are not present as a prerequisite")
		cmdOut, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(cmdOut).NotTo(o.ContainSubstring("available channels:"))

		version, err := getCVObyJP(oc, ".status.desired.version")
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Set to an unknown channel when available channels are not present")
		cmdOut, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade", "channel", "unknown-channel").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(cmdOut).To(o.ContainSubstring(
			fmt.Sprintf("warning: No channels known to be compatible with the current version \"%s\"; unable to vali"+
				"date \"unknown-channel\". Setting the update channel to \"unknown-channel\" anyway.", version)))
		cmdOut, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(cmdOut).To(o.ContainSubstring("Channel: unknown-channel"))

		exutil.By("Clear an unknown channel when available channels are not present")
		cmdOut, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade", "channel").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(cmdOut).To(o.ContainSubstring(
			"warning: Clearing channel \"unknown-channel\"; cluster will no longer request available update recommendations."))
		cmdOut, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(cmdOut).To(o.ContainSubstring("NoChannel"))

		// Prerequisite: a dummy update server is ready and the available channels is present
		exutil.By("Change to a dummy update server")
		_, err = ocJSONPatch(oc, "", "clusterversion/version", []JSONp{
			{"add", "/spec/upstream", graphURL},
			{"add", "/spec/channel", "channel-a"},
		})
		o.Expect(err).NotTo(o.HaveOccurred())
		time.Sleep(5 * time.Second)
		cmdOut, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(cmdOut).To(o.ContainSubstring("Channel: channel-a (available channels: channel-a, channel-b)"))

		exutil.By("Specify multiple channels")
		cmdOut, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade", "channel", "channel-a", "channel-b").Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(cmdOut).To(o.ContainSubstring(
			"error: multiple positional arguments given\nSee 'oc adm upgrade channel -h' for help and examples"))

		exutil.By("Set a channel which is same as the current channel")
		cmdOut, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade", "channel", "channel-a").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(cmdOut).To(o.ContainSubstring("info: Cluster is already in channel-a (no change)"))

		exutil.By("Clear a known channel which is in the available channels without --allow-explicit-channel")
		cmdOut, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade", "channel").Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(cmdOut).To(o.ContainSubstring(
			"error: You are requesting to clear the update channel. The current channel \"channel-a\" is " +
				"one of the available channels, you must pass --allow-explicit-channel to continue"))

		exutil.By("Clear a known channel which is in the available channels with --allow-explicit-channel")
		cmdOut, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade", "channel", "--allow-explicit-channel").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(cmdOut).To(o.ContainSubstring(
			"warning: Clearing channel \"channel-a\"; cluster will no longer request available update recommendations."))
		cmdOut, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(cmdOut).To(o.ContainSubstring("NoChannel"))

		exutil.By("Re-clear the channel")
		cmdOut, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade", "channel").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(cmdOut).To(o.ContainSubstring("info: Cluster channel is already clear (no change)"))

		exutil.By("Set to an unknown channel when the available channels are not present without --allow-explicit-channel")
		cmdOut, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade", "channel", "channel-d").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		time.Sleep(5 * time.Second)
		o.Expect(cmdOut).To(o.ContainSubstring(
			fmt.Sprintf("warning: No channels known to be compatible with the current version \"%s\"; unable to vali"+
				"date \"channel-d\". Setting the update channel to \"channel-d\" anyway.", version)))
		cmdOut, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(cmdOut).To(o.ContainSubstring("Channel: channel-d (available channels: channel-a, channel-b)"))

		exutil.By("Set to an unknown channel which is not in the available channels without --allow-explicit-channel")
		cmdOut, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade", "channel", "channel-f").Output()
		o.Expect(err).To(o.HaveOccurred())
		time.Sleep(5 * time.Second)
		o.Expect(cmdOut).To(o.ContainSubstring(
			"error: the requested channel \"channel-f\" is not one of the avail" +
				"able channels (channel-a, channel-b), you must pass --allow-explicit-channel to continue"))

		exutil.By("Set to an unknown channel which is not in the available channels with --allow-explicit-channel")
		cmdOut, err = oc.AsAdmin().WithoutNamespace().Run("adm").
			Args("upgrade", "channel", "channel-f", "--allow-explicit-channel").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		time.Sleep(5 * time.Second)
		o.Expect(cmdOut).To(o.ContainSubstring(
			"warning: The requested channel \"channel-f\" is not one of the avail" +
				"able channels (channel-a, channel-b). You have used --allow-explicit-cha" +
				"nnel to proceed anyway. Setting the update channel to \"channel-f\"."))
		cmdOut, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(cmdOut).To(o.ContainSubstring("Channel: channel-f (available channels: channel-a, channel-b)"))

		exutil.By("Clear an unknown channel which is not in the available channels without --allow-explicit-channel")
		cmdOut, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade", "channel").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(cmdOut).To(o.ContainSubstring(
			"warning: Clearing channel \"channel-f\"; cluster will no longer request available update recommendations."))
		cmdOut, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(cmdOut).To(o.ContainSubstring("NoChannel"))

		exutil.By("Set to a known channel when the available channels are not present")
		cmdOut, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade", "channel", "channel-a").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		time.Sleep(5 * time.Second)
		o.Expect(cmdOut).To(o.ContainSubstring(
			fmt.Sprintf("warning: No channels known to be compatible with the current version \"%s\"; un"+
				"able to validate \"channel-a\". Setting the update channel to \"channel-a\" anyway.", version)))
		cmdOut, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(cmdOut).To(o.ContainSubstring("Channel: channel-a (available channels: channel-a, channel-b)"))

		exutil.By("Set to a known channel without --allow-explicit-channel")
		_, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade", "channel", "channel-b").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		time.Sleep(5 * time.Second)
		cmdOut, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(cmdOut).To(o.ContainSubstring("Channel: channel-b (available channels: channel-a, channel-b)"))
	})

	//author: jianl@redhat.com
	g.It("Author:jianl-High-42543-the removed resources are not created in a fresh installed cluster", func() {
		exutil.By("Check the annotation delete:true for imagestream/hello-openshift is set in manifest")
		tempDataDir, err := extractManifest(oc)
		defer func() { o.Expect(os.RemoveAll(tempDataDir)).NotTo(o.HaveOccurred()) }()
		o.Expect(err).NotTo(o.HaveOccurred())
		manifestDir := filepath.Join(tempDataDir, "manifest")
		cmd := fmt.Sprintf("grep -rl \"name: hello-openshift\" %s", manifestDir)
		out, err := exec.Command("bash", "-c", cmd).CombinedOutput()
		o.Expect(err).NotTo(o.HaveOccurred(), "Command: \"%s\" returned error: %s", cmd, string(out))
		o.Expect(string(out)).NotTo(o.BeEmpty())
		file := strings.TrimSpace(string(out))
		cmd = fmt.Sprintf("grep -C5 'name: hello-openshift' %s | grep 'release.openshift.io/delete: \"true\"'", file)
		out, err = exec.Command("bash", "-c", cmd).CombinedOutput()
		o.Expect(err).NotTo(o.HaveOccurred(), "Command: \"%s\" returned error: %s", cmd, string(out))
		o.Expect(string(out)).NotTo(o.BeEmpty())

		exutil.By("Check imagestream hello-openshift not present in a fresh installed cluster")
		cmdOut, err := oc.AsAdmin().WithoutNamespace().Run("get").
			Args("imagestream", "hello-openshift", "-n", "openshift").Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(cmdOut).To(o.ContainSubstring(
			"Error from server (NotFound): imagestreams.image.openshift.io \"hello-openshift\" not found"))
	})

	//author: jianl@redhat.com
	g.It("ConnectedOnly-Author:jianl-Medium-43172-get the upstream and channel info by using oc adm upgrade [Serial]", func() {
		exutil.By("Check if it's a GCP cluster")
		exutil.SkipIfPlatformTypeNot(oc, "gcp")

		orgUpstream, err := getCVObyJP(oc, ".spec.upstream")
		o.Expect(err).NotTo(o.HaveOccurred())
		orgChannel, err := getCVObyJP(oc, ".spec.channel")
		o.Expect(err).NotTo(o.HaveOccurred())

		e2e.Logf("Original upstream: %s, original channel: %s", orgUpstream, orgChannel)

		defer restoreCVSpec(orgUpstream, orgChannel, oc)

		exutil.By("Check when upstream is unset")
		if orgUpstream != "" {
			_, err := ocJSONPatch(oc, "", "clusterversion/version", []JSONp{{"remove", "/spec/upstream", nil}})
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		_, err = ocJSONPatch(oc, "", "clusterversion/version", []JSONp{{"add", "/spec/channel", "stable-a"}})
		o.Expect(err).NotTo(o.HaveOccurred())

		cmdOut, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(cmdOut).To(o.ContainSubstring("Upstream is unset, so the cluster will use an appropriate default."))
		o.Expect(cmdOut).To(o.ContainSubstring("Channel: stable-a"))

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

		exutil.By("Check when upstream is set")
		projectID := "openshift-qe"
		ctx := context.Background()
		client, err := storage.NewClient(ctx)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() { o.Expect(client.Close()).NotTo(o.HaveOccurred()) }()

		graphURL, bucket, object, targetVersion, targetPayload, err := buildGraph(client, oc, projectID, "cincy.json")
		defer func() { o.Expect(DeleteBucket(client, bucket)).NotTo(o.HaveOccurred()) }()
		defer func() { o.Expect(DeleteObject(client, bucket, object)).NotTo(o.HaveOccurred()) }()
		o.Expect(err).NotTo(o.HaveOccurred())

		_, err = ocJSONPatch(oc, "", "clusterversion/version", []JSONp{
			{"add", "/spec/upstream", graphURL},
			{"add", "/spec/channel", "channel-a"},
		})
		o.Expect(err).NotTo(o.HaveOccurred())
		time.Sleep(5 * time.Second)
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

		exutil.By("Check when channel is unset")
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

		exutil.By("Get goodOauthFile from the initial oauth yaml file to oauth-41728.yaml")
		goodOauthFile, err := oc.AsAdmin().WithoutNamespace().Run("get").
			Args("oauth", "cluster", "-o", "yaml").OutputToFile("oauth-41728.yaml")
		defer func() { o.Expect(os.RemoveAll(goodOauthFile)).NotTo(o.HaveOccurred()) }()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Prune goodOauthFile")
		cmd := fmt.Sprintf("sed -i \"/resourceVersion/d\" %s && cat %s", goodOauthFile, goodOauthFile)
		oauthfile, err := exec.Command("bash", "-c", cmd).CombinedOutput()
		o.Expect(err).NotTo(o.HaveOccurred(), "Command: \"%s\" returned error: %s", cmd, string(oauthfile))
		o.Expect(string(oauthfile)).NotTo(o.ContainSubstring("resourceVersion"))

		exutil.By("Enable ClusterOperatorDegraded alert")
		err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", badOauthFile).Execute()
		defer func() {
			// after applying good auth, co is back to normal, while cvo condition failing is still present for up to ~2-4 minutes
			o.Expect(waitForCVOStatus(oc, 30, 4*60, "ClusterOperatorDegraded",
				".status.conditions[?(.type=='Failing')].reason", false)).NotTo(o.HaveOccurred())
		}()
		defer func() {
			o.Expect(oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", goodOauthFile).Execute()).NotTo(o.HaveOccurred())
		}()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Check ClusterOperatorDegraded condition...")
		if err = waitForCondition(oc, 60, 480, "True",
			"get", "co", "authentication", "-o", "jsonpath={.status.conditions[?(@.type=='Degraded')].status}"); err != nil {
			//dump contents to log
			_ = oc.AsAdmin().WithoutNamespace().Run("get").Args("co", "authentication", "-o", "yaml").Execute()
			_ = oc.AsAdmin().WithoutNamespace().Run("get").Args("oauth", "cluster", "-o", "yaml").Execute()
			exutil.AssertWaitPollNoErr(err, "authentication operator is not degraded in 8m")
		}

		exutil.By("Check ClusterOperatorDown alert is not firing and ClusterOperatorDegraded alert is fired correctly.")
		var alertDown, alertDegraded map[string]interface{}
		err = wait.Poll(5*time.Minute, 35*time.Minute, func() (bool, error) {
			alertDown = getAlertByName(oc, "ClusterOperatorDown", "authentication")
			alertDegraded = getAlertByName(oc, "ClusterOperatorDegraded", "authentication")
			if alertDown != nil {
				return false, fmt.Errorf("alert ClusterOperatorDown is not nil: %v", alertDown)
			}
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
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("ClusterOperatorDegraded alert is not fired in 30m: %v", alertDegraded))
		exutil.By("Disable ClusterOperatorDegraded alert")
		err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", goodOauthFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Check alert is disabled")
		exutil.AssertWaitPollNoErr(wait.Poll(10*time.Second, 60*time.Second, func() (bool, error) {
			alertDegraded = getAlertByName(oc, "ClusterOperatorDegraded", "authentication")
			e2e.Logf("Waiting for alert being disabled...")
			return alertDegraded == nil, nil
		}), fmt.Sprintf("alert is not disabled: %v", alertDegraded))
	})

	//author: jiajliu@redhat.com
	g.It("Longduration-NonPreRelease-Author:jiajliu-Medium-41778-ClusterOperatorDown and ClusterOperatorDegradedon alerts when unset conditions [Slow]", func() {

		testDataDir := exutil.FixturePath("testdata", "ota/cvo")
		badOauthFile := filepath.Join(testDataDir, "co-test.yaml")

		exutil.By("Enable alerts")
		err := oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", badOauthFile).Execute()
		// if normal already deleted before. discarding error.
		defer func() { _ = oc.AsAdmin().WithoutNamespace().Run("delete").Args("co", "test").Execute() }()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Check operator's condition...")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co", "test", "-o=jsonpath={.status}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.Equal(""))

		exutil.By("Waiting for alerts triggered...")
		var alertDown, alertDegraded map[string]interface{}
		exutil.AssertWaitPollNoErr(wait.Poll(30*time.Second, 180*time.Second, func() (bool, error) {
			alertDown = getAlertByName(oc, "ClusterOperatorDown", "test")
			alertDegraded = getAlertByName(oc, "ClusterOperatorDegraded", "test")
			e2e.Logf("Waiting for alerts to be triggered...")
			return alertDown != nil && alertDegraded != nil, nil
		}), fmt.Sprintf("failed expecting both alerts triggered: Down=%v Degraded=%v", alertDown, alertDegraded))

		exutil.By("Check alert ClusterOperatorDown fired.")
		exutil.AssertWaitPollNoErr(wait.Poll(5*time.Minute, 10*time.Minute, func() (bool, error) {
			alertDown = getAlertByName(oc, "ClusterOperatorDown", "test")
			e2e.Logf("Waiting for alert ClusterOperatorDown to be triggered and fired...")
			return alertDown["state"] == "firing", nil
		}), fmt.Sprintf("ClusterOperatorDown alert is not fired in 10m: %v", alertDown))

		exutil.By("Check alert ClusterOperatorDegraded fired.")
		exutil.AssertWaitPollNoErr(wait.Poll(5*time.Minute, 20*time.Minute, func() (bool, error) {
			alertDegraded = getAlertByName(oc, "ClusterOperatorDegraded", "test")
			e2e.Logf("Waiting for alert ClusterOperatorDegraded to be triggered and fired...")
			return alertDegraded["state"] == "firing", nil
		}), fmt.Sprintf("ClusterOperatorDegraded alert is not fired in 30m: %v", alertDegraded))

		exutil.By("Disable alerts")
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("co", "test").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Check alerts are disabled...")
		exutil.AssertWaitPollNoErr(wait.Poll(10*time.Second, 60*time.Second, func() (bool, error) {
			alertDown := getAlertByName(oc, "ClusterOperatorDown", "test")
			alertDegraded := getAlertByName(oc, "ClusterOperatorDegraded", "test")
			e2e.Logf("Waiting for alerts being disabled...")
			return alertDown == nil && alertDegraded == nil, nil
		}), fmt.Sprintf("alerts are not disabled: Down=%v Degraded=%v", alertDown, alertDegraded))
	})

	//author: jiajliu@redhat.com
	g.It("Longduration-NonPreRelease-Author:jiajliu-Medium-41736-cvo alert ClusterOperatorDown on unavailable operators [Disruptive][Slow]", func() {
		operator := "image-registry"
		nodeLabel := "node-role.kubernetes.io/worker="
		exutil.By("Cordon worker nodes")
		workerNodes, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "--selector", nodeLabel, "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred(), "fail to get node list: %v", err)
		nodeList := strings.Fields(workerNodes)
		defer func() {
			e2e.Logf("Restore worker node in defer")
			oc.AsAdmin().WithoutNamespace().Run("adm").Args("uncordon", "-l", nodeLabel).Execute()
			for _, node := range nodeList {
				err = wait.PollUntilContextTimeout(context.Background(), 1*time.Minute, 5*time.Minute, true, func(context.Context) (bool, error) {
					nodeReady, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", node, "-o=jsonpath={.status.conditions[?(@.type==\"Ready\")].status}").Output()
					if err != nil || nodeReady != "True" {
						e2e.Logf("error: %v; node %s status: %s", err, node, nodeReady)
						return false, nil
					}
					return true, nil
				})
				exutil.AssertWaitPollNoErr(err, "timeout to restore node!")
			}
			e2e.Logf("Ensure operator back to good status after uncordon")
			err = wait.PollUntilContextTimeout(context.Background(), 1*time.Minute, 5*time.Minute, true, func(context.Context) (bool, error) {
				output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co", operator).Output()
				matched, _ := regexp.MatchString("True.*False.*False", output)
				if err != nil || !matched {
					e2e.Logf("error:%; operator status: %s", err, output)
					return false, nil
				}
				return true, nil
			})
			exutil.AssertWaitPollNoErr(err, "timeout to restore operator!")
		}()

		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("cordon", "-l", nodeLabel).Execute()
		o.Expect(err).NotTo(o.HaveOccurred(), "fail to cordon worker node: %v", err)
		exutil.By("Delete namespace openshift-image-registry")
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("namespace", "openshift-image-registry").Execute()
		o.Expect(err).NotTo(o.HaveOccurred(), "fail to delete namespace: %v", err)

		exutil.By("Check ClusterOperatorDown condition...")
		if err = waitForCondition(oc, 60, 900, "False",
			"get", "co", operator, "-o", "jsonpath={.status.conditions[?(@.type=='Available')].status}"); err != nil {
			//dump contents to log
			_ = oc.AsAdmin().WithoutNamespace().Run("get").Args("co", operator, "-o", "yaml").Execute()
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("%s operator is not down in 15m", operator))
		}

		exutil.By("Check ClusterOperatorDown alert is fired correctly")
		var alertDown map[string]interface{}
		err = wait.Poll(2*time.Minute, 10*time.Minute, func() (bool, error) {
			alertDown = getAlertByName(oc, "ClusterOperatorDown", operator)
			if alertDown == nil || alertDown["state"] != "firing" {
				e2e.Logf("Waiting for alert ClusterOperatorDown to be triggered and fired...")
				return false, nil
			}
			o.Expect(alertDown["labels"].(map[string]interface{})["severity"].(string)).To(o.Equal("critical"))
			o.Expect(alertDown["labels"].(map[string]interface{})["namespace"].(string)).To(o.Equal("openshift-cluster-version"))
			o.Expect(alertDown["annotations"].(map[string]interface{})["summary"].(string)).
				To(o.ContainSubstring("Cluster operator has not been available for 10 minutes."))
			o.Expect(alertDown["annotations"].(map[string]interface{})["description"].(string)).
				To(o.ContainSubstring(fmt.Sprintf("The %s operator may be down or disabled", operator)))
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("ClusterOperatorDown alert is not fired in 10m: %v", alertDown))

		exutil.By("Disable ClusterOperatorDown alert")
		e2e.Logf("Uncordon worker node")
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("uncordon", "-l", nodeLabel).Execute()
		o.Expect(err).NotTo(o.HaveOccurred(), "fail to uncordon worker node: %v", err)

		exutil.By("Check alert is disabled")
		exutil.AssertWaitPollNoErr(wait.Poll(1*time.Minute, 5*time.Minute, func() (bool, error) {
			alertDown = getAlertByName(oc, "ClusterOperatorDown", operator)
			e2e.Logf("Waiting for alert being disabled...")
			return alertDown == nil, nil
		}), fmt.Sprintf("alert is not disabled: %v", alertDown))
	})

	//author: jiajliu@redhat.com
	g.It("NonHyperShiftHOST-Author:jiajliu-Low-46922-check runlevel in cvo ns", func() {
		exutil.By("Check runlevel in cvo namespace.")
		runLevel, err := oc.AsAdmin().WithoutNamespace().Run("get").
			Args("ns", "openshift-cluster-version",
				"-o=jsonpath={.metadata.labels.openshift\\.io/run-level}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(runLevel).To(o.Equal(""))

		exutil.By("Check scc of cvo pod.")
		runningPodName, err := oc.AsAdmin().WithoutNamespace().Run("get").
			Args("pod", "-n", "openshift-cluster-version", "-o=jsonpath='{.items[?(@.status.phase == \"Running\")].metadata.name}'").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(runningPodName).NotTo(o.Equal("''"))
		runningPodList := strings.Fields(runningPodName)
		if len(runningPodList) != 1 {
			e2e.Failf("Unexpected running cvo pods detected:" + runningPodName)
		}
		scc, err := oc.AsAdmin().WithoutNamespace().Run("get").
			Args("pod", "-n", "openshift-cluster-version", strings.Trim(runningPodList[0], "'"),
				"-o=jsonpath={.metadata.annotations.openshift\\.io/scc}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(scc).To(o.Equal("hostaccess"))
	})

	//author: dis@redhat.com
	g.It("Author:dis-Medium-46724-cvo defaults deployment replicas to one if it's unset in manifest [Flaky]", func() {
		exutil.SkipBaselineCaps(oc, "None, v4.11")
		exutil.By("Check the replicas for openshift-insights/insights-operator is unset in manifest")
		tempDataDir, err := extractManifest(oc)
		defer func() { o.Expect(os.RemoveAll(tempDataDir)).NotTo(o.HaveOccurred()) }()
		o.Expect(err).NotTo(o.HaveOccurred())
		manifestDir := filepath.Join(tempDataDir, "manifest")
		namespace, name := "openshift-insights", "insights-operator"
		cmd := fmt.Sprintf(
			"grep -rlZ 'kind: Deployment' %s | xargs -0 grep -l 'name: %s\\|namespace: %s' | xargs grep replicas",
			manifestDir, name, namespace)
		e2e.Logf("executing: bash -c %s", cmd)
		out, err := exec.Command("bash", "-c", cmd).CombinedOutput()
		// We expect no replicas could be found, so the cmd should return with non-zero
		o.Expect(err).To(o.HaveOccurred(), "Command: \"%s\" returned success instead of error: %s", cmd, string(out))
		o.Expect(string(out)).To(o.BeEmpty())

		exutil.By("Check only one insights-operator pod in a fresh installed cluster")
		num, err := oc.AsAdmin().WithoutNamespace().Run("get").
			Args("deployment", name,
				"-o=jsonpath={.spec.replicas}", "-n", namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(num).To(o.Equal("1"))

		defer func() {
			out, err := oc.AsAdmin().WithoutNamespace().Run("scale").
				Args("--replicas", "1",
					fmt.Sprintf("deployment/%s", name),
					"-n", namespace).Output()
			o.Expect(err).NotTo(o.HaveOccurred(), out)
		}()

		exutil.By("Scale down insights-operator replica to 0")
		_, err = oc.AsAdmin().WithoutNamespace().Run("scale").
			Args("--replicas", "0",
				fmt.Sprintf("deployment/%s", name),
				"-n", namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Check the insights-operator replica recovers to one")
		exutil.AssertWaitPollNoErr(wait.Poll(30*time.Second, 5*time.Minute, func() (bool, error) {
			num, err = oc.AsAdmin().WithoutNamespace().Run("get").
				Args("deployment", name,
					"-o=jsonpath={.spec.replicas}",
					"-n", namespace).Output()
			return num == "1", err
		}), "insights-operator replicas is not 1")

		exutil.By("Scale up insights-operator replica to 2")
		_, err = oc.AsAdmin().WithoutNamespace().Run("scale").
			Args("--replicas", "2",
				fmt.Sprintf("deployment/%s", name),
				"-n", namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Check the insights-operator replica recovers to one")
		exutil.AssertWaitPollNoErr(wait.Poll(30*time.Second, 5*time.Minute, func() (bool, error) {
			num, err = oc.AsAdmin().WithoutNamespace().Run("get").
				Args("deployment", name,
					"-o=jsonpath={.spec.replicas}",
					"-n", namespace).Output()
			return num == "1", err
		}), "insights-operator replicas is not 1")
	})

	//author: jiajliu@redhat.com
	g.It("Author:jiajliu-Medium-47198-Techpreview operator will not be installed on a fresh installed", func() {
		tpOperatorNames := []string{"cluster-api"}
		tpOperator := []map[string]string{
			{"ns": "openshift-cluster-api", "co": tpOperatorNames[0]}}

		featuregate, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("featuregate", "cluster", "-o=jsonpath={.spec}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Featuregate: %s", featuregate)
		if featuregate != "{}" {
			if strings.Contains(featuregate, "TechPreviewNoUpgrade") {
				g.Skip("This case is only suitable for non-techpreview cluster!")
			} else if strings.Contains(featuregate, "CustomNoUpgrade") {
				e2e.Logf("Drop openshift-cluster-api ns due to CustomNoUpgrade fs enabled!")
				delete(tpOperator[0], "ns")
			} else {
				e2e.Failf("Neither TechPreviewNoUpgrade fs nor CustomNoUpgrade fs enabled, stop here to confirm expected behavior first!")
			}
		}

		exutil.By("Check annotation release.openshift.io/feature-set=TechPreviewNoUpgrade in manifests are correct.")
		tempDataDir, err := extractManifest(oc)
		defer func() { o.Expect(os.RemoveAll(tempDataDir)).NotTo(o.HaveOccurred()) }()
		o.Expect(err).NotTo(o.HaveOccurred())
		manifestDir := filepath.Join(tempDataDir, "manifest")
		cmd := fmt.Sprintf("grep -rl 'release.openshift.io/feature-set: .*TechPreviewNoUpgrade.*' %s|grep 'cluster.*operator.yaml'", manifestDir)
		featuresetTechPreviewManifest, err := exec.Command("bash", "-c", cmd).CombinedOutput()
		o.Expect(err).NotTo(o.HaveOccurred(), "Command: \"%s\" returned error: %s", cmd, string(featuresetTechPreviewManifest))
		tpOperatorFilePaths := strings.Split(strings.TrimSpace(string(featuresetTechPreviewManifest)), "\n")
		o.Expect(len(tpOperatorFilePaths)).To(o.Equal(len(tpOperator)))
		e2e.Logf("Expected number of cluster operator manifest files with correct annotation found!")

		for _, file := range tpOperatorFilePaths {
			data, err := os.ReadFile(file)
			o.Expect(err).NotTo(o.HaveOccurred())
			var co configv1.ClusterOperator
			err = yaml.Unmarshal(data, &co)
			o.Expect(err).NotTo(o.HaveOccurred())
			for i := 0; i < len(tpOperatorNames); i++ {
				if co.Name == tpOperatorNames[i] {
					e2e.Logf("Found %s in file %v!", tpOperatorNames[i], file)
					tpOperatorNames = append(tpOperatorNames[:i], tpOperatorNames[i+1:]...)
					break
				}
			}
		}
		o.Expect(len(tpOperatorNames)).To(o.Equal(0))
		e2e.Logf("All expected tp operators found in manifests!")

		exutil.By("Check no TP operator installed by default.")
		for i := 0; i < len(tpOperator); i++ {
			for k, v := range tpOperator[i] {
				output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(k, v).Output()
				o.Expect(err).To(o.HaveOccurred(), "techpreview operator '%s %s' absence check failed: expecting an error, received: '%s'", k, v, output)
				o.Expect(output).To(o.ContainSubstring("NotFound"))
				e2e.Logf("Expected: Resource %s/%v not found!", k, v)
			}
		}
	})

	//author: dis@redhat.com
	g.It("Author:dis-Medium-47757-cvo respects the deployment strategy in manifests [Serial]", func() {
		exutil.SkipBaselineCaps(oc, "None, v4.11")
		exutil.By("Get the strategy for openshift-insights/insights-operator in manifest")
		tempDataDir, err := extractManifest(oc)
		defer func() { o.Expect(os.RemoveAll(tempDataDir)).NotTo(o.HaveOccurred()) }()
		o.Expect(err).NotTo(o.HaveOccurred())
		manifestDir := filepath.Join(tempDataDir, "manifest")
		namespace, name := "openshift-insights", "insights-operator"
		cmd := fmt.Sprintf(
			"grep -rlZ 'kind: Deployment' %s | xargs -0 grep -l 'name: %s' | xargs grep strategy -A1 | sed -n 2p | cut -f2 -d ':'",
			manifestDir, name)
		e2e.Logf("executing: bash -c %s", cmd)
		out, err := exec.Command("bash", "-c", cmd).CombinedOutput()
		o.Expect(err).NotTo(o.HaveOccurred(), "Command: \"%s\" returned error: %s", cmd, string(out))
		o.Expect(out).NotTo(o.BeEmpty())
		expectStrategy := strings.TrimSpace(string(out))
		e2e.Logf(expectStrategy)

		exutil.By("Check in-cluster insights-operator has the same strategy with manifest")
		existStrategy, err := oc.AsAdmin().WithoutNamespace().Run("get").
			Args("deployment", name,
				"-o=jsonpath={.spec.strategy}",
				"-n", namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(existStrategy).To(o.ContainSubstring(expectStrategy))

		exutil.By("Change the strategy")
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

		exutil.By("Check the strategy reverted after 5 minutes")
		if pollErr := wait.Poll(30*time.Second, 5*time.Minute, func() (bool, error) {
			curStrategy, err := oc.AsAdmin().WithoutNamespace().Run("get").
				Args("deployment", name, "-o=jsonpath={.spec.strategy}", "-n", namespace).Output()
			if err != nil {
				return false, fmt.Errorf("oc get deployment %s returned error: %v", name, err)
			}
			return strings.Contains(curStrategy, expectStrategy), nil
		}); pollErr != nil {
			//dump contents to log
			_ = oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", name, "-o", "yaml").Execute()
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
		exutil.By("Check for alerts Before signer ca rotation.")
		alertCVODown := getAlert(oc, ".labels.alertname == \"ClusterVersionOperatorDown\"")
		alertTargetDown := getAlert(oc, ".labels.alertname == \"TargetDown\" and .labels.service == \"cluster-version-operator\"")
		o.Expect(alertCVODown).To(o.BeNil())
		o.Expect(alertTargetDown).To(o.BeNil())

		exutil.By("Force signer ca rotation by deleting signing-key.")
		result, err := oc.AsAdmin().WithoutNamespace().Run("delete").
			Args("secret/signing-key", "-n", "openshift-service-ca").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("delete returned: %s", result)
		o.Expect(result).To(o.ContainSubstring("deleted"))

		exutil.By("Check new signing-key is recreated")
		exutil.AssertWaitPollNoErr(wait.Poll(3*time.Second, 30*time.Second, func() (bool, error) {
			// supposed to fail until available so polling and suppressing the error
			out, _ := exec.Command(
				"bash", "-c", "oc -n openshift-service-ca get secret/signing-key -o jsonpath='{.metadata.name}'").Output()
			e2e.Logf("signing-key name: %s", string(out))
			return strings.Contains(string(out), "signing-key"), nil
		}), "signing-key not recreated within 30s")

		exutil.By("Wait for Prometheus route to be available")
		// firstly wait until route is unavailable
		err = wait.Poll(3*time.Second, 30*time.Second, func() (bool, error) {
			out, cmderr := exec.Command("bash", "-c", "oc get route prometheus-k8s -n openshift-monitoring").CombinedOutput()
			if cmderr != nil {
				// oc get route returns "exit status 1" once unavailable
				if !strings.Contains(cmderr.Error(), "exit status 1") {
					return false, fmt.Errorf("oc get route prometheus-k8s returned different unexpected error: %v\n%s", cmderr, string(out))
				}
				return true, nil
			}
			return false, nil
		})
		if err != nil {
			// sometimes route stays available, won't impact rest of the test
			o.Expect(err.Error()).To(o.ContainSubstring("timed out waiting for the condition"))
		}
		// wait until available again
		exutil.AssertWaitPollNoErr(wait.Poll(10*time.Second, 600*time.Second, func() (bool, error) {
			// supposed to fail until available so polling and suppressing the error
			out, _ := exec.Command(
				"bash", "-c", "oc get route prometheus-k8s -n openshift-monitoring -o jsonpath='{.status.ingress[].conditions[].status}'").Output()
			e2e.Logf("prometheus route status: '%s'", string(out))
			return strings.Contains(string(out), "True"), nil
		}), "Prometheus route is unavailable for 10m")

		exutil.By("Check CVO accessible by Prometheus - After signer ca rotation.")
		seenAlertCVOd, seenAlertTD := false, false
		// alerts may appear within first 5 minutes, and fire after 10 more mins
		err = wait.Poll(1*time.Minute, 15*time.Minute, func() (bool, error) {
			alertCVODown = getAlert(oc, ".labels.alertname == \"ClusterVersionOperatorDown\"")
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

	//author: jianl@redhat.com
	g.It("ConnectedOnly-Author:jianl-Low-21771-Upgrade cluster when current version is not in the graph from upstream [Serial]", func() {
		var graphURL, bucket, object, targetVersion, targetPayload string
		origVersion, err := getCVObyJP(oc, ".status.desired.version")
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Check if upstream patch required")
		jsonpath := ".status.conditions[?(.type=='RetrievedUpdates')].reason"
		reason, err := getCVObyJP(oc, jsonpath)
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(reason, "VersionNotFound") {
			e2e.Logf("no patch required. skipping upstream creation")
			targetVersion = GenerateReleaseVersion(oc)
			targetPayload = GenerateReleasePayload(oc)
		} else {
			exutil.By("Check if it's a GCP cluster")
			exutil.SkipIfPlatformTypeNot(oc, "gcp")
			origUpstream, err := getCVObyJP(oc, ".spec.upstream")
			o.Expect(err).NotTo(o.HaveOccurred())
			origChannel, err := getCVObyJP(oc, ".spec.channel")
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("Original upstream: %s, original channel: %s", origUpstream, origChannel)
			defer restoreCVSpec(origUpstream, origChannel, oc)

			exutil.By("Patch upstream")
			projectID := "openshift-qe"
			ctx := context.Background()
			client, err := storage.NewClient(ctx)
			o.Expect(err).NotTo(o.HaveOccurred())
			defer func() { o.Expect(client.Close()).NotTo(o.HaveOccurred()) }()

			graphURL, bucket, object, targetVersion, targetPayload, err = buildGraph(
				client, oc, projectID, "cincy-source-not-in-graph.json")
			defer func() { o.Expect(DeleteBucket(client, bucket)).NotTo(o.HaveOccurred()) }()
			defer func() { o.Expect(DeleteObject(client, bucket, object)).NotTo(o.HaveOccurred()) }()
			o.Expect(err).NotTo(o.HaveOccurred())

			_, err = ocJSONPatch(oc, "", "clusterversion/version", []JSONp{
				{"add", "/spec/upstream", graphURL},
				{"add", "/spec/channel", "channel-a"},
			})
			o.Expect(err).NotTo(o.HaveOccurred())

			exutil.By("Check RetrievedUpdates reason VersionNotFound after patching upstream")
			jsonpath = ".status.conditions[?(.type=='RetrievedUpdates')].reason"
			exutil.AssertWaitPollNoErr(wait.Poll(5*time.Second, 15*time.Second, func() (bool, error) {
				reason, err := getCVObyJP(oc, jsonpath)
				if err != nil {
					return false, fmt.Errorf("get CVO RetrievedUpdates condition returned error: %v", err)
				}
				e2e.Logf("received reason: '%s'", reason)
				return strings.Contains(reason, "VersionNotFound"), nil
			}), "Failed to check RetrievedUpdates!=True")
		}

		exutil.By("Give appropriate error on oc adm upgrade --to")
		toOutput, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade", "--to", targetVersion).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(toOutput).To(o.ContainSubstring("Unable to retrieve available updates"))
		o.Expect(toOutput).To(o.ContainSubstring("specify --to-image to continue with the update"))

		exutil.By("Give appropriate error on oc adm upgrade --to-image")
		toImageOutput, err := oc.AsAdmin().WithoutNamespace().Run("adm").
			Args("upgrade", "--to-image", targetPayload).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(toImageOutput).To(o.ContainSubstring("Unable to retrieve available updates"))
		o.Expect(toImageOutput).To(o.ContainSubstring("specify --allow-explicit-upgrade to continue with the update"))

		defer func() {
			o.Expect(recoverReleaseAccepted(oc)).NotTo(o.HaveOccurred())
		}()

		exutil.By("give appropriate error on CVO for upgrade to invalid payload ")
		invalidPayload := "quay.io/openshift-release-dev/ocp-release@sha256:0000000000000000000000000000000000000000000000000000000000000000"
		invalidPayloadOutput, err := oc.AsAdmin().WithoutNamespace().Run("adm").
			Args("upgrade", "--allow-explicit-upgrade", "--force", "--to-image", invalidPayload).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(invalidPayloadOutput).To(o.ContainSubstring("Updating to release image"))
		// usually happens quicker, but 8 minutes is safe deadline
		if err = waitForCondition(oc, 30, 480, "False",
			"get", "clusterversion", "version", "-o", "jsonpath={.status.conditions[?(@.type=='ReleaseAccepted')].status}"); err != nil {
			//dump contents to log
			_ = oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterversion", "version", "-o", "yaml").Execute()
			exutil.AssertWaitPollNoErr(err, "ReleaseAccepted condition is not false in 8m")
		}
		message, err := getCVObyJP(oc, ".status.conditions[?(.type=='ReleaseAccepted')].message")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(message).To(o.ContainSubstring("Retrieving payload failed"))
		o.Expect(message).To(o.ContainSubstring("status initcontainer cleanup is waiting with reason \"ErrImagePull\""))
		o.Expect(message).To(o.ContainSubstring(invalidPayload))
		o.Expect(recoverReleaseAccepted(oc)).NotTo(o.HaveOccurred())

		exutil.By("Find enable-auto-update index in deployment")
		origAutoState, autoUpdIndex, err := getCVOcontArg(oc, "enable-auto-update")
		defer func() {
			out, err := patchCVOcontArg(oc, autoUpdIndex, fmt.Sprintf("--enable-auto-update=%s", origAutoState))
			o.Expect(err).NotTo(o.HaveOccurred(), out)
		}()
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = patchCVOcontArg(oc, autoUpdIndex, "--enable-auto-update=true")
		o.Expect(err).NotTo(o.HaveOccurred())

		// recovery: once enable-auto-update is reconciled (~30sec), deployment becomes unavailable for up to CVO minimum reconcile period (~2-4min)
		defer func() {
			if err = waitForCondition(oc, 30, 240, "True",
				"get", "-n", "openshift-cluster-version", "deployments/cluster-version-operator", "-o", "jsonpath={.status.conditions[?(.type=='Available')].status}"); err != nil {
				//dump contents to log
				_ = oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-cluster-version", "deployments/cluster-version-operator", "-o", "yaml").Execute()
				exutil.AssertWaitPollNoErr(err, "deployments/cluster-version-operator not available after 4m")
			}
		}()

		defer func() {
			exutil.AssertWaitPollNoErr(wait.PollImmediate(10*time.Second, 60*time.Second, func() (bool, error) {
				depArgs, _, err := getCVOcontArg(oc, "enable-auto-update")
				if err != nil {
					return false, fmt.Errorf("get CVO container args returned error: %v", err)
				}
				e2e.Logf("argument: %s", depArgs)
				return strings.Contains(depArgs, "false"), nil
			}), "Failed waiting for enable-auto-update=false")
		}()

		exutil.By("Wait for enable-auto-update")
		exutil.AssertWaitPollNoErr(wait.PollImmediate(2*time.Second, 10*time.Second, func() (bool, error) {
			depArgs, _, err := getCVOcontArg(oc, "enable-auto-update")
			if err != nil {
				return false, fmt.Errorf("get CVO container args returned error: %v", err)
			}
			e2e.Logf("argument: %s", depArgs)
			return strings.Contains(depArgs, "true"), nil
		}), "Failed waiting for enable-auto-update=true")

		exutil.By("Check cvo can not get available update after setting enable-auto-update")
		exutil.AssertWaitPollNoErr(wait.Poll(5*time.Second, 15*time.Second, func() (bool, error) {
			reason, err := getCVObyJP(oc, ".status.conditions[?(.type=='RetrievedUpdates')].reason")
			if err != nil {
				return false, fmt.Errorf("get CVO RetreivedUpdates condition returned error: %v", err)
			}
			e2e.Logf("reason: %s", reason)
			return strings.Contains(reason, "VersionNotFound"), nil
		}), "Failed to check cvo can not get available update")

		exutil.By("Check availableUpdates is null")
		o.Expect(getCVObyJP(oc, ".status.availableUpdates")).To(o.Equal("null"), "unexpected availableUpdates") // changed from <nil> to null in 4.16

		exutil.By("Check desired version haven't changed")
		o.Expect(getCVObyJP(oc, ".status.desired.version")).To(o.Equal(origVersion), "unexpected desired version change")
	})

	//author: evakhoni@redhat.com
	g.It("Longduration-NonPreRelease-Author:evakhoni-Medium-22641-Rollback against a dummy start update with oc adm upgrade clear [Serial]", func() {
		// preserve original message
		originalMessage, err := getCVObyJP(oc, ".status.conditions[?(.type=='ReleaseAccepted')].message")
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("request upgrade to fake payload")
		fakeReleasePayload := "registry.ci.openshift.org/ocp/release@sha256:5a561dc23a9d323c8bd7a8631bed078a9e5eec690ce073f78b645c83fb4cdf74"
		err = oc.AsAdmin().WithoutNamespace().Run("adm").
			Args("upgrade", "--allow-explicit-upgrade", "--force", "--to-image", fakeReleasePayload).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() { o.Expect(recoverReleaseAccepted(oc)).NotTo(o.HaveOccurred()) }()

		exutil.By("check ReleaseAccepted=False")
		// usually happens quicker, but 8 minutes is safe deadline
		if err = waitForCondition(oc, 30, 480, "False",
			"get", "clusterversion", "version", "-o", "jsonpath={.status.conditions[?(@.type=='ReleaseAccepted')].status}"); err != nil {
			//dump contents to log
			_ = oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterversion", "version", "-o", "yaml").Execute()
			exutil.AssertWaitPollNoErr(err, "ReleaseAccepted condition is not false in 8m")
		}

		exutil.By("check ReleaseAccepted False have correct message")
		message, err := getCVObyJP(oc, ".status.conditions[?(.type=='ReleaseAccepted')].message")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(message).To(o.ContainSubstring("Unable to download and prepare the update: deadline exceeded"))
		o.Expect(message).To(o.ContainSubstring("Job was active longer than specified deadline"))
		o.Expect(message).To(o.ContainSubstring(fakeReleasePayload))

		exutil.By("check version pod in ImagePullBackOff")
		// swinging betseen Init:0/4 Init:ErrImagePull and Init:ImagePullBackOff so need a few retries
		if err = waitForCondition(oc, 5, 30, "ImagePullBackOff",
			"get", "-n", "openshift-cluster-version", "pods", "-o", "jsonpath={.items[*].status.initContainerStatuses[0].state.waiting.reason}"); err != nil {
			//dump contents to log
			_ = oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-cluster-version", "pods", "-o", "yaml").Execute()
			exutil.AssertWaitPollNoErr(err, "ImagePullBackOff not detected in 30s")
		}

		exutil.By("Clear above unstarted upgrade")
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade", "--clear").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		if err = waitForCondition(oc, 30, 480, "True",
			"get", "clusterversion", "version", "-o", "jsonpath={.status.conditions[?(@.type=='ReleaseAccepted')].status}"); err != nil {
			//dump contents to log
			_ = oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterversion", "version", "-o", "yaml").Execute()
			exutil.AssertWaitPollNoErr(err, "ReleaseAccepted condition is not false in 8m")
		}

		exutil.By("check ReleaseAccepted False have correct message")
		message, err = getCVObyJP(oc, ".status.conditions[?(.type=='ReleaseAccepted')].message")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(message).To(o.ContainSubstring(regexp.MustCompile(` architecture=".*"`).ReplaceAllString(originalMessage, ""))) // until OCPBUGS-4032 is fixed

		exutil.By("no version pod in ImagePullBackOff")
		if err = waitForCondition(oc, 5, 30, "",
			"get", "-n", "openshift-cluster-version", "pods", "-o", "jsonpath={.items[*].status.initContainerStatuses[0].state.waiting.reason}"); err != nil {
			//dump contents to log
			_ = oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-cluster-version", "pods", "-o", "yaml").Execute()
			exutil.AssertWaitPollNoErr(err, "ImagePullBackOff not cleared in 30s")
		}
	})

	//author: jiajliu@redhat.com
	g.It("Longduration-NonPreRelease-Author:jiajliu-High-46017-CVO should keep reconcile manifests when update failed on precondition check [Disruptive]", func() {
		exutil.SkipBaselineCaps(oc, "None")
		//Take openshift-marketplace/deployment as an example, it can be any resource which included in manifest files
		resourceKindName := "deployment/marketplace-operator"
		resourceNamespace := "openshift-marketplace"
		exutil.By("Check default rollingUpdate strategy in a fresh installed cluster.")
		defaultValueMaxUnavailable, err := oc.AsAdmin().WithoutNamespace().Run("get").
			Args(resourceKindName, "-o=jsonpath={.spec.strategy.rollingUpdate.maxUnavailable}",
				"-n", resourceNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(defaultValueMaxUnavailable).To(o.Equal("25%"))

		exutil.By("Ensure upgradeable=false.")
		upgStatusOutput, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(upgStatusOutput, "Upgradeable=False") {
			e2e.Logf("Enable upgradeable=false explicitly...")
			//set overrides in cv to trigger upgradeable=false condition if it is not enabled by default
			err = setCVOverrides(oc, "deployment", "network-operator", "openshift-network-operator")
			defer unsetCVOverrides(oc)
			exutil.AssertWaitPollNoErr(err, "timeout to set overrides!")
		}

		exutil.By("Trigger update when upgradeable=false and precondition check fail.")
		//Choose a fixed old release payload to trigger a fake upgrade when upgradeable=false
		oldReleasePayload := "quay.io/openshift-release-dev/ocp-release@sha256:fd96300600f9585e5847f5855ca14e2b3cafbce12aefe3b3f52c5da10c4476eb"
		err = oc.AsAdmin().WithoutNamespace().Run("adm").
			Args("upgrade", "--allow-explicit-upgrade", "--to-image", oldReleasePayload).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() { o.Expect(recoverReleaseAccepted(oc)).NotTo(o.HaveOccurred()) }()

		if err = waitForCondition(oc, 30, 480, "False",
			"get", "clusterversion", "version", "-o", "jsonpath={.status.conditions[?(@.type=='ReleaseAccepted')].status}"); err != nil {
			//dump contents to log
			_ = oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterversion", "version", "-o", "yaml").Execute()
			exutil.AssertWaitPollNoErr(err, "ReleaseAccepted condition is not false in 8m")
		}

		exutil.By("Change strategy.rollingUpdate.maxUnavailable to be 50%.")
		_, err = ocJSONPatch(oc, resourceNamespace, resourceKindName, []JSONp{
			{"replace", "/spec/strategy/rollingUpdate/maxUnavailable", "50%"},
		})
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			out, err := ocJSONPatch(oc, resourceNamespace, resourceKindName, []JSONp{
				{"replace", "/spec/strategy/rollingUpdate/maxUnavailable", "25%"},
			})
			o.Expect(err).NotTo(o.HaveOccurred(), out)
		}()

		exutil.By("Check the deployment was reconciled back.")
		exutil.AssertWaitPollNoErr(wait.Poll(30*time.Second, 20*time.Minute, func() (bool, error) {
			valueMaxUnavailable, err := oc.AsAdmin().WithoutNamespace().Run("get").
				Args(resourceKindName, "-o=jsonpath={.spec.strategy.rollingUpdate.maxUnavailable}", "-n", resourceNamespace).Output()
			if err != nil {
				return false, fmt.Errorf("oc get %s -n %s returned error: %v", resourceKindName, resourceNamespace, err)
			}
			if strings.Compare(valueMaxUnavailable, defaultValueMaxUnavailable) != 0 {
				e2e.Logf("valueMaxUnavailable is %v. Waiting for deployment being reconciled...", valueMaxUnavailable)
				return false, nil
			}
			return true, nil
		}), "the deployment was not reconciled back in 20min.")
	})

	//author: jiajliu@redhat.com
	g.It("Longduration-NonPreRelease-Author:jiajliu-Medium-51973-setting cv.overrides should work while ReleaseAccepted=False [Disruptive]", func() {
		resourceKind := "deployment"
		resourceName := "network-operator"
		resourceNamespace := "openshift-network-operator"

		exutil.By("Trigger ReleaseAccepted=False condition.")
		fakeReleasePayload := "quay.io/openshift-release-dev-test/ocp-release@sha256:39efe13ef67cb4449f5e6cdd8a26c83c07c6a2ce5d235dfbc3ba58c64418fcf3"
		err := oc.AsAdmin().WithoutNamespace().Run("adm").
			Args("upgrade", "--allow-explicit-upgrade", "--to-image", fakeReleasePayload).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() { o.Expect(recoverReleaseAccepted(oc)).NotTo(o.HaveOccurred()) }()

		if err = waitForCondition(oc, 30, 480, "False",
			"get", "clusterversion", "version", "-o", "jsonpath={.status.conditions[?(@.type=='ReleaseAccepted')].status}"); err != nil {
			//dump contents to log
			_ = oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterversion", "version", "-o", "yaml").Execute()
			exutil.AssertWaitPollNoErr(err, "ReleaseAccepted condition is not false in 8m")
		}

		exutil.By("Disable deployment/network-operator's management through setting cv.overrides.")
		err = setCVOverrides(oc, resourceKind, resourceName, resourceNamespace)
		defer unsetCVOverrides(oc)
		exutil.AssertWaitPollNoErr(err, "timeout to set overrides!")

		exutil.By("Check default rollingUpdate strategy.")
		defaultValueMaxUnavailable, err := oc.AsAdmin().WithoutNamespace().Run("get").
			Args(resourceKind, resourceName, "-o=jsonpath={.spec.strategy.rollingUpdate.maxUnavailable}",
				"-n", resourceNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(defaultValueMaxUnavailable).To(o.Equal("1"))

		exutil.By("Change strategy.rollingUpdate.maxUnavailable to be 50%.")
		_, err = ocJSONPatch(oc, resourceNamespace, fmt.Sprintf("%s/%s", resourceKind, resourceName), []JSONp{
			{"replace", "/spec/strategy/rollingUpdate/maxUnavailable", "50%"},
		})
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			out, err := ocJSONPatch(oc, resourceNamespace, fmt.Sprintf("%s/%s", resourceKind, resourceName), []JSONp{
				{"replace", "/spec/strategy/rollingUpdate/maxUnavailable", 1},
			})
			o.Expect(err).NotTo(o.HaveOccurred(), out)
		}()

		exutil.By("Check the deployment will not be reconciled back.")
		err = wait.Poll(30*time.Second, 8*time.Minute, func() (bool, error) {
			valueMaxUnavailable, err := oc.AsAdmin().WithoutNamespace().Run("get").
				Args(resourceKind, resourceName, "-o=jsonpath={.spec.strategy.rollingUpdate.maxUnavailable}", "-n", resourceNamespace).Output()
			if err != nil {
				return false, fmt.Errorf("oc get %s %s -n %s returned error: %v", resourceKind, resourceName, resourceNamespace, err)
			}
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

	//author: jiajliu@redhat.com
	g.It("Author:jiajliu-Medium-53906-The architecture info in clusterversion’s status should be correct", func() {
		const heterogeneousArchKeyword = "multi"
		expectedArchMsg := "architecture=\"Multi\""
		exutil.By("Get release info from current cluster")
		releaseInfo, err := getReleaseInfo(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(releaseInfo).NotTo(o.BeEmpty())

		exutil.By("Check the arch info cv.status is expected")
		cvArchInfo, err := getCVObyJP(oc, ".status.conditions[?(.type=='ReleaseAccepted')].message")
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Release payload info in cv.status: %v", cvArchInfo)

		if releaseArch := gjson.Get(releaseInfo, `metadata.metadata.release\.openshift\.io/architecture`).String(); releaseArch != heterogeneousArchKeyword {
			e2e.Logf("This current release is a non-heterogeneous payload")
			//It's a non-heterogeneous payload, the architecture info in clusterversion’s status should be consistent with runtime.GOARCH.

			output, err := oc.AsAdmin().WithoutNamespace().
				Run("get").Args("nodes", "-o",
				"jsonpath={.items[*].status.nodeInfo.architecture}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			nodesArchInfo := strings.Split(strings.TrimSpace(output), " ")
			e2e.Logf("Nodes arch list: %v", nodesArchInfo)

			for _, nArch := range nodesArchInfo {
				if nArch != nodesArchInfo[0] {
					e2e.Failf("unexpected node arch in non-hetero cluster: %s expecting: %s",
						nArch, nodesArchInfo[0])
				}
			}

			e2e.Logf("Expected arch info: %v", nodesArchInfo[0])
			o.Expect(cvArchInfo).To(o.ContainSubstring(nodesArchInfo[0]))
		} else {
			e2e.Logf("This current release is a heterogeneous payload")
			// It's a heterogeneous payload, the architecture info in clusterversion’s status should be multi.
			e2e.Logf("Expected arch info: %v", expectedArchMsg)
			o.Expect(cvArchInfo).To(o.ContainSubstring(expectedArchMsg))
		}
	})

	// author: jianl@redhat.com
	g.It("Longduration-NonPreRelease-Author:jianl-high-68398-CVO reconcile SCC resources which have release.openshift.io/create-only: true [Slow]", func() {
		exutil.By("Get default SCC spec")
		scc := "restricted"
		sccManifest := "0000_20_kube-apiserver-operator_00_scc-restricted.yaml"
		tempDataDir, err := extractManifest(oc)
		defer func() { o.Expect(os.RemoveAll(tempDataDir)).NotTo(o.HaveOccurred()) }()
		o.Expect(err).NotTo(o.HaveOccurred())

		goodSCCFile, getSCCFileErr := oc.AsAdmin().WithoutNamespace().
			Run("get").Args("scc", scc, "-ojson").OutputToFile("ocp-68398.json")
		o.Expect(getSCCFileErr).NotTo(o.HaveOccurred())
		defer func() {
			o.Expect(oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", goodSCCFile, "--force").Execute()).NotTo(o.HaveOccurred())
			o.Expect(os.RemoveAll(goodSCCFile)).NotTo(o.HaveOccurred())
		}()
		originalOutputByte, readFileErr := os.ReadFile(goodSCCFile)
		o.Expect(readFileErr).NotTo(o.HaveOccurred())
		originalOutput := string(originalOutputByte)
		o.Expect(originalOutput).Should(o.ContainSubstring("release.openshift.io/create-only"))
		createOnly := gjson.Get(originalOutput, "metadata.annotations.release?openshift?io/create-only").Bool()
		o.Expect(createOnly).Should(o.BeTrue())

		// update allowHostIPC should not cause upgradeable=false and will not be reconsiled
		originalAllowHostIPC := gjson.Get(originalOutput, "allowHostIPC").Bool()
		ocJSONPatch(oc, "", fmt.Sprintf("scc/%s", scc), []JSONp{{"replace", "/allowHostIPC", !originalAllowHostIPC}})
		o.Consistently(func() bool {
			hostIPC_output, _ := oc.AsAdmin().WithoutNamespace().
				Run("get").Args("scc", scc, "-ojson").Output()
			boolValue := gjson.Get(hostIPC_output, "allowHostIPC").Bool()
			// boolValue == original_allowHostIPC means resource has been reconciled
			return boolValue
		}, 300*time.Second, 30*time.Second).ShouldNot(o.Equal(originalAllowHostIPC), "Error: allowHostIPC was reconciled back, check point: allowHostIPC")

		upgradeableOutput, _ := oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade").Output()
		o.Expect(string(upgradeableOutput)).ShouldNot(o.ContainSubstring("Detected modified SecurityContextConstraints"), "Error occured in oc adm upgrade")

		originalVolumes := gjson.Get(originalOutput, "volumes").Array()
		ocJSONPatch(oc, "", fmt.Sprintf("scc/%s", scc), []JSONp{
			{"remove", "/volumes/0", nil},
			{"add", "/volumes/0", "Test"},
		})
		o.Consistently(func() bool {
			volumesOutput, _ := oc.AsAdmin().WithoutNamespace().
				Run("get").Args("scc", scc, "-ojson").Output()
			newVolumes := gjson.Get(volumesOutput, "volumes").Array()
			return newVolumes[0].String() == "Test" && newVolumes[5].String() != originalVolumes[4].String()
		}, 5*time.Minute, 30*time.Second).Should(o.BeTrue(), fmt.Sprintf("Error: %s was reconciled back, check point: volumes", scc))

		upgradeableOutput, _ = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade").Output()
		o.Expect(string(upgradeableOutput)).ShouldNot(o.ContainSubstring("Detected modified SecurityContextConstraints"), "Error occured in oc adm upgrade")

		// allowPrivilegeEscalation should be set to true immediately after removing it
		pe_log, _ := ocJSONPatch(oc, "", fmt.Sprintf("scc/%s", scc), []JSONp{
			{"remove", "/allowPrivilegeEscalation", nil},
		})
		e2e.Logf(string(pe_log))
		o.Consistently(func() bool {
			pe_output, _ := oc.AsAdmin().WithoutNamespace().
				Run("get").Args("scc", scc, "-ojson").Output()
			pe_value := gjson.Get(pe_output, "allowPrivilegeEscalation").Bool()
			return pe_value
		}, 30*time.Second, 10*time.Second).Should(o.BeTrue(), "Error: allowPrivilegeEscalation is not true")

		upgradeableOutput, _ = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade").Output()
		o.Expect(string(upgradeableOutput)).ShouldNot(o.ContainSubstring("Detected modified SecurityContextConstraints"), "Error occured in oc adm upgrade")

		// SCC should be recreated after deleting it
		outputBeforeDelete, _ := oc.AsAdmin().WithoutNamespace().
			Run("get").Args("scc", scc, "-ojson").Output()
		resourceVersion := gjson.Get(outputBeforeDelete, "metadata.resourceVersion").String()
		deleteLog, deleteErr := oc.AsAdmin().WithoutNamespace().Run("delete").Args("scc", scc).Output()
		e2e.Logf("Delete scc %s: %s", scc, deleteLog)
		o.Expect(deleteErr).NotTo(o.HaveOccurred())
		err = wait.PollUntilContextTimeout(context.Background(), 30*time.Second, 10*time.Minute, true, func(context.Context) (bool, error) {
			newOutput, newErr := oc.AsAdmin().WithoutNamespace().
				Run("get").Args("scc", scc, "-ojson").Output()

			if newErr != nil {
				return false, nil
			} else {
				newResourceVersion := gjson.Get(newOutput, "metadata.resourceVersion").String()
				return resourceVersion != newResourceVersion, nil
			}
		})
		exutil.AssertWaitPollNoErr(err, "Error: SCC have not recreated after 5 minutes")

		manifest := filepath.Join(tempDataDir, "manifest", sccManifest)
		manifestContent, _ := os.ReadFile(manifest)
		expectedValues, err := exutil.Yaml2Json(string(manifestContent))
		o.Expect(err).NotTo(o.HaveOccurred())

		finalOutput, _ := oc.AsAdmin().WithoutNamespace().
			Run("get").Args("scc", scc, "-ojson").Output()
		o.Expect(finalOutput).Should(o.ContainSubstring("release.openshift.io/create-only"))
		createOnly = gjson.Get(finalOutput, "metadata.annotations.release?openshift?io/create-only").Bool()
		o.Expect(createOnly).Should(o.BeTrue())

		final_allowHostIPC := gjson.Get(finalOutput, "allowHostIPC").Bool()
		o.Expect(final_allowHostIPC).Should(o.Equal(gjson.Get(expectedValues, "allowHostIPC").Bool()), "allowHostIPC is not correct")
		final_pe_value := gjson.Get(finalOutput, "allowPrivilegeEscalation").Bool()
		pe := gjson.Get(expectedValues, "allowPrivilegeEscalation").Bool()
		e2e.Logf("pe: %v", pe)
		o.Expect(final_pe_value).Should(o.Equal(pe), "allowPrivilegeEscalation is not correct")
		finalVolumes := gjson.Get(finalOutput, "volumes").Array()
		expectedVolumes := gjson.Get(expectedValues, "volumes").Array()
		o.Expect(len(finalVolumes)).Should(o.Equal(len(expectedVolumes)), "volumes have different number of expected values")
		var finalResult []string
		for _, v := range finalVolumes {
			finalResult = append(finalResult, v.Str)
		}
		var expectedResult []string
		for _, v := range expectedVolumes {
			expectedResult = append(expectedResult, v.Str)
		}
		e2e.Logf("Final volumes are: %v", finalResult)
		e2e.Logf("Expected volumes are: %v", expectedResult)
		o.Expect(finalResult).Should(o.ContainElements(expectedResult), "volumns are not exact equal to manifest")

		upgradeableOutput, _ = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade").Output()
		o.Expect(string(upgradeableOutput)).ShouldNot(o.ContainSubstring("Detected modified SecurityContextConstraints"), "Error occured in oc adm upgrade")
	})

	// author: jianl@redhat.com
	g.It("Longduration-NonPreRelease-Author:jianl-high-68397-CVO reconciles SCC resources which do not have release.openshift.io/create-only: true [Disruptive]", func() {
		scc := "restricted-v2"

		exutil.By("Get default SCC spec")
		sccManifest := "0000_20_kube-apiserver-operator_00_scc-restricted-v2.yaml"
		tempDataDir, err := extractManifest(oc)
		defer func() { o.Expect(os.RemoveAll(tempDataDir)).NotTo(o.HaveOccurred()) }()
		o.Expect(err).NotTo(o.HaveOccurred())

		goodSCCFile, getSCCFileErr := oc.AsAdmin().WithoutNamespace().
			Run("get").Args("scc", scc, "-ojson").OutputToFile("ocp-68397-scc.json")
		o.Expect(getSCCFileErr).NotTo(o.HaveOccurred())
		defer func() {
			o.Expect(oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", goodSCCFile, "--force").Execute()).NotTo(o.HaveOccurred())
			o.Expect(os.RemoveAll(goodSCCFile)).NotTo(o.HaveOccurred())
			output, _ := oc.AsAdmin().WithoutNamespace().
				Run("get").Args("scc", scc, "-ojson").Output()
			e2e.Logf("New scc after runing apply command: \n %s", output)
		}()
		originalOutputByte, readFileErr := os.ReadFile(goodSCCFile)
		o.Expect(readFileErr).NotTo(o.HaveOccurred())
		originalOutput := string(originalOutputByte)
		o.Expect(originalOutput).ShouldNot(o.ContainSubstring("release.openshift.io/create-only"))

		// update allowHostIPC should not cause upgradeable=false and will be reconciled
		originalAllowHostIPC := gjson.Get(originalOutput, "allowHostIPC").Bool()
		ocJSONPatch(oc, "", fmt.Sprintf("scc/%s", scc), []JSONp{{"replace", "/allowHostIPC", !originalAllowHostIPC}})

		var observedAllowHostIPC bool
		var output string
		err = wait.Poll(30*time.Second, 5*time.Minute, func() (bool, error) {
			output, err = oc.AsAdmin().WithoutNamespace().
				Run("get").Args("scc", scc, "-ojson").Output()
			if err != nil {
				return false, err
			} else {
				observedAllowHostIPC = gjson.Get(output, "allowHostIPC").Bool()
				// observedAllowHostIPC == original_allowHostIPC means resource has been reconciled
				return observedAllowHostIPC == originalAllowHostIPC, nil
			}
		})
		exutil.AssertWaitPollNoErr(err, "AllowHostIPC is not reconciled")

		// there is no Upgradeable=False guard
		o.Expect(checkUpdates(oc, false, 10, 30,
			"Detected modified SecurityContextConstraints")).To(o.BeFalse(), "Error occured in oc adm upgrade after updating allowHostIPC")

		// SCC should be recreated after deleting it
		outputBeforeDelete, _ := oc.AsAdmin().WithoutNamespace().
			Run("get").Args("scc", scc, "-ojson").Output()
		resourceVersion := gjson.Get(outputBeforeDelete, "metadata.resourceVersion").String()
		deleteLog, deleteErr := oc.AsAdmin().WithoutNamespace().Run("delete").Args("scc", scc).Output()
		e2e.Logf("Delete scc %s: %s", scc, deleteLog)
		o.Expect(deleteErr).NotTo(o.HaveOccurred())
		// wait some minutes scc will regenerated
		var newErr error
		err = wait.Poll(30*time.Second, 5*time.Minute, func() (bool, error) {
			output, newErr = oc.AsAdmin().WithoutNamespace().
				Run("get").Args("scc", scc, "-ojson").Output()

			if newErr != nil {
				return false, nil
			} else {
				newResourceVersion := gjson.Get(output, "metadata.resourceVersion").String()
				return resourceVersion != newResourceVersion, nil
			}
		})
		exutil.AssertWaitPollNoErr(err, "Error: SCC have not recreated after 5 minutes")

		o.Expect(checkUpdates(oc, false, 30, 60*3,
			"Detected modified SecurityContextConstraints")).To(o.BeFalse(), "Error occured in oc adm upgrade after deleting scc")

		manifest := filepath.Join(tempDataDir, "manifest", sccManifest)
		manifestContent, _ := os.ReadFile(manifest)
		expectedValues, err := exutil.Yaml2Json(string(manifestContent))
		o.Expect(err).NotTo(o.HaveOccurred())

		observedAllowHostIPC = gjson.Get(output, "allowHostIPC").Bool()
		allowHostIPCManifest := gjson.Get(expectedValues, "allowHostIPC").Bool()
		o.Expect(allowHostIPCManifest).Should(o.Equal(observedAllowHostIPC), "Error: allowHostIPC is not same with its value in manifest")

		finalVolumes := gjson.Get(output, "volumes").Array()
		expectedVolumesManifest := gjson.Get(expectedValues, "volumes").Array()
		o.Expect(len(finalVolumes)).Should(o.Equal(len(expectedVolumesManifest)), "Error: volumes have different number of expected values")
		var finalResult []string
		for _, v := range finalVolumes {
			finalResult = append(finalResult, v.Str)
		}
		var expectedResult []string
		for _, v := range expectedVolumesManifest {
			expectedResult = append(expectedResult, v.Str)
		}
		e2e.Logf("Final volumes are: %v", finalResult)
		e2e.Logf("Expected volumes in menifest are: %v", expectedResult)
		o.Expect(finalResult).Should(o.ContainElements(expectedResult), "Error: volumns are not exactly equal to manifest")

		// allowPrivilegeEscalation should be set to true immediately after removing it
		allowPrivilegeEscalationManifest := gjson.Get(manifest, "allowPrivilegeEscalation").Bool()
		ocJSONPatch(oc, "", fmt.Sprintf("scc/%s", scc), []JSONp{
			{"remove", "/allowPrivilegeEscalation", nil},
		})
		output, _ = oc.AsAdmin().WithoutNamespace().
			Run("get").Args("scc", scc, "-ojson").Output()
		allowPrivilegeEscalation := gjson.Get(output, "allowPrivilegeEscalation").Bool()
		o.Expect(allowPrivilegeEscalation).Should(o.BeTrue(), "Error: allowPrivilegeEscalation is not be set to true immediately")

		err = wait.Poll(30*time.Second, 10*time.Minute, func() (bool, error) {
			output, _ = oc.AsAdmin().WithoutNamespace().
				Run("get").Args("scc", scc, "-ojson").Output()
			allowPrivilegeEscalation = gjson.Get(output, "allowPrivilegeEscalation").Bool()
			return allowPrivilegeEscalation == allowPrivilegeEscalationManifest, nil
		})
		exutil.AssertWaitPollNoErr(err, "Error: allowPrivilegeEscalation is not be set to manifest")

		o.Expect(checkUpdates(oc, false, 30, 60*3,
			"Detected modified SecurityContextConstraints")).To(o.BeFalse(), "Error: upgrade guard error occured for SecurityContextConstraints")

		ocJSONPatch(oc, "", fmt.Sprintf("scc/%s", scc), []JSONp{
			{"remove", "/volumes/4", nil},
			{"add", "/volumes/0", "Test"},
		})
		expectedResult = expectedResult[:0]
		for _, v := range expectedVolumesManifest {
			expectedResult = append(expectedResult, v.Str)
		}
		err = wait.Poll(30*time.Second, 5*time.Minute, func() (bool, error) {
			output, _ = oc.AsAdmin().WithoutNamespace().
				Run("get").Args("scc", scc, "-ojson").Output()
			finalVolumes := gjson.Get(output, "volumes").Array()
			if len(finalVolumes) != len(expectedResult) {
				return false, nil
			}
			finalResult = finalResult[:0]
			for _, v := range finalVolumes {
				finalResult = append(finalResult, v.Str)
			}
			return reflect.DeepEqual(finalResult, expectedResult), nil
		})
		exutil.AssertWaitPollNoErr(err, "volumns are not correct")
		o.Expect(checkUpdates(oc, false, 1, 10,
			"Detected modified SecurityContextConstraints")).To(o.BeFalse(), "Error: There should not be upgradeable=false gate for non-4.13 cluster")
	})

	//author: jiajliu@redhat.com
	g.It("NonPreRelease-Author:jiajliu-Medium-70931-CVO reconcile metadata on ClusterOperators [Disruptive]", func() {
		var annotationCOs []annotationCO
		resourcePath := "/metadata/annotations"
		exutil.By("Remove metadata.annotation")
		operatorName, err := oc.AsAdmin().WithoutNamespace().Run("get").
			Args("co", "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		operatorList := strings.Fields(operatorName)
		defer func() {
			for _, annotationCO := range annotationCOs {
				anno, _ := oc.AsAdmin().WithoutNamespace().Run("get").
					Args("co", annotationCO.name, "-o=jsonpath={.metadata.annotations}").Output()
				if anno == "" {
					_, err = ocJSONPatch(oc, "", "clusteroperator/"+annotationCO.name, []JSONp{{"add", resourcePath, annotationCO.annotation}})
					o.Expect(err).NotTo(o.HaveOccurred())
				}
			}
		}()
		for _, op := range operatorList {
			var anno map[string]string
			annoOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").
				Args("co", op, "-o=jsonpath={.metadata.annotations}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			err = json.Unmarshal([]byte(annoOutput), &anno)
			o.Expect(err).NotTo(o.HaveOccurred())
			annotationCOs = append(annotationCOs, annotationCO{op, anno})
			_, err = ocJSONPatch(oc, "", "clusteroperator/"+op, []JSONp{{"remove", resourcePath, nil}})
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		exutil.By("Check metadata.annotation is reconciled back")
		for _, op := range operatorList {
			o.Eventually(func() string {
				anno, _ := oc.AsAdmin().WithoutNamespace().Run("get").
					Args("co", op, "-o=jsonpath={.metadata.annotations}").Output()
				return anno
			}, 5*time.Minute, 1*time.Minute).ShouldNot(o.BeEmpty(), fmt.Sprintf("Fail to reconcile metadata of %s", op))
		}
	})

	g.It("Author:jianl-ConnectedOnly-Medium-77520-oc adm upgrade recommend", func() {
		exutil.By("Check if it's a GCP cluster")
		exutil.SkipIfPlatformTypeNot(oc, "gcp")

		exutil.By("export OC_ENABLE_CMD_UPGRADE_RECOMMEND=true")
		os.Setenv("OC_ENABLE_CMD_UPGRADE_RECOMMEND", "true")
		defer func() { os.Setenv("OC_ENABLE_CMD_UPGRADE_RECOMMEND", "") }()

		exutil.By("oc adm upgrade recommend --help")
		help, err := oc.AsAdmin().WithoutNamespace().Run("adm").
			Args("upgrade", "recommend", "--help").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(help).Should(o.ContainSubstring("This subcommand is read-only and does not affect the state of the cluster. To request an update, use the 'oc adm upgrade' subcommand."))
		o.Expect(help).Should(o.ContainSubstring("--show-outdated-releases=false"))
		o.Expect(help).Should(o.ContainSubstring("--version=''"))

		exutil.By("Update graph data")
		testDataDir := exutil.FixturePath("testdata", "ota/cvo")
		graphFile := filepath.Join(testDataDir, "cincy-77520.json")
		e2e.Logf("Origin graph template file path: ", graphFile)
		dest := filepath.Join(testDataDir, "cincy-77520_bak.json")
		err = copy(graphFile, dest)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() { os.Remove(dest) }()

		version, err := getCVObyJP(oc, ".status.history[0].version")
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Current OCP version is: ", version)
		major_minor := strings.Split(string(version), ".")

		exutil.By("Update graphFile with real version")
		err = updateFile(dest, "current_version", version)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = updateFile(dest, "major", major_minor[0])
		o.Expect(err).NotTo(o.HaveOccurred())
		err = updateFile(dest, "minor", major_minor[1])
		o.Expect(err).NotTo(o.HaveOccurred())
		next, _ := strconv.Atoi(major_minor[1])
		next_minor := strconv.Itoa(next + 1)
		err = updateFile(dest, "next", next_minor)
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("New graph template file path: ", dest)

		exutil.By("Patch upstream and channel")
		projectID := "openshift-qe"
		ctx := context.Background()
		client, err := storage.NewClient(ctx)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() { o.Expect(client.Close()).NotTo(o.HaveOccurred()) }()

		graphURL, bucket, object, _, _, err := buildGraph(
			client, oc, projectID, dest)
		defer func() { o.Expect(DeleteBucket(client, bucket)).NotTo(o.HaveOccurred()) }()
		defer func() { o.Expect(DeleteObject(client, bucket, object)).NotTo(o.HaveOccurred()) }()
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = ocJSONPatch(oc, "", "clusterversion/version", []JSONp{
			{"add", "/spec/upstream", graphURL},
			{"add", "/spec/channel", "channel-b"},
		})
		o.Expect(err).NotTo(o.HaveOccurred())
		z_stream_version1 := fmt.Sprintf("%s.%s.998", major_minor[0], major_minor[1])
		z_stream_version2 := fmt.Sprintf("%s.%s.999", major_minor[0], major_minor[1])
		y_stream_version1 := fmt.Sprintf("%s.%s.997", major_minor[0], next_minor)
		y_stream_version2 := fmt.Sprintf("%s.%s.998", major_minor[0], next_minor)
		y_stream_version3 := fmt.Sprintf("%s.%s.999", major_minor[0], next_minor)

		exutil.By("Check oc adm upgrade recommend")
		// We need to wait some minutes for the first time to get recommend after patch upstream
		err = wait.Poll(10*time.Second, 1*time.Minute, func() (bool, error) {
			output, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade", "recommend").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if !strings.Contains(output, "Upstream: "+graphURL) {
				return false, nil
			}
			if !strings.Contains(output, "Channel:") {
				return false, nil
			}
			if !strings.Contains(output, z_stream_version1+"    no known issues relevant to this cluster") {
				return false, nil
			}
			if !strings.Contains(output, z_stream_version2+"    no known issues relevant to this cluster") {
				return false, nil
			}
			if !strings.Contains(output, y_stream_version3+"    no known issues relevant to this cluster") {
				return false, nil
			}
			if !strings.Contains(output, "MultipleReasons") {
				return false, nil
			}
			//major.next.997 is older than major.next.998 and major.next.999, so should not display it in output
			if strings.Contains(output, y_stream_version1) {
				return false, nil
			}

			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "oc adm upgrade recommend fail")
		output, _ := oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade", "recommend").Output()
		e2e.Logf("output: \n", output)

		exutil.By("Check oc adm upgrade recommend --show-outdated-releases")
		output, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade", "recommend", "--show-outdated-releases").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("output: \n", output)
		o.Expect(output).Should(o.ContainSubstring("Upstream: " + graphURL))
		o.Expect(output).Should(o.ContainSubstring("Channel"))
		o.Expect(output).Should(o.ContainSubstring(fmt.Sprintf("%s    no known issues relevant to this cluster", z_stream_version1)))
		o.Expect(output).Should(o.ContainSubstring(fmt.Sprintf("%s    no known issues relevant to this cluster", z_stream_version2)))
		o.Expect(output).Should(o.ContainSubstring(fmt.Sprintf("%s    no known issues relevant to this cluster", y_stream_version1)))
		o.Expect(output).Should(o.ContainSubstring(fmt.Sprintf("%s    MultipleReasons", y_stream_version2)))
		o.Expect(output).Should(o.ContainSubstring(fmt.Sprintf("%s    no known issues relevant to this cluster", y_stream_version3)))

		exutil.By("Check oc adm upgrade recommend --version " + y_stream_version2)
		output, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade", "recommend", "--version", y_stream_version2).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("output: \n", output)
		o.Expect(output).Should(o.ContainSubstring("Upstream: " + graphURL))
		o.Expect(output).Should(o.ContainSubstring("Channel"))
		o.Expect(output).Should(o.ContainSubstring(fmt.Sprintf("Update to %s Recommended=False", y_stream_version2)))
		o.Expect(output).Should(o.ContainSubstring("Reason: MultipleReasons"))
		o.Expect(output).Should(o.ContainSubstring("On clusters on default invoker user, this imaginary bug can happen"))
		o.Expect(output).Should(o.ContainSubstring("Too many CI failures on this release, so do not update to it"))

		exutil.By("Check oc adm upgrade recommend --version " + y_stream_version3)
		output, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade", "recommend", "--version", y_stream_version3).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("output: \n", output)
		o.Expect(output).Should(o.ContainSubstring("Upstream: " + graphURL))
		o.Expect(output).Should(o.ContainSubstring("Channel"))
		expected_msg := fmt.Sprintf("Update to %s has no known issues relevant to this cluster.", y_stream_version3)
		o.Expect(output).Should(o.ContainSubstring(expected_msg))
		o.Expect(output).Should(o.ContainSubstring("Image: quay.io/openshift-release-dev/ocp-release@sha256:d2d34aafe0adda79953dd928b946ecbda34673180ee9a80d2ee37c123a0f510c"))
		o.Expect(output).Should(o.ContainSubstring("Release URL: https://amd64.ocp.releases.ci.openshift.org/releasestream/4-dev-preview/release/4.y+1.0"))

		exutil.By("Check oc adm upgrade recommend --version 4.999.999")
		output, _ = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade", "recommend", "--version", "4.999.999").Output()
		e2e.Logf("output: \n", output)
		o.Expect(output).Should(o.ContainSubstring("Upstream: " + graphURL))
		o.Expect(output).Should(o.ContainSubstring("Channel"))
		o.Expect(output).Should(o.ContainSubstring("error: no updates to 4.999 available, so cannot display context for the requested release 4.999.999"))
	})

})
