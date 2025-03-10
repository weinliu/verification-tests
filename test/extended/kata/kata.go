// Package kata operator tests
package kata

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/exp/slices"
	"golang.org/x/mod/semver"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"github.com/tidwall/gjson"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-kata] Kata", func() {
	defer g.GinkgoRecover()

	var (
		oc                               = exutil.NewCLI("kata", exutil.KubeConfigPath())
		testDataDir                      = exutil.FixturePath("testdata", "kata")
		kcTemplate                       = filepath.Join(testDataDir, "kataconfig.yaml")
		defaultDeployment                = filepath.Join(testDataDir, "workload-deployment-securityContext.yaml")
		defaultPod                       = filepath.Join(testDataDir, "workload-pod-securityContext.yaml")
		subTemplate                      = filepath.Join(testDataDir, "subscription_template.yaml")
		namespaceTemplate                = filepath.Join(testDataDir, "namespace.yaml")
		ogTemplate                       = filepath.Join(testDataDir, "operatorgroup.yaml")
		redirectFile                     = filepath.Join(testDataDir, "ImageTag-DigestMirrorSet.yaml")
		redirectType                     = "ImageTagMirrorSet"
		redirectName                     = "kata-brew-registry"
		clusterVersion                   string
		cloudPlatform                    string
		configmapExists                  bool
		ocpMajorVer                      string
		ocpMinorVer                      string
		minorVer                         int
		opNamespace                      = "openshift-sandboxed-containers-operator"
		ppParam                          PeerpodParam
		ppRuntimeClass                   = "kata-remote"
		ppSecretName                     = "peer-pods-secret"
		ppConfigMapName                  = "peer-pods-cm"
		ppParamsLibvirtConfigMapName     = "libvirt-podvm-image-cm"
		secretTemplateAws                = filepath.Join(testDataDir, "peer-pod-secret-aws.yaml")
		secretTemplateLibvirt            = filepath.Join(testDataDir, "peer-pod-secret-libvirt.yaml")
		ppConfigMapTemplate              string
		ppAWSConfigMapTemplate           = filepath.Join(testDataDir, "peer-pod-aws-cm-template.yaml")
		ppAzureConfigMapTemplate         = filepath.Join(testDataDir, "peer-pod-azure-cm-template.yaml")
		ppLibvirtConfigMapTemplate       = filepath.Join(testDataDir, "peer-pod-libvirt-cm-template.yaml")
		ppParamsLibvirtConfigMapTemplate = filepath.Join(testDataDir, "peer-pods-param-libvirt-cm-template.yaml")
		podAnnotatedTemplate             = filepath.Join(testDataDir, "pod-annotations-template.yaml")
		featureGatesFile                 = filepath.Join(testDataDir, "cc-feature-gates-cm.yaml")
		kbsClientTemplate                = filepath.Join(testDataDir, "kbs-client-template.yaml")
		trusteeCosignedPodFile           = filepath.Join(testDataDir, "trustee-cosigned-pod.yaml")
		testrunConfigmapNs               = "default"
		testrunConfigmapName             = "osc-config"
		scratchRpmName                   string
		trusteeRouteHost                 string
		operatorCsvVersion               string
		caaDaemonName                    = "osc-caa-ds" // after 1.9.0
	)

	subscription := SubscriptionDescription{
		subName:                "sandboxed-containers-operator",
		namespace:              opNamespace,
		catalogSourceName:      "redhat-operators",
		catalogSourceNamespace: "openshift-marketplace",
		channel:                "stable",
		ipApproval:             "Automatic",
		operatorPackage:        "sandboxed-containers-operator",
		template:               subTemplate,
	}

	kataconfig := KataconfigDescription{
		name:             "example-kataconfig",
		template:         kcTemplate,
		logLevel:         "info",
		eligibility:      false,
		runtimeClassName: "kata",
		enablePeerPods:   false,
	}

	testrun := TestRunDescription{
		checked:                  false,
		operatorVer:              "1.7.0",
		catalogSourceName:        subscription.catalogSourceName,
		channel:                  subscription.channel,
		redirectNeeded:           false,
		mustgatherImage:          "registry.redhat.io/openshift-sandboxed-containers/osc-must-gather-rhel9:latest",
		eligibility:              kataconfig.eligibility,
		labelSingleNode:          false,
		eligibleSingleNode:       false,
		runtimeClassName:         kataconfig.runtimeClassName,
		enablePeerPods:           kataconfig.enablePeerPods,
		enableGPU:                false,
		podvmImageUrl:            "https://raw.githubusercontent.com/openshift/sandboxed-containers-operator/devel/config/peerpods/podvm/",
		workloadImage:            "quay.io/openshift/origin-hello-openshift",
		installKataRPM:           false,
		workloadToTest:           "kata",
		trusteeCatalogSourcename: "redhat-operators",
		trusteeUrl:               "",
	}

	trusteeSubscription := SubscriptionDescription{
		subName:                "trustee-operator",
		namespace:              "trustee-operator-system",
		catalogSourceName:      testrun.trusteeCatalogSourcename,
		catalogSourceNamespace: "openshift-marketplace",
		channel:                "stable",
		ipApproval:             "Automatic",
		operatorPackage:        "trustee-operator",
		template:               subTemplate,
	}

	g.BeforeEach(func() {
		// Creating/deleting kataconfig reboots all worker node and extended-platform-tests may timeout.
		// --------- AWS baremetal may take >20m per node ----------------
		// add --timeout 70m
		// tag with [Slow][Serial][Disruptive] when deleting/recreating kataconfig

		var (
			err error
			msg string
		)

		// run once on startup to populate vars, create ns, og, label nodes
		// always log cluster setup on each test
		if testrun.checked {
			e2e.Logf("\n    Cluster: %v.%v on %v\n    configmapExists %v\n    testrun %v\n    subscription %v\n    kataconfig %v\ncsv version %v\n", ocpMajorVer, ocpMinorVer, cloudPlatform, configmapExists, testrun, subscription, kataconfig, operatorCsvVersion)
			if scratchRpmName != "" {
				e2e.Logf("Scratch rpm %v was installed", scratchRpmName)
			}
		} else {
			cloudPlatform = getCloudProvider(oc)
			clusterVersion, ocpMajorVer, ocpMinorVer, minorVer = getClusterVersion(oc)

			configmapExists, err := getTestRunParameters(oc, &subscription, &kataconfig, &testrun, testrunConfigmapNs, testrunConfigmapName)
			if err != nil {
				// if there is an error, fail every test
				e2e.Failf("ERROR: testrun configmap %v errors: %v\n%v %v", testrunConfigmapName, testrun, configmapExists, err)
			}

			// trusteeSubscription isn't passed into getTestRunParameters()
			trusteeSubscription.catalogSourceName = testrun.trusteeCatalogSourcename

			e2e.Logf("\n    Cluster: %v.%v on %v\n    configmapExists %v\n    testrun %v\n    subscription %v\n    kataconfig %v\n\n", ocpMajorVer, ocpMinorVer, cloudPlatform, configmapExists, testrun, subscription, kataconfig)
			testrun.checked = false // only set it true at the end

			if testrun.redirectNeeded {
				if ocpMajorVer == "4" && minorVer <= 12 {
					redirectType = "ImageContentSourcePolicy"
					redirectFile = filepath.Join(testDataDir, "ImageContentSourcePolicy-brew.yaml")
				}
				err = applyImageRedirect(oc, redirectFile, redirectType, redirectName)
				o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("%v", err))
			}

			err = ensureNamespaceIsInstalled(oc, subscription.namespace, namespaceTemplate)
			o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("%v", err))

			err = ensureOperatorGroupIsInstalled(oc, subscription.namespace, ogTemplate)
			o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("%v", err))

			checkAndLabelCustomNodes(oc, testrun)
			// o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("%v", err))

			if kataconfig.eligibility {
				labelEligibleNodes(oc, testrun)
				// o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("%v", err))
			}

			testrun.checked = true
			e2e.Logf("configmapExists %v\n    testrun %v\n\n", configmapExists, testrun)
		}

		e2e.Logf("The current platform is %v. OCP %v.%v cluster version: %v", cloudPlatform, ocpMajorVer, ocpMinorVer, clusterVersion)

		err = ensureOperatorIsSubscribed(oc, subscription, subTemplate)
		o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("%v", err))

		// get CSV version from the operator and set caaDaemonName if needed
		csvName, err := oc.AsAdmin().Run("get").Args("sub", subscription.operatorPackage, "-o=jsonpath={.status.installedCSV}", "-n", subscription.namespace).Output()
		operatorCsvVersion, found := strings.CutPrefix(csvName, subscription.operatorPackage+".")
		o.Expect(found).To(o.BeTrue(), fmt.Sprintf("Could not get csv subscription version from %v.  Got %v", csvName, operatorCsvVersion))

		e2e.Logf("---------- subscription %v ver %v succeeded with channel %v %v", subscription.subName, operatorCsvVersion, subscription.channel, err)

		if semver.Compare(operatorCsvVersion, "v1.9.0") == -1 { // The operator is before 1.9.0.
			caaDaemonName = "peerpodconfig-ctrl-caa-daemon" // use this name on older versions
		}

		err = checkKataconfigIsCreated(oc, subscription, kataconfig.name)
		if err == nil {
			msgSuccess := fmt.Sprintf("(2) subscription %v and kataconfig %v exists, skipping operator deployment", subscription.subName, kataconfig.name)
			e2e.Logf(msgSuccess)

			// kata is installed already
			// have rpms been installed before we skip out of g.BeforeEach()?
			if testrun.installKataRPM {
				e2e.Logf("INFO Trying to install scratch rpm")
				scratchRpmName, err = installKataContainerRPM(oc, &testrun)
				o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("ERROR: could not rpm -Uvh /var/local/%v: %v", scratchRpmName, err))
				// Its installed now
				e2e.Logf("INFO Scratch rpm %v was installed", scratchRpmName)
				testrun.installKataRPM = false
				msg, err = oc.AsAdmin().Run("patch").Args("configmap", testrunConfigmapName, "-n", testrunConfigmapNs, "--type", "merge",
					"--patch", "{\"data\":{\"installKataRPM\":\"false\"}}").Output()
				o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("Error: Patching fails on %v with installKataRPM=false: %v %v", testrunConfigmapName, msg, err))

			}
			return
		}

		// REST OF THIS FUNC will be executed ONLY if kataconfig was not found

		// should be ensurePeerPodSecrets & configmaps
		//create peer pods secret and peer pods cm for OSC prev to 1.7.0
		if kataconfig.enablePeerPods {
			baseVer := strings.Split(testrun.operatorVer, "-")[0]
			if strings.Contains(testrun.operatorVer, "1.6.0") || strings.Contains(testrun.operatorVer, "1.5.3") {
				msg, err = createApplyPeerPodSecrets(oc, cloudPlatform, ppParam, opNamespace, ppSecretName, secretTemplateAws)
				if err != nil {
					err = fmt.Errorf("Cloud Credentials not found") // Generate a custom error
					e2e.Failf("Cloud Credentials not found. Skipping test suite execution msg: %v , err: %v", msg, err)
				}
			} else if semver.Compare(baseVer, "v1.7.0") >= 0 && cloudPlatform == "libvirt" {
				msg, err = createApplyPeerPodsParamLibvirtConfigMap(oc, cloudPlatform, ppParam, opNamespace, ppParamsLibvirtConfigMapName, ppParamsLibvirtConfigMapTemplate)
				if err != nil {
					err = fmt.Errorf("Libvirt configs not found") // Generate a custom error
					e2e.Failf("Cloud Credentials not found. Skipping test suite execution msg: %v , err: %v", msg, err)
				}

				msg, err = createApplyPeerPodSecrets(oc, cloudPlatform, ppParam, opNamespace, ppSecretName, secretTemplateLibvirt)
				if err != nil {
					err = fmt.Errorf("Libvirt configs not found") // Generate a custom error
					e2e.Failf("Cloud Credentials not found. Skipping test suite execution msg: %v , err: %v", msg, err)
				}
			}
			switch cloudPlatform {
			case "azure":
				ppConfigMapTemplate = ppAzureConfigMapTemplate
			case "aws":
				ppConfigMapTemplate = ppAWSConfigMapTemplate
			case "libvirt":
				ppConfigMapTemplate = ppLibvirtConfigMapTemplate
			default:
				e2e.Failf("Cloud provider %v is not supported", cloudPlatform)
			}

			msg, err = createApplyPeerPodConfigMap(oc, cloudPlatform, ppParam, opNamespace, ppConfigMapName, ppConfigMapTemplate)
			if err != nil {
				e2e.Failf("peer-pods-cm NOT applied msg: %v , err: %v", msg, err)
			} else if cloudPlatform == "azure" || cloudPlatform == "libvirt" {
				err = createSSHPeerPodsKeys(oc, ppParam, cloudPlatform)
				o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("%v", err))
			}

			if cloudPlatform == "aws" && (strings.Contains(testrun.operatorVer, "1.6.0") || strings.Contains(testrun.operatorVer, "1.5.3")) {
				e2e.Logf("patch cm for DISABLECVM=true for OSC 1.5.3 and 1.6.0")
				msg, err = oc.AsAdmin().Run("patch").Args("configmap", ppConfigMapName, "-n", opNamespace, "--type", "merge",
					"--patch", "{\"data\":{\"DISABLECVM\":\"true\"}}").Output()
				o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("Could not patch DISABLECVM=true \n error: %v %v", msg, err))
			}
			//new flow for GPU prior to image building job
			if testrun.enableGPU {
				cmName := cloudPlatform + "-podvm-image-cm"
				//fix till CI will be fixed
				//cmUrl := testrun.podvmImageUrl + cmName + ".yaml"
				cmUrl := "https://raw.githubusercontent.com/openshift/sandboxed-containers-operator/devel/config/peerpods/podvm/" + cmName + ".yaml"
				msg, err := oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", cmUrl).Output()
				o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("issue applying podvm image configmap %v: %v, %v", cmUrl, msg, err))
				patchPodvmEnableGPU(oc, opNamespace, cmName, "yes")
				if cloudPlatform == "azure" {
					msg, err = oc.AsAdmin().Run("patch").Args("configmap", cmName, "-n", opNamespace, "--type", "merge",
						"--patch", "{\"data\":{\"IMAGE_GALLERY_NAME\":\"ginkgo"+getRandomString()+"\"}}").Output()
					o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("Could not patch IMAGE_GALLERY_NAME\n error: %v %v", msg, err))
				}
			}
		}

		if testrun.workloadToTest == "coco" {
			err = ensureFeatureGateIsApplied(oc, subscription, featureGatesFile)
			o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("ERROR: could not apply osc-feature-gates cm: %v", err))

			trusteeRouteHost, err = ensureTrusteeIsInstalled(oc, trusteeSubscription, namespaceTemplate, ogTemplate, subTemplate)
			o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("ERROR: %v err: %v", trusteeRouteHost, err))

			msg, err = configureTrustee(oc, trusteeSubscription, testDataDir, testrun.trusteeUrl)
			o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("ERROR: configuring trustee: %v", err))
			testrun.trusteeUrl = msg
			e2e.Logf("INFO in-cluster TRUSTEE_HOST is %v.\nINFO The trusteeUrl to be used is %v", trusteeRouteHost, testrun.trusteeUrl)

			err = ensureTrusteeUrlReturnIsValid(oc, kbsClientTemplate, testrun.trusteeUrl, "cmVzMXZhbDE=", trusteeSubscription.namespace)
			if err != nil {
				testrun.checked = false // fail all tests
			}
			o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("ERROR: failing all tests \nERROR: %v", err))

			patch := fmt.Sprintf("{\"data\":{\"AA_KBC_PARAMS\": \"cc_kbc::%v\"}}", testrun.trusteeUrl)
			msg, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("cm", "peer-pods-cm", "--type", "merge", "-p", patch, "-n", subscription.namespace).Output()
			if err != nil {
				e2e.Logf("WARNING: patching peer-pods-cm: %v %v", msg, err)
			}

			cocoDefaultSize := map[string]string{
				"aws":   "{\"data\":{\"PODVM_INSTANCE_TYPE\":\"m6a.large\"}}",
				"azure": "{\"data\":{\"AZURE_INSTANCE_SIZE\":\"Standard_DC2as_v5\"}}",
			}

			msg, err = oc.AsAdmin().Run("patch").Args("configmap", ppConfigMapName, "-n", opNamespace, "--type", "merge",
				"--patch", cocoDefaultSize[cloudPlatform]).Output()
			o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("Could not patch default instance for coco \n error: %v %v", msg, err))
		}

		// should check kataconfig here & already have checked subscription
		msg, err = ensureKataconfigIsCreated(oc, kataconfig, subscription)

		e2e.Logf("---------- kataconfig %v create succeeded %v %v", kataconfig.name, msg, err)

		// this should be a seperate day0 test for control pods
		if kataconfig.enablePeerPods {
			//TODO implement single function with the list of deployments for kata/peer pod/ coco
			checkPeerPodControl(oc, opNamespace, podRunState, caaDaemonName)
		}

		// kata wasn't installed before so this isn't skipped
		// Do rpms need installion?
		if testrun.installKataRPM {
			e2e.Logf("INFO Trying to install scratch rpm")
			scratchRpmName, err = installKataContainerRPM(oc, &testrun)
			o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("ERROR: could not install scratch %v failed: %v", scratchRpmName, err))
			// Its installed now
			e2e.Logf("INFO Scratch rpm %v was installed", scratchRpmName)
			testrun.installKataRPM = false
			msg, err = oc.AsAdmin().Run("patch").Args("configmap", testrunConfigmapName, "-n", testrunConfigmapNs, "--type", "merge",
				"--patch", "{\"data\":{\"installKataRPM\":\"false\"}}").Output()
			o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("Error: Patching fails on %v with installKataRPM=false: %v %v", testrunConfigmapName, msg, err))
		}

	})

	g.It("Author:abhbaner-High-39499-Operator installation", func() {
		g.By("Checking sandboxed-operator operator installation")
		_, err := subscriptionIsFinished(oc, subscription)
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("SUCCESSS - sandboxed-operator operator installed")
	})

	g.It("Author:abhbaner-High-43522-Common Kataconfig installation", func() {
		g.Skip("test require structure rework")
		g.By("Install Common kataconfig and verify it")
		e2e.Logf("common kataconfig %v is installed", kataconfig.name)

		err := checkKataconfigIsCreated(oc, subscription, kataconfig.name)
		if err != nil {
			e2e.Failf("ERROR: kataconfig install failed: %v", err)
		}

		/* kataconfig status changed so this does not work.
		These check should be moved to a function

		nodeKataList := getAllKataNodes(oc, kataconfig.eligibility, subscription.namespace, featureLabel, customLabel)
		o.Expect(len(nodeKataList) > 0).To(o.BeTrue())
		nodeKataCount := fmt.Sprintf("%d", len(nodeKataList))

		jsonKataStatus, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("kataconfig", kataconfig.name, "-o=jsonpath={.status}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		totalCount := gjson.Get(jsonKataStatus, "totalNodesCount").String()
		o.Expect(totalCount).To(o.Equal(nodeKataCount))

		completeCount := gjson.Get(jsonKataStatus, "installationStatus.completed.completedNodesCount").String()
		o.Expect(totalCount).To(o.Equal(completeCount))

		completededListCount := gjson.Get(jsonKataStatus, "installationStatus.completed.completedNodesList.#").String()
		o.Expect(completededListCount == totalCount)
		e2e.Logf("Completed nodes are %v", gjson.Get(jsonKataStatus, "installationStatus.completed.completedNodesList").String())

			o.Expect(totalCount).To(o.Equal(nodeKataCount))

			completeCount := gjson.Get(jsonKataStatus, "installationStatus.completed.completedNodesCount").String()
			o.Expect(totalCount).To(o.Equal(completeCount))

			completededListCount := gjson.Get(jsonKataStatus, "installationStatus.completed.completedNodesList.#").String()
			o.Expect(completededListCount == totalCount)
			e2e.Logf("Completed nodes are %v", gjson.Get(jsonKataStatus, "installationStatus.completed.completedNodesList").String())

			g.By("SUCCESSS - kataconfig installed and it's structure is verified")
		*/

	})

	g.It("Author:tbuskey-High-66108-Version in operator CSV should match expected version", func() {
		if !testrun.checked {
			g.Skip("osc-config cm or OSCSOPERATORVER are not set so there is no expected version to compare")
		}

		var (
			err        error
			csvName    string
			csvVersion string
		)
		csvName, err = oc.AsAdmin().Run("get").Args("sub", subscription.subName, "-n", subscription.namespace, "-o=jsonpath={.status.installedCSV}").Output()
		if err != nil || csvName == "" {
			e2e.Logf("Error: Not able to get csv from sub %v: %v %v", subscription.subName, csvName, err)
		}
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(csvName).NotTo(o.BeEmpty())

		csvVersion, err = oc.AsAdmin().Run("get").Args("csv", csvName, "-n", subscription.namespace, "-o=jsonpath={.spec.version}").Output()
		if err != nil || csvVersion == "" {
			e2e.Logf("Error: Not able to get version from csv %v: %v %v", csvName, csvVersion, err)
		}
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(csvVersion).NotTo(o.BeEmpty())
		cleanVer := strings.Split(testrun.operatorVer, "-")
		if csvVersion != cleanVer[0] {
			e2e.Logf("Error: expecting %v but CSV has %v", testrun.operatorVer, csvVersion)
		}
		o.Expect(csvVersion).To(o.Equal(cleanVer[0]))

	})

	g.It("Author:tbuskey-Medium-63122-Checking if cluster is ready for peer pods", func() {
		//	can't *VERIFY* all values but we can ensure the cm/secret variables were added by the users
		if !kataconfig.enablePeerPods {
			g.Skip("STEP Peer pods are not enabled with osc-config or OSCSENABLEPEERPODS")
		}

		var (
			err       error
			msg       string
			errors    = 0
			errorList = []string{""}
		)

		// set the CLOUD_PROVIDER value from the peerpods configmap
		cloudProvider, err := oc.AsAdmin().Run("get").Args("cm", ppConfigMapName, "-n", subscription.namespace, "-o=jsonpath={.data.CLOUD_PROVIDER}").Output()
		if err != nil || strings.Contains(cloudProvider, "not found") {
			e2e.Logf("STEP ERROR: peerpod configmap issue %v %v", cloudProvider, err)
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		if len(cloudProvider) == 0 {
			e2e.Logf("STEP ERROR: CLOUD_PROVIDER is not set on peerpod config")
			o.Expect(cloudProvider).ToNot(o.BeZero())
		}

		msg = fmt.Sprintf("checking %v ", ppSecretName)
		g.By(msg)
		msg, err = checkPeerPodSecrets(oc, subscription.namespace, cloudProvider, ppSecretName)
		if err != nil {
			e2e.Logf("%v", msg)
			errors = errors + 1
			errorList = append(errorList, msg)
		}

		msg = fmt.Sprintf("checking %v ", ppConfigMapName)
		g.By(msg)
		msg, err = checkPeerPodConfigMap(oc, subscription.namespace, cloudProvider, ppConfigMapName)
		if err != nil {
			e2e.Logf("%v", msg)
			errors = errors + 1
			errorList = append(errorList, msg)
		}

		g.By("Verify enablePeerPods is set in kataconfig")
		msg, err = oc.AsAdmin().Run("get").Args("kataconfig", kataconfig.name, "-n", subscription.namespace, "-o=jsonpath={.spec.enablePeerPods}").Output()
		if err != nil || msg != "true" {
			e2e.Logf("STEP ERROR querying kataconfig %v and enablePeerPods setting", kataconfig.name)
			errors = errors + 1
			errorList = append(errorList, msg)
		}

		msg = fmt.Sprintf("check runtimeclass for %v", ppRuntimeClass)
		g.By(msg)
		msg, err = oc.AsAdmin().Run("get").Args("runtimeclass", "-n", subscription.namespace, "--no-headers").Output()
		if err != nil || !strings.Contains(msg, ppRuntimeClass) {
			e2e.Logf("STEP ERROR runtimeclass %v not found", ppRuntimeClass, msg, err)
			errors = errors + 1
			errorList = append(errorList, msg)
		}

		g.By("Check errors")
		if errors != 0 {
			e2e.Logf("STEP ERROR: %v error areas:\n    %v", errors, errorList)
		}
		o.Expect(errors).To(o.BeZero())

		g.By("SUCCESS - cluster has cm and secrets for peerpods")
	})

	g.It("Author:abhbaner-High-41566-High-41574-deploy & delete a pod with kata runtime", func() {

		oc.SetupProject()

		var (
			msg            string
			err            error
			defaultPodName = "-example-41566"
			podNs          = oc.Namespace()
		)

		g.By("Deploying pod with kata runtime and verify it")
		newPodName := createKataPod(oc, podNs, defaultPod, defaultPodName, kataconfig.runtimeClassName, testrun.workloadImage)
		defer deleteKataResource(oc, "pod", podNs, newPodName)
		msg, err = checkResourceJsonpath(oc, "pod", newPodName, podNs, "-o=jsonpath={.status.phase}", podRunState, podSnooze*time.Second, 10*time.Second)
		if err != nil {
			e2e.Logf("ERROR: pod %v could not be installed: %v %v", newPodName, msg, err)
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		g.By("SUCCESS - Pod with kata runtime installed")
	})

	// author: tbuskey@redhat.com
	g.It("Author:tbuskey-High-43238-Operator prohibits creation of multiple kataconfigs", func() {
		var (
			kataConfigName2 = kataconfig.name + "2"
			configFile      string
			msg             string
			err             error
			expectError     = "KataConfig instance already exists, refusing to create a duplicate"
		)
		g.By("Create 2nd kataconfig file")
		configFile, err = oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", kcTemplate, "-p", "NAME="+kataConfigName2, "-n", subscription.namespace).OutputToFile(getRandomString() + "kataconfig-common.json")
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("the file of resource is %s", configFile)

		g.By("Apply 2nd kataconfig")
		//Error from server (A KataConfig instance already exists, refusing to create a duplicate): error when creating "kataconfig2.yaml":
		// admission webhook "vkataconfig.kb.io" denied the request: A KataConfig instance already exists, refusing to create a duplicate

		msg, err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", configFile).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(msg).To(o.ContainSubstring(expectError))
		g.By("Success - cannot apply 2nd kataconfig")

	})

	g.It("Author:abhbaner-High-41263-Namespace check", func() {
		g.By("Checking if ns 'openshift-sandboxed-containers-operator' exists")
		msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("namespaces", subscription.namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(msg).To(o.ContainSubstring(subscription.namespace))
		g.By("SUCCESS - Namespace check complete")

	})

	g.It("Author:abhbaner-High-43620-validate podmetrics for pod running kata", func() {
		if kataconfig.enablePeerPods {
			g.Skip("skipping.  metrics are not available on pods with Peer Pods enabled")
		}

		oc.SetupProject()

		var (
			msg            string
			err            error
			defaultPodName = "example"
			podNs          = oc.Namespace()
		)

		g.By("Deploying pod with kata runtime and verify it")
		newPodName := createKataPod(oc, podNs, defaultPod, defaultPodName, kataconfig.runtimeClassName, testrun.workloadImage)
		defer deleteKataResource(oc, "pod", podNs, newPodName)
		msg, err = checkResourceJsonpath(oc, "pod", newPodName, podNs, "-o=jsonpath={.status.phase}", podRunState, podSnooze*time.Second, 10*time.Second)
		if err != nil {
			e2e.Logf("ERROR: %v %v", msg, err)
		}

		errCheck := wait.Poll(10*time.Second, 120*time.Second, func() (bool, error) {
			podMetrics, err := oc.AsAdmin().Run("describe").Args("podmetrics", newPodName, "-n", podNs).Output()
			if err != nil {
				return false, nil
			}
			e2e.Logf("Pod metrics output below  \n %s ", podMetrics)
			o.Expect(podMetrics).To(o.ContainSubstring("Cpu"))
			o.Expect(podMetrics).To(o.ContainSubstring("Memory"))
			o.Expect(podMetrics).To(o.ContainSubstring("Events"))
			return true, nil
		})
		exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("can not describe podmetrics %v in ns %v", newPodName, podNs))

		g.By("SUCCESS - Podmetrics for pod with kata runtime validated")
		g.By("TEARDOWN - deleting the kata pod")
	})

	g.It("Author:abhbaner-High-43617-High-43616-CLI checks pod logs & fetching pods in podNs", func() {

		if testrun.workloadToTest == "coco" {
			g.Skip("Test not supported with coco")
		}

		oc.SetupProject()
		var (
			msg            string
			err            error
			defaultPodName = "example"
			podNs          = oc.Namespace()
		)

		g.By("Deploying pod with kata runtime and verify it")
		newPodName := createKataPod(oc, podNs, defaultPod, defaultPodName, kataconfig.runtimeClassName, testrun.workloadImage)
		defer deleteKataResource(oc, "pod", podNs, newPodName)

		msg, err = checkResourceJsonpath(oc, "pod", newPodName, podNs, "-o=jsonpath={.status.phase}", podRunState, podSnooze*time.Second, 10*time.Second)
		e2e.Logf("Pod (with Kata runtime) with name -  %v , is installed: %v %v", newPodName, msg, err)
		errCheck := wait.Poll(10*time.Second, 200*time.Second, func() (bool, error) {
			podlogs, err := oc.AsAdmin().Run("logs").Args("pod/"+newPodName, "-n", podNs).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(podlogs).NotTo(o.BeEmpty())
			if strings.Contains(podlogs, "serving on") {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("Pod %v logs are not getting generated", newPodName))
		g.By("SUCCESS - Logs for pods with kata validated")
		g.By("TEARDOWN - deleting the kata pod")
	})

	g.It("Author:abhbaner-High-43514-kata pod displaying correct overhead", func() {
		const (
			defaultPodName                = "example"
			ppWebhookDeploymentName       = "peer-pods-webhook"
			ppVMExtendedResourceEnv       = "POD_VM_EXTENDED_RESOURCE"
			expPPVmExtendedResourceLimit  = "1"
			expPPVExtendedResourceRequest = "1"
		)

		oc.SetupProject()
		podNs := oc.Namespace()

		g.By("Deploying pod with kata runtime")
		newPodName := createKataPod(oc, podNs, defaultPod, defaultPodName, kataconfig.runtimeClassName, testrun.workloadImage)
		defer deleteKataResource(oc, "pod", podNs, newPodName)

		g.By("Verifying pod state")
		msg, err := checkResourceJsonpath(oc, "pod", newPodName, podNs, "-o=jsonpath={.status.phase}", podRunState, podSnooze*time.Second, 10*time.Second)
		if err != nil {
			e2e.Logf("ERROR: unable to get podState %v of %v in namespace %v %v %v", podRunState, newPodName, podNs, msg, err)
		}

		kataPodObj, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", newPodName, "-n", podNs, "-o=json").Output()
		if err != nil {
			e2e.Logf("ERROR: unable to get pod: %v in namepsace: %v - error: %v", newPodName, podNs, err)
		}
		o.Expect(err).NotTo(o.HaveOccurred())

		// peerpod webhook erases the pod overhead
		g.By("Checking peerpod resources")
		if kataconfig.enablePeerPods {

			g.By("Fetching peer POD_VM_EXTENDED_RESOURCE defaults from peer-pods-webhook pod")
			ppVMResourceDefaults, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", ppWebhookDeploymentName, "-n", subscription.namespace, "-o=jsonpath={.spec.template.spec.containers[?(@.name=='"+ppWebhookDeploymentName+"')].env[?(@.name=='"+ppVMExtendedResourceEnv+"')].value}").Output()
			if err != nil {
				e2e.Logf("ERROR: unable to get peerpod webhook deployment: %v in namepsace: %v - error: %v", ppWebhookDeploymentName, subscription.namespace, err)
			}
			o.Expect(err).ToNot(o.HaveOccurred())

			gjson.Get(kataPodObj, "spec.containers").ForEach(func(key, container gjson.Result) bool {

				e2e.Logf("checking container: %s on pod: %s in namespace: %s ", gjson.Get(container.String(), "name").String(), newPodName, podNs)

				ppVMResourceDefaults := strings.Replace(ppVMResourceDefaults, ".", "\\.", -1)

				actualResourceLimit := gjson.Get(container.String(), "resources.limits."+ppVMResourceDefaults).String()
				if strings.Compare(actualResourceLimit, expPPVmExtendedResourceLimit) != 0 {
					e2e.Logf("ERROR: peerpod: %v in namepsace: %v has incorrect pod VM extended resource limit: %v", newPodName, podNs, actualResourceLimit)
				}
				o.Expect(actualResourceLimit).To(o.Equal(expPPVmExtendedResourceLimit))

				actualResourceRequest := gjson.Get(container.String(), "resources.requests."+ppVMResourceDefaults).String()
				if strings.Compare(actualResourceRequest, expPPVExtendedResourceRequest) != 0 {
					e2e.Logf("ERROR: peerpod: %v in namepsace: %v has incorrect pod VM extended resource request: %v", newPodName, podNs, actualResourceRequest)
				}
				o.Expect(actualResourceRequest).To(o.Equal(expPPVExtendedResourceRequest))

				return true
			})
		}

		g.By("Checking Kata pod overhead")
		// for non-peer kata pods, overhead is expected to be same as set in runtimeclass
		runtimeClassObj, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("runtimeclass", kataconfig.runtimeClassName, "-o=json").Output()
		if err != nil {
			e2e.Logf("ERROR: unable to get runtimeclass: %v - error: %v", kataconfig.runtimeClassName, err)
		}
		o.Expect(err).NotTo(o.HaveOccurred())

		actualCpu := gjson.Get(kataPodObj, "spec.overhead.cpu").String()
		expectedCpu := gjson.Get(runtimeClassObj, "overhead.podFixed.cpu").String()
		if strings.Compare(expectedCpu, actualCpu) != 0 {
			e2e.Logf("ERROR: kata pod: %v in namepsace: %v has incorrect cpu overhead: %v", newPodName, podNs, actualCpu)
		}
		o.Expect(expectedCpu).To(o.Equal(actualCpu))

		actualMem := gjson.Get(kataPodObj, "spec.overhead.memory").String()
		expectedMem := gjson.Get(runtimeClassObj, "overhead.podFixed.memory").String()
		if strings.Compare(expectedMem, actualMem) != 0 {
			e2e.Logf("ERROR: kata pod: %v in namepsace: %v has incorrect memory overhead: %v", newPodName, podNs, actualMem)
		}
		o.Expect(expectedMem).To(o.Equal(actualMem))

		g.By("SUCCESS - kata pod overhead verified")
		g.By("TEARDOWN - deleting the kata pod")
	})

	// author: tbuskey@redhat.com
	g.It("Author:tbuskey-High-43619-oc admin top pod metrics works for pods that use kata runtime", func() {

		if kataconfig.enablePeerPods {
			g.Skip("skipping.  metrics are not in oc admin top pod with Peer Pods enabled")
		}

		oc.SetupProject()
		var (
			podNs       = oc.Namespace()
			podName     string
			err         error
			msg         string
			waitErr     error
			metricCount = 0
		)

		g.By("Deploy a pod with kata runtime")
		podName = createKataPod(oc, podNs, defaultPod, "admtop", kataconfig.runtimeClassName, testrun.workloadImage)
		defer deleteKataResource(oc, "pod", podNs, podName)
		msg, err = checkResourceJsonpath(oc, "pod", podName, podNs, "-o=jsonpath={.status.phase}", podRunState, podSnooze*time.Second, 10*time.Second)

		g.By("Get oc top adm metrics for the pod")
		snooze = 360
		waitErr = wait.Poll(10*time.Second, snooze*time.Second, func() (bool, error) {
			msg, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("top", "pod", "-n", podNs, podName, "--no-headers").Output()
			if err == nil { // Will get error with msg: error: metrics not available yet
				metricCount = len(strings.Fields(msg))
			}
			if metricCount == 3 {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "metrics never appeared")
		if metricCount == 3 {
			e2e.Logf("metrics for pod %v", msg)
		}
		o.Expect(metricCount).To(o.Equal(3))

		g.By("Success")

	})

	g.It("Author:abhbaner-High-43516-operator is available in CatalogSource", func() {

		g.By("Checking catalog source for the operator")
		opMarketplace, err := oc.AsAdmin().Run("get").Args("packagemanifests", "-n", "openshift-marketplace").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(opMarketplace).NotTo(o.BeEmpty())
		o.Expect(opMarketplace).To(o.ContainSubstring("sandboxed-containers-operator"))
		o.Expect(opMarketplace).To(o.ContainSubstring("Red Hat Operators"))
		g.By("SUCCESS -  'sandboxed-containers-operator' is present in packagemanifests")

	})

	g.It("Author:tbuskey-High-43523-Monitor deletion[Disruptive][Serial][Slow]", func() {
		g.By("Delete kataconfig and verify it")
		msg, err := deleteKataConfig(oc, kataconfig.name)
		e2e.Logf("kataconfig %v was deleted\n--------- %v %v", kataconfig.name, msg, err)

		g.By("SUCCESS")
	})

	g.It("Author:tbuskey-High-41813-Build Acceptance test with deletion[Disruptive][Serial][Slow]", func() {
		g.Skip("kataconfig deletion steps are skipped")
		//This test will install operator,kataconfig,pod with kata - delete pod, delete kataconfig

		oc.SetupProject()

		var (
			msg            string
			err            error
			defaultPodName = "example"
			podNs          = oc.Namespace()
		)

		g.By("Deploying pod and verify it")
		newPodName := createKataPod(oc, podNs, defaultPod, defaultPodName, kataconfig.runtimeClassName, testrun.workloadImage)
		msg, err = checkResourceJsonpath(oc, "pod", newPodName, podNs, "-o=jsonpath={.status.phase}", podRunState, podSnooze*time.Second, 10*time.Second)
		e2e.Logf("Pod (with Kata runtime) with name -  %v , is installed: %v %v", newPodName, msg, err)

		g.By("Deleting pod")
		deleteKataResource(oc, "pod", podNs, newPodName)

		g.By("Deleting kataconfig")

		msg, err = deleteKataConfig(oc, kataconfig.name)
		e2e.Logf("common kataconfig %v was deleted %v %v", kataconfig.name, msg, err)
		g.By("SUCCESSS - build acceptance passed")

	})

	// author: tbuskey@redhat.com
	g.It("Author:tbuskey-High-46235-Kata Metrics Verify that Namespace is labeled to enable monitoring", func() {
		var (
			err        error
			msg        string
			s          string
			label      = ""
			hasMetrics = false
		)

		g.By("Get labels of openshift-sandboxed-containers-operator namespace to check for monitoring")
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("ns", "openshift-sandboxed-containers-operator", "-o=jsonpath={.metadata.labels}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		for _, s = range strings.SplitAfter(msg, ",") {
			if strings.Contains(s, "openshift.io/cluster-monitoring") {
				label = s
				if strings.Contains(strings.SplitAfter(s, ":")[1], "true") {
					hasMetrics = true
				}
			}
		}
		e2e.Logf("Label is %v", label)
		o.Expect(hasMetrics).To(o.BeTrue())

		g.By("Success")
	})

	g.It("Author:abhbaner-High-43524-Existing deployments (with runc) should restart normally after kata runtime install", func() {

		oc.SetupProject()
		var (
			podNs      = oc.Namespace()
			deployName = "dep-43524-" + getRandomString()
			msg        string
			podName    string
			newPodName string
		)

		g.By("Create deployment config from template")
		configFile, err := oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", defaultDeployment,
			"-p", "NAME="+deployName, "-p", "RUNTIMECLASSNAME="+kataconfig.runtimeClassName,
			"-p", "IMAGE="+testrun.workloadImage).OutputToFile(getRandomString() + "dep-common.json")
		if err != nil {
			e2e.Logf("Could not create configFile %v %v", configFile, err)
		}
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Applying deployment file " + configFile)
		msg, err = oc.AsAdmin().Run("apply").Args("-f", configFile, "-n", podNs).Output()
		if err != nil {
			e2e.Logf("Could not apply configFile %v %v", msg, err)
		}
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Wait for deployment to be ready")
		defer oc.AsAdmin().Run("delete").Args("deploy", "-n", podNs, deployName, "--ignore-not-found").Execute()
		msg, err = waitForDeployment(oc, podNs, deployName)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(msg).NotTo(o.BeEmpty())

		// If the deployment is ready, pod will be.  Might not need this
		g.By("Wait for pods to be ready")
		errCheck := wait.Poll(10*time.Second, 600*time.Second, func() (bool, error) {
			msg, err = oc.AsAdmin().Run("get").Args("pods", "-n", podNs, "--no-headers").Output()
			if !strings.Contains(msg, "No resources found") {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("Timed out waiting for pods %v %v", msg, err))

		g.By("Get pod name")
		msg, err = oc.AsAdmin().Run("get").Args("pods", "-n", podNs, "--no-headers").Output()
		podName = strings.Split(msg, " ")[0]
		e2e.Logf("podname %v %v", msg, err)

		msg = fmt.Sprintf("Deleting pod %v from deployment", podName)
		g.By(msg)
		msg, err = deleteResource(oc, "pod", podName, podNs, podSnooze*time.Second, 10*time.Second)
		e2e.Logf("%v pod deleted: %v %v", podName, msg, err)

		g.By("Wait for deployment to re-replicate")
		msg, err = waitForDeployment(oc, podNs, deployName)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(msg).NotTo(o.BeEmpty())

		g.By("Get new pod name")
		msg, err = oc.AsAdmin().Run("get").Args("pods", "-n", podNs, "--no-headers").Output()
		newPodName = strings.Split(msg, " ")[0]
		e2e.Logf("new podname %v %v", msg, err)
		if newPodName == podName {
			e2e.Failf("A new pod did not get created")
		}

		g.By("SUCCESSS - kataconfig installed and post that pod with runc successfully restarted ")
		msg, err = deleteResource(oc, "deploy", deployName, podNs, podSnooze*time.Second, 10*time.Second)

	})

	// author: tbuskey@redhat.com
	g.It("Longduration-NonPreRelease-Author:tbuskey-High-42167-Must-gather collects sandboxed operator logs[Serial]", func() {
		g.Skip("mustgather test must be done manually")

		type counts struct {
			audits           int
			crio             int
			qemuLogs         int
			qemuVersion      int
			describeCsv      int
			describeKc       int
			describeServices int
			describeSub      int
			describeVwebhook int
		}

		oc.SetupProject()

		var (
			deployConfigFile = ""
			deployName       = "mg-42167-" + getRandomString()
			deploymentFile   = getRandomString() + "dep-common.json"
			podNs            = oc.Namespace()
			err              error
			fails            = 0
			kcLogLevel       = "{\"spec\":{\"logLevel\":\"debug\"}}"
			logFile          string
			mustgatherFiles  = []string{""}
			mustgatherName   = "mustgather" + getRandomString()
			mustgatherDir    = "/tmp/" + mustgatherName
			mustgatherLog    = mustgatherName + ".log"
			msg              string
			nodeControlCount int
			nodeWorkerCount  int
			singleNode       = false
			isWorker         = false
		)

		mustgatherChecks := counts{
			audits:           0,
			crio:             0,
			qemuLogs:         0,
			qemuVersion:      0,
			describeCsv:      0,
			describeKc:       0,
			describeServices: 0,
			describeSub:      0,
			describeVwebhook: 0,
		}

		nodeControlList, err := exutil.GetClusterNodesBy(oc, "master")
		msgIfErr := fmt.Sprintf("getClusterNodesBy master %v %v", nodeControlList, err)
		o.Expect(err).NotTo(o.HaveOccurred(), msgIfErr)
		nodeControlCount = len(nodeControlList)

		nodeWorkerList, err := exutil.GetClusterNodesBy(oc, "worker")
		msgIfErr = fmt.Sprintf("getClusterNodesBy worker %v %v", nodeWorkerList, err)
		o.Expect(err).NotTo(o.HaveOccurred(), msgIfErr)
		nodeWorkerCount = len(nodeWorkerList)

		mustgatherExpected := counts{
			audits:           1,
			crio:             nodeWorkerCount + nodeControlCount,
			qemuLogs:         nodeWorkerCount, // Need to change from deployment
			qemuVersion:      nodeWorkerCount,
			describeCsv:      1,
			describeKc:       1,
			describeServices: 1,
			describeSub:      1,
			describeVwebhook: 1,
		}

		// for SNO
		if nodeWorkerCount == 1 && !strings.Contains(nodeWorkerList[0], "worker") {
			singleNode = true
			mustgatherExpected.crio = nodeWorkerCount
		}

		// patch kataconfig for debug
		_, _ = oc.AsAdmin().Run("patch").Args("kataconfig", kataconfig.name, "-n", subscription.namespace, "--type", "merge", "--patch", kcLogLevel).Output()
		msg, err = waitForNodesInDebug(oc, subscription.namespace)
		e2e.Logf("%v", msg)

		/* Create a deployment file from template with N replicas where N=worker nodes
		 It does not ensure that there is a replica on each worker node.
		 Loop because on 4.12 SNO, nodes might not respond at 1st
			error: unable to process template
			service unavailable
			exit status 1 */
		errCheck := wait.Poll(10*time.Second, 360*time.Second, func() (bool, error) {
			deployConfigFile, err = oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", defaultDeployment,
				"-p", "NAME="+deployName, "-p", "NAMESPACE="+podNs, "-p", "REPLICAS="+fmt.Sprintf("%v", nodeWorkerCount),
				"-p", "RUNTIMECLASSNAME="+kataconfig.runtimeClassName, "-p", "IMAGE="+testrun.workloadImage).OutputToFile(deploymentFile)
			if strings.Contains(deployConfigFile, deploymentFile) {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("Error: Unable to create deployment file from template: %v %v", deployConfigFile, err))
		o.Expect(deployConfigFile).NotTo(o.BeEmpty(), "empty deploy file error %v", err)

		_, err = oc.AsAdmin().Run("apply").Args("-f", deployConfigFile, "-n", podNs).Output()
		defer oc.AsAdmin().Run("delete").Args("deploy", "-n", podNs, deployName, "--ignore-not-found").Execute()
		msg, err = waitForDeployment(oc, podNs, deployName)
		msgIfErr = fmt.Sprintf("ERROR: waitForDeployment %v: %v %v", deployName, msg, err)
		o.Expect(err).NotTo(o.HaveOccurred(), msgIfErr)
		o.Expect(msg).NotTo(o.BeEmpty(), msgIfErr)

		defer os.RemoveAll(mustgatherDir)
		logFile, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("-n", subscription.namespace, "must-gather", "--image="+testrun.mustgatherImage, "--dest-dir="+mustgatherDir).OutputToFile(mustgatherLog)
		o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("ERROR: mustgather %v has an error %v %v", mustgatherLog, logFile, err))

		files, err := os.ReadDir(mustgatherDir)
		msgIfErr = fmt.Sprintf("ERROR %v contents %v\n%v", mustgatherDir, err, files)
		o.Expect(err).NotTo(o.HaveOccurred(), msgIfErr)
		o.Expect(files).NotTo(o.BeEmpty(), msgIfErr)

		err = filepath.Walk(mustgatherDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				e2e.Logf("Error on %v: %v", path, err)
				return err
			}

			isWorker = false
			for _, worker := range nodeWorkerList {
				if strings.Contains(path, worker) {
					isWorker = true
					break
				}
			}

			if info.IsDir() { // qemu will create a directory but might not create files
				if isWorker == true && strings.Contains(path, "/run/vc/crio/fifo") && !strings.Contains(path, "/run/vc/crio/fifo/io") {
					mustgatherChecks.qemuLogs++
				}
			}

			if !info.IsDir() {
				mustgatherFiles = append(mustgatherFiles, path)
				if strings.Contains(path, "audit.log") {
					mustgatherChecks.audits++
				}

				if strings.Contains(path, "/nodes/") {
					if strings.Contains(path, "_logs_crio") {
						mustgatherChecks.crio++
					}
					// in SNO, no worker, just master
					if (isWorker == true || (singleNode == true && isWorker != true)) && strings.Contains(path, "/version") {
						mustgatherChecks.qemuVersion++
					}
				}

				if strings.Contains(path, "/sandboxed-containers") {
					if strings.Contains(path, "/clusterserviceversion_description") {
						mustgatherChecks.describeCsv++
					}
					if strings.Contains(path, "/kataconfig_description") {
						mustgatherChecks.describeKc++
					}
					if strings.Contains(path, "/services_description") {
						mustgatherChecks.describeServices++
					}
					if strings.Contains(path, "/subscription_description") {
						mustgatherChecks.describeSub++
					}
					if strings.Contains(path, "/validatingwebhookconfigurations_description") {
						mustgatherChecks.describeVwebhook++
					}
				}
			}
			return nil
		})
		e2e.Logf("%v files in must-gather dir %v", len(mustgatherFiles), mustgatherDir)
		e2e.Logf("expected: %v", mustgatherExpected)
		e2e.Logf("actual  : %v", mustgatherChecks)

		e2e.Logf("mustgatherChecks.audits : %v", mustgatherChecks.audits)
		if mustgatherChecks.audits < mustgatherExpected.audits {
			e2e.Logf("Audit logs (%v) not found on any worker nodes (%v)", mustgatherChecks.audits, mustgatherExpected.audits)
			fails++
		}
		e2e.Logf("mustgatherChecks.crio : %v", mustgatherChecks.crio)
		if mustgatherChecks.crio != (mustgatherExpected.crio) {
			e2e.Logf("crio logs (%v) did exist on all nodes (%v)", mustgatherChecks.crio, (mustgatherExpected.crio))
			fails++
		}

		// A deployment will place VMs based on loads
		// to ensure a VM is on each node another method is needed
		e2e.Logf("mustgatherChecks.qemuLogs : %v", mustgatherChecks.qemuLogs)
		if mustgatherChecks.qemuLogs != mustgatherExpected.qemuLogs {
			e2e.Logf("qemu log directory (%v) does not exist on all worker nodes (%v), is ok", mustgatherChecks.qemuLogs, mustgatherExpected.qemuLogs)
			// VMs should be 1 on each worker node but k8s might put 2 on a node & 0 on another per node load
			if !singleNode && mustgatherChecks.qemuLogs < 1 { // because deployment is used
				fails++
			}
		}

		e2e.Logf("mustgatherChecks.qemuVersion : %v", mustgatherChecks.qemuVersion)
		if mustgatherChecks.qemuVersion != mustgatherExpected.qemuVersion {
			e2e.Logf("rpm version log (%v) did not exist on worker nodes (%v)", mustgatherChecks.qemuVersion, mustgatherExpected.qemuVersion)
			fails++
		}

		e2e.Logf("mustgatherChecks.describeCsv : %v", mustgatherChecks.describeCsv)
		if mustgatherChecks.describeCsv != mustgatherExpected.describeCsv {
			e2e.Logf("describeCsv (%v) did not exist", mustgatherChecks.describeCsv)
			fails++
		}
		e2e.Logf("mustgatherChecks.describeKc : %v", mustgatherChecks.describeKc)
		if mustgatherChecks.describeKc != mustgatherExpected.describeKc {
			e2e.Logf("describeKc (%v) did not exist", mustgatherChecks.describeKc)
			fails++
		}
		e2e.Logf("mustgatherChecks.describeServices : %v", mustgatherChecks.describeServices)
		if mustgatherChecks.describeServices != mustgatherExpected.describeServices {
			e2e.Logf("describeServices (%v) did not exist", mustgatherChecks.describeServices)
			fails++
		}
		e2e.Logf("mustgatherChecks.describeSub : %v", mustgatherChecks.describeSub)
		if mustgatherChecks.describeSub != mustgatherExpected.describeSub {
			e2e.Logf("describeSub (%v) did not exist", mustgatherChecks.describeSub)
			fails++
		}
		e2e.Logf("mustgatherChecks.describeVwebhook : %v", mustgatherChecks.describeVwebhook)
		if mustgatherChecks.describeVwebhook != mustgatherExpected.describeVwebhook {
			e2e.Logf("describeVwebhook (%v) did not exist", mustgatherChecks.describeVwebhook)
			fails++
		}
		o.Expect(fails).To(o.Equal(0), fmt.Sprintf("%v logs did not match expectd results\n%v", fails, mustgatherExpected))

		e2e.Logf("STEP: SUCCESS")
	})

	// author: tbuskey@redhat.com
	g.It("Longduration-Author:tbuskey-High-53583-upgrade osc operator by changing subscription [Disruptive][Serial]", func() {
		g.Skip("Upgrade tests should be manually done")

		var (
			subscriptionUpgrade            = subscription
			kataconfigUpgrade              = kataconfig
			testrunUpgradeWithSubscription = testrun
			testrunConfigmapName           = "osc-config-upgrade-subscription"
			msg                            string
			msgIfErr                       string
		)

		testrunUpgradeWithSubscription.checked = false

		upgradeConfigMapExists, err := getTestRunParameters(oc, &subscriptionUpgrade, &kataconfigUpgrade, &testrunUpgradeWithSubscription, testrunConfigmapNs, testrunConfigmapName)
		if err != nil {
			e2e.Failf("ERROR: testrunUpgradeWithSubscription configmap %v errors: %v\n%v", testrunUpgradeWithSubscription, err)
		}

		if !upgradeConfigMapExists {
			msg = fmt.Sprintf("SKIP: %v configmap does not exist. Cannot upgrade by changing subscription", testrunConfigmapName)
			g.Skip(msg)
		}

		if testrunUpgradeWithSubscription.redirectNeeded {
			if ocpMajorVer == "4" && minorVer <= 12 {
				redirectType = "ImageContentSourcePolicy"
				redirectFile = filepath.Join(testDataDir, "ImageContentSourcePolicy-brew.yaml")
			}
			err = applyImageRedirect(oc, redirectFile, redirectType, redirectName)
			o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("%v", err))
		}

		if testrunUpgradeWithSubscription.catalogSourceName != subscription.catalogSourceName {
			waitForCatalogReadyOrFail(oc, testrunUpgradeWithSubscription.catalogSourceName)

			g.By("Check catalog for " + subscriptionUpgrade.subName)
			label := fmt.Sprintf("catalog=%v", testrunUpgradeWithSubscription.catalogSourceName)
			errCheck := wait.Poll(10*time.Second, 240*time.Second, func() (bool, error) {
				msg, err = oc.AsAdmin().Run("get").Args("packagemanifest", "-l", label, "-n", subscriptionUpgrade.catalogSourceNamespace).Output()
				if strings.Contains(msg, subscriptionUpgrade.subName) {
					return true, nil
				}
				return false, nil
			})
			exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("%v is not in the %v catalog. Cannot change subscription: %v %v", subscriptionUpgrade.subName, testrunUpgradeWithSubscription.catalogSourceName, msg, err))

			msg, err = changeSubscriptionCatalog(oc, subscriptionUpgrade, testrunUpgradeWithSubscription)
			msgIfErr = fmt.Sprintf("ERROR: patching the subscription catalog %v failed %v %v", subscriptionUpgrade, msg, err)
			o.Expect(err).NotTo(o.HaveOccurred(), msgIfErr)
			o.Expect(msg).NotTo(o.BeEmpty(), msgIfErr)

			msg, err = subscriptionIsFinished(oc, subscriptionUpgrade)
			msgIfErr = fmt.Sprintf("ERROR: subscription wait for catalog patch %v failed %v %v", subscriptionUpgrade, msg, err)
			o.Expect(err).NotTo(o.HaveOccurred(), msgIfErr)
			o.Expect(msg).NotTo(o.BeEmpty(), msgIfErr)
		}

		if testrunUpgradeWithSubscription.channel != subscription.channel {
			g.By("Changing the subscription channel")
			msg, err = changeSubscriptionChannel(oc, subscriptionUpgrade, testrunUpgradeWithSubscription)
			msgIfErr = fmt.Sprintf("ERROR: patching the subscription channel %v: %v %v", subscriptionUpgrade, msg, err)
			o.Expect(err).NotTo(o.HaveOccurred(), msgIfErr)
			o.Expect(msg).NotTo(o.BeEmpty(), msgIfErr)

			// all pods restart & subscription gets recreated
			msg, err = subscriptionIsFinished(oc, subscriptionUpgrade)
			msgIfErr = fmt.Sprintf("ERROR: subscription wait after channel changed %v: %v %v", subscriptionUpgrade, msg, err)
			o.Expect(err).NotTo(o.HaveOccurred(), msgIfErr)
			o.Expect(msg).NotTo(o.BeEmpty(), msgIfErr)
		}
	})

	g.It("Author:vvoronko-High-60231-Scale-up deployment [Serial]", func() {

		oc.SetupProject()
		var (
			podNs        = oc.Namespace()
			deployName   = "dep-60231-" + getRandomString()
			initReplicas = 3
			maxReplicas  = 6
			numOfVMs     int
			msg          string
		)
		kataNodes := exutil.GetNodeListByLabel(oc, kataocLabel)
		o.Expect(len(kataNodes) > 0).To(o.BeTrue(), fmt.Sprintf("kata nodes list is empty %v", kataNodes))

		if !kataconfig.enablePeerPods {
			g.By("Verify no instaces exists before the test")
			numOfVMs = getTotalInstancesOnNodes(oc, opNamespace, kataNodes)
			//TO DO wait for some time to enable disposal of previous test instances
			o.Expect(numOfVMs).To(o.Equal(0), fmt.Sprintf("initial number of VM instances should be zero"))
		}

		g.By("Create deployment config from template")
		configFile, err := oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", defaultDeployment,
			"-p", "NAME="+deployName, "-p", "REPLICAS="+strconv.Itoa(initReplicas),
			"-p", "RUNTIMECLASSNAME="+kataconfig.runtimeClassName, "-p", "IMAGE="+testrun.workloadImage).OutputToFile(getRandomString() + "dep-common.json")
		o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("Could not create deployment configFile %v", configFile))

		g.By("Applying deployment file " + configFile)
		msg, err = oc.AsAdmin().Run("apply").Args("-f", configFile, "-n", podNs).Output()
		o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("Could not apply configFile %v", msg))

		g.By("Wait for deployment to be ready")
		defer oc.AsAdmin().Run("delete").Args("deploy", "-n", podNs, deployName, "--ignore-not-found").Execute()
		msg, err = waitForDeployment(oc, podNs, deployName)
		o.Expect(err).NotTo(o.HaveOccurred())
		errReplicasMsg := fmt.Sprintf("Deployment %v number of ready replicas don't match requested", deployName)
		o.Expect(msg).To(o.Equal(strconv.Itoa(initReplicas)), errReplicasMsg)

		if !kataconfig.enablePeerPods {
			g.By("Verifying actual number of VM instances")
			numOfVMs = getTotalInstancesOnNodes(oc, opNamespace, kataNodes)
			o.Expect(numOfVMs).To(o.Equal(initReplicas), fmt.Sprintf("actual number of VM instances doesn't match"))
		}

		g.By(fmt.Sprintf("Scaling deployment from %v to %v", initReplicas, maxReplicas))
		err = oc.AsAdmin().Run("scale").Args("deployment", deployName, "--replicas="+strconv.Itoa(maxReplicas), "-n", podNs).Execute()
		o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("Could not Scale deployment %v", msg))
		msg, err = waitForDeployment(oc, podNs, deployName)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(msg).To(o.Equal(strconv.Itoa(maxReplicas)), errReplicasMsg)

		if !kataconfig.enablePeerPods {
			numOfVMs = getTotalInstancesOnNodes(oc, opNamespace, kataNodes)
			o.Expect(numOfVMs).To(o.Equal(maxReplicas), fmt.Sprintf("actual number of VM instances doesn't match"))
		}
		g.By("SUCCESSS - deployment scale-up finished successfully")
	})

	g.It("Author:vvoronko-High-60233-Scale-down deployment [Serial]", func() {
		oc.SetupProject()
		var (
			podNs        = oc.Namespace()
			deployName   = "dep-60233-" + getRandomString()
			initReplicas = 6
			updReplicas  = 3
			numOfVMs     int
			msg          string
		)

		kataNodes := exutil.GetNodeListByLabel(oc, kataocLabel)
		o.Expect(len(kataNodes) > 0).To(o.BeTrue(), fmt.Sprintf("kata nodes list is empty %v", kataNodes))

		if !kataconfig.enablePeerPods {
			g.By("Verify no instaces exists before the test")
			numOfVMs = getTotalInstancesOnNodes(oc, opNamespace, kataNodes)
			//TO DO wait for some time to enable disposal of previous test instances
			o.Expect(numOfVMs).To(o.Equal(0), fmt.Sprintf("initial number of VM instances should be zero"))
		}

		g.By("Create deployment config from template")
		configFile, err := oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", defaultDeployment,
			"-p", "NAME="+deployName, "-p", "REPLICAS="+strconv.Itoa(initReplicas), "-p", "RUNTIMECLASSNAME="+kataconfig.runtimeClassName,
			"-p", "IMAGE="+testrun.workloadImage).OutputToFile(getRandomString() + "dep-common.json")
		o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("Could not create deployment configFile %v", configFile))

		g.By("Applying deployment file " + configFile)
		msg, err = oc.AsAdmin().Run("apply").Args("-f", configFile, "-n", podNs).Output()
		o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("Could not apply configFile %v", msg))

		g.By("Wait for deployment to be ready")
		defer oc.AsAdmin().Run("delete").Args("deploy", "-n", podNs, deployName, "--ignore-not-found").Execute()
		msg, err = waitForDeployment(oc, podNs, deployName)
		o.Expect(err).NotTo(o.HaveOccurred())
		errReplicasMsg := fmt.Sprintf("Deployment %v number of ready replicas don't match requested", deployName)
		o.Expect(msg).To(o.Equal(strconv.Itoa(initReplicas)), errReplicasMsg)

		if !kataconfig.enablePeerPods {
			g.By("Verifying actual number of VM instances")
			numOfVMs = getTotalInstancesOnNodes(oc, opNamespace, kataNodes)
			o.Expect(numOfVMs).To(o.Equal(initReplicas), fmt.Sprintf("actual number of VM instances doesn't match"))
		}

		g.By(fmt.Sprintf("Scaling deployment from %v to %v", initReplicas, updReplicas))
		err = oc.AsAdmin().Run("scale").Args("deployment", deployName, "--replicas="+strconv.Itoa(updReplicas), "-n", podNs).Execute()
		o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("Could not Scale deployment %v", msg))

		msg, err = waitForDeployment(oc, podNs, deployName)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(msg).To(o.Equal(strconv.Itoa(updReplicas)), errReplicasMsg)

		if !kataconfig.enablePeerPods {
			numOfVMs = getTotalInstancesOnNodes(oc, opNamespace, kataNodes)
			o.Expect(numOfVMs).To(o.Equal(updReplicas), fmt.Sprintf("actual number of VM instances doesn't match"))
		}
		g.By("SUCCESSS - deployment scale-down finished successfully")
	})

	g.It("Author:vvoronko-High-64043-expose-serice deployment", func() {

		oc.SetupProject()
		var (
			podNs         = oc.Namespace()
			deployName    = "dep-64043-" + getRandomString()
			msg           string
			statusCode    = 200
			testPageBody  = "Hello OpenShift!"
			ocpHelloImage = "quay.io/openshifttest/hello-openshift:1.2.0" // should this be testrun.workloadImage?
		)

		g.By("Create deployment config from template")
		configFile, err := oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", defaultDeployment,
			"-p", "NAME="+deployName, "-p", "IMAGE="+ocpHelloImage,
			"-p", "RUNTIMECLASSNAME="+kataconfig.runtimeClassName).OutputToFile(getRandomString() + "dep-common.json")
		o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("Could not create configFile %v", configFile))

		g.By("Applying deployment file " + configFile)
		msg, err = oc.AsAdmin().Run("apply").Args("-f", configFile, "-n", podNs).Output()
		o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("Could not apply configFile %v", msg))

		g.By("Wait for deployment to be ready")
		defer oc.AsAdmin().Run("delete").Args("deploy", "-n", podNs, deployName, "--ignore-not-found").Execute()
		msg, err = waitForDeployment(oc, podNs, deployName)
		o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("Deployment %v didn't reached expected state: %v", deployName, msg))

		g.By("Expose deployment and its service")
		defer deleteRouteAndService(oc, deployName, podNs)
		host, err := createServiceAndRoute(oc, deployName, podNs)
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("route host=%v", host)

		g.By("send request via the route")
		strURL := "http://" + host
		resp, err := getHttpResponse(strURL, statusCode)
		o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("send request via the route %v failed with: %v", strURL, err))
		o.Expect(resp).To(o.ContainSubstring(testPageBody), fmt.Sprintf("Response doesn't match"))

		g.By("SUCCESSS - deployment Expose service finished successfully")
	})

	g.It("Author:vvoronko-High-63121-Peerpods-cluster-limit [Serial]", func() {

		//TODO edge case: check no podvms are up in the air somehow others test will fail
		if !kataconfig.enablePeerPods {
			g.Skip("63121 podvm limit test is only for peer pods")
		}

		oc.SetupProject()

		var (
			podNs           = oc.Namespace()
			deployName      = "dep-63121-" + getRandomString()
			podIntLimit     = 2
			defaultLimit    = "10"
			kataNodesAmount = len(exutil.GetNodeListByLabel(oc, kataocLabel))
			msg             string
			cleanupRequired = true
		)

		defer func() {
			if cleanupRequired {
				e2e.Logf("Cleanup required, restoring to default %v", defaultLimit)
				patchPeerPodLimit(oc, opNamespace, defaultLimit)
			}
		}()

		patchPeerPodLimit(oc, opNamespace, strconv.Itoa(podIntLimit))

		g.By("Create deployment config from template")
		initReplicas := strconv.Itoa(podIntLimit * kataNodesAmount)
		configFile, err := oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", defaultDeployment,
			"-p", "NAME="+deployName, "-p", "REPLICAS="+initReplicas,
			"-p", "RUNTIMECLASSNAME="+kataconfig.runtimeClassName, "-p", "IMAGE="+testrun.workloadImage).OutputToFile(getRandomString() + "dep-common.json")
		o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("Could not create deployment configFile %v", configFile))

		g.By("Applying deployment file " + configFile)
		msg, err = oc.AsAdmin().Run("apply").Args("-f", configFile, "-n", podNs).Output()
		o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("Could not apply configFile %v", msg))

		defer deleteKataResource(oc, "deploy", podNs, deployName)

		g.By("Wait for deployment to be ready")
		msg, err = waitForDeployment(oc, podNs, deployName)
		e2e.Logf("Deployment has initially %v pods", msg)
		o.Expect(err).NotTo(o.HaveOccurred())
		errReplicasMsg := fmt.Sprintf("Deployment %v number of ready replicas don't match requested", deployName)
		o.Expect(msg).To(o.Equal(initReplicas), errReplicasMsg)

		extraReplicas := strconv.Itoa((podIntLimit + 1) * kataNodesAmount)
		g.By(fmt.Sprintf("Scaling deployment from %v to %v", initReplicas, extraReplicas))
		msg, err = oc.AsAdmin().Run("scale").Args("deployment", deployName, "--replicas="+extraReplicas, "-n", podNs).Output()
		o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("Could not Scale deployment %v", msg))

		extraPods := strconv.Itoa(kataNodesAmount)
		g.By("Wait for 30sec to check deployment has " + extraPods + " pending pods w/o corresponding podvm, because of the limit")
		errCheck := wait.Poll(30*time.Second, snooze*time.Second, func() (bool, error) {
			msg, err = oc.AsAdmin().Run("get").Args("deploy", "-n", podNs, deployName, "-o=jsonpath={.status.unavailableReplicas}").Output()
			if msg == extraPods {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("Timed out waiting for %v additional pending pods %v %v", extraPods, msg, err))

		msg, err = oc.AsAdmin().Run("get").Args("deploy", "-n", podNs, deployName, "-o=jsonpath={.status.readyReplicas}").Output()
		o.Expect(msg).To(o.Equal(initReplicas), errReplicasMsg)

		g.By("restore podvm limit")
		patchPeerPodLimit(oc, opNamespace, defaultLimit)
		cleanupRequired = false

		msg, err = waitForDeployment(oc, podNs, deployName)
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Deployment has %v running pods after patching the limit", msg)
		o.Expect(msg).To(o.Equal(extraReplicas), errReplicasMsg)

		g.By("SUCCESSS - deployment peer pods podvm limit - finished successfully")
	})

	g.It("Author:vvoronko-High-57339-Eligibility", func() {

		if !kataconfig.eligibility {
			g.Skip("57339-Eligibility test is only for eligibility=true in kataconfig")
		}

		oc.SetupProject()

		kataNodes := exutil.GetNodeListByLabel(oc, kataocLabel)
		o.Expect(len(kataNodes) > 0).To(o.BeTrue(), fmt.Sprintf("kata nodes list is empty %v", kataNodes))

		eligibleNodes := exutil.GetNodeListByLabel(oc, featureLabel)
		o.Expect(len(eligibleNodes) == len(kataNodes)).To(o.BeTrue(), fmt.Sprintf("kata nodes list length is differ from eligible ones"))

		for _, node := range kataNodes {
			found, _ := exutil.StringsSliceContains(eligibleNodes, node)
			o.Expect(found).To(o.BeTrue(), fmt.Sprintf("node %v is not in the list of eligible nodes %v", node, eligibleNodes))
		}
	})

	g.It("Author:vvoronko-High-67650-pod-with-filesystem", func() {
		oc.SetupProject()
		var (
			podNs    = oc.Namespace()
			pvcName  = "pvc-67650-" + getRandomString()
			capacity = "2"
		)
		err := createRWOfilePVC(oc, podNs, pvcName, capacity)
		defer oc.WithoutNamespace().AsAdmin().Run("delete").Args("pvc", pvcName, "-n", podNs, "--ignore-not-found").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		//some platforms provision automatically while others wait got the 1st customer with "Pending" status
		//_, err = checkResourceJsonpath(oc, "pvc", pvcName, podNs, "-o=jsonpath={.status.phase}", "Bound", 30*time.Second, 5*time.Second)

		//TODO: add a function that takes any pod and know to inject storage part to it)
		// run pod with kata
		//TODO: test IO
	})

	g.It("Author:tbuskey-High-66554-Check and verify control plane pods and other components", func() {

		var (
			duration time.Duration = 300
			interval time.Duration = 10
		)

		testControlPod := func(resType, resName, desiredCountJsonPath, actualCountJsonPath, podLabel string) {
			// Check the resource Type for desired count by looking at the jsonpath
			// Check the actual count at this jsonpath
			// Wait until the actual count == desired count then set expectedPods to the actual count
			// Verify count of "Running" pods with podLabel matches expectedPods
			expectedPods, msg, err := checkResourceJsonpathMatch(oc, resType, resName, subscription.namespace, desiredCountJsonPath, actualCountJsonPath)
			if err != nil || msg == "" {
				e2e.Logf("%v does not match %v in %v %v %v %v", desiredCountJsonPath, actualCountJsonPath, resName, resType, msg, err)
			}
			o.Expect(err).NotTo(o.HaveOccurred())

			msg, err = checkLabeledPodsExpectedRunning(oc, subscription.namespace, podLabel, expectedPods)
			if err != nil || msg == "" {
				e2e.Logf("Could not find pods labeled %v %v %v", podLabel, msg, err)
			}
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		testControlPod("deployment", "controller-manager", "-o=jsonpath={.spec.replicas}", "-o=jsonpath={.status.readyReplicas}", "control-plane=controller-manager")
		testControlPod("daemonset", "openshift-sandboxed-containers-monitor", "-o=jsonpath={.status.desiredNumberScheduled}", "-o=jsonpath={.status.numberReady}", "name=openshift-sandboxed-containers-monitor")

		if kataconfig.enablePeerPods {
			testControlPod("deployment", "peer-pods-webhook", "-o=jsonpath={.spec.replicas}", "-o=jsonpath={.status.readyReplicas}", "app=peer-pods-webhook")
			testControlPod("daemonset", caaDaemonName, "-o=jsonpath={.status.desiredNumberScheduled}", "-o=jsonpath={.status.numberReady}", "name="+caaDaemonName)
			// Check for the peer pod RuntimeClass
			msg, err := checkResourceExists(oc, "RuntimeClass", ppRuntimeClass, subscription.namespace, duration, interval)
			if err != nil || msg == "" {
				e2e.Logf("Could not find %v in RuntimeClass %v %v", ppRuntimeClass, msg, err)
			}

			// and kata RuntimeClass
			msg, err = checkResourceExists(oc, "RuntimeClass", "kata", subscription.namespace, duration, interval)
			if err != nil || msg == "" {
				e2e.Logf("Could not find kata in RuntimeClass %v %v", msg, err)
			}
		}
	})

	g.It("Author:tbuskey-High-68945-Check FIPS on pods", func() {

		if !clusterHasEnabledFIPS(oc, subscription.namespace) {
			g.Skip("The cluster does not have FIPS enabled")
		}

		oc.SetupProject()
		podNamespace := oc.Namespace()

		podName := createKataPod(oc, podNamespace, defaultPod, "pod68945", kataconfig.runtimeClassName, testrun.workloadImage)
		defer deleteKataResource(oc, "pod", podNamespace, podName)
		msg, err := checkResourceJsonpath(oc, "pod", podName, podNamespace, "-o=jsonpath={.status.phase}", podRunState, podSnooze*time.Second, 10*time.Second)
		o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("ERROR: pod %v could not be created: %v %v", podName, msg, err))

		msgIfErr := "ERROR: The cluster is in FIPS but pods are not"
		// check that the pod(vm) booted with fips
		podCmdline, podCmdlineErr := oc.AsAdmin().Run("rsh").Args("-T", "-n", podNamespace, podName, "cat", "/proc/cmdline").Output()
		if podCmdlineErr != nil || !strings.Contains(podCmdline, "fips=1") {
			msgIfErr = fmt.Sprintf("%v\nERROR: %v did not boot with fips enabled:%v %v", msgIfErr, podName, podCmdline, podCmdlineErr)
		}

		// check that pod(vm) has fips enabled
		podFipsEnabled, podFipsEnabledErr := oc.AsAdmin().Run("rsh").Args("-T", "-n", podNamespace, podName, "cat", "/proc/sys/crypto/fips_enabled").Output()
		if podFipsEnabledErr != nil || podFipsEnabled != "1" {
			msgIfErr = fmt.Sprintf("%v\nERROR: %v does not have fips_enabled: %v %v", msgIfErr, podName, podFipsEnabled, podFipsEnabledErr)
		}

		// fail with all possible debugging logs included
		o.Expect(podCmdlineErr).NotTo(o.HaveOccurred(), msgIfErr)
		o.Expect(podCmdline).To(o.ContainSubstring("fips=1"), msgIfErr)
		o.Expect(podFipsEnabledErr).NotTo(o.HaveOccurred(), msgIfErr)
		o.Expect(podFipsEnabled).To(o.Equal("1"), msgIfErr)

	})

	g.It("Author:vvoronko-High-68930-deploy peerpod with type annotation", func() {

		if testrun.workloadToTest == "coco" {
			g.Skip("Test not supported with coco")
		}

		oc.SetupProject()

		var (
			basePodName = "-example-68930"
			podNs       = oc.Namespace()
			annotations = map[string]string{
				"MEMORY":       "256",
				"CPU":          "0",
				"INSTANCESIZE": "",
			}
			instanceSize = map[string]string{
				"aws":   "t3.xlarge",
				"azure": "Standard_D4as_v5",
			}
		)

		provider := getCloudProvider(oc)

		val, ok := instanceSize[provider]

		if !(kataconfig.enablePeerPods && ok) {
			g.Skip("68930-deploy peerpod with type annotation supported only for kata-remote on AWS and AZURE")
		}
		annotations["INSTANCESIZE"] = val

		g.By("Deploying pod with kata runtime and verify it")
		podName, err := createKataPodAnnotated(oc, podNs, podAnnotatedTemplate, basePodName, kataconfig.runtimeClassName, testrun.workloadImage, annotations)
		defer deleteKataResource(oc, "pod", podNs, podName)
		o.Expect(err).NotTo(o.HaveOccurred())

		actualSize, err := getPeerPodMetadataInstanceType(oc, podNs, podName, provider)
		e2e.Logf("Podvm with required instance type %v was launched as %v", instanceSize[provider], actualSize)
		o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("Failed rsh to pod %v to provide metadata: %v", podName, err))
		o.Expect(actualSize).To(o.Equal(instanceSize[provider]), fmt.Sprintf("Instance size don't match provided annotations: %v", err))

		g.By("SUCCESS - Podvm with required instance type was launched")
	})

	g.It("Author:vvoronko-High-69018-deploy peerpod with default vcpu and memory", func() {

		if testrun.workloadToTest == "coco" {
			g.Skip("Test not supported with coco")
		}

		oc.SetupProject()

		var (
			basePodName = "-example-69018"
			podNs       = oc.Namespace()
			annotations = map[string]string{
				"MEMORY":       "6000",
				"CPU":          "2",
				"INSTANCESIZE": "",
			}
			instanceSize = map[string]string{
				"aws":   "t3.large",
				"azure": "Standard_D2as_v5",
			}
		)

		provider := getCloudProvider(oc)

		val, ok := instanceSize[provider]

		if !(kataconfig.enablePeerPods && ok) {
			g.Skip("69018-deploy peerpod with type annotation not supported on " + provider)
		}
		annotations["INSTANCESIZE"] = val

		g.By("Deploying pod with kata runtime and verify it")
		podName, err := createKataPodAnnotated(oc, podNs, podAnnotatedTemplate, basePodName, kataconfig.runtimeClassName, testrun.workloadImage, annotations)
		defer deleteKataResource(oc, "pod", podNs, podName)
		o.Expect(err).NotTo(o.HaveOccurred())

		actualSize, err := getPeerPodMetadataInstanceType(oc, podNs, podName, provider)
		e2e.Logf("Podvm with required instance type %v was launched as %v", instanceSize[provider], actualSize)
		o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("Failed rsh to pod %v to provide metadata: %v", podName, err))
		o.Expect(actualSize).To(o.Equal(instanceSize[provider]), fmt.Sprintf("Instance size don't match provided annotations: %v", err))

		g.By("SUCCESS - Podvm with required instance type was launched")
	})

	g.It("Author:vvoronko-High-69589-deploy kata with cpu and memory annotation", func() {

		oc.SetupProject()

		var (
			basePodName = "-example-69589"
			podNs       = oc.Namespace()
			annotations = map[string]string{
				"MEMORY":       "1234",
				"CPU":          "2",
				"INSTANCESIZE": "",
			}
			supportedProviders = []string{"azure", "gcp", "none"}
			memoryOptions      = fmt.Sprintf("-m %vM", annotations["MEMORY"])
		)

		provider := getCloudProvider(oc)
		if kataconfig.enablePeerPods || !slices.Contains(supportedProviders, provider) {
			g.Skip("69589-deploy kata with type annotation supported only for kata runtime on platforms with nested virtualization enabled")
		}

		g.By("Deploying pod with kata runtime and verify it")
		podName, err := createKataPodAnnotated(oc, podNs, podAnnotatedTemplate, basePodName, kataconfig.runtimeClassName, testrun.workloadImage, annotations)
		defer deleteKataResource(oc, "pod", podNs, podName)
		o.Expect(err).NotTo(o.HaveOccurred())

		//get annotations from the live pod
		podAnnotations, _ := oc.Run("get").Args("pods", podName, "-o=jsonpath={.metadata.annotations}", "-n", podNs).Output()
		podCmd := []string{"-n", oc.Namespace(), podName, "--", "nproc"}
		//check CPU available from the kata pod itself by nproc command:
		actualCPU, err := oc.WithoutNamespace().AsAdmin().Run("exec").Args(podCmd...).Output()
		o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("'oc exec %v' Failed", podCmd))
		strErr := fmt.Sprintf("Actual CPU count for the pod %v isn't matching expected %v full annotations:\n%v", actualCPU, annotations["CPU"], podAnnotations)
		o.Expect(actualCPU).To(o.Equal(annotations["CPU"]), strErr)

		//check MEMORY from the node running kata VM:
		nodeName, _ := exutil.GetPodNodeName(oc, podNs, podName)
		cmd := "ps -ef | grep uuid | grep -v grep"
		vmFlags, err := exutil.DebugNodeWithOptionsAndChroot(oc, nodeName, []string{"-q"}, "bin/sh", "-c", cmd)
		o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("Failed debug node to get qemu instance options"))
		strErr = fmt.Sprintf("VM flags for the pod doesn't contain expected %v full annotations:\n%v", memoryOptions, podAnnotations)
		o.Expect(vmFlags).To(o.ContainSubstring(memoryOptions), strErr)
		g.By("SUCCESS - KATA pod with required VM instance size was launched")
	})

	g.It("Author:abhbaner-High-66123-podvm Image ID check peer pods", func() {

		var (
			msg     string
			err     error
			imageID string
		)

		if !kataconfig.enablePeerPods {
			g.Skip("OCP-66123 is only for peerpods")
		}

		oc.SetupProject()
		cloudPlatform := getCloudProvider(oc)

		// check if IMAGE ID exists in peer-pod-cm
		msg, err, imageID = CheckPodVMImageID(oc, ppConfigMapName, cloudPlatform, opNamespace)

		if imageID == "" {
			e2e.Logf("IMAGE ID: %v", imageID)
			msgIfErr := fmt.Sprintf("ERROR: IMAGE ID could not be retrieved from the peer-pods-cm even after kataconfig install: %v %v %v", imageID, msg, err)
			o.Expect(imageID).NotTo(o.BeEmpty(), msgIfErr)
			o.Expect(err).NotTo(o.HaveOccurred(), msgIfErr)
		}

		e2e.Logf("The Image ID present in the peer-pods-cm is: %v , msg: %v", imageID, msg)
		g.By("SUCCESS - IMAGE ID check complete")
	})

	g.It("Author:tbuskey-Medium-70824-Catalog upgrade osc operator [Disruptive]", func() {
		g.Skip("Upgrade tests should be manually done")
		upgradeCatalog := UpgradeCatalogDescription{
			name:        "osc-config-upgrade-catalog",
			namespace:   "default",
			exists:      false,
			imageAfter:  "",
			imageBefore: "",
			catalogName: subscription.catalogSourceName,
		}

		err := getUpgradeCatalogConfigMap(oc, &upgradeCatalog)
		if !upgradeCatalog.exists {
			skipMessage := fmt.Sprintf("%v configmap for Catalog upgrade does not exist", upgradeCatalog.name)
			g.Skip(skipMessage)
		}
		o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("ERROR: could not get %v configmap in ns %v %v", upgradeCatalog.name, upgradeCatalog.namespace, err))

		// what is the current CSV name?
		csvNameBefore, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", subscription.subName, "-n", subscription.namespace, "-o=jsonpath={.status.currentCSV}").Output()
		o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("ERROR: could not get the CSV name of sub %v %v %v", subscription.subName, csvNameBefore, err))
		o.Expect(csvNameBefore).NotTo(o.BeEmpty(), fmt.Sprintf("ERROR: the csv name is empty for sub %v", subscription.subName))

		// what is the controller-manager pod name?
		listOfPodsBefore, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", subscription.namespace, "-l", "control-plane=controller-manager", "-o=jsonpath={.items..metadata.name}").Output()

		err = changeCatalogImage(oc, upgradeCatalog.catalogName, upgradeCatalog.imageAfter)
		o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("ERROR: could not change catalog %v image to %v Error %v", upgradeCatalog.catalogName, upgradeCatalog.imageAfter, err))

		e2e.Logf("Waiting for pods (%v) to get replaced", listOfPodsBefore)
		waitForPodsToTerminate(oc, subscription.namespace, listOfPodsBefore)

		// subscription .status.installedCsv is "AtLatestKnown" & will not changed so it doesn't show subscription is done
		// wait until the currentCSV in the sub changes & get the new CSV name
		csvNameAfter, _ := checkResourceJsonPathChanged(oc, "sub", subscription.subName, subscription.namespace, "-o=jsonpath={.status.currentCSV}", csvNameBefore, 300*time.Second, 10*time.Second)
		e2e.Logf("Watch CSV %v to show Succeed", csvNameAfter)
		_, _ = checkResourceJsonpath(oc, "csv", csvNameAfter, subscription.namespace, "-o=jsonpath={.status.phase}", "Succeeded", 300*time.Second, 10*time.Second)
	})

	g.It("Author:vvoronko-High-C00210-run [peerpodGPU] cuda-vectoradd", func() {

		oc.SetupProject()

		var (
			basePodName  = "-example-00210"
			cudaImage    = "nvidia/samples:vectoradd-cuda11.2.1"
			podNs        = oc.Namespace()
			instanceSize = map[string]string{
				"aws":   "g5.2xlarge",
				"azure": "Standard_NC8as_T4_v3",
			}
			phase     = "Succeeded"
			logPassed = "Test PASSED"
		)

		if !(kataconfig.enablePeerPods && testrun.enableGPU) {
			g.Skip("210-run peerpod with GPU cuda-vectoradd supported only with GPU enabled in podvm")
		}

		instance := instanceSize[getCloudProvider(oc)]

		g.By("Deploying pod with kata runtime and verify it")
		newPodName := getRandomString() + basePodName
		configFile, err := oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", podAnnotatedTemplate,
			"-p", "NAME="+newPodName,
			"-p", "RUNTIMECLASSNAME="+kataconfig.runtimeClassName,
			"-p", "INSTANCESIZE="+instance,
			"-p", "IMAGE="+cudaImage).OutputToFile(getRandomString() + "Pod-common.json")
		o.Expect(err).NotTo(o.HaveOccurred())
		podName, err := createKataPodFromTemplate(oc, podNs, newPodName, configFile, kataconfig.runtimeClassName, phase)
		defer deleteKataResource(oc, "pod", podNs, podName)
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("vectoradd-cuda on peer pod with GPU instance type %v reached %v phase", instance, phase)

		//verify the log of the pod
		log, err := exutil.GetSpecificPodLogs(oc, podNs, "", podName, "")
		o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("unable to get the pod (%s) logs", podName))
		o.Expect(log).To(o.ContainSubstring(logPassed), "required lines are missing in log")

		g.By("SUCCESS - Podvm with GPU instance type was launched successfully")
	})

	g.It("Author:Anjana-High-43221-Verify PodVM image creation job completion", func() {
		if getCloudProvider(oc) != "libvirt" {
			g.Skip("43221 PodVM image creation job is specific to libvirt")
		}
		if !kataconfig.enablePeerPods {
			g.Skip("43221 PodVM image creation job is only for peer pods")
		}
		g.By("Checking the status of the PodVM image creation job")
		msg, err := verifyImageCreationJobSuccess(oc, opNamespace, ppParam, ppParamsLibvirtConfigMapName, cloudPlatform)
		o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("%v", err))
		expMsg := "Uploaded the image successfully"
		o.Expect(strings.Contains(msg, expMsg)).To(o.BeTrue(), fmt.Sprintf("Expected message: %v not found in the job output.", expMsg))
		g.By("SUCCESS - PodVM image creation job completed successfully")
	})

	g.It("Author:Anjana-High-422081-Verify SE-enabled pod deployment", func() {
		if getCloudProvider(oc) != "libvirt" {
			g.Skip("422081 SE-enabled pod deployment is specific to libvirt")
		}
		oc.SetupProject()

		var (
			msg            string
			err            error
			defaultPodName = "-se-check"
			podNs          = oc.Namespace()
		)

		g.By("Deploying pod to verify SE enablement")
		newPodName := createKataPod(oc, podNs, defaultPod, defaultPodName, kataconfig.runtimeClassName, testrun.workloadImage)
		defer func() {
			deleteKataResource(oc, "pod", podNs, newPodName)
			g.By("Deleted SE-enabled pod")
		}()

		msg, err = checkResourceJsonpath(oc, "pod", newPodName, podNs, "-o=jsonpath={.status.phase}", podRunState, podSnooze*time.Second, 10*time.Second)
		if err != nil {
			e2e.Logf("ERROR: pod %v could not be installed: %v %v", newPodName, msg, err)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		g.By("SUCCESS - Pod installed for SE verification")

		g.By("Checking if pod is SE-enabled")
		err = checkSEEnabled(oc, newPodName, podNs)
		o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("%v", err))
	})

	g.It("Author:tbuskey-High-C00316-run and verify cosigned pod", func() {

		if testrun.workloadToTest != "coco" {
			g.Skip("Run and verify cosigned pod is only for workloadToTest = 'coco'")
		}
		oc.SetupProject()

		var (
			podName            = "ocp-cc-pod"
			testNamespace      = oc.Namespace()
			podLastEventReason string
			loopCount          int
			loopMax            = 450
			countIncrement     = 15
			sleepTime          = time.Duration(countIncrement) * time.Second
			outputFromOc       string
		)

		defer deleteResource(oc, "pod", podName, testNamespace, 90*time.Second, 10*time.Second)

		msg, err := oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", trusteeCosignedPodFile, "-n", testNamespace).Output()
		if err != nil {
			e2e.Logf("Error: applying cosigned pod file %v failed: %v %v", trusteeCosignedPodFile, msg, err)
		}

		for !strings.Contains(podLastEventReason, "Started") && loopCount < loopMax {
			loopCount = loopCount + countIncrement
			outputFromOc, err = oc.AsAdmin().WithoutNamespace().Run("events").Args("-o=jsonpath={.items..reason}", "-n", testNamespace).Output()
			splitString := strings.Split(outputFromOc, " ")
			podLastEventReason = splitString[len(splitString)-1]
			e2e.Logf("%v pod event reason: %v", podName, podLastEventReason)
			if strings.Contains(outputFromOc, "Failed") || loopCount >= loopMax {
				err = fmt.Errorf("pod %v failed err: %v timeout: %v of %v\n\n", podName, err, loopCount, loopMax)
			}
			o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("ERROR: %v", err))
			time.Sleep(sleepTime)
		}
	})

	g.It("Author:vvoronko-High-C00317-delete operator with running workload [Serial]", func() {

		oc.SetupProject()

		var (
			msg            string
			err            error
			defaultPodName = "-example-00317"
			podNs          = oc.Namespace()
		)

		g.By("Deploying pod with kata runtime and verify it")
		fstPodName := createKataPod(oc, podNs, defaultPod, defaultPodName, kataconfig.runtimeClassName, testrun.workloadImage)
		defer deleteKataResource(oc, "pod", podNs, fstPodName)

		msg, err = checkResourceJsonpath(oc, "pod", fstPodName, podNs, "-o=jsonpath={.status.phase}", podRunState, podSnooze*time.Second, 10*time.Second)
		o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("ERROR: pod %v could not be installed: %v %v", fstPodName, msg, err))

		g.By("delete csv and sub")
		msg, err = deleteOperator(oc, subscription)
		o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("msg:%v err:%v", msg, err))

		g.By("verify control plane pods are running")
		if kataconfig.enablePeerPods {
			msg, err = testControlPod(oc, subscription.namespace, "daemonset", caaDaemonName,
				"-o=jsonpath={.status.desiredNumberScheduled}", "-o=jsonpath={.status.numberReady}", "name="+caaDaemonName)
			o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("msg:%v err:%v", msg, err))
			msg, err = testControlPod(oc, subscription.namespace, "deployment", "peer-pods-webhook",
				"-o=jsonpath={.spec.replicas}", "-o=jsonpath={.status.readyReplicas}", "app=peer-pods-webhook")
			o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("msg:%v err:%v", msg, err))
		}

		msg, err = testControlPod(oc, subscription.namespace, "daemonset", "openshift-sandboxed-containers-monitor",
			"-o=jsonpath={.status.desiredNumberScheduled}", "-o=jsonpath={.status.numberReady}", "name=openshift-sandboxed-containers-monitor")
		o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("msg:%v err:%v", msg, err))

		g.By("monitor the 1st pod is still running")
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", fstPodName, "-n", podNs, "-o=jsonpath={.status.phase}").Output()
		o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("ERROR: pod %v is not in expected state: %v, actual is: %v %v", fstPodName, podRunState, msg, err))

		//launch another pod
		secPodName := createKataPod(oc, podNs, defaultPod, defaultPodName, kataconfig.runtimeClassName, testrun.workloadImage)
		defer deleteKataResource(oc, "pod", podNs, secPodName)

		msg, err = checkResourceJsonpath(oc, "pod", secPodName, podNs, "-o=jsonpath={.status.phase}", podRunState, podSnooze*time.Second, 10*time.Second)
		o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("ERROR: pod %v could not be installed: %v %v", secPodName, msg, err))

		g.By("SUCCESS - operator deleted while workload keep running")
	})

	g.It("Author:vvoronko-High-C00999-deploy peerpod with tags", func() {

		if !(testrun.workloadToTest == "peer-pods" && getCloudProvider(oc) == "azure") {
			g.Skip("Test supported only with peer-pods on Azure since AWS tags disabled for metadata by default")
		}

		oc.SetupProject()

		var (
			basePodName = "-example-00999"
			podNs       = oc.Namespace()
			//works with default configmap value
			tagValue = map[string]string{
				"aws":   "value1",
				"azure": "key1:value1;key2:value2", //format is different than in configmap
			}
		)

		provider := getCloudProvider(oc)

		g.By("Deploying pod with kata runtime and verify it")
		newPodName := createKataPod(oc, podNs, defaultPod, basePodName, kataconfig.runtimeClassName, testrun.workloadImage)
		defer deleteKataResource(oc, "pod", podNs, newPodName)
		msg, err := checkResourceJsonpath(oc, "pod", newPodName, podNs, "-o=jsonpath={.status.phase}", podRunState, podSnooze*time.Second, 10*time.Second)
		if err != nil {
			e2e.Logf("ERROR: pod %v could not be installed: %v %v", newPodName, msg, err)
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		actualValue, err := getPeerPodMetadataTags(oc, podNs, newPodName, provider)
		o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("Failed rsh to pod %v to provide metadata: %v", newPodName, err))
		e2e.Logf("%v pod tags: %v", newPodName, actualValue)
		o.Expect(actualValue).To(o.ContainSubstring(tagValue[provider]), fmt.Sprintf("Instance size don't match provided annotations: %v", err))

		g.By("SUCCESS - Podvm with required instance type was launched")
	})
})
