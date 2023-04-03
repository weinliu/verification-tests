package networking

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// get file contents to be modified for Ushift
func getFileContentforUshift(baseDir string, name string) (fileContent string) {
	filePath := filepath.Join(exutil.FixturePath("testdata", "networking", baseDir), name)
	fileOpen, err := os.Open(filePath)
	defer fileOpen.Close()
	if err != nil {
		e2e.Failf("Failed to open file: %s", filePath)
	}
	fileRead, _ := io.ReadAll(fileOpen)
	if err != nil {
		e2e.Failf("Failed to read file: %s", filePath)
	}
	return string(fileRead)
}

// get service yaml file, replace variables as per requirements in ushift and create service post that
func createServiceforUshift(oc *exutil.CLI, svc_pmtrs map[string]string) (err error) {
	e2e.Logf("Getting filecontent")
	ServiceGenericYaml := getFileContentforUshift("microshift", "service-generic.yaml")
	//replace all variables as per createServiceforUshift() arguements
	for rep, value := range svc_pmtrs {
		ServiceGenericYaml = strings.ReplaceAll(ServiceGenericYaml, rep, value)
	}
	svcFileName := "temp-service-" + getRandomString() + ".yaml"
	defer os.Remove(svcFileName)
	os.WriteFile(svcFileName, []byte(ServiceGenericYaml), 0644)
	// create service for Microshift
	_, err = oc.WithoutNamespace().Run("create").Args("-f", svcFileName).Output()
	return err
}

// get generic pod yaml file, replace varibles as per requirements in ushift and create service post that
func createPingPodforUshift(oc *exutil.CLI, pod_pmtrs map[string]string) (err error) {
	PodGenericYaml := getFileContentforUshift("microshift", "ping-for-pod-generic.yaml")
	//replace all variables as per createPodforUshift() arguements
	for rep, value := range pod_pmtrs {
		PodGenericYaml = strings.ReplaceAll(PodGenericYaml, rep, value)
	}
	podFileName := "temp-ping-pod-" + getRandomString() + ".yaml"
	defer os.Remove(podFileName)
	os.WriteFile(podFileName, []byte(PodGenericYaml), 0644)
	// create ping pod for Microshift
	_, err = oc.WithoutNamespace().Run("create").Args("-f", podFileName).Output()
	return err
}

func rebootUshiftNode(oc *exutil.CLI, nodeName string) {
	rebootNode(oc, nodeName)
	exec.Command("bash", "-c", "sleep 60").Output()
	checkNodeStatus(oc, nodeName, "Ready")
}
func setMTU(oc *exutil.CLI, nodeName string, mtu string) {
	_, err := exutil.DebugNodeWithChroot(oc, nodeName, "/bin/bash", "-c", "cd /etc/microshift && cp ovn.yaml.default ovn.yaml && echo mtu: "+mtu+" >> ovn.yaml")
	o.Expect(err).NotTo(o.HaveOccurred())

	g.By("reboot node")
	rebootUshiftNode(oc, nodeName)
}

func rollbackMTU(oc *exutil.CLI, nodeName string) {
	_, err := exutil.DebugNodeWithChroot(oc, nodeName, "/bin/bash", "-c", "rm -f /etc/microshift/ovn.yaml")
	o.Expect(err).NotTo(o.HaveOccurred())

	g.By("reboot node")
	rebootUshiftNode(oc, nodeName)
}
