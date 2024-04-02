package storage

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
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
	g.It("ROSA-OSD_CCS-Author:pewang-High-55151-[Local-ephemeral-storage] [emptyDir] setting requests and limits should work on Pod level", func() {
		// Set the resource objects definition for the scenario
		var (
			podTemplate = filepath.Join(storageTeamBaseDir, "pod-with-multiple-containers-using-emptydir-template.yaml")
			pod         = newPod(setPodTemplate(podTemplate), setPodImage("quay.io/openshifttest/base-fedora@sha256:8962182b4bfc7ee362726ad66871334587e7e5695bec3d7cfc3acbca7a4d309c"))
		)

		exutil.By("# Create new project for the scenario")
		oc.SetupProject()

		exutil.By("# Create pod with ephemeral-storage requests and limits setting")
		podObjJSONBatchActions := []map[string]string{{"items.0.spec.containers.0.resources.requests.": "set"}, {"items.0.spec.containers.0.resources.limits.": "set"},
			{"items.0.spec.containers.1.resources.requests.": "set"}, {"items.0.spec.containers.1.resources.limits.": "set"}}
		multiExtraParameters := []map[string]interface{}{{"ephemeral-storage": "1Gi"}, {"ephemeral-storage": "2Gi"}, {"ephemeral-storage": "2Gi"}, {"ephemeral-storage": "4Gi"}}
		pod.createWithMultiExtraParameters(oc, podObjJSONBatchActions, multiExtraParameters)
		defer pod.deleteAsAdmin(oc)

		// Waiting for the pods become running
		pod.waitReady(oc)

		exutil.By("# Write 4G data to pod container-0's emptyDir volume")
		pod.execCommandInSpecifiedContainer(oc, pod.name+"-container-0", "fallocate -l 4G /mnt/storage/testdada_4G")

		exutil.By("# Check pod should be still Running")
		// Even though pod's container-0 ephemeral-storage limits is 2Gi, since the ephemeral-storage limits is pod level(disk usage from all containers plus the pod's emptyDir volumes)
		// the pod's total ephemeral-storage limits is 6Gi, so it'll not be evicted by kubelet
		isPodReady := func() bool {
			isPodReady, _ := checkPodReady(oc, pod.namespace, pod.name)
			return isPodReady
		}
		o.Consistently(isPodReady, 60*time.Second, 10*time.Second).Should(o.BeTrue())

		exutil.By("# Continue write 3G data to pod container-1's emptyDir volume")
		pod.execCommandInSpecifiedContainer(oc, pod.name+"-container-1", "fallocate -l 3G /mnt/storage/testdada_3G")

		exutil.By("# Check pod should be evicted by kubelet")
		// pod's emptyDir volume(shared by container-0 and container-1) data reached 7Gi exceeds total ephemeral-storage limits 6Gi, it'll be evicted by kubelet
		o.Eventually(func() string {
			reason, _ := pod.getValueByJSONPath(oc, `{.status.reason}`)
			return reason
		}, 180*time.Second, 10*time.Second).Should(o.ContainSubstring("Evicted"))
		o.Eventually(func() string {
			msg, _ := pod.getValueByJSONPath(oc, `{.status.message}`)
			return msg
		}, 180*time.Second, 10*time.Second).Should(o.ContainSubstring(`Pod ephemeral local storage usage exceeds the total limit of containers 6Gi.`))
		// pod should be evicted by kubelet and keep the evicted(failed) phase
		o.Eventually(isPodReady, 180*time.Second, 10*time.Second).Should(o.BeFalse())
		o.Consistently(isPodReady, 30*time.Second, 10*time.Second).Should(o.BeFalse())
	})

	// author: pewang@redhat.com
	g.It("ROSA-OSD_CCS-Author:pewang-LEVEL0-High-56080-[Local-ephemeral-storage] [emptyDir] with sizeLimit should work on Pod level", func() {
		// Set the resource objects definition for the scenario
		var (
			podTemplate = filepath.Join(storageTeamBaseDir, "pod-with-multiple-containers-using-emptydir-template.yaml")
			pod         = newPod(setPodTemplate(podTemplate), setPodImage("quay.io/openshifttest/base-fedora@sha256:8962182b4bfc7ee362726ad66871334587e7e5695bec3d7cfc3acbca7a4d309c"))
		)

		exutil.By("# Create new project for the scenario")
		oc.SetupProject()

		exutil.By("# Create pod with ephemeral-storage requests and limits setting, emptyDir volume with sizeLimit")
		podObjJSONBatchActions := []map[string]string{{"items.0.spec.containers.0.resources.requests.": "set"}, {"items.0.spec.containers.0.resources.limits.": "set"},
			{"items.0.spec.containers.1.resources.requests.": "set"}, {"items.0.spec.containers.1.resources.limits.": "set"}, {"items.0.spec.volumes.0.emptyDir.": "set"}}
		multiExtraParameters := []map[string]interface{}{{"ephemeral-storage": "1Gi"}, {"ephemeral-storage": "2Gi"}, {"ephemeral-storage": "2Gi"},
			{"ephemeral-storage": "4Gi"}, {"sizeLimit": "4Gi"}}
		pod.createWithMultiExtraParameters(oc, podObjJSONBatchActions, multiExtraParameters)
		defer pod.deleteAsAdmin(oc)

		// Waiting for the pod become 'Running'
		pod.waitReady(oc)

		exutil.By("# Write 3.0G data to pod container-0's emptyDir volume")
		pod.execCommandInSpecifiedContainer(oc, pod.name+"-container-0", "fallocate -l 3.0G /mnt/storage/testdada_3.0G")

		exutil.By("# Check pod should be still Running")
		// Even though pod's container-0 limits is 2Gi, since the sizeLimit is pod level, it'll not be evicted by kubelet
		// pod's emptyDir volume mount path "/mnt/storage" usage (3G) is less than sizeLimit: "4Gi"
		isPodReady := func() bool {
			isPodReady, _ := checkPodReady(oc, pod.namespace, pod.name)
			return isPodReady
		}
		o.Consistently(isPodReady, 60*time.Second, 10*time.Second).Should(o.BeTrue())

		exutil.By("# Write 2.0G data to pod container-1's emptyDir volume")
		pod.execCommandInSpecifiedContainer(oc, pod.name+"-container-1", "fallocate -l 2.0G /mnt/storage/testdada_2.0G")

		exutil.By("# Check pod should be evicted by kubelet")
		// pod has 'emptyDir.sizeLimit: "4Gi"', even though pod's total ephemeral-storage limits is 6Gi, it'll be evicted by kubelet
		// pod's emptyDir volume(shared by container-0 and container-1) reached 5G exceeds 'emptyDir.sizeLimit: "4Gi"'
		o.Eventually(func() string {
			reason, _ := pod.getValueByJSONPath(oc, `{.status.reason}`)
			return reason
		}, 180*time.Second, 10*time.Second).Should(o.ContainSubstring("Evicted"))
		o.Eventually(func() string {
			msg, _ := pod.getValueByJSONPath(oc, `{.status.message}`)
			return msg
		}, 180*time.Second, 10*time.Second).Should(o.ContainSubstring(`Usage of EmptyDir volume "data" exceeds the limit "4Gi".`))
		// pod should be evicted by kubelet and keep the evicted(failed) phase
		o.Eventually(isPodReady, 180*time.Second, 10*time.Second).Should(o.BeFalse())
		o.Consistently(isPodReady, 30*time.Second, 10*time.Second).Should(o.BeFalse())
	})

	// author: pewang@redhat.com
	g.It("ROSA-OSD_CCS-Author:pewang-High-56223-[Local-ephemeral-storage] setting requests and limits should work on container level", func() {
		// Set the resource objects definition for the scenario
		var (
			podTemplate = filepath.Join(storageTeamBaseDir, "pod-with-multiple-containers-using-emptydir-template.yaml")
			pod         = newPod(setPodTemplate(podTemplate), setPodImage("quay.io/openshifttest/base-fedora@sha256:8962182b4bfc7ee362726ad66871334587e7e5695bec3d7cfc3acbca7a4d309c"))
		)

		exutil.By("# Create new project for the scenario")
		oc.SetupProject()

		exutil.By("# Create pod with ephemeral-storage requests and limits setting")
		podObjJSONBatchActions := []map[string]string{{"items.0.spec.containers.0.resources.requests.": "set"}, {"items.0.spec.containers.0.resources.limits.": "set"},
			{"items.0.spec.containers.1.resources.limits.": "set"}, {"items.0.spec.containers.0.volumeMounts": "delete"}, {"items.0.spec.containers.1.volumeMounts": "delete"}, {"items.0.spec.volumes": "delete"}}
		multiExtraParameters := []map[string]interface{}{{"ephemeral-storage": "1Gi"}, {"ephemeral-storage": "2Gi"}, {"ephemeral-storage": "4Gi"}}
		pod.createWithMultiExtraParameters(oc, podObjJSONBatchActions, multiExtraParameters)
		defer pod.deleteAsAdmin(oc)

		// Waiting for the pods become running
		pod.waitReady(oc)

		exutil.By("# Write 3G data to pod container-0's '/tmp' path")
		pod.execCommandInSpecifiedContainer(oc, pod.name+"-container-0", "fallocate -l 3G /tmp/testdada_3G")

		exutil.By("# Check pod should be evicted by kubelet")
		isPodReady := func() bool {
			isPodReady, _ := checkPodReady(oc, pod.namespace, pod.name)
			return isPodReady
		}
		// Pod's container-0 data local ephemeral storage reached 3Gi exceeds its local ephemeral storage limit "2Gi"
		// Even though the pod's total ephemeral-storage limits is 6Gi, it'll be evicted by kubelet
		// Since only emptyDir volume limit used for pod level(the emptyDir volume shared by containers)
		o.Eventually(func() string {
			reason, _ := pod.getValueByJSONPath(oc, `{.status.reason}`)
			return reason
		}, 180*time.Second, 10*time.Second).Should(o.ContainSubstring("Evicted"))
		o.Eventually(func() string {
			msg, _ := pod.getValueByJSONPath(oc, `{.status.message}`)
			return msg
		}, 180*time.Second, 10*time.Second).Should(o.ContainSubstring(`Container ` + pod.name + `-container-0 exceeded its local ephemeral storage limit "2Gi".`))
		// pod should be evicted by kubelet and keep the evicted(failed) phase
		o.Eventually(isPodReady, 180*time.Second, 10*time.Second).Should(o.BeFalse())
		o.Consistently(isPodReady, 30*time.Second, 10*time.Second).Should(o.BeFalse())
	})

	// author: pewang@redhat.com
	g.It("ROSA-OSD_CCS-Author:pewang-High-56225-[Local-ephemeral-storage] should calculate total requests of Pod when scheduling", func() {
		// Set the resource objects definition for the scenario
		var (
			podTemplate                      = filepath.Join(storageTeamBaseDir, "pod-with-multiple-containers-using-emptydir-template.yaml")
			pod                              = newPod(setPodTemplate(podTemplate), setPodImage("quay.io/openshifttest/base-fedora@sha256:8962182b4bfc7ee362726ad66871334587e7e5695bec3d7cfc3acbca7a4d309c"))
			nodesAllocatableEphemeralStorage = make([]int64, 0, 6)
			maxAllocatableEphemeralStorage   int64
		)

		exutil.By("# Create new project for the scenario")
		oc.SetupProject()

		exutil.By("# Check all nodes should have the 'ephemeral storage capacity' and 'allocatable ephemeral storage' attributes")
		nodesInfo := getAllNodesInfo(oc)
		for _, node := range nodesInfo {
			o.Expect(strSliceContains([]string{node.ephemeralStorageCapacity, node.ephemeralStorageCapacity}, "")).Should(o.BeFalse())
		}

		exutil.By("# Get the schedulable Linux worker nodes maximal 'allocatable ephemeral storage'")
		schedulableLinuxWorkers := getSchedulableLinuxWorkers(nodesInfo)
		for _, node := range schedulableLinuxWorkers {
			currentNodeAllocatableEphemeralStorage, err := strconv.ParseInt(node.allocatableEphemeralStorage, 10, 64)
			o.Expect(err).NotTo(o.HaveOccurred())
			nodesAllocatableEphemeralStorage = append(nodesAllocatableEphemeralStorage, currentNodeAllocatableEphemeralStorage)
		}
		// Sort the schedulable Linux worker nodes 'allocatable ephemeral storage'"
		sort.Sort(int64Slice(nodesAllocatableEphemeralStorage))
		e2e.Logf(`The nodesAllocatableEphemeralStorage sort from smallest to largest is: "%+v"`, nodesAllocatableEphemeralStorage)
		maxAllocatableEphemeralStorage = nodesAllocatableEphemeralStorage[len(nodesAllocatableEphemeralStorage)-1]

		exutil.By("# Create pod with ephemeral-storage total requests setting exceeds the maximal 'allocatable ephemeral storage' among all schedulable Linux worker nodes")
		podObjJSONBatchActions := []map[string]string{{"items.0.spec.containers.0.resources.requests.": "set"}, {"items.0.spec.containers.0.resources.limits.": "set"},
			{"items.0.spec.containers.1.resources.requests.": "set"}, {"items.0.spec.containers.0.volumeMounts": "delete"}, {"items.0.spec.containers.1.volumeMounts": "delete"}, {"items.0.spec.volumes": "delete"}}
		multiExtraParameters := []map[string]interface{}{{"ephemeral-storage": fmt.Sprint(maxAllocatableEphemeralStorage)}, {"ephemeral-storage": fmt.Sprint(maxAllocatableEphemeralStorage)}, {"ephemeral-storage": "1"}}
		pod.createWithMultiExtraParameters(oc, podObjJSONBatchActions, multiExtraParameters)
		defer pod.deleteAsAdmin(oc)

		exutil.By("# Check pod should stuck at 'Pending' status")
		// Since the total ephemeral-storage requests exceeds all schedulable Linux worker nodes maximal 'allocatable ephemeral storage'
		// the pod will stuck at 'Pending' and the pod's events should contains 'Insufficient ephemeral-storage' messages
		o.Eventually(func() string {
			msg, _ := pod.getValueByJSONPath(oc, `{.status.conditions[?(@.type=="PodScheduled")].message}`)
			return msg
		}, 180*time.Second, 10*time.Second).Should(o.ContainSubstring("Insufficient ephemeral-storage"))
		o.Consistently(func() string {
			podStatus, _ := getPodStatus(oc, pod.namespace, pod.name)
			return podStatus
		}, 60*time.Second, 10*time.Second).Should(o.Equal("Pending"))
	})

	// author: pewang@redhat.com
	g.It("ROSA-OSD_CCS-Author:pewang-High-56364-[Local-ephemeral-storage] [emptyDir] setting requests and limits should obey the LimitRange policy", func() {
		// Set the resource objects definition for the scenario
		var (
			podTemplate                = filepath.Join(storageTeamBaseDir, "pod-with-multiple-containers-using-emptydir-template.yaml")
			pod                        = newPod(setPodTemplate(podTemplate), setPodImage("quay.io/openshifttest/base-fedora@sha256:8962182b4bfc7ee362726ad66871334587e7e5695bec3d7cfc3acbca7a4d309c"))
			ephemeralStorageLimitRange = LimitRange{
				Name:            "ephemeral-limitrange-" + getRandomString(),
				Type:            "Container",
				Kind:            "ephemeral-storage",
				DefaultRequests: "1Gi",
				DefaultLimits:   "1Gi",
				MinRequests:     "500Mi",
				MaxLimits:       "2Gi",
				Template:        filepath.Join(storageTeamBaseDir, "limitrange-template.yaml"),
			}
		)

		exutil.By("# Create new project for the scenario")
		oc.SetupProject()

		exutil.By("# Create ephemeral-storage LimitRange")
		ephemeralStorageLimitRange.Create(oc.AsAdmin())
		defer ephemeralStorageLimitRange.DeleteAsAdmin(oc)

		exutil.By("# Create pod with ephemeral-storage limits setting exceeds the maximal ephemeral-storage LimitRange should be failed")
		podObjJSONBatchActions := []map[string]string{{"items.0.spec.containers.0.resources.requests.": "set"}, {"items.0.spec.containers.0.resources.limits.": "set"},
			{"items.0.spec.containers.1.resources.requests.": "set"}, {"items.0.spec.containers.1.resources.limits.": "set"}}
		multiExtraParameters := []map[string]interface{}{{"ephemeral-storage": "1Gi"}, {"ephemeral-storage": "2Gi"}, {"ephemeral-storage": "2Gi"}, {"ephemeral-storage": "4Gi"}}
		defer pod.deleteAsAdmin(oc)
		msg, _ := pod.isInvalid().createWithMultiExtraParameters(oc, podObjJSONBatchActions, multiExtraParameters)
		o.Expect(msg).Should(o.ContainSubstring("is forbidden: maximum ephemeral-storage usage per Container is 2Gi, but limit is 4Gi"))

		exutil.By("# Create pod with ephemeral-storage limits setting less than the minimal ephemeral-storage LimitRange should be failed")
		multiExtraParameters[2] = map[string]interface{}{"ephemeral-storage": "400Mi"}
		multiExtraParameters[3] = map[string]interface{}{"ephemeral-storage": "400Mi"}
		msg, _ = pod.isInvalid().createWithMultiExtraParameters(oc, podObjJSONBatchActions, multiExtraParameters)
		o.Expect(msg).Should(o.ContainSubstring("is forbidden: minimum ephemeral-storage usage per Container is 500Mi, but request is 400Mi"))

		exutil.By("# Create pod without ephemeral-storage requests and limits setting should use the default setting of LimitRange")
		pod.create(oc)
		pod.waitReady(oc)

		exutil.By("# Write 1.5G data to pod container-0's emptyDir volume")
		pod.execCommandInSpecifiedContainer(oc, pod.name+"-container-0", "fallocate -l 1.5G /mnt/storage/testdada_1.5G")

		exutil.By("# Check the pod should be still Running")
		// Even though pod's container-0 ephemeral-storage limits is 1Gi(default setting in LimitRange per container),
		// Since the ephemeral-storage limits is pod level(disk usage from all containers plus the pod's emptyDir volumes)
		// the pod's total ephemeral-storage limits is 2Gi, so it'll not be evicted by kubelet
		o.Consistently(func() bool {
			isPodReady, _ := checkPodReady(oc, pod.namespace, pod.name)
			return isPodReady
		}, 60*time.Second, 10*time.Second).Should(o.BeTrue())

		exutil.By("# Continue write 1G data to pod container-1's emptyDir volume")
		pod.execCommandInSpecifiedContainer(oc, pod.name+"-container-1", "fallocate -l 1G /mnt/storage/testdada_1G")

		exutil.By("# Check pod should be evicted by kubelet")
		// Pod's emptyDir volume(shared by container-0 and container-1) reached 2.5G exceeds pod's total ephemeral-storage limits 2Gi
		// It'll be evicted by kubelet
		o.Eventually(func() string {
			reason, _ := pod.getValueByJSONPath(oc, `{.status.reason}`)
			return reason
		}, 180*time.Second, 10*time.Second).Should(o.ContainSubstring("Evicted"))
		o.Eventually(func() string {
			msg, _ := pod.getValueByJSONPath(oc, `{.status.message}`)
			return msg
		}, 180*time.Second, 10*time.Second).Should(o.ContainSubstring(`Pod ephemeral local storage usage exceeds the total limit of containers 2Gi.`))
	})

	// author: pewang@redhat.com
	g.It("ROSA-OSD_CCS-Author:pewang-High-56365-[Local-ephemeral-storage] setting requests and limits should obey ResourceQuota hard policy", func() {
		// Set the resource objects definition for the scenario
		var (
			podTemplate                   = filepath.Join(storageTeamBaseDir, "pod-with-multiple-containers-using-emptydir-template.yaml")
			pod                           = newPod(setPodTemplate(podTemplate), setPodImage("quay.io/openshifttest/base-fedora@sha256:8962182b4bfc7ee362726ad66871334587e7e5695bec3d7cfc3acbca7a4d309c"))
			ephemeralStorageResourceQuota = ResourceQuota{
				Name:         "ephemeral-resourcequota-" + getRandomString(),
				Type:         "ephemeral-storage",
				HardRequests: "6Gi",
				HardLimits:   "6Gi",
				Template:     filepath.Join(storageTeamBaseDir, "resourcequota-template.yaml"),
			}
		)

		exutil.By("# Create new project for the scenario")
		oc.SetupProject()

		exutil.By("# Create ephemeral-storage ResourceQuota")
		ephemeralStorageResourceQuota.Create(oc.AsAdmin())
		defer ephemeralStorageResourceQuota.DeleteAsAdmin(oc)

		exutil.By("# Create pod with ephemeral-storage limits setting exceeds the ephemeral-storage ResourceQuota hard limits should be failed")
		podObjJSONBatchActions := []map[string]string{{"items.0.spec.containers.0.resources.requests.": "set"}, {"items.0.spec.containers.0.resources.limits.": "set"},
			{"items.0.spec.containers.1.resources.requests.": "set"}, {"items.0.spec.containers.1.resources.limits.": "set"}}
		multiExtraParameters := []map[string]interface{}{{"ephemeral-storage": "2Gi"}, {"ephemeral-storage": "2Gi"}, {"ephemeral-storage": "1Gi"}, {"ephemeral-storage": "5Gi"}}
		defer pod.deleteAsAdmin(oc)
		msg, _ := pod.isInvalid().createWithMultiExtraParameters(oc, podObjJSONBatchActions, multiExtraParameters)
		o.Expect(msg).Should(o.ContainSubstring("is forbidden: exceeded quota: " + ephemeralStorageResourceQuota.Name + ", requested: limits.ephemeral-storage=7Gi, used: limits.ephemeral-storage=0, limited: limits.ephemeral-storage=6Gi"))

		exutil.By("# Create pod with ephemeral-storage requests and limits setting exceeds the ephemeral-storage ResourceQuota hard requests and limits should be failed")
		multiExtraParameters[2] = map[string]interface{}{"ephemeral-storage": "5Gi"}
		msg, _ = pod.isInvalid().createWithMultiExtraParameters(oc, podObjJSONBatchActions, multiExtraParameters)
		o.Expect(msg).Should(o.ContainSubstring("is forbidden: exceeded quota: " + ephemeralStorageResourceQuota.Name + ", requested: limits.ephemeral-storage=7Gi,requests.ephemeral-storage=7Gi, used: limits.ephemeral-storage=0,requests.ephemeral-storage=0, limited: limits.ephemeral-storage=6Gi,requests.ephemeral-storage=6Gi"))

		exutil.By("# Create pod with ephemeral-storage limits setting less than or equal to hard limits should be successful")
		container2EphemeralStorageRequests := getRandomNum(1, 4)
		container2EphemeralStorageLimits := container2EphemeralStorageRequests
		podEphemeralStorageTotalLimits := container2EphemeralStorageLimits + 2
		multiExtraParameters[2] = map[string]interface{}{"ephemeral-storage": strconv.FormatInt(container2EphemeralStorageRequests, 10) + "Gi"}
		multiExtraParameters[3] = map[string]interface{}{"ephemeral-storage": strconv.FormatInt(container2EphemeralStorageLimits, 10) + "Gi"}
		pod.createWithMultiExtraParameters(oc, podObjJSONBatchActions, multiExtraParameters)
		pod.waitReady(oc)

		exutil.By("# Check the ephemeral-storage ResourceQuota status is as expected")
		o.Eventually(ephemeralStorageResourceQuota.GetValueByJSONPath(oc, `{.status.used.requests\.ephemeral-storage}`),
			60*time.Second, 10*time.Second).Should(o.Equal(strconv.FormatInt(podEphemeralStorageTotalLimits, 10) + "Gi"))
		o.Eventually(ephemeralStorageResourceQuota.GetValueByJSONPath(oc, `{.status.used.limits\.ephemeral-storage}`),
			60*time.Second, 10*time.Second).Should(o.Equal(strconv.FormatInt(podEphemeralStorageTotalLimits, 10) + "Gi"))

		exutil.By("# Write data exceeds pod's total ephemeral-storage limits to its emptyDir volume")
		pod.execCommandInSpecifiedContainer(oc, pod.name+"-container-0", "fallocate -l "+strconv.FormatInt(podEphemeralStorageTotalLimits+2, 10)+"G /mnt/storage/testdada")

		exutil.By("# Check pod should be evicted by kubelet and the ResourceQuota usage updated")
		// Pod's emptyDir volume exceeds pod's total ephemeral-storage limits
		// It'll be evicted by kubelet
		o.Eventually(func() string {
			reason, _ := pod.getValueByJSONPath(oc, `{.status.reason}`)
			return reason
		}, 180*time.Second, 10*time.Second).Should(o.ContainSubstring("Evicted"))
		o.Eventually(func() string {
			msg, _ := pod.getValueByJSONPath(oc, `{.status.message}`)
			return msg
		}, 180*time.Second, 10*time.Second).Should(o.ContainSubstring("Pod ephemeral local storage usage exceeds the total limit of containers " + strconv.FormatInt(podEphemeralStorageTotalLimits, 10) + "Gi."))
		// ResourceQuota usage should update after pod is evicted by kubelet
		o.Eventually(ephemeralStorageResourceQuota.PollGetValueByJSONPath(oc, `{.status.used.requests\.ephemeral-storage}`),
			180*time.Second, 10*time.Second).Should(o.Equal("0"))
		o.Eventually(ephemeralStorageResourceQuota.PollGetValueByJSONPath(oc, `{.status.used.limits\.ephemeral-storage}`),
			180*time.Second, 10*time.Second).Should(o.Equal("0"))
	})

	// author: pewang@redhat.com
	// OCP-60915-[CSI Inline Volume] [Admission plugin] should deny pods with inline volumes when the driver uses the privileged label
	g.It("ROSA-OSD_CCS-Author:pewang-High-60915-[CSI Inline Volume] [Admission plugin] should deny pods with inline volumes when the driver uses the privileged label [Disruptive]", func() {

		// Currently only sharedresource csi driver support csi inline volume which is still TP in 4.13, it will auto installed on TechPreviewNoUpgrade clusters
		if !checkCSIDriverInstalled(oc, []string{"csi.sharedresource.openshift.io"}) {
			g.Skip("Skip for support inline volume csi driver is not installed on the test cluster !!!")
		}

		// Set the resource objects definition for the scenario
		var (
			caseID                  = "60915"
			sharedResourceCsiDriver = "csidriver/csi.sharedresource.openshift.io"
			deploymentTemplate      = filepath.Join(storageTeamBaseDir, "deployment-with-inline-volume-template.yaml")
			dep                     = newDeployment(setDeploymentTemplate(deploymentTemplate))
			cm                      = newConfigMap(setConfigMapTemplate(filepath.Join(storageTeamBaseDir, "configmap-template.yaml")))
			mySharedConfigMap       = sharedConfigMap{
				name:     "my-sharedconfigmap-" + caseID,
				refCm:    &cm,
				template: filepath.Join(storageTeamBaseDir, "csi-sharedconfigmap-template.yaml"),
			}
			myCsiSharedresourceInlineVolume = InlineVolume{
				Kind:             "csiSharedresourceInlineVolume",
				VolumeDefinition: newCsiSharedresourceInlineVolume(setCsiSharedresourceInlineVolumeSharedCM(mySharedConfigMap.name)),
			}
			shareCmRoleName                                  = "shared-cm-role-" + caseID
			clusterVersionOperator                           = newDeployment(setDeploymentName("cluster-version-operator"), setDeploymentNamespace("openshift-cluster-version"), setDeploymentApplabel("k8s-app=cluster-version-operator"))
			clusterStorageOperator                           = newDeployment(setDeploymentName("cluster-storage-operator"), setDeploymentNamespace("openshift-cluster-storage-operator"), setDeploymentApplabel("name=cluster-storage-operator"))
			sharedResourceCsiDriverOperator                  = newDeployment(setDeploymentName("shared-resource-csi-driver-operator"), setDeploymentNamespace("openshift-cluster-csi-drivers"), setDeploymentApplabel("name=shared-resource-csi-driver-operator"))
			clusterVersionOperatorOriginReplicasNum          = clusterVersionOperator.getReplicasNum(oc.AsAdmin())
			clusterStorageOperatorOriginReplicasNum          = clusterStorageOperator.getReplicasNum(oc.AsAdmin())
			sharedResourceCsiDriverOperatorOriginReplicasNum = sharedResourceCsiDriverOperator.getReplicasNum(oc.AsAdmin())
		)

		exutil.By("# Scale down CVO,CSO,SharedResourceCsiDriverOperator and add privileged label to the sharedresource csi driver")
		defer waitCSOhealthy(oc.AsAdmin())
		defer clusterVersionOperator.waitReady(oc.AsAdmin())
		defer clusterStorageOperator.waitReady(oc.AsAdmin())
		defer sharedResourceCsiDriverOperator.waitReady(oc.AsAdmin())

		clusterVersionOperator.scaleReplicas(oc.AsAdmin(), "0")
		defer clusterVersionOperator.scaleReplicas(oc.AsAdmin(), clusterVersionOperatorOriginReplicasNum)
		clusterStorageOperator.scaleReplicas(oc.AsAdmin(), "0")
		defer clusterStorageOperator.scaleReplicas(oc.AsAdmin(), clusterStorageOperatorOriginReplicasNum)
		sharedResourceCsiDriverOperator.scaleReplicas(oc.AsAdmin(), "0")
		defer sharedResourceCsiDriverOperator.scaleReplicas(oc.AsAdmin(), sharedResourceCsiDriverOperatorOriginReplicasNum)

		admissionStandards, getInfoError := exutil.GetResourceSpecificLabelValue(oc.AsAdmin(), sharedResourceCsiDriver, "", "security\\.openshift\\.io/csi-ephemeral-volume-profile")
		o.Expect(getInfoError).ShouldNot(o.HaveOccurred())
		defer exutil.AddLabelsToSpecificResource(oc.AsAdmin(), sharedResourceCsiDriver, "", "security.openshift.io/csi-ephemeral-volume-profile="+admissionStandards)
		o.Expect(exutil.AddLabelsToSpecificResource(oc.AsAdmin(), sharedResourceCsiDriver, "", "security.openshift.io/csi-ephemeral-volume-profile=privileged")).Should(o.ContainSubstring("labeled"))

		exutil.By("# Create new project for the scenario")
		oc.SetupProject()

		exutil.By("# Create test configmap")
		cm.create(oc)
		defer cm.deleteAsAdmin(oc)

		exutil.By("# Create test sharedconfigmap")
		mySharedConfigMap.create(oc.AsAdmin())
		defer mySharedConfigMap.deleteAsAdmin(oc)

		exutil.By("# Create sharedconfigmap role and add the role to default sa ans project user under the test project")
		o.Expect(oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", cm.namespace, "role", shareCmRoleName, "--verb=get", "--resource=sharedconfigmaps").Execute()).ShouldNot(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", cm.namespace, "role", shareCmRoleName).Execute()
		patchResourceAsAdmin(oc, cm.namespace, "role/"+shareCmRoleName, `[{"op":"replace","path":"/rules/0/verbs/0","value":"use"}]`, "json")
		o.Expect(oc.AsAdmin().WithoutNamespace().Run("policy").Args("-n", cm.namespace, "add-role-to-user", shareCmRoleName, "-z", "default", "--role-namespace="+cm.namespace).Execute()).ShouldNot(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("policy").Args("-n", cm.namespace, "remove-role-from-user", shareCmRoleName, "-z", "default", "--role-namespace="+cm.namespace).Execute()
		o.Expect(oc.AsAdmin().WithoutNamespace().Run("policy").Args("-n", cm.namespace, "add-role-to-user", shareCmRoleName, cm.namespace+"-user", "--role-namespace="+cm.namespace).Execute()).ShouldNot(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("policy").Args("-n", cm.namespace, "remove-role-from-user", shareCmRoleName, cm.namespace+"-user", "--role-namespace="+cm.namespace).Execute()

		exutil.By("# Create deployment with csi sharedresource volume should be denied")
		defer dep.deleteAsAdmin(oc)
		msg, err := dep.createWithInlineVolumeWithOutAssert(oc, myCsiSharedresourceInlineVolume)
		o.Expect(err).Should(o.HaveOccurred())
		// TODO: when https://issues.redhat.com/browse/OCPBUGS-8220 fixed enhance the check info contains the pod name
		keyMsg := fmt.Sprintf("uses an inline volume provided by CSIDriver csi.sharedresource.openshift.io and namespace %s has a pod security enforce level that is lower than privileged", cm.namespace)
		o.Expect(msg).Should(o.ContainSubstring(keyMsg))

		exutil.By(`# Add label "pod-security.kubernetes.io/enforce=privileged", "security.openshift.io/scc.podSecurityLabelSync=false" to the test namespace`)
		o.Expect(exutil.AddLabelsToSpecificResource(oc.AsAdmin(), "ns/"+cm.namespace, "", "pod-security.kubernetes.io/enforce=privileged", "security.openshift.io/scc.podSecurityLabelSync=false")).Should(o.ContainSubstring("labeled"))

		exutil.By("# Create deployment with csi sharedresource volume should be successful")
		keyMsg = fmt.Sprintf("uses an inline volume provided by CSIDriver csi.sharedresource.openshift.io and namespace %s has a pod security warn level that is lower than privileged", cm.namespace)
		defer dep.deleteAsAdmin(oc)
		msg, err = dep.createWithInlineVolumeWithOutAssert(oc, myCsiSharedresourceInlineVolume)
		o.Expect(err).ShouldNot(o.HaveOccurred())
		// As the ns has default pod-security.kubernetes.io/audit=restricted label the deployment could create successfully but still with a warning info
		o.Expect(msg).Should(o.ContainSubstring(keyMsg))
		dep.waitReady(oc)

		exutil.By("# Check the audit log record the csi inline volume used in the test namespace")
		keyMsg = fmt.Sprintf("uses an inline volume provided by CSIDriver csi.sharedresource.openshift.io and namespace %s has a pod security audit level that is lower than privileged", cm.namespace)
		msg, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("node-logs", "--role=master", "--path=kube-apiserver/audit.log").Output()
		o.Expect(err).ShouldNot(o.HaveOccurred())
		o.Expect(strings.Count(msg, keyMsg) > 0).Should(o.BeTrue())
	})
})
