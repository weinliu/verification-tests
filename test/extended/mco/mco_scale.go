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
	g.It("Author:sregidor-NonHyperShiftHOST-NonPreRelease-LongDuration-High-63894-Scaleup using 4.1 cloud image[Disruptive]", func() {
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
			clonedSecret := NewSecret(oc.AsAdmin(), MachineAPINamespace, secretPrefix+newMsName)
			if newMs.Exists() {
				logger.Infof("Scaling %s machineset to zero", newMsName)
				_ = newMs.ScaleTo(0)

				logger.Infof("Waiting %s machineset for being ready", newMsName)
				_ = newMs.WaitUntilReady("15m")

				logger.Infof("Removing %s machineset", newMsName)
				_ = newMs.Delete()
				o.Eventually(wMcp.GetNodesOrFail, "5m", "30s").Should(o.HaveLen(initialNumWorkers),
					"The the number of worker nodes should not have changed.\n%s", wMcp.PrettyString())
			}
			if clonedSecret.Exists() {
				_ = clonedSecret.Delete()
			}
			logger.Infof("End TC defer block")
		}()

		newMs := cloneMachineConfig(oc.AsAdmin(), newMsName, amiVersion, useIgnitionV2)

		g.By("Scale MachineSet up")
		logger.Infof("Scaling up machineset %s", newMs.GetName())
		scaleErr := newMs.ScaleTo(numNewNodes)
		o.Expect(scaleErr).NotTo(o.HaveOccurred(), "Error scaling up MachineSet %s", newMs.GetName())

		logger.Infof("Waiting %s machineset for being ready", newMsName)
		o.Eventually(newMs.GetIsReady, "20m", "2m").Should(o.BeTrue(), "MachineSet %s is not ready", newMs.GetName())
		logger.Infof("OK!\n")

		g.By("Check that worker pool is increased and updated")
		o.Eventually(wMcp.GetNodesOrFail, "5m", "30s").Should(o.HaveLen(initialNumWorkers+numNewNodes),
			"The worker pool has not added the new nodes created by the new Machineset.\n%s", wMcp.PrettyString())
		wMcp.waitForComplete()
		logger.Infof("OK!\n")

	})
})

func cloneMachineConfig(oc *exutil.CLI, newMsName, amiVersion string, useIgnitionV2 bool) *MachineSet {
	var (
		newSecretName = secretPrefix + newMsName
	)

	// Duplicate an existing MachineSet
	g.By("Duplicate a MachineSet resource")
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
	g.By("Create a new secret with 2.2.0 ignition version and 'append' configuration")
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
	g.By(fmt.Sprintf("Configure the duplicated MachineSet to use the %s boot image", amiVersion))
	err = newMs.Patch("json", fmt.Sprintf(`[{ "op": "replace", "path": "/spec/template/spec/providerSpec/value/ami/id", "value": "%s" }]`, amiVersion))
	o.Expect(err).NotTo(o.HaveOccurred(), "Error patching MachineSet %s to use the new %s boot image", newMs.GetName(), amiVersion)
	logger.Infof("OK!\n")

	// Use new secret
	g.By("Configure the duplicated MachineSet to use the new secret")
	err = newMs.Patch("json", `[{ "op": "replace", "path": "/spec/template/spec/providerSpec/value/userDataSecret/name", "value": "`+newSecretName+`" }]`)
	o.Expect(err).NotTo(o.HaveOccurred(), "Error patching MachineSet %s to use the new secret %s", newMs.GetName(), newSecretName)
	logger.Infof("OK!\n")

	return newMs
}
