package storage

import (
	"encoding/json"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

var _ = g.Describe("[sig-storage] STORAGE", func() {
	defer g.GinkgoRecover()
	var (
		oc = exutil.NewCLI("storage-external", exutil.KubeConfigPath())
	)
	g.BeforeEach(func() {
		checkOptionalCapability(oc, "Storage")
		cloudProvider = getCloudProvider(oc)

		if !strings.Contains(cloudProvider, "external") {
			g.Skip("Skip for non-supported cloud provider: *" + cloudProvider + "* !!!")
		}
	})

	// author: chaoyang@redhat.com
	// https://issues.redhat.com/browse/STOR-1074
	g.It("NonHyperShiftHOST-Author:chaoyang-High-61553-[EXTERNAL][CSO] should be healthy on external platform", func() {

		exutil.By("Check CSO is healthy")
		checkCSOhealthy(oc)

		exutil.By("Check clusteroperator storage type==Available message is as expected")
		conditions, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co", "storage", `-o=jsonpath={.status.conditions[?(@.type=="Available")]}`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		var conditionJSONMap map[string]interface{}
		err = json.Unmarshal([]byte(conditions), &conditionJSONMap)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(conditionJSONMap["message"]).To(o.Equal("DefaultStorageClassControllerAvailable: No default StorageClass for this platform"))

		exutil.By("Check pods cluster-storage-operator is running")
		checkPodStatusByLabel(oc.AsAdmin(), "openshift-cluster-storage-operator", "name=cluster-storage-operator", "Running")

	})

})
