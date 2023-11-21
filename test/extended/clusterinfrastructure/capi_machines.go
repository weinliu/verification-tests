package clusterinfrastructure

import (
	"io/ioutil"
	"path/filepath"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"github.com/tidwall/sjson"
)

var _ = g.Describe("[sig-cluster-lifecycle] Cluster_Infrastructure", func() {
	defer g.GinkgoRecover()
	var (
		oc                         = exutil.NewCLI("capi-machines", exutil.KubeConfigPath())
		iaasPlatform               string
		clusterID                  string
		region                     string
		host                       string
		profile                    string
		instanceType               string
		zone                       string
		ami                        string
		subnetName                 string
		subnetID                   string
		sgName                     string
		image                      string
		machineType                string
		capiBaseDir                string
		clusterTemplate            string
		awsClusterTemplate         string
		awsMachineTemplateTemplate string
		gcpClusterTemplate         string
		gcpMachineTemplateTemplate string
		capiMachinesetAWSTemplate  string
		capiMachinesetgcpTemplate  string
		err                        error
		cluster                    clusterDescription
		awscluster                 awsClusterDescription
		awsMachineTemplate         awsMachineTemplateDescription
		gcpcluster                 gcpClusterDescription
		gcpMachineTemplate         gcpMachineTemplateDescription
		capiMachineSetAWS          capiMachineSetAWSDescription
		capiMachineSetgcp          capiMachineSetgcpDescription
	)

	g.BeforeEach(func() {
		exutil.SkipForSNOCluster(oc)
		exutil.SkipConditionally(oc)
		iaasPlatform = exutil.CheckPlatform(oc)
		switch iaasPlatform {
		case "aws":
			region, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.aws.region}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			apiServerInternalURI, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.apiServerInternalURI}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			start := strings.Index(apiServerInternalURI, "://")
			end := strings.LastIndex(apiServerInternalURI, ":")
			host = apiServerInternalURI[start+3 : end]
			randomMachinesetName := exutil.GetRandomMachineSetName(oc)
			profile, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, randomMachinesetName, "-n", machineAPINamespace, "-o=jsonpath={.spec.template.spec.providerSpec.value.iamInstanceProfile.id}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			instanceType, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, randomMachinesetName, "-n", machineAPINamespace, "-o=jsonpath={.spec.template.spec.providerSpec.value.instanceType}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			zone, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, randomMachinesetName, "-n", machineAPINamespace, "-o=jsonpath={.spec.template.spec.providerSpec.value.placement.availabilityZone}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			ami, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, randomMachinesetName, "-n", machineAPINamespace, "-o=jsonpath={.spec.template.spec.providerSpec.value.ami.id}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			subnetName, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, randomMachinesetName, "-n", machineAPINamespace, "-o=jsonpath={.spec.template.spec.providerSpec.value.subnet.filters[0].values[0]}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			subnetID, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, randomMachinesetName, "-n", machineAPINamespace, "-o=jsonpath={.spec.template.spec.providerSpec.value.subnet.id}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			sgName, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, randomMachinesetName, "-n", machineAPINamespace, "-o=jsonpath={.spec.template.spec.providerSpec.value.securityGroups[0].filters[0].values[0]}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
		case "gcp":
			region, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.gcp.region}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			randomMachinesetName := exutil.GetRandomMachineSetName(oc)
			zone, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, randomMachinesetName, "-n", machineAPINamespace, "-o=jsonpath={.spec.template.spec.providerSpec.value.zone}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			image, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, randomMachinesetName, "-n", machineAPINamespace, "-o=jsonpath={.spec.template.spec.providerSpec.value.disks[0].image}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			machineType, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, randomMachinesetName, "-n", machineAPINamespace, "-o=jsonpath={.spec.template.spec.providerSpec.value.machineType}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
		default:
			g.Skip("IAAS platform is " + iaasPlatform + " which is NOT supported cluster api ...")
		}
		clusterID, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.infrastructureName}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		capiBaseDir = exutil.FixturePath("testdata", "clusterinfrastructure", "capi")
		clusterTemplate = filepath.Join(capiBaseDir, "cluster.yaml")
		awsClusterTemplate = filepath.Join(capiBaseDir, "awscluster.yaml")
		if subnetName != "" {
			awsMachineTemplateTemplate = filepath.Join(capiBaseDir, "machinetemplate-aws.yaml")
		} else {
			awsMachineTemplateTemplate = filepath.Join(capiBaseDir, "machinetemplate-aws-id.yaml")
		}
		gcpClusterTemplate = filepath.Join(capiBaseDir, "gcpcluster.yaml")
		gcpMachineTemplateTemplate = filepath.Join(capiBaseDir, "machinetemplate-gcp.yaml")
		capiMachinesetAWSTemplate = filepath.Join(capiBaseDir, "machinesetaws.yaml")
		capiMachinesetgcpTemplate = filepath.Join(capiBaseDir, "machinesetgcp.yaml")
		cluster = clusterDescription{
			name:     clusterID,
			template: clusterTemplate,
		}
		awscluster = awsClusterDescription{
			name:     clusterID,
			region:   region,
			host:     host,
			template: awsClusterTemplate,
		}
		gcpcluster = gcpClusterDescription{
			name:     clusterID,
			region:   region,
			template: gcpClusterTemplate,
		}
		awsMachineTemplate = awsMachineTemplateDescription{
			name:         "aws-machinetemplate",
			profile:      profile,
			instanceType: instanceType,
			zone:         zone,
			ami:          ami,
			subnetName:   subnetName,
			sgName:       sgName,
			subnetID:     subnetID,
			template:     awsMachineTemplateTemplate,
		}
		gcpMachineTemplate = gcpMachineTemplateDescription{
			name:        "gcp-machinetemplate",
			region:      region,
			image:       image,
			machineType: machineType,
			clusterID:   clusterID,
			template:    gcpMachineTemplateTemplate,
		}
		capiMachineSetAWS = capiMachineSetAWSDescription{
			name:        "capi-machineset",
			clusterName: clusterID,
			template:    capiMachinesetAWSTemplate,
			replicas:    1,
		}
		capiMachineSetgcp = capiMachineSetgcpDescription{
			name:          "capi-machineset-gcp",
			clusterName:   clusterID,
			template:      capiMachinesetgcpTemplate,
			failureDomain: zone,
			replicas:      1,
		}

		switch iaasPlatform {
		case "aws":
			cluster.kind = "AWSCluster"
			capiMachineSetAWS.kind = "AWSMachineTemplate"
			capiMachineSetAWS.machineTemplateName = awsMachineTemplate.name
		case "gcp":
			cluster.kind = "GCPCluster"
			capiMachineSetgcp.kind = "GCPMachineTemplate"
			capiMachineSetgcp.machineTemplateName = gcpMachineTemplate.name
			capiMachineSetgcp.failureDomain = zone

		default:
			g.Skip("IAAS platform is " + iaasPlatform + " which is NOT supported cluster api ...")
		}
	})

	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-Author:zhsun-High-51071-Create machineset with CAPI on aws [Disruptive][Slow]", func() {
		g.By("Check if cluster api on this platform is supported")
		exutil.SkipConditionally(oc)
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "aws")
		skipForCAPINotExist(oc)

		g.By("Create capi machineset")
		cluster.createCluster(oc)
		defer awscluster.deleteAWSCluster(oc)
		awscluster.createAWSCluster(oc)
		defer awsMachineTemplate.deleteAWSMachineTemplate(oc)
		awsMachineTemplate.createAWSMachineTemplate(oc)

		capiMachineSetAWS.name = "capi-machineset-51071"
		defer waitForCapiMachinesDisapper(oc, capiMachineSetAWS.name)
		defer capiMachineSetAWS.deleteCapiMachineSet(oc)
		capiMachineSetAWS.createCapiMachineSet(oc)
	})

	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-Author:zhsun-High-53100-Create machineset with CAPI on gcp [Disruptive][Slow]", func() {
		exutil.SkipConditionally(oc)
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "gcp")
		skipForCAPINotExist(oc)

		g.By("Create capi machineset")
		cluster.createCluster(oc)
		defer gcpcluster.deleteGCPCluster(oc)
		gcpcluster.createGCPCluster(oc)

		defer gcpMachineTemplate.deleteGCPMachineTemplate(oc)
		gcpMachineTemplate.createGCPMachineTemplate(oc)

		capiMachineSetgcp.name = "capi-machineset-53100"
		defer waitForCapiMachinesDisapper(oc, capiMachineSetgcp.name)
		defer capiMachineSetgcp.deleteCapiMachineSetgcp(oc)
		capiMachineSetgcp.createCapiMachineSetgcp(oc)

	})

	// author: zhsun@redhat.com
	g.It("NonHyperShiftHOST-Author:zhsun-medium-55205-[CAPI] Webhook validations for CAPI [Disruptive]", func() {
		exutil.SkipConditionally(oc)
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "aws", "gcp")
		skipForCAPINotExist(oc)

		g.By("Shouldn't allow to create/update cluster with invalid kind")
		clusters, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("cluster", "-n", clusterAPINamespace, "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(clusters) == 0 {
			cluster.createCluster(oc)
		}

		clusterKind, _ := oc.AsAdmin().WithoutNamespace().Run("patch").Args("cluster", cluster.name, "-n", clusterAPINamespace, "-p", `{"spec":{"infrastructureRef":{"kind":"invalid"}}}`, "--type=merge").Output()
		o.Expect(clusterKind).To(o.ContainSubstring("invalid"))

		g.By("Shouldn't allow to delete cluster")
		clusterDelete, _ := oc.AsAdmin().WithoutNamespace().Run("delete").Args("cluster", cluster.name, "-n", clusterAPINamespace).Output()
		o.Expect(clusterDelete).To(o.ContainSubstring("deletion of cluster is not allowed"))

		g.By("Core provider name is immutable")
		coreProviderJSON, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("coreprovider/cluster-api", "-n", clusterAPINamespace, "-o=json").OutputToFile("coreprovider.json")
		o.Expect(err).NotTo(o.HaveOccurred())
		bytes, _ := ioutil.ReadFile(coreProviderJSON)
		coreProviderName, _ := sjson.Set(string(bytes), "metadata.name", "cluster-api-2")
		err = ioutil.WriteFile(coreProviderJSON, []byte(coreProviderName), 0644)
		o.Expect(err).NotTo(o.HaveOccurred())
		coreProviderNameUpdate, _ := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", coreProviderJSON).Output()
		o.Expect(coreProviderNameUpdate).To(o.ContainSubstring("incorrect core provider name: cluster-api-2"))

		g.By("Shouldn't allow to delete coreprovider")
		coreProviderDelete, _ := oc.AsAdmin().WithoutNamespace().Run("delete").Args("coreprovider", "cluster-api", "-n", clusterAPINamespace).Output()
		o.Expect(coreProviderDelete).To(o.ContainSubstring("deletion of core provider is not allowed"))

		g.By("infrastructureprovider name is immutable ")
		infrastructureproviderJSON, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructureprovider", iaasPlatform, "-n", clusterAPINamespace, "-o=json").OutputToFile("coreprovider.json")
		o.Expect(err).NotTo(o.HaveOccurred())
		bytes, _ = ioutil.ReadFile(infrastructureproviderJSON)
		infrastructureproviderName, _ := sjson.Set(string(bytes), "metadata.name", "invalid")
		err = ioutil.WriteFile(infrastructureproviderJSON, []byte(infrastructureproviderName), 0644)
		o.Expect(err).NotTo(o.HaveOccurred())
		infrastructureproviderNameUpdate, _ := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", infrastructureproviderJSON).Output()
		o.Expect(infrastructureproviderNameUpdate).To(o.ContainSubstring("incorrect infra provider name"))

		g.By("Shouldn't allow to delete infrastructureprovider")
		infrastructureproviderDelete, _ := oc.AsAdmin().WithoutNamespace().Run("delete").Args("infrastructureprovider", "--all", "-n", clusterAPINamespace).Output()
		o.Expect(infrastructureproviderDelete).To(o.ContainSubstring("deletion of infrastructure provider is not allowed"))
	})
})
