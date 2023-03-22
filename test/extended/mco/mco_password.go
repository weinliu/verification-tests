package mco

import (
	"fmt"
	"regexp"

	expect "github.com/google/goexpect"
	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	logger "github.com/openshift/openshift-tests-private/test/extended/util/logext"
)

var _ = g.Describe("[sig-mco] MCO password", func() {
	defer g.GinkgoRecover()

	var (
		oc                = exutil.NewCLI("mco-password", exutil.KubeConfigPath())
		passwordHash      string
		updatedPasswdHash string
		user              string
		password          string
		updatedPassword   string
		wMcp              *MachineConfigPool
	)

	g.JustBeforeEach(func() {
		passwordHash = "$6$uim4LuKWqiko1l5K$QJUwg.4lAyU4egsM7FNaNlSbuI6JfQCRufb99QuF082BpbqFoHP3WsWdZ5jCypS0veXWN1HDqO.bxUpE9aWYI1"      // sha-512 "coretest"
		updatedPasswdHash = "$6$sGXk8kzDPwf165.v$9Oc0fXJpFmUy8cSZzzjrW7pDQwaYbPojAR7CHAKRl81KDYrk2RQrcFI9gLfhfrPMHI2WuX4Us6ZBkO1KfF48/." // sha-512 "coretest2"
		user = "core"
		password = "coretest"
		updatedPassword = "coretest2"
		wMcp = NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolWorker)

		preChecks(oc)
	})

	g.It("Author:sregidor-NonPreRelease-High-59417-MCD create/update password with MachineConfig in CoreOS nodes[Disruptive]", func() {
		var (
			mcName = "tc-59417-test-core-passwd"
		)

		allCoreos := NewNodeList(oc).GetAllCoreOsWokerNodesOrFail()
		if len(allCoreos) == 0 {
			g.Skip("There are no coreOs worker nodes in this cluster")
		}

		workerNode := allCoreos[0]

		g.By("Configure a password for 'core' user")
		_, _ = workerNode.GetDate() // for debugging purposes, it prints the node's current time in the logs
		o.Expect(workerNode.IgnoreEventsBeforeNow()).NotTo(o.HaveOccurred(),
			"Error getting the latest event in node %s", workerNode.GetName())

		mc := NewMachineConfig(oc.AsAdmin(), mcName, MachineConfigPoolWorker)
		mc.parameters = []string{fmt.Sprintf(`PWDUSERS=[{"name":"%s", "passwordHash": "%s" }]`, user, passwordHash)}
		mc.skipWaitForMcp = true

		defer mc.delete()
		mc.create()

		wMcp.waitForComplete()
		logger.Infof("OK!\n")

		g.By("Check MCD logs to make sure drain and reboot are skipped")
		podLogs, err := exutil.GetSpecificPodLogs(oc, MachineConfigNamespace, MachineConfigDaemon, workerNode.GetMachineConfigDaemon(), `"drain\|reboot"`)
		o.Expect(err).NotTo(o.HaveOccurred(), "Errot getting the drain and reboot logs: %s", err)
		logger.Infof("Pod logs to skip node drain and reboot:\n %v", podLogs)
		o.Expect(podLogs).Should(
			o.And(
				o.ContainSubstring("Changes do not require drain, skipping"),
				o.ContainSubstring("skipping reboot")))
		logger.Infof("OK!\n")

		g.By("Check events to make sure that drain and reboot events were not triggered")
		nodeEvents, eErr := workerNode.GetEvents()
		o.Expect(eErr).ShouldNot(o.HaveOccurred(), "Error getting drain events for node %s", workerNode.GetName())
		o.Expect(nodeEvents).NotTo(HaveEventsSequence("Drain"), "Error, a Drain event was triggered but it shouldn't")
		o.Expect(nodeEvents).NotTo(HaveEventsSequence("Reboot"), "Error, a Reboot event was triggered but it shouldn't")
		logger.Infof("OK!\n")

		g.By("Verify that user 'core' can login with the configured password")
		logger.Infof("verifying node %s", workerNode.GetName())
		bresp, err := workerNode.ExecuteDebugExpectBatch(DefaultExpectTimeout, getSSHValidator(user, password))
		o.Expect(err).NotTo(o.HaveOccurred(), "Error in the ssh login process in node %s:\n %s", workerNode.GetName(), bresp)
		logger.Infof("OK!\n")

		g.By("Update the password value")
		patchErr := mc.Patch("json",
			fmt.Sprintf(`[{ "op": "add", "path": "/spec/config/passwd/users/0/passwordHash", "value": "%s"}]`, updatedPasswdHash))

		o.Expect(patchErr).NotTo(o.HaveOccurred(),
			"Error patching mc %s to update the 'core' user password")

		wMcp.waitForComplete()

		logger.Infof("OK!\n")

		g.By("Verify that user 'core' can login with the new password")
		logger.Infof("verifying node %s", workerNode.GetName())
		bresp, err = workerNode.ExecuteDebugExpectBatch(DefaultExpectTimeout, getSSHValidator(user, updatedPassword))
		o.Expect(err).NotTo(o.HaveOccurred(), "Error in the ssh login process in node %s:\n %s", workerNode.GetName(), bresp)
		logger.Infof("OK!\n")

		g.By("Remove the password")
		mc.deleteNoWait()
		wMcp.waitForComplete()
		logger.Infof("OK!\n")

		g.By("Verify that user 'core' can not login using a password anymore")
		logger.Infof("verifying node %s", workerNode.GetName())
		bresp, err = workerNode.ExecuteDebugExpectBatch(DefaultExpectTimeout, getSSHValidator(user, updatedPassword))
		o.Expect(err).To(o.HaveOccurred(), "User 'core' was able to login using a password in node %s, but it should not be possible:\n %s", workerNode.GetName(), bresp)
		logger.Infof("OK!\n")

	})

	g.It("Author:sregidor-NonPreRelease-High-60129-MCD create/update password with MachineConfig in RHEL nodes[Disruptive]", func() {
		var (
			mcName = "tc-60129-test-core-passwd"
		)

		allRhelNodes := NewNodeList(oc).GetAllRhelWokerNodesOrFail()
		if len(allRhelNodes) == 0 {
			g.Skip("There are no rhel worker nodes in this cluster")
		}

		allWorkerNodes := NewNodeList(oc).GetAllLinuxWorkerNodesOrFail()

		g.By("Create the 'core' user in RHEL nodes")
		for _, rhelWorker := range allRhelNodes {
			// we need to do this to avoid the loop variable to override our value
			if !rhelWorker.UserExists(user) {
				worker := rhelWorker
				defer func() { worker.UserDel(user) }()

				o.Expect(worker.UserAdd(user)).NotTo(o.HaveOccurred(),
					"Error creating user in node %s", worker.GetName())
			} else {
				logger.Infof("User %s already exists in node %s. Skip creation.", user, rhelWorker.GetName())
			}
		}

		g.By("Configure a password for 'core' user")
		mc := NewMachineConfig(oc.AsAdmin(), mcName, MachineConfigPoolWorker)
		mc.parameters = []string{fmt.Sprintf(`PWDUSERS=[{"name":"%s", "passwordHash": "%s" }]`, user, passwordHash)}
		mc.skipWaitForMcp = true

		defer mc.delete()
		mc.create()

		wMcp.waitForComplete()
		logger.Infof("OK!\n")

		g.By("Verify that user 'core' can login with the configured password")
		for _, workerNode := range allWorkerNodes {
			logger.Infof("Verifying node %s", workerNode.GetName())
			bresp, err := workerNode.ExecuteDebugExpectBatch(DefaultExpectTimeout, getSSHValidator(user, password))
			o.Expect(err).NotTo(o.HaveOccurred(), "Error in the ssh login process in node %s:\n %s", workerNode.GetName(), bresp)
		}
		logger.Infof("OK!\n")

		g.By("Update the password value")
		patchErr := mc.Patch("json",
			fmt.Sprintf(`[{ "op": "add", "path": "/spec/config/passwd/users/0/passwordHash", "value": "%s"}]`, updatedPasswdHash))

		o.Expect(patchErr).NotTo(o.HaveOccurred(),
			"Error patching mc %s to update the 'core' user password")

		wMcp.waitForComplete()

		logger.Infof("OK!\n")

		g.By("Verify that user 'core' can login with the new password")
		for _, workerNode := range allWorkerNodes {
			logger.Infof("Verifying node %s", workerNode.GetName())
			bresp, err := workerNode.ExecuteDebugExpectBatch(DefaultExpectTimeout, getSSHValidator(user, updatedPassword))
			o.Expect(err).NotTo(o.HaveOccurred(), "Error in the ssh login process in node %s:\n %s", workerNode.GetName(), bresp)
		}
		logger.Infof("OK!\n")

		g.By("Remove the password")
		mc.deleteNoWait()
		wMcp.waitForComplete()
		logger.Infof("OK!\n")

		g.By("Verify that user 'core' can not login using a password anymore")
		for _, workerNode := range allWorkerNodes {
			logger.Infof("Verifying node %s", workerNode.GetName())
			bresp, err := workerNode.ExecuteDebugExpectBatch(DefaultExpectTimeout, getSSHValidator(user, updatedPassword))
			o.Expect(err).To(o.HaveOccurred(), "User 'core' was able to login using a password in node %s, but it should not be possible:\n %s", workerNode.GetName(), bresp)
		}
		logger.Infof("OK!\n")

	})

	g.It("Author:sregidor-NonPreRelease-Medium-59900-Create a password for a user different from 'core' user[Disruptive]", func() {
		var (
			mcName       = "mco-tc-59900-wrong-user-password"
			wrongUser    = "root"
			passwordHash = "fake-hash"
			mcp          = NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolWorker)

			expectedNDReason = "1 nodes are reporting degraded status on sync"
		)

		g.By("Create a password for a non-core user using a MC")
		sortedNodes, err := mcp.GetSortedNodes()
		o.Expect(err).NotTo(o.HaveOccurred(), "Error getting the nodes in the worker MCP")
		fistUpdatedNode := sortedNodes[0]

		expectedNDMessage := regexp.QuoteMeta(fmt.Sprintf(`Node %s is reporting: "can't reconcile config`, fistUpdatedNode.GetName())) +
			`.*` +
			regexp.QuoteMeta(`ignition passwd user section contains unsupported changes: non-core user`)
		mc := NewMachineConfig(oc.AsAdmin(), mcName, MachineConfigPoolWorker)
		mc.parameters = []string{fmt.Sprintf(`PWDUSERS=[{"name":"%s", "passwordHash": "%s" }]`, wrongUser, passwordHash)}
		mc.skipWaitForMcp = true

		validateMcpNodeDegraded(mc, mcp, expectedNDMessage, expectedNDReason)

	})
})

// getSSHValidator returns the commands that need to be executed in an interactive expect shell to validate that a user can login via ssh
func getSSHValidator(user, passwd string) []expect.Batcher {

	return []expect.Batcher{
		&expect.BExpT{R: "#", T: 120}, // wait for prompt. We wait 120 seconds here, because the debug pod can take some time to be run
		// in the rest of the commands we use the default timeout
		&expect.BSnd{S: "chroot /host\n"}, // execute the chroot command
		// &expect.BExp{R: "#"},               // wait for prompt
		&expect.BExp{R: ".*"}, // wait for any prompt or no prompt (sometimes it does not return a prompt)
		&expect.BSnd{S: fmt.Sprintf(`su %s -c "su %s -c 'echo OK'"`, user, user) + "\n"}, // run an echo command forcing the user authentication
		&expect.BExp{R: "[pP]assword:"},              // wait for password question
		&expect.BSnd{S: fmt.Sprintf("%s\n", passwd)}, // write the password
		&expect.BExp{R: `OK`},                        // wait for succeess message
	}
}
