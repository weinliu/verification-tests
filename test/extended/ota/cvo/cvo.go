package cvo

import (
	"context"
	"encoding/json"
	"fmt"
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

	project_name := "openshift-cluster-version"

	oc := exutil.NewCLIWithoutNamespace(project_name)

	//author: yanyang@redhat.com
	g.It("ConnectedOnly-Author:yanyang-Low-47175-upgrade cluster when current version is in the upstream but there are not update paths [Serial]", func() {
		orgUpstream, _ := getCVObyJP(oc, ".spec.upstream")

		defer restoreCVSpec(orgUpstream, "nochange", oc)

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

		_, err = ocJsonPatch(oc, "", "clusterversion/version", []JSONp{{"add", "/spec/upstream", graphURL}})
		o.Expect(err).NotTo(o.HaveOccurred())

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
		cvo_deployment_yaml, err := GetDeploymentsYaml(oc, "cluster-version-operator", project_name)
		o.Expect(err).NotTo(o.HaveOccurred())
		var keywords = []string{"--listen=0.0.0.0:9099",
			"--serving-cert-file=/etc/tls/serving-cert/tls.crt",
			"--serving-key-file=/etc/tls/serving-cert/tls.key"}
		for _, v := range keywords {
			o.Expect(cvo_deployment_yaml).Should(o.ContainSubstring(v))
		}

		g.By("Check cluster-version-operator binary help")
		cvo_pods_list, err := exutil.WaitForPods(
			oc.AdminKubeClient().CoreV1().Pods(project_name),
			exutil.ParseLabelsOrDie("k8s-app=cluster-version-operator"),
			exutil.CheckPodIsReady, 1, 3*time.Minute)
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Get cvo pods: %v", cvo_pods_list)
		output, err := PodExec(oc, "/usr/bin/cluster-version-operator start --help", project_name, cvo_pods_list[0])
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf(
			"/usr/bin/cluster-version-operator start --help executs error on %v", cvo_pods_list[0]))
		e2e.Logf(output)
		keywords = []string{"You must set both --serving-cert-file and --serving-key-file unless you set --listen empty"}
		for _, v := range keywords {
			o.Expect(output).Should(o.ContainSubstring(v))
		}

		g.By("Verify cvo metrics is only exported via https")
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").
			Args("servicemonitor", "cluster-version-operator",
				"-n", project_name, "-o=json").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		var result map[string]interface{}
		json.Unmarshal([]byte(output), &result)
		endpoints := result["spec"].(map[string]interface{})["endpoints"]
		e2e.Logf("Get cvo's spec.endpoints: %v", endpoints)
		o.Expect(endpoints).Should(o.HaveLen(1))

		output, err = oc.AsAdmin().WithoutNamespace().Run("get").
			Args("servicemonitor", "cluster-version-operator",
				"-n", project_name, "-o=jsonpath={.spec.endpoints[].scheme}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Get cvo's spec.endpoints scheme: %v", output)
		o.Expect(output).Should(o.Equal("https"))

		g.By("Get cvo endpoint URI")
		//output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("endpoints", "cluster-version-operator", "-n", project_name, "-o=jsonpath='{.subsets[0].addresses[0].ip}:{.subsets[0].ports[0].port}'").Output()
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").
			Args("endpoints", "cluster-version-operator",
				"-n", project_name, "--no-headers").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		re := regexp.MustCompile(`cluster-version-operator\s+([^\s]*)`)
		matched_result := re.FindStringSubmatch(output)
		e2e.Logf("Regex mached result: %v", matched_result)
		o.Expect(matched_result).Should(o.HaveLen(2))
		endpoint_uri := matched_result[1]
		e2e.Logf("Get cvo endpoint URI: %v", endpoint_uri)
		o.Expect(endpoint_uri).ShouldNot(o.BeEmpty())

		g.By("Check metric server is providing service https, but not http")
		cmd := fmt.Sprintf("curl http://%s/metrics", endpoint_uri)
		output, err = PodExec(oc, cmd, project_name, cvo_pods_list[0])
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("cmd %s executs error on %v", cmd, cvo_pods_list[0]))
		e2e.Logf(output)
		keywords = []string{"Client sent an HTTP request to an HTTPS server"}
		for _, v := range keywords {
			o.Expect(output).Should(o.ContainSubstring(v))
		}

		g.By("Check metric server is providing service via https correctly.")
		cmd = fmt.Sprintf("curl -k -I https://%s/metrics", endpoint_uri)
		output, err = PodExec(oc, cmd, project_name, cvo_pods_list[0])
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("cmd %s executs error on %v", cmd, cvo_pods_list[0]))
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
		projectID := "openshift-qe"
		ctx := context.Background()
		client, err := storage.NewClient(ctx)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer client.Close()

		graphURL, bucket, object, _, _, err := buildGraph(client, oc, projectID, "cincy.json")
		defer DeleteBucket(client, bucket)
		defer DeleteObject(client, bucket, object)
		o.Expect(err).NotTo(o.HaveOccurred())

		orgUpstream, _ := getCVObyJP(oc, ".spec.upstream")
		orgChannel, _ := getCVObyJP(oc, ".spec.channel")

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
		_, err = ocJsonPatch(oc, "", "clusterversion/version", []JSONp{
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
		orgUpstream, _ := getCVObyJP(oc, ".spec.upstream")
		orgChannel, _ := getCVObyJP(oc, ".spec.channel")

		//fmt.Printf("The original upstream is %s", orgUpstream)
		defer restoreCVSpec(orgUpstream, orgChannel, oc)

		g.By("Check when upstream is unset")
		if orgUpstream != "" {
			_, err := ocJsonPatch(oc, "", "clusterversion/version", []JSONp{{"remove", "/spec/upstream", nil}})
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

		_, err = ocJsonPatch(oc, "", "clusterversion/version", []JSONp{
			{"add", "/spec/upstream", graphURL},
			{"add", "/spec/channel", "channel-a"},
		})
		o.Expect(err).NotTo(o.HaveOccurred())
		exec.Command("bash", "-c", "sleep 5").Output()
		cmdOut, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(cmdOut).To(o.ContainSubstring(fmt.Sprintf("Upstream: %s", graphURL)))
		o.Expect(cmdOut).To(o.ContainSubstring("Channel: channel-a (available channels: channel-a, channel-b)"))
		o.Expect(cmdOut).To(o.ContainSubstring("Recommended updates:"))
		o.Expect(cmdOut).To(o.ContainSubstring(targetVersion + " " + targetPayload))

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
		o.Expect(cmdOut).NotTo(o.ContainSubstring("Upstream:"))
		o.Expect(cmdOut).NotTo(o.ContainSubstring("Channel:"))
		o.Expect(cmdOut).To(o.ContainSubstring("Reason: NoChannel"))
		o.Expect(cmdOut).To(o.ContainSubstring("Message: The update channel has not been configured."))
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
			alertDown := getAlertByName("ClusterOperatorDown")
			alertDegraded := getAlertByName("ClusterOperatorDegraded")
			o.Expect(alertDown).To(o.BeNil())
			if alertDegraded == nil || alertDegraded["state"] != "firing" {
				e2e.Logf("Waiting for alert ClusterOperatorDegraded to be triggered and fired...")
				return false, nil
			}
			o.Expect(alertDegraded["labels"].(map[string]interface{})["severity"].(string)).To(o.Equal("warning"))
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
			alertDegraded := getAlertByName("ClusterOperatorDegraded")
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
			alertDown := getAlertByName("ClusterOperatorDown")
			alertDegraded := getAlertByName("ClusterOperatorDegraded")
			if alertDown == nil || alertDegraded == nil {
				e2e.Logf("Waiting for alerts to be triggered...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "No alert triggerred!")

		g.By("Check alert ClusterOperatorDown fired.")
		err = wait.Poll(5*time.Minute, 10*time.Minute, func() (bool, error) {
			alertDown := getAlertByName("ClusterOperatorDown")
			if alertDown["state"] != "firing" {
				e2e.Logf("Waiting for alert ClusterOperatorDown to be triggered and fired...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "ClusterOperatorDown alert is not fired in 10m")

		g.By("Check alert ClusterOperatorDegraded fired.")
		err = wait.Poll(5*time.Minute, 20*time.Minute, func() (bool, error) {
			alertDegraded := getAlertByName("ClusterOperatorDegraded")
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
			alertDown := getAlertByName("ClusterOperatorDown")
			alertDegraded := getAlertByName("ClusterOperatorDegraded")
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

		masterNode, err := oc.AsAdmin().WithoutNamespace().Run("get").
			Args("pod", "-n", "openshift-authentication-operator",
				"-o=jsonpath={.items[].spec.nodeName}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Enable ClusterOperatorDown alert")
		err = oc.AsAdmin().Run("label").Args("node", masterNode, "kubernetes.io/os-").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().Run("label").Args("node", masterNode, "kubernetes.io/os=linux").Execute()

		g.By("Check ClusterOperatorDown condition...")
		err = waitForCondition(60, 300, "False", "oc get co authentication -ojson|jq -r '.status.conditions[]|select(.type==\"Available\").status'")
		exutil.AssertWaitPollNoErr(err, "authentication operator is not down in 5m")

		g.By("Check ClusterOperatorDown alert is fired correctly")
		err = wait.Poll(100*time.Second, 600*time.Second, func() (bool, error) {
			alertDown := getAlertByName("ClusterOperatorDown")
			if alertDown == nil || alertDown["state"] != "firing" {
				e2e.Logf("Waiting for alert ClusterOperatorDown to be triggered and fired...")
				return false, nil
			}
			o.Expect(alertDown["labels"].(map[string]interface{})["severity"].(string)).To(o.Equal("critical"))
			o.Expect(alertDown["annotations"].(map[string]interface{})["summary"].(string)).
				To(o.ContainSubstring("Cluster operator has not been available for 10 minutes."))
			o.Expect(alertDown["annotations"].(map[string]interface{})["description"].(string)).
				To(o.ContainSubstring("The authentication operator may be down or disabled"))
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "ClusterOperatorDown alert is not fired in 10m")

		g.By("Disable ClusterOperatorDown alert")
		err = oc.AsAdmin().Run("label").Args("node", masterNode, "kubernetes.io/os=linux").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check alert is disabled")
		err = wait.Poll(30*time.Second, 180*time.Second, func() (bool, error) {
			alertDown := getAlertByName("ClusterOperatorDown")
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
		_, err = ocJsonPatch(oc, namespace, fmt.Sprintf("deployment/%s", name), patch)
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
			_, err = ocJsonPatch(oc, namespace, fmt.Sprintf("deployment/%s", name), patch)
			o.Expect(err).NotTo(o.HaveOccurred())
			exutil.AssertWaitPollNoErr(pollErr, "Strategy is not reverted back after 5 minutes")
		}
	})

	//author: evakhoni@redhat.com
	g.It("Longduration-NonPreRelease-Author:evakhoni-Medium-48247-Prometheus is able to scrape metrics from the CVO after rotation of the signer ca in openshift-service-ca [Disruptive]", func() {

		g.By("Check for alerts Before signer ca rotation.")
		alertCVODown := getAlertByName("ClusterVersionOperatorDown")
		alertTargetDown := getAlert(".labels.alertname == \"TargetDown\" and .labels.service == \"cluster-version-operator\"")
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
			alertCVODown = getAlertByName("ClusterVersionOperatorDown")
			alertTargetDown = getAlert(".labels.alertname == \"TargetDown\" and .labels.service == \"cluster-version-operator\"")
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
		jsonpath := ".status.conditions[?(.type=='RetrievedUpdates')].status"
		status, err := getCVObyJP(oc, jsonpath)
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(status, "False") {
			e2e.Logf("no patch required. skipping upstream creation")
			targetVersion = GenerateReleaseVersion(oc)
			targetPayload = GenerateReleasePayload(oc)
		} else {
			origUpstream, _ := getCVObyJP(oc, ".spec.upstream")
			defer restoreCVSpec(origUpstream, "nochange", oc)

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

			_, err = ocJsonPatch(oc, "", "clusterversion/version", []JSONp{{"add", "/spec/upstream", graphURL}})
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Check RetrievedUpdates!=True after patching upstream")
			jsonpath = ".status.conditions[?(.type=='RetrievedUpdates')].status"
			err = wait.Poll(5*time.Second, 15*time.Second, func() (bool, error) {
				status, err := getCVObyJP(oc, jsonpath)
				o.Expect(err).NotTo(o.HaveOccurred())
				e2e.Logf("received status: '%s'", status)
				if strings.Contains(status, "False") {
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
		jsonpath = ".status.conditions[?(.type=='RetrievedUpdates')].status"
		err = wait.Poll(5*time.Second, 15*time.Second, func() (bool, error) {
			status, err := getCVObyJP(oc, jsonpath)
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(status, "False") {
				e2e.Logf("success - found status: %s", status)
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
		resourceName := "deployment/marketplace-operator"
		resourceNamespace := "openshift-marketplace"
		g.By("Check default rollingUpdate strategy in a fresh installed cluster.")
		defaultValueMaxUnavailable, err := oc.AsAdmin().WithoutNamespace().Run("get").
			Args(resourceName, "-o=jsonpath={.spec.strategy.rollingUpdate.maxUnavailable}",
				"-n", resourceNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(defaultValueMaxUnavailable).To(o.Equal("25%"))

		g.By("Ensure upgradeable=false.")
		upgStatusOutput, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(upgStatusOutput, "Upgradeable=False") {
			e2e.Logf("Enable upgradeable=false explicitly...")
			//set overrides in cv to trigger upgradeable=false condition if it is not enabled by default
			type ovrd struct {
				Ki string `json:"kind"`
				Na string `json:"name"`
				Ns string `json:"namespace"`
				Un bool   `json:"unmanaged"`
				Gr string `json:"group"`
			}
			_, err := ocJsonPatch(oc, "", "clusterversion/version", []JSONp{
				{"add", "/spec/overrides", []ovrd{{"Deployment", "network-operator", "openshift-network-operator", true, "apps"}}},
			})
			o.Expect(err).NotTo(o.HaveOccurred())
			defer ocJsonPatch(oc, "", "clusterversion/version", []JSONp{{"remove", "/spec/overrides", nil}})

			e2e.Logf("Wait for Upgradeable=false...")
			err = waitForCondition(30, 300, "False",
				"oc get clusterversion version -ojson|jq -r '.status.conditions[]|select(.type==\"Upgradeable\").status'")
			exutil.AssertWaitPollNoErr(err, "Upgradeable condition is not false in 5m")

			e2e.Logf("Wait for Progressing=false...")
			//to workaround the fake upgrade by cv.overrrides, refer to https://issues.redhat.com/browse/OTA-586
			err = waitForCondition(30, 180, "False",
				"oc get clusterversion version -ojson|jq -r '.status.conditions[]|select(.type==\"Progressing\").status'")
			exutil.AssertWaitPollNoErr(err, "Progressing condition is not false in 3m")
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
		_, err = ocJsonPatch(oc, resourceNamespace, resourceName, []JSONp{
			{"replace", "/spec/strategy/rollingUpdate/maxUnavailable", "50%"},
		})
		o.Expect(err).NotTo(o.HaveOccurred())
		defer ocJsonPatch(oc, resourceNamespace, resourceName, []JSONp{
			{"replace", "/spec/strategy/rollingUpdate/maxUnavailable", "25%"},
		})

		g.By("Check the deployment was reconciled back.")
		err = wait.Poll(30*time.Second, 5*time.Minute, func() (bool, error) {
			valueMaxUnavailable, _ := oc.AsAdmin().WithoutNamespace().Run("get").
				Args(resourceName, "-o=jsonpath={.spec.strategy.rollingUpdate.maxUnavailable}", "-n", resourceNamespace).Output()
			if strings.Compare(valueMaxUnavailable, defaultValueMaxUnavailable) != 0 {
				e2e.Logf("valueMaxUnavailable is %v. Waiting for deployment being reconciled...", valueMaxUnavailable)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "the deployment was not reconciled back in 5min.")
	})
})
