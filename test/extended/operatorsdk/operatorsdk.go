package operatorsdk

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/openshift/openshift-tests-private/test/extended/util/architecture"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

	"path/filepath"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	container "github.com/openshift/openshift-tests-private/test/extended/util/container"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-operators] Operator_SDK should", func() {
	defer g.GinkgoRecover()

	var operatorsdkCLI = NewOperatorSDKCLI()
	var makeCLI = NewMakeCLI()
	var mvnCLI = NewMVNCLI()
	var oc = exutil.NewCLIWithoutNamespace("default")
	var ocpversion = "4.16"
	var ocppreversion = "4.15"
	var upstream = true

	// author: jfan@redhat.com
	g.It("VMonly-Author:jfan-High-37465-SDK olm improve olm related sub commands", func() {

		operatorsdkCLI.showInfo = true
		exutil.By("check the olm status")
		output, _ := operatorsdkCLI.Run("olm").Args("status", "--olm-namespace", "openshift-operator-lifecycle-manager").Output()
		o.Expect(output).To(o.ContainSubstring("Fetching CRDs for version"))
	})

	// author: jfan@redhat.com
	g.It("Author:jfan-High-37312-SDK olm improve manage operator bundles in new manifests metadata format", func() {

		operatorsdkCLI.showInfo = true
		tmpBasePath := "/tmp/ocp-37312-" + getRandomString()
		tmpPath := filepath.Join(tmpBasePath, "memcached-operator-37312")
		err := os.MkdirAll(tmpPath, 0o755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(tmpBasePath)
		operatorsdkCLI.ExecCommandPath = tmpPath
		makeCLI.ExecCommandPath = tmpPath

		exutil.By("Step: init Ansible Based Operator")
		_, err = operatorsdkCLI.Run("init").Args("--plugins", "ansible", "--domain", "example.com", "--group", "cache", "--version", "v1alpha1", "--kind", "Memcached", "--generate-playbook").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Step: Bundle manifests generate")
		result, err := exec.Command("bash", "-c", "cd "+tmpPath+" && operator-sdk generate bundle --deploy-dir=config --crds-dir=config/crds --version=0.0.1").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(result).To(o.ContainSubstring("Bundle manifests generated successfully in bundle"))

	})

	// author: jfan@redhat.com
	g.It("Author:jfan-High-37141-SDK Helm support simple structural schema generation for Helm CRDs", func() {

		operatorsdkCLI.showInfo = true
		tmpBasePath := "/tmp/ocp-37141-" + getRandomString()
		tmpPath := filepath.Join(tmpBasePath, "nginx-operator-37141")
		err := os.MkdirAll(tmpPath, 0o755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(tmpBasePath)
		operatorsdkCLI.ExecCommandPath = tmpPath
		makeCLI.ExecCommandPath = tmpPath

		exutil.By("Step: init Ansible Based Operator")
		_, err = operatorsdkCLI.Run("init").Args("--project-name", "nginx-operator", "--plugins", "helm.sdk.operatorframework.io/v1").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Step: Create API.")
		result, err := operatorsdkCLI.Run("create").Args("api", "--group", "apps", "--version", "v1beta1", "--kind", "Nginx").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(result).To(o.ContainSubstring("Created helm-charts/nginx"))

		annotationsFile := filepath.Join(tmpPath, "config", "crd", "bases", "apps.my.domain_nginxes.yaml")
		content := getContent(annotationsFile)
		o.Expect(content).To(o.ContainSubstring("x-kubernetes-preserve-unknown-fields: true"))

	})

	// author: jfan@redhat.com
	g.It("Author:jfan-High-37311-SDK ansible valid structural schemas for ansible based operators", func() {

		operatorsdkCLI.showInfo = true
		tmpBasePath := "/tmp/ocp-37311-" + getRandomString()
		tmpPath := filepath.Join(tmpBasePath, "ansible-operator-37311")
		err := os.MkdirAll(tmpPath, 0o755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(tmpBasePath)
		operatorsdkCLI.ExecCommandPath = tmpPath
		makeCLI.ExecCommandPath = tmpPath

		exutil.By("Step: init Ansible Based Operator")
		_, err = operatorsdkCLI.Run("init").Args("--project-name", "nginx-operator", "--plugins", "ansible.sdk.operatorframework.io/v1").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Step: Create API.")

		_, err = operatorsdkCLI.Run("create").Args("api", "--group", "apps", "--version", "v1beta1", "--kind", "Nginx").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		annotationsFile := filepath.Join(tmpPath, "config", "crd", "bases", "apps.my.domain_nginxes.yaml")
		content := getContent(annotationsFile)
		o.Expect(content).To(o.ContainSubstring("x-kubernetes-preserve-unknown-fields: true"))

	})

	// author: jfan@redhat.com
	g.It("VMonly-ConnectedOnly-Author:jfan-High-37627-SDK run bundle upgrade test [Serial]", func() {
		skipOnProxyCluster(oc)
		operatorsdkCLI.showInfo = true
		oc.SetupProject()
		defer func() {
			output, err := operatorsdkCLI.Run("cleanup").Args("upgradeoperator", "-n", oc.Namespace()).Output()
			if err != nil {
				e2e.Logf(output)
				o.Expect(err).NotTo(o.HaveOccurred())
			}
		}()
		output, err := operatorsdkCLI.Run("run").Args("bundle", "quay.io/olmqe/upgradeoperator-bundle:v0.1", "-n", oc.Namespace(), "--timeout", "5m", "--security-context-config=restricted").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("OLM has successfully installed"))
		output, err = operatorsdkCLI.Run("run").Args("bundle-upgrade", "quay.io/olmqe/upgradeoperator-bundle:v0.2", "-n", oc.Namespace(), "--timeout", "5m", "--security-context-config=restricted").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Successfully upgraded to"))
		waitErr := wait.PollUntilContextTimeout(context.TODO(), 15*time.Second, 360*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", "upgradeoperator.v0.0.2", "-n", oc.Namespace()).Output()
			if strings.Contains(msg, "Succeeded") {
				e2e.Logf("upgrade to 0.2 success")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("upgradeoperator upgrade failed in %s ", oc.Namespace()))

	})

	// author: jfan@redhat.com
	g.It("VMonly-ConnectedOnly-Author:jfan-Medium-38054-SDK run bundle create pods and csv and registry image pod [Serial]", func() {
		skipOnProxyCluster(oc)
		operatorsdkCLI.showInfo = true
		oc.SetupProject()
		output, err := operatorsdkCLI.Run("run").Args("bundle", "quay.io/olmqe/podcsvcheck-bundle:v0.0.1", "-n", oc.Namespace(), "--timeout", "5m", "--security-context-config=restricted").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("OLM has successfully installed"))
		output, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", oc.Namespace()).Output()
		o.Expect(output).To(o.ContainSubstring("quay-io-olmqe-podcsvcheck-bundle-v0-0-1"))
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", "podcsvcheck.v0.0.1", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Succeeded"))
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("catalogsource", "podcsvcheck-catalog", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("grpc"))
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("installplan", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("podcsvcheck.v0.0.1"))
		output, err = operatorsdkCLI.Run("cleanup").Args("podcsvcheck", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("uninstalled"))
	})

	// author: jfan@redhat.com
	g.It("ConnectedOnly-Author:jfan-High-38060-SDK run bundle detail message about failed", func() {
		skipOnProxyCluster(oc)
		operatorsdkCLI.showInfo = true
		oc.SetupProject()
		output, _ := operatorsdkCLI.Run("run").Args("bundle", "quay.io/olmqe/etcd-bundle:0.0.1", "-n", oc.Namespace(), "--security-context-config=restricted").Output()
		o.Expect(output).To(o.ContainSubstring("quay.io/olmqe/etcd-bundle:0.0.1: not found"))
	})

	// author: jfan@redhat.com
	g.It("Author:jfan-High-34441-SDK commad operator sdk support init help message", func() {
		output, err := operatorsdkCLI.Run("init").Args("--help").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("--project-name"))
	})

	// author: jfan@redhat.com
	g.It("Author:jfan-Medium-40521-SDK olm improve manage operator bundles in new manifests metadata format", func() {

		operatorsdkCLI.showInfo = true
		tmpBasePath := "/tmp/ocp-40521-" + getRandomString()
		tmpPath := filepath.Join(tmpBasePath, "memcached-operator-40521")
		err := os.MkdirAll(tmpPath, 0o755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(tmpBasePath)
		operatorsdkCLI.ExecCommandPath = tmpPath
		makeCLI.ExecCommandPath = tmpPath

		exutil.By("Step: init Ansible Based Operator")
		_, err = operatorsdkCLI.Run("init").Args("--plugins", "ansible.sdk.operatorframework.io/v1", "--domain", "example.com", "--group", "cache", "--version", "v1alpha1", "--kind", "Memcached", "--generate-playbook").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Step: Bundle manifests generate")
		result, err := exec.Command("bash", "-c", "cd "+tmpPath+" && operator-sdk generate bundle --deploy-dir=config --crds-dir=config/crds --version=0.0.1").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(result).To(o.ContainSubstring("Bundle manifests generated successfully in bundle"))

		exec.Command("bash", "-c", "cd "+tmpPath+" && sed -i '/icon/,+2d' ./bundle/manifests/memcached-operator-40521.clusterserviceversion.yaml").Output()
		msg, err := exec.Command("bash", "-c", "cd "+tmpPath+" && operator-sdk bundle validate ./bundle &> ./validateresult && cat validateresult").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(msg).To(o.ContainSubstring("All validation tests have completed successfully"))

	})

	// author: jfan@redhat.com
	g.It("VMonly-ConnectedOnly-Author:jfan-Medium-40520-SDK k8sutil 1123Label creates invalid values", func() {
		skipOnProxyCluster(oc)
		operatorsdkCLI.showInfo = true
		oc.SetupProject()
		namespace := oc.Namespace()
		msg, _ := operatorsdkCLI.Run("run").Args("bundle", "quay.io/olmqe/raffaelespazzoli-proactive-node-scaling-operator-bundle:latest-", "-n", namespace, "--timeout", "5m", "--security-context-config=restricted").Output()
		o.Expect(msg).To(o.ContainSubstring("reated registry pod: raffaelespazzoli-proactive-node-scaling-operator-bundle-latest"))
	})

	// author: jfan@redhat.com
	g.It("VMonly-ConnectedOnly-Author:jfan-Medium-35443-SDK run bundle InstallMode for own namespace [Slow] [Serial]", func() {
		skipOnProxyCluster(oc)
		buildPruningBaseDir := exutil.FixturePath("testdata", "olm")
		var operatorGroup = filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
		operatorsdkCLI.showInfo = true
		oc.SetupProject()
		namespace := oc.Namespace()
		// install the operator without og with installmode
		msg, err := operatorsdkCLI.Run("run").Args("bundle", "quay.io/olmqe/ownsingleallsupport-bundle:v4.11", "--install-mode", "OwnNamespace", "-n", namespace, "--timeout", "5m", "--security-context-config=restricted").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(msg).To(o.ContainSubstring("OLM has successfully installed"))
		msg, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args("og", "operator-sdk-og", "-n", namespace, "-o=jsonpath={.spec.targetNamespaces}").Output()
		o.Expect(msg).To(o.ContainSubstring(namespace))
		output, err := operatorsdkCLI.Run("cleanup").Args("ownsingleallsupport", "-n", namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("uninstalled"))
		waitErr := wait.PollUntilContextTimeout(context.TODO(), 15*time.Second, 360*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "quay-io-olmqe-ownsingleallsupport-bundle-v4-11", "-n", namespace, "--no-headers").Output()
			if strings.Contains(msg, "not found") {
				e2e.Logf("not found pod quay-io-olmqe-ownsingleallsupport-bundle")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("pod quay-io-olmqe-ownsingleallsupport-bundle can't be deleted in %s", namespace))
		// install the operator with og and installmode
		configFile, err := oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", operatorGroup, "-p", "NAME=og-own", "NAMESPACE="+namespace).OutputToFile("config-35443.json")
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", configFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		msg, err = operatorsdkCLI.Run("run").Args("bundle", "quay.io/olmqe/ownsingleallsupport-bundle:v4.11", "--install-mode", "OwnNamespace", "-n", namespace, "--timeout", "5m", "--security-context-config=restricted").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(msg).To(o.ContainSubstring("OLM has successfully installed"))
		output, _ = operatorsdkCLI.Run("cleanup").Args("ownsingleallsupport", "-n", namespace).Output()
		o.Expect(output).To(o.ContainSubstring("uninstalled"))
		// install the operator with og without installmode
		waitErr = wait.PollUntilContextTimeout(context.TODO(), 15*time.Second, 360*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "quay-io-olmqe-ownsingleallsupport-bundle-v4-11", "-n", namespace, "--no-headers").Output()
			if strings.Contains(msg, "not found") {
				e2e.Logf("not found pod quay-io-olmqe-ownsingleallsupport-bundle")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("pod quay-io-olmqe-ownsingleallsupport-bundle can't be deleted in %s", namespace))
		msg, err = operatorsdkCLI.Run("run").Args("bundle", "quay.io/olmqe/ownsingleallsupport-bundle:v4.11", "-n", namespace, "--timeout", "5m", "--security-context-config=restricted").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(msg).To(o.ContainSubstring("OLM has successfully installed"))
		output, _ = operatorsdkCLI.Run("cleanup").Args("ownsingleallsupport", "-n", namespace).Output()
		o.Expect(output).To(o.ContainSubstring("uninstalled"))
		// delete the og
		_, err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("og", "og-own", "-n", namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		waitErr = wait.PollUntilContextTimeout(context.TODO(), 15*time.Second, 360*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "quay-io-olmqe-ownsingleallsupport-bundle-v4-11", "-n", namespace, "--no-headers").Output()
			if strings.Contains(msg, "not found") {
				e2e.Logf("quay-io-olmqe-ownsingleallsupport-bundle")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("pod quay-io-olmqe-ownsingleallsupport-bundle can't be deleted in %s", namespace))
		// install the operator without og and installmode, the csv support ownnamespace and singlenamespace
		msg, err = operatorsdkCLI.Run("run").Args("bundle", "quay.io/olmqe/ownsinglesupport-bundle:v4.11", "-n", namespace, "--timeout", "5m", "--security-context-config=restricted").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(msg).To(o.ContainSubstring("OLM has successfully installed"))
		msg, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args("og", "operator-sdk-og", "-n", namespace, "-o=jsonpath={.spec.targetNamespaces}").Output()
		o.Expect(msg).To(o.ContainSubstring(namespace))
		output, _ = operatorsdkCLI.Run("cleanup").Args("ownsinglesupport", "-n", namespace).Output()
		o.Expect(output).To(o.ContainSubstring("uninstalled"))
	})

	// author: jfan@redhat.com
	g.It("VMonly-ConnectedOnly-Author:jfan-Medium-41064-SDK run bundle InstallMode for single namespace [Slow] [Serial]", func() {
		skipOnProxyCluster(oc)
		buildPruningBaseDir := exutil.FixturePath("testdata", "operatorsdk")
		var operatorGroup = filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
		operatorsdkCLI.showInfo = true
		oc.SetupProject()
		namespace := oc.Namespace()
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", "test-sdk-41064").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("ns", "test-sdk-41064").Execute()
		msg, err := operatorsdkCLI.Run("run").Args("bundle", "quay.io/olmqe/all1support-bundle:v4.11", "--install-mode", "SingleNamespace=test-sdk-41064", "-n", namespace, "--timeout", "5m", "--security-context-config=restricted").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(msg).To(o.ContainSubstring("OLM has successfully installed"))
		msg, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args("og", "operator-sdk-og", "-n", namespace, "-o=jsonpath={.spec.targetNamespaces}").Output()
		o.Expect(msg).To(o.ContainSubstring("test-sdk-41064"))
		output, err := operatorsdkCLI.Run("cleanup").Args("all1support", "-n", namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("uninstalled"))
		waitErr := wait.PollUntilContextTimeout(context.TODO(), 15*time.Second, 360*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "quay-io-olmqe-all1support-bundle-v4-11", "-n", namespace, "--no-headers").Output()
			if strings.Contains(msg, "not found") {
				e2e.Logf("not found pod quay-io-olmqe-all1support-bundle")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("pod quay-io-olmqe-all1support-bundle can't be deleted in %s", namespace))
		// install the operator with og and installmode
		configFile, err := oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", operatorGroup, "-p", "NAME=og-single", "NAMESPACE="+namespace, "KAKA=test-sdk-41064").OutputToFile("config-41064.json")
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", configFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		msg, err = operatorsdkCLI.Run("run").Args("bundle", "quay.io/olmqe/all1support-bundle:v4.11", "--install-mode", "SingleNamespace=test-sdk-41064", "-n", namespace, "--timeout", "5m", "--security-context-config=restricted").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(msg).To(o.ContainSubstring("OLM has successfully installed"))
		output, _ = operatorsdkCLI.Run("cleanup").Args("all1support", "-n", namespace).Output()
		o.Expect(output).To(o.ContainSubstring("uninstalled"))
		// install the operator with og without installmode
		waitErr = wait.PollUntilContextTimeout(context.TODO(), 15*time.Second, 360*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "quay-io-olmqe-all1support-bundle-v4-11", "-n", namespace, "--no-headers").Output()
			if strings.Contains(msg, "not found") {
				e2e.Logf("not found pod quay-io-olmqe-all1support-bundle")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("pod quay-io-olmqe-all1support-bundle can't be deleted in %s", namespace))
		msg, err = operatorsdkCLI.Run("run").Args("bundle", "quay.io/olmqe/all1support-bundle:v4.11", "-n", namespace, "--timeout", "5m", "--security-context-config=restricted").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(msg).To(o.ContainSubstring("OLM has successfully installed"))
		output, _ = operatorsdkCLI.Run("cleanup").Args("all1support", "-n", namespace).Output()
		o.Expect(output).To(o.ContainSubstring("uninstalled"))
		// delete the og
		_, err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("og", "og-single", "-n", namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		waitErr = wait.PollUntilContextTimeout(context.TODO(), 15*time.Second, 360*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "quay-io-olmqe-all1support-bundle-v4-11", "-n", namespace, "--no-headers").Output()
			if strings.Contains(msg, "not found") {
				e2e.Logf("not found pod quay-io-olmqe-all1support-bundle")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("pod quay-io-olmqe-all1support-bundle can't be deleted in %s", namespace))
		// install the operator without og and installmode, the csv only support singlenamespace
		msg, _ = operatorsdkCLI.Run("run").Args("bundle", "quay.io/olmqe/singlesupport-bundle:v4.11", "-n", namespace, "--timeout", "5m", "--security-context-config=restricted").Output()
		o.Expect(msg).To(o.ContainSubstring("AllNamespaces InstallModeType not supported"))
		output, _ = operatorsdkCLI.Run("cleanup").Args("singlesupport", "-n", namespace).Output()
		o.Expect(output).To(o.ContainSubstring("uninstalled"))
	})

	// author: jfan@redhat.com
	g.It("VMonly-ConnectedOnly-Author:jfan-Medium-41065-SDK run bundle InstallMode for all namespace [Slow] [Serial]", func() {
		skipOnProxyCluster(oc)
		buildPruningBaseDir := exutil.FixturePath("testdata", "olm")
		var operatorGroup = filepath.Join(buildPruningBaseDir, "og-allns.yaml")
		operatorsdkCLI.showInfo = true
		oc.SetupProject()
		namespace := oc.Namespace()
		// install the operator without og with installmode all namespace
		msg, err := operatorsdkCLI.Run("run").Args("bundle", "quay.io/olmqe/all2support-bundle:v4.11", "--install-mode", "AllNamespaces", "-n", namespace, "--timeout", "5m", "--security-context-config=restricted").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(msg).To(o.ContainSubstring("OLM has successfully installed"))
		defer operatorsdkCLI.Run("cleanup").Args("all2support").Output()
		msg, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args("og", "operator-sdk-og", "-o=jsonpath={.spec.targetNamespaces}", "-n", namespace).Output()
		o.Expect(msg).To(o.ContainSubstring(""))
		waitErr := wait.PollUntilContextTimeout(context.TODO(), 15*time.Second, 360*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", "-n", "openshift-operators").Output()
			if strings.Contains(msg, "all2support.v0.0.1") {
				e2e.Logf("csv all2support.v0.0.1")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("can't get csv all2support.v0.0.1 in %s", namespace))
		output, err := operatorsdkCLI.Run("cleanup").Args("all2support", "-n", namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("uninstalled"))
		waitErr = wait.PollUntilContextTimeout(context.TODO(), 15*time.Second, 360*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "quay-io-olmqe-all2support-bundle-v4-11", "--no-headers", "-n", namespace).Output()
			if strings.Contains(msg, "not found") {
				e2e.Logf("not found pod quay-io-olmqe-all2support-bundle")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("pod quay-io-olmqe-all2support-bundle can't be deleted in %s", namespace))
		// install the operator with og and installmode
		configFile, err := oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", operatorGroup, "-p", "NAME=og-allnames", "NAMESPACE="+namespace).OutputToFile("config-41065.json")
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", configFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		msg, err = operatorsdkCLI.Run("run").Args("bundle", "quay.io/olmqe/all2support-bundle:v4.11", "--install-mode", "AllNamespaces", "-n", namespace, "--timeout", "5m", "--security-context-config=restricted").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(msg).To(o.ContainSubstring("OLM has successfully installed"))
		waitErr = wait.PollUntilContextTimeout(context.TODO(), 15*time.Second, 360*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", "-n", "openshift-operators").Output()
			if strings.Contains(msg, "all2support.v0.0.1") {
				e2e.Logf("csv all2support.v0.0.1")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("can't get csv all2support.v0.0.1 in %s", namespace))
		output, _ = operatorsdkCLI.Run("cleanup").Args("all2support", "-n", namespace).Output()
		o.Expect(output).To(o.ContainSubstring("uninstalled"))
		// install the operator with og without installmode
		waitErr = wait.PollUntilContextTimeout(context.TODO(), 15*time.Second, 360*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "quay-io-olmqe-all2support-bundle-v4-11", "--no-headers", "-n", namespace).Output()
			if strings.Contains(msg, "not found") {
				e2e.Logf("not found pod quay-io-olmqe-all2support-bundle")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("pod quay-io-olmqe-all2support-bundle can't be deleted in %s", namespace))
		msg, err = operatorsdkCLI.Run("run").Args("bundle", "quay.io/olmqe/all2support-bundle:v4.11", "-n", namespace, "--timeout", "5m", "--security-context-config=restricted").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(msg).To(o.ContainSubstring("OLM has successfully installed"))
		waitErr = wait.PollUntilContextTimeout(context.TODO(), 15*time.Second, 360*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", "-n", "openshift-operators").Output()
			if strings.Contains(msg, "all2support.v0.0.1") {
				e2e.Logf("csv all2support.v0.0.1")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("can't get csv all2support.v0.0.1 in %s", namespace))
		output, _ = operatorsdkCLI.Run("cleanup").Args("all2support", "-n", namespace).Output()
		o.Expect(output).To(o.ContainSubstring("uninstalled"))
		// delete the og
		_, err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("og", "og-allnames", "-n", namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		waitErr = wait.PollUntilContextTimeout(context.TODO(), 15*time.Second, 360*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "quay-io-olmqe-all2support-bundle-v4-11", "--no-headers", "-n", namespace).Output()
			if strings.Contains(msg, "not found") {
				e2e.Logf("not found pod quay-io-olmqe-all2support-bundle")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("pod quay-io-olmqe-all2support-bundle can't be deleted in %s", namespace))
		// install the operator without og and installmode, the csv support allnamespace and ownnamespace
		msg, _ = operatorsdkCLI.Run("run").Args("bundle", "quay.io/olmqe/all2support-bundle:v4.11", "-n", namespace, "--timeout", "5m", "--security-context-config=restricted").Output()
		o.Expect(msg).To(o.ContainSubstring("OLM has successfully installed"))
		waitErr = wait.PollUntilContextTimeout(context.TODO(), 15*time.Second, 360*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", "-n", "openshift-operators").Output()
			if strings.Contains(msg, "all2support.v0.0.1") {
				e2e.Logf("csv all2support.v0.0.1")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("can't get csv all2support.v0.0.1 in %s", namespace))
		output, _ = operatorsdkCLI.Run("cleanup").Args("all2support", "-n", namespace).Output()
		o.Expect(output).To(o.ContainSubstring("uninstalled"))
	})

	// author: jfan@redhat.com
	g.It("VMonly-ConnectedOnly-Author:jfan-Medium-38757-SDK operator bundle upgrade from traditional operator installation", func() {
		skipOnProxyCluster(oc)
		buildPruningBaseDir := exutil.FixturePath("testdata", "operatorsdk")
		var catalogofupgrade = filepath.Join(buildPruningBaseDir, "catalogsource.yaml")
		var ogofupgrade = filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
		var subofupgrade = filepath.Join(buildPruningBaseDir, "sub.yaml")
		operatorsdkCLI.showInfo = true
		oc.SetupProject()
		namespace := oc.Namespace()
		// install operator from sub
		defer operatorsdkCLI.Run("cleanup").Args("upgradeindex", "-n", namespace).Output()
		createCatalog, _ := oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", catalogofupgrade, "-p", "NAME=upgradetest", "NAMESPACE="+namespace, "ADDRESS=quay.io/olmqe/upgradeindex-index:v0.1", "DISPLAYNAME=KakaTest").OutputToFile("catalogsource-41497.json")
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", createCatalog, "-n", namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		createOg, _ := oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", ogofupgrade, "-p", "NAME=kakatest-single", "NAMESPACE="+namespace, "KAKA="+namespace).OutputToFile("createog-41497.json")
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", createOg, "-n", namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		createSub, _ := oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", subofupgrade, "-p", "NAME=subofupgrade", "NAMESPACE="+namespace, "SOURCENAME=upgradetest", "OPERATORNAME=upgradeindex", "SOURCENAMESPACE="+namespace, "STARTINGCSV=upgradeindex.v0.0.1").OutputToFile("createsub-41497.json")
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", createSub, "-n", namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		waitErr := wait.PollUntilContextTimeout(context.TODO(), 15*time.Second, 360*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", "upgradeindex.v0.0.1", "-o=jsonpath={.status.phase}", "-n", namespace).Output()
			if strings.Contains(msg, "Succeeded") {
				e2e.Logf("upgradeindexv0.1 installed successfully")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("can't get csv upgradeindex.v0.0.1 %s", namespace))
		// upgrade by operator-sdk
		msg, err := operatorsdkCLI.Run("run").Args("bundle-upgrade", "quay.io/olmqe/upgradeindex-bundle:v0.2", "-n", namespace, "--timeout", "5m", "--security-context-config=restricted").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(msg).To(o.ContainSubstring("Successfully upgraded to"))
	})

	// author: jfan@redhat.com
	g.It("VMonly-ConnectedOnly-Author:jfan-High-42928-SDK support the previous base ansible image [Slow]", func() {
		clusterArchitecture := architecture.SkipNonAmd64SingleArch(oc)
		// test data
		buildPruningBaseDir := exutil.FixturePath("testdata", "operatorsdk")
		dataPath := filepath.Join(buildPruningBaseDir, "ocp-42928-data")
		crFilePath := filepath.Join(dataPath, "ansibletest_v1_previoustest.yaml")
		// exec dir
		tmpBasePath := "/tmp/ocp-42928-" + getRandomString()
		tmpPath := filepath.Join(tmpBasePath, "previousansibletest")
		operatorsdkCLI.ExecCommandPath = tmpPath
		makeCLI.ExecCommandPath = tmpPath
		// exec ns & image tag
		nsOperator := "previousansibletest-system"
		imageTag := "quay.io/olmqe/previousansibletest:" + ocpversion + "-" + getRandomString()
		// cleanup the test data
		err := os.MkdirAll(tmpPath, 0o755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(tmpBasePath)
		quayCLI := container.NewQuayCLI()
		defer quayCLI.DeleteTag(strings.Replace(imageTag, "quay.io/", "", 1))

		defer func() {
			_, err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", crFilePath, "-n", nsOperator).Output()
			exutil.By("step: undeploy")
			_, err = makeCLI.Run("undeploy").Args().Output()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()

		exutil.By("step: init Ansible Based Operator")
		output, err := operatorsdkCLI.Run("init").Args("--plugins=ansible", "--domain", "qetest.com").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Next"))

		exutil.By("step: Create API.")
		output, err = operatorsdkCLI.Run("create").Args("api", "--group", "ansibletest", "--version", "v1", "--kind", "Previoustest", "--generate-role").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Writing kustomize manifests"))

		exutil.By("step: modify files to get the quay.io/olmqe images.")
		// copy task main.yml
		err = copy(filepath.Join(dataPath, "main.yml"), filepath.Join(tmpPath, "roles", "previoustest", "tasks", "main.yml"))
		o.Expect(err).NotTo(o.HaveOccurred())
		// copy Dockerfile
		dockerfileFilePath := filepath.Join(dataPath, "Dockerfile")
		err = copy(dockerfileFilePath, filepath.Join(tmpPath, "Dockerfile"))
		o.Expect(err).NotTo(o.HaveOccurred())
		replaceContent(filepath.Join(tmpPath, "Dockerfile"), "brew.registry.redhat.io/rh-osbs/openshift-ose-ansible-operator:vocpversion", "brew.registry.redhat.io/rh-osbs/openshift-ose-ansible-operator:v"+ocppreversion)
		// copy manager_auth_proxy_patch.yaml
		authFilePath := filepath.Join(tmpPath, "config", "default", "manager_auth_proxy_patch.yaml")
		err = copy(filepath.Join(dataPath, "manager_auth_proxy_patch.yaml"), authFilePath)
		o.Expect(err).NotTo(o.HaveOccurred())
		replaceContent(authFilePath, "registry.redhat.io/openshift4/ose-kube-rbac-proxy:vocpversion", "quay.io/olmqe/kube-rbac-proxy:v"+ocppreversion)
		// copy manager.yaml
		err = copy(filepath.Join(dataPath, "manager.yaml"), filepath.Join(tmpPath, "config", "manager", "manager.yaml"))
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("step: Build and push the operator image")
		tokenDir := "/tmp/ocp-42928" + getRandomString()
		err = os.MkdirAll(tokenDir, os.ModePerm)
		defer os.RemoveAll(tokenDir)
		if err != nil {
			e2e.Failf("fail to create the token folder:%s", tokenDir)
		}
		_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", fmt.Sprintf("--to=%s", tokenDir), "--confirm").Output()
		if err != nil {
			e2e.Failf("Fail to get the cluster auth %v", err)
		}
		buildPushOperatorImage(clusterArchitecture, tmpPath, imageTag, tokenDir)

		exutil.By("step: Install the CRD")
		output, err = makeCLI.Run("install").Args().Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("previoustests.ansibletest.qetest.com"))

		exutil.By("step: Deploy the operator")
		output, err = makeCLI.Run("deploy").Args("IMG=" + imageTag).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("deployment.apps/previousansibletest-controller-manager"))

		waitErr := wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 300*time.Second, false, func(ctx context.Context) (bool, error) {
			podList, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", nsOperator).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			lines := strings.Split(podList, "\n")
			for _, line := range lines {
				if strings.Contains(line, "previousansibletest-controller-manager") {
					e2e.Logf("found pod previousansibletest-controller-manager")
					if strings.Contains(line, "2/2") {
						e2e.Logf("the status of pod previousansibletest-controller-manager is Running")
						return true, nil
					}
					e2e.Logf("the status of pod previousansibletest-controller-manager is not Running")
					return false, nil
				}
			}
			return false, nil
		})
		if waitErr != nil {
			logDebugInfo(oc, nsOperator, "events", "pod")
		}
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("No previousansibletest-controller-manager in project %s", nsOperator))
		msg, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("deployment.apps/previousansibletest-controller-manager", "-c", "manager", "-n", nsOperator).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(msg, "Starting workers") {
			e2e.Failf("Starting workers failed")
		}

		// max concurrent reconciles
		waitErr = wait.PollUntilContextTimeout(context.TODO(), 5*time.Second, 180*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, _ := oc.AsAdmin().WithoutNamespace().Run("logs").Args("deploy/previousansibletest-controller-manager", "-c", "manager", "-n", nsOperator).Output()
			if strings.Contains(msg, "\"worker count\":1") {
				e2e.Logf("found worker count:1")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("log of deploy/previousansibletest-controller-manager of %s doesn't have worker count:4", nsOperator))

		// add the admin policy
		err = oc.AsAdmin().Run("adm").Args("policy", "add-cluster-role-to-user", "cluster-admin", "system:serviceaccount:"+nsOperator+":previousansibletest-controller-manager").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("step: Create the resource")
		_, err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", crFilePath, "-n", nsOperator).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		waitErr = wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 300*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", nsOperator).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(msg, "previoustest-sample") {
				e2e.Logf("found pod previoustest-sample")
				return true, nil
			}
			return false, nil
		})
		if waitErr != nil {
			logDebugInfo(oc, nsOperator, "events", "pod")
		}
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("No previoustest-sample in project %s", nsOperator))

		waitErr = wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 180*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, err = oc.AsAdmin().WithoutNamespace().Run("describe").Args("deployment/previoustest-sample", "-n", nsOperator).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(msg, "2 desired | 2 updated | 2 total | 2 available | 0 unavailable") {
				e2e.Logf("deployment/previoustest-sample is created successfully")
				return true, nil
			}
			return false, nil
		})
		if waitErr != nil {
			logDebugInfo(oc, nsOperator, "events")
		}
		exutil.AssertWaitPollNoErr(waitErr, "the status of deployment/previoustest-sample is wrong")

		// k8s event
		waitErr = wait.PollUntilContextTimeout(context.TODO(), 5*time.Second, 180*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("event", "-n", nsOperator).Output()
			if strings.Contains(msg, "test-reason") {
				e2e.Logf("k8s_event test")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("can't get k8s event test-name in %s", nsOperator))

		// k8s status
		waitErr = wait.PollUntilContextTimeout(context.TODO(), 5*time.Second, 180*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("previoustest.ansibletest.qetest.com/previoustest-sample", "-n", nsOperator, "-o", "yaml").Output()
			if strings.Contains(msg, "hello world") {
				e2e.Logf("k8s_status test hello world")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("can't get previoustest-sample hello world in %s", nsOperator))

		// migrate test
		waitErr = wait.PollUntilContextTimeout(context.TODO(), 5*time.Second, 180*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret", "-n", nsOperator).Output()
			if strings.Contains(msg, "test-secret") {
				e2e.Logf("found secret test-secret")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("doesn't get secret test-secret %s", nsOperator))
		msg, err = oc.AsAdmin().WithoutNamespace().Run("describe").Args("secret", "test-secret", "-n", nsOperator).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(msg).To(o.ContainSubstring("test:  6 bytes"))

		// blacklist
		waitErr = wait.PollUntilContextTimeout(context.TODO(), 5*time.Second, 300*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, _ := oc.AsAdmin().WithoutNamespace().Run("describe").Args("configmap", "test-blacklist-watches", "-n", nsOperator).Output()
			if strings.Contains(msg, "afdasdfsajsafj") {
				e2e.Logf("Skipping the blacklist")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("log of deploy/previousansibletest-controller-manager of %s doesn't work the blacklist", nsOperator))
		_, err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", crFilePath, "-n", nsOperator).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = makeCLI.Run("undeploy").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	// author: jfan@redhat.com
	g.It("VMonly-ConnectedOnly-Author:jfan-High-42929-SDK support the previous base helm image", func() {
		skipOnProxyCluster(oc)
		buildPruningBaseDir := exutil.FixturePath("testdata", "operatorsdk")
		var nginx = filepath.Join(buildPruningBaseDir, "helmbase_v1_nginx.yaml")
		operatorsdkCLI.showInfo = true
		oc.SetupProject()
		namespace := oc.Namespace()
		defer operatorsdkCLI.Run("cleanup").Args("previoushelmbase", "-n", namespace).Output()
		_, err := operatorsdkCLI.Run("run").Args("bundle", "quay.io/olmqe/previoushelmbase-bundle:v4.11", "-n", namespace, "--timeout", "5m", "--security-context-config=restricted").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		createPreviousNginx, err := oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", nginx, "-p", "NAME=previousnginx-sample").OutputToFile("config-42929.json")
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", createPreviousNginx, "-n", namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		waitErr := wait.PollUntilContextTimeout(context.TODO(), 15*time.Second, 360*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", namespace, "--no-headers").Output()
			if strings.Contains(msg, "nginx-sample") {
				e2e.Logf("found pod nginx-sample")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("can't get pod nginx-sample in %s", namespace))
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("nginx.helmbase.previous.com", "previousnginx-sample", "-n", namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		output, _ := operatorsdkCLI.Run("cleanup").Args("previoushelmbase", "-n", namespace).Output()
		o.Expect(output).To(o.ContainSubstring("uninstalled"))
	})

	// author: jfan@redhat.com
	g.It("ConnectedOnly-VMonly-Author:jfan-High-42614-SDK validate the deprecated APIs and maxOpenShiftVersion", func() {
		operatorsdkCLI.showInfo = true
		exec.Command("bash", "-c", "mkdir -p /tmp/ocp-42614/traefikee-operator").Output()
		defer exec.Command("bash", "-c", "rm -rf /tmp/ocp-42614").Output()
		exec.Command("bash", "-c", "cp -rf test/extended/testdata/operatorsdk/ocp-42614-data/bundle/ /tmp/ocp-42614/traefikee-operator/").Output()

		exutil.By("with deprecated api, with maxOpenShiftVersion")
		msg, err := operatorsdkCLI.Run("bundle").Args("validate", "/tmp/ocp-42614/traefikee-operator/bundle", "--select-optional", "name=community", "-o", "json-alpha1").Output()
		o.Expect(msg).To(o.ContainSubstring("This bundle is using APIs which were deprecated and removed in"))
		o.Expect(msg).NotTo(o.ContainSubstring("error"))
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("with deprecated api, with higher version maxOpenShiftVersion")
		exec.Command("bash", "-c", "sed -i 's/4.8/4.9/g' /tmp/ocp-42614/traefikee-operator/bundle/manifests/traefikee-operator.v2.1.1.clusterserviceversion.yaml").Output()
		msg, _ = operatorsdkCLI.Run("bundle").Args("validate", "/tmp/ocp-42614/traefikee-operator/bundle", "--select-optional", "name=community", "-o", "json-alpha1").Output()
		o.Expect(msg).To(o.ContainSubstring("This bundle is using APIs which were deprecated and removed"))
		o.Expect(msg).To(o.ContainSubstring("error"))

		exutil.By("with deprecated api, with wrong maxOpenShiftVersion")
		exec.Command("bash", "-c", "sed -i 's/4.9/invalid/g' /tmp/ocp-42614/traefikee-operator/bundle/manifests/traefikee-operator.v2.1.1.clusterserviceversion.yaml").Output()
		msg, _ = operatorsdkCLI.Run("bundle").Args("validate", "/tmp/ocp-42614/traefikee-operator/bundle", "--select-optional", "name=community", "-o", "json-alpha1").Output()
		o.Expect(msg).To(o.ContainSubstring("csv.Annotations.olm.properties has an invalid value"))
		o.Expect(msg).To(o.ContainSubstring("error"))

		exutil.By("with deprecated api, without maxOpenShiftVersion")
		exec.Command("bash", "-c", "sed -i '/invalid/d' /tmp/ocp-42614/traefikee-operator/bundle/manifests/traefikee-operator.v2.1.1.clusterserviceversion.yaml").Output()
		msg, _ = operatorsdkCLI.Run("bundle").Args("validate", "/tmp/ocp-42614/traefikee-operator/bundle", "--select-optional", "name=community", "-o", "json-alpha1").Output()
		o.Expect(msg).To(o.ContainSubstring("csv.Annotations not specified olm.maxOpenShiftVersion for an OCP version"))
		o.Expect(msg).To(o.ContainSubstring("error"))
	})

	// author: jfan@redhat.com
	g.It("VMonly-ConnectedOnly-Author:jfan-High-34462-SDK playbook ansible operator generate the catalog", func() {

		clusterArchitecture := architecture.SkipArchitectures(oc, architecture.ARM64, architecture.MULTI, architecture.PPC64LE, architecture.S390X)
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "operatorsdk")
			catalogofcatalog    = filepath.Join(buildPruningBaseDir, "catalogsource.yaml")
			ogofcatalog         = filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
			subofcatalog        = filepath.Join(buildPruningBaseDir, "sub.yaml")
			dataPath            = filepath.Join(buildPruningBaseDir, "ocp-34462-data")
			tmpBasePath         = "/tmp/ocp-34462-" + getRandomString()
			tmpPath             = filepath.Join(tmpBasePath, "catalogtest")
			imageTag            = "quay.io/olmqe/catalogtest-operator:34462-v" + ocpversion + getRandomString()
			bundleImage         = "quay.io/olmqe/catalogtest-bundle:34462-v" + ocpversion + getRandomString()
			catalogImage        = "quay.io/olmqe/catalogtest-index:34462-v" + ocpversion + getRandomString()
			quayCLI             = container.NewQuayCLI()
		)

		operatorsdkCLI.showInfo = true
		oc.SetupProject()

		defer os.RemoveAll(tmpBasePath)
		err := os.MkdirAll(tmpPath, 0o755)
		o.Expect(err).NotTo(o.HaveOccurred())

		operatorsdkCLI.ExecCommandPath = tmpPath
		makeCLI.ExecCommandPath = tmpPath
		defer quayCLI.DeleteTag(strings.Replace(imageTag, "quay.io/", "", 1))

		exutil.By("Create the playbook ansible operator")
		output, err := operatorsdkCLI.Run("init").Args("--plugins=ansible", "--domain=catalogtest.com").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Next"))

		exutil.By("Create api")
		output, err = operatorsdkCLI.Run("create").Args("api", "--group=cache", "--version=v1", "--kind=Catalogtest", "--generate-playbook").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Writing kustomize manifests"))

		err = copy(filepath.Join(dataPath, "Dockerfile"), filepath.Join(tmpPath, "Dockerfile"))
		o.Expect(err).NotTo(o.HaveOccurred())
		replaceContent(filepath.Join(tmpPath, "Dockerfile"), "brew.registry.redhat.io/rh-osbs/openshift-ose-ansible-operator:vocpversion", "brew.registry.redhat.io/rh-osbs/openshift-ose-ansible-operator:v"+ocpversion)

		err = copy(filepath.Join(dataPath, "config", "default", "manager_auth_proxy_patch.yaml"), filepath.Join(tmpPath, "config", "default", "manager_auth_proxy_patch.yaml"))
		o.Expect(err).NotTo(o.HaveOccurred())
		replaceContent(filepath.Join(tmpPath, "config", "default", "manager_auth_proxy_patch.yaml"), "registry.redhat.io/openshift4/ose-kube-rbac-proxy:vocppreversion", "registry.redhat.io/openshift4/ose-kube-rbac-proxy:v"+ocppreversion)

		exutil.By("step: Build and push the operator image")
		tokenDir := "/tmp/ocp-34462" + getRandomString()
		defer os.RemoveAll(tokenDir)
		err = os.MkdirAll(tokenDir, os.ModePerm)
		if err != nil {
			e2e.Failf("fail to create the token folder:%s", tokenDir)
		}
		_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", fmt.Sprintf("--to=%s", tokenDir), "--confirm").Output()
		if err != nil {
			e2e.Failf("Fail to get the cluster auth %v", err)
		}
		buildPushOperatorImage(clusterArchitecture, tmpPath, imageTag, tokenDir)

		// OCP-40219
		exutil.By("Generate the bundle image and catalog index image")
		replaceContent(filepath.Join(tmpPath, "Makefile"), "controller:latest", imageTag)

		manifestsPath := filepath.Join(tmpPath, "config", "manifests", "bases")
		err = os.MkdirAll(manifestsPath, 0o755)
		o.Expect(err).NotTo(o.HaveOccurred())
		manifestsFile := filepath.Join(manifestsPath, "catalogtest.clusterserviceversion.yaml")
		_, err = os.Create(manifestsFile)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = copy(filepath.Join(dataPath, "manifests", "bases", "catalogtest.clusterserviceversion.yaml"), filepath.Join(manifestsFile))
		o.Expect(err).NotTo(o.HaveOccurred())

		waitErr := wait.PollUntilContextTimeout(context.TODO(), 30*time.Second, 120*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, _ := makeCLI.Run("bundle").Args().Output()
			if strings.Contains(msg, "operator-sdk bundle validate ./bundle") {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "operator-sdk bundle generate failed")

		replaceContent(filepath.Join(tmpPath, "Makefile"), " --container-tool docker ", " ")

		defer quayCLI.DeleteTag(strings.Replace(bundleImage, "quay.io/", "", 1))
		defer quayCLI.DeleteTag(strings.Replace(catalogImage, "quay.io/", "", 1))
		_, err = makeCLI.Run("bundle-build").Args("bundle-push", "catalog-build", "catalog-push", "BUNDLE_IMG="+bundleImage, "CATALOG_IMG="+catalogImage).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Install the operator through olm")
		namespace := oc.Namespace()
		createCatalog, _ := oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", catalogofcatalog, "-p", "NAME=cs-catalog", "NAMESPACE="+namespace, "ADDRESS="+catalogImage, "DISPLAYNAME=CatalogTest").OutputToFile("catalogsource-34462.json")
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", createCatalog, "-n", namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		createOg, _ := oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", ogofcatalog, "-p", "NAME=catalogtest-single", "NAMESPACE="+namespace, "KAKA="+namespace).OutputToFile("createog-34462.json")
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", createOg, "-n", namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		createSub, _ := oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", subofcatalog, "-p", "NAME=cataloginstall", "NAMESPACE="+namespace, "SOURCENAME=cs-catalog", "OPERATORNAME=catalogtest", "SOURCENAMESPACE="+namespace, "STARTINGCSV=catalogtest.v0.0.1").OutputToFile("createsub-34462.json")
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", createSub, "-n", namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		waitErr = wait.PollUntilContextTimeout(context.TODO(), 30*time.Second, 390*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", "catalogtest.v0.0.1", "-o=jsonpath={.status.phase}", "-n", namespace).Output()
			if strings.Contains(msg, "Succeeded") {
				e2e.Logf("catalogtest installed successfully")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("can't get csv catalogtest.v0.0.1 in %s", namespace))
	})

	// author: chuo@redhat.com
	g.It("Author:chuo-Medium-27718-scorecard remove version flag", func() {
		operatorsdkCLI.showInfo = true
		output, _ := operatorsdkCLI.Run("scorecard").Args("--version").Output()
		o.Expect(output).To(o.ContainSubstring("unknown flag: --version"))
	})

	// author: chuo@redhat.com
	g.It("VMonly-Author:chuo-Critical-37655-run bundle upgrade connect to the Operator SDK CLI", func() {
		operatorsdkCLI.showInfo = true
		output, _ := operatorsdkCLI.Run("run").Args("bundle-upgrade", "-h").Output()
		o.Expect(output).To(o.ContainSubstring("help for bundle-upgrade"))
	})

	// author: chuo@redhat.com
	g.It("VMonly-Author:chuo-Medium-34945-ansible Add flag metricsaddr for ansible operator", func() {
		operatorsdkCLI.showInfo = true
		result, err := exec.Command("bash", "-c", "ansible-operator run --help").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(result).To(o.ContainSubstring("--metrics-bind-address"))
	})

	// author: chuo@redhat.com
	g.It("Author:xzha-High-52126-Sync 1.28 to downstream", func() {
		operatorsdkCLI.showInfo = true
		output, _ := operatorsdkCLI.Run("version").Args().Output()
		o.Expect(output).To(o.ContainSubstring("v1.28"))
	})

	// author: chuo@redhat.com
	g.It("ConnectedOnly-VMonly-Author:chuo-High-34427-Ensure that Ansible Based Operators creation is working", func() {
		clusterArchitecture := architecture.SkipArchitectures(oc, architecture.MULTI, architecture.PPC64LE, architecture.S390X)

		imageTag := "quay.io/olmqe/memcached-operator-ansible-base:v" + ocpversion + getRandomString()
		// TODO[aleskandro,chuo]: this is a workaround See https://issues.redhat.com/browse/ARMOCP-531
		if clusterArchitecture == architecture.ARM64 {
			imageTag = "quay.io/olmqe/memcached-operator-ansible-base:v" + ocpversion + "-34427"
		}
		nsSystem := "system-ocp34427" + getRandomString()
		nsOperator := "memcached-operator-34427-system-" + getRandomString()

		tmpBasePath := "/tmp/ocp-34427-" + getRandomString()
		tmpPath := filepath.Join(tmpBasePath, "memcached-operator-34427")
		err := os.MkdirAll(tmpPath, 0o755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(tmpBasePath)
		operatorsdkCLI.ExecCommandPath = tmpPath
		makeCLI.ExecCommandPath = tmpPath

		if imageTag != "quay.io/olmqe/memcached-operator-ansible-base:v"+ocpversion+"-34427" {
			quayCLI := container.NewQuayCLI()
			defer quayCLI.DeleteTag(strings.Replace(imageTag, "quay.io/", "", 1))
		}

		defer func() {
			exutil.By("step: undeploy")
			_, err = makeCLI.Run("undeploy").Args().Output()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()

		exutil.By("step: init Ansible Based Operator")
		output, err := operatorsdkCLI.Run("init").Args("--plugins=ansible", "--domain", "example.com").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Next"))

		exutil.By("step: Create API.")
		output, err = operatorsdkCLI.Run("create").Args("api", "--group", "cache", "--version", "v1alpha1", "--kind", "Memcached34427", "--generate-role").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Writing kustomize manifests"))

		if !upstream {
			exutil.By("step: OCP-52625 operatorsdk generate operator base image match the release version.")
			dockerFile := filepath.Join(tmpPath, "Dockerfile")
			content := getContent(dockerFile)
			o.Expect(content).To(o.ContainSubstring("registry.redhat.io/openshift4/ose-ansible-operator:v" + ocpversion))
			replaceContent(dockerFile, "registry.redhat.io/openshift4/ose-ansible-operator:v"+ocpversion, "brew.registry.redhat.io/rh-osbs/openshift-ose-ansible-operator:v"+ocpversion)

			managerAuthProxyPatch := filepath.Join(tmpPath, "config", "default", "manager_auth_proxy_patch.yaml")
			content = getContent(managerAuthProxyPatch)
			o.Expect(content).To(o.ContainSubstring("registry.redhat.io/openshift4/ose-kube-rbac-proxy:v" + ocpversion))
			replaceContent(managerAuthProxyPatch, "registry.redhat.io/openshift4/ose-kube-rbac-proxy:v"+ocpversion, "registry.redhat.io/openshift4/ose-kube-rbac-proxy:v"+ocppreversion)
		}

		buildPruningBaseDir := exutil.FixturePath("testdata", "operatorsdk")
		dataPath := filepath.Join(buildPruningBaseDir, "ocp-34427-data", "roles", "memcached")
		err = copy(filepath.Join(dataPath, "tasks", "main.yml"), filepath.Join(tmpPath, "roles", "memcached34427", "tasks", "main.yml"))
		o.Expect(err).NotTo(o.HaveOccurred())
		err = copy(filepath.Join(dataPath, "defaults", "main.yml"), filepath.Join(tmpPath, "roles", "memcached34427", "defaults", "main.yml"))
		o.Expect(err).NotTo(o.HaveOccurred())
		exec.Command("bash", "-c", fmt.Sprintf("sed -i '$d' %s/config/samples/cache_v1alpha1_memcached34427.yaml", tmpPath)).Output()
		exec.Command("bash", "-c", fmt.Sprintf("sed -i '$a\\  size: 3' %s/config/samples/cache_v1alpha1_memcached34427.yaml", tmpPath)).Output()

		exec.Command("bash", "-c", fmt.Sprintf("sed -i 's/name: system/name: %s/g' `grep -rl \"name: system\" %s`", nsSystem, tmpPath)).Output()
		exec.Command("bash", "-c", fmt.Sprintf("sed -i 's/namespace: system/namespace: %s/g'  `grep -rl \"namespace: system\" %s`", nsSystem, tmpPath)).Output()
		exec.Command("bash", "-c", fmt.Sprintf("sed -i 's/namespace: memcached-operator-34427-system/namespace: %s/g'  `grep -rl \"namespace: memcached-operator-34427-system\" %s`", nsOperator, tmpPath)).Output()

		exutil.By("step: Push the operator image")
		dockerFilePath := filepath.Join(tmpPath, "Dockerfile")
		replaceContent(dockerFilePath, "RUN ansible-galaxy collection install -r ${HOME}/requirements.yml", "RUN ansible-galaxy collection install -r ${HOME}/requirements.yml --force")
		tokenDir := "/tmp/ocp-34427-auth" + getRandomString()
		err = os.MkdirAll(tokenDir, os.ModePerm)
		defer os.RemoveAll(tokenDir)
		if err != nil {
			e2e.Failf("fail to create the token folder:%s", tokenDir)
		}
		_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", fmt.Sprintf("--to=%s", tokenDir), "--confirm").Output()
		if err != nil {
			e2e.Failf("Fail to get the cluster auth %v", err)
		}
		// TODO[aleskandro,chuo]: this is a workaround: https://issues.redhat.com/browse/ARMOCP-531
		architecture.SkipArchitectures(oc, architecture.MULTI, architecture.PPC64LE, architecture.S390X)
		switch clusterArchitecture {
		case architecture.AMD64:
			buildPushOperatorImage(clusterArchitecture, tmpPath, imageTag, tokenDir)
		case architecture.ARM64:
			e2e.Logf(fmt.Sprintf("platform is %s, IMG is %s", clusterArchitecture.String(), imageTag))
		}

		exutil.By("step: Install the CRD")
		output, err = makeCLI.Run("install").Args().Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("memcached34427s.cache.example.com created"))
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("crd", "memcached34427s.cache.example.com").Output()
		e2e.Logf(output)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).NotTo(o.ContainSubstring("NotFound"))

		exutil.By("step: Deploy the operator")
		output, err = makeCLI.Run("deploy").Args("IMG=" + imageTag).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("deployment.apps/memcached-operator-34427-controller-manager"))

		waitErr := wait.PollUntilContextTimeout(context.TODO(), 30*time.Second, 180*time.Second, false, func(ctx context.Context) (bool, error) {
			podList, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", nsOperator).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			lines := strings.Split(podList, "\n")
			for _, line := range lines {
				if strings.Contains(line, "memcached-operator-34427-controller-manager") {
					e2e.Logf("found pod memcached-operator-34427-controller-manager")
					if strings.Contains(line, "Running") {
						e2e.Logf("the status of pod memcached-operator-34427-controller-manager is Running")
						return true, nil
					}
					e2e.Logf("the status of pod memcached-operator-34427-controller-manager is not Running")
					return false, nil
				}
			}
			return false, nil
		})
		if waitErr != nil {
			logDebugInfo(oc, nsOperator, "events", "pod")
		}
		exutil.AssertWaitPollNoErr(waitErr, "No memcached-operator-34427-controller-manager")

		exutil.By("step: Create the resource")
		filePath := filepath.Join(tmpPath, "config", "samples", "cache_v1alpha1_memcached34427.yaml")
		_, err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", filePath, "-n", nsOperator).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		waitErr = wait.PollUntilContextTimeout(context.TODO(), 30*time.Second, 180*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", nsOperator).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(msg, "memcached34427-sample") {
				e2e.Logf("found pod memcached34427-sample")
				return true, nil
			}
			return false, nil
		})
		if waitErr != nil {
			logDebugInfo(oc, nsOperator, "events", "pod")
		}
		exutil.AssertWaitPollNoErr(waitErr, "No pod memcached34427-sample")
		waitErr = wait.PollUntilContextTimeout(context.TODO(), 30*time.Second, 180*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, err := oc.AsAdmin().WithoutNamespace().Run("describe").Args("deployment/memcached34427-sample-memcached", "-n", nsOperator).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(msg, "3 desired | 3 updated | 3 total | 3 available | 0 unavailable") {
				e2e.Logf("deployment/memcached34427-sample-memcached is created successfully")
				return true, nil
			}
			return false, nil
		})
		if waitErr != nil {
			msg, err := oc.AsAdmin().WithoutNamespace().Run("describe").Args("deployment/memcached34427-sample-memcached", "-n", nsOperator).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf(msg)
			msg, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args("events", "-n", nsOperator).Output()
			e2e.Logf(msg)
		}
		exutil.AssertWaitPollNoErr(waitErr, "the status of deployment/memcached34427-sample-memcached is wrong")

		exutil.By("34427 SUCCESS")
	})

	// author: chuo@redhat.com
	g.It("ConnectedOnly-VMonly-Author:chuo-Medium-34366-change ansible operator flags from maxWorkers using env MAXCONCURRENTRECONCILES ", func() {
		clusterArchitecture := architecture.SkipArchitectures(oc, architecture.MULTI, architecture.PPC64LE, architecture.S390X)
		imageTag := "quay.io/olmqe/memcached-operator-max-worker:v" + ocpversion + getRandomString()
		// TODO[aleskandro,chuo]: this is a workaround: https://issues.redhat.com/browse/ARMOCP-531
		if clusterArchitecture == architecture.ARM64 {
			imageTag = "quay.io/olmqe/memcached-operator-max-worker:v" + ocpversion + "-34366"
		}
		nsSystem := "system-ocp34366" + getRandomString()
		nsOperator := "memcached-operator-34366-system-" + getRandomString()

		tmpBasePath := "/tmp/ocp-34366-" + getRandomString()
		tmpPath := filepath.Join(tmpBasePath, "memcached-operator-34366")
		err := os.MkdirAll(tmpPath, 0o755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(tmpBasePath)
		operatorsdkCLI.ExecCommandPath = tmpPath
		makeCLI.ExecCommandPath = tmpPath

		if imageTag != "quay.io/olmqe/memcached-operator-max-worker:v"+ocpversion+"-34366" {
			quayCLI := container.NewQuayCLI()
			defer quayCLI.DeleteTag(strings.Replace(imageTag, "quay.io/", "", 1))
		}

		defer func() {
			exutil.By("step: undeploy")
			_, err = makeCLI.Run("undeploy").Args().Output()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()

		exutil.By("step: init Ansible Based Operator")
		output, err := operatorsdkCLI.Run("init").Args("--plugins=ansible", "--domain", "example.com").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Next"))

		if !upstream {
			exutil.By("step: modify Dockerfile.")
			dockerFile := filepath.Join(tmpPath, "Dockerfile")
			content := getContent(dockerFile)
			o.Expect(content).To(o.ContainSubstring("registry.redhat.io/openshift4/ose-ansible-operator:v" + ocpversion))
			replaceContent(dockerFile, "registry.redhat.io/openshift4/ose-ansible-operator:v"+ocpversion, "brew.registry.redhat.io/rh-osbs/openshift-ose-ansible-operator:v"+ocpversion)

			managerAuthProxyPatch := filepath.Join(tmpPath, "config", "default", "manager_auth_proxy_patch.yaml")
			content = getContent(managerAuthProxyPatch)
			o.Expect(content).To(o.ContainSubstring("registry.redhat.io/openshift4/ose-kube-rbac-proxy:v" + ocpversion))
			replaceContent(managerAuthProxyPatch, "registry.redhat.io/openshift4/ose-kube-rbac-proxy:v"+ocpversion, "registry.redhat.io/openshift4/ose-kube-rbac-proxy:v"+ocppreversion)
		}

		exutil.By("step: Create API.")
		output, err = operatorsdkCLI.Run("create").Args("api", "--group", "cache", "--version", "v1alpha1", "--kind", "Memcached34366", "--generate-role").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Writing kustomize manifests"))

		buildPruningBaseDir := exutil.FixturePath("testdata", "operatorsdk")
		dataPath := filepath.Join(buildPruningBaseDir, "ocp-34366-data")
		err = copy(filepath.Join(dataPath, "roles", "memcached", "tasks", "main.yml"), filepath.Join(tmpPath, "roles", "memcached34366", "tasks", "main.yml"))
		o.Expect(err).NotTo(o.HaveOccurred())
		err = copy(filepath.Join(dataPath, "config", "manager", "manager.yaml"), filepath.Join(tmpPath, "config", "manager", "manager.yaml"))
		o.Expect(err).NotTo(o.HaveOccurred())

		exec.Command("bash", "-c", fmt.Sprintf("sed -i 's/name: system/name: %s/g' `grep -rl \"name: system\" %s`", nsSystem, tmpPath)).Output()
		exec.Command("bash", "-c", fmt.Sprintf("sed -i 's/namespace: system/namespace: %s/g'  `grep -rl \"namespace: system\" %s`", nsSystem, tmpPath)).Output()
		exec.Command("bash", "-c", fmt.Sprintf("sed -i 's/namespace: memcached-operator-34366-system/namespace: %s/g'  `grep -rl \"namespace: memcached-operator-34366-system\" %s`", nsOperator, tmpPath)).Output()

		exutil.By("step: build and push img.")
		dockerFilePath := filepath.Join(tmpPath, "Dockerfile")
		replaceContent(dockerFilePath, "RUN ansible-galaxy collection install -r ${HOME}/requirements.yml", "RUN ansible-galaxy collection install -r ${HOME}/requirements.yml --force")
		tokenDir := "/tmp/ocp-34366-auth" + getRandomString()
		err = os.MkdirAll(tokenDir, os.ModePerm)
		defer os.RemoveAll(tokenDir)
		if err != nil {
			e2e.Failf("fail to create the token folder:%s", tokenDir)
		}
		_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", fmt.Sprintf("--to=%s", tokenDir), "--confirm").Output()
		if err != nil {
			e2e.Failf("Fail to get the cluster auth %v", err)
		}
		// TODO[aleskandro,chuo]: this is a workaround: https://issues.redhat.com/browse/ARMOCP-531
		architecture.SkipArchitectures(oc, architecture.MULTI, architecture.PPC64LE, architecture.S390X)
		switch clusterArchitecture {
		case architecture.AMD64:
			buildPushOperatorImage(clusterArchitecture, tmpPath, imageTag, tokenDir)
		case architecture.ARM64:
			e2e.Logf(fmt.Sprintf("platform is %s, IMG is %s", clusterArchitecture.String(), imageTag))
		}

		exutil.By("step: Install the CRD")
		output, err = makeCLI.Run("install").Args().Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("memcached34366s.cache.example.com created"))
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("crd", "memcached34366s.cache.example.com").Output()
		e2e.Logf(output)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).NotTo(o.ContainSubstring("NotFound"))

		exutil.By("step: Deploy the operator")
		output, err = makeCLI.Run("deploy").Args("IMG=" + imageTag).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("deployment.apps/memcached-operator-34366-controller-manager"))
		_, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "add-cluster-role-to-user", "cluster-admin", fmt.Sprintf("system:serviceaccount:%s:default", nsOperator)).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		waitErr := wait.PollUntilContextTimeout(context.TODO(), 30*time.Second, 300*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", nsOperator).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(msg, "Running") {
				return true, nil
			}
			return false, nil
		})
		if waitErr != nil {
			logDebugInfo(oc, nsOperator, "events", "pod")
		}
		exutil.AssertWaitPollNoErr(waitErr, "memcached-operator-34366-controller-manager has no Starting workers")

		output, err = oc.AsAdmin().WithoutNamespace().Run("logs").Args("deployment.apps/memcached-operator-34366-controller-manager", "-c", "manager", "-n", nsOperator).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("\"worker count\":6"))
	})

	// author: jitli@redhat.com
	g.It("VMonly-ConnectedOnly-Author:jitli-Medium-34883-SDK stamp on Operator bundle image [Slow]", func() {

		operatorsdkCLI.showInfo = true
		tmpBasePath := "/tmp/ocp-34883-" + getRandomString()
		tmpPath := filepath.Join(tmpBasePath, "memcached-operator-34883")
		err := os.MkdirAll(tmpPath, 0o755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(tmpBasePath)
		operatorsdkCLI.ExecCommandPath = tmpPath
		makeCLI.ExecCommandPath = tmpPath
		buildPruningBaseDir := exutil.FixturePath("testdata", "operatorsdk")
		dataPath := filepath.Join(buildPruningBaseDir, "ocp-34883-data")

		exutil.By("Step: init Ansible Based Operator")
		_, err = operatorsdkCLI.Run("init").Args("--plugins=ansible", "--domain", "example.com").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Step: Create API.")
		_, err = operatorsdkCLI.Run("create").Args("api", "--group", "cache", "--version", "v1alpha1", "--kind", "Memcached34883", "--generate-role").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Step: make bundle.")
		manifestsPath := filepath.Join(tmpPath, "config", "manifests", "bases")
		err = os.MkdirAll(manifestsPath, 0o755)
		o.Expect(err).NotTo(o.HaveOccurred())
		manifestsFile := filepath.Join(manifestsPath, "memcached-operator-34883.clusterserviceversion.yaml")
		_, err = os.Create(manifestsFile)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = copy(filepath.Join(dataPath, "manifests", "bases", "memcached-operator-34883.clusterserviceversion.yaml"), filepath.Join(manifestsFile))
		o.Expect(err).NotTo(o.HaveOccurred())

		waitErr := wait.PollUntilContextTimeout(context.TODO(), 30*time.Second, 120*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, _ := makeCLI.Run("bundle").Args().Output()
			if strings.Contains(msg, "operator-sdk bundle validate ./bundle") {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "operator-sdk bundle generate failed")

		exutil.By("Step: check annotations")
		annotationsFile := filepath.Join(tmpPath, "bundle", "metadata", "annotations.yaml")
		content := getContent(annotationsFile)
		o.Expect(content).To(o.ContainSubstring("operators.operatorframework.io.metrics.builder: operator-sdk"))

	})

	// author: chuo@redhat.com
	g.It("VMonly-ConnectedOnly-Author:chuo-Critical-45431-Critical-45428-Medium-43973-Medium-43976-Medium-48630-scorecard basic test migration and migrate olm tests and proxy configurable and xunit adjustment and sa ", func() {
		operatorsdkCLI.showInfo = true
		oc.SetupProject()
		tmpBasePath := "/tmp/ocp-45431-" + getRandomString()
		tmpPath := filepath.Join(tmpBasePath, "memcached-operator")
		err := os.MkdirAll(tmpPath, 0o755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(tmpBasePath)
		operatorsdkCLI.ExecCommandPath = tmpPath
		makeCLI.ExecCommandPath = tmpPath

		exutil.By("step: init Ansible Based Operator")
		_, err = operatorsdkCLI.Run("init").Args("--plugins=ansible", "--domain", "example.com").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("step: Create API.")
		_, err = operatorsdkCLI.Run("create").Args("api", "--group", "cache", "--version", "v1alpha1", "--kind", "Memcached", "--generate-role").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		buildPruningBaseDir := exutil.FixturePath("testdata", "operatorsdk")
		dataPath := filepath.Join(buildPruningBaseDir, "ocp-43973-data")
		manifestsPath := filepath.Join(tmpPath, "config", "manifests", "bases")
		err = os.MkdirAll(manifestsPath, 0o755)
		o.Expect(err).NotTo(o.HaveOccurred())
		manifestsFile := filepath.Join(manifestsPath, "memcached-operator.clusterserviceversion.yaml")
		_, err = os.Create(manifestsFile)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = copy(filepath.Join(dataPath, "manifests", "bases", "memcached-operator.clusterserviceversion.yaml"), filepath.Join(manifestsFile))
		o.Expect(err).NotTo(o.HaveOccurred())

		waitErr := wait.PollUntilContextTimeout(context.TODO(), 30*time.Second, 120*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, err := makeCLI.Run("bundle").Args().Output()
			if err != nil {
				e2e.Logf("make bundle failed, try again")
				return false, nil
			}
			if strings.Contains(msg, "operator-sdk bundle validate ./bundle") {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "operator-sdk bundle generate failed")

		output, _ := operatorsdkCLI.Run("version").Args("").Output()
		e2e.Logf("The OperatorSDK version is %s", output)

		// OCP-43973
		exutil.By("scorecard basic test migration")
		output, err = operatorsdkCLI.Run("scorecard").Args("./bundle", "-c", "./bundle/tests/scorecard/config.yaml", "-w", "100s", "--selector=test=basic-check-spec-test", "-n", oc.Namespace(), "--pod-security=restricted").Output()
		e2e.Logf(" scorecard bundle %v", err)
		o.Expect(output).To(o.ContainSubstring("State: pass"))
		o.Expect(output).To(o.ContainSubstring("spec missing from [memcached-sample]"))

		// OCP-43976
		exutil.By("migrate OLM tests-bundle validation")
		output, err = operatorsdkCLI.Run("scorecard").Args("./bundle", "-c", "./bundle/tests/scorecard/config.yaml", "-w", "100s", "--selector=test=olm-bundle-validation-test", "-n", oc.Namespace(), "--pod-security=restricted").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("State: pass"))

		exutil.By("migrate OLM tests-crds have validation test")
		output, err = operatorsdkCLI.Run("scorecard").Args("./bundle", "-c", "./bundle/tests/scorecard/config.yaml", "-w", "100s", "--selector=test=olm-crds-have-validation-test", "-n", oc.Namespace(), "--pod-security=restricted").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("State: pass"))

		exutil.By("migrate OLM tests-crds have resources test")
		output, err = operatorsdkCLI.Run("scorecard").Args("./bundle", "-c", "./bundle/tests/scorecard/config.yaml", "-w", "100s", "--selector=test=olm-crds-have-resources-test", "-n", oc.Namespace(), "--pod-security=restricted").Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("State: fail"))
		o.Expect(output).To(o.ContainSubstring("Owned CRDs do not have resources specified"))

		exutil.By("migrate OLM tests- spec descriptors test")
		output, err = operatorsdkCLI.Run("scorecard").Args("./bundle", "-c", "./bundle/tests/scorecard/config.yaml", "-w", "100s", "--selector=test=olm-spec-descriptors-test", "-n", oc.Namespace(), "--pod-security=restricted").Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("State: fail"))

		exutil.By("migrate OLM tests- status descriptors test")
		output, err = operatorsdkCLI.Run("scorecard").Args("./bundle", "-c", "./bundle/tests/scorecard/config.yaml", "-w", "100s", "--selector=test=olm-status-descriptors-test", "-n", oc.Namespace(), "--pod-security=restricted").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("State: pass"))
		o.Expect(output).To(o.ContainSubstring("memcacheds.cache.example.com does not have status spec"))

		// OCP-48630
		exutil.By("scorecard proxy container port should be configurable")
		exec.Command("bash", "-c", fmt.Sprintf("sed -i '$a\\proxy-port: 9001' %s/bundle/tests/scorecard/config.yaml", tmpPath)).Output()
		output, err = operatorsdkCLI.Run("scorecard").Args("./bundle", "-c", "./bundle/tests/scorecard/config.yaml", "-w", "100s", "--selector=test=olm-bundle-validation-test", "-n", oc.Namespace(), "--pod-security=restricted").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("State: pass"))

		// OCP-45428 xunit adjustments - add nested tags and attributes
		exutil.By("migrate OLM tests-bundle validation to generate a pass xunit output")
		output, err = operatorsdkCLI.Run("scorecard").Args("./bundle", "-c", "./bundle/tests/scorecard/config.yaml", "-w", "100s", "--selector=test=olm-bundle-validation-test", "-o", "xunit", "-n", oc.Namespace(), "--pod-security=restricted").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("<testsuite name=\"olm-bundle-validation-test\""))
		o.Expect(output).To(o.ContainSubstring("<testcase name=\"olm-bundle-validation\""))

		exutil.By("migrate OLM tests-status descriptors to generate a failed xunit output")
		output, err = operatorsdkCLI.Run("scorecard").Args("./bundle", "-c", "./bundle/tests/scorecard/config.yaml", "-w", "100s", "--selector=test=olm-status-descriptors-test", "-o", "xunit", "-n", oc.Namespace(), "--pod-security=restricted").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("<testcase name=\"olm-status-descriptors\""))
		o.Expect(output).To(o.ContainSubstring("<system-out>Loaded ClusterServiceVersion:"))
		o.Expect(output).To(o.ContainSubstring("failure"))

		// OCP-45431 bring in latest o-f/api to SDK BEFORE 1.13
		exutil.By("use an non-exist service account to run test")
		output, err = operatorsdkCLI.Run("scorecard").Args("./bundle", "-c", "./bundle/tests/scorecard/config.yaml", "-w", "100s", "--selector=test=olm-bundle-validation-test ", "-s", "testing", "-n", oc.Namespace(), "--pod-security=restricted").Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("serviceaccount \"testing\" not found"))
		_, err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", "test/extended/testdata/operatorsdk/ocp-43973-data/sa_testing.yaml", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = operatorsdkCLI.Run("scorecard").Args("./bundle", "-c", "./bundle/tests/scorecard/config.yaml", "-w", "100s", "--selector=test=olm-bundle-validation-test ", "-s", "testing", "-n", oc.Namespace(), "--pod-security=restricted").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

	})

	// author: chuo@redhat.com
	g.It("VMonly-ConnectedOnly-Author:chuo-High-43660-scorecard support storing test output", func() {
		operatorsdkCLI.showInfo = true
		oc.SetupProject()
		tmpBasePath := "/tmp/ocp-43660-" + getRandomString()
		tmpPath := filepath.Join(tmpBasePath, "memcached-operator-43660")
		err := os.MkdirAll(tmpPath, 0o755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(tmpBasePath)
		operatorsdkCLI.ExecCommandPath = tmpPath
		makeCLI.ExecCommandPath = tmpPath
		buildPruningBaseDir := exutil.FixturePath("testdata", "operatorsdk")
		dataPath := filepath.Join(buildPruningBaseDir, "ocp-43660-data")

		exutil.By("step: init Ansible Based Operator")
		_, err = operatorsdkCLI.Run("init").Args("--plugins=ansible", "--domain", "example.com").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("step: Create API.")
		_, err = operatorsdkCLI.Run("create").Args("api", "--group", "cache", "--version", "v1alpha1", "--kind", "Memcached43660", "--generate-role").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("step: make bundle.")
		manifestsPath := filepath.Join(tmpPath, "config", "manifests", "bases")
		err = os.MkdirAll(manifestsPath, 0o755)
		o.Expect(err).NotTo(o.HaveOccurred())
		manifestsFile := filepath.Join(manifestsPath, "memcached-operator-43660.clusterserviceversion.yaml")
		_, err = os.Create(manifestsFile)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = copy(filepath.Join(dataPath, "manifests", "bases", "memcached-operator-43660.clusterserviceversion.yaml"), filepath.Join(manifestsFile))
		o.Expect(err).NotTo(o.HaveOccurred())

		waitErr := wait.PollUntilContextTimeout(context.TODO(), 30*time.Second, 120*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, err := makeCLI.Run("bundle").Args().Output()
			if err != nil {
				e2e.Logf("make bundle failed, try again")
				return false, nil
			}
			if strings.Contains(msg, "operator-sdk bundle validate ./bundle") {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "operator-sdk bundle generate failed")

		exutil.By("run scorecard ")
		output, err := operatorsdkCLI.Run("scorecard").Args("./bundle", "-c", "./bundle/tests/scorecard/config.yaml", "-w", "4m", "--selector=test=olm-bundle-validation-test", "--test-output", "/testdata", "-n", oc.Namespace(), "--pod-security=restricted").Output()
		e2e.Logf(" scorecard bundle %v", err)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("State: pass"))
		o.Expect(output).To(o.ContainSubstring("Name: olm-bundle-validation"))

		exutil.By("step: modify test config.")
		configFilePath := filepath.Join(tmpPath, "bundle", "tests", "scorecard", "config.yaml")
		replaceContent(configFilePath, "mountPath: {}", "mountPath:\n          path: /test-output")

		exutil.By("scorecard basic test migration")
		output, err = operatorsdkCLI.Run("scorecard").Args("./bundle", "-c", "./bundle/tests/scorecard/config.yaml", "-w", "60s", "--selector=test=basic-check-spec-test", "-n", oc.Namespace(), "--pod-security=restricted").Output()
		e2e.Logf(" scorecard bundle %v", err)
		o.Expect(output).To(o.ContainSubstring("State: pass"))
		o.Expect(output).To(o.ContainSubstring("spec missing from [memcached43660-sample]"))
		pathOutput := filepath.Join(tmpPath, "test-output", "basic", "basic-check-spec-test")
		waitErr = wait.PollUntilContextTimeout(context.TODO(), 2*time.Second, 6*time.Second, false, func(ctx context.Context) (bool, error) {
			if _, err := os.Stat(pathOutput); os.IsNotExist(err) {
				e2e.Logf("get basic-check-spec-test Failed")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "get basic-check-spec-test Failed")

		output, err = operatorsdkCLI.Run("scorecard").Args("./bundle", "-c", "./bundle/tests/scorecard/config.yaml", "-w", "60s", "--selector=test=olm-bundle-validation-test", "-n", oc.Namespace(), "--pod-security=restricted").Output()
		e2e.Logf(" scorecard bundle %v", err)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("State: pass"))
		pathOutput = filepath.Join(tmpPath, "test-output", "olm", "olm-bundle-validation-test")
		waitErr = wait.PollUntilContextTimeout(context.TODO(), 2*time.Second, 6*time.Second, false, func(ctx context.Context) (bool, error) {
			if _, err := os.Stat(pathOutput); os.IsNotExist(err) {
				e2e.Logf("get olm-bundle-validation-test Failed")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "get olm-bundle-validation-test Failed")

	})

	// author: chuo@redhat.com
	g.It("ConnectedOnly-Author:chuo-High-31219-scorecard bundle is mandatory ", func() {
		operatorsdkCLI.showInfo = true
		oc.SetupProject()
		tmpBasePath := "/tmp/ocp-31219-" + getRandomString()
		tmpPath := filepath.Join(tmpBasePath, "memcached-operator-31219")
		err := os.MkdirAll(tmpPath, 0o755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(tmpBasePath)
		operatorsdkCLI.ExecCommandPath = tmpPath
		makeCLI.ExecCommandPath = tmpPath

		exutil.By("Step: init Ansible Based Operator")
		_, err = operatorsdkCLI.Run("init").Args("--plugins", "ansible", "--domain", "example.com").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Step: Create API.")
		output, err := operatorsdkCLI.Run("create").Args("api", "--group", "cache", "--version", "v1alpha1", "--kind", "Memcached", "--generate-role").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Writing kustomize manifests"))

		result, err := exec.Command("bash", "-c", "cd "+tmpPath+" && operator-sdk generate bundle --deploy-dir=config --crds-dir=config/crds --version=0.0.1").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(result).To(o.ContainSubstring("Bundle manifests generated successfully in bundle"))

		output, err = operatorsdkCLI.Run("scorecard").Args("bundle", "-c", "config/scorecard/bases/config.yaml", "-w", "60s", "--selector=test=basic-check-spec-test", "-n", oc.Namespace(), "--pod-security=restricted").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("tests selected"))

	})

	// author: chuo@redhat.com
	g.It("ConnectedOnly-VMonly-Author:chuo-High-34426-Critical-52625-Medium-37142-ensure that Helm Based Operators creation and cr create delete ", func() {
		clusterArchitecture := architecture.SkipArchitectures(oc, architecture.MULTI, architecture.PPC64LE, architecture.S390X)
		imageTag := "quay.io/olmqe/nginx-operator-base:v" + ocpversion + "-34426" + getRandomString()
		nsSystem := "system-34426-" + getRandomString()
		nsOperator := "nginx-operator-34426-system"

		tmpBasePath := "/tmp/ocp-34426-" + getRandomString()
		tmpPath := filepath.Join(tmpBasePath, "nginx-operator-34426")
		err := os.MkdirAll(tmpPath, 0o755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(tmpBasePath)
		operatorsdkCLI.ExecCommandPath = tmpPath
		makeCLI.ExecCommandPath = tmpPath

		defer func() {
			quayCLI := container.NewQuayCLI()
			quayCLI.DeleteTag(strings.Replace(imageTag, "quay.io/", "", 1))
		}()

		defer func() {
			exutil.By("delete nginx-sample")
			filePath := filepath.Join(tmpPath, "config", "samples", "demo_v1_nginx34426.yaml")
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", filePath, "-n", nsOperator).Output()
			waitErr := wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 30*time.Second, false, func(ctx context.Context) (bool, error) {
				output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("Nginx34426", "-n", nsOperator).Output()
				if strings.Contains(output, "nginx34426-sample") {
					e2e.Logf("nginx34426-sample still exists")
					return false, nil
				}
				return true, nil
			})
			if waitErr != nil {
				e2e.Logf("delete nginx-sample failed, still try to run make undeploy")
			}
			exutil.By("step: run make undeploy")
			_, err = makeCLI.Run("undeploy").Args().Output()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		exutil.By("step: init Helm Based Operators")
		output, err := operatorsdkCLI.Run("init").Args("--plugins=helm").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Next: define a resource with"))

		exutil.By("step: Create API.")
		output, err = operatorsdkCLI.Run("create").Args("api", "--group", "demo", "--version", "v1", "--kind", "Nginx34426").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("nginx"))

		if !upstream {
			exutil.By("step: OCP-52625 operatorsdk generate operator base image match the release version	.")
			dockerFile := filepath.Join(tmpPath, "Dockerfile")
			content := getContent(dockerFile)
			o.Expect(content).To(o.ContainSubstring("registry.redhat.io/openshift4/ose-helm-operator:v" + ocpversion))
			replaceContent(dockerFile, "registry.redhat.io/openshift4/ose-helm-operator:v"+ocpversion, "brew.registry.redhat.io/rh-osbs/openshift-ose-helm-operator:v"+ocpversion)

			managerAuthProxyPatch := filepath.Join(tmpPath, "config", "default", "manager_auth_proxy_patch.yaml")
			content = getContent(managerAuthProxyPatch)
			o.Expect(content).To(o.ContainSubstring("registry.redhat.io/openshift4/ose-kube-rbac-proxy:v" + ocpversion))
			replaceContent(managerAuthProxyPatch, "registry.redhat.io/openshift4/ose-kube-rbac-proxy:v"+ocpversion, "registry.redhat.io/openshift4/ose-kube-rbac-proxy:v"+ocppreversion)
		}

		exutil.By("step: modify namespace")
		exec.Command("bash", "-c", fmt.Sprintf("sed -i 's/name: system/name: %s/g' `grep -rl \"name: system\" %s`", nsSystem, tmpPath)).Output()
		exec.Command("bash", "-c", fmt.Sprintf("sed -i 's/namespace: system/namespace: %s/g'  `grep -rl \"namespace: system\" %s`", nsSystem, tmpPath)).Output()
		exec.Command("bash", "-c", fmt.Sprintf("sed -i 's/namespace: nginx-operator-34426-system/namespace: %s/g'  `grep -rl \"namespace: nginx-operator-system\" %s`", nsOperator, tmpPath)).Output()

		exutil.By("step: build and Push the operator image")
		tokenDir := "/tmp/ocp-34426" + getRandomString()
		err = os.MkdirAll(tokenDir, os.ModePerm)
		defer os.RemoveAll(tokenDir)
		if err != nil {
			e2e.Failf("fail to create the token folder:%s", tokenDir)
		}
		_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", fmt.Sprintf("--to=%s", tokenDir), "--confirm").Output()
		if err != nil {
			e2e.Failf("Fail to get the cluster auth %v", err)
		}
		buildPushOperatorImage(clusterArchitecture, tmpPath, imageTag, tokenDir)

		exutil.By("step: Install the CRD")
		output, err = makeCLI.Run("install").Args().Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("nginx34426s.demo.my.domain"))
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("crd", "nginx34426s.demo.my.domain").Output()
		e2e.Logf(output)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).NotTo(o.ContainSubstring("NotFound"))

		exutil.By("step: Deploy the operator")
		output, err = makeCLI.Run("deploy").Args("IMG=" + imageTag).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("deployment.apps/nginx-operator-34426-controller-manager created"))
		waitErr := wait.PollUntilContextTimeout(context.TODO(), 30*time.Second, 180*time.Second, false, func(ctx context.Context) (bool, error) {
			podList, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", nsOperator).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			lines := strings.Split(podList, "\n")
			for _, line := range lines {
				if strings.Contains(line, "nginx-operator-34426-controller-manager") {
					e2e.Logf("found pod nginx-operator-34426-controller-manager")
					if strings.Contains(line, "Running") {
						e2e.Logf("the status of pod nginx-operator-34426-controller-manager is Running")
						return true, nil
					}
					e2e.Logf("the status of pod nginx-operator-34426-controller-manager is not Running")
					return false, nil
				}
			}
			return false, nil
		})
		if err != nil {
			logDebugInfo(oc, nsOperator, "events", "pod")
		}
		exutil.AssertWaitPollNoErr(waitErr, "No nginx-operator-34426-controller-manager")

		_, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "add-scc-to-user", "anyuid", fmt.Sprintf("system:serviceaccount:%s:nginx34426-sample", nsOperator)).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		// OCP-37142
		exutil.By("step: Create the resource")
		filePath := filepath.Join(tmpPath, "config", "samples", "demo_v1_nginx34426.yaml")
		replaceContent(filePath, "repository: nginx", "repository: quay.io/olmqe/nginx-docker")
		replaceContent(filePath, "tag: \"\"", "tag: multi-arch")

		_, err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", filePath, "-n", nsOperator).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		waitErr = wait.PollUntilContextTimeout(context.TODO(), 30*time.Second, 180*time.Second, false, func(ctx context.Context) (bool, error) {
			podList, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", nsOperator).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if !strings.Contains(podList, "nginx34426-sample") {
				e2e.Logf("No nginx34426-sample")
				return false, nil
			}
			lines := strings.Split(podList, "\n")
			for _, line := range lines {
				if strings.Contains(line, "nginx34426-sample") {
					e2e.Logf("found pod nginx34426-sample")
					if strings.Contains(line, "Running") {
						e2e.Logf("the status of pod nginx34426-sample is Running")
						return true, nil
					}
					e2e.Logf("the status of pod nginx34426-sample is not Running")
					return false, nil
				}
			}
			return false, nil
		})
		if waitErr != nil {
			logDebugInfo(oc, nsOperator, "events", "pod")
		}
		exutil.AssertWaitPollNoErr(waitErr, "No nginx34426-sample is in Running status")
	})

	// author: xzha@redhat.com
	g.It("VMonly-ConnectedOnly-Author:xzha-Critical-38101-implement IndexImageCatalogCreator [Serial]", func() {
		skipOnProxyCluster(oc)
		operatorsdkCLI.showInfo = true
		exutil.By("0) check the cluster proxy configuration")
		httpProxy, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("proxy", "cluster", "-o=jsonpath={.status.httpProxy}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if httpProxy != "" {
			g.Skip("Skip for cluster with proxy")
		}

		exutil.By("step: create new project")
		oc.SetupProject()
		ns := oc.Namespace()

		exutil.By("step: run bundle 1")
		output, err := operatorsdkCLI.Run("run").Args("bundle", "quay.io/olmqe/kubeturbo-bundle:v8.4.0", "-n", ns, "--timeout", "5m", "--security-context-config=restricted").Output()
		if err != nil {
			logDebugInfo(oc, ns, "csv", "pod", "ip")
		}
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("OLM has successfully installed"))

		exutil.By("step: check catsrc annotations")
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("catalogsource", "kubeturbo-catalog", "-n", oc.Namespace(), "-o=jsonpath={.metadata.annotations}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("index-image"))
		o.Expect(output).To(o.ContainSubstring("injected-bundles"))
		o.Expect(output).To(o.ContainSubstring("registry-pod-name"))
		o.Expect(output).To(o.ContainSubstring("quay.io/olmqe/kubeturbo-bundle:v8.4.0"))
		o.Expect(output).NotTo(o.ContainSubstring("quay.io/olmqe/kubeturbo-bundle:v8.5.0"))

		exutil.By("step: check catsrc address")
		podname1, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("catalogsource", "kubeturbo-catalog", "-n", oc.Namespace(), "-o=jsonpath={.metadata.annotations.operators\\.operatorframework\\.io/registry-pod-name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(podname1).NotTo(o.BeEmpty())

		ip, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", podname1, "-n", oc.Namespace(), "-o=jsonpath={.status.podIP}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(ip).NotTo(o.BeEmpty())

		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("catalogsource", "kubeturbo-catalog", "-n", oc.Namespace(), "-o=jsonpath={.spec.address}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.Equal(ip + ":50051"))

		exutil.By("step: check catsrc sourceType")
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("catalogsource", "kubeturbo-catalog", "-n", oc.Namespace(), "-o=jsonpath={.spec.sourceType}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("grpc"))

		exutil.By("step: check packagemanifest")
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("packagemanifest", "kubeturbo", "-n", oc.Namespace(), "-o=jsonpath={.status.channels[*].currentCSV}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("kubeturbo-operator.v8.4.0"))

		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("packagemanifest", "kubeturbo", "-n", oc.Namespace(), "-o=jsonpath={.status.channels[*].name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("operator-sdk-run-bundle"))

		exutil.By("step: upgrade bundle")
		output, err = operatorsdkCLI.Run("run").Args("bundle-upgrade", "quay.io/olmqe/kubeturbo-bundle:v8.5.0", "-n", ns, "--timeout", "5m", "--security-context-config=restricted").Output()
		if err != nil {
			logDebugInfo(oc, ns, "csv", "pod", "ip")
		}
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Successfully upgraded to"))
		waitErr := wait.PollUntilContextTimeout(context.TODO(), 15*time.Second, 360*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", "kubeturbo-operator.v8.5.0", "-n", ns).Output()
			if strings.Contains(msg, "Succeeded") {
				e2e.Logf("upgrade to 8.5.0 success")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("upgradeoperator upgrade failed in %s ", ns))

		exutil.By("step: check catsrc annotations")
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("catalogsource", "kubeturbo-catalog", "-n", oc.Namespace(), "-o=jsonpath={.metadata.annotations}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("index-image"))
		o.Expect(output).To(o.ContainSubstring("injected-bundles"))
		o.Expect(output).To(o.ContainSubstring("registry-pod-name"))
		o.Expect(output).To(o.ContainSubstring("quay.io/olmqe/kubeturbo-bundle:v8.4.0"))
		o.Expect(output).To(o.ContainSubstring("quay.io/olmqe/kubeturbo-bundle:v8.5.0"))

		exutil.By("step: check catsrc address")
		podname2, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("catalogsource", "kubeturbo-catalog", "-n", oc.Namespace(), "-o=jsonpath={.metadata.annotations.operators\\.operatorframework\\.io/registry-pod-name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(podname2).NotTo(o.BeEmpty())
		o.Expect(podname2).NotTo(o.Equal(podname1))

		ip, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", podname2, "-n", oc.Namespace(), "-o=jsonpath={.status.podIP}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(ip).NotTo(o.BeEmpty())

		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("catalogsource", "kubeturbo-catalog", "-n", oc.Namespace(), "-o=jsonpath={.spec.address}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.Equal(ip + ":50051"))

		exutil.By("step: check catsrc sourceType")
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("catalogsource", "kubeturbo-catalog", "-n", oc.Namespace(), "-o=jsonpath={.spec.sourceType}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.Equal("grpc"))

		exutil.By("step: check packagemanifest")
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("packagemanifest", "kubeturbo", "-n", oc.Namespace(), "-o=jsonpath={.status.channels[*].currentCSV}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("kubeturbo-operator.v8.5.0"))

		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("packagemanifest", "kubeturbo", "-n", oc.Namespace(), "-o=jsonpath={.status.channels[*].name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("operator-sdk-run-bundle"))

		exutil.By("SUCCESS")
	})

	// author: xzha@redhat.com
	g.It("VMonly-ConnectedOnly-Author:xzha-High-42028-Update python kubernetes and python openshift to kubernetes 12.0.0", func() {
		if os.Getenv("HTTP_PROXY") != "" || os.Getenv("http_proxy") != "" {
			g.Skip("HTTP_PROXY is not empty - skipping test ...")
		}
		imageTag := "registry-proxy.engineering.redhat.com/rh-osbs/openshift-ose-ansible-operator:v" + ocpversion
		containerCLI := container.NewPodmanCLI()
		e2e.Logf("create container with image %s", imageTag)
		id, err := containerCLI.ContainerCreate(imageTag, "test-42028", "/bin/sh", true)
		defer func() {
			e2e.Logf("stop container %s", id)
			containerCLI.ContainerStop(id)
			e2e.Logf("remove container %s", id)
			err := containerCLI.ContainerRemove(id)
			if err != nil {
				e2e.Failf("Defer: fail to remove container %s", id)
			}
		}()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("container id is %s", id)

		e2e.Logf("start container %s", id)
		err = containerCLI.ContainerStart(id)
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("start container %s successful", id)

		commandStr := []string{"pip3", "show", "kubernetes"}
		output, err := containerCLI.Exec(id, commandStr)
		e2e.Logf("command %s: %s", commandStr, output)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Version:"))
		o.Expect(output).To(o.ContainSubstring("25.3."))

		commandStr = []string{"pip3", "show", "openshift"}
		output, err = containerCLI.Exec(id, commandStr)
		e2e.Logf("command %s: %s", commandStr, output)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Version:"))
		o.Expect(output).To(o.ContainSubstring("0.12."))

		e2e.Logf("OCP 42028 SUCCESS")
	})

	// author: xzha@redhat.com
	g.It("ConnectedOnly-VMonly-Author:xzha-High-44295-Ensure that Go type Operators creation is working [Slow]", func() {
		if os.Getenv("HTTP_PROXY") != "" || os.Getenv("http_proxy") != "" {
			g.Skip("HTTP_PROXY is not empty - skipping test ...")
		}
		architecture.SkipNonAmd64SingleArch(oc)
		buildPruningBaseDir := exutil.FixturePath("testdata", "operatorsdk")
		dataPath := filepath.Join(buildPruningBaseDir, "ocp-44295-data")
		quayCLI := container.NewQuayCLI()
		imageTag := "quay.io/olmqe/memcached-operator:44295-" + getRandomString()
		tmpBasePath := "/tmp/ocp-44295-" + getRandomString()
		tmpPath := filepath.Join(tmpBasePath, "memcached-operator-44295")
		nsSystem := "system-ocp44295" + getRandomString()
		nsOperator := "memcached-operator-system-ocp44295" + getRandomString()
		defer os.RemoveAll(tmpBasePath)
		defer quayCLI.DeleteTag(strings.Replace(imageTag, "quay.io/", "", 1))
		err := os.MkdirAll(tmpPath, 0o755)
		o.Expect(err).NotTo(o.HaveOccurred())
		operatorsdkCLI.ExecCommandPath = tmpPath
		makeCLI.ExecCommandPath = tmpPath

		exutil.By("step: generate go type operator")
		defer func() {
			_, err = makeCLI.Run("undeploy").Args().Output()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		output, err := operatorsdkCLI.Run("init").Args("--domain=example.com", "--repo=github.com/example-inc/memcached-operator-44295").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Writing scaffold for you to edit"))
		o.Expect(output).To(o.ContainSubstring("Next: define a resource with"))

		exutil.By("step: Create a Memcached API.")
		output, err = operatorsdkCLI.Run("create").Args("api", "--resource=true", "--controller=true", "--group=cache", "--version=v1alpha1", "--kind=Memcached44295").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Update dependencies"))
		o.Expect(output).To(o.ContainSubstring("Next"))

		exutil.By("step: update API")
		err = copy(filepath.Join(dataPath, "memcached_types.go"), filepath.Join(tmpPath, "api", "v1alpha1", "memcached44295_types.go"))
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = makeCLI.Run("generate").Args().Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("step: make manifests")
		_, err = makeCLI.Run("manifests").Args().Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("step: modify namespace and controllers")
		crFilePath := filepath.Join(tmpPath, "config", "samples", "cache_v1alpha1_memcached44295.yaml")
		exec.Command("bash", "-c", fmt.Sprintf("sed -i '$d' %s", crFilePath)).Output()
		exec.Command("bash", "-c", fmt.Sprintf("sed -i '$a\\  size: 3' %s", crFilePath)).Output()
		exec.Command("bash", "-c", fmt.Sprintf("sed -i 's/name: system/name: system-ocp44295/g' `grep -rl \"name: system\" %s`", tmpPath)).Output()
		exec.Command("bash", "-c", fmt.Sprintf("sed -i 's/namespace: system/namespace: %s/g'  `grep -rl \"namespace: system\" %s`", nsSystem, tmpPath)).Output()
		exec.Command("bash", "-c", fmt.Sprintf("sed -i 's/namespace: memcached-operator-44295-system/namespace: %s/g'  `grep -rl \"namespace: memcached-operator-44295-system\" %s`", nsOperator, tmpPath)).Output()
		err = copy(filepath.Join(dataPath, "memcached_controller.go"), filepath.Join(tmpPath, "controllers", "memcached44295_controller.go"))
		o.Expect(err).NotTo(o.HaveOccurred())

		if !upstream {
			exutil.By("step: update manager_auth_proxy_patch")
			managerAuthProxyPatch := filepath.Join(tmpPath, "config", "default", "manager_auth_proxy_patch.yaml")
			content := getContent(managerAuthProxyPatch)
			o.Expect(content).To(o.ContainSubstring("registry.redhat.io/openshift4/ose-kube-rbac-proxy:v" + ocpversion))
			replaceContent(managerAuthProxyPatch, "registry.redhat.io/openshift4/ose-kube-rbac-proxy:v"+ocpversion, "registry.redhat.io/openshift4/ose-kube-rbac-proxy:v"+ocppreversion)
		}

		exutil.By("step: Build the operator image")
		dockerFilePath := filepath.Join(tmpPath, "Dockerfile")
		replaceContent(dockerFilePath, "golang:", "quay.io/olmqe/golang:")
		output, err = makeCLI.Run("docker-build").Args("IMG=" + imageTag).Output()
		if (err != nil) && strings.Contains(output, "go mod tidy") {
			e2e.Logf("execute go mod tidy")
			exec.Command("bash", "-c", fmt.Sprintf("cd %s; go mod tidy", tmpPath)).Output()
			output, err = makeCLI.Run("docker-build").Args("IMG=" + imageTag).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).To(o.ContainSubstring("docker build -t"))
		} else {
			o.Expect(output).To(o.ContainSubstring("docker build -t"))
		}

		exutil.By("step: Push the operator image")
		_, err = makeCLI.Run("docker-push").Args("IMG=" + imageTag).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("step: Install kustomize")
		kustomizePath := "/root/kustomize"
		binPath := filepath.Join(tmpPath, "bin")
		exec.Command("bash", "-c", fmt.Sprintf("cp %s %s", kustomizePath, binPath)).Output()

		exutil.By("step: Install the CRD")
		output, err = makeCLI.Run("install").Args().Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("memcached44295s.cache.example.com"))

		exutil.By("step: Deploy the operator")
		output, err = makeCLI.Run("deploy").Args("IMG=" + imageTag).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("deployment.apps/memcached-operator-44295-controller-manager"))

		waitErr := wait.PollUntilContextTimeout(context.TODO(), 30*time.Second, 180*time.Second, false, func(ctx context.Context) (bool, error) {
			podList, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", nsOperator).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			lines := strings.Split(podList, "\n")
			for _, line := range lines {
				if strings.Contains(line, "memcached-operator-44295-controller-manager") {
					e2e.Logf("found pod memcached-operator-44295-controller-manager")
					if strings.Contains(line, "Running") {
						e2e.Logf("the status of pod memcached-operator-44295-controller-manager is Running")
						return true, nil
					}
					e2e.Logf("the status of pod memcached-operator-44295-controller-manager is not Running")
					return false, nil
				}
			}
			return false, nil
		})
		if waitErr != nil {
			logDebugInfo(oc, nsOperator, "events", "pod")
		}
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("No memcached-operator-44295-controller-manager in project %s", nsOperator))
		msg, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("deployment.apps/memcached-operator-44295-controller-manager", "-c", "manager", "-n", nsOperator).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(msg).To(o.ContainSubstring("Starting workers"))

		exutil.By("step: Create the resource")
		_, err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", crFilePath, "-n", nsOperator).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		waitErr = wait.PollUntilContextTimeout(context.TODO(), 30*time.Second, 180*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", nsOperator).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(msg, "memcached44295-sample") {
				e2e.Logf("found pod memcached44295-sample")
				return true, nil
			}
			return false, nil
		})
		if waitErr != nil {
			logDebugInfo(oc, nsOperator, "events", "pod")
		}
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("No memcached44295-sample in project %s", nsOperator))

		waitErr = wait.PollUntilContextTimeout(context.TODO(), 30*time.Second, 180*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, err = oc.AsAdmin().WithoutNamespace().Run("describe").Args("deployment/memcached44295-sample", "-n", nsOperator).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(msg, "3 desired | 3 updated | 3 total | 3 available | 0 unavailable") {
				e2e.Logf("deployment/memcached44295-sample is created successfully")
				return true, nil
			}
			return false, nil
		})
		if waitErr != nil {
			msg, err = oc.AsAdmin().WithoutNamespace().Run("describe").Args("deployment/memcached44295-sample", "-n", nsOperator).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf(msg)
			msg, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("events", "-n", nsOperator).Output()
			e2e.Logf(msg)
		}
		exutil.AssertWaitPollNoErr(waitErr, "the status of deployment/memcached44295-sample is wrong")

		exutil.By("OCP 44295 SUCCESS")
	})

	// author: xzha@redhat.com
	g.It("ConnectedOnly-VMonly-Author:xzha-High-52371-Enable Micrometer Metrics from java operator plugins", func() {
		if os.Getenv("HTTP_PROXY") != "" || os.Getenv("http_proxy") != "" {
			g.Skip("HTTP_PROXY is not empty - skipping test ...")
		}
		architecture.SkipNonAmd64SingleArch(oc)
		var (
			buildPruningBaseDir        = exutil.FixturePath("testdata", "operatorsdk")
			dataPath                   = filepath.Join(buildPruningBaseDir, "ocp-52371-data")
			clusterrolebindingtemplate = filepath.Join(buildPruningBaseDir, "cluster-role-binding.yaml")
			quayCLI                    = container.NewQuayCLI()
			imageTag                   = "quay.io/olmqe/memcached-quarkus-operator:52371-" + getRandomString()
			tmpBasePath                = "/tmp/ocp-52371-" + getRandomString()
			tmpOperatorPath            = filepath.Join(tmpBasePath, "memcached-quarkus-operator-52371")
			kubernetesYamlFilePath     = filepath.Join(tmpOperatorPath, "target", "kubernetes", "kubernetes.yml")
		)
		operatorsdkCLI.ExecCommandPath = tmpOperatorPath
		makeCLI.ExecCommandPath = tmpOperatorPath
		mvnCLI.ExecCommandPath = tmpOperatorPath
		err := os.MkdirAll(tmpOperatorPath, 0o755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(tmpBasePath)

		exutil.By("step: generate java type operator")
		output, err := operatorsdkCLI.Run("init").Args("--plugins=quarkus", "--domain=example.com").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("operator-sdk create api"))

		exutil.By("step: Create API.")
		_, err = operatorsdkCLI.Run("create").Args("api", "--plugins=quarkus", "--group=cache", "--version=v1", "--kind=Memcached52371").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("step: update API and Controller")
		examplePath := filepath.Join(tmpOperatorPath, "src", "main", "java", "com", "example")
		err = copy(filepath.Join(dataPath, "Memcached52371Reconciler.java"), filepath.Join(examplePath, "Memcached52371Reconciler.java"))
		o.Expect(err).NotTo(o.HaveOccurred())
		err = copy(filepath.Join(dataPath, "Memcached52371Spec.java"), filepath.Join(examplePath, "Memcached52371Spec.java"))
		o.Expect(err).NotTo(o.HaveOccurred())
		err = copy(filepath.Join(dataPath, "Memcached52371Status.java"), filepath.Join(examplePath, "Memcached52371Status.java"))
		o.Expect(err).NotTo(o.HaveOccurred())
		err = copy(filepath.Join(dataPath, "pom.xml"), filepath.Join(tmpOperatorPath, "pom.xml"))
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("step:mvn clean install")
		output, err = mvnCLI.Run("clean").Args("install").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("BUILD SUCCESS"))

		exutil.By("step: Build and push the operator image")
		defer quayCLI.DeleteTag(strings.Replace(imageTag, "quay.io/", "", 1))
		output, err = makeCLI.Run("docker-build").Args("docker-push", "IMG="+imageTag).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("BUILD SUCCESS"))

		exutil.By("step: Deploy the operator")
		oc.SetupProject()
		ns := oc.Namespace()

		exutil.By("step: Install the CRD")
		defer func() {
			exutil.By("step: delete crd.")
			_, err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", filepath.Join(tmpOperatorPath, "target", "kubernetes", "memcached52371s.cache.example.com-v1.yml")).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		_, err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", filepath.Join(tmpOperatorPath, "target", "kubernetes", "memcached52371s.cache.example.com-v1.yml")).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("step: Create rcbc file")
		insertContent(kubernetesYamlFilePath, "- kind: ServiceAccount", "    namespace: "+ns)

		exutil.By("step: Deploy the operator")
		defer func() {
			exutil.By("step: delete operator.")
			_, err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", kubernetesYamlFilePath, "-n", ns).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		_, err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", kubernetesYamlFilePath, "-n", ns).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		clusterrolebinding := clusterrolebindingDescription{
			name:      "memcached52371-operator-admin",
			namespace: ns,
			saname:    "memcached-quarkus-operator-52371-operator",
			template:  clusterrolebindingtemplate,
		}
		clusterrolebinding.create(oc)
		waitErr := wait.PollUntilContextTimeout(context.TODO(), 30*time.Second, 180*time.Second, false, func(ctx context.Context) (bool, error) {
			podList, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", ns).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			lines := strings.Split(podList, "\n")
			for _, line := range lines {
				if strings.Contains(line, "memcached-quarkus-operator-52371-operator") {
					e2e.Logf("found pod memcached-quarkus-operator-52371-operator")
					if strings.Contains(line, "Running") {
						e2e.Logf("the status of pod memcached-quarkus-operator-52371-operator is Running")
						return true, nil
					}
					e2e.Logf("the status of pod memcached-quarkus-operator-52371-operator is not Running")
					return false, nil
				}
			}
			return false, nil
		})
		if waitErr != nil {
			logDebugInfo(oc, ns, "events", "pod")
		}
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("No memcached-quarkus-operator-52371-operator in project %s", ns))
		label := `app.kubernetes.io/name=memcached-quarkus-operator-52371-operator`
		podName, err := oc.AsAdmin().Run("get").Args("-n", ns, "pod", "-l", label, "-ojsonpath={..metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(podName).NotTo(o.BeEmpty())

		exutil.By("step: Create CR")
		crFilePath := filepath.Join(dataPath, "memcached-sample.yaml")
		defer func() {
			exutil.By("step: delete cr.")
			_, err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", crFilePath, "-n", ns).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		_, err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", crFilePath, "-n", ns).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		waitErr = wait.PollUntilContextTimeout(context.TODO(), 30*time.Second, 180*time.Second, false, func(ctx context.Context) (bool, error) {
			podList, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", ns).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			lines := strings.Split(podList, "\n")
			for _, line := range lines {
				if strings.Contains(line, "memcached52371-sample") {
					e2e.Logf("found pod memcached52371-sample")
					if strings.Contains(line, "Running") {
						e2e.Logf("the status of pod memcached52371-sample is Running")
						return true, nil
					}
					e2e.Logf("the status of pod memcached52371-sample is not Running")
					return false, nil
				}
			}
			return false, nil
		})
		if waitErr != nil {
			output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("Memcached52371", "-n", ns).Output()
			e2e.Logf(output)
			output, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", ns).Output()
			e2e.Logf(output)
			output, _ = oc.AsAdmin().WithoutNamespace().Run("logs").Args("-n", ns, podName).Output()
			e2e.Logf(output)
		}
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("No memcached52371-sample in project %s or the pod is not running", ns))

		exutil.By("check Micrometer Metrics is enable")
		clusterIP, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ns, "service", "memcached-quarkus-operator-52371-operator", "-o=jsonpath={.spec.clusterIP}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(clusterIP).NotTo(o.BeEmpty())
		url := fmt.Sprintf("http://%s/q/metrics", clusterIP)
		output, err = oc.AsAdmin().WithoutNamespace().Run("rsh").Args("-n", ns, podName, "curl", url).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).NotTo(o.BeEmpty())
		o.Expect(output).To(o.ContainSubstring("system_cpu_count"))

		exutil.By("OCP 52371 SUCCESS")
	})

	// author: xzha@redhat.com
	g.It("ConnectedOnly-VMonly-Author:xzha-High-52377-operatorSDK support java plugin", func() {
		if os.Getenv("HTTP_PROXY") != "" || os.Getenv("http_proxy") != "" {
			g.Skip("HTTP_PROXY is not empty - skipping test ...")
		}
		skipOnProxyCluster(oc)
		architecture.SkipNonAmd64SingleArch(oc)
		var (
			buildPruningBaseDir        = exutil.FixturePath("testdata", "operatorsdk")
			dataPath                   = filepath.Join(buildPruningBaseDir, "ocp-52377-data")
			clusterrolebindingtemplate = filepath.Join(buildPruningBaseDir, "cluster-role-binding.yaml")
			quayCLI                    = container.NewQuayCLI()
			imageTag                   = "quay.io/olmqe/memcached-quarkus-operator:52377-" + getRandomString()
			bundleImage                = "quay.io/olmqe/memcached-quarkus-bundle:52377-" + getRandomString()
			tmpBasePath                = "/tmp/ocp-52377-" + getRandomString()
			tmpOperatorPath            = filepath.Join(tmpBasePath, "memcached-quarkus-operator-52377")
		)
		operatorsdkCLI.ExecCommandPath = tmpOperatorPath
		makeCLI.ExecCommandPath = tmpOperatorPath
		mvnCLI.ExecCommandPath = tmpOperatorPath
		err := os.MkdirAll(tmpOperatorPath, 0o755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(tmpBasePath)

		exutil.By("step: generate java type operator")
		output, err := operatorsdkCLI.Run("init").Args("--plugins=quarkus", "--domain=example.com").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("operator-sdk create api"))

		exutil.By("step: Create API.")
		_, err = operatorsdkCLI.Run("create").Args("api", "--plugins=quarkus", "--group=cache", "--version=v1", "--kind=Memcached52377").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("step: update API and Controller")
		examplePath := filepath.Join(tmpOperatorPath, "src", "main", "java", "com", "example")
		err = copy(filepath.Join(dataPath, "Memcached52377Reconciler.java"), filepath.Join(examplePath, "Memcached52377Reconciler.java"))
		o.Expect(err).NotTo(o.HaveOccurred())
		err = copy(filepath.Join(dataPath, "Memcached52377Spec.java"), filepath.Join(examplePath, "Memcached52377Spec.java"))
		o.Expect(err).NotTo(o.HaveOccurred())
		err = copy(filepath.Join(dataPath, "Memcached52377Status.java"), filepath.Join(examplePath, "Memcached52377Status.java"))
		o.Expect(err).NotTo(o.HaveOccurred())
		err = copy(filepath.Join(dataPath, "pom.xml"), filepath.Join(tmpOperatorPath, "pom.xml"))
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("step:mvn clean install")
		output, err = mvnCLI.Run("clean").Args("install").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("BUILD SUCCESS"))

		exutil.By("step: Build and push the operator image")
		defer quayCLI.DeleteTag(strings.Replace(imageTag, "quay.io/", "", 1))
		output, err = makeCLI.Run("docker-build").Args("docker-push", "IMG="+imageTag).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("BUILD SUCCESS"))

		exutil.By("step: make bundle.")
		output, err = makeCLI.Run("bundle").Args().Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("operator-sdk bundle validate"))

		exutil.By("step: update csv file")
		csvFile := filepath.Join(tmpOperatorPath, "bundle", "manifests", "memcached-quarkus-operator-52377.clusterserviceversion.yaml")
		replaceContent(csvFile, "supported: false", "supported: true")

		exutil.By("step: build and push bundle image.")
		defer quayCLI.DeleteTag(strings.Replace(bundleImage, "quay.io/", "", 1))
		_, err = makeCLI.Run("bundle-build").Args("bundle-push", "BUNDLE_IMG="+bundleImage).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("step: create new project")
		oc.SetupProject()
		ns := oc.Namespace()

		exutil.By("step: run bundle")
		defer func() {
			output, err = operatorsdkCLI.Run("cleanup").Args("memcached-quarkus-operator-52377", "-n", ns).Output()
			if err != nil {
				e2e.Logf(output)
				o.Expect(err).NotTo(o.HaveOccurred())
			}
		}()
		output, err = operatorsdkCLI.Run("run").Args("bundle", bundleImage, "-n", ns, "--timeout", "5m", "--security-context-config=restricted").Output()
		if err != nil {
			logDebugInfo(oc, ns, "csv", "pod", "ip")
		}
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("OLM has successfully installed"))

		exutil.By("step: create clusterrolebinding")
		clusterrolebinding := clusterrolebindingDescription{
			name:      "memcached52377-operator-admin",
			namespace: ns,
			saname:    "memcached-quarkus-operator-52377-operator",
			template:  clusterrolebindingtemplate,
		}
		clusterrolebinding.create(oc)

		waitErr := wait.PollUntilContextTimeout(context.TODO(), 30*time.Second, 180*time.Second, false, func(ctx context.Context) (bool, error) {
			podList, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", ns).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			lines := strings.Split(podList, "\n")
			for _, line := range lines {
				if strings.Contains(line, "memcached-quarkus-operator-52377-operator") {
					e2e.Logf("found pod memcached-quarkus-operator-52377-operator")
					if strings.Contains(line, "Running") {
						e2e.Logf("the status of pod memcached-quarkus-operator-52377-operator is Running")
						return true, nil
					}
					e2e.Logf("the status of pod memcached-quarkus-operator-52377-operator is not Running")
					return false, nil
				}
			}
			return false, nil
		})
		if waitErr != nil {
			logDebugInfo(oc, ns, "pod", "csv", "catsrc")
		}

		exutil.By("step: Create CR")
		crFilePath := filepath.Join(dataPath, "memcached-sample.yaml")
		defer func() {
			exutil.By("step: delete cr.")
			_, err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", crFilePath, "-n", ns).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		_, err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", crFilePath, "-n", ns).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		waitErr = wait.PollUntilContextTimeout(context.TODO(), 30*time.Second, 180*time.Second, false, func(ctx context.Context) (bool, error) {
			podList, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", ns).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			lines := strings.Split(podList, "\n")
			for _, line := range lines {
				if strings.Contains(line, "memcached52377-sample") {
					e2e.Logf("found pod memcached52377-sample")
					if strings.Contains(line, "Running") {
						e2e.Logf("the status of pod memcached52377-sample is Running")
						return true, nil
					}
					e2e.Logf("the status of pod memcached52377-sample is not Running")
					return false, nil
				}
			}
			return false, nil
		})
		if waitErr != nil {
			logDebugInfo(oc, ns, "Memcached52377", "pod", "events")
		}
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("No memcached52377-sample in project %s or the pod is not running", ns))

		exutil.By("OCP 52377 SUCCESS")
	})

	// author: chuo@redhat.com
	g.It("VMonly-ConnectedOnly-Author:chuo-High-40341-Ansible operator needs a way to pass vars as unsafe ", func() {
		imageTag := "quay.io/olmqe/memcached-operator-pass-unsafe:v" + ocpversion + getRandomString()
		clusterArchitecture := architecture.SkipArchitectures(oc, architecture.MULTI, architecture.PPC64LE, architecture.S390X)
		if clusterArchitecture == architecture.ARM64 {
			imageTag = "quay.io/olmqe/memcached-operator-pass-unsafe:v" + ocpversion + "-40341"
		}
		nsSystem := "system-40341-" + getRandomString()
		nsOperator := "memcached-operator-40341-system-" + getRandomString()

		tmpBasePath := "/tmp/ocp-40341-" + getRandomString()
		tmpPath := filepath.Join(tmpBasePath, "memcached-operator-40341")
		err := os.MkdirAll(tmpPath, 0o755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(tmpBasePath)
		operatorsdkCLI.ExecCommandPath = tmpPath
		makeCLI.ExecCommandPath = tmpPath

		defer func() {
			if imageTag != "quay.io/olmqe/memcached-operator-pass-unsafe:v"+ocpversion+"-40341" {
				quayCLI := container.NewQuayCLI()
				quayCLI.DeleteTag(strings.Replace(imageTag, "quay.io/", "", 1))
			}
		}()

		defer func() {
			deployfilepath := filepath.Join(tmpPath, "config", "samples", "cache_v1alpha1_memcached40341.yaml")
			_, err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", deployfilepath, "-n", nsOperator).Output()
			exutil.By("step: undeploy")
			_, err = makeCLI.Run("undeploy").Args().Output()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()

		exutil.By("step: init Ansible Based Operator")
		output, err := operatorsdkCLI.Run("init").Args("--plugins=ansible", "--domain", "example.com").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Next"))

		exutil.By("step: Create API.")
		output, err = operatorsdkCLI.Run("create").Args("api", "--group", "cache", "--version", "v1alpha1", "--kind", "Memcached40341", "--generate-role").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Writing kustomize manifests"))

		if !upstream {
			exutil.By("step: modify Dockerfile.")
			dockerFile := filepath.Join(tmpPath, "Dockerfile")
			content := getContent(dockerFile)
			o.Expect(content).To(o.ContainSubstring("registry.redhat.io/openshift4/ose-ansible-operator:v" + ocpversion))
			replaceContent(dockerFile, "registry.redhat.io/openshift4/ose-ansible-operator:v"+ocpversion, "brew.registry.redhat.io/rh-osbs/openshift-ose-ansible-operator:v"+ocpversion)
			managerAuthProxyPatch := filepath.Join(tmpPath, "config", "default", "manager_auth_proxy_patch.yaml")
			content = getContent(managerAuthProxyPatch)
			o.Expect(content).To(o.ContainSubstring("registry.redhat.io/openshift4/ose-kube-rbac-proxy:v" + ocpversion))
			replaceContent(managerAuthProxyPatch, "registry.redhat.io/openshift4/ose-kube-rbac-proxy:v"+ocpversion, "registry.redhat.io/openshift4/ose-kube-rbac-proxy:v"+ocppreversion)
		}

		deployfilepath := filepath.Join(tmpPath, "config", "samples", "cache_v1alpha1_memcached40341.yaml")
		exec.Command("bash", "-c", fmt.Sprintf("sed -i '$d' %s", deployfilepath)).Output()
		exec.Command("bash", "-c", fmt.Sprintf("sed -i '$a\\  size: 3' %s", deployfilepath)).Output()
		exec.Command("bash", "-c", fmt.Sprintf("sed -i '$a\\  testKey: testVal' %s", deployfilepath)).Output()

		exec.Command("bash", "-c", fmt.Sprintf("sed -i 's/name: system/name: %s/g' `grep -rl \"name: system\" %s`", nsSystem, tmpPath)).Output()
		exec.Command("bash", "-c", fmt.Sprintf("sed -i 's/namespace: system/namespace: %s/g'  `grep -rl \"namespace: system\" %s`", nsSystem, tmpPath)).Output()
		exec.Command("bash", "-c", fmt.Sprintf("sed -i 's/namespace: memcached-operator-40341-system/namespace: %s/g'  `grep -rl \"namespace: memcached-operator-40341-system\" %s`", nsOperator, tmpPath)).Output()

		exutil.By("step: build and Push the operator image")
		dockerFilePath := filepath.Join(tmpPath, "Dockerfile")
		replaceContent(dockerFilePath, "RUN ansible-galaxy collection install -r ${HOME}/requirements.yml", "RUN ansible-galaxy collection install -r ${HOME}/requirements.yml --force")
		tokenDir := "/tmp/ocp-34426" + getRandomString()
		err = os.MkdirAll(tokenDir, os.ModePerm)
		defer os.RemoveAll(tokenDir)
		if err != nil {
			e2e.Failf("fail to create the token folder:%s", tokenDir)
		}
		_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", fmt.Sprintf("--to=%s", tokenDir), "--confirm").Output()
		if err != nil {
			e2e.Failf("Fail to get the cluster auth %v", err)
		}
		// TODO[aleskandro,chuo]: this is a workaround: https://issues.redhat.com/browse/ARMOCP-531
		architecture.SkipArchitectures(oc, architecture.MULTI, architecture.PPC64LE, architecture.S390X)
		switch clusterArchitecture {
		case architecture.AMD64:
			buildPushOperatorImage(clusterArchitecture, tmpPath, imageTag, tokenDir)
		case architecture.ARM64:
			e2e.Logf(fmt.Sprintf("platform is %s, IMG is %s", clusterArchitecture, imageTag))
		}

		exutil.By("step: Install the CRD")
		output, err = makeCLI.Run("install").Args().Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("memcached40341s.cache.example.com created"))
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("crd", "memcached40341s.cache.example.com").Output()
		e2e.Logf(output)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).NotTo(o.ContainSubstring("NotFound"))

		exutil.By("step: Deploy the operator")
		output, err = makeCLI.Run("deploy").Args("IMG=" + imageTag).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("deployment.apps/memcached-operator-40341-controller-manager"))

		waitErr := wait.PollUntilContextTimeout(context.TODO(), 30*time.Second, 180*time.Second, false, func(ctx context.Context) (bool, error) {
			podList, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", nsOperator).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			lines := strings.Split(podList, "\n")
			for _, line := range lines {
				if strings.Contains(line, "memcached-operator-40341-controller-manager") {
					e2e.Logf("found pod memcached-operator-40341-controller-manager")
					if strings.Contains(line, "Running") {
						e2e.Logf("the status of pod memcached-operator-40341-controller-manager is Running")
						return true, nil
					}
					e2e.Logf("the status of pod memcached-operator-40341-controller-manager is not Running")
					return false, nil
				}
			}
			return false, nil
		})
		if waitErr != nil {
			logDebugInfo(oc, nsOperator, "events", "pod")
		}
		exutil.AssertWaitPollNoErr(waitErr, "No memcached-operator-40341-controller-manager")

		exutil.By("step: Create the resource")
		_, err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", deployfilepath, "-n", nsOperator).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		waitErr = wait.PollUntilContextTimeout(context.TODO(), 5*time.Second, 10*time.Second, false, func(ctx context.Context) (bool, error) {
			output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("memcached40341s.cache.example.com", "-n", nsOperator).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(output, "memcached40341-sample") {
				e2e.Logf("cr memcached40341-sample is created")
				return true, nil
			}
			return false, nil
		})
		if waitErr != nil {
			logDebugInfo(oc, nsOperator, "events", "pod")
		}
		exutil.AssertWaitPollNoErr(waitErr, "No cr memcached40341-sample")

		exutil.By("step: check vars")
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("memcached40341s.cache.example.com/memcached40341-sample", "-n", nsOperator, "-o=jsonpath={.spec.size}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.Equal("3"))
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("memcached40341s.cache.example.com/memcached40341-sample", "-n", nsOperator, "-o=jsonpath={.spec.testKey}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.Equal("testVal"))
		exutil.By("40341 SUCCESS")
	})

	// author: jfan@redhat.com
	g.It("Author:jfan-Critical-49960-verify an empty CRD description", func() {
		tmpBasePath := exutil.FixturePath("testdata", "operatorsdk", "ocp-49960-data")

		exutil.By("step: validate CRD description success")
		output, err := operatorsdkCLI.Run("bundle").Args("validate", tmpBasePath+"/bundle", "--select-optional", "name=good-practices").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("All validation tests have completed successfully"))

		exutil.By("step: validate empty CRD description")
		csvFilePath := filepath.Join(tmpBasePath, "bundle", "manifests", "k8sevent.clusterserviceversion.yaml")
		replaceContent(csvFilePath, "description: test", "description:")
		output, err = operatorsdkCLI.Run("bundle").Args("validate", tmpBasePath+"/bundle", "--select-optional", "name=good-practices").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("has an empty description"))

		exutil.By("step: validate olm unsupported resource")
		crdFilePath := filepath.Join(tmpBasePath, "bundle", "manifests", "k8s.k8sevent.com_k8sevents.yaml")
		replaceContent(crdFilePath, "CustomResourceDefinition", "CustomResource")
		output, _ = operatorsdkCLI.Run("bundle").Args("validate", tmpBasePath+"/bundle", "--select-optional", "name=good-practices").Output()
		o.Expect(output).To(o.ContainSubstring("unsupported media type"))

		exutil.By("SUCCESS 49960")
	})

	// author: jfan@redhat.com
	g.It("VMonly-ConnectedOnly-Author:jfan-High-48885-SDK generate digest type bundle of ansible", func() {
		architecture.SkipArchitectures(oc, architecture.MULTI, architecture.PPC64LE, architecture.S390X)
		buildPruningBaseDir := exutil.FixturePath("testdata", "operatorsdk")
		dataPath := filepath.Join(buildPruningBaseDir, "ocp-48885-data")
		tmpBasePath := "/tmp/ocp-48885-" + getRandomString()
		tmpPath := filepath.Join(tmpBasePath, "memcached-operator-48885")
		err := os.MkdirAll(tmpPath, 0o755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(tmpBasePath)
		operatorsdkCLI.ExecCommandPath = tmpPath
		makeCLI.ExecCommandPath = tmpPath

		exutil.By("step: init Ansible Based Operator")
		output, err := operatorsdkCLI.Run("init").Args("--plugins=ansible", "--domain", "example.com").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Next"))

		exutil.By("step: Create API.")
		output, err = operatorsdkCLI.Run("create").Args("api", "--group", "cache", "--version", "v1", "--kind", "Memcached48885", "--generate-role").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Writing kustomize manifests"))

		exutil.By("step: modify files to get the quay.io/olmqe images.")
		// copy task main.yml
		err = copy(filepath.Join(dataPath, "main.yml"), filepath.Join(tmpPath, "roles", "memcached48885", "tasks", "main.yml"))
		o.Expect(err).NotTo(o.HaveOccurred())
		// copy manager.yaml
		err = copy(filepath.Join(dataPath, "manager.yaml"), filepath.Join(tmpPath, "config", "manager", "manager.yaml"))
		o.Expect(err).NotTo(o.HaveOccurred())
		// update the Makefile
		makefileFilePath := filepath.Join(tmpPath, "Makefile")
		replaceContent(makefileFilePath, "controller:latest", "quay.io/olmqe/ansibledisconnected:v4.11")
		replaceContent(makefileFilePath, "operator-sdk generate bundle -q --overwrite --version $(VERSION) $(BUNDLE_METADATA_OPTS)", "operator-sdk generate bundle $(BUNDLE_GEN_FLAGS)")
		// update the rbac file
		rbacFilePath := filepath.Join(tmpPath, "config", "default", "manager_auth_proxy_patch.yaml")
		replaceContent(rbacFilePath, "registry.redhat.io/openshift4/ose-kube-rbac-proxy:v"+ocpversion, "quay.io/olmqe/kube-rbac-proxy:v"+ocppreversion)

		exutil.By("step: make bundle.")
		// copy manifests
		manifestsPath := filepath.Join(tmpPath, "config", "manifests", "bases")
		err = os.MkdirAll(manifestsPath, 0o755)
		o.Expect(err).NotTo(o.HaveOccurred())
		manifestsFile := filepath.Join(manifestsPath, "memcached-operator-48885.clusterserviceversion.yaml")
		_, err = os.Create(manifestsFile)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = copy(filepath.Join(dataPath, "memcached-operator-48885.clusterserviceversion.yaml"), filepath.Join(manifestsFile))
		o.Expect(err).NotTo(o.HaveOccurred())
		// make bundle use image digests
		waitErr := wait.PollUntilContextTimeout(context.TODO(), 30*time.Second, 120*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, err := makeCLI.Run("bundle").Args("USE_IMAGE_DIGESTS=true").Output()
			if err != nil {
				e2e.Logf("make bundle failed, try again")
				return false, nil
			}
			if strings.Contains(msg, "operator-sdk bundle validate ./bundle") {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "operator-sdk bundle generate failed")
		csvFile := filepath.Join(tmpPath, "bundle", "manifests", "memcached-operator-48885.clusterserviceversion.yaml")
		content := getContent(csvFile)
		if !strings.Contains(content, "quay.io/olmqe/memcached@sha256:") || !strings.Contains(content, "kube-rbac-proxy@sha256:") || !strings.Contains(content, "quay.io/olmqe/ansibledisconnected@sha256:") {
			e2e.Failf("Fail to get the image info with digest type")
		}
	})

	// author: jfan@redhat.com
	g.It("VMonly-ConnectedOnly-Author:jfan-High-52813-SDK generate digest type bundle of helm", func() {
		if os.Getenv("HTTP_PROXY") != "" || os.Getenv("http_proxy") != "" {
			g.Skip("HTTP_PROXY is not empty - skipping test ...")
		}
		clusterArchitecture := architecture.SkipNonAmd64SingleArch(oc)
		buildPruningBaseDir := exutil.FixturePath("testdata", "operatorsdk")
		dataPath := filepath.Join(buildPruningBaseDir, "ocp-52813-data")
		tmpBasePath := "/tmp/ocp-52813-" + getRandomString()
		tmpPath := filepath.Join(tmpBasePath, "memcached-operator-52813")
		imageTag := "quay.io/olmqe/memcached-operator:52813-" + getRandomString()
		err := os.MkdirAll(tmpPath, 0o755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(tmpBasePath)
		operatorsdkCLI.ExecCommandPath = tmpPath
		makeCLI.ExecCommandPath = tmpPath
		defer func() {
			quayCLI := container.NewQuayCLI()
			quayCLI.DeleteTag(strings.Replace(imageTag, "quay.io/", "", 1))
		}()

		exutil.By("step: init Helm Based Operator")
		output, err := operatorsdkCLI.Run("init").Args("--plugins=helm", "--domain=disconnected.com", "--group=test", "--version=v1", "--kind=Nginx").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Created helm-charts"))

		exutil.By("step: modify files to get the quay.io/olmqe images.")

		// copy watches.yaml
		err = copy(filepath.Join(dataPath, "watches.yaml"), filepath.Join(tmpPath, "watches.yaml"))
		o.Expect(err).NotTo(o.HaveOccurred())
		// copy helm-charts/nginx/values.yaml
		err = copy(filepath.Join(dataPath, "values.yaml"), filepath.Join(tmpPath, "helm-charts", "nginx", "values.yaml"))
		o.Expect(err).NotTo(o.HaveOccurred())
		// update the file helm-charts/nginx/templates/deployment.yaml
		deployFilepath := filepath.Join(tmpPath, "helm-charts", "nginx", "templates", "deployment.yaml")
		replaceContent(deployFilepath, ".Values.image.repository }}:{{ .Values.image.tag | default .Chart.AppVersion", ".Values.relatedImage")
		// update the Dockerfile
		dockerFile := filepath.Join(tmpPath, "Dockerfile")
		replaceContent(dockerFile, "registry.redhat.io/openshift4/ose-helm-operator:v"+ocpversion, "brew.registry.redhat.io/rh-osbs/openshift-ose-helm-operator:v"+ocpversion)
		// copy the manager
		err = copy(filepath.Join(dataPath, "manager.yaml"), filepath.Join(tmpPath, "config", "manager", "manager.yaml"))
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("step: Build and push the operator image")
		tokenDir := "/tmp/ocp-52813" + getRandomString()
		err = os.MkdirAll(tokenDir, os.ModePerm)
		defer os.RemoveAll(tokenDir)
		if err != nil {
			e2e.Failf("fail to create the token folder:%s", tokenDir)
		}
		_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", fmt.Sprintf("--to=%s", tokenDir), "--confirm").Output()
		if err != nil {
			e2e.Failf("Fail to get the cluster auth %v", err)
		}
		buildPushOperatorImage(clusterArchitecture, tmpPath, imageTag, tokenDir)

		// update the Makefile
		makefileFilePath := filepath.Join(tmpPath, "Makefile")
		replaceContent(makefileFilePath, "controller:latest", imageTag)
		replaceContent(makefileFilePath, "operator-sdk generate bundle -q --overwrite --version $(VERSION) $(BUNDLE_METADATA_OPTS)", "operator-sdk generate bundle $(BUNDLE_GEN_FLAGS)")
		// update the rbac file
		rbacFilePath := filepath.Join(tmpPath, "config", "default", "manager_auth_proxy_patch.yaml")
		replaceContent(rbacFilePath, "registry.redhat.io/openshift4/ose-kube-rbac-proxy:v"+ocpversion, "quay.io/olmqe/kube-rbac-proxy:v"+ocppreversion)

		exutil.By("step: make bundle.")
		// copy manifests
		manifestsPath := filepath.Join(tmpPath, "config", "manifests", "bases")
		err = os.MkdirAll(manifestsPath, 0o755)
		o.Expect(err).NotTo(o.HaveOccurred())
		manifestsFile := filepath.Join(manifestsPath, "memcached-operator-52813.clusterserviceversion.yaml")
		_, err = os.Create(manifestsFile)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = copy(filepath.Join(dataPath, "memcached-operator-52813.clusterserviceversion.yaml"), filepath.Join(manifestsFile))
		o.Expect(err).NotTo(o.HaveOccurred())
		// make bundle use image digests
		waitErr := wait.PollUntilContextTimeout(context.TODO(), 30*time.Second, 120*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, err := makeCLI.Run("bundle").Args("USE_IMAGE_DIGESTS=true").Output()
			if err != nil {
				e2e.Logf("make bundle failed, try again")
				return false, nil
			}
			if strings.Contains(msg, "operator-sdk bundle validate ./bundle") {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "operator-sdk bundle generate failed")
		csvFile := filepath.Join(tmpPath, "bundle", "manifests", "memcached-operator-52813.clusterserviceversion.yaml")
		content := getContent(csvFile)
		if !strings.Contains(content, "quay.io/olmqe/nginx@sha256:") || !strings.Contains(content, "kube-rbac-proxy@sha256:") || !strings.Contains(content, "quay.io/olmqe/memcached-operator@sha256:") {
			e2e.Failf("Fail to get the image info with digest type")
		}
	})

	// author: jfan@redhat.com
	g.It("VMonly-ConnectedOnly-Author:jfan-High-52814-SDK generate digest type bundle of go", func() {
		if os.Getenv("HTTP_PROXY") != "" || os.Getenv("http_proxy") != "" {
			g.Skip("HTTP_PROXY is not empty - skipping test ...")
		}
		clusterArchitecture := architecture.SkipArchitectures(oc, architecture.MULTI, architecture.PPC64LE, architecture.S390X)
		buildPruningBaseDir := exutil.FixturePath("testdata", "operatorsdk")
		dataPath := filepath.Join(buildPruningBaseDir, "ocp-52814-data")
		tmpBasePath := "/tmp/ocp-52814-" + getRandomString()
		tmpPath := filepath.Join(tmpBasePath, "memcached-operator-52814")
		imageTag := "quay.io/olmqe/memcached-operator:52814-" + getRandomString()
		err := os.MkdirAll(tmpPath, 0o755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(tmpBasePath)
		operatorsdkCLI.ExecCommandPath = tmpPath
		makeCLI.ExecCommandPath = tmpPath
		defer func() {
			quayCLI := container.NewQuayCLI()
			quayCLI.DeleteTag(strings.Replace(imageTag, "quay.io/", "", 1))
		}()

		exutil.By("step: init Go Based Operator")
		output, err := operatorsdkCLI.Run("init").Args("--domain=disconnected.com", "--repo=github.com/example-inc/memcached-operator").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Next"))

		exutil.By("step: create api")
		output, err = operatorsdkCLI.Run("create").Args("api", "--group=test", "--version=v1", "--kind=Memcached52814", "--controller", "--resource").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("make manifests"))

		exutil.By("step: modify files to get the quay.io/olmqe images.")
		// copy api/v1/memcached52814_types.go
		err = copy(filepath.Join(dataPath, "memcached52814_types.go"), filepath.Join(tmpPath, "api", "v1", "memcached52814_types.go"))
		o.Expect(err).NotTo(o.HaveOccurred())
		// copy controllers/memcached52814_controller.go
		err = copy(filepath.Join(dataPath, "memcached52814_controller.go"), filepath.Join(tmpPath, "internal", "controller", "memcached52814_controller.go"))
		o.Expect(err).NotTo(o.HaveOccurred())
		// copy the manager
		err = copy(filepath.Join(dataPath, "manager.yaml"), filepath.Join(tmpPath, "config", "manager", "manager.yaml"))
		o.Expect(err).NotTo(o.HaveOccurred())
		// update the Dockerfile
		dockerfileFilePath := filepath.Join(tmpPath, "Dockerfile")
		replaceContent(dockerfileFilePath, "golang", "quay.io/olmqe/golang")

		exutil.By("step: Build and push the operator image")
		tokenDir := "/tmp/ocp-52814" + getRandomString()
		err = os.MkdirAll(tokenDir, os.ModePerm)
		defer os.RemoveAll(tokenDir)
		if err != nil {
			e2e.Failf("fail to create the token folder:%s", tokenDir)
		}
		_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", fmt.Sprintf("--to=%s", tokenDir), "--confirm").Output()
		if err != nil {
			e2e.Failf("Fail to get the cluster auth %v", err)
		}
		buildPushOperatorImage(clusterArchitecture, tmpPath, imageTag, tokenDir)

		// update the Makefile
		makefileFilePath := filepath.Join(tmpPath, "Makefile")
		replaceContent(makefileFilePath, "controller:latest", imageTag)
		// update the rbac file
		rbacFilePath := filepath.Join(tmpPath, "config", "default", "manager_auth_proxy_patch.yaml")
		replaceContent(rbacFilePath, "registry.redhat.io/openshift4/ose-kube-rbac-proxy:v"+ocpversion, "quay.io/olmqe/kube-rbac-proxy:v"+ocppreversion)

		exutil.By("step: Install kustomize")
		kustomizePath := "/root/kustomize"
		binPath := filepath.Join(tmpPath, "bin")
		exec.Command("bash", "-c", fmt.Sprintf("cp %s %s", kustomizePath, binPath)).Output()

		exutil.By("step: make bundle.")
		// copy manifests
		manifestsPath := filepath.Join(tmpPath, "config", "manifests", "bases")
		err = os.MkdirAll(manifestsPath, 0o755)
		o.Expect(err).NotTo(o.HaveOccurred())
		manifestsFile := filepath.Join(manifestsPath, "memcached-operator-52814.clusterserviceversion.yaml")
		_, err = os.Create(manifestsFile)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = copy(filepath.Join(dataPath, "memcached-operator-52814.clusterserviceversion.yaml"), filepath.Join(manifestsFile))
		o.Expect(err).NotTo(o.HaveOccurred())
		// make bundle use image digests
		waitErr := wait.PollUntilContextTimeout(context.TODO(), 30*time.Second, 120*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, err := makeCLI.Run("bundle").Args("USE_IMAGE_DIGESTS=true").Output()
			if err != nil {
				e2e.Logf("make bundle failed, try again")
				return false, nil
			}
			if strings.Contains(msg, "operator-sdk bundle validate ./bundle") {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "operator-sdk bundle generate failed")
		csvFile := filepath.Join(tmpPath, "bundle", "manifests", "memcached-operator-52814.clusterserviceversion.yaml")
		content := getContent(csvFile)
		if !strings.Contains(content, "quay.io/olmqe/memcached@sha256:") || !strings.Contains(content, "kube-rbac-proxy@sha256:") || !strings.Contains(content, "quay.io/olmqe/memcached-operator@sha256:") {
			e2e.Failf("Fail to get the image info with digest type")
		}
	})

	// author: jfan@redhat.com
	g.It("VMonly-ConnectedOnly-Author:jfan-High-44550-SDK support ansible type operator for http_proxy env", func() {
		g.By("Check if it is a proxy platform")
		proxySet, _ := oc.WithoutNamespace().AsAdmin().Run("get").Args("proxy/cluster", "-o=jsonpath={.spec.httpProxy}").Output()
		if proxySet == " " {
			g.Skip("Skip for no-proxy platform")
		}
		clusterArchitecture := architecture.SkipNonAmd64SingleArch(oc)
		tmpBasePath := "/tmp/ocp-44550-" + getRandomString()
		tmpPath := filepath.Join(tmpBasePath, "memcached-operator-44550")
		err := os.MkdirAll(tmpPath, 0o755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(tmpBasePath)
		operatorsdkCLI.ExecCommandPath = tmpPath
		makeCLI.ExecCommandPath = tmpPath
		nsOperator := "memcached-operator-44550-system"
		imageTag := "quay.io/olmqe/memcached-operator:44550-" + getRandomString()
		buildPruningBaseDir := exutil.FixturePath("testdata", "operatorsdk")
		dataPath := filepath.Join(buildPruningBaseDir, "ocp-44550-data")
		crFilePath := filepath.Join(dataPath, "cache_v1_memcached44550.yaml")
		quayCLI := container.NewQuayCLI()
		defer quayCLI.DeleteTag(strings.Replace(imageTag, "quay.io/", "", 1))

		defer func() {
			_, err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", crFilePath, "-n", nsOperator).Output()
			exutil.By("step: undeploy")
			_, err = makeCLI.Run("undeploy").Args().Output()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()

		exutil.By("step: init Ansible Based Operator")
		output, err := operatorsdkCLI.Run("init").Args("--plugins=ansible", "--domain", "example.com").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Next"))

		exutil.By("step: Create API.")
		output, err = operatorsdkCLI.Run("create").Args("api", "--group", "cache", "--version", "v1", "--kind", "Memcached44550", "--generate-role").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Writing kustomize manifests"))

		exutil.By("step: modify files to get the quay.io/olmqe images.")
		// copy task main.yml
		err = copy(filepath.Join(dataPath, "main.yml"), filepath.Join(tmpPath, "roles", "memcached44550", "tasks", "main.yml"))
		o.Expect(err).NotTo(o.HaveOccurred())
		// copy manager.yaml
		err = copy(filepath.Join(dataPath, "manager.yaml"), filepath.Join(tmpPath, "config", "manager", "manager.yaml"))
		o.Expect(err).NotTo(o.HaveOccurred())
		// update the Makefile
		makefileFilePath := filepath.Join(tmpPath, "Makefile")
		replaceContent(makefileFilePath, "controller:latest", imageTag)
		replaceContent(makefileFilePath, "build config/default | kubectl apply -f -", "build config/default | CLUSTER_PROXY=$(shell kubectl get proxies.config.openshift.io cluster  -o json | jq '.spec.httpProxy') envsubst | kubectl apply -f -")
		// update the Dockerfile
		dockerFile := filepath.Join(tmpPath, "Dockerfile")
		replaceContent(dockerFile, "registry.redhat.io/openshift4/ose-ansible-operator:v"+ocpversion, "brew.registry.redhat.io/rh-osbs/openshift-ose-ansible-operator:v"+ocpversion)
		replaceContent(dockerFile, "install -r ${HOME}/requirements.yml", "install -r ${HOME}/requirements.yml --force")
		// update the rbac file
		rbacFilePath := filepath.Join(tmpPath, "config", "default", "manager_auth_proxy_patch.yaml")
		replaceContent(rbacFilePath, "registry.redhat.io/openshift4/ose-kube-rbac-proxy:v"+ocpversion, "quay.io/olmqe/kube-rbac-proxy:v"+ocppreversion)

		exutil.By("step: Build and push the operator image")
		tokenDir := "/tmp/ocp-44550" + getRandomString()
		err = os.MkdirAll(tokenDir, os.ModePerm)
		defer os.RemoveAll(tokenDir)
		if err != nil {
			e2e.Failf("fail to create the token folder:%s", tokenDir)
		}
		_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", fmt.Sprintf("--to=%s", tokenDir), "--confirm").Output()
		if err != nil {
			e2e.Failf("Fail to get the cluster auth %v", err)
		}
		buildPushOperatorImage(clusterArchitecture, tmpPath, imageTag, tokenDir)

		exutil.By("step: Install the CRD")
		output, err = makeCLI.Run("install").Args().Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("memcached44550s.cache.example.com"))

		exutil.By("step: Deploy the operator")
		output, err = makeCLI.Run("deploy").Args("IMG=" + imageTag).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("deployment.apps/memcached-operator-44550-controller-manager"))

		waitErr := wait.PollUntilContextTimeout(context.TODO(), 30*time.Second, 180*time.Second, false, func(ctx context.Context) (bool, error) {
			podList, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", nsOperator).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			lines := strings.Split(podList, "\n")
			for _, line := range lines {
				if strings.Contains(line, "memcached-operator-44550-controller-manager") {
					e2e.Logf("found pod memcached-operator-44550-controller-manager")
					if strings.Contains(line, "2/2") {
						e2e.Logf("the status of pod memcached-operator-44550-controller-manager is Running")
						return true, nil
					}
					e2e.Logf("the status of pod memcached-operator-44550-controller-manager is not Running")
					return false, nil
				}
			}
			return false, nil
		})
		if waitErr != nil {
			logDebugInfo(oc, nsOperator, "events", "pod")
		}
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("No memcached-operator-44550-controller-manager in project %s", nsOperator))
		msg, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("deployment.apps/memcached-operator-44550-controller-manager", "-c", "manager", "-n", nsOperator).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(msg, "Starting workers") {
			e2e.Failf("Starting workers failed")
		}

		exutil.By("step: Create the resource")
		_, err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", crFilePath, "-n", nsOperator).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		waitErr = wait.PollUntilContextTimeout(context.TODO(), 30*time.Second, 180*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", nsOperator).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(msg, "memcached44550-sample-ansiblehttp") {
				e2e.Logf("found pod memcached44550-sample-ansiblehttp")
				return true, nil
			}
			return false, nil
		})
		if waitErr != nil {
			logDebugInfo(oc, nsOperator, "events", "pod")
		}
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("No memcached44550-sample in project %s", nsOperator))

		waitErr = wait.PollUntilContextTimeout(context.TODO(), 30*time.Second, 180*time.Second, false, func(ctx context.Context) (bool, error) {
			proxyMsg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("proxies.config.openshift.io", "cluster", "-o=jsonpath={.spec.httpProxy}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			msg, err = oc.AsAdmin().WithoutNamespace().Run("describe").Args("deployment/memcached44550-sample-ansiblehttp", "-n", nsOperator).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(msg).To(o.ContainSubstring("HTTP_PROXY:  " + proxyMsg))
			if strings.Contains(msg, "3 desired | 3 updated | 3 total | 3 available | 0 unavailable") {
				e2e.Logf("deployment/memcach44550-sample is created successfully")
				return true, nil
			}
			return false, nil
		})
		if waitErr != nil {
			logDebugInfo(oc, nsOperator, "events")
		}
		exutil.AssertWaitPollNoErr(waitErr, "the status of deployment/memcached44550-sample-ansiblehttp is wrong")
	})

	// author: jfan@redhat.com
	g.It("VMonly-ConnectedOnly-Author:jfan-High-44551-SDK support helm type operator for http_proxy env", func() {
		clusterArchitecture := architecture.SkipNonAmd64SingleArch(oc)
		tmpBasePath := "/tmp/ocp-44551-" + getRandomString()
		tmpPath := filepath.Join(tmpBasePath, "memcached-operator-44551")
		err := os.MkdirAll(tmpPath, 0o755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(tmpBasePath)
		operatorsdkCLI.ExecCommandPath = tmpPath
		makeCLI.ExecCommandPath = tmpPath
		nsOperator := "memcached-operator-44551-system"
		imageTag := "quay.io/olmqe/memcached-operator:44551-" + getRandomString()
		dataPath := "test/extended/testdata/operatorsdk/ocp-44551-data/"
		crFilePath := filepath.Join(dataPath, "kakademo_v1_nginx.yaml")
		quayCLI := container.NewQuayCLI()
		defer quayCLI.DeleteTag(strings.Replace(imageTag, "quay.io/", "", 1))

		defer func() {
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", crFilePath, "-n", nsOperator).Output()
			exutil.By("step: undeploy")
			_, err = makeCLI.Run("undeploy").Args().Output()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()

		exutil.By("step: init Helm Based Operator")
		output, err := operatorsdkCLI.Run("init").Args("--plugins=helm", "--domain=httpproxy.com", "--group=kakademo", "--version=v1", "--kind=Nginx").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Created helm-charts"))

		exutil.By("step: modify files to generate the quay.io/olmqe images.")
		// update the Dockerfile
		dockerFile := filepath.Join(tmpPath, "Dockerfile")
		replaceContent(dockerFile, "registry.redhat.io/openshift4/ose-helm-operator:v"+ocpversion, "brew.registry.redhat.io/rh-osbs/openshift-ose-helm-operator:v"+ocpversion)
		// update the rbac file
		rbacFilePath := filepath.Join(tmpPath, "config", "default", "manager_auth_proxy_patch.yaml")
		replaceContent(rbacFilePath, "registry.redhat.io/openshift4/ose-kube-rbac-proxy:v"+ocpversion, "quay.io/olmqe/kube-rbac-proxy:v"+ocppreversion)
		// update the Makefile
		makefileFilePath := filepath.Join(tmpPath, "Makefile")
		replaceContent(makefileFilePath, "controller:latest", imageTag)
		replaceContent(makefileFilePath, "build config/default | kubectl apply -f -", "build config/default | CLUSTER_PROXY=$(shell kubectl get proxies.config.openshift.io cluster  -o json | jq '.spec.httpProxy') envsubst | kubectl apply -f -")
		// copy watches.yaml
		err = copy(filepath.Join(dataPath, "watches.yaml"), filepath.Join(tmpPath, "watches.yaml"))
		o.Expect(err).NotTo(o.HaveOccurred())
		// copy helm-charts/nginx/values.yaml
		err = copy(filepath.Join(dataPath, "values.yaml"), filepath.Join(tmpPath, "helm-charts", "nginx", "values.yaml"))
		o.Expect(err).NotTo(o.HaveOccurred())
		// copy helm-charts/nginx/templates/deployment.yaml
		err = copy(filepath.Join(dataPath, "deployment.yaml"), filepath.Join(tmpPath, "helm-charts", "nginx", "templates", "deployment.yaml"))
		o.Expect(err).NotTo(o.HaveOccurred())
		// copy config/manager/manager.yaml
		err = copy(filepath.Join(dataPath, "manager.yaml"), filepath.Join(tmpPath, "config", "manager", "manager.yaml"))
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("step: Build and push the operator image")
		tokenDir := "/tmp/ocp-44551" + getRandomString()
		err = os.MkdirAll(tokenDir, os.ModePerm)
		defer os.RemoveAll(tokenDir)
		if err != nil {
			e2e.Failf("fail to create the token folder:%s", tokenDir)
		}
		_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", fmt.Sprintf("--to=%s", tokenDir), "--confirm").Output()
		if err != nil {
			e2e.Failf("Fail to get the cluster auth %v", err)
		}
		buildPushOperatorImage(clusterArchitecture, tmpPath, imageTag, tokenDir)

		exutil.By("step: Install the CRD")
		output, err = makeCLI.Run("install").Args().Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("kakademo"))

		exutil.By("step: Deploy the operator")
		output, err = makeCLI.Run("deploy").Args("IMG=" + imageTag).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("deployment.apps/memcached-operator-44551-controller-manager"))

		waitErr := wait.PollUntilContextTimeout(context.TODO(), 30*time.Second, 180*time.Second, false, func(ctx context.Context) (bool, error) {
			podList, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", nsOperator).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			lines := strings.Split(podList, "\n")
			for _, line := range lines {
				if strings.Contains(line, "memcached-operator-44551-controller-manager") {
					e2e.Logf("found pod memcached-operator-44551-controller-manager")
					if strings.Contains(line, "2/2") {
						e2e.Logf("the status of pod memcached-operator-44551-controller-manager is Running")
						return true, nil
					}
					e2e.Logf("the status of pod memcached-operator-44551-controller-manager is not Running")
					return false, nil
				}
			}
			return false, nil
		})
		if waitErr != nil {
			logDebugInfo(oc, nsOperator, "events", "pod")
		}
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("No memcached-operator-44551-controller-manager in project %s", nsOperator))
		waitErr = wait.PollUntilContextTimeout(context.TODO(), 30*time.Second, 180*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("deployment.apps/memcached-operator-44551-controller-manager", "-c", "manager", "-n", nsOperator).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(msg, "Starting workers") {
				e2e.Logf("Starting workers successfully")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "container manager doesn't work")

		exutil.By("step: Create the resource")
		_, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "add-scc-to-user", "anyuid", fmt.Sprintf("system:serviceaccount:%s:nginx-sample", nsOperator)).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", crFilePath, "-n", nsOperator).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		waitErr = wait.PollUntilContextTimeout(context.TODO(), 30*time.Second, 180*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", nsOperator).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(msg, "nginx-sample") {
				e2e.Logf("found pod nginx-sample")
				return true, nil
			}
			return false, nil
		})
		if waitErr != nil {
			logDebugInfo(oc, nsOperator, "events", "pod")
		}
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("No memcached44551-sample in project %s", nsOperator))

		waitErr = wait.PollUntilContextTimeout(context.TODO(), 30*time.Second, 180*time.Second, false, func(ctx context.Context) (bool, error) {
			proxyMsg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("proxies.config.openshift.io", "cluster", "-o=jsonpath={.spec.httpProxy}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			msg, err := oc.AsAdmin().WithoutNamespace().Run("describe").Args("deployment/nginx-sample", "-n", nsOperator).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(msg).To(o.ContainSubstring("HTTP_PROXY:  " + proxyMsg))
			if strings.Contains(msg, "HTTP_PROXY:  "+proxyMsg) {
				e2e.Logf("deployment/nginx-sample is created successfully")
				return true, nil
			}
			return false, nil
		})
		if waitErr != nil {
			logDebugInfo(oc, nsOperator, "events")
		}
		exutil.AssertWaitPollNoErr(waitErr, "the status of deployment/memcached44551-sample-ansiblehttp is wrong")
	})

	// author: jitli@redhat.com
	g.It("NonPreRelease-Longduration-VMonly-ConnectedOnly-Author:jitli-High-50065-SDK Add file based catalog support to run bundle", func() {

		skipOnProxyCluster(oc)
		operatorsdkCLI.showInfo = true
		oc.SetupProject()
		exutil.By("Run bundle without index")
		defer operatorsdkCLI.Run("cleanup").Args("k8sevent", "-n", oc.Namespace()).Output()
		output, err := operatorsdkCLI.Run("run").Args("bundle", "quay.io/olmqe/k8sevent-bundle:v4.11", "-n", oc.Namespace(), "--timeout", "5m", "--security-context-config=restricted").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Generated a valid File-Based Catalog"))
		o.Expect(output).To(o.ContainSubstring("OLM has successfully installed"))

		exutil.By("Run bundle with FBC index")
		defer operatorsdkCLI.Run("cleanup").Args("blacklist", "-n", oc.Namespace()).Output()
		output, err = operatorsdkCLI.Run("run").Args("bundle", "quay.io/olmqe/blacklist-bundle:v4.11", "--index-image", "quay.io/olmqe/nginxolm-operator-index:v1", "-n", oc.Namespace(), "--timeout", "5m", "--security-context-config=restricted").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring(`Creating a File-Based Catalog of the bundle \"quay.io/olmqe/blacklist-bundle`))
		o.Expect(output).To(o.ContainSubstring(`Rendering a File-Based Catalog of the Index Image \"quay.io/olmqe/nginxolm-operator-index:v1\"`))
		o.Expect(output).To(o.ContainSubstring("OLM has successfully installed"))

		exutil.By("Run bundle with SQLite index")
		defer operatorsdkCLI.Run("cleanup").Args("k8sstatus", "-n", oc.Namespace()).Output()
		output, err = operatorsdkCLI.Run("run").Args("bundle", "quay.io/olmqe/k8sstatus-bundle:v4.11", "--index-image", "quay.io/olmqe/ditto-index:50065", "-n", oc.Namespace(), "--timeout", "5m", "--security-context-config=restricted").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("SQLite based index images are being deprecated and will be removed in a future release"))
		o.Expect(output).To(o.ContainSubstring("OLM has successfully installed"))

		exutil.By("Run bundle with index that contains the bundle message")
		defer operatorsdkCLI.Run("cleanup").Args("upgradeindex", "-n", oc.Namespace()).Output()
		_, err = operatorsdkCLI.Run("run").Args("bundle", "quay.io/olmqe/upgradeindex-bundle:v0.1", "--index-image", "quay.io/olmqe/upgradeindex-index:v0.1", "-n", oc.Namespace(), "--timeout", "5m", "--security-context-config=restricted").Output()
		o.Expect(err).To(o.HaveOccurred())
		output, _ = oc.AsAdmin().WithoutNamespace().Run("logs").Args("-n", oc.Namespace(), "quay-io-olmqe-upgradeindex-bundle-v0-1").Output()
		if !strings.Contains(output, "Bundle quay.io/olmqe/upgradeindex-bundle:v0.1 already exists, Bundle already added that provides package and csv") {
			e2e.Failf("Cannot get log Bundle quay.io/olmqe/upgradeindex-bundle:v0.1 already exists, Bundle already added that provides package and csv")
		}

	})

	// author: jitli@redhat.com
	g.It("VMonly-ConnectedOnly-Author:jitli-High-52364-SDK Run bundle support large FBC index [Serial]", func() {

		skipOnProxyCluster(oc)
		operatorsdkCLI.showInfo = true
		oc.SetupProject()
		exutil.By("Run bundle with large FBC index (size >3M)")
		defer operatorsdkCLI.Run("cleanup").Args("upgradefbc", "-n", oc.Namespace()).Output()
		output, err := operatorsdkCLI.Run("run").Args("bundle", "quay.io/olmqe/upgradefbc-bundle:v0.1", "--index-image", "quay.io/operatorhubio/catalog:latest", "-n", oc.Namespace(), "--timeout", "5m", "--security-context-config=restricted").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("OLM has successfully installed"))

		exutil.By("Run bundle with large FBC index (size >3M) that contains the bundle message")
		output, err = operatorsdkCLI.Run("run").Args("bundle", "quay.io/olmqe/upgradefbc-bundle:v0.1", "--index-image", "quay.io/olmqe/largefbcindexwithupgradefbc:v4.11", "-n", oc.Namespace(), "--timeout", "5m", "--security-context-config=restricted").Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring(`bundle \"upgradefbc.v0.0.1\" already exists in the index image`))

	})

	// author: jitli@redhat.com
	g.It("VMonly-ConnectedOnly-Author:jitli-High-52514-SDK-Run bundle upgrade support large FBC index [Slow]", func() {

		skipOnProxyCluster(oc)
		operatorsdkCLI.showInfo = true
		oc.SetupProject()
		exutil.By("Run bundle with large FBC index (size >3M)")
		defer operatorsdkCLI.Run("cleanup").Args("upgradeindex", "-n", oc.Namespace()).Output()
		output, err := operatorsdkCLI.Run("run").Args("bundle", "quay.io/olmqe/upgradeindex-bundle:v0.1", "--index-image", "quay.io/olmqe/largefbcindexwithupgradefbc:v4.11", "-n", oc.Namespace(), "--timeout", "5m", "--security-context-config=restricted").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("OLM has successfully installed"))

		exutil.By("Run bundle-upgrade operator")
		output, err = operatorsdkCLI.Run("run").Args("bundle-upgrade", "quay.io/olmqe/upgradeindex-bundle:v0.2", "-n", oc.Namespace(), "--timeout", "5m", "--security-context-config=restricted").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Successfully upgraded to"))
		waitErr := wait.PollUntilContextTimeout(context.TODO(), 15*time.Second, 360*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", "upgradeindex.v0.0.2", "-n", oc.Namespace()).Output()
			if strings.Contains(msg, "Succeeded") {
				e2e.Logf("upgrade to 0.0.2 success")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("upgradeindex upgrade failed in %s ", oc.Namespace()))

	})

	// author: jitli@redhat.com
	g.It("VMonly-ConnectedOnly-Author:jitli-High-51295-SDK-Run bundle-upgrade from bundle installation without index image [Serial]", func() {

		skipOnProxyCluster(oc)
		operatorsdkCLI.showInfo = true
		oc.SetupProject()
		exutil.By("Run bundle install operator without index image v0.1")
		defer operatorsdkCLI.Run("cleanup").Args("upgradeindex", "-n", oc.Namespace()).Output()
		output, err := operatorsdkCLI.Run("run").Args("bundle", "quay.io/olmqe/upgradeindex-bundle:v0.1", "-n", oc.Namespace(), "--timeout", "5m", "--security-context-config=restricted").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("OLM has successfully installed"))

		exutil.By("Run bundle-upgrade from the csv v.0.1->v0.2")
		output, err = operatorsdkCLI.Run("run").Args("bundle-upgrade", "quay.io/olmqe/upgradeindex-bundle:v0.2", "-n", oc.Namespace(), "--timeout", "5m", "--security-context-config=restricted").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Generated a valid Upgraded File-Based Catalog"))
		o.Expect(output).To(o.ContainSubstring("Successfully upgraded to"))

	})

	// author: jitli@redhat.com
	g.It("VMonly-ConnectedOnly-Author:jitli-High-51296-SDK-Run bundle-upgrade from bundle installation with index image [Serial]", func() {

		skipOnProxyCluster(oc)
		operatorsdkCLI.showInfo = true
		oc.SetupProject()

		exutil.By("Run bundle install operator with SQLite index image csv 0.1")
		defer operatorsdkCLI.Run("cleanup").Args("upgradeoperator", "-n", oc.Namespace()).Output()
		output, err := operatorsdkCLI.Run("run").Args("bundle", "quay.io/olmqe/upgradeoperator-bundle:v0.1", "--index-image", "quay.io/olmqe/upgradeindex-index:v0.1", "-n", oc.Namespace(), "--timeout", "5m", "--security-context-config=restricted").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("OLM has successfully installed"))

		exutil.By("Run bundle-upgrade from the csv v.0.1->v0.2")
		output, err = operatorsdkCLI.Run("run").Args("bundle-upgrade", "quay.io/olmqe/upgradeoperator-bundle:v0.2", "-n", oc.Namespace(), "--timeout", "5m", "--security-context-config=restricted").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Updated catalog source upgradeoperator-catalog"))
		o.Expect(output).To(o.ContainSubstring("Successfully upgraded"))

		exutil.By("Run bundle install operator with FBC index image")
		defer operatorsdkCLI.Run("cleanup").Args("upgradeindex", "-n", oc.Namespace()).Output()
		output, err = operatorsdkCLI.Run("run").Args("bundle", "quay.io/olmqe/upgradeindex-bundle:v0.1", "--index-image", "quay.io/olmqe/nginxolm-operator-index:v1", "-n", oc.Namespace(), "--timeout", "5m", "--security-context-config=restricted").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Generated a valid File-Based Catalog"))
		o.Expect(output).To(o.ContainSubstring("OLM has successfully installed"))

		exutil.By("Run bundle-upgrade from the csv v.0.1->v0.2")
		output, err = operatorsdkCLI.Run("run").Args("bundle-upgrade", "quay.io/olmqe/upgradeindex-bundle:v0.2", "-n", oc.Namespace(), "--timeout", "5m", "--security-context-config=restricted").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Generated a valid Upgraded File-Based Catalog"))
		o.Expect(output).To(o.ContainSubstring("Successfully upgraded"))

	})

	// author: jfan@redhat.com
	g.It("VMonly-ConnectedOnly-Author:jfan-High-44553-SDK support go type operator for http_proxy env [Slow]", func() {
		architecture.SkipNonAmd64SingleArch(oc)
		tmpBasePath := "/tmp/ocp-44553-" + getRandomString()
		tmpPath := filepath.Join(tmpBasePath, "memcached-operator-44553")
		err := os.MkdirAll(tmpPath, 0o755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(tmpBasePath)
		operatorsdkCLI.ExecCommandPath = tmpPath
		makeCLI.ExecCommandPath = tmpPath
		nsOperator := "memcached-operator-44553-system"
		imageTag := "quay.io/olmqe/memcached-operator:44553-" + getRandomString()
		dataPath := "test/extended/testdata/operatorsdk/ocp-44553-data/"
		crFilePath := filepath.Join(dataPath, "cache_v1_memcached44553.yaml")
		quayCLI := container.NewQuayCLI()
		defer quayCLI.DeleteTag(strings.Replace(imageTag, "quay.io/", "", 1))

		defer func() {
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", crFilePath, "-n", nsOperator).Output()
			exutil.By("step: undeploy")
			_, err = makeCLI.Run("undeploy").Args().Output()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()

		exutil.By("step: init Go Based Operator")
		output, err := operatorsdkCLI.Run("init").Args("--domain=httpproxy.com", "--repo=github.com/example-inc/memcached-operator").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("create api"))

		exutil.By("step: Create API.")
		output, err = operatorsdkCLI.Run("create").Args("api", "--group=cache", "--version=v1", "--kind=Memcached44553", "--resource", "--controller").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("make manifests"))

		exutil.By("step: modify files to generate the quay.io/olmqe images.")
		// update the rbac file
		rbacFilePath := filepath.Join(tmpPath, "config", "default", "manager_auth_proxy_patch.yaml")
		replaceContent(rbacFilePath, "registry.redhat.io/openshift4/ose-kube-rbac-proxy:v"+ocpversion, "quay.io/olmqe/kube-rbac-proxy:v"+ocppreversion)
		// update the Dockerfile
		dockerFilePath := filepath.Join(tmpPath, "Dockerfile")
		replaceContent(dockerFilePath, "golang:", "quay.io/olmqe/golang:")
		// update the Makefile
		makefileFilePath := filepath.Join(tmpPath, "Makefile")
		replaceContent(makefileFilePath, "controller:latest", imageTag)
		replaceContent(makefileFilePath, "build config/default | kubectl apply -f -", "build config/default | CLUSTER_PROXY=$(shell kubectl get proxies.config.openshift.io cluster  -o json | jq '.spec.httpProxy') envsubst | kubectl apply -f -")
		// copy config/manager/manager.yaml
		err = copy(filepath.Join(dataPath, "manager.yaml"), filepath.Join(tmpPath, "config", "manager", "manager.yaml"))
		o.Expect(err).NotTo(o.HaveOccurred())
		// copy controllers/memcached44553_controller.go
		err = copy(filepath.Join(dataPath, "memcached44553_controller.go"), filepath.Join(tmpPath, "controllers", "memcached44553_controller.go"))
		o.Expect(err).NotTo(o.HaveOccurred())
		// copy api/v1/memcached44553_types.go
		err = copy(filepath.Join(dataPath, "memcached44553_types.go"), filepath.Join(tmpPath, "api", "v1", "memcached44553_types.go"))
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("step: Build and push the operator image")
		tokenDir := "/tmp/ocp-44553" + getRandomString()
		err = os.MkdirAll(tokenDir, os.ModePerm)
		defer os.RemoveAll(tokenDir)
		if err != nil {
			e2e.Failf("fail to create the token folder:%s", tokenDir)
		}
		_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", fmt.Sprintf("--to=%s", tokenDir), "--confirm").Output()
		if err != nil {
			e2e.Failf("Fail to get the cluster auth %v", err)
		}

		exec.Command("bash", "-c", "cd "+tmpPath+" && go get github.com/operator-framework/operator-lib/proxy").Output()
		exec.Command("bash", "-c", "cd "+tmpPath+" && go get k8s.io/apimachinery/pkg/util/diff@v0.24.0").Output()
		exec.Command("bash", "-c", "cd "+tmpPath+" && go get github.com/evanphx/json-patch").Output()
		exec.Command("bash", "-c", "cd "+tmpPath+" && go get github.com/example-inc/memcached-operator").Output()
		exec.Command("bash", "-c", "cd "+tmpPath+" && go get github.com/google/gnostic/openapiv2").Output()

		podmanCLI := container.NewPodmanCLI()
		podmanCLI.ExecCommandPath = tmpPath
		output, err = podmanCLI.Run("build").Args(tmpPath, "--arch", "amd64", "--tag", imageTag, "--authfile", fmt.Sprintf("%s/.dockerconfigjson", tokenDir)).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Successfully"))
		output, err = podmanCLI.Run("push").Args(imageTag).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Writing manifest to image destination"))

		exutil.By("step: Deploy the operator")
		output, err = makeCLI.Run("deploy").Args("IMG=" + imageTag).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("deployment.apps/memcached-operator-44553-controller-manager"))

		waitErr := wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 180*time.Second, false, func(ctx context.Context) (bool, error) {
			podList, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", nsOperator).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			lines := strings.Split(podList, "\n")
			for _, line := range lines {
				if strings.Contains(line, "memcached-operator-44553-controller-manager") {
					e2e.Logf("found pod memcached-operator-44553-controller-manager")
					if strings.Contains(line, "2/2") {
						e2e.Logf("the status of pod memcached-operator-44553-controller-manager is Running")
						return true, nil
					}
					e2e.Logf("the status of pod memcached-operator-44553-controller-manager is not Running")
					return false, nil
				}
			}
			return false, nil
		})
		if waitErr != nil {
			logDebugInfo(oc, nsOperator, "events", "pod")
		}
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("No memcached-operator-44553-controller-manager in project %s", nsOperator))
		waitErr = wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 180*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("deployment.apps/memcached-operator-44553-controller-manager", "-c", "manager", "-n", nsOperator).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(msg, "Starting workers") {
				e2e.Logf("Starting workers successfully")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "container manager doesn't work")

		exutil.By("step: Create the resource")
		_, err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", crFilePath, "-n", nsOperator).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		waitErr = wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 180*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", nsOperator).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(msg, "memcached44553-sample") {
				e2e.Logf("found pod memcached44553-sample")
				return true, nil
			}
			return false, nil
		})
		if waitErr != nil {
			logDebugInfo(oc, nsOperator, "events", "pod")
		}
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("No memcached44553-sample in project %s", nsOperator))

		waitErr = wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 180*time.Second, false, func(ctx context.Context) (bool, error) {
			proxyMsg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("proxies.config.openshift.io", "cluster", "-o=jsonpath={.spec.httpProxy}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			msg, err := oc.AsAdmin().WithoutNamespace().Run("describe").Args("deployment/memcached44553-sample", "-n", nsOperator).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(msg, "HTTP_PROXY:  "+proxyMsg) {
				e2e.Logf("deployment/memcached44553-sample is created successfully")
				return true, nil
			}
			return false, nil
		})
		if waitErr != nil {
			logDebugInfo(oc, nsOperator, "events")
		}
		exutil.AssertWaitPollNoErr(waitErr, "the status of deployment/memcached44553-sample is wrong")
	})

	// author: jitli@redhat.com
	g.It("VMonly-ConnectedOnly-Author:jitli-High-51300-SDK Run bundle upgrade from bundle installation with multi bundles index image", func() {

		skipOnProxyCluster(oc)
		operatorsdkCLI.showInfo = true
		oc.SetupProject()

		var (
			dr                  = make(describerResrouce)
			itName              = g.CurrentSpecReport().FullText()
			buildPruningBaseDir = exutil.FixturePath("testdata", "olm")
			ogSingleTemplate    = filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
			catsrcImageTemplate = filepath.Join(buildPruningBaseDir, "catalogsource-image.yaml")
			subTemplate         = filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
			og                  = operatorGroupDescription{
				name:      "test-og-51300",
				namespace: oc.Namespace(),
				template:  ogSingleTemplate,
			}
			catsrc = catalogSourceDescription{
				name:        "upgradefbc-index-51300",
				namespace:   oc.Namespace(),
				displayName: "Test 51300 Operators",
				publisher:   "OperatorSDK QE",
				sourceType:  "grpc",
				address:     "quay.io/olmqe/upgradefbcmuli-index:v0.1",
				interval:    "15m",
				template:    catsrcImageTemplate,
			}
			sub = subscriptionDescription{
				subName:                "upgradefbc-index-51300",
				namespace:              oc.Namespace(),
				catalogSourceName:      catsrc.name,
				catalogSourceNamespace: oc.Namespace(),
				channel:                "alpha",
				ipApproval:             "Automatic",
				operatorPackage:        "upgradefbc",
				template:               subTemplate,
			}
		)
		dr.addIr(itName)

		exutil.By("Install the OperatorGroup")
		og.createwithCheck(oc, itName, dr)

		exutil.By("Create catalog source")
		catsrc.createWithCheck(oc, itName, dr)

		exutil.By("Install operator")
		sub.create(oc, itName, dr)

		exutil.By("Check Operator is Succeeded")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded", ok, []string{"csv", sub.installedCSV, "-n", oc.Namespace(), "-o=jsonpath={.status.phase}"}).check(oc)

		exutil.By("Run bundle-upgrade operator")
		output, err := operatorsdkCLI.Run("run").Args("bundle-upgrade", "quay.io/olmqe/upgradefbc-bundle:v0.0.2", "-n", oc.Namespace(), "--timeout", "5m", "--security-context-config=restricted").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Successfully upgraded to"))
		waitErr := wait.PollUntilContextTimeout(context.TODO(), 15*time.Second, 360*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", "upgradefbc.v0.0.2", "-n", oc.Namespace()).Output()
			if strings.Contains(msg, "Succeeded") {
				e2e.Logf("upgrade to 0.0.2 success")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("upgradefbc upgrade failed in %s ", oc.Namespace()))

	})

	// author: jitli@redhat.com
	g.It("VMonly-ConnectedOnly-Author:jitli-High-50141-SDK Run bundle upgrade from OLM installed operator [Slow]", func() {

		skipOnProxyCluster(oc)
		operatorsdkCLI.showInfo = true
		oc.SetupProject()

		var (
			dr                  = make(describerResrouce)
			itName              = g.CurrentSpecReport().FullText()
			buildPruningBaseDir = exutil.FixturePath("testdata", "olm")
			ogSingleTemplate    = filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
			catsrcImageTemplate = filepath.Join(buildPruningBaseDir, "catalogsource-image.yaml")
			subTemplate         = filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
			og                  = operatorGroupDescription{
				name:      "test-og-50141",
				namespace: oc.Namespace(),
				template:  ogSingleTemplate,
			}

			catsrcfbc = catalogSourceDescription{
				name:        "upgradefbc-index-50141",
				namespace:   oc.Namespace(),
				displayName: "Test 50141 Operators FBC",
				publisher:   "OperatorSDK QE",
				sourceType:  "grpc",
				address:     "quay.io/olmqe/upgradefbc-index:v0.1",
				interval:    "15m",
				template:    catsrcImageTemplate,
			}
			subfbc = subscriptionDescription{
				subName:                "upgradefbc-index-50141",
				namespace:              oc.Namespace(),
				catalogSourceName:      catsrcfbc.name,
				catalogSourceNamespace: oc.Namespace(),
				channel:                "alpha",
				ipApproval:             "Automatic",
				operatorPackage:        "upgradefbc",
				template:               subTemplate,
			}

			catsrcsqlite = catalogSourceDescription{
				name:        "upgradeindex-sqlite-50141",
				namespace:   oc.Namespace(),
				displayName: "Test 50141 Operators SQLite",
				publisher:   "OperatorSDK QE",
				sourceType:  "grpc",
				address:     "quay.io/olmqe/upgradeindex-index:v0.1",
				interval:    "15m",
				template:    catsrcImageTemplate,
			}
			subsqlite = subscriptionDescription{
				subName:                "upgradesqlite-index-50141",
				namespace:              oc.Namespace(),
				catalogSourceName:      catsrcsqlite.name,
				catalogSourceNamespace: oc.Namespace(),
				channel:                "alpha",
				ipApproval:             "Automatic",
				operatorPackage:        "upgradeindex",
				startingCSV:            "upgradeindex.v0.0.1",
				template:               subTemplate,
			}
		)
		dr.addIr(itName)

		exutil.By("Install the OperatorGroup")
		og.createwithCheck(oc, itName, dr)

		exutil.By("Create catalog source")
		catsrcfbc.createWithCheck(oc, itName, dr)

		exutil.By("Install operator through OLM and the index iamge is kind of FBC")
		subfbc.create(oc, itName, dr)

		exutil.By("Check Operator is Succeeded")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded", ok, []string{"csv", subfbc.installedCSV, "-n", oc.Namespace(), "-o=jsonpath={.status.phase}"}).check(oc)

		exutil.By("Run bundle-upgrade operator")
		output, err := operatorsdkCLI.Run("run").Args("bundle-upgrade", "quay.io/olmqe/upgradefbc-bundle:v0.0.2", "-n", oc.Namespace(), "--timeout", "5m", "--security-context-config=restricted").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Successfully upgraded to"))
		waitErr := wait.PollUntilContextTimeout(context.TODO(), 15*time.Second, 360*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", "upgradefbc.v0.0.2", "-n", oc.Namespace()).Output()
			if strings.Contains(msg, "Succeeded") {
				e2e.Logf("upgrade to 0.0.2 success")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("upgradefbc upgrade failed in %s ", oc.Namespace()))

		exutil.By("Create catalog source")
		catsrcsqlite.createWithCheck(oc, itName, dr)

		exutil.By("Install operator through OLM and the index iamge is kind of SQLITE")
		subsqlite.createWithoutCheck(oc, itName, dr)

		exutil.By("Check Operator is Succeeded")
		subsqlite.findInstalledCSV(oc, itName, dr)
		newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded", ok, []string{"csv", subsqlite.installedCSV, "-n", oc.Namespace(), "-o=jsonpath={.status.phase}"}).check(oc)

		exutil.By("Run bundle-upgrade operator")
		output, err = operatorsdkCLI.Run("run").Args("bundle-upgrade", "quay.io/olmqe/upgradeindex-bundle:v0.2", "-n", oc.Namespace(), "--timeout", "5m", "--security-context-config=restricted").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Successfully upgraded to"))
		waitErr = wait.PollUntilContextTimeout(context.TODO(), 15*time.Second, 360*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", "upgradeindex.v0.0.2", "-n", oc.Namespace()).Output()
			if strings.Contains(msg, "Succeeded") {
				e2e.Logf("upgrade to 0.0.2 success")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("upgradeindex upgrade failed in %s ", oc.Namespace()))

	})

	// author: jitli@redhat.com
	g.It("VMonly-DisconnectedOnly-Author:jitli-High-52571-Disconnected test for ansible type operator", func() {
		skipOnProxyCluster(oc)
		clusterArchitecture := architecture.SkipNonAmd64SingleArch(oc)
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "operatorsdk")
			dataPath            = filepath.Join(buildPruningBaseDir, "ocp-48885-data")
			tmpBasePath         = "/tmp/ocp-52571-" + getRandomString()
			tmpPath             = filepath.Join(tmpBasePath, "memcached-operator-52571")
			imageTag            = "quay.io/olmqe/memcached-operator:52571-" + getRandomString()
			quayCLI             = container.NewQuayCLI()
			bundleImage         = "quay.io/olmqe/memcached-operator-bundle:52571-" + getRandomString()
		)

		err := os.MkdirAll(tmpPath, 0o755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(tmpBasePath)
		operatorsdkCLI.ExecCommandPath = tmpPath
		makeCLI.ExecCommandPath = tmpPath
		defer quayCLI.DeleteTag(strings.Replace(imageTag, "quay.io/", "", 1))

		exutil.By("step: init Ansible Based Operator")
		output, err := operatorsdkCLI.Run("init").Args("--plugins=ansible", "--domain=disconnected.com").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Next"))

		exutil.By("step: Create API.")
		output, err = operatorsdkCLI.Run("create").Args("api", "--group=test", "--version=v1", "--kind", "Memcached52571", "--generate-role").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Writing kustomize manifests"))

		exutil.By("step: modify files to get the quay.io/olmqe images.")
		// copy task main.yml
		err = copy(filepath.Join(dataPath, "main.yml"), filepath.Join(tmpPath, "roles", "memcached52571", "tasks", "main.yml"))
		o.Expect(err).NotTo(o.HaveOccurred())
		// copy manager.yaml
		err = copy(filepath.Join(dataPath, "manager.yaml"), filepath.Join(tmpPath, "config", "manager", "manager.yaml"))
		o.Expect(err).NotTo(o.HaveOccurred())
		// update the Dockerfile
		dockerFile := filepath.Join(tmpPath, "Dockerfile")
		replaceContent(dockerFile, "registry.redhat.io/openshift4/ose-ansible-operator:v"+ocpversion, "brew.registry.redhat.io/rh-osbs/openshift-ose-ansible-operator:v"+ocpversion)
		replaceContent(dockerFile, "install -r ${HOME}/requirements.yml", "install -r ${HOME}/requirements.yml --force")

		exutil.By("step: Build and push the operator image")
		tokenDir := "/tmp/ocp-52571" + getRandomString()
		err = os.MkdirAll(tokenDir, os.ModePerm)
		defer os.RemoveAll(tokenDir)
		if err != nil {
			e2e.Failf("fail to create the token folder:%s", tokenDir)
		}
		_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", fmt.Sprintf("--to=%s", tokenDir), "--confirm").Output()
		if err != nil {
			e2e.Failf("Fail to get the cluster auth %v", err)
		}

		// update the Makefile
		makefileFilePath := filepath.Join(tmpPath, "Makefile")
		replaceContent(makefileFilePath, "controller:latest", imageTag)
		replaceContent(makefileFilePath, "operator-sdk generate bundle -q --overwrite --version $(VERSION) $(BUNDLE_METADATA_OPTS)", "operator-sdk generate bundle $(BUNDLE_GEN_FLAGS)")
		// update the rbac file
		rbacFilePath := filepath.Join(tmpPath, "config", "default", "manager_auth_proxy_patch.yaml")
		replaceContent(rbacFilePath, "registry.redhat.io/openshift4/ose-kube-rbac-proxy:v"+ocpversion, "quay.io/olmqe/kube-rbac-proxy:v"+ocppreversion)
		if upstream {
			replaceContent(rbacFilePath, "gcr.io/kubebuilder/kube-rbac-proxy:", "quay.io/olmqe/kube-rbac-proxy:v"+ocppreversion+" #")
		}

		// copy manifests
		manifestsPath := filepath.Join(tmpPath, "config", "manifests", "bases")
		err = os.MkdirAll(manifestsPath, 0o755)
		o.Expect(err).NotTo(o.HaveOccurred())
		manifestsFile := filepath.Join(manifestsPath, "memcached-operator-52571.clusterserviceversion.yaml")
		_, err = os.Create(manifestsFile)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = copy(filepath.Join(dataPath, "memcached-operator-52571.clusterserviceversion.yaml"), filepath.Join(manifestsFile))
		o.Expect(err).NotTo(o.HaveOccurred())

		buildPushOperatorImage(clusterArchitecture, tmpPath, imageTag, tokenDir)

		exutil.By("step: make bundle use image digests")
		waitErr := wait.PollUntilContextTimeout(context.TODO(), 30*time.Second, 120*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, err := makeCLI.Run("bundle").Args("USE_IMAGE_DIGESTS=true").Output()
			if err != nil {
				e2e.Logf("make bundle failed, try again")
				return false, nil
			}
			if strings.Contains(msg, "operator-sdk bundle validate ./bundle") {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "operator-sdk bundle generate failed")
		csvFile := filepath.Join(tmpPath, "bundle", "manifests", "memcached-operator-52571.clusterserviceversion.yaml")
		content := getContent(csvFile)
		if !strings.Contains(content, "quay.io/olmqe/memcached@sha256:") || !strings.Contains(content, "kube-rbac-proxy@sha256:") || !strings.Contains(content, "quay.io/olmqe/memcached-operator@sha256:") {
			e2e.Failf("Fail to get the image info with digest type")
		}

		exutil.By("step: build and push bundle image.")
		defer quayCLI.DeleteTag(strings.Replace(bundleImage, "quay.io/", "", 1))
		_, err = makeCLI.Run("bundle-build").Args("BUNDLE_IMG=" + bundleImage).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		podmanCLI := container.NewPodmanCLI()

		waitErr = wait.PollUntilContextTimeout(context.TODO(), 30*time.Second, 60*time.Second, false, func(ctx context.Context) (bool, error) {
			output, _ = podmanCLI.Run("push").Args(bundleImage).Output()
			if strings.Contains(output, "Writing manifest to image destination") {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "Podman push bundle image failed.")

		exutil.By("step: create new project")
		oc.SetupProject()
		ns := oc.Namespace()

		exutil.By("step: get digestID")
		bundleImageDigest, err := quayCLI.GetImageDigest(strings.Replace(bundleImage, "quay.io/", "", 1))
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(bundleImageDigest).NotTo(o.BeEmpty())

		indexImage := "quay.io/olmqe/nginxolm-operator-index:v1"
		indexImageDigest, err := quayCLI.GetImageDigest(strings.Replace(indexImage, "quay.io/", "", 1))
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(indexImageDigest).NotTo(o.BeEmpty())

		exutil.By("step: run bundle")
		defer func() {
			output, err = operatorsdkCLI.Run("cleanup").Args("memcached-operator-52571", "-n", ns).Output()
			if err != nil {
				e2e.Logf(output)
				o.Expect(err).NotTo(o.HaveOccurred())
			}
		}()

		output, err = operatorsdkCLI.Run("run").Args("bundle", "quay.io/olmqe/memcached-operator-bundle@"+bundleImageDigest, "--index-image", "quay.io/olmqe/nginxolm-operator-index@"+indexImageDigest, "-n", ns, "--timeout", "5m", "--security-context-config=restricted", "--decompression-image", "quay.io/olmqe/busybox@sha256:e39d9c8ac4963d0b00a5af08678757b44c35ea8eb6be0cdfbeb1282e7f7e6003").Output()
		if err != nil {
			logDebugInfo(oc, ns, "csv", "pod", "ip")
		}
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("OLM has successfully installed"))

		waitErr = wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 120*time.Second, false, func(ctx context.Context) (bool, error) {
			podList, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", ns).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			lines := strings.Split(podList, "\n")
			for _, line := range lines {
				if strings.Contains(line, "memcached-operator-52571-controller-manager") {
					e2e.Logf("found pod memcached-operator-52571-controller-manager")
					if strings.Contains(line, "Running") {
						e2e.Logf("the status of pod memcached-operator-52571-controller-manager is Running")
						return true, nil
					}
					e2e.Logf("the status of pod memcached-operator-52571-controller-manager is not Running")
					return false, nil
				}
			}
			return false, nil
		})
		if waitErr != nil {
			logDebugInfo(oc, ns, "pod", "csv", "catsrc")
		}
		output, err = oc.WithoutNamespace().AsAdmin().Run("get").Args("pods", "-o=jsonpath={.items[*].spec.containers[*].image}", "-n", ns).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("kube-rbac-proxy@sha256:"))
		o.Expect(output).To(o.ContainSubstring("quay.io/olmqe/memcached-operator@sha256:"))

		exutil.By("step: Create CR")
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "add-scc-to-user", "anyuid", fmt.Sprintf("system:serviceaccount:%s:memcached52571-sample-nginx", ns)).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		crFilePath := filepath.Join(dataPath, "memcached-sample.yaml")
		defer func() {
			exutil.By("step: delete cr.")
			_, err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", crFilePath, "-n", ns).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()

		_, err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", crFilePath, "-n", ns).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		waitErr = wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 120*time.Second, false, func(ctx context.Context) (bool, error) {
			podList, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", ns).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			lines := strings.Split(podList, "\n")
			for _, line := range lines {
				if strings.Contains(line, "memcached52571-sample") {
					e2e.Logf("found pod memcached52571-sample")
					if strings.Contains(line, "Running") {
						e2e.Logf("the status of pod memcached52571-sample is Running")
						return true, nil
					}
					e2e.Logf("the status of pod memcached52571-sample is not Running")
					return false, nil
				}
			}
			return false, nil
		})
		if waitErr != nil {
			logDebugInfo(oc, ns, "Memcached52571", "pod", "events")
		}
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("No memcached52571-sample in project %s or the pod is not running", ns))

	})

	// author: jitli@redhat.com
	g.It("VMonly-DisconnectedOnly-Author:jitli-High-52572-Disconnected test for helm type operator", func() {
		skipOnProxyCluster(oc)
		clusterArchitecture := architecture.SkipArchitectures(oc, architecture.MULTI, architecture.PPC64LE, architecture.S390X)
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "operatorsdk")
			dataPath            = filepath.Join(buildPruningBaseDir, "ocp-52813-data")
			tmpBasePath         = "/tmp/ocp-52572-" + getRandomString()
			tmpPath             = filepath.Join(tmpBasePath, "memcached-operator-52572")
			imageTag            = "quay.io/olmqe/memcached-operator:52572-" + getRandomString()
			quayCLI             = container.NewQuayCLI()
			bundleImage         = "quay.io/olmqe/memcached-operator-bundle:52572-" + getRandomString()
		)
		err := os.MkdirAll(tmpPath, 0o755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(tmpBasePath)
		operatorsdkCLI.ExecCommandPath = tmpPath
		makeCLI.ExecCommandPath = tmpPath
		defer quayCLI.DeleteTag(strings.Replace(imageTag, "quay.io/", "", 1))

		exutil.By("step: init Helm Based Operator")
		output, err := operatorsdkCLI.Run("init").Args("--plugins=helm", "--domain=disconnected.com", "--group=test", "--version=v1", "--kind=Nginx").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Created helm-charts"))

		exutil.By("step: modify files to get the quay.io/olmqe images.")
		// copy watches.yaml
		err = copy(filepath.Join(dataPath, "watches.yaml"), filepath.Join(tmpPath, "watches.yaml"))
		o.Expect(err).NotTo(o.HaveOccurred())
		// copy helm-charts/nginx/values.yaml
		err = copy(filepath.Join(dataPath, "values.yaml"), filepath.Join(tmpPath, "helm-charts", "nginx", "values.yaml"))
		o.Expect(err).NotTo(o.HaveOccurred())
		// update the file helm-charts/nginx/templates/deployment.yaml
		deployFilepath := filepath.Join(tmpPath, "helm-charts", "nginx", "templates", "deployment.yaml")
		replaceContent(deployFilepath, ".Values.image.repository }}:{{ .Values.image.tag | default .Chart.AppVersion", ".Values.relatedImage")
		// update the Dockerfile
		dockerFile := filepath.Join(tmpPath, "Dockerfile")
		replaceContent(dockerFile, "registry.redhat.io/openshift4/ose-helm-operator:v"+ocpversion, "brew.registry.redhat.io/rh-osbs/openshift-ose-helm-operator:v"+ocpversion)
		// copy the manager
		err = copy(filepath.Join(dataPath, "manager.yaml"), filepath.Join(tmpPath, "config", "manager", "manager.yaml"))
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("step: Build and push the operator image")
		tokenDir := "/tmp/ocp-52572" + getRandomString()
		err = os.MkdirAll(tokenDir, os.ModePerm)
		defer os.RemoveAll(tokenDir)
		if err != nil {
			e2e.Failf("fail to create the token folder:%s", tokenDir)
		}
		_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", fmt.Sprintf("--to=%s", tokenDir), "--confirm").Output()
		if err != nil {
			e2e.Failf("Fail to get the cluster auth %v", err)
		}
		buildPushOperatorImage(clusterArchitecture, tmpPath, imageTag, tokenDir)

		// update the Makefile
		makefileFilePath := filepath.Join(tmpPath, "Makefile")
		replaceContent(makefileFilePath, "controller:latest", imageTag)
		replaceContent(makefileFilePath, "operator-sdk generate bundle -q --overwrite --version $(VERSION) $(BUNDLE_METADATA_OPTS)", "operator-sdk generate bundle $(BUNDLE_GEN_FLAGS)")
		// update the rbac file
		rbacFilePath := filepath.Join(tmpPath, "config", "default", "manager_auth_proxy_patch.yaml")
		replaceContent(rbacFilePath, "registry.redhat.io/openshift4/ose-kube-rbac-proxy:v"+ocpversion, "quay.io/olmqe/kube-rbac-proxy:v"+ocppreversion)
		if upstream {
			replaceContent(rbacFilePath, "gcr.io/kubebuilder/kube-rbac-proxy:", "quay.io/olmqe/kube-rbac-proxy:v"+ocppreversion+" #")
		}

		exutil.By("step: make bundle.")
		// copy manifests
		manifestsPath := filepath.Join(tmpPath, "config", "manifests", "bases")
		err = os.MkdirAll(manifestsPath, 0o755)
		o.Expect(err).NotTo(o.HaveOccurred())
		manifestsFile := filepath.Join(manifestsPath, "memcached-operator-52572.clusterserviceversion.yaml")
		_, err = os.Create(manifestsFile)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = copy(filepath.Join(dataPath, "memcached-operator-52572.clusterserviceversion.yaml"), filepath.Join(manifestsFile))
		o.Expect(err).NotTo(o.HaveOccurred())
		// make bundle use image digests
		waitErr := wait.PollUntilContextTimeout(context.TODO(), 30*time.Second, 120*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, err := makeCLI.Run("bundle").Args("USE_IMAGE_DIGESTS=true").Output()
			if err != nil {
				e2e.Logf("make bundle failed, try again")
				return false, nil
			}
			if strings.Contains(msg, "operator-sdk bundle validate ./bundle") {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "operator-sdk bundle generate failed")
		csvFile := filepath.Join(tmpPath, "bundle", "manifests", "memcached-operator-52572.clusterserviceversion.yaml")
		content := getContent(csvFile)
		if !strings.Contains(content, "quay.io/olmqe/nginx@sha256:") || !strings.Contains(content, "kube-rbac-proxy@sha256:") || !strings.Contains(content, "quay.io/olmqe/memcached-operator@sha256:") {
			e2e.Failf("Fail to get the image info with digest type")
		}

		exutil.By("step: build and push bundle image.")
		defer quayCLI.DeleteTag(strings.Replace(bundleImage, "quay.io/", "", 1))
		_, err = makeCLI.Run("bundle-build").Args("BUNDLE_IMG=" + bundleImage).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		podmanCLI := container.NewPodmanCLI()

		waitErr = wait.PollUntilContextTimeout(context.TODO(), 30*time.Second, 60*time.Second, false, func(ctx context.Context) (bool, error) {
			output, _ = podmanCLI.Run("push").Args(bundleImage).Output()
			if strings.Contains(output, "Writing manifest to image destination") {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "Podman push bundle image failed.")

		exutil.By("step: create new project")
		oc.SetupProject()
		ns := oc.Namespace()

		exutil.By("step: get digestID")
		bundleImageDigest, err := quayCLI.GetImageDigest(strings.Replace(bundleImage, "quay.io/", "", 1))
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(bundleImageDigest).NotTo(o.BeEmpty())

		indexImage := "quay.io/olmqe/nginxolm-operator-index:v1"
		indexImageDigest, err := quayCLI.GetImageDigest(strings.Replace(indexImage, "quay.io/", "", 1))
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(indexImageDigest).NotTo(o.BeEmpty())

		exutil.By("step: run bundle")

		defer func() {
			output, err = operatorsdkCLI.Run("cleanup").Args("memcached-operator-52572", "-n", ns).Output()
			if err != nil {
				e2e.Logf(output)
				o.Expect(err).NotTo(o.HaveOccurred())
			}
		}()

		output, err = operatorsdkCLI.Run("run").Args("bundle", "quay.io/olmqe/memcached-operator-bundle@"+bundleImageDigest, "--index-image", "quay.io/olmqe/nginxolm-operator-index@"+indexImageDigest, "-n", ns, "--timeout", "5m", "--security-context-config=restricted", "--decompression-image", "quay.io/olmqe/busybox@sha256:e39d9c8ac4963d0b00a5af08678757b44c35ea8eb6be0cdfbeb1282e7f7e6003").Output()
		if err != nil {
			logDebugInfo(oc, ns, "csv", "pod", "ip")
		}
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("OLM has successfully installed"))

		waitErr = wait.PollUntilContextTimeout(context.TODO(), 30*time.Second, 120*time.Second, false, func(ctx context.Context) (bool, error) {
			podList, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", ns).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			lines := strings.Split(podList, "\n")
			for _, line := range lines {
				if strings.Contains(line, "memcached-operator-52572-controller-manager") {
					e2e.Logf("found pod memcached-operator-52572-controller-manager")
					if strings.Contains(line, "Running") {
						e2e.Logf("the status of pod memcached-operator-52572-controller-manager is Running")
						return true, nil
					}
					e2e.Logf("the status of pod memcached-operator-52572-controller-manager is not Running")
					return false, nil
				}
			}
			return false, nil
		})
		if waitErr != nil {
			logDebugInfo(oc, ns, "pod", "csv", "catsrc")
		}
		output, err = oc.WithoutNamespace().AsAdmin().Run("get").Args("pods", "-o=jsonpath={.items[*].spec.containers[*].image}", "-n", ns).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("kube-rbac-proxy@sha256:"))
		o.Expect(output).To(o.ContainSubstring("quay.io/olmqe/memcached-operator@sha256:"))

		exutil.By("step: Create CR")
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "add-scc-to-user", "anyuid", fmt.Sprintf("system:serviceaccount:%s:memcached52572-sample-nginx", ns)).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		crFilePath := filepath.Join(dataPath, "memcached-sample.yaml")

		defer func() {
			exutil.By("step: delete cr.")
			_, err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", crFilePath, "-n", ns).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		_, err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", crFilePath, "-n", ns).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		waitErr = wait.PollUntilContextTimeout(context.TODO(), 30*time.Second, 180*time.Second, false, func(ctx context.Context) (bool, error) {
			podList, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", ns).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			lines := strings.Split(podList, "\n")
			for _, line := range lines {
				if strings.Contains(line, "memcached52572-sample") {
					e2e.Logf("found pod memcached52572-sample")
					if strings.Contains(line, "Running") {
						e2e.Logf("the status of pod memcached52572-sample is Running")
						return true, nil
					}
					e2e.Logf("the status of pod memcached52572-sample is not Running")
					return false, nil
				}
			}
			return false, nil
		})
		if waitErr != nil {
			logDebugInfo(oc, ns, "Memcached52572", "pod", "events")
		}
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("No memcached52572-sample in project %s or the pod is not running", ns))

	})

	// author: jitli@redhat.com
	g.It("VMonly-DisconnectedOnly-Author:jitli-High-52305-Disconnected test for go type operator [Slow]", func() {
		skipOnProxyCluster(oc)
		clusterArchitecture := architecture.SkipArchitectures(oc, architecture.MULTI, architecture.PPC64LE, architecture.S390X)
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "operatorsdk")
			dataPath            = filepath.Join(buildPruningBaseDir, "ocp-52814-data")
			tmpBasePath         = "/tmp/ocp-52305-" + getRandomString()
			tmpPath             = filepath.Join(tmpBasePath, "memcached-operator-52814")
			imageTag            = "quay.io/olmqe/memcached-operator:52305-" + getRandomString()
			quayCLI             = container.NewQuayCLI()
			bundleImage         = "quay.io/olmqe/memcached-operator-bundle:52305-" + getRandomString()
		)

		err := os.MkdirAll(tmpPath, 0o755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(tmpBasePath)
		operatorsdkCLI.ExecCommandPath = tmpPath
		makeCLI.ExecCommandPath = tmpPath
		defer quayCLI.DeleteTag(strings.Replace(imageTag, "quay.io/", "", 1))

		exutil.By("step: init Go Based Operator")
		output, err := operatorsdkCLI.Run("init").Args("--domain=disconnected.com", "--repo=github.com/example-inc/memcached-operator").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Next"))

		exutil.By("step: create api")
		output, err = operatorsdkCLI.Run("create").Args("api", "--group=test", "--version=v1", "--kind=Memcached52814", "--controller", "--resource").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("make manifests"))

		exutil.By("step: modify files to get the quay.io/olmqe images.")
		// copy api/v1/memcached52814_types.go
		err = copy(filepath.Join(dataPath, "memcached52814_types.go"), filepath.Join(tmpPath, "api", "v1", "memcached52814_types.go"))
		o.Expect(err).NotTo(o.HaveOccurred())
		// copy controllers/memcached52814_controller.go
		err = copy(filepath.Join(dataPath, "memcached52814_controller.go"), filepath.Join(tmpPath, "controllers", "memcached52814_controller.go"))
		o.Expect(err).NotTo(o.HaveOccurred())
		// copy the manager
		err = copy(filepath.Join(dataPath, "manager.yaml"), filepath.Join(tmpPath, "config", "manager", "manager.yaml"))
		o.Expect(err).NotTo(o.HaveOccurred())
		// update the Dockerfile
		dockerfileFilePath := filepath.Join(tmpPath, "Dockerfile")
		replaceContent(dockerfileFilePath, "golang", "quay.io/olmqe/golang")

		exutil.By("step: Build and push the operator image")
		tokenDir := "/tmp/ocp-52305" + getRandomString()
		err = os.MkdirAll(tokenDir, os.ModePerm)
		defer os.RemoveAll(tokenDir)
		if err != nil {
			e2e.Failf("fail to create the token folder:%s", tokenDir)
		}
		_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", fmt.Sprintf("--to=%s", tokenDir), "--confirm").Output()
		if err != nil {
			e2e.Failf("Fail to get the cluster auth %v", err)
		}
		buildPushOperatorImage(clusterArchitecture, tmpPath, imageTag, tokenDir)

		// update the Makefile
		makefileFilePath := filepath.Join(tmpPath, "Makefile")
		replaceContent(makefileFilePath, "controller:latest", imageTag)
		// update the rbac file
		rbacFilePath := filepath.Join(tmpPath, "config", "default", "manager_auth_proxy_patch.yaml")
		replaceContent(rbacFilePath, "registry.redhat.io/openshift4/ose-kube-rbac-proxy:v"+ocpversion, "quay.io/olmqe/kube-rbac-proxy:v"+ocppreversion)
		if upstream {
			replaceContent(rbacFilePath, "gcr.io/kubebuilder/kube-rbac-proxy:", "quay.io/olmqe/kube-rbac-proxy:v"+ocppreversion+" #")
		}

		exutil.By("step: Install kustomize")
		kustomizePath := "/root/kustomize"
		binPath := filepath.Join(tmpPath, "bin")
		exec.Command("bash", "-c", fmt.Sprintf("cp %s %s", kustomizePath, binPath)).Output()

		exutil.By("step: make bundle.")
		// copy manifests
		manifestsPath := filepath.Join(tmpPath, "config", "manifests", "bases")
		err = os.MkdirAll(manifestsPath, 0o755)
		o.Expect(err).NotTo(o.HaveOccurred())
		manifestsFile := filepath.Join(manifestsPath, "memcached-operator-52814.clusterserviceversion.yaml")
		_, err = os.Create(manifestsFile)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = copy(filepath.Join(dataPath, "memcached-operator-52814.clusterserviceversion.yaml"), filepath.Join(manifestsFile))
		o.Expect(err).NotTo(o.HaveOccurred())
		// make bundle use image digests
		waitErr := wait.PollUntilContextTimeout(context.TODO(), 30*time.Second, 120*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, err := makeCLI.Run("bundle").Args("USE_IMAGE_DIGESTS=true").Output()
			if err != nil {
				e2e.Logf("make bundle failed, try again")
				return false, nil
			}
			if strings.Contains(msg, "operator-sdk bundle validate ./bundle") {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "operator-sdk bundle generate failed")
		csvFile := filepath.Join(tmpPath, "bundle", "manifests", "memcached-operator-52814.clusterserviceversion.yaml")
		content := getContent(csvFile)
		if !strings.Contains(content, "quay.io/olmqe/memcached@sha256:") || !strings.Contains(content, "kube-rbac-proxy@sha256:") || !strings.Contains(content, "quay.io/olmqe/memcached-operator@sha256:") {
			e2e.Failf("Fail to get the image info with digest type")
		}

		exutil.By("step: build and push bundle image.")
		defer quayCLI.DeleteTag(strings.Replace(bundleImage, "quay.io/", "", 1))
		_, err = makeCLI.Run("bundle-build").Args("BUNDLE_IMG=" + bundleImage).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		podmanCLI := container.NewPodmanCLI()

		waitErr = wait.PollUntilContextTimeout(context.TODO(), 30*time.Second, 60*time.Second, false, func(ctx context.Context) (bool, error) {
			output, _ = podmanCLI.Run("push").Args(bundleImage).Output()
			if strings.Contains(output, "Writing manifest to image destination") {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "Podman push bundle image failed.")

		exutil.By("step: create new project")
		oc.SetupProject()
		ns := oc.Namespace()

		exutil.By("step: get digestID")
		bundleImageDigest, err := quayCLI.GetImageDigest(strings.Replace(bundleImage, "quay.io/", "", 1))
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(bundleImageDigest).NotTo(o.BeEmpty())

		indexImage := "quay.io/olmqe/nginxolm-operator-index:v1"
		indexImageDigest, err := quayCLI.GetImageDigest(strings.Replace(indexImage, "quay.io/", "", 1))
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(indexImageDigest).NotTo(o.BeEmpty())

		exutil.By("step: run bundle")
		defer func() {
			output, err = operatorsdkCLI.Run("cleanup").Args("memcached-operator-52814", "-n", ns).Output()
			if err != nil {
				e2e.Logf(output)
				o.Expect(err).NotTo(o.HaveOccurred())
			}
		}()

		output, err = operatorsdkCLI.Run("run").Args("bundle", "quay.io/olmqe/memcached-operator-bundle@"+bundleImageDigest, "--index-image", "quay.io/olmqe/nginxolm-operator-index@"+indexImageDigest, "-n", ns, "--timeout", "5m", "--security-context-config=restricted", "--decompression-image", "quay.io/olmqe/busybox@sha256:e39d9c8ac4963d0b00a5af08678757b44c35ea8eb6be0cdfbeb1282e7f7e6003").Output()
		if err != nil {
			logDebugInfo(oc, ns, "csv", "pod", "ip")
		}
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("OLM has successfully installed"))

		waitErr = wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 120*time.Second, false, func(ctx context.Context) (bool, error) {
			podList, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", ns).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			lines := strings.Split(podList, "\n")
			for _, line := range lines {
				if strings.Contains(line, "memcached-operator-52814-controller-manager") {
					e2e.Logf("found pod memcached-operator-52814-controller-manager")
					if strings.Contains(line, "Running") {
						e2e.Logf("the status of pod memcached-operator-52814-controller-manager is Running")
						return true, nil
					}
					e2e.Logf("the status of pod memcached-operator-52814-controller-manager is not Running")
					return false, nil
				}
			}
			return false, nil
		})
		if waitErr != nil {
			logDebugInfo(oc, ns, "pod", "csv", "catsrc")
		}
		output, err = oc.WithoutNamespace().AsAdmin().Run("get").Args("pods", "-o=jsonpath={.items[*].spec.containers[*].image}", "-n", ns).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("kube-rbac-proxy@sha256:"))
		o.Expect(output).To(o.ContainSubstring("quay.io/olmqe/memcached-operator@sha256:"))

		exutil.By("step: Create CR")
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "add-scc-to-user", "anyuid", fmt.Sprintf("system:serviceaccount:%s:memcached52305-sample", ns)).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		crFilePath := filepath.Join(dataPath, "memcached-sample.yaml")
		defer func() {
			exutil.By("step: delete cr.")
			_, err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", crFilePath, "-n", ns).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()

		_, err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", crFilePath, "-n", ns).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		waitErr = wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 120*time.Second, false, func(ctx context.Context) (bool, error) {
			podList, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", ns).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			lines := strings.Split(podList, "\n")
			for _, line := range lines {
				if strings.Contains(line, "memcached52305-sample") {
					e2e.Logf("found pod memcached52305-sample")
					if strings.Contains(line, "Running") {
						e2e.Logf("the status of pod memcached52305-sample is Running")
						return true, nil
					}
					e2e.Logf("the status of pod memcached52305-sample is not Running")
					return false, nil
				}
			}
			return false, nil
		})
		if waitErr != nil {
			logDebugInfo(oc, ns, "Memcached52814", "pod", "events")
		}
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("No memcached52305-sample in project %s or the pod is not running", ns))

	})

	// author: jfan@redhat.com
	g.It("VMonly-ConnectedOnly-Author:jfan-High-45141-High-41497-High-34292-High-29374-High-28157-High-27977-ansible k8sevent k8sstatus maxConcurrentReconciles modules to a collect blacklist [Slow]", func() {
		clusterArchitecture := architecture.SkipNonAmd64SingleArch(oc)
		// test data
		buildPruningBaseDir := exutil.FixturePath("testdata", "operatorsdk")
		dataPath := filepath.Join(buildPruningBaseDir, "ocp-27977-data")
		crFilePath := filepath.Join(dataPath, "ansiblebase_v1_basetest.yaml")
		// exec dir
		tmpBasePath := "/tmp/ocp-27977-" + getRandomString()
		tmpPath := filepath.Join(tmpBasePath, "ansibletest")
		operatorsdkCLI.ExecCommandPath = tmpPath
		makeCLI.ExecCommandPath = tmpPath
		// exec ns & image tag
		nsOperator := "ansibletest-system"
		imageTag := "quay.io/olmqe/ansibletest:" + ocpversion + "-" + getRandomString()
		// cleanup the test data
		err := os.MkdirAll(tmpPath, 0o755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(tmpBasePath)
		quayCLI := container.NewQuayCLI()
		defer quayCLI.DeleteTag(strings.Replace(imageTag, "quay.io/", "", 1))

		defer func() {
			_, err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", crFilePath, "-n", nsOperator).Output()
			exutil.By("step: undeploy")
			_, err = makeCLI.Run("undeploy").Args().Output()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()

		exutil.By("step: init Ansible Based Operator")
		output, err := operatorsdkCLI.Run("init").Args("--plugins=ansible", "--domain", "qetest.com").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Next"))

		exutil.By("step: Create API.")
		output, err = operatorsdkCLI.Run("create").Args("api", "--group", "ansiblebase", "--version", "v1", "--kind", "Basetest", "--generate-role").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Writing kustomize manifests"))

		exutil.By("step: modify files to get the quay.io/olmqe images.")
		// copy task main.yml
		err = copy(filepath.Join(dataPath, "main.yml"), filepath.Join(tmpPath, "roles", "basetest", "tasks", "main.yml"))
		o.Expect(err).NotTo(o.HaveOccurred())
		// copy Dockerfile
		dockerfileFilePath := filepath.Join(dataPath, "Dockerfile")
		err = copy(dockerfileFilePath, filepath.Join(tmpPath, "Dockerfile"))
		o.Expect(err).NotTo(o.HaveOccurred())
		replaceContent(filepath.Join(tmpPath, "Dockerfile"), "brew.registry.redhat.io/rh-osbs/openshift-ose-ansible-operator:vocpversion", "brew.registry.redhat.io/rh-osbs/openshift-ose-ansible-operator:v"+ocpversion)
		// copy manager_auth_proxy_patch.yaml
		authFilePath := filepath.Join(tmpPath, "config", "default", "manager_auth_proxy_patch.yaml")
		err = copy(filepath.Join(dataPath, "manager_auth_proxy_patch.yaml"), authFilePath)
		o.Expect(err).NotTo(o.HaveOccurred())
		replaceContent(authFilePath, "registry.redhat.io/openshift4/ose-kube-rbac-proxy:vocpversion", "quay.io/olmqe/kube-rbac-proxy:v"+ocppreversion)
		// copy manager.yaml
		err = copy(filepath.Join(dataPath, "manager.yaml"), filepath.Join(tmpPath, "config", "manager", "manager.yaml"))
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("step: Build and push the operator image")
		tokenDir := "/tmp/ocp-27977" + getRandomString()
		err = os.MkdirAll(tokenDir, os.ModePerm)
		defer os.RemoveAll(tokenDir)
		if err != nil {
			e2e.Failf("fail to create the token folder:%s", tokenDir)
		}
		_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", fmt.Sprintf("--to=%s", tokenDir), "--confirm").Output()
		if err != nil {
			e2e.Failf("Fail to get the cluster auth %v", err)
		}
		buildPushOperatorImage(clusterArchitecture, tmpPath, imageTag, tokenDir)

		exutil.By("step: Install the CRD")
		output, err = makeCLI.Run("install").Args().Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("basetests.ansiblebase.qetest.com"))

		exutil.By("step: Deploy the operator")
		output, err = makeCLI.Run("deploy").Args("IMG=" + imageTag).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("deployment.apps/ansibletest-controller-manager"))

		waitErr := wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 300*time.Second, false, func(ctx context.Context) (bool, error) {
			podList, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", nsOperator).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			lines := strings.Split(podList, "\n")
			for _, line := range lines {
				if strings.Contains(line, "ansibletest-controller-manager") {
					e2e.Logf("found pod ansibletest-controller-manager")
					if strings.Contains(line, "2/2") {
						e2e.Logf("the status of pod ansibletest-controller-manager is Running")
						return true, nil
					}
					e2e.Logf("the status of pod ansibletest-controller-manager is not Running")
					return false, nil
				}
			}
			return false, nil
		})
		if waitErr != nil {
			logDebugInfo(oc, nsOperator, "events", "pod")
		}
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("No ansibletest-controller-manager in project %s", nsOperator))
		msg, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("deployment.apps/ansibletest-controller-manager", "-c", "manager", "-n", nsOperator).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(msg, "Starting workers") {
			e2e.Failf("Starting workers failed")
		}

		// OCP-34292
		waitErr = wait.PollUntilContextTimeout(context.TODO(), 5*time.Second, 180*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, _ := oc.AsAdmin().WithoutNamespace().Run("logs").Args("deploy/ansibletest-controller-manager", "-c", "manager", "-n", nsOperator).Output()
			if strings.Contains(msg, "\"worker count\":1") {
				e2e.Logf("found worker count:1")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("log of deploy/ansibletest-controller-manager of %s doesn't have worker count:4", nsOperator))

		// add the admin policy
		err = oc.AsAdmin().Run("adm").Args("policy", "add-cluster-role-to-user", "cluster-admin", "system:serviceaccount:"+nsOperator+":ansibletest-controller-manager").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("step: Create the resource")
		_, err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", crFilePath, "-n", nsOperator).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		waitErr = wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 300*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", nsOperator).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(msg, "basetest-sample") {
				e2e.Logf("found pod basetest-sample")
				return true, nil
			}
			return false, nil
		})
		if waitErr != nil {
			logDebugInfo(oc, nsOperator, "events", "pod")
		}
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("No basetest-sample in project %s", nsOperator))

		// OCP-27977
		waitErr = wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 180*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, err = oc.AsAdmin().WithoutNamespace().Run("describe").Args("deployment/basetest-sample", "-n", nsOperator).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(msg, "2 desired | 2 updated | 2 total | 2 available | 0 unavailable") {
				e2e.Logf("deployment/basetest-sample is created successfully")
				return true, nil
			}
			return false, nil
		})
		if waitErr != nil {
			logDebugInfo(oc, nsOperator, "events")
		}
		exutil.AssertWaitPollNoErr(waitErr, "the status of deployment/basetest-sample is wrong")

		// OCP-45141
		waitErr = wait.PollUntilContextTimeout(context.TODO(), 5*time.Second, 180*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("event", "-n", nsOperator).Output()
			if strings.Contains(msg, "test-reason") {
				e2e.Logf("k8s_event test")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("can't get k8s event test-name in %s", nsOperator))

		// OCP-41497
		waitErr = wait.PollUntilContextTimeout(context.TODO(), 5*time.Second, 180*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("basetest.ansiblebase.qetest.com/basetest-sample", "-n", nsOperator, "-o", "yaml").Output()
			if strings.Contains(msg, "hello world") {
				e2e.Logf("k8s_status test hello world")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("can't get basetest-sample hello world in %s", nsOperator))

		// OCP-29374
		waitErr = wait.PollUntilContextTimeout(context.TODO(), 5*time.Second, 180*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret", "-n", nsOperator).Output()
			if strings.Contains(msg, "test-secret") {
				e2e.Logf("found secret test-secret")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("doesn't get secret test-secret %s", nsOperator))
		msg, err = oc.AsAdmin().WithoutNamespace().Run("describe").Args("secret", "test-secret", "-n", nsOperator).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(msg).To(o.ContainSubstring("test:  6 bytes"))

		// OCP-28157
		waitErr = wait.PollUntilContextTimeout(context.TODO(), 5*time.Second, 300*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, _ := oc.AsAdmin().WithoutNamespace().Run("describe").Args("configmap", "test-blacklist-watches", "-n", nsOperator).Output()
			if strings.Contains(msg, "afdasdfsajsafj") {
				e2e.Logf("Skipping the blacklist")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("log of deploy/ansibletest-controller-manager of %s doesn't work the blacklist", nsOperator))
	})

	// author: jfan@redhat.com
	g.It("VMonly-ConnectedOnly-Author:jfan-High-28586-ansible Content Collections Support in watches.yaml", func() {
		clusterArchitecture := architecture.SkipNonAmd64SingleArch(oc)
		// test data
		buildPruningBaseDir := exutil.FixturePath("testdata", "operatorsdk")
		dataPath := filepath.Join(buildPruningBaseDir, "ocp-28586-data")
		crFilePath := filepath.Join(dataPath, "cache5_v1_collectiontest.yaml")
		// exec dir
		tmpBasePath := "/tmp/ocp-28586-" + getRandomString()
		tmpPath := filepath.Join(tmpBasePath, "contentcollections")
		operatorsdkCLI.ExecCommandPath = tmpPath
		makeCLI.ExecCommandPath = tmpPath
		// exec ns & image tag
		nsOperator := "contentcollections-system"
		imageTag := "quay.io/olmqe/contentcollections:" + ocpversion + "-" + getRandomString()
		// cleanup the test data
		err := os.MkdirAll(tmpPath, 0o755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(tmpBasePath)
		quayCLI := container.NewQuayCLI()
		defer quayCLI.DeleteTag(strings.Replace(imageTag, "quay.io/", "", 1))

		defer func() {
			_, err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", crFilePath, "-n", nsOperator).Output()
			exutil.By("step: undeploy")
			_, err = makeCLI.Run("undeploy").Args().Output()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()

		exutil.By("step: init Ansible Based Operator")
		output, err := operatorsdkCLI.Run("init").Args("--plugins=ansible", "--domain", "cotentcollect.com").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Next"))

		exutil.By("step: Create API.")
		output, err = operatorsdkCLI.Run("create").Args("api", "--group", "cache5", "--version", "v1", "--kind", "CollectionTest", "--generate-role").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Writing kustomize manifests"))

		exutil.By("step: modify files to get the quay.io/olmqe images.")
		// mkdir fixture_collection
		collectionFilePath := filepath.Join(tmpPath, "fixture_collection", "roles", "dummy", "tasks")
		err = os.MkdirAll(collectionFilePath, 0o755)
		o.Expect(err).NotTo(o.HaveOccurred())
		// copy galaxy.yml & main.yml
		err = copy(filepath.Join(dataPath, "galaxy.yml"), filepath.Join(tmpPath, "fixture_collection", "galaxy.yml"))
		o.Expect(err).NotTo(o.HaveOccurred())
		err = copy(filepath.Join(dataPath, "main.yml"), filepath.Join(collectionFilePath, "main.yml"))
		o.Expect(err).NotTo(o.HaveOccurred())
		// copy Dockerfile
		dockerfileFilePath := filepath.Join(dataPath, "Dockerfile")
		err = copy(dockerfileFilePath, filepath.Join(tmpPath, "Dockerfile"))
		o.Expect(err).NotTo(o.HaveOccurred())
		replaceContent(filepath.Join(tmpPath, "Dockerfile"), "brew.registry.redhat.io/rh-osbs/openshift-ose-ansible-operator:vocpversion", "brew.registry.redhat.io/rh-osbs/openshift-ose-ansible-operator:v"+ocpversion)
		// copy the watches.yaml
		err = copy(filepath.Join(dataPath, "watches.yaml"), filepath.Join(tmpPath, "watches.yaml"))
		o.Expect(err).NotTo(o.HaveOccurred())
		// update the rbac file
		rbacFilePath := filepath.Join(tmpPath, "config", "default", "manager_auth_proxy_patch.yaml")
		replaceContent(rbacFilePath, "registry.redhat.io/openshift4/ose-kube-rbac-proxy:v"+ocpversion, "quay.io/olmqe/kube-rbac-proxy:v"+ocppreversion)

		exutil.By("step: Build and push the operator image")
		tokenDir := "/tmp/ocp-28586" + getRandomString()
		err = os.MkdirAll(tokenDir, os.ModePerm)
		defer os.RemoveAll(tokenDir)
		if err != nil {
			e2e.Failf("fail to create the token folder:%s", tokenDir)
		}
		_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", fmt.Sprintf("--to=%s", tokenDir), "--confirm").Output()
		if err != nil {
			e2e.Failf("Fail to get the cluster auth %v", err)
		}
		buildPushOperatorImage(clusterArchitecture, tmpPath, imageTag, tokenDir)

		exutil.By("step: Install the CRD")
		output, err = makeCLI.Run("install").Args().Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("collectiontests.cache5.cotentcollect.com"))

		exutil.By("step: Deploy the operator")
		output, err = makeCLI.Run("deploy").Args("IMG=" + imageTag).Output()
		o.Expect(output).To(o.ContainSubstring("deployment.apps/contentcollections-controller-manager"))

		waitErr := wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 240*time.Second, false, func(ctx context.Context) (bool, error) {
			podMsg, _ := oc.AsAdmin().WithoutNamespace().Run("describe").Args("pods", "-n", nsOperator).Output()
			if !strings.Contains(podMsg, "Started container manager") {
				e2e.Logf("Started container manager failed")
				logDebugInfo(oc, nsOperator, "events", "pod")
				return false, nil
			}
			return true, nil
		})
		if waitErr != nil {
			logDebugInfo(oc, nsOperator, "events", "pod")
		}

		waitErr = wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 240*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, _ := oc.AsAdmin().WithoutNamespace().Run("logs").Args("deployment.apps/contentcollections-controller-manager", "-c", "manager", "-n", nsOperator).Output()
			if !strings.Contains(msg, "Starting workers") {
				e2e.Logf("Starting workers failed")
				return false, nil
			}
			return true, nil
		})
		if waitErr != nil {
			logDebugInfo(oc, nsOperator, "events", "pod")
		}
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("No contentcollections-controller-manager in project %s", nsOperator))

		exutil.By("step: Create the resource")
		msg, err := oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", crFilePath, "-n", nsOperator).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(msg, "collectiontest-sample created") {
			e2e.Failf("collectiontest-sample created failed")
		}

		// check the dummy task
		waitErr = wait.PollUntilContextTimeout(context.TODO(), 15*time.Second, 360*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, _ := oc.AsAdmin().WithoutNamespace().Run("logs").Args("deploy/contentcollections-controller-manager", "-c", "manager", "-n", nsOperator).Output()
			if strings.Contains(msg, "dummy : Create ConfigMap") {
				e2e.Logf("found dummy : Create ConfigMap")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("miss log dummy : Create ConfigMap in %s", nsOperator))
	})

	// author: jfan@redhat.com
	g.It("VMonly-ConnectedOnly-Author:jfan-High-48366-add ansible prometheus metrics", func() {
		clusterArchitecture := architecture.SkipNonAmd64SingleArch(oc)
		// test data
		buildPruningBaseDir := exutil.FixturePath("testdata", "operatorsdk")
		dataPath := filepath.Join(buildPruningBaseDir, "ocp-48366-data")
		crFilePath := filepath.Join(dataPath, "metrics_v1_testmetrics.yaml")
		// exec dir
		tmpBasePath := "/tmp/ocp-48366-" + getRandomString()
		tmpPath := filepath.Join(tmpBasePath, "ansiblemetrics")
		operatorsdkCLI.ExecCommandPath = tmpPath
		makeCLI.ExecCommandPath = tmpPath
		// exec ns & image tag
		nsOperator := "ansiblemetrics-system"
		imageTag := "quay.io/olmqe/testmetrics:" + ocpversion + "-" + getRandomString()
		// cleanup the test data
		err := os.MkdirAll(tmpPath, 0o755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(tmpBasePath)
		quayCLI := container.NewQuayCLI()
		defer quayCLI.DeleteTag(strings.Replace(imageTag, "quay.io/", "", 1))

		defer func() {
			_, err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", crFilePath, "-n", nsOperator).Output()
			exutil.By("step: undeploy")
			_, err = makeCLI.Run("undeploy").Args().Output()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()

		exutil.By("step: init Ansible metrics Operator")
		output, err := operatorsdkCLI.Run("init").Args("--plugins=ansible", "--domain", "testmetrics.com").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Next"))

		exutil.By("step: Create API.")
		output, err = operatorsdkCLI.Run("create").Args("api", "--group", "metrics", "--version", "v1", "--kind", "Testmetrics", "--generate-role").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Writing kustomize manifests"))

		exutil.By("step: modify files to get the quay.io/olmqe images.")

		// copy Dockerfile
		dockerfileFilePath := filepath.Join(tmpPath, "Dockerfile")
		err = copy(filepath.Join(dataPath, "Dockerfile"), dockerfileFilePath)
		o.Expect(err).NotTo(o.HaveOccurred())
		replaceContent(dockerfileFilePath, "brew.registry.redhat.io/rh-osbs/openshift-ose-ansible-operator:vocpversion", "brew.registry.redhat.io/rh-osbs/openshift-ose-ansible-operator:v"+ocpversion)
		// copy the roles/testmetrics/tasks/main.yml
		err = copy(filepath.Join(dataPath, "main.yml"), filepath.Join(tmpPath, "roles", "testmetrics", "tasks", "main.yml"))
		o.Expect(err).NotTo(o.HaveOccurred())
		// update the rbac file
		rbacFilePath := filepath.Join(tmpPath, "config", "default", "manager_auth_proxy_patch.yaml")
		replaceContent(rbacFilePath, "registry.redhat.io/openshift4/ose-kube-rbac-proxy:v"+ocpversion, "quay.io/olmqe/kube-rbac-proxy:v"+ocppreversion)

		exutil.By("step: Build and push the operator image")
		tokenDir := "/tmp/ocp-48366" + getRandomString()
		err = os.MkdirAll(tokenDir, os.ModePerm)
		defer os.RemoveAll(tokenDir)
		if err != nil {
			e2e.Failf("fail to create the token folder:%s", tokenDir)
		}
		_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", fmt.Sprintf("--to=%s", tokenDir), "--confirm").Output()
		if err != nil {
			e2e.Failf("Fail to get the cluster auth %v", err)
		}
		buildPushOperatorImage(clusterArchitecture, tmpPath, imageTag, tokenDir)

		exutil.By("step: Install the CRD")
		output, err = makeCLI.Run("install").Args().Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("metrics"))

		exutil.By("step: Deploy the operator")
		output, err = makeCLI.Run("deploy").Args("IMG=" + imageTag).Output()
		o.Expect(output).To(o.ContainSubstring("deployment.apps/ansiblemetrics-controller-manager"))

		waitErr := wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 300*time.Second, false, func(ctx context.Context) (bool, error) {
			podList, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", nsOperator).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			lines := strings.Split(podList, "\n")
			for _, line := range lines {
				if strings.Contains(line, "ansiblemetrics-controller-manager") {
					if strings.Contains(line, "2/2") {
						e2e.Logf("the status of pod ansiblemetrics-controller-manager is Running")
						return true, nil
					}
					e2e.Logf("the status of pod ansiblemetrics-controller-manager is not Running")
					return false, nil
				}
			}
			return false, nil
		})
		if waitErr != nil {
			logDebugInfo(oc, nsOperator, "events", "pod")
		}

		waitErr = wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 300*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, _ := oc.AsAdmin().WithoutNamespace().Run("logs").Args("deployment.apps/ansiblemetrics-controller-manager", "-c", "manager", "-n", nsOperator).Output()
			if !strings.Contains(msg, "Starting workers") {
				e2e.Logf("Starting workers failed")
				return false, nil
			}
			return true, nil
		})
		if waitErr != nil {
			logDebugInfo(oc, nsOperator, "events", "pod")
		}
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("No ansiblemetrics-controller-manager in project %s", nsOperator))

		exutil.By("step: Create the resource")
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", crFilePath, "-n", nsOperator).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		waitErr = wait.PollUntilContextTimeout(context.TODO(), 15*time.Second, 360*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", nsOperator).Output()
			if strings.Contains(msg, "metrics-sample") {
				e2e.Logf("metrics created success")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("can't get metrics samples in %s", nsOperator))
		metricsToken, _ := exutil.GetSAToken(oc)
		o.Expect(metricsToken).NotTo(o.BeEmpty())

		promeEp, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ep", "ansiblemetrics-controller-manager-metrics-service", "-o=jsonpath={.subsets[0].addresses[0].ip}", "-n", nsOperator).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		metricsMsg, err := exec.Command("bash", "-c", "oc exec deployment/ansiblemetrics-controller-manager -n "+nsOperator+" -- curl -k -H \"Authorization: Bearer "+metricsToken+"\" 'https://["+promeEp+"]:8443/metrics'").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		var strMetricsMsg string
		strMetricsMsg = string(metricsMsg)
		if !strings.Contains(strMetricsMsg, "my gague and set it to 2") {
			e2e.Logf("%s", strMetricsMsg)
			e2e.Failf("my gague and set it to 2 failed")
		}
		if !strings.Contains(strMetricsMsg, "counter") {
			e2e.Logf("%s", strMetricsMsg)
			e2e.Failf("counter failed")
		}
		if !strings.Contains(strMetricsMsg, "Observe my histogram") {
			e2e.Logf("%s", strMetricsMsg)
			e2e.Failf("Observe my histogram failed")
		}
		if !strings.Contains(strMetricsMsg, "Observe my summary") {
			e2e.Logf("%s", strMetricsMsg)
			e2e.Failf("Observe my summary failed")
		}
	})

	// author: jfan@redhat.com
	g.It("VMonly-ConnectedOnly-Author:jfan-Medium-48359-SDK init plugin about hybird helm operator [Slow]", func() {
		clusterArchitecture := architecture.SkipNonAmd64SingleArch(oc)
		tmpBasePath := "/tmp/ocp-48359-" + getRandomString()
		tmpPath := filepath.Join(tmpBasePath, "memcached-operator-48359")
		err := os.MkdirAll(tmpPath, 0o755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(tmpBasePath)
		operatorsdkCLI.ExecCommandPath = tmpPath
		makeCLI.ExecCommandPath = tmpPath
		nsOperator := "memcached-operator-48359-system"
		imageTag := "quay.io/olmqe/memcached-operator:48359-" + getRandomString()
		dataPath := "test/extended/testdata/operatorsdk/ocp-48359-data/"
		crFilePath := filepath.Join(dataPath, "cache6_v1_memcachedbackup.yaml")
		quayCLI := container.NewQuayCLI()
		defer quayCLI.DeleteTag(strings.Replace(imageTag, "quay.io/", "", 1))

		defer func() {
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", crFilePath, "-n", nsOperator).Output()
			exutil.By("step: undeploy")
			_, err = makeCLI.Run("undeploy").Args().Output()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()

		exutil.By("step: init Helm Based Operator")
		output, err := operatorsdkCLI.Run("init").Args("--plugins=hybrid.helm.sdk.operatorframework.io", "--domain=hybird.com", "--project-version=3", "--repo=github.com/example/memcached-operator").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("go mod tidy"))

		exutil.By("step: create the apis")
		// Create helm api
		output, err = operatorsdkCLI.Run("create").Args("api", "--plugins=helm.sdk.operatorframework.io/v1", "--group=cache6", "--version=v1", "--kind=Memcached").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Created helm-charts"))
		// Create go api
		output, err = operatorsdkCLI.Run("create").Args("api", "--group=cache6", "--version=v1", "--kind=MemcachedBackup", "--resource", "--controller", "--plugins=go/v3").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("make manifests"))

		exutil.By("step: modify files to generate the operator image.")
		// update the rbac file
		rbacFilePath := filepath.Join(tmpPath, "config", "default", "manager_auth_proxy_patch.yaml")
		replaceContent(rbacFilePath, "registry.redhat.io/openshift4/ose-kube-rbac-proxy:v"+ocpversion, "quay.io/olmqe/kube-rbac-proxy:v"+ocppreversion)
		// update the Dockerfile
		dockerFilePath := filepath.Join(tmpPath, "Dockerfile")
		replaceContent(dockerFilePath, "golang:", "quay.io/olmqe/golang:")
		// update the Makefile
		makefileFilePath := filepath.Join(tmpPath, "Makefile")
		replaceContent(makefileFilePath, "controller:latest", imageTag)
		replaceContent(makefileFilePath, "docker build -t ${IMG}", "docker build -t ${IMG} .")
		// copy memcachedbackup_types.go
		err = copy(filepath.Join(dataPath, "memcachedbackup_types.go"), filepath.Join(tmpPath, "api", "v1", "memcachedbackup_types.go"))
		o.Expect(err).NotTo(o.HaveOccurred())
		// copy memcachedbackup_controller.go ./controllers/memcachedbackup_controller.go
		err = copy(filepath.Join(dataPath, "memcachedbackup_controller.go"), filepath.Join(tmpPath, "controllers", "memcachedbackup_controller.go"))
		o.Expect(err).NotTo(o.HaveOccurred())
		// copy kubmize
		exutil.By("step: Install kustomize")
		kustomizePath := "/root/kustomize"
		binPath := filepath.Join(tmpPath, "bin", "kustomize")
		err = copy(filepath.Join(kustomizePath), filepath.Join(binPath))
		o.Expect(err).NotTo(o.HaveOccurred())
		// chmod 644 watches.yaml
		err = os.Chmod(filepath.Join(tmpPath, "watches.yaml"), 0o644)

		exutil.By("step: Build and push the operator image")
		tokenDir := "/tmp/ocp-48359" + getRandomString()
		err = os.MkdirAll(tokenDir, os.ModePerm)
		defer os.RemoveAll(tokenDir)
		if err != nil {
			e2e.Failf("fail to create the token folder:%s", tokenDir)
		}
		_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", fmt.Sprintf("--to=%s", tokenDir), "--confirm").Output()
		if err != nil {
			e2e.Failf("Fail to get the cluster auth %v", err)
		}
		buildPushOperatorImage(clusterArchitecture, tmpPath, imageTag, tokenDir)

		exutil.By("step: Deploy the operator")
		output, err = makeCLI.Run("deploy").Args("IMG=" + imageTag).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("deployment.apps/memcached-operator-48359-controller-manager"))

		waitErr := wait.PollUntilContextTimeout(context.TODO(), 30*time.Second, 180*time.Second, false, func(ctx context.Context) (bool, error) {
			podList, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", nsOperator).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			lines := strings.Split(podList, "\n")
			for _, line := range lines {
				if strings.Contains(line, "memcached-operator-48359-controller-manager") {
					e2e.Logf("found pod memcached-operator-48359-controller-manager")
					if strings.Contains(line, "2/2") {
						e2e.Logf("the status of pod memcached-operator-48359-controller-manager is Running")
						return true, nil
					}
					e2e.Logf("the status of pod memcached-operator-48359-controller-manager is not Running")
					return false, nil
				}
			}
			return false, nil
		})
		if waitErr != nil {
			logDebugInfo(oc, nsOperator, "events", "pod")
		}
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("No memcached-operator-48359-controller-manager in project %s", nsOperator))
		waitErr = wait.PollUntilContextTimeout(context.TODO(), 30*time.Second, 180*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("deployment.apps/memcached-operator-48359-controller-manager", "-c", "manager", "-n", nsOperator).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(msg, "Starting workers") {
				e2e.Logf("Starting workers successfully")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "container manager doesn't work")
		if waitErr != nil {
			logDebugInfo(oc, nsOperator, "events", "pod")
		}

		exutil.By("step: Create the resource")
		_, err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", crFilePath, "-n", nsOperator).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		waitErr = wait.PollUntilContextTimeout(context.TODO(), 30*time.Second, 180*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", nsOperator).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(msg, "memcachedbackup-sample") {
				e2e.Logf("found pod memcachedbackup-sample")
				return true, nil
			}
			return false, nil
		})
		if waitErr != nil {
			logDebugInfo(oc, nsOperator, "events", "pod")
		}
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("hybird test: No memcachedbackup-sample in project %s", nsOperator))
	})

	// author: jitli@redhat.com
	g.It("VMonly-ConnectedOnly-Author:jitli-High-40964-migrate packagemanifest to bundle", func() {
		skipOnProxyCluster(oc)
		architecture.SkipNonAmd64SingleArch(oc)
		var (
			tmpBasePath           = "/tmp/ocp-40964-" + getRandomString()
			pacakagemanifestsPath = exutil.FixturePath("testdata", "operatorsdk", "ocp-40964-data", "manifests", "etcd")
			quayCLI               = container.NewQuayCLI()
			containerCLI          = container.NewPodmanCLI()
			bundleImage           = "quay.io/olmqe/etcd-operatorsdk:0.9.4"
			bundleImageTag        = "quay.io/olmqe/etcd-operatorsdk:0.9.4-" + getRandomString()
		)
		oc.SetupProject()
		operatorsdkCLI.showInfo = true
		defer os.RemoveAll(tmpBasePath)
		err := os.MkdirAll(tmpBasePath, 0o755)
		o.Expect(err).NotTo(o.HaveOccurred())
		operatorsdkCLI.ExecCommandPath = tmpBasePath

		exutil.By("transfer the packagemanifest to bundle dir")
		output, err := operatorsdkCLI.Run("pkgman-to-bundle").Args(pacakagemanifestsPath, "--output-dir=test-40964-bundle").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Bundle metadata generated successfully"))

		bundleDir := filepath.Join(tmpBasePath, "test-40964-bundle", "bundle-0.9.4", "bundle")
		if _, err := os.Stat(bundleDir); os.IsNotExist(err) {
			e2e.Failf("bundle dir not found")
		}

		exutil.By("generate the bundle image")
		output, err = operatorsdkCLI.Run("pkgman-to-bundle").Args(pacakagemanifestsPath, "--image-tag-base", "quay.io/olmqe/etcd-operatorsdk", "--output-dir=test-40964-bundle-image").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Successfully built image quay.io/olmqe/etcd-operatorsdk:0.9.4"))

		exutil.By("run the generated bundle")
		defer quayCLI.DeleteTag(strings.Replace(bundleImageTag, "quay.io/", "", 1))
		defer containerCLI.RemoveImage(bundleImage)

		if output, err := containerCLI.Run("tag").Args(bundleImage, bundleImageTag).Output(); err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		if output, err = containerCLI.Run("push").Args(bundleImageTag).Output(); err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		defer func() {
			output, err := operatorsdkCLI.Run("cleanup").Args("etcd", "-n", oc.Namespace()).Output()
			if err != nil {
				e2e.Logf(output)
				o.Expect(err).NotTo(o.HaveOccurred())
			}
		}()

		output, _ = operatorsdkCLI.Run("run").Args("bundle", bundleImageTag, "-n", oc.Namespace(), "--timeout", "5m", "--security-context-config=restricted").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("OLM has successfully installed"))

	})

	// author: jitli@redhat.com
	g.It("VMonly-Author:jitli-Critical-49884-Add support for external bundle validators", func() {

		tmpPath := "/tmp/ocp-49884-" + getRandomString()
		err := os.MkdirAll(tmpPath, 0o755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(tmpPath)

		tmpBasePath := exutil.FixturePath("testdata", "operatorsdk", "ocp-49960-data")
		exutil.By("Get the external bundle validator")
		command := "wget -P " + tmpPath + " http://virt-openshift-05.lab.eng.nay.redhat.com/jitli/testdata/validator-poc"
		wgetOutput, err := exec.Command("bash", "-c", command).Output()
		if err != nil {
			e2e.Logf(string(wgetOutput))
			e2e.Failf("Fail to wget the validator-poc %v", err)
		}
		exvalidator := filepath.Join(tmpPath, "validator-poc")
		waitErr := wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 30*time.Second, false, func(ctx context.Context) (bool, error) {
			if _, err := os.Stat(exvalidator); os.IsNotExist(err) {
				e2e.Logf("get validator-poc Failed")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "get validator-poc Failed")

		err = os.Chmod(exvalidator, os.FileMode(0o755))
		o.Expect(err).NotTo(o.HaveOccurred())
		updatedFileInfo, err := os.Stat(exvalidator)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(updatedFileInfo.Mode().Perm()).To(o.Equal(os.FileMode(0o755)), "File permissions not set correctly")

		exutil.By("bundle validate with external validater")
		output, _ := operatorsdkCLI.Run("bundle").Args("validate", tmpBasePath+"/bundle", "--alpha-select-external", exvalidator).Output()
		o.Expect(output).To(o.ContainSubstring("csv.Spec.Icon elements should contain both data and mediatype"))

		exutil.By("bundle validate with 2 external validater")
		output, _ = operatorsdkCLI.Run("bundle").Args("validate", tmpBasePath+"/bundle", "--alpha-select-external", exvalidator+":"+exvalidator).Output()
		o.Expect(output).To(o.ContainSubstring("csv.Spec.Icon elements should contain both data and mediatype"))

	})

	// author: jitli@redhat.com
	g.It("VMonly-ConnectedOnly-Author:jitli-Critical-59885-Run bundle on different security level namespaces [Serial]", func() {

		skipOnProxyCluster(oc)
		operatorsdkCLI.showInfo = true
		oc.SetupProject()
		defer func() {
			output, err := operatorsdkCLI.Run("cleanup").Args("upgradeoperator", "-n", oc.Namespace()).Output()
			if err != nil {
				e2e.Logf(output)
				o.Expect(err).NotTo(o.HaveOccurred())
			}
		}()
		exutil.By("Run bundle without options --security-context-config=restricted")
		output, _ := operatorsdkCLI.Run("run").Args("bundle", "quay.io/olmqe/upgradeoperator-bundle:v0.1", "-n", oc.Namespace(), "--timeout", "5m").Output()
		if strings.Contains(output, "violates PodSecurity") {

			e2e.Logf("violates PodSecurity, need add label to decrease namespace safety factor")
			output, err := operatorsdkCLI.Run("cleanup").Args("upgradeoperator", "-n", oc.Namespace()).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).To(o.ContainSubstring("uninstalled"))

			exutil.By("Add label to decrease namespace safety factor")
			_, err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", oc.Namespace(), "security.openshift.io/scc.podSecurityLabelSync=false", "pod-security.kubernetes.io/enforce=privileged", "pod-security.kubernetes.io/audit=privileged", "pod-security.kubernetes.io/warn=privileged", "--overwrite").Output()
			o.Expect(err).NotTo(o.HaveOccurred())

			exutil.By("Run bundle without options --security-context-config=restricted again")
			output, _ = operatorsdkCLI.Run("run").Args("bundle", "quay.io/olmqe/upgradeoperator-bundle:v0.1", "-n", oc.Namespace(), "--timeout", "5m").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).To(o.ContainSubstring("OLM has successfully installed"))
		} else {
			o.Expect(output).To(o.ContainSubstring("OLM has successfully installed"))
		}

	})

	// author: jitli@redhat.com
	g.It("ConnectedOnly-VMonly-Author:jitli-High-69005-helm operator recoilne the different namespaces", func() {
		clusterArchitecture := architecture.SkipArchitectures(oc, architecture.MULTI, architecture.PPC64LE, architecture.S390X)
		imageTag := "quay.io/olmqe/nginx-operator-base:v" + ocpversion + "-69005" + getRandomString()
		nsSystem := "system-69005-" + getRandomString()
		nsOperator := "nginx-operator-69005-system"

		tmpBasePath := "/tmp/ocp-69005-" + getRandomString()
		tmpPath := filepath.Join(tmpBasePath, "nginx-operator-69005")
		err := os.MkdirAll(tmpPath, 0o755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(tmpBasePath)
		operatorsdkCLI.ExecCommandPath = tmpPath
		makeCLI.ExecCommandPath = tmpPath

		defer func() {
			quayCLI := container.NewQuayCLI()
			quayCLI.DeleteTag(strings.Replace(imageTag, "quay.io/", "", 1))
		}()

		exutil.By("init Helm Based Operators")
		output, err := operatorsdkCLI.Run("init").Args("--plugins=helm").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Next: define a resource with"))

		exutil.By("Create API.")
		output, err = operatorsdkCLI.Run("create").Args("api", "--group", "demo", "--version", "v1", "--kind", "Nginx69005").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("nginx"))

		if !upstream {
			dockerFile := filepath.Join(tmpPath, "Dockerfile")
			content := getContent(dockerFile)
			o.Expect(content).To(o.ContainSubstring("registry.redhat.io/openshift4/ose-helm-operator:v" + ocpversion))
			replaceContent(dockerFile, "registry.redhat.io/openshift4/ose-helm-operator:v"+ocpversion, "brew.registry.redhat.io/rh-osbs/openshift-ose-helm-operator:v"+ocpversion)

			managerAuthProxyPatch := filepath.Join(tmpPath, "config", "default", "manager_auth_proxy_patch.yaml")
			content = getContent(managerAuthProxyPatch)
			o.Expect(content).To(o.ContainSubstring("registry.redhat.io/openshift4/ose-kube-rbac-proxy:v" + ocpversion))
			replaceContent(managerAuthProxyPatch, "registry.redhat.io/openshift4/ose-kube-rbac-proxy:v"+ocpversion, "registry.redhat.io/openshift4/ose-kube-rbac-proxy:v"+ocppreversion)
		}

		exutil.By("modify namespace")
		exec.Command("bash", "-c", fmt.Sprintf("sed -i 's/name: system/name: %s/g' `grep -rl \"name: system\" %s`", nsSystem, tmpPath)).Output()
		exec.Command("bash", "-c", fmt.Sprintf("sed -i 's/namespace: system/namespace: %s/g'  `grep -rl \"namespace: system\" %s`", nsSystem, tmpPath)).Output()
		exec.Command("bash", "-c", fmt.Sprintf("sed -i 's/namespace: nginx-operator-69005-system/namespace: %s/g'  `grep -rl \"namespace: nginx-operator-system\" %s`", nsOperator, tmpPath)).Output()

		exutil.By("build and Push the operator image")
		tokenDir := "/tmp/ocp-69005" + getRandomString()
		err = os.MkdirAll(tokenDir, os.ModePerm)
		defer os.RemoveAll(tokenDir)
		if err != nil {
			e2e.Failf("fail to create the token folder:%s", tokenDir)
		}
		_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", fmt.Sprintf("--to=%s", tokenDir), "--confirm").Output()
		if err != nil {
			e2e.Failf("Fail to get the cluster auth %v", err)
		}
		buildPushOperatorImage(clusterArchitecture, tmpPath, imageTag, tokenDir)

		defer func() {
			exutil.By("run make undeploy")
			_, err = makeCLI.Run("undeploy").Args().Output()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()

		exutil.By("Edit manager.yaml to add the multiple namespaces")
		managerFilePath := filepath.Join(tmpPath, "config", "manager", "manager.yaml")
		replaceContent(managerFilePath, "name: manager", "name: manager\n        env:\n          - name: \"WATCH_NAMESPACE\"\n            value: default,nginx-operator-69005-system")

		exutil.By("Deploy the operator")
		output, err = makeCLI.Run("deploy").Args("IMG=" + imageTag).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("deployment.apps/nginx-operator-69005-controller-manager created"))
		waitErr := wait.PollUntilContextTimeout(context.TODO(), 30*time.Second, 180*time.Second, false, func(ctx context.Context) (bool, error) {
			podList, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", nsOperator).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			lines := strings.Split(podList, "\n")
			for _, line := range lines {
				if strings.Contains(line, "nginx-operator-69005-controller-manager") {
					e2e.Logf("found pod nginx-operator-69005-controller-manager")
					if strings.Contains(line, "Running") {
						e2e.Logf("the status of pod nginx-operator-69005-controller-manager is Running")
						return true, nil
					}
					e2e.Logf("the status of pod nginx-operator-69005-controller-manager is not Running")
					return false, nil
				}
			}
			return false, nil
		})
		if err != nil {
			logDebugInfo(oc, nsOperator, "events", "pod")
		}
		exutil.AssertWaitPollNoErr(waitErr, "No nginx-operator-69005-controller-manager")

		exutil.By("Check the namespaces watching")
		podName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", nsOperator, "-o=jsonpath={.items..metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(podName).NotTo(o.BeEmpty())
		podLogs, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args(podName, "-n", nsOperator, "--limit-bytes", "50000").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(podLogs).To(o.ContainSubstring("multiple"))

		exutil.By("run make undeploy")
		_, err = makeCLI.Run("undeploy").Args().Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Edit manager.yaml to add the single namespaces")
		managerFilePath = filepath.Join(tmpPath, "config", "manager", "manager.yaml")
		replaceContent(managerFilePath, "default,nginx-operator-69005-system", "nginx-operator-69005-system")

		exutil.By("Deploy the operator")
		output, err = makeCLI.Run("deploy").Args("IMG=" + imageTag).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("deployment.apps/nginx-operator-69005-controller-manager created"))
		waitErr = wait.PollUntilContextTimeout(context.TODO(), 30*time.Second, 180*time.Second, false, func(ctx context.Context) (bool, error) {
			podList, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", nsOperator).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			lines := strings.Split(podList, "\n")
			for _, line := range lines {
				if strings.Contains(line, "nginx-operator-69005-controller-manager") {
					e2e.Logf("found pod nginx-operator-69005-controller-manager")
					if strings.Contains(line, "Running") {
						e2e.Logf("the status of pod nginx-operator-69005-controller-manager is Running")
						return true, nil
					}
					e2e.Logf("the status of pod nginx-operator-69005-controller-manager is not Running")
					return false, nil
				}
			}
			return false, nil
		})
		if err != nil {
			logDebugInfo(oc, nsOperator, "events", "pod")
		}
		exutil.AssertWaitPollNoErr(waitErr, "No nginx-operator-69005-controller-manager")

		exutil.By("Check the namespaces watching")
		podName, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", nsOperator, "-o=jsonpath={.items..metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(podName).NotTo(o.BeEmpty())
		podLogs, err = oc.AsAdmin().WithoutNamespace().Run("logs").Args(podName, "-n", nsOperator, "--limit-bytes", "50000").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(podLogs).To(o.ContainSubstring("single"))

	})

	// author: jitli@redhat.com
	g.It("VMonly-ConnectedOnly-Author:jitli-Medium-69118-Run bundle init image choosable [Serial]", func() {

		skipOnProxyCluster(oc)
		operatorsdkCLI.showInfo = true
		oc.SetupProject()
		defer func() {
			output, err := operatorsdkCLI.Run("cleanup").Args("upgradeoperator", "-n", oc.Namespace()).Output()
			if err != nil {
				e2e.Logf(output)
				o.Expect(err).NotTo(o.HaveOccurred())
			}
		}()
		exutil.By("Run bundle")
		output, err := operatorsdkCLI.Run("run").Args("bundle", "quay.io/olmqe/upgradeoperator-bundle:v0.1", "-n", oc.Namespace(), "--timeout", "5m", "--security-context-config=restricted").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("OLM has successfully installed"))

		exutil.By("Check the default init image")
		output, err = oc.WithoutNamespace().AsAdmin().Run("get").Args("pod", "quay-io-olmqe-upgradeoperator-bundle-v0-1", "-o=jsonpath={.spec.initContainers[*].image}", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("docker.io/library/busybox:1.36.0"))

		output, err = operatorsdkCLI.Run("cleanup").Args("upgradeoperator", "-n", oc.Namespace()).Output()
		if err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		exutil.By("Customize the initialization image to run the bundle")
		output, err = operatorsdkCLI.Run("run").Args("bundle", "quay.io/olmqe/upgradeoperator-bundle:v0.1", "-n", oc.Namespace(), "--timeout", "5m", "--security-context-config=restricted", "--decompression-image", "quay.io/olmqe/busybox:latest").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("OLM has successfully installed"))

		exutil.By("Check the custom init image")
		output, err = oc.WithoutNamespace().AsAdmin().Run("get").Args("pod", "quay-io-olmqe-upgradeoperator-bundle-v0-1", "-o=jsonpath={.spec.initContainers[*].image}", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("quay.io/olmqe/busybox:latest"))

	})

})
