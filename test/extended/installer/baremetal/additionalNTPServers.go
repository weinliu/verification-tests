package baremetal

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// var _ = g.Describe("[sig-baremetal] INSTALLER UPI for INSTALLER_GENERAL job on BareMetal", func() {
// 	defer g.GinkgoRecover()
// 	var (
// 		oc           = exutil.NewCLI("cluster-baremetal-operator", exutil.KubeConfigPath())

// 	)
// 	g.BeforeEach(func() {

// 	})

// 	g.AfterEach(func() {

// 	})

// 	// author: sgoveas@redhat.com
// 	g.It("Author:sgoveas--Medium-12345-example case", func() {

// 	})

// })

// var _ = g.Describe("[sig-baremetal] INSTALLER UPI for INSTALLER_DEDICATED job on BareMetal", func() {
// 	defer g.GinkgoRecover()
// 	var (
// 		oc           = exutil.NewCLI("cluster-baremetal-operator", exutil.KubeConfigPath())

// 	)
// 	g.BeforeEach(func() {

// 	})

// 	g.AfterEach(func() {

// 	})

// 	// author: sgoveas@redhat.com
// 	g.It("Author:sgoveas--Medium-12345-example case", func() {

// 	})

// })

// var _ = g.Describe("[sig-baremetal] INSTALLER IPI for INSTALLER_GENERAL job on BareMetal", func() {
// 	defer g.GinkgoRecover()
// 	var (
// 		oc           = exutil.NewCLI("cluster-baremetal-operator", exutil.KubeConfigPath())

// 	)
// 	g.BeforeEach(func() {

// 	})

// 	g.AfterEach(func() {

// 	})

// 	// author: sgoveas@redhat.com
// 	g.It("Author:sgoveas--Medium-12345-example case", func() {

// 	})

// })

var _ = g.Describe("[sig-baremetal] INSTALLER IPI for INSTALLER_GENERAL job on BareMetal", func() {
	defer g.GinkgoRecover()
	var (
		oc           = exutil.NewCLI("additional-ntp-servers", exutil.KubeConfigPath())
		iaasPlatform string
	)

	g.BeforeEach(func() {
		exutil.SkipForSNOCluster(oc)
		iaasPlatform = exutil.CheckPlatform(oc)
		if !(iaasPlatform == "baremetal") {
			e2e.Logf("Cluster is: %s", iaasPlatform)
			g.Skip("This feature is not supported for Non-baremetal cluster!")
		}

		clusterVersion, _, err := exutil.GetClusterVersion(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		if CompareVersions(clusterVersion, "<", "4.18") {
			g.Skip("This feature is not supported for cluster version less than 4.18!")
		}
	})

	// author: sgoveas@redhat.com
	g.It("Author:sgoveas-NonPreRelease-Medium-79243-Check Additional NTP servers were added in install-config.yaml", func() {
		exutil.By("1) Get the internal NTP server")
		ntpHost := "aux-host-internal-name"
		ntpFile := filepath.Join(os.Getenv(clusterProfileDir), ntpHost)
		ntpServersList, err := ioutil.ReadFile(ntpFile)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("2) Check additionalNTPServer was added to install-config.yaml")
		installConfig, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", "-n", "kube-system", "cluster-config-v1", "-o=jsonpath={.data.install-config}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		yqCmd := fmt.Sprintf(`echo "%s" | yq .platform.baremetal.additionalNTPServers`, installConfig)
		ntpList, err := exec.Command("bash", "-c", yqCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		if !strings.Contains(string(ntpList), string(ntpServersList)) {
			e2e.Failf("Additional NTP server was not added to install-config.yaml", err)
		}
	})
})
