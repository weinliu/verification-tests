package hive

import (
	"context"
	"fmt"
	"path/filepath"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"github.com/openshift/openshift-tests-private/test/extended/util/architecture"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

/*
Notes:
1) VSphere test cases are meant to be tested locally (instead of on Jenkins).
In fact, an additional set of AWS credentials are required for DNS setup,
and those credentials are loaded using external AWS configurations (which
are only available locally) when running in non-CI environments.
2) A stable VPN connection is required for running vSphere test cases locally.
*/
var _ = g.Describe("[sig-hive] Cluster_Operator hive should", func() {
	defer g.GinkgoRecover()

	var (
		// Clients
		oc = exutil.NewCLI("hive", exutil.KubeConfigPath())

		// Test-specific
		testDataDir  string
		testOCPImage string
		randStr      string

		// Platform-specific
		datacenter string
		datastore  string
		network    string
		vCenter    string
		cluster    string
		basedomain string
	)

	// Under the hood, "extended-platform-tests run" calls "extended-platform-tests run-test" on each test
	// case separately. This means that all necessary initializations need to be done before every single
	// test case, either globally or in a Ginkgo node like BeforeEach.
	g.BeforeEach(func() {
		// Skip if non-compatible platforms
		exutil.SkipIfPlatformTypeNot(oc, "vsphere")
		architecture.SkipNonAmd64SingleArch(oc)

		// Install Hive operator if non-existent
		testDataDir = exutil.FixturePath("testdata", "cluster_operator/hive")
		_, _ = installHiveOperator(oc, &hiveNameSpace{}, &operatorGroup{}, &subscription{}, &hiveconfig{}, testDataDir)

		// Get OCP release image used for provisioning
		testOCPImage = getTestOCPImage()

		// Get random string
		randStr = getRandomString()[:ClusterSuffixLen]

		// Get platform info
		basedomain = getBasedomain(oc)
		infrastructure, err := oc.
			AdminConfigClient().
			ConfigV1().
			Infrastructures().
			Get(context.Background(), "cluster", metav1.GetOptions{})
		o.Expect(err).NotTo(o.HaveOccurred())
		failureDomains := infrastructure.Spec.PlatformSpec.VSphere.FailureDomains
		datacenter = failureDomains[0].Topology.Datacenter
		datastore = failureDomains[0].Topology.Datastore
		network = failureDomains[0].Topology.Networks[0]
		vCenter = failureDomains[0].Server
		cluster = failureDomains[0].Topology.ComputeCluster
		e2e.Logf(fmt.Sprintf(`Found platform configurations:
1) Datacenter: %s
2) Datastore: %s
3) Network: %s
4) vCenter Server: %s
5) Cluster: %s
6) Base domain: %s`, datacenter, datastore, network, vCenter, cluster, basedomain))
	})

	// Author: fxie@redhat.com
	// Timeout: 60min
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Author:fxie-High-32026-Add hive api for vsphere provisioning [Serial]", func() {
		var (
			testCaseID                = "32026"
			cdName                    = fmt.Sprintf("cd-%s-%s", testCaseID, randStr)
			icSecretName              = fmt.Sprintf("%s-install-config", cdName)
			imageSetName              = fmt.Sprintf("%s-imageset", cdName)
			networkCIDR, minIp, maxIp = getVSphereCIDR(network)
			machineNetwork            = networkCIDR
			apiDomain                 = fmt.Sprintf("api.%v.%v", cdName, basedomain)
			ingressDomain             = fmt.Sprintf("*.apps.%v.%v", cdName, basedomain)
			domains2Reserve           = []string{apiDomain, ingressDomain}
		)

		exutil.By("Extracting root credentials")
		username, password := getVSphereCredentials(oc, vCenter)

		exutil.By(fmt.Sprintf("Reserving API/ingress IPs for domains %v", domains2Reserve))
		fReserve, fRelease, domain2Ip := getIps2ReserveFromAWSHostedZone(oc, basedomain,
			networkCIDR, minIp, maxIp, domains2Reserve)
		defer fRelease()
		fReserve()

		exutil.By("Creating ClusterDeployment and related resources")
		installConfigSecret := vSphereInstallConfig{
			secretName:     icSecretName,
			secretNs:       oc.Namespace(),
			baseDomain:     basedomain,
			icName:         cdName,
			cluster:        cluster,
			machineNetwork: machineNetwork,
			apiVip:         domain2Ip[apiDomain],
			datacenter:     datacenter,
			datastore:      datastore,
			ingressVip:     domain2Ip[ingressDomain],
			network:        network,
			password:       password,
			username:       username,
			vCenter:        vCenter,
			template:       filepath.Join(testDataDir, "vsphere-install-config.yaml"),
		}
		cd := vSphereClusterDeployment{
			fake:                 false,
			name:                 cdName,
			namespace:            oc.Namespace(),
			baseDomain:           basedomain,
			manageDns:            false,
			clusterName:          cdName,
			certRef:              VSphereCerts,
			cluster:              cluster,
			credRef:              VSphereCreds,
			datacenter:           datacenter,
			datastore:            datastore,
			network:              network,
			vCenter:              vCenter,
			imageSetRef:          imageSetName,
			installConfigSecret:  icSecretName,
			pullSecretRef:        PullSecret,
			installAttemptsLimit: 1,
			template:             filepath.Join(testDataDir, "clusterdeployment-vsphere.yaml"),
		}
		defer cleanCD(oc, imageSetName, oc.Namespace(), installConfigSecret.secretName, cd.name)
		createCD(testDataDir, testOCPImage, oc, oc.Namespace(), installConfigSecret, cd)

		exutil.By("Waiting for the CD to be installed")
		newCheck("expect", "get", asAdmin, requireNS, compare, "true", ok,
			ClusterInstallTimeout, []string{"ClusterDeployment", cdName, "-o=jsonpath={.spec.installed}"}).check(oc)
	})
})
