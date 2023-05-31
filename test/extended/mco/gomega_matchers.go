package mco

import (
	"encoding/json"
	"fmt"

	g "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega/types"
)

// struct implementing gomaega matcher interface
type conditionMatcher struct {
	conditionType    string
	field            string
	value            string
	currentCondition string // stores the current condition being checked, so that it can be displayed in the error message if the check fails
}

// Match checks it the condition with the given type has the right value in the given field.
func (matcher *conditionMatcher) Match(actual interface{}) (success bool, err error) {

	resource, ok := actual.(ResourceInterface)
	if !ok {
		return false, fmt.Errorf(`Wrong type. Matcher expects a type "Resource" in test case %v`, g.CurrentSpecReport().FullText())
	}

	matcher.currentCondition, err = resource.Get(`{.status.conditions[?(@.type=="` + matcher.conditionType + `")]}`)
	if err != nil {
		return false, err
	}

	if matcher.currentCondition == "" {
		return false, fmt.Errorf(`Condition type "%s" cannot be found in resource %s in test case %v`, matcher.conditionType, resource, g.CurrentSpecReport().FullText())
	}

	var conditionMap map[string]string
	jsonerr := json.Unmarshal([]byte(matcher.currentCondition), &conditionMap)
	if jsonerr != nil {
		return false, jsonerr
	}

	value, ok := conditionMap[matcher.field]
	if !ok {
		return false, fmt.Errorf(`Condition field "%s" cannot be found in condition %s for resource %s in test case %v`,
			matcher.field, matcher.conditionType, resource, g.CurrentSpecReport().FullText())
	}

	return value == matcher.value, nil
}

// FailureMessage returns the message in case of successful match
func (matcher *conditionMatcher) FailureMessage(actual interface{}) (message string) {
	// The type was already validated in Match, we can safely ignore the error
	resource, _ := actual.(ResourceInterface)

	return fmt.Sprintf("In resource %s, the following condition does not match value %s=%s.\n%s", resource, matcher.field, matcher.value, matcher.currentCondition)
}

// FailureMessage returns the message in case of failed match
func (matcher *conditionMatcher) NegatedFailureMessage(actual interface{}) (message string) {
	// The type was already validated in Match, we can safely ignore the error
	resource, _ := actual.(ResourceInterface)

	return fmt.Sprintf("In resource %s, the following condition does not match value %s!=%s.\n%s", resource, matcher.field, matcher.value, matcher.currentCondition)
}

// DegradedMatcher struct implementing gomaega matcher interface to check Degraded condition
type DegradedMatcher struct {
	*conditionMatcher
}

// FailureMessage returns the message in case of successful match
func (matcher *DegradedMatcher) FailureMessage(actual interface{}) (message string) {
	// The type was already validated in Match, we can safely ignore the error
	resource, _ := actual.(ResourceInterface)

	return fmt.Sprintf("Resource %s is NOT Degraded but it should.\nDegraded condition: %s", resource, matcher.currentCondition)
}

// FailureMessage returns the message in case of failed match
func (matcher *DegradedMatcher) NegatedFailureMessage(actual interface{}) (message string) {
	// The type was already validated in Match, we can safely ignore the error
	resource, _ := actual.(ResourceInterface)

	return fmt.Sprintf("Resource %s is Degraded but it should not.\nDegraded condition: %s", resource, matcher.currentCondition)
}

// BeDegraded returns the gomega matcher to check if a resource is degraded or not.
func BeDegraded() types.GomegaMatcher {
	return &DegradedMatcher{&conditionMatcher{conditionType: "Degraded", field: "status", value: "True"}}
}
