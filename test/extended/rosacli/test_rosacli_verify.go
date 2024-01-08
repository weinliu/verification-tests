package rosacli

import (
	"fmt"
	"io"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	rosacli "github.com/openshift/openshift-tests-private/test/extended/util/rosacli"

	nets "net/http"
)

var _ = g.Describe("[sig-rosacli] Cluster_Management_Service verify test", func() {
	defer g.GinkgoRecover()

	var (
		rosaSensitiveClient *rosacli.Client
	)

	g.BeforeEach(func() {
		rosaSensitiveClient = rosacli.NewSensitiveClient()
	})

	g.It("Author:yingzhan-Medium-64040-rosacli testing: Test rosa window certificates expiration [Serial]", func() {
		//If the case fails,please open a card to ask dev update windows certificates.
		//Example card: https://issues.redhat.com/browse/SDA-8990
		g.By("Get ROSA windows certificates on ocm-sdk repo")
		sdkCAFileURL := "https://raw.githubusercontent.com/openshift-online/ocm-sdk-go/main/internal/system_cas_windows.go"
		resp, err := nets.Get(sdkCAFileURL)
		o.Expect(err).ToNot(o.HaveOccurred())
		defer resp.Body.Close()
		content, err := io.ReadAll(resp.Body)
		o.Expect(err).ToNot(o.HaveOccurred())
		sdkContent := string(content)

		g.By("Check the domains certificates if it is updated")
		domains := []string{"api.openshift.com", "sso.redhat.com"}
		for _, url := range domains {
			cmd := fmt.Sprintf("openssl s_client -connect %s:443 -showcerts 2>&1  | sed -ne '/-BEGIN CERTIFICATE-/,/-END CERTIFICATE-/p'", url)
			stdout, err := rosaSensitiveClient.Runner.RunCMD([]string{"bash", "-c", cmd})
			o.Expect(err).ToNot(o.HaveOccurred())
			result := strings.Trim(stdout.String(), "\n")
			ca := strings.Split(result, "-----END CERTIFICATE-----")
			o.Expect(strings.Contains(sdkContent, ca[0])).Should(o.BeTrue())
			o.Expect(strings.Contains(sdkContent, ca[1])).Should(o.BeTrue())
		}

	})

})
