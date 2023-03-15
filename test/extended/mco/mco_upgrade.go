package mco

import (
	"fmt"
	"os"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	logger "github.com/openshift/openshift-tests-private/test/extended/util/logext"
)

var _ = g.Describe("[sig-mco] MCO Upgrade", func() {
	defer g.GinkgoRecover()

	var (
		// init cli object, temp namespace contains prefix mco.
		// tip: don't put this in BeforeEach/JustBeforeEach, you will get error
		// "You may only call AfterEach from within a Describe, Context or When"
		oc = exutil.NewCLI("mco-upgrade", exutil.KubeConfigPath())
		// temp dir to store all test files, and it will be recycled when test is finished
		tmpdir string
	)

	g.JustBeforeEach(func() {
		tmpdir = createTmpDir()
		preChecks(oc)
	})

	g.JustAfterEach(func() {
		os.RemoveAll(tmpdir)
		logger.Infof("test dir %s is cleaned up", tmpdir)
	})

	g.It("Author:rioliu-PstChkUpgrade-NonPreRelease-High-45550-upgrade cluster is failed on RHEL node", func() {

		skipTestIfOsIsNotRhelOs(oc)

		g.By("iterate all rhel nodes to check the machine config related annotations")

		allRhelNodes := NewNodeList(oc).GetAllRhelWokerNodesOrFail()
		for _, node := range allRhelNodes {
			state := node.GetAnnotationOrFail(NodeAnnotationState)
			reason := node.GetAnnotationOrFail(NodeAnnotationReason)
			logger.Infof("checking node %s ...", node.GetName())
			o.Expect(state).Should(o.Equal("Done"), fmt.Sprintf("annotation [%s] value is not expected: %s", NodeAnnotationState, state))
			o.Expect(reason).ShouldNot(o.ContainSubstring(`Failed to find /dev/disk/by-label/root`),
				fmt.Sprintf("annotation [%s] value has unexpected error message", NodeAnnotationReason))
		}

	})

	g.It("Author:rioliu-PstChkUpgrade-NonPreRelease-High-55748-Upgrade failed with Transaction in progress", func() {

		g.By("check machine config daemon log to verify no error `Transaction in progress` found")

		allNodes, getNodesErr := NewNodeList(oc).GetAllLinux()
		o.Expect(getNodesErr).NotTo(o.HaveOccurred(), "Get all linux nodes error")
		for _, node := range allNodes {
			logger.Infof("checking mcd log on %s", node.GetName())
			errLog, getLogErr := node.GetMCDaemonLogs("'Transaction in progress: (null)'")
			o.Expect(getLogErr).Should(o.HaveOccurred(), "Unexpected error found in MCD log")
			o.Expect(errLog).Should(o.BeEmpty(), "Transaction in progress error found, it is unexpected")
			logger.Infof("no error found")
		}
	})

	g.It("Author:rioliu-PstChkUpgrade-NonPreRelease-High-59427-ssh keys can be migrated to new dir when node is upgraded from RHCOS8 to RHCOS9", func() {

		var (
			oldAuthorizedKeyPath = "/home/core/.ssh/authorized_key"
			newAuthorizedKeyPath = "/home/core/.ssh/authorized_keys.d/ignition"
		)

		allCoreOsNodes := NewNodeList(oc).GetAllCoreOsNodesOrFail()
		for _, node := range allCoreOsNodes {
			g.By(fmt.Sprintf("check authorized key dir and file on %s", node.GetName()))
			output, err := node.DebugNodeWithChroot("stat", oldAuthorizedKeyPath)
			o.Expect(err).Should(o.HaveOccurred(), "old authorized key file still exists")
			o.Expect(output).Should(o.ContainSubstring("No such file or directory"))
			output, err = node.DebugNodeWithChroot("stat", newAuthorizedKeyPath)
			o.Expect(err).ShouldNot(o.HaveOccurred(), "new authorized key file not found")
			o.Expect(output).Should(o.ContainSubstring("File: " + newAuthorizedKeyPath))
		}

	})
})
