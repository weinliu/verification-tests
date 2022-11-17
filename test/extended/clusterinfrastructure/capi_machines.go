package clusterinfrastructure

import (
	"path/filepath"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

var _ = g.Describe("[sig-cluster-lifecycle] Cluster_Infrastructure", func() {
	defer g.GinkgoRecover()
	var (
		oc                         = exutil.NewCLI("capi-machines", exutil.KubeConfigPath())
		iaasPlatform               string
		clusterID                  string
		region                     string
		profile                    string
		zone                       string
		ami                        string
		subnetName                 string
		sgName                     string
		capiBaseDir                string
		clusterTemplate            string
		awsClusterTemplate         string
		awsMachineTemplateTemplate string
		capiMachinesetTemplate     string
		err                        error
		cluster                    clusterDescription
		awscluster                 awsClusterDescription
		awsMachineTemplate         awsMachineTemplateDescription
		capiMachineSet             capiMachineSetDescription
	)

	g.BeforeEach(func() {
		iaasPlatform = exutil.CheckPlatform(oc)
		if iaasPlatform == "aws" {
			clusterID, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.infrastructureName}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			region, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.aws.region}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			randomMachinesetName := exutil.GetRandomMachineSetName(oc)
			profile, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, randomMachinesetName, "-n", machineAPINamespace, "-o=jsonpath={.spec.template.spec.providerSpec.value.iamInstanceProfile.id}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			zone, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, randomMachinesetName, "-n", machineAPINamespace, "-o=jsonpath={.spec.template.spec.providerSpec.value.placement.availabilityZone}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			ami, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, randomMachinesetName, "-n", machineAPINamespace, "-o=jsonpath={.spec.template.spec.providerSpec.value.ami.id}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			subnetName, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, randomMachinesetName, "-n", machineAPINamespace, "-o=jsonpath={.spec.template.spec.providerSpec.value.subnet.filters[0].values[0]}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			sgName, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, randomMachinesetName, "-n", machineAPINamespace, "-o=jsonpath={.spec.template.spec.providerSpec.value.securityGroups[0].filters[0].values[0]}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		capiBaseDir = exutil.FixturePath("testdata", "clusterinfrastructure", "capi")
		clusterTemplate = filepath.Join(capiBaseDir, "cluster.yaml")
		awsClusterTemplate = filepath.Join(capiBaseDir, "awscluster.yaml")
		awsMachineTemplateTemplate = filepath.Join(capiBaseDir, "machinetemplate-aws.yaml")
		capiMachinesetTemplate = filepath.Join(capiBaseDir, "machineset.yaml")
		cluster = clusterDescription{
			name:     clusterID,
			template: clusterTemplate,
		}
		awscluster = awsClusterDescription{
			name:     clusterID,
			region:   region,
			template: awsClusterTemplate,
		}
		awsMachineTemplate = awsMachineTemplateDescription{
			name:       "aws-machinetemplate",
			profile:    profile,
			zone:       zone,
			ami:        ami,
			subnetName: subnetName,
			sgName:     sgName,
			template:   awsMachineTemplateTemplate,
		}
		capiMachineSet = capiMachineSetDescription{
			name:        "capi-machineset",
			clusterName: clusterID,
			template:    capiMachinesetTemplate,
		}
		if iaasPlatform == "aws" {
			cluster.kind = "AWSCluster"
			capiMachineSet.kind = "AWSMachineTemplate"
			capiMachineSet.machineTemplateName = awsMachineTemplate.name
		}
	})

	g.It("NonHyperShiftHOST-NonPreRelease-Author:zhsun-High-51071-Create machineset with CAPI on aws [Disruptive][Slow]", func() {
		g.By("Check if cluster api on this platform is supported")
		exutil.SkipConditionally(oc)
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "aws")
		g.By("Check if cluster api is deployed")
		capi, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deploy", "-n", clusterAPINamespace, "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(capi) == 0 {
			g.Skip("Skip for cluster api is not deployed!")
		}

		g.By("Create capi machineset")
		defer cluster.deleteCluster(oc)
		cluster.createCluster(oc)
		defer awscluster.deleteAWSCluster(oc)
		awscluster.createAWSCluster(oc)
		defer awsMachineTemplate.deleteAWSMachineTemplate(oc)
		awsMachineTemplate.createAWSMachineTemplate(oc)

		capiMachineSet.name = "capi-machineset-51071"
		defer capiMachineSet.deleteCapiMachineSet(oc)
		capiMachineSet.createCapiMachineSet(oc)
	})
})
