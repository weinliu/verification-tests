package logging

import (
	"encoding/json"
	"path/filepath"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

var _ = g.Describe("[sig-openshift-logging] Logging NonPreRelease", func() {
	defer g.GinkgoRecover()

	var (
		oc             = exutil.NewCLI("logging-json-log", exutil.KubeConfigPath())
		loggingBaseDir string
	)

	g.Context("JSON structured logs -- outputs testing", func() {

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
			EO := SubscriptionObjects{
				OperatorName:  "elasticsearch-operator",
				Namespace:     eoNS,
				PackageName:   "elasticsearch-operator",
				Subscription:  filepath.Join(loggingBaseDir, "subscription", "sub-template.yaml"),
				OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "allnamespace-og.yaml"),
			}
			CLO.SubscribeOperator(oc)
			EO.SubscribeOperator(oc)
			oc.SetupProject()
		})

		// author: qitang@redhat.com
		g.It("CPaasrunOnly-Author:qitang-Medium-41302-structuredTypeKey for external ES which doesn't enabled ingress plugin[Serial]", func() {
			app := oc.Namespace()
			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-f", jsonLogFile, "-n", app).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			oc.SetupProject()
			esProj := oc.Namespace()
			ees := externalES{
				namespace:  esProj,
				version:    "7",
				serverName: "elasticsearch-server",
				httpSSL:    true,
				clientAuth: true,
				secretName: "external-es-41302",
				loggingNS:  loggingNS,
			}
			defer ees.remove(oc)
			ees.deploy(oc)

			g.By("create clusterlogforwarder/instance")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    loggingNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "41729.yaml"),
				secretName:   ees.secretName,
			}
			defer clf.delete(oc)
			projects, _ := json.Marshal([]string{app})
			eesURL := "https://" + getRouteAddress(oc, ees.namespace, ees.serverName) + ":443"
			clf.create(oc, "DATA_PROJECTS="+string(projects), "STRUCTURED_TYPE_KEY=kubernetes.namespace_name", "URL="+eesURL)

			g.By("deploy collector pods")
			cl := clusterlogging{
				name:          "instance",
				namespace:     loggingNS,
				collectorType: "fluentd",
				waitForReady:  true,
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "collector_only.yaml"),
			}
			defer cl.delete(oc)
			cl.create(oc)

			g.By("check indices in external ES pod")
			ees.waitForIndexAppear(oc, "app-"+app+"-write")

			//check if the JSON logs are parsed
			logs := ees.searchDocByQuery(oc, "app-"+app, "{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match_phrase\": {\"kubernetes.namespace_name\": \""+app+"\"}}}")
			o.Expect(logs.Hits.DataHits[0].Source.Structured.Message).Should(o.Equal("MERGE_JSON_LOG=true"))
		})
	})

})
