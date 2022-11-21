package storage

import (
	"path/filepath"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

var _ = g.Describe("[sig-storage] STORAGE", func() {
	defer g.GinkgoRecover()

	var (
		oc                 = exutil.NewCLI("storage-ephemeral", exutil.KubeConfigPath())
		storageTeamBaseDir string
	)

	g.BeforeEach(func() {
		storageTeamBaseDir = exutil.FixturePath("testdata", "storage")
	})

	// author: pewang@redhat.com
	g.It("ROSA-OSD_CCS-Author:pewang-High-55151-[Local ephemeral storage] [emptyDir] setting requests and limits should work on Pod level", func() {
		// Set the resource objects definition for the scenario
		var (
			podTemplate = filepath.Join(storageTeamBaseDir, "pod-with-multiple-containers-using-emptydir-template.yaml")
			pod         = newPod(setPodTemplate(podTemplate), setPodImage("quay.io/openshifttest/base-fedora@sha256:8962182b4bfc7ee362726ad66871334587e7e5695bec3d7cfc3acbca7a4d309c"))
		)

		g.By("# Create new project for the scenario")
		oc.SetupProject()

		g.By("# Create pod with ephemeral-storage requests and limits setting")
		podObjJSONBatchActions := []map[string]string{{"items.0.spec.containers.0.resources.requests.": "set"}, {"items.0.spec.containers.0.resources.limits.": "set"},
			{"items.0.spec.containers.1.resources.requests.": "set"}, {"items.0.spec.containers.1.resources.limits.": "set"}}
		multiExtraParameters := []map[string]interface{}{{"ephemeral-storage": "1Gi"}, {"ephemeral-storage": "2Gi"}, {"ephemeral-storage": "2Gi"}, {"ephemeral-storage": "4Gi"}}
		pod.createWithMultiExtraParameters(oc, podObjJSONBatchActions, multiExtraParameters)
		defer pod.deleteAsAdmin(oc)

		// Waiting for the pods become running
		pod.waitReady(oc)

		g.By("# Write 4G data to pod container-0's emptyDir volume")
		pod.execCommandInSpecifiedContainer(oc, pod.name+"-container-0", "fallocate -l 4G /mnt/storage/testdada_4G")

		g.By("# Check pod should be still Running")
		// Even though pod's container-0 ephemeral-storage limits is 2Gi, since the ephemeral-storage limits is pod level(disk usage from all containers plus the pod's emptyDir volumes)
		// the pod's total ephemeral-storage limits is 6Gi, so it'll not be evicted by kubelet
		isPodReady := func() bool {
			isPodReady, _ := checkPodReady(oc, pod.namespace, pod.name)
			return isPodReady
		}
		o.Consistently(isPodReady, 60*time.Second, 10*time.Second).Should(o.BeTrue())

		g.By("# Continue write 3G data to pod container-1's emptyDir volume")
		pod.execCommandInSpecifiedContainer(oc, pod.name+"-container-1", "fallocate -l 3G /mnt/storage/testdada_3G")

		g.By("# Check pod should be evicted by kubelet")
		// pod's emptyDir volume(shared by container-0 and container-1) data reached 7Gi exceeds total ephemeral-storage limits 6Gi, it'll be evicted by kubelet
		o.Eventually(func() string {
			return describePod(oc, pod.namespace, pod.name)
		}, 180*time.Second, 10*time.Second).Should(o.And(
			o.ContainSubstring("Evicted"),
			o.ContainSubstring(`Pod ephemeral local storage usage exceeds the total limit of containers 6Gi.`),
		))
		// pod should be evicted by kubelet and keep the evicted(failed) phase
		o.Eventually(isPodReady, 180*time.Second, 10*time.Second).Should(o.BeFalse())
		o.Consistently(isPodReady, 30*time.Second, 10*time.Second).Should(o.BeFalse())
	})

	// author: pewang@redhat.com
	g.It("ROSA-OSD_CCS-Author:pewang-High-56080-[Local ephemeral storage] [emptyDir] with sizeLimit should work on Pod level", func() {
		// Set the resource objects definition for the scenario
		var (
			podTemplate = filepath.Join(storageTeamBaseDir, "pod-with-multiple-containers-using-emptydir-template.yaml")
			pod         = newPod(setPodTemplate(podTemplate), setPodImage("quay.io/openshifttest/base-fedora@sha256:8962182b4bfc7ee362726ad66871334587e7e5695bec3d7cfc3acbca7a4d309c"))
		)

		g.By("# Create new project for the scenario")
		oc.SetupProject()

		g.By("# Create pod with ephemeral-storage requests and limits setting, emptyDir volume with sizeLimit")
		podObjJSONBatchActions := []map[string]string{{"items.0.spec.containers.0.resources.requests.": "set"}, {"items.0.spec.containers.0.resources.limits.": "set"},
			{"items.0.spec.containers.1.resources.requests.": "set"}, {"items.0.spec.containers.1.resources.limits.": "set"}, {"items.0.spec.volumes.0.emptyDir.": "set"}}
		multiExtraParameters := []map[string]interface{}{{"ephemeral-storage": "1Gi"}, {"ephemeral-storage": "2Gi"}, {"ephemeral-storage": "2Gi"},
			{"ephemeral-storage": "4Gi"}, {"sizeLimit": "4Gi"}}
		pod.createWithMultiExtraParameters(oc, podObjJSONBatchActions, multiExtraParameters)
		defer pod.deleteAsAdmin(oc)

		// Waiting for the pod become 'Running'
		pod.waitReady(oc)

		g.By("# Write 3.0G data to pod container-0's emptyDir volume")
		pod.execCommandInSpecifiedContainer(oc, pod.name+"-container-0", "fallocate -l 3.0G /mnt/storage/testdada_3.0G")

		g.By("# Check pod should be still Running")
		// Even though pod's container-0 limits is 2Gi, since the sizeLimit is pod level, it'll not be evicted by kubelet
		// pod's emptyDir volume mount path "/mnt/storage" usage (3G) is less than sizeLimit: "4Gi"
		isPodReady := func() bool {
			isPodReady, _ := checkPodReady(oc, pod.namespace, pod.name)
			return isPodReady
		}
		o.Consistently(isPodReady, 60*time.Second, 10*time.Second).Should(o.BeTrue())

		g.By("# Write 2.0G data to pod container-1's emptyDir volume")
		pod.execCommandInSpecifiedContainer(oc, pod.name+"-container-1", "fallocate -l 2.0G /mnt/storage/testdada_2.0G")

		g.By("# Check pod should be evicted by kubelet")
		// pod has 'emptyDir.sizeLimit: "4Gi"', even though pod's total ephemeral-storage limits is 6Gi, it'll be evicted by kubelet
		// pod's emptyDir volume(shared by container-0 and container-1) reached 5G exceeds 'emptyDir.sizeLimit: "4Gi"'
		o.Eventually(func() string {
			return describePod(oc, pod.namespace, pod.name)
		}, 180*time.Second, 10*time.Second).Should(o.And(
			o.ContainSubstring("Evicted"),
			o.ContainSubstring(`Usage of EmptyDir volume "data" exceeds the limit "4Gi".`),
		))
		// pod should be evicted by kubelet and keep the evicted(failed) phase
		o.Eventually(isPodReady, 180*time.Second, 10*time.Second).Should(o.BeFalse())
		o.Consistently(isPodReady, 30*time.Second, 10*time.Second).Should(o.BeFalse())
	})
})
