package workloads

import (
	"fmt"
	"github.com/openshift/openshift-tests-private/test/extended/util/architecture"
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
		oc                       = exutil.NewCLI("default-"+getRandomString(), exutil.KubeConfigPath())
		kubeNamespace            = "openshift-secondary-scheduler-operator"
		buildPruningBaseDir      string
		ssoOperatorGroupT        string
		ssoSubscriptionT         string
		secondarySchedulerT      string
		secondarySchedulerConfig string
		sub                      ssoSubscription
		og                       ssoOperatorgroup
		secschu                  secondaryScheduler
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
			startingCSV: "secondaryscheduleroperator.v1.1.1",
			template:    ssoSubscriptionT,
		}

		og = ssoOperatorgroup{
			name:      "openshift-secondary-scheduler-operator",
			namespace: kubeNamespace,
			template:  ssoOperatorGroupT,
		}

		// Get secheduler Image
		ssImage := getSchedulerImage(oc)

		secschu = secondaryScheduler{
			namespace:        kubeNamespace,
			schedulerImage:   ssImage,
			logLevel:         "Normal",
			operatorLogLevel: "Normal",
			schedulerConfig:  "secondary-scheduler-config",
			template:         secondarySchedulerT,
		}

		// Skip case on arm64 cluster
		architecture.SkipNonAmd64SingleArch(oc)

		// Skip case on multi-arch cluster
		architecture.SkipArchitectures(oc, architecture.MULTI)

		// Skip the test if no qe-app-registry catalog is present
		skipMissingCatalogsource(oc)

	})

	// author: knarra@redhat.com
	g.It("NonHyperShiftHOST-ROSA-OSD_CCS-ARO-Author:knarra-Critical-48916-Critical-48917-Install seconday scheduler operator via a deployment & verify user is able to schedule pod using secondary-scheduler [Serial][Flaky]", func() {
		secondarySchedPodT := filepath.Join(buildPruningBaseDir, "deployPodWithScheduler.yaml")
		schedPodWithOutSchedT := filepath.Join(buildPruningBaseDir, "deployPodWithOutScheduler.yaml")

		g.By("Create the secondary-scheduler namespace")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("ns", kubeNamespace).Execute()
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", kubeNamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create the operatorgroup")
		defer og.deleteOperatorGroup(oc)
		og.createOperatorGroup(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create the subscription")
		defer sub.deleteSubscription(oc)
		sub.createSubscription(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Wait for the secondary scheduler operator pod running")
		if ok := waitForAvailableRsRunning(oc, "deploy", "secondary-scheduler-operator", kubeNamespace, "1"); ok {
			e2e.Logf("SecondarySchedulerOperator operator runnnig now\n")
		}

		// Create secondary scheduler configmap
		g.By("create secondary scheduler configmap")
		createConfigErr := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", secondarySchedulerConfig).Execute()
		o.Expect(createConfigErr).NotTo(o.HaveOccurred())

		g.By("Create secondary scheduler cluster")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("SecondaryScheduler", "--all", "-n", kubeNamespace).Execute()
		secschu.createSecondaryScheduler(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		err = wait.Poll(5*time.Second, 180*time.Second, func() (bool, error) {
			output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", "secondary-scheduler", "-n", kubeNamespace, "-o=jsonpath={.status.observedGeneration}").Output()
			if err != nil {
				e2e.Logf("deploy is still inprogress, error: %s. Trying again", err)
				return false, nil
			}
			if matched, _ := regexp.MatchString("2", output); matched {
				e2e.Logf("deploy is up:\n%s", output)
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("observed Generation is not expected"))
		g.By("Check the secondary scheduler run well")
		checkAvailable(oc, "deploy", "secondary-scheduler", kubeNamespace, "1")
		g.By("Validate that right version of secondary-scheduler is running")
		ssCsvOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", "-n", kubeNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(ssCsvOutput, "secondaryscheduleroperator.v1.1.1")).To(o.BeTrue())

		// Create test project
		g.By("Create test project")
		oc.SetupProject()

		secondarySchedPod := deployPodWithScheduler{
			pName:         "p48917-ss",
			namespace:     oc.Namespace(),
			schedulerName: "secondary-scheduler",
			template:      secondarySchedPodT,
		}

		defaultSchedPod := deployPodWithScheduler{
			pName:         "p48917-ds",
			namespace:     oc.Namespace(),
			schedulerName: "default-scheduler",
			template:      secondarySchedPodT,
		}

		podWithOutSched := deployPodWithOutScheduler{
			pName:     "p48917-wos",
			namespace: oc.Namespace(),
			template:  schedPodWithOutSchedT,
		}

		// Create and validate pods to make sure they are created by specified schedulers

		g.By("Create pod so that it is scheduled via secondary-scheduler")
		secondarySchedPod.createPodWithScheduler(oc)

		// Check pod status
		checkPodStatus(oc, "app=p48917-ss", oc.Namespace(), "Running")

		g.By("Validate that pod has been created via secondary-scheduler")
		ssPodOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "p48917-ss", "-n", oc.Namespace(), "-o=jsonpath={.spec.schedulerName}").Output()
		e2e.Logf("ssPodOutput is:\n%s", ssPodOutput)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(ssPodOutput, "secondary-scheduler")).To(o.BeTrue())

		g.By("Create pod so that it is scheduled via default-scheduler")
		defaultSchedPod.createPodWithScheduler(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		// Check pod status
		checkPodStatus(oc, "app=p48917-ds", oc.Namespace(), "Running")

		g.By("Validate that pod has been created via default-scheduler")
		dsPodOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "p48917-ds", "-n", oc.Namespace(), "-o=jsonpath={.spec.schedulerName}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(dsPodOutput, "default-scheduler")).To(o.BeTrue())

		g.By("Create pod so that it is scheduled via default-scheduler")
		podWithOutSched.createPodWithOutScheduler(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		// Check pod status
		checkPodStatus(oc, "app=p48917-wos", oc.Namespace(), "Running")

		g.By("Validate that pod has been created via default-scheduler")
		podWosOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "p48917-wos", "-n", oc.Namespace(), "-o=jsonpath={.spec.schedulerName}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(podWosOutput, "default-scheduler")).To(o.BeTrue())
	})

})
