package cloudcredential

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"

	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type prometheusQueryResult struct {
	Data struct {
		Result []struct {
			Metric struct {
				Name      string `json:"__name__"`
				Container string `json:"container"`
				Endpoint  string `json:"endpoint"`
				Instance  string `json:"instance"`
				Job       string `json:"job"`
				Mode      string `json:"mode"`
				Namespace string `json:"namespace"`
				Pod       string `json:"pod"`
				Service   string `json:"service"`
			} `json:"metric"`
			Value []interface{} `json:"value"`
		} `json:"result"`
		ResultType string `json:"resultType"`
	} `json:"data"`
	Status string `json:"status"`
}

type credentialsRequest struct {
	name      string
	namespace string
	provider  string
	template  string
}

func getCloudCredentialMode(oc *exutil.CLI) (string, error) {
	var (
		mode           string
		iaasPlatform   string
		rootSecretName string
		err            error
	)
	iaasPlatform, err = getIaasPlatform(oc)
	if err != nil {
		return "", err
	}
	rootSecretName, err = getRootSecretName(oc)
	if err != nil {
		return "", err
	}
	modeInCloudCredential, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("cloudcredential", "cluster", "-o=jsonpath={.spec.credentialsMode}").Output()
	if err != nil {
		return "", err
	}
	if modeInCloudCredential != "Manual" {
		modeInSecretAnnotation, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret", rootSecretName, "-n=kube-system", "-o=jsonpath={.metadata.annotations.cloudcredential\\.openshift\\.io/mode}").Output()
		if err != nil {
			if strings.Contains(modeInSecretAnnotation, "NotFound") {
				if iaasPlatform != "aws" && iaasPlatform != "azure" && iaasPlatform != "gcp" {
					mode = "passthrough"
					return mode, nil
				}
				mode = "credsremoved"
				return mode, nil
			}
			return "", err
		}
		if modeInSecretAnnotation == "insufficient" {
			mode = "degraded"
			return mode, nil
		}
		mode = modeInSecretAnnotation
		return mode, nil
	}
	if iaasPlatform == "aws" {
		if isSTSMode(oc) {
			mode = "manualpodidentity"
			return mode, nil
		}
	}
	mode = "manual"
	return mode, nil
}

func getRootSecretName(oc *exutil.CLI) (string, error) {
	var rootSecretName string

	iaasPlatform, err := getIaasPlatform(oc)
	if err != nil {
		return "", err
	}
	switch iaasPlatform {
	case "aws":
		rootSecretName = "aws-creds"
	case "gcp":
		rootSecretName = "gcp-credentials"
	case "azure":
		rootSecretName = "azure-credentials"
	case "vsphere":
		rootSecretName = "vsphere-creds"
	case "openstack":
		rootSecretName = "openstack-credentials"
	case "ovirt":
		rootSecretName = "ovirt-credentials"
	default:
		e2e.Logf("Unsupport platform: %v", iaasPlatform)
		return "", nil

	}
	return rootSecretName, nil
}

func isSTSMode(oc *exutil.CLI) bool {
	output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret", "installer-cloud-credentials", "-n=openshift-image-registry", "-o=jsonpath={.data.credentials}").Output()
	credentials, _ := base64.StdEncoding.DecodeString(output)
	return strings.Contains(string(credentials), "web_identity_token_file")
}

func getIaasPlatform(oc *exutil.CLI) (string, error) {
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.type}").Output()
	if err != nil {
		return "", err
	}
	iaasPlatform := strings.ToLower(output)
	return iaasPlatform, nil
}

func checkModeInMetric(oc *exutil.CLI, token string, mode string) error {
	var (
		data         prometheusQueryResult
		modeInMetric string
	)
	return wait.Poll(10*time.Second, 3*time.Minute, func() (bool, error) {
		msg, _, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-monitoring", "prometheus-k8s-0", "-c", "prometheus", "--", "curl", "-k", "-H", fmt.Sprintf("Authorization: Bearer %v", token), "https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query?query=cco_credentials_mode").Outputs()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(msg).NotTo(o.BeEmpty())
		json.Unmarshal([]byte(msg), &data)
		modeInMetric = data.Data.Result[0].Metric.Mode
		e2e.Logf("cco mode in metric is %v", modeInMetric)
		if modeInMetric != mode {
			e2e.Logf("cco mode should be %v, but is %v in metric", mode, modeInMetric)
			return false, nil
		}
		return true, nil
	})
}

func checkSTSStyle(oc *exutil.CLI, mode string) bool {
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret", "installer-cloud-credentials", "-n", "openshift-image-registry", "-o=jsonpath={.data.credentials}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(output).NotTo(o.BeEmpty())
	credentials, _ := base64.StdEncoding.DecodeString(output)
	credConfig := strings.Split(string(credentials), "\n")
	if len(credConfig) != 3 {
		return false
	}
	if mode == "manualpodidentity" {
		return strings.Contains(credConfig[0], "[default]") && strings.Contains(credConfig[1], "role_arn") && strings.Contains(credConfig[2], "web_identity_token_file")
	}
	return strings.Contains(credConfig[0], "[default]") && strings.Contains(credConfig[1], "aws_access_key_id") && strings.Contains(credConfig[2], "aws_secret_access_key")
}

func patchResourceAsAdmin(oc *exutil.CLI, ns, resource, rsname, patch string) {
	err := oc.AsAdmin().WithoutNamespace().Run("patch").Args(resource, rsname, "--type=json", "-p", patch, "-n", ns).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (cr *credentialsRequest) create(oc *exutil.CLI) {
	exutil.ApplyNsResourceFromTemplate(oc, "openshift-cloud-credential-operator", "--ignore-unknown-parameters=true", "-f", cr.template, "-p", "NAME="+cr.name, "NAMESPACE="+cr.namespace, "PROVIDER="+cr.provider)
}

//Check if CCO conditions are health
func checkCCOHealth(oc *exutil.CLI, mode string) {
	availableStatus, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co", "cloud-credential", `-o=jsonpath={.status.conditions[?(@.type=="Available")].status}`).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(availableStatus).To(o.Equal("True"))
	degradedStatus, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co", "cloud-credential", `-o=jsonpath={.status.conditions[?(@.type=="Degraded")].status}`).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(degradedStatus).To(o.Equal("False"))
	progressingStatus, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co", "cloud-credential", `-o=jsonpath={.status.conditions[?(@.type=="Progressing")].status}`).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(progressingStatus).To(o.Equal("False"))
	upgradeableStatus, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co", "cloud-credential", `-o=jsonpath={.status.conditions[?(@.type=="Upgradeable")].status}`).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	//when cco mode is manual or manual+sts, upgradeableStatus is "False" due to MissingUpgradeableAnnotation
	if mode == "manual" || mode == "manualpodidentity" {
		o.Expect(upgradeableStatus).To(o.Equal("False"))
		upgradeableReason, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co", "cloud-credential", `-o=jsonpath={.status.conditions[?(@.type=="Upgradeable")].reason}`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(upgradeableReason).To(o.Equal("MissingUpgradeableAnnotation"))
	} else {
		o.Expect(upgradeableStatus).To(o.Equal("True"))
	}
}
