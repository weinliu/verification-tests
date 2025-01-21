package securityandcompliance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"github.com/openshift/openshift-tests-private/test/extended/util/architecture"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type operatorGroupDescription struct {
	name         string
	namespace    string
	multinslabel string
	template     string
}

// PackageManifest gets the status filed of a packagemanifest
type PackageManifest struct {
	metav1.ObjectMeta `json:"metadata"`
	Status            struct {
		CatalogSource          string `json:"catalogSource"`
		CatalogSourceNamespace string `json:"catalogSourceNamespace"`
		Channels               []struct {
			CurrentCSV string `json:"currentCSV"`
			Name       string `json:"name"`
		} `json:"channels"`
		DefaultChannel string `json:"defaultChannel"`
	} `json:"status"`
}

type resourceDescription struct {
	oc               *exutil.CLI
	asAdmin          bool
	withoutNamespace bool
	kind             string
	name             string
	requireNS        bool
	namespace        string
}

type subscriptionDescription struct {
	subName                string
	namespace              string
	channel                string
	ipApproval             string
	operatorPackage        string
	catalogSourceName      string
	catalogSourceNamespace string
	startingCSV            string
	currentCSV             string
	installedCSV           string
	platform               string
	template               string
	singleNamespace        bool
	ipCsv                  string
}

const (
	asAdmin          = true
	withoutNamespace = true
	requireNS        = true
	compare          = true
	contain          = false
	present          = true
	notPresent       = false
	ok               = true
	nok              = false
)

func getNodeNumberPerLabel(oc *exutil.CLI, label string) int {
	nodeNameString, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", "-l", label, "-o=jsonpath={.items[*].metadata.name}", "--ignore-not-found").Output()
	nodeNumber := len(strings.Fields(nodeNameString))
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("the result of nodeNumber:%v", nodeNumber)
	return nodeNumber
}

func (sub *subscriptionDescription) create(oc *exutil.CLI, itName string, dr describerResrouce) {
	sub.createWithoutCheck(oc, itName, dr)
	if strings.Compare(sub.ipApproval, "Automatic") == 0 {
		sub.findInstalledCSV(oc, itName, dr)
	} else {
		newCheck("expect", asAdmin, withoutNamespace, compare, "UpgradePending", ok, []string{"sub", sub.subName, "-n", sub.namespace, "-o=jsonpath={.status.state}"}).check(oc)
	}
}

func (sub *subscriptionDescription) createWithoutCheck(oc *exutil.CLI, itName string, dr describerResrouce) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", sub.template, "-p", "SUBNAME="+sub.subName, "SUBNAMESPACE="+sub.namespace, "CHANNEL="+sub.channel,
		"APPROVAL="+sub.ipApproval, "OPERATORNAME="+sub.operatorPackage, "SOURCENAME="+sub.catalogSourceName, "SOURCENAMESPACE="+sub.catalogSourceNamespace, "STARTINGCSV="+sub.startingCSV)

	o.Expect(err).NotTo(o.HaveOccurred())
	dr.getIr(itName).add(newResource(oc, "sub", sub.subName, requireNS, sub.namespace))
}

func (sub *subscriptionDescription) findInstalledCSV(oc *exutil.CLI, itName string, dr describerResrouce) {
	newCheck("expect", asAdmin, withoutNamespace, compare, "AtLatestKnown", ok, []string{"sub", sub.subName, "-n", sub.namespace, "-o=jsonpath={.status.state}"}).check(oc)
	installedCSV, err := getResource(oc, asAdmin, withoutNamespace, "sub", sub.subName, "-n", sub.namespace, "-o=jsonpath={.status.installedCSV}")
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(installedCSV).NotTo(o.BeEmpty())
	if strings.Compare(sub.installedCSV, installedCSV) != 0 {
		sub.installedCSV = installedCSV
		dr.getIr(itName).add(newResource(oc, "csv", sub.installedCSV, requireNS, sub.namespace))
	}
	e2e.Logf("the installed CSV name is %s", sub.installedCSV)
}

func (sub *subscriptionDescription) expectCSV(oc *exutil.CLI, itName string, dr describerResrouce, cv string) {
	err := wait.Poll(3*time.Second, 180*time.Second, func() (bool, error) {
		sub.findInstalledCSV(oc, itName, dr)
		if strings.Compare(sub.installedCSV, cv) == 0 {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("the csv %s is not expected", cv))
}

func expectedResource(oc *exutil.CLI, asAdmin bool, withoutNamespace bool, isCompare bool, content string, expect bool, parameters ...string) error {
	cc := func(a, b string, ic bool) bool {
		bs := strings.Split(b, "+2+")
		ret := false
		for _, s := range bs {
			if (ic && strings.Compare(a, s) == 0) || (!ic && strings.Contains(a, s)) {
				ret = true
			}
		}
		return ret
	}
	return wait.Poll(4*time.Second, 240*time.Second, func() (bool, error) {
		output, err := doAction(oc, "get", asAdmin, withoutNamespace, parameters...)
		if err != nil {
			e2e.Logf("the get error is %v, and try next", err)
			return false, nil
		}
		e2e.Logf("the queried resource:%s", output)
		if isCompare && expect && cc(output, content, isCompare) {
			e2e.Logf("the output %s matches one of the content %s, expected", output, content)
			return true, nil
		}
		if isCompare && !expect && !cc(output, content, isCompare) {
			e2e.Logf("the output %s does not matche the content %s, expected", output, content)
			return true, nil
		}
		if !isCompare && expect && cc(output, content, isCompare) {
			e2e.Logf("the output %s contains one of the content %s, expected", output, content)
			return true, nil
		}
		if !isCompare && !expect && !cc(output, content, isCompare) {
			e2e.Logf("the output %s does not contain the content %s, expected", output, content)
			return true, nil
		}
		return false, nil
	})
}

func isPresentResource(oc *exutil.CLI, asAdmin bool, withoutNamespace bool, present bool, parameters ...string) bool {
	parameters = append(parameters, "--ignore-not-found")
	err := wait.Poll(3*time.Second, 60*time.Second, func() (bool, error) {
		output, err := doAction(oc, "get", asAdmin, withoutNamespace, parameters...)
		if err != nil {
			e2e.Logf("the get error is %v, and try next", err)
			return false, nil
		}
		if !present && strings.Compare(output, "") == 0 {
			return true, nil
		}
		if present && strings.Compare(output, "") != 0 {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		return false
	}
	return true
}

func getResourceToBeReady(oc *exutil.CLI, asAdmin bool, withoutNamespace bool, parameters ...string) string {
	var result string
	err := wait.Poll(3*time.Second, 120*time.Second, func() (bool, error) {
		output, err := doAction(oc, "get", asAdmin, withoutNamespace, parameters...)
		if err != nil {
			e2e.Logf("the get error is %v, and try next", err)
			return false, nil
		}
		result = output
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to get %v", parameters))
	e2e.Logf("the result of queried resource:%v", result)
	return result
}

type checkDescription struct {
	method          string
	executor        bool
	inlineNamespace bool
	expectAction    bool
	expectContent   string
	expect          bool
	resource        []string
}

func newCheck(method string, executor bool, inlineNamespace bool, expectAction bool,
	expectContent string, expect bool, resource []string) checkDescription {
	return checkDescription{
		method:          method,
		executor:        executor,
		inlineNamespace: inlineNamespace,
		expectAction:    expectAction,
		expectContent:   expectContent,
		expect:          expect,
		resource:        resource,
	}
}

func (ck checkDescription) check(oc *exutil.CLI) {
	switch ck.method {
	case "present":
		ok := isPresentResource(oc, ck.executor, ck.inlineNamespace, ck.expectAction, ck.resource...)
		o.Expect(ok).To(o.BeTrue())
	case "expect":
		err := expectedResource(oc, ck.executor, ck.inlineNamespace, ck.expectAction, ck.expectContent, ck.expect, ck.resource...)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("content %s not got by %v", ck.expectContent, ck.resource))
	default:
		err := fmt.Errorf("unknown method")
		o.Expect(err).NotTo(o.HaveOccurred())
	}
}

func (ck checkDescription) checkWithoutAssert(oc *exutil.CLI) error {
	switch ck.method {
	case "present":
		ok := isPresentResource(oc, ck.executor, ck.inlineNamespace, ck.expectAction, ck.resource...)
		if ok {
			return nil
		}
		return fmt.Errorf("it is not epxected")
	case "expect":
		return expectedResource(oc, ck.executor, ck.inlineNamespace, ck.expectAction, ck.expectContent, ck.expect, ck.resource...)
	default:
		return fmt.Errorf("unknown method")
	}
}

func (og *operatorGroupDescription) checkOperatorgroup(oc *exutil.CLI, expected string) {
	err := wait.Poll(5*time.Second, 60*time.Second, func() (bool, error) {
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", og.namespace, "operatorgroup", og.name).Output()
		e2e.Logf("the result of checkOperatorgroup:%v", output)
		if strings.Contains(output, expected) {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("og %s not found", og.name))
}

type itResource map[string]resourceDescription

func (ir itResource) add(r resourceDescription) {
	ir[r.name+r.kind+r.namespace] = r
}
func (ir itResource) get(name string, kind string, namespace string) resourceDescription {
	r, ok := ir[name+kind+namespace]
	o.Expect(ok).To(o.BeTrue())
	return r
}
func (ir itResource) remove(name string, kind string, namespace string) {
	rKey := name + kind + namespace
	if r, ok := ir[rKey]; ok {
		r.delete()
		delete(ir, rKey)
	}
}
func (ir itResource) cleanup() {
	for _, r := range ir {
		e2e.Logf("cleanup resource %s,   %s", r.kind, r.name)
		ir.remove(r.name, r.kind, r.namespace)
	}
}

type describerResrouce map[string]itResource

func (dr describerResrouce) addIr(itName string) {
	dr[itName] = itResource{}
}
func (dr describerResrouce) getIr(itName string) itResource {
	ir, ok := dr[itName]
	o.Expect(ok).To(o.BeTrue())
	return ir
}
func (dr describerResrouce) rmIr(itName string) {
	delete(dr, itName)
}

func newResource(oc *exutil.CLI, kind string, name string, nsflag bool, namespace string) resourceDescription {
	return resourceDescription{
		oc:               oc,
		asAdmin:          asAdmin,
		withoutNamespace: withoutNamespace,
		kind:             kind,
		name:             name,
		requireNS:        nsflag,
		namespace:        namespace,
	}
}

func (r resourceDescription) delete() {
	if r.withoutNamespace && r.requireNS {
		removeResource(r.oc, r.asAdmin, r.withoutNamespace, r.kind, r.name, "-n", r.namespace)
	} else {
		removeResource(r.oc, r.asAdmin, r.withoutNamespace, r.kind, r.name)
	}
}

func applyResourceFromTemplate(oc *exutil.CLI, parameters ...string) error {
	var configFile string
	err := wait.Poll(3*time.Second, 15*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().Run("process").Args(parameters...).OutputToFile(getRandomString() + "isc-config.json")
		if err != nil {
			e2e.Logf("the err:%v, and try next round", err)
			return false, nil
		}
		configFile = output
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to process %v", parameters))

	e2e.Logf("the file of resource is %s", configFile)
	return oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", configFile).Execute()
}

func applyResourceFromTemplateWithoutKeyword(oc *exutil.CLI, keyword string, parameters ...string) error {
	var configFile string
	err := wait.Poll(3*time.Second, 15*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().Run("process").Args(parameters...).OutputToFile(getRandomString() + "isc-config.json")
		if err != nil {
			e2e.Logf("the err:%v, and try next round", err)
			return false, nil
		}
		configFile = output
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("without keyword, fail to process %v", parameters))
	e2e.Logf("the file of resource is %s", configFile)
	removeKeywordFromFile(configFile, keyword)
	return oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", configFile).Execute()
}

func removeKeywordFromFile(fileLocation string, keyword string) {
	var newlines []string
	input, err := ioutil.ReadFile(fileLocation)
	if err != nil {
		e2e.Failf("the result of ReadFile:%v", err)
	}

	lines := strings.Split(string(input), "\n")
	for i, line := range lines {
		if strings.Contains(line, keyword) {
			continue
		}
		newlines = append(newlines, lines[i])
	}
	output := strings.Join(newlines, "\n")
	err = ioutil.WriteFile(fileLocation, []byte(output), 0644)
	if err != nil {
		e2e.Failf("the result of WriteFile:%v", err)
	}
}

func removeResource(oc *exutil.CLI, asAdmin bool, withoutNamespace bool, parameters ...string) {
	output, err := doAction(oc, "delete", asAdmin, withoutNamespace, parameters...)
	if err != nil && (strings.Contains(output, "NotFound") || strings.Contains(output, "No resources found")) {
		e2e.Logf("the resource is deleted already")
		return
	}
	o.Expect(err).NotTo(o.HaveOccurred())

	err = wait.Poll(3*time.Second, 120*time.Second, func() (bool, error) {
		output, err := doAction(oc, "get", asAdmin, withoutNamespace, parameters...)
		if err != nil && (strings.Contains(output, "NotFound") || strings.Contains(output, "No resources found")) {
			e2e.Logf("the resource is delete successfully")
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to remove %v", parameters))
}

func doAction(oc *exutil.CLI, action string, asAdmin bool, withoutNamespace bool, parameters ...string) (string, error) {
	if asAdmin && withoutNamespace {
		return oc.AsAdmin().WithoutNamespace().Run(action).Args(parameters...).Output()
	}
	if asAdmin && !withoutNamespace {
		return oc.AsAdmin().Run(action).Args(parameters...).Output()
	}
	if !asAdmin && withoutNamespace {
		return oc.WithoutNamespace().Run(action).Args(parameters...).Output()
	}
	if !asAdmin && !withoutNamespace {
		return oc.Run(action).Args(parameters...).Output()
	}
	return "", nil
}

func getRandomString() string {
	chars := "abcdefghijklmnopqrstuvwxyz0123456789"
	seed := rand.New(rand.NewSource(time.Now().UnixNano()))
	buffer := make([]byte, 10)
	for index := range buffer {
		buffer[index] = chars[seed.Intn(len(chars))]
	}
	return string(buffer)
}

func (og *operatorGroupDescription) create(oc *exutil.CLI, itName string, dr describerResrouce) {
	var err error
	if strings.Compare(og.multinslabel, "") == 0 {
		err = applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", og.template, "-p", "NAME="+og.name, "NAMESPACE="+og.namespace)
	} else {
		err = applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", og.template, "-p", "NAME="+og.name, "NAMESPACE="+og.namespace,
			"MULTINSLABEL="+og.multinslabel)
	}
	o.Expect(err).NotTo(o.HaveOccurred())
	dr.getIr(itName).add(newResource(oc, "og", og.name, requireNS, og.namespace))
}

func patchResource(oc *exutil.CLI, asAdmin bool, withoutNamespace bool, parameters ...string) {
	_, err := doAction(oc, "patch", asAdmin, withoutNamespace, parameters...)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func taintNode(oc *exutil.CLI, parameters ...string) {
	_, err := doAction(oc, "adm", asAdmin, withoutNamespace, parameters...)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func labelTaintNode(oc *exutil.CLI, parameters ...string) {
	_, err := doAction(oc, "label", asAdmin, withoutNamespace, parameters...)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// skipNotelemetryFound means to skip test if telemetry not found
func skipNotelemetryFound(oc *exutil.CLI) {
	output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("prometheusrules", "telemetry", "-n", "openshift-monitoring").Output()
	if strings.Contains(output, `"telemetry" not found`) {
		e2e.Logf("output: %s", output)
		g.Skip("this env does not have telemetry prometheusrule, skip the case")
	}
}

func (sub *subscriptionDescription) skipMissingCatalogsources(oc *exutil.CLI, customCatalogsource string) {
	// Check for the custom catalogsource first
	output, errCustom := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-marketplace", "catalogsource", customCatalogsource).Output()
	if errCustom != nil && strings.Contains(output, "NotFound") {
		// If custom catalogsource is missing, check for "qe-app-registry"
		output, errQeReg := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-marketplace", "catalogsource", "qe-app-registry").Output()
		if errQeReg != nil && strings.Contains(output, "NotFound") {
			// If both custom and "qe-app-registry" are missing, check for "redhat-operators"
			output, errRed := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-marketplace", "catalogsource", "redhat-operators").Output()
			if errRed != nil && strings.Contains(output, "NotFound") {
				// Skip the test if all catalog sources are missing
				g.Skip("Skip since catalogsources not available")
			} else {
				o.Expect(errRed).NotTo(o.HaveOccurred())
				sub.catalogSourceName = "redhat-operators"
			}
		} else {
			o.Expect(errQeReg).NotTo(o.HaveOccurred())
			sub.catalogSourceName = "qe-app-registry"
		}
	} else {
		o.Expect(errCustom).NotTo(o.HaveOccurred())
		sub.catalogSourceName = customCatalogsource
	}
}

// skipMissingDefaultSC mean to skip test when default storageclass is not available or not applicable
func skipMissingOrNotApplicableDefaultSC(oc *exutil.CLI) {
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("sc", "-o=jsonpath={.items[?(@.metadata.annotations.storageclass\\.kubernetes\\.io/is-default-class==\"true\")].metadata.name}", "-n", oc.Namespace()).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("This default sc is: %s", output)

	if output == "" {
		g.Skip("Skipping test: No default storageclass is available in the cluster")
	}

	// The 'Hyperdisk' type has a minimum disk size requirement of 4G which is expensive and unnecessary for our scenarios.
	// xref: https://cloud.google.com/compute/docs/disks/hyperdisks#limits-disk
	if strings.Contains(strings.ToLower(output), "hyperdisk") {
		g.Skip("Skipping test: Default storageclass 'Hyperdisk' is not applicable")
	}

	// Handle multiple default storage classes
	scNames := strings.Split(output, " ")
	if len(scNames) > 1 {
		g.Skip(fmt.Sprintf("Skipping test: Multiple default storageclasses detected: %v", scNames))
	}
}

// SkipNonRosaHcpCluster means to skip test when test cluster is not ROSA HCP
func SkipNonRosaHcpCluster(oc *exutil.CLI) {
	if !exutil.IsRosaCluster(oc) || !exutil.IsHypershiftHostedCluster(oc) {
		g.Skip("Skip since it is NOT a ROSA HCP cluster")
	}
}

// SkipMissingRhcosWorkers mean to skip test for env without rhcos workers. The Compliance Operator is available for rhcos deployments only.
func SkipMissingRhcosWorkers(oc *exutil.CLI) {
	rhcosWorkerNodesNumber := getNodeNumberPerLabel(oc, "node.openshift.io/os_id=rhcos,node-role.kubernetes.io/worker=")
	if rhcosWorkerNodesNumber == 0 {
		g.Skip("Skip since no rhcos workers is not available")
	}
}

// SkipForIBMCloud mean to skip test for IBMCloud as related test case not applicalbe
func SkipForIBMCloud(oc *exutil.CLI) {
	platform, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-n", oc.Namespace(), "-o=jsonpath={.status.platform}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if strings.Compare(platform, "IBMCloud") == 0 {
		g.Skip("Skip since it is IBMCloud")
	}
}

// SkipNonHypershiftGuestClusters mean to skip test for non hypershift hosted cluster
func SkipNonHypershiftHostedClusters(oc *exutil.CLI) {
	if !exutil.IsHypershiftHostedCluster(oc) {
		g.Skip("Skip since it is non hypershift hosted cluster")
	}
}

// SkipClustersWithRhelNodes mean to skip test for clusters with rhel nodes
func SkipClustersWithRhelNodes(oc *exutil.CLI) {
	rhelWorkers, err := exutil.GetAllWorkerNodesByOSID(oc, "rhel")
	o.Expect(err).NotTo(o.HaveOccurred())
	if len(rhelWorkers) > 0 {
		g.Skip("Skip for clusters with rhel nodes")
	}
}

func assertKeywordsExistsInFile(oc *exutil.CLI, ns string, keywords string, filePath string, flag bool) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		nodeName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("securityprofilenodestatuses", "-n", ns, "-o=jsonpath={.items[0].nodeName}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		standOut, _, _ := exutil.DebugNodeWithOptionsAndChrootWithoutRecoverNsLabel(oc, nodeName, []string{"-q"}, "ls", "-ltr", filePath)
		content, _, _ := exutil.DebugNodeWithOptionsAndChrootWithoutRecoverNsLabel(oc, nodeName, []string{"-q"}, "cat", filePath)
		filePathMatched, _ := regexp.MatchString(filePath, string(standOut))
		contentMatched, _ := regexp.MatchString(keywords, string(content))

		if flag == true {
			if filePathMatched && contentMatched {
				return true, nil
			}
		}
		if flag == false {
			if filePathMatched && !contentMatched {
				return true, nil
			}
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("File %s not exists on node! \n", filePath))
}

func createOperator(oc *exutil.CLI, subD subscriptionDescription, ogD operatorGroupDescription) {
	g.By("Create namespace !!!")
	msg, err := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", subD.namespace).Output()
	e2e.Logf("err %v, msg %v", err, msg)
	msg, err = oc.AsAdmin().WithoutNamespace().Run("label").Args("namespace", subD.namespace, "openshift.io/cluster-monitoring=true", "--overwrite").Output()
	e2e.Logf("err %v, msg %v", err, msg)

	g.By("Create operatorGroup !!!")
	ogFile, err := oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", ogD.template, "-p", "NAME="+ogD.name, "NAMESPACE="+ogD.namespace, "-n", ogD.namespace).OutputToFile(getRandomString() + "og.json")
	e2e.Logf("Created the operator-group yaml %s, %v", ogFile, err)
	msg, err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", ogFile).Output()
	e2e.Logf("err %v, msg %v", err, msg)

	g.By("Create subscription for above catalogsource !!!")
	createSubscription(oc, subD)
	msg, err = subscriptionIsFinished(oc, subD)
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("err %v, msg %v", err, msg)
}

// Get a random number of int64 type [m,n], n > m
func getRandomNum(m int64, n int64) int64 {
	rand.Seed(time.Now().UnixNano())
	return rand.Int63n(n-m+1) + m
}

// create subscription
func createSubscription(oc *exutil.CLI, subD subscriptionDescription) {
	err := wait.Poll(5*time.Second, 15*time.Second, func() (bool, error) {
		output, errGet := oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", subD.subName, "-n", subD.namespace).Output()
		if errGet == nil {
			return true, nil
		}
		if errGet != nil && (strings.Contains(output, "NotFound")) {
			randomExpandInt64 := getRandomNum(3, 8)
			time.Sleep(time.Duration(randomExpandInt64) * time.Second)
			return false, nil
		}
		return false, nil
	})
	if err != nil {
		subFile, errApply := oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", subD.template, "-p", "SUBNAME="+subD.subName, "SUBNAMESPACE="+subD.namespace, "CHANNEL="+subD.channel, "APPROVAL="+subD.ipApproval,
			"OPERATORNAME="+subD.operatorPackage, "SOURCENAME="+subD.catalogSourceName, "SOURCENAMESPACE="+subD.catalogSourceNamespace, "STARTINGCSV="+subD.startingCSV, "PLATFORM="+subD.platform, "-n", subD.namespace).OutputToFile(getRandomString() + "sub.json")
		e2e.Logf("Created the subscription yaml %s, %v", subFile, err)
		o.Expect(errApply).NotTo(o.HaveOccurred())
		msg, errCreate := oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", subFile).Output()
		e2e.Logf("err %v, msg %v", errCreate, msg)
	}
}

func createFileIntegrityOperator(oc *exutil.CLI, subD subscriptionDescription, ogD operatorGroupDescription) {
	g.By("Check file integrity operator pod are in running state !!!")
	createOperator(oc, subD, ogD)

	g.By("Check file integrity operator pod are in running state !!!")
	subD.checkPodFioStatus(oc, "running")

	g.By("Check file integrity operator sucessfully installed !!! ")
}

func createComplianceOperator(oc *exutil.CLI, subD subscriptionDescription, ogD operatorGroupDescription) {
	g.By("Install compliance-operator !!!")
	createOperator(oc, subD, ogD)

	g.By("Check Compliance Operator is in running state !!!")
	newCheck("expect", asAdmin, withoutNamespace, compare, "Running", ok, []string{"pod", "--selector=name=compliance-operator", "-n",
		subD.namespace, "-o=jsonpath={.items[0].status.phase}"}).check(oc)
	newCheck("expect", asAdmin, withoutNamespace, compare, "VALID", ok, []string{"profilebundle", "ocp4", "-n", subD.namespace,
		"-ojsonpath={.status.dataStreamStatus}"}).check(oc)

	g.By("Check profilebundle status !!!")
	arch := architecture.ClusterArchitecture(oc)
	switch arch {
	case architecture.PPC64LE:
	case architecture.S390X:
	case architecture.AMD64:
		newCheck("expect", asAdmin, withoutNamespace, compare, "VALID", ok, []string{"profilebundle", "rhcos4", "-n", subD.namespace,
			"-ojsonpath={.status.dataStreamStatus}"}).check(oc)
	default:
		e2e.Logf("Architecture %s is not supported", arch.String())
	}

	g.By("Check compliance operator sucessfully installed !!! ")
}

// skipForSingleNodeCluster mean to skip test on Single Node Cluster
func skipForSingleNodeCluster(oc *exutil.CLI) {
	nodeMasterString, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", "-l", "node-role.kubernetes.io/master=", "-o=jsonpath={.items[*].metadata.name}").Output()
	nodeMasterWorkerString, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", "-l", "node-role.kubernetes.io/master=,node-role.kubernetes.io/worker=", "-o=jsonpath={.items[*].metadata.name}").Output()
	nodeMasterList := strings.Fields(nodeMasterString)
	nodeMasterCount := len(nodeMasterList)
	nodeMasterWorkerList := strings.Fields(nodeMasterWorkerString)
	nodeMasterWorkerCount := len(nodeMasterWorkerList)
	if nodeMasterCount == 1 && nodeMasterWorkerCount == 1 && !strings.Contains(nodeMasterWorkerList[0], "worker") {
		g.Skip("Skip since it is single node cluster")
	}
}

func getLinuxWorkerCount(oc *exutil.CLI) int {
	workerNodeDetails, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "--selector=kubernetes.io/os=linux,node-role.kubernetes.io/worker=").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	nodeCount := int(strings.Count(workerNodeDetails, "Ready")) + int(strings.Count(workerNodeDetails, "NotReady"))
	return nodeCount
}

func getAlertmanagerVersion(oc *exutil.CLI) (string, error) {
	version, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterversion/version", "-ojsonpath={.status.desired.version}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	major := strings.Split(version, ".")[0]
	minor := strings.Split(version, ".")[1]
	minorInt, err := strconv.Atoi(minor)
	o.Expect(err).NotTo(o.HaveOccurred())
	switch {
	case major == "4" && minorInt >= 17:
		return "v2", nil
	case major == "4" && minorInt < 17:
		return "v1", nil
	default:
		return "", errors.New("Unknown version " + major + "." + minor)
	}
}

func getAlertQueries(oc *exutil.CLI) (string, string, error) {
	version, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterversion/version", "-ojsonpath={.status.desired.version}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	major := strings.Split(version, ".")[0]
	minor := strings.Split(version, ".")[1]
	minorInt, err := strconv.Atoi(minor)
	o.Expect(err).NotTo(o.HaveOccurred())
	switch {
	case major == "4" && minorInt >= 17:
		return "#.labels.alertname", "#(labels.alertname=", nil
	case major == "4" && minorInt < 17:
		return "data.#.labels.alertname", "data.#(labels.alertname=", nil
	default:
		return "", "", errors.New("Unknown version " + major + "." + minor)
	}
}

// create job for rapiddast test
// Run a job to do rapiddast, the scan result will be written into pod logs and store in artifactdirPath
func rapidastScan(oc *exutil.CLI, ns, configFile string, scanPolicyFile string, apiGroupName string) (bool, error) {
	//update the token and create a new config file
	content, err := os.ReadFile(configFile)
	if err != nil {
		return false, err
	}
	defer oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "remove-cluster-role-from-user", "cluster-admin", fmt.Sprintf("system:serviceaccount:%s:default", ns)).Execute()
	oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "add-cluster-role-to-user", "cluster-admin", fmt.Sprintf("system:serviceaccount:%s:default", ns)).Execute()
	token := getSAToken(oc, "default", ns)
	originConfig := string(content)
	targetConfig := strings.Replace(originConfig, "Bearer sha256~xxxxxxxx", "Bearer "+token, -1)
	newConfigFile := "/tmp/iscdast" + getRandomString()
	f, err := os.Create(newConfigFile)
	defer f.Close()
	defer exec.Command("rm", newConfigFile).Output()
	if err != nil {
		return false, err
	}
	f.WriteString(targetConfig)

	//Create configmap
	err = oc.WithoutNamespace().Run("create").Args("-n", ns, "configmap", "rapidast-configmap", "--from-file=rapidastconfig.yaml="+newConfigFile, "--from-file=customscan.policy="+scanPolicyFile).Execute()
	if err != nil {
		return false, err
	}

	//Create job
	iscBaseDir := exutil.FixturePath("testdata", "securityandcompliance")
	jobTemplate := filepath.Join(iscBaseDir, "rapidast/job_rapidast.yaml")
	err = oc.WithoutNamespace().Run("create").Args("-n", ns, "-f", jobTemplate).Execute()
	if err != nil {
		return false, err
	}
	//Waiting up to 10 minutes until pod Failed or Success
	err = wait.PollUntilContextTimeout(context.Background(), 30*time.Second, 10*time.Minute, true, func(context.Context) (done bool, err error) {
		jobStatus, err1 := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ns, "pod", "-l", "job-name=rapidast-job", "-ojsonpath={.items[0].status.phase}").Output()
		e2e.Logf(" rapidast Job status %s ", jobStatus)
		if err1 != nil {
			return false, nil
		}
		if jobStatus == "Pending" || jobStatus == "Running" {
			return false, nil
		}
		if jobStatus == "Failed" {
			return true, fmt.Errorf("rapidast-job status failed")
		}
		return jobStatus == "Succeeded", nil
	})
	//return if the pod status is not Succeeded
	if err != nil {
		return false, err
	}
	// Get the rapidast pod name
	jobPods, err := oc.AdminKubeClient().CoreV1().Pods(ns).List(context.Background(), metav1.ListOptions{LabelSelector: "job-name=rapidast-job"})
	if err != nil {
		return false, err
	}
	podLogs, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("-n", ns, jobPods.Items[0].Name).Output()
	//return if failed to get logs
	if err != nil {
		return false, err
	}

	// Copy DAST Report into $ARTIFACT_DIR
	artifactAvaiable := true
	artifactdirPath := os.Getenv("ARTIFACT_DIR")
	if artifactdirPath == "" {
		artifactAvaiable = false
	}
	info, err := os.Stat(artifactdirPath)
	if err != nil {
		e2e.Logf("%s doesn't exist", artifactdirPath)
		artifactAvaiable = false
	} else if !info.IsDir() {
		e2e.Logf("%s isn't a directory", artifactdirPath)
		artifactAvaiable = false
	}

	if artifactAvaiable {
		rapidastResultsSubDir := artifactdirPath + "/rapiddastresultsISC"
		err = os.MkdirAll(rapidastResultsSubDir, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		artifactFile := rapidastResultsSubDir + "/" + apiGroupName + "_rapidast.result"
		e2e.Logf("Write report into %s", artifactFile)
		f1, err := os.Create(artifactFile)
		defer f1.Close()
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = f1.WriteString(podLogs)
		o.Expect(err).NotTo(o.HaveOccurred())
	} else {
		// print pod logs if artifactdirPath is not writable
		e2e.Logf("#oc logs -n %s %s \n %s", jobPods.Items[0].Name, ns, podLogs)
	}

	//return false, if high risk is reported
	podLogA := strings.Split(podLogs, "\n")
	riskHigh := 0
	riskMedium := 0
	re1 := regexp.MustCompile(`"riskdesc": .*High`)
	re2 := regexp.MustCompile(`"riskdesc": .*Medium`)
	for _, item := range podLogA {
		if re1.MatchString(item) {
			riskHigh++
		}
		if re2.MatchString(item) {
			riskMedium++
		}
	}
	e2e.Logf("rapidast result: riskHigh=%v riskMedium=%v", riskHigh, riskMedium)

	if riskHigh > 0 {
		return false, fmt.Errorf("High risk alert, please check the scan result report")
	}
	return true, nil
}

func checkUpgradable(oc *exutil.CLI, source string, channel string, packagemanifest string, oldVersion string, csvPrefix string) (bool, error) {
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("packagemanifest", "-n", "openshift-marketplace", "-l", "catalog="+source, "-ojsonpath={.items}").Output()
	if err != nil {
		e2e.Logf("Failed to get packagemanifest for catalogsource %v", source)
		return false, err
	}
	packMS := []PackageManifest{}
	json.Unmarshal([]byte(output), &packMS)
	for _, pm := range packMS {
		if pm.Name == packagemanifest {
			for _, channels := range pm.Status.Channels {
				currentCSV := channels.CurrentCSV
				currentVersion := strings.ReplaceAll(currentCSV, csvPrefix, "")
				if channels.Name == channel && compareVersion(currentVersion, oldVersion) == 1 {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

func compareVersion(version1 string, version2 string) int {
	slicev1 := strings.Split(version1, ".")
	slicev2 := strings.Split(version2, ".")
	l1, l2 := len(slicev1), len(slicev2)

	i := 0
	for ; i < l1 && i < l2; i++ {
		num1, _ := strconv.Atoi(slicev1[i])
		num2, _ := strconv.Atoi(slicev2[i])
		if num1 > num2 {
			return 1
		} else if num1 < num2 {
			return -1
		}
	}

	if i == l1 {
		for k := i; k < l2; k++ {
			if n, _ := strconv.Atoi(slicev2[k]); n > 0 {
				return -1
			}
		}
	}

	if i == l2 {
		for k := i; k < l1; k++ {
			if n, _ := strconv.Atoi(slicev1[k]); n > 0 {
				return 1
			}
		}
	}
	return 0
}

func getMachineSetNameForOneSepecificNode(oc *exutil.CLI, nodeName string) string {
	machineNameAnnotation, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", nodeName, "-o=jsonpath={.metadata.annotations.machine\\.openshift\\.io/machine}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	machineName := strings.Split(machineNameAnnotation, "/")[1]
	machinesetName, errGet := oc.AsAdmin().WithoutNamespace().Run("get").Args("machine/"+machineName, "-n", "openshift-machine-api", "-o=jsonpath={.metadata.labels.machine\\.openshift\\.io/cluster-api-machineset}").Output()
	o.Expect(errGet).NotTo(o.HaveOccurred())
	o.Expect(machinesetName).NotTo(o.Equal(""))
	return machinesetName
}
