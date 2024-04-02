package storage

import (
	"path/filepath"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-storage] STORAGE", func() {
	defer g.GinkgoRecover()

	var (
		oc                   = exutil.NewCLI("storage-azure-file-csi", exutil.KubeConfigPath())
		storageTeamBaseDir   string
		storageClassTemplate string
		pvcTemplate          string
		podTemplate          string
		deploymentTemplate   string
		supportedProtocols   []string
	)

	// azure-file-csi test suite cloud provider support check
	g.BeforeEach(func() {
		// Function to check optional enabled capabilities
		checkOptionalCapability(oc, "Storage")
		provisioner = "file.csi.azure.com"

		cloudProvider = getCloudProvider(oc)
		if !strings.Contains(cloudProvider, "azure") {
			g.Skip("Skip for non-supported cloud provider: *" + cloudProvider + "* !!!")
		}

		// azure-file-csi support both "smb" and "nfs" protocols for most scenarios, so add it here as default support protocols in 4.14. But "smb" doesn't support in FIPS enabled cluster.
		// "nfs" protocols only supports Premium account type
		// Adding "networkEndpointType: privateEndpoint" in storageclass.parameters as WA to avoid https://issues.redhat.com/browse/OCPBUGS-18581, it is implemented in test/extended/storage/storageclass_utils.go
		// azure-file-csi doesn't support on Azure Stack Hub

		if isAzureStackCluster(oc) {
			g.Skip("Azure-file CSI Driver don't support AzureStack cluster, skip!!!")
		}

		supportedProtocols = append(supportedProtocols, "smb", "nfs")
		if checkFips(oc) {
			supportedProtocols = deleteElement(supportedProtocols, "smb")
		}

		storageTeamBaseDir = exutil.FixturePath("testdata", "storage")
		storageClassTemplate = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
		pvcTemplate = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
		podTemplate = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
		deploymentTemplate = filepath.Join(storageTeamBaseDir, "dep-template.yaml")
	})

	// author: wduan@redhat.com
	// OCP-50377-[Azure-File-CSI-Driver] support using resource group in storageclass
	g.It("ARO-Author:wduan-High-50377-[Azure-File-CSI-Driver] support using resource group in storageclass", func() {
		// Set up a specified project share for all the phases
		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project

		// Define the supported skuname
		exutil.By("Get resource group from infrastructures")
		rg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructures", "cluster", "-o=jsonpath={.status.platformStatus.azure.resourceGroupName}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		for _, protocol := range supportedProtocols {
			exutil.By("****** Azure-File CSI Driver with \"" + protocol + "\" protocol test phase start" + "******")
			func() {
				// Set the resource definition for the scenario
				storageClassParameters := map[string]string{
					"resourceGroup": rg,
					"protocol":      protocol,
				}
				extraParameters := map[string]interface{}{
					"parameters": storageClassParameters,
				}
				sc := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner("file.csi.azure.com"), setStorageClassVolumeBindingMode("Immediate"))
				pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(sc.name))
				dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name))

				exutil.By("Create storageclass")
				sc.createWithExtraParameters(oc, extraParameters)
				defer sc.deleteAsAdmin(oc)

				exutil.By("Create PVC")
				pvc.create(oc)
				defer pvc.deleteAsAdmin(oc)

				exutil.By("Create deployment")
				dep.create(oc)
				defer dep.deleteAsAdmin(oc)
				dep.waitReady(oc)

				exutil.By("Check the deployment's pod mounted volume can be read and write")
				dep.checkPodMountedVolumeCouldRW(oc)

				exutil.By("Check the deployment's pod mounted volume have the exec right")
				dep.checkPodMountedVolumeHaveExecRight(oc)
			}()
		}
	})

	// author: wduan@redhat.com
	// OCP-50360 - [Azure-File-CSI-Driver] support using storageAccount in storageclass
	g.It("ARO-Author:wduan-High-50360-[Azure-File-CSI-Driver] support using storageAccount in storageclass", func() {
		// Set up a specified project share for all the phases
		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project

		// Define the supported skuname
		for _, protocol := range supportedProtocols {
			exutil.By("****** Azure-File CSI Driver with \"" + protocol + "\" protocol test phase start" + "******")
			func() {
				exutil.By("Get storageAccount from new created Azure-file volume")
				scI := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner("file.csi.azure.com"), setStorageClassVolumeBindingMode("Immediate"))
				pvcI := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(scI.name), setPersistentVolumeClaimNamespace(oc.Namespace()))
				defer pvcI.deleteAsAdmin(oc)
				defer scI.deleteAsAdmin(oc)
				_, sa, _ := getAzureFileVolumeHandle(oc, scI, pvcI, protocol)

				// Set the resource definition for the scenario
				storageClassParameters := map[string]string{
					"storageAccount": sa,
					"protocol":       protocol,
				}
				extraParameters := map[string]interface{}{
					"parameters": storageClassParameters,
				}
				sc := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner("file.csi.azure.com"), setStorageClassVolumeBindingMode("Immediate"))
				pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(sc.name))
				dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name))

				exutil.By("Create storageclass")
				sc.createWithExtraParameters(oc, extraParameters)
				defer sc.deleteAsAdmin(oc)

				exutil.By("Create PVC")
				pvc.create(oc)
				defer pvc.deleteAsAdmin(oc)

				exutil.By("Create deployment")
				dep.create(oc)
				defer dep.deleteAsAdmin(oc)
				dep.waitReady(oc)

				exutil.By("Check the deployment's pod mounted volume can be read and write")
				dep.checkPodMountedVolumeCouldRW(oc)

				exutil.By("Check the deployment's pod mounted volume have the exec right")
				dep.checkPodMountedVolumeHaveExecRight(oc)
			}()
		}
	})

	// author: wduan@redhat.com
	// OCP-50471 - [Azure-File-CSI-Driver] support using sharename in storageclass
	g.It("ARO-Author:wduan-High-50471-[Azure-File-CSI-Driver] support using sharename in storageclass", func() {
		// Set up a specified project share for all the phases
		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project

		for _, protocol := range supportedProtocols {
			exutil.By("****** Azure-File CSI Driver with \"" + protocol + "\" protocol test phase start" + "******")
			func() {
				exutil.By("Get resourcegroup, storageAccount,sharename from new created Azure-file volume")
				scI := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner("file.csi.azure.com"), setStorageClassVolumeBindingMode("Immediate"))
				pvcI := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(scI.name), setPersistentVolumeClaimNamespace(oc.Namespace()))
				defer pvcI.deleteAsAdmin(oc)
				defer scI.deleteAsAdmin(oc)
				rg, sa, share := getAzureFileVolumeHandle(oc, scI, pvcI, protocol)

				// Set the resource definition for the scenario
				storageClassParameters := map[string]string{
					"resourceGroup":  rg,
					"storageAccount": sa,
					"shareName":      share,
					"protocol":       protocol,
				}
				extraParameters := map[string]interface{}{
					"parameters": storageClassParameters,
				}
				sc := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner("file.csi.azure.com"), setStorageClassVolumeBindingMode("Immediate"))
				// Only suport creating pvc with same size as existing share
				pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(sc.name), setPersistentVolumeClaimCapacity(pvcI.capacity))
				dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name))

				exutil.By("Create storageclass")
				sc.createWithExtraParameters(oc, extraParameters)
				defer sc.deleteAsAdmin(oc)

				exutil.By("Create PVC")
				pvc.create(oc)
				defer pvc.deleteAsAdmin(oc)

				exutil.By("Create deployment")
				dep.create(oc)
				defer dep.deleteAsAdmin(oc)
				dep.waitReady(oc)

				exutil.By("Check the deployment's pod mounted volume can be read and write")
				dep.checkPodMountedVolumeCouldRW(oc)

				exutil.By("Check the deployment's pod mounted volume have the exec right")
				dep.checkPodMountedVolumeHaveExecRight(oc)
			}()
		}
	})

	// author: rdeore@redhat.com
	// Author:rdeore-[Azure-File-CSI-Driver] [SKU-NAMES] support different skuName in storageclass
	var azureSkuNamesCaseIDMap = map[string]string{
		"50392": "Standard_LRS",    // High-50392-[Azure-File-CSI-Driver] [Standard_LRS] support different skuName in storageclass
		"50590": "Standard_GRS",    // High-50590-[Azure-File-CSI-Driver] [Standard_GRS] support different skuName in storageclass
		"50591": "Standard_RAGRS",  // High-50591-[Azure-File-CSI-Driver] [Standard_RAGRS] support different skuName in storageclass
		"50592": "Standard_RAGZRS", // High-50592-[Azure-File-CSI-Driver] [Standard_RAGZRS] support different skuName in storageclass
		"50593": "Premium_LRS",     // High-50593-[Azure-File-CSI-Driver] [Premium_LRS] support different skuName in storageclass
		"50594": "Standard_ZRS",    // High-50594-[Azure-File-CSI-Driver] [Standard_ZRS] support different skuName in storageclass
		"50595": "Premium_ZRS",     // High-50595-[Azure-File-CSI-Driver] [Premium_ZRS] support different skuName in storageclass
	}
	caseIds := []string{"50392", "50590", "50591", "50592", "50593", "50594", "50595"}
	for i := 0; i < len(caseIds); i++ {
		skuName := azureSkuNamesCaseIDMap[caseIds[i]]

		g.It("ARO-Author:rdeore-High-"+caseIds[i]+"-[Azure-File-CSI-Driver] [SKU-NAMES] support different skuName in storageclass with "+skuName, func() {
			region := getClusterRegion(oc)
			supportRegions := []string{"westus2", "westeurope", "northeurope", "francecentral"}
			if strings.Contains(skuName, "ZRS") && !contains(supportRegions, region) {
				g.Skip("Current region doesn't support zone-redundant storage")
			}
			// nfs protocol only supports Premium type
			if strings.Contains(skuName, "Standard_") {
				supportedProtocols = deleteElement(supportedProtocols, "nfs")
			}

			// skip case if there is no supported protocol
			if len(supportedProtocols) == 0 {
				g.Skip("Azure-file CSI Driver case is not applicable on this cluster, skip!!!")
			}

			for _, protocol := range supportedProtocols {
				exutil.By("****** Azure-File CSI Driver with \"" + protocol + "\" protocol test phase start" + "******")
				func() {
					// Set up a specified project share for all the phases
					exutil.By("#. Create new project for the scenario")
					oc.SetupProject()

					// Set the resource definition for the scenario
					storageClassParameters := map[string]string{
						"skuname":  skuName,
						"protocol": protocol,
					}
					extraParameters := map[string]interface{}{
						"parameters": storageClassParameters,
					}

					storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner(provisioner), setStorageClassVolumeBindingMode("Immediate"))
					pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClass.name))
					pod := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))

					exutil.By("#. Create csi storageclass with skuname: " + skuName)
					storageClass.createWithExtraParameters(oc, extraParameters)
					defer storageClass.deleteAsAdmin(oc)

					exutil.By("#. Create a pvc with the csi storageclass")
					pvc.create(oc)
					defer pvc.deleteAsAdmin(oc)

					exutil.By("#. Create pod with the created pvc and wait for the pod ready")
					pod.create(oc)
					defer pod.deleteAsAdmin(oc)
					pod.waitReady(oc)

					exutil.By("#. Check the pod volume can be read and write")
					pod.checkMountedVolumeCouldRW(oc)

					exutil.By("#. Check the pod volume have the exec right")
					pod.checkMountedVolumeHaveExecRight(oc)

					exutil.By("#. Check the pv.spec.csi.volumeAttributes.skuname")
					pvName := pvc.getVolumeName(oc)
					skunamePv, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pv", pvName, "-o=jsonpath={.spec.csi.volumeAttributes.skuname}").Output()
					o.Expect(err).NotTo(o.HaveOccurred())
					e2e.Logf("The skuname in PV is: %v.", skunamePv)
					o.Expect(skunamePv).To(o.Equal(skuName))
				}()
			}
		})
	}

	// author: rdeore@redhat.com
	// OCP-50634 -[Azure-File-CSI-Driver] fail to provision Block volumeMode
	g.It("ARO-Author:rdeore-High-50634-[Azure-File-CSI-Driver] fail to provision Block volumeMode", func() {
		// Set up a specified project share for all the phases
		exutil.By("#. Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, protocol := range supportedProtocols {
			exutil.By("****** Azure-File CSI Driver with \"" + protocol + "\" protocol test phase start" + "******")
			func() {
				// Set the resource definition for the scenario
				storageClassParameters := map[string]string{
					"protocol": protocol,
				}
				extraParameters := map[string]interface{}{
					"parameters": storageClassParameters,
				}
				sc := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner("file.csi.azure.com"), setStorageClassVolumeBindingMode("Immediate"))

				exutil.By("Create storageclass")
				sc.createWithExtraParameters(oc, extraParameters)
				defer sc.deleteAsAdmin(oc)

				exutil.By("#. Create a pvc with the csi storageclass")
				pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(sc.name), setPersistentVolumeClaimVolumemode("Block"))
				e2e.Logf("%s", pvc.scname)
				pvc.create(oc)
				defer pvc.deleteAsAdmin(oc)

				exutil.By("#. Check pvc: " + pvc.name + " is in pending status")
				pvc.waitPvcStatusToTimer(oc, "Pending")

				exutil.By("#. Check pvc provisioning failure information is clear")
				waitResourceSpecifiedEventsOccurred(oc, pvc.namespace, pvc.name, "driver does not support block volume")
			}()
		}
	})

	// author: wduan@redhat.com
	// OCP-50732 - [Azure-File-CSI-Driver] specify shareNamePrefix in storageclass
	g.It("ARO-Author:wduan-Medium-50732-[Azure-File-CSI-Driver] specify shareNamePrefix in storageclass", func() {
		// Set up a specified project share for all the phases
		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project

		for _, protocol := range supportedProtocols {
			exutil.By("****** Azure-File CSI Driver with \"" + protocol + "\" protocol test phase start" + "******")
			func() {
				// Set the resource definition for the scenario
				prefix := getRandomString()
				storageClassParameters := map[string]string{
					"shareNamePrefix": prefix,
					"protocol":        protocol,
				}

				extraParameters := map[string]interface{}{
					"parameters": storageClassParameters,
				}

				sc := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner("file.csi.azure.com"), setStorageClassVolumeBindingMode("Immediate"))
				pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(sc.name))
				pod := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))

				exutil.By("Create storageclass")
				sc.createWithExtraParameters(oc, extraParameters)
				defer sc.deleteAsAdmin(oc)

				exutil.By("Create PVC")
				pvc.create(oc)
				defer pvc.deleteAsAdmin(oc)

				exutil.By("Create pod")
				pod.create(oc)
				defer pod.deleteAsAdmin(oc)
				pod.waitReady(oc)

				exutil.By("Check the pod mounted volume can be read and write")
				pod.checkMountedVolumeCouldRW(oc)

				exutil.By("Check the pod mounted volume have the exec right")
				pod.checkMountedVolumeHaveExecRight(oc)

				exutil.By("Check pv has the prefix in the pv.spec.csi.volumeHandle")
				pvName := pvc.getVolumeName(oc)
				volumeHandle, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pv", pvName, "-o=jsonpath={.spec.csi.volumeHandle}").Output()
				o.Expect(err).NotTo(o.HaveOccurred())
				e2e.Logf("The Azure-File volumeHandle is: %v.", volumeHandle)
				o.Expect(volumeHandle).To(o.ContainSubstring(prefix))
			}()
		}
	})

	// author: wduan@redhat.com
	// OCP-50919 - [Azure-File-CSI-Driver] support smb file share protocol
	g.It("ARO-Author:wduan-LEVEL0-High-50919-[Azure-File-CSI-Driver] support smb file share protocol", func() {
		// Skip case in FIPS enabled cluster with smb protocol
		if checkFips(oc) {
			g.Skip("Azure-file CSI Driver with smb protocol don't support FIPS enabled env, skip!!!")
		}

		// Set up a specified project share for all the phases
		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project

		// Set the resource definition for the scenario
		storageClassParameters := map[string]string{
			"protocol": "smb",
		}
		extraParameters := map[string]interface{}{
			"parameters": storageClassParameters,
		}
		sc := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner("file.csi.azure.com"), setStorageClassVolumeBindingMode("Immediate"))
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(sc.name))
		dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name))

		exutil.By("Create storageclass")
		sc.createWithExtraParameters(oc, extraParameters)
		defer sc.deleteAsAdmin(oc)

		exutil.By("Create PVC")
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)

		exutil.By("Create deployment")
		dep.create(oc)
		defer dep.deleteAsAdmin(oc)
		dep.waitReady(oc)

		exutil.By("Check the deployment's pod mounted volume can be read and write")
		dep.checkPodMountedVolumeCouldRW(oc)

		exutil.By("Check the deployment's pod mounted volume have the exec right")
		dep.checkPodMountedVolumeHaveExecRight(oc)

		exutil.By("Check pv has protocol parameter in the pv.spec.csi.volumeAttributes.protocol")
		pvName := pvc.getVolumeName(oc)
		protocol, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pv", pvName, "-o=jsonpath={.spec.csi.volumeAttributes.protocol}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("The protocol is: %v.", protocol)
		o.Expect(protocol).To(o.ContainSubstring("smb"))
	})

	// author: wduan@redhat.com
	// OCP-50918 - [Azure-File-CSI-Driver] support nfs file share protocol
	g.It("ARO-Author:wduan-LEVEL0-High-50918-[Azure-File-CSI-Driver] support nfs file share protocol", func() {
		// Set up a specified project share for all the phases
		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project

		// Set the resource definition for the scenario
		storageClassParameters := map[string]string{
			"protocol": "nfs",
		}
		extraParameters := map[string]interface{}{
			"parameters": storageClassParameters,
		}
		sc := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner("file.csi.azure.com"), setStorageClassVolumeBindingMode("Immediate"))
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(sc.name))
		dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name))

		exutil.By("Create storageclass")
		sc.createWithExtraParameters(oc, extraParameters)
		defer sc.deleteAsAdmin(oc)

		exutil.By("Create PVC")
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)

		exutil.By("Create deployment")
		dep.create(oc)
		defer dep.deleteAsAdmin(oc)
		dep.waitReady(oc)

		exutil.By("Check the deployment's pod mounted volume can be read and write")
		dep.checkPodMountedVolumeCouldRW(oc)

		exutil.By("Check the deployment's pod mounted volume have the exec right")
		dep.checkPodMountedVolumeHaveExecRight(oc)

		exutil.By("Check pv has protocol parameter in the pv.spec.csi.volumeAttributes.protocol")
		pvName := pvc.getVolumeName(oc)
		protocol, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pv", pvName, "-o=jsonpath={.spec.csi.volumeAttributes.protocol}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("The protocol is: %v.", protocol)
		o.Expect(protocol).To(o.ContainSubstring("nfs"))
	})

	// author: chaoyang@redhat.com
	// OCP-60540&60578 - [Azure-File-CSI-Driver]Support fsgroup for nfs and smb file share protocol
	protocolTestSuite := map[string][]string{
		"60540": {"smb", "cifs_t"},
		"60578": {"nfs", "nfs_t"},
	}

	caseIds_fsgroup := []string{"60540", "60578"}

	for i := 0; i < len(caseIds_fsgroup); i++ {
		protocolType := protocolTestSuite[caseIds_fsgroup[i]]

		g.It("ARO-Author:chaoyang-High-"+caseIds_fsgroup[i]+"-[Azure-File-CSI-Driver]Support fsgroup for "+protocolType[0]+" file share protocol", func() {
			// Skip case in FIPS enabled cluster with smb protocol
			if checkFips(oc) && protocolType[0] == "smb" {
				g.Skip("Azure-file CSI Driver with smb protocol don't support FIPS enabled env, skip!!!")
			}

			exutil.By("Create new project for the scenario")
			oc.SetupProject()

			exutil.By("Start to test fsgroup for protoclol " + protocolType[0])
			storageClassParameters := map[string]string{
				"protocol": protocolType[0],
			}
			extraParameters_sc := map[string]interface{}{
				"parameters": storageClassParameters,
			}

			sc := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner("file.csi.azure.com"), setStorageClassVolumeBindingMode("Immediate"))
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(sc.name))
			pod := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))
			extraParameters_pod := map[string]interface{}{
				"jsonPath":  `items.0.spec.securityContext.`,
				"fsGroup":   10000,
				"runAsUser": 12345,
			}
			exutil.By("Create storageclass")
			sc.createWithExtraParameters(oc, extraParameters_sc)
			defer sc.deleteAsAdmin(oc)

			exutil.By("Create PVC")
			pvc.create(oc)
			defer pvc.deleteAsAdmin(oc)

			exutil.By("Create pod")
			pod.createWithExtraParameters(oc, extraParameters_pod)
			defer pod.deleteAsAdmin(oc)
			pod.waitReady(oc)

			exutil.By("Check pod security--uid")
			outputUID, err := pod.execCommandAsAdmin(oc, "id -u")
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("%s", outputUID)
			o.Expect(outputUID).To(o.ContainSubstring("12345"))

			exutil.By("Check pod security--fsGroup")
			outputGid, err := pod.execCommandAsAdmin(oc, "id -G")
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("%s", outputGid)
			o.Expect(outputGid).To(o.ContainSubstring("10000"))

			_, err = pod.execCommandAsAdmin(oc, "touch "+pod.mountPath+"/testfile")
			o.Expect(err).NotTo(o.HaveOccurred())
			outputTestfile, err := pod.execCommandAsAdmin(oc, "ls -lZ "+pod.mountPath+"/testfile")
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("%s", outputTestfile)
			o.Expect(outputTestfile).To(o.ContainSubstring("10000"))
			o.Expect(outputTestfile).To(o.ContainSubstring("system_u:object_r:" + protocolType[1]))

		})
	}

	// author: wduan@redhat.com
	// Author:wdaun-[Azure-File-CSI-Driver] support different shareAccessTier in storageclass
	// For general-purpose v2 account, the available tiers are TransactionOptimized(default), Hot, and Cool. For file storage account, the available tier is Premium. (https://github.com/kubernetes-sigs/azurefile-csi-driver/blob/master/docs/driver-parameters.md)
	var azureShareAccessTierCaseIDMap = map[string]string{
		"60195": "Hot",                  // Medium-60195-[Azure-File-CSI-Driver] suport specify shareAccessTier:Hot in storageclass
		"60196": "Cool",                 // Medium-60196-[Azure-File-CSI-Driver] suport specify shareAccessTier:Cool in storageclass
		"60197": "TransactionOptimized", // Medium-60197-[Azure-File-CSI-Driver] suport specify shareAccessTier:TransactionOptimized in storageclass
		"60856": "Premium",              // Medium-60856-[Azure-File-CSI-Driver] suport specify shareAccessTier:Premium in storageclass

	}
	atCaseIds := []string{"60195", "60196", "60197", "60856"}
	for i := 0; i < len(atCaseIds); i++ {
		shareAccessTier := azureShareAccessTierCaseIDMap[atCaseIds[i]]

		g.It("ARO-Author:wduan-Medium-"+atCaseIds[i]+"-[Azure-File-CSI-Driver] support shareAccessTier in storageclass with "+shareAccessTier, func() {
			// nfs protocol only supports Premium type
			if shareAccessTier != "Premium" {
				supportedProtocols = deleteElement(supportedProtocols, "nfs")
			}

			// skip case if there is no supported protocol
			if len(supportedProtocols) == 0 {
				g.Skip("Azure-file CSI Driver case is not applicable on this cluster, skip!!!")
			}

			exutil.By("#. Create new project for the scenario")
			oc.SetupProject()

			for _, protocol := range supportedProtocols {
				exutil.By("****** Azure-File CSI Driver with \"" + protocol + "\" protocol test phase start" + "******")
				func() {

					// Set the resource definition for the scenario
					storageClassParameters := map[string]string{
						"shareAccessTier": shareAccessTier,
						"protocol":        protocol,
					}
					if shareAccessTier == "Premium" {
						storageClassParameters["skuName"] = "Premium_LRS"
					}
					extraParameters := map[string]interface{}{
						"parameters": storageClassParameters,
					}

					storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner(provisioner), setStorageClassVolumeBindingMode("Immediate"))
					pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClass.name))
					pod := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))

					exutil.By("#. Create csi storageclass with shareAccessTier: " + shareAccessTier)
					storageClass.createWithExtraParameters(oc, extraParameters)
					defer storageClass.deleteAsAdmin(oc)

					exutil.By("#. Create a pvc with the csi storageclass")
					pvc.create(oc)
					defer pvc.deleteAsAdmin(oc)

					exutil.By("#. Create pod with the created pvc and wait for the pod ready")
					pod.create(oc)
					defer pod.deleteAsAdmin(oc)
					pod.waitReady(oc)

					exutil.By("#. Check the pod volume has the read and write access right")
					pod.checkMountedVolumeCouldRW(oc)

					exutil.By("#. Check the pv.spec.csi.volumeAttributes.shareAccessTier")
					pvName := pvc.getVolumeName(oc)
					shareAccessTierPv, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pv", pvName, "-o=jsonpath={.spec.csi.volumeAttributes.shareAccessTier}").Output()
					o.Expect(err).NotTo(o.HaveOccurred())
					e2e.Logf("The shareAccessTier in PV is: %v.", shareAccessTierPv)
					o.Expect(shareAccessTierPv).To(o.Equal(shareAccessTier))
					// Todo: Add check from the back end
				}()
			}
		})
	}

	// author: wduan@redhat.com
	// OCP-71559 - [Azure-File-CSI-Driver] [nfs] support actimeo as the default mount option if not specified
	g.It("ARO-Author:wduan-Medium-71559-[Azure-File-CSI-Driver] [nfs] support actimeo as the default mount option if not specified", func() {
		// Set up a specified project share for all the phases
		exutil.By("Create new project for the scenario")
		oc.SetupProject()

		// Set the resource definition for the scenario
		// This only supported with nfs protocol
		storageClassParameters := map[string]string{
			"protocol": "nfs",
		}
		extraParameters := map[string]interface{}{
			"parameters": storageClassParameters,
		}
		sc := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner("file.csi.azure.com"), setStorageClassVolumeBindingMode("Immediate"))
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(sc.name))
		dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name))

		exutil.By("Create storageclass")
		sc.createWithExtraParameters(oc, extraParameters)
		defer sc.deleteAsAdmin(oc)

		exutil.By("Create PVC")
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)

		exutil.By("Create deployment")
		dep.create(oc)
		defer dep.deleteAsAdmin(oc)
		dep.waitReady(oc)

		exutil.By("Check the deployment's pod mounted volume can be read and write")
		dep.checkPodMountedVolumeCouldRW(oc)

		// Specifying actimeo sets all of acregmin, acregmax, acdirmin, and acdirmax, the acdirmin(default value is 30) is absent.
		exutil.By("Check the mount option on the node should contain the actimeo series parameters")
		pvName := pvc.getVolumeName(oc)
		nodeName := getNodeNameByPod(oc, dep.namespace, dep.getPodList(oc)[0])
		output, err := execCommandInSpecificNode(oc, nodeName, "mount | grep "+pvName)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("acregmin=30,acregmax=30,acdirmax=30"))
	})

})

// Get resourceGroup/account/share name by creating a new azure-file volume
func getAzureFileVolumeHandle(oc *exutil.CLI, sc storageClass, pvc persistentVolumeClaim, protocol string) (resourceGroup string, account string, share string) {
	storageClassParameters := map[string]string{
		"protocol": protocol,
	}
	extraParameters := map[string]interface{}{
		"parameters": storageClassParameters,
	}
	sc.createWithExtraParameters(oc, extraParameters)
	pvc.create(oc)
	pvc.waitStatusAsExpected(oc, "Bound")
	pvName := pvc.getVolumeName(oc)
	volumeHandle, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pv", pvName, "-o=jsonpath={.spec.csi.volumeHandle}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The Azure-File volumeHandle is: %v.", volumeHandle)
	items := strings.Split(volumeHandle, "#")
	debugLogf("resource-group-name is \"%s\", account-name is \"%s\", share-name is \"%s\"", items[0], items[1], items[2])
	return items[0], items[1], items[2]
}
