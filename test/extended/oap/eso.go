package oap

import (
	"encoding/base64"
	"path/filepath"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-oap] OAP eso", func() {
	defer g.GinkgoRecover()

	var (
		oc                  = exutil.NewCLI("eso", exutil.KubeConfigPath())
		buildPruningBaseDir = exutil.FixturePath("testdata", "oap/eso")
	)
	g.BeforeEach(func() {

		g.Skip("Skip testing until downstream operator build is handed over")

		if !isDeploymentReady(oc, esoNamespace, esoDeploymentName) {
			e2e.Logf("Creating External Secrets Operator...")
			createExternalSecretsOperator(oc)
		}

	})

	// author: jitli@redhat.com
	g.It("Author:jitli-ROSA-ConnectedOnly-High-80066-Get the secret value from AWS Secrets Manager", func() {

		exutil.SkipIfPlatformTypeNot(oc, "AWS")
		if exutil.IsSTSCluster(oc) {
			g.Skip("Skip for STS cluster")
		}
		exutil.SkipOnProxyCluster(oc)

		var (
			awsSecretName      = "aws-creds"
			secretstoreName    = "secretstore-80066"
			externalsecretName = "externalsecret-80066"
			secretRegion       = "us-east-2"
			ns                 = oc.Namespace()
		)

		exutil.By("Create secret that contains AWS accessKey")
		defer func() {
			e2e.Logf("Cleanup the created secret")
			err := oc.AsAdmin().Run("delete").Args("-n", ns, "secret", awsSecretName).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		accessKeyID, secureKey := getCredentialFromCluster(oc, "aws")
		oc.NotShowInfo()
		err := oc.AsAdmin().Run("create").Args("-n", ns, "secret", "generic", awsSecretName, "--from-literal=access-key="+accessKeyID, "--from-literal=secret-access-key="+secureKey).Execute()
		oc.SetShowInfo()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create secret store")
		secretStoreTemplate := filepath.Join(buildPruningBaseDir, "secretstore.yaml")
		params := []string{"-f", secretStoreTemplate, "-p", "NAME=" + secretstoreName, "REGION=" + secretRegion, "SECRETNAME=" + awsSecretName}
		exutil.ApplyNsResourceFromTemplate(oc, ns, params...)
		err = waitForResourceReadiness(oc, ns, "secretstore", secretstoreName, 10*time.Second, 120*time.Second)
		if err != nil {
			dumpResource(oc, ns, "secretstore", secretstoreName, "-o=yaml")
		}
		exutil.AssertWaitPollNoErr(err, "timeout waiting for secretstore to become Ready")

		exutil.By("Create external secret")
		externalSecretTemplate := filepath.Join(buildPruningBaseDir, "externalsecret.yaml")
		params = []string{"-f", externalSecretTemplate, "-p", "NAME=" + externalsecretName, "REFREASHINTERVAL=" + "1m", "SECRETSTORENAME=" + secretstoreName, "SECRETNAME=" + "secret-from-awssm"}
		exutil.ApplyNsResourceFromTemplate(oc, ns, params...)
		err = waitForResourceReadiness(oc, ns, "externalsecret", externalsecretName, 10*time.Second, 120*time.Second)
		if err != nil {
			dumpResource(oc, ns, "externalsecret", externalsecretName, "-o=yaml")
		}
		exutil.AssertWaitPollNoErr(err, "timeout waiting for externalsecret to become Ready")

		exutil.By("Check the secret exists and verify the secret content")
		data, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ns, "secret", "secret-from-awssm", "-o=jsonpath={.data.secret-value-from-awssm}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		value, err := base64.StdEncoding.DecodeString(data)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(value).To(o.ContainSubstring(`{"username":"jitli","password":"qwertyuioplkj"}`))

	})

})
