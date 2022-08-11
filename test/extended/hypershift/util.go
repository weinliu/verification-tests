package hypershift

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// OcpClientVerb is the oc verb operation of OCP
type OcpClientVerb string

/*
oc <OcpClientVerb> resources
*/
const (
	OcpGet      OcpClientVerb = "get"
	OcpPatch    OcpClientVerb = "patch"
	OcpWhoami   OcpClientVerb = "whoami"
	OcpDelete   OcpClientVerb = "delete"
	OcpAnnotate OcpClientVerb = "annotate"
	OcpDebug    OcpClientVerb = "debug"
	OcpExec     OcpClientVerb = "exec"

	//NodepoolNameSpace is the namespace where the nodepool CR is always created
	NodepoolNameSpace = "clusters"

	ClusterInstallTimeout = 1800 * time.Second
	LongTimeout           = 600 * time.Second
	DefaultTimeout        = 300 * time.Second
	ShortTimeout          = 50 * time.Second
)

func doOcpReq(oc *exutil.CLI, verb OcpClientVerb, notEmpty bool, args []string) string {
	e2e.Logf("running command : oc %s %s \n", string(verb), strings.Join(args, " "))
	res, err := oc.AsAdmin().WithoutNamespace().Run(string(verb)).Args(args...).Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
	if notEmpty {
		o.Expect(res).ShouldNot(o.BeEmpty())
	}
	return res
}

func checkSubstring(src string, expect []string) {
	if expect == nil || len(expect) <= 0 {
		o.Expect(expect).ShouldNot(o.BeEmpty())
	}

	for i := 0; i < len(expect); i++ {
		o.Expect(src).To(o.ContainSubstring(expect[i]))
	}
}

type workload struct {
	name      string
	namespace string
	template  string
}

func (wl *workload) create(oc *exutil.CLI, kubeconfig, parsedTemplate string) {
	err := wl.applyResourceFromTemplate(oc, kubeconfig, parsedTemplate, "--ignore-unknown-parameters=true", "-f", wl.template, "-p", "NAME="+wl.name, "NAMESPACE="+wl.namespace)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (wl *workload) delete(oc *exutil.CLI, kubeconfig, parsedTemplate string) {
	defer func() {
		path := filepath.Join(e2e.TestContext.OutputDir, oc.Namespace()+"-"+parsedTemplate)
		os.Remove(path)
	}()
	args := []string{"job", wl.name, "-n", wl.namespace}
	if kubeconfig != "" {
		args = append(args, "--kubeconfig="+kubeconfig)
	}
	err := oc.AsAdmin().WithoutNamespace().Run("delete").Args(args...).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (wl *workload) applyResourceFromTemplate(oc *exutil.CLI, kubeconfig, parsedTemplate string, parameters ...string) error {
	return applyResourceFromTemplate(oc, kubeconfig, parsedTemplate, parameters...)
}

// parse a struct for a Template variables to generate params like "NAME=myname", "NAMESPACE=clusters" ...
// currently only support int, string, bool, *int, *string, *bool. A pointer is used to check whether it is set explicitly.
// use json tag as the true variable Name in the struct e.g. < Name string `json:"NAME"`>
func parseTemplateVarParams(obj interface{}) ([]string, error) {
	v := reflect.ValueOf(obj)
	if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Struct {
		return []string{}, errors.New("params must be a pointer pointed to a struct")
	}

	var params []string
	t := v.Elem().Type()
	for i := 0; i < t.NumField(); i++ {
		varName := t.Field(i).Name
		varType := t.Field(i).Type
		varValue := v.Elem().Field(i).Interface()
		tagName := t.Field(i).Tag.Get("json")

		if tagName == "" {
			continue
		}

		//handle non nil pointer that set the params explicitly
		if varType.Kind() == reflect.Ptr {
			if reflect.ValueOf(varValue).IsNil() {
				continue
			}

			switch reflect.ValueOf(varValue).Elem().Type().Kind() {
			case reflect.Int:
				p := fmt.Sprintf("%s=%d", tagName, reflect.ValueOf(varValue).Elem().Interface().(int))
				params = append(params, p)
			case reflect.String:
				params = append(params, tagName+"="+reflect.ValueOf(varValue).Elem().Interface().(string))
			case reflect.Bool:
				v, _ := reflect.ValueOf(varValue).Elem().Interface().(bool)
				params = append(params, tagName+"="+strconv.FormatBool(v))
			default:
				e2e.Logf("parseTemplateVarParams params %v invalid, ignore it", varName)
			}
			continue
		}

		//non-pointer
		switch varType.Kind() {
		case reflect.String:
			if varValue.(string) != "" {
				params = append(params, tagName+"="+varValue.(string))
			}
		case reflect.Int:
			params = append(params, tagName+"="+strconv.Itoa(varValue.(int)))
		case reflect.Bool:
			params = append(params, tagName+"="+strconv.FormatBool(varValue.(bool)))
		default:
			e2e.Logf("parseTemplateVarParams params %v not support, ignore it", varValue)
		}
	}

	return params, nil
}

func applyResourceFromTemplate(oc *exutil.CLI, kubeconfig, parsedTemplate string, parameters ...string) error {
	var configFile string

	err := wait.Poll(3*time.Second, 15*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().Run("process").Args(parameters...).OutputToFile(parsedTemplate)
		if err != nil {
			e2e.Logf("the err:%v, and try next round", err)
			return false, nil
		}
		configFile = output
		return true, nil
	})
	o.Expect(err).NotTo(o.HaveOccurred())

	e2e.Logf("the file of resource is %s", configFile)

	var args = []string{"-f", configFile}
	if kubeconfig != "" {
		args = append(args, "--kubeconfig="+kubeconfig)
	}
	return oc.AsAdmin().WithoutNamespace().Run("apply").Args(args...).Execute()
}

func getClusterRegion(oc *exutil.CLI) (string, error) {
	return oc.AsAdmin().WithoutNamespace().Run("get").Args("node", `-ojsonpath={.items[].metadata.labels.topology\.kubernetes\.io/region}`).Output()
}

func getBaseDomain(oc *exutil.CLI) (string, error) {
	str, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("dns/cluster", `-ojsonpath={.spec.baseDomain}`).Output()
	if err != nil {
		return "", err
	}
	index := strings.Index(str, ".")
	if index == -1 {
		return "", fmt.Errorf("can not parse baseDomain because not finding '.'")
	}
	return str[index+1:], nil
}

func getAWSKey(oc *exutil.CLI) (string, string, error) {
	accessKeyID, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret/aws-creds", "-n", "kube-system", "-o", `template={{index .data "aws_access_key_id"|base64decode}}`).Output()
	if err != nil {
		return "", "", err
	}
	secureKey, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret/aws-creds", "-n", "kube-system", "-o", `template={{index .data "aws_secret_access_key"|base64decode}}`).Output()
	if err != nil {
		return "", "", err
	}
	return accessKeyID, secureKey, nil
}

/* parse a structure's tag 'param' and output cli command parameters
e.g.
Input:
  type example struct {
	Name string `param:"name"`
    PullSecret string `param:"pull_secret"`
  } {
  	Name:"hypershift",
    PullSecret:"pullsecret.txt",
  }
Output:
  --name="hypershift" --pull_secret="pullsecret.txt"
*/
func parse(obj interface{}) ([]string, error) {
	v := reflect.ValueOf(obj)
	if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Struct {
		return []string{}, errors.New("params must be a pointer pointed to a struct")
	}
	var params []string
	t := v.Elem().Type()
	for i := 0; i < t.NumField(); i++ {
		varName := t.Field(i).Name
		varType := t.Field(i).Type
		varValue := v.Elem().Field(i).Interface()
		tagName := t.Field(i).Tag.Get("param")
		if tagName == "" {
			continue
		}
		if varType.Kind() == reflect.Ptr {
			if reflect.ValueOf(varValue).IsNil() {
				continue
			}
			switch reflect.ValueOf(varValue).Elem().Type().Kind() {
			case reflect.Int:
				p := fmt.Sprintf("--%s=%d", tagName, reflect.ValueOf(varValue).Elem().Interface().(int))
				params = append(params, p)
			case reflect.String:
				params = append(params, "--"+tagName+"="+reflect.ValueOf(varValue).Elem().Interface().(string))
			case reflect.Bool:
				v, _ := reflect.ValueOf(varValue).Elem().Interface().(bool)
				params = append(params, "--"+tagName+"="+strconv.FormatBool(v))
			default:
				e2e.Logf("parseTemplateVarParams params %v invalid, ignore it", varName)
			}
			continue
		}
		switch varType.Kind() {
		case reflect.String:
			if varValue.(string) != "" {
				params = append(params, "--"+tagName+"="+varValue.(string))
			}
		case reflect.Int:
			params = append(params, "--"+tagName+"="+strconv.Itoa(varValue.(int)))
		case reflect.Bool:
			params = append(params, "--"+tagName+"="+strconv.FormatBool(varValue.(bool)))
		default:
			e2e.Logf("parseTemplateVarParams params %v not support, ignore it", varValue)
		}
	}
	return params, nil
}
