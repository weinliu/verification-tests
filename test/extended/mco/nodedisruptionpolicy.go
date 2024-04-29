package mco

import (
	"fmt"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"github.com/tidwall/gjson"
)

// NodeDisruptionPolicy, represents content of machineconfigurations.operator.openshift.io/cluster
type NodeDisruptionPolicy struct {
	Resource
}

// Policy, represents content of every policy
type Policy struct {
	result gjson.Result
}

// Action, represents content of every action in policy
type Action struct {
	result gjson.Result
}

// NewNodeDisruptionPolicy constructor of NodeDisruptionPolicy
func NewNodeDisruptionPolicy(oc *exutil.CLI) NodeDisruptionPolicy {
	return NodeDisruptionPolicy{Resource: *NewResource(oc.AsAdmin(), "machineconfigurations.operator.openshift.io", "cluster")}
}

// NewPolicy constructor of policy, input param is parsed gjson.Result
func NewPolicy(json gjson.Result) Policy {
	return Policy{result: json}
}

// NewAction constructor of action, input param is parsed gjson.Result
func NewAction(json gjson.Result) Action {
	return Action{result: json}
}

// GetType get action type
func (actn Action) GetType() string {
	return actn.result.Get("type").String()
}

// GetServieNameOfReload get serviceName of the reload object
func (actn Action) GetServieNameOfReload() string {
	return actn.result.Get("reload.serviceName").String()
}

// GetServiceNameOfRestart get serviceName of the restart object
func (actn Action) GetServiceNameOfRestart() string {
	return actn.result.Get("restart.serviceName").String()
}

// GetPath get file path of the policy
func (p Policy) GetPath() string {
	return p.result.Get("path").String()
}

// GetActions get actions of the policy
func (p Policy) GetActions() []Action {
	actions := []Action{}
	for _, action := range p.result.Get("actions").Array() {
		actions = append(actions, NewAction(action))
	}
	return actions
}

// GetPolicies get policies by policy type e.g. files/units/sshkey
func (ndp NodeDisruptionPolicy) GetPolicies(policyType string) []Policy {
	policies := []Policy{}
	// don't cache the policies, because we can get latest change when invoke this func
	files := gjson.Get(ndp.GetOrFail("{.status.nodeDisruptionPolicyStatus.clusterPolicies}"), policyType)
	for _, policy := range files.Array() {
		policies = append(policies, NewPolicy(policy))
	}
	return policies
}

// Rollback rollback the spec to the original values, it should be called in defer block
func (ndp NodeDisruptionPolicy) Rollback() {
	ndp.Patch("json", fmt.Sprintf(`[{"op": "replace", "path": "/spec", "value": %s}]`, ndp.GetOrFail("{.spec}")))
}
