package clusterinfrastructure

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type clusterDescription struct {
	name      string
	namespace string
	kind      string
	template  string
}

type clusterDescriptionNotInCapi struct {
	name      string
	namespace string
	kind      string
	template  string
}

type awsMachineTemplateDescription struct {
	name                    string
	namespace               string
	profile                 string
	instanceType            string
	zone                    string
	ami                     string
	subnetName              string
	subnetID                string
	sgName                  string
	template                string
	placementGroupName      string
	placementGroupPartition int
}

type gcpMachineTemplateDescription struct {
	name           string
	namespace      string
	region         string
	image          string
	machineType    string
	clusterID      string
	subnetwork     string
	serviceAccount string
	template       string
}

type capiMachineSetAWSDescription struct {
	name                string
	namespace           string
	clusterName         string
	kind                string
	replicas            int
	machineTemplateName string
	template            string
}

type capiMachineSetgcpDescription struct {
	name                string
	namespace           string
	clusterName         string
	kind                string
	replicas            int
	machineTemplateName string
	template            string
	failureDomain       string
}

type capiMachineSetvsphereDescription struct {
	name                string
	namespace           string
	clusterName         string
	kind                string
	replicas            int
	machineTemplateName string
	template            string
	dataSecretName      string
}

type vsphereMachineTemplateDescription struct {
	kind            string
	name            string
	namespace       string
	server          string
	diskGiB         int
	datacenter      string
	datastore       string
	folder          string
	resourcePool    string
	numCPUs         int
	memoryMiB       int
	dhcp            bool
	networkName     string
	template        string
	cloneMode       string
	machineTemplate string
}

// skipForCAPINotExist skip the test if capi doesn't exist
func skipForCAPINotExist(oc *exutil.CLI) {
	capi, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deploy", "-n", clusterAPINamespace, "-o=jsonpath={.items[*].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if err != nil || len(capi) == 0 {
		g.Skip("Skip for cluster api is not deployed!")
	}
}

func (cluster *clusterDescription) createCluster(oc *exutil.CLI) {
	e2e.Logf("Creating cluster ...")
	err := applyResourceFromTemplate(oc, "-f", cluster.template, "-p", "NAME="+cluster.name, "NAMESPACE="+clusterAPINamespace, "KIND="+cluster.kind)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (clusterNotInCapi *clusterDescriptionNotInCapi) createClusterNotInCapiNamespace(oc *exutil.CLI) {
	e2e.Logf("Creating cluster in namepsace not openshift-cluster-api ...")
	err := applyResourceFromTemplate(oc, "-f", clusterNotInCapi.template, "-p", "NAME="+clusterNotInCapi.name, "NAMESPACE="+clusterNotInCapi.namespace, "KIND="+clusterNotInCapi.kind)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (awsMachineTemplate *awsMachineTemplateDescription) createAWSMachineTemplate(oc *exutil.CLI) {
	e2e.Logf("Creating awsMachineTemplate ...")
	if awsMachineTemplate.placementGroupPartition != 0 {
		err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", awsMachineTemplate.template, "-p", "NAME="+awsMachineTemplate.name, "NAMESPACE="+clusterAPINamespace, "PROFILE="+awsMachineTemplate.profile, "INSTANCETYPE="+awsMachineTemplate.instanceType, "ZONE="+awsMachineTemplate.zone, "AMI="+awsMachineTemplate.ami, "SUBNETNAME="+awsMachineTemplate.subnetName, "SUBNETID="+awsMachineTemplate.subnetID, "SGNAME="+awsMachineTemplate.sgName, "PLACEMENTGROUPNAME="+awsMachineTemplate.placementGroupName, "PLACEMENTGROUPPARTITION="+strconv.Itoa(awsMachineTemplate.placementGroupPartition))
		o.Expect(err).NotTo(o.HaveOccurred())
	} else {
		err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", awsMachineTemplate.template, "-p", "NAME="+awsMachineTemplate.name, "NAMESPACE="+clusterAPINamespace, "PROFILE="+awsMachineTemplate.profile, "INSTANCETYPE="+awsMachineTemplate.instanceType, "ZONE="+awsMachineTemplate.zone, "AMI="+awsMachineTemplate.ami, "SUBNETNAME="+awsMachineTemplate.subnetName, "SUBNETID="+awsMachineTemplate.subnetID, "SGNAME="+awsMachineTemplate.sgName, "PLACEMENTGROUPNAME="+awsMachineTemplate.placementGroupName, "PLACEMENTGROUPPARTITION=null")
		o.Expect(err).NotTo(o.HaveOccurred())
	}
}

func (awsMachineTemplate *awsMachineTemplateDescription) deleteAWSMachineTemplate(oc *exutil.CLI) error {
	e2e.Logf("Deleting awsMachineTemplate ...")
	return oc.AsAdmin().WithoutNamespace().Run("delete").Args("awsmachinetemplate", awsMachineTemplate.name, "-n", clusterAPINamespace).Execute()
}

func (gcpMachineTemplate *gcpMachineTemplateDescription) createGCPMachineTemplate(oc *exutil.CLI) {
	e2e.Logf("Creating gcpMachineTemplate ...")
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", gcpMachineTemplate.template, "-p", "NAME="+gcpMachineTemplate.name, "NAMESPACE="+clusterAPINamespace, "IMAGE="+gcpMachineTemplate.image, "REGION="+gcpMachineTemplate.region, "CLUSTERID="+gcpMachineTemplate.clusterID, "MACHINETYPE="+gcpMachineTemplate.machineType, "SUBNETWORK="+gcpMachineTemplate.subnetwork, "SERVICEACCOUNT="+gcpMachineTemplate.serviceAccount)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (gcpMachineTemplatepdbal *gcpMachineTemplateDescription) createGCPMachineTemplatePdBal(oc *exutil.CLI) {
	e2e.Logf("Creating gcpMachineTemplate with pd balanced disk...")
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", gcpMachineTemplatepdbal.template, "-p", "NAME="+gcpMachineTemplatepdbal.name, "NAMESPACE="+clusterAPINamespace, "IMAGE="+gcpMachineTemplatepdbal.image, "REGION="+gcpMachineTemplatepdbal.region, "CLUSTERID="+gcpMachineTemplatepdbal.clusterID, "MACHINETYPE="+gcpMachineTemplatepdbal.machineType, "SUBNETWORK="+gcpMachineTemplatepdbal.subnetwork, "SERVICEACCOUNT="+gcpMachineTemplatepdbal.serviceAccount)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (gcpMachineTemplate *gcpMachineTemplateDescription) deleteGCPMachineTemplate(oc *exutil.CLI) error {
	e2e.Logf("Deleting gcpMachineTemplate ...")
	return oc.AsAdmin().WithoutNamespace().Run("delete").Args("gcpmachinetemplate", gcpMachineTemplate.name, "-n", clusterAPINamespace).Execute()
}

func (capiMachineSetAWS *capiMachineSetAWSDescription) createCapiMachineSet(oc *exutil.CLI) {
	e2e.Logf("Creating awsMachineSet ...")
	if err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", capiMachineSetAWS.template, "-p", "NAME="+capiMachineSetAWS.name, "NAMESPACE="+clusterAPINamespace, "CLUSTERNAME="+capiMachineSetAWS.clusterName, "MACHINETEMPLATENAME="+capiMachineSetAWS.machineTemplateName, "KIND="+capiMachineSetAWS.kind, "REPLICAS="+strconv.Itoa(capiMachineSetAWS.replicas)); err != nil {
		capiMachineSetAWS.deleteCapiMachineSet(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
	} else {
		waitForCapiMachinesRunning(oc, capiMachineSetAWS.replicas, capiMachineSetAWS.name)
	}
}

func (capiMachineSetAWS *capiMachineSetAWSDescription) deleteCapiMachineSet(oc *exutil.CLI) error {
	e2e.Logf("Deleting awsMachineSet ...")
	return oc.AsAdmin().WithoutNamespace().Run("delete").Args(capiMachineset, capiMachineSetAWS.name, "-n", clusterAPINamespace).Execute()
}

func (capiMachineSetgcp *capiMachineSetgcpDescription) createCapiMachineSetgcp(oc *exutil.CLI) {
	e2e.Logf("Creating gcpMachineSet ...")
	if err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", capiMachineSetgcp.template, "-p", "NAME="+capiMachineSetgcp.name, "NAMESPACE="+clusterAPINamespace, "CLUSTERNAME="+capiMachineSetgcp.clusterName, "MACHINETEMPLATENAME="+capiMachineSetgcp.machineTemplateName, "KIND="+capiMachineSetgcp.kind, "FAILUREDOMAIN="+capiMachineSetgcp.failureDomain, "REPLICAS="+strconv.Itoa(capiMachineSetgcp.replicas)); err != nil {
		capiMachineSetgcp.deleteCapiMachineSetgcp(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
	} else {
		waitForCapiMachinesRunning(oc, capiMachineSetgcp.replicas, capiMachineSetgcp.name)
	}
}

func (capiMachineSetgcp *capiMachineSetgcpDescription) deleteCapiMachineSetgcp(oc *exutil.CLI) error {
	e2e.Logf("Deleting gcpMachineSet ...")
	return oc.AsAdmin().WithoutNamespace().Run("delete").Args(capiMachineset, capiMachineSetgcp.name, "-n", clusterAPINamespace).Execute()
}

func (vsphereMachineTemplate *vsphereMachineTemplateDescription) createvsphereMachineTemplate(oc *exutil.CLI) {
	e2e.Logf("Creating vsphereMachineTemplate ...")
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", vsphereMachineTemplate.template, "-p", "KIND="+vsphereMachineTemplate.kind, "NAME="+vsphereMachineTemplate.name, "NAMESPACE="+clusterAPINamespace, "VSPHERE_SERVER="+vsphereMachineTemplate.server, "DISKGIB="+strconv.Itoa(vsphereMachineTemplate.diskGiB), "CLONEMODE="+"linkedClone", "DATASTORE="+vsphereMachineTemplate.datastore, "DATACENTER="+vsphereMachineTemplate.datacenter, "FOLDER="+vsphereMachineTemplate.folder, "RESOURCEPOOL="+vsphereMachineTemplate.resourcePool, "NUMCPUS="+strconv.Itoa(vsphereMachineTemplate.numCPUs), "MEMORYMIB="+strconv.Itoa(vsphereMachineTemplate.memoryMiB), "NETWORKNAME="+vsphereMachineTemplate.networkName, "MACHINETEMPLATE="+vsphereMachineTemplate.machineTemplate)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (vsphereMachineTemplate *vsphereMachineTemplateDescription) deletevsphereMachineTemplate(oc *exutil.CLI) error {
	e2e.Logf("Deleting vsphereMachineTemplate ...")
	return oc.AsAdmin().WithoutNamespace().Run("delete").Args("VSpheremachinetemplate", vsphereMachineTemplate.name, "-n", clusterAPINamespace).Execute()
}
func (capiMachineSetvsphere *capiMachineSetvsphereDescription) createCapiMachineSetvsphere(oc *exutil.CLI) {
	e2e.Logf("Creating vsphereMachineSet ...")
	if err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", capiMachineSetvsphere.template, "-p", "NAME="+capiMachineSetvsphere.name, "NAMESPACE="+clusterAPINamespace, "CLUSTERNAME="+capiMachineSetvsphere.clusterName, "MACHINETEMPLATENAME="+capiMachineSetvsphere.machineTemplateName, "KIND="+capiMachineSetvsphere.kind, "DATASECRET="+capiMachineSetvsphere.dataSecretName, "REPLICAS="+strconv.Itoa(capiMachineSetvsphere.replicas)); err != nil {
		capiMachineSetvsphere.deleteCapiMachineSetvsphere(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
	} else {
		waitForCapiMachinesRunning(oc, capiMachineSetvsphere.replicas, capiMachineSetvsphere.name)
	}
}

func (capiMachineSetvsphere *capiMachineSetvsphereDescription) deleteCapiMachineSetvsphere(oc *exutil.CLI) error {
	e2e.Logf("Deleting vsphereMachineSet ...")
	return oc.AsAdmin().WithoutNamespace().Run("delete").Args(capiMachineset, capiMachineSetvsphere.name, "-n", clusterAPINamespace).Execute()
}

// waitForCapiMachinesRunning check if all the machines are Running in a MachineSet
func waitForCapiMachinesRunning(oc *exutil.CLI, machineNumber int, machineSetName string) {
	e2e.Logf("Waiting for the machines Running ...")
	if machineNumber >= 1 {
		// Wait 180 seconds first, as it uses total 1200 seconds in wait.poll, it may not be enough for some platform(s)
		time.Sleep(180 * time.Second)
	}
	pollErr := wait.Poll(60*time.Second, 1200*time.Second, func() (bool, error) {
		msg, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args(capiMachineset, machineSetName, "-o=jsonpath={.status.readyReplicas}", "-n", clusterAPINamespace).Output()
		machinesRunning, _ := strconv.Atoi(msg)
		if machinesRunning != machineNumber {
			e2e.Logf("Expected %v  machine are not Running yet and waiting up to 1 minutes ...", machineNumber)
			return false, nil
		}
		e2e.Logf("Expected %v  machines are Running", machineNumber)
		return true, nil
	})
	exutil.AssertWaitPollNoErr(pollErr, fmt.Sprintf("Expected %v  machines are not Running after waiting up to 20 minutes ...", machineNumber))
	e2e.Logf("All machines are Running ...")
	e2e.Logf("Check nodes haven't uninitialized taints...")
	for _, nodeName := range getNodeNamesFromCapiMachineSet(oc, machineSetName) {
		taints, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", nodeName, "-o=jsonpath={.spec.taints}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(taints).ShouldNot(o.ContainSubstring("uninitialized"))
	}
	e2e.Logf("All nodes haven't uninitialized taints ...")
}

// waitForCapiMachinesDisapper check if all the machines are Dissappered in a MachineSet
func waitForCapiMachinesDisapper(oc *exutil.CLI, machineSetName string) {
	e2e.Logf("Waiting for the machines Dissapper ...")
	err := wait.Poll(60*time.Second, 1200*time.Second, func() (bool, error) {
		machineNames, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args(capiMachine, "-o=jsonpath={.items[*].metadata.name}", "-l", "cluster.x-k8s.io/set-name="+machineSetName, "-n", clusterAPINamespace).Output()
		if machineNames != "" {
			e2e.Logf(" Still have machines are not Disappered yet and waiting up to 1 minutes ...")
			return false, nil
		}
		e2e.Logf("All machines are Disappered")
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, "Wait machine disappear failed.")
}

// waitForCapiMachinesDisappergcp check if all the machines are Dissappered in a MachineSet
func waitForCapiMachinesDisappergcp(oc *exutil.CLI, machineSetName string) {
	e2e.Logf("Waiting for the machines Dissapper ...")
	err := wait.Poll(60*time.Second, 1200*time.Second, func() (bool, error) {
		machineNames, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args(capiMachine, "-o=jsonpath={.items[*].metadata.name}", "-n", clusterAPINamespace).Output()
		if strings.Contains(machineNames, machineSetName) {
			e2e.Logf(" Still have machines are not Disappered yet and waiting up to 1 minutes ...")
			return false, nil
		}
		e2e.Logf("All machines are Disappered")
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, "Wait machine disappear failed.")
}

func matchProviderIDWithNode(oc *exutil.CLI, resourceType, resourceName, namespace string) (bool, error) {
	machineProviderID, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(resourceType, resourceName, "-o=jsonpath={.spec.providerID}", "-n", namespace).Output()
	o.Expect(err).NotTo(o.HaveOccurred())

	nodeName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(resourceType, resourceName, "-o=jsonpath={.status.nodeRef.name}", "-n", namespace).Output()
	o.Expect(err).NotTo(o.HaveOccurred())

	nodeProviderID, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", nodeName, "-o=jsonpath={.spec.providerID}", "-n", namespace).Output()
	o.Expect(err).NotTo(o.HaveOccurred())

	if string(machineProviderID) != string(nodeProviderID) {
		e2e.Logf("Node & machine provider ID not matched")
		return false, err
	}
	e2e.Logf("Node & machine provider ID matched")
	return true, nil
}

// getNodeNamesFromCapiMachineSet get all Nodes in a CAPI Machineset
func getNodeNamesFromCapiMachineSet(oc *exutil.CLI, machineSetName string) []string {
	e2e.Logf("Getting all Nodes in a CAPI Machineset ...")
	nodeNames, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(capiMachine, "-o=jsonpath={.items[*].status.nodeRef.name}", "-l", "cluster.x-k8s.io/set-name="+machineSetName, "-n", clusterAPINamespace).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if nodeNames == "" {
		return []string{}
	}
	return strings.Split(nodeNames, " ")
}
