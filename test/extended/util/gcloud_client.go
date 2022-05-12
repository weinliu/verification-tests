package util

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"

	o "github.com/onsi/gomega"
)

type Gcloud struct {
	ProjectId string
}

// Login logins to the gcloud. This function needs to be used only once to login into the GCP.
// the gcloud client is only used for the cluster which is on gcp platform.
func (gcloud *Gcloud) Login() *Gcloud {
	checkCred, err := exec.Command("bash", "-c", `gcloud auth list --format="value(account)"`).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if string(checkCred) != "" {
		return gcloud
	}
	credErr := exec.Command("bash", "-c", "gcloud auth login --cred-file=$GOOGLE_APPLICATION_CREDENTIALS").Run()
	o.Expect(credErr).NotTo(o.HaveOccurred())
	projectErr := exec.Command("bash", "-c", fmt.Sprintf("gcloud config set project %s", gcloud.ProjectId)).Run()
	o.Expect(projectErr).NotTo(o.HaveOccurred())
	return gcloud
}

// GetIntSvcExternalIp returns the int svc external IP
func (gcloud *Gcloud) GetIntSvcExternalIp(infraId string) (string, error) {
	externalIp, err := exec.Command("bash", "-c", fmt.Sprintf(`gcloud compute instances list --filter="%s-int-svc"  --format="value(EXTERNAL_IP)"`, infraId)).Output()
	if string(externalIp) == "" {
		return "", errors.New("additional VM is not found")
	}
	return strings.Trim(string(externalIp), "\n"), err
}

// GetIntSvcInternalIp returns the int svc internal IP
func (gcloud *Gcloud) GetIntSvcInternalIp(infraId string) (string, error) {
	internalIp, err := exec.Command("bash", "-c", fmt.Sprintf(`gcloud compute instances list --filter="%s-int-svc"  --format="value(networkInterfaces.networkIP)"`, infraId)).Output()
	if string(internalIp) == "" {
		return "", errors.New("additional VM is not found")
	}
	return strings.Trim(string(internalIp), "\n"), err
}

// GetFirewallAllowPorts returns firewall allow ports
func (gcloud *Gcloud) GetFirewallAllowPorts(ruleName string) (string, error) {
	ports, err := exec.Command("bash", "-c", fmt.Sprintf(`gcloud compute firewall-rules list --filter="name=(%s)" --format="value(ALLOW)"`, ruleName)).Output()
	return strings.Trim(string(ports), "\n"), err
}

// UpdateFirewallAllowPorts updates the firewall allow ports
func (gcloud *Gcloud) UpdateFirewallAllowPorts(ruleName string, ports string) error {
	return exec.Command("bash", "-c", fmt.Sprintf(`gcloud compute firewall-rules update %s --allow %s`, ruleName, ports)).Run()
}

// get zone information for an instance
func (gcloud *Gcloud) GetZone(infraId string, workerName string) (string, error) {
	output, err := exec.Command("bash", "-c", fmt.Sprintf(`gcloud compute instances list --filter="%s" --format="value(ZONE)"`, workerName)).Output()
	if string(output) == "" {
		return "", errors.New("Zone info for the instance is not found")
	}
	return string(output), err
}

// Bring GCP node/instance back up
func (gcloud *Gcloud) StartInstance(nodeName string, zoneName string) error {
	return exec.Command("bash", "-c", fmt.Sprintf(`gcloud compute instances start %s --zone=%s`, nodeName, zoneName)).Run()
}

// Shutdown GCP node/instance
func (gcloud *Gcloud) StopInstance(nodeName string, zoneName string) error {
	return exec.Command("bash", "-c", fmt.Sprintf(`gcloud compute instances stop %s --zone=%s`, nodeName, zoneName)).Run()
}
