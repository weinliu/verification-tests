package perfscale

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// getImagestreamImageName Return an imagestream's image repository name
func getImagestreamImageName(oc *exutil.CLI, imagestreamName string) string {
	var imageName string
	imageName = ""

	// Ignore NotFound error, it will return a empty string, then use another image in ocperf.go if the image doesn't exit
	imageRepos, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("is", imagestreamName, "-n", "openshift", "-ojsonpath={.status.dockerImageRepository}").Output()
	if !strings.Contains(imageRepos, "NotFound") {
		imageTags, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("is", imagestreamName, "-n", "openshift", "-ojsonpath={.status.tags[*].tag}").Output()
		imageTagList := strings.Split(imageTags, " ")
		// Because some image stream tag is broken, we need to find which image is available in disconnected cluster.
		for i := 0; i < len(imageTagList); i++ {
			jsonathStr := fmt.Sprintf(`-ojsonpath='{.status.tags[%v].conditions[?(@.status=="False")]}{.status.tags[%v].tag}'`, i, i)
			stdOut, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("is", imagestreamName, "-n", "openshift", jsonathStr).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(stdOut).NotTo(o.BeEmpty())
			e2e.Logf("stdOut is: %v", stdOut)
			if !strings.Contains(stdOut, "NotFound") {
				imageTag := strings.ReplaceAll(stdOut, "'", "")
				imageName = imageRepos + ":" + imageTag
				break
			}

		}

	}
	return imageName
}

func createNSUsingOCCLI(oc *exutil.CLI, namespace string, wg *sync.WaitGroup) {

	err := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", namespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	wg.Done()

}

func checkIfNSIsInExpectedState(oc *exutil.CLI, expectedNum int, nsPattern string) {

	o.Eventually(func() bool {
		stdOut, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ns").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		nsReg := regexp.MustCompile(nsPattern + ".*")
		perfCliNsList := nsReg.FindAllString(stdOut, -1)
		nsNum := len(perfCliNsList)
		e2e.Logf("current ns is: %v", nsNum)
		return nsNum == expectedNum
	}, 120*time.Second, 5*time.Second).Should(o.BeTrue())
}

func createDeploymentServiceUsingOCCLI(oc *exutil.CLI, namespace string, ocpPerfAppService string, ocpPerfAppDeployment string, ocPerfAppImageName string, wg *sync.WaitGroup) {

	exutil.ApplyNsResourceFromTemplate(oc, namespace, "--ignore-unknown-parameters=true", "-f", ocpPerfAppDeployment, "-p", "IMAGENAME="+ocPerfAppImageName)
	err := oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", ocpPerfAppService, "-n", namespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	wg.Done()
}

func checkIfDeploymentIsInExpectedState(oc *exutil.CLI, namespace string, resName string) {
	var (
		isCreated  bool
		desiredNum string
		readyNum   string
	)
	o.Eventually(func() bool {
		kindNames, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", resName, "-n", namespace, "-oname").Output()
		if strings.Contains(kindNames, "NotFound") || strings.Contains(kindNames, "No resources") || len(kindNames) == 0 || err != nil {
			isCreated = false
		} else {
			//deployment/statefulset has been created, but not running, need to compare .status.readyReplicas and  in .status.replicas
			isCreated = true
			readyNum, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args(kindNames, "-n", namespace, "-o=jsonpath={.status.readyReplicas}").Output()
			desiredNum, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args(kindNames, "-n", namespace, "-o=jsonpath={.status.replicas}").Output()
		}
		return isCreated && desiredNum == readyNum
	}, 120*time.Second, time.Second).Should(o.BeTrue())
}

func getResourceUsingOCCLI(oc *exutil.CLI, namespace string, wg *sync.WaitGroup) {

	err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment,sa,secret", "-n", namespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	wg.Done()
}

func scaleDownDeploymentUsingOCCLI(oc *exutil.CLI, namespace string, wg *sync.WaitGroup) {

	err := oc.AsAdmin().WithoutNamespace().Run("scale").Args("deployment", "-n", namespace, "--replicas=0", "--all").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	wg.Done()
}

func deleteNSUsingOCCLI(oc *exutil.CLI, namespace string, wg *sync.WaitGroup) {

	err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("ns", namespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	wg.Done()
}
