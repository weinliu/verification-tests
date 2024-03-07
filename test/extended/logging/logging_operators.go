package logging

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-openshift-logging] Logging NonPreRelease cluster-logging-operator should", func() {
	defer g.GinkgoRecover()
	var (
		oc             = exutil.NewCLI("logging-clo", exutil.KubeConfigPath())
		loggingBaseDir string
	)

	g.BeforeEach(func() {
		loggingBaseDir = exutil.FixturePath("testdata", "logging")
		g.By("deploy CLO and EO")
		CLO := SubscriptionObjects{
			OperatorName:  "cluster-logging-operator",
			Namespace:     cloNS,
			PackageName:   "cluster-logging",
			Subscription:  filepath.Join(loggingBaseDir, "subscription", "sub-template.yaml"),
			OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "allnamespace-og.yaml"),
		}
		eoSource := CatalogSourceObjects{
			Channel: "stable",
		}
		EO := SubscriptionObjects{
			OperatorName:  "elasticsearch-operator",
			Namespace:     eoNS,
			PackageName:   "elasticsearch-operator",
			Subscription:  filepath.Join(loggingBaseDir, "subscription", "sub-template.yaml"),
			OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "allnamespace-og.yaml"),
			CatalogSource: eoSource,
		}
		CLO.SubscribeOperator(oc)
		EO.SubscribeOperator(oc)
	})

	// author qitang@redhat.com
	g.It("CPaasrunOnly-Author:qitang-Medium-42405-No configurations when forward to external ES with only username or password set in pipeline secret[Serial]", func() {
		if isFipsEnabled(oc) {
			g.Skip("skip fluentd test on fips enabled cluster for LOG-3933")
		}
		oc.SetupProject()
		esProj := oc.Namespace()
		ees := externalES{
			namespace:  esProj,
			version:    "7",
			serverName: "elasticsearch-server",
			httpSSL:    true,
			clientAuth: true,
			userAuth:   true,
			username:   "test",
			password:   getRandomString(),
			secretName: "external-es-42405",
			loggingNS:  loggingNS,
		}
		defer ees.remove(oc)
		ees.deploy(oc)
		eesURL := "https://" + ees.serverName + "." + ees.namespace + ".svc:9200"

		g.By("create secret in openshift-logging namespace")
		s := resource{"secret", "pipelinesecret", loggingNS}
		defer s.clear(oc)
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args(s.kind, "-n", s.namespace, "generic", s.name, "--from-literal=username=test").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("create CLF")
		clf := clusterlogforwarder{
			name:         "instance",
			namespace:    loggingNS,
			templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-external-es-pipelinesecret.yaml"),
			secretName:   s.name,
		}
		defer clf.delete(oc)
		clf.create(oc, "ES_URL="+eesURL)

		g.By("deploy collector pods")
		cl := clusterlogging{
			name:          "instance",
			namespace:     loggingNS,
			collectorType: "fluentd",
			templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "collector_only.yaml"),
		}
		defer cl.delete(oc)
		cl.create(oc)
		checkResource(oc, true, false, "cannot have username without password", []string{"clusterlogforwarder", clf.name, "-n", clf.namespace, "-ojsonpath={.status.outputs.es-created-by-user}"})
		_, err = oc.AdminKubeClient().CoreV1().ConfigMaps(cl.namespace).Get(context.Background(), "collector-config", metav1.GetOptions{})
		o.Expect(apierrors.IsNotFound(err)).Should(o.BeTrue())
	})

	// author qitang@redhat.com
	g.It("CPaasrunOnly-Author:qitang-Medium-49440-[LOG-1415] Allow users to set fluentd read_lines_limit.[Serial]", func() {
		if isFipsEnabled(oc) {
			g.Skip("skip fluentd test on fips enabled cluster for LOG-3933")
		}
		clf := clusterlogforwarder{
			name:         "instance",
			namespace:    loggingNS,
			templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "forward_to_default.yaml"),
		}
		defer clf.delete(oc)
		clf.create(oc)

		g.By("deploy EFK pods")
		cl := clusterlogging{
			name:          "instance",
			namespace:     loggingNS,
			collectorType: "fluentd",
			logStoreType:  "elasticsearch",
			esNodeCount:   1,
			templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "cl-template.yaml"),
		}
		defer cl.delete(oc)
		cl.create(oc)
		patch := "{\"spec\": {\"collection\": {\"fluentd\": {\"inFile\": {\"readLinesLimit\": 50}}}}}"
		cl.update(oc, "", patch, "--type=merge")
		WaitForECKPodsToBeReady(oc, cl.namespace)

		// extract fluent.conf from cm/collector-config
		baseDir := exutil.FixturePath("testdata", "logging")
		TestDataPath := filepath.Join(baseDir, "temp-"+getRandomString())
		defer exec.Command("rm", "-r", TestDataPath).Output()
		err := os.MkdirAll(TestDataPath, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("-n", cl.namespace, "cm/collector-config", "--confirm", "--to="+TestDataPath).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		data, _ := os.ReadFile(filepath.Join(TestDataPath, "fluent.conf"))
		o.Expect(strings.Contains(string(data), "read_lines_limit 50")).Should(o.BeTrue())
	})

	// author qitang@redhat.com
	g.It("CPaasrunOnly-Author:qitang-Medium-53221-Expose more fluentd knobs to support optimizing fluentd for different environments[Serial]", func() {
		if isFipsEnabled(oc) {
			g.Skip("skip fluentd test on fips enabled cluster for LOG-3933")
		}
		g.By("Create Cluster Logging instance")
		sc, err := getStorageClassName(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		cl := clusterlogging{
			name:             "instance",
			namespace:        loggingNS,
			collectorType:    "fluentd",
			logStoreType:     "elasticsearch",
			esNodeCount:      1,
			storageClassName: sc,
			waitForReady:     true,
			templateFile:     filepath.Join(loggingBaseDir, "clusterlogging", "cl-fluentd-buffer.yaml"),
		}
		defer cl.delete(oc)
		cl.create(oc, "TOTAL_LIMIT_SIZE=64m", "RETRY_TIMEOUT=30m", "CHUNK_LIMIT_SIZE=16m", "FLUSH_INTERVAL=5s", "FLUSH_THREAD_COUNT=3",
			"OVERFLOW_ACTION=throw_exception", "RETRY_MAX_INTERVAL=100s", "RETRY_TYPE=periodic", "RETRY_WAIT=2s")

		g.By("check configurations in fluent.conf")
		fluentConf, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("cm/collector-config", "-n", cl.namespace, `-ojsonpath={.data.fluent\.conf}`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		expectedConfigs := []string{"flush_interval 5s", "flush_thread_count 3", "retry_type periodic",
			"retry_wait 2s", "retry_max_interval 100s", "retry_timeout 30m", "total_limit_size 64m", "chunk_limit_size 16m", "overflow_action throw_exception"}
		for i := 0; i < len(expectedConfigs); i++ {
			o.Expect(strings.Contains(fluentConf, expectedConfigs[i])).Should(o.BeTrue(), fmt.Sprintf("can't find config %s in fluent.conf\n", expectedConfigs[i]))
		}

		// merge case OCP-33894 for logging 5.8 and later
		g.By("modify the optimizing variables")
		cl.update(oc, "", `{"spec": {"collection": {"fluentd": {"buffer": {"flushMode":"lazy"}}}}}`, "--type=merge")

		g.By("verify the flunentd are redeployed and the new values are set in fluentd.conf")
		WaitForDaemonsetPodsToBeReady(oc, cl.namespace, "collector")
		fluentConf, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("cm/collector-config", "-n", cl.namespace, `-ojsonpath={.data.fluent\.conf}`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(fluentConf, "flush_mode lazy")).Should(o.BeTrue(), "can't find config \"flush_mode lazy\" in fluent.conf\n")
	})

})

var _ = g.Describe("[sig-openshift-logging] Logging NonPreRelease elasticsearch-operator should", func() {
	defer g.GinkgoRecover()
	var (
		oc             = exutil.NewCLI("logging-eo", exutil.KubeConfigPath())
		loggingBaseDir string
	)

	g.BeforeEach(func() {
		loggingBaseDir = exutil.FixturePath("testdata", "logging")
		g.By("deploy CLO and EO")
		CLO := SubscriptionObjects{
			OperatorName:  "cluster-logging-operator",
			Namespace:     cloNS,
			PackageName:   "cluster-logging",
			Subscription:  filepath.Join(loggingBaseDir, "subscription", "sub-template.yaml"),
			OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "allnamespace-og.yaml"),
		}
		eoSource := CatalogSourceObjects{
			Channel: "stable",
		}
		EO := SubscriptionObjects{
			OperatorName:  "elasticsearch-operator",
			Namespace:     eoNS,
			PackageName:   "elasticsearch-operator",
			Subscription:  filepath.Join(loggingBaseDir, "subscription", "sub-template.yaml"),
			OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "allnamespace-og.yaml"),
			CatalogSource: eoSource,
		}
		CLO.SubscribeOperator(oc)
		EO.SubscribeOperator(oc)
	})

	// author qitang@redhat.com
	g.It("CPaasrunOnly-Author:qitang-Low-55198-[BZ 1942609]Should not deploy kibana pods when the kibana.replicas is set to 0.[Serial]", func() {
		g.By("deploy ECK pods")
		cl := clusterlogging{
			name:          "instance",
			namespace:     loggingNS,
			collectorType: "fluentd",
			logStoreType:  "elasticsearch",
			esNodeCount:   1,
			templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "cl-template.yaml"),
		}
		defer cl.delete(oc)
		cl.create(oc, "KIBANA_REPLICAS=0")
		g.By("waiting for collector pods to be ready...")
		WaitForDaemonsetPodsToBeReady(oc, cl.namespace, "collector")

		kibanaPods, err := oc.AdminKubeClient().CoreV1().Pods(cl.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "component=kibana"})
		o.Expect(len(kibanaPods.Items) == 0).Should(o.BeTrue())
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("scale up kibana pods")
		cl.update(oc, "", "{\"spec\": {\"visualization\": {\"kibana\": {\"replicas\": 1}}}}", "--type=merge")
		waitForPodReadyWithLabel(oc, cl.namespace, "component=kibana")
	})

})

var _ = g.Describe("[sig-openshift-logging] Logging NonPreRelease fluentd-elasticsearch upgrade testing", func() {
	defer g.GinkgoRecover()
	var (
		oc             = exutil.NewCLI("logging-upgrade", exutil.KubeConfigPath())
		loggingBaseDir string
	)

	g.BeforeEach(func() {
		if isFipsEnabled(oc) {
			g.Skip("skip fluentd test on fips enabled cluster for LOG-3933")
		}
		loggingBaseDir = exutil.FixturePath("testdata", "logging")
		CLO := SubscriptionObjects{
			OperatorName: "cluster-logging-operator",
			Namespace:    cloNS,
			PackageName:  "cluster-logging",
		}
		eoSource := CatalogSourceObjects{
			Channel: "stable",
		}
		EO := SubscriptionObjects{
			OperatorName:  "elasticsearch-operator",
			Namespace:     eoNS,
			PackageName:   "elasticsearch-operator",
			CatalogSource: eoSource,
		}
		g.By("uninstall CLO and EO")
		CLO.uninstallOperator(oc)
		EO.uninstallOperator(oc)
		resource{"operatorgroup", cloNS, cloNS}.clear(oc)
	})
	g.AfterEach(func() {
		resource{"operatorgroup", cloNS, cloNS}.clear(oc)
	})

	// author: qitang@redhat.com
	g.It("Longduration-CPaasrunOnly-Author:qitang-High-44983-Logging auto upgrade in minor version[Serial][Slow]", func() {
		var targetchannel = "stable-5.9"
		var oh OperatorHub
		g.By("check source/redhat-operators status in operatorhub")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("operatorhub/cluster", "-ojson").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		json.Unmarshal([]byte(output), &oh)
		var disabled bool
		for _, source := range oh.Status.Sources {
			if source.Name == "redhat-operators" {
				disabled = source.Disabled
				break
			}
		}
		if disabled {
			g.Skip("source/redhat-operators is disabled, skip this case.")
		}

		g.By(fmt.Sprintf("Subscribe operators to %s channel", targetchannel))
		source := CatalogSourceObjects{
			Channel:         targetchannel,
			SourceName:      "redhat-operators",
			SourceNamespace: "openshift-marketplace",
		}
		subTemplate := filepath.Join(loggingBaseDir, "subscription", "sub-template.yaml")
		preCLO := SubscriptionObjects{
			OperatorName:  "cluster-logging-operator",
			Namespace:     cloNS,
			PackageName:   "cluster-logging",
			Subscription:  subTemplate,
			OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "allnamespace-og.yaml"),
			CatalogSource: source,
		}
		eoSource := CatalogSourceObjects{
			Channel: "stable",
		}
		preEO := SubscriptionObjects{
			OperatorName:  "elasticsearch-operator",
			Namespace:     eoNS,
			PackageName:   "elasticsearch-operator",
			Subscription:  subTemplate,
			OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "allnamespace-og.yaml"),
			CatalogSource: eoSource,
		}
		defer preCLO.uninstallOperator(oc)
		preCLO.SubscribeOperator(oc)
		defer preEO.uninstallOperator(oc)
		preEO.SubscribeOperator(oc)

		g.By("Deploy clusterlogging")
		sc, err := getStorageClassName(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		cl := clusterlogging{
			name:             "instance",
			namespace:        loggingNS,
			collectorType:    "fluentd",
			logStoreType:     "elasticsearch",
			storageClassName: sc,
			esNodeCount:      3,
			//waitForReady:     true,
			templateFile: filepath.Join(loggingBaseDir, "clusterlogging", "cl-storage-template.yaml"),
		}
		defer cl.delete(oc)
		cl.create(oc, "REDUNDANCY_POLICY=SingleRedundancy")

		// since logging 5.8 is not released, in catsrc/redhat-operators, stable channel is pointed to logging 5.7
		// in logging 5.7, there is only one ds, using ls.waitForLokiStackToBeReady(oc) will always fail.
		var esDeployNames []string
		err = wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 2*time.Minute, true, func(context.Context) (bool, error) {
			esDeployNames = getDeploymentsNameByLabel(oc, cl.namespace, "cluster-name=elasticsearch")
			if len(esDeployNames) != cl.esNodeCount {
				e2e.Logf("expect %d ES deployments, but only find %d, try next time...", cl.esNodeCount, len(esDeployNames))
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "some ES deployments are not created")

		for _, name := range esDeployNames {
			WaitForDeploymentPodsToBeReady(oc, cl.namespace, name)
		}
		WaitForDaemonsetPodsToBeReady(oc, cl.namespace, "collector")
		WaitForDeploymentPodsToBeReady(oc, cl.namespace, "kibana")

		//get current csv version
		preCloCSV := preCLO.getInstalledCSV(oc)

		// get currentCSV in packagemanifests
		currentCloCSV := getCurrentCSVFromPackage(oc, "qe-app-registry", targetchannel, preCLO.PackageName)

		var upgraded = false
		var esPods []string
		//change source to qe-app-registry if needed, and wait for the new operators to be ready
		if preCloCSV != currentCloCSV {
			g.By(fmt.Sprintf("upgrade CLO to %s", currentCloCSV))
			err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("-n", preCLO.Namespace, "sub/"+preCLO.PackageName, "-p", "{\"spec\": {\"source\": \"qe-app-registry\"}}", "--type=merge").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			checkResource(oc, true, true, currentCloCSV, []string{"sub", preCLO.PackageName, "-n", preCLO.Namespace, "-ojsonpath={.status.currentCSV}"})
			WaitForDeploymentPodsToBeReady(oc, preCLO.Namespace, preCLO.OperatorName)
			upgraded = true
		}
		if upgraded {
			g.By("waiting for the ECK pods to be ready after upgrade")
			WaitForDaemonsetPodsToBeReady(oc, cl.namespace, "collector")
			for _, pod := range esPods {
				err := resource{"pod", pod, cl.namespace}.WaitUntilResourceIsGone(oc)
				exutil.AssertWaitPollNoErr(err, fmt.Sprintf("pod %s is not removed", pod))
			}
			WaitForECKPodsToBeReady(oc, cl.namespace)
			checkResource(oc, true, true, "green", []string{"elasticsearches.logging.openshift.io", "elasticsearch", "-n", preCLO.Namespace, "-ojsonpath={.status.cluster.status}"})
			//check PVC count, it should be equal to ES node count
			pvc, _ := oc.AdminKubeClient().CoreV1().PersistentVolumeClaims(cl.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "logging-cluster=elasticsearch"})
			o.Expect(len(pvc.Items) == 3).To(o.BeTrue())

			g.By("checking if the collector can collect logs after upgrading")
			oc.SetupProject()
			appProj := oc.Namespace()
			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			prePodList, err := oc.AdminKubeClient().CoreV1().Pods(cl.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForProjectLogsAppear(cl.namespace, prePodList.Items[0].Name, appProj, "app-00")
		}
	})

	// author: qitang@redhat.com
	g.It("Longduration-CPaasrunOnly-Author:qitang-Medium-40508-upgrade from prior version to current version[Serial][Slow]", func() {
		// to add logging 5.8, create a new catalog source with image: quay.io/openshift-qe-optional-operators/aosqe-index
		catsrcTemplate := exutil.FixturePath("testdata", "logging", "subscription", "catsrc.yaml")
		catsrc := resource{"catsrc", "logging-upgrade-" + getRandomString(), "openshift-marketplace"}
		tag, err := getIndexImageTag(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer catsrc.clear(oc)
		catsrc.applyFromTemplate(oc, "-f", catsrcTemplate, "-n", catsrc.namespace, "-p", "NAME="+catsrc.name, "-p", "IMAGE=quay.io/openshift-qe-optional-operators/aosqe-index:v"+tag)
		waitForPodReadyWithLabel(oc, catsrc.namespace, "olm.catalogSource="+catsrc.name)

		// for 5.9, only test CLO upgrade from 5.8 to 5.9
		preSource := CatalogSourceObjects{"stable-5.8", catsrc.name, catsrc.namespace}
		g.By(fmt.Sprintf("Subscribe operators to %s channel", preSource.Channel))
		subTemplate := filepath.Join(loggingBaseDir, "subscription", "sub-template.yaml")
		preCLO := SubscriptionObjects{
			OperatorName:  "cluster-logging-operator",
			Namespace:     cloNS,
			PackageName:   "cluster-logging",
			Subscription:  subTemplate,
			OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "singlenamespace-og.yaml"),
			CatalogSource: preSource,
		}
		eoSource := CatalogSourceObjects{
			Channel: "stable",
		}
		preEO := SubscriptionObjects{
			OperatorName:  "elasticsearch-operator",
			Namespace:     eoNS,
			PackageName:   "elasticsearch-operator",
			Subscription:  subTemplate,
			OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "allnamespace-og.yaml"),
			CatalogSource: eoSource,
		}
		defer preCLO.uninstallOperator(oc)
		preCLO.SubscribeOperator(oc)
		defer preEO.uninstallOperator(oc)
		preEO.SubscribeOperator(oc)

		g.By("Deploy clusterlogging")
		sc, err := getStorageClassName(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		cl := clusterlogging{
			name:             "instance",
			namespace:        loggingNS,
			collectorType:    "fluentd",
			logStoreType:     "elasticsearch",
			storageClassName: sc,
			esNodeCount:      3,
			templateFile:     filepath.Join(loggingBaseDir, "clusterlogging", "cl-storage-template.yaml"),
		}
		defer cl.delete(oc)
		cl.create(oc, "REDUNDANCY_POLICY=SingleRedundancy")
		var esDeployNames []string
		err = wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 2*time.Minute, true, func(context.Context) (bool, error) {
			esDeployNames = getDeploymentsNameByLabel(oc, cl.namespace, "cluster-name=elasticsearch")
			if len(esDeployNames) != cl.esNodeCount {
				e2e.Logf("expect %d ES deployments, but only find %d, try next time...", cl.esNodeCount, len(esDeployNames))
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "some ES deployments are not created")
		for _, name := range esDeployNames {
			WaitForDeploymentPodsToBeReady(oc, cl.namespace, name)
		}
		WaitForDaemonsetPodsToBeReady(oc, cl.namespace, "collector")
		WaitForDeploymentPodsToBeReady(oc, cl.namespace, "kibana")
		esPods, err := getPodNames(oc, cl.namespace, "component=elasticsearch")
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Before upgrade, ES pods are: %v", esPods)

		//change channel, and wait for the new operators to be ready
		var source = CatalogSourceObjects{"stable-5.9", "qe-app-registry", "openshift-marketplace"}
		//change channel, and wait for the new operators to be ready
		version := strings.Split(source.Channel, "-")[1]
		g.By(fmt.Sprintf("upgrade CLO to %s", source.Channel))
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("-n", preCLO.Namespace, "sub/"+preCLO.PackageName, "-p", "{\"spec\": {\"channel\": \""+source.Channel+"\", \"source\": \""+source.SourceName+"\", \"sourceNamespace\": \""+source.SourceNamespace+"\"}}", "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		checkResource(oc, true, false, version, []string{"sub", preCLO.PackageName, "-n", preCLO.Namespace, "-ojsonpath={.status.currentCSV}"})
		cloCurrentCSV, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", "-n", preCLO.Namespace, preCLO.PackageName, "-ojsonpath={.status.currentCSV}").Output()
		resource{"csv", cloCurrentCSV, preCLO.Namespace}.WaitForResourceToAppear(oc)
		checkResource(oc, true, true, "Succeeded", []string{"csv", cloCurrentCSV, "-n", preCLO.Namespace, "-ojsonpath={.status.phase}"})
		WaitForDeploymentPodsToBeReady(oc, preCLO.Namespace, preCLO.OperatorName)

		g.By("waiting for the ECK pods to be ready after upgrade")
		WaitForDaemonsetPodsToBeReady(oc, cl.namespace, "collector")
		for _, pod := range esPods {
			err := resource{"pod", pod, cl.namespace}.WaitUntilResourceIsGone(oc)
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("pod %s is not removed", pod))
		}
		WaitForECKPodsToBeReady(oc, cl.namespace)
		checkResource(oc, true, true, "green", []string{"elasticsearches.logging.openshift.io", "elasticsearch", "-n", preCLO.Namespace, "-ojsonpath={.status.cluster.status}"})

		g.By("checking if the collector can collect logs after upgrading")
		oc.SetupProject()
		appProj := oc.Namespace()
		jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
		err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		prePodList, err := oc.AdminKubeClient().CoreV1().Pods(cl.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "es-node-master=true"})
		o.Expect(err).NotTo(o.HaveOccurred())
		waitForProjectLogsAppear(cl.namespace, prePodList.Items[0].Name, appProj, "app-00")
	})
})

var _ = g.Describe("[sig-openshift-logging] Logging NonPreRelease vector-loki upgrade testing", func() {
	defer g.GinkgoRecover()
	var (
		oc             = exutil.NewCLI("logging-loki-upgrade", exutil.KubeConfigPath())
		loggingBaseDir string
	)

	g.BeforeEach(func() {
		if len(getStorageType(oc)) == 0 {
			g.Skip("Current cluster doesn't have a proper object storage for this test!")
		}
		if !validateInfraForLoki(oc) {
			g.Skip("Current platform not supported!")
		}
		loggingBaseDir = exutil.FixturePath("testdata", "logging")
		clo := SubscriptionObjects{
			OperatorName: "cluster-logging-operator",
			Namespace:    cloNS,
			PackageName:  "cluster-logging",
		}
		lo := SubscriptionObjects{
			OperatorName: "loki-operator-controller-manager",
			Namespace:    "openshift-operators-redhat",
			PackageName:  "loki-operator",
		}
		g.By("uninstall CLO and LO")
		clo.uninstallOperator(oc)
		lo.uninstallOperator(oc)
		for _, crd := range []string{"alertingrules.loki.grafana.com", "lokistacks.loki.grafana.com", "recordingrules.loki.grafana.com", "rulerconfigs.loki.grafana.com"} {
			_ = oc.AsAdmin().WithoutNamespace().Run("delete").Args("crd", crd).Execute()
		}
	})
	g.AfterEach(func() {
		for _, crd := range []string{"alertingrules.loki.grafana.com", "lokistacks.loki.grafana.com", "recordingrules.loki.grafana.com", "rulerconfigs.loki.grafana.com"} {
			_ = oc.AsAdmin().WithoutNamespace().Run("delete").Args("crd", crd).Execute()
		}
	})

	// author qitang@redhat.com
	g.It("Longduration-CPaasrunOnly-Author:qitang-Critical-53407-Cluster Logging upgrade with Vector as collector - minor version.[Serial][Slow]", func() {
		g.Skip("Skip for logging 5.9 is not released!")
		var targetchannel = "stable"
		var oh OperatorHub
		g.By("check source/redhat-operators status in operatorhub")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("operatorhub/cluster", "-ojson").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		json.Unmarshal([]byte(output), &oh)
		var disabled bool
		for _, source := range oh.Status.Sources {
			if source.Name == "redhat-operators" {
				disabled = source.Disabled
				break
			}
		}
		if disabled {
			g.Skip("source/redhat-operators is disabled, skip this case.")
		}
		g.By(fmt.Sprintf("Subscribe operators to %s channel", targetchannel))
		source := CatalogSourceObjects{
			Channel:         targetchannel,
			SourceName:      "redhat-operators",
			SourceNamespace: "openshift-marketplace",
		}
		subTemplate := filepath.Join(loggingBaseDir, "subscription", "sub-template.yaml")
		preCLO := SubscriptionObjects{
			OperatorName:  "cluster-logging-operator",
			Namespace:     cloNS,
			PackageName:   "cluster-logging",
			Subscription:  subTemplate,
			OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "allnamespace-og.yaml"),
			CatalogSource: source,
		}
		preLO := SubscriptionObjects{
			OperatorName:  "loki-operator-controller-manager",
			Namespace:     loNS,
			PackageName:   "loki-operator",
			Subscription:  subTemplate,
			OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "allnamespace-og.yaml"),
			CatalogSource: source,
		}
		defer preCLO.uninstallOperator(oc)
		preCLO.SubscribeOperator(oc)
		defer preLO.uninstallOperator(oc)
		preLO.SubscribeOperator(oc)

		g.By("Deploy lokistack")
		sc, err := getStorageClassName(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		ls := lokiStack{
			name:          "loki-53407",
			namespace:     loggingNS,
			tSize:         "1x.demo",
			storageType:   getStorageType(oc),
			storageSecret: "storage-secret-53407",
			storageClass:  sc,
			bucketName:    "logging-loki-53407-" + getInfrastructureName(oc),
			template:      filepath.Join(loggingBaseDir, "lokistack", "lokistack-simple.yaml"),
		}
		defer ls.removeObjectStorage(oc)
		err = ls.prepareResourcesForLokiStack(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer ls.removeLokiStack(oc)
		err = ls.deployLokiStack(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		ls.waitForLokiStackToBeReady(oc)

		g.By("Deploy clusterlogging")
		cl := clusterlogging{
			name:          "instance",
			namespace:     loggingNS,
			collectorType: "vector",
			logStoreType:  "lokistack",
			lokistackName: ls.name,
			waitForReady:  true,
			templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "cl-default-loki.yaml"),
		}
		defer cl.delete(oc)
		cl.create(oc)

		//get current csv version
		preCloCSV := preCLO.getInstalledCSV(oc)
		preLoCSV := preLO.getInstalledCSV(oc)

		// get currentCSV in packagemanifests
		currentCloCSV := getCurrentCSVFromPackage(oc, "qe-app-registry", targetchannel, preCLO.PackageName)
		currentLoCSV := getCurrentCSVFromPackage(oc, "qe-app-registry", targetchannel, preLO.PackageName)
		var upgraded = false
		//change source to qe-app-registry if needed, and wait for the new operators to be ready
		if preCloCSV != currentCloCSV {
			g.By(fmt.Sprintf("upgrade CLO to %s", currentCloCSV))
			err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("-n", preCLO.Namespace, "sub/"+preCLO.PackageName, "-p", "{\"spec\": {\"source\": \"qe-app-registry\"}}", "--type=merge").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			checkResource(oc, true, true, currentCloCSV, []string{"sub", preCLO.PackageName, "-n", preCLO.Namespace, "-ojsonpath={.status.currentCSV}"})
			WaitForDeploymentPodsToBeReady(oc, preCLO.Namespace, preCLO.OperatorName)
			upgraded = true
		}
		if preLoCSV != currentLoCSV {
			g.By(fmt.Sprintf("upgrade LO to %s", currentLoCSV))
			err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("-n", preLO.Namespace, "sub/"+preLO.PackageName, "-p", "{\"spec\": {\"source\": \"qe-app-registry\"}}", "--type=merge").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			checkResource(oc, true, true, currentLoCSV, []string{"sub", preLO.PackageName, "-n", preLO.Namespace, "-ojsonpath={.status.currentCSV}"})
			WaitForDeploymentPodsToBeReady(oc, preLO.Namespace, preLO.OperatorName)
			upgraded = true
		}

		if upgraded {
			g.By("waiting for the Loki and Vector pods to be ready after upgrade")
			ls.waitForLokiStackToBeReady(oc)
			cl.waitForLoggingReady(oc)
			// In upgrade testing, sometimes a pod may not be ready but the deployment/statefulset might be ready
			// here add a step to check the pods' status
			waitForPodReadyWithLabel(oc, ls.namespace, "app.kubernetes.io/instance="+ls.name)

			g.By("checking if the collector can collect logs after upgrading")
			oc.SetupProject()
			appProj := oc.Namespace()
			defer removeClusterRoleFromServiceAccount(oc, appProj, "default", "cluster-admin")
			addClusterRoleToServiceAccount(oc, appProj, "default", "cluster-admin")
			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			bearerToken := getSAToken(oc, "default", appProj)
			route := "https://" + getRouteAddress(oc, ls.namespace, ls.name)
			lc := newLokiClient(route).withToken(bearerToken).retry(5)
			err = wait.PollUntilContextTimeout(context.Background(), 30*time.Second, 180*time.Second, true, func(context.Context) (done bool, err error) {
				res, err := lc.searchByNamespace("application", appProj)
				if err != nil {
					e2e.Logf("\ngot err when getting application logs: %v, continue\n", err)
					return false, nil
				}
				if len(res.Data.Result) > 0 {
					return true, nil
				}
				e2e.Logf("\n len(res.Data.Result) not > 0, continue\n")
				return false, nil
			})
			exutil.AssertWaitPollNoErr(err, "application logs are not found")
		}
	})

	// author: qitang@redhat.com
	g.It("Longduration-CPaasrunOnly-Author:qitang-Critical-53404-Cluster Logging upgrade with Vector as collector - major version.[Serial][Slow]", func() {
		// to add logging 5.8, create a new catalog source with image: quay.io/openshift-qe-optional-operators/aosqe-index
		catsrcTemplate := exutil.FixturePath("testdata", "logging", "subscription", "catsrc.yaml")
		catsrc := resource{"catsrc", "logging-upgrade-" + getRandomString(), "openshift-marketplace"}
		tag, err := getIndexImageTag(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer catsrc.clear(oc)
		catsrc.applyFromTemplate(oc, "-f", catsrcTemplate, "-n", catsrc.namespace, "-p", "NAME="+catsrc.name, "-p", "IMAGE=quay.io/openshift-qe-optional-operators/aosqe-index:v"+tag)
		waitForPodReadyWithLabel(oc, catsrc.namespace, "olm.catalogSource="+catsrc.name)

		// for 5.9, test upgrade from 5.8 to 5.9
		preSource := CatalogSourceObjects{"stable-5.8", catsrc.name, catsrc.namespace}
		g.By(fmt.Sprintf("Subscribe operators to %s channel", preSource.Channel))
		subTemplate := filepath.Join(loggingBaseDir, "subscription", "sub-template.yaml")
		preCLO := SubscriptionObjects{
			OperatorName:  "cluster-logging-operator",
			Namespace:     cloNS,
			PackageName:   "cluster-logging",
			Subscription:  subTemplate,
			OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "singlenamespace-og.yaml"),
			CatalogSource: preSource,
		}
		preLO := SubscriptionObjects{
			OperatorName:  "loki-operator-controller-manager",
			Namespace:     loNS,
			PackageName:   "loki-operator",
			Subscription:  subTemplate,
			OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "allnamespace-og.yaml"),
			CatalogSource: preSource,
		}
		defer preCLO.uninstallOperator(oc)
		preCLO.SubscribeOperator(oc)
		defer preLO.uninstallOperator(oc)
		preLO.SubscribeOperator(oc)

		g.By("Deploy lokistack")
		sc, err := getStorageClassName(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		ls := lokiStack{
			name:          "loki-53404",
			namespace:     loggingNS,
			tSize:         "1x.demo",
			storageType:   getStorageType(oc),
			storageSecret: "storage-secret-53404",
			storageClass:  sc,
			bucketName:    "logging-loki-53404-" + getInfrastructureName(oc),
			template:      filepath.Join(loggingBaseDir, "lokistack", "lokistack-simple.yaml"),
		}
		defer ls.removeObjectStorage(oc)
		err = ls.prepareResourcesForLokiStack(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer ls.removeLokiStack(oc)
		err = ls.deployLokiStack(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		ls.waitForLokiStackToBeReady(oc)

		g.By("Deploy clusterlogging")
		cl := clusterlogging{
			name:          "instance",
			namespace:     loggingNS,
			collectorType: "vector",
			logStoreType:  "lokistack",
			lokistackName: ls.name,
			templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "cl-default-loki.yaml"),
		}
		defer cl.delete(oc)
		cl.create(oc)
		WaitForDaemonsetPodsToBeReady(oc, cl.namespace, "collector")
		WaitForDeploymentPodsToBeReady(oc, cl.namespace, "logging-view-plugin")

		//change channel, and wait for the new operators to be ready
		var source = CatalogSourceObjects{"stable-5.9", "qe-app-registry", "openshift-marketplace"}
		//change channel, and wait for the new operators to be ready
		version := strings.Split(source.Channel, "-")[1]
		g.By(fmt.Sprintf("upgrade CLO&LO to %s", source.Channel))
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("-n", preCLO.Namespace, "sub/"+preCLO.PackageName, "-p", "{\"spec\": {\"channel\": \""+source.Channel+"\", \"source\": \""+source.SourceName+"\", \"sourceNamespace\": \""+source.SourceNamespace+"\"}}", "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("-n", preLO.Namespace, "sub/"+preLO.PackageName, "-p", "{\"spec\": {\"channel\": \""+source.Channel+"\", \"source\": \""+source.SourceName+"\", \"sourceNamespace\": \""+source.SourceNamespace+"\"}}", "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		checkResource(oc, true, false, version, []string{"sub", preCLO.PackageName, "-n", preCLO.Namespace, "-ojsonpath={.status.currentCSV}"})
		cloCurrentCSV, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", "-n", preCLO.Namespace, preCLO.PackageName, "-ojsonpath={.status.currentCSV}").Output()
		resource{"csv", cloCurrentCSV, preCLO.Namespace}.WaitForResourceToAppear(oc)
		checkResource(oc, true, true, "Succeeded", []string{"csv", cloCurrentCSV, "-n", preCLO.Namespace, "-ojsonpath={.status.phase}"})
		WaitForDeploymentPodsToBeReady(oc, preCLO.Namespace, preCLO.OperatorName)

		checkResource(oc, true, false, version, []string{"sub", preLO.PackageName, "-n", preLO.Namespace, "-ojsonpath={.status.currentCSV}"})
		loCurrentCSV, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", "-n", preLO.Namespace, preLO.PackageName, "-ojsonpath={.status.currentCSV}").Output()
		resource{"csv", loCurrentCSV, preLO.Namespace}.WaitForResourceToAppear(oc)
		checkResource(oc, true, true, "Succeeded", []string{"csv", loCurrentCSV, "-n", preLO.Namespace, "-ojsonpath={.status.phase}"})
		WaitForDeploymentPodsToBeReady(oc, preLO.Namespace, preLO.OperatorName)

		ls.waitForLokiStackToBeReady(oc)
		cl.waitForLoggingReady(oc)
		// In upgrade testing, sometimes a pod may not be ready but the deployment/statefulset might be ready
		// here add a step to check the pods' status
		waitForPodReadyWithLabel(oc, ls.namespace, "app.kubernetes.io/instance="+ls.name)

		g.By("checking if the collector can collect logs after upgrading")
		oc.SetupProject()
		appProj := oc.Namespace()
		defer removeClusterRoleFromServiceAccount(oc, appProj, "default", "cluster-admin")
		addClusterRoleToServiceAccount(oc, appProj, "default", "cluster-admin")
		jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
		err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		bearerToken := getSAToken(oc, "default", appProj)
		route := "https://" + getRouteAddress(oc, ls.namespace, ls.name)
		lc := newLokiClient(route).withToken(bearerToken).retry(5)
		err = wait.PollUntilContextTimeout(context.Background(), 30*time.Second, 180*time.Second, true, func(context.Context) (done bool, err error) {
			res, err := lc.searchByNamespace("application", appProj)
			if err != nil {
				e2e.Logf("\ngot err when getting application logs: %v, continue\n", err)
				return false, nil
			}
			if len(res.Data.Result) > 0 {
				return true, nil
			}
			e2e.Logf("\n len(res.Data.Result) not > 0, continue\n")
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "application logs are not found")

		g.By("checking if regular user can view his logs after upgrading")
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "add-role-to-user", "cluster-logging-application-view", oc.Username(), "-n", appProj).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		userToken, err := oc.Run("whoami").Args("-t").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		lc0 := newLokiClient(route).withToken(userToken).retry(5)
		err = wait.PollUntilContextTimeout(context.Background(), 30*time.Second, 180*time.Second, true, func(context.Context) (done bool, err error) {
			res, err := lc0.searchByNamespace("application", appProj)
			if err != nil {
				e2e.Logf("\ngot err when getting application logs: %v, continue\n", err)
				return false, nil
			}
			if len(res.Data.Result) > 0 {
				return true, nil
			}
			e2e.Logf("\n len(res.Data.Result) not > 0, continue\n")
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "can't get application logs with normal user")
	})
})

var _ = g.Describe("[sig-openshift-logging] Logging NonPreRelease operator deployments", func() {
	defer g.GinkgoRecover()
	var (
		oc             = exutil.NewCLI("logging-operators", exutil.KubeConfigPath())
		loggingBaseDir string
	)

	g.BeforeEach(func() {
		loggingBaseDir = exutil.FixturePath("testdata", "logging")
	})

	g.It("CPaasrunOnly-Author:anli-Low-65518-deploy cluster-logging-operator after datadog-agent is deployed [Disruptive]", func() {
		oc.SetupProject()
		datadogNS := oc.Namespace()
		subTemplate := filepath.Join(loggingBaseDir, "subscription", "sub-template.yaml")
		ogPath := filepath.Join(loggingBaseDir, "subscription", "allnamespace-og.yaml")
		podLabel := "app.kubernetes.io/name=datadog-operator"

		g.By("Make the datadog operator ready")
		sourceCert := CatalogSourceObjects{
			Channel:         "stable",
			SourceName:      "certified-operators",
			SourceNamespace: "openshift-marketplace",
		}
		subDog := SubscriptionObjects{
			OperatorName:       "datadog-operator-certified",
			PackageName:        "datadog-operator-certified",
			Namespace:          datadogNS,
			Subscription:       subTemplate,
			OperatorPodLabel:   podLabel,
			OperatorGroup:      ogPath,
			CatalogSource:      sourceCert,
			SkipCaseWhenFailed: true,
		}

		subDog.SubscribeOperator(oc)

		g.By("Delete cluster-logging operator if exist")
		sourceQE := CatalogSourceObjects{
			Channel:         "stable",
			SourceName:      "qe-app-registry",
			SourceNamespace: "openshift-marketplace",
		}
		subCLO := SubscriptionObjects{
			OperatorName:  "cluster-logging-operator",
			Namespace:     "openshift-logging",
			PackageName:   "cluster-logging",
			Subscription:  subTemplate,
			OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "allnamespace-og.yaml"),
			CatalogSource: sourceQE,
		}
		subCLO.uninstallOperator(oc)

		g.By("deploy cluster-logging operator")
		subCLO.SubscribeOperator(oc)
	})
})

var _ = g.Describe("[sig-openshift-logging] Logging NonPreRelease multi-mode testing", func() {
	defer g.GinkgoRecover()
	var (
		oc             = exutil.NewCLI("logging-multiple-mode", exutil.KubeConfigPath())
		loggingBaseDir string
	)

	g.BeforeEach(func() {
		loggingBaseDir = exutil.FixturePath("testdata", "logging")
		subTemplate := filepath.Join(loggingBaseDir, "subscription", "sub-template.yaml")
		CLO := SubscriptionObjects{
			OperatorName:  "cluster-logging-operator",
			Namespace:     cloNS,
			PackageName:   "cluster-logging",
			Subscription:  subTemplate,
			OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "allnamespace-og.yaml"),
		}
		LO := SubscriptionObjects{
			OperatorName:  "loki-operator-controller-manager",
			Namespace:     loNS,
			PackageName:   "loki-operator",
			Subscription:  subTemplate,
			OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "allnamespace-og.yaml"),
		}
		g.By("deploy CLO and LO")
		CLO.SubscribeOperator(oc)
		LO.SubscribeOperator(oc)
		oc.SetupProject()
	})

	// author: qitang@redhat.com
	g.It("CPaasrunOnly-Author:qitang-Medium-64147-Deploy Logfilesmetricexporter as an independent pod.[Serial]", func() {
		template := filepath.Join(loggingBaseDir, "logfilemetricexporter", "lfme.yaml")
		lfme := logFileMetricExporter{
			name:          "instance",
			namespace:     loggingNS,
			template:      template,
			waitPodsReady: true,
		}
		defer lfme.delete(oc)
		lfme.create(oc)
		token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")

		g.By("check metrics exposed by logfilemetricexporter")
		checkMetric(oc, token, "{job=\"logfilesmetricexporter\"}", 5)

		sc, err := getStorageClassName(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Deploying LokiStack CR")
		ls := lokiStack{
			name:          "loki-64147",
			namespace:     loggingNS,
			tSize:         "1x.demo",
			storageType:   getStorageType(oc),
			storageSecret: "storage-64147",
			storageClass:  sc,
			bucketName:    "logging-loki-64147-" + getInfrastructureName(oc),
			template:      filepath.Join(loggingBaseDir, "lokistack", "lokistack-simple.yaml"),
		}
		defer ls.removeObjectStorage(oc)
		err = ls.prepareResourcesForLokiStack(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer ls.removeLokiStack(oc)
		err = ls.deployLokiStack(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		ls.waitForLokiStackToBeReady(oc)
		e2e.Logf("LokiStack deployed")

		g.By("Create ClusterLogging instance with Loki as logstore")
		cl := clusterlogging{
			name:          "instance",
			namespace:     loggingNS,
			logStoreType:  "lokistack",
			collectorType: "vector",
			templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "cl-default-loki.yaml"),
			lokistackName: ls.name,
			waitForReady:  true,
		}
		defer cl.delete(oc)
		cl.create(oc)

		g.By("Remove cluserlogging")
		cl.delete(oc)

		g.By("Check LFME pods, they should not be removed")
		WaitForDaemonsetPodsToBeReady(oc, lfme.namespace, "logfilesmetricexporter")

		g.By("Remove LFME, the pods should be removed")
		lfme.delete(oc)

		g.By("Create LFME with invalid name")
		lfmeInvalidName := resource{
			kind:      "logfilemetricexporters.logging.openshift.io",
			name:      "test-lfme-64147",
			namespace: loggingNS,
		}
		defer lfmeInvalidName.clear(oc)
		err = lfmeInvalidName.applyFromTemplate(oc, "-f", template, "-p", "NAME="+lfmeInvalidName.name, "-p", "NAMESPACE="+lfmeInvalidName.namespace)
		o.Expect(strings.Contains(err.Error(), "metadata.name: Unsupported value: \""+lfmeInvalidName.name+"\": supported values: \"instance\"")).Should(o.BeTrue())

		g.By("Create LFME with invalid namespace")
		lfmeInvalidNamespace := logFileMetricExporter{
			name:      "instance",
			namespace: oc.Namespace(),
			template:  filepath.Join(loggingBaseDir, "logfilemetricexporter", "lfme.yaml"),
		}
		defer lfmeInvalidNamespace.delete(oc)
		lfmeInvalidNamespace.create(oc)
		checkResource(oc, true, false, "validation failed: Invalid namespace name \""+lfmeInvalidNamespace.namespace+"\", instance must be in \"openshift-logging\" namespace", []string{"lfme/" + lfmeInvalidNamespace.name, "-n", lfmeInvalidNamespace.namespace, "-ojsonpath={.status.conditions[*].message}"})
	})

	// author: qitang@redhat.com
	g.It("CPaasrunOnly-Author:qitang-Medium-65023-Clusterlogging multi-mode validation.", func() {
		g.By("Deploy CLF")
		esProj := oc.Namespace()
		ees := externalES{
			namespace:  esProj,
			version:    "6",
			serverName: "elasticsearch-server",
			loggingNS:  esProj,
		}
		defer ees.remove(oc)
		ees.deploy(oc)

		clf := clusterlogforwarder{
			name:                   "clf-65023-" + getRandomString(),
			namespace:              esProj,
			serviceAccountName:     "clf-65023",
			templateFile:           filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-external-es.yaml"),
			collectApplicationLogs: true,
			waitForPodReady:        true,
		}
		defer clf.delete(oc)
		inputRefs := "[\"application\"]"
		clf.create(oc, "INPUTREFS="+inputRefs, "ES_URL=http://"+ees.serverName+"."+esProj+".svc:9200", "ES_VERSION="+ees.version)

		g.By("Deploy clusterlogging")
		cl := clusterlogging{
			name:          clf.name,
			namespace:     clf.namespace,
			collectorType: "vector",
			esNodeCount:   1,
			templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "cl-template.yaml"),
		}
		defer cl.delete(oc)
		cl.create(oc)
		checkResource(oc, true, false, "validation failed: Only spec.collection is allowed when using multiple instances of ClusterLogForwarder: "+cl.namespace+"/"+cl.name, []string{"cl/" + cl.name, "-n", cl.namespace, "-ojsonpath={.status.conditions[*].message}"})
		cl.delete(oc)

		tmpdir := "/tmp/65023-" + exutil.GetRandomString() + "/"
		defer os.RemoveAll(tmpdir)
		err := os.MkdirAll(tmpdir, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		legacyCLFile := tmpdir + "cl.yaml"
		legacyCL := fmt.Sprintf(`apiVersion: logging.openshift.io/v1
kind: ClusterLogging
metadata:
  name: %s
spec:
  collection:
    logs:
      type: vector
`, clf.name)
		pf, errp := os.Create(legacyCLFile)
		o.Expect(errp).NotTo(o.HaveOccurred())
		defer pf.Close()
		w2 := bufio.NewWriter(pf)
		_, perr := w2.WriteString(legacyCL)
		w2.Flush()
		o.Expect(perr).NotTo(o.HaveOccurred())

		err = oc.WithoutNamespace().Run("create").Args("-f", legacyCLFile, "-n", clf.namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		checkResource(oc, true, false, "spec.collection.logs.* is deprecated in favor of spec.collection.*", []string{"cl/" + clf.name, "-n", clf.namespace, "-ojsonpath={.status.conditions[*].message}"})
		cl.delete(oc)

		clInvalidCollectorType := clusterlogging{
			name:          clf.name,
			namespace:     clf.namespace,
			collectorType: "fluentd",
			esNodeCount:   1,
			templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "collector_only.yaml"),
		}
		clInvalidCollectorType.create(oc)
		checkResource(oc, true, false, "validation failed: Only vector collector impl is supported when using multiple instances of ClusterLogForwarder: "+cl.namespace+"/"+cl.name, []string{"cl/" + cl.name, "-n", cl.namespace, "-ojsonpath={.status.conditions[*].message}"})
		cl.delete(oc)

		validCL := clusterlogging{
			name:          clf.name,
			namespace:     clf.namespace,
			collectorType: "vector",
			esNodeCount:   1,
			templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "collector_only.yaml"),
		}
		validCL.create(oc)
		checkResource(oc, true, false, "True", []string{"cl/" + validCL.name, "-n", validCL.namespace, "-ojsonpath={.status.conditions[?(@.type == \"Ready\")].status}"})
	})

	// author qitang@redhat.com
	g.It("CPaasrunOnly-Author:qitang-Medium-65405-ClusterLogForwarder validation for name and namespace.", func() {
		g.By("Create Loki project and deploy Loki Server")
		lokiNS := oc.Namespace()
		loki := externalLoki{"loki-server", lokiNS}
		defer loki.remove(oc)
		loki.deployLoki(oc)
		lokiURL := "http://" + loki.name + "." + lokiNS + ".svc:3100"

		clfTemplate := filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-external-loki.yaml")
		g.By("Create ClusterLogForwarder with serviceAccountName=logcollector in openshift-logging project")
		clfInvalidSA := clusterlogforwarder{
			name:                   "invalid-sa",
			namespace:              loggingNS,
			templateFile:           clfTemplate,
			serviceAccountName:     "logcollector",
			collectApplicationLogs: true,
			waitForPodReady:        false,
		}
		defer clfInvalidSA.delete(oc)
		clfInvalidSA.create(oc, "URL="+lokiURL, "INPUTREFS=[\"application\"]")
		checkResource(oc, true, false, "logcollector is a reserved serviceaccount name for legacy ClusterLogForwarder(openshift-logging/instance)", []string{"clf/" + clfInvalidSA.name, "-n", clfInvalidSA.namespace, "-ojsonpath={.status.conditions[*].message}"})

		g.By("Create ClusterLogForwarder with name=collector in openshift-logging project")
		clfInvalidName := clusterlogforwarder{
			name:                   "collector",
			namespace:              loggingNS,
			templateFile:           clfTemplate,
			serviceAccountName:     "test-invalid-name",
			collectApplicationLogs: true,
			waitForPodReady:        false,
		}
		defer clfInvalidName.delete(oc)
		clfInvalidName.create(oc, "URL="+lokiURL, "INPUTREFS=[\"application\"]")
		checkResource(oc, true, false, "Name \"collector\" conflicts with an object for the legacy ClusterLogForwarder deployment.  Choose another", []string{"clf/" + clfInvalidName.name, "-n", clfInvalidName.namespace, "-ojsonpath={.status.conditions[*].message}"})

		exutil.By("Create ClusterLogForwarder, set it's name starts with numerical character")
		clfInvalidName2 := clusterlogforwarder{
			name:                   "65045-test-invalid-name",
			namespace:              loggingNS,
			templateFile:           clfTemplate,
			serviceAccountName:     "65045-test-invalid-name",
			collectApplicationLogs: true,
			waitForPodReady:        false,
		}
		defer clfInvalidName2.delete(oc)
		clfInvalidName2.create(oc, "URL="+lokiURL, "INPUTREFS=[\"application\"]")
		checkResource(oc, true, false, "Name \""+clfInvalidName2.name+"\" will result in an invalid object", []string{"clf/" + clfInvalidName2.name, "-n", clfInvalidName2.namespace, "-ojsonpath={.status.conditions[*].message}"})

		exutil.By("Create ClusterLogForwarder to forward logs to the external Loki instance")
		clf := clusterlogforwarder{
			name:                      "test-65405",
			namespace:                 loggingNS,
			templateFile:              clfTemplate,
			serviceAccountName:        "collector-65405",
			collectInfrastructureLogs: true,
			waitForPodReady:           true,
		}
		defer clf.delete(oc)
		inputRefs := "[\"infrastructure\"]"
		clf.create(oc, "URL="+lokiURL, "INPUTREFS="+inputRefs)
		route := "http://" + getRouteAddress(oc, loki.namespace, loki.name)
		lc := newLokiClient(route)
		g.By("Searching for Application Logs in Loki")

		err := wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 300*time.Second, true, func(context.Context) (done bool, err error) {
			infraLogs, err := lc.searchByKey("", "log_type", "infrastructure")
			if err != nil {
				return false, err
			}
			return len(infraLogs.Data.Result) > 0, nil
		})
		exutil.AssertWaitPollNoErr(err, "failed searching for infrastructure logs in Loki")
		e2e.Logf("Infrastructure Logs Query is a success")
	})

	// author qitang@redhat.com
	g.It("CPaasrunOnly-Author:qitang-Medium-65407-ClusterLogForwarder validation for the serviceaccount.[Slow]", func() {
		clfNS := oc.Namespace()
		exutil.By("Deploy ES server")
		ees := externalES{
			namespace:  clfNS,
			version:    "8",
			serverName: "elasticsearch-server",
			loggingNS:  clfNS,
		}
		defer ees.remove(oc)
		ees.deploy(oc)

		logFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
		exutil.By("create pod to generate logs")
		oc.SetupProject()
		proj := oc.Namespace()
		err := oc.WithoutNamespace().Run("new-app").Args("-n", proj, "-f", logFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create clusterlogforwarder with a non-existing serviceaccount")
		clf := clusterlogforwarder{
			name:         "collector-65407",
			namespace:    clfNS,
			templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-external-es.yaml"),
		}
		defer clf.delete(oc)
		clf.create(oc, "ES_URL=http://"+ees.serverName+"."+ees.namespace+".svc:9200", "ES_VERSION="+ees.version, "SERVICE_ACCOUNT_NAME=logcollector", "INPUTREFS=[\"application\"]")
		checkResource(oc, true, false, "service account not found: "+clf.namespace+"/logcollector", []string{"clf/" + clf.name, "-n", clf.namespace, "-ojsonpath={.status.conditions[*].message}"})

		ds := resource{
			kind:      "daemonset",
			name:      clf.name,
			namespace: clf.namespace,
		}
		dsErr := ds.WaitUntilResourceIsGone(oc)
		o.Expect(dsErr).NotTo(o.HaveOccurred())

		exutil.By("Create the serviceaccount and create rolebinding to bind clusterrole to the serviceaccount")
		sa := resource{
			kind:      "serviceaccount",
			name:      "logcollector",
			namespace: clfNS,
		}
		defer sa.clear(oc)
		err = createServiceAccount(oc, sa.namespace, sa.name)
		o.Expect(err).NotTo(o.HaveOccurred(), "get error when creating serviceaccount "+sa.name)
		defer oc.AsAdmin().WithoutNamespace().Run("policy").Args("remove-role-from-user", "collect-application-logs", fmt.Sprintf("system:serviceaccount:%s:%s", sa.namespace, sa.name), "-n", sa.namespace).Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("policy").Args("add-role-to-user", "collect-application-logs", fmt.Sprintf("system:serviceaccount:%s:%s", sa.namespace, sa.name), "-n", sa.namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		// wait for 2 minutes for CLO to update the status in CLF
		time.Sleep(2 * time.Minute)
		checkResource(oc, true, false, `insufficient permissions on service account, not authorized to collect ["application"] logs`, []string{"clf/" + clf.name, "-n", clf.namespace, "-ojsonpath={.status.conditions[*].message}"})
		dsErr = ds.WaitUntilResourceIsGone(oc)
		o.Expect(dsErr).NotTo(o.HaveOccurred())

		exutil.By("Create clusterrolebinding to bind clusterrole to the serviceaccount")
		defer removeClusterRoleFromServiceAccount(oc, sa.namespace, sa.name, "collect-application-logs")
		addClusterRoleToServiceAccount(oc, sa.namespace, sa.name, "collect-application-logs")
		// wait for 2 minutes for CLO to update the status in CLF
		time.Sleep(2 * time.Minute)
		checkResource(oc, true, false, "True", []string{"clf/" + clf.name, "-n", clf.namespace, "-ojsonpath={.status.conditions[?(@.type == \"Ready\")].status}"})
		exutil.By("Collector pods should be deployed and logs can be forwarded to external log store")
		WaitForDaemonsetPodsToBeReady(oc, clf.namespace, clf.name)
		ees.waitForIndexAppear(oc, "app")

		exutil.By("Delete the serviceaccount, the collector pods should be removed")
		err = sa.clear(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		checkResource(oc, true, false, "service account not found: "+clf.namespace+"/"+sa.name, []string{"clf/" + clf.name, "-n", clf.namespace, "-ojsonpath={.status.conditions[*].message}"})
		dsErr = ds.WaitUntilResourceIsGone(oc)
		o.Expect(dsErr).NotTo(o.HaveOccurred())

		exutil.By("Recreate the sa and add proper clusterroles to it, the collector pods should be recreated")
		err = createServiceAccount(oc, sa.namespace, sa.name)
		o.Expect(err).NotTo(o.HaveOccurred(), "get error when creating serviceaccount "+sa.name)
		addClusterRoleToServiceAccount(oc, sa.namespace, sa.name, "collect-application-logs")
		WaitForDaemonsetPodsToBeReady(oc, clf.namespace, clf.name)

		exutil.By("Remove spec.serviceAccountName from CLF")
		clf.update(oc, "", `[{"op": "remove", "path": "/spec/serviceAccountName"}]`, "--type=json")
		checkResource(oc, true, false, "custom clusterlogforwarders must specify a service account name", []string{"clf/" + clf.name, "-n", clf.namespace, "-ojsonpath={.status.conditions[*].message}"})
		dsErr = ds.WaitUntilResourceIsGone(oc)
		o.Expect(dsErr).NotTo(o.HaveOccurred())
	})

	// author qitang@redhat.com
	g.It("CPaasrunOnly-Author:qitang-Medium-65408-ClusterLogForwarder validation when roles don't match.", func() {
		clfNS := oc.Namespace()
		loki := externalLoki{"loki-server", clfNS}
		defer loki.remove(oc)
		loki.deployLoki(oc)

		exutil.By("Create ClusterLogForwarder with a serviceaccount which doesn't have proper clusterroles")
		clf := clusterlogforwarder{
			name:               "collector-65408",
			namespace:          clfNS,
			templateFile:       filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-external-loki.yaml"),
			serviceAccountName: "clf-collector",
		}
		defer clf.delete(oc)
		clf.create(oc, "URL=http://"+loki.name+"."+loki.namespace+".svc:3100")
		checkResource(oc, true, false, `insufficient permissions on service account, not authorized to collect ["application" "audit" "infrastructure"] logs`, []string{"clf/" + clf.name, "-n", clf.namespace, "-ojsonpath={.status.conditions[*].message}"})
		ds := resource{
			kind:      "daemonset",
			name:      clf.name,
			namespace: clf.namespace,
		}
		dsErr := ds.WaitUntilResourceIsGone(oc)
		o.Expect(dsErr).NotTo(o.HaveOccurred())

		exutil.By("Create a new sa and add clusterrole/collect-application-logs to the new sa, then update the CLF to use the new sa")
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("sa", "collect-application-logs", "-n", clf.namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer removeClusterRoleFromServiceAccount(oc, clf.namespace, "collect-application-logs", "collect-application-logs")
		err = addClusterRoleToServiceAccount(oc, clf.namespace, "collect-application-logs", "collect-application-logs")
		o.Expect(err).NotTo(o.HaveOccurred())
		clf.update(oc, "", "{\"spec\": {\"serviceAccountName\": \"collect-application-logs\"}}", "--type=merge")
		checkResource(oc, true, false, `insufficient permissions on service account, not authorized to collect ["audit" "infrastructure"] logs`, []string{"clf/" + clf.name, "-n", clf.namespace, "-ojsonpath={.status.conditions[*].message}"})
		dsErr = ds.WaitUntilResourceIsGone(oc)
		o.Expect(dsErr).NotTo(o.HaveOccurred())

		exutil.By("Create a new sa and add clusterrole/collect-infrastructure-logs to the new sa, then update the CLF to use the new sa")
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("sa", "collect-infrastructure-logs", "-n", clf.namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer removeClusterRoleFromServiceAccount(oc, clf.namespace, "collect-infrastructure-logs", "collect-infrastructure-logs")
		err = addClusterRoleToServiceAccount(oc, clf.namespace, "collect-infrastructure-logs", "collect-infrastructure-logs")
		o.Expect(err).NotTo(o.HaveOccurred())
		clf.update(oc, "", "{\"spec\": {\"serviceAccountName\": \"collect-infrastructure-logs\"}}", "--type=merge")
		checkResource(oc, true, false, `insufficient permissions on service account, not authorized to collect ["application" "audit"] logs`, []string{"clf/" + clf.name, "-n", clf.namespace, "-ojsonpath={.status.conditions[*].message}"})
		dsErr = ds.WaitUntilResourceIsGone(oc)
		o.Expect(dsErr).NotTo(o.HaveOccurred())

		exutil.By("Create a new sa and add clusterrole/collect-audit-logs to the new sa, then update the CLF to use the new sa")
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("sa", "collect-audit-logs", "-n", clf.namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer removeClusterRoleFromServiceAccount(oc, clf.namespace, "collect-audit-logs", "collect-audit-logs")
		err = addClusterRoleToServiceAccount(oc, clf.namespace, "collect-audit-logs", "collect-audit-logs")
		o.Expect(err).NotTo(o.HaveOccurred())
		clf.update(oc, "", "{\"spec\": {\"serviceAccountName\": \"collect-audit-logs\"}}", "--type=merge")
		checkResource(oc, true, false, `insufficient permissions on service account, not authorized to collect ["application" "infrastructure"] logs`, []string{"clf/" + clf.name, "-n", clf.namespace, "-ojsonpath={.status.conditions[*].message}"})
		dsErr = ds.WaitUntilResourceIsGone(oc)
		o.Expect(dsErr).NotTo(o.HaveOccurred())

		exutil.By("Create a new sa and add all clusterroles to the new sa, then update the CLF to use the new sa")
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("sa", "collect-all-logs", "-n", clf.namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		for _, logType := range []string{"application", "infrastructure", "audit"} {
			role := "collect-" + logType + "-logs"
			defer removeClusterRoleFromServiceAccount(oc, clf.namespace, "collect-all-logs", role)
			err = addClusterRoleToServiceAccount(oc, clf.namespace, "collect-all-logs", role)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		clf.update(oc, "", "{\"spec\": {\"serviceAccountName\": \"collect-all-logs\"}}", "--type=merge")
		checkResource(oc, true, false, "True", []string{"clf/" + clf.name, "-n", clf.namespace, "-ojsonpath={.status.conditions[?(@.type == \"Ready\")].status}"})
		WaitForDaemonsetPodsToBeReady(oc, clf.namespace, clf.name)

		exutil.By("Remove clusterrole from the serviceaccount, the collector pods should be removed")
		err = removeClusterRoleFromServiceAccount(oc, clf.namespace, "collect-all-logs", "collect-audit-logs")
		o.Expect(err).NotTo(o.HaveOccurred())
		checkResource(oc, true, false, `insufficient permissions on service account, not authorized to collect ["audit"] logs`, []string{"clf/" + clf.name, "-n", clf.namespace, "-ojsonpath={.status.conditions[*].message}"})
		dsErr = ds.WaitUntilResourceIsGone(oc)
		o.Expect(dsErr).NotTo(o.HaveOccurred())
	})

	// author qitang@redhat.com
	g.It("CPaasrunOnly-Author:qitang-Critical-65644-Creating multiple ClusterLogForwarders shouldn't affect the legacy forwarders.[Serial][Slow]", func() {
		if isFipsEnabled(oc) {
			g.Skip("skip fluentd test on fips enabled cluster for LOG-3933")
		}
		jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
		appProj := oc.Namespace()
		err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		sc, err := getStorageClassName(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Deploying LokiStack CR for 1x.demo tshirt size")
		ls := lokiStack{
			name:          "loki-65644",
			namespace:     loggingNS,
			tSize:         "1x.demo",
			storageType:   getStorageType(oc),
			storageSecret: "storage-65644",
			storageClass:  sc,
			bucketName:    "logging-loki-65644-" + getInfrastructureName(oc),
			template:      filepath.Join(loggingBaseDir, "lokistack", "lokistack-simple.yaml"),
		}
		defer ls.removeObjectStorage(oc)
		err = ls.prepareResourcesForLokiStack(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer ls.removeLokiStack(oc)
		err = ls.deployLokiStack(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		ls.waitForLokiStackToBeReady(oc)
		e2e.Logf("LokiStack deployed")

		g.By("Create ClusterLogForwarder with Loki as default logstore for all tenants")
		clf := clusterlogforwarder{
			name:         "instance",
			namespace:    loggingNS,
			templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "forward_to_default.yaml"),
		}
		defer clf.delete(oc)
		clf.create(oc)

		g.By("Create ClusterLogging instance with Loki as logstore")
		cl := clusterlogging{
			name:          "instance",
			namespace:     loggingNS,
			collectorType: "fluentd",
			logStoreType:  "lokistack",
			lokistackName: ls.name,
			waitForReady:  true,
			templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "cl-default-loki.yaml"),
		}
		defer cl.delete(oc)
		cl.create(oc)
		resource{"serviceaccount", "logcollector", cl.namespace}.WaitForResourceToAppear(oc)
		defer removeLokiStackPermissionFromSA(oc, "my-loki-tenant-logs")
		grantLokiPermissionsToSA(oc, "my-loki-tenant-logs", "logcollector", cl.namespace)

		oc.SetupProject()
		clfNS := oc.Namespace()
		loki := externalLoki{"loki-server", clfNS}
		defer loki.remove(oc)
		loki.deployLoki(oc)

		g.By("Create ClusterLogForwarder in a new project")
		clf1 := clusterlogforwarder{
			name:                   "collector-65644",
			namespace:              clfNS,
			templateFile:           filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-external-loki.yaml"),
			serviceAccountName:     "clf-collector",
			collectApplicationLogs: true,
			waitForPodReady:        true,
			enableMonitoring:       true,
		}
		defer clf1.delete(oc)
		clf1.create(oc, "URL=http://"+loki.name+"."+loki.namespace+".svc:3100", "INPUTREFS=[\"application\"]")

		g.By("check logs in lokistack")
		bearerToken := getSAToken(oc, "logcollector", cl.namespace)
		route := "https://" + getRouteAddress(oc, ls.namespace, ls.name)
		lc := newLokiClient(route).withToken(bearerToken).retry(5)
		for _, logType := range []string{"application", "infrastructure", "audit"} {
			lc.waitForLogsAppearByKey(logType, "log_type", logType)
		}

		g.By("check logs in external loki")
		eLoki := "http://" + getRouteAddress(oc, loki.namespace, loki.name)
		elc := newLokiClient(eLoki)
		elc.waitForLogsAppearByKey("", "log_type", "application")

		g.By("check metrics exposed by collector pods in openshift-logging project")
		proToken := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")
		checkMetric(oc, proToken, "{namespace=\"openshift-logging\",job=\"collector\"}", 3)

		g.By("check metrics exposed by multiple-CLF")
		checkMetric(oc, proToken, "{namespace=\""+clf1.namespace+"\",job=\""+clf1.name+"\"}", 3)

		g.By("update outputRefs in multiple-CLF")
		clf1.update(oc, clf1.templateFile, `OUTPUTREFS=["loki-server", "default"]`, "INPUTREFS=[\"application\"]", "URL=http://"+loki.name+"."+loki.namespace+".svc:3100")
		checkResource(oc, true, false, "unrecognized outputs: [default]", []string{"clf/" + clf1.name, "-n", clf1.namespace, "-ojsonpath={.status.pipelines.forward-to-loki[*].message}"})

		g.By("check logging stack in openshift-logging project, everything should work as usual")
		res, err := lc.query("application", "{log_type=\"application\"}", 5, false, time.Now())
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(res.Data.Result) > 0).Should(o.BeTrue())

		g.By("revert changes in mulitple-CLF to make it back to normal")
		clf1.update(oc, clf1.templateFile, "URL=http://"+loki.name+"."+loki.namespace+".svc:3100", "INPUTREFS=[\"application\"]")
		WaitForDaemonsetPodsToBeReady(oc, clf1.namespace, clf1.name)

		g.By("re-check logging stack in openshift-logging project, everything should work as usual")
		res, err = lc.query("application", "{log_type=\"application\"}", 5, false, time.Now())
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(res.Data.Result) > 0).Should(o.BeTrue())
	})

	// author qitang@redhat.com
	g.It("CPaasrunOnly-Author:qitang-High-65685-Deploy CLO to all namespaces and verify prometheusrule/collector and cm/grafana-dashboard-cluster-logging are created along with the CLO.", func() {
		csvs, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", "-n", "default", "-oname").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(csvs, "cluster-logging")).Should(o.BeTrue())

		prometheusrule := resource{
			kind:      "prometheusrule",
			name:      "collector",
			namespace: loggingNS,
		}
		prometheusrule.WaitForResourceToAppear(oc)

		configmap := resource{
			kind:      "configmap",
			name:      "grafana-dashboard-cluster-logging",
			namespace: "openshift-config-managed",
		}
		configmap.WaitForResourceToAppear(oc)
	})

	// author qitang@redhat.com
	g.It("CPaasrunOnly-Author:qitang-Medium-66038-Manage collector pods via clusterlogging when multiple-clusterlogforwarders is enabled.", func() {
		clfNS := oc.Namespace()

		g.By("create log producer")
		oc.SetupProject()
		appProj := oc.Namespace()
		jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
		err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Deploy ES server")
		ees := externalES{
			namespace:  clfNS,
			version:    "8",
			serverName: "elasticsearch-server",
			loggingNS:  clfNS,
		}
		defer ees.remove(oc)
		ees.deploy(oc)

		g.By("create clusterlogforwarder")
		clf := clusterlogforwarder{
			name:      "test-66038",
			namespace: clfNS,

			templateFile:           filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-external-es.yaml"),
			serviceAccountName:     "test-66038",
			collectApplicationLogs: true,
			waitForPodReady:        true,
		}
		defer clf.delete(oc)
		inputRefs := "[\"application\"]"
		clf.create(oc, "INPUTREFS="+inputRefs, "ES_URL=http://"+ees.serverName+"."+ees.namespace+".svc:9200", "ES_VERSION="+ees.version)

		g.By("create a clusterlogging CR")
		cl := clusterlogging{
			name:          clf.name,
			namespace:     clf.namespace,
			collectorType: "vector",
			templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "collector_only.yaml"),
			waitForReady:  true,
		}
		defer cl.delete(oc)
		cl.create(oc, `RESOURCES={"requests": {"cpu": "200m"}}`)
		assertResourceStatus(oc, "daemonset", clf.name, clf.namespace, "{.spec.template.spec.containers[0].resources.requests.cpu}", "200m")

		g.By("add nodeSelector to clusterlogging")
		cl.update(oc, "", `{"spec": {"collection": {"nodeSelector": {"test-clf": "66038", "non-exist-label": "true"}}}}`, "--type=merge")
		assertResourceStatus(oc, "daemonset", clf.name, clf.namespace, "{.spec.template.spec.nodeSelector}", `{"kubernetes.io/os":"linux","non-exist-label":"true","test-clf":"66038"}`)

		g.By("add tolerations to clusterlogging")
		cl.update(oc, "", `{"spec": {"collection": {"tolerations": [{"key": "logging", "operator": "Equal", "value": "test", "effect": "NoSchedule"}]}}}`, "--type=merge")
		assertResourceStatus(oc, "daemonset", clf.name, clf.namespace, "{.spec.template.spec.tolerations}", `[{"effect":"NoSchedule","key":"node-role.kubernetes.io/master","operator":"Exists"},{"effect":"NoSchedule","key":"node.kubernetes.io/disk-pressure","operator":"Exists"},{"effect":"NoSchedule","key":"logging","operator":"Equal","value":"test"}]`)

	})

})

var _ = g.Describe("[sig-openshift-logging] Logging NonPreRelease rapidast scan", func() {
	defer g.GinkgoRecover()
	var (
		oc             = exutil.NewCLI("logging-dast", exutil.KubeConfigPath())
		loggingBaseDir string
	)
	g.BeforeEach(func() {
		loggingBaseDir = exutil.FixturePath("testdata", "logging")
	})
	// author anli@redhat.com
	g.It("CPaasrunOnly-Author:anli-High-67423-Cluster Logging Operator should pass DAST test", func() {
		CLO := SubscriptionObjects{
			OperatorName:  "cluster-logging-operator",
			Namespace:     cloNS,
			PackageName:   "cluster-logging",
			Subscription:  filepath.Join(loggingBaseDir, "subscription", "sub-template.yaml"),
			OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "allnamespace-og.yaml"),
		}
		eoSource := CatalogSourceObjects{
			Channel: "stable",
		}
		EO := SubscriptionObjects{
			OperatorName:  "elasticsearch-operator",
			Namespace:     eoNS,
			PackageName:   "elasticsearch-operator",
			Subscription:  filepath.Join(loggingBaseDir, "subscription", "sub-template.yaml"),
			OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "allnamespace-og.yaml"),
			CatalogSource: eoSource,
		}
		CLO.SubscribeOperator(oc)
		EO.SubscribeOperator(oc)
		proj := oc.Namespace()
		configFile := filepath.Join(loggingBaseDir, "rapidast/data_rapidastconfig_logging_v1.yaml")
		policyFile := filepath.Join(loggingBaseDir, "rapidast/customscan.policy")
		_, err := rapidastScan(oc, proj, configFile, policyFile, "logging.openshift.io_v1")
		o.Expect(err).NotTo(o.HaveOccurred())
	})
	// author anli@redhat.com
	g.It("CPaasrunOnly-Author:anli-High-67424-Loki Operator should pass DAST test", func() {
		LO := SubscriptionObjects{
			OperatorName:  "loki-operator-controller-manager",
			Namespace:     loNS,
			PackageName:   "loki-operator",
			Subscription:  filepath.Join(loggingBaseDir, "subscription", "sub-template.yaml"),
			OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "allnamespace-og.yaml"),
		}
		LO.SubscribeOperator(oc)
		proj := oc.Namespace()
		configFile := filepath.Join(loggingBaseDir, "rapidast/data_rapidastconfig_loki_v1.yaml")
		policyFile := filepath.Join(loggingBaseDir, "rapidast/customscan.policy")
		_, err := rapidastScan(oc, proj, configFile, policyFile, "loki.grafana.com_v1")
		o.Expect(err).NotTo(o.HaveOccurred())
	})
})
