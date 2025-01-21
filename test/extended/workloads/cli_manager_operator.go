package workloads

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

	"strings"
	"time"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-scheduling] Workloads The Descheduler Operator automates pod evictions using different profiles", func() {
	defer g.GinkgoRecover()
	var (
		oc                  = exutil.NewCLI("default-"+getRandomString(), exutil.KubeConfigPath())
		kubeNamespace       = "openshift-cli-manager-operator"
		buildPruningBaseDir string
		cmoOperatorGroupT   string
		cmoSubscriptionT    string
		sub                 cmoSubscription
		og                  cmoOperatorgroup
		plugin              pluginDetails
	)

	g.BeforeEach(func() {
		if os.Getenv("JENKINS_AGENT_NAME") != "" {
			g.Skip("it is jenkins without supporting krew, so skip. currently only support it in prow")
		}
		buildPruningBaseDir = exutil.FixturePath("testdata", "workloads")
		cmoOperatorGroupT = filepath.Join(buildPruningBaseDir, "cmo_operatorgroup.yaml")
		cmoSubscriptionT = filepath.Join(buildPruningBaseDir, "cmo_subscription.yaml")

		og = cmoOperatorgroup{
			name:      "openshift-cli-manager-operator",
			namespace: kubeNamespace,
			template:  cmoOperatorGroupT,
		}

		// Skip the test if no qe-app-registry catalog is present
		sub.skipMissingCatalogsources(oc)

	})

	// author: knarra@redhat.com
	g.It("Author:knarra-ROSA-OSD_CCS-ARO-High-75260-Validate user is able to deploy openshift cli manager successfully [Serial]", func() {
		cliManager := filepath.Join(buildPruningBaseDir, "cliManager.yaml")
		ocMirrorPluginYamlT := filepath.Join(buildPruningBaseDir, "deployOCMirrorPlugin.yaml")
		customCliRegistry := filepath.Join(buildPruningBaseDir, "cli_manager_cs.yaml")
		imageSetYamlFileF := filepath.Join(buildPruningBaseDir, "cli_manager_isc.yaml")

		g.By("Check if imageContentSourcePolicy image-policy-aosqe exists, if not skip the case")
		existingIcspOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ImageContentSourcePolicy", "--ignore-not-found").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(existingIcspOutput, "image-policy-aosqe") {
			// Retreive image registry name
			imageRegistryName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ImageContentSourcePolicy", "image-policy-aosqe", "-o=jsonpath={.spec.repositoryDigestMirrors[0].mirrors[0]}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			imageRegistryName = strings.Split(imageRegistryName, ":")[0]
			e2e.Logf("ImageRegistryName is %s", imageRegistryName)

			// Mirror the images using oc-mirror v2
			dirname := "/tmp/case75260"
			defer os.RemoveAll(dirname)
			err = os.MkdirAll(dirname, 0755)
			o.Expect(err).NotTo(o.HaveOccurred())
			err = locatePodmanCred(oc, dirname)
			o.Expect(err).NotTo(o.HaveOccurred())
			err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", "--to="+dirname, "--confirm").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			exutil.By("Start mirror2mirror")
			defer os.RemoveAll(".oc-mirror.log")
			waitErr := wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
				err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+imageRegistryName+":5000", "--v2", "--workspace", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Execute()
				if err != nil {
					e2e.Logf("The mirror2mirror failed, retrying...")
					return false, nil
				}
				return true, nil
			})
			exutil.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2mirror still failed")

			exutil.By("Create the catalogsource, idms and itms")
			defer operateCSAndMs(oc, dirname+"/working-dir/cluster-resources", "delete")
			operateCSAndMs(oc, dirname+"/working-dir/cluster-resources", "create")
			exutil.By("Check for the catalogsource pod status")
			assertPodOutput(oc, "olm.catalogSource=cs-iib-bd6382120df0", "openshift-marketplace", "Running")

			sub = cmoSubscription{
				name:        "cli-manager",
				namespace:   kubeNamespace,
				channelName: "tech-preview",
				opsrcName:   "cs-iib-bd6382120df0",
				sourceName:  "openshift-marketplace",
				startingCSV: "cli-manager-operator.v0.1.0",
				template:    cmoSubscriptionT,
			}

			// Retrieve the certificate from the ConfigMap
			escapedRegistryName := `jsonpath={.data.` + escapeDots(imageRegistryName) + `\.\.5000}`
			configMapCmd := []string{"get", "configmap", "-n", "openshift-config", "registry-config", "-o", escapedRegistryName}
			outputFile := "registry-ca-bundle.crt"
			cmdOutput, err := runCommand("oc", configMapCmd)
			if err != nil {
				e2e.Failf("Failed to retreive configmap %s", err)
			}

			e2e.Logf("command Output is %s", cmdOutput)

			// Clean up unwanted strings from the output
			cleanedOutput := strings.ReplaceAll(cmdOutput, "[fedora@preserve-fedora-yinzhou openshift-tests-private]$", "")
			cleanedOutput = strings.TrimSpace(cleanedOutput)

			// Write the cleaned output to a file
			err = os.WriteFile(outputFile, []byte(cleanedOutput), 0644)
			if err != nil {
				e2e.Failf("Error writing to file %s: %v", outputFile, err)
			}
			fmt.Printf("Cleaned certificate saved to %s\n", outputFile)

			// Read the file and convert its content to base64
			content, err := os.ReadFile(outputFile)
			if err != nil {
				e2e.Logf("Error reading file %s: %v", outputFile, err)
			}

			// Encode the certificate using base64
			base64Content := base64.StdEncoding.EncodeToString(content)

			// Retreive clusterVersion
			versionOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterversion", "-o", "jsonpath={..desired.version}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())

			plugin = pluginDetails{
				name:     "oc-mirror",
				image:    imageRegistryName + ":5000/ocp/release:" + versionOutput + "-x86_64-oc-mirror",
				caBundle: base64Content,
				template: ocMirrorPluginYamlT,
			}

		} else {
			g.By("Create custom cli app registry")
			defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", customCliRegistry).Execute()
			err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", customCliRegistry).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("wait for the pod to be in running & ready state")
			waitErr := waitForPodWithLabelReady(oc, "openshift-marketplace", "olm.catalogSource=custom-cli-app-registry")
			exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("custom cli manager pod is not ready"))

			checkPodStatus(oc, "olm.catalogSource=custom-cli-app-registry", "openshift-marketplace", "Running")

			sub = cmoSubscription{
				name:        "cli-manager",
				namespace:   kubeNamespace,
				channelName: "tech-preview",
				opsrcName:   "custom-cli-app-registry",
				sourceName:  "openshift-marketplace",
				startingCSV: "cli-manager-operator.v0.1.0",
				template:    cmoSubscriptionT,
			}

			plugin = pluginDetails{
				name:     "oc-mirror",
				image:    "registry.redhat.io/openshift4/oc-mirror-plugin-rhel9:v4.17.0-202410112132.p0.g07714b7.assembly.stream.el9",
				caBundle: "",
				template: ocMirrorPluginYamlT,
			}

		}

		g.By("Create the cli manager operator  namespace")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("ns", kubeNamespace).Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", kubeNamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create the operatorgroup")
		defer og.deleteOperatorGroup(oc)
		og.createOperatorGroup(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create the subscription")
		defer sub.deleteSubscription(oc)
		sub.createSubscription(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Wait for the cli manager operator pod running")
		if ok := waitForAvailableRsRunning(oc, "deploy", "openshift-cli-manager-operator", kubeNamespace, "1"); ok {
			e2e.Logf("CliManagerOperator runnnig now\n")
		} else {
			e2e.Failf("CliManagerOperator not running")
		}

		g.By("Create cli manager instance")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", cliManager).Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", cliManager).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check the cli manager run well")
		waitForDeploymentPodsToBeReady(oc, kubeNamespace, "openshift-cli-manager")

		g.By("Validate that right version of openshift cli manager is running")
		ssCsvOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", "-n", kubeNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("ssCsvOutput is %s", ssCsvOutput)
		o.Expect(strings.Contains(ssCsvOutput, "cli-manager-operator.v0.1.0")).To(o.BeTrue())

		// Add plugin to the cli manager
		g.By("Add oc mirror plugin to cli manager")
		plugin.createPlugin(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Verify that plugin is ready to be served")
		err = wait.Poll(5*time.Second, 180*time.Second, func() (bool, error) {
			pluginMessage, _ := oc.AsAdmin().Run("get").Args("plugin/oc-mirror", "-n", kubeNamespace, "-o=jsonpath={.status.conditions[*].message}").Output()
			e2e.Logf("pluginMessage is %s", pluginMessage)
			if err != nil {
				e2e.Logf("plugin deployment still in progress, error: %s. Trying again", err)
				return false, nil
			}
			if matched, _ := regexp.MatchString("plugin oc-mirror is ready to be served", pluginMessage); matched {
				e2e.Logf("Plugin ready to be served:\n%s", pluginMessage)
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("plugin is not ready to be served"))
	})
})
