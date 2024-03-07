package mco

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	logger "github.com/openshift/openshift-tests-private/test/extended/util/logext"

	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type ocGetter struct {
	oc        *exutil.CLI
	kind      string
	namespace string
	name      string
}

// Template helps to create resources using openshift templates
type Template struct {
	oc           *exutil.CLI
	templateFile string
}

// ResourceInterface defines all methods available in a resource
type ResourceInterface interface {
	GetKind() string
	GetName() string
	GetNamespace() string
	Get(jsonPath string, extraParams ...string) (string, error)
	GetSafe(jsonPath string, defaultValue string, extraParams ...string) string
	GetOrFail(jsonPath string, extraParams ...string) string
	Poll(jsonPath string) func() string
	Delete() error
	DeleteOrFail()
	Exists() bool
	Patch(patchType string, patch string) error
	GetAnnotationOrFail(annotation string) string
	GetConditionByType(ctype string) string
	AddLabel(label, value string) error
	GetLabel(label string) (string, error)
	Describe() (string, error)
	ExportToFile(fileName string) error
	PrettyString() string
}

// Resource will provide the functionality to hanlde general openshift resources
type Resource struct {
	ocGetter
}

// getCommonParams returns the params that are necessary for all commands involving this object
// It returns these 3 params (or 2 if the object is not namespaced): {kind} {resourcename} ({-n} {namespace} only if namespaced)
func (r *ocGetter) getCommonParams() []string {
	params := []string{r.kind}
	if r.name != "" {
		params = append(params, r.name)
	}

	if r.namespace != "" {
		params = append([]string{"-n", r.namespace}, params...)
	}

	return params
}

// GetName returns the 'name' field
func (r ocGetter) GetName() string {
	return r.name
}

// GetKind returns the 'kind' field
func (r ocGetter) GetKind() string {
	return r.kind
}

// GetNamespace returns the 'namespace' field
func (r ocGetter) GetNamespace() string {
	return r.namespace
}

// PrintDebugCommand prints the output of a "oc get $kind -n $namespace $name" command
func (r ocGetter) PrintDebugCommand() error {
	params := r.getCommonParams()
	err := r.oc.WithoutNamespace().Run("get").Args(params...).Execute()

	return err
}

// Get uses the CLI to retrieve the return value for this jsonpath
func (r *ocGetter) Get(jsonPath string, extraParams ...string) (string, error) {
	params := r.getCommonParams()

	params = append(params, extraParams...)

	params = append(params, []string{"-o", fmt.Sprintf("jsonpath=%s", jsonPath)}...)

	logger.Debugf("resource params %v:", params)
	result, err := r.oc.WithoutNamespace().Run("get").Args(params...).Output()

	return result, err
}

// GetSafe uses the CLI to retrieve the return value for this jsonpath, if the resource does not exist, it returns the defaut value
func (r *ocGetter) GetSafe(jsonPath, defaultValue string, extraParams ...string) string {
	ret, err := r.Get(jsonPath, extraParams...)
	if err != nil {
		return defaultValue
	}

	return ret
}

// GetOrFail uses the CLI to retrieve the return value for this jsonpath, if the resource does not exist, it fails the test
func (r *ocGetter) GetOrFail(jsonPath string, extraParams ...string) string {
	ret, err := r.Get(jsonPath, extraParams...)
	if err != nil {
		e2e.Failf("Could not get value %s. Error: %v", jsonPath, err)
	}

	return ret
}

// PollValue returns a function suitable to be used with the gomega Eventually/Consistently checks
func (r *ocGetter) Poll(jsonPath string) func() string {
	return func() string {
		ret, _ := r.Get(jsonPath)
		return ret
	}
}

// NewResource constructs a Resource struct for a not-namespaced resource
func NewResource(oc *exutil.CLI, kind, name string) *Resource {
	return &Resource{ocGetter: ocGetter{oc, kind, "", name}}
}

// NewNamespacedResource constructs a Resource struct for a namespaced resource
func NewNamespacedResource(oc *exutil.CLI, kind, namespace, name string) *Resource {
	return &Resource{ocGetter: ocGetter{oc, kind, namespace, name}}
}

// Delete removes the resource from openshift cluster
func (r *Resource) Delete() error {
	params := r.getCommonParams()

	_, err := r.oc.WithoutNamespace().Run("delete").Args(params...).Output()
	if err != nil {
		logger.Errorf("%v", err)
	}

	return err
}

// DeleteOrFail deletes the resource, and if any error happens it fails the testcase
func (r *Resource) DeleteOrFail() {
	err := r.Delete()
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Exists returns true if the resource exists and false if not
func (r *Resource) Exists() bool {
	_, err := r.Get("{.}")
	return err == nil
}

// String implements the Stringer interface
func (r Resource) String() string {
	return fmt.Sprintf("<Kind: %s, Name: %s, Namespace: %s>", r.kind, r.name, r.namespace)
}

// Patch patches the resource using the given patch type
// The following patches are exactly the same patch but using different types, 'merge' and 'json'
// --type merge -p '{"spec": {"selector": {"app": "frommergepatch"}}}'
// --type json  -p '[{ "op": "replace", "path": "/spec/selector/app", "value": "fromjsonpatch"}]'
func (r *Resource) Patch(patchType, patch string) error {
	params := r.getCommonParams()

	params = append(params, []string{"--type", patchType, "-p", patch}...)

	_, err := r.oc.WithoutNamespace().Run("patch").Args(params...).Output()
	if err != nil {
		logger.Errorf("%v", err)
	}

	return err
}

// GetAnnotationOrFail returns the value
func (r *Resource) GetAnnotationOrFail(annotation string) string {
	scapedAnnotation := strings.ReplaceAll(annotation, `.`, `\.`)
	return r.GetOrFail(fmt.Sprintf(`{.metadata.annotations.%s}`, scapedAnnotation))
}

// GetConditionByType returns the status.condition matching the given type
func (r *Resource) GetConditionByType(ctype string) string {
	return r.GetOrFail(`{.status.conditions[?(@.type=="` + ctype + `")]}`)
}

func (r *Resource) GetConditionStatusByType(ctype string) string {
	return r.GetOrFail(`{.status.conditions[?(@.type=="` + ctype + `")].status}`)
}

// GetLabel returns the label's value if the value exists. It returns an error if the label does not exist
func (r *Resource) GetLabel(label string) (string, error) {
	labels := map[string]string{}
	labelsJSON, err := r.Get(`{.metadata.labels}`)
	if err != nil {
		return "", err
	}
	if labelsJSON == "" {
		return "", fmt.Errorf("Labels not defined. Could not get .metadata.labels attribute")
	}

	if err := json.Unmarshal([]byte(labelsJSON), &labels); err != nil {
		return "", err
	}

	value, ok := labels[label]
	if !ok {
		return "", fmt.Errorf("%s. Label not found in -n %s %s",
			label, r.GetNamespace(), r.GetName())
	}

	return value, nil
}

// AddLabel adds a label to the resource
func (r *Resource) AddLabel(label, value string) error {
	params := r.getCommonParams()

	params = append(params, []string{label + "=" + value}...)

	return r.oc.WithoutNamespace().Run("label").Args(params...).Execute()
}

// RemoveLabel removes a label to the resource
func (r *Resource) RemoveLabel(label string) error {
	params := r.getCommonParams()

	params = append(params, []string{label + "-"}...)

	return r.oc.WithoutNamespace().Run("label").Args(params...).Execute()
}

func (r *Resource) Describe() (string, error) {
	params := []string{r.kind, r.name}
	if r.namespace != "" {
		params = append([]string{"-n", r.namespace}, params...)
	}
	return r.oc.WithoutNamespace().Run("describe").Args(params...).Output()
}

// ExportToFile writes the resource json information in a given file.
func (r *Resource) ExportToFile(fileName string) error {

	// We want to write the json info as "pretty", so that it is human readable.
	// But we don't want to use "PrettyString" because we want full control on the errors
	definition, dErr := r.Get(`{}`)
	if dErr != nil {
		return dErr
	}

	var data interface{}
	if err := json.Unmarshal([]byte(definition), &data); err != nil {
		return err
	}

	formattedDefinition, err := json.MarshalIndent(data, "", "    ")
	if err != nil {
		return err
	}

	value := string(formattedDefinition)

	err = os.WriteFile(fileName, []byte(value), 0o644)
	if err != nil {
		logger.Infof("Resource %s  has been saved in file %s", r, fileName)
	}

	return err
}

// PrettyString returns an indented json string with the definition of the resource
func (r *Resource) PrettyString() string {
	definition, dErr := r.Get(`{}`)
	if dErr != nil {
		return dErr.Error()
	}

	var data interface{}
	if err := json.Unmarshal([]byte(definition), &data); err != nil {
		return err.Error()
	}

	formattedDefinition, err := json.MarshalIndent(data, "", "    ")
	if err != nil {
		return err.Error()
	}
	return string(formattedDefinition)

}

// NewMCOTemplate creates a new template using the MCO fixture directory as the base path of the template file
func NewMCOTemplate(oc *exutil.CLI, fileName string) *Template {
	return &Template{oc: oc, templateFile: generateTemplateAbsolutePath(fileName)}
}

// SetTemplate sets the template file that will be used to create this resource
func (t *Template) SetTemplate(template string) {
	t.templateFile = template
}

// Create the resources defined in the template file
// The template will be created using oc with no namespace (-n NAMESPACE) argument. So if we want to
// create a namespaced resource we need to add the NAMESPACE parameter to the template and
// provide the "-p NAMESPACE" argument to this function.
func (t *Template) Create(parameters ...string) error {
	if t.templateFile == "" {
		return fmt.Errorf("There is no template configured")
	}

	allParams := []string{"--ignore-unknown-parameters=true", "-f", t.templateFile}
	allParams = append(allParams, parameters...)

	exutil.CreateClusterResourceFromTemplate(t.oc, allParams...)

	return nil
}

// ResourceList provides the functionality to handle lists of openshift resources
type ResourceList struct {
	ocGetter
	extraParams []string
	itemsFilter string
}

// NewResourceList constructs a ResourceList struct for not-namespaced resources
func NewResourceList(oc *exutil.CLI, kind string) *ResourceList {
	return &ResourceList{ocGetter{oc.AsAdmin(), kind, "", ""}, []string{}, ""}
}

// NewNamespacedResourceList constructs a ResourceList struct for namespaced resources
func NewNamespacedResourceList(oc *exutil.CLI, kind, namespace string) *ResourceList {
	return &ResourceList{ocGetter{oc.AsAdmin(), kind, namespace, ""}, []string{}, ""}
}

// CleanParams removes the extraparams added by methods like "ByLabel" or "SorBy..."
func (l *ResourceList) CleanParams() {
	l.extraParams = []string{}
}

// SortByTimestamp will configure the list to be sorted by creation timestamp
func (l *ResourceList) SortByTimestamp() {
	l.SortBy("metadata.creationTimestamp")
}

// SortByZone will configure the list to be sorted by HA topology zone
func (l *ResourceList) SortByZone() {
	l.SortBy(`.metadata.labels.topology\.kubernetes\.io/zone`)
}

// SortBy will configure the list to be sorted by the given field
func (l *ResourceList) SortBy(field string) {
	l.extraParams = append(l.extraParams, fmt.Sprintf(`--sort-by=%s`, field))
}

// ByLabel will use the given label to filter the list
func (l *ResourceList) ByLabel(label string) {
	l.extraParams = append(l.extraParams, fmt.Sprintf("--selector=%s", label))
}

// ByFieldSelector will use the given field selector to fileter the list
func (l *ResourceList) ByFieldSelector(fieldSelector string) {
	l.extraParams = append(l.extraParams, fmt.Sprintf("--field-selector=%s", fieldSelector))
}

// SetItemsFilter sets the filter used by jsonpath expression when getting all resources "{.items["+ itemsFilter + "].metadata.name}"
// an example of a valid filter is: `?(@.metadata.annotations.machine\.openshift\.io/machine=="openshift-machine-api/mymachinesetname-rc2-g5wx5-worker-us-east-2a-t9hw2")`
func (l *ResourceList) SetItemsFilter(filter string) {
	l.itemsFilter = filter
}

// GetAll returns a list of Resource structs with the resources found in this list
func (l ResourceList) GetAll() ([]Resource, error) {
	if l.itemsFilter == "" {
		l.itemsFilter = "*"
	}

	// silently look for the elements in order not to create a dirty log
	// TODO: Improve this. There is no method to get the current showInfo value, so we can't restore it
	l.oc.NotShowInfo()
	defer l.oc.SetShowInfo()

	allItemsNames, err := l.Get("{.items["+l.itemsFilter+"].metadata.name}", l.extraParams...)
	if err != nil {
		return nil, err
	}

	allNames := strings.Split(strings.Trim(allItemsNames, " "), " ")

	allResources := []Resource{}
	for _, name := range allNames {
		if name != "" {
			newResource := Resource{ocGetter: ocGetter{l.oc, l.kind, l.namespace, name}}
			allResources = append(allResources, newResource)
		}
	}

	return allResources, nil
}

// Exist returns a gomega matcher that checks if a resource exists or not
func Exist() types.GomegaMatcher {
	return &existMatcher{}
}

type existMatcher struct {
}

func (matcher *existMatcher) Match(actual interface{}) (success bool, err error) {
	resource, ok := actual.(ResourceInterface)
	if !ok {
		return false, fmt.Errorf("Exist matcher expects a Resource in case %v", g.CurrentSpecReport().FullText())
	}

	return resource.Exists(), nil
}

func (matcher *existMatcher) FailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("Expected resource\n\t%s\nto exist", actual)
}

func (matcher *existMatcher) NegatedFailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("Expected resource\n\t%s\nnot to exist", actual)
}
