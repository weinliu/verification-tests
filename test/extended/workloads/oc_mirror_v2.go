package workloads

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-cli] Workloads ocmirror v2 works well", func() {
	defer g.GinkgoRecover()

	var (
		oc = exutil.NewCLI("ocmirrorv2", exutil.KubeConfigPath())
	)

	g.It("NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Author:knarra-Medium-72973-support mirror multi-arch additional images for v2 [Serial]", func() {
		dirname := "/tmp/case72973"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", "--to="+dirname, "--confirm").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-72973.yaml")

		err = getRouteCAToFile(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}
		exutil.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)

		exutil.By("Configure the Registry Certificate as trusted for cincinnati")
		addCA, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("image.config.openshift.io/cluster", "-o=jsonpath={.spec.additionalTrustedCA}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer restoreAddCA(oc, addCA, "trusted-ca-72973")
		err = trustCert(oc, serInfo.serviceName, dirname+"/tls.crt", "trusted-ca-72973")
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Start mirroring of additionalImages to disk")
		waitErr := wait.Poll(300*time.Second, 3600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "file://"+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson").Execute()
			if err != nil {
				e2e.Logf("The mirror2disk for additionalImages failed, retrying...")
				return false, nil
			}
			return true, nil

		})
		exutil.AssertWaitPollNoErr(waitErr, "Max time reached but mirror2disk for additionalImages still failed")

		exutil.By("Start mirroring of additionalImages to registry")
		waitErr = wait.Poll(300*time.Second, 3600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "--from", "file://"+dirname, "docker://"+serInfo.serviceName+"/multiarch", "--v2", "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Execute()
			if err != nil {
				e2e.Logf("The disk2mirror of additionalImages failed, retrying...")
				return false, nil
			}
			return true, nil

		})
		exutil.AssertWaitPollNoErr(waitErr, "Max time reached but disk2mirror for additionalImages still failed")

		// Validate if multi arch additionalImages have been mirrored
		exutil.By("Validate if multi arch additionalImages have been mirrored")
		additionalImageList := []string{"/multiarch/ubi8/ubi:latest", "/multiarch/openshifttest/hello-openshift@sha256:61b8f5e1a3b5dbd9e2c35fd448dc5106337d7a299873dd3a6f0cd8d4891ecc27", "/multiarch/openshifttest/scratch@sha256:b045c6ba28db13704c5cbf51aff3935dbed9a692d508603cc80591d89ab26308"}
		for _, image := range additionalImageList {
			if strings.Contains(image, "scratch") {
				o.Expect(assertMultiImage(serInfo.serviceName+image, dirname+"/.dockerconfigjson")).To(o.BeFalse())
			} else {
				o.Expect(assertMultiImage(serInfo.serviceName+image, dirname+"/.dockerconfigjson")).To(o.BeTrue())
			}
		}

	})

	g.It("NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Author:yinzhou-Critical-73359-Validate mirror2mirror for operator for v2 [Serial]", func() {
		exutil.By("Set registry config")
		dirname := "/tmp/case73359"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Get root ca")
		err = getRouteCAToFile(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		exutil.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)
		setRegistryVolume(oc, "deploy", "registry", oc.Namespace(), "20G", "/var/lib/registry")

		exutil.By("Configure the Registry Certificate as trusted for cincinnati")
		addCA, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("image.config.openshift.io/cluster", "-o=jsonpath={.spec.additionalTrustedCA}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer restoreAddCA(oc, addCA, "trusted-ca-73359")
		err = trustCert(oc, serInfo.serviceName, dirname+"/tls.crt", "trusted-ca-73359")
		o.Expect(err).NotTo(o.HaveOccurred())
		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-73359.yaml")

		exutil.By("Start mirror2mirror")
		defer os.RemoveAll(".oc-mirror.log")
		waitErr := wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--workspace", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Execute()
			if err != nil {
				e2e.Logf("The mirror2mirror failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2mirror still failed")

		exutil.By("Create the catalogsource, idms and itms")
		defer operateCSAndMs(oc, dirname+"/working-dir/cluster-resources", "delete")
		operateCSAndMs(oc, dirname+"/working-dir/cluster-resources", "create")
		exutil.By("Check for the catalogsource pod status")
		assertPodOutput(oc, "olm.catalogSource=cs-redhatcatalog73359-v4-14", "openshift-marketplace", "Running")

		exutil.By("Install the operator from the new catalogsource")
		localstorageSub, localstorageOG := getOperatorInfo(oc, "local-storage-operator", "openshift-local-storage", "registry.redhat.io/redhat/redhat-operator-index:v4.14", "cs-redhatcatalog73359-v4-14")
		defer removeOperatorFromCustomCS(oc, localstorageSub, localstorageOG, "openshift-local-storage")
		installOperatorFromCustomCS(oc, localstorageSub, localstorageOG, "openshift-local-storage", "local-storage-operator")
	})

	g.It("NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Author:yinzhou-High-73452-Validate mirror2mirror for OCI operator  and addition image for v2 [Serial]", func() {
		exutil.By("Set registry config")
		dirname := "/tmp/case73452"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Get root ca")
		err = getRouteCAToFile(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		exutil.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)
		setRegistryVolume(oc, "deploy", "registry", oc.Namespace(), "20G", "/var/lib/registry")

		exutil.By("Configure the Registry Certificate as trusted for cincinnati")
		addCA, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("image.config.openshift.io/cluster", "-o=jsonpath={.spec.additionalTrustedCA}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer restoreAddCA(oc, addCA, "trusted-ca-73452")
		err = trustCert(oc, serInfo.serviceName, dirname+"/tls.crt", "trusted-ca-73452")
		o.Expect(err).NotTo(o.HaveOccurred())
		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-73452.yaml")

		exutil.By("Skopeo oci to localhost")
		command := fmt.Sprintf("skopeo copy --all docker://registry.redhat.io/redhat/redhat-operator-index:v4.16 oci://%s  --remove-signatures --insecure-policy --authfile %s", dirname+"/redhat-operator-index", dirname+"/.dockerconfigjson")
		waitErr := wait.Poll(30*time.Second, 180*time.Second, func() (bool, error) {
			_, err := exec.Command("bash", "-c", command).Output()
			if err != nil {
				e2e.Logf("copy failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("max time reached but the skopeo copy still failed"))

		exutil.By("Start mirror2mirror")
		defer os.RemoveAll(".oc-mirror.log")
		waitErr = wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--workspace", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Execute()
			if err != nil {
				e2e.Logf("The mirror2mirror failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2mirror still failed")

		exutil.By("Create the catalogsource, idms and itms")
		defer operateCSAndMs(oc, dirname+"/working-dir/cluster-resources", "delete")
		operateCSAndMs(oc, dirname+"/working-dir/cluster-resources", "create")
		exutil.By("Check for the catalogsource pod status")
		assertPodOutput(oc, "olm.catalogSource=cs-ocicatalog73452-v14", "openshift-marketplace", "Running")

		exutil.By("Install the operator from the new catalogsource")
		deschedulerSub, deschedulerOG := getOperatorInfo(oc, "cluster-kube-descheduler-operator", "openshift-kube-descheduler-operator", "registry.redhat.io/redhat/redhat-operator-index:v4.16", "cs-ocicatalog73452-v14")
		defer removeOperatorFromCustomCS(oc, deschedulerSub, deschedulerOG, "openshift-kube-descheduler-operator")
		installOperatorFromCustomCS(oc, deschedulerSub, deschedulerOG, "openshift-kube-descheduler-operator", "descheduler-operator")
	})

	g.It("NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Author:knarra-Medium-73377-support dry-run for v2 [Serial]", func() {
		dirname := "/tmp/case73377"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", "--to="+dirname, "--confirm").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-73377.yaml")

		err = getRouteCAToFile(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())
		command := fmt.Sprintf("skopeo copy --all --format v2s2 docker://icr.io/cpopen/ibm-zcon-zosconnect-catalog@sha256:6f02ecef46020bcd21bdd24a01f435023d5fc3943972ef0d9769d5276e178e76 oci://%s", dirname+"/ibm-catalog")
		waitErr := wait.Poll(30*time.Second, 180*time.Second, func() (bool, error) {
			_, err := exec.Command("bash", "-c", command).Output()
			if err != nil {
				e2e.Logf("Copy of ibm catalog failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("Max time reached but skopeo copy of ibm catalog failed"))

		exutil.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}
		exutil.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)
		exutil.By("Configure the Registry Certificate as trusted for cincinnati")
		addCA, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("image.config.openshift.io/cluster", "-o=jsonpath={.spec.additionalTrustedCA}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer restoreAddCA(oc, addCA, "trusted-ca-73377")
		err = trustCert(oc, serInfo.serviceName, dirname+"/tls.crt", "trusted-ca-73377")
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Start dry run of mirrro2disk")
		waitErr = wait.Poll(30*time.Second, 900*time.Second, func() (bool, error) {
			mirrorToDiskOutput, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "file://"+dirname, "--dry-run", "--v2", "--authfile", dirname+"/.dockerconfigjson").Output()
			if err != nil {
				e2e.Logf("The mirror2disk failed, retrying...")
				return false, nil
			}
			if strings.Contains(mirrorToDiskOutput, "dry-run/missing.txt") && strings.Contains(mirrorToDiskOutput, "dry-run/mapping.txt") {
				e2e.Logf("Mirror to Disk dry run has been completed successfully")
				return true, nil
			}
			return false, nil

		})
		exutil.AssertWaitPollNoErr(waitErr, "Max time reached but mirror2disk still failed")

		// Validate if source and destination are right in the mapping.txt file
		exutil.By("check if source and destination are right in the mapping.txt file")
		mappingTextContent, err := exec.Command("bash", "-c", fmt.Sprintf("cat /tmp/case73377/working-dir/dry-run/mapping.txt | head -n 10")).Output()
		e2e.Logf("mappingTextContent is %s", mappingTextContent)
		if err != nil {
			e2e.Logf("Error reading file must-gather.logs:", err)
		}
		mappingTextContentStr := string(mappingTextContent)

		if matched, _ := regexp.MatchString(".*docker://registry.redhat.io.*=docker://localhost:55000.*", mappingTextContentStr); !matched {
			e2e.Failf("Source and destination for mirror2disk mode is incorrect in mapping.txt")
		} else {
			e2e.Logf("Source and destination for mirror2disk are set correctly")
		}

		exutil.By("Start mirror2disk")
		waitErr = wait.Poll(30*time.Second, 900*time.Second, func() (bool, error) {
			_, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "file://"+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson").Output()
			if err != nil {
				e2e.Logf("The mirror2disk failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "Max time reached but mirror2disk still failed")

		exutil.By("Start dry run of disk2mirror")
		waitErr = wait.Poll(30*time.Second, 900*time.Second, func() (bool, error) {
			diskToMirrorOutput, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "--from", "file://"+dirname, "docker://"+serInfo.serviceName+":5000/d2m", "--v2", "--dry-run", "--authfile", dirname+"/.dockerconfigjson").Output()
			if err != nil {
				e2e.Logf("The disk2mirror failed, retrying...")
				return false, nil
			}
			if strings.Contains(diskToMirrorOutput, "dry-run/mapping.txt") {
				e2e.Logf("Disk to mirror dry run has been completed successfully")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "Max time reached but disk2mirror still failed")

		// Check if source and destination are right for disk2mirror in mapping.txt file
		mappingTextContentd2m, err := exec.Command("bash", "-c", fmt.Sprintf("cat /tmp/case73377/working-dir/dry-run/mapping.txt | head -n 10")).Output()
		e2e.Logf("mappingTextContent is %s", mappingTextContentd2m)
		if err != nil {
			e2e.Logf("Error reading file must-gather.logs:", err)
		}
		mappingTextContentd2mStr := string(mappingTextContentd2m)

		if matched, _ := regexp.MatchString(".*docker://localhost:55000.*=docker://"+serInfo.serviceName+":5000/d2m.*", mappingTextContentd2mStr); !matched {
			e2e.Failf("Source and destination for disk2mirror mode is incorrect in mapping.txt")
		} else {
			e2e.Logf("Source and destination for disk2mirror are set correctly")
		}

		exutil.By("Start dry run of mirror2mirror")
		waitErr = wait.Poll(30*time.Second, 900*time.Second, func() (bool, error) {
			mirrorToMirrorOutput, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName+":5000/m2m", "--workspace", "file://"+dirname, "--v2", "--dry-run", "--authfile", dirname+"/.dockerconfigjson").Output()
			if err != nil {
				e2e.Logf("The mirror2mirror failed, retrying...")
				return false, nil
			}
			if strings.Contains(mirrorToMirrorOutput, "dry-run/mapping.txt") {
				e2e.Logf("Mirror to mirror dry run has been completed successfully")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "Max time reached but mirror2mirror still failed")

		// Check if source and destination are right for mirror2mirror in mapping.txt file
		mappingTextContentm2m, err := exec.Command("bash", "-c", fmt.Sprintf("cat /tmp/case73377/working-dir/dry-run/mapping.txt | head -n 10")).Output()
		e2e.Logf("mappingTextContent is %s", mappingTextContentm2m)
		if err != nil {
			e2e.Logf("Error reading file must-gather.logs:", err)
		}
		mappingTextContentm2mStr := string(mappingTextContentm2m)

		if matched, _ := regexp.MatchString(".*docker://registry.redhat.io.*=docker://"+serInfo.serviceName+":5000/m2m.*", mappingTextContentm2mStr); !matched {
			e2e.Failf("Source and destination for mirror2mirror mode is incorrect in mapping.txt")
		} else {
			e2e.Logf("Source and destination for mirror2mirror are set correctly")
		}

	})

	g.It("NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Author:yinzhou-Medium-72949-support targetCatalog and targetTag setting of mirror v2docker2 and oci for v2 [Serial]", func() {
		exutil.By("Set registry config")
		dirname := "/tmp/case72949"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		exutil.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)
		setRegistryVolume(oc, "deploy", "registry", oc.Namespace(), "30G", "/var/lib/registry")

		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-72949-1.yaml")
		imageSetYamlFileS := filepath.Join(ocmirrorBaseDir, "config-72949-2.yaml")

		exutil.By("Use skopoe copy catalogsource to localhost")
		skopeExecute(fmt.Sprintf("skopeo copy --all --format v2s2 docker://icr.io/cpopen/ibm-zcon-zosconnect-catalog@sha256:6f02ecef46020bcd21bdd24a01f435023d5fc3943972ef0d9769d5276e178e76 oci://%s --remove-signatures", dirname+"/ibm-catalog"))

		exutil.By("Start mirror2mirror for oci & rh marketplace operators")
		waitErr := wait.PollImmediate(300*time.Second, 600*time.Second, func() (bool, error) {
			_, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileS, "docker://"+serInfo.serviceName, "--v2", "--workspace", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
			if err != nil {
				e2e.Logf("The mirror2mirror failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2mirror still failed")

		rhMarketUri := "https://" + serInfo.serviceName + "/v2/72949/redhat-marketplace-index/tags/list"
		validateTargetcatalogAndTag(rhMarketUri, "v15")
		ibmOciUri := "https://" + serInfo.serviceName + "/v2/72949/catalog/tags/list"
		validateTargetcatalogAndTag(ibmOciUri, "v15")

		os.RemoveAll(".oc-mirror.log")
		exutil.By("Start mirror2disk")
		defer os.RemoveAll(".oc-mirror.log")
		defer os.RemoveAll("~/.oc-mirror/")
		waitErr = wait.PollImmediate(300*time.Second, 600*time.Second, func() (bool, error) {
			_, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "--v2", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson").Output()
			if err != nil {
				e2e.Logf("The mirror2disk failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2disk still failed")

		exutil.By("Start disk2mirror")
		waitErr = wait.PollImmediate(300*time.Second, 600*time.Second, func() (bool, error) {
			_, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--from", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
			if err != nil {
				e2e.Logf("The disk2mirror failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "max time reached but the disk2mirror still failed")

		exutil.By("Validate the target catalog and tag")
		rhOperatorUri := "https://" + serInfo.serviceName + "/v2/72949/redhat-operator-index/tags/list"
		e2e.Logf("The rhOperatorUri is %v", rhOperatorUri)
		validateTargetcatalogAndTag(rhOperatorUri, "v4.15")
	})

	g.It("NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Author:knarra-Medium-72938-should give clear information for invalid operator filter setting [Serial]", func() {
		dirname := "/tmp/case72938"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", "--to="+dirname, "--confirm").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-72938.yaml")
		imageSetYamlFileT := filepath.Join(ocmirrorBaseDir, "config-72938-1.yaml")

		exutil.By("Start mirrro2disk with min/max filtering")
		mirrorToDiskOutput, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "file://"+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson").Output()
		if err != nil {
			if strings.Contains(mirrorToDiskOutput, "cannot use channels/full and min/max versions at the same time") {
				e2e.Logf("Error related to invalid operator filter by min/max is seen")
			} else {
				e2e.Failf("Error related to filtering by channel and package min/max is not seen")
			}
		}

		exutil.By("Start mirror2disk min/max with full true filtering")
		mirrorToDiskOutputFT, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileT, "file://"+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson").Output()
		if err != nil {
			if strings.Contains(mirrorToDiskOutputFT, "cannot use channels/full and min/max versions at the same time") {
				e2e.Logf("Error related to invalid operator filtering with full true is seen")
			} else {
				e2e.Failf("Error related to invalid operator filtering with full true is not seen")
			}
		}

	})

	g.It("NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Author:yinzhou-High-72942-High-72918-High-72709-support max-nested-paths for v2 [Serial]", func() {
		exutil.By("Set registry config")
		dirname := "/tmp/case72942"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.By("Get root ca")
		err = getRouteCAToFile(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Use skopoe copy catalogsource to localhost")
		skopeExecute(fmt.Sprintf("skopeo copy --all docker://registry.redhat.io/redhat/redhat-operator-index:v4.15 --remove-signatures  --insecure-policy oci://%s --authfile %s", dirname+"/redhat-operator-index", dirname+"/.dockerconfigjson"))

		exutil.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		exutil.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)
		setRegistryVolume(oc, "deploy", "registry", oc.Namespace(), "30G", "/var/lib/registry")

		exutil.By("Configure the Registry Certificate as trusted for cincinnati")
		addCA, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("image.config.openshift.io/cluster", "-o=jsonpath={.spec.additionalTrustedCA}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer restoreAddCA(oc, addCA, "trusted-ca-72942")
		err = trustCert(oc, serInfo.serviceName, dirname+"/tls.crt", "trusted-ca-72942")
		o.Expect(err).NotTo(o.HaveOccurred())
		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-72942.yaml")

		exutil.By("Start mirror2disk, checkpoint for 72947 and 72918")
		defer os.RemoveAll(".oc-mirror.log")
		defer os.RemoveAll("~/.oc-mirror/")
		waitErr := wait.PollImmediate(300*time.Second, 600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "--v2", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson").Execute()
			if err != nil {
				e2e.Logf("The mirror2disk failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2disk still failed")

		exutil.By("Start disk2mirror with max-nested-paths")
		waitErr = wait.PollImmediate(300*time.Second, 600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName+"/test/72942", "--v2", "--from", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false", "--max-nested-paths", "2").Execute()
			if err != nil {
				e2e.Logf("The disk2mirror failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "max time reached but the disk2mirror still failed")

		exutil.By("Check if net path is right in idms/itms file")
		idmsTextContentStr := readFileContent("/tmp/case72942/working-dir/cluster-resources/idms-oc-mirror.yaml")
		validateFileContent(idmsTextContentStr, "test/72942-albo-aws-load-balancer-operator-bundle", "operator")
		validateFileContent(idmsTextContentStr, "test/72942-openshifttest-hello-openshift", "additionalimage")

		itmsTextContentStr := readFileContent("/tmp/case72942/working-dir/cluster-resources/itms-oc-mirror.yaml")
		validateFileContent(itmsTextContentStr, "test/72942-ubi8-ubi", "additionalimage")

		exutil.By("Create the catalogsource, idms and itms")
		defer operateCSAndMs(oc, dirname+"/working-dir/cluster-resources", "delete")
		operateCSAndMs(oc, dirname+"/working-dir/cluster-resources", "create")
		exutil.By("Check for the catalogsource pod status")
		assertPodOutput(oc, "olm.catalogSource=cs-72942-catalog-v15", "openshift-marketplace", "Running")
		assertPodOutput(oc, "olm.catalogSource=cs-72942-redhat-redhat-operator-index-v4-15", "openshift-marketplace", "Running")

		exutil.By("Checkpoint for 72709, validate the result for additional images")
		_, outErr, err := oc.AsAdmin().WithoutNamespace().Run("image").Args("info", "--registry-config", dirname+"/.dockerconfigjson", serInfo.serviceName+"/test/72942-ubi8-ubi:latest", "--insecure").Outputs()
		o.Expect(err).Should(o.HaveOccurred())
		o.Expect(strings.Contains(outErr, "the image is a manifest list")).To(o.BeTrue())
		_, outErr, err = oc.AsAdmin().WithoutNamespace().Run("image").Args("info", "--registry-config", dirname+"/.dockerconfigjson", serInfo.serviceName+"/test/72942-openshifttest-hello-openshift@sha256:4200f438cf2e9446f6bcff9d67ceea1f69ed07a2f83363b7fb52529f7ddd8a83", "--insecure").Outputs()
		o.Expect(err).Should(o.HaveOccurred())
		o.Expect(strings.Contains(outErr, "the image is a manifest list")).To(o.BeTrue())
	})

	// author: yinzhou@redhat.com
	g.It("Author:yinzhou-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Critical-72947-High-72948-support OCI filtering for v2 [Serial]", func() {
		exutil.By("Set registry config")
		dirname := "/tmp/case72947"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		exutil.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)
		setRegistryVolume(oc, "deploy", "registry", oc.Namespace(), "50G", "/var/lib/registry")

		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-72948.yaml")

		exutil.By("Use skopoe copy catalogsource to localhost")
		skopeExecute(fmt.Sprintf("skopeo copy --all docker://registry.redhat.io/redhat/redhat-operator-index:v4.15 oci://%s --remove-signatures --insecure-policy --authfile %s", dirname+"/redhat-operator-index", dirname+"/.dockerconfigjson"))

		exutil.By("Start mirror2mirror for oci operators")
		defer os.RemoveAll(".oc-mirror.log")
		waitErr := wait.PollImmediate(300*time.Second, 600*time.Second, func() (bool, error) {
			_, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--workspace", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
			if err != nil {
				e2e.Logf("The mirror2mirror failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2mirror still failed")
		exutil.By("Create the catalogsource, idms and itms")
		defer operateCSAndMs(oc, dirname+"/working-dir/cluster-resources", "delete")
		operateCSAndMs(oc, dirname+"/working-dir/cluster-resources", "create")
	})

	g.It("Author:yinzhou-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-High-72913-Respect archive max size for v2 [Serial]", func() {
		exutil.By("Set registry config")
		dirname := "/tmp/case72913"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())
		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-72913.yaml")

		exutil.By("Start mirror2disk with strict-archive")
		defer os.RemoveAll(".oc-mirror.log")
		defer os.RemoveAll("~/.oc-mirror/")
		outputMes, _, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "--v2", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--strict-archive").Outputs()
		o.Expect(err).Should(o.HaveOccurred())
		o.Expect(strings.Contains(outputMes, "maxArchiveSize 1G is too small compared to sizes of files")).To(o.BeTrue())

		exutil.By("Start mirror2disk without strict-archive")
		waitErr := wait.PollImmediate(300*time.Second, 600*time.Second, func() (bool, error) {
			_, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "--v2", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson").Output()
			if err != nil {
				e2e.Logf("The mirror2disk failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2disk still failed")
	})

	g.It("Author:yinzhou-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-High-72972-Medium-73381-Medium-74519-support to specify architectures of payload for v2 [Serial]", func() {
		exutil.By("Set registry config")
		dirname := "/tmp/case72972"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())
		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-72972.yaml")
		imageDeleteYamlFileF := filepath.Join(ocmirrorBaseDir, "delete-config-72972.yaml")

		exutil.By("Use skopoe copy catalogsource to localhost")
		skopeExecute(fmt.Sprintf("skopeo copy --all docker://registry.redhat.io/redhat/redhat-operator-index:v4.15 --remove-signatures  --insecure-policy oci://%s  --authfile %s", dirname+"/redhat-operator-index", dirname+"/.dockerconfigjson"))

		exutil.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		exutil.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)
		setRegistryVolume(oc, "deploy", "registry", oc.Namespace(), "40G", "/var/lib/registry")

		exutil.By("Start mirror2mirror ")
		defer os.RemoveAll(".oc-mirror.log")
		defer os.RemoveAll("~/.oc-mirror/")
		waitErr := wait.PollImmediate(300*time.Second, 600*time.Second, func() (bool, error) {
			_, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--workspace", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
			if err != nil {
				e2e.Logf("The mirror2mirror failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2mirror still failed")
		payloadImageInfo, err := oc.WithoutNamespace().Run("image").Args("info", "--insecure", serInfo.serviceName+"/openshift/release-images:4.15.19-s390x").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Payloadinfo is %s", payloadImageInfo)
		o.Expect(strings.Contains(payloadImageInfo, "s390x")).To(o.BeTrue())

		exutil.By("Checkpoint for 74519")
		exutil.By("Generete delete image file")
		_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("delete", "--config", imageDeleteYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--workspace", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--generate").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Execute delete without force-cache-delete")
		_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("delete", "--delete-yaml-file", dirname+"/working-dir/delete/delete-images.yaml", "docker://"+serInfo.serviceName, "--v2", "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.By("Checked the payload manifest again should failed")
		_, err = oc.WithoutNamespace().Run("image").Args("info", "--insecure", serInfo.serviceName+"/openshift/release-images:4.15.19-s390x").Output()
		o.Expect(err).Should(o.HaveOccurred())

		exutil.By("Checked the operator manifest again should failed")
		_, err = oc.WithoutNamespace().Run("image").Args("info", "--insecure", serInfo.serviceName+"/redhat/redhat-operator-index:v4.15").Output()
		o.Expect(err).Should(o.HaveOccurred())
	})

	g.It("Author:yinzhou-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-High-74649-Low-74646-Show warning when an eus channel with minor versions range >=2 for v2[Serial]", func() {
		dirname := "/tmp/case74649"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", "--to="+dirname, "--confirm").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-74649.yaml")

		exutil.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		exutil.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)
		setRegistryVolume(oc, "deploy", "registry", oc.Namespace(), "50G", "/var/lib/registry")

		exutil.By("Checkpoint for v2 m2d")
		err = wait.Poll(300*time.Second, 900*time.Second, func() (bool, error) {
			mirrorOutFile, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "--v2", "file://"+dirname+"/m2d", "--authfile", dirname+"/.dockerconfigjson").OutputToFile(getRandomString() + "workload-mirror.txt")
			if err != nil {
				e2e.Logf("the err:%v, and try next round", err)
				return false, nil
			}
			if !validateStringFromFile(mirrorOutFile, "To correctly determine the upgrade path for EUS releases") {
				return false, fmt.Errorf("Upgrade warning related to correctly determing the upgrade path is not seen for m2d v2")
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Mirror command failed with %s", err))
		err = os.RemoveAll(dirname + "/m2d")
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Checkpoint for v2 m2m")
		err = wait.Poll(300*time.Second, 900*time.Second, func() (bool, error) {
			mirrorOutFile, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "--v2", "--workspace", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "docker://"+serInfo.serviceName, "--dest-tls-verify=false").OutputToFile(getRandomString() + "workload-m2m.txt")
			if err != nil {
				e2e.Logf("the err:%v, and try next round", err)
				return false, nil
			}
			if !validateStringFromFile(mirrorOutFile, "To correctly determine the upgrade path for EUS releases") {
				return false, fmt.Errorf("Upgrade warning related to correctly determing the upgrade path is not seen for m2m v2")
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Mirror command failed with %s", err))

		exutil.By("Checkpoint for 74646: show warning when an eus channle with minor versions range >=2 for v2 m2d with dry-run")
		err = wait.Poll(300*time.Second, 900*time.Second, func() (bool, error) {
			mirrorOutFile, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "--v2", "file://"+dirname+"/m2d", "--authfile", dirname+"/.dockerconfigjson", "--dry-run").OutputToFile(getRandomString() + "workload-mirror.txt")
			if err != nil {
				e2e.Logf("the err:%v, and try next round", err)
				return false, nil
			}
			if !validateStringFromFile(mirrorOutFile, "To correctly determine the upgrade path for EUS releases") {
				return false, fmt.Errorf("Upgrade warning related to correctly determing the upgrade path is not seen for m2d v2 dry-run")
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Mirror command failed with %s", err))

		exutil.By("Checkpoint for 74646: show warning when an eus channle with minor versions range >=2 for  v2 m2m with dry-run")
		err = wait.Poll(300*time.Second, 900*time.Second, func() (bool, error) {
			mirrorOutFile, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "--v2", "--workspace", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "docker://"+serInfo.serviceName, "--dest-tls-verify=false", "--dry-run").OutputToFile(getRandomString() + "workload-m2m.txt")
			if err != nil {
				e2e.Logf("the err:%v, and try next round", err)
				return false, nil
			}
			if !validateStringFromFile(mirrorOutFile, "To correctly determine the upgrade path for EUS releases") {
				return false, fmt.Errorf("Upgrade warning related to correctly determing the upgrade path is not seen for m2m v2 dry-run")
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Mirror command failed with %s", err))
	})

	g.It("Author:yinzhou-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-High-74650-Should no warning about eus when use the eus channel with minor versions range < 2  for V1[Serial]", func() {
		dirname := "/tmp/case74650"
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(dirname)
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		exutil.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads")
		imageSetV1MinorFile := filepath.Join(ocmirrorBaseDir, "config-74650-minor-v1.yaml")
		imageSetV1PatchFile := filepath.Join(ocmirrorBaseDir, "config-74650-patch-v1.yaml")

		defer os.RemoveAll("oc-mirror-workspace")
		exutil.By("Step 1 : no warning when minor diff < 2 for v1")
		err = wait.Poll(300*time.Second, 900*time.Second, func() (bool, error) {
			mirrorOutFile, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetV1MinorFile, "file://"+dirname+"/m2d", "--dry-run").OutputToFile(getRandomString() + "workload-mirror.txt")
			if err != nil {
				e2e.Logf("the err:%v, and try next round", err)
				return false, nil
			}
			if validateStringFromFile(mirrorOutFile, "To correctly determine the upgrade path for EUS releases") {
				return false, fmt.Errorf("Upgrade warning related to correctly determing the upgrade path is showing for minor diff <2 for v1 m2d")
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Mirror command failed with %s", err))

		err = wait.Poll(300*time.Second, 900*time.Second, func() (bool, error) {
			mirrorOutFile, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetV1MinorFile, "docker://"+serInfo.serviceName, "--dest-skip-tls", "--dry-run").OutputToFile(getRandomString() + "workload-m2m.txt")
			if err != nil {
				e2e.Logf("the err:%v, and try next round", err)
				return false, nil
			}
			if validateStringFromFile(mirrorOutFile, "To correctly determine the upgrade path for EUS releases") {
				return false, fmt.Errorf("Upgrade warning related to correctly determing the upgrade path is showing for minor diff <2 for v1 m2m")
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Mirror command failed with %s", err))

		exutil.By("Step 2 : no warning when patch diff for v1")
		err = wait.Poll(300*time.Second, 900*time.Second, func() (bool, error) {
			mirrorOutFile, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetV1PatchFile, "file://"+dirname+"/m2d", "--dry-run").OutputToFile(getRandomString() + "workload-mirror.txt")
			if err != nil {
				e2e.Logf("the err:%v, and try next round", err)
				return false, nil
			}
			if validateStringFromFile(mirrorOutFile, "To correctly determine the upgrade path for EUS releases") {
				return false, fmt.Errorf("Upgrade warning related to correctly determing the upgrade path is showing for patch diff for v1 m2d")
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Mirror command failed with %s", err))

		err = wait.Poll(300*time.Second, 900*time.Second, func() (bool, error) {
			mirrorOutFile, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetV1PatchFile, "docker://"+serInfo.serviceName, "--dest-skip-tls", "--dry-run").OutputToFile(getRandomString() + "workload-m2m.txt")
			if err != nil {
				e2e.Logf("the err:%v, and try next round", err)
				return false, nil
			}
			if validateStringFromFile(mirrorOutFile, "To correctly determine the upgrade path for EUS releases") {
				return false, fmt.Errorf("Upgrade warning related to correctly determing the upgrade path is showing for patch diff for v1 m2m")
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Mirror command failed with  %s", err))

	})

	g.It("Author:yinzhou-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-High-74660-Low-74646-Show warning when an eus channel with minor versions range >=2 for v1[Serial]", func() {
		dirname := "/tmp/case74660"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())
		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-74660.yaml")

		exutil.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		exutil.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)
		setRegistryVolume(oc, "deploy", "registry", oc.Namespace(), "50G", "/var/lib/registry")

		defer os.RemoveAll("oc-mirror-workspace")
		exutil.By("Checkpoint for v1 m2d")
		err = wait.Poll(300*time.Second, 900*time.Second, func() (bool, error) {
			mirrorOutFile, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "file://"+dirname+"/m2d").OutputToFile(getRandomString() + "workload-mirror.txt")
			if err != nil {
				e2e.Logf("the err:%v, and try next round", err)
				return false, nil
			}
			if !validateStringFromFile(mirrorOutFile, "To correctly determine the upgrade path for EUS releases") {
				return false, fmt.Errorf("V1 m2d test failed as can't find the expected warning")
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Mirror command failed with %s", err))
		err = os.RemoveAll(dirname + "/m2d")
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Checkpoint for v1 m2m")
		err = wait.Poll(300*time.Second, 900*time.Second, func() (bool, error) {
			mirrorOutFile, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName, "--dest-skip-tls").OutputToFile(getRandomString() + "workload-m2m.txt")
			if err != nil {
				e2e.Logf("the err:%v, and try next round", err)
				return false, nil
			}
			if !validateStringFromFile(mirrorOutFile, "To correctly determine the upgrade path for EUS releases") {
				return false, fmt.Errorf("V1 m2m test failed as can't find the expected warning")
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Mirror command failed with %s", err))

		exutil.By("Checkpoint for 74646: show warning when an eus channle with minor versions range >=2 for v1 m2d with dry-run")
		err = wait.Poll(300*time.Second, 900*time.Second, func() (bool, error) {
			mirrorOutFile, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "file://"+dirname+"/m2d", "--dry-run").OutputToFile(getRandomString() + "workload-mirror.txt")
			if err != nil {
				e2e.Logf("the err:%v, and try next round", err)
				return false, nil
			}
			if !validateStringFromFile(mirrorOutFile, "To correctly determine the upgrade path for EUS releases") {
				return false, fmt.Errorf("V1 m2d with dry-run test failed as can't find the expected warning")
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Mirror command failed with %s", err))

		exutil.By("Checkpoint for 74646: show warning when an eus channel with minor versions range >=2 for v1 m2m with dry-run")
		err = wait.Poll(300*time.Second, 900*time.Second, func() (bool, error) {
			mirrorOutFile, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName, "--dest-skip-tls", "--dry-run").OutputToFile(getRandomString() + "workload-m2m.txt")
			if err != nil {
				e2e.Logf("the err:%v, and try next round", err)
				return false, nil
			}
			if !validateStringFromFile(mirrorOutFile, "To correctly determine the upgrade path for EUS releases") {
				return false, fmt.Errorf("V1 m2m with dry-run test failed as can't find the expected warning")
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Mirror command failed with %s", err))
	})

	g.It("Author:yinzhou-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-High-74733-Should no warning about eus when use the eus channel with minor versions range < 2  for V2[Serial]", func() {
		dirname := "/tmp/case74733"
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(dirname)
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		exutil.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads")
		imageSetV2MinorFile := filepath.Join(ocmirrorBaseDir, "config-74650-minor-v2.yaml")
		imageSetV2PatchFile := filepath.Join(ocmirrorBaseDir, "config-74650-patch-v2.yaml")

		exutil.By("Step 1 : no warning when minor diff < 2 for v2")
		err = wait.Poll(300*time.Second, 900*time.Second, func() (bool, error) {
			mirrorOutFile, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetV2MinorFile, "--v2", "file://"+dirname+"/m2d", "--authfile", dirname+"/.dockerconfigjson", "--dry-run").OutputToFile(getRandomString() + "workload-mirror.txt")
			if err != nil {
				e2e.Logf("the err:%v, and try next round", err)
				return false, nil
			}
			if validateStringFromFile(mirrorOutFile, "To correctly determine the upgrade path for EUS releases") {
				return false, fmt.Errorf("Upgrade warning related to correctly determing the upgrade path is showing for minor diff <2 for v2 m2d")
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Mirror command failed with  %s", err))

		err = wait.Poll(300*time.Second, 900*time.Second, func() (bool, error) {
			mirrorOutFile, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetV2MinorFile, "--v2", "--workspace", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "docker://"+serInfo.serviceName, "--dest-tls-verify=false", "--dry-run").OutputToFile(getRandomString() + "workload-m2m.txt")
			if err != nil {
				e2e.Logf("the err:%v, and try next round", err)
				return false, nil
			}
			if validateStringFromFile(mirrorOutFile, "To correctly determine the upgrade path for EUS releases") {
				return false, fmt.Errorf("Upgrade warning related to correctly determing the upgrade path is showing for minor diff <2 for v2 m2m")
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Mirror command failed with %s", err))

		exutil.By("Step 2 : no warning when patch diff for v2")
		err = wait.Poll(300*time.Second, 900*time.Second, func() (bool, error) {
			mirrorOutFile, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetV2PatchFile, "--v2", "file://"+dirname+"/m2d", "--authfile", dirname+"/.dockerconfigjson", "--dry-run").OutputToFile(getRandomString() + "workload-mirror.txt")
			if err != nil {
				e2e.Logf("the err:%v, and try next round", err)
				return false, nil
			}
			if validateStringFromFile(mirrorOutFile, "To correctly determine the upgrade path for EUS releases") {
				return false, fmt.Errorf("Upgrade warning related to correctly determing the upgrade path is showing for patch diff for v2 m2d")
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Mirror command failed with %s", err))

		err = wait.Poll(300*time.Second, 900*time.Second, func() (bool, error) {
			mirrorOutFile, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetV2PatchFile, "--v2", "--workspace", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "docker://"+serInfo.serviceName, "--dest-tls-verify=false", "--dry-run").OutputToFile(getRandomString() + "workload-m2m.txt")
			if err != nil {
				e2e.Logf("the err:%v, and try next round", err)
				return false, nil
			}
			if validateStringFromFile(mirrorOutFile, "To correctly determine the upgrade path for EUS releases") {
				return false, fmt.Errorf("Upgrade warning related to correctly determing the upgrade path is showing for patch diff for v2 m2m")
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Mirror command failed with  %s", err))
	})

	// author: knarra@redhat.com
	g.It("Author:knarra-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Medium-73783-Do not generate IDMS or ITMS if nothing has been mirrored [Serial]", func() {
		dirname := "/tmp/case73783"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", "--to="+dirname, "--confirm").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-73783.yaml")

		exutil.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}
		exutil.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)

		exutil.By("Start mirror2mirror and verify no idms and itms has been generated since nothing is mirrored")
		waitErr := wait.Poll(30*time.Second, 900*time.Second, func() (bool, error) {
			mirrorOutput, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "--workspace", "file://"+dirname, "docker://"+serInfo.serviceName+"/noidmsitms", "--v2", "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
			if err != nil {
				e2e.Logf("The mirror failed, retrying...")
				return false, nil
			}
			if strings.Contains(mirrorOutput, "Nothing mirrored. Skipping IDMS and ITMS files generation") && strings.Contains(mirrorOutput, "No catalogs mirrored. Skipping CatalogSource file generation") {
				e2e.Logf("No ITMS & IDMS generated when nothing is mirrored")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "Max time reached but could not find message about no IDMS and ITMS generation")
	})

	// author: knarra@redhat.com
	g.It("Author:knarra-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Low-73421-Verify oc-mirror throws error when using invalid imageSetConfig with bundles [Serial]", func() {
		dirname := "/tmp/case73421"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", "--to="+dirname, "--confirm").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-73421.yaml")
		imageSetYamlFileT := filepath.Join(ocmirrorBaseDir, "config-73421-1.yaml")

		exutil.By("Start mirrro2disk with bundles and min/max filtering")
		mirrorToDiskOutput, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "file://"+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson").Output()
		if err != nil {
			if strings.Contains(mirrorToDiskOutput, "mixing both filtering by bundles and filtering by channels or minVersion/maxVersion is not allowed") {
				e2e.Logf("Error related to mixing both bundles, min & max version allowed is seen")
			} else {
				e2e.Failf("Error related to mixing both bundles, min & max version allowed is not seen")
			}
		}

		exutil.By("Start mirror2disk with bundles & filtering with full true")
		mirrorToDiskOutputFT, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileT, "file://"+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson").Output()
		if err != nil {
			if strings.Contains(mirrorToDiskOutputFT, "cannot use filtering by bundle selection and full the same time") {
				e2e.Logf("Error related to cannot use filtering by bundle selection and full at the same time is seen")
			} else {
				e2e.Failf("Error related to cannot use filtering by bundle selection and full at the same time is not seen")
			}
		}

	})

	g.It("Author:yinzhou-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-High-72971-support mirror multiple catalogs (v2docker2 +oci) for v2 [Serial]", func() {
		exutil.By("Set registry config")
		dirname := "/tmp/case72971"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())
		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-72971.yaml")

		exutil.By("Use skopoe copy catalogsource to localhost")
		skopeExecute(fmt.Sprintf("skopeo copy --all docker://registry.redhat.io/redhat/redhat-operator-index:v4.15 --remove-signatures  --insecure-policy oci://%s", dirname+"/redhat-operator-index"))

		exutil.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		exutil.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)
		setRegistryVolume(oc, "deploy", "registry", oc.Namespace(), "20G", "/var/lib/registry")

		exutil.By("Get root ca")
		err = getRouteCAToFile(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.By("Configure the Registry Certificate as trusted for cincinnati")
		addCA, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("image.config.openshift.io/cluster", "-o=jsonpath={.spec.additionalTrustedCA}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer restoreAddCA(oc, addCA, "trusted-ca-72971")
		err = trustCert(oc, serInfo.serviceName, dirname+"/tls.crt", "trusted-ca-72971")
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Start mirror2mirror ")
		defer os.RemoveAll(".oc-mirror.log")
		defer os.RemoveAll("~/.oc-mirror/")
		waitErr := wait.PollImmediate(300*time.Second, 600*time.Second, func() (bool, error) {
			_, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--workspace", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
			if err != nil {
				e2e.Logf("The mirror2mirror failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2mirror still failed")

		exutil.By("Create the catalogsource, idms and itms")
		defer operateCSAndMs(oc, dirname+"/working-dir/cluster-resources", "delete")
		operateCSAndMs(oc, dirname+"/working-dir/cluster-resources", "create")
		exutil.By("Check for the catalogsource pod status")
		assertPodOutput(oc, "olm.catalogSource=cs-certified-operator-index-v4-16", "openshift-marketplace", "Running")
		assertPodOutput(oc, "olm.catalogSource=cs-community-operator-index-v4-16", "openshift-marketplace", "Running")
		assertPodOutput(oc, "olm.catalogSource=cs-redhat-marketplace-index-v4-16", "openshift-marketplace", "Running")

		exutil.By("Install operator from certified-operator CS")
		nginxSub, nginxOG := getOperatorInfo(oc, "nginx-ingress-operator", "nginx-ingress-operator-ns", "registry.redhat.io/redhat/certified-operator-index:v4.16", "cs-certified-operator-index-v4-16")
		defer removeOperatorFromCustomCS(oc, nginxSub, nginxOG, "nginx-ingress-operator-ns")
		installOperatorFromCustomCS(oc, nginxSub, nginxOG, "nginx-ingress-operator-ns", "nginx-ingress-operator-controller-manager")

		exutil.By("Install operator from redhat-marketplace CS")
		aerospikeSub, aerospikeOG := getOperatorInfo(oc, "aerospike-kubernetes-operator-rhmp", "aerospike-ns", "registry.redhat.io/redhat/redhat-marketplace-index:v4.16", "cs-redhat-marketplace-index-v4-16")
		defer removeOperatorFromCustomCS(oc, aerospikeSub, aerospikeOG, "aerospike-ns")
		installCustomOperator(oc, aerospikeSub, aerospikeOG, "aerospike-ns", "aerospike-operator-controller-manager", "2")
	})

	g.It("Author:yinzhou-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Critical-72917-support v2docker2 operator catalog filtering for v2 [Serial]", func() {
		exutil.By("Set registry config")
		dirname := "/tmp/case72917"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())
		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-72917.yaml")

		exutil.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}
		exutil.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)
		setRegistryVolume(oc, "deploy", "registry", oc.Namespace(), "35G", "/var/lib/registry")

		exutil.By("Get root ca")
		err = getRouteCAToFile(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.By("Configure the Registry Certificate as trusted for cincinnati")
		addCA, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("image.config.openshift.io/cluster", "-o=jsonpath={.spec.additionalTrustedCA}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer restoreAddCA(oc, addCA, "trusted-ca-72917")
		err = trustCert(oc, serInfo.serviceName, dirname+"/tls.crt", "trusted-ca-72917")
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Start mirror2mirror ")
		defer os.RemoveAll(".oc-mirror.log")
		defer os.RemoveAll("~/.oc-mirror/")
		waitErr := wait.PollImmediate(300*time.Second, 600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--workspace", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Execute()
			if err != nil {
				e2e.Logf("The mirror2mirror failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2mirror still failed")

		exutil.By("Create the catalogsource, idms and itms")
		defer operateCSAndMs(oc, dirname+"/working-dir/cluster-resources", "delete")
		operateCSAndMs(oc, dirname+"/working-dir/cluster-resources", "create")
		exutil.By("Check for the catalogsource pod status")
		assertPodOutput(oc, "olm.catalogSource=cs-redhat-operator-index-v4-15", "openshift-marketplace", "Running")
	})

	// author: knarra@redhat.com
	g.It("Author:knarra-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-High-73420-Critical-73419-Verify oc-mirror v2 skips and continues if selected bundle does not exist [Serial]", func() {
		dirname := "/tmp/case73420"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", "--to="+dirname, "--confirm").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-73420.yaml")

		err = getRouteCAToFile(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}
		exutil.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)

		exutil.By("Configure the Registry Certificate as trusted for cincinnati")
		addCA, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("image.config.openshift.io/cluster", "-o=jsonpath={.spec.additionalTrustedCA}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer restoreAddCA(oc, addCA, "trusted-ca-73420")
		err = trustCert(oc, serInfo.serviceName, dirname+"/tls.crt", "trusted-ca-73420")
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Start mirrro2disk")
		waitErr := wait.Poll(30*time.Second, 900*time.Second, func() (bool, error) {
			m2dOutputFile, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "file://"+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson").OutputToFile(getRandomString() + "workload-m2m.txt")
			if err != nil {
				e2e.Logf("The mirror2disk failed, retrying...")
				return false, nil
			}
			if !validateStringFromFile(m2dOutputFile, "bundle clusterkubedescheduleroperator.v3.0 of operator  cluster-kube-descheduler-operator not found in catalog: SKIPPING") && !validateStringFromFile(m2dOutputFile, "bundle cockroach-operator.v2.13.1 of operator cockroachdb-certified not found in catalog: SKIPPING") && !validateStringFromFile(m2dOutputFile, "bundle 3scale-community-operator.v0.9.1 of operator 3scale-community-operator not found in catalog: SKIPPING") {
				return false, fmt.Errorf("Do not see any bundles being skipped which is not expected")
			}
			return true, nil

		})
		exutil.AssertWaitPollNoErr(waitErr, "Max time reached but mirror2disk still failed")

		exutil.By("Start disk2mirror")
		waitErr = wait.Poll(30*time.Second, 900*time.Second, func() (bool, error) {
			_, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "--from", "file://"+dirname, "docker://"+serInfo.serviceName+"/d2m", "--v2", "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
			if err != nil {
				e2e.Logf("The disk2mirror failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "Max time reached but disk2mirror still failed")

		exutil.By("Create the catalogsource, idms and itms")
		defer operateCSAndMs(oc, dirname+"/working-dir/cluster-resources", "delete")
		operateCSAndMs(oc, dirname+"/working-dir/cluster-resources", "create")
		assertPodOutput(oc, "olm.catalogSource=cs-certified-operator-index-v4-14", "openshift-marketplace", "Running")
		assertPodOutput(oc, "olm.catalogSource=cs-community-operator-index-v4-14", "openshift-marketplace", "Running")
		assertPodOutput(oc, "olm.catalogSource=cs-redhat-operator-index-v4-16", "openshift-marketplace", "Running")

		exutil.By("Install the operator from the new catalogsource")
		rhkdoSub, rhkdoOG := getOperatorInfo(oc, "cluster-kube-descheduler-operator", "openshift-kube-descheduler-operator", "registry.redhat.io/redhat/redhat-operator-index:v4.16", "cs-redhat-operator-index-v4-16")
		defer removeOperatorFromCustomCS(oc, rhkdoSub, rhkdoOG, "openshift-kube-descheduler-operator")
		installOperatorFromCustomCS(oc, rhkdoSub, rhkdoOG, "openshift-kube-descheduler-operator", "descheduler-operator")

	})

	// Marking the test flaky due to issue https://issues.redhat.com/browse/CLID-214
	g.It("Author:knarra-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-High-73124-Validate operator mirroring works fine for the catalog that does not follow same structure as RHOI [Serial] [Flaky]", func() {
		exutil.By("Set registry config")
		dirname := "/tmp/case73124"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Get root ca")
		err = getRouteCAToFile(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		exutil.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)
		setRegistryVolume(oc, "deploy", "registry", oc.Namespace(), "20G", "/var/lib/registry")

		exutil.By("Configure the Registry Certificate as trusted for cincinnati")
		addCA, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("image.config.openshift.io/cluster", "-o=jsonpath={.spec.additionalTrustedCA}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer restoreAddCA(oc, addCA, "trusted-ca-73124")
		err = trustCert(oc, serInfo.serviceName, dirname+"/tls.crt", "trusted-ca-73124")
		o.Expect(err).NotTo(o.HaveOccurred())
		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-73124.yaml")

		exutil.By("Skopeo oci to localhost")
		command := fmt.Sprintf("skopeo copy --all --format v2s2 docker://icr.io/cpopen/ibm-bts-operator-catalog@sha256:866f0212eab7bc70cc7fcf7ebdbb4dfac561991f6d25900bd52f33cd90846adf oci://%s  --remove-signatures --insecure-policy", dirname+"/ibm-catalog")
		waitErr := wait.Poll(30*time.Second, 180*time.Second, func() (bool, error) {
			_, err := exec.Command("bash", "-c", command).Output()
			if err != nil {
				e2e.Logf("copy failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("max time reached but the skopeo copy still failed"))

		exutil.By("Start mirror2disk")
		defer os.RemoveAll(".oc-mirror.log")
		waitErr = wait.PollImmediate(30*time.Second, 900*time.Second, func() (bool, error) {
			_, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "file://"+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson").Output()
			if err != nil {
				e2e.Logf("The mirror2disk for oci ibm catalog failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2disk for oci ibm catalog failed")

		exutil.By("Start mirror2mirror")
		defer os.RemoveAll(".oc-mirror.log")
		waitErr = wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--workspace", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Execute()
			if err != nil {
				e2e.Logf("The mirror2mirror for ibm oci catalog failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2mirror for ibm oci catalog still failed")

		exutil.By("Create the catalogsource, idms and itms")
		defer operateCSAndMs(oc, dirname+"/working-dir/cluster-resources", "delete")
		operateCSAndMs(oc, dirname+"/working-dir/cluster-resources", "create")
		ibmCatalogSourceName, err := exec.Command("bash", "-c", fmt.Sprintf("oc get catalogsource -n openshift-marketplace | awk '{print $1}' | grep ibm")).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("ibmCatalogSourceName is %s", ibmCatalogSourceName)

		exutil.By("Check for the catalogsource pod status")
		assertPodOutput(oc, "olm.catalogSource="+string(ibmCatalogSourceName), "openshift-marketplace", "Running")

		exutil.By("Install the operator from the new catalogsource")
		buildPruningBaseDir := exutil.FixturePath("testdata", "workloads")
		ibmcatalogSubscription := filepath.Join(buildPruningBaseDir, "ibmcustomsub.yaml")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", ibmcatalogSubscription).Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", ibmcatalogSubscription).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		e2e.Logf("Wait for the operator pod running")
		if ok := waitForAvailableRsRunning(oc, "deploy", "ibm-zcon-zosconnect-controller-manager", "openshift-operators", "1"); ok {
			e2e.Logf("IBM operator with index structure different than RHOCI has been deployed successfully\n")
		} else {
			e2e.Failf("All pods related to ibm deployment are not running")
		}
	})

	g.It("Author:yinzhou-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Critical-72708-support delete function with force-cache-delete for V2 [Serial]", func() {
		exutil.By("Set registry config")
		dirname := "/tmp/case72708"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())
		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-72708.yaml")
		imageDeleteYamlFileF := filepath.Join(ocmirrorBaseDir, "delete-config-72708.yaml")

		exutil.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		exutil.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)
		setRegistryVolume(oc, "deploy", "registry", oc.Namespace(), "40G", "/var/lib/registry")

		exutil.By("Start mirror2disk")
		defer os.RemoveAll(".oc-mirror.log")
		defer os.RemoveAll("~/.oc-mirror/")
		waitErr := wait.Poll(300*time.Second, 900*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "file://"+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson").Execute()
			if err != nil {
				e2e.Logf("The mirror2disk for additionalImages failed, retrying...")
				return false, nil
			}
			return true, nil

		})
		exutil.AssertWaitPollNoErr(waitErr, "Max time reached but mirror2disk for additionalImages still failed")

		exutil.By("Start mirroring to registry")
		waitErr = wait.Poll(300*time.Second, 900*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "--from", "file://"+dirname, "docker://"+serInfo.serviceName, "--v2", "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Execute()
			if err != nil {
				e2e.Logf("The disk2mirror of additionalImages failed, retrying...")
				return false, nil
			}
			return true, nil

		})
		exutil.AssertWaitPollNoErr(waitErr, "Max time reached but disk2mirror for additionalImages still failed")

		exutil.By("Generete delete image file")
		_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("delete", "--config", imageDeleteYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--workspace", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--generate").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Execute delete with force-cache-delete")
		_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("delete", "--delete-yaml-file", dirname+"/working-dir/delete/delete-images.yaml", "docker://"+serInfo.serviceName, "--v2", "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false", "--force-cache-delete=true").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	g.It("Author:yinzhou-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Medium-75117-support to set max-parallel-downloads for v2 [Serial]", func() {
		dirname := "/tmp/case75117"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", "--to="+dirname, "--confirm").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-75117-1.yaml")
		imageSetYamlFileS := filepath.Join(ocmirrorBaseDir, "config-75117-2.yaml")

		exutil.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}
		exutil.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)

		skopeExecute(fmt.Sprintf("skopeo copy --all docker://registry.redhat.io/redhat/redhat-operator-index:v4.16 oci://%s --remove-signatures --insecure-policy --authfile %s", dirname+"/redhat-operator-index", dirname+"/.dockerconfigjson"))

		exutil.By("Start m2d with max-parallel-downloads")
		waitErr := wait.Poll(300*time.Second, 900*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "file://"+dirname, "--v2", "--max-parallel-downloads=50", "--authfile", dirname+"/.dockerconfigjson").Execute()
			if err != nil {
				e2e.Logf("The mirror2disk failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "Max time reached but mirror2disk still failed")

		exutil.By("Start d2m with max-parallel-downloads")
		waitErr = wait.Poll(300*time.Second, 900*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "--from", "file://"+dirname, "docker://"+serInfo.serviceName+"/maxparallel", "--v2", "--max-parallel-downloads=100", "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Execute()
			if err != nil {
				e2e.Logf("The disk2mirror failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "Max time reached but disk2mirror still failed")

		exutil.By("Start mirror2mirror")
		defer os.RemoveAll(".oc-mirror.log")
		waitErr = wait.PollImmediate(300*time.Second, 900*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileS, "docker://"+serInfo.serviceName, "--v2", "--workspace", "file://"+dirname, "--max-parallel-downloads=150", "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Execute()
			if err != nil {
				e2e.Logf("The mirror2mirror failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2mirror still failed")

		exutil.By("Negative test for max-parallel-downloads")
		_, outErr, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileS, "docker://"+serInfo.serviceName, "--v2", "--workspace", "file://"+dirname, "--max-parallel-downloads=abdedf", "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Outputs()
		o.Expect(err).Should(o.HaveOccurred())
		e2e.Logf("The out error is %v", outErr)
		o.Expect(strings.Contains(outErr, "invalid argument")).To(o.BeTrue())
	})

	g.It("Author:yinzhou-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-High-72983-support registries.conf for normal operator mirror of v2 [Serial]", func() {
		exutil.By("Set registry config")
		dirname := "/tmp/case72983"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", "--to="+dirname, "--confirm").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-72983.yaml")

		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}
		exutil.By("Trying to launch the first registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		setRegistryVolume(oc, "deploy", "registry", oc.Namespace(), "35G", "/var/lib/registry")

		exutil.By("Trying to launch the second registry app")
		defer registry.deleteregistrySpecifyName(oc, "secondregistry")
		secondSerInfo := registry.createregistrySpecifyName(oc, "secondregistry")
		setRegistryVolume(oc, "deploy", "secondregistry", oc.Namespace(), "35G", "/var/lib/registry")

		exutil.By("Mirror to first registry")
		waitErr := wait.Poll(30*time.Second, 900*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--workspace", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Execute()
			if err != nil {
				e2e.Logf("The mirror2mirror failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2mirror still failed")

		exutil.By("Create and set registries.conf")
		registryConfContent := getRegistryConfContentStr(serInfo.serviceName, "quay.io", "registry.redhat.io")
		homePath := getHomePath()
		homeRistryConfExist, _ := ensureContainersConfigDirectory(homePath)
		if !homeRistryConfExist {
			e2e.Failf("Failed to get or create the home registry config directory")
		}

		defer restoreRegistryConf(homePath)
		_, errStat := os.Stat(homePath + "/.config/containers/registries.conf")
		if errStat == nil {
			backupContainersConfig(homePath)
			setRegistryConf(registryConfContent, homePath)
		} else if os.IsNotExist(errStat) {
			setRegistryConf(registryConfContent, homePath)
		} else {
			e2e.Failf("Unexpected error %v", errStat)
		}

		exutil.By("Mirror to second registry")
		waitErr = wait.Poll(30*time.Second, 900*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+secondSerInfo.serviceName, "--v2", "--workspace", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Execute()
			if err != nil {
				e2e.Logf("The mirror2mirror failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2mirror still failed")
	})

	g.It("Author:yinzhou-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-High-72982-support registries.conf for OCI of v2 [Serial]", func() {
		exutil.By("Set registry config")
		dirname := "/tmp/case72982"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", "--to="+dirname, "--confirm").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-72982.yaml")

		exutil.By("Use skopoe copy catalogsource to localhost")
		skopeExecute(fmt.Sprintf("skopeo copy --all docker://registry.redhat.io/redhat/redhat-operator-index:v4.15 --remove-signatures  --insecure-policy oci://%s --authfile %s", dirname+"/redhat-operator-index", dirname+"/.dockerconfigjson"))

		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}
		exutil.By("Trying to launch the first registry app")
		serInfo := registry.createregistry(oc)
		setRegistryVolume(oc, "deploy", "registry", oc.Namespace(), "35G", "/var/lib/registry")

		exutil.By("Trying to launch the second registry app")
		secondSerInfo := registry.createregistrySpecifyName(oc, "secondregistry")
		setRegistryVolume(oc, "deploy", "secondregistry", oc.Namespace(), "35G", "/var/lib/registry")

		exutil.By("Mirror to first registry")
		waitErr := wait.Poll(30*time.Second, 900*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--workspace", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Execute()
			if err != nil {
				e2e.Logf("The mirror2mirror failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2mirror still failed")

		exutil.By("Create and set registries.conf")
		/*registryConfFile := dirname + "/registries.conf"
		createRegistryConf(registryConfFile, serInfo.serviceName)
		defer restoreRegistryConf()
		setRegistryConf(registryConfFile)*/

		registryConfContent := getRegistryConfContentStr(serInfo.serviceName, "quay.io", "registry.redhat.io")
		homePath := getHomePath()
		homeRistryConfExist, _ := ensureContainersConfigDirectory(homePath)
		if !homeRistryConfExist {
			e2e.Failf("Failed to get or create the home registry config directory")
		}

		defer restoreRegistryConf(homePath)
		_, errStat := os.Stat(homePath + "/.config/containers/registries.conf")
		if errStat == nil {
			backupContainersConfig(homePath)
			setRegistryConf(registryConfContent, homePath)
		} else if os.IsNotExist(errStat) {
			setRegistryConf(registryConfContent, homePath)
		} else {
			e2e.Failf("Unexpected error %v", errStat)
		}

		exutil.By("Mirror to second registry")
		waitErr = wait.Poll(30*time.Second, 900*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+secondSerInfo.serviceName, "--v2", "--workspace", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Execute()
			if err != nil {
				e2e.Logf("The mirror2mirror failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2mirror still failed")
	})

	g.It("Author:knarra-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-High-75425-Validate oc-mirror is able to pull hypershift kubevirt coreos container image and mirror the same [Serial]", func() {
		exutil.By("Set registry config")
		dirname := "/tmp/case75425"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		exutil.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)
		setRegistryVolume(oc, "deploy", "registry", oc.Namespace(), "30G", "/var/lib/registry")

		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-75425.yaml")

		exutil.By("Start mirror2disk")
		defer os.RemoveAll(".oc-mirror.log")
		defer os.RemoveAll("~/.oc-mirror/")
		waitErr := wait.PollImmediate(30*time.Second, 900*time.Second, func() (bool, error) {
			kubeVirtContainerImageOutputm2d, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "file://"+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson").Output()
			if err != nil {
				e2e.Logf("Mirror2disk for kubevirt coreos container image failed, retrying...")
				return false, nil
			}
			if strings.Contains(kubeVirtContainerImageOutputm2d, "kubeVirtContainer set to true [ including : quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:f7f96da0be48b0010bcc45caec160409cbdbc50c15e3cf5f47abfa6203498c3b ]") {
				e2e.Logf("Mirror to disk for KubeVirt CoreOs Container image completed successfully")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2disk for kubevirt core os container failed")

		exutil.By("Start disk2mirror")
		defer os.RemoveAll(".oc-mirror.log")
		defer os.RemoveAll("~/.oc-mirror/")
		waitErr = wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			kubeVirtContainerImageOutputd2m, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--from", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
			if err != nil {
				e2e.Logf("Disk2mirror for kubeVirt coreos container image failed, retrying...")
				return false, nil
			}
			if strings.Contains(kubeVirtContainerImageOutputd2m, "kubeVirtContainer set to true [ including : quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:f7f96da0be48b0010bcc45caec160409cbdbc50c15e3cf5f47abfa6203498c3b ]") {
				e2e.Logf("Disk to mirror for KubeVirt CoreOs Container image completed successfully")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "max time reached but the disk2mirror for KubeVirt CoreOs Container image failed")

		exutil.By("Start mirror2mirror")
		defer os.RemoveAll(".oc-mirror.log")
		defer os.RemoveAll("~/.oc-mirror/")
		waitErr = wait.Poll(30*time.Second, 900*time.Second, func() (bool, error) {
			kubeVirtContainerImageOutputm2m, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName+"/m2m", "--workspace", "file://"+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
			if err != nil {
				e2e.Logf("The mirror2mirror for KubeVirt Coreos Container image failed, retrying...")
				return false, nil
			}
			if strings.Contains(kubeVirtContainerImageOutputm2m, "kubeVirtContainer set to true [ including : quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:f7f96da0be48b0010bcc45caec160409cbdbc50c15e3cf5f47abfa6203498c3b ]") {
				e2e.Logf("Mirror to mirror for KubeVirt CoreOs Container image completed successfully")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "Max time reached but mirror2mirror for kubevirt coreos container image still failed")

	})

	g.It("Author:knarra-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-High-75437-Validate oc-mirror does not error out when kubeVirtContainer is set to false in the ImageSetConfig yaml [Serial]", func() {
		exutil.By("Set registry config")
		dirname := "/tmp/case75437"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		exutil.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)
		setRegistryVolume(oc, "deploy", "registry", oc.Namespace(), "30G", "/var/lib/registry")

		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-75437.yaml")

		exutil.By("Start mirror2disk")
		defer os.RemoveAll(".oc-mirror.log")
		defer os.RemoveAll("~/.oc-mirror/")
		waitErr := wait.PollImmediate(30*time.Second, 900*time.Second, func() (bool, error) {
			kubeVirtContainerImageOutputm2d, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "file://"+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson").Output()
			if err != nil {
				e2e.Logf("Mirror2disk when kubevirtContainer set to false is still failing, retrying...")
				return false, nil
			}
			if !strings.Contains(kubeVirtContainerImageOutputm2d, "kubeVirtContainer set to true [ including : quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:f7f96da0be48b0010bcc45caec160409cbdbc50c15e3cf5f47abfa6203498c3b ]") {
				e2e.Logf("Mirror to disk completed successfully when kubeVirtContainer is set to false in the imageSetConfig.yaml file")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2disk still failed, when kubeVirtContainer is set to false")

		exutil.By("Start disk2mirror")
		defer os.RemoveAll(".oc-mirror.log")
		defer os.RemoveAll("~/.oc-mirror/")
		waitErr = wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			kubeVirtContainerImageOutputd2m, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--from", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
			if err != nil {
				e2e.Logf("Disk2mirror when kubeVirtContainer set to false is still failing, retrying...")
				return false, nil
			}
			if !strings.Contains(kubeVirtContainerImageOutputd2m, "kubeVirtContainer set to true [ including : quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:f7f96da0be48b0010bcc45caec160409cbdbc50c15e3cf5f47abfa6203498c3b ]") {
				e2e.Logf("Disk to mirror when kubeVirtContainer set to false has been completed successfully")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "max time reached but the disk2mirror still failed, when kubeVirtContainer is set to false")

		exutil.By("Start mirror2mirror")
		defer os.RemoveAll(".oc-mirror.log")
		defer os.RemoveAll("~/.oc-mirror/")
		waitErr = wait.Poll(30*time.Second, 900*time.Second, func() (bool, error) {
			kubeVirtContainerImageOutputm2m, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName+"/m2m", "--workspace", "file://"+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
			if err != nil {
				e2e.Logf("The mirror2mirror when kubeVirtContainer set to false still failed, retrying...")
				return false, nil
			}
			if !strings.Contains(kubeVirtContainerImageOutputm2m, "kubeVirtContainer set to true [ including : quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:f7f96da0be48b0010bcc45caec160409cbdbc50c15e3cf5f47abfa6203498c3b ]") {
				e2e.Logf("Mirror to mirror when kubeVirtContainer set to false has been completed successfully")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "Max time reached but mirror2mirror still failed, when kubevirtContainer is set to false")

	})

	g.It("Author:knarra-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-High-75438-Validate oc-mirror does not error out when kubeVirtContainer is set to true for a release that does not contain this image [Serial]", func() {
		exutil.By("Set registry config")
		dirname := "/tmp/case75438"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		exutil.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)
		setRegistryVolume(oc, "deploy", "registry", oc.Namespace(), "30G", "/var/lib/registry")

		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-75438.yaml")

		exutil.By("Start mirror2disk")
		defer os.RemoveAll(".oc-mirror.log")
		defer os.RemoveAll("~/.oc-mirror/")
		waitErr := wait.PollImmediate(30*time.Second, 900*time.Second, func() (bool, error) {
			kubeVirtContainerImageOutputm2d, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "file://"+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson").Output()
			if err != nil {
				e2e.Logf("Mirror2disk when kubeVirtContainer set to true for a release that does not have this image failing, retrying...")
				return false, nil
			}
			if strings.Contains(kubeVirtContainerImageOutputm2d, "could not find kubevirt image") {
				e2e.Logf("Mirror to disk completed successfully when kubeVirtContainer set to true for a release that does not have this image")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "Max time reached but the mirror2disk still failed, when kubeVirtContainer set to true for a release that does not have this image")

		exutil.By("Start disk2mirror")
		defer os.RemoveAll(".oc-mirror.log")
		defer os.RemoveAll("~/.oc-mirror/")
		waitErr = wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			kubeVirtContainerImageOutputd2m, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--from", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
			if err != nil {
				e2e.Logf("Disk2mirror when kubeVirtContainer set to true for a release that does not have this image is still failing, retrying...")
				return false, nil
			}
			if strings.Contains(kubeVirtContainerImageOutputd2m, "could not find kubevirt image") {
				e2e.Logf("Disk to mirror when kubeVirtContainer set to true for a release that does not have this image completed successfully")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "Max time reached but the disk2mirror still failed, when kubeVirtContainer set to true for a release that does not have this image")

		exutil.By("Start mirror2mirror")
		defer os.RemoveAll(".oc-mirror.log")
		defer os.RemoveAll("~/.oc-mirror/")
		waitErr = wait.Poll(30*time.Second, 900*time.Second, func() (bool, error) {
			kubeVirtContainerImageOutputm2m, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName+"/m2m", "--workspace", "file://"+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
			if err != nil {
				e2e.Logf("The mirror2mirror when kubeVirtContainer set to true that does not contain this image still failed, retrying...")
				return false, nil
			}
			if strings.Contains(kubeVirtContainerImageOutputm2m, "could not find kubevirt image") {
				e2e.Logf("Mirror to mirror when kubeVirtContainer set to true for a release that does not have this image has been completed successfully")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "Max time reached but mirror2mirror still failed, when kubevirtContainer set to true for a release that does not have this image")

	})

	g.It("Author:yinzhou-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Medium-75366-show operator on error logs and skip bundle when related image associated failed [Serial]", func() {
		exutil.By("Set registry config")
		dirname := "/tmp/case75366"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", "--to="+dirname, "--confirm").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-75366.yaml")

		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}
		exutil.By("Trying to launch the first registry app")
		serInfo := registry.createregistry(oc)
		setRegistryVolume(oc, "deploy", "registry", oc.Namespace(), "15G", "/var/lib/registry")

		exutil.By("Start m2d")
		waitErr := wait.Poll(60*time.Second, 300*time.Second, func() (bool, error) {
			mirrorToDiskOutput, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "file://"+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson").Output()
			if err != nil {
				e2e.Logf("The mirror2disk failed, retrying...")
				return false, nil
			}
			if !strings.Contains(mirrorToDiskOutput, "Operator bundles: [nginxmirror6.v1.1.0]") && !strings.Contains(mirrorToDiskOutput, "Operators: [nginxmirror6]") {
				e2e.Logf("Can't find the bundles and operators information")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "Max time reached but mirror2disk still failed")

		exutil.By("Start d2m")
		waitErr = wait.Poll(60*time.Second, 300*time.Second, func() (bool, error) {
			diskToMirrorOutput, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "--from", "file://"+dirname, "docker://"+serInfo.serviceName, "--v2", "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
			if err != nil {
				e2e.Logf("The disk2mirror failed, retrying...")
				return false, nil
			}
			if !strings.Contains(diskToMirrorOutput, "Operator bundles: [nginxmirror6.v1.1.0]") && !strings.Contains(diskToMirrorOutput, "Operators: [nginxmirror6]") {
				e2e.Logf("Can't find the bundles and operators information")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "Max time reached but disk2mirror still failed")

		exutil.By("Start mirror2mirror")
		defer os.RemoveAll(".oc-mirror.log")
		waitErr = wait.PollImmediate(60*time.Second, 300*time.Second, func() (bool, error) {
			mirrorToMirrorOutput, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--workspace", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
			if err != nil {
				e2e.Logf("The mirror2mirror failed, retrying...")
				return false, nil
			}
			if !strings.Contains(mirrorToMirrorOutput, "Operator bundles: [nginxmirror6.v1.1.0]") && !strings.Contains(mirrorToMirrorOutput, "Operators: [nginxmirror6]") {
				e2e.Logf("Can't find the bundles and operators information")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2mirror still failed")
	})

	g.It("Author:yinzhou-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Low-72920-support head-only for catalog [Serial]", func() {
		exutil.By("Set registry config")
		dirname := "/tmp/case72920"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", "--to="+dirname, "--confirm").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-72920.yaml")

		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}
		exutil.By("Trying to launch the first registry app")
		serInfo := registry.createregistry(oc)
		setRegistryVolume(oc, "deploy", "registry", oc.Namespace(), "15G", "/var/lib/registry")

		exutil.By("Start m2d")
		waitErr := wait.Poll(60*time.Second, 300*time.Second, func() (bool, error) {
			m2dOutput, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "file://"+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson").Output()
			if err != nil {
				e2e.Logf("The mirror2disk failed, retrying...")
				return false, nil
			}
			if !strings.Contains(m2dOutput, "images to copy 10") {
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "Max time reached but mirror2disk still failed")

		exutil.By("Start d2m")
		waitErr = wait.Poll(60*time.Second, 300*time.Second, func() (bool, error) {
			d2mOutput, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "--from", "file://"+dirname, "docker://"+serInfo.serviceName, "--v2", "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
			if err != nil {
				e2e.Logf("The disk2mirror failed, retrying...")
				return false, nil
			}
			if !strings.Contains(d2mOutput, "images to copy 10") {
				e2e.Logf("Failed to find the image num")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "Max time reached but disk2mirror still failed")

		exutil.By("Start m2m")
		waitErr = wait.Poll(60*time.Second, 300*time.Second, func() (bool, error) {
			m2mOutput, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "--workspace", "file://"+dirname+"/m2m", "docker://"+serInfo.serviceName+"/m2m", "--v2", "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
			if err != nil {
				e2e.Logf("The mirro2mirror failed, retrying...")
				return false, nil
			}
			if !strings.Contains(m2mOutput, "images to copy 11") {
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "Max time reached but mirror2mirror still failed")
	})

	g.It("Author:knarra-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-High-75422-Verify Skip deletion of operator catalog image in delete feature [Serial]", func() {
		exutil.By("Set registry config")
		dirname := "/tmp/case75422"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		exutil.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)
		setRegistryVolume(oc, "deploy", "registry", oc.Namespace(), "30G", "/var/lib/registry")

		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-75422.yaml")
		imageDeleteYamlFileF := filepath.Join(ocmirrorBaseDir, "config-75422-delete.yaml")

		exutil.By("Skopeo oci to localhost")
		command := fmt.Sprintf("skopeo copy --all --format v2s2 docker://icr.io/cpopen/ibm-bts-operator-catalog@sha256:866f0212eab7bc70cc7fcf7ebdbb4dfac561991f6d25900bd52f33cd90846adf oci://%s  --remove-signatures --insecure-policy", dirname+"/ibm-catalog")
		waitErr := wait.Poll(30*time.Second, 180*time.Second, func() (bool, error) {
			_, err := exec.Command("bash", "-c", command).Output()
			if err != nil {
				e2e.Logf("copy failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("max time reached but the skopeo copy still failed"))

		exutil.By("Start mirror2disk")
		defer os.RemoveAll("~/.oc-mirror/")
		defer os.RemoveAll("~/.oc-mirror.log")
		waitErr = wait.PollImmediate(30*time.Second, 900*time.Second, func() (bool, error) {
			_, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "file://"+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson").Output()
			if err != nil {
				e2e.Logf("The mirror2disk for skip deletion of operator catalog image in delete feature failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2disk for skip deletion of operator catalog image in delete feature failed, retrying...")

		exutil.By("Start disk2mirror")
		defer os.RemoveAll(".oc-mirror.log")
		waitErr = wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--from", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Execute()
			if err != nil {
				e2e.Logf("The disk2mirror for skip deletion of operator catalog image in delete feature failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "max time reached but the disk2mirror for skip deletion of operator catalog image in delete feature failed")

		exutil.By("Generate delete image file")
		_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("delete", "--config", imageDeleteYamlFileF, "--generate", "--workspace", "file://"+dirname, "docker://"+serInfo.serviceName, "--v2", "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false", "--src-tls-verify=false").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Validate delete-images yaml file does not contain any thing with respect to catalog index image")
		deleteImagesYamlOutput, err := exec.Command("bash", "-c", fmt.Sprintf("cat %s", dirname+"/working-dir/delete/delete-images.yaml")).Output()
		if err != nil {
			e2e.Failf("Error is %v", err)
		}
		e2e.Logf("deleteImagesYamlOutput is %s", deleteImagesYamlOutput)

		catalogIndexDetails := []string{"registry.redhat.io/redhat/redhat-operator-index:v4.15", "registry.redhat.io/redhat/certified-operator-index:v4.15", "registry.redhat.io/redhat/community-operator-index:v4.15", "ibm-catalog"}
		for _, catalogIndex := range catalogIndexDetails {
			o.Expect(deleteImagesYamlOutput).ShouldNot(o.ContainSubstring(catalogIndex), "UnExpected Catalog Index Found")
		}

	})

	g.It("Author:knarra-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Medium-73791-verify blockedImages feature for v2 [Serial]", func() {
		exutil.By("Set registry config")
		dirname := "/tmp/case73791"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())
		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-73791.yaml")

		exutil.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}
		exutil.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)
		setRegistryVolume(oc, "deploy", "registry", oc.Namespace(), "20G", "/var/lib/registry")

		exutil.By("Verify blockedImages feature for mirror2disk")
		waitErr := wait.Poll(30*time.Second, 900*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "file://"+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson").Execute()
			if err != nil {
				e2e.Logf("The mirror2disk failed, retrying...")
				return false, nil
			}
			return true, nil

		})
		exutil.AssertWaitPollNoErr(waitErr, "Max time reached but mirror2disk still failed")

		exutil.By("Verify blockedImages feature for disk2mirror")
		waitErr = wait.Poll(30*time.Second, 900*time.Second, func() (bool, error) {
			err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "--from", "file://"+dirname, "docker://"+serInfo.serviceName+"/d2m", "--v2", "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Execute()
			if err != nil {
				e2e.Logf("The disk2mirror failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "Max time reached but disk2mirror still failed")

		exutil.By("Verify blockedImages feature for mirror2mirror")
		waitErr = wait.Poll(30*time.Second, 900*time.Second, func() (bool, error) {
			err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "--workspace", "file://"+dirname, "docker://"+serInfo.serviceName+"/m2m", "--v2", "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Execute()
			if err != nil {
				e2e.Logf("The mirror2mirror failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "Max time reached but mirror2mirror still failed")
	})

	// author: knarra@redhat.com
	g.It("Author:knarra-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-High-74662-Verify oc-mirror does not crash when invalid catalogs are found in imageSetConfig file [Serial]", func() {
		exutil.By("Set registry config")
		dirname := "/tmp/case74662"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())
		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-74662.yaml")

		exutil.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}
		exutil.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)

		exutil.By("Start mirror2disk with invalid catalogs")
		defer os.RemoveAll(".oc-mirror.log")
		defer os.RemoveAll("~/.oc-mirror/")
		exutil.By("Start dry run of mirror2disk")
		waitErr := wait.Poll(30*time.Second, 900*time.Second, func() (bool, error) {
			mirrorToDiskOutput, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "file://"+dirname, "--dry-run", "--v2", "--authfile", dirname+"/.dockerconfigjson").Output()
			if err != nil {
				e2e.Logf("The mirror2disk failed, retrying...")
				return false, nil
			}
			if strings.Contains(mirrorToDiskOutput, "dry-run/missing.txt") && strings.Contains(mirrorToDiskOutput, "dry-run/mapping.txt") {
				e2e.Logf("Mirror to Disk dry run for invalid catalog has been completed successfully")
				return true, nil
			}
			return false, nil

		})
		exutil.AssertWaitPollNoErr(waitErr, "Max time reached but mirror2disk still failed for invalid catalog")

		exutil.By("Start dry run of mirror2mirror")
		waitErr = wait.Poll(30*time.Second, 900*time.Second, func() (bool, error) {
			mirrorToMirrorOutput, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName+"/m2minvalid", "--workspace", "file://"+dirname, "--v2", "--dry-run", "--authfile", dirname+"/.dockerconfigjson").Output()
			if err != nil {
				e2e.Logf("The mirror2mirror failed, retrying...")
				return false, nil
			}
			if strings.Contains(mirrorToMirrorOutput, "dry-run/mapping.txt") {
				e2e.Logf("Mirror to mirror dry run for invalid catalog has been completed successfully")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "Max time reached but mirror2mirror still failed for invalid catalog")
	})

	g.It("Author:knarra-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Critical-73419-Verify filtering and mirroring specific bundle versions for operators works fine with oc-mirror v2 [Serial]", func() {
		dirname := "/tmp/case73419"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", "--to="+dirname, "--confirm").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-73419.yaml")

		err = getRouteCAToFile(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}
		exutil.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)
		setRegistryVolume(oc, "deploy", "registry", oc.Namespace(), "20G", "/var/lib/registry")
		exutil.By("Configure the Registry Certificate as trusted for cincinnati")
		addCA, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("image.config.openshift.io/cluster", "-o=jsonpath={.spec.additionalTrustedCA}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer restoreAddCA(oc, addCA, "trusted-ca-73419")
		err = trustCert(oc, serInfo.serviceName, dirname+"/tls.crt", "trusted-ca-73419")
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Verify filtering and mirroring specific bundle versions for operators via mirror2disk")
		waitErr := wait.Poll(30*time.Second, 900*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "file://"+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson").Execute()
			if err != nil {
				e2e.Logf("The mirror2disk failed, retrying...")
				return false, nil
			}
			return true, nil

		})
		exutil.AssertWaitPollNoErr(waitErr, "Max time reached but mirror2disk still failed")

		exutil.By("Verify filtering and mirroring specific bundle versions for operators via disk2mirror")
		waitErr = wait.Poll(30*time.Second, 900*time.Second, func() (bool, error) {
			err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "--from", "file://"+dirname, "docker://"+serInfo.serviceName+"/d2m", "--v2", "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Execute()
			if err != nil {
				e2e.Logf("The disk2mirror failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "Max time reached but disk2mirror still failed")

		exutil.By("Create the catalogsource, idms and itms")
		defer operateCSAndMs(oc, dirname+"/working-dir/cluster-resources", "delete")
		operateCSAndMs(oc, dirname+"/working-dir/cluster-resources", "create")
		assertPodOutput(oc, "olm.catalogSource=cs-redhat-operator-index-v4-14", "openshift-marketplace", "Running")
		assertPodOutput(oc, "olm.catalogSource=cs-redhat-operator-index-v4-16", "openshift-marketplace", "Running")

		exutil.By("Installing operators from 4.16 catalog")
		rhkdoSub, rhkdoOG := getOperatorInfo(oc, "cluster-kube-descheduler-operator", "openshift-kube-descheduler-operator", "registry.redhat.io/redhat/redhat-operator-index:v4.16", "cs-redhat-operator-index-v4-16")
		defer removeOperatorFromCustomCS(oc, rhkdoSub, rhkdoOG, "openshift-kube-descheduler-operator")
		installOperatorFromCustomCS(oc, rhkdoSub, rhkdoOG, "openshift-kube-descheduler-operator", "cluster-kube-descheduler-operator")

		exutil.By("Installing operators from redhat-operator-index 4.14 catalog")
		buildPruningBaseDir := exutil.FixturePath("testdata", "workloads")
		awsRedhatcatalogSubscription := filepath.Join(buildPruningBaseDir, "awsredhatcustomsub73419.yaml")
		awsRedhatcatalogOperator := filepath.Join(buildPruningBaseDir, "awsredhatcustomog73419.yaml")

		exutil.By("Create the aws-load-balancer-operator namespace")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("ns", "aws-load-balancer-operator").Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", "aws-load-balancer-operator").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create operatorgroup for aws load balancer operator")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", awsRedhatcatalogOperator).Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", awsRedhatcatalogOperator).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create subscription for aws load balancer operator")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", awsRedhatcatalogSubscription).Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", awsRedhatcatalogSubscription).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		e2e.Logf("Wait for the operator pod running")
		if ok := waitForAvailableRsRunning(oc, "deploy", "aws-load-balancer-operator-controller-manager", "aws-load-balancer-operator", "1"); ok {
			e2e.Logf("AWS Load Balancer from redhat catalog index 4.14 has been depolyed successfully\n")
		} else {
			e2e.Failf("All pods related to aws load balancer operator from redhat catalog index 4.14 is not running")
		}

	})

	g.It("Author:knarra-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-High-76469-Verify Creating release signature configmap with oc-mirror v2 [Serial]", func() {
		exutil.By("Set registry config")
		dirname := "/tmp/case76469"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		exutil.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)
		setRegistryVolume(oc, "deploy", "registry", oc.Namespace(), "30G", "/var/lib/registry")

		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-76469.yaml")

		exutil.By("Start mirror2disk")
		defer os.RemoveAll("~/.oc-mirror/")
		defer os.RemoveAll("~/.oc-mirror.log")
		waitErr := wait.PollImmediate(30*time.Second, 900*time.Second, func() (bool, error) {
			_, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "file://"+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson").Output()
			if err != nil {
				e2e.Logf("The mirror2disk for creating release signature configmap with oc-mirror v2 failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2disk for creating release signature configmap with oc-mirror v2 failed, retrying...")

		exutil.By("Start disk2mirror")
		defer os.RemoveAll(".oc-mirror.log")
		waitErr = wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--from", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Execute()
			if err != nil {
				e2e.Logf("The disk2mirror for creating release signature configmap with oc-mirror v2 failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "max time reached but the disk2mirror for creating release signature configmap with oc-mirror v2 failed")

		// Validate if the content from the signature configmap in cluster-resources directory and signature directory matches
		validateConfigmapAndSignatureContent(oc, dirname, "4.16.0")

		exutil.By("Start mirror2mirror")
		defer os.RemoveAll(".oc-mirror.log")
		dirnameM2M := "/tmp/case76469m2m"
		defer os.RemoveAll(dirnameM2M)
		err = os.MkdirAll(dirnameM2M, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		waitErr = wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--workspace", "file://"+dirnameM2M, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Execute()
			if err != nil {
				e2e.Logf("The mirror2mirror for creating release signature configmap with oc-mirror v2 failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "max time reached but the disk2mirror for creating release signature configmap with oc-mirror v2 still failed")

		// Validate if the content from the signature configmap in cluster-resources directory and signature directory matches
		validateConfigmapAndSignatureContent(oc, dirnameM2M, "4.16.0")

	})

	g.It("Author:knarra-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Medium-76596-oc-mirror should not GenerateSignatureConfigMap when not mirror the release images [Serial]", func() {
		exutil.By("Set registry config")
		dirname := "/tmp/case76596"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		exutil.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)
		setRegistryVolume(oc, "deploy", "registry", oc.Namespace(), "30G", "/var/lib/registry")

		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-76596.yaml")

		exutil.By("Start mirror2disk")
		defer os.RemoveAll("~/.oc-mirror/")
		defer os.RemoveAll("~/.oc-mirror.log")
		waitErr := wait.PollImmediate(30*time.Second, 900*time.Second, func() (bool, error) {
			_, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "file://"+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson").Output()
			if err != nil {
				e2e.Logf("The mirror2disk for should not generate signature configmap when not mirror the release images  with oc-mirror v2 failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2disk for should not generate signature configmap when not mirror the release images  with oc-mirror v2 failed, retrying...")

		exutil.By("Start disk2mirror")
		defer os.RemoveAll(".oc-mirror.log")
		waitErr = wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			d2mOutput, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--from", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
			if err != nil {
				e2e.Logf("The disk2mirror for should not generate signature configmap when not mirror the release images with oc-mirror v2 failed, retrying...")
				return false, nil
			}
			if strings.Contains(d2mOutput, "signature files not found, could not generate signature configmap") {
				e2e.Failf("Signature Configmaps are being generated when nothing related to platform is set in the isc which is not expected")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "max time reached but the disk2mirror for should not generate signature configmap when not mirror the release images with oc-mirror v2 failed")

		exutil.By("Start mirror2mirror")
		defer os.RemoveAll(".oc-mirror.log")
		dirnameM2M := "/tmp/case76469m2m"
		defer os.RemoveAll(dirnameM2M)
		err = os.MkdirAll(dirnameM2M, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		waitErr = wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			m2mOutput, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--workspace", "file://"+dirnameM2M, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
			if err != nil {
				e2e.Logf("The mirror2mirror for should not generate signature configmap when not mirror the release images with oc-mirror v2 failed, retrying...")
				return false, nil
			}
			if strings.Contains(m2mOutput, "signature files not found, could not generate signature configmap") {
				e2e.Failf("ignature Configmaps are being generated when nothing related to platform is set in the isc which is not expected")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "max time reached but mirror2mirror for should not generate signature configmap when not mirror the release images with oc-mirror v2 failed")
	})

	g.It("Author:knarra-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Medium-76489-oc-mirror should fail when the cincinnati API has errors [Serial]", func() {
		exutil.By("Set registry config")
		dirname := "/tmp/case76489"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		exutil.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)
		setRegistryVolume(oc, "deploy", "registry", oc.Namespace(), "30G", "/var/lib/registry")

		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-76489.yaml")
		imageSetYamlFileS := filepath.Join(ocmirrorBaseDir, "config-76596.yaml")

		// Set UPDATE_URL_OVERRIDE to a site that does not work
		exutil.By("Set UPDATE_URL_OVERRIDE to a site that does not work")
		defer os.Unsetenv("UPDATE_URL_OVERRIDE")
		err = os.Setenv("UPDATE_URL_OVERRIDE", "https://a-site-that-does-not-work")
		if err != nil {
			e2e.Failf("Error setting environment variable:", err)
		}

		// Verify that the environment variable is set
		e2e.Logf("UPDATE_URL_OVERRIDE:", os.Getenv("UPDATE_URL_OVERRIDE"))

		exutil.By("Start mirror2disk")
		defer os.RemoveAll("~/.oc-mirror/")
		defer os.RemoveAll("~/.oc-mirror.log")
		m2dOutput, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "file://"+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson").Output()
		o.Expect(err).To(o.HaveOccurred())
		if matched, _ := regexp.Match("ERROR"+".*"+"RemoteFailed"+".*"+"lookup a-site-that-does-not-work"+".*"+"no such host", []byte(m2dOutput)); !matched {
			e2e.Failf("Do not see the expected output while doing mirror2disk\n")
		}

		exutil.By("Start mirror2mirror")
		defer os.RemoveAll(".oc-mirror.log")
		m2mOutput, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--workspace", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
		o.Expect(err).To(o.HaveOccurred())
		if matched, _ := regexp.Match("ERROR"+".*"+"RemoteFailed"+".*"+"lookup a-site-that-does-not-work"+".*"+"no such host", []byte(m2mOutput)); !matched {
			e2e.Failf("Do not see the expected output while doing mirror2disk\n")
		}

		// Unset the update_url_override
		err = os.Unsetenv("UPDATE_URL_OVERRIDE")
		if err != nil {
			e2e.Failf("Error unsetting environment variable:", err)
		}

		// Verify that the environment variable is unset
		e2e.Logf("UPDATE_URL_OVERRIDE:", os.Getenv("UPDATE_URL_OVERRIDE"))

		exutil.By("Start mirror2mirror")
		waitErr := wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			_, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileS, "docker://"+serInfo.serviceName, "--v2", "--workspace", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
			if err != nil {
				e2e.Logf("The mirror2mirror after unsetting the UPDATE_URL_OVERRIDE for oc-mirror v2 failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "max time reached but mirror2mirror after unsetting the UPDATE_URL_OVERRIDE for oc-mirror v2 failed")
	})

	g.It("Author:knarra-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Medium-76597-oc-mirror throws error when performing delete operation with --generate [Serial]", func() {
		exutil.By("Set registry config")
		dirname := "/tmp/case76597"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		exutil.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)
		setRegistryVolume(oc, "deploy", "registry", oc.Namespace(), "30G", "/var/lib/registry")

		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-76597.yaml")
		imageDeleteYamlFileF := filepath.Join(ocmirrorBaseDir, "config-76597-delete.yaml")

		exutil.By("Start mirror2disk")
		defer os.RemoveAll("~/.oc-mirror/")
		defer os.RemoveAll("~/.oc-mirror.log")
		waitErr := wait.PollImmediate(30*time.Second, 900*time.Second, func() (bool, error) {
			_, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "file://"+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson").Output()
			if err != nil {
				e2e.Logf("The mirror2disk for performing delete operatiorn with --generate failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2disk but performing delete operation with --generate failed, retrying...")

		exutil.By("Start disk2mirror")
		defer os.RemoveAll(".oc-mirror.log")
		waitErr = wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--from", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Execute()
			if err != nil {
				e2e.Logf("The disk2mirror for performing delete operation with --generate failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "max time reached but the disk2mirror for performing delete operation with --generate failed")

		exutil.By("Generate delete image file")
		dirnameDelete := "/tmp/case76597delete"
		defer os.RemoveAll(dirnameDelete)
		err = os.MkdirAll(dirnameDelete, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("delete", "--config", imageDeleteYamlFileF, "--generate", "--workspace", "file://"+dirnameDelete, "docker://"+serInfo.serviceName, "--v2", "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

	})

	g.It("Author:knarra-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Medium-77060-support to mirror helm for oc-mirror v2 [Serial]", func() {
		exutil.By("Set registry config")
		dirname := "/tmp/case77060"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		exutil.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)
		setRegistryVolume(oc, "deploy", "registry", oc.Namespace(), "30G", "/var/lib/registry")

		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-77060.yaml")

		exutil.By("Start mirror2disk")
		defer os.RemoveAll("~/.oc-mirror/")
		defer os.RemoveAll("~/.oc-mirror.log")
		waitErr := wait.PollImmediate(30*time.Second, 900*time.Second, func() (bool, error) {
			_, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "file://"+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson").Output()
			if err != nil {
				e2e.Logf("The mirror2disk for mirroring helm chars with oc-mirror v2 failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2disk for mirroring helm charts with oc-mirror v2 failed, retrying...")

		exutil.By("Start disk2mirror")
		defer os.RemoveAll(".oc-mirror.log")
		waitErr = wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			disk2mirrorOutput, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--from", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
			if err != nil {
				e2e.Logf("The disk2mirror for mirroring helm charts with oc-mirror v2 failed, retrying...")
				return false, nil
			}
			if strings.Contains(disk2mirrorOutput, "idms-oc-mirror.yaml") && strings.Contains(disk2mirrorOutput, "itms-oc-mirror.yaml") {
				e2e.Logf("Helm chart mirroring via disk2mirror completed successfully")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "max time reached but the disk2mirror for mirroring helm charts with oc-mirror v2 failed")

		exutil.By("Start mirror2mirror")
		defer os.RemoveAll(".oc-mirror.log")
		dirnameM2M := "/tmp/case77060m2m"
		defer os.RemoveAll(dirnameM2M)
		err = os.MkdirAll(dirnameM2M, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		waitErr = wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			mirror2mirrorOutput, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--workspace", "file://"+dirnameM2M, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
			if err != nil {
				e2e.Logf("The mirror2mirror for helm chart mirroring via oc-mirror v2 failed, retrying...")
				return false, nil
			}
			if strings.Contains(mirror2mirrorOutput, "idms-oc-mirror.yaml") && strings.Contains(mirror2mirrorOutput, "itms-oc-mirror.yaml") {
				e2e.Logf("Helm chart mirroring via mirror2mirror completed successfully")
				return true, nil
			}
			return false, nil

		})
		exutil.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2mirror for mirroring helm charts with oc-mirror v2 still failed")
	})

	g.It("Author:knarra-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Medium-77061-support the delete helm for v2 [Serial]", func() {
		exutil.By("Set registry config")
		dirname := "/tmp/case77061"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		exutil.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)
		setRegistryVolume(oc, "deploy", "registry", oc.Namespace(), "30G", "/var/lib/registry")

		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-77060.yaml")
		imageDeleteYamlFileF := filepath.Join(ocmirrorBaseDir, "delete-config-77061.yaml")

		exutil.By("Start mirror2disk")
		defer os.RemoveAll("~/.oc-mirror/")
		defer os.RemoveAll("~/.oc-mirror.log")
		waitErr := wait.PollImmediate(30*time.Second, 900*time.Second, func() (bool, error) {
			_, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "file://"+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson").Output()
			if err != nil {
				e2e.Logf("The mirror2disk for mirroring helm chars with oc-mirror v2 failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2disk for mirroring helm charts with oc-mirror v2 failed, retrying...")

		exutil.By("Start disk2mirror")
		defer os.RemoveAll(".oc-mirror.log")
		waitErr = wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			disk2mirrorOutput, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--from", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
			if err != nil {
				e2e.Logf("The disk2mirror for mirroring helm charts with oc-mirror v2 failed, retrying...")
				return false, nil
			}
			if strings.Contains(disk2mirrorOutput, "idms-oc-mirror.yaml") && strings.Contains(disk2mirrorOutput, "itms-oc-mirror.yaml") {
				e2e.Logf("Helm chart mirroring via disk2mirror completed successfully")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "max time reached but the disk2mirror for mirroring helm charts with oc-mirror v2 failed")

		exutil.By("Generete delete image file")
		_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("delete", "--config", imageDeleteYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--workspace", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--generate").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Execute delete with out force-cache-delete")
		_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("delete", "--delete-yaml-file", dirname+"/working-dir/delete/delete-images.yaml", "docker://"+serInfo.serviceName, "--v2", "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Start mirror2mirror")
		defer os.RemoveAll(".oc-mirror.log")
		dirnameM2M := "/tmp/case77061m2m"
		defer os.RemoveAll(dirnameM2M)
		err = os.MkdirAll(dirnameM2M, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		waitErr = wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			mirror2mirrorOutput, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--workspace", "file://"+dirnameM2M, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
			if err != nil {
				e2e.Logf("The mirror2mirror for helm chart mirroring via oc-mirror v2 failed, retrying...")
				return false, nil
			}
			if strings.Contains(mirror2mirrorOutput, "idms-oc-mirror.yaml") && strings.Contains(mirror2mirrorOutput, "itms-oc-mirror.yaml") {
				e2e.Logf("Helm chart mirroring via mirror2mirror completed successfully")
				return true, nil
			}
			return false, nil

		})
		exutil.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2mirror for mirroring helm charts with oc-mirror v2 still failed")

		exutil.By("Generete delete image file")
		_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("delete", "--config", imageDeleteYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--workspace", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--generate").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Execute delete with out force-cache-delete")
		_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("delete", "--delete-yaml-file", dirname+"/working-dir/delete/delete-images.yaml", "docker://"+serInfo.serviceName, "--v2", "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	g.It("Author:knarra-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Medium-77693-support the delete helm for v2 with --force-cache-delete=true [Serial]", func() {
		exutil.By("Set registry config")
		dirname := "/tmp/case77061"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		exutil.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)
		setRegistryVolume(oc, "deploy", "registry", oc.Namespace(), "30G", "/var/lib/registry")

		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-77060.yaml")
		imageDeleteYamlFileF := filepath.Join(ocmirrorBaseDir, "delete-config-77061.yaml")

		exutil.By("Start mirror2disk")
		defer os.RemoveAll("~/.oc-mirror/")
		defer os.RemoveAll("~/.oc-mirror.log")
		waitErr := wait.PollImmediate(30*time.Second, 900*time.Second, func() (bool, error) {
			_, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "file://"+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson").Output()
			if err != nil {
				e2e.Logf("The mirror2disk for mirroring helm chars with oc-mirror v2 failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2disk for mirroring helm charts with oc-mirror v2 failed, retrying...")

		exutil.By("Start disk2mirror")
		defer os.RemoveAll(".oc-mirror.log")
		waitErr = wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			disk2mirrorOutput, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--from", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
			if err != nil {
				e2e.Logf("The disk2mirror for mirroring helm charts with oc-mirror v2 failed, retrying...")
				return false, nil
			}
			if strings.Contains(disk2mirrorOutput, "idms-oc-mirror.yaml") && strings.Contains(disk2mirrorOutput, "itms-oc-mirror.yaml") {
				e2e.Logf("Helm chart mirroring via disk2mirror completed successfully")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "max time reached but the disk2mirror for mirroring helm charts with oc-mirror v2 failed")

		exutil.By("Generete delete image file")
		_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("delete", "--config", imageDeleteYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--workspace", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--generate").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Execute delete with force-cache-delete")
		_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("delete", "--delete-yaml-file", dirname+"/working-dir/delete/delete-images.yaml", "docker://"+serInfo.serviceName, "--v2", "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false", "--force-cache-delete=true").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Start mirror2mirror")
		defer os.RemoveAll(".oc-mirror.log")
		dirnameM2M := "/tmp/case77061m2m"
		defer os.RemoveAll(dirnameM2M)
		err = os.MkdirAll(dirnameM2M, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		waitErr = wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			mirror2mirrorOutput, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--workspace", "file://"+dirnameM2M, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
			if err != nil {
				e2e.Logf("The mirror2mirror for helm chart mirroring via oc-mirror v2 failed, retrying...")
				return false, nil
			}
			if strings.Contains(mirror2mirrorOutput, "idms-oc-mirror.yaml") && strings.Contains(mirror2mirrorOutput, "itms-oc-mirror.yaml") {
				e2e.Logf("Helm chart mirroring via mirror2mirror completed successfully")
				return true, nil
			}
			return false, nil

		})
		exutil.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2mirror for mirroring helm charts with oc-mirror v2 still failed")

		exutil.By("Generete delete image file")
		_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("delete", "--config", imageDeleteYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--workspace", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--generate").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Execute delete with force-cache-delete")
		_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("delete", "--delete-yaml-file", dirname+"/working-dir/delete/delete-images.yaml", "docker://"+serInfo.serviceName, "--v2", "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false", "--force-cache-delete=true").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

	})

})
