package mco

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	logger "github.com/openshift/openshift-tests-private/test/extended/util/logext"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// Secret struct encapsulates the functionalities regarding ocp secrets
type Secret struct {
	Resource
}

// SecretList handles list of secrets
type SecretList struct {
	ResourceList
}

// NewSecret creates a Secret struct
func NewSecret(oc *exutil.CLI, namespace, name string) *Secret {
	return &Secret{Resource: *NewNamespacedResource(oc, "secret", namespace, name)}
}

// NewSecretList creates a new  SecretList struct
func NewSecretList(oc *exutil.CLI, namespace string) *SecretList {
	return &SecretList{*NewNamespacedResourceList(oc, "secret", namespace)}
}

// GetAll returns a []Secret list with all existing secrets
func (sl *SecretList) GetAll() ([]Secret, error) {
	allSecretResources, err := sl.ResourceList.GetAll()
	if err != nil {
		return nil, err
	}
	allSecrets := make([]Secret, 0, len(allSecretResources))

	for _, secretRes := range allSecretResources {
		allSecrets = append(allSecrets, *NewSecret(sl.oc, sl.GetNamespace(), secretRes.name))
	}

	return allSecrets, nil
}

// ExtractToDir extracts the secret's content to a given directory
func (s Secret) ExtractToDir(directory string) error {
	err := s.oc.WithoutNamespace().Run("extract").Args(s.GetKind()+"/"+s.GetName(), "-n", s.GetNamespace(), "--to", directory).Execute()
	if err != nil {
		return err
	}

	return nil
}

// Extract extracts the secret's content to a random directory in the testcase's output directory
func (s Secret) Extract() (string, error) {
	layout := "2006_01_02T15-04-05Z.000"

	directory := filepath.Join(e2e.TestContext.OutputDir, fmt.Sprintf("%s-%s-secret-%s", s.GetNamespace(), s.GetName(), time.Now().Format(layout)))
	os.MkdirAll(directory, os.ModePerm)
	return directory, s.ExtractToDir(directory)
}

// GetDataValue gets the value stored in the secret's key
func (s Secret) GetDataValue(key string) (string, error) {
	templateArg := fmt.Sprintf(`--template={{index .data "%s" | base64decode}}`, key)
	return s.oc.AsAdmin().WithoutNamespace().Run("get").Args(s.GetKind(), s.GetName(), "-n", s.GetNamespace(), templateArg).Output()
}

// GetDataValueOrFail gets the value stored in the secret's key and fail the test if there is any error
func (s Secret) GetDataValueOrFail(key string) string {
	data, err := s.GetDataValue(key)
	o.ExpectWithOffset(1, err).NotTo(o.HaveOccurred(),
		"Cannot get the value of %s from secret %s -n %s",
		key, s.GetName(), s.GetNamespace())
	return data
}

// SetDataValue sets a key/value to store in the secret
func (s Secret) SetDataValue(key, value string) error {
	logger.Debugf("Secret %s -n %s. Setting value: %s=%s", s.GetName(), s.GetNamespace(), key, value)
	// command example: oc secret pull-secret -n openshift-config set data .dockerconfigjson={}
	return s.oc.AsAdmin().WithoutNamespace().Run("set").Args("data", s.GetKind(), s.GetName(),
		"-n", s.GetNamespace(),
		fmt.Sprintf("%s=%s", key, value)).Execute()
}

// GetPullSecret returns the cluster's pull secret
func GetPullSecret(oc *exutil.CLI) *Secret {
	return NewSecret(oc.AsAdmin(), "openshift-config", "pull-secret")
}

// waitUntilSecretHasStableValue polls a data in a secret and returns its value once it has been retrieved "numTries" times with the same value
func waitUntilSecretHasStableValue(secret *Secret, data string, timeout, poll time.Duration, numTries int) (string, error) {
	var (
		count  = 0
		oldVal string
	)

	immediate := true
	waitErr := wait.PollUntilContextTimeout(context.TODO(), poll, timeout, immediate,
		func(ctx context.Context) (bool, error) {
			val, err := secret.GetDataValue(data)

			if val == oldVal && err == nil {
				count++
			} else {
				count = 0
				oldVal = val
			}

			if count == numTries {
				return true, nil
			}
			return false, nil

		})

	if waitErr == nil {
		return oldVal, nil
	}

	return "", waitErr
}
