package clusterinfrastructure

import (
	"strconv"
	"strings"

	o "github.com/onsi/gomega"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"github.com/openshift/openshift-tests-private/test/extended/util/architecture"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type ExpanderImplementation int

const (
	Random ExpanderImplementation = iota
	LeastWaste
	Priority
)

type clusterAutoscalerDescription struct {
	maxNode              int
	minCore              int
	maxCore              int
	minMemory            int
	maxMemory            int
	utilizationThreshold string
	template             string
	logVerbosity         int
	expander             ExpanderImplementation
}

type machineAutoscalerDescription struct {
	name           string
	namespace      string
	maxReplicas    int
	minReplicas    int
	template       string
	machineSetName string
}

type workLoadDescription struct {
	name      string
	namespace string
	template  string
	arch      architecture.Architecture
	cpu       string
	label     string
}

type priorityExpanderDescription struct {
	template  string
	namespace string
	p10       string
	p20       string
}

func (clusterAutoscaler *clusterAutoscalerDescription) createClusterAutoscaler(oc *exutil.CLI) {
	e2e.Logf("Creating clusterautoscaler ...")
	var err error
	if strings.Contains(clusterAutoscaler.template, "util") {
		err = applyResourceFromTemplate(oc, "-f", clusterAutoscaler.template, "-p", "MAXNODE="+strconv.Itoa(clusterAutoscaler.maxNode), "MINCORE="+strconv.Itoa(clusterAutoscaler.minCore), "MAXCORE="+strconv.Itoa(clusterAutoscaler.maxCore), "MINMEMORY="+strconv.Itoa(clusterAutoscaler.minMemory), "MAXMEMORY="+strconv.Itoa(clusterAutoscaler.maxMemory), "UTILIZATIONTHRESHOLD="+clusterAutoscaler.utilizationThreshold)
	} else if strings.Contains(clusterAutoscaler.template, "verbose") {
		err = applyResourceFromTemplate(oc, "-f", clusterAutoscaler.template, "-p", "MAXNODE="+strconv.Itoa(clusterAutoscaler.maxNode), "MINCORE="+strconv.Itoa(clusterAutoscaler.minCore), "MAXCORE="+strconv.Itoa(clusterAutoscaler.maxCore), "MINMEMORY="+strconv.Itoa(clusterAutoscaler.minMemory), "MAXMEMORY="+strconv.Itoa(clusterAutoscaler.maxMemory), "LOGVERBOSITY="+strconv.Itoa(clusterAutoscaler.logVerbosity))
	} else {
		err = applyResourceFromTemplate(oc, "-f", clusterAutoscaler.template, "-p", "MAXNODE="+strconv.Itoa(clusterAutoscaler.maxNode), "MINCORE="+strconv.Itoa(clusterAutoscaler.minCore), "MAXCORE="+strconv.Itoa(clusterAutoscaler.maxCore), "MINMEMORY="+strconv.Itoa(clusterAutoscaler.minMemory), "MAXMEMORY="+strconv.Itoa(clusterAutoscaler.maxMemory), "EXPANDER="+clusterAutoscaler.expander.String())
	}
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (clusterAutoscaler *clusterAutoscalerDescription) deleteClusterAutoscaler(oc *exutil.CLI) error {
	e2e.Logf("Deleting clusterautoscaler ...")
	return oc.AsAdmin().WithoutNamespace().Run("delete").Args("clusterautoscaler", "default").Execute()
}

func (machineAutoscaler *machineAutoscalerDescription) createMachineAutoscaler(oc *exutil.CLI) {
	e2e.Logf("Creating machineautoscaler ...")
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", machineAutoscaler.template, "-p", "NAME="+machineAutoscaler.name, "NAMESPACE="+machineAPINamespace, "MAXREPLICAS="+strconv.Itoa(machineAutoscaler.maxReplicas), "MINREPLICAS="+strconv.Itoa(machineAutoscaler.minReplicas), "MACHINESETNAME="+machineAutoscaler.machineSetName)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (machineAutoscaler *machineAutoscalerDescription) deleteMachineAutoscaler(oc *exutil.CLI) error {
	e2e.Logf("Deleting a machineautoscaler ...")
	return oc.AsAdmin().WithoutNamespace().Run("delete").Args("machineautoscaler", machineAutoscaler.name, "-n", machineAPINamespace).Execute()
}

func (workLoad *workLoadDescription) createWorkLoad(oc *exutil.CLI) {
	e2e.Logf("Creating workLoad ...")
	var err error
	if strings.Contains(workLoad.template, "affinity") {
		err = applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", workLoad.template, "-p", "NAME="+workLoad.name, "NAMESPACE="+workLoad.namespace, "ARCH="+workLoad.arch.String(), "CPU="+workLoad.cpu)
	} else if strings.Contains(workLoad.template, "label") {
		err = applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", workLoad.template, "-p", "NAME="+workLoad.name, "NAMESPACE="+workLoad.namespace, "LABEL="+workLoad.label)
	} else {
		err = applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", workLoad.template, "-p", "NAME="+workLoad.name, "NAMESPACE="+workLoad.namespace)
	}
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (workLoad *workLoadDescription) deleteWorkLoad(oc *exutil.CLI) error {
	e2e.Logf("Deleting workload ...")
	return oc.AsAdmin().WithoutNamespace().Run("delete").Args("job", workLoad.name, "-n", machineAPINamespace).Execute()
}

func getWorkLoadCPU(oc *exutil.CLI, machineSetName string) string {
	e2e.Logf("Setting workload CPU ...")
	cpuAnnotation, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, machineSetName, "-n", machineAPINamespace, "-o=jsonpath={.metadata.annotations.machine\\.openshift\\.io\\/vCPU}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	cpuFloat, err := strconv.ParseFloat(cpuAnnotation, 64)
	o.Expect(err).NotTo(o.HaveOccurred())
	cpu := cpuFloat * 1000 * 0.6
	return strconv.FormatFloat(cpu, 'f', -1, 64) + "m"
}

func (priorityExpander *priorityExpanderDescription) createPriorityExpander(oc *exutil.CLI) {
	e2e.Logf("Creating clusterAutoscalerPriorityExpander ...")
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", priorityExpander.template, "NAMESPACE="+priorityExpander.namespace, "-p", "P10="+priorityExpander.p10, "P20="+priorityExpander.p20)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (priorityExpander *priorityExpanderDescription) deletePriorityExpander(oc *exutil.CLI) error {
	e2e.Logf("Deleting clusterAutoscalerPriorityExpander ...")
	return oc.AsAdmin().WithoutNamespace().Run("delete").Args("cm", "cluster-autoscaler-priority-expander", "-n", machineAPINamespace).Execute()
}

// String returns the string value for the given Expander
func (a ExpanderImplementation) String() string {
	switch a {
	case Random:
		return "Random"
	case LeastWaste:
		return "LeastWaste"
	case Priority:
		return "Priority"
	default:
		e2e.Failf("Unknown expander %d", a)
	}
	return ""
}
