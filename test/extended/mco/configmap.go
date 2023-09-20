package mco

import (
	"fmt"

	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"github.com/tidwall/gjson"
)

// ConfigMap struct encapsulates the functionalities regarding ocp configmaps
type ConfigMap struct {
	Resource
}

// NewConfigMap creates a Secret struct
func NewConfigMap(oc *exutil.CLI, namespace, name string) *ConfigMap {
	return &ConfigMap{Resource: *NewNamespacedResource(oc, "ConfigMap", namespace, name)}
}

// GetDataValue return the value of a key stored in "data".
func (cm *ConfigMap) GetDataValue(key string) (string, error) {
	// We cant use the "resource.Get" method, because exutil.client will trim the output, removing spaces and newlines that could be important in a configuration.
	dataMap, err := cm.GetDataMap()

	if err != nil {
		return "", err
	}

	data, ok := dataMap[key]
	if !ok {
		return "", fmt.Errorf("Key %s does not exist in the .data in Configmap -n %s %s",
			key, cm.GetNamespace(), cm.GetName())
	}

	return data, nil

}

// GetDataMap returns the valus in the .data field as a map[string][string]
func (cm *ConfigMap) GetDataMap() (map[string]string, error) {
	data := map[string]string{}
	dataJSON, err := cm.Get(`{.data}`)
	if err != nil {
		return nil, err
	}

	parsedData := gjson.Parse(dataJSON)
	parsedData.ForEach(func(key, value gjson.Result) bool {
		data[key.String()] = value.String()
		return true // keep iterating
	})

	return data, nil
}

// GetDataValueOrFail return the value of a key stored in "data" and fails the test if the value cannot be retreived. If the "key" does not exist, it returns an empty string but does not fail
func (cm *ConfigMap) GetDataValueOrFail(key string) string {
	value, err := cm.GetDataValue(key)
	o.ExpectWithOffset(1, err).NotTo(o.HaveOccurred(),
		"Could get the value for key %s in configmap -n %s %s",
		key, cm.GetNamespace(), cm.GetName())

	return value
}

// CreateConfigMapWithRandomCert creates a configmap that stores a random CA in it
func CreateConfigMapWithRandomCert(oc *exutil.CLI, cmNamespace, cmName, certKey string) (*ConfigMap, error) {
	_, caPath, err := createCA(createTmpDir(), certKey)
	if err != nil {
		return nil, err
	}

	err = oc.WithoutNamespace().Run("create").Args("cm", "-n", cmNamespace, cmName, "--from-file", caPath).Execute()

	if err != nil {
		return nil, err
	}

	return NewConfigMap(oc, cmNamespace, cmName), nil
}
