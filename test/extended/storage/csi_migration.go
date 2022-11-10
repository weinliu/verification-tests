package storage

import (
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-storage] STORAGE", func() {
	defer g.GinkgoRecover()
	var (
		oc                               = exutil.NewCLI("storage-general-intree", exutil.KubeConfigPath())
		cloudProviderSupportProvisioners []string
		caseID                           string
	)

	// csi test suite cloud provider support check
	g.BeforeEach(func() {
		cloudProvider = getCloudProvider(oc)
		generalIntreeSupportCheck(cloudProvider)
		cloudProviderSupportProvisioners = getIntreeSupportProvisionersByCloudProvider(oc)

		// Identify the cluster version
		clusterVersion, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterversion", "version", "-o=jsonpath={.status.desired.version}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("The cluster version for platform %s is %s", cloudProvider, clusterVersion)

		// Get the history, previous version of cluster
		historyVersionOp := getClusterHistoryVersions(oc)
		if len(historyVersionOp) != 2 || !strings.Contains(strings.Join(historyVersionOp, ";"), "4.11") {
			g.Skip("Skipping the execution due to Multi/Minor version upgrades")
		}
	})

	// author: ropatil@redhat.com
	// [CSI-Migration] PVCs created with in-tree storageclass are processed by CSI Driver after CSI migration is enabled
	g.It("NonPreRelease-PstChkUpgrade-Author:ropatil-Medium-49496-Upgrade [CSIMigration] PVCs created with in-tree storageclass are processed by CSI Driver after CSI migration is enabled", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"kubernetes.io/aws-ebs", "kubernetes.io/gce-pd"}
		caseID = "49496"
		// Set the resource template for the scenario
		var (
			supportProvisioners = sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		g.By("Using the existing project")
		namespace, depName, pvcName, scName := setNames(caseID)

		// Check the project exists after upgrade
		_, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("project", namespace).Output()
		if err != nil {
			e2e.Failf("There is no project existing with ns %s", namespace)
		}

		defer deleteFuncCall(oc, namespace, depName, pvcName, scName)

		for _, provisioner := range supportProvisioners {
			g.By("****** CSIMigration post for " + cloudProvider + " platform with provisioner: \"" + provisioner + "\" test phase start" + "******")

			csiMigrationPostCheckCommonTestSteps(oc, pvcName, depName, namespace)

			g.By("****** CSIMigration post for " + cloudProvider + " platform with provisioner: \"" + provisioner + "\" test phase finished" + "******")

		}
	})

	// author: ropatil@redhat.com
	// [CSI-Migration] PVCs created with in-tree storageclass, block volume are processed by CSI Driver after CSI migration is enabled
	g.It("NonPreRelease-PstChkUpgrade-Author:ropatil-Medium-49678-Upgrade [CSIMigration] PVCs created with in-tree storageclass, block volume are processed by CSI Driver after CSI migration is enabled", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"kubernetes.io/aws-ebs", "kubernetes.io/gce-pd"}
		caseID = "49678"
		// Set the resource template for the scenario
		var (
			supportProvisioners = sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		g.By("Using the existing project")
		namespace, depName, pvcName, scName := setNames(caseID)

		_, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("project", namespace).Output()
		if err != nil {
			e2e.Failf("There is no project existing with ns %s", namespace)
		}

		defer deleteFuncCall(oc, namespace, depName, pvcName, scName)

		for _, provisioner := range supportProvisioners {
			g.By("****** CSIMigration post for " + cloudProvider + " platform with provisioner: \"" + provisioner + "\" test phase start" + "******")

			csiMigrationPostCheckCommonTestSteps(oc, pvcName, depName, namespace)

			g.By("****** CSIMigration post for " + cloudProvider + " platform with provisioner: \"" + provisioner + "\" test phase finished" + "******")

		}
	})
})

// function to delete all the resources from the project
func deleteFuncCall(oc *exutil.CLI, namespace string, depName string, pvcName string, scName string) {
	defer deleteSpecifiedResource(oc.AsAdmin(), "sc", scName, "")                 // Delete the storageclass
	defer deleteProjectAsAdmin(oc, namespace)                                     // Delete the project
	defer deleteSpecifiedResource(oc.AsAdmin(), "pvc", pvcName, namespace)        // Delete the pvc
	defer deleteSpecifiedResource(oc.AsAdmin(), "deployment", depName, namespace) // Delete the dep
}

// function to retrun ns, depname, pvcname, scname
func setNames(caseID string) (string, string, string, string) {
	namespace := "migration-upgrade-" + caseID
	depName := "mydep-" + caseID
	pvcName := "mypvc-" + caseID
	scName := "mysc-" + caseID
	return namespace, depName, pvcName, scName
}

// function to check postcheck test steps
func csiMigrationPostCheckCommonTestSteps(oc *exutil.CLI, pvcName string, depName string, namespace string) {
	g.By("# Get the info volName, podsList")
	volName := getPersistentVolumeNameByPersistentVolumeClaim(oc.AsAdmin(), namespace, pvcName)
	e2e.Logf("The PVC  %s in namespace %s Bound pv is %q", pvcName, namespace, volName)
	podsList, _ := getPodsListByLabel(oc.AsAdmin(), namespace, "app")
	e2e.Logf("The podsList is %v", podsList)

	g.By("# Check the pod status in Running state")
	checkPodStatusByLabel(oc.AsAdmin(), namespace, "app", "Running")

	g.By("# Check if pv have migration annotation parameters after migration")
	annotationValues := getPvAnnotationValues(oc.AsAdmin(), namespace, pvcName)
	o.Expect(strings.Contains(annotationValues, "pv.kubernetes.io/migrated-to")).Should(o.BeTrue())

	g.By("# Check the pod has original data")
	volumeMode, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pvc", pvcName, "-n", namespace, "-o=jsonpath={.spec.volumeMode}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if volumeMode == "Filesystem" {
		output, err := execCommandInSpecificPod(oc.AsAdmin(), namespace, podsList[0], "cat /mnt/storage/testfile_*")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("storage test"))

		_, err = execCommandInSpecificPod(oc.AsAdmin(), namespace, podsList[0], "echo newdata > /mnt/storage/testfile2.txt")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(execCommandInSpecificPod(oc.AsAdmin(), namespace, podsList[0], "cat /mnt/storage/testfile2.txt")).To(o.ContainSubstring("newdata"))

	} else {
		_, err := execCommandInSpecificPod(oc.AsAdmin(), namespace, podsList[0], "/bin/dd if=/dev/dblock of=/tmp/testfile bs=512 count=1")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(execCommandInSpecificPod(oc.AsAdmin(), namespace, podsList[0], "cat /tmp/testfile | grep 'test data' ")).To(o.ContainSubstring("matches"))

		e2e.Logf("Writing the data as Block level")
		_, err = execCommandInSpecificPod(oc.AsAdmin(), namespace, podsList[0], "/bin/dd  if=/dev/null of=/dev/dblock bs=512 count=1")
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = execCommandInSpecificPod(oc.AsAdmin(), namespace, podsList[0], "echo 'test data' > /dev/dblock ")
		o.Expect(err).NotTo(o.HaveOccurred())
	}

	g.By("# Delete the  Resources: deployment, pvc from namespace and check pv does not exist")
	deleteSpecifiedResource(oc.AsAdmin(), "deployment", depName, namespace)
	deleteSpecifiedResource(oc.AsAdmin(), "pvc", pvcName, namespace)
	checkResourcesNotExist(oc.AsAdmin(), "pv", volName, "")
}
