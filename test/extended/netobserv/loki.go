package netobserv

import (
	"fmt"

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
func (l lokiStack) deployLokiStack(oc *exutil.CLI, optionalParameters ...string) error {
	var storage string
	if l.StorageType == "odf" || l.StorageType == "minio" {
		storage = "s3"
	} else {
		storage = l.StorageType
	}
	parameters := []string{"-f", l.Template, "-n", l.Namespace, "-p", "NAME=" + l.Name, "NAMESPACE=" + l.Namespace, "SIZE=" + l.TSize, "SECRET_NAME=" + l.StorageSecret, "STORAGE_TYPE=" + storage, "STORAGE_CLASS=" + l.StorageClass}
	if len(optionalParameters) != 0 {
		parameters = append(parameters, optionalParameters...)
	}
	file, err := processTemplate(oc, parameters...)
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Can not process %v", parameters))
	err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", file, "-n", l.Namespace).Execute()
	ls := resource{"lokistack", l.Name, l.Namespace}
	ls.waitForResourceToAppear(oc)
	return err
}

func (l lokiStack) waitForLokiStackToBeReady(oc *exutil.CLI) {
	for _, deploy := range []string{l.Name + "-distributor", l.Name + "-gateway", l.Name + "-querier", l.Name + "-query-frontend"} {
		waitForDeploymentPodsToBeReady(oc, l.Namespace, deploy)
	}
	for _, ss := range []string{l.Name + "-compactor", l.Name + "-index-gateway", l.Name + "-ingester"} {
		waitForStatefulsetReady(oc, l.Namespace, ss)
	}
}

func (l lokiStack) removeLokiStack(oc *exutil.CLI) {
	resource{"lokistack", l.Name, l.Namespace}.clear(oc)
	_ = oc.AsAdmin().WithoutNamespace().Run("delete").Args("pvc", "-n", l.Namespace, "-l", "app.kubernetes.io/instance="+l.Name).Execute()
}
