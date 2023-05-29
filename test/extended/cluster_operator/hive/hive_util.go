package hive

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type clusterMonitoringConfig struct {
	enableUserWorkload bool
	namespace          string
	template           string
}

type hiveNameSpace struct {
	name     string
	template string
}

type operatorGroup struct {
	name      string
	namespace string
	template  string
}

type subscription struct {
	name            string
	namespace       string
	channel         string
	approval        string
	operatorName    string
	sourceName      string
	sourceNamespace string
	startingCSV     string
	currentCSV      string
	installedCSV    string
	template        string
}

type hiveconfig struct {
	logLevel        string
	targetNamespace string
	template        string
}

type clusterImageSet struct {
	name         string
	releaseImage string
	template     string
}

type clusterPool struct {
	name           string
	namespace      string
	fake           string
	baseDomain     string
	imageSetRef    string
	platformType   string
	credRef        string
	region         string
	pullSecretRef  string
	size           int
	maxSize        int
	runningCount   int
	maxConcurrent  int
	hibernateAfter string
	template       string
}

type clusterClaim struct {
	name            string
	namespace       string
	clusterPoolName string
	template        string
}

type installConfig struct {
	name1      string
	namespace  string
	baseDomain string
	name2      string
	region     string
	template   string
}

type clusterDeployment struct {
	fake                 string
	name                 string
	namespace            string
	baseDomain           string
	clusterName          string
	manageDNS            bool
	platformType         string
	credRef              string
	region               string
	imageSetRef          string
	installConfigSecret  string
	pullSecretRef        string
	installAttemptsLimit int
	template             string
}

type machinepool struct {
	clusterName string
	namespace   string
	iops        int
	template    string
}

type syncSetResource struct {
	name          string
	namespace     string
	namespace2    string
	cdrefname     string
	ramode        string
	applybehavior string
	cmname        string
	cmnamespace   string
	template      string
}

type syncSetPatch struct {
	name        string
	namespace   string
	cdrefname   string
	cmname      string
	cmnamespace string
	pcontent    string
	patchType   string
	template    string
}

type syncSetSecret struct {
	name       string
	namespace  string
	cdrefname  string
	sname      string
	snamespace string
	tname      string
	tnamespace string
	template   string
}

type objectTableRef struct {
	kind      string
	namespace string
	name      string
}

// Azure
type azureInstallConfig struct {
	name1      string
	namespace  string
	baseDomain string
	name2      string
	resGroup   string
	azureType  string
	region     string
	template   string
}

type azureClusterDeployment struct {
	fake                   string
	copyCliDomain          string
	name                   string
	namespace              string
	baseDomain             string
	clusterName            string
	platformType           string
	credRef                string
	region                 string
	resGroup               string
	azureType              string
	imageSetRef            string
	installConfigSecret    string
	installerImageOverride string
	pullSecretRef          string
	template               string
}

type azureClusterPool struct {
	name           string
	namespace      string
	fake           string
	baseDomain     string
	imageSetRef    string
	platformType   string
	credRef        string
	region         string
	resGroup       string
	pullSecretRef  string
	size           int
	maxSize        int
	runningCount   int
	maxConcurrent  int
	hibernateAfter string
	template       string
}

// GCP
type gcpInstallConfig struct {
	name1      string
	namespace  string
	baseDomain string
	name2      string
	region     string
	projectid  string
	template   string
}

type gcpClusterDeployment struct {
	fake                   string
	name                   string
	namespace              string
	baseDomain             string
	clusterName            string
	platformType           string
	credRef                string
	region                 string
	imageSetRef            string
	installConfigSecret    string
	pullSecretRef          string
	installerImageOverride string
	installAttemptsLimit   int
	template               string
}

type gcpClusterPool struct {
	name           string
	namespace      string
	fake           string
	baseDomain     string
	imageSetRef    string
	platformType   string
	credRef        string
	region         string
	pullSecretRef  string
	size           int
	maxSize        int
	runningCount   int
	maxConcurrent  int
	hibernateAfter string
	template       string
}

type prometheusQueryResult struct {
	Data struct {
		Result []struct {
			Metric struct {
				Name                 string `json:"__name__"`
				ClusterpoolName      string `json:"clusterpool_name"`
				ClusterpoolNamespace string `json:"clusterpool_namespace"`
				ClusterDeployment    string `json:"cluster_deployment"`
				ExportedNamespace    string `json:"exported_namespace"`
				ClusterType          string `json:"cluster_type"`
				ClusterVersion       string `json:"cluster_version"`
				InstallAttempt       string `json:"install_attempt"`
				Platform             string `json:"platform"`
				Region               string `json:"region"`
				Prometheus           string `json:"prometheus"`
				Condition            string `json:"condition"`
				Reason               string `json:"reason"`
				Endpoint             string `json:"endpoint"`
				Instance             string `json:"instance"`
				Job                  string `json:"job"`
				Namespace            string `json:"namespace"`
				Pod                  string `json:"pod"`
				Workers              string `json:"workers"`
				Service              string `json:"service"`
			} `json:"metric"`
			Value []interface{} `json:"value"`
		} `json:"result"`
		ResultType string `json:"resultType"`
	} `json:"data"`
	Status string `json:"status"`
}

// Hive Configurations
const (
	HiveNamespace             = "hive" //Hive Namespace
	PullSecret                = "pull-secret"
	PrometheusURL             = "https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query?query="
	thanosQuerierURL          = "https://thanos-querier.openshift-monitoring.svc:9091/api/v1/query?query="
	ClusterInstallTimeout     = 3600
	DefaultTimeout            = 120
	FakeClusterInstallTimeout = 600
	ClusterResumeTimeout      = 1200
	ClusterUninstallTimeout   = 1800
	HibernateAfterTimer       = 300
	ClusterSuffixLen          = 4
	LogsLimitLen              = 1024
)

// AWS Configurations
const (
	AWSBaseDomain  = "qe.devcluster.openshift.com" //AWS BaseDomain
	AWSRegion      = "us-east-2"
	AWSCreds       = "aws-creds"
	HiveManagedDNS = "hivemanageddns" //for all manage DNS Domain
)

// Azure Configurations
const (
	AzureClusterInstallTimeout = 4500
	AzureBaseDomain            = "qe.azure.devcluster.openshift.com" //Azure BaseDomain
	AzureRegion                = "centralus"
	AzureCreds                 = "azure-credentials"
	AzureRESGroup              = "os4-common"
	AzurePublic                = "AzurePublicCloud"
)

// GCP Configurations
const (
	GCPBaseDomain  = "qe.gcp.devcluster.openshift.com" //GCP BaseDomain
	GCPBaseDomain2 = "qe1.gcp.devcluster.openshift.com"
	GCPRegion      = "us-central1"
	GCPCreds       = "gcp-credentials"
)

func applyResourceFromTemplate(oc *exutil.CLI, parameters ...string) error {
	var cfgFileJSON string
	err := wait.Poll(3*time.Second, 15*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().Run("process").Args(parameters...).OutputToFile(getRandomString() + "-hive-resource-cfg.json")
		if err != nil {
			e2e.Logf("the err:%v, and try next round", err)
			return false, nil
		}
		cfgFileJSON = output
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, "fail to create config file")

	e2e.Logf("the file of resource is %s", cfgFileJSON)
	defer os.Remove(cfgFileJSON)
	return oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", cfgFileJSON).Execute()
}

func getRandomString() string {
	chars := "abcdefghijklmnopqrstuvwxyz0123456789"
	seed := rand.New(rand.NewSource(time.Now().UnixNano()))
	buffer := make([]byte, 8)
	for index := range buffer {
		buffer[index] = chars[seed.Intn(len(chars))]
	}
	return string(buffer)
}

func (cmc *clusterMonitoringConfig) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", cmc.template, "-p", "ENABLEUSERWORKLOAD="+strconv.FormatBool(cmc.enableUserWorkload), "NAMESPACE="+cmc.namespace)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Create hive namespace if not exist
func (ns *hiveNameSpace) createIfNotExist(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", ns.template, "-p", "NAME="+ns.name)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Create operatorGroup for Hive if not exist
func (og *operatorGroup) createIfNotExist(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", og.template, "-p", "NAME="+og.name, "NAMESPACE="+og.namespace)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (sub *subscription) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", sub.template, "-p", "NAME="+sub.name, "NAMESPACE="+sub.namespace, "CHANNEL="+sub.channel,
		"APPROVAL="+sub.approval, "OPERATORNAME="+sub.operatorName, "SOURCENAME="+sub.sourceName, "SOURCENAMESPACE="+sub.sourceNamespace, "STARTINGCSV="+sub.startingCSV)
	o.Expect(err).NotTo(o.HaveOccurred())
	if strings.Compare(sub.approval, "Automatic") == 0 {
		sub.findInstalledCSV(oc)
	} else {
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "UpgradePending", ok, DefaultTimeout, []string{"sub", sub.name, "-n", sub.namespace, "-o=jsonpath={.status.state}"}).check(oc)
	}
}

// Create subscription for Hive if not exist and wait for resource is ready
func (sub *subscription) createIfNotExist(oc *exutil.CLI) {

	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", "-n", sub.namespace).Output()
	if strings.Contains(output, "NotFound") || strings.Contains(output, "No resources") || err != nil {
		e2e.Logf("No hive subscription, Create it.")
		err = applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", sub.template, "-p", "NAME="+sub.name, "NAMESPACE="+sub.namespace, "CHANNEL="+sub.channel,
			"APPROVAL="+sub.approval, "OPERATORNAME="+sub.operatorName, "SOURCENAME="+sub.sourceName, "SOURCENAMESPACE="+sub.sourceNamespace, "STARTINGCSV="+sub.startingCSV)
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Compare(sub.approval, "Automatic") == 0 {
			sub.findInstalledCSV(oc)
		} else {
			newCheck("expect", "get", asAdmin, withoutNamespace, compare, "UpgradePending", ok, DefaultTimeout, []string{"sub", sub.name, "-n", sub.namespace, "-o=jsonpath={.status.state}"}).check(oc)
		}
		//wait for pod running
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "Running", ok, DefaultTimeout, []string{"pod", "--selector=control-plane=hive-operator", "-n",
			sub.namespace, "-o=jsonpath={.items[0].status.phase}"}).check(oc)
	} else {
		e2e.Logf("hive subscription already exists.")
	}

}

func (sub *subscription) findInstalledCSV(oc *exutil.CLI) {
	newCheck("expect", "get", asAdmin, withoutNamespace, compare, "AtLatestKnown", ok, DefaultTimeout, []string{"sub", sub.name, "-n", sub.namespace, "-o=jsonpath={.status.state}"}).check(oc)
	installedCSV := getResource(oc, asAdmin, withoutNamespace, "sub", sub.name, "-n", sub.namespace, "-o=jsonpath={.status.installedCSV}")
	o.Expect(installedCSV).NotTo(o.BeEmpty())
	if strings.Compare(sub.installedCSV, installedCSV) != 0 {
		sub.installedCSV = installedCSV
	}
	e2e.Logf("the installed CSV name is %s", sub.installedCSV)
}

func (hc *hiveconfig) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", hc.template, "-p", "LOGLEVEL="+hc.logLevel, "TARGETNAMESPACE="+hc.targetNamespace)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Create hivconfig if not exist and wait for resource is ready
func (hc *hiveconfig) createIfNotExist(oc *exutil.CLI) {
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("HiveConfig", "hive").Output()
	if strings.Contains(output, "have a resource type") || err != nil {
		e2e.Logf("No hivconfig, Create it.")
		err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", hc.template, "-p", "LOGLEVEL="+hc.logLevel, "TARGETNAMESPACE="+hc.targetNamespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		//wait for pods running
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "hive-clustersync", ok, DefaultTimeout, []string{"pod", "--selector=control-plane=clustersync",
			"-n", HiveNamespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "Running", ok, DefaultTimeout, []string{"pod", "--selector=control-plane=clustersync", "-n",
			HiveNamespace, "-o=jsonpath={.items[0].status.phase}"}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "hive-controllers", ok, DefaultTimeout, []string{"pod", "--selector=control-plane=controller-manager",
			"-n", HiveNamespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "Running", ok, DefaultTimeout, []string{"pod", "--selector=control-plane=controller-manager", "-n",
			HiveNamespace, "-o=jsonpath={.items[0].status.phase}"}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "hiveadmission", ok, DefaultTimeout, []string{"pod", "--selector=app=hiveadmission",
			"-n", HiveNamespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "Running Running", ok, DefaultTimeout, []string{"pod", "--selector=app=hiveadmission", "-n",
			HiveNamespace, "-o=jsonpath={.items[*].status.phase}"}).check(oc)
	} else {
		e2e.Logf("hivconfig already exists.")
	}

}

func (imageset *clusterImageSet) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", imageset.template, "-p", "NAME="+imageset.name, "RELEASEIMAGE="+imageset.releaseImage)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (pool *clusterPool) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", pool.template, "-p", "NAME="+pool.name, "NAMESPACE="+pool.namespace, "FAKE="+pool.fake, "BASEDOMAIN="+pool.baseDomain, "IMAGESETREF="+pool.imageSetRef, "PLATFORMTYPE="+pool.platformType, "CREDREF="+pool.credRef, "REGION="+pool.region, "PULLSECRETREF="+pool.pullSecretRef, "SIZE="+strconv.Itoa(pool.size), "MAXSIZE="+strconv.Itoa(pool.maxSize), "RUNNINGCOUNT="+strconv.Itoa(pool.runningCount), "MAXCONCURRENT="+strconv.Itoa(pool.maxConcurrent), "HIBERNATEAFTER="+pool.hibernateAfter)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (claim *clusterClaim) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", claim.template, "-p", "NAME="+claim.name, "NAMESPACE="+claim.namespace, "CLUSTERPOOLNAME="+claim.clusterPoolName)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (config *installConfig) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", config.template, "-p", "NAME1="+config.name1, "NAMESPACE="+config.namespace, "BASEDOMAIN="+config.baseDomain, "NAME2="+config.name2, "REGION="+config.region)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (cluster *clusterDeployment) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", cluster.template, "-p", "FAKE="+cluster.fake, "NAME="+cluster.name, "NAMESPACE="+cluster.namespace, "BASEDOMAIN="+cluster.baseDomain, "CLUSTERNAME="+cluster.clusterName, "MANAGEDNS="+strconv.FormatBool(cluster.manageDNS), "PLATFORMTYPE="+cluster.platformType, "CREDREF="+cluster.credRef, "REGION="+cluster.region, "IMAGESETREF="+cluster.imageSetRef, "INSTALLCONFIGSECRET="+cluster.installConfigSecret, "PULLSECRETREF="+cluster.pullSecretRef, "INSTALLATTEMPTSLIMIT="+strconv.Itoa(cluster.installAttemptsLimit))
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (machine *machinepool) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", machine.template, "-p", "CLUSTERNAME="+machine.clusterName, "NAMESPACE="+machine.namespace, "IOPS="+strconv.Itoa(machine.iops))
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (syncresource *syncSetResource) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", syncresource.template, "-p", "NAME="+syncresource.name, "NAMESPACE="+syncresource.namespace, "CDREFNAME="+syncresource.cdrefname, "NAMESPACE2="+syncresource.namespace2, "RAMODE="+syncresource.ramode, "APPLYBEHAVIOR="+syncresource.applybehavior, "CMNAME="+syncresource.cmname, "CMNAMESPACE="+syncresource.cmnamespace)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (syncpatch *syncSetPatch) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", syncpatch.template, "-p", "NAME="+syncpatch.name, "NAMESPACE="+syncpatch.namespace, "CDREFNAME="+syncpatch.cdrefname, "CMNAME="+syncpatch.cmname, "CMNAMESPACE="+syncpatch.cmnamespace, "PCONTENT="+syncpatch.pcontent, "PATCHTYPE="+syncpatch.patchType)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (syncsecret *syncSetSecret) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", syncsecret.template, "-p", "NAME="+syncsecret.name, "NAMESPACE="+syncsecret.namespace, "CDREFNAME="+syncsecret.cdrefname, "SNAME="+syncsecret.sname, "SNAMESPACE="+syncsecret.snamespace, "TNAME="+syncsecret.tname, "TNAMESPACE="+syncsecret.tnamespace)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Azure
func (config *azureInstallConfig) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", config.template, "-p", "NAME1="+config.name1, "NAMESPACE="+config.namespace, "BASEDOMAIN="+config.baseDomain, "NAME2="+config.name2, "RESGROUP="+config.resGroup, "AZURETYPE="+config.azureType, "REGION="+config.region)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (cluster *azureClusterDeployment) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", cluster.template, "-p", "FAKE="+cluster.fake, "COPYCLIDOMAIN="+cluster.copyCliDomain, "NAME="+cluster.name, "NAMESPACE="+cluster.namespace, "BASEDOMAIN="+cluster.baseDomain, "CLUSTERNAME="+cluster.clusterName, "PLATFORMTYPE="+cluster.platformType, "CREDREF="+cluster.credRef, "REGION="+cluster.region, "RESGROUP="+cluster.resGroup, "AZURETYPE="+cluster.azureType, "IMAGESETREF="+cluster.imageSetRef, "INSTALLCONFIGSECRET="+cluster.installConfigSecret, "INSTALLERIMAGEOVERRIDE="+cluster.installerImageOverride, "PULLSECRETREF="+cluster.pullSecretRef)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (pool *azureClusterPool) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", pool.template, "-p", "NAME="+pool.name, "NAMESPACE="+pool.namespace, "FAKE="+pool.fake, "BASEDOMAIN="+pool.baseDomain, "IMAGESETREF="+pool.imageSetRef, "PLATFORMTYPE="+pool.platformType, "CREDREF="+pool.credRef, "REGION="+pool.region, "RESGROUP="+pool.resGroup, "PULLSECRETREF="+pool.pullSecretRef, "SIZE="+strconv.Itoa(pool.size), "MAXSIZE="+strconv.Itoa(pool.maxSize), "RUNNINGCOUNT="+strconv.Itoa(pool.runningCount), "MAXCONCURRENT="+strconv.Itoa(pool.maxConcurrent), "HIBERNATEAFTER="+pool.hibernateAfter)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// GCP
func (config *gcpInstallConfig) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", config.template, "-p", "NAME1="+config.name1, "NAMESPACE="+config.namespace, "BASEDOMAIN="+config.baseDomain, "NAME2="+config.name2, "REGION="+config.region, "PROJECTID="+config.projectid)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (cluster *gcpClusterDeployment) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", cluster.template, "-p", "FAKE="+cluster.fake, "NAME="+cluster.name, "NAMESPACE="+cluster.namespace, "BASEDOMAIN="+cluster.baseDomain, "CLUSTERNAME="+cluster.clusterName, "PLATFORMTYPE="+cluster.platformType, "CREDREF="+cluster.credRef, "REGION="+cluster.region, "IMAGESETREF="+cluster.imageSetRef, "INSTALLCONFIGSECRET="+cluster.installConfigSecret, "PULLSECRETREF="+cluster.pullSecretRef, "INSTALLERIMAGEOVERRIDE="+cluster.installerImageOverride, "INSTALLATTEMPTSLIMIT="+strconv.Itoa(cluster.installAttemptsLimit))
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (pool *gcpClusterPool) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", pool.template, "-p", "NAME="+pool.name, "NAMESPACE="+pool.namespace, "FAKE="+pool.fake, "BASEDOMAIN="+pool.baseDomain, "IMAGESETREF="+pool.imageSetRef, "PLATFORMTYPE="+pool.platformType, "CREDREF="+pool.credRef, "REGION="+pool.region, "PULLSECRETREF="+pool.pullSecretRef, "SIZE="+strconv.Itoa(pool.size), "MAXSIZE="+strconv.Itoa(pool.maxSize), "RUNNINGCOUNT="+strconv.Itoa(pool.runningCount), "MAXCONCURRENT="+strconv.Itoa(pool.maxConcurrent), "HIBERNATEAFTER="+pool.hibernateAfter)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func getResource(oc *exutil.CLI, asAdmin bool, withoutNamespace bool, parameters ...string) string {
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
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("cat not get %v without empty", parameters))
	e2e.Logf("the result of queried resource:%v", result)
	return result
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

// Check the resource meets the expect
// parameter method: expect or present
// parameter action: get, patch, delete, ...
// parameter executor: asAdmin or not
// parameter inlineNamespace: withoutNamespace or not
// parameter expectAction: Compare or not
// parameter expectContent: expected string
// parameter expect: ok, expected to have expectContent; nok, not expected to have expectContent
// parameter timeout: use CLUSTER_INSTALL_TIMEOUT de default, and CLUSTER_INSTALL_TIMEOUT, CLUSTER_RESUME_TIMEOUT etc in different scenarios
// parameter resource: resource
func newCheck(method string, action string, executor bool, inlineNamespace bool, expectAction bool,
	expectContent string, expect bool, timeout int, resource []string) checkDescription {
	return checkDescription{
		method:          method,
		action:          action,
		executor:        executor,
		inlineNamespace: inlineNamespace,
		expectAction:    expectAction,
		expectContent:   expectContent,
		expect:          expect,
		timeout:         timeout,
		resource:        resource,
	}
}

type checkDescription struct {
	method          string
	action          string
	executor        bool
	inlineNamespace bool
	expectAction    bool
	expectContent   string
	expect          bool
	timeout         int
	resource        []string
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

func (ck checkDescription) check(oc *exutil.CLI) {
	switch ck.method {
	case "present":
		ok := isPresentResource(oc, ck.action, ck.executor, ck.inlineNamespace, ck.expectAction, ck.resource...)
		o.Expect(ok).To(o.BeTrue())
	case "expect":
		err := expectedResource(oc, ck.action, ck.executor, ck.inlineNamespace, ck.expectAction, ck.expectContent, ck.expect, ck.timeout, ck.resource...)
		exutil.AssertWaitPollNoErr(err, "can not get expected result")
	default:
		err := fmt.Errorf("unknown method")
		o.Expect(err).NotTo(o.HaveOccurred())
	}
}

func isPresentResource(oc *exutil.CLI, action string, asAdmin bool, withoutNamespace bool, present bool, parameters ...string) bool {
	parameters = append(parameters, "--ignore-not-found")
	err := wait.Poll(3*time.Second, 60*time.Second, func() (bool, error) {
		output, err := doAction(oc, action, asAdmin, withoutNamespace, parameters...)
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
	return err == nil
}

func expectedResource(oc *exutil.CLI, action string, asAdmin bool, withoutNamespace bool, isCompare bool, content string, expect bool, timeout int, parameters ...string) error {
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
	var interval, inputTimeout time.Duration
	if timeout >= ClusterInstallTimeout {
		inputTimeout = time.Duration(timeout/60) * time.Minute
		interval = 6 * time.Minute
	} else {
		inputTimeout = time.Duration(timeout) * time.Second
		interval = time.Duration(timeout/60) * time.Second
	}
	return wait.Poll(interval, inputTimeout, func() (bool, error) {
		output, err := doAction(oc, action, asAdmin, withoutNamespace, parameters...)
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

// clean up the object resource
func cleanupObjects(oc *exutil.CLI, objs ...objectTableRef) {
	for _, v := range objs {
		e2e.Logf("Start to remove: %v", v)
		//Print out debugging info if CD installed is false
		var provisionPodOutput, installedFlag string
		if v.kind == "ClusterPool" {
			if v.namespace != "" {
				cdListStr := getCDlistfromPool(oc, v.name)
				var cdArray []string
				cdArray = strings.Split(strings.TrimSpace(cdListStr), "\n")
				for i := range cdArray {
					installedFlag, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args("ClusterDeployment", "-n", cdArray[i], cdArray[i], "-o=jsonpath={.spec.installed}").Output()
					if installedFlag == "false" {
						failedCdName := cdArray[i]
						e2e.Logf("failedCdName is %s", failedCdName)
						//At present, the maximum size of clusterpool in auto test is 2, we can print them all to get more information if cd installed is false
						printStatusConditions(oc, "ClusterDeployment", failedCdName, failedCdName)
						printProvisionPodLogs(oc, provisionPodOutput, failedCdName)
					}
				}
			}
		} else if v.kind == "ClusterDeployment" {
			installedFlag, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args(v.kind, "-n", v.namespace, v.name, "-o=jsonpath={.spec.installed}").Output()
			if installedFlag == "false" {
				printStatusConditions(oc, v.kind, v.namespace, v.name)
				printProvisionPodLogs(oc, provisionPodOutput, v.namespace)
			}
		}
		if v.namespace != "" {
			oc.AsAdmin().WithoutNamespace().Run("delete").Args(v.kind, "-n", v.namespace, v.name, "--ignore-not-found").Output()
		} else {
			oc.AsAdmin().WithoutNamespace().Run("delete").Args(v.kind, v.name, "--ignore-not-found").Output()
		}
		//For ClusterPool or ClusterDeployment, need to wait ClusterDeployment delete done
		if v.kind == "ClusterPool" || v.kind == "ClusterDeployment" {
			e2e.Logf("Wait ClusterDeployment delete done for %s", v.name)
			newCheck("expect", "get", asAdmin, withoutNamespace, contain, v.name, nok, ClusterUninstallTimeout, []string{"ClusterDeployment", "-A"}).check(oc)
		}
	}
}

// print out the status conditions
func printStatusConditions(oc *exutil.CLI, kind, namespace, name string) {
	statusConditions, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args(kind, "-n", namespace, name, "-o=jsonpath={.status.conditions}").Output()
	if len(statusConditions) <= LogsLimitLen {
		e2e.Logf("statusConditions is %s", statusConditions)
	} else {
		e2e.Logf("statusConditions is %s", statusConditions[:LogsLimitLen])
	}
}

// print out provision pod logs
func printProvisionPodLogs(oc *exutil.CLI, provisionPodOutput, namespace string) {
	provisionPodOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-l", "hive.openshift.io/job-type=provision", "-n", namespace, "-o=jsonpath={.items[*].metadata.name}").Output()
	e2e.Logf("provisionPodOutput is %s", provisionPodOutput)
	//if err == nil , print out provision pod logs
	if err == nil && len(strings.TrimSpace(provisionPodOutput)) > 0 {
		var provisionPod []string
		provisionPod = strings.Split(strings.TrimSpace(provisionPodOutput), " ")
		e2e.Logf("provisionPod is %s", provisionPod)
		if len(provisionPod) > 0 {
			e2e.Logf("provisionPod len is %d. provisionPod[0] is %s", len(provisionPod), provisionPod[0])
			provisionPodLogsFile := "logs_output_" + getRandomString()[:ClusterSuffixLen] + ".txt"
			provisionPodLogs, _ := oc.AsAdmin().WithoutNamespace().Run("logs").Args(provisionPod[0], "-c", "hive", "-n", namespace).OutputToFile(provisionPodLogsFile)
			defer os.Remove(provisionPodLogs)
			failLogs, _ := exec.Command("bash", "-c", "grep -E 'level=error|level=fatal' "+provisionPodLogs).Output()
			if len(failLogs) <= LogsLimitLen {
				e2e.Logf("provisionPodLogs is %s", failLogs)
			} else {
				e2e.Logf("provisionPodLogs is %s", failLogs[len(failLogs)-LogsLimitLen:])
			}
		}
	}
}

func getProvisionPodName(oc *exutil.CLI, cdName, namespace string) string {
	provisionPodName, _, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-l", "hive.openshift.io/job-type=provision", "-l", "hive.openshift.io/cluster-deployment-name="+cdName, "-n", namespace, "-o=jsonpath={.items[0].metadata.name}").Outputs()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(provisionPodName).To(o.ContainSubstring("provision"))
	o.Expect(provisionPodName).To(o.ContainSubstring(cdName))

	return provisionPodName
}

/*
Looks for targetLines in the transformed provision log stream with a timeout.
Default lineTransformation is the identity function.
Suitable for test cases for which logs can be checked before the provision is finished.

Example:

Provision logs (logStream.r's underlying data) = "foo\nbar\nbaz\nquux";
targetLines = []string{"ar", "baz", "qu"};
lineTransformation = nil;
targetLines found in provision logs -> returns true
*/
func assertLogs(logStream *os.File, targetLines []string, lineTransformation func(line string) string, timeout time.Duration) bool {
	// Set timeout (applies to future AND currently-blocked Read calls)
	endTime := time.Now().Add(timeout)
	err := logStream.SetReadDeadline(endTime)
	o.Expect(err).NotTo(o.HaveOccurred())

	// Default line transformation: the identity function
	if lineTransformation == nil {
		e2e.Logf("Using default line transformation (the identity function)")
		lineTransformation = func(line string) string { return line }
	}

	// Line scanning
	scanner := bufio.NewScanner(logStream)
	targetIdx := 0
	// In case of timeout, current & subsequent Read calls error out, resulting in scanner.Scan() returning false immediately
	for scanner.Scan() {
		switch tranformedLine, targetLine := lineTransformation(scanner.Text()), targetLines[targetIdx]; {
		// We have a match, proceed to the next target line
		case targetIdx == 0 && strings.HasSuffix(tranformedLine, targetLine) ||
			targetIdx == len(targetLines)-1 && strings.HasPrefix(tranformedLine, targetLine) ||
			tranformedLine == targetLine:
			if targetIdx++; targetIdx == len(targetLines) {
				e2e.Logf("Found substring [%v] in the logs", strings.Join(targetLines, "\n"))
				return true
			}
		// Restart from target line 0
		default:
			targetIdx = 0
		}
	}

	return false
}

func removeResource(oc *exutil.CLI, parameters ...string) {
	output, err := oc.AsAdmin().WithoutNamespace().Run("delete").Args(parameters...).Output()
	if err != nil && (strings.Contains(output, "NotFound") || strings.Contains(output, "No resources found")) {
		e2e.Logf("No resource found!")
		return
	}
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (hc *hiveconfig) delete(oc *exutil.CLI) {
	removeResource(oc, "hiveconfig", "hive")
}

// Create pull-secret in current project namespace
func createPullSecret(oc *exutil.CLI, namespace string) {
	dirname := "/tmp/" + oc.Namespace() + "-pull"
	err := os.MkdirAll(dirname, 0777)
	o.Expect(err).NotTo(o.HaveOccurred())
	defer os.RemoveAll(dirname)

	err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", "--to="+dirname, "--confirm").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())

	err = oc.Run("create").Args("secret", "generic", "pull-secret", "--from-file="+dirname+"/.dockerconfigjson", "-n", namespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Create AWS credentials in current project namespace
func createAWSCreds(oc *exutil.CLI, namespace string) {
	dirname := "/tmp/" + oc.Namespace() + "-creds"
	err := os.MkdirAll(dirname, 0777)
	o.Expect(err).NotTo(o.HaveOccurred())
	defer os.RemoveAll(dirname)
	err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/aws-creds", "-n", "kube-system", "--to="+dirname, "--confirm").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	err = oc.Run("create").Args("secret", "generic", "aws-creds", "--from-file="+dirname+"/aws_access_key_id", "--from-file="+dirname+"/aws_secret_access_key", "-n", namespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Create Route53 AWS credentials in hive namespace
func createRoute53AWSCreds(oc *exutil.CLI, namespace string) {
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret", "route53-aws-creds", "-n", HiveNamespace).Output()
	if strings.Contains(output, "NotFound") || err != nil {
		e2e.Logf("No route53-aws-creds, Create it.")
		dirname := "/tmp/" + oc.Namespace() + "-route53-creds"
		err = os.MkdirAll(dirname, 0777)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(dirname)
		err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/aws-creds", "-n", "kube-system", "--to="+dirname, "--confirm").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("secret", "generic", "route53-aws-creds", "--from-file="+dirname+"/aws_access_key_id", "--from-file="+dirname+"/aws_secret_access_key", "-n", HiveNamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	} else {
		e2e.Logf("route53-aws-creds already exists.")
	}
}

// Create Azure credentials in current project namespace
func createAzureCreds(oc *exutil.CLI, namespace string) {
	dirname := "/tmp/" + oc.Namespace() + "-creds"
	err := os.MkdirAll(dirname, 0777)
	o.Expect(err).NotTo(o.HaveOccurred())
	defer os.RemoveAll(dirname)

	var azureClientID, azureClientSecret, azureSubscriptionID, azureTenantID string
	azureClientID, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("secret/azure-credentials", "-n", "kube-system", "--template='{{.data.azure_client_id | base64decode}}'").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	azureClientSecret, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("secret/azure-credentials", "-n", "kube-system", "--template='{{.data.azure_client_secret | base64decode}}'").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	azureSubscriptionID, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("secret/azure-credentials", "-n", "kube-system", "--template='{{.data.azure_subscription_id | base64decode}}'").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	azureTenantID, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("secret/azure-credentials", "-n", "kube-system", "--template='{{.data.azure_tenant_id | base64decode}}'").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	//Convert credentials to osServicePrincipal.json format
	output := fmt.Sprintf("{\"subscriptionId\":\"%s\",\"clientId\":\"%s\",\"clientSecret\":\"%s\",\"tenantId\":\"%s\"}", azureSubscriptionID[1:len(azureSubscriptionID)-1], azureClientID[1:len(azureClientID)-1], azureClientSecret[1:len(azureClientSecret)-1], azureTenantID[1:len(azureTenantID)-1])
	outputFile, outputErr := os.OpenFile(dirname+"/osServicePrincipal.json", os.O_CREATE|os.O_WRONLY, 0666)
	o.Expect(outputErr).NotTo(o.HaveOccurred())
	defer outputFile.Close()
	outputWriter := bufio.NewWriter(outputFile)
	writeByte, writeError := outputWriter.WriteString(output)
	o.Expect(writeError).NotTo(o.HaveOccurred())
	writeError = outputWriter.Flush()
	o.Expect(writeError).NotTo(o.HaveOccurred())
	e2e.Logf("%d byte written to osServicePrincipal.json", writeByte)
	err = oc.Run("create").Args("secret", "generic", AzureCreds, "--from-file="+dirname+"/osServicePrincipal.json", "-n", namespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Create GCP credentials in current project namespace
func createGCPCreds(oc *exutil.CLI, namespace string) {
	dirname := "/tmp/" + oc.Namespace() + "-creds"
	err := os.MkdirAll(dirname, 0777)
	o.Expect(err).NotTo(o.HaveOccurred())
	defer os.RemoveAll(dirname)

	err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/gcp-credentials", "-n", "kube-system", "--to="+dirname, "--confirm").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())

	err = oc.Run("create").Args("secret", "generic", GCPCreds, "--from-file=osServiceAccount.json="+dirname+"/service_account.json", "-n", namespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Reutrn Rlease version from Image
func extractRelfromImg(image string) string {
	index := strings.Index(image, ":")
	if index != -1 {
		tempStr := image[index+1:]
		index = strings.Index(tempStr, "-")
		if index != -1 {
			e2e.Logf("Extracted OCP release: %s", tempStr[:index])
			return tempStr[:index]
		}
	}
	e2e.Logf("Failed to extract OCP release from Image.")
	return ""
}

// Get CD list from Pool
// Return string CD list such as "pool-44945-2bbln5m47s\n pool-44945-f8xlv6m6s"
func getCDlistfromPool(oc *exutil.CLI, pool string) string {
	fileName := "cd_output_" + getRandomString() + ".txt"
	cdOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("cd", "-A").OutputToFile(fileName)
	o.Expect(err).NotTo(o.HaveOccurred())
	defer os.Remove(cdOutput)
	poolCdList, err := exec.Command("bash", "-c", "cat "+cdOutput+" | grep "+pool+" | awk '{print $1}'").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("CD list is %s for pool %s", poolCdList, pool)
	return string(poolCdList)
}

// Get cluster kubeconfig file
func getClusterKubeconfig(oc *exutil.CLI, clustername, namespace, dir string) {
	kubeconfigsecretname, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("cd", clustername, "-n", namespace, "-o=jsonpath={.spec.clusterMetadata.adminKubeconfigSecretRef.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("Extract cluster %s kubeconfig to %s", clustername, dir)
	err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/"+kubeconfigsecretname, "-n", namespace, "--to="+dir, "--confirm").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Check resource number after filtering
func checkResourceNumber(oc *exutil.CLI, filterName string, resource []string) int {
	resourceOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(resource...).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return strings.Count(resourceOutput, filterName)
}

func getPullSecret(oc *exutil.CLI) (string, error) {
	return oc.AsAdmin().WithoutNamespace().Run("get").Args("secret/pull-secret", "-n", "openshift-config", `--template={{index .data ".dockerconfigjson" | base64decode}}`).OutputToFile("auth.dockerconfigjson")
}

func getCommitID(oc *exutil.CLI, component string, clusterVersion string) (string, error) {
	secretFile, secretErr := getPullSecret(oc)
	defer os.Remove(secretFile)
	if secretErr != nil {
		return "", secretErr
	}
	outFilePath, ocErr := oc.AsAdmin().WithoutNamespace().Run("adm").Args("release", "info", "--registry-config="+secretFile, "--commits", clusterVersion, "--insecure=true").OutputToFile("commitIdLogs.txt")
	defer os.Remove(outFilePath)
	if ocErr != nil {
		return "", ocErr
	}
	commitID, cmdErr := exec.Command("bash", "-c", "cat "+outFilePath+" | grep "+component+" | awk '{print $3}'").Output()
	return strings.TrimSuffix(string(commitID), "\n"), cmdErr
}

func getPullSpec(oc *exutil.CLI, component string, clusterVersion string) (string, error) {
	secretFile, secretErr := getPullSecret(oc)
	defer os.Remove(secretFile)
	if secretErr != nil {
		return "", secretErr
	}
	pullSpec, ocErr := oc.AsAdmin().WithoutNamespace().Run("adm").Args("release", "info", "--registry-config="+secretFile, "--image-for="+component, clusterVersion, "--insecure=true").Output()
	if ocErr != nil {
		return "", ocErr
	}
	return pullSpec, nil
}

const (
	enable  = true
	disable = false
)

// Expose Hive metrics as a user-defined project
// The cluster's status of monitoring before running this function is stored for recoverability.
// *needRecoverPtr: whether recovering is needed
// *prevConfigPtr: data stored in ConfigMap/cluster-monitoring-config before running this function
func exposeMetrics(oc *exutil.CLI, testDataDir string, needRecoverPtr *bool, prevConfigPtr *string) {
	// Look for cluster-level monitoring configuration
	getOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ConfigMap", "cluster-monitoring-config", "-n", "openshift-monitoring", "--ignore-not-found").Output()
	o.Expect(err).NotTo(o.HaveOccurred())

	// Enable user workload monitoring
	if len(getOutput) > 0 {
		e2e.Logf("ConfigMap cluster-monitoring-config exists, extracting cluster-monitoring-config ...")
		extractOutput, _, _ := oc.AsAdmin().WithoutNamespace().Run("extract").Args("ConfigMap/cluster-monitoring-config", "-n", "openshift-monitoring", "--to=-").Outputs()

		if strings.Contains(strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(extractOutput, "'", ""), "\"", ""), " ", ""), "enableUserWorkload:true") {
			e2e.Logf("User workload is enabled, doing nothing ... ")
			*needRecoverPtr, *prevConfigPtr = false, ""
		} else {
			e2e.Logf("User workload is not enabled, enabling ...")
			*needRecoverPtr, *prevConfigPtr = true, extractOutput

			extractOutputParts := strings.Split(extractOutput, "\n")
			containKeyword := false
			for idx, part := range extractOutputParts {
				if strings.Contains(part, "enableUserWorkload") {
					e2e.Logf("Keyword \"enableUserWorkload\" found in cluster-monitoring-config, setting enableUserWorkload to true ...")
					extractOutputParts[idx] = "enableUserWorkload: true"
					containKeyword = true
					break
				}
			}
			if !containKeyword {
				e2e.Logf("Keyword \"enableUserWorkload\" not found in cluster-monitoring-config, adding ...")
				extractOutputParts = append(extractOutputParts, "enableUserWorkload: true")
			}
			modifiedExtractOutput := strings.ReplaceAll(strings.Join(extractOutputParts, "\\n"), "\"", "\\\"")

			e2e.Logf("Patching ConfigMap cluster-monitoring-config to enable user workload monitoring ...")
			err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("ConfigMap", "cluster-monitoring-config", "-n", "openshift-monitoring", "--type", "merge", "-p", fmt.Sprintf("{\"data\":{\"config.yaml\": \"%s\"}}", modifiedExtractOutput)).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}
	} else {
		e2e.Logf("ConfigMap cluster-monitoring-config does not exist, creating ...")
		*needRecoverPtr, *prevConfigPtr = true, ""

		clusterMonitoringConfigTemp := clusterMonitoringConfig{
			enableUserWorkload: true,
			namespace:          "openshift-monitoring",
			template:           filepath.Join(testDataDir, "cluster-monitoring-config.yaml"),
		}
		clusterMonitoringConfigTemp.create(oc)
	}

	// Check monitoring-related pods are created in the openshift-user-workload-monitoring namespace
	newCheck("expect", "get", asAdmin, withoutNamespace, contain, "prometheus-operator", ok, DefaultTimeout, []string{"pod", "-n", "openshift-user-workload-monitoring"}).check(oc)
	newCheck("expect", "get", asAdmin, withoutNamespace, contain, "prometheus-user-workload", ok, DefaultTimeout, []string{"pod", "-n", "openshift-user-workload-monitoring"}).check(oc)
	newCheck("expect", "get", asAdmin, withoutNamespace, contain, "thanos-ruler-user-workload", ok, DefaultTimeout, []string{"pod", "-n", "openshift-user-workload-monitoring"}).check(oc)

	// Check if ServiceMonitors and PodMonitors are created
	e2e.Logf("Checking if ServiceMonitors and PodMonitors exist ...")
	getOutput, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("ServiceMonitor", "hive-clustersync", "-n", HiveNamespace, "--ignore-not-found").Output()
	o.Expect(err).NotTo(o.HaveOccurred())

	if len(getOutput) == 0 {
		e2e.Logf("Creating PodMonitor for hive-operator ...")
		podMonitorYaml := filepath.Join(testDataDir, "hive-operator-podmonitor.yaml")
		err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", podMonitorYaml).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		e2e.Logf("Creating ServiceMonitor for hive-controllers ...")
		serviceMonitorControllers := filepath.Join(testDataDir, "hive-controllers-servicemonitor.yaml")
		err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", serviceMonitorControllers).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		e2e.Logf("Creating ServiceMonitor for hive-clustersync ...")
		serviceMonitorClustersync := filepath.Join(testDataDir, "hive-clustersync-servicemonitor.yaml")
		err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", serviceMonitorClustersync).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	}
}

// Recover cluster monitoring state, neutralizing the effect of exposeMetrics.
func recoverClusterMonitoring(oc *exutil.CLI, needRecoverPtr *bool, prevConfigPtr *string) {
	if *needRecoverPtr {
		e2e.Logf("Recovering cluster monitoring configurations ...")
		if len(*prevConfigPtr) == 0 {
			e2e.Logf("ConfigMap/cluster-monitoring-config did not exist before calling exposeMetrics, deleting ...")
			err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("ConfigMap", "cluster-monitoring-config", "-n", "openshift-monitoring", "--ignore-not-found").Execute()
			if err != nil {
				e2e.Logf("Error occurred when deleting ConfigMap/cluster-monitoring-config: %v", err)
			}
		} else {
			e2e.Logf("Reverting changes made to ConfigMap/cluster-monitoring-config ...")
			*prevConfigPtr = strings.ReplaceAll(strings.ReplaceAll(*prevConfigPtr, "\n", "\\n"), "\"", "\\\"")
			err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("ConfigMap", "cluster-monitoring-config", "-n", "openshift-monitoring", "--type", "merge", "-p", fmt.Sprintf("{\"data\":{\"config.yaml\": \"%s\"}}", *prevConfigPtr)).Execute()
			if err != nil {
				e2e.Logf("Error occurred when patching ConfigMap/cluster-monitoring-config: %v", err)
			}
		}

		e2e.Logf("Deleting ServiceMonitors and PodMonitors in the hive namespace ...")
		err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("ServiceMonitor", "hive-clustersync", "-n", HiveNamespace, "--ignore-not-found").Execute()
		if err != nil {
			e2e.Logf("Error occurred when deleting ServiceMonitor/hive-clustersync: %v", err)
		}
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("ServiceMonitor", "hive-controllers", "-n", HiveNamespace, "--ignore-not-found").Execute()
		if err != nil {
			e2e.Logf("Error occurred when deleting ServiceMonitor/hive-controllers: %v", err)
		}
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("PodMonitor", "hive-operator", "-n", HiveNamespace, "--ignore-not-found").Execute()
		if err != nil {
			e2e.Logf("Error occurred when deleting PodMonitor/hive-operator: %v", err)
		}

		return
	}

	e2e.Logf("No recovering needed for cluster monitoring configurations. ")
}

// If enable hive exportMetric
func exportMetric(oc *exutil.CLI, action bool) {
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("HiveConfig", "hive", "-o=jsonpath={.spec.exportMetrics}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if action {
		if strings.Contains(output, "true") {
			e2e.Logf("The exportMetrics has been enabled in hiveconfig, won't change")
		} else {
			e2e.Logf("Enable hive exportMetric in Hiveconfig.")
			newCheck("expect", "patch", asAdmin, withoutNamespace, contain, "patched", ok, DefaultTimeout, []string{"HiveConfig", "hive", "--type", "merge", "-p", `{"spec":{"exportMetrics": true}}`}).check(oc)
			hiveNS, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("Hiveconfig", "hive", "-o=jsonpath={.spec.targetNamespace}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(hiveNS).NotTo(o.BeEmpty())
			newCheck("expect", "get", asAdmin, withoutNamespace, contain, "prometheus-k8s", ok, DefaultTimeout, []string{"role", "-n", hiveNS}).check(oc)
			newCheck("expect", "get", asAdmin, withoutNamespace, contain, "prometheus-k8s", ok, DefaultTimeout, []string{"rolebinding", "-n", hiveNS}).check(oc)
			newCheck("expect", "get", asAdmin, withoutNamespace, contain, "hive-clustersync", ok, DefaultTimeout, []string{"servicemonitor", "-n", hiveNS, "-o=name"}).check(oc)
			newCheck("expect", "get", asAdmin, withoutNamespace, contain, "hive-controllers", ok, DefaultTimeout, []string{"servicemonitor", "-n", hiveNS, "-o=name"}).check(oc)
		}
	}
	if !action {
		if !strings.Contains(output, "true") {
			e2e.Logf("The exportMetrics has been disabled in hiveconfig, won't change")
		} else {
			e2e.Logf("Disable hive exportMetric in Hiveconfig.")
			newCheck("expect", "patch", asAdmin, withoutNamespace, contain, "patched", ok, DefaultTimeout, []string{"HiveConfig", "hive", "--type", "merge", "-p", `{"spec":{"exportMetrics": false}}`}).check(oc)
			hiveNS, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("Hiveconfig", "hive", "-o=jsonpath={.spec.targetNamespace}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(hiveNS).NotTo(o.BeEmpty())
			newCheck("expect", "get", asAdmin, withoutNamespace, contain, "prometheus-k8s", nok, DefaultTimeout, []string{"role", "-n", hiveNS}).check(oc)
			newCheck("expect", "get", asAdmin, withoutNamespace, contain, "prometheus-k8s", nok, DefaultTimeout, []string{"rolebinding", "-n", hiveNS}).check(oc)
			newCheck("expect", "get", asAdmin, withoutNamespace, contain, "hive-clustersync", nok, DefaultTimeout, []string{"servicemonitor", "-n", hiveNS, "-o=name"}).check(oc)
			newCheck("expect", "get", asAdmin, withoutNamespace, contain, "hive-controllers", nok, DefaultTimeout, []string{"servicemonitor", "-n", hiveNS, "-o=name"}).check(oc)
		}
	}

}

func doPrometheusQuery(oc *exutil.CLI, token string, url string, query string) prometheusQueryResult {
	var data prometheusQueryResult
	msg, _, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args(
		"-n", "openshift-monitoring", "-c", "prometheus", "prometheus-k8s-0", "-i", "--",
		"curl", "-k", "-H", fmt.Sprintf("Authorization: Bearer %v", token),
		fmt.Sprintf("%s%s", url, query)).Outputs()
	if err != nil {
		e2e.Failf("Failed Prometheus query, error: %v", err)
	}
	o.Expect(msg).NotTo(o.BeEmpty())
	json.Unmarshal([]byte(msg), &data)
	return data
}

// parameter expect: ok, expected to have expectContent; nok, not expected to have expectContent
func checkMetricExist(oc *exutil.CLI, expect bool, token string, url string, query []string) {
	for _, v := range query {
		e2e.Logf("Check metric %s", v)
		err := wait.Poll(1*time.Minute, (ClusterResumeTimeout/60)*time.Minute, func() (bool, error) {
			data := doPrometheusQuery(oc, token, url, v)
			if expect && len(data.Data.Result) > 0 {
				e2e.Logf("Metric %s exist, expected", v)
				return true, nil
			}
			if !expect && len(data.Data.Result) == 0 {
				e2e.Logf("Metric %s doesn't exist, expected", v)
				return true, nil
			}
			return false, nil

		})
		exutil.AssertWaitPollNoErr(err, "\"checkMetricExist\" fail, can not get expected result")
	}

}

func checkResourcesMetricValue(oc *exutil.CLI, resourceName, resourceNamespace string, expectedResult string, token string, url string, query string) {
	err := wait.Poll(1*time.Minute, (ClusterResumeTimeout/60)*time.Minute, func() (bool, error) {
		data := doPrometheusQuery(oc, token, url, query)
		for _, v := range data.Data.Result {
			switch query {
			case "hive_clusterclaim_assignment_delay_seconds_count", "hive_clusterpool_stale_clusterdeployments_deleted":
				if v.Metric.ClusterpoolName == resourceName && v.Metric.ClusterpoolNamespace == resourceNamespace {
					e2e.Logf("Found metric for pool %s in namespace %s", resourceName, resourceNamespace)
					if v.Value[1].(string) == expectedResult {
						e2e.Logf("The metric Value %s matches expected %s", v.Value[1].(string), expectedResult)
						return true, nil
					}
					e2e.Logf("The metric Value %s didn't match expected %s, try next round", v.Value[1].(string), expectedResult)
					return false, nil
				}
			case "hive_cluster_deployment_provision_underway_install_restarts":
				if v.Metric.ClusterDeployment == resourceName && v.Metric.ExportedNamespace == resourceNamespace {
					e2e.Logf("Found metric for ClusterDeployment %s in namespace %s", resourceName, resourceNamespace)
					if v.Value[1].(string) == expectedResult {
						e2e.Logf("The metric Value %s matches expected %s", v.Value[1].(string), expectedResult)
						return true, nil
					}
					e2e.Logf("The metric Value %s didn't match expected %s, try next round", v.Value[1].(string), expectedResult)
					return false, nil
				}
			case "hive_cluster_deployment_install_success_total_count":
				if v.Metric.Region == resourceName && v.Metric.Namespace == resourceNamespace {
					if data.Data.Result[0].Metric.InstallAttempt == expectedResult {
						e2e.Logf("The region %s has %s install attempts", v.Metric.Region, data.Data.Result[0].Metric.InstallAttempt)
						return true, nil
					}
					e2e.Logf("The metric InstallAttempt lable %s didn't match expected %s, try next round", data.Data.Result[0].Metric.InstallAttempt, expectedResult)
					return false, nil
				}
			case "hive_cluster_deployment_install_failure_total_count":
				if v.Metric.Region == resourceName && v.Metric.Namespace == resourceNamespace {
					if data.Data.Result[2].Metric.InstallAttempt == expectedResult {
						e2e.Logf("The region %s has %s install attempts", v.Metric.Region, data.Data.Result[2].Metric.InstallAttempt)
						return true, nil
					}
					e2e.Logf("The metric InstallAttempt lable %s didn't match expected %s, try next round", data.Data.Result[2].Metric.InstallAttempt, expectedResult)
					return false, nil
				}
			}
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "\"checkResourcesMetricValue\" fail, can not get expected result")
}

func checkHiveConfigMetric(oc *exutil.CLI, field string, expectedResult string, token string, url string, query string) {
	err := wait.Poll(1*time.Minute, (ClusterResumeTimeout/60)*time.Minute, func() (bool, error) {
		data := doPrometheusQuery(oc, token, url, query)
		switch field {
		case "condition":
			if data.Data.Result[0].Metric.Condition == expectedResult {
				e2e.Logf("the Metric %s field \"%s\" matched the expected result \"%s\"", query, field, expectedResult)
				return true, nil
			}
		case "reason":
			if data.Data.Result[0].Metric.Reason == expectedResult {
				e2e.Logf("the Metric %s field \"%s\" matched the expected result \"%s\"", query, field, expectedResult)
				return true, nil
			}
		default:
			e2e.Logf("the Metric %s doesn't contain field %s", query, field)
			return false, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "\"checkHiveConfigMetric\" fail, can not get expected result")
}

func createCD(testDataDir string, testOCPImage string, oc *exutil.CLI, ns string, installConfigSecret interface{}, cd interface{}) {
	switch x := cd.(type) {
	case clusterDeployment:
		g.By("Create AWS ClusterDeployment..." + ns)
		imageSet := clusterImageSet{
			name:         x.name + "-imageset",
			releaseImage: testOCPImage,
			template:     filepath.Join(testDataDir, "clusterimageset.yaml"),
		}
		g.By("Create ClusterImageSet...")
		imageSet.create(oc)
		//secrets can be accessed by pod in the same namespace, so copy pull-secret and aws-creds to target namespace for the pool
		g.By("Copy AWS platform credentials...")
		createAWSCreds(oc, ns)
		g.By("Copy pull-secret...")
		createPullSecret(oc, ns)
		g.By("Create AWS Install-Config Secret...")
		switch ic := installConfigSecret.(type) {
		case installConfig:
			ic.create(oc)
		default:
			g.Fail("Please provide correct install-config type")
		}
		x.create(oc)
	case gcpClusterDeployment:
		g.By("Create gcp ClusterDeployment..." + ns)
		imageSet := clusterImageSet{
			name:         x.name + "-imageset",
			releaseImage: testOCPImage,
			template:     filepath.Join(testDataDir, "clusterimageset.yaml"),
		}
		g.By("Create ClusterImageSet...")
		imageSet.create(oc)
		//secrets can be accessed by pod in the same namespace, so copy pull-secret and aws-creds to target namespace for the pool
		g.By("Copy GCP platform credentials...")
		createGCPCreds(oc, ns)
		g.By("Copy pull-secret...")
		createPullSecret(oc, ns)
		g.By("Create GCP Install-Config Secret...")
		switch ic := installConfigSecret.(type) {
		case gcpInstallConfig:
			ic.create(oc)
		default:
			g.Fail("Please provide correct install-config type")
		}
		x.create(oc)
	case azureClusterDeployment:
		g.By("Create azure ClusterDeployment..." + ns)
		imageSet := clusterImageSet{
			name:         x.name + "-imageset",
			releaseImage: testOCPImage,
			template:     filepath.Join(testDataDir, "clusterimageset.yaml"),
		}
		g.By("Create ClusterImageSet...")
		imageSet.create(oc)
		//secrets can be accessed by pod in the same namespace, so copy pull-secret and aws-creds to target namespace for the pool
		g.By("Copy Azure platform credentials...")
		createAzureCreds(oc, ns)
		g.By("Copy pull-secret...")
		createPullSecret(oc, ns)
		g.By("Create Azure Install-Config Secret...")
		switch ic := installConfigSecret.(type) {
		case azureInstallConfig:
			ic.create(oc)
		default:
			g.Fail("Please provide correct install-config type")
		}
		x.create(oc)
	default:
		g.By("unknown ClusterDeployment type")
	}
}

func cleanCD(oc *exutil.CLI, clusterImageSetName string, ns string, secretName string, cdName string) {
	defer cleanupObjects(oc, objectTableRef{"ClusterImageSet", "", clusterImageSetName})
	defer cleanupObjects(oc, objectTableRef{"secret", ns, secretName})
	defer cleanupObjects(oc, objectTableRef{"ClusterDeployment", ns, cdName})
}

// Install Hive Operator if not
func installHiveOperator(oc *exutil.CLI, ns *hiveNameSpace, og *operatorGroup, sub *subscription, hc *hiveconfig, testDataDir string) {
	nsTemp := filepath.Join(testDataDir, "namespace.yaml")
	ogTemp := filepath.Join(testDataDir, "operatorgroup.yaml")
	subTemp := filepath.Join(testDataDir, "subscription.yaml")
	hcTemp := filepath.Join(testDataDir, "hiveconfig.yaml")

	*ns = hiveNameSpace{
		name:     HiveNamespace,
		template: nsTemp,
	}

	*og = operatorGroup{
		name:      "hive-og",
		namespace: HiveNamespace,
		template:  ogTemp,
	}

	*sub = subscription{
		name:            "hive-sub",
		namespace:       HiveNamespace,
		channel:         "alpha",
		approval:        "Automatic",
		operatorName:    "hive-operator",
		sourceName:      "community-operators",
		sourceNamespace: "openshift-marketplace",
		startingCSV:     "",
		currentCSV:      "",
		installedCSV:    "",
		template:        subTemp,
	}

	*hc = hiveconfig{
		logLevel:        "debug",
		targetNamespace: HiveNamespace,
		template:        hcTemp,
	}
	//Create Hive Resources if not exist
	ns.createIfNotExist(oc)
	og.createIfNotExist(oc)
	sub.createIfNotExist(oc)
	hc.createIfNotExist(oc)
}

// Get hiveadmission pod name
func getHiveadmissionPod(oc *exutil.CLI, namespace string) string {
	hiveadmissionOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "--selector=app=hiveadmission", "-n", namespace, "-o=jsonpath={.items[*].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	podArray := strings.Split(strings.TrimSpace(hiveadmissionOutput), " ")
	o.Expect(len(podArray)).To(o.BeNumerically(">", 0))
	e2e.Logf("Hiveadmission pod list is %s,first pod name is %s", podArray, podArray[0])
	return podArray[0]
}

// Get OCP Image for Hive testing, default is 4.13-nightly image for now and if not exist, fail the test
func getTestOCPImage() string {
	//get the latest 4.13-nightly image for Hive testing
	testOCPImage, err := exutil.GetLatestNightlyImage("4.13")
	o.Expect(err).NotTo(o.HaveOccurred())
	if testOCPImage == "" {
		e2e.Fail("Can't get the latest 4.13-nightly image")
	}
	return testOCPImage
}

func getCondition(oc *exutil.CLI, kind, resourceName, namespace, conditionType string) map[string]string {
	e2e.Logf("Extracting the %v condition from %v/%v in namespace %v", conditionType, kind, resourceName, namespace)
	stdout, _, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(kind, resourceName, "-n", namespace, fmt.Sprintf("-o=jsonpath={.status.conditions[?(@.type==\"%s\")]}", conditionType)).Outputs()
	o.Expect(err).NotTo(o.HaveOccurred())

	var condition map[string]string
	err = json.Unmarshal([]byte(stdout), &condition)
	o.Expect(err).NotTo(o.HaveOccurred())

	return condition
}
