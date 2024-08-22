package storage

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	o "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/util/wait"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// Pod workload related functions
type pod struct {
	name             string
	namespace        string
	pvcname          string
	template         string
	image            string
	volumeType       string
	pathType         string
	mountPath        string
	maxWaitReadyTime time.Duration
	invalid          bool
}

// Define the global Storage Operators && Driver deploments object
var (
	vSphereDetectorOperator = newDeployment(setDeploymentName("vsphere-problem-detector-operator"), setDeploymentNamespace("openshift-cluster-storage-operator"),
		setDeploymentApplabel("name=vsphere-problem-detector-operator"))
	vSphereDriverController = newDeployment(setDeploymentName("vmware-vsphere-csi-driver-controller"), setDeploymentNamespace("openshift-cluster-csi-drivers"),
		setDeploymentApplabel("app=vmware-vsphere-csi-driver-controller"), setDeploymentReplicasNumber("2"))
	vSphereCSIDriverOperator = newDeployment(setDeploymentName("vmware-vsphere-csi-driver-operator"), setDeploymentNamespace("openshift-cluster-csi-drivers"),
		setDeploymentApplabel("name=vmware-vsphere-csi-driver-operator"), setDeploymentReplicasNumber("1"))
	efsDriverController = newDeployment(setDeploymentName("aws-efs-csi-driver-controller"), setDeploymentNamespace("openshift-cluster-csi-drivers"),
		setDeploymentApplabel("app=aws-efs-csi-driver-controller"), setDeploymentReplicasNumber("2"))
)

// function option mode to change the default values of pod parameters, e.g. name, namespace, persistent volume claim, image etc.
type podOption func(*pod)

// Replace the default value of pod name parameter
func setPodName(name string) podOption {
	return func(this *pod) {
		this.name = name
	}
}

// Replace the default value of pod template parameter
func setPodTemplate(template string) podOption {
	return func(this *pod) {
		this.template = template
	}
}

// Replace the default value of pod namespace parameter
func setPodNamespace(namespace string) podOption {
	return func(this *pod) {
		this.namespace = namespace
	}
}

// Replace the default value of pod persistent volume claim parameter
func setPodPersistentVolumeClaim(pvcname string) podOption {
	return func(this *pod) {
		this.pvcname = pvcname
	}
}

// Replace the default value of pod image parameter
func setPodImage(image string) podOption {
	return func(this *pod) {
		this.image = image
	}
}

// Replace the default value of pod volume type
func setPodVolumeType(volumeType string) podOption {
	return func(this *pod) {
		this.volumeType = volumeType
	}
}

// Replace the default value of pod mount path type
func setPodPathType(pathType string) podOption {
	return func(this *pod) {
		this.pathType = pathType
	}
}

// Replace the default value of pod mount path value
func setPodMountPath(mountPath string) podOption {
	return func(this *pod) {
		this.mountPath = mountPath
	}
}

// Create a new customized pod object
func newPod(opts ...podOption) pod {
	defaultMaxWaitReadyTime := defaultMaxWaitingTime
	if provisioner == "filestore.csi.storage.gke.io" {
		defaultMaxWaitReadyTime = longerMaxWaitingTime
	}
	defaultPod := pod{
		name:             "mypod-" + getRandomString(),
		template:         "pod-template.yaml",
		namespace:        "",
		pvcname:          "mypvc",
		image:            "quay.io/openshifttest/hello-openshift@sha256:56c354e7885051b6bb4263f9faa58b2c292d44790599b7dde0e49e7c466cf339",
		volumeType:       "volumeMounts",
		pathType:         "mountPath",
		mountPath:        "/mnt/storage",
		maxWaitReadyTime: defaultMaxWaitReadyTime,
		invalid:          false,
	}

	for _, o := range opts {
		o(&defaultPod)
	}

	return defaultPod
}

// isInvalid changes pod.invalid to true
// Using for negative test that the pod is invalid and should create failed
func (po *pod) isInvalid() *pod {
	newPod := *po
	newPod.invalid = true
	return &newPod
}

// Create new pod with customized parameters
func (po *pod) create(oc *exutil.CLI) {
	if po.namespace == "" {
		po.namespace = oc.Namespace()
	}
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", po.template, "-p", "PODNAME="+po.name, "PODNAMESPACE="+po.namespace, "PVCNAME="+po.pvcname, "PODIMAGE="+po.image, "VOLUMETYPE="+po.volumeType, "PATHTYPE="+po.pathType, "PODMOUNTPATH="+po.mountPath)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// create new pod with multiple persistentVolumeClaim
func (po *pod) createWithMultiPVCAndNodeSelector(oc *exutil.CLI, pvclist []persistentVolumeClaim, nodeSelector map[string]string) {
	if po.namespace == "" {
		po.namespace = oc.Namespace()
	}
	jsonPathsAndActions := make([]map[string]string, 2*int64(len(pvclist))+2)
	multiExtraParameters := make([]map[string]interface{}, 2*int64(len(pvclist))+2)
	for i := int64(1); i < int64(len(pvclist)); i++ {
		count := strconv.FormatInt(i, 10)
		volumeMount := map[string]interface{}{
			"mountPath": "/mnt/storage/" + count,
			"name":      "data" + count,
		}
		volumeMountPath := "items.0.spec.containers.0.volumeMounts." + count + "."
		jsonPathsAndActions[2*i-2] = map[string]string{volumeMountPath: "set"}
		multiExtraParameters[2*i-2] = volumeMount

		pvcname := map[string]string{
			"claimName": pvclist[i].name,
		}
		volumeParam := map[string]interface{}{
			"name":                  "data" + count,
			"persistentVolumeClaim": pvcname,
		}
		volumeParamPath := "items.0.spec.volumes." + count + "."
		jsonPathsAndActions[2*i-1] = map[string]string{volumeParamPath: "set"}
		multiExtraParameters[2*i-1] = volumeParam
	}
	if len(nodeSelector) != 0 {
		nodeType := nodeSelector["nodeType"]
		nodeName := nodeSelector["nodeName"]
		nodeNamePath := "items.0.spec.nodeSelector."
		nodeNameParam := map[string]interface{}{
			"kubernetes\\.io/hostname": nodeName,
		}
		jsonPathsAndActions[2*int64(len(pvclist))] = map[string]string{nodeNamePath: "set"}
		multiExtraParameters[2*int64(len(pvclist))] = nodeNameParam

		if strings.Contains(nodeType, "master") {
			tolerationPath := "items.0.spec.tolerations.0."
			tolerationParam := map[string]interface{}{
				"operator": "Exists",
				"effect":   "NoSchedule",
			}
			jsonPathsAndActions[2*int64(len(pvclist))+1] = map[string]string{tolerationPath: "set"}
			multiExtraParameters[2*int64(len(pvclist))+1] = tolerationParam
		}
	}
	o.Expect(applyResourceFromTemplateWithMultiExtraParameters(oc, jsonPathsAndActions, multiExtraParameters, "--ignore-unknown-parameters=true", "-f", po.template, "-p", "PODNAME="+po.name, "PODNAMESPACE="+po.namespace, "PVCNAME="+po.pvcname, "PODIMAGE="+po.image, "VOLUMETYPE="+po.volumeType, "PATHTYPE="+po.pathType, "PODMOUNTPATH="+po.mountPath)).Should(o.ContainSubstring("created"))
}

// Create new Pod with InlineVolume
func (po *pod) createWithInlineVolume(oc *exutil.CLI, inVol InlineVolume) {
	if po.namespace == "" {
		po.namespace = oc.Namespace()
	}
	var (
		extraParameters map[string]interface{}
		jsonPath        = `items.0.spec.volumes.0.`
	)
	switch inVol.Kind {
	case "genericEphemeralVolume", "csiEphemeralVolume":
		extraParameters = map[string]interface{}{
			"jsonPath":  jsonPath,
			"ephemeral": inVol.VolumeDefinition,
		}
	case "emptyDir":
		extraParameters = map[string]interface{}{
			"emptyDir": map[string]string{},
		}
	case "csiSharedresourceInlineVolume":
		extraParameters = map[string]interface{}{
			"csi": inVol.VolumeDefinition,
		}
	default:
		extraParameters = map[string]interface{}{
			inVol.Kind: map[string]string{},
		}
	}
	err := applyResourceFromTemplateWithExtraParametersAsAdmin(oc, extraParameters, "--ignore-unknown-parameters=true", "-f", po.template, "-p", "PODNAME="+po.name, "PODNAMESPACE="+po.namespace, "PVCNAME="+po.pvcname, "PODIMAGE="+po.image, "VOLUMETYPE="+po.volumeType, "PATHTYPE="+po.pathType, "PODMOUNTPATH="+po.mountPath)
	o.Expect(err).ShouldNot(o.HaveOccurred())
}

// Create new pod with extra parameters
func (po *pod) createWithExtraParameters(oc *exutil.CLI, extraParameters map[string]interface{}) {
	if po.namespace == "" {
		po.namespace = oc.Namespace()
	}
	err := applyResourceFromTemplateWithExtraParametersAsAdmin(oc, extraParameters, "--ignore-unknown-parameters=true", "-f", po.template, "-p", "PODNAME="+po.name, "PODNAMESPACE="+po.namespace, "PVCNAME="+po.pvcname, "PODIMAGE="+po.image, "VOLUMETYPE="+po.volumeType, "PATHTYPE="+po.pathType, "PODMOUNTPATH="+po.mountPath)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Create new pod with multi extra parameters
func (po *pod) createWithMultiExtraParameters(oc *exutil.CLI, jsonPathsAndActions []map[string]string, multiExtraParameters []map[string]interface{}) (string, error) {
	if po.namespace == "" {
		po.namespace = oc.Namespace()
	}
	output, err := applyResourceFromTemplateWithMultiExtraParameters(oc, jsonPathsAndActions, multiExtraParameters, "--ignore-unknown-parameters=true", "-f", po.template, "-p", "PODNAME="+po.name, "PODNAMESPACE="+po.namespace, "PVCNAME="+po.pvcname, "PODIMAGE="+po.image, "VOLUMETYPE="+po.volumeType, "PATHTYPE="+po.pathType, "PODMOUNTPATH="+po.mountPath)
	if po.invalid {
		o.Expect(err).Should(o.HaveOccurred())
		return output, err
	}
	o.Expect(err).ShouldNot(o.HaveOccurred())
	return output, nil
}

// Create new pod with extra parameters for readonly
func (po *pod) createWithReadOnlyVolume(oc *exutil.CLI) {
	if po.namespace == "" {
		po.namespace = oc.Namespace()
	}
	extraParameters := map[string]interface{}{
		"jsonPath": `items.0.spec.containers.0.volumeMounts.0.`,
		"readOnly": true,
	}
	err := applyResourceFromTemplateWithExtraParametersAsAdmin(oc, extraParameters, "--ignore-unknown-parameters=true", "-f", po.template, "-p", "PODNAME="+po.name, "PODNAMESPACE="+po.namespace, "PVCNAME="+po.pvcname, "PODIMAGE="+po.image, "VOLUMETYPE="+po.volumeType, "PATHTYPE="+po.pathType, "PODMOUNTPATH="+po.mountPath)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Create new pod with subpath
func (po *pod) createWithSubpathVolume(oc *exutil.CLI, subPath string) {
	if po.namespace == "" {
		po.namespace = oc.Namespace()
	}
	extraParameters := map[string]interface{}{
		"jsonPath": `items.0.spec.containers.0.volumeMounts.0.`,
		"subPath":  subPath,
	}
	err := applyResourceFromTemplateWithExtraParametersAsAdmin(oc, extraParameters, "--ignore-unknown-parameters=true", "-f", po.template, "-p", "PODNAME="+po.name, "PODNAMESPACE="+po.namespace, "PVCNAME="+po.pvcname, "PODIMAGE="+po.image, "VOLUMETYPE="+po.volumeType, "PATHTYPE="+po.pathType, "PODMOUNTPATH="+po.mountPath)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Create new pod for security check
func (po *pod) createWithSecurity(oc *exutil.CLI) {
	seLevel := map[string]string{
		"level": "s0:c13,c2",
	}
	extraParameters := map[string]interface{}{
		"jsonPath":       `items.0.spec.securityContext.`,
		"seLinuxOptions": seLevel,
		"fsGroup":        24680,
		"runAsUser":      1000160000,
	}
	if po.namespace == "" {
		po.namespace = oc.Namespace()
	}
	err := applyResourceFromTemplateWithExtraParametersAsAdmin(oc, extraParameters, "--ignore-unknown-parameters=true", "-f", po.template, "-p", "PODNAME="+po.name, "PODNAMESPACE="+po.namespace, "PVCNAME="+po.pvcname, "PODIMAGE="+po.image, "VOLUMETYPE="+po.volumeType, "PATHTYPE="+po.pathType, "PODMOUNTPATH="+po.mountPath)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Create new pod with extra parameters for nodeSelector
func (po *pod) createWithNodeSelector(oc *exutil.CLI, labelName string, labelValue string) {
	extraParameters := map[string]interface{}{
		"jsonPath": `items.0.spec.nodeSelector.`,
		labelName:  labelValue,
	}
	if po.namespace == "" {
		po.namespace = oc.Namespace()
	}
	err := applyResourceFromTemplateWithExtraParametersAsAdmin(oc, extraParameters, "--ignore-unknown-parameters=true", "-f", po.template, "-p", "PODNAME="+po.name, "PODNAMESPACE="+po.namespace, "PVCNAME="+po.pvcname, "PODIMAGE="+po.image, "VOLUMETYPE="+po.volumeType, "PATHTYPE="+po.pathType, "PODMOUNTPATH="+po.mountPath)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Create new pod with extra parameters for nodeAffinity, key, operator and values should be provided in matchExpressions
func (po *pod) createWithNodeAffinity(oc *exutil.CLI, key string, operator string, values []string) {
	extraParameters := map[string]interface{}{
		"jsonPath": `items.0.spec.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms.0.matchExpressions.0.`,
		"key":      key,
		"operator": operator,
		"values":   values,
	}
	if po.namespace == "" {
		po.namespace = oc.Namespace()
	}
	err := applyResourceFromTemplateWithExtraParametersAsAdmin(oc, extraParameters, "--ignore-unknown-parameters=true", "-f", po.template, "-p", "PODNAME="+po.name, "PODNAMESPACE="+po.namespace, "PVCNAME="+po.pvcname, "PODIMAGE="+po.image, "VOLUMETYPE="+po.volumeType, "PATHTYPE="+po.pathType, "PODMOUNTPATH="+po.mountPath)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Delete the pod
func (po *pod) delete(oc *exutil.CLI) {
	err := oc.WithoutNamespace().Run("delete").Args("pod", po.name, "-n", po.namespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Force delete the pod
func (po *pod) forceDelete(oc *exutil.CLI) {
	err := oc.WithoutNamespace().Run("delete").Args("pod", po.name, "-n", po.namespace, "--force", "--grace-period=0").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Delete the pod use kubeadmin
func (po *pod) deleteAsAdmin(oc *exutil.CLI) {
	oc.WithoutNamespace().AsAdmin().Run("delete").Args("pod", po.name, "-n", po.namespace, "--ignore-not-found").Execute()
}

// Pod exec the bash CLI
func (po *pod) execCommand(oc *exutil.CLI, command string) (string, error) {
	return execCommandInSpecificPod(oc, po.namespace, po.name, command)
}

// Pod exec the bash CLI in specific container
func (po *pod) execCommandInSpecifiedContainer(oc *exutil.CLI, containerName string, command string) (string, error) {
	finalArgs := []string{"-n", po.namespace, po.name, "-c", containerName, "--", "/bin/sh", "-c", command}
	return oc.WithoutNamespace().Run("exec").Args(finalArgs...).Output()
}

// Pod exec the bash CLI with admin
func (po *pod) execCommandAsAdmin(oc *exutil.CLI, command string) (string, error) {
	command1 := []string{"-n", po.namespace, po.name, "--", "/bin/sh", "-c", command}
	msg, err := oc.WithoutNamespace().AsAdmin().Run("exec").Args(command1...).Output()
	if err != nil {
		e2e.Logf(po.name+"# "+command+" *failed with* :\"%v\".", err)
		return msg, err
	}
	debugLogf(po.name+"# "+command+" *Output is* :\"%s\".", msg)
	return msg, nil
}

// Check the pod mounted filesystem type volume could write data
func (po *pod) checkMountedVolumeCouldWriteData(oc *exutil.CLI, checkFlag bool) {
	_, err := execCommandInSpecificPod(oc, po.namespace, po.name, "echo \"storage test\" >"+po.mountPath+"/testfile")
	o.Expect(err == nil).Should(o.Equal(checkFlag))
	if err == nil && checkFlag {
		_, err = execCommandInSpecificPod(oc, po.namespace, po.name, "sync -f "+po.mountPath+"/testfile")
		o.Expect(err).NotTo(o.HaveOccurred())
	}
}

// Check the pod mounted volume could read and write
func (po *pod) checkMountedVolumeCouldRW(oc *exutil.CLI) {
	po.checkMountedVolumeCouldWriteData(oc, true)
	po.checkMountedVolumeDataExist(oc, true)
}

// Check the pod mounted volume could write in specific container
func (po *pod) checkMountedVolumeCouldWriteInSpecificContainer(oc *exutil.CLI, containerName string) {
	_, err := po.execCommandInSpecifiedContainer(oc, containerName, "echo \"storage test\" >"+po.mountPath+"/testfile")
	o.Expect(err).NotTo(o.HaveOccurred())
	_, err = po.execCommandInSpecifiedContainer(oc, containerName, "sync -f "+po.mountPath+"/testfile")
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Check the pod mounted volume could read from specific container
func (po *pod) checkMountedVolumeCouldReadFromSpecificContainer(oc *exutil.CLI, containerName string) {
	o.Expect(po.execCommandInSpecifiedContainer(oc, containerName, "cat "+po.mountPath+"/testfile")).To(o.ContainSubstring("storage test"))
}

// Check the pod mounted volume origin wrote data 'testfile' exist or not
func (po *pod) checkMountedVolumeDataExist(oc *exutil.CLI, checkFlag bool) {
	if checkFlag {
		o.Expect(execCommandInSpecificPod(oc, po.namespace, po.name, "cat "+po.mountPath+"/testfile")).To(o.ContainSubstring("storage test"))
	} else {
		output, err := execCommandInSpecificPod(oc, po.namespace, po.name, "cat "+po.mountPath+"/testfile")
		o.Expect(err).Should(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("No such file or directory"))
	}
}

// Check the pod mounted volume have exec right
func (po *pod) checkMountedVolumeHaveExecRight(oc *exutil.CLI) {
	if provisioner == "file.csi.azure.com" {
		o.Expect(execCommandInSpecificPod(oc, po.namespace, po.name, fmt.Sprintf("echo '#!/bin/bash\necho \"Hello OpenShift Storage\"' > %s", po.mountPath+"/hello"))).Should(o.Equal(""))
		o.Expect(execCommandInSpecificPod(oc, po.namespace, po.name, "ls -l "+po.mountPath+"/hello")).To(o.Or(o.ContainSubstring("-rwxrwxrwx"), (o.ContainSubstring("-rw-r--r--"))))
	} else {
		o.Expect(execCommandInSpecificPod(oc, po.namespace, po.name, fmt.Sprintf("echo '#!/bin/bash\necho \"Hello OpenShift Storage\"' > %s && chmod +x %s ", po.mountPath+"/hello", po.mountPath+"/hello"))).Should(o.Equal(""))
		o.Expect(execCommandInSpecificPod(oc, po.namespace, po.name, po.mountPath+"/hello")).To(o.ContainSubstring("Hello OpenShift Storage"))
	}
}

// Check the pod mounted volume could write data into raw block volume
func (po *pod) writeDataIntoRawBlockVolume(oc *exutil.CLI) {
	e2e.Logf("Writing the data into Raw Block volume")
	_, err := po.execCommand(oc, "/bin/dd  if=/dev/null of="+po.mountPath+" bs=512 count=1")
	o.Expect(err).NotTo(o.HaveOccurred())
	_, err = po.execCommand(oc, "echo 'storage test' > "+po.mountPath)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Check data in raw block volume could be read
func (po *pod) checkDataInRawBlockVolume(oc *exutil.CLI) {
	e2e.Logf("Check the data in Raw Block volume")
	_, err := po.execCommand(oc, "/bin/dd  if="+po.mountPath+" of=/tmp/testfile bs=512 count=1")
	o.Expect(err).NotTo(o.HaveOccurred())
	output, err := po.execCommand(oc, "cat /tmp/testfile")
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(output).To(o.ContainSubstring("storage test"))
}

func (po *pod) checkFsgroup(oc *exutil.CLI, command string, expect string) {
	output, err := po.execCommandAsAdmin(oc, command)
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(output).To(o.ContainSubstring(expect))
}

// longerTime changes po.maxWaitReadyTime to LongerMaxWaitingTime
// Used for some Longduration test
func (po *pod) longerTime() *pod {
	newPod := *po
	newPod.maxWaitReadyTime = longerMaxWaitingTime
	return &newPod
}

// longerTime changes po.maxWaitReadyTime to specifiedDuring max wait time
// Used for some Longduration test
func (po *pod) specifiedLongerTime(specifiedDuring time.Duration) *pod {
	newPod := *po
	newPod.maxWaitReadyTime = specifiedDuring
	return &newPod
}

// longestTime changes po.maxWaitReadyTime to longestMaxWaitingTime
// Used for some Longduration test
func (po *pod) longestTime() *pod {
	newPod := *po
	newPod.maxWaitReadyTime = longestMaxWaitingTime
	return &newPod
}

// Waiting for the Pod ready
func (po *pod) waitReady(oc *exutil.CLI) {
	err := wait.Poll(po.maxWaitReadyTime/defaultIterationTimes, po.maxWaitReadyTime, func() (bool, error) {
		status, err1 := checkPodReady(oc, po.namespace, po.name)
		if err1 != nil {
			e2e.Logf("the err:%v, wait for pod %v to become ready.", err1, po.name)
			return status, err1
		}
		if !status {
			return status, nil
		}
		e2e.Logf("Pod: \"%s\" is running on the node: \"%s\"", po.name, getNodeNameByPod(oc, po.namespace, po.name))
		return status, nil
	})

	if err != nil {
		podDescribe := describePod(oc, po.namespace, po.name)
		e2e.Logf("oc describe pod %s:\n%s", po.name, podDescribe)
		describePersistentVolumeClaim(oc, po.namespace, po.pvcname)
	}
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("pod %s not ready", po.name))
}

// Get the pod mount filesystem type volume size by df command
func (po *pod) getPodMountFsVolumeSize(oc *exutil.CLI) int64 {
	sizeString, err := po.execCommand(oc, "df -BG|grep "+po.mountPath+"|awk '{print $2}'")
	o.Expect(err).NotTo(o.HaveOccurred())
	sizeInt64, err := strconv.ParseInt(strings.TrimSuffix(sizeString, "G"), 10, 64)
	o.Expect(err).NotTo(o.HaveOccurred())
	return sizeInt64
}

// GetValueByJSONPath gets the specified JSONPath value of pod
func (po *pod) getValueByJSONPath(oc *exutil.CLI, jsonPath string) (string, error) {
	return oc.WithoutNamespace().AsAdmin().Run("get").Args("-n", po.namespace, "pod/"+po.name, "-o", "jsonpath="+jsonPath).Output()
}

// GetUID gets the pod uid
func (po *pod) getUID(oc *exutil.CLI) string {
	podUID, getErr := po.getValueByJSONPath(oc, "{.metadata.uid}")
	o.Expect(getErr).NotTo(o.HaveOccurred(), fmt.Sprintf("Failed to get pod %q uid\n%v", po.name, getErr))
	e2e.Logf("Pod %q uid is: %q", po.name, podUID)
	return podUID
}

// Check Pod status consistently
func (po *pod) checkStatusConsistently(oc *exutil.CLI, status string, waitTime time.Duration) {
	o.Consistently(func() string {
		podStatus, _ := getPodStatus(oc, po.namespace, po.name)
		return podStatus
	}, waitTime*time.Second, 5*time.Second).Should(o.ContainSubstring(status))
}

// Check Pod status eventually, minimum waitTime required is 20 seconds
func (po *pod) checkStatusEventually(oc *exutil.CLI, status string, waitTime time.Duration) {
	o.Eventually(func() string {
		podStatus, _ := getPodStatus(oc, po.namespace, po.name)
		return podStatus
	}, waitTime*time.Second, waitTime*time.Second/20).Should(o.ContainSubstring(status))
}

// Get the phase, status of specified pod
func getPodStatus(oc *exutil.CLI, namespace string, podName string) (string, error) {
	podStatus, err := oc.WithoutNamespace().Run("get").Args("pod", "-n", namespace, podName, "-o=jsonpath={.status.phase}").Output()
	e2e.Logf("The pod  %s status in namespace %s is %q", podName, namespace, podStatus)
	return podStatus, err
}

// Check the pod status becomes ready, status is "Running", "Ready" or "Complete"
func checkPodReady(oc *exutil.CLI, namespace string, podName string) (bool, error) {
	podOutPut, err := getPodStatus(oc, namespace, podName)
	status := []string{"Running", "Ready", "Complete"}
	return contains(status, podOutPut), err
}

// Get the detail info of specified pod
func describePod(oc *exutil.CLI, namespace string, podName string) string {
	podDescribe, err := oc.WithoutNamespace().Run("describe").Args("pod", "-n", namespace, podName).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return podDescribe
}

// Waiting for the pod becomes ready, such as "Running", "Ready", "Complete"
func waitPodReady(oc *exutil.CLI, namespace string, podName string) {
	err := wait.Poll(defaultMaxWaitingTime/defaultIterationTimes, defaultMaxWaitingTime, func() (bool, error) {
		status, err1 := checkPodReady(oc, namespace, podName)
		if err1 != nil {
			e2e.Logf("the err:%v, wait for pod %v to become ready.", err1, podName)
			return status, err1
		}
		if !status {
			return status, nil
		}
		return status, nil
	})

	if err != nil {
		podDescribe := describePod(oc, namespace, podName)
		e2e.Logf("oc describe pod %v.", podName)
		e2e.Logf(podDescribe)
	}
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("pod %s not ready", podName))
}

// Specified pod exec the bash CLI
// If failed execute will retry 3 times, because of the network instability or other action cause the pod recreate flake.
// Flake info : "error: unable to upgrade connection: container not found"  It maybe the container suddenly crashed.
func execCommandInSpecificPod(oc *exutil.CLI, namespace string, podName string, command string) (output string, errInfo error) {
	command1 := []string{"-n", namespace, podName, "--", "/bin/sh", "-c", command}
	wait.Poll(5*time.Second, 15*time.Second, func() (bool, error) {
		output, errInfo = oc.WithoutNamespace().Run("exec").Args(command1...).Output()
		if errInfo != nil {
			e2e.Logf(podName+"# "+command+" *failed with* :\"%v\".", errInfo)
			// Retry to avoid system issues
			if strings.Contains(errInfo.Error(), "unable to upgrade connection: container not found") ||
				strings.Contains(errInfo.Error(), "Error from server: error dialing backend: EOF") ||
				strings.Contains(output, "Resource temporarily unavailable") ||
				strings.Contains(errInfo.Error(), "error dialing backend") {
				e2e.Logf(`Pod %q executed %q failed with:\n%v\n try again ...`, podName, command, errInfo)
				return false, nil
			}
			return false, errInfo
		}
		e2e.Logf(podName+"# "+command+" *Output is* :\"%s\".", output)
		return true, nil
	})

	return
}

// Wait for pods selected with selector name to be removed
func waitUntilPodsAreGoneByLabel(oc *exutil.CLI, namespace string, labelName string) {
	err := wait.Poll(defaultMaxWaitingTime/defaultIterationTimes, defaultMaxWaitingTime, func() (bool, error) {
		output, err := oc.WithoutNamespace().Run("get").Args("pods", "-l", labelName, "-n", namespace).Output()
		if err != nil {
			return false, err
		}
		if strings.Contains(output, "No resources found") {
			e2e.Logf(fmt.Sprintf("%v", output))
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Error waiting for pods to be removed using labelName  %s", labelName))
}

// Get the pod details
func getPodDetailsByLabel(oc *exutil.CLI, namespace string, labelName string) (string, error) {
	output, err := oc.WithoutNamespace().Run("get").Args("pods", "-l", labelName, "-n", namespace).Output()
	if err != nil {
		e2e.Logf("Get pod details failed with  err:%v .", err)
		return output, err
	}
	e2e.Logf("Get pod details output is:\"%v\"", output)
	return output, nil
}

// Get the pods List by label
func getPodsListByLabel(oc *exutil.CLI, namespace string, selectorLabel string) ([]string, error) {
	podsOp, err := oc.WithoutNamespace().Run("get").Args("pod", "-n", namespace, "-l", selectorLabel, "-o=jsonpath={.items[*].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return strings.Fields(podsOp), err
}

// Get the pods List by namespace
func getPodsListByNamespace(oc *exutil.CLI, namespace string) []string {
	podsOp, err := oc.WithoutNamespace().Run("get").Args("pod", "-n", namespace, "-o=jsonpath={.items[*].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return strings.Fields(podsOp)
}

// Get the pods List by keyword and namespace
func getPodsListByKeyword(oc *exutil.CLI, namespace string, keyword string) []string {
	cmd := fmt.Sprintf(`oc get pod -n %v -o custom-columns=POD:.metadata.name --no-headers | grep %v`, namespace, keyword)
	podlist, err := exec.Command("bash", "-c", cmd).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return strings.Fields(string(podlist))
}

// Get the pvcName from the pod
func getPvcNameFromPod(oc *exutil.CLI, podName string, namespace string) string {
	pvcName, err := oc.WithoutNamespace().Run("get").Args("pod", podName, "-n", namespace, "-o=jsonpath={.spec.volumes[*].persistentVolumeClaim.claimName}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return pvcName
}

// Get the pod status by label, Checking status for n numbers of deployments
func checkPodStatusByLabel(oc *exutil.CLI, namespace string, selectorLabel string, expectedstatus string) {
	var podDescribe string
	var pvcList []string
	podsList, _ := getPodsListByLabel(oc, namespace, selectorLabel)
	e2e.Logf("PodLabelName \"%s\", expected status is \"%s\", podsList=%s", selectorLabel, expectedstatus, podsList)
	err := wait.Poll(defaultMaxWaitingTime/defaultIterationTimes, defaultMaxWaitingTime, func() (bool, error) {
		podflag := 0
		for _, podName := range podsList {
			podstatus, err := oc.WithoutNamespace().Run("get").Args("pod", podName, "-n", namespace, "-o=jsonpath={.status.phase}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if matched, _ := regexp.MatchString(expectedstatus, podstatus); !matched {
				podDescribe = describePod(oc, namespace, podName)
				pvcList = append(pvcList, getPvcNameFromPod(oc, podName, namespace))
				podflag = 1
			}
		}
		if podflag == 1 {
			return false, nil
		}
		e2e.Logf("%s is with expected status: \"%s\"", podsList, expectedstatus)
		return true, nil
	})
	if err != nil && podDescribe != "" {
		e2e.Logf(podDescribe)
		for _, pvcName := range pvcList {
			describePersistentVolumeClaim(oc, oc.Namespace(), pvcName)
		}
	}
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("pod with label %s not ready", selectorLabel))
}

// Specified pod exec the bash CLI
func execCommandInSpecificPodWithLabel(oc *exutil.CLI, namespace string, labelName string, command string) (string, error) {
	podsList, err := getPodsListByLabel(oc, namespace, labelName)
	e2e.Logf("Pod List is %s.", podsList)
	podflag := 0
	var data, podDescribe string
	for _, pod := range podsList {
		msg, err := execCommandInSpecificPod(oc, namespace, pod, command)
		if err != nil {
			e2e.Logf("Execute command failed with  err: %v.", err)
			podDescribe = describePod(oc, namespace, pod)
			podflag = 1
		} else {
			e2e.Logf("Executed \"%s\" on pod \"%s\" result: %s", command, pod, msg)
			data = msg
		}
	}
	if podflag == 0 {
		e2e.Logf("Executed commands on Pods labeled %s successfully", labelName)
		return data, nil
	}
	if err != nil && podDescribe != "" {
		e2e.Logf(podDescribe)
	}
	o.Expect(err).NotTo(o.HaveOccurred())
	return strings.Join(podsList, " "), err
}

// Deployment workload related functions
type deployment struct {
	name             string
	namespace        string
	replicasno       string
	applabel         string
	mpath            string
	pvcname          string
	template         string
	volumetype       string
	typepath         string
	maxWaitReadyTime time.Duration
}

// function option mode to change the default value of deployment parameters,eg. name, replicasno, mpath
type deployOption func(*deployment)

// Replace the default value of Deployment name parameter
func setDeploymentName(name string) deployOption {
	return func(this *deployment) {
		this.name = name
	}
}

// Replace the default value of Deployment template parameter
func setDeploymentTemplate(template string) deployOption {
	return func(this *deployment) {
		this.template = template
	}
}

// Replace the default value of Deployment namespace parameter
func setDeploymentNamespace(namespace string) deployOption {
	return func(this *deployment) {
		this.namespace = namespace
	}
}

// Replace the default value of Deployment replicasno parameter
func setDeploymentReplicasNumber(replicasno string) deployOption {
	return func(this *deployment) {
		this.replicasno = replicasno
	}
}

// Replace the default value of Deployment app label
func setDeploymentApplabel(applabel string) deployOption {
	return func(this *deployment) {
		this.applabel = applabel
	}
}

// Replace the default value of Deployment mountpath parameter
func setDeploymentMountpath(mpath string) deployOption {
	return func(this *deployment) {
		this.mpath = mpath
	}
}

// Replace the default value of Deployment pvcname parameter
func setDeploymentPVCName(pvcname string) deployOption {
	return func(this *deployment) {
		this.pvcname = pvcname
	}
}

// Replace the default value of Deployment volume type parameter
func setDeploymentVolumeType(volumetype string) deployOption {
	return func(this *deployment) {
		this.volumetype = volumetype
	}
}

// Replace the default value of Deployment volume type path parameter
func setDeploymentVolumeTypePath(typepath string) deployOption {
	return func(this *deployment) {
		this.typepath = typepath
	}
}

// Replace the default value of Deployment replicas number
func setDeploymentReplicasNo(replicasno string) deployOption {
	return func(this *deployment) {
		this.replicasno = replicasno
	}
}

// Replace the default value of Deployment maximum Wait Ready Time
func setDeploymentMaxWaitReadyTime(maxWaitReadyTime time.Duration) deployOption {
	return func(this *deployment) {
		this.maxWaitReadyTime = maxWaitReadyTime
	}
}

// Create a new customized Deployment object
func newDeployment(opts ...deployOption) deployment {
	defaultMaxWaitReadyTime := defaultMaxWaitingTime
	if provisioner == "filestore.csi.storage.gke.io" {
		defaultMaxWaitReadyTime = longerMaxWaitingTime
	}
	defaultDeployment := deployment{
		name:             "my-dep-" + getRandomString(),
		template:         "dep-template.yaml",
		namespace:        "",
		replicasno:       "1",
		applabel:         "myapp-" + getRandomString(),
		mpath:            "/mnt/storage",
		pvcname:          "",
		volumetype:       "volumeMounts",
		typepath:         "mountPath",
		maxWaitReadyTime: defaultMaxWaitReadyTime,
	}

	for _, o := range opts {
		o(&defaultDeployment)
	}

	return defaultDeployment
}

// Create new Deployment with customized parameters
func (dep *deployment) create(oc *exutil.CLI) {
	if dep.namespace == "" {
		dep.namespace = oc.Namespace()
	}
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", dep.template, "-p", "DNAME="+dep.name, "DNAMESPACE="+dep.namespace, "PVCNAME="+dep.pvcname, "REPLICASNUM="+dep.replicasno, "DLABEL="+dep.applabel, "MPATH="+dep.mpath, "VOLUMETYPE="+dep.volumetype, "TYPEPATH="+dep.typepath)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Create new Deployment with extra parameters
func (dep *deployment) createWithExtraParameters(oc *exutil.CLI, extraParameters map[string]interface{}) {
	if dep.namespace == "" {
		dep.namespace = oc.Namespace()
	}
	err := applyResourceFromTemplateWithExtraParametersAsAdmin(oc, extraParameters, "--ignore-unknown-parameters=true", "-f", dep.template, "-p", "DNAME="+dep.name, "DNAMESPACE="+dep.namespace, "PVCNAME="+dep.pvcname, "REPLICASNUM="+dep.replicasno, "DLABEL="+dep.applabel, "MPATH="+dep.mpath, "VOLUMETYPE="+dep.volumetype, "TYPEPATH="+dep.typepath)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Create new Deployment with multi extra parameters
func (dep *deployment) createWithMultiExtraParameters(oc *exutil.CLI, jsonPathsAndActions []map[string]string, multiExtraParameters []map[string]interface{}) {
	if dep.namespace == "" {
		dep.namespace = oc.Namespace()
	}
	_, err := applyResourceFromTemplateWithMultiExtraParameters(oc, jsonPathsAndActions, multiExtraParameters, "--ignore-unknown-parameters=true", "-f", dep.template, "-p", "DNAME="+dep.name, "DNAMESPACE="+dep.namespace, "PVCNAME="+dep.pvcname, "REPLICASNUM="+dep.replicasno, "DLABEL="+dep.applabel, "MPATH="+dep.mpath, "VOLUMETYPE="+dep.volumetype, "TYPEPATH="+dep.typepath)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Create new Deployment with InlineVolume
func (dep *deployment) createWithInlineVolume(oc *exutil.CLI, inVol InlineVolume) {
	o.Expect(dep.createWithInlineVolumeWithOutAssert(oc, inVol)).Should(o.ContainSubstring("created"))
}

// Create new Deployment with InlineVolume without assert returns msg and error info
func (dep *deployment) createWithInlineVolumeWithOutAssert(oc *exutil.CLI, inVol InlineVolume) (string, error) {
	if dep.namespace == "" {
		dep.namespace = oc.Namespace()
	}
	var (
		extraParameters map[string]interface{}
		jsonPath        = `items.0.spec.template.spec.volumes.0.`
	)
	switch inVol.Kind {
	case "genericEphemeralVolume", "csiEphemeralVolume":
		extraParameters = map[string]interface{}{
			"ephemeral": inVol.VolumeDefinition,
		}
	case "emptyDir":
		extraParameters = map[string]interface{}{
			"emptyDir": map[string]string{},
		}
	case "csiSharedresourceInlineVolume":
		extraParameters = map[string]interface{}{
			"csi": inVol.VolumeDefinition,
		}
	default:
		extraParameters = map[string]interface{}{
			inVol.Kind: map[string]string{},
		}
	}
	return applyResourceFromTemplateWithMultiExtraParameters(oc, []map[string]string{{jsonPath: "set"}}, []map[string]interface{}{extraParameters}, "--ignore-unknown-parameters=true", "-f", dep.template, "-p", "DNAME="+dep.name, "DNAMESPACE="+dep.namespace, "PVCNAME="+dep.pvcname, "REPLICASNUM="+dep.replicasno, "DLABEL="+dep.applabel, "MPATH="+dep.mpath, "VOLUMETYPE="+dep.volumetype, "TYPEPATH="+dep.typepath)
}

// Create new deployment with extra parameters for topologySpreadConstraints
func (dep *deployment) createWithTopologySpreadConstraints(oc *exutil.CLI) {
	if dep.namespace == "" {
		dep.namespace = oc.Namespace()
	}
	matchLabels := map[string]interface{}{
		"app": dep.applabel,
	}
	labelSelector := map[string]interface{}{
		"matchLabels": matchLabels,
	}
	extraParameters := map[string]interface{}{
		"jsonPath":          `items.0.spec.template.spec.topologySpreadConstraints.0.`,
		"maxSkew":           1,
		"topologyKey":       "kubernetes.io/hostname",
		"whenUnsatisfiable": "DoNotSchedule",
		"labelSelector":     labelSelector,
	}
	dep.createWithExtraParameters(oc, extraParameters)
}

// Create new deployment with extra parameters for nodeSelector
func (dep *deployment) createWithNodeSelector(oc *exutil.CLI, labelName string, labelValue string) {
	if dep.namespace == "" {
		dep.namespace = oc.Namespace()
	}
	extraParameters := map[string]interface{}{
		"jsonPath": `items.0.spec.template.spec.nodeSelector.`,
		labelName:  labelValue,
	}
	err := applyResourceFromTemplateWithExtraParametersAsAdmin(oc, extraParameters, "--ignore-unknown-parameters=true", "-f", dep.template, "-p", "DNAME="+dep.name, "DNAMESPACE="+dep.namespace, "PVCNAME="+dep.pvcname, "REPLICASNUM="+dep.replicasno, "DLABEL="+dep.applabel, "MPATH="+dep.mpath, "VOLUMETYPE="+dep.volumetype, "TYPEPATH="+dep.typepath)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Create new deployment with extra parameters for nodeAffinity, key, operator and values should be provided in matchExpressions
func (dep *deployment) createWithNodeAffinity(oc *exutil.CLI, key string, operator string, values []string) {
	if dep.namespace == "" {
		dep.namespace = oc.Namespace()
	}
	extraParameters := map[string]interface{}{
		"jsonPath": `items.0.spec.template.spec.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms.0.matchExpressions.0.`,
		"key":      key,
		"operator": operator,
		"values":   values,
	}
	err := applyResourceFromTemplateWithExtraParametersAsAdmin(oc, extraParameters, "--ignore-unknown-parameters=true", "-f", dep.template, "-p", "DNAME="+dep.name, "DNAMESPACE="+dep.namespace, "PVCNAME="+dep.pvcname, "REPLICASNUM="+dep.replicasno, "DLABEL="+dep.applabel, "MPATH="+dep.mpath, "VOLUMETYPE="+dep.volumetype, "TYPEPATH="+dep.typepath)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Delete Deployment from the namespace
func (dep *deployment) delete(oc *exutil.CLI) {
	err := oc.WithoutNamespace().Run("delete").Args("deployment", dep.name, "-n", dep.namespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Delete Deployment from the namespace
func (dep *deployment) deleteAsAdmin(oc *exutil.CLI) {
	oc.WithoutNamespace().AsAdmin().Run("delete").Args("deployment", dep.name, "-n", dep.namespace, "--ignore-not-found").Execute()
}

// Get deployment 'Running' pods list
func (dep *deployment) getPodList(oc *exutil.CLI) (podList []string) {
	selectorLabel := dep.applabel
	if !strings.Contains(dep.applabel, "=") {
		selectorLabel = "app=" + dep.applabel
	}
	dep.replicasno = dep.getReplicasNum(oc)
	o.Eventually(func() bool {
		podListStr, _ := oc.WithoutNamespace().Run("get").Args("pod", "-n", dep.namespace, "-l", selectorLabel, "-o=jsonpath={.items[?(@.status.phase==\"Running\")].metadata.name}").Output()
		podList = strings.Fields(podListStr)
		return strings.EqualFold(fmt.Sprint(len(podList)), dep.replicasno)
	}, 120*time.Second, 5*time.Second).Should(o.BeTrue(), fmt.Sprintf("Failed to get deployment %s's ready podlist", dep.name))
	e2e.Logf("Deployment/%s's ready podlist is: %v", dep.name, podList)
	return
}

// Get deployment pods list without filter pods 'Running' status
func (dep *deployment) getPodListWithoutFilterStatus(oc *exutil.CLI) (podList []string) {
	selectorLabel := dep.applabel
	if !strings.Contains(dep.applabel, "=") {
		selectorLabel = "app=" + dep.applabel
	}
	podListStr, getPodListErr := oc.WithoutNamespace().Run("get").Args("pod", "-n", dep.namespace, "-l", selectorLabel, "-o=jsonpath={.items[*].metadata.name}").Output()
	o.Expect(getPodListErr).NotTo(o.HaveOccurred(), fmt.Sprintf("Failed to get deployment %s's podlist", dep.name))
	podList = strings.Fields(podListStr)
	e2e.Logf("Deployment/%s's podlist is: %v", dep.name, podList)
	return
}

// Get ReplicasNum of the Deployment
func (dep *deployment) getReplicasNum(oc *exutil.CLI) string {
	replicasNum, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", dep.name, "-n", dep.namespace, "-o", "jsonpath={.spec.replicas}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return replicasNum
}

// Get names of PVC used by the Deployment
func (dep *deployment) getPVCNames(oc *exutil.CLI) []string {
	pvcNames, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", dep.name, "-n", dep.namespace, "-o", "jsonpath={.spec.template.spec.volumes.*.persistentVolumeClaim.claimName}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return strings.Fields(pvcNames)
}

// Get the Deployments in mentioned ns
func getSpecifiedNamespaceDeployments(oc *exutil.CLI, ns string) []string {
	depNames, err := oc.WithoutNamespace().Run("get").Args("deployments", "-n", ns, "-o=jsonpath={range.items[*]}{.metadata.name}{\" \"}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if len(depNames) == 0 {
		return []string{}
	}
	return strings.Split(depNames, " ")
}

// Scale Replicas for the Deployment
func (dep *deployment) scaleReplicas(oc *exutil.CLI, replicasno string) {
	err := oc.WithoutNamespace().Run("scale").Args("deployment", dep.name, "--replicas="+replicasno, "-n", dep.namespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	dep.replicasno = replicasno
}

// GetSpecifiedJSONPathValue gets the specified jsonpath value of the Deployment
func (dep *deployment) getSpecifiedJSONPathValue(oc *exutil.CLI, jsonPath string) string {
	value, getValueErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", dep.name, "-n", dep.namespace, "-o", fmt.Sprintf("jsonpath=%s", jsonPath)).Output()
	o.Expect(getValueErr).NotTo(o.HaveOccurred())
	e2e.Logf(`Deployment/%s jsonPath->"%s" value is %s`, dep.name, jsonPath, value)
	return value
}

// pollGetSpecifiedJSONPathValue gets the specified jsonpath value of the Deployment satisfy the Eventually check
func (dep *deployment) pollGetSpecifiedJSONPathValue(oc *exutil.CLI, jsonPath string) func() string {
	return func() string {
		return dep.getSpecifiedJSONPathValue(oc, jsonPath)
	}
}

// Restart the Deployment by rollout restart
func (dep *deployment) restart(oc *exutil.CLI) {
	resourceVersionOri := dep.getSpecifiedJSONPathValue(oc, "{.metadata.resourceVersion}")
	readyPodListOri := dep.getPodList(oc)
	err := oc.WithoutNamespace().Run("rollout").Args("-n", dep.namespace, "restart", "deployment", dep.name).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Eventually(func() bool {
		currentResourceVersion := dep.getSpecifiedJSONPathValue(oc, "{.metadata.resourceVersion}")
		currentReadyPodList := dep.getPodList(oc)
		return currentResourceVersion != resourceVersionOri && len(sliceIntersect(readyPodListOri, currentReadyPodList)) == 0
	}).WithTimeout(defaultMaxWaitingTime).WithPolling(defaultMaxWaitingTime/defaultIterationTimes).Should(o.BeTrue(), fmt.Sprintf("deployment %q restart failed", dep.name))
	dep.waitReady(oc)
	e2e.Logf("deployment/%s in namespace %s restart successfully", dep.name, dep.namespace)
}

// Hard restart the Deployment by deleting it's pods
func (dep *deployment) hardRestart(oc *exutil.CLI) {
	err := oc.WithoutNamespace().Run("delete").Args("-n", dep.namespace, "pod", "-l", dep.applabel).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	dep.waitReady(oc)
	e2e.Logf("deployment/%s in namespace %s hard restart successfully", dep.name, dep.namespace)
}

// Check the deployment ready
func (dep *deployment) checkReady(oc *exutil.CLI) (bool, error) {
	dep.replicasno = dep.getReplicasNum(oc)
	readyReplicas, err := oc.WithoutNamespace().Run("get").Args("deployment", dep.name, "-n", dep.namespace, "-o", "jsonpath={.status.availableReplicas}").Output()
	if err != nil {
		return false, err
	}
	if dep.replicasno == "0" && readyReplicas == "" {
		readyReplicas = "0"
	}
	return strings.EqualFold(dep.replicasno, readyReplicas), nil
}

// Describe the deployment
func (dep *deployment) describe(oc *exutil.CLI) string {
	deploymentDescribe, err := oc.WithoutNamespace().Run("describe").Args("deployment", dep.name, "-n", dep.namespace).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return deploymentDescribe
}

// longerTime changes dep.maxWaitReadyTime to LongerMaxWaitingTime
// Used for some Longduration test
func (dep *deployment) longerTime() *deployment {
	newDep := *dep
	newDep.maxWaitReadyTime = longerMaxWaitingTime
	return &newDep
}

// longerTime changes dep.maxWaitReadyTime to specifiedDuring max wait time
// Used for some Longduration test
func (dep *deployment) specifiedLongerTime(specifiedDuring time.Duration) *deployment {
	newDep := *dep
	newDep.maxWaitReadyTime = specifiedDuring
	return &newDep
}

// Waiting the deployment become ready
func (dep *deployment) waitReady(oc *exutil.CLI) {
	err := wait.Poll(dep.maxWaitReadyTime/defaultIterationTimes, dep.maxWaitReadyTime, func() (bool, error) {
		deploymentReady, err := dep.checkReady(oc)
		if err != nil {
			return deploymentReady, err
		}
		if !deploymentReady {
			return deploymentReady, nil
		}
		e2e.Logf(dep.name + " availableReplicas is as expected")
		return deploymentReady, nil
	})

	if err != nil {
		podsNames := dep.getPodListWithoutFilterStatus(oc)
		if len(podsNames) > 0 {
			for _, podName := range podsNames {
				e2e.Logf("$ oc describe pod %s:\n%s", podName, describePod(oc, dep.namespace, podName))
			}
		} else {
			e2e.Logf("The deployment/%s currently has no pods scheduled", dep.name)
		}
		// When the deployment with persistVolumeClaim and not ready describe the persistVolumeClaim detail
		if dep.pvcname != "" {
			describePersistentVolumeClaim(oc, dep.namespace, dep.pvcname)
		}
	}
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Deployment %s not ready", dep.name))
}

// Check the deployment mounted volume could read and write
func (dep *deployment) checkPodMountedVolumeCouldRW(oc *exutil.CLI) {
	for _, podinstance := range dep.getPodList(oc) {
		content := fmt.Sprintf(`"storage test %v"`, getRandomString())
		randomFileName := "/testfile_" + getRandomString()
		_, err := execCommandInSpecificPod(oc, dep.namespace, podinstance, "echo "+content+">"+dep.mpath+randomFileName)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(execCommandInSpecificPod(oc, dep.namespace, podinstance, "cat "+dep.mpath+randomFileName)).To(o.Equal(strings.Replace(content, "\"", "", 2)))
	}
}

// Check whether the deployment pod mounted volume orgin written data exist
func (dep *deployment) checkPodMountedVolumeDataExist(oc *exutil.CLI, checkFlag bool) {
	if checkFlag {
		o.Expect(execCommandInSpecificPod(oc, dep.namespace, dep.getPodList(oc)[0], "cat "+dep.mpath+"/testfile_*")).To(o.ContainSubstring("storage test"))
	} else {
		output, err := execCommandInSpecificPod(oc, dep.namespace, dep.getPodList(oc)[0], "cat "+dep.mpath+"/testfile_*")
		o.Expect(err).Should(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("No such file or directory"))
	}
}

// Check the deployment mounted volume have exec right
func (dep *deployment) checkPodMountedVolumeHaveExecRight(oc *exutil.CLI) {
	for _, podinstance := range dep.getPodList(oc) {
		if provisioner == "file.csi.azure.com" {
			o.Expect(execCommandInSpecificPod(oc, dep.namespace, podinstance, fmt.Sprintf("echo '#!/bin/bash\necho \"Hello OpenShift Storage\"' > %s", dep.mpath+"/hello"))).Should(o.Equal(""))
			o.Expect(execCommandInSpecificPod(oc, dep.namespace, podinstance, "ls -l "+dep.mpath+"/hello")).To(o.Or(o.ContainSubstring("-rwxrwxrwx"), (o.ContainSubstring("-rw-r--r--"))))
		} else {
			o.Expect(execCommandInSpecificPod(oc, dep.namespace, podinstance, fmt.Sprintf("echo '#!/bin/bash\necho \"Hello OpenShift Storage\"' > %s && chmod +x %s ", dep.mpath+"/hello", dep.mpath+"/hello"))).Should(o.Equal(""))
			o.Expect(execCommandInSpecificPod(oc, dep.namespace, podinstance, dep.mpath+"/hello")).To(o.ContainSubstring("Hello OpenShift Storage"))
		}
	}
}

// Check the deployment mounted volume type
func (dep *deployment) checkPodMountedVolumeContain(oc *exutil.CLI, content string) {
	for _, podinstance := range dep.getPodList(oc) {
		output, err := execCommandInSpecificPod(oc, dep.namespace, podinstance, "mount | grep "+dep.mpath)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring(content))
	}
}

// Write data in block level
func (dep *deployment) writeDataBlockType(oc *exutil.CLI) {
	e2e.Logf("Writing the data as Block level")
	_, err := execCommandInSpecificPod(oc, dep.namespace, dep.getPodList(oc)[0], "/bin/dd  if=/dev/null of="+dep.mpath+" bs=512 count=1")
	o.Expect(err).NotTo(o.HaveOccurred())
	_, err = execCommandInSpecificPod(oc, dep.namespace, dep.getPodList(oc)[0], `echo "block-data" > `+dep.mpath)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Check data written
func (dep *deployment) checkDataBlockType(oc *exutil.CLI) {
	_, err := execCommandInSpecificPod(oc, dep.namespace, dep.getPodList(oc)[0], "/bin/dd if="+dep.mpath+" of=/tmp/testfile bs=512 count=1")
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(execCommandInSpecificPod(oc, dep.namespace, dep.getPodList(oc)[0], "cat /tmp/testfile")).To(o.ContainSubstring("block-data"))
}

// Get deployment all replicas logs by filter
func (dep *deployment) getLogs(oc *exutil.CLI, filterArgs ...string) string {
	finalArgs := append([]string{"-n", dep.namespace, "-l", dep.applabel}, filterArgs...)
	depLogs, err := oc.WithoutNamespace().Run("logs").Args(finalArgs...).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf(`Get deployment/%s logs with "--selector=%s" successfully, additional options is %v`, dep.name, dep.applabel, filterArgs)
	debugLogf("Log details are:\n%s", depLogs)
	return depLogs
}

// Add the volume to existing deployment via setVolume command and wait till it reaches to Running state
func (dep *deployment) setVolumeAdd(oc *exutil.CLI, mPath string, volName string, claimName string) {
	msg, err := oc.WithoutNamespace().Run("set").Args("volumes", "deployment", dep.name, "--add", "-t", "persistentVolumeClaim", "-m", mPath, "--name", volName, "--claim-name", claimName, "-n", oc.Namespace()).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(msg).To(o.ContainSubstring("volume updated"))
	dep.waitReady(oc)
}

// Function to delete the project
func deleteProjectAsAdmin(oc *exutil.CLI, namespace string) {
	_, err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("project", namespace).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	err = wait.Poll(defaultMaxWaitingTime/defaultIterationTimes, defaultMaxWaitingTime, func() (bool, error) {
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("project", namespace).Output()
		if strings.Contains(output, "not found") {
			e2e.Logf("Project %s got deleted successfully", namespace)
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("The Project \"%s\" did not get deleted within the time period", namespace))
}

// Function to return the command combinations based on resourceName, namespace
func getCommandCombinations(oc *exutil.CLI, resourceType string, resourceName string, namespace string) []string {
	var command []string
	if resourceName != "" && namespace != "" {
		command = []string{resourceType, resourceName, "-n", namespace}
	}
	if resourceName != "" && namespace == "" {
		command = []string{resourceType, resourceName}
	}
	if resourceName == "" && namespace != "" {
		command = []string{resourceType, "--all", "-n", namespace}
	}
	if resourceName == "" && namespace == "" {
		command = []string{resourceType, "--all"}
	}
	return command
}

// Function to check the resources exists or no
func checkResourcesNotExist(oc *exutil.CLI, resourceType string, resourceName string, namespace string) {
	command := deleteElement(getCommandCombinations(oc, resourceType, resourceName, namespace), "--all")
	err := wait.Poll(defaultMaxWaitingTime/defaultIterationTimes, defaultMaxWaitingTime, func() (bool, error) {
		output, _ := oc.WithoutNamespace().Run("get").Args(command...).Output()
		if strings.Contains(output, "not found") && namespace != "" {
			e2e.Logf("No %s resource exists in the namespace %s", resourceType, namespace)
			return true, nil
		}
		if (strings.Contains(output, "not found") || strings.Contains(output, "No resources found")) && namespace == "" {
			e2e.Logf("No %s resource exists", resourceType)
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("The Resources %s still exists in the namespace %s", resourceType, namespace))
}

// Function to delete the resources ex: dep, pvc, pod, sts, ds
func deleteSpecifiedResource(oc *exutil.CLI, resourceType string, resourceName string, namespace string) {
	command := getCommandCombinations(oc, resourceType, resourceName, namespace)
	command = append(command, "--ignore-not-found")
	_, err := oc.WithoutNamespace().Run("delete").Args(command...).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	checkResourcesNotExist(oc, resourceType, resourceName, namespace)
}

// Statefulset workload related functions
type statefulset struct {
	name             string
	namespace        string
	replicasno       string
	applabel         string
	mpath            string
	pvcname          string
	template         string
	volumetype       string
	typepath         string
	capacity         string
	scname           string
	volumemode       string
	maxWaitReadyTime time.Duration
}

// function option mode to change the default value of Statefulset parameters,eg. name, replicasno, mpath
type statefulsetOption func(*statefulset)

// Replace the default value of Statefulset name parameter
func setStsName(name string) statefulsetOption {
	return func(this *statefulset) {
		this.name = name
	}
}

// Replace the default value of Statefulset template parameter
func setStsTemplate(template string) statefulsetOption {
	return func(this *statefulset) {
		this.template = template
	}
}

// Replace the default value of Statefulset namespace parameter
func setStsNamespace(namespace string) statefulsetOption {
	return func(this *statefulset) {
		this.namespace = namespace
	}
}

// Replace the default value of Statefulset replicasno parameter
func setStsReplicasNumber(replicasno string) statefulsetOption {
	return func(this *statefulset) {
		this.replicasno = replicasno
	}
}

// Replace the default value of Statefulset app label
func setStsApplabel(applabel string) statefulsetOption {
	return func(this *statefulset) {
		this.applabel = applabel
	}
}

// Replace the default value of Statefulset mountpath parameter
func setStsMountpath(mpath string) statefulsetOption {
	return func(this *statefulset) {
		this.mpath = mpath
	}
}

// Replace the default value of Statefulset volname parameter
func setStsVolName(pvcname string) statefulsetOption {
	return func(this *statefulset) {
		this.pvcname = pvcname
	}
}

// Replace the default value of Statefulset volume type parameter
func setStsVolumeType(volumetype string) statefulsetOption {
	return func(this *statefulset) {
		this.volumetype = volumetype
	}
}

// Replace the default value of Statefulset volume type path parameter
func setStsVolumeTypePath(typepath string) statefulsetOption {
	return func(this *statefulset) {
		this.typepath = typepath
	}
}

// Replace the default value of Statefulset size parameter
func setStsVolumeCapacity(capacity string) statefulsetOption {
	return func(this *statefulset) {
		this.capacity = capacity
	}
}

// Replace the default value of Statefulset size parameter
func setStsSCName(scname string) statefulsetOption {
	return func(this *statefulset) {
		this.scname = scname
	}
}

// Replace the default value of Statefulset volumeMode parameter
func setStsVolumeMode(volumemode string) statefulsetOption {
	return func(this *statefulset) {
		this.volumemode = volumemode
	}
}

// Create a new customized Statefulset object
func newSts(opts ...statefulsetOption) statefulset {
	var defaultVolSize string
	switch cloudProvider {
	// AlibabaCloud minimum volume size is 20Gi
	case "alibabacloud":
		defaultVolSize = strconv.FormatInt(getRandomNum(20, 30), 10) + "Gi"
	// IBMCloud minimum volume size is 10Gi
	case "ibmcloud":
		defaultVolSize = strconv.FormatInt(getRandomNum(10, 20), 10) + "Gi"
	// Other Clouds(AWS GCE Azure OSP vSphere) minimum volume size is 1Gi
	default:
		defaultVolSize = strconv.FormatInt(getRandomNum(1, 10), 10) + "Gi"
	}
	defaultStatefulset := statefulset{
		name:             "my-sts-" + getRandomString(),
		template:         "sts-template.yaml",
		namespace:        "",
		replicasno:       "2",
		applabel:         "myapp-" + getRandomString(),
		mpath:            "/mnt/local",
		pvcname:          "stsvol-" + getRandomString(),
		volumetype:       "volumeMounts",
		typepath:         "mountPath",
		capacity:         defaultVolSize,
		scname:           "gp2-csi",
		volumemode:       "Filesystem",
		maxWaitReadyTime: defaultMaxWaitingTime,
	}

	for _, o := range opts {
		o(&defaultStatefulset)
	}

	return defaultStatefulset
}

// Create new Statefulset with customized parameters
func (sts *statefulset) create(oc *exutil.CLI) {
	if sts.namespace == "" {
		sts.namespace = oc.Namespace()
	}
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", sts.template, "-p", "STSNAME="+sts.name, "STSNAMESPACE="+sts.namespace, "VOLUMENAME="+sts.pvcname, "REPLICASNUM="+sts.replicasno, "APPLABEL="+sts.applabel, "MPATH="+sts.mpath, "VOLUMETYPE="+sts.volumetype, "TYPEPATH="+sts.typepath, "CAPACITY="+sts.capacity, "SCNAME="+sts.scname, "VOLUMEMODE="+sts.volumemode)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Delete the Statefulset from the namespace
func (sts *statefulset) delete(oc *exutil.CLI) {
	err := oc.WithoutNamespace().Run("delete").Args("sts", sts.name, "-n", sts.namespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Delete the Statefulset from the namespace
func (sts *statefulset) deleteAsAdmin(oc *exutil.CLI) {
	oc.WithoutNamespace().AsAdmin().Run("delete").Args("sts", sts.name, "-n", sts.namespace, "--ignore-not-found").Execute()

}

// Get ReplicasNum of the Statefulset
func (sts *statefulset) getReplicasNum(oc *exutil.CLI) string {
	replicasNum, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("sts", sts.name, "-n", sts.namespace, "-o", "jsonpath={.spec.replicas}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return replicasNum
}

// Describe Statefulset
func (sts *statefulset) describeSTS(oc *exutil.CLI) {
	output, err := oc.WithoutNamespace().Run("describe").Args("sts", "-n", sts.namespace, sts.name).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("****** The STS  %s in namespace %s with detail info: ******\n %s", sts.name, sts.namespace, output)
}

// Check pvc counts matches with STS replicas no
func (sts *statefulset) matchPvcNumWithReplicasNo(oc *exutil.CLI) bool {
	return checkPvcNumWithLabel(oc, "app="+sts.applabel, sts.replicasno)
}

// longerTime changes sts.maxWaitReadyTime to LongerMaxWaitingTime
// Used for some Longduration test
func (sts *statefulset) longerTime() *statefulset {
	newSts := *sts
	newSts.maxWaitReadyTime = longerMaxWaitingTime
	return &newSts
}

// Waiting the Statefulset become ready
func (sts *statefulset) waitReady(oc *exutil.CLI) {
	err := wait.Poll(sts.maxWaitReadyTime/defaultIterationTimes, sts.maxWaitReadyTime, func() (bool, error) {
		stsReady, err := sts.checkReady(oc)
		if err != nil {
			return false, err
		}
		if !stsReady {
			return false, nil
		}
		e2e.Logf(sts.name + " availableReplicas is as expected")
		return true, nil
	})

	if err != nil {
		sts.describeSTS(oc)
		podsList, err := getPodsListByLabel(oc, sts.namespace, "app="+sts.applabel)
		o.Expect(err).NotTo(o.HaveOccurred())
		for _, podName := range podsList {
			podstatus, err := oc.WithoutNamespace().Run("get").Args("pod", podName, "-n", sts.namespace, "-o=jsonpath={.status.phase}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if !strings.Contains(podstatus, "Running") {
				e2e.Logf("$ oc describe pod %s:\n%s", podName, describePod(oc, sts.namespace, podName))
				describePersistentVolumeClaim(oc, sts.namespace, getPvcNameFromPod(oc, podName, sts.namespace))
			}
		}
	}
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Deployment %s not ready", sts.name))
}

// Check the Statefulset ready
func (sts *statefulset) checkReady(oc *exutil.CLI) (bool, error) {
	sts.replicasno = sts.getReplicasNum(oc)
	// As the status.availableReplicas is a beta field yet use readyReplicas instead
	// https://kubernetes.io/docs/concepts/workloads/controllers/statefulset/#minimum-ready-seconds
	// $ oc explain sts.status.availableReplicas
	// KIND:     StatefulSet
	// VERSION:  apps/v1

	// FIELD:    availableReplicas <integer>

	// DESCRIPTION:
	//      Total number of available pods (ready for at least minReadySeconds)
	//      targeted by this statefulset. This is a beta field and enabled/disabled by
	//      StatefulSetMinReadySeconds feature gate.
	readyReplicas, err := oc.WithoutNamespace().Run("get").Args("sts", sts.name, "-n", sts.namespace, "-o", "jsonpath={.status.readyReplicas}").Output()
	if err != nil {
		return false, err
	}
	if sts.replicasno == "0" && readyReplicas == "" {
		readyReplicas = "0"
	}
	return strings.EqualFold(sts.replicasno, readyReplicas), nil
}

// Check the pod mounted volume could read and write
func (sts *statefulset) checkMountedVolumeCouldRW(oc *exutil.CLI) {
	podList, err := getPodsListByLabel(oc, sts.namespace, "app="+sts.applabel)
	o.Expect(err).NotTo(o.HaveOccurred())
	for _, podName := range podList {
		content := fmt.Sprintf(`"storage test %v"`, getRandomString())
		randomFileName := "/testfile_" + getRandomString()
		_, err := execCommandInSpecificPod(oc, sts.namespace, podName, "echo "+content+">"+sts.mpath+randomFileName)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(execCommandInSpecificPod(oc, sts.namespace, podName, "cat "+sts.mpath+randomFileName)).To(o.Equal(strings.Replace(content, "\"", "", 2)))
	}
}

// Check the pod mounted volume have exec right
func (sts *statefulset) checkMountedVolumeHaveExecRight(oc *exutil.CLI) {
	podList, err := getPodsListByLabel(oc, sts.namespace, "app="+sts.applabel)
	o.Expect(err).NotTo(o.HaveOccurred())
	for _, podName := range podList {
		if provisioner == "file.csi.azure.com" {
			o.Expect(execCommandInSpecificPod(oc, sts.namespace, podName, fmt.Sprintf("echo '#!/bin/bash\necho \"Hello OpenShift Storage\"' > %s", sts.mpath+"/hello"))).Should(o.Equal(""))
			o.Expect(execCommandInSpecificPod(oc, sts.namespace, podName, "ls -l "+sts.mpath+"/hello")).To(o.Or(o.ContainSubstring("-rwxrwxrwx"), (o.ContainSubstring("-rw-r--r--"))))
		} else {
			o.Expect(execCommandInSpecificPod(oc, sts.namespace, podName, fmt.Sprintf("echo '#!/bin/bash\necho \"Hello OpenShift Storage\"' > %s && chmod +x %s ", sts.mpath+"/hello", sts.mpath+"/hello"))).Should(o.Equal(""))
			o.Expect(execCommandInSpecificPod(oc, sts.namespace, podName, sts.mpath+"/hello")).To(o.ContainSubstring("Hello OpenShift Storage"))
		}
	}
}

// Check the pod mounted volume could write data into raw block volume
func (sts *statefulset) writeDataIntoRawBlockVolume(oc *exutil.CLI) {
	e2e.Logf("Write the data in Raw Block volume")
	podList, err := getPodsListByLabel(oc, sts.namespace, "app="+sts.applabel)
	o.Expect(err).NotTo(o.HaveOccurred())
	for _, podName := range podList {
		_, err := execCommandInSpecificPod(oc, sts.namespace, podName, "/bin/dd  if=/dev/null of="+sts.mpath+" bs=512 count=1")
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = execCommandInSpecificPod(oc, sts.namespace, podName, "echo 'storage test' > "+sts.mpath)
		o.Expect(err).NotTo(o.HaveOccurred())
	}
}

// Check data into raw block volume could be read
func (sts *statefulset) checkDataIntoRawBlockVolume(oc *exutil.CLI) {
	e2e.Logf("Check the data in Raw Block volume")
	podList, err := getPodsListByLabel(oc, sts.namespace, "app="+sts.applabel)
	o.Expect(err).NotTo(o.HaveOccurred())
	for _, podName := range podList {
		_, err := execCommandInSpecificPod(oc, sts.namespace, podName, "/bin/dd  if="+sts.mpath+" of=/tmp/testfile bs=512 count=1")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(execCommandInSpecificPod(oc, sts.namespace, podName, "cat /tmp/testfile")).To(o.ContainSubstring("storage test"))
	}
}

// Daemonset workload related functions
type daemonset struct {
	name             string
	namespace        string
	applabel         string
	mpath            string
	pvcname          string
	template         string
	volumetype       string
	typepath         string
	maxWaitReadyTime time.Duration
}

// function option mode to change the default value of daemonset parameters,eg. name, mpath
type daemonSetOption func(*daemonset)

// Replace the default value of Daemonset name parameter
func setDsName(name string) daemonSetOption {
	return func(this *daemonset) {
		this.name = name
	}
}

// Replace the default value of Daemonset template parameter
func setDsTemplate(template string) daemonSetOption {
	return func(this *daemonset) {
		this.template = template
	}
}

// Replace the default value of Daemonset namespace parameter
func setDsNamespace(namespace string) daemonSetOption {
	return func(this *daemonset) {
		this.namespace = namespace
	}
}

// Replace the default value of Daemonset app label
func setDsApplabel(applabel string) daemonSetOption {
	return func(this *daemonset) {
		this.applabel = applabel
	}
}

// Replace the default value of Daemonset mountpath parameter
func setDsMountpath(mpath string) daemonSetOption {
	return func(this *daemonset) {
		this.mpath = mpath
	}
}

// Replace the default value of Daemonset pvcname parameter
func setDsPVCName(pvcname string) daemonSetOption {
	return func(this *daemonset) {
		this.pvcname = pvcname
	}
}

// Replace the default value of Daemonset volume type parameter
func setDsVolumeType(volumetype string) daemonSetOption {
	return func(this *daemonset) {
		this.volumetype = volumetype
	}
}

// Replace the default value of Daemonset volume type path parameter
func setDsVolumeTypePath(typepath string) daemonSetOption {
	return func(this *daemonset) {
		this.typepath = typepath
	}
}

// Create a new customized Daemonset object
func newDaemonSet(opts ...daemonSetOption) daemonset {
	defaultDaemonSet := daemonset{
		name:             "my-ds-" + getRandomString(),
		template:         "ds-template.yaml",
		namespace:        "",
		applabel:         "myapp-" + getRandomString(),
		mpath:            "/mnt/ds",
		pvcname:          "",
		volumetype:       "volumeMounts",
		typepath:         "mountPath",
		maxWaitReadyTime: defaultMaxWaitingTime,
	}

	for _, o := range opts {
		o(&defaultDaemonSet)
	}

	return defaultDaemonSet
}

// Create new Daemonset with customized parameters
func (ds *daemonset) create(oc *exutil.CLI) {
	if ds.namespace == "" {
		ds.namespace = oc.Namespace()
	}
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", ds.template, "-p", "DSNAME="+ds.name, "DSNAMESPACE="+ds.namespace, "PVCNAME="+ds.pvcname, "DSLABEL="+ds.applabel, "MPATH="+ds.mpath, "VOLUMETYPE="+ds.volumetype, "TYPEPATH="+ds.typepath)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Create new Daemonset with extra parameters
func (ds *daemonset) createWithExtraParameters(oc *exutil.CLI, extraParameters map[string]interface{}) {
	if ds.namespace == "" {
		ds.namespace = oc.Namespace()
	}
	err := applyResourceFromTemplateWithExtraParametersAsAdmin(oc, extraParameters, "--ignore-unknown-parameters=true", "-f", ds.template, "-p", "DNAME="+ds.name, "DNAMESPACE="+ds.namespace, "PVCNAME="+ds.pvcname, "DLABEL="+ds.applabel, "MPATH="+ds.mpath, "VOLUMETYPE="+ds.volumetype, "TYPEPATH="+ds.typepath)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Delete Daemonset from the namespace
func (ds *daemonset) delete(oc *exutil.CLI) {
	err := oc.WithoutNamespace().Run("delete").Args("daemonset", ds.name, "-n", ds.namespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Delete Daemonset from the namespace
func (ds *daemonset) deleteAsAdmin(oc *exutil.CLI) {
	oc.WithoutNamespace().AsAdmin().Run("delete").Args("daemonset", ds.name, "-n", ds.namespace, "--ignore-not-found").Execute()
}

// Describe Daemonset
func (ds *daemonset) describeDaemonSet(oc *exutil.CLI) {
	output, err := oc.WithoutNamespace().Run("describe").Args("daemonset", "-n", ds.namespace, ds.name).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("****** The Daemonset  %s in namespace %s with detail info: ******\n %s", ds.name, ds.namespace, output)
}

// Get daemonset pod list
func (ds *daemonset) getPodsList(oc *exutil.CLI) []string {
	selectorLable := ds.applabel
	if !strings.Contains(ds.applabel, "=") {
		selectorLable = "app=" + ds.applabel
	}
	output, err := oc.WithoutNamespace().Run("get").Args("pod", "-n", ds.namespace, "-l", selectorLable, "-o=jsonpath={.items[*].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return strings.Split(output, " ")
}

// Get daemonset pod's Node list
func (ds *daemonset) getNodesList(oc *exutil.CLI) []string {
	var nodeList []string
	for _, podName := range ds.getPodsList(oc) {
		nodeList = append(nodeList, getNodeNameByPod(oc, ds.namespace, podName))
	}
	return nodeList
}

// GetSpecifiedJSONPathValue gets the specified jsonpath value of the daemonset
func (ds *daemonset) getSpecifiedJSONPathValue(oc *exutil.CLI, jsonPath string) string {
	value, getValueErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("daemonset", ds.name, "-n", ds.namespace, "-o", fmt.Sprintf("jsonpath=%s", jsonPath)).Output()
	o.Expect(getValueErr).NotTo(o.HaveOccurred())
	e2e.Logf(`Daemonset/%s jsonPath->%q value is %q`, ds.name, jsonPath, value)
	return value
}

// Check the Daemonset ready
func (ds *daemonset) checkReady(oc *exutil.CLI) (bool, error) {
	dsAvailableNumber, err1 := oc.WithoutNamespace().Run("get").Args("daemonset", ds.name, "-n", ds.namespace, "-o", "jsonpath={.status.numberAvailable}").Output()
	dsDesiredScheduledNumber, err2 := oc.WithoutNamespace().Run("get").Args("daemonset", ds.name, "-n", ds.namespace, "-o", "jsonpath={.status.desiredNumberScheduled}").Output()
	e2e.Logf("Available number of daemonsets: %s, Desired number of scheduled daemonsets: %s ", dsAvailableNumber, dsDesiredScheduledNumber)
	if err1 != nil || err2 != nil {
		return false, fmt.Errorf("get dsAvailableNumber errinfo:\"%v\";\nget dsDesiredScheduledNumber errinfo:\"%v\";", err1, err2)
	}
	return strings.EqualFold(dsAvailableNumber, dsDesiredScheduledNumber), nil
}

// Check the daemonset mounted volume could write
func (ds *daemonset) checkPodMountedVolumeCouldWrite(oc *exutil.CLI) {
	for indexValue, podinstance := range ds.getPodsList(oc) {
		content := "storage test " + getRandomString()
		FileName := "/testfile_" + strconv.Itoa(indexValue+1)
		_, err := execCommandInSpecificPod(oc, ds.namespace, podinstance, "echo "+content+">"+ds.mpath+FileName)
		o.Expect(err).NotTo(o.HaveOccurred())
	}
}

// Check the daemonset mounted volume has the original data
func (ds *daemonset) checkPodMountedVolumeCouldRead(oc *exutil.CLI) {
	podList := ds.getPodsList(oc)
	for _, podInstance := range podList {
		for indexValue := 1; indexValue <= len(podList); indexValue++ {
			o.Expect(execCommandInSpecificPod(oc, ds.namespace, podInstance, "cat "+ds.mpath+"/testfile_"+strconv.Itoa(indexValue))).To(o.ContainSubstring("storage test"))
		}
	}
}

// longerTime changes ds.maxWaitReadyTime to LongerMaxWaitingTime
// Used for some Longduration test
func (ds *daemonset) longerTime() *daemonset {
	newDs := *ds
	newDs.maxWaitReadyTime = longerMaxWaitingTime
	return &newDs
}

// Waiting the Daemonset to become ready
func (ds *daemonset) waitReady(oc *exutil.CLI) {
	err := wait.Poll(ds.maxWaitReadyTime/defaultIterationTimes, ds.maxWaitReadyTime, func() (bool, error) {
		dsReady, err := ds.checkReady(oc)
		if err != nil {
			return dsReady, err
		}
		if !dsReady {
			return dsReady, nil
		}
		e2e.Logf(ds.name + " reached to expected availableNumbers")
		return dsReady, nil
	})

	if err != nil {
		ds.describeDaemonSet(oc)
		podsList, _ := getPodsListByLabel(oc, ds.namespace, "app="+ds.applabel)
		for _, podName := range podsList {
			podstatus, _ := oc.WithoutNamespace().Run("get").Args("pod", podName, "-n", ds.namespace, "-o=jsonpath={.status.phase}").Output()
			if !strings.Contains(podstatus, "Running") {
				e2e.Logf("$ oc describe pod %s:\n%s", podName, describePod(oc, ds.namespace, podName))
				describePersistentVolumeClaim(oc, ds.namespace, getPvcNameFromPod(oc, podName, ds.namespace))
			}
		}
	}
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Daemonset %s not ready", ds.name))
}
