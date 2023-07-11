package storage

import (
	g "github.com/onsi/ginkgo/v2"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-storage] STORAGE", func() {
	defer g.GinkgoRecover()
	var (
		oc = exutil.NewCLI("storage-capabilities", exutil.KubeConfigPath())
	)

	g.BeforeEach(func() {
		cloudProvider = getCloudProvider(oc)
	})

	// author: wduan@redhat.com
	// OCP-54899-Check Storage CO as optional capability
	g.It("ROSA-OSD_CCS-Author:wduan-High-54899-Check Storage CO as optional capability", func() {

		// Skip if Storage CO is enabled
		if isEnabledCapability(oc, "Storage") {
			g.Skip("Skip for Storage capability enabled!")
		}

		// Check resouerce should not be present when Storage CO is disabled
		exutil.By("Check resouerce should not be present when Storage CO is disabled")
		expectSpecifiedResourceExist(oc, "clusteroperator/storage", "", false)
		expectSpecifiedResourceExist(oc, "storage/cluster", "", false)
		expectSpecifiedResourceExist(oc, "deployment/cluster-storage-operator", "openshift-cluster-storage-operator", false)
		// Get pre-defined storageclass from config file and check
		preDefinedStorageclassList := getClusterPreDefinedStorageclassByPlatform(cloudProvider)
		e2e.Logf("The pre-defined storageclass list is: %v", preDefinedStorageclassList)
		for _, sc := range preDefinedStorageclassList {
			expectSpecifiedResourceExist(oc, "storage/"+sc, "", false)
		}

		// openshift-cluster-csi-drivers namespace might be created in post-action like installing CSI Driver, keep it currently
		expectSpecifiedResourceExist(oc, "namespace/openshift-cluster-csi-drivers", "", false)
	})

	// author: wduan@redhat.com
	// OCP-54900-Check csi-snapshot-controller CO as optional capability
	g.It("ROSA-OSD_CCS-Author:wduan-High-54900-Check csi-snapshot-controller CO as optional capability", func() {

		// Skip if CSISnapshot CO is enabled
		if isEnabledCapability(oc, "CSISnapshot") {
			g.Skip("Skip for CSISnapshot capability enabled!")
		}

		// Check resouerce should not be present when CSISnapshot CO is disabled
		exutil.By("Check resouerce should not be present when CSISnapshot CO is disabled")
		expectSpecifiedResourceExist(oc, "clusteroperator/csi-snapshot-controller", "", false)
		expectSpecifiedResourceExist(oc, "CSISnapshotController/cluster", "", false)
		expectSpecifiedResourceExist(oc, "deployment/csi-snapshot-controller-operator", "openshift-cluster-storage-operator", false)
	})
})
