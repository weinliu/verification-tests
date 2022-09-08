package storage

import (
	"path/filepath"
	"time"

	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

var _ = g.Describe("[sig-storage] STORAGE", func() {
	defer g.GinkgoRecover()

	var (
		oc                 = exutil.NewCLI("storage-cm-features", exutil.KubeConfigPath())
		storageTeamBaseDir string
	)
	g.BeforeEach(func() {
		storageTeamBaseDir = exutil.FixturePath("testdata", "storage")
	})

	// author: pewang@redhat.com
	// https://bugzilla.redhat.com/show_bug.cgi?id=1640079
	// https://github.com/kubernetes/kubernetes/pull/89629
	g.It("HyperShiftGUEST-ROSA-OSD_CCS-ARO-Author:pewang-High-26747-[ConfigMap] Pod with configmap using subpath should not stuck", func() {
		// Set the resource objects definition for the scenario
		var (
			cm  = newConfigMap(setConfigMapTemplate(filepath.Join(storageTeamBaseDir, "configmap-template.yaml")))
			dep = newDeployment(setDeploymentTemplate(filepath.Join(storageTeamBaseDir, "deployment-with-inline-volume-template.yaml")),
				setDeploymentMountpath("/mnt/storage/cm.cnf"))
		)

		g.By("# Create new project for the scenario")
		oc.SetupProject()

		g.By("# Create configmap")
		cm.create(oc)
		defer cm.deleteAsAdmin(oc)

		g.By("# Create a deployment with the configmap using subpath and wait for the deployment ready")
		// Define the configMap volume
		var myCmVol configMapVolumeWithPaths
		myCmVol.Name = cm.name
		myCmVol.Items = make([]struct {
			Key  string "json:\"key\""
			Path string "json:\"path\""
		}, 2)
		myCmVol.Items[0].Key = "storage.properties"
		myCmVol.Items[0].Path = "storageConfig-0"
		myCmVol.Items[1].Key = "storage.cnf"
		myCmVol.Items[1].Path = "storageConfig"
		// Create deployment with configmap using subpath
		depObjJSONBatchActions := []map[string]string{{"items.0.spec.template.spec.containers.0.volumeMounts.0.": "set"}, {"items.0.spec.template.spec.containers.0.volumeMounts.1.": "set"}, {"items.0.spec.template.spec.volumes.0.": "set"}}
		multiExtraParameters := []map[string]interface{}{{"subPath": "storageConfig"}, {"name": "inline-volume", "mountPath": "/mnt/storage/storage.properties", "subPath": "not-match-cm-paths"}, {"configMap": myCmVol}}
		dep.createWithMultiExtraParameters(oc, depObjJSONBatchActions, multiExtraParameters)
		defer dep.deleteAsAdmin(oc)
		dep.waitReady(oc)

		g.By("# Check the deployment's pod mount the configmap correctly")
		// Only when subPath and configMap volume items.*.path match config file content could mount
		podName := dep.getPodList(oc)[0]
		output, err := execCommandInSpecificPod(oc, dep.namespace, podName, "cat /mnt/storage/cm.cnf")
		o.Expect(err).ShouldNot(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("e2e-test = true"))
		// As the deployment's container volumeMounts.1.subPath doesn't match any of the configMap volume items paths
		// "subPath": "not-match-cm-paths" != "storageConfig" && "not-match-cm-paths" != "storageConfig-0"
		// The storage.properties should be a empty directory in the container(the config content cm.data.'storage.properties' should not exist)
		output, err = execCommandInSpecificPod(oc, dep.namespace, podName, "cat /mnt/storage/storage.properties")
		o.Expect(err).Should(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("cat: /mnt/storage/storage.properties: Is a directory"))

		// https://kubernetes.io/docs/tasks/configure-pod-container/configure-pod-configmap/#mounted-configmaps-are-updated-automatically
		// Note: A container using a ConfigMap as a subPath volume will not receive ConfigMap updates.
		g.By("# Update the configMap and check the deployment's mount content couldn't hot update because of using subpath")
		patchResourceAsAdmin(oc, cm.namespace, "cm/"+cm.name, "[{\"op\":\"replace\", \"path\":\"/data/storage.cnf\", \"value\":\"newConfig-v2\"}]", "json")
		o.Consistently(func() string {
			cmContent, _ := execCommandInSpecificPod(oc, dep.namespace, podName, "cat /mnt/storage/cm.cnf")
			return cmContent
		}, 30*time.Second, 10*time.Second).Should(o.ContainSubstring("e2e-test = true"))

		g.By("# Restart the deployment the mount content should update to the newest")
		dep.restart(oc)
		podName = dep.getPodList(oc)[0]
		output, err = execCommandInSpecificPod(oc, dep.namespace, podName, "cat /mnt/storage/cm.cnf")
		o.Expect(err).ShouldNot(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("newConfig-v2"))

		g.By("# Delete the deployment should successfully not stuck at Terminating status")
		deleteSpecifiedResource(oc, "deployment", dep.name, dep.namespace)
	})
})
