package operatorsdk

import (
	"fmt"

	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"
	container "github.com/openshift/openshift-tests-private/test/extended/util/container"
)

func buildPushOperatorImage(architecture, tmpPath, imageTag, tokenDir string) {

	g.By("Build and Push the operator image with architecture")
	podmanCLI := container.NewPodmanCLI()
	podmanCLI.ExecCommandPath = tmpPath
	switch architecture {
	case "amd64":
		output, err := podmanCLI.Run("build").Args(tmpPath, "--arch", "amd64", "--tag", imageTag, "--authfile", fmt.Sprintf("%s/.dockerconfigjson", tokenDir)).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Successfully"))
	case "arm64":
		output, err := podmanCLI.Run("build").Args(tmpPath, "--arch", "arm64", "--tag", imageTag, "--authfile", fmt.Sprintf("%s/.dockerconfigjson", tokenDir)).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Successfully"))
	}
	output, err := podmanCLI.Run("push").Args(imageTag).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(output).To(o.ContainSubstring("Storing signatures"))

}
