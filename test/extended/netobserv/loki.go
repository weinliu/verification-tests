package netobserv

import (
	"fmt"
	"reflect"
	"strings"

	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"

	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// lokiStack contains the configurations of loki stack
type lokiStack struct {
	Name          string // lokiStack name
	Namespace     string // lokiStack namespace
	TSize         string // size
	StorageType   string // the backend storage type, currently support s3, gcs, azure, swift, ODF and minIO
	StorageSecret string // the secret name for loki to use to connect to backend storage
	StorageClass  string // storage class name
	BucketName    string // the butcket or the container name where loki stores it's data in
	Tenant        string // Loki tenant name
	Template      string // the file used to create the loki stack
	Route         string // lokistack-gateway-http route to be initialized after lokistack is up.
	EnableIPV6    string // enable IPV6
}

// LokiPersistentVolumeClaim struct to handle Loki PVC resources
type LokiPersistentVolumeClaim struct {
	Namespace string
	Template  string
}

// LokiStorage struct to handle LokiStorage resources
type LokiStorage struct {
	Namespace string
	Template  string
}

// deploy LokiPVC
func (loki *LokiPersistentVolumeClaim) deployLokiPVC(oc *exutil.CLI) {
	e2e.Logf("Deploy Loki PVC")
	parameters := []string{"--ignore-unknown-parameters=true", "-f", loki.Template, "-p", "NAMESPACE=" + loki.Namespace}
	exutil.ApplyNsResourceFromTemplate(oc, loki.Namespace, parameters...)
}

// deploy LokiStorage
func (loki *LokiStorage) deployLokiStorage(oc *exutil.CLI) {
	e2e.Logf("Deploy Loki storage")
	parameters := []string{"--ignore-unknown-parameters=true", "-f", loki.Template, "-p", "NAMESPACE=" + loki.Namespace}
	exutil.ApplyNsResourceFromTemplate(oc, loki.Namespace, parameters...)
}

// delete LokiStorage
func (loki *LokiStorage) deleteLokiStorage(oc *exutil.CLI) {
	e2e.Logf("Delete Loki PVC")
	command1 := []string{"pod", "loki", "-n", loki.Namespace}
	_, err1 := oc.AsAdmin().WithoutNamespace().Run("delete").Args(command1...).Output()

	command2 := []string{"configmap", "loki-config", "-n", loki.Namespace}
	_, err2 := oc.AsAdmin().WithoutNamespace().Run("delete").Args(command2...).Output()

	command3 := []string{"service", "loki", "-n", loki.Namespace}
	_, err3 := oc.AsAdmin().WithoutNamespace().Run("delete").Args(command3...).Output()
	o.Expect(err1).NotTo(o.HaveOccurred())
	o.Expect(err2).NotTo(o.HaveOccurred())
	o.Expect(err3).NotTo(o.HaveOccurred())
}

// DeployLokiStack creates the lokiStack CR with basic settings: name, namespace, size, storage.secret.name, storage.secret.type, storageClassName
// optionalParameters is designed for adding parameters to deploy lokiStack with different tenants or some other settings
func (l lokiStack) deployLokiStack(oc *exutil.CLI) error {
	parameters := []string{"--ignore-unknown-parameters=true", "-f", l.Template, "-p"}

	lokistack := reflect.ValueOf(&l).Elem()

	for i := 0; i < lokistack.NumField(); i++ {
		if lokistack.Field(i).Interface() != "" {
			if lokistack.Type().Field(i).Name == "StorageType" {
				if lokistack.Field(i).Interface() == "odf" || lokistack.Field(i).Interface() == "minio" {
					parameters = append(parameters, fmt.Sprintf("%s=%s", lokistack.Type().Field(i).Name, "s3"))
				} else {
					parameters = append(parameters, fmt.Sprintf("%s=%s", lokistack.Type().Field(i).Name, lokistack.Field(i).Interface()))
				}
			} else {
				if lokistack.Type().Field(i).Name == "Template" {
					continue
				} else {
					parameters = append(parameters, fmt.Sprintf("%s=%s", lokistack.Type().Field(i).Name, lokistack.Field(i).Interface()))
				}
			}
		}
	}

	file, err := processTemplate(oc, parameters...)
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Can not process %v", parameters))
	err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", file, "-n", l.Namespace).Execute()
	return err
}

func (l lokiStack) waitForLokiStackToBeReady(oc *exutil.CLI) error {
	var err error
	for _, deploy := range []string{l.Name + "-distributor", l.Name + "-gateway", l.Name + "-querier", l.Name + "-query-frontend"} {
		err = waitForDeploymentPodsToBeReady(oc, l.Namespace, deploy)
	}
	for _, ss := range []string{l.Name + "-compactor", l.Name + "-index-gateway", l.Name + "-ingester"} {
		err = waitForStatefulsetReady(oc, l.Namespace, ss)
	}
	return err
}

func (l lokiStack) removeLokiStack(oc *exutil.CLI) {
	Resource{"lokistack", l.Name, l.Namespace}.clear(oc)
	_ = oc.AsAdmin().WithoutNamespace().Run("delete").Args("pvc", "-n", l.Namespace, "-l", "app.kubernetes.io/instance="+l.Name).Execute()
}

// Get OIDC provider for the cluster
func getOIDC(oc *exutil.CLI) (string, error) {
	oidc, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("authentication.config", "cluster", "-o=jsonpath={.spec.serviceAccountIssuer}").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimPrefix(oidc, "https://"), nil
}

func getLokiChannel(oc *exutil.CLI, catalog string) (lokiChannel string, err error) {
	channels, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("packagemanifests", "-l", "catalog="+catalog, "-n", "openshift-marketplace", "-o=jsonpath={.items[?(@.metadata.name==\"loki-operator\")].status.channels[*].name}").Output()
	channelArr := strings.Split(channels, " ")
	return channelArr[len(channelArr)-1], err
}
