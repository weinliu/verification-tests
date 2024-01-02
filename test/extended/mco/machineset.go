package mco

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	logger "github.com/openshift/openshift-tests-private/test/extended/util/logext"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	// MachineAPINamespace is the namespace where openshift machinesets are created
	MachineAPINamespace = "openshift-machine-api"
)

// MachineSet struct to handle MachineSet resources
type MachineSet struct {
	Resource
}

// MachineSetList struct to handle lists of MachineSet resources
type MachineSetList struct {
	ResourceList
}

// NewMachineSet constructs a new MachineSet struct
func NewMachineSet(oc *exutil.CLI, namespace, name string) *MachineSet {
	return &MachineSet{*NewNamespacedResource(oc, MachineSetFullName, namespace, name)}
}

// NewMachineSetList constructs a new MachineSetListist struct to handle all existing MachineSets
func NewMachineSetList(oc *exutil.CLI, namespace string) *MachineSetList {
	return &MachineSetList{*NewNamespacedResourceList(oc, MachineSetFullName, namespace)}
}

// String implements the Stringer interface
func (ms MachineSet) String() string {
	return ms.GetName()
}

// ScaleTo scales the MachineSet to the exact given value
func (ms MachineSet) ScaleTo(scale int) error {
	return ms.Patch("merge", fmt.Sprintf(`{"spec": {"replicas": %d}}`, scale))
}

// GetReplicaOfSpec return replica number of spec
func (ms MachineSet) GetReplicaOfSpec() string {
	return ms.GetOrFail(`{.spec.replicas}`)
}

// AddToScale scales the MachineSet adding the given value (positive or negative).
func (ms MachineSet) AddToScale(delta int) error {
	currentReplicas, err := strconv.Atoi(ms.GetOrFail(`{.spec.replicas}`))
	if err != nil {
		return err
	}

	return ms.ScaleTo(currentReplicas + delta)
}

// GetIsReady returns true the MachineSet instances are ready
func (ms MachineSet) GetIsReady() bool {
	configuredReplicasString, err := ms.Get(`{.spec.replicas}`)
	if err != nil {
		logger.Infof("Cannot get configured replicas. Err: %s", err)
		return false
	}

	configuredReplicas, err := strconv.Atoi(configuredReplicasString)
	if err != nil {
		logger.Infof("Could not parse configured replicas. Error: %s", err)
		return false
	}

	statusString, err := ms.Get(`{.status}`)
	if err != nil {
		logger.Infof("Cannot get status. Err: %s", err)
		return false
	}

	status := JSON(statusString)
	logger.Infof("status %s", status)
	replicasData := status.Get("replicas")
	readyReplicasData := status.Get("readyReplicas")

	if !replicasData.Exists() {
		logger.Infof("Replicasdata does not exist")
		return false
	}
	replicas := replicasData.ToInt()
	if replicas == 0 {
		if replicas == configuredReplicas {
			// We cant check the ready status when there is 0 replica configured.
			// So if status.replicas == spec.replicas == 0 then we consider that it is ok
			logger.Infof("Zero replicas")
			return true
		}
		logger.Infof("Zero replicas. Status not updated already.")
		return false
	}
	if !readyReplicasData.Exists() {
		logger.Infof("ReadyReplicasdata does not exist")
		return false
	}
	readyReplicas := readyReplicasData.ToInt()

	logger.Infof("Replicas %d, readyReplicas %d", replicas, readyReplicas)

	replicasAreReady := replicas == readyReplicas
	replicasAreConfigured := replicas == configuredReplicas

	return replicasAreReady && replicasAreConfigured
}

// GetMachines returns a slice with the machines created for this MachineSet
func (ms MachineSet) GetMachines() ([]Machine, error) {
	ml := NewMachineList(ms.oc, ms.GetNamespace())
	ml.ByLabel("machine.openshift.io/cluster-api-machineset=" + ms.GetName())
	ml.SortByTimestamp()
	return ml.GetAll()
}

// GetMachinesOrFail get machines from machineset or fail the test if any error occurred
func (ms MachineSet) GetMachinesOrFail() []Machine {
	ml, err := ms.GetMachines()
	o.Expect(err).NotTo(o.HaveOccurred(), "Get machines of machineset %s failed", ms.GetName())
	return ml
}

// GetMachinesByPhase get machine by phase e.g. Running, Provisioning, Provisioned, Deleting etc.
func (ms MachineSet) GetMachinesByPhase(phase string) ([]Machine, error) {
	// add poller to check machine phase periodically.
	machines := []Machine{}
	pollerr := wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 20*time.Second, true, func(ctx context.Context) (bool, error) {
		ml, err := ms.GetMachines()
		if err != nil {
			return false, err
		}
		for _, m := range ml {
			if m.GetPhase() == phase {
				machines = append(machines, m)
			}
		}
		return len(machines) > 0, nil
	})

	return machines, pollerr
}

// GetMachinesByPhaseOrFail call GetMachineByPhase or fail the test if any error occurred
func (ms MachineSet) GetMachinesByPhaseOrFail(phase string) []Machine {
	ml, err := ms.GetMachinesByPhase(phase)
	o.Expect(err).NotTo(o.HaveOccurred(), "Get machine by phase %s failed", phase)
	o.Expect(ml).ShouldNot(o.BeEmpty(), "No machine found by phase %s in machineset %s", phase, ms.GetName())
	return ml
}

// GetNodes returns a slice with all nodes that have been created for this MachineSet
func (ms MachineSet) GetNodes() ([]*Node, error) {
	machines, mErr := ms.GetMachines()
	if mErr != nil {
		return nil, mErr
	}

	nodes := []*Node{}
	for _, m := range machines {
		n, nErr := m.GetNode()
		if nErr != nil {
			return nil, nErr
		}

		nodes = append(nodes, n)
	}
	return nodes, nil
}

// WaitUntilReady waits until the MachineSet reports a Ready status
func (ms MachineSet) WaitUntilReady(duration string) error {
	pDuration, err := time.ParseDuration(duration)
	if err != nil {
		logger.Errorf("Error parsing duration %s. Errot: %s", duration, err)
		return err
	}

	immediate := false
	pollerr := wait.PollUntilContextTimeout(context.TODO(), 20*time.Second, pDuration, immediate, func(ctx context.Context) (bool, error) {
		return ms.GetIsReady(), nil
	})

	return pollerr
}

// Duplicate creates a new MachineSet by ducplicating the MachineSet information but using a new name, the new duplicated Machineset has 0 replicas
// If you need to further modify the new machineset, patch it, and scale it up
// For example, to duplicate a machineset and use a new secret in the duplicated machineset:
// newMs := ms.Duplicate("newname")
// err = newMs.Patch("json", `[{ "op": "replace", "path": "/spec/template/spec/providerSpec/value/userDataSecret/name", "value": "newSecretName" }]`)
// newMs.ScaleTo(1)
func (ms MachineSet) Duplicate(newName string) (*MachineSet, error) {
	jMachineset := JSON(ms.GetOrFail(`{}`))

	jAPIVersion := jMachineset.Get(`apiVersion`)
	if !jAPIVersion.Exists() {
		return nil, fmt.Errorf(".apiVersion does not exist in  machineset %s. Definition: %s", ms.GetName(), jMachineset)
	}

	jSpec := jMachineset.Get(`spec`)
	if !jSpec.Exists() {
		return nil, fmt.Errorf(".spec does not exist in  machineset %s. Definition: %s", ms.GetName(), jMachineset)
	}

	rErr := jSpec.PutSafe("replicas", 0)
	if rErr != nil {
		return nil, rErr
	}

	jMatchLabels := jSpec.Get("selector").Get("matchLabels")
	if !jMatchLabels.Exists() {
		return nil, fmt.Errorf(".spec.selector.matchLabels does not exist in  machineset %s spec: %s", ms.GetName(), jSpec)
	}

	// Remove old matchlabel
	dErr := jMatchLabels.DeleteSafe("machine.openshift.io/cluster-api-machineset")
	if dErr != nil {
		return nil, dErr
	}

	// Add new matchlabel
	pErr := jMatchLabels.PutSafe("machine.openshift.io/cluster-api-machineset", newName)
	if pErr != nil {
		return nil, pErr
	}

	tplLabels := jSpec.Get("template").Get("metadata").Get("labels")
	if !tplLabels.Exists() {
		return nil, fmt.Errorf(".template.metadata.labels does not exist in  machineset %s spec: %s", ms.GetName(), jSpec)
	}

	// Remove old machine label
	tdErr := tplLabels.DeleteSafe("machine.openshift.io/cluster-api-machineset")
	if tdErr != nil {
		return nil, tdErr
	}

	// Add new machine label
	tpErr := tplLabels.PutSafe("machine.openshift.io/cluster-api-machineset", newName)
	if tpErr != nil {
		return nil, tpErr
	}

	specAsJSONString, jsErr := jSpec.AsJSONString()
	if jsErr != nil {
		return nil, jsErr
	}

	newMsAsJSONString := fmt.Sprintf(`{"kind": "%s", "apiVersion": "%s", "metadata": {"name":"%s", "namespace": "%s"}, "spec": %s}`,
		ms.GetKind(), jAPIVersion.ToString(), newName, ms.GetNamespace(), specAsJSONString)

	tmpFile := generateTmpFile(ms.oc, "machineset-"+newName+".yml")

	wErr := os.WriteFile(tmpFile, []byte(newMsAsJSONString), 0o644)
	if wErr != nil {
		return nil, wErr
	}

	logger.Infof("New machinset created using definition file %s", tmpFile)

	_, cErr := ms.oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", tmpFile).Output()

	if cErr != nil {
		return nil, cErr
	}

	return NewMachineSet(ms.oc, ms.GetNamespace(), newName), nil
}

// GetAll returns a []node list with all existing nodes
func (msl *MachineSetList) GetAll() ([]MachineSet, error) {
	allMSResources, err := msl.ResourceList.GetAll()
	if err != nil {
		return nil, err
	}
	allMS := make([]MachineSet, 0, len(allMSResources))

	for _, msRes := range allMSResources {
		allMS = append(allMS, *NewMachineSet(msl.oc, msRes.GetNamespace(), msRes.GetName()))
	}

	return allMS, nil
}

// msDuplicatedSecretChanges struct with all values that will be changed in a duplicated machinset secret
type msDuplicatedSecretChanges struct {
	Name                 string
	IgnitionVersion      string
	IgnitionConfigAction string
}

func duplicateMachinesetSecret(oc *exutil.CLI, secretName string, changes msDuplicatedSecretChanges) (*Secret, error) {

	userData, udErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret", secretName, "-n", MachineAPINamespace,
		"--template", `{{index .data "userData" | base64decode}}`).Output()

	if udErr != nil {
		logger.Errorf("Error getting userData info from secret %s -n %s.\n%s", secretName, MachineAPINamespace, udErr)
		return nil, udErr
	}

	disableTemplating, dtErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret", secretName, "-n", MachineAPINamespace, "--template", `{{index .data "disableTemplating" | base64decode}}`).Output()

	if dtErr != nil {
		logger.Errorf("Error getting disableTemplating info from secret %s -n %s.\n%s", secretName, MachineAPINamespace, dtErr)
		return nil, dtErr
	}

	// if necessary, replace the ignition config action and the ignition version
	if changes.IgnitionVersion != "" || changes.IgnitionConfigAction != "" {

		jUserData := JSON(userData)

		jIgnition := jUserData.Get("ignition")
		if !jIgnition.Exists() {
			return nil, fmt.Errorf("No 'ignition' key in userData: %s", userData)
		}

		if changes.IgnitionVersion != "" {

			jpErr := jIgnition.PutSafe("version", changes.IgnitionVersion)
			if jpErr != nil {
				return nil, jpErr
			}
		}

		if changes.IgnitionConfigAction != "" {
			jConfig := jIgnition.Get("config")
			if !jConfig.Exists() {
				return nil, fmt.Errorf("No 'config' key in userData.ignition: %s", userData)
			}

			jMerge := jConfig.Get("merge")
			if !jMerge.Exists() {
				return nil, fmt.Errorf("No 'merge' key in userData.ignition.config: %s", userData)
			}

			cpErr := jConfig.PutSafe("append", jMerge.ToInterface())
			if cpErr != nil {
				return nil, cpErr
			}

			dErr := jConfig.DeleteSafe("merge")
			if dErr != nil {
				return nil, dErr
			}
		}
		var mErr error
		userData, mErr = jUserData.AsJSONString()
		if mErr != nil {
			logger.Errorf("Error marshaling userData info from secret %s -n %s.UserData: %s \n \n%s", secretName, MachineAPINamespace, jUserData, mErr)
			return nil, mErr
		}

	}

	_, err := oc.AsAdmin().WithoutNamespace().Run("create").Args("secret", "generic", changes.Name, "-n", MachineAPINamespace,
		"--from-literal", fmt.Sprintf("userData=%s", userData),
		"--from-literal", fmt.Sprintf("disableTemplating=%s", disableTemplating)).Output()

	return NewSecret(oc.AsAdmin(), MachineAPINamespace, changes.Name), err
}
