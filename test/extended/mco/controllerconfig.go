package mco

import (
	"encoding/json"

	b64 "encoding/base64"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	logger "github.com/openshift/openshift-tests-private/test/extended/util/logext"
)

// ControllerConfig struct is used to handle ControllerConfig resources in OCP
type ControllerConfig struct {
	Resource
}

// CertificateInfo stores the information regarding a given certificate
type CertificateInfo struct {
	// subject is the cert subject
	Subject string `json:"subject"`

	// signer is the  cert Issuer
	Signer string `json:"signer"`

	// Date fields have been temporarily removed by devs:  https://github.com/openshift/machine-config-operator/pull/3866
	// notBefore is the lower boundary for validity
	// NotBefore string `json:"notBefore"`

	// notAfter is the upper boundary for validity
	// NotAfter string `json:"notAfter"`

	// bundleFile is the larger bundle a cert comes from
	BundleFile string `json:"bundleFile"`
}

// NewControllerConfig create a ControllerConfig struct
func NewControllerConfig(oc *exutil.CLI, name string) *ControllerConfig {
	return &ControllerConfig{Resource: *NewResource(oc, "ControllerConfig", name)}
}

// GetKubeAPIServerServingCAData return the base64 decoded value of the kubeAPIServerServingCAData bundle stored in the ControllerConfig
func (cc *ControllerConfig) GetKubeAPIServerServingCAData() (string, error) {
	b64KubeAPIServerServingData, err := cc.Get(`{.spec.kubeAPIServerServingCAData}`)
	if err != nil {
		return "", err
	}

	kubeAPIServerServingCAData, err := b64.StdEncoding.DecodeString(b64KubeAPIServerServingData)
	if err != nil {
		return "", err
	}
	return string(kubeAPIServerServingCAData), err
}

// GetRootCAData return the base64 decoded value of the rootCA bundle stored in the ControllerConfig
func (cc *ControllerConfig) GetRootCAData() (string, error) {
	b64RootCAData, err := cc.Get(`{.spec.rootCAData}`)
	if err != nil {
		return "", err
	}

	rootCAData, err := b64.StdEncoding.DecodeString(b64RootCAData)
	if err != nil {
		return "", err
	}
	return string(rootCAData), err
}

// Returns a list of CertificateInfo structs with the information of all the certificates tracked by ControllerConfig
func (cc *ControllerConfig) GetCertificatesInfo() ([]CertificateInfo, error) {
	certsInfoString := cc.GetOrFail(`{.status.controllerCertificates}`)

	logger.Debugf("CERTIFICATES: %s", certsInfoString)

	var certsInfo []CertificateInfo

	jsonerr := json.Unmarshal([]byte(certsInfoString), &certsInfo)

	if jsonerr != nil {
		return nil, jsonerr
	}

	return certsInfo, nil
}

func (cc *ControllerConfig) GetCertificatesInfoByBundleFileName(bundleFile string) ([]CertificateInfo, error) {

	var certsInfo []CertificateInfo

	allCertsInfo, err := cc.GetCertificatesInfo()
	if err != nil {
		return nil, err
	}

	for _, ciLoop := range allCertsInfo {
		ci := ciLoop
		if ci.BundleFile == bundleFile {
			certsInfo = append(certsInfo, ci)
		}
	}

	return certsInfo, nil
}
