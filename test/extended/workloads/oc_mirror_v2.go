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
		_, warningOutput, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "--v2", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--strict-archive").Outputs()
		o.Expect(err).Should(o.HaveOccurred())
		o.Expect(strings.Contains(warningOutput, "maxArchiveSize 1G is too small compared to sizes of files")).To(o.BeTrue())

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
		payloadImageInfo, err := oc.WithoutNamespace().Run("image").Args("info", "--insecure", serInfo.serviceName+"/openshift-release-dev/ocp-release:4.15.19-s390x").Output()
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
		_, err = oc.WithoutNamespace().Run("image").Args("info", "--insecure", serInfo.serviceName+"/openshift-release-dev/ocp-release:4.15.19-s390x").Output()
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
		assertPodOutput(oc, "olm.catalogSource=cs-certified-operator-index-v4-15", "openshift-marketplace", "Running")
		assertPodOutput(oc, "olm.catalogSource=cs-community-operator-index-v4-15", "openshift-marketplace", "Running")
		assertPodOutput(oc, "olm.catalogSource=cs-redhat-marketplace-index-v4-15", "openshift-marketplace", "Running")

		exutil.By("Install operator from certified-operator CS")
		portworxSub, portworxOG := getOperatorInfo(oc, "portworx-certified", "portworx-certified-ns", "registry.redhat.io/redhat/certified-operator-index:v4.15", "cs-certified-operator-index-v4-15")
		defer removeOperatorFromCustomCS(oc, portworxSub, portworxOG, "portworx-certified-ns")
		installOperatorFromCustomCS(oc, portworxSub, portworxOG, "portworx-certified-ns", "portworx-operator")

		exutil.By("Install operator from redhat-marketplace CS")
		crunchySub, crunchyOG := getOperatorInfo(oc, "crunchy-postgres-operator-rhmp", "marketoperatortest", "registry.redhat.io/redhat/redhat-marketplace-index:v4.15", "cs-redhat-marketplace-index-v4-15")
		defer removeOperatorFromCustomCS(oc, crunchySub, crunchyOG, "marketoperatortest")
		installOperatorFromCustomCS(oc, crunchySub, crunchyOG, "marketoperatortest", "pgo")
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
			if !validateStringFromFile(m2dOutputFile, "bundle secondaryscheduleroperator.v5.0 of operator openshift-secondary-scheduler-operator not found in catalog: SKIPPING") && !validateStringFromFile(m2dOutputFile, "bundle cockroach-operator.v2.13.1 of operator cockroachdb-certified not found in catalog: SKIPPING") && !validateStringFromFile(m2dOutputFile, "bundle 3scale-community-operator.v0.9.1 of operator 3scale-community-operator not found in catalog: SKIPPING") {
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
		assertPodOutput(oc, "olm.catalogSource=cs-redhat-operator-index-v4-15", "openshift-marketplace", "Running")

		exutil.By("Install the operator from the new catalogsource")
		rhssoSub, rhssoOG := getOperatorInfo(oc, "openshift-secondary-scheduler-operator", "openshift-secondary-scheduler-operator", "registry.redhat.io/redhat/redhat-operator-index:v4.15", "cs-redhat-operator-index-v4-15")
		defer removeOperatorFromCustomCS(oc, rhssoSub, rhssoOG, "openshift-secondary-scheduler-operator")
		installOperatorFromCustomCS(oc, rhssoSub, rhssoOG, "openshift-secondary-scheduler-operator", "secondary-scheduler-operator")

	})

	g.It("Author:knarra-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-High-73124-Validate operator mirroring works fine for the catalog that does not follow same structure as RHOI [Serial]", func() {
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
		command := fmt.Sprintf("skopeo copy --all --format v2s2 docker://icr.io/cpopen/ibm-zcon-zosconnect-catalog@sha256:6f02ecef46020bcd21bdd24a01f435023d5fc3943972ef0d9769d5276e178e76 oci://%s  --remove-signatures --insecure-policy", dirname+"/ibm-catalog")
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

})
