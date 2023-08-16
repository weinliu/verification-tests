package mco

import (
	"fmt"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	logger "github.com/openshift/openshift-tests-private/test/extended/util/logext"
)

const (
	secretPrefix = "cloned-for-"
)

var _ = g.Describe("[sig-mco] MCO scale", func() {
	defer g.GinkgoRecover()

	var (
		oc = exutil.NewCLI("mco-scale", exutil.KubeConfigPath())
		// worker MachineConfigPool
		wMcp *MachineConfigPool
	)

	g.JustBeforeEach(func() {
		// Skip if no machineset
		skipTestIfWorkersCannotBeScaled(oc.AsAdmin())

		// Skip if not AWS
		platform := exutil.CheckPlatform(oc)
		if platform != AWSPlatform {
			g.Skip(fmt.Sprintf("Current platform is %s. AWS platform is required to execute this test case!.", platform))
		}

		// Skip if not east-2 zone
		infra := NewResource(oc.AsAdmin(), "infrastructure", "cluster")
		zone := infra.GetOrFail(`{.status.platformStatus.aws.region}`)
		if zone != "us-east-2" {
			g.Skip(fmt.Sprintf(`Current AWS zone is '%s'. AWS 'us-east-2' zone is required to execute this test case!.`, zone))
		}

		wMcp = NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolWorker)
		preChecks(oc)
	})

	g.It("Author:sregidor-NonHyperShiftHOST-NonPreRelease-Longduration-LongDuration-High-63894-Scaleup using 4.1 cloud image[Disruptive]", func() {
		var (
			newMsName     = "mco-tc-63894-cloned"
			amiVersion    = "ami-0649fd5d42859bdfc" // OCP4.1 ami for AWS and use-east2 zone: https://github.com/openshift/installer/blob/release-4.1/data/data/rhcos.json
			useIgnitionV2 = true                    // 4.1 version uses ignition V2
			numNewNodes   = 1                       // the number of nodes scaled up in the new Machineset
		)

		initialNumWorkers := len(wMcp.GetNodesOrFail())

		defer func() {
			logger.Infof("Start TC defer block")
			newMs := NewMachineSet(oc.AsAdmin(), MachineAPINamespace, newMsName)
			errors := o.InterceptGomegaFailures(func() { removeClonedMachineSet(newMs, wMcp, initialNumWorkers) }) // We don't want gomega to fail and stop the deferred cleanup process
			if len(errors) != 0 {
				logger.Infof("There were errors restoring the original MachineSet resources in the cluster")
				for _, e := range errors {
					logger.Errorf(e)
				}
			}

			// We don't want the test to pass if there were errors while restoring the initial state
			o.Expect(len(errors)).To(o.BeZero(),
				"There were %d errors while recovering the cluster's initial state", len(errors))

			logger.Infof("End TC defer block")
		}()

		newMs := cloneMachineSet(oc.AsAdmin(), newMsName, amiVersion, useIgnitionV2)

		exutil.By("Scale MachineSet up")
		logger.Infof("Scaling up machineset %s", newMs.GetName())
		scaleErr := newMs.ScaleTo(numNewNodes)
		o.Expect(scaleErr).NotTo(o.HaveOccurred(), "Error scaling up MachineSet %s", newMs.GetName())

		logger.Infof("Waiting %s machineset for being ready", newMsName)
		o.Eventually(newMs.GetIsReady, "20m", "2m").Should(o.BeTrue(), "MachineSet %s is not ready", newMs.GetName())
		logger.Infof("OK!\n")

		exutil.By("Check that worker pool is increased and updated")
		o.Eventually(wMcp.GetNodesOrFail, "5m", "30s").Should(o.HaveLen(initialNumWorkers+numNewNodes),
			"The worker pool has not added the new nodes created by the new Machineset.\n%s", wMcp.PrettyString())
		wMcp.waitForComplete()
		logger.Infof("OK!\n")

		exutil.By("Scale down and remove the cloned Machineset")
		removeClonedMachineSet(newMs, wMcp, initialNumWorkers)
		logger.Infof("OK!\n")

	})

	g.It("Author:sregidor-NonPreRelease-Longduration-High-52822-Create new config resources with 2.2.0 ignition boot image nodes [Disruptive]", func() {
		var (
			newMsName  = "copied-machineset-modified-tc-52822"
			kcName     = "change-maxpods-kubelet-config"
			kcTemplate = generateTemplateAbsolutePath(kcName + ".yaml")
			crName     = "change-ctr-cr-config"
			crTemplate = generateTemplateAbsolutePath(crName + ".yaml")
			mcName     = "generic-config-file-test-52822"
			mcpWorker  = NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolWorker)
			// Set the 4.5 boot image ami for east-2 zone.
			// the right ami should be selected from here https://github.com/openshift/installer/blob/release-4.5/data/data/rhcos.json
			amiVersion    = "ami-0ba8d5168e13bbcce"
			useIgnitionV2 = true // 4.5 version uses ignition V2
			numNewNodes   = 1    // the number of nodes scaled up in the new Machineset
		)

		initialNumWorkers := len(wMcp.GetNodesOrFail())

		defer func() {
			logger.Infof("Start TC defer block")
			newMs := NewMachineSet(oc.AsAdmin(), MachineAPINamespace, newMsName)
			errors := o.InterceptGomegaFailures(func() { // We don't want gomega to fail and stop the deferred cleanup process
				removeClonedMachineSet(newMs, wMcp, initialNumWorkers)

				cr := NewContainerRuntimeConfig(oc.AsAdmin(), crName, crTemplate)
				if cr.Exists() {
					logger.Infof("Removing ContainerRuntimeConfig %s", cr.GetName())
					o.Expect(cr.Delete()).To(o.Succeed(), "Error removing %s", cr)
				}
				kc := NewKubeletConfig(oc.AsAdmin(), kcName, kcTemplate)
				if kc.Exists() {
					logger.Infof("Removing KubeletConfig %s", kc.GetName())
					o.Expect(kc.Delete()).To(o.Succeed(), "Error removing %s", kc)
				}

				// MachineConfig struct has not been refactored to compose the "Resource" struct
				// so there is no "Exists" method available. Use it after refactoring MachineConfig
				mc := NewMachineConfig(oc.AsAdmin(), mcName, MachineConfigPoolWorker)
				logger.Infof("Removing machineconfig %s", mcName)
				mc.delete()

			})

			if len(errors) != 0 {
				logger.Infof("There were errors restoring the original MachineSet resources in the cluster")
				for _, e := range errors {
					logger.Errorf(e)
				}
			}

			logger.Infof("Waiting for worker pool to be updated")
			mcpWorker.waitForComplete()

			// We don't want the test to pass if there were errors while restoring the initial state
			o.Expect(len(errors)).To(o.BeZero(),
				"There were %d errors while recovering the cluster's initial state", len(errors))

			logger.Infof("End TC defer block")
		}()

		// Duplicate an existing MachineSet
		newMs := cloneMachineSet(oc.AsAdmin(), newMsName, amiVersion, useIgnitionV2)

		// KubeletConfig
		exutil.By("Create KubeletConfig")
		kc := NewKubeletConfig(oc.AsAdmin(), kcName, kcTemplate)
		kc.create()
		kc.waitUntilSuccess("10s")
		logger.Infof("OK!\n")

		// ContainterRuntimeConfig
		exutil.By("Create ContainterRuntimeConfig")
		cr := NewContainerRuntimeConfig(oc.AsAdmin(), crName, crTemplate)
		cr.create()
		cr.waitUntilSuccess("10s")
		logger.Infof("OK!\n")

		// Generic machineconfig
		exutil.By("Create generic config file")
		genericConfigFilePath := "/etc/test-52822"
		genericConfig := "config content for test case 52822"

		fileConfig := getURLEncodedFileConfig(genericConfigFilePath, genericConfig, "420")
		template := NewMCOTemplate(oc, "generic-machine-config-template.yml")
		errCreate := template.Create("-p", "NAME="+mcName, "-p", "POOL=worker", "-p", fmt.Sprintf("FILES=[%s]", fileConfig))
		o.Expect(errCreate).NotTo(o.HaveOccurred(), "Error creating MachineConfig %s", mcName)
		logger.Infof("OK!\n")

		// Wait for all pools to apply the configs
		exutil.By("Wait for worker MCP to be updated")
		mcpWorker.waitForComplete()
		logger.Infof("OK!\n")

		// Scale up the MachineSet
		exutil.By("Scale MachineSet up")
		logger.Infof("Scaling up machineset %s", newMs.GetName())
		scaleErr := newMs.ScaleTo(numNewNodes)
		o.Expect(scaleErr).NotTo(o.HaveOccurred(), "Error scaling up MachineSet %s", newMs.GetName())

		logger.Infof("Waiting %s machineset for being ready", newMsName)
		o.Eventually(newMs.GetIsReady, "20m", "2m").Should(o.BeTrue(), "MachineSet %s is not ready", newMs.GetName())
		logger.Infof("OK!\n")

		exutil.By("Check that worker pool is increased and updated")
		o.Eventually(wMcp.GetNodesOrFail, "5m", "30s").Should(o.HaveLen(initialNumWorkers+numNewNodes),
			"The worker pool has not added the new nodes created by the new Machineset.\n%s", wMcp.PrettyString())

		// Verify that the scaled nodes has been configured properly
		exutil.By("Check config in the new node")
		newNodes, nErr := newMs.GetNodes()
		o.Expect(nErr).NotTo(o.HaveOccurred(), "Error getting the nodes created by MachineSet %s", newMs.GetName())
		o.Expect(newNodes).To(o.HaveLen(numNewNodes), "Only %d nodes should have been created by MachineSet %s", numNewNodes, newMs.GetName())
		newNode := newNodes[0]
		logger.Infof("New node: %s", newNode.GetName())
		logger.Infof("OK!\n")

		exutil.By("Check kubelet config")
		kcFile := NewRemoteFile(*newNode, "/etc/kubernetes/kubelet.conf")
		kcrErr := kcFile.Fetch()
		o.Expect(kcrErr).NotTo(o.HaveOccurred(), "Error reading kubelet config in node %s", newNode.GetName())
		o.Expect(kcFile.GetTextContent()).Should(o.ContainSubstring("\"maxPods\": 500"),
			"File /etc/kubernetes/kubelet.conf has not the expected content")
		logger.Infof("OK!\n")

		exutil.By("Check container runtime config")
		crFile := NewRemoteFile(*newNode, "/etc/containers/storage.conf")
		crrErr := crFile.Fetch()
		o.Expect(crrErr).NotTo(o.HaveOccurred(), "Error reading container runtime config in node %s", newNode.GetName())
		o.Expect(crFile.GetTextContent()).Should(o.ContainSubstring("size = \"8G\""),
			"File /etc/containers/storage.conf has not the expected content")
		logger.Infof("OK!\n")

		exutil.By("Check generic machine config")
		cFile := NewRemoteFile(*newNode, genericConfigFilePath)
		crErr := cFile.Fetch()
		o.Expect(crErr).NotTo(o.HaveOccurred(), "Error reading generic config file in node %s", newNode.GetName())
		o.Expect(cFile.GetTextContent()).Should(o.Equal(genericConfig),
			"File %s has not the expected content", genericConfigFilePath)
		logger.Infof("OK!\n")

		exutil.By("Scale down and remove the cloned Machineset")
		removeClonedMachineSet(newMs, wMcp, initialNumWorkers)
		logger.Infof("OK!\n")

	})
})

func cloneMachineSet(oc *exutil.CLI, newMsName, amiVersion string, useIgnitionV2 bool) *MachineSet {
	var (
		newSecretName = secretPrefix + newMsName
	)

	// Duplicate an existing MachineSet
	exutil.By("Duplicate a MachineSet resource")
	allMs, err := NewMachineSetList(oc, MachineAPINamespace).GetAll()
	o.Expect(err).NotTo(o.HaveOccurred(), "Error getting a list of MachineSet resources")
	ms := allMs[0]
	newMs, dErr := ms.Duplicate(newMsName)
	o.Expect(dErr).NotTo(o.HaveOccurred(), "Error duplicating MachineSet %s -n %s", ms.GetName(), ms.GetNamespace())

	// Get current secret used by the MachineSet
	currentSecret := ms.GetOrFail(`{.spec.template.spec.providerSpec.value.userDataSecret.name}`)
	logger.Infof("Currently used secret: %s", currentSecret)
	logger.Infof("OK!\n")

	// Create a new secret using "append" and "2.2.0" Ignition version
	exutil.By("Create a new secret with 2.2.0 ignition version and 'append' configuration")
	logger.Infof("Duplicating secret %s with new name %s", currentSecret, newSecretName)
	var changes msDuplicatedSecretChanges
	if useIgnitionV2 {
		changes = msDuplicatedSecretChanges{Name: newSecretName, IgnitionVersion: "2.2.0", IgnitionConfigAction: "append"}
	} else {
		changes = msDuplicatedSecretChanges{Name: newSecretName}
	}
	clonedSecret, sErr := duplicateMachinesetSecret(oc, currentSecret, changes)
	o.Expect(sErr).NotTo(o.HaveOccurred(), "Error duplicating machine-api secret")
	o.Expect(clonedSecret).To(Exist(), "The secret was not duplicated for machineset %s", newMs)
	logger.Infof("OK!\n")

	// Set the new boot image ami.
	exutil.By(fmt.Sprintf("Configure the duplicated MachineSet to use the %s boot image", amiVersion))
	err = newMs.Patch("json", fmt.Sprintf(`[{ "op": "replace", "path": "/spec/template/spec/providerSpec/value/ami/id", "value": "%s" }]`, amiVersion))
	o.Expect(err).NotTo(o.HaveOccurred(), "Error patching MachineSet %s to use the new %s boot image", newMs.GetName(), amiVersion)
	logger.Infof("OK!\n")

	// Use new secret
	exutil.By("Configure the duplicated MachineSet to use the new secret")
	err = newMs.Patch("json", `[{ "op": "replace", "path": "/spec/template/spec/providerSpec/value/userDataSecret/name", "value": "`+newSecretName+`" }]`)
	o.Expect(err).NotTo(o.HaveOccurred(), "Error patching MachineSet %s to use the new secret %s", newMs.GetName(), newSecretName)
	logger.Infof("OK!\n")

	return newMs
}

func removeClonedMachineSet(ms *MachineSet, wMcp *MachineConfigPool, expectedNumWorkers int) {
	if ms.Exists() {
		logger.Infof("Scaling %s machineset to zero", ms.GetName())
		o.Expect(ms.ScaleTo(0)).To(o.Succeed(),
			"Error scaling MachineSet %s to 0", ms.GetName())

		logger.Infof("Waiting %s machineset for being ready", ms.GetName())
		o.Eventually(ms.GetIsReady, "1s", "2m").Should(o.BeTrue(), "MachineSet %s is not ready", ms.GetName())

		logger.Infof("Removing %s machineset", ms.GetName())
		o.Expect(ms.Delete()).To(o.Succeed(),
			"Error deleting MachineSet %s", ms.GetName())

		if expectedNumWorkers >= 0 {
			exutil.By("Check that worker pool is increased and updated")
			o.Eventually(wMcp.GetNodes, "5m", "30s").Should(o.HaveLen(expectedNumWorkers),
				"The worker pool has not added the new nodes created by the new Machineset.\n%s", wMcp.PrettyString())
		}
	}

	clonedSecret := NewSecret(ms.oc, MachineAPINamespace, secretPrefix+ms.GetName())
	if clonedSecret.Exists() {
		logger.Infof("Removing %s secret", clonedSecret)
		o.Expect(clonedSecret.Delete()).To(o.Succeed(),
			"Error deleting  %s", ms.GetName())
	}
}
