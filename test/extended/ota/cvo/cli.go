package cvo

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-updates] OTA oc should", func() {
	defer g.GinkgoRecover()
	oc := exutil.NewCLIWithoutNamespace("ota-oc")
	//author: jiajliu@redhat.com
	//this test does not reply on a live cluster, so connectedonly is for access to quay.io
	g.It("ConnectedOnly-Author:jiajliu-High-66746-Extract CredentialsRequest from a single-arch release image with --included --install-config", func() {
		testDataDir := exutil.FixturePath("testdata", "ota/cvo/cfg-ocp-66746")

		exutil.By("Get expected release image for the test")
		clusterVersion, _, err := exutil.GetClusterVersion(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		latest4StableImage, err := exutil.GetLatest4StableImage()
		o.Expect(latest4StableImage).NotTo(o.BeEmpty())
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(latest4StableImage, clusterVersion) {
			g.Skip("There is not expected release image for the test")
		}

		exutil.By("Check the help info on the two vars --included and --install-config")
		out, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("release", "extract", "--help").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(out, `--included=false:
        Exclude manifests that are not expected to be included in the cluster.`))
		o.Expect(strings.Contains(out, `--install-config='':
        Path to an install-config file, as consumed by the openshift-install command.  Works only in combination with --included.`))

		exutil.By("Extract all CR manifests from the specified release payload and check correct caps and featureset credential request extracted")
		files, err := ioutil.ReadDir(testDataDir)
		o.Expect(err).NotTo(o.HaveOccurred())
		caps := getDefaultCapsInCR(clusterVersion)
		o.Expect(caps).NotTo(o.BeNil())
		for _, file := range files {
			filepath := filepath.Join(testDataDir, file.Name())
			if strings.Contains(file.Name(), "4.x") {
				cmd := fmt.Sprintf("sed -i 's/4.x/%s/g' %s", clusterVersion, filepath)
				out, err := exec.Command("bash", "-c", cmd).CombinedOutput()
				o.Expect(err).NotTo(o.HaveOccurred(), "Command: \"%s\" returned error: %s", cmd, string(out))
				defer func() {
					cmd := fmt.Sprintf("sed -i 's/%s/4.x/g' %s", clusterVersion, filepath)
					out, err := exec.Command("bash", "-c", cmd).CombinedOutput()
					o.Expect(err).NotTo(o.HaveOccurred(), "Command: \"%s\" returned error: %s", cmd, string(out))
				}()
			}
			extractedCR, err := extractIncludedManifestWithInstallcfg(oc, true, filepath, latest4StableImage, "")
			defer func() { o.Expect(os.RemoveAll(extractedCR)).NotTo(o.HaveOccurred()) }()
			o.Expect(err).NotTo(o.HaveOccurred())
			cmd := fmt.Sprintf("grep -r 'release.openshift.io/feature-set\\|capability.openshift.io/name' %s|awk -F\":\" '{print $NF}'|sort -u", extractedCR)
			out, _ := exec.Command("bash", "-c", cmd).CombinedOutput()
			extractedCAP := strings.Fields(string(out))
			switch {
			case strings.Contains(file.Name(), "featureset"):
				o.Expect(len(extractedCAP)).To(o.Equal(len(caps) + 1))
				o.Expect(string(out)).To(o.ContainSubstring("TechPreviewNoUpgrade"))
				for _, cap := range caps {
					o.Expect(string(out)).To(o.ContainSubstring(cap))
				}
			case strings.Contains(file.Name(), "none"):
				o.Expect(string(out)).To(o.BeEmpty())
			case strings.Contains(file.Name(), "4.x"), strings.Contains(file.Name(), "vcurrent"):
				o.Expect(len(extractedCAP)).To(o.Equal(len(caps)))
				o.Expect(string(out)).NotTo(o.ContainSubstring("TechPreviewNoUpgrade"))
				for _, cap := range caps {
					o.Expect(string(out)).To(o.ContainSubstring(cap))
				}
			default:
				e2e.Failf("No expected test file found!")
			}
		}
	})

	//author: jiajliu@redhat.com
	//this is an oc test which does not need a live cluster, so connectedonly is for access to quay.io, and getRandomPlatform is to limit test frequency
	g.It("ConnectedOnly-Author:jiajliu-Medium-66751-Extract CredentialsRequest from a multi-arch release image with --included --install-config", func() {
		platform := getRandomPlatform()
		exutil.SkipIfPlatformTypeNot(oc, platform)
		testDataDir := exutil.FixturePath("testdata", "ota/cvo")
		cfgFile := filepath.Join(testDataDir, "ocp-66751.yaml")

		exutil.By("Get expected release image for the test")
		clusterVersion, _, err := exutil.GetClusterVersion(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		minorVer, err := strconv.Atoi(strings.Split(clusterVersion, ".")[1])
		o.Expect(err).NotTo(o.HaveOccurred())
		stream := fmt.Sprintf("4-stable-multi/latest?in=>4.%s.0-0+<4.%s.0-0", minorVer, minorVer+1)
		latest4StableMultiImage, err := exutil.GetLatest4StableImageByStream("multi", stream)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(latest4StableMultiImage).NotTo(o.BeEmpty())

		exutil.By("Extract all manifests from the specified release payload")
		extractedManifest, err := extractIncludedManifestWithInstallcfg(oc, false, cfgFile, latest4StableMultiImage, "")
		defer func() { o.Expect(os.RemoveAll(extractedManifest)).NotTo(o.HaveOccurred()) }()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Check correct caps and featureset manifests extracted")
		caps := []string{"Console", "MachineAPI"}
		cmd := fmt.Sprintf("grep -rh 'release.openshift.io/feature-set\\|capability.openshift.io/name' %s|awk -F\":\" '{print $NF}'|sort -u", extractedManifest)
		out, _ := exec.Command("bash", "-c", cmd).CombinedOutput()
		extractedCAP := strings.Fields(string(out))
		o.Expect(len(extractedCAP)).To(o.Equal(4), "unexpected extracted cap lengh in: %v", extractedCAP) // in 4.16 we also have "CustomNoUpgrade,TechPreviewNoUpgrade"
		o.Expect(string(out)).To(o.ContainSubstring("TechPreviewNoUpgrade"))
		for _, cap := range caps {
			o.Expect(string(out)).To(o.ContainSubstring(cap))
		}
	})

	//author: jiajliu@redhat.com
	//this is an oc test which does not need a live cluster, so connectedonly is for access to quay.io, and getRandomPlatform is to limit test frequency
	g.It("ConnectedOnly-Author:jiajliu-Low-66747-Run extract --included --install-config against bad config files or unavailable release payload", func() {
		platform := getRandomPlatform()
		exutil.SkipIfPlatformTypeNot(oc, platform)
		testDataDir := exutil.FixturePath("testdata", "ota/cvo/cfg-ocp-66747")
		cfgFile := filepath.Join(testDataDir, "gcp.yaml")
		badFile := filepath.Join(testDataDir, "bad.yaml")
		fakeReleasePayload := "quay.io/openshift-release-dev/ocp-release@sha256:fd96300600f9585e5847f5855ca14e2b3cafbce12aefe3b3f52c5da10c466666"

		exutil.By("Get expected release image for the test")
		latest4StableImage, err := exutil.GetLatest4StableImage()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(latest4StableImage).NotTo(o.BeEmpty())

		exutil.By("Check the error msg is about the wrong cloud type")
		_, err = extractIncludedManifestWithInstallcfg(oc, true, cfgFile, latest4StableImage, "aws")
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(err.Error()).To(o.ContainSubstring("error: --cloud \"aws\" set"))
		o.Expect(err.Error()).To(o.ContainSubstring("has \"gcp\""))

		exutil.By("Check the error msg is about wrong format of baselineCapabilitySet")
		_, err = extractIncludedManifestWithInstallcfg(oc, true, badFile, latest4StableImage, "")
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(err.Error()).To(o.ContainSubstring("error: unrecognized baselineCapabilitySet \"none\""))

		exutil.By("Check the error msg is about image not found")
		_, err = extractIncludedManifestWithInstallcfg(oc, true, cfgFile, fakeReleasePayload, "")
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(err.Error()).To(o.ContainSubstring("not found: manifest unknown: manifest unknown"))
	})
})
