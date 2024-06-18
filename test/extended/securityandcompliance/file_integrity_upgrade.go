package securityandcompliance

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/openshift/openshift-tests-private/test/extended/util/architecture"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

var _ = g.Describe("[sig-isc] Security_and_Compliance File_Integrity_Operator Pre-check and post-check for file integrity operator upgrade", func() {
	defer g.GinkgoRecover()
	const (
		ns1 = "openshift-file-integrity"
	)
	var (
		oc = exutil.NewCLI("file-integrity-"+getRandomString(), exutil.KubeConfigPath())
	)

	g.Context("When the file-integrity-operator is installed", func() {

		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "securityandcompliance")
			fioTemplate         = filepath.Join(buildPruningBaseDir, "fileintegrity.yaml")
			fi1                 = fileintegrity{
				name:              "example-fileintegrity",
				namespace:         "",
				configname:        "",
				configkey:         "",
				graceperiod:       30,
				debug:             false,
				nodeselectorkey:   "node.openshift.io/os_id",
				nodeselectorvalue: "rhcos",
				template:          fioTemplate,
			}
		)

		g.BeforeEach(func() {
			g.By("Skip the test if the cluster has no OLM component")
			exutil.SkipNoOLMCore(oc)

			g.By("Skip test when missingcatalogsource, ARM64, or SkipHetegenous !!!")
			SkipMissingCatalogsource(oc)
			architecture.SkipArchitectures(oc, architecture.ARM64, architecture.MULTI)

			g.By("Check csv and pods for ns1 !!!")
			rsCsvName := getResourceNameWithKeywordForNamespace(oc, "csv", "file-integrity-operator", ns1)
			newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded", ok, []string{"csv", rsCsvName, "-n", ns1, "-o=jsonpath={.status.phase}"}).check(oc)
			newCheck("expect", asAdmin, withoutNamespace, contain, "file-integrity-operator", ok, []string{"pod", "--selector=name=file-integrity-operator", "-n",
				ns1, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
			g.By("Check file-integrity Operator pod is in running state !!!")
			newCheck("expect", asAdmin, withoutNamespace, compare, "Running", ok, []string{"pod", "--selector=name=file-integrity-operator", "-n",
				ns1, "-o=jsonpath={.items[0].status.phase}"}).check(oc)
		})

		// author: pdhamdhe@redhat.com
		g.It("Author:pdhamdhe-NonPreRelease-CPaasrunOnly-Critical-42663-Critical-45366-precheck for file integrity operator", func() {
			g.By("Create file integrity object  !!!\n")
			fi1.namespace = ns1
			err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", fi1.template, "-p", "NAME="+fi1.name, "NAMESPACE="+fi1.namespace, "GRACEPERIOD="+strconv.Itoa(fi1.graceperiod),
				"DEBUG="+strconv.FormatBool(fi1.debug), "NODESELECTORKEY="+fi1.nodeselectorkey, "NODESELECTORVALUE="+fi1.nodeselectorvalue)
			o.Expect(err).NotTo(o.HaveOccurred())
			newCheck("expect", asAdmin, withoutNamespace, contain, fi1.name, ok, []string{"fileintegrity", "-n", fi1.namespace,
				"-o=jsonpath={.items[0].metadata.name}"}).check(oc)

			g.By("Check aid pod and file integrity object status.. !!!\n")
			aidpodNames, err1 := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-l app=aide-example-fileintegrity", "-n", fi1.namespace,
				"-o=jsonpath={.items[*].metadata.name}").Output()
			o.Expect(err1).NotTo(o.HaveOccurred())
			aidpodName := strings.Fields(aidpodNames)
			for _, v := range aidpodName {
				newCheck("expect", asAdmin, withoutNamespace, contain, "Running", ok, []string{"pods", v, "-n", fi1.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
			}
			newCheck("expect", asAdmin, withoutNamespace, compare, "Active", ok, []string{"fileintegrity", fi1.name, "-n", ns1, "-o=jsonpath={.status.phase}"}).check(oc)
			fionodeNames, err2 := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "-n", fi1.namespace,
				"-o=jsonpath={.items[*].metadata.name}").Output()
			o.Expect(err2).NotTo(o.HaveOccurred())
			fionodeName := strings.Fields(fionodeNames)
			for _, v := range fionodeName {
				fi1.checkFileintegritynodestatus(oc, v, "Succeeded")
			}
		})

		// author: pdhamdhe@redhat.com
		g.It("Author:pdhamdhe-NonPreRelease-CPaasrunOnly-Critical-42663-Critical-45366-postcheck for file integrity operator", func() {
			fi1.namespace = ns1
			defer cleanupObjects(oc,
				objectTableRef{"project", ns1, ns1})

			g.By("Check aid pod and file integrity object status after upgrade.. !!!\n")
			aidpodNames, err1 := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-l app=aide-example-fileintegrity", "-n", fi1.namespace,
				"-o=jsonpath={.items[*].metadata.name}").Output()
			o.Expect(err1).NotTo(o.HaveOccurred())
			aidpodName := strings.Fields(aidpodNames)
			for _, v := range aidpodName {
				newCheck("expect", asAdmin, withoutNamespace, contain, "Running", ok, []string{"pods", v, "-n", fi1.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
			}
			newCheck("expect", asAdmin, withoutNamespace, compare, "Active", ok, []string{"fileintegrity", fi1.name, "-n", ns1, "-o=jsonpath={.status.phase}"}).check(oc)
			fionodeNames, err2 := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "-n", fi1.namespace,
				"-o=jsonpath={.items[*].metadata.name}").Output()
			o.Expect(err2).NotTo(o.HaveOccurred())
			fionodeName := strings.Fields(fionodeNames)
			for _, v := range fionodeName {
				fi1.checkFileintegritynodestatus(oc, v, "Succeeded")
			}

			g.By("trigger reinit")
			fi1.reinitFileintegrity(oc, "annotated")
			fi1.checkFileintegrityStatus(oc, "running")
			newCheck("expect", asAdmin, withoutNamespace, compare, "Active", ok, []string{"fileintegrity", fi1.name, "-n", ns1, "-o=jsonpath={.status.phase}"}).check(oc)
			aidpodNames, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-l app=aide-example-fileintegrity", "-n", fi1.namespace,
				"-o=jsonpath={.items[*].metadata.name}").Output()
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("aidepodNames is %s ", aidpodNames))
			aidpodName = strings.Fields(aidpodNames)
			for _, v := range aidpodName {
				newCheck("expect", asAdmin, withoutNamespace, contain, "Running", ok, []string{"pods", v, "-n", fi1.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
			}
			fionodeNames, err3 := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "-n", fi1.namespace, "-l node.openshift.io/os_id=rhcos",
				"-o=jsonpath={.items[*].metadata.name}").Output()
			exutil.AssertWaitPollNoErr(err3, fmt.Sprintf("fionodeNames is %s ", fionodeNames))
			fionodeName = strings.Fields(fionodeNames)
			for _, node := range fionodeName {
				fi1.checkFileintegritynodestatus(oc, node, "Succeeded")
				fi1.checkDBBackupResult(oc, node)
			}

		})
	})
})
