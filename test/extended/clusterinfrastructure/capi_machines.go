package clusterinfrastructure

import (
	"path/filepath"
	"strconv"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	clusterinfra "github.com/openshift/openshift-tests-private/test/extended/util/clusterinfra"
)

var _ = g.Describe("[sig-cluster-lifecycle] Cluster_Infrastructure CAPI", func() {
	defer g.GinkgoRecover()
	var (
		oc                              = exutil.NewCLI("capi-machines", exutil.KubeConfigPath())
		iaasPlatform                    clusterinfra.PlatformType
		clusterID                       string
		region                          string
		profile                         string
		instanceType                    string
		zone                            string
		ami                             string
		subnetName                      string
		subnetID                        string
		sgName                          string
		image                           string
		machineType                     string
		subnetwork                      string
		serviceAccount                  string
		capiBaseDir                     string
		clusterTemplate                 string
		awsMachineTemplateTemplate      string
		gcpMachineTemplateTemplate      string
		gcpMachineTemplateTemplatepdbal string
		capiMachinesetAWSTemplate       string
		capiMachinesetgcpTemplate       string
		capiMachinesetvsphereTemplate   string
		vsphereMachineTemplateTemplate  string
		vsphere_server                  string
		diskGiB                         string
		int_diskGiB                     int
		datacenter                      string
		datastore                       string
		machineTemplate                 string
		folder                          string
		resourcePool                    string
		numCPUs                         string
		int_numCPUs                     int
		networkname                     string
		memoryMiB                       string
		int_memoryMiB                   int

		err                     error
		cluster                 clusterDescription
		awsMachineTemplate      awsMachineTemplateDescription
		gcpMachineTemplate      gcpMachineTemplateDescription
		gcpMachineTemplatepdbal gcpMachineTemplateDescription
		capiMachineSetAWS       capiMachineSetAWSDescription
		capiMachineSetgcp       capiMachineSetgcpDescription
		clusterNotInCapi        clusterDescriptionNotInCapi

		vsphereMachineTemplate vsphereMachineTemplateDescription
		capiMachineSetvsphere  capiMachineSetvsphereDescription
	)

	g.BeforeEach(func() {
		exutil.SkipForSNOCluster(oc)
		clusterinfra.SkipConditionally(oc)
		iaasPlatform = clusterinfra.CheckPlatform(oc)
		switch iaasPlatform {
		case clusterinfra.AWS:
			region, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.aws.region}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			randomMachinesetName := clusterinfra.GetRandomMachineSetName(oc)
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
		case clusterinfra.GCP:
			region, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.gcp.region}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			randomMachinesetName := clusterinfra.GetRandomMachineSetName(oc)
			zone, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, randomMachinesetName, "-n", machineAPINamespace, "-o=jsonpath={.spec.template.spec.providerSpec.value.zone}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			image, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, randomMachinesetName, "-n", machineAPINamespace, "-o=jsonpath={.spec.template.spec.providerSpec.value.disks[0].image}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			machineType, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, randomMachinesetName, "-n", machineAPINamespace, "-o=jsonpath={.spec.template.spec.providerSpec.value.machineType}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			subnetwork, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, randomMachinesetName, "-n", machineAPINamespace, "-o=jsonpath={.spec.template.spec.providerSpec.value.networkInterfaces[0].subnetwork}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			serviceAccount, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, randomMachinesetName, "-n", machineAPINamespace, "-o=jsonpath={.spec.template.spec.providerSpec.value.serviceAccounts[0].email}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
		case clusterinfra.VSphere:
			vsphere_server, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.spec.platformSpec.vsphere.vcenters[0].server}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			randomMachinesetName := clusterinfra.GetRandomMachineSetName(oc)
			diskGiB, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, randomMachinesetName, "-n", machineAPINamespace, "-o=jsonpath={.spec.template.spec.providerSpec.value.diskGiB}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			int_diskGiB, err = strconv.Atoi(diskGiB)
			datacenter, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, randomMachinesetName, "-n", machineAPINamespace, "-o=jsonpath={.spec.template.spec.providerSpec.value.workspace.datacenter}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			machineTemplate, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, randomMachinesetName, "-n", machineAPINamespace, "-o=jsonpath={.spec.template.spec.providerSpec.value.template}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			datastore, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, randomMachinesetName, "-n", machineAPINamespace, "-o=jsonpath={.spec.template.spec.providerSpec.value.workspace.datastore}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			folder, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, randomMachinesetName, "-n", machineAPINamespace, "-o=jsonpath={.spec.template.spec.providerSpec.value.workspace.folder}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			resourcePool, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, randomMachinesetName, "-n", machineAPINamespace, "-o=jsonpath={.spec.template.spec.providerSpec.value.workspace.resourcePool}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			numCPUs, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, randomMachinesetName, "-n", machineAPINamespace, "-o=jsonpath={.spec.template.spec.providerSpec.value.numCPUs}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			int_numCPUs, err = strconv.Atoi(numCPUs)
			networkname, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, randomMachinesetName, "-n", machineAPINamespace, "-o=jsonpath={.spec.template.spec.providerSpec.value.network.devices[0].networkName}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			memoryMiB, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, randomMachinesetName, "-n", machineAPINamespace, "-o=jsonpath={.spec.template.spec.providerSpec.value.memoryMiB}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			int_memoryMiB, err = strconv.Atoi(memoryMiB)

		default:
			g.Skip("IAAS platform is " + iaasPlatform.String() + " which is NOT supported cluster api ...")
		}
		clusterID = clusterinfra.GetInfrastructureName(oc)

		capiBaseDir = exutil.FixturePath("testdata", "clusterinfrastructure", "capi")
		clusterTemplate = filepath.Join(capiBaseDir, "cluster.yaml")
		if subnetName != "" {
			awsMachineTemplateTemplate = filepath.Join(capiBaseDir, "machinetemplate-aws.yaml")
		} else {
			awsMachineTemplateTemplate = filepath.Join(capiBaseDir, "machinetemplate-aws-id.yaml")
		}
		gcpMachineTemplateTemplate = filepath.Join(capiBaseDir, "machinetemplate-gcp.yaml")
		gcpMachineTemplateTemplatepdbal = filepath.Join(capiBaseDir, "machinetemplate-gcp-pd-bal.yaml")
		capiMachinesetAWSTemplate = filepath.Join(capiBaseDir, "machinesetaws.yaml")
		capiMachinesetgcpTemplate = filepath.Join(capiBaseDir, "machinesetgcp.yaml")

		vsphereMachineTemplateTemplate = filepath.Join(capiBaseDir, "machinetemplate-vsphere.yaml")
		capiMachinesetvsphereTemplate = filepath.Join(capiBaseDir, "machinesetvsphere.yaml")

		cluster = clusterDescription{
			name:     clusterID,
			template: clusterTemplate,
		}
		clusterNotInCapi = clusterDescriptionNotInCapi{
			name:      clusterID,
			namespace: "openshift-machine-api",
			template:  clusterTemplate,
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
			name:           "gcp-machinetemplate",
			region:         region,
			image:          image,
			machineType:    machineType,
			subnetwork:     subnetwork,
			serviceAccount: serviceAccount,
			clusterID:      clusterID,
			template:       gcpMachineTemplateTemplate,
		}
		//gcpMachineTemplateTemplate-pd-bal
		gcpMachineTemplatepdbal = gcpMachineTemplateDescription{
			name:           "gcp-machinetemplate",
			region:         region,
			image:          image,
			machineType:    machineType,
			subnetwork:     subnetwork,
			serviceAccount: serviceAccount,
			clusterID:      clusterID,
			template:       gcpMachineTemplateTemplatepdbal,
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
		capiMachineSetvsphere = capiMachineSetvsphereDescription{
			name:           "capi-machineset-vsphere",
			clusterName:    clusterID,
			template:       capiMachinesetvsphereTemplate,
			dataSecretName: "worker-user-data",
			replicas:       1,
		}
		vsphereMachineTemplate = vsphereMachineTemplateDescription{
			kind:            "VSphereMachineTemplate",
			name:            clusterID,
			namespace:       "openshift-cluster-api",
			server:          vsphere_server,
			diskGiB:         int_diskGiB,
			datacenter:      datacenter,
			machineTemplate: machineTemplate,
			datastore:       datastore,
			folder:          folder,
			resourcePool:    resourcePool,
			numCPUs:         int_numCPUs,
			memoryMiB:       int_memoryMiB,
			dhcp:            true,
			networkName:     networkname,
			template:        vsphereMachineTemplateTemplate,
		}
		switch iaasPlatform {
		case clusterinfra.AWS:
			cluster.kind = "AWSCluster"
			clusterNotInCapi.kind = "AWSCluster"
			capiMachineSetAWS.kind = "AWSMachineTemplate"
			capiMachineSetAWS.machineTemplateName = awsMachineTemplate.name
		case clusterinfra.GCP:
			cluster.kind = "GCPCluster"
			clusterNotInCapi.kind = "GCPCluster"
			capiMachineSetgcp.kind = "GCPMachineTemplate"
			capiMachineSetgcp.machineTemplateName = gcpMachineTemplate.name
			capiMachineSetgcp.failureDomain = zone
		case clusterinfra.VSphere:
			cluster.kind = "VSphereCluster"
			capiMachineSetvsphere.kind = "VSphereMachineTemplate"
			capiMachineSetvsphere.machineTemplateName = vsphereMachineTemplate.name
			capiMachineSetvsphere.dataSecretName = ""

		default:
			g.Skip("IAAS platform is " + iaasPlatform.String() + " which is NOT supported cluster api ...")
		}
	})

	g.It("Author:zhsun-NonHyperShiftHOST-NonPreRelease-Longduration-High-51071-Create machineset with CAPI on aws [Disruptive][Slow]", func() {
		g.By("Check if cluster api on this platform is supported")
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS)
		skipForCAPINotExist(oc)

		g.By("Create capi machineset")
		/*create cluster no longer necessary - OCPCLOUD-2202
		cluster.createCluster(oc)*/
		defer awsMachineTemplate.deleteAWSMachineTemplate(oc)
		awsMachineTemplate.createAWSMachineTemplate(oc)

		capiMachineSetAWS.name = "capi-machineset-51071"
		defer waitForCapiMachinesDisapper(oc, capiMachineSetAWS.name)
		defer capiMachineSetAWS.deleteCapiMachineSet(oc)
		capiMachineSetAWS.createCapiMachineSet(oc)
		machineName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(capiMachine, "-o=jsonpath={.items[*].metadata.name}", "-n", "openshift-cluster-api").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = matchProviderIDWithNode(oc, capiMachine, machineName, "openshift-cluster-api")
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	g.It("Author:zhsun-NonHyperShiftHOST-NonPreRelease-Longduration-High-53100-Medium-74794-Create machineset with CAPI on gcp [Disruptive][Slow]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.GCP)
		skipForCAPINotExist(oc)

		g.By("Create capi machineset")
		/*create cluster no longer necessary - OCPCLOUD-2202
		cluster.createCluster(oc)*/

		// rootDeviceTypes included to cover multiple cases
		rootDeviceTypes := map[string]string{
			//	"pd-ssd":      "53100", This is now covered in cluster-actuator-pkg-tests pd-balanced is not possible due to change need to a vendor file"
			// We can leave rootDeviceTypes as map to accomodate any type like pd-balanced later
			"pd-balanced": "74794",
		}
		for rootDeviceType, machineNameSuffix := range rootDeviceTypes {

			g.By("Patching GCPMachineTemplate with rootDeviceType: " + rootDeviceType)
			if rootDeviceType == "pd-ssd" {
				gcpMachineTemplate.createGCPMachineTemplate(oc)
			} else if rootDeviceType == "pd-balanced" {
				gcpMachineTemplatepdbal.createGCPMachineTemplatePdBal(oc)
			}
			capiMachineSetgcp.name = "capi-machineset-" + machineNameSuffix
			defer waitForCapiMachinesDisappergcp(oc, capiMachineSetgcp.name)
			defer capiMachineSetgcp.deleteCapiMachineSetgcp(oc)
			capiMachineSetgcp.createCapiMachineSetgcp(oc)

			// Retrieve the machine name and validate it with the node
			machineName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(capiMachine, "-o=jsonpath={.items[*].metadata.name}", "-n", "openshift-cluster-api").Output()
			o.Expect(err).NotTo(o.HaveOccurred())

			// Match the provider ID with the node for verification
			_, err = matchProviderIDWithNode(oc, capiMachine, machineName, "openshift-cluster-api")
			o.Expect(err).NotTo(o.HaveOccurred())

			// gcpmachinetemplate is immutable
			capiMachineSetgcp.deleteCapiMachineSetgcp(oc)
			waitForCapiMachinesDisappergcp(oc, capiMachineSetgcp.name)
			if rootDeviceType == "pd-ssd" {
				gcpMachineTemplate.deleteGCPMachineTemplate(oc)
			} else if rootDeviceType == "pd-balanced" {
				gcpMachineTemplatepdbal.deleteGCPMachineTemplate(oc)
			}
		}
	})

	// author: zhsun@redhat.com
	g.It("Author:zhsun-NonHyperShiftHOST-Medium-55205-Webhook validations for CAPI [Disruptive]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS, clusterinfra.GCP)
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
	})

	// author: miyadav@redhat.com
	g.It("Author:miyadav-NonHyperShiftHOST-High-69188-cluster object can be deleted in non-cluster-api namespace [Disruptive]", func() {
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS, clusterinfra.GCP)
		skipForCAPINotExist(oc)
		g.By("Create cluster object in namespace other than openshift-cluster-api")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("cluster", clusterNotInCapi.name, "-n", clusterNotInCapi.namespace).Execute()
		clusterNotInCapi.createClusterNotInCapiNamespace(oc)
		g.By("Deleting cluster object in namespace other than openshift-cluster-api, should be successful")
		err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("cluster", clusterNotInCapi.name, "-n", clusterNotInCapi.namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	// author: zhsun@redhat.com
	g.It("Author:zhsun-NonHyperShiftHOST-NonPreRelease-Longduration-Medium-62928-Enable IMDSv2 on existing worker machines via machine set [Disruptive][Slow]", func() {
		g.By("Check if cluster api on this platform is supported")
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS)
		skipForCAPINotExist(oc)

		g.By("Create cluster, awscluster, awsmachinetemplate")
		/*create cluster no longer necessary - OCPCLOUD-2202
		cluster.createCluster(oc)
		//OCPCLOUD-2204
		defer awscluster.deleteAWSCluster(oc)
			awscluster.createAWSCluster(oc)*/
		defer awsMachineTemplate.deleteAWSMachineTemplate(oc)
		awsMachineTemplate.createAWSMachineTemplate(oc)
		err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("awsmachinetemplate", capiMachineSetAWS.machineTemplateName, "-n", clusterAPINamespace, "-p", `{"spec":{"template":{"spec":{"instanceMetadataOptions":{"httpEndpoint":"enabled","httpPutResponseHopLimit":1,"httpTokens":"required","instanceMetadataTags":"disabled"}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check machineTemplate with httpTokens: required")
		out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("awsmachinetemplate", capiMachineSetAWS.machineTemplateName, "-n", clusterAPINamespace, "-o=jsonpath={.spec.template.spec.instanceMetadataOptions.httpTokens}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(out).Should(o.Equal("required"))

		g.By("Create capi machineset with IMDSv2")
		capiMachineSetAWS.name = "capi-machineset-62928"
		defer waitForCapiMachinesDisapper(oc, capiMachineSetAWS.name)
		defer capiMachineSetAWS.deleteCapiMachineSet(oc)
		capiMachineSetAWS.createCapiMachineSet(oc)
	})
	// author: miyadav@redhat.com
	g.It("Author:miyadav-NonHyperShiftHOST-NonPreRelease-Longduration-High-72433-Create machineset with CAPI on vsphere [Disruptive][Slow]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.VSphere)
		skipForCAPINotExist(oc)

		g.By("Create capi machineset")
		/*create cluster no longer necessary - OCPCLOUD-2202
		cluster.createCluster(oc)*/

		defer vsphereMachineTemplate.deletevsphereMachineTemplate(oc)
		vsphereMachineTemplate.createvsphereMachineTemplate(oc)

		capiMachineSetvsphere.name = "capi-machineset-72433"
		capiMachineSetvsphere.createCapiMachineSetvsphere(oc)
		defer waitForCapiMachinesDisapper(oc, capiMachineSetvsphere.name)
		defer capiMachineSetvsphere.deleteCapiMachineSetvsphere(oc)
		machineName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(capiMachine, "-o=jsonpath={.items[*].metadata.name}", "-n", "openshift-cluster-api").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = matchProviderIDWithNode(oc, capiMachine, machineName, "openshift-cluster-api")
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	g.It("Author:huliu-NonHyperShiftHOST-NonPreRelease-Longduration-Medium-74803-[CAPI] Support AWS Placement Group Partition Number [Disruptive][Slow]", func() {
		g.By("Check if cluster api on this platform is supported")
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS)
		skipForCAPINotExist(oc)
		clusterinfra.GetAwsCredentialFromCluster(oc)
		awsClient := exutil.InitAwsSession()
		_, err := awsClient.GetPlacementGroupByName("pgpartition3")
		if err != nil {
			g.Skip("There is no this placement group for testing, skip the cases!!")
		}

		g.By("Create capi machineset")
		/*create cluster no longer necessary - OCPCLOUD-2202
		cluster.createCluster(oc)*/
		awsMachineTemplate.placementGroupName = "pgpartition3"
		awsMachineTemplate.placementGroupPartition = 3
		defer awsMachineTemplate.deleteAWSMachineTemplate(oc)
		awsMachineTemplate.createAWSMachineTemplate(oc)

		g.By("Check machineTemplate with placementGroupName: pgpartition3 and placementGroupPartition: 3")
		out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("awsmachinetemplate", capiMachineSetAWS.machineTemplateName, "-n", clusterAPINamespace, "-o=jsonpath={.spec.template.spec.placementGroupName}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(out).Should(o.Equal("pgpartition3"))
		out, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("awsmachinetemplate", capiMachineSetAWS.machineTemplateName, "-n", clusterAPINamespace, "-o=jsonpath={.spec.template.spec.placementGroupPartition}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(out).Should(o.Equal("3"))

		capiMachineSetAWS.name = "capi-machineset-74803"
		defer waitForCapiMachinesDisapper(oc, capiMachineSetAWS.name)
		defer capiMachineSetAWS.deleteCapiMachineSet(oc)
		capiMachineSetAWS.createCapiMachineSet(oc)
	})

	g.It("Author:huliu-NonHyperShiftHOST-NonPreRelease-Longduration-Medium-76088-[CAPI] New machine can join cluster when VPC has custom DHCP option set [Disruptive][Slow]", func() {
		g.By("Check if cluster api on this platform is supported")
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS)
		skipForCAPINotExist(oc)

		g.By("Create a new dhcpOptions")
		var newDhcpOptionsID, currentDhcpOptionsID string
		clusterinfra.GetAwsCredentialFromCluster(oc)
		awsClient := exutil.InitAwsSession()
		newDhcpOptionsID, err := awsClient.CreateDhcpOptionsWithDomainName("capi76088-CAPI.com")
		if err != nil {
			g.Skip("The credential is insufficient to perform create dhcpOptions operation, skip the cases!!")
		}
		defer func() {
			err := awsClient.DeleteDhcpOptions(newDhcpOptionsID)
			o.Expect(err).NotTo(o.HaveOccurred())
		}()

		g.By("Associate the VPC with the new dhcpOptionsId")
		machineName := clusterinfra.ListMasterMachineNames(oc)[0]
		instanceID, err := awsClient.GetAwsInstanceID(machineName)
		o.Expect(err).NotTo(o.HaveOccurred())
		vpcID, err := awsClient.GetAwsInstanceVPCId(instanceID)
		o.Expect(err).NotTo(o.HaveOccurred())
		currentDhcpOptionsID, err = awsClient.GetDhcpOptionsIDOfVpc(vpcID)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			err := awsClient.AssociateDhcpOptions(vpcID, currentDhcpOptionsID)
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		err = awsClient.AssociateDhcpOptions(vpcID, newDhcpOptionsID)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create capi machineset")
		/*create cluster no longer necessary - OCPCLOUD-2202
		cluster.createCluster(oc)*/
		defer awsMachineTemplate.deleteAWSMachineTemplate(oc)
		awsMachineTemplate.createAWSMachineTemplate(oc)

		capiMachineSetAWS.name = "capi-machineset-76088"
		defer waitForCapiMachinesDisapper(oc, capiMachineSetAWS.name)
		defer capiMachineSetAWS.deleteCapiMachineSet(oc)
		capiMachineSetAWS.createCapiMachineSet(oc)
	})
})
