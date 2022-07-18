package util

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	o "github.com/onsi/gomega"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// A Osp represents object ...
type Osp struct {
}

// GetOspInstance represents to list osp instance ...
func (osp *Osp) GetOspInstance(instanceName string) (string, error) {
	cmd := fmt.Sprintf("openstack --os-auth-url %s --os-password %s --os-project-id %s --os-username %s --os-domain-name %s server list --name %s -c Name -f value", os.Getenv("OSP_DR_AUTH_URL"), os.Getenv("OSP_DR_PASSWORD"), os.Getenv("OSP_DR_PROJECT_ID"), os.Getenv("OSP_DR_USERNAME"), os.Getenv("OSP_DR_USER_DOMAIN_NAME"), instanceName)
	instance, err := exec.Command("bash", "-c", cmd).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if string(instance) == "" {
		return "", fmt.Errorf("VM is not found")
	}
	e2e.Logf("Virtual machines instance found:", strings.Trim(string(instance), "\n"))
	return strings.Trim(string(instance), "\n"), err
}

// GetOspInstanceState represents to list osp instance state ...
func (osp *Osp) GetOspInstanceState(instanceName string) (string, error) {
	cmd := fmt.Sprintf("openstack --os-auth-url %s --os-password %s --os-project-id %s --os-username %s --os-domain-name %s server list --name %s -c Status -f value", os.Getenv("OSP_DR_AUTH_URL"), os.Getenv("OSP_DR_PASSWORD"), os.Getenv("OSP_DR_PROJECT_ID"), os.Getenv("OSP_DR_USERNAME"), os.Getenv("OSP_DR_USER_DOMAIN_NAME"), instanceName)
	instanceState, err := exec.Command("bash", "-c", cmd).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if string(instanceState) == "" {
		return "", fmt.Errorf("Not able to get vm instance state")
	}
	return strings.Trim(string(instanceState), "\n"), err
}

// GetStopOspInstance represents to stop osp instance ...
func (osp *Osp) GetStopOspInstance(instanceName string) error {
	cmd := fmt.Sprintf("openstack --os-auth-url %s --os-password %s --os-project-id %s --os-username %s --os-domain-name %s server pause  %s", os.Getenv("OSP_DR_AUTH_URL"), os.Getenv("OSP_DR_PASSWORD"), os.Getenv("OSP_DR_PROJECT_ID"), os.Getenv("OSP_DR_USERNAME"), os.Getenv("OSP_DR_USER_DOMAIN_NAME"), instanceName)
	_, err := exec.Command("bash", "-c", cmd).Output()
	if err != nil {
		return fmt.Errorf("Not able to stop VM")
	}
	return err
}

// GetStartOspInstance represents to start osp instance ...
func (osp *Osp) GetStartOspInstance(instanceName string) error {
	cmd := fmt.Sprintf("openstack --os-auth-url %s --os-password %s --os-project-id %s --os-username %s --os-domain-name %s server unpause  %s", os.Getenv("OSP_DR_AUTH_URL"), os.Getenv("OSP_DR_PASSWORD"), os.Getenv("OSP_DR_PROJECT_ID"), os.Getenv("OSP_DR_USERNAME"), os.Getenv("OSP_DR_USER_DOMAIN_NAME"), instanceName)
	_, err := exec.Command("bash", "-c", cmd).Output()
	if err != nil {
		return fmt.Errorf("Not able to start VM")
	}
	return err
}
