package baremetal

import (
	"encoding/base64"
	"fmt"
	"os/exec"
	"regexp"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// var _ = g.Describe("[sig-baremetal] INSTALLER UPI for INSTALLER_GENERAL job on BareMetal", func() {
// 	defer g.GinkgoRecover()
// 	var (
// 		oc           = exutil.NewCLI("baremetal-ironic-authentication", exutil.KubeConfigPath())

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
// 		oc           = exutil.NewCLI("baremetal-ironic-authentication", exutil.KubeConfigPath())

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
		oc              = exutil.NewCLI("baremetal-ironic-authentication", exutil.KubeConfigPath())
		iaasPlatform    string
		endpointIP      []string
		encodedUserPass string
	)
	g.BeforeEach(func() {
		exutil.SkipForSNOCluster(oc)
		iaasPlatform = exutil.CheckPlatform(oc)
		if !(iaasPlatform == "baremetal") {
			e2e.Logf("Cluster is: %s", iaasPlatform)
			g.Skip("For Non-baremetal cluster , this is not supported!")
		}

		user := getUserFromSecret(oc, machineAPINamespace, "metal3-ironic-password")
		pass := getPassFromSecret(oc, machineAPINamespace, "metal3-ironic-password")
		encodedUserPass = base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))

		metal3Pod, err := oc.AsAdmin().Run("get").Args("-n", machineAPINamespace, "pods", "-l baremetal.openshift.io/cluster-baremetal-operator=metal3-state", "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).ShouldNot(o.HaveOccurred())
		endpoint, err := oc.AsAdmin().Run("exec").Args("-n", machineAPINamespace, metal3Pod, "-c", "metal3-ironic", "--", "cat", "/etc/ironic/ironic.conf").Output()
		o.Expect(err).ShouldNot(o.HaveOccurred())
		re := regexp.MustCompile(`public_endpoint\s*=\s*https://(\d+\.\d+\.\d+\.\d+:\d+)`)
		endpointIP = re.FindStringSubmatch(endpoint)

	})

	// author: jhajyahy@redhat.com
	g.It("Author:jhajyahy-Medium-40655-An unauthenticated user can't do actions in the ironic-api when using --insecure flag with https", func() {
		curlCmd := `curl -k -I -X get "https://%s/v1/nodes"`
		formattedCmd := fmt.Sprintf(curlCmd, endpointIP[1])
		out, cmdErr := exec.Command("bash", "-c", formattedCmd).Output()
		o.Expect(cmdErr).ShouldNot(o.HaveOccurred())
		o.Expect(out).Should(o.ContainSubstring("HTTP/1.1 401 Unauthorized"))
	})

	// author: jhajyahy@redhat.com
	g.It("Author:jhajyahy-Medium-40560-An unauthenticated user can't do actions in the ironic-api when using http", func() {
		curlCmd := `curl -I -X get "http://%s/v1/nodes"`
		formattedCmd := fmt.Sprintf(curlCmd, endpointIP[1])
		out, cmdErr := exec.Command("bash", "-c", formattedCmd).Output()
		o.Expect(cmdErr).Should(o.HaveOccurred())
		o.Expect(out).ShouldNot(o.ContainSubstring("HTTP/1.1 200 OK"))
		o.Expect(cmdErr.Error()).Should(o.ContainSubstring("exit status 52"))
	})

	// author: jhajyahy@redhat.com
	g.It("Author:jhajyahy-Medium-40561-An authenticated user can't do actions in the ironic-api when using http", func() {
		curlCmd := `curl	 -I -X get --header "Authorization: Basic %s" "http://%s/v1/nodes"`
		formattedCmd := fmt.Sprintf(curlCmd, encodedUserPass, endpointIP[1])
		out, cmdErr := exec.Command("bash", "-c", formattedCmd).Output()
		o.Expect(cmdErr).Should(o.HaveOccurred())
		o.Expect(out).ShouldNot(o.ContainSubstring("HTTP/1.1 200 OK"))
		o.Expect(cmdErr.Error()).Should(o.ContainSubstring("exit status 52"))
	})

	// author: jhajyahy@redhat.com
	g.It("Author:jhajyahy-Medium-40562-An authenticated user can do actions in the ironic-api when using --insecure flag with https", func() {
		curlCmd := `curl -I -k -X get --header "Authorization: Basic %s" "https://%s/v1/nodes"`
		formattedCmd := fmt.Sprintf(curlCmd, encodedUserPass, endpointIP[1])
		out, cmdErr := exec.Command("bash", "-c", formattedCmd).Output()
		o.Expect(cmdErr).ShouldNot(o.HaveOccurred())
		o.Expect(out).Should(o.ContainSubstring("HTTP/1.1 200 OK"))
	})
})

// var _ = g.Describe("[sig-baremetal] INSTALLER IPI for INSTALLER_DEDICATED job on BareMetal", func() {
// 	defer g.GinkgoRecover()
// 	var (
// 		oc           = exutil.NewCLI("baremetal-ironic-authentication", exutil.KubeConfigPath())

// 	)
// 	g.BeforeEach(func() {

// 	})

// 	g.AfterEach(func() {

// 	})

// 	// author: sgoveas@redhat.com
// 	g.It("Author:sgoveas--Medium-12345-example case", func() {

// 	})

// })
