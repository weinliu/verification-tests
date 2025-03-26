package operators

import (
	"context"
	"fmt"
	"math/rand"
	"path/filepath"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

var _ = g.Describe("[sig-operators] OLM for stress", func() {
	// NOTE: !!!! for all olm stress case, please add CPaasrunOnly label.
	// actually it is not CPaasrunOnly cases, but we add it because we use it in order to let other golang step not to select it.
	defer g.GinkgoRecover()

	var (
		oc = exutil.NewCLI("olm-stress"+getRandomString(), exutil.KubeConfigPath())
		dr = make(describerResrouce)
	)

	g.BeforeEach(func() {
		exutil.SkipNoOLMCore(oc)
		itName := g.CurrentSpecReport().FullText()
		dr.addIr(itName)
	})

	// author: kuiwang@redhat.com
	g.It("Author:kuiwang-ConnectedOnly-CPaasrunOnly-NonPreRelease-Medium-80299-[OlmStress]create mass operator to see if they all are installed successfully with different ns", func() {

		var (
			itName              = g.CurrentSpecReport().FullText()
			buildPruningBaseDir = exutil.FixturePath("testdata", "olm")
			ogSingleTemplate    = filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
			subTemplate         = filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")

			og = operatorGroupDescription{
				name:      "og-singlenamespace",
				namespace: "",
				template:  ogSingleTemplate,
			}
			sub = subscriptionDescription{
				subName:                "local-storage-operator",
				namespace:              "",
				channel:                "stable",
				ipApproval:             "Automatic",
				operatorPackage:        "local-storage-operator",
				catalogSourceName:      "redhat-operators",
				catalogSourceNamespace: "openshift-marketplace",
				startingCSV:            "",
				currentCSV:             "",
				installedCSV:           "",
				template:               subTemplate,
				singleNamespace:        true,
			}
		)

		for i := 0; i < 45; i++ {
			// for i := 0; i < 30; i++ { //1h11m
			e2e.Logf("=================it is round %v=================", i)
			func() {
				seed := time.Now().UnixNano()
				r := rand.New(rand.NewSource(seed))
				randomNum := r.Intn(5) + 5
				e2e.Logf("=================round %v has %v namespaces =================", i, randomNum)
				namespaces := []string{}
				for j := 0; j < randomNum; j++ {
					namespaces = append(namespaces, "olm-stress-"+getRandomString())
				}

				for _, ns := range namespaces {
					exutil.By(fmt.Sprintf("create ns %s, and then install og and sub", ns))
					err := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", ns).Execute()
					o.Expect(err).NotTo(o.HaveOccurred())
					defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("ns", ns, "--force", "--grace-period=0", "--wait=false").Execute()
					og.namespace = ns
					og.create(oc, itName, dr)
					sub.namespace = ns
					sub.createWithoutCheckNoPrint(oc, itName, dr)
				}
				for _, ns := range namespaces {
					exutil.By(fmt.Sprintf("find the installed csv ns %s", ns))
					sub.namespace = ns
					sub.findInstalledCSV(oc, itName, dr)
				}
				for _, ns := range namespaces {
					exutil.By(fmt.Sprintf("check the installed csv is ok in %s", ns))
					sub.namespace = ns

					errWait := wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 150*time.Second, false, func(ctx context.Context) (bool, error) {
						phase, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", sub.installedCSV, "-n", sub.namespace, "-o=jsonpath={.status.phase}").Output()
						if err != nil {
							if strings.Contains(phase, "NotFound") || strings.Contains(phase, "No resources found") {
								e2e.Logf("the existing csv does not exist, and try to get new csv")
								sub.findInstalledCSV(oc, itName, dr)
							} else {
								e2e.Logf("the error: %v, and try next", err)
							}
							return false, nil
						}

						e2e.Logf("---> we expect value: %s, in returned value: %s", "Succeeded+2+InstallSucceeded", phase)
						if strings.Compare(phase, "Succeeded") == 0 || strings.Compare(phase, "InstallSucceeded") == 0 {
							e2e.Logf("the output %s matches one of the content %s, expected", phase, "Succeeded+2+InstallSucceeded")
							return true, nil
						}
						e2e.Logf("the output %s does not matche one of the content %s, unexpected", phase, "Succeeded+2+InstallSucceeded")
						return false, nil
					})
					if errWait != nil {
						getResource(oc, asAdmin, withoutNamespace, "pod", "-n", "openshift-marketplace")
						getResource(oc, asAdmin, withoutNamespace, "operatorgroup", "-n", sub.namespace, "-o", "yaml")
						getResource(oc, asAdmin, withoutNamespace, "subscription", "-n", sub.namespace, "-o", "yaml")
						getResource(oc, asAdmin, withoutNamespace, "installplan", "-n", sub.namespace)
						getResource(oc, asAdmin, withoutNamespace, "csv", "-n", sub.namespace)
						getResource(oc, asAdmin, withoutNamespace, "pods", "-n", sub.namespace)
					}
					exutil.AssertWaitPollNoErr(errWait, fmt.Sprintf("expected content %s not found by %v", "Succeeded+2+InstallSucceeded", strings.Join([]string{"csv", sub.installedCSV, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}, " ")))

				}

			}()
		}
	})
	// author: kuiwang@redhat.com
	g.It("Author:kuiwang-ConnectedOnly-CPaasrunOnly-NonPreRelease-Medium-80413-[OlmStress]install operator repeatedly serially with same ns", func() {

		var (
			itName              = g.CurrentSpecReport().FullText()
			buildPruningBaseDir = exutil.FixturePath("testdata", "olm")
			ogSingleTemplate    = filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
			catsrcImageTemplate = filepath.Join(buildPruningBaseDir, "catalogsource-image.yaml")
			subTemplate         = filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
			ns                  = "openshift-must-gather-operator"

			catsrc = catalogSourceDescription{
				name:        "catsrc-80413",
				namespace:   "openshift-marketplace",
				displayName: "Test 80413",
				publisher:   "OLM QE",
				sourceType:  "grpc",
				address:     "quay.io/app-sre/must-gather-operator-registry@sha256:0a0610e37a016fb4eed1b000308d840795838c2306f305a151c64cf3b4fd6bb4",
				template:    catsrcImageTemplate,
			}
			og = operatorGroupDescription{
				name:      "og",
				namespace: ns,
				template:  ogSingleTemplate,
			}
			sub = subscriptionDescription{
				subName:                "must-gather-operator",
				namespace:              ns,
				channel:                "stable",
				ipApproval:             "Automatic",
				operatorPackage:        "must-gather-operator",
				catalogSourceName:      "catsrc-80413",
				catalogSourceNamespace: "openshift-marketplace",
				startingCSV:            "",
				currentCSV:             "",
				installedCSV:           "",
				template:               subTemplate,
				singleNamespace:        true,
			}
		)

		exutil.By("install catsrc in openshift-marketplace")
		defer catsrc.delete(itName, dr)
		catsrc.create(oc, itName, dr)

		for i := 0; i < 150; i++ { // for 200, it is more than 150m
			// for i := 0; i < 100; i++ { //1h21m
			e2e.Logf("=================it is round %v=================", i)
			func() {
				exutil.By(fmt.Sprintf("create ns %s", ns))
				err := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", ns).Execute()
				o.Expect(err).NotTo(o.HaveOccurred())
				// defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("ns", ns, "--force", "--grace-period=0", "--wait=true").Execute()
				defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("ns", ns).Execute()

				exutil.By(fmt.Sprintf("install og in %s", ns))
				og.create(oc, itName, dr)

				exutil.By(fmt.Sprintf("install catsrc in %s", ns))
				catsrc.create(oc, itName, dr)

				exutil.By(fmt.Sprintf("install sub in %s", ns))

				// defer sub.delete(itName, dr)
				// defer func() {
				// 	if sub.installedCSV == "" {
				// 		sub.findInstalledCSV(oc, itName, dr)
				// 	}
				// 	sub.deleteCSV(itName, dr)
				// }()
				sub.createWithoutCheckNoPrint(oc, itName, dr)

				exutil.By(fmt.Sprintf("find the installed csv ns %s", ns))
				sub.findInstalledCSV(oc, itName, dr)

				exutil.By(fmt.Sprintf("check the installed csv is ok in %s", ns))
				newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded+2+InstallSucceeded", ok, []string{"csv", sub.installedCSV, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)

			}()
		}
	})

})
