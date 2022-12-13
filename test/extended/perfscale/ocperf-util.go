package perfscale

import (
	"fmt"
	"strings"

	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// getImagestreamImageName Return an imagestream's image repository name
func getImagestreamImageName(oc *exutil.CLI, imagestreamName string) string {
	var imageName string
	imageName = ""

	//Ignore NotFound error, it will return a empty string, then use another image in ocperf.go if the image doesn't exit
	imageRepos, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("is", imagestreamName, "-n", "openshift", "-ojsonpath={.status.dockerImageRepository}").Output()
	if !strings.Contains(imageRepos, "NotFound") {
		imageTags, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("is", imagestreamName, "-n", "openshift", "-ojsonpath={.status.tags[*].tag}").Output()
		imageTagList := strings.Split(imageTags, " ")
		//Because some image stream tag is broken, we need to find which image is available in disconnected cluster.
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
