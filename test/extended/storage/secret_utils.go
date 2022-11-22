package storage

import (
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

// Secret related functions
type secret struct {
	name      string
	namespace string
	secType   string
	template  string
}

// function option mode to change the default values of secret Object attributes
type secretOption func(*secret)

// Replace the default value of secret name
func setSecretName(name string) secretOption {
	return func(sec *secret) {
		sec.name = name
	}
}

// Replace the default value of secret type
func setSecretType(secType string) secretOption {
	return func(sec *secret) {
		sec.secType = secType
	}
}

// Replace the default value of secret template
func setSecretTemplate(template string) secretOption {
	return func(sec *secret) {
		sec.template = template
	}
}

// Replace the default value of secret namespace
func setSecretNamespace(namespace string) secretOption {
	return func(sec *secret) {
		sec.namespace = namespace
	}
}

// Create a new customized secret object
func newSecret(opts ...secretOption) secret {
	defaultSecret := secret{
		name:      "secret-" + getRandomString(),
		namespace: "",
		secType:   "",
	}
	for _, o := range opts {
		o(&defaultSecret)
	}
	return defaultSecret
}

// Create a new customized secret with extra parameters
func (sec *secret) createWithExtraParameters(oc *exutil.CLI, extraParameters map[string]interface{}) {
	if sec.namespace == "" {
		sec.namespace = oc.Namespace()
	}
	err := applyResourceFromTemplateWithExtraParametersAsAdmin(oc, extraParameters, "--ignore-unknown-parameters=true", "-f", sec.template, "-p", "SECNAME="+sec.name, "TYPE="+sec.secType, "SECNAMESPACE="+sec.namespace)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Delete a specified secret with kubeadmin user
func (sec *secret) deleteAsAdmin(oc *exutil.CLI) {
	oc.WithoutNamespace().AsAdmin().Run("delete").Args("-n", sec.namespace, "secret", sec.name, "--ignore-not-found").Execute()
}
