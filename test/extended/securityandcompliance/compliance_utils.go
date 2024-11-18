package securityandcompliance

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"github.com/tidwall/gjson"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type complianceSuiteDescription struct {
	name                string
	namespace           string
	schedule            string
	scanname            string
	scanType            string
	profile             string
	content             string
	contentImage        string
	rule                string
	debug               bool
	noExternalResources bool
	key                 string
	value               string
	operator            string
	nodeSelector        string
	priorityClass       string
	pvAccessModes       string
	size                string
	rotation            int
	storageClassName    string
	tailoringConfigMap  string
	template            string
}

type profileBundleDescription struct {
	name         string
	namespace    string
	contentimage string
	contentfile  string
	template     string
}

type scanSettingDescription struct {
	autoapplyremediations  bool
	autoupdateremediations bool
	name                   string
	namespace              string
	roles1                 string
	roles2                 string
	rotation               int
	schedule               string
	size                   string
	strictnodescan         bool
	suspend                bool
	debug                  bool
	priorityclassname      string
	template               string
	cpu_limit              string
	memory_limit           string
}

type scanSettingBindingDescription struct {
	name            string
	namespace       string
	profilekind1    string
	profilename1    string
	profilename2    string
	scansettingname string
	template        string
}

type tailoredProfileDescription struct {
	name         string
	namespace    string
	extends      string
	enrulename1  string
	disrulename1 string
	disrulename2 string
	varname      string
	value        string
	template     string
}

type tailoredProfileWithoutVarDescription struct {
	name         string
	namespace    string
	extends      string
	title        string
	description  string
	enrulename1  string
	rationale1   string
	enrulename2  string
	rationale2   string
	disrulename1 string
	drationale1  string
	disrulename2 string
	drationale2  string
	template     string
}

type tailoredProfileTwoVarsDescription struct {
	name      string
	namespace string
	extends   string
	varname1  string
	value1    string
	varname2  string
	value2    string
	template  string
}

type objectTableRef struct {
	kind      string
	namespace string
	name      string
}

type complianceScanDescription struct {
	name             string
	namespace        string
	scanType         string
	profile          string
	content          string
	contentImage     string
	rule             string
	debug            bool
	key              string
	value            string
	operator         string
	key1             string
	value1           string
	operator1        string
	nodeSelector     string
	pvAccessModes    string
	size             string
	storageClassName string
	template         string
}

type storageClassDescription struct {
	name              string
	provisioner       string
	reclaimPolicy     string
	volumeBindingMode string
	template          string
}

type resourceConfigMapDescription struct {
	name      string
	namespace string
	rule      int
	variable  int
	profile   int
	template  string
}

type pvExtractDescription struct {
	name      string
	namespace string
	scanname  string
	template  string
}

type priorityClassDescription struct {
	name         string
	namespace    string
	prirotyValue int
	template     string
}

type serviceDescription struct {
	name        string
	namespace   string
	profilekind string
	template    string
}

type counts struct {
	pods                int
	varibles            int
	services            int
	sa                  int
	scansettings        int
	rules               int
	routes              int
	roles               int
	rolebindings        int
	pvc                 int
	pv                  int
	profiles            int
	profilebundles      int
	leases              int
	events              int
	clusterroles        int
	clusterrolebindings int
	configmaps          int
}

func (service *serviceDescription) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", service.template, "-p", "NAME="+service.name,
		"NAMESPACE="+service.namespace, "PROFILEKIND="+service.profilekind)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (priorityClassD *priorityClassDescription) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-n", priorityClassD.namespace, "-f", priorityClassD.template, "-p", "NAME="+priorityClassD.name,
		"NAMESPACE="+priorityClassD.namespace, "PRIORITYVALUE="+strconv.Itoa(priorityClassD.prirotyValue))
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (csuite *complianceSuiteDescription) create(oc *exutil.CLI) {
	e2e.Logf("The value of debug is: %v", csuite.debug)
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", csuite.template, "-p", "NAME="+csuite.name, "NAMESPACE="+csuite.namespace,
		"SCHEDULE="+csuite.schedule, "SCANNAME="+csuite.scanname, "SCANTYPE="+csuite.scanType, "PROFILE="+csuite.profile, "CONTENT="+csuite.content, "PRIORITYCLASSNAME="+csuite.priorityClass,
		"CONTENTIMAGE="+csuite.contentImage, "RULE="+csuite.rule, "NOEXTERNALRESOURCES="+strconv.FormatBool(csuite.noExternalResources), "KEY="+csuite.key,
		"VALUE="+csuite.value, "OPERATOR="+csuite.operator, "NODESELECTOR="+csuite.nodeSelector, "PVACCESSMODE="+csuite.pvAccessModes, "STORAGECLASSNAME="+csuite.storageClassName,
		"SIZE="+csuite.size, "ROTATION="+strconv.Itoa(csuite.rotation), "TAILORCONFIGMAPNAME="+csuite.tailoringConfigMap, "DEBUG="+strconv.FormatBool(csuite.debug))
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (pvExtract *pvExtractDescription) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", pvExtract.template, "-p", "NAME="+pvExtract.name, "NAMESPACE="+pvExtract.namespace,
		"SCANNAME="+pvExtract.scanname, "-n", pvExtract.namespace)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (pb *profileBundleDescription) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", pb.template, "-p", "NAME="+pb.name, "NAMESPACE="+pb.namespace,
		"CONTENIMAGE="+pb.contentimage, "CONTENTFILE="+pb.contentfile, "-n", pb.namespace)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (ss *scanSettingDescription) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", ss.template, "-p", "NAME="+ss.name, "NAMESPACE="+ss.namespace, "PRIORITYCLASSNAME="+ss.priorityclassname,
		"AUTOAPPLYREMEDIATIONS="+strconv.FormatBool(ss.autoapplyremediations), "SCHEDULE="+ss.schedule, "SIZE="+ss.size, "ROTATION="+strconv.Itoa(ss.rotation), "ROLES1="+ss.roles1,
		"ROLES2="+ss.roles2, "STRICTNODESCAN="+strconv.FormatBool(ss.strictnodescan), "AUTOUPDATEREMEDIATIONS="+strconv.FormatBool(ss.autoupdateremediations), "DEBUG="+strconv.FormatBool(ss.debug),
		"SUSPEND="+strconv.FormatBool(ss.suspend), "CPU_LIMIT="+ss.cpu_limit, "MEMORY_LIMIT="+ss.memory_limit)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (ssb *scanSettingBindingDescription) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", ssb.template, "-p", "NAME="+ssb.name, "NAMESPACE="+ssb.namespace,
		"PROFILENAME1="+ssb.profilename1, "PROFILEKIND1="+ssb.profilekind1, "PROFILENAME2="+ssb.profilename2, "SCANSETTINGNAME="+ssb.scansettingname)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (csuite *complianceSuiteDescription) delete(itName string, dr describerResrouce) {
	dr.getIr(itName).remove(csuite.name, "compliancesuite", csuite.namespace)
}

func cleanupObjects(oc *exutil.CLI, objs ...objectTableRef) {
	for _, v := range objs {
		e2e.Logf("Start to remove: %v", v)
		_, err := oc.AsAdmin().WithoutNamespace().Run("delete").Args(v.kind, "-n", v.namespace, v.name, "--ignore-not-found").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
	}
}

func (cscan *complianceScanDescription) create(oc *exutil.CLI) {
	e2e.Logf("The value of debug is: %v", cscan.debug)
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", cscan.template, "-p", "NAME="+cscan.name,
		"NAMESPACE="+cscan.namespace, "SCANTYPE="+cscan.scanType, "PROFILE="+cscan.profile, "CONTENT="+cscan.content,
		"CONTENTIMAGE="+cscan.contentImage, "RULE="+cscan.rule, "KEY="+cscan.key, "VALUE="+cscan.value, "OPERATOR="+cscan.operator,
		"KEY1="+cscan.key1, "VALUE1="+cscan.value1, "OPERATOR1="+cscan.operator1, "NODESELECTOR="+cscan.nodeSelector,
		"PVACCESSMODE="+cscan.pvAccessModes, "STORAGECLASSNAME="+cscan.storageClassName, "SIZE="+cscan.size, "DEBUG="+strconv.FormatBool(cscan.debug))
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (tprofile *tailoredProfileDescription) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", tprofile.template, "-p", "NAME="+tprofile.name, "NAMESPACE="+tprofile.namespace,
		"EXTENDS="+tprofile.extends, "ENRULENAME1="+tprofile.enrulename1, "DISRULENAME1="+tprofile.disrulename1, "DISRULENAME2="+tprofile.disrulename2,
		"VARNAME="+tprofile.varname, "VALUE="+tprofile.value)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (tprofile *tailoredProfileWithoutVarDescription) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", tprofile.template, "-p", "NAME="+tprofile.name, "NAMESPACE="+tprofile.namespace,
		"EXTENDS="+tprofile.extends, "TITLE="+tprofile.title, "DISCRIPTION="+tprofile.description, "ENRULENAME1="+tprofile.enrulename1, "RATIONALE1="+tprofile.rationale1,
		"ENRULENAME2="+tprofile.enrulename2, "RATIONALE2="+tprofile.rationale2, "DISRULENAME1="+tprofile.disrulename1, "DRATIONALE1="+tprofile.drationale1,
		"DISRULENAME2="+tprofile.disrulename2, "DRATIONALE2="+tprofile.drationale2)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (sclass *storageClassDescription) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", sclass.template, "-p", "NAME="+sclass.name,
		"PROVISIONER="+sclass.provisioner, "RECLAIMPOLICY="+sclass.reclaimPolicy, "VOLUMEBINDINGMODE="+sclass.volumeBindingMode)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (confmap *resourceConfigMapDescription) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", confmap.template, "-p", "NAME="+confmap.name,
		"NAMESPACE="+confmap.namespace, "RULE="+strconv.Itoa(confmap.rule), "VARIABLE="+strconv.Itoa(confmap.variable), "PROFILE="+strconv.Itoa(confmap.profile))
	o.Expect(err).NotTo(o.HaveOccurred())
}

func checkComplianceSuiteStatus(oc *exutil.CLI, csuiteName string, nameSpace string, expected string) {
	err := wait.Poll(5*time.Second, 300*time.Second, func() (bool, error) {
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", nameSpace, "compliancesuite", csuiteName, "-o=jsonpath={.status.phase}").Output()
		e2e.Logf("the result of complianceSuite:%v", output)
		if strings.Contains(output, expected) {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("the status of %s is not expected %s", csuiteName, expected))
}

func checkComplianceScanStatus(oc *exutil.CLI, cscanName string, nameSpace string, expected string) {
	err := wait.Poll(5*time.Second, 300*time.Second, func() (bool, error) {
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", nameSpace, "compliancescan", cscanName, "-o=jsonpath={.status.phase}").Output()
		e2e.Logf("the result of complianceScan:%v", output)
		if strings.Contains(output, expected) {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("the status of %s is not expected %s", cscanName, expected))
}

func setLabelToNode(oc *exutil.CLI, label string) {
	nodeName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "--selector=node-role.kubernetes.io/worker=,node.openshift.io/os_id=rhcos",
		"-o=jsonpath={.items[*].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	node := strings.Fields(nodeName)
	for _, v := range node {
		_, err := oc.AsAdmin().WithoutNamespace().Run("label").Args("node", fmt.Sprintf("%s", v), label, "--overwrite").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
	}
}

func getOneRhcosWorkerNodeName(oc *exutil.CLI) string {
	nodeName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", "--selector=node-role.kubernetes.io/edge!=,node-role.kubernetes.io/worker=,node.openshift.io/os_id=rhcos",
		"-o=jsonpath={.items[0].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("the result of nodename:%v", nodeName)
	return nodeName
}

func (subD *subscriptionDescription) scanPodName(oc *exutil.CLI, expected string) {
	podName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", subD.namespace, "pods", "--selector=workload=scanner", "-o=jsonpath={.items[*].metadata.name}").Output()
	e2e.Logf("\n%v\n", podName)
	o.Expect(err).NotTo(o.HaveOccurred())
	pods := strings.Fields(podName)
	for _, pod := range pods {
		if strings.Contains(pod, expected) {
			continue
		}
	}
}

func (subD *subscriptionDescription) scanPodStatus(oc *exutil.CLI, expected string) {
	podStat, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", subD.namespace, "pods", "--selector=workload=scanner", "-o=jsonpath={.items[*].status.phase}").Output()
	e2e.Logf("\n%v\n", podStat)
	o.Expect(err).NotTo(o.HaveOccurred())
	lines := strings.Fields(podStat)
	for _, line := range lines {
		if strings.Contains(line, expected) {
			continue
		} else {
			e2e.Failf("Compliance scan failed on one or more nodes")
		}
	}
}

func (subD *subscriptionDescription) scanPodStatusWithLabel(oc *exutil.CLI, scanName string, expected string) {
	label := "compliance.openshift.io/scan-name=" + scanName + ",workload=scanner"
	podStat, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", subD.namespace, "pods", "--selector", label, "-o=jsonpath={.items[*].status.phase}").Output()
	e2e.Logf("\n%v\n", podStat)
	o.Expect(err).NotTo(o.HaveOccurred())
	lines := strings.Fields(podStat)
	for _, line := range lines {
		if strings.Contains(line, expected) {
			continue
		} else {
			e2e.Failf("Compliance scan failed on one or more nodes")
		}
	}
}

func (subD *subscriptionDescription) complianceSuiteName(oc *exutil.CLI, expected string) {
	csuiteName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", subD.namespace, "compliancesuite", "-o=jsonpath={.items[*].metadata.name}").Output()
	lines := strings.Fields(csuiteName)
	for _, line := range lines {
		if strings.Contains(line, expected) {
			e2e.Logf("\n%v\n\n", line)
			break
		}
	}
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (subD *subscriptionDescription) complianceScanName(oc *exutil.CLI, expected string) {
	cscanName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", subD.namespace, "compliancescan", "-o=jsonpath={.items[*].metadata.name}").Output()
	lines := strings.Fields(cscanName)
	for _, line := range lines {
		if strings.Contains(line, expected) {
			e2e.Logf("\n%v\n\n", line)
			break
		}
	}
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (subD *subscriptionDescription) complianceSuiteResult(oc *exutil.CLI, csuiteNmae string, expected string) {
	csuiteResult, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", subD.namespace, "compliancesuite", csuiteNmae, "-o=jsonpath={.status.result}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("the result of csuiteResult:%v", csuiteResult)
	expectedStrings := strings.Fields(expected)
	lenExpectedStrings := len(strings.Fields(expected))
	switch {
	case lenExpectedStrings == 1, strings.Compare(expected, csuiteResult) == 0:
		e2e.Logf("Case 1: the expected string %v equals csuiteResult %v", expected, expectedStrings)
		return
	case lenExpectedStrings == 2, strings.Compare(expectedStrings[0], csuiteResult) == 0 || strings.Compare(expectedStrings[1], csuiteResult) == 0:
		e2e.Logf("Case 2: csuiteResult %v equals expected string %v or %v", csuiteResult, expectedStrings[0], expectedStrings[1])
		return
	default:
		e2e.Failf("Default: The expected string %v doesn't contain csuiteResult %v", expected, csuiteResult)
	}
}

func (subD *subscriptionDescription) complianceScanResult(oc *exutil.CLI, expected string) {
	cscanResult, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", subD.namespace, "compliancescan", "-o=jsonpath={.items[*].status.result}").Output()
	lines := strings.Fields(cscanResult)
	for _, line := range lines {
		if strings.Compare(line, expected) == 0 {
			return
		}
		o.Expect(err).NotTo(o.HaveOccurred())
	}
}

func (subD *subscriptionDescription) getComplianceScanResult(oc *exutil.CLI, scanName string, expected string) {
	cscanResult, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", subD.namespace, "compliancescan", scanName, "-o=jsonpath={.status.result}").Output()
	lines := strings.Fields(cscanResult)
	for _, line := range lines {
		if strings.Compare(line, expected) == 0 {
			e2e.Logf("\n%v\n\n", line)
			return
		}
		o.Expect(err).NotTo(o.HaveOccurred())
	}
}

func (subD *subscriptionDescription) getScanExitCodeFromConfigmapWithScanName(oc *exutil.CLI, scanName string, expected string) {
	configmapLabel := "compliance.openshift.io/scan-name=" + scanName + ",complianceoperator.openshift.io/scan-result="
	configmapNameList, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", "-l", configmapLabel, "-n", subD.namespace, "-o=jsonpath={.items[*].metadata.name}").Output()
	configmaps := strings.Fields(configmapNameList)
	for _, cm := range configmaps {
		cmCode, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", cm, "-n", subD.namespace, "-o=jsonpath={.data.exit-code}").Output()
		e2e.Logf("\n%v\n\n", cmCode)
		if strings.Contains(cmCode, expected) {
			break
		}
		o.Expect(err).NotTo(o.HaveOccurred())
	}
}

func (subD *subscriptionDescription) getScanExitCodeFromConfigmapWithSuiteName(oc *exutil.CLI, suiteName string, expected string) {
	label := "compliance.openshift.io/suite=" + suiteName
	scanNameList, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", subD.namespace, "scan", "-l", label, "-o=jsonpath={.items[*].metadata.name}").Output()
	scanName := strings.Fields(scanNameList)
	for _, scan := range scanName {
		subD.getScanExitCodeFromConfigmapWithScanName(oc, scan, expected)
	}
}

func (subD *subscriptionDescription) getScanResultFromConfigmap(oc *exutil.CLI, scanName string, expected string) {
	configmapLabel := "compliance.openshift.io/scan-name=" + scanName + ",complianceoperator.openshift.io/scan-result="
	configmaps, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", subD.namespace, "configmap", "-l", configmapLabel, "-o=jsonpath={.items[*].metadata.name}").Output()
	cms := strings.Fields(configmaps)
	for _, cm := range cms {
		cmMsgs, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", subD.namespace, "configmap", cm, "-o=jsonpath={.data.error-msg}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(cmMsgs, expected) {
			continue
		} else {
			e2e.Failf("No expected error-msg !!!")
		}
	}
}

func (subD *subscriptionDescription) getPVCName(oc *exutil.CLI, expected string) {
	pvcName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", subD.namespace, "pvc", "-o=jsonpath={.items[*].metadata.name}").Output()
	lines := strings.Fields(pvcName)
	for _, line := range lines {
		if strings.Contains(line, expected) {
			e2e.Logf("\n%v\n\n", line)
			break
		}
	}
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (subD *subscriptionDescription) getPVCSize(oc *exutil.CLI, expected string) {
	pvcSize, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", subD.namespace, "pvc", "-o=jsonpath={.items[*].status.capacity.storage}").Output()
	lines := strings.Fields(pvcSize)
	for _, line := range lines {
		if strings.Contains(line, expected) {
			e2e.Logf("\n%v\n\n", line)
			break
		}
	}
	o.Expect(err).NotTo(o.HaveOccurred())
}

func getRuleStatus(oc *exutil.CLI, remsRule []string, expected string, scanName string, namespace string) {
	err := wait.Poll(5*time.Second, 30*time.Second, func() (bool, error) {
		for _, rule := range remsRule {
			ruleStatus, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("compliancecheckresult", scanName+"-"+rule, "-n", namespace, "-o=jsonpath={.status}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Compare(ruleStatus, expected) == 0 {
				return true, nil
			}
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("The rule status %s is not matching", expected))
}

func (subD *subscriptionDescription) getProfileBundleNameandStatus(oc *exutil.CLI, pbName string, status string) {
	err := wait.Poll(5*time.Second, 30*time.Second, func() (bool, error) {
		pbStatus, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", subD.namespace, "profilebundles", pbName, "-o=jsonpath={.status.dataStreamStatus}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Compare(pbStatus, status) == 0 {
			e2e.Logf("\n%v\n\n", pbStatus)
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("the status of %s profilebundle is not VALID", pbName))
}

func (subD *subscriptionDescription) getTailoredProfileNameandStatus(oc *exutil.CLI, expected string) {
	err := wait.Poll(5*time.Second, 30*time.Second, func() (bool, error) {
		tpName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", subD.namespace, "tailoredprofile", "-o=jsonpath={.items[*].metadata.name}").Output()
		lines := strings.Fields(tpName)
		for _, line := range lines {
			if strings.Compare(line, expected) == 0 {
				e2e.Logf("\n%v\n\n", line)
				// verify tailoredprofile status
				tpStatus, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", subD.namespace, "tailoredprofile", line, "-o=jsonpath={.status.state}").Output()
				e2e.Logf("\n%v\n\n", tpStatus)
				o.Expect(tpStatus).To(o.ContainSubstring("READY"))
				o.Expect(err).NotTo(o.HaveOccurred())
				return true, nil
			}
		}
		o.Expect(err).NotTo(o.HaveOccurred())
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "the status of tailoredprofile is not READY")
}

func (subD *subscriptionDescription) getProfileName(oc *exutil.CLI, expected string) {
	err := wait.Poll(2*time.Second, 10*time.Second, func() (bool, error) {
		pName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", subD.namespace, "profile.compliance", "-o=jsonpath={.items[*].metadata.name}").Output()
		lines := strings.Fields(pName)
		for _, line := range lines {
			if strings.Compare(line, expected) == 0 {
				e2e.Logf("\n%v\n\n", line)
				return true, nil
			}
		}
		o.Expect(err).NotTo(o.HaveOccurred())
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "The profile name does not match")
}

func (subD *subscriptionDescription) getARFreportFromPVC(oc *exutil.CLI, pvPodName string, expected string) {
	err := wait.Poll(2*time.Second, 10*time.Second, func() (bool, error) {
		commands := []string{"-n", subD.namespace, "exec", "pod/" + pvPodName, "--", "ls", "/workers-scan-results/0"}
		arfReport, err := oc.AsAdmin().WithoutNamespace().Run(commands...).Args().Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		lines := strings.Fields(arfReport)
		for _, line := range lines {
			if strings.Contains(line, expected) {
				return true, nil
			}
		}
		o.Expect(err).NotTo(o.HaveOccurred())
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "The ARF report from PVC failed")
}

func assertCoPodNumerEqualNodeNumber(oc *exutil.CLI, namespace string, nodeLabel string, scanName string) {
	intNodeNumber := getNodeNumberPerLabel(oc, nodeLabel)
	podNameString, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", namespace, "-l", "compliance.openshift.io/scan-name="+scanName+",workload=scanner", "-o=jsonpath={.items[*].metadata.name}").Output()
	e2e.Logf("the result of podNameString:%v", podNameString)
	o.Expect(err).NotTo(o.HaveOccurred())
	intPodNumber := len(strings.Fields(podNameString))
	e2e.Logf("the result of intNodeNumber:%v", intNodeNumber)
	e2e.Logf("the result of intPodNumber:%v", intPodNumber)
	if intNodeNumber != intPodNumber {
		e2e.Failf("the intNodeNumber and intPodNumber not equal!")
	}
}

func getResourceNameWithKeyword(oc *exutil.CLI, rs string, namespace string, keyword string) string {
	var resourceName string
	rsList, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args(rs, "-n", namespace, "-o=jsonpath={.items[*].metadata.name}").Output()
	rsl := strings.Fields(rsList)
	for _, v := range rsl {
		resourceName = fmt.Sprintf("%s", v)
		if strings.Contains(resourceName, keyword) {
			break
		}
	}
	if resourceName == "" {
		e2e.Failf("Failed to get resource name!")
	}
	return resourceName
}

func getResourceNameWithKeywordFromResourceList(oc *exutil.CLI, rs string, namespace string, keyword string) string {
	var result, resourceName string
	err := wait.Poll(1*time.Second, 120*time.Second, func() (bool, error) {
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args(rs, "-n", namespace, "-o=jsonpath={.items[*].metadata.name}").Output()
		e2e.Logf("the result of output:%v", output)
		if strings.Contains(output, keyword) {
			result = output
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("the rs does not has %s", keyword))
	rsl := strings.Fields(result)
	for _, v := range rsl {
		resourceName = fmt.Sprintf("%s", v)
		if strings.Contains(resourceName, keyword) {
			break
		}
	}
	if resourceName == "" {
		e2e.Failf("Failed to get resource name!")
	}
	return resourceName
}

func checkKeyWordsForRspod(oc *exutil.CLI, podname string, namespace string, keyword [3]string) {
	var flag = true
	var kw string
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", podname, "-n", namespace, "-o=json").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	for _, v := range keyword {
		kw = fmt.Sprintf("%s", v)
		if !strings.Contains(output, kw) {
			e2e.Failf("The keyword %kw not exist!", v)
			flag = false
			break
		}
	}
	if flag == false {
		e2e.Failf("The keyword not exist!")
	}
}

func checkResourceNumber(oc *exutil.CLI, exceptedRsNo int, parameters ...string) {
	rs, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(parameters...).OutputToFile(getRandomString() + "isc-rs.json")
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("the result of rs:%v", rs)
	result, err := exec.Command("bash", "-c", "cat "+rs+" | wc -l").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	r1 := strings.TrimSpace(string(result))
	rsNumber, _ := strconv.Atoi(r1)
	if rsNumber < exceptedRsNo {
		e2e.Failf("The rsNumber %v not equals the exceptedRsNo %v!", rsNumber, exceptedRsNo)
	}
}

func checkWarnings(oc *exutil.CLI, expectedString string, parameters ...string) {
	rs, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(parameters...).OutputToFile(getRandomString() + "isc-rs.json")
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("the result of rs:%v", rs)
	result, err := exec.Command("bash", "-c", "cat "+rs+" | awk '{print $1}'").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	checkresults := strings.Fields(string(result))
	for _, checkresult := range checkresults {
		e2e.Logf("the result of checkresult:%v", checkresult)
		instructions, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("compliancecheckresult", checkresult, "-n", oc.Namespace(),
			"-o=jsonpath={.warnings}").Output()
		e2e.Logf("the result of instructions:%v", instructions)
		if !strings.Contains(instructions, expectedString) {
			e2e.Failf("The instructions %v don't contain expectedString %v!", instructions, expectedString)
			break
		}
		o.Expect(err).NotTo(o.HaveOccurred())
	}
}

func checkFipsStatus(oc *exutil.CLI, namespace string) string {
	mnodeName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", "--selector=node.openshift.io/os_id=rhcos,node-role.kubernetes.io/master=",
		"-o=jsonpath={.items[0].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	efips, err := oc.AsAdmin().WithoutNamespace().Run("debug").Args("-n", namespace, "node/"+mnodeName, "--", "chroot", "/host", "fips-mode-setup", "--check").Output()
	if strings.Contains(efips, "FIPS mode is disabled.") {
		e2e.Logf("Fips is disabled on master node %v ", mnodeName)
	} else {
		e2e.Logf("Fips is enabled on master node %v ", mnodeName)
	}
	o.Expect(err).NotTo(o.HaveOccurred())
	return efips
}

func checkInstructionsForManualRules(oc *exutil.CLI, compliancesuite string) {
	label := fmt.Sprintf("compliance.openshift.io/check-status=MANUAL,compliance.openshift.io/suite=%s", compliancesuite)
	out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("compliancecheckresult", "-n", oc.Namespace(), "-l", label, "-o=jsonpath={.items[*].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	list := strings.Fields(out)
	for _, rule := range list {
		ruleinst, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("compliancecheckresult", rule, "-n", oc.Namespace(), "-o=jsonpath={.instructions}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if ruleinst == "" {
			e2e.Failf("This CIS rule '%v' do not have any instruction", rule)
		} else {
			e2e.Logf("This CIS rule '%v' has instruction", rule)
		}
	}
}

func checkOauthPodsStatus(oc *exutil.CLI) {
	newCheck("expect", asAdmin, withoutNamespace, contain, "Pending", ok, []string{"pods", "-n", "openshift-authentication",
		"-o=jsonpath={.items[*].status.phase}"}).check(oc)
	newCheck("expect", asAdmin, withoutNamespace, contain, "3", ok, []string{"deployment", "oauth-openshift", "-n", "openshift-authentication",
		"-o=jsonpath={.status.readyReplicas}"}).check(oc)
	podnames, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", "openshift-authentication", "-o=jsonpath={.items[*].metadata.name}").Output()
	podname := strings.Fields(podnames)
	for _, v := range podname {
		newCheck("expect", asAdmin, withoutNamespace, contain, "Running", ok, []string{"pods", v, "-n", "openshift-authentication",
			"-o=jsonpath={.status.phase}"}).check(oc)
	}

}

func checkComplianceSuiteResult(oc *exutil.CLI, namespace string, csuiteNmae string, expected string) {
	csuiteResult, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", namespace, "compliancesuite", csuiteNmae, "-o=jsonpath={.status.result}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("the result of csuiteResult:%v", csuiteResult)
	expectedStrings := strings.Fields(expected)
	lenExpectedStrings := len(strings.Fields(expected))
	switch {
	case lenExpectedStrings == 1, strings.Compare(expected, csuiteResult) == 0:
		e2e.Logf("Case 1: the expected string %v equals csuiteResult %v", expected, expectedStrings)
		return
	case lenExpectedStrings == 2, strings.Compare(expectedStrings[0], csuiteResult) == 0 || strings.Compare(expectedStrings[1], csuiteResult) == 0:
		e2e.Logf("Case 2: csuiteResult %v equals expected string %v or %v", csuiteResult, expectedStrings[0], expectedStrings[1])
		return
	default:
		e2e.Failf("Default: The expected string %v doesn't contain csuiteResult %v", expected, csuiteResult)
	}
}

func getResourceNameWithKeywordForNamespace(oc *exutil.CLI, rs string, keyword string, namespace string) string {
	var resourceName string
	rsList, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args(rs, "-n", namespace, "-o=jsonpath={.items[*].metadata.name}").Output()
	rsl := strings.Fields(rsList)
	for _, v := range rsl {
		resourceName = fmt.Sprintf("%s", v)
		e2e.Logf("the result of resourceName:%v", resourceName)
		if strings.Contains(resourceName, keyword) {
			break
		}
	}
	if resourceName == "" {
		e2e.Failf("Failed to get resource name!")
	}
	return resourceName
}

func checkOperatorPodStatus(oc *exutil.CLI, namespace string) string {
	var podname string
	podnames, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", namespace).Output()
	podname = fmt.Sprintf("%s", podnames)
	if strings.Contains(podname, "cluster-logging-operator") {
		podStat, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", namespace, "-o=jsonpath={.items[0].status.phase}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		return podStat
	}
	return podname
}

func assertCheckAuditLogsForword(oc *exutil.CLI, namespace string, csvname string) {
	var auditlogs string
	podnames, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-l logging-infra=fluentdserver", "-n", namespace, "-o=jsonpath={.items[0].metadata.name}").Output()
	auditlog, err := oc.AsAdmin().WithoutNamespace().Run("rsh").Args("-n", namespace, podnames, "cat", "/fluentd/log/audit.log").OutputToFile(getRandomString() + "isc-audit.log")
	o.Expect(err).NotTo(o.HaveOccurred())
	result, err1 := exec.Command("bash", "-c", "cat "+auditlog+" | grep "+csvname+" |tail -n5; rm -rf "+auditlog).Output()
	o.Expect(err1).NotTo(o.HaveOccurred())
	auditlogs = fmt.Sprintf("%s", result)
	if strings.Contains(auditlogs, csvname) {
		e2e.Logf("The keyword does match with auditlogs: %v", csvname)
	} else {
		e2e.Failf("The keyword does not match with auditlogs: %v", csvname)
	}
}

func createLoginTemp(oc *exutil.CLI, namespace string) {
	e2e.Logf("Create a login.html template.. !!")
	login, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("create-login-template", "-n", namespace).OutputToFile(getRandomString() + "login.html")
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("Create a login-secret.. !!")
	_, err1 := oc.AsAdmin().WithoutNamespace().Run("create").Args("secret", "generic", "login-secret", "--from-file=login.html="+login, "-n", namespace).Output()
	o.Expect(err1).NotTo(o.HaveOccurred())
}

func getNonControlNamespaces(oc *exutil.CLI, status string) []string {
	e2e.Logf("Get the all non-control plane '%s' namespaces... !!\n", status)
	projects, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("projects", "--no-headers", "-n", oc.Namespace()).OutputToFile(getRandomString() + "project.json")
	o.Expect(err).NotTo(o.HaveOccurred())
	result, _ := exec.Command("bash", "-c", "cat "+projects+" | grep -v -e default -e kube- -e openshift | grep -e "+status+" | sed \"s/"+status+"//g\" ; rm -rf "+projects).Output()
	projectList := strings.Fields(string(result))
	return projectList
}

func getNonControlNamespacesWithoutStatusChecking(oc *exutil.CLI) []string {
	e2e.Logf("Get the all non-control plane namespaces without status check... !!\n")
	projects, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("projects", "--no-headers", "--all-namespaces", "-n", oc.Namespace()).OutputToFile(getRandomString() + "project.json")
	o.Expect(err).NotTo(o.HaveOccurred())
	result, _ := exec.Command("bash", "-c", "cat "+projects+" | awk '{print $1}'| grep -Ev \"default|kube-|openshift\"; rm -rf "+projects).Output()
	projectList := strings.Fields(string(result))
	return projectList
}

func checkRulesExistInComplianceCheckResult(oc *exutil.CLI, cisRlueList []string, namespace string) {
	ccr, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ccr", "-n", namespace, "-o=jsonpath={.items[*].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	for _, v := range cisRlueList {
		if !strings.Contains(ccr, v) {
			e2e.Failf("The ccr list %s doesn't contains the rule %v", ccr, v)
		}
	}
}

func setLabelToOneWorkerNode(oc *exutil.CLI, workerNodeName string) {
	nodeLabel, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", workerNodeName, "--show-labels").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if !strings.Contains(nodeLabel, "node-role.kubernetes.io/wrscan=") {
		_, err := oc.AsAdmin().WithoutNamespace().Run("label").Args("node", workerNodeName, "node-role.kubernetes.io/wrscan=").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
	}
}

func removeLabelFromWorkerNode(oc *exutil.CLI, workerNodeName string) {
	_, err := oc.AsAdmin().WithoutNamespace().Run("label").Args("node", workerNodeName, "node-role.kubernetes.io/wrscan-").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("\nThe label is removed from node %s \n", workerNodeName)
}

func checkMachineConfigPoolStatus(oc *exutil.CLI, nodeSelector string) {
	err := wait.Poll(10*time.Second, 900*time.Second, func() (bool, error) {
		mCount, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("mcp", nodeSelector, "-n", oc.Namespace(), "-o=jsonpath={.status.machineCount}").Output()
		e2e.Logf("MachineCount:%v", mCount)
		unmCount, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("mcp", nodeSelector, "-n", oc.Namespace(), "-o=jsonpath={.status.unavailableMachineCount}").Output()
		e2e.Logf("unavailableMachineCount:%v", unmCount)
		dmCount, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("mcp", nodeSelector, "-n", oc.Namespace(), "-o=jsonpath={.status.degradedMachineCount}").Output()
		e2e.Logf("degradedMachineCount:%v", dmCount)
		rmCount, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("mcp", nodeSelector, "-n", oc.Namespace(), "-o=jsonpath={.status.readyMachineCount}").Output()
		e2e.Logf("ReadyMachineCount:%v", rmCount)
		if strings.Compare(mCount, rmCount) == 0 && strings.Compare(unmCount, "0") == 0 && strings.Compare(dmCount, "0") == 0 {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Fails to update %v machineconfigpool", nodeSelector))
}

func checkNodeContents(oc *exutil.CLI, nodeName string, namespace string, contentList []string, cmd string, opt string, filePath string, pattern string) {
	err := wait.Poll(5*time.Second, 30*time.Second, func() (bool, error) {
		nContent, err := oc.AsAdmin().WithoutNamespace().Run("debug").Args("nodes/"+nodeName, "-n", namespace, "--", "chroot", "/host", cmd, opt, filePath).OutputToFile(getRandomString() + "content.json")
		o.Expect(err).NotTo(o.HaveOccurred())
		results, _ := exec.Command("bash", "-c", "cat "+nContent+" | grep "+pattern+"; rm -rf "+nContent).Output()
		result := string(results)
		for _, line := range contentList {
			if !strings.Contains(result, line) {
				return false, nil
			}
			e2e.Logf("The string '%s' contains in '%s' file on node \n", line, filePath)
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("The string does not contain in '%s' file on node", filePath))
}

func checkNodeStatus(oc *exutil.CLI) {
	err := wait.Poll(10*time.Second, 1*time.Minute, func() (bool, error) {
		nodeName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "--selector=node-role.kubernetes.io/worker=", "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		node := strings.Fields(nodeName)
		//"reason":"KubeletReady","status":"True","type":"Ready"
		nodeConditionStr := "reason.*KubeletReady.*status.*True.*type.*Ready"
		for _, v := range node {
			nodeConditions, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", fmt.Sprintf("%s", v), "-o=jsonpath={.status.conditions}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			matched, err := regexp.MatchString(nodeConditionStr, nodeConditions)
			o.Expect(err).NotTo(o.HaveOccurred())
			if err != nil || !matched {
				return false, nil
			}
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, "One or more nodes are NotReady state")
}

func extractResultFromConfigMap(oc *exutil.CLI, label string, namespace string) {
	err := wait.Poll(5*time.Second, 30*time.Second, func() (bool, error) {
		nodeName, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "--selector=node-role.kubernetes.io/"+label+"=,node.openshift.io/os_id=rhcos", "-o=jsonpath={.items[0].metadata.name}", "-n", namespace).Output()
		e2e.Logf("%s nodename : %s \n", label, nodeName)
		cmNames, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", "-lcompliance.openshift.io/scan-name=ocp4-cis-node-"+label+",complianceoperator.openshift.io/scan-result=", "--no-headers", "-ojsonpath={.items[*].metadata.name}", "-n", namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		cmName := strings.Fields(cmNames)
		for _, v := range cmName {
			cmResult, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", v, "-ojsonpath={.data.results}", "-n", namespace).OutputToFile(getRandomString() + "result.json")
			o.Expect(err).NotTo(o.HaveOccurred())
			result, _ := exec.Command("bash", "-c", "cat "+cmResult+" | grep -e \"<target>\" -e identifier ; rm -rf "+cmResult).Output()
			tiResult := string(result)
			if strings.Contains(tiResult, nodeName) {
				e2e.Logf("Node name '%s' shows in ComplianceScan XCCDF format result \n\n %s \n", nodeName, tiResult)
				return true, nil
			}
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "Node name does not matches with the ComplianceScan XCCDF result output")
}

func genFluentdSecret(oc *exutil.CLI, namespace string, serverName string) {
	baseDir := exutil.FixturePath("testdata", "securityandcompliance")
	keysPath := filepath.Join(baseDir, "temp"+getRandomString())
	defer exec.Command("rm", "-r", keysPath).Output()
	err := os.MkdirAll(keysPath, 0755)
	o.Expect(err).NotTo(o.HaveOccurred())
	//generate certs
	generateCertsSH := exutil.FixturePath("testdata", "securityandcompliance", "cert_generation.sh")
	cmd := []string{generateCertsSH, keysPath, namespace, serverName}
	e2e.Logf("%s", cmd)
	_, err1 := exec.Command("sh", cmd...).Output()
	o.Expect(err1).NotTo(o.HaveOccurred())
	e2e.Logf("The certificates and keys are generated for %s \n", serverName)
	//create secret for fluentd server
	_, err2 := oc.AsAdmin().WithoutNamespace().Run("create").Args("secret", "generic", serverName, "-n", namespace, "--from-file=ca-bundle.crt="+keysPath+"/ca.crt", "--from-file=tls.key="+keysPath+"/logging-es.key", "--from-file=tls.crt="+keysPath+"/logging-es.crt", "--from-file=ca.key="+keysPath+"/ca.key").Output()
	o.Expect(err2).NotTo(o.HaveOccurred())
	e2e.Logf("The secrete is generated for %s in %s namespace \n", serverName, namespace)
}

func getOperatorResources(oc *exutil.CLI, resourcename string, namespace string) int {
	rList, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(resourcename, "-n", namespace, "-o=jsonpath={.items[*].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	orgCnt := len(strings.Fields(rList))
	return orgCnt
}

func readFileLinesToCompare(oc *exutil.CLI, confMap string, actCnt int, namespace string, resName string) {
	err := wait.Poll(5*time.Second, 30*time.Second, func() (bool, error) {
		rsPath, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", confMap, "-n", namespace, "-ojsonpath={.data."+resName+"}").OutputToFile(getRandomString() + ".json")
		o.Expect(err).NotTo(o.HaveOccurred())
		result, _ := exec.Command("bash", "-c", "cat "+rsPath+"; rm -rf "+rsPath).Output()
		orgCnt, _ := strconv.Atoi(string(result))
		if actCnt >= orgCnt {
			e2e.Logf("The original %s count before upgrade was %s and that matches with the actual %s count %s after upgrade \n", orgCnt, strconv.Itoa(orgCnt), resName, strconv.Itoa(actCnt))
			return true, nil
		}
		e2e.Logf("The original %s count before upgrade was %s and that does not matches with the actual %s count %s after upgrade \n", resName, strconv.Itoa(orgCnt), resName, strconv.Itoa(actCnt))
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("The orignal count before upgrade does not match with the actual count after upgrade \n"))
}

func labelNameSpace(oc *exutil.CLI, namespace string, label string) {
	err := oc.AsAdmin().WithoutNamespace().Run("label").Args("namespace", namespace, label, "--overwrite").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The namespace %s is labeled by %q", namespace, label)

}

func getSAToken(oc *exutil.CLI, account string, ns string) string {
	e2e.Logf("Getting a token assgined to specific serviceaccount from %s namespace...", ns)
	token, err := oc.AsAdmin().WithoutNamespace().Run("create").Args("token", account, "-n", ns).Output()
	if err != nil {
		if strings.Contains(token, "unknown command") {
			token, err = oc.AsAdmin().WithoutNamespace().Run("sa").Args("get-token", account, "-n", ns).Output()
		}
	}
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(token).NotTo(o.BeEmpty())
	return token
}

func checkMetric(oc *exutil.CLI, metricString []string, url string) {
	token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")
	var metricsLog []byte
	err := wait.Poll(5*time.Second, 180*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-monitoring", "-c", "prometheus", "prometheus-k8s-0", "--", "curl", "-k", "-H", fmt.Sprintf("Authorization: Bearer %v", token), url).OutputToFile("metrics.txt")
		if err != nil {
			e2e.Logf("Can't get metrics and try again, the error is:%s", err)
			return false, nil
		}
		metricsLog, _ = exec.Command("bash", "-c", "cat "+output+" | grep =; rm -rf "+output).Output()
		for _, metricStr := range metricString {
			if !strings.Contains(string(metricsLog), metricStr) {
				return false, nil
			}
		}
		return true, nil
	})
	if err != nil {
		e2e.Logf("The result of the metrics log is: %s", string(metricsLog))
	}
	exutil.AssertWaitPollNoErr(err, "The expected string does not contain in the matrics")
}

func getRemRuleStatus(oc *exutil.CLI, suiteName string, expected string, namespace string) {
	err := wait.Poll(5*time.Second, 30*time.Second, func() (bool, error) {
		remsRules, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("complianceremediations", "--no-headers", "-lcompliance.openshift.io/suite="+suiteName, "-n", namespace).OutputToFile(getRandomString() + "remrules.json")
		o.Expect(err).NotTo(o.HaveOccurred())
		result, _ := exec.Command("bash", "-c", "cat "+remsRules+" | grep -v -e protect-kernel-defaults; rm -rf "+remsRules).Output()
		remsRule := strings.Fields(string(result))
		for _, rule := range remsRule {
			ruleStatus, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("compliancecheckresult", rule, "-n", namespace, "-o=jsonpath={.status}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Compare(ruleStatus, expected) == 0 {
				return true, nil
			}
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("The remsRule status %s is not matching", expected))
}

func verifyTailoredProfile(oc *exutil.CLI, errmsgs []string, namespace string, filename string) {
	err := wait.Poll(2*time.Second, 20*time.Second, func() (bool, error) {
		tpErr, _ := oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", namespace, "-f", filename).Output()
		for _, errmsg := range errmsgs {
			if strings.Contains(errmsg, tpErr) {
				return true, nil
			}
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "The tailoredprofile requires title and description to create")
}

func assertKeywordsExists(oc *exutil.CLI, timeout int, keywords string, parameters ...string) {
	errWait := wait.Poll(5*time.Second, time.Duration(timeout)*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(parameters...).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if matched, _ := regexp.MatchString(keywords, string(output)); matched {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(errWait, fmt.Sprintf("The keywords %s not exists", keywords))
}

func getAlertManager(oc *exutil.CLI) string {
	exutil.AssertPodToBeReady(oc, "prometheus-k8s-0", "openshift-monitoring")
	alertManager, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("route", "alertmanager-main", "-n", "openshift-monitoring", "-o=jsonpath={.spec.host}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(alertManager).NotTo(o.BeEmpty())
	return alertManager
}

func checkAlert(oc *exutil.CLI, labelName string, alertString string, timeout time.Duration) {
	var alerts []byte
	token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")
	url := getAlertManager(oc)
	alertManagerVersion, errGetAlertmanagerVersion := getAlertmanagerVersion(oc)
	o.Expect(errGetAlertmanagerVersion).NotTo(o.HaveOccurred())
	gjsonQueryAlertName, gjsonQueryAlertNameEqual, errGetAlertQueries := getAlertQueriesWithLabelName(oc)
	o.Expect(errGetAlertQueries).NotTo(o.HaveOccurred())
	alertCMD := fmt.Sprintf("curl -s -k -H \"Authorization: Bearer %s\" https://%s/api/%s/alerts", token, url, alertManagerVersion)
	err := wait.Poll(3*time.Second, timeout*time.Second, func() (bool, error) {
		alerts, _ = exec.Command("bash", "-c", alertCMD).Output()
		if strings.Contains(gjson.Get(string(alerts), gjsonQueryAlertName).String(), labelName) && strings.Contains(gjson.Get(string(alerts), gjsonQueryAlertNameEqual+labelName+").annotations.description").String(), alertString) {
			return true, nil
		}
		return false, nil
	})

	if err != nil {
		e2e.Logf("The alerts are: %s", string(alerts))
		e2e.Logf("The description for the query alerts is: %s", gjson.Get(string(alerts), gjsonQueryAlertNameEqual+labelName+").annotations.description").String())
	}
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("The alerts NOT to contain %s", alertString))
}

func getAlertQueriesWithLabelName(oc *exutil.CLI) (string, string, error) {
	version, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterversion/version", "-ojsonpath={.status.desired.version}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	major := strings.Split(version, ".")[0]
	minor := strings.Split(version, ".")[1]
	minorInt, err := strconv.Atoi(minor)
	o.Expect(err).NotTo(o.HaveOccurred())
	switch {
	case major == "4" && minorInt >= 17:
		return "#.labels.name", "#(labels.name=", nil
	case major == "4" && minorInt < 17:
		return "data.#.labels.name", "data.#(labels.name=", nil
	default:
		return "", "", errors.New("Unknown version " + major + "." + minor)
	}
}

func assertParameterValueForBulkPods(oc *exutil.CLI, expected string, parameters ...string) {
	result, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(parameters...).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	rList := strings.Fields(result)
	for _, res := range rList {
		o.Expect(res).Should(o.Equal(expected), fmt.Sprintf("The %s NOT equals to %s", res, expected))
	}
}

func assertEventMessageRegexpMatch(oc *exutil.CLI, expected string, parameters ...string) {
	var eventsMessage string
	err := wait.Poll(5*time.Second, 300*time.Second, func() (bool, error) {
		eventsMessage, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(parameters...).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if matched, _ := regexp.MatchString(expected, eventsMessage); matched {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		e2e.Logf("The event %s NOT to contain %s", eventsMessage, expected)
		exutil.AssertWaitPollNoErr(err, "the expected message is missed")
	}
}

// skip cluster when apiserver encryption type is eqauls to aescbc as enable/disable encryption is destructive and time consuming
// will add support for aesgcm when the related bug https://issues.redhat.com/browse/OCPBUGS-11696 fixed
func skipEtcdEncryptionOn(oc *exutil.CLI) {
	encryptionType, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("apiserver/cluster", "-o=jsonpath={.spec.encryption.type}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if encryptionType != "aescbc" {
		g.Skip(fmt.Sprintf("EncryptionType %s is not aescbc, SKIP!", encryptionType))
	}
}

// skip cluster when apiserver encryption type is eqauls to aescbc as enable/disable encryption is destructive and time consuming
func skipEtcdEncryptionOff(oc *exutil.CLI) {
	encryptionType, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("apiserver/cluster", "-o=jsonpath={.spec.encryption.type}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if encryptionType != "aescbc" && encryptionType != "aesgcm" {
		g.Skip(fmt.Sprintf("ETCD encryption disabled, SKIP!", encryptionType))
	}
}

func checkInstructionNotEmpty(oc *exutil.CLI, namespace string, ssbName string, exemptRuleList []string) {
	ccrs, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("compliancecheckresult", "-n", namespace, "-l", "compliance.openshift.io/suite="+ssbName,
		"-o=jsonpath={.items[*].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	ccrList := strings.Fields(ccrs)
	for _, ccr := range ccrList {
		if !IsContainStr(exemptRuleList, ccr) {
			instruction, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("compliancecheckresult", ccr, "-n", namespace, "-o=jsonpath={.instructions}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if instruction == "" {
				e2e.Failf("This ccr '%v' do not have any instruction", ccr)
			}
		}
	}
}

func IsContainStr(items []string, item string) bool {
	for _, eachItem := range items {
		if eachItem == item {
			return true
		}
	}
	return false
}

func setApplyToFalseForAllCrs(oc *exutil.CLI, namespace string, ssbName string) {
	crList, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("complianceremediaiton", "-n", namespace, "-l", "compliance.openshift.io/suite="+ssbName,
		"-o=jsonpath={.items[*].metadata.name}").Output()
	if err != nil || strings.Contains(crList, "NotFound") {
		return
	} else {
		o.Expect(err).NotTo(o.HaveOccurred())
	}
	crs := strings.Fields(crList)
	patch := fmt.Sprintf("{\"spec\":{\"apply\":false}}")
	for _, cr := range crs {
		patchResource(oc, asAdmin, withoutNamespace, "complianceremediaiton", cr, "-n", namespace, "--type", "merge", "-p", patch)
		newCheck("expect", asAdmin, withoutNamespace, contain, "NotApplied", ok, []string{"complianceremediaiton", cr, "-n", namespace,
			"-o=jsonpath={.status.applicationState}"}).check(oc)
	}
}

func assertCompliancescanDone(oc *exutil.CLI, namespace string, parameters ...string) {
	var res string
	var errRes error
	err := wait.Poll(5*time.Second, 360*time.Second, func() (bool, error) {
		res, errRes = oc.AsAdmin().WithoutNamespace().Run("get").Args(parameters...).Output()
		if errRes != nil {
			return false, errRes
		}
		return res == "DONE", nil
	})
	if err != nil {
		// Expose more info when scan not finished in the hardcoded timer
		e2e.Logf("The scan phase is: ", res)
		resPod, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", namespace).Output()
		e2e.Logf("The result of \"oc get pod -n %s\" is: %s", namespace, resPod)
		resPv, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pv", "-n", namespace).Output()
		e2e.Logf("The result of \"oc get pv -n %s\" is: %s", namespace, resPv)
		resPvc, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pvc", "-n", namespace).Output()
		e2e.Logf("The result of \"oc get pvc -n %s\" is: %s", namespace, resPvc)
	}
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("The compliance scan not finished in the limited timer"))
}

func checkRuleCountMatch(oc *exutil.CLI, ns string, scanName string) {
	ccrNames, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ccr", "-l", "compliance.openshift.io/scan-name="+scanName, "-n", ns, "-o=jsonpath={.items[*].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	ccrCount := len(strings.Fields(ccrNames))
	checkCount, errGet := oc.AsAdmin().WithoutNamespace().Run("get").Args("scan", scanName, "-n", ns, "-o=jsonpath={.metadata.annotations.compliance\\.openshift\\.io/check-count}").Output()
	o.Expect(errGet).NotTo(o.HaveOccurred())
	checkCountInt, errInt := strconv.Atoi(checkCount)
	o.Expect(errInt).NotTo(o.HaveOccurred())
	o.Expect(ccrCount).Should(o.Equal(checkCountInt), fmt.Sprintf("The actual ccr count %d NOT equals to checkcount in the annotation %d", ccrCount, checkCountInt))

}

func waitCoBecomes(oc *exutil.CLI, coName string, waitTime int, expectedStatus map[string]string) error {
	errCo := wait.Poll(20*time.Second, time.Duration(waitTime)*time.Second, func() (bool, error) {
		gottenStatus := getCoStatus(oc, coName, expectedStatus)
		eq := reflect.DeepEqual(expectedStatus, gottenStatus)
		if eq {
			eq := reflect.DeepEqual(expectedStatus, map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"})
			if eq {
				// For True False False, we want to wait some bit more time and double check, to ensure it is stably healthy
				time.Sleep(100 * time.Second)
				gottenStatus := getCoStatus(oc, coName, expectedStatus)
				eq := reflect.DeepEqual(expectedStatus, gottenStatus)
				if eq {
					e2e.Logf("Given operator %s becomes available/non-progressing/non-degraded", coName)
					return true, nil
				}
			} else {
				e2e.Logf("Given operator %s becomes %s", coName, gottenStatus)
				return true, nil
			}
		}
		return false, nil
	})
	if errCo != nil {
		err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	}
	return errCo
}

func getCoStatus(oc *exutil.CLI, coName string, statusToCompare map[string]string) map[string]string {
	newStatusToCompare := make(map[string]string)
	for key := range statusToCompare {
		args := fmt.Sprintf(`-o=jsonpath={.status.conditions[?(.type == '%s')].status}`, key)
		status, _ := getResource(oc, asAdmin, withoutNamespace, "co", coName, args)
		newStatusToCompare[key] = status
	}
	return newStatusToCompare
}

// Get something existing resource
func getResource(oc *exutil.CLI, asAdmin bool, withoutNamespace bool, parameters ...string) (string, error) {
	return doAction(oc, "get", asAdmin, withoutNamespace, parameters...)
}

func create_machineconfig(oc *exutil.CLI, ns string, mcName string, id int) {
	buildPruningBaseDir := exutil.FixturePath("testdata", "securityandcompliance")
	mcTemplate := filepath.Join(buildPruningBaseDir, "machine-config.yaml")
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", mcTemplate, "-n", ns, "-p", "NAME="+mcName, "ID="+strconv.Itoa(id))
	o.Expect(err).NotTo(o.HaveOccurred())
}

func delete_machineconfig(oc *exutil.CLI, nsName string, mcName string) {
	cleanupObjects(oc, objectTableRef{"machineconfig", nsName, mcName})
}

func getResouceCnt(oc *exutil.CLI, resource string, keyword string) int {
	cmd := fmt.Sprintf(`oc get %v -A | grep %v | wc -l | awk '$1=$1' | tr -d '\n'`, resource, keyword)
	cnt, errCmd := exec.Command("bash", "-c", cmd).Output()
	o.Expect(errCmd).NotTo(o.HaveOccurred())
	nsCntint, _ := strconv.Atoi(string(cnt))
	return nsCntint
}

// clusterOperatorHealthcheck check abnormal operators
func clusterOperatorHealthcheck(oc *exutil.CLI, waitTime int) {
	var (
		coLogs []byte
		errCmd error
	)
	e2e.Logf("Check the abnormal operators")
	errCo := wait.Poll(30*time.Second, time.Duration(waitTime)*time.Second, func() (bool, error) {
		coLogFile, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co", "--no-headers").OutputToFile(getRandomString() + "-costatus.txt")
		o.Expect(err).NotTo(o.HaveOccurred())
		if err == nil {
			cmd := fmt.Sprintf(`cat %v | grep -v '.True.*False.*False' || true`, coLogFile)
			coLogs, errCmd = exec.Command("bash", "-c", cmd).Output()
			o.Expect(errCmd).NotTo(o.HaveOccurred())
			if len(coLogs) > 0 {
				return false, nil
			}
			return true, nil
		} else {
			return false, nil
		}
	})
	if errCo != nil {
		e2e.Logf("coLogs: %s", coLogs)
	}
	exutil.AssertWaitPollNoErr(errCmd, fmt.Sprintf("Failed to cluster health check::Abnormal cluster operators found: %s", coLogs))
}

func checkFailedRulesForTwoProfiles(oc *exutil.CLI, ns string, ssbName string, ssbNameWithSuffix string, versionSuffix string) {
	labelCISLatest := fmt.Sprintf("compliance.openshift.io/suite=%s,compliance.openshift.io/check-status=FAIL", ssbName)
	labelCISSuffix := fmt.Sprintf("compliance.openshift.io/suite=%s,compliance.openshift.io/check-status=FAIL", ssbNameWithSuffix)
	failedCcrLatest, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("compliancecheckresult", "-n", ns, "-l", labelCISLatest,
		"-o=jsonpath={.items[*].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	failedCcrsSuffix, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("compliancecheckresult", "-n", ns, "-l", labelCISSuffix,
		"-o=jsonpath={.items[*].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	failedCcrLatestNew := strings.Fields(failedCcrLatest)
	newFailedCcrsSuffixNew := strings.Fields(strings.ReplaceAll(failedCcrsSuffix, versionSuffix, ""))
	sort.Strings(failedCcrLatestNew)
	sort.Strings(newFailedCcrsSuffixNew)
	o.Expect(reflect.DeepEqual(newFailedCcrsSuffixNew, failedCcrLatestNew)).Should(o.BeTrue())
}

func getWorkloadLimitNamespacesExempt(oc *exutil.CLI, workloadKind string) string {
	var varNs string
	res, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(workloadKind, "--all-namespaces", "-o", "json").OutputToFile(getRandomString() + "project.json")
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The file name  is %s", res)
	jqCmd := fmt.Sprintf(`cat %s | jq '[ .items[] | select(.metadata.namespace | startswith("kube-") or startswith("openshift-") | not)  | 
		select(.metadata.namespace != "rhacs-operator" and (true)) | select( .spec.template.spec.containers[].resources.requests.cpu == null  
		or  .spec.template.spec.containers[].resources.requests.memory == null or .spec.template.spec.containers[].resources.limits.cpu == null 
		or  .spec.template.spec.containers[].resources.limits.memory == null )  | .metadata.namespace ]' | tr -d [ | tr -d ] | tr -d ',' | tr -d '\"'`, res)
	result, errCmd := exec.Command("bash", "-c", jqCmd).Output()
	o.Expect(errCmd).NotTo(o.HaveOccurred())
	resString := string(result)
	projectList := strings.Fields(resString)
	length := len(projectList)
	for i, ns := range projectList {
		e2e.Logf("varNs is %s", varNs)
		e2e.Logf("ns is %s", ns)
		if i == length-1 {
			varNs = varNs + string(ns)
		} else {
			varNs = varNs + string(ns) + "|"
		}
	}
	return varNs
}

func verifyFilesUnderMustgather(mustgatherDir string, fails int) int {
	mustgatherChecks := counts{
		pods:                0,
		varibles:            0,
		services:            0,
		sa:                  0,
		scansettings:        0,
		rules:               0,
		routes:              0,
		roles:               0,
		rolebindings:        0,
		pvc:                 0,
		pv:                  0,
		profiles:            0,
		profilebundles:      0,
		leases:              0,
		events:              0,
		clusterroles:        0,
		clusterrolebindings: 0,
		configmaps:          0,
	}
	mustgatherExpected := counts{
		pods:                2,
		varibles:            2,
		services:            2,
		sa:                  2,
		scansettings:        2,
		rules:               2,
		routes:              2,
		roles:               2,
		rolebindings:        2,
		pvc:                 2,
		pv:                  2,
		profiles:            2,
		profilebundles:      2,
		leases:              2,
		events:              2,
		clusterroles:        114,
		clusterrolebindings: 6,
		configmaps:          2,
	}

	err := filepath.Walk(mustgatherDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			e2e.Logf("Error on %v: %v", path, err)
			return err
		}

		if !info.IsDir() {
			if strings.Contains(path, "openshift-compliance-must-gather") {
				if strings.Contains(path, "/openshift-compliance/") {
					if strings.Contains(path, "/openshift-compliance/pods/") {
						if strings.Contains(path, "/openshift-compliance/pods/describe.log") || strings.Contains(path, "/openshift-compliance/pods/get.yaml") {
							mustgatherChecks.pods++
						}
					}
					if strings.Contains(path, "/openshift-compliance/variables.compliance.openshift.io/") {
						if strings.Contains(path, "/openshift-compliance/variables.compliance.openshift.io/describe.log") ||
							strings.Contains(path, "/openshift-compliance/variables.compliance.openshift.io/get.yaml") {
							mustgatherChecks.varibles++
						}
					}
					if strings.Contains(path, "/openshift-compliance/services/") {
						if strings.Contains(path, "/openshift-compliance/services/describe.log") || strings.Contains(path, "/openshift-compliance/services/get.yaml") {
							mustgatherChecks.services++
						}
					}
					if strings.Contains(path, "/openshift-compliance/serviceaccounts/") {
						if strings.Contains(path, "/openshift-compliance/serviceaccounts/describe.log") || strings.Contains(path, "/openshift-compliance/serviceaccounts/get.yaml") {
							mustgatherChecks.sa++
						}
					}
					if strings.Contains(path, "/openshift-compliance/scansettings.compliance.openshift.io/") {
						if strings.Contains(path, "/openshift-compliance/scansettings.compliance.openshift.io/describe.log") ||
							strings.Contains(path, "/openshift-compliance/scansettings.compliance.openshift.io/get.yaml") {
							mustgatherChecks.scansettings++
						}
					}
					if strings.Contains(path, "/openshift-compliance/rules.compliance.openshift.io/") {
						if strings.Contains(path, "/openshift-compliance/rules.compliance.openshift.io/describe.log") || strings.Contains(path, "/openshift-compliance/rules.compliance.openshift.io/get.yaml") {
							mustgatherChecks.rules++
						}
					}
					if strings.Contains(path, "/openshift-compliance/routes/") {
						if strings.Contains(path, "/openshift-compliance/routes/describe.log") || strings.Contains(path, "/openshift-compliance/routes/get.yaml") {
							mustgatherChecks.routes++
						}
					}
					if strings.Contains(path, "/openshift-compliance/roles/") {
						if strings.Contains(path, "/openshift-compliance/roles/describe.log") || strings.Contains(path, "/openshift-compliance/roles/get.yaml") {
							mustgatherChecks.roles++
						}
					}
					if strings.Contains(path, "/openshift-compliance/rolebindings/") {
						if strings.Contains(path, "/openshift-compliance/rolebindings/describe.log") || strings.Contains(path, "/openshift-compliance/rolebindings/get.yaml") {
							mustgatherChecks.rolebindings++
						}
					}
					if strings.Contains(path, "/openshift-compliance/pvc/") {
						if strings.Contains(path, "/openshift-compliance/pvc/describe.log") || strings.Contains(path, "/openshift-compliance/pvc/get.yaml") {
							mustgatherChecks.pvc++
						}
					}
					if strings.Contains(path, "/openshift-compliance/pv/") {
						if strings.Contains(path, "/openshift-compliance/pv/describe.log") || strings.Contains(path, "/openshift-compliance/pv/get.yaml") {
							mustgatherChecks.pv++
						}
					}
					if strings.Contains(path, "/openshift-compliance/profiles.compliance.openshift.io/") {
						if strings.Contains(path, "/openshift-compliance/profiles.compliance.openshift.io/describe.log") ||
							strings.Contains(path, "/openshift-compliance/profiles.compliance.openshift.io/get.yaml") {
							mustgatherChecks.profiles++
						}
					}
					if strings.Contains(path, "/openshift-compliance/profilebundles.compliance.openshift.io/") {
						if strings.Contains(path, "/openshift-compliance/profilebundles.compliance.openshift.io/describe.log") ||
							strings.Contains(path, "/openshift-compliance/profilebundles.compliance.openshift.io/get.yaml") {
							mustgatherChecks.profilebundles++
						}
					}
					if strings.Contains(path, "/openshift-compliance/leases/") {
						if strings.Contains(path, "/openshift-compliance/leases/describe.log") || strings.Contains(path, "/openshift-compliance/leases/get.yaml") {
							mustgatherChecks.leases++
						}
					}
					if strings.Contains(path, "/openshift-compliance/events/") {
						if strings.Contains(path, "/openshift-compliance/events/describe.log") || strings.Contains(path, "/openshift-compliance/events/get.yaml") {
							mustgatherChecks.events++
						}
					}
					if strings.Contains(path, "/openshift-compliance/configmaps/") {
						if strings.Contains(path, "/openshift-compliance/configmaps/describe.log") || strings.Contains(path, "/openshift-compliance/configmaps/get.yaml") {
							mustgatherChecks.configmaps++
						}
					}
					if strings.Contains(path, "/openshift-compliance/clusterroles/") {
						describeLog, _ := regexp.MatchString(".*describe.log", path)
						yamlFile, _ := regexp.MatchString(".*.yaml", path)
						if describeLog || yamlFile {
							mustgatherChecks.clusterroles++
						}
					}
					if strings.Contains(path, "/openshift-compliance/clusterrolebindings/") {
						describeLog, _ := regexp.MatchString(".*describe.log", path)
						yamlFile, _ := regexp.MatchString(".*.yaml", path)
						if describeLog || yamlFile {
							mustgatherChecks.clusterrolebindings++
						}
					}
				}
			}

		}
		return nil
	})
	e2e.Logf("Error: %s", err)
	e2e.Logf("mustgatherChecks.pods : %v", mustgatherChecks.pods)
	if mustgatherChecks.pods < mustgatherExpected.pods {
		e2e.Logf("(%v) Logs not found in /openshift-compliance/pods directory. Expected number of logs (%v)", mustgatherChecks.pods, mustgatherExpected.pods)
		fails++
	}

	e2e.Logf("mustgatherChecks.varibles : %v", mustgatherChecks.varibles)
	if mustgatherChecks.varibles < mustgatherExpected.varibles {
		e2e.Logf("(%v) Logs not found in /openshift-compliance/varibles directory. Expected number of logs (%v)", mustgatherChecks.varibles, mustgatherExpected.varibles)
		fails++
	}

	e2e.Logf("mustgatherChecks.services : %v", mustgatherChecks.services)
	if mustgatherChecks.services < mustgatherExpected.services {
		e2e.Logf("(%v) Logs not found in /openshift-compliance/services directory. Expected number of logs (%v)", mustgatherChecks.services, mustgatherExpected.services)
		fails++
	}

	e2e.Logf("mustgatherChecks.sa : %v", mustgatherChecks.sa)
	if mustgatherChecks.sa < mustgatherExpected.sa {
		e2e.Logf("(%v) Logs not found in /openshift-compliance/sa directory. Expected number of logs (%v)", mustgatherChecks.sa, mustgatherExpected.sa)
		fails++
	}

	e2e.Logf("mustgatherChecks.scansettings : %v", mustgatherChecks.scansettings)
	if mustgatherChecks.scansettings < mustgatherExpected.scansettings {
		e2e.Logf("(%v) Logs not found in /openshift-compliance/scansettings directory. Expected number of logs (%v)", mustgatherChecks.scansettings, mustgatherExpected.scansettings)
		fails++
	}

	e2e.Logf("mustgatherChecks.rules : %v", mustgatherChecks.rules)
	if mustgatherChecks.rules < mustgatherExpected.rules {
		e2e.Logf("(%v) Logs not found in /openshift-compliance/rules directory. Expected number of logs (%v)", mustgatherChecks.rules, mustgatherExpected.rules)
		fails++
	}

	e2e.Logf("mustgatherChecks.routes : %v", mustgatherChecks.routes)
	if mustgatherChecks.routes < mustgatherExpected.routes {
		e2e.Logf("(%v) Logs not found in /openshift-compliance/routes directory. Expected number of logs (%v)", mustgatherChecks.routes, mustgatherExpected.routes)
		fails++
	}
	e2e.Logf("mustgatherChecks.roles : %v", mustgatherChecks.roles)
	if mustgatherChecks.roles < mustgatherExpected.roles {
		e2e.Logf("(%v) Logs not found in /openshift-compliance/roles directory. Expected number of logs (%v)", mustgatherChecks.roles, mustgatherExpected.roles)
		fails++
	}

	e2e.Logf("mustgatherChecks.rolebindings : %v", mustgatherChecks.rolebindings)
	if mustgatherChecks.rolebindings < mustgatherExpected.rolebindings {
		e2e.Logf("(%v) Logs not found in /openshift-compliance/rolebindings directory. Expected number of logs (%v)", mustgatherChecks.rolebindings, mustgatherExpected.rolebindings)
		fails++
	}

	e2e.Logf("mustgatherChecks.pvc : %v", mustgatherChecks.pvc)
	if mustgatherChecks.pvc < mustgatherExpected.pvc {
		e2e.Logf("(%v) Logs not found in /openshift-compliance/pvc directory. Expected number of logs (%v)", mustgatherChecks.pvc, mustgatherExpected.pvc)
		fails++
	}

	e2e.Logf("mustgatherChecks.pv : %v", mustgatherChecks.pv)
	if mustgatherChecks.pv < mustgatherExpected.pv {
		e2e.Logf("(%v) Logs not found in /openshift-compliance/pv directory. Expected number of logs (%v)", mustgatherChecks.pv, mustgatherExpected.pv)
		fails++
	}

	e2e.Logf("mustgatherChecks.profiles : %v", mustgatherChecks.profiles)
	if mustgatherChecks.profiles < mustgatherExpected.profiles {
		e2e.Logf("(%v) Logs not found in /openshift-compliance/profiles directory. Expected number of logs (%v)", mustgatherChecks.profiles, mustgatherExpected.profiles)
		fails++
	}

	e2e.Logf("mustgatherChecks.profilebundles : %v", mustgatherChecks.profilebundles)
	if mustgatherChecks.profilebundles < mustgatherExpected.profilebundles {
		e2e.Logf("(%v) Logs not found in /openshift-compliance/profilebundles directory. Expected number of logs (%v)", mustgatherChecks.profilebundles, mustgatherExpected.profilebundles)
		fails++
	}

	e2e.Logf("mustgatherChecks.leases : %v", mustgatherChecks.leases)
	if mustgatherChecks.leases < mustgatherExpected.leases {
		e2e.Logf("(%v) Logs not found in /openshift-compliance/leases directory. Expected number of logs (%v)", mustgatherChecks.leases, mustgatherExpected.leases)
		fails++
	}

	e2e.Logf("mustgatherChecks.events : %v", mustgatherChecks.events)
	if mustgatherChecks.events < mustgatherExpected.events {
		e2e.Logf("(%v) Logs not found in /openshift-compliance/events directory. Expected number of logs (%v)", mustgatherChecks.events, mustgatherExpected.events)
		fails++
	}

	e2e.Logf("mustgatherChecks.clusterroles : %v %v", mustgatherChecks.clusterroles, mustgatherExpected.clusterroles)
	if mustgatherChecks.clusterroles < mustgatherExpected.clusterroles {
		e2e.Logf("(%v) Logs not found in /openshift-compliance/clusterroles directory. Expected number of logs (%v)", mustgatherChecks.clusterroles, mustgatherExpected.clusterroles)
		fails++
	}

	e2e.Logf("mustgatherChecks.clusterrolebindings : %v", mustgatherChecks.clusterrolebindings)
	if mustgatherChecks.clusterrolebindings < mustgatherExpected.clusterrolebindings {
		e2e.Logf("(%v) Logs not found in /openshift-compliance/clusterrolebindings directory. Expected number of logs (%v)", mustgatherChecks.clusterrolebindings, mustgatherExpected.clusterrolebindings)
		fails++
	}

	e2e.Logf("mustgatherChecks.configmaps : %v", mustgatherChecks.configmaps)
	if mustgatherChecks.configmaps < mustgatherExpected.configmaps {
		e2e.Logf("(%v) Logs not found in /openshift-compliance/configmaps directory. Expected number of logs (%v)", mustgatherChecks.configmaps, mustgatherExpected.configmaps)
		fails++
	}
	return fails
}

func uninstallComplianceOperator(oc *exutil.CLI, namespace string) {
	g.By("Delete compliance operator !!!")
	oc.AsAdmin().WithoutNamespace().Run("delete").Args("ssb", "--all", "-n", namespace, "--ignore-not-found").Execute()
	oc.AsAdmin().WithoutNamespace().Run("delete").Args("suite", "--all", "-n", namespace, "--ignore-not-found").Execute()
	oc.AsAdmin().WithoutNamespace().Run("delete").Args("scan", "--all", "-n", namespace, "--ignore-not-found").Execute()
	oc.AsAdmin().WithoutNamespace().Run("delete").Args("pb", "--all", "-n", namespace, "--ignore-not-found").Execute()
	oc.AsAdmin().WithoutNamespace().Run("delete").Args("ns", namespace, "-n", namespace, "--ignore-not-found").Execute()
}
