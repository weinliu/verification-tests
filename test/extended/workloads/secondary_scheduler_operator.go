package workloads

import (
	"github.com/openshift/openshift-tests-private/test/extended/util/architecture"
	"path/filepath"
	"regexp"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

	"strings"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-scheduling] Workloads The Descheduler Operator automates pod evictions using different profiles", func() {
	defer g.GinkgoRecover()
	var (
		oc                                                          = exutil.NewCLI("default-"+getRandomString(), exutil.KubeConfigPath())
		kubeNamespace                                               = "openshift-secondary-scheduler-operator"
		buildPruningBaseDir                                         string
		ssoOperatorGroupT                                           string
		ssoSubscriptionT                                            string
		secondarySchedulerT                                         string
		secondarySchedulerConfig                                    string
		sub                                                         ssoSubscription
		og                                                          ssoOperatorgroup
		secschu                                                     secondaryScheduler
		ssImage                                                     string
		guestClusterName, guestClusterKubeconfig, hostedClusterName string
	)

	g.BeforeEach(func() {
		buildPruningBaseDir = exutil.FixturePath("testdata", "workloads")
		ssoOperatorGroupT = filepath.Join(buildPruningBaseDir, "sso_operatorgroup.yaml")
		ssoSubscriptionT = filepath.Join(buildPruningBaseDir, "sso_subscription.yaml")
		secondarySchedulerT = filepath.Join(buildPruningBaseDir, "secondaryScheduler.yaml")
		secondarySchedulerConfig = filepath.Join(buildPruningBaseDir, "SecondarySchedulerConfig.yaml")

		sub = ssoSubscription{
			name:        "openshift-secondary-scheduler-operator",
			namespace:   kubeNamespace,
			channelName: "stable",
			opsrcName:   "qe-app-registry",
			sourceName:  "openshift-marketplace",
			startingCSV: "secondaryscheduleroperator.v1.2.1",
			template:    ssoSubscriptionT,
		}

		og = ssoOperatorgroup{
			name:      "openshift-secondary-scheduler-operator",
			namespace: kubeNamespace,
			template:  ssoOperatorGroupT,
		}

		// Get secheduler Image
		guestClusterName, guestClusterKubeconfig, hostedClusterName = exutil.ValidHypershiftAndGetGuestKubeConfWithNoSkip(oc)
		if guestClusterKubeconfig != "" {
			hostedClusterNS := hostedClusterName + "-" + guestClusterName
			e2e.Logf("hostedClusterNS is %s", hostedClusterNS)
			schedulerPodNameHypershift, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", hostedClusterNS, "-l=app=kube-scheduler", "-o=jsonpath={.items[0].metadata.name}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(schedulerPodNameHypershift).NotTo(o.BeEmpty())
			ssImage, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", hostedClusterNS, schedulerPodNameHypershift, "-o", "yaml", "-o=jsonpath={.spec.containers[0].image}").Output()
			oc.SetGuestKubeconf(guestClusterKubeconfig)
		} else {
			ssImage = getSchedulerImage(getOCPerKubeConf(oc, guestClusterKubeconfig))
		}

		secschu = secondaryScheduler{
			namespace:        kubeNamespace,
			schedulerImage:   ssImage,
			logLevel:         "Normal",
			operatorLogLevel: "Normal",
			schedulerConfig:  "secondary-scheduler-config",
			template:         secondarySchedulerT,
		}

		// Skip case on arm64 cluster
		architecture.SkipNonAmd64SingleArch(getOCPerKubeConf(oc, guestClusterKubeconfig))

		// Skip case on multi-arch cluster
		architecture.SkipArchitectures(getOCPerKubeConf(oc, guestClusterKubeconfig), architecture.MULTI)

		// Skip the test if no qe-app-registry catalog is present
		skipMissingCatalogsource(getOCPerKubeConf(oc, guestClusterKubeconfig))

	})

	// author: knarra@redhat.com
	g.It("HyperShiftMGMT-ROSA-OSD_CCS-ARO-Author:knarra-Critical-48916-Critical-48917-Install seconday scheduler operator via a deployment & verify user is able to schedule pod using secondary-scheduler [Serial]", func() {
		secondarySchedPodT := filepath.Join(buildPruningBaseDir, "deployPodWithScheduler.yaml")
		schedPodWithOutSchedT := filepath.Join(buildPruningBaseDir, "deployPodWithOutScheduler.yaml")

		g.By("Create the secondary-scheduler namespace")
		defer getOCPerKubeConf(oc, guestClusterKubeconfig).AsAdmin().WithoutNamespace().Run("delete").Args("ns", kubeNamespace).Execute()
		err := getOCPerKubeConf(oc, guestClusterKubeconfig).AsAdmin().WithoutNamespace().Run("create").Args("ns", kubeNamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create the operatorgroup")
		defer og.deleteOperatorGroup(getOCPerKubeConf(oc, guestClusterKubeconfig))
		og.createOperatorGroup(getOCPerKubeConf(oc, guestClusterKubeconfig))
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create the subscription")
		defer sub.deleteSubscription(getOCPerKubeConf(oc, guestClusterKubeconfig))
		sub.createSubscription(getOCPerKubeConf(oc, guestClusterKubeconfig))
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Wait for the secondary scheduler operator pod running")
		if ok := waitForAvailableRsRunning(getOCPerKubeConf(oc, guestClusterKubeconfig), "deploy", "secondary-scheduler-operator", kubeNamespace, "1"); ok {
			e2e.Logf("SecondarySchedulerOperator operator runnnig now\n")
		}

		// Create secondary scheduler configmap
		g.By("create secondary scheduler configmap")
		createConfigErr := getOCPerKubeConf(oc, guestClusterKubeconfig).AsAdmin().WithoutNamespace().Run("create").Args("-f", secondarySchedulerConfig).Execute()
		o.Expect(createConfigErr).NotTo(o.HaveOccurred())

		g.By("Create secondary scheduler cluster")
		defer getOCPerKubeConf(oc, guestClusterKubeconfig).AsAdmin().WithoutNamespace().Run("delete").Args("SecondaryScheduler", "--all", "-n", kubeNamespace).Execute()
		secschu.createSecondaryScheduler(getOCPerKubeConf(oc, guestClusterKubeconfig))
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check the secondary scheduler run well")
		checkAvailable(getOCPerKubeConf(oc, guestClusterKubeconfig), "deploy", "secondary-scheduler", kubeNamespace, "1")

		g.By("Validate that right version of secondary-scheduler is running")
		ssCsvOutput, err := getOCPerKubeConf(oc, guestClusterKubeconfig).AsAdmin().WithoutNamespace().Run("get").Args("csv", "-n", kubeNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(ssCsvOutput, "secondaryscheduleroperator.v1.2.1")).To(o.BeTrue())

		//Add the k8 dependencies checkpoint for SSO
		g.By("Get the latest version of Kubernetes")
		ocVersion, versionErr := getOCPerKubeConf(oc, guestClusterKubeconfig).AsAdmin().WithoutNamespace().Run("get").Args("node", "-o=jsonpath={.items[0].status.nodeInfo.kubeletVersion}").Output()
		o.Expect(versionErr).NotTo(o.HaveOccurred())
		kubenetesVersion := strings.Split(strings.Split(ocVersion, "+")[0], "v")[1]
		kuberVersion := strings.Split(kubenetesVersion, ".")[0] + "." + strings.Split(kubenetesVersion, ".")[1]
		e2e.Logf("kuberVersion is %s", kuberVersion)

		g.By("Get rebased version of kubernetes from sso operator")
		minkuberversion, deschedulerErr := getOCPerKubeConf(oc, guestClusterKubeconfig).AsAdmin().WithoutNamespace().Run("get").Args("csv", "-l=operators.coreos.com/openshift-secondary-scheduler-operator.openshift-secondary-sche=", "-n", kubeNamespace, "-o=jsonpath={.items[0].spec.minKubeVersion}").Output()
		o.Expect(deschedulerErr).NotTo(o.HaveOccurred())
		rebasedVersion := strings.Split(minkuberversion, ".")[0] + "." + strings.Split(minkuberversion, ".")[1]
		e2e.Logf("RebasedVersion is %s", rebasedVersion)

		if matched, _ := regexp.MatchString(rebasedVersion, kuberVersion); !matched {
			e2e.Failf("SSO operator not rebased with latest kubernetes")
		}

		// Create test project
		g.By("Create a new project test-sso-48916")
		defer getOCPerKubeConf(oc, guestClusterKubeconfig).AsAdmin().Run("delete").Args("ns", "test-sso-48916").Execute()
		err = getOCPerKubeConf(oc, guestClusterKubeconfig).AsAdmin().Run("create").Args("ns", "test-sso-48916").Execute()
		o.Expect(err).ShouldNot(o.HaveOccurred())

		g.By("Set namespace privileged")
		exutil.SetNamespacePrivileged(getOCPerKubeConf(oc, guestClusterKubeconfig), "test-sso-48916")

		secondarySchedPod := deployPodWithScheduler{
			pName:         "p48917-ss",
			namespace:     "test-sso-48916",
			schedulerName: "secondary-scheduler",
			template:      secondarySchedPodT,
		}

		defaultSchedPod := deployPodWithScheduler{
			pName:         "p48917-ds",
			namespace:     "test-sso-48916",
			schedulerName: "default-scheduler",
			template:      secondarySchedPodT,
		}

		podWithOutSched := deployPodWithOutScheduler{
			pName:     "p48917-wos",
			namespace: "test-sso-48916",
			template:  schedPodWithOutSchedT,
		}

		// Create and validate pods to make sure they are created by specified schedulers

		g.By("Create pod so that it is scheduled via secondary-scheduler")
		secondarySchedPod.createPodWithScheduler(getOCPerKubeConf(oc, guestClusterKubeconfig))

		// Check pod status
		checkPodStatus(getOCPerKubeConf(oc, guestClusterKubeconfig), "app=p48917-ss", "test-sso-48916", "Running")

		g.By("Validate that pod has been created via secondary-scheduler")
		ssPodOutput, err := getOCPerKubeConf(oc, guestClusterKubeconfig).AsAdmin().WithoutNamespace().Run("get").Args("pod", "p48917-ss", "-n", "test-sso-48916", "-o=jsonpath={.spec.schedulerName}").Output()
		e2e.Logf("ssPodOutput is:\n%s", ssPodOutput)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(ssPodOutput, "secondary-scheduler")).To(o.BeTrue())

		g.By("Create pod so that it is scheduled via default-scheduler")
		defaultSchedPod.createPodWithScheduler(getOCPerKubeConf(oc, guestClusterKubeconfig))
		o.Expect(err).NotTo(o.HaveOccurred())

		// Check pod status
		checkPodStatus(getOCPerKubeConf(oc, guestClusterKubeconfig), "app=p48917-ds", "test-sso-48916", "Running")

		g.By("Validate that pod has been created via default-scheduler")
		dsPodOutput, err := getOCPerKubeConf(oc, guestClusterKubeconfig).AsAdmin().WithoutNamespace().Run("get").Args("pod", "p48917-ds", "-n", "test-sso-48916", "-o=jsonpath={.spec.schedulerName}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(dsPodOutput, "default-scheduler")).To(o.BeTrue())

		g.By("Create pod so that it is scheduled via default-scheduler")
		podWithOutSched.createPodWithOutScheduler(getOCPerKubeConf(oc, guestClusterKubeconfig))
		o.Expect(err).NotTo(o.HaveOccurred())

		// Check pod status
		checkPodStatus(getOCPerKubeConf(oc, guestClusterKubeconfig), "app=p48917-wos", "test-sso-48916", "Running")

		g.By("Validate that pod has been created via default-scheduler")
		podWosOutput, err := getOCPerKubeConf(oc, guestClusterKubeconfig).AsAdmin().WithoutNamespace().Run("get").Args("pod", "p48917-wos", "-n", "test-sso-48916", "-o=jsonpath={.spec.schedulerName}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(podWosOutput, "default-scheduler")).To(o.BeTrue())
	})

})
