package clusterinfrastructure

import (
	"fmt"
	"strconv"
	"time"

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

type awsClusterDescription struct {
	name      string
	namespace string
	region    string
	template  string
}

type awsMachineTemplateDescription struct {
	name       string
	namespace  string
	profile    string
	zone       string
	ami        string
	subnetName string
	sgName     string
	template   string
}

type capiMachineSetDescription struct {
	name                string
	namespace           string
	clusterName         string
	kind                string
	replicas            int
	machineTemplateName string
	template            string
}

func (cluster *clusterDescription) createCluster(oc *exutil.CLI) {
	e2e.Logf("Creating cluster ...")
	err := applyResourceFromTemplate(oc, "-f", cluster.template, "-p", "NAME="+cluster.name, "NAMESPACE="+clusterAPINamespace, "KIND="+cluster.kind)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (cluster *clusterDescription) deleteCluster(oc *exutil.CLI) error {
	e2e.Logf("Deleting cluster ...")
	return oc.AsAdmin().WithoutNamespace().Run("delete").Args("cluster", cluster.name, "-n", clusterAPINamespace).Execute()
}

func (awsCluster *awsClusterDescription) createAWSCluster(oc *exutil.CLI) {
	e2e.Logf("Creating awsCluster ...")
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", awsCluster.template, "-p", "NAME="+awsCluster.name, "NAMESPACE="+clusterAPINamespace, "REGION="+awsCluster.region)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (awsCluster *awsClusterDescription) deleteAWSCluster(oc *exutil.CLI) error {
	e2e.Logf("Deleting a awsCluster ...")
	return oc.AsAdmin().WithoutNamespace().Run("delete").Args("awscluster", awsCluster.name, "-n", clusterAPINamespace).Execute()
}

func (awsMachineTemplate *awsMachineTemplateDescription) createAWSMachineTemplate(oc *exutil.CLI) {
	e2e.Logf("Creating awsMachineTemplate ...")
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", awsMachineTemplate.template, "-p", "NAME="+awsMachineTemplate.name, "NAMESPACE="+clusterAPINamespace, "PROFILE="+awsMachineTemplate.profile, "ZONE="+awsMachineTemplate.zone, "AMI="+awsMachineTemplate.ami, "SUBNETNAME="+awsMachineTemplate.subnetName, "SGNAME="+awsMachineTemplate.sgName)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (awsMachineTemplate *awsMachineTemplateDescription) deleteAWSMachineTemplate(oc *exutil.CLI) error {
	e2e.Logf("Deleting awsMachineTemplate ...")
	return oc.AsAdmin().WithoutNamespace().Run("delete").Args("awsmachinetemplate", awsMachineTemplate.name, "-n", clusterAPINamespace).Execute()
}

func (capiMachineSet *capiMachineSetDescription) createCapiMachineSet(oc *exutil.CLI) {
	e2e.Logf("Creating awsMachineSet ...")
	if err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", capiMachineSet.template, "-p", "NAME="+capiMachineSet.name, "NAMESPACE="+clusterAPINamespace, "CLUSTERNAME="+capiMachineSet.clusterName, "MACHINETEMPLATENAME="+capiMachineSet.machineTemplateName, "KIND="+capiMachineSet.kind, "REPLICAS="+strconv.Itoa(capiMachineSet.replicas)); err != nil {
		capiMachineSet.deleteCapiMachineSet(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
	} else {
		waitForCapiMachinesRunning(oc, capiMachineSet.replicas, capiMachineSet.name)
	}
}

func (capiMachineSet *capiMachineSetDescription) deleteCapiMachineSet(oc *exutil.CLI) error {
	e2e.Logf("Deleting awsMachineSet ...")
	return oc.AsAdmin().WithoutNamespace().Run("delete").Args(capiMachineset, capiMachineSet.name, "-n", clusterAPINamespace).Execute()
}

// waitForCapiMachinesRunning check if all the machines are Running in a MachineSet
func waitForCapiMachinesRunning(oc *exutil.CLI, machineNumber int, machineSetName string) {
	e2e.Logf("Waiting for the machines Running ...")
	pollErr := wait.Poll(60*time.Second, 720*time.Second, func() (bool, error) {
		msg, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args(capiMachineset, machineSetName, "-o=jsonpath={.status.readyReplicas}", "-n", clusterAPINamespace).Output()
		machinesRunning, _ := strconv.Atoi(msg)
		if machinesRunning != machineNumber {
			e2e.Logf("Expected %v  machine are not Running yet and waiting up to 1 minutes ...", machineNumber)
			return false, nil
		}
		e2e.Logf("Expected %v  machines are Running", machineNumber)
		return true, nil
	})
	exutil.AssertWaitPollNoErr(pollErr, fmt.Sprintf("Expected %v  machines are not Running after waiting up to 12 minutes ...", machineNumber))
	e2e.Logf("All machines are Running ...")
}
