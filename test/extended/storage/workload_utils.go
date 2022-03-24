package storage

import (
	"fmt"
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
	name       string
	namespace  string
	pvcname    string
	template   string
	image      string
	volumeType string
	pathType   string
	mountPath  string
}

// Define the global vSphere Storage Operator && Driver deploments object
var (
	detectorOperator = newDeployment(setDeploymentName("vsphere-problem-detector-operator"), setDeploymentNamespace("openshift-cluster-storage-operator"),
		setDeploymentApplabel("name=vsphere-problem-detector-operator"))
	driverController = newDeployment(setDeploymentName("vmware-vsphere-csi-driver-controller"), setDeploymentNamespace("openshift-cluster-csi-drivers"),
		setDeploymentApplabel("app=vmware-vsphere-csi-driver-controller"), setDeploymentReplicasNumber("2"))
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

//  Create a new customized pod object
func newPod(opts ...podOption) pod {
	defaultPod := pod{
		name:       "mypod-" + getRandomString(),
		template:   "pod-template.yaml",
		namespace:  "",
		pvcname:    "mypvc",
		image:      "quay.io/openshifttest/hello-openshift@sha256:b1aabe8c8272f750ce757b6c4263a2712796297511e0c6df79144ee188933623",
		volumeType: "volumeMounts",
		pathType:   "mountPath",
		mountPath:  "/mnt/storage",
	}

	for _, o := range opts {
		o(&defaultPod)
	}

	return defaultPod
}

// Create new pod with customized parameters
func (pod *pod) create(oc *exutil.CLI) {
	if pod.namespace == "" {
		pod.namespace = oc.Namespace()
	}
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", pod.template, "-p", "PODNAME="+pod.name, "PODNAMESPACE="+pod.namespace, "PVCNAME="+pod.pvcname, "PODIMAGE="+pod.image, "VOLUMETYPE="+pod.volumeType, "PATHTYPE="+pod.pathType, "PODMOUNTPATH="+pod.mountPath)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Create new pod with extra parameters
func (pod *pod) createWithExtraParameters(oc *exutil.CLI, extraParameters map[string]interface{}) {
	if pod.namespace == "" {
		pod.namespace = oc.Namespace()
	}
	err := applyResourceFromTemplateWithExtraParametersAsAdmin(oc, extraParameters, "--ignore-unknown-parameters=true", "-f", pod.template, "-p", "PODNAME="+pod.name, "PODNAMESPACE="+pod.namespace, "PVCNAME="+pod.pvcname, "PODIMAGE="+pod.image, "VOLUMETYPE="+pod.volumeType, "PATHTYPE="+pod.pathType, "PODMOUNTPATH="+pod.mountPath)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Create new pod with extra parameters for readonly
func (pod *pod) createWithReadOnlyVolume(oc *exutil.CLI) {
	if pod.namespace == "" {
		pod.namespace = oc.Namespace()
	}
	extraParameters := map[string]interface{}{
		"jsonPath": `items.0.spec.containers.0.volumeMounts.0.`,
		"readOnly": true,
	}
	err := applyResourceFromTemplateWithExtraParametersAsAdmin(oc, extraParameters, "--ignore-unknown-parameters=true", "-f", pod.template, "-p", "PODNAME="+pod.name, "PODNAMESPACE="+pod.namespace, "PVCNAME="+pod.pvcname, "PODIMAGE="+pod.image, "VOLUMETYPE="+pod.volumeType, "PATHTYPE="+pod.pathType, "PODMOUNTPATH="+pod.mountPath)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Create new pod with subpath
func (pod *pod) createWithSubpathVolume(oc *exutil.CLI, subPath string) {
	if pod.namespace == "" {
		pod.namespace = oc.Namespace()
	}
	extraParameters := map[string]interface{}{
		"jsonPath": `items.0.spec.containers.0.volumeMounts.0.`,
		"subPath":  subPath,
	}
	err := applyResourceFromTemplateWithExtraParametersAsAdmin(oc, extraParameters, "--ignore-unknown-parameters=true", "-f", pod.template, "-p", "PODNAME="+pod.name, "PODNAMESPACE="+pod.namespace, "PVCNAME="+pod.pvcname, "PODIMAGE="+pod.image, "VOLUMETYPE="+pod.volumeType, "PATHTYPE="+pod.pathType, "PODMOUNTPATH="+pod.mountPath)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Create new pod for security check
func (pod *pod) createWithSecurity(oc *exutil.CLI) {
	seLevel := map[string]string{
		"level": "s0:c13,c2",
	}
	securityContext := map[string]interface{}{
		"seLinuxOptions": seLevel,
		"fsGroup":        24680,
		"runAsUser":      1000160000,
	}
	extraParameters := map[string]interface{}{
		"jsonPath":        `items.0.spec.`,
		"securityContext": securityContext,
	}
	if pod.namespace == "" {
		pod.namespace = oc.Namespace()
	}
	err := applyResourceFromTemplateWithExtraParametersAsAdmin(oc, extraParameters, "--ignore-unknown-parameters=true", "-f", pod.template, "-p", "PODNAME="+pod.name, "PODNAMESPACE="+pod.namespace, "PVCNAME="+pod.pvcname, "PODIMAGE="+pod.image, "VOLUMETYPE="+pod.volumeType, "PATHTYPE="+pod.pathType, "PODMOUNTPATH="+pod.mountPath)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Create new pod with extra parameters for nodeSelector
func (pod *pod) createWithNodeSelector(oc *exutil.CLI, labelName string, labelValue string) {
	extraParameters := map[string]interface{}{
		"jsonPath": `items.0.spec.nodeSelector.`,
		labelName:  labelValue,
	}
	if pod.namespace == "" {
		pod.namespace = oc.Namespace()
	}
	err := applyResourceFromTemplateWithExtraParametersAsAdmin(oc, extraParameters, "--ignore-unknown-parameters=true", "-f", pod.template, "-p", "PODNAME="+pod.name, "PODNAMESPACE="+pod.namespace, "PVCNAME="+pod.pvcname, "PODIMAGE="+pod.image, "VOLUMETYPE="+pod.volumeType, "PATHTYPE="+pod.pathType, "PODMOUNTPATH="+pod.mountPath)
	o.Expect(err).NotTo(o.HaveOccurred())
}

//  Delete the pod
func (pod *pod) delete(oc *exutil.CLI) {
	err := oc.WithoutNamespace().Run("delete").Args("pod", pod.name, "-n", pod.namespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

//  Delete the pod use kubeadmin
func (pod *pod) deleteAsAdmin(oc *exutil.CLI) {
	oc.WithoutNamespace().AsAdmin().Run("delete").Args("pod", pod.name, "-n", pod.namespace).Execute()
}

//  Pod exec the bash CLI
func (pod *pod) execCommand(oc *exutil.CLI, command string) (string, error) {
	return execCommandInSpecificPod(oc, pod.namespace, pod.name, command)
}

// Pod exec the bash CLI with admin
func (pod *pod) execCommandAsAdmin(oc *exutil.CLI, command string) (string, error) {
	command1 := []string{"-n", pod.namespace, pod.name, "--", "/bin/sh", "-c", command}
	msg, err := oc.WithoutNamespace().AsAdmin().Run("exec").Args(command1...).Output()
	if err != nil {
		e2e.Logf(pod.name+"# "+command+" *failed with* :\"%v\".", err)
		return msg, err
	}
	debugLogf(pod.name+"# "+command+" *Output is* :\"%s\".", msg)
	return msg, nil
}

//  Check the pod mounted volume could read and write
func (pod *pod) checkMountedVolumeCouldRW(oc *exutil.CLI) {
	_, err := execCommandInSpecificPod(oc, pod.namespace, pod.name, "echo \"storage test\" >"+pod.mountPath+"/testfile")
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(execCommandInSpecificPod(oc, pod.namespace, pod.name, "cat "+pod.mountPath+"/testfile")).To(o.ContainSubstring("storage test"))
}

//  Check the pod mounted volume have exec right
func (pod *pod) checkMountedVolumeHaveExecRight(oc *exutil.CLI) {
	_, err := execCommandInSpecificPod(oc, pod.namespace, pod.name, "cp hello "+pod.mountPath)
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(execCommandInSpecificPod(oc, pod.namespace, pod.name, pod.mountPath+"/hello")).To(o.ContainSubstring("Hello OpenShift Storage"))
}

//  Check the pod mounted volume could write data into raw block volume
func (pod *pod) writeDataIntoRawBlockVolume(oc *exutil.CLI) {
	e2e.Logf("Writing the data into Raw Block volume")
	_, err := pod.execCommand(oc, "/bin/dd  if=/dev/null of="+pod.mountPath+" bs=512 count=1")
	o.Expect(err).NotTo(o.HaveOccurred())
	_, err = pod.execCommand(oc, "echo 'storage test' > "+pod.mountPath)
	o.Expect(err).NotTo(o.HaveOccurred())
}

//  Check data in raw block volume could be read
func (pod *pod) checkDataInRawBlockVolume(oc *exutil.CLI) {
	e2e.Logf("Check the data in Raw Block volume")
	_, err := pod.execCommand(oc, "/bin/dd  if="+pod.mountPath+" of=/tmp/testfile bs=512 count=1")
	o.Expect(err).NotTo(o.HaveOccurred())
	output, err := pod.execCommand(oc, "cat /tmp/testfile")
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(output).To(o.ContainSubstring("storage test"))
}

func (pod *pod) checkFsgroup(oc *exutil.CLI, command string, expect string) {
	output, err := pod.execCommandAsAdmin(oc, command)
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(output).To(o.ContainSubstring(expect))
}

// Waiting for the Pod ready
func (pod *pod) waitReady(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 180*time.Second, func() (bool, error) {
		status, err1 := checkPodReady(oc, pod.namespace, pod.name)
		if err1 != nil {
			e2e.Logf("the err:%v, wait for pod %v to become ready.", err1, pod.name)
			return status, err1
		}
		if !status {
			return status, nil
		}
		e2e.Logf("Pod: \"%s\" is running on the node: \"%s\"", pod.name, getNodeNameByPod(oc, pod.namespace, pod.name))
		return status, nil
	})

	if err != nil {
		podDescribe := describePod(oc, pod.namespace, pod.name)
		e2e.Logf("oc describe pod %s:\n%s", pod.name, podDescribe)
		describePersistentVolumeClaim(oc, pod.namespace, pod.pvcname)
	}
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("pod %s not ready", pod.name))
}

//  Get the phase, status of specified pod
func getPodStatus(oc *exutil.CLI, namespace string, podName string) (string, error) {
	podStatus, err := oc.WithoutNamespace().Run("get").Args("pod", "-n", namespace, podName, "-o=jsonpath={.status.phase}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The pod  %s status in namespace %s is %q", podName, namespace, podStatus)
	return podStatus, err
}

//  Check the pod status becomes ready, status is "Running", "Ready" or "Complete"
func checkPodReady(oc *exutil.CLI, namespace string, podName string) (bool, error) {
	podOutPut, err := getPodStatus(oc, namespace, podName)
	status := []string{"Running", "Ready", "Complete"}
	return contains(status, podOutPut), err
}

//  Get the detail info of specified pod
func describePod(oc *exutil.CLI, namespace string, podName string) string {
	podDescribe, err := oc.WithoutNamespace().Run("describe").Args("pod", "-n", namespace, podName).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return podDescribe
}

//  Waiting for the pod becomes ready, such as "Running", "Ready", "Complete"
func waitPodReady(oc *exutil.CLI, namespace string, podName string) {
	err := wait.Poll(5*time.Second, 180*time.Second, func() (bool, error) {
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

//  Specified pod exec the bash CLI
//  If failed execute will retry 3 times, because of the network instability or other action cause the pod recreate flake.
//  Flake info : "error: unable to upgrade connection: container not found"  It maybe the container suddenly crashed.
func execCommandInSpecificPod(oc *exutil.CLI, namespace string, podName string, command string) (output string, errInfo error) {
	command1 := []string{"-n", namespace, podName, "--", "/bin/sh", "-c", command}
	err := wait.Poll(5*time.Second, 15*time.Second, func() (bool, error) {
		output, errInfo = oc.WithoutNamespace().Run("exec").Args(command1...).Output()
		if errInfo != nil {
			e2e.Logf(podName+"# "+command+" *failed with* :\"%v\".", errInfo)
			return false, errInfo
		}
		e2e.Logf(podName+"# "+command+" *Output is* :\"%s\".", output)
		return true, nil
	})

	if err != nil {
		e2e.Logf("oc describe pod %s:\n%s", podName, describePod(oc, namespace, podName))
		return output, errInfo
	}
	return output, nil
}

// Wait for pods selected with selector name to be removed
func WaitUntilPodsAreGoneByLabel(oc *exutil.CLI, namespace string, labelName string) {
	err := wait.Poll(5*time.Second, 180*time.Second, func() (bool, error) {
		output, err := oc.WithoutNamespace().Run("get").Args("pods", "-l", labelName, "-n", namespace).Output()
		if err != nil {
			return false, err
		} else {
			errstring := fmt.Sprintf("%v", output)
			if strings.Contains(errstring, "No resources found") {
				e2e.Logf(output)
				return true, nil
			} else {
				return false, nil
			}
		}
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Error waiting for pods to be removed using labelName  %s", labelName))
}

// Get the pod details
func getPodDetailsByLabel(oc *exutil.CLI, namespace string, labelName string) (string, error) {
	output, err := oc.WithoutNamespace().Run("get").Args("pods", "-l", labelName, "-n", namespace).Output()
	if err != nil {
		e2e.Logf("Get pod details failed with  err:%v .", err)
		return output, err
	} else {
		e2e.Logf("Get pod details output is:\"%v\"", output)
	}
	o.Expect(err).NotTo(o.HaveOccurred())
	return output, nil
}

// Get the pods List by label
func getPodsListByLabel(oc *exutil.CLI, namespace string, selectorLabel string) ([]string, error) {
	podsOp, err := oc.WithoutNamespace().Run("get").Args("pod", "-n", namespace, "-l", selectorLabel, "-o=jsonpath={.items[*].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return strings.Split(podsOp, " "), err
}

// Get the pvcName from the pod
func getPvcNameFromPod(oc *exutil.CLI, podName string) string {
	pvcName, err := oc.WithoutNamespace().Run("get").Args("pod", "-n", oc.Namespace(), "-o=jsonpath={.spec.volumes[*].persistentVolumeClaim.claimName}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return pvcName
}

// Get the pod status by label, Checking status for n numbers of deployments
func checkPodStatusByLabel(oc *exutil.CLI, namespace string, selectorLabel string, expectedstatus string) {
	var podDescribe string
	var pvcList []string
	podsList, _ := getPodsListByLabel(oc, namespace, selectorLabel)
	e2e.Logf("PodLabelName \"%s\", expected status is \"%s\", podsList=%s", selectorLabel, expectedstatus, podsList)
	err := wait.Poll(3*time.Second, 120*time.Second, func() (bool, error) {
		podflag := 0
		for _, podName := range podsList {
			podstatus, err := oc.WithoutNamespace().Run("get").Args("pod", podName, "-n", namespace, "-o=jsonpath={.status.phase}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if matched, _ := regexp.MatchString(expectedstatus, podstatus); !matched {
				podDescribe = describePod(oc, namespace, podName)
				pvcList = append(pvcList, getPvcNameFromPod(oc, podName))
				podflag = 1
			}
		}
		if podflag == 1 {
			return false, nil
		} else {
			e2e.Logf("%s is with expected status: \"%s\"", podsList, expectedstatus)
			return true, nil
		}
	})
	if err != nil && podDescribe != "" {
		e2e.Logf(podDescribe)
		for _, pvcName := range pvcList {
			describePersistentVolumeClaim(oc, oc.Namespace(), pvcName)
		}
	}
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("pod with label %s not ready", selectorLabel))
}

//  Specified pod exec the bash CLI
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
	name       string
	namespace  string
	replicasno string
	applabel   string
	mpath      string
	pvcname    string
	template   string
	volumetype string
	typepath   string
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

//  Create a new customized Deployment object
func newDeployment(opts ...deployOption) deployment {
	defaultDeployment := deployment{
		name:       "my-dep-" + getRandomString(),
		template:   "dep-template.yaml",
		namespace:  "",
		replicasno: "1",
		applabel:   "myapp-" + getRandomString(),
		mpath:      "/mnt/storage",
		pvcname:    "",
		volumetype: "volumeMounts",
		typepath:   "mountPath",
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

// Delete Deployment from the namespace
func (dep *deployment) delete(oc *exutil.CLI) {
	err := oc.WithoutNamespace().Run("delete").Args("deployment", dep.name, "-n", dep.namespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Delete Deployment from the namespace
func (dep *deployment) deleteAsAdmin(oc *exutil.CLI) {
	oc.WithoutNamespace().AsAdmin().Run("delete").Args("deployment", dep.name, "-n", dep.namespace).Execute()
}

// Get deployment pod list
func (dep *deployment) getPodList(oc *exutil.CLI) []string {
	selectorLable := dep.applabel
	if !strings.Contains(dep.applabel, "=") {
		selectorLable = "app=" + dep.applabel
	}
	output, err := oc.WithoutNamespace().Run("get").Args("pod", "-n", dep.namespace, "-l", selectorLable, "-o=jsonpath={.items[*].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return strings.Split(output, " ")
}

// Get ReplicasNum of the Deployment
func (dep *deployment) getReplicasNum(oc *exutil.CLI) string {
	replicasNum, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", dep.name, "-n", dep.namespace, "-o", "jsonpath={.spec.replicas}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return replicasNum
}

// Scale Replicas for the Deployment
func (dep *deployment) scaleReplicas(oc *exutil.CLI, replicasno string) {
	err := oc.WithoutNamespace().Run("scale").Args("deployment", dep.name, "--replicas="+replicasno, "-n", dep.namespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	dep.replicasno = replicasno
}

// Restart the Deployment by scale down to '0' and then scale up to origin number
func (dep *deployment) restart(oc *exutil.CLI) error {
	originReplicasNum := dep.replicasno
	dep.scaleReplicas(oc, "0")
	// VSphereProblemDetectorController will automated recover the dector replicas number
	if dep == &detectorOperator {
		dep.replicasno = originReplicasNum
		dep.waitReady(oc)
		e2e.Logf("deployment/%s in namespace %s restart successfully", dep.name, dep.namespace)
		return nil
	}
	dep.waitReady(oc)
	dep.scaleReplicas(oc, originReplicasNum)
	dep.waitReady(oc)
	e2e.Logf("deployment/%s in namespace %s restart successfully", dep.name, dep.namespace)
	return nil
}

// Check the deployment ready
func (dep *deployment) checkReady(oc *exutil.CLI) (bool, error) {
	readyReplicas, err := oc.WithoutNamespace().Run("get").Args("deployment", dep.name, "-n", dep.namespace, "-o", "jsonpath={.status.availableReplicas}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if dep.replicasno == "0" && readyReplicas == "" {
		readyReplicas = "0"
	}
	return strings.EqualFold(dep.replicasno, readyReplicas), err
}

// Describe the deployment
func (dep *deployment) describe(oc *exutil.CLI) string {
	deploymentDescribe, err := oc.WithoutNamespace().Run("describe").Args("deployment", dep.name, "-n", dep.namespace).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return deploymentDescribe
}

// Waiting the deployment become ready
func (dep *deployment) waitReady(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 180*time.Second, func() (bool, error) {
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
		e2e.Logf("$ oc describe pod %s:\n%s", dep.name, dep.describe(oc))
		for _, podName := range dep.getPodList(oc) {
			e2e.Logf("$ oc describe pod %s:\n%s", podName, describePod(oc, dep.namespace, podName))
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
		content := "storage test " + getRandomString()
		randomFileName := "/testfile_" + getRandomString()
		_, err := execCommandInSpecificPod(oc, dep.namespace, podinstance, "echo "+content+">"+dep.mpath+randomFileName)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(execCommandInSpecificPod(oc, dep.namespace, podinstance, "cat "+dep.mpath+randomFileName)).To(o.ContainSubstring(content))
	}
}

// Get the deployment data written from checkPodMountedVolumeCouldRW
func (dep *deployment) getPodMountedVolumeData(oc *exutil.CLI) {
	for _, podinstance := range dep.getPodList(oc) {
		o.Expect(execCommandInSpecificPod(oc, dep.namespace, podinstance, "cat "+dep.mpath+"/testfile_*")).To(o.ContainSubstring("storage test"))
	}
}

// Check the deployment mounted volume have exec right
func (dep *deployment) checkPodMountedVolumeHaveExecRight(oc *exutil.CLI) {
	for _, podinstance := range dep.getPodList(oc) {
		_, err := execCommandInSpecificPod(oc, dep.namespace, podinstance, "cp hello "+dep.mpath)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(execCommandInSpecificPod(oc, dep.namespace, podinstance, dep.mpath+"/hello")).To(o.ContainSubstring("Hello OpenShift Storage"))
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
	_, err = execCommandInSpecificPod(oc, dep.namespace, dep.getPodList(oc)[0], "echo 'test data' > "+dep.mpath)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Check data written
func (dep *deployment) checkDataBlockType(oc *exutil.CLI) {
	_, err := execCommandInSpecificPod(oc, dep.namespace, dep.getPodList(oc)[0], "/bin/dd if="+dep.mpath+" of=/tmp/testfile bs=512 count=1")
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(execCommandInSpecificPod(oc, dep.namespace, dep.getPodList(oc)[0], "cat /tmp/testfile | grep 'test data' ")).To(o.ContainSubstring("matches"))
}

// Statefulset workload related functions
type statefulset struct {
	name       string
	namespace  string
	replicasno string
	applabel   string
	mpath      string
	pvcname    string
	template   string
	volumetype string
	typepath   string
	capacity   string
	scname     string
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

//  Create a new customized Statefulset object
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
		name:       "my-sts-" + getRandomString(),
		template:   "sts-template.yaml",
		namespace:  "",
		replicasno: "2",
		applabel:   "myapp-" + getRandomString(),
		mpath:      "/mnt/local",
		pvcname:    "stsvol-" + getRandomString(),
		volumetype: "volumeMounts",
		typepath:   "mountPath",
		capacity:   defaultVolSize,
		scname:     "gp2-csi",
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
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", sts.template, "-p", "STSNAME="+sts.name, "STSNAMESPACE="+sts.namespace, "VOLUMENAME="+sts.pvcname, "REPLICASNUM="+sts.replicasno, "APPLABEL="+sts.applabel, "MPATH="+sts.mpath, "VOLUMETYPE="+sts.volumetype, "TYPEPATH="+sts.typepath, "CAPACITY="+sts.capacity, "SCNAME="+sts.scname)
	o.Expect(err).NotTo(o.HaveOccurred())
}

//  Delete the Statefulset from the namespace
func (sts *statefulset) delete(oc *exutil.CLI) {
	err := oc.WithoutNamespace().Run("delete").Args("sts", sts.name, "-n", sts.namespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

//  Delete the Statefulset from the namespace
func (sts *statefulset) deleteAsAdmin(oc *exutil.CLI) {
	oc.WithoutNamespace().AsAdmin().Run("delete").Args("sts", sts.name, "-n", sts.namespace).Execute()

}

//  Describe Statefulset
func (sts *statefulset) describeSTS(oc *exutil.CLI) {
	output, err := oc.WithoutNamespace().Run("describe").Args("sts", "-n", sts.namespace, sts.name).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("****** The STS  %s in namespace %s with detail info: ******\n %s", sts.name, sts.namespace, output)
}

// Check pvc counts matches with STS replicas no
func (sts *statefulset) matchPvcNumWithReplicasNo(oc *exutil.CLI) bool {
	return checkPvcNumWithLabel(oc, "app="+sts.applabel, sts.replicasno)
}

// Waiting the Statefulset become ready
func (sts *statefulset) waitReady(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 60*time.Second, func() (bool, error) {
		stsReady, err := sts.checkReady(oc)
		if err != nil {
			return stsReady, err
		}
		if !stsReady {
			return stsReady, nil
		}
		e2e.Logf(sts.name + " availableReplicas is as expected")
		return stsReady, nil
	})

	if err != nil {
		sts.describeSTS(oc)
	}
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Deployment %s not ready", sts.name))
}

// Check the Statefulset ready
func (sts *statefulset) checkReady(oc *exutil.CLI) (bool, error) {
	readyReplicas, err := oc.WithoutNamespace().Run("get").Args("sts", sts.name, "-n", sts.namespace, "-o", "jsonpath={.status.availableReplicas}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if sts.replicasno == "0" && readyReplicas == "" {
		readyReplicas = "0"
	}
	return strings.EqualFold(sts.replicasno, readyReplicas), err
}

//  Check the pod mounted volume could read and write
func (sts *statefulset) checkMountedVolumeCouldRW(oc *exutil.CLI) {
	podList, _ := getPodsListByLabel(oc, sts.namespace, "app="+sts.applabel)
	for _, podName := range podList {
		content := "storage test " + getRandomString()
		randomFileName := "/testfile_" + getRandomString()
		_, err := execCommandInSpecificPod(oc, sts.namespace, podName, "echo "+content+">"+sts.mpath+randomFileName)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(execCommandInSpecificPod(oc, sts.namespace, podName, "cat "+sts.mpath+randomFileName)).To(o.ContainSubstring(content))
	}
}

//  Check the pod mounted volume have exec right
func (sts *statefulset) checkMountedVolumeHaveExecRight(oc *exutil.CLI) {
	podList, _ := getPodsListByLabel(oc, sts.namespace, "app="+sts.applabel)
	for _, podName := range podList {
		_, err := execCommandInSpecificPod(oc, sts.namespace, podName, "cp hello "+sts.mpath)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(execCommandInSpecificPod(oc, sts.namespace, podName, sts.mpath+"/hello")).To(o.ContainSubstring("Hello OpenShift Storage"))
	}
}
