package imageregistry

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/openshift/openshift-tests-private/test/extended/util/architecture"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	container "github.com/openshift/openshift-tests-private/test/extended/util/container"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-imageregistry] Image_Registry", func() {
	defer g.GinkgoRecover()
	var (
		oc                   = exutil.NewCLI("default-image-registry", exutil.KubeConfigPath())
		errInfo              = "http.response.status=404"
		ApiServerInfo        = "unexpected error when reading response body. Please retry. Original error: read tcp"
		logInfo              = `Unsupported value: "abc": supported values: "", "Normal", "Debug", "Trace", "TraceAll"`
		updatePolicy         = `"maxSurge":0,"maxUnavailable":"10%"`
		patchAuthURL         = `"authURL":"invalid"`
		patchRegion          = `"regionName":"invaild"`
		patchDomain          = `"domain":"invaild"`
		patchDomainID        = `"domainID":"invalid"`
		patchTenantID        = `"tenantID":"invalid"`
		authErrInfo          = `Get "invalid/": unsupported`
		regionErrInfo        = "No suitable endpoint could be found"
		domainErrInfo        = "Failed to authenticate provider client"
		domainIDErrInfo      = "You must provide exactly one of DomainID or DomainName"
		tenantIDErrInfo      = "Authentication failed"
		imageRegistryBaseDir = exutil.FixturePath("testdata", "image_registry")
		requireRules         = "requiredDuringSchedulingIgnoredDuringExecution"
	)

	g.BeforeEach(func() {
		var message string
		if !checkOptionalOperatorInstalled(oc, "ImageRegistry") {
			g.Skip("Skip for the test due to image registry not installed")
		}
		waitErr := wait.Poll(10*time.Second, 1*time.Minute, func() (bool, error) {
			output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co/image-registry", "-o=jsonpath={.status.conditions[?(@.type==\"Available\")].status}{.status.conditions[?(@.type==\"Progressing\")].status}{.status.conditions[?(@.type==\"Degraded\")].status}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if !strings.Contains(output, "TrueFalseFalse") && !strings.Contains(output, "TrueTrueFalse") {
				message, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args("co/image-registry", "-o=jsonpath={.status.conditions[?(@.type==\"Available\")].message}{.status.conditions[?(@.type==\"Progressing\")].message}{.status.conditions[?(@.type==\"Degraded\")].message}").Output()
				e2e.Logf("Wait for image-registry coming ready")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("Image registry is not ready with info %s\n", message))
	})

	// author: wewang@redhat.com
	g.It("NonHyperShiftHOST-ConnectedOnly-Author:wewang-High-39027-Check AWS secret and access key with an OpenShift installed in a regular way", func() {
		exutil.SkipIfPlatformTypeNot(oc, "AWS")
		g.By("Skip test when the cluster is with STS credential")
		credType, err := oc.AsAdmin().Run("get").Args("cloudcredentials.operator.openshift.io/cluster", "-o=jsonpath={.spec.credentialsMode}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(credType, "Manual") {
			g.Skip("Skip test on aws sts cluster")
		}

		g.By("Check AWS secret and access key inside image registry pod")
		var keyInfo string
		waitErr := wait.Poll(5*time.Second, 60*time.Second, func() (bool, error) {
			result, err := oc.AsAdmin().WithoutNamespace().Run("rsh").Args("-n", "openshift-image-registry", "deployment.apps/image-registry", "cat", "/var/run/secrets/cloud/credentials").Output()
			if err != nil {
				e2e.Logf("Fail to rsh into registry pod, error: %s. Trying again", err)
				return false, nil
			}
			keyInfo = result
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("Registry pods are not ready %s\n", waitErr))
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(keyInfo).To(o.ContainSubstring("aws_access_key_id"))
		o.Expect(keyInfo).To(o.ContainSubstring("aws_secret_access_key"))
		g.By("Check installer-cloud-credentials secret")
		credentials, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret/installer-cloud-credentials", "-n", "openshift-image-registry", "-o=jsonpath={.data.credentials}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		sDec, err := base64.StdEncoding.DecodeString(credentials)
		if err != nil {
			fmt.Printf("Error decoding string: %s ", err.Error())
		}
		o.Expect(sDec).To(o.ContainSubstring("aws_access_key_id"))
		o.Expect(sDec).To(o.ContainSubstring("aws_secret_access_key"))
		g.By("push/pull image to registry")
		oc.SetupProject()
		checkRegistryFunctionFine(oc, "test-39027", oc.Namespace())
	})

	// author: wewang@redhat.com
	g.It("Author:wewang-High-34992-Add logLevel to registry config object with invalid value", func() {
		g.By("Change spec.loglevel with invalid values")
		out, _ := oc.WithoutNamespace().AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"logLevel":"abc"}}`, "--type=merge").Output()
		o.Expect(out).To(o.ContainSubstring(logInfo))
		g.By("Change spec.operatorLogLevel with invalid values")
		out, _ = oc.WithoutNamespace().AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"operatorLogLevel":"abc"}}`, "--type=merge").Output()
		o.Expect(out).To(o.ContainSubstring(logInfo))
	})

	// author: wewang@redhat.com
	g.It("Author:wewang-LEVEL0-Critical-24262-Image registry operator can read/overlap global proxy setting [Disruptive]", func() {
		var (
			buildFile = filepath.Join(imageRegistryBaseDir, "inputimage.yaml")
			buildsrc  = bcSource{
				outname:   "inputimage",
				namespace: "",
				name:      "imagesourcebuildconfig",
				template:  buildFile,
			}
		)

		g.By("Check if it's a proxy cluster")
		output, _ := oc.WithoutNamespace().AsAdmin().Run("get").Args("proxy/cluster", "-o=jsonpath={.spec}").Output()
		if !strings.Contains(output, "httpProxy") {
			g.Skip("Skip for non-proxy platform")
		}

		g.By("Check if it's a https_proxy cluster")
		output, _ = oc.WithoutNamespace().AsAdmin().Run("get").Args("proxy/cluster", "-o=jsonpath={.spec}").Output()
		if strings.Contains(output, "httpProxy") && strings.Contains(output, "user-ca-bundle") {
			g.Skip("Skip for https_proxy platform")
		}

		// Check if openshift-sample operator installed
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co/openshift-samples").Output()
		if err != nil && strings.Contains(output, `openshift-samples" not found`) {
			g.Skip("Skip test for openshift-samples which managed templates and imagestream are not installed")
		}
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("config.samples/cluster", "-o=jsonpath={.spec.managementState}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if output == "Removed" {
			g.Skip("Skip test for openshift-samples which is removed")
		}

		g.By("Start a build and pull image from internal registry")
		oc.SetupProject()
		buildsrc.namespace = oc.Namespace()
		g.By("Create buildconfig")
		buildsrc.create(oc)
		g.By("starting a build to output internal imagestream")
		err = oc.Run("start-build").Args(buildsrc.outname).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("waiting for build to finish")
		err = exutil.WaitForABuild(oc.BuildClient().BuildV1().Builds(oc.Namespace()), fmt.Sprintf("%s-1", buildsrc.outname), nil, nil, nil)
		if err != nil {
			exutil.DumpBuildLogs(buildsrc.outname, oc)
		}
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("starting a build using internal registry image")
		err = oc.Run("start-build").Args(buildsrc.name).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("waiting for build to finish")
		err = exutil.WaitForABuild(oc.BuildClient().BuildV1().Builds(oc.Namespace()), buildsrc.name+"-1", nil, nil, nil)
		if err != nil {
			exutil.DumpBuildLogs(buildsrc.name, oc)
		}
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Set status variables")
		expectedStatus1 := map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}

		g.By("Set wrong proxy to imageregistry cluster")
		defer func() {
			g.By("Remove proxy for imageregistry cluster")
			err = oc.WithoutNamespace().AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec": {"proxy": null}}`, "--type=merge").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			err = waitCoBecomes(oc, "image-registry", 240, expectedStatus1)
			o.Expect(err).NotTo(o.HaveOccurred())
			result, err := oc.AsAdmin().WithoutNamespace().Run("rsh").Args("-n", "openshift-image-registry", "deployment.apps/image-registry", "env").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(result).NotTo(o.ContainSubstring("HTTP_PROXY=http://test:3128"))
			o.Expect(result).NotTo(o.ContainSubstring("HTTPS_PROXY=http://test:3128"))
		}()
		err = oc.WithoutNamespace().AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"proxy":{"http": "http://test:3128","https":"http://test:3128","noProxy":"test.no-proxy.com"}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = wait.Poll(25*time.Second, 2*time.Minute, func() (bool, error) {
			result, err := oc.AsAdmin().WithoutNamespace().Run("set").Args("env", "-n", "openshift-image-registry", "deployment.apps/image-registry", "--list").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(result, "HTTP_PROXY=http://test:3128") && strings.Contains(result, "HTTPS_PROXY=http://test:3128") && strings.Contains(result, "NO_PROXY=test.no-proxy.com") {
				return true, nil
			}
			e2e.Logf("Continue to next round")
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "The global proxy is not override")
	})

	// author: wewang@redhat.com
	g.It("Author:wewang-Critical-22893-PodAntiAffinity should work for image registry pod[Serial]", func() {
		g.Skip("According devel comments: https://bugzilla.redhat.com/show_bug.cgi?id=2014940, still not work,when find a solution, will enable it")
		g.By("Check platforms")
		// We set registry use pv on openstack&disconnect cluster, the case will fail on this scenario.
		// Skip all the fs volume test, only run on object storage backend.
		if checkRegistryUsingFSVolume(oc) {
			g.Skip("Skip for fs volume")
		}

		var numi, numj int
		g.By("Add podAntiAffinity in image registry config")
		err := oc.WithoutNamespace().AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"affinity":{"podAntiAffinity":{"preferredDuringSchedulingIgnoredDuringExecution":[{"podAffinityTerm":{"topologyKey":"kubernetes.io/hostname"},"weight":100}]}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.WithoutNamespace().AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"affinity":null}}`, "--type=merge").Execute()

		g.By("Set status variables")
		expectedStatus1 := map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}

		g.By("Set image registry replica to 3")
		err = oc.WithoutNamespace().AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"replicas":3}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			g.By("Set image registry replica to 2")
			err = oc.WithoutNamespace().AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"replicas":2}}`, "--type=merge").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			err = waitCoBecomes(oc, "image-registry", 240, expectedStatus1)
			o.Expect(err).NotTo(o.HaveOccurred())
		}()

		g.By("Confirm 3 pods scaled up")
		err = wait.Poll(1*time.Minute, 2*time.Minute, func() (bool, error) {
			podList, _ := oc.AdminKubeClient().CoreV1().Pods("openshift-image-registry").List(context.Background(), metav1.ListOptions{LabelSelector: "docker-registry=default"})
			if len(podList.Items) != 3 {
				e2e.Logf("Continue to next round")
				return false, nil
			}
			for _, pod := range podList.Items {
				if pod.Status.Phase != corev1.PodRunning {
					e2e.Logf("Continue to next round")
					return false, nil
				}
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "Image registry pod list is not 3")

		g.By("At least 2 pods in different nodes")
		_, numj = comparePodHostIP(oc)
		o.Expect(numj >= 2).To(o.BeTrue())

		g.By("Set image registry replica to 4")
		err = oc.WithoutNamespace().AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"replicas":4}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Confirm 4 pods scaled up")
		err = wait.Poll(50*time.Second, 2*time.Minute, func() (bool, error) {
			podList, _ := oc.AdminKubeClient().CoreV1().Pods("openshift-image-registry").List(context.Background(), metav1.ListOptions{LabelSelector: "docker-registry=default"})
			if len(podList.Items) != 4 {
				e2e.Logf("Continue to next round")
				return false, nil
			}
			for _, pod := range podList.Items {
				if pod.Status.Phase != corev1.PodRunning {
					e2e.Logf("Continue to next round")
					return false, nil
				}
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "Image registry pod list is not 4")

		g.By("Check 2 pods in the same node")
		numi, _ = comparePodHostIP(oc)
		o.Expect(numi >= 1).To(o.BeTrue())
	})

	// author: xiuwang@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:xiuwang-Low-43669-Update openshift-image-registry/node-ca DaemonSet using maxUnavailable", func() {
		g.By("Check node-ca updatepolicy")
		out := getResource(oc, asAdmin, withoutNamespace, "daemonset/node-ca", "-n", "openshift-image-registry", "-o=jsonpath={.spec.updateStrategy.rollingUpdate}")
		o.Expect(out).To(o.ContainSubstring(updatePolicy))
	})

	// author: xiuwang@redhat.com
	g.It("NonHyperShiftHOST-DisconnectedOnly-Author:xiuwang-High-43715-Image registry pullthough should support pull image from the mirror registry with auth via imagecontentsourcepolicy", func() {
		g.By("Create a imagestream using payload image with pullthrough policy")
		oc.SetupProject()
		err := waitForAnImageStreamTag(oc, "openshift", "tools", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().Run("tag").Args("openshift/tools:latest", "mytools:latest", "--reference-policy=local", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		waitForAnImageStreamTag(oc, oc.Namespace(), "mytools", "latest")

		g.By("Check the imagestream imported with digest id using pullthrough policy")
		err = oc.Run("set").Args("image-lookup", "mytools", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		expectInfo := `Successfully pulled image "image-registry.openshift-image-registry.svc:5000/` + oc.Namespace()
		createSimpleRunPod(oc, "mytools", expectInfo)
	})

	// author: wewang@redhat.com
	g.It("Author:wewang-Medium-ConnectedOnly-27961-Create imagestreamtag with insufficient permissions [Disruptive]", func() {
		var (
			roleFile = filepath.Join(imageRegistryBaseDir, "role.yaml")
			rolesrc  = authRole{
				namespace: "",
				rolename:  "tag-bug-role",
				template:  roleFile,
			}
		)
		g.By("Import an image")
		oc.SetupProject()
		err := oc.Run("import-image").Args("test-img", "--from", "registry.access.redhat.com/rhel7", "--confirm").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create role with insufficient permissions")
		rolesrc.namespace = oc.Namespace()
		rolesrc.create(oc)
		err = oc.Run("create").Args("sa", "tag-bug-sa").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().Run("policy").Args("add-role-to-user", "view", "-z", "tag-bug-sa", "--role-namespace", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().Run("policy").Args("remove-role-from-user", "view", "tag-bug-sa", "--role-namespace", oc.Namespace()).Execute()
		token, err := getSAToken(oc, "tag-bug-sa", oc.Namespace())
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.Run("config").Args("set-credentials", "tag-bug-sa", fmt.Sprintf("--token=%s", token)).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defuser, err := oc.Run("config").Args("get-users").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		out, err := oc.Run("config").Args("current-context").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.Run("config").Args("set-context", out, "--user=tag-bug-sa").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.Run("config").Args("set-context", out, "--user="+defuser).Execute()

		g.By("Create imagestreamtag with insufficient permissions")
		err = oc.AsAdmin().Run("tag").Args("test-img:latest", "test-img:v1").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check if new imagestreamtag created")
		out = getResource(oc, true, withoutNamespace, "istag", "-n", oc.Namespace())
		o.Expect(out).To(o.ContainSubstring("test-img:latest"))
		o.Expect(out).To(o.ContainSubstring("test-img:v1"))
	})

	// author: xiuwang@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-Author:xiuwang-Medium-43664-Check ServiceMonitor of registry which will not hotloop CVO", func() {
		g.By("Check the servicemonitor of openshift-image-registry")
		out := getResource(oc, asAdmin, withoutNamespace, "servicemonitor", "-n", "openshift-image-registry", "-o=jsonpath={.items[1].spec.selector.matchLabels.name}")
		o.Expect(out).To(o.ContainSubstring("image-registry-operator"))

		g.By("Check CVO not hotloop due to registry")
		masterlogs, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("node-logs", "--role", "master", "--path=kube-apiserver/audit.log", "--raw").OutputToFile("audit.log")
		o.Expect(err).NotTo(o.HaveOccurred())

		result, err := exec.Command("bash", "-c", "cat "+masterlogs+" | grep verb.*update.*resource.*servicemonitors").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(result).NotTo(o.ContainSubstring("image-registry"))
	})

	// author: wewang@redhat.com
	g.It("Author:wewang-Medium-27985-Image with invalid resource name can be pruned [Disruptive]", func() {
		// When registry configured pvc or emptryDir, the replicas is 1 and with recreate pod policy.
		// This is not suitable for the defer recoverage. Only run this case on cloud storage.
		platforms := map[string]bool{
			"aws":          true,
			"azure":        true,
			"gcp":          true,
			"alibabacloud": true,
			"ibmcloud":     true,
		}
		if !platforms[exutil.CheckPlatform(oc)] {
			g.Skip("Skip for non-supported platform")
		}

		g.By("Config image registry to emptydir")
		defer recoverRegistryStorageConfig(oc)
		defer recoverRegistryDefaultReplicas(oc)
		configureRegistryStorageToEmptyDir(oc)

		g.By("Import image to internal registry")
		oc.SetupProject()
		// Change to use qe image to create build so we can run this on disconnect cluster.
		var invalidInfo = "Invalid image name foo/bar/" + oc.Namespace() + "/test-27985"
		checkRegistryFunctionFine(oc, "test-27985", oc.Namespace())

		g.By("Add system:image-pruner role to system:serviceaccount:openshift-image-registry:registry")
		defer oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "remove-cluster-role-from-user", "system:image-pruner", "system:serviceaccount:openshift-image-registry:registry").Execute()
		err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "add-cluster-role-to-user", "system:image-pruner", "system:serviceaccount:openshift-image-registry:registry").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check invaild image source can be pruned")
		err = oc.AsAdmin().WithoutNamespace().Run("rsh").Args("-n", "openshift-image-registry", "deployment.apps/image-registry", "mkdir", "-p", "/registry/docker/registry/v2/repositories/foo/bar").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("rsh").Args("-n", "openshift-image-registry", "deployment.apps/image-registry", "cp", "-r", "/registry/docker/registry/v2/repositories/"+oc.Namespace(), "/registry/docker/registry/v2/repositories/foo/bar").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		out, err := oc.AsAdmin().WithoutNamespace().Run("rsh").Args("-n", "openshift-image-registry", "deployment.apps/image-registry", "/bin/bash", "-c", "/usr/bin/dockerregistry -prune=check").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(out).To(o.ContainSubstring(invalidInfo))
	})

	// author: wewang@redhat.com
	g.It("NonHyperShiftHOST-ROSA-OSD_CCS-ARO-Author:wewang-High-41414-There are 2 replicas for image registry on HighAvailable workers S3/Azure/GCS/Swift storage", func() {
		g.By("Check image registry pod")
		// We set registry use pv on openstack&disconnect cluster, the case will fail on this scenario.
		// Skip all the fs volume test, only run on object storage backend.
		if checkRegistryUsingFSVolume(oc) {
			g.Skip("Skip for fs volume")
		}
		g.By("Check if cluster is sno")
		workerNodes, _ := exutil.GetClusterNodesBy(oc, "worker")
		if len(workerNodes) == 1 {
			g.Skip("Skip for sno cluster")
		}

		checkPodsRunningWithLabel(oc, "openshift-image-registry", "docker-registry=default", 2)
		oc.SetupProject()
		checkRegistryFunctionFine(oc, "test-41414", oc.Namespace())
	})

	//author: xiuwang@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-Author:xiuwang-Critical-21593-Critical-34680-Medium-35906-High-27588-Image registry storage cannot be removed if set to Unamanaged when image registry is set to Removed [Disruptive]", func() {
		g.By("Set status variables")
		expectedStatus1 := map[string]string{"Progressing": "True"}
		expectedStatus2 := map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}

		g.By("Get registry storage info")
		var storageinfo1, storageinfo2, storageinfo3 string
		_, storageinfo1 = getRegistryStorageConfig(oc)
		podNum := getImageRegistryPodNumber(oc)

		g.By("In default image registry cluster Managed and prune-registry flag is true")
		out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("configs.imageregistry/cluster", "-o=jsonpath={.spec.managementState}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(out).Should(o.Equal("Managed"))
		out, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("cronjob.batch/image-pruner", "-n", "openshift-image-registry", "-o=jsonpath={.spec.jobTemplate.spec.template.spec.containers[0]}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(out).To(o.ContainSubstring("--prune-registry=true"))

		g.By("Set image registry storage to Unmanaged, image registry operator to Removed")
		defer func() {
			g.By("Recover image registry change")
			patchInfo := fmt.Sprintf("{\"spec\":{\"managementState\":\"Managed\",\"replicas\": %v,\"storage\":{\"managementState\":\"Managed\"}}}", podNum)
			err := oc.AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", patchInfo, "--type=merge").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			err = waitCoBecomes(oc, "image-registry", 240, expectedStatus2)
			o.Expect(err).NotTo(o.HaveOccurred())
			err = waitCoBecomes(oc, "openshift-apiserver", 480, expectedStatus2)
			o.Expect(err).NotTo(o.HaveOccurred())
			err = waitCoBecomes(oc, "kube-apiserver", 600, expectedStatus2)
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		err = oc.WithoutNamespace().AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"managementState":"Removed","storage":{"managementState":"Unmanaged"}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check image-registry pods are removed")
		checkRegistrypodsRemoved(oc)
		_, storageinfo2 = getRegistryStorageConfig(oc)
		if strings.Compare(storageinfo1, storageinfo2) != 0 {
			e2e.Failf("Image stroage has changed")
		}

		g.By("Check prune-registry flag is false")
		out, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("cronjob.batch/image-pruner", "-n", "openshift-image-registry", "-o=jsonpath={.spec.jobTemplate.spec.template.spec.containers[0]}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(out).To(o.ContainSubstring("--prune-registry=false"))

		g.By("Make update in the pruning custom resource")
		defer oc.AsAdmin().Run("patch").Args("imagepruner/cluster", "-p", `{"spec":{"schedule":""}}`, "--type=merge").Execute()
		err = oc.AsAdmin().Run("patch").Args("imagepruner/cluster", "-p", `{"spec":{"schedule":"*/1 * * * *"}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		logInfo := "Only API objects will be removed.  No modifications to the image registry will be made"
		warnInfo := "batch/v1beta1 CronJob is deprecated in v1.21+, unavailable in v1.25+; use batch/v1 CronJob"
		imagePruneLog(oc, logInfo, warnInfo)

		g.By("Set image registry operator to Managed again")
		err = oc.WithoutNamespace().AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"managementState":"Managed"}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitCoBecomes(oc, "image-registry", 60, expectedStatus1)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitCoBecomes(oc, "image-registry", 240, expectedStatus2)
		o.Expect(err).NotTo(o.HaveOccurred())

		_, storageinfo3 = getRegistryStorageConfig(oc)
		if strings.Compare(storageinfo1, storageinfo3) != 0 {
			e2e.Failf("Image stroage has changed")
		}

		g.By("Change managementSet from Managed to Unmanaged and replicas to 3")
		err = oc.AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"managementState":"Unmanaged","replicas": 3}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check image registry pods are not change")
		checkPodsRunningWithLabel(oc, "openshift-image-registry", "docker-registry=default", podNum)

	})

	// author: wewang@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:wewang-High-45952-ConnectedOnly-Imported imagestreams should success in deploymentconfig", func() {
		var (
			statefulsetFile = filepath.Join(imageRegistryBaseDir, "statefulset.yaml")
			statefulsetsrc  = staSource{
				namespace: "",
				name:      "example-statefulset",
				image:     "hello-openshift:1.2.0",
				template:  statefulsetFile,
			}
		)
		g.By("Import an image stream and set image-lookup")
		oc.SetupProject()
		err := oc.Run("import-image").Args("quay.io/openshifttest/hello-openshift:1.2.0", "--scheduled", "--confirm", "--reference-policy=local", "--import-mode=PreserveOriginal", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, oc.Namespace(), "hello-openshift", "1.2.0")
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.Run("set").Args("image-lookup", "hello-openshift", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create the initial statefulset")
		statefulsetsrc.namespace = oc.Namespace()
		g.By("Create statefulset")
		statefulsetsrc.create(oc)
		g.By("Check the pods are running")
		checkPodsRunningWithLabel(oc, oc.Namespace(), "app=example-statefulset", 3)

		g.By("Reapply the sample yaml")
		applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", statefulsetsrc.template, "-p", "NAME="+statefulsetsrc.name, "IMAGE="+statefulsetsrc.image, "NAMESPACE="+statefulsetsrc.namespace)
		g.By("Check the pods are running")
		checkPodsRunningWithLabel(oc, oc.Namespace(), "app=example-statefulset", 3)

		g.By("setting a trigger, pods are still running")
		err = oc.Run("set").Args("triggers", "statefulset/example-statefulset", "--from-image=hello-openshift:latest", "--containers", "example-statefulset", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Check the pods are running")
		checkPodsRunningWithLabel(oc, oc.Namespace(), "app=example-statefulset", 3)
		interReg := "image-registry.openshift-image-registry.svc:5000/" + oc.Namespace() + "/hello-openshift"
		output, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("pods", "-o=jsonpath={.items[*].spec.containers[*].image}", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring(interReg))
	})

	// author: wewang@redhat.com
	g.It("Author:wewang-NonHyperShiftHOST-Medium-39028-Check aws secret and access key with an openShift installed with an STS credential", func() {
		g.By("Check platforms")
		exutil.SkipIfPlatformTypeNot(oc, "AWS")
		g.By("Check if the cluster is with STS credential")
		credType, err := oc.AsAdmin().Run("get").Args("cloudcredentials.operator.openshift.io/cluster", "-o=jsonpath={.spec.credentialsMode}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(credType, "Manual") {
			g.Skip("Skip test on none aws sts cluster")
		}
		credentials, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret/aws-cloud-credentials", "-n", "openshift-machine-api", "-o=jsonpath={.data.credentials}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		sDec, _ := base64.StdEncoding.DecodeString(credentials)
		if !strings.Contains(string(sDec), "role_arn") {
			g.Skip("Skip test on none aws sts cluster")
		}

		g.By("Check role_arn/web_identity_token_file inside image registry pod")
		result, err := oc.AsAdmin().WithoutNamespace().Run("rsh").Args("-n", "openshift-image-registry", "deployment.apps/image-registry", "cat", "/var/run/secrets/cloud/credentials").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(result).To(o.ContainSubstring("role_arn"))
		o.Expect(result).To(o.ContainSubstring("web_identity_token_file"))

		g.By("Check installer-cloud-credentials secret")
		credentials, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("secret/installer-cloud-credentials", "-n", "openshift-image-registry", "-o=jsonpath={.data.credentials}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		sDec, _ = base64.StdEncoding.DecodeString(credentials)
		if !strings.Contains(string(sDec), "role_arn") {
			e2e.Failf("credentials does not contain role_arn")
		}
		if !strings.Contains(string(sDec), "web_identity_token_file") {
			e2e.Failf("credentials does not contain web_identity_token_file")
		}

		g.By("push/pull image to registry")
		oc.SetupProject()
		checkRegistryFunctionFine(oc, "test-39028", oc.Namespace())
	})

	//author: xiuwang@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-Author:xiuwang-High-45540-Registry should fall back to secondary ImageContentSourcePolicy Mirror [Disruptive]", func() {
		var (
			icspFile = filepath.Join(imageRegistryBaseDir, "icsp-multi-mirrors.yaml")
			icspsrc  = icspSource{
				name:     "image-policy-fake",
				template: icspFile,
			}
			mc = machineConfig{
				name:     "",
				pool:     "worker",
				source:   "",
				path:     "",
				template: "",
			}
		)
		g.By("Create imagecontentsourcepolicy with multiple mirrors")
		defer func() {
			icspsrc.delete(oc)
			// Update registry of icsp will restart crio to apply change to every node
			// Need ensure master and worker update completed
			mc.waitForMCPComplete(oc)
			mc.pool = "master"
			mc.waitForMCPComplete(oc)
		}()
		icspsrc.create(oc)

		g.By("Check registry configs get updated")
		masterNode, _ := exutil.GetFirstMasterNode(oc)
		err := wait.Poll(25*time.Second, 2*time.Minute, func() (bool, error) {
			output, _ := exutil.DebugNodeWithChroot(oc, masterNode, "cat /etc/containers/registries.conf | grep fake.rhcloud.com")
			if !strings.Contains(output, "fake.rhcloud.com") {
				e2e.Logf("Continue to next round")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "registry configs are not changed")

		g.By("Create a pod to check pulling issue")
		oc.SetupProject()
		err = waitForAnImageStreamTag(oc, "openshift", "cli", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("deployment", "cli-test", "--image", "image-registry.openshift-image-registry.svc:5000/openshift/cli:latest", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Get project events")
		err = wait.Poll(10*time.Second, 1*time.Minute, func() (bool, error) {
			events, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("events", "-n", oc.Namespace()).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if !strings.Contains(events, `Successfully pulled image "image-registry.openshift-image-registry.svc:5000/openshift/cli:latest"`) {
				e2e.Logf("Continue to next round")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "Image pulls failed")
	})

	// author: wewang@redhat.com
	g.It("ARO-Author:wewang-Medium-23583-Registry should not try to pullthrough himself by any name ", func() {
		g.By("Get server host")
		routeName1 := getRandomString()
		routeName2 := getRandomString()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("route", routeName1, routeName2, "-n", "openshift-image-registry").Execute()
		userroute1 := exposeRouteFromSVC(oc, "reencrypt", "openshift-image-registry", routeName1, "image-registry")
		userroute2 := exposeRouteFromSVC(oc, "reencrypt", "openshift-image-registry", routeName2, "image-registry")
		waitRouteReady(userroute1)
		waitRouteReady(userroute2)

		g.By("Get token from secret")
		oc.SetupProject()
		token, err := getSAToken(oc, "builder", oc.Namespace())
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(token).NotTo(o.BeEmpty())

		g.By("Create a secret for user-defined route")
		err = oc.NotShowInfo().WithoutNamespace().AsAdmin().Run("create").Args("secret", "docker-registry", "mysecret", "--docker-server="+userroute1, "--docker-username="+oc.Username(), "--docker-password="+token, "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Import an image")
		// Use multiarch image with digest, so it could be test on ARM cluster and disconnect cluster.
		err = oc.WithoutNamespace().AsAdmin().Run("import-image").Args("myimage", "--from=quay.io/openshifttest/busybox@sha256:c5439d7db88ab5423999530349d327b04279ad3161d7596d2126dfb5b02bfd1f", "--confirm", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, oc.Namespace(), "myimage", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Tag the image point to itself address")
		err = oc.WithoutNamespace().AsAdmin().Run("import-image").Args("myimage:test", "--from="+userroute1+"/"+oc.Namespace()+"/myimage", "--insecure=true", "--confirm", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, oc.Namespace(), "myimage", "test")
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Get blobs from the default registry")
		getURL := "curl -Lks -u \"" + oc.Username() + ":" + token + "\" -I HEAD https://" + userroute2 + "/v2/" + oc.Namespace() + "/myimage@sha256:0000000000000000000000000000000000000000000000000000000000000000"
		curlOutput, err := exec.Command("bash", "-c", getURL).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(string(curlOutput)).To(o.ContainSubstring("HTTP/1.1 404"))
		var podsOfImageRegistry []corev1.Pod
		podsOfImageRegistry = listPodStartingWith("image-registry", oc, "openshift-image-registry")
		if len(podsOfImageRegistry) == 0 {
			e2e.Failf("Error retrieving logs")
		}
		foundErrLog := false
		foundErrLog = dePodLogs(podsOfImageRegistry, oc, errInfo)
		o.Expect(foundErrLog).To(o.BeTrue())
	})

	// author: jitli@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-Author:jitli-ConnectedOnly-Medium-33051-Images can be imported from an insecure registry without 'insecure: true' if it is in insecureRegistries in image.config/cluster [Disruptive]", func() {
		var (
			expectedStatus1 = map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}
			mc              = machineConfig{
				name:     "",
				pool:     "worker",
				source:   "",
				path:     "",
				template: "",
			}
		)

		masterNode, _ := exutil.GetFirstMasterNode(oc)

		g.By("Create route to expose the registry")
		routeName := getRandomString()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("route", routeName, "-n", "openshift-image-registry").Execute()
		host := exposeRouteFromSVC(oc, "reencrypt", "openshift-image-registry", routeName, "image-registry")
		waitRouteReady(host)

		g.By("Get token from secret")
		oc.SetupProject()
		token, err := getSAToken(oc, "builder", oc.Namespace())
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(token).NotTo(o.BeEmpty())

		g.By("Create a secret for user-defined route")
		err = oc.NotShowInfo().WithoutNamespace().AsAdmin().Run("create").Args("secret", "docker-registry", "secret33051", "--docker-server="+host, "--docker-username="+oc.Username(), "--docker-password="+token, "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Import image from an insecure registry directly without --insecure=true")
		output, _ := oc.WithoutNamespace().AsAdmin().Run("import-image").Args("image-33051", "--from="+host+"/openshift/tools:latest", "--confirm", "-n", oc.Namespace()).Output()
		o.Expect(output).To(o.ContainSubstring("x509"))

		g.By("Add the insecure registry to images.config.openshift.io cluster, add docker.io to blockedRegistries list")
		defer func() {
			err = oc.AsAdmin().Run("patch").Args("images.config.openshift.io/cluster", "-p", `{"spec": {"registrySources": null}}`, "--type=merge").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			// image.conf.spec.registrySources will restart crio to apply change to every node
			// Need ensure master and worker update completed
			mc.waitForMCPComplete(oc)
			mc.pool = "master"
			mc.waitForMCPComplete(oc)
			err := waitCoBecomes(oc, "image-registry", 240, expectedStatus1)
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		err = oc.AsAdmin().Run("patch").Args("images.config.openshift.io/cluster", "-p", `{"spec": {"registrySources": {"insecureRegistries": ["`+host+`"],"blockedRegistries": ["docker.io"]}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("registries.conf gets updated")
		err = wait.Poll(30*time.Second, 6*time.Minute, func() (bool, error) {
			registriesstatus, _ := exutil.DebugNodeWithChroot(oc, masterNode, "cat", "/etc/containers/registries.conf")
			if strings.Contains(registriesstatus, host) {
				e2e.Logf("registries.conf updated")
				return true, nil
			}
			e2e.Logf("registries.conf not update")
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "registries.conf not update")

		g.By("Importing image from an insecure registry directly without --insecure=true should succeed")
		err = oc.WithoutNamespace().AsAdmin().Run("import-image").Args("tools:33051", "--from="+host+"/openshift/tools:latest", "--confirm", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, oc.Namespace(), "tools", "33051")
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Import an image from docker.io")
		output, _ = oc.WithoutNamespace().AsAdmin().Run("import-image").Args("image2-33051", "--from=docker.io/centos/ruby-22-centos7", "--confirm=true", "-n", oc.Namespace()).Output()
		o.Expect(output).To(o.ContainSubstring("error: Import failed (Forbidden): forbidden: registry docker.io blocked"))
	})

	// author: wewang@redhat.com
	g.It("NonPreRelease-Longduration-Author:wewang-Critical-24838-Registry OpenStack Storage test with invalid settings [Disruptive]", func() {
		exutil.SkipIfPlatformTypeNot(oc, "OpenStack")

		g.By("Set status variables")
		expectedStatus1 := map[string]string{"Progressing": "True"}
		expectedStatus2 := map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}

		g.By("Set a different container in registry config")
		oricontainer, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("configs.imageregistry/cluster", "-o=jsonpath={.spec.storage.swift.container}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		newcontainer := strings.Replace(oricontainer, "image", "images", 1)
		defer func() {
			err = oc.AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"storage":{"swift":{"container": "`+oricontainer+`"}}}}`, "--type=merge").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			err = waitCoBecomes(oc, "image-registry", 240, expectedStatus2)
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		err = oc.AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"storage":{"swift":{"container": "`+newcontainer+`"}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitCoBecomes(oc, "image-registry", 60, expectedStatus1)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitCoBecomes(oc, "image-registry", 240, expectedStatus2)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Set invalid authURL in image registry crd")
		foundErrLog1 := false
		foundErrLog1 = setImageregistryConfigs(oc, patchAuthURL, authErrInfo)
		o.Expect(foundErrLog1).To(o.BeTrue())

		g.By("Set invalid regionName")
		foundErrLog2 := false
		foundErrLog2 = setImageregistryConfigs(oc, patchRegion, regionErrInfo)
		o.Expect(foundErrLog2).To(o.BeTrue())

		g.By("Set invalid domain")
		foundErrLog3 := false
		foundErrLog3 = setImageregistryConfigs(oc, patchDomain, domainErrInfo)
		o.Expect(foundErrLog3).To(o.BeTrue())

		g.By("Set invalid domainID")
		foundErrLog4 := false
		foundErrLog4 = setImageregistryConfigs(oc, patchDomainID, domainIDErrInfo)
		o.Expect(foundErrLog4).To(o.BeTrue())

		g.By("Set invalid tenantID")
		foundErrLog5 := false
		foundErrLog5 = setImageregistryConfigs(oc, patchTenantID, tenantIDErrInfo)
		o.Expect(foundErrLog5).To(o.BeTrue())
	})

	// author: xiuwang@redhat.com
	g.It("Author:xiuwang-Critical-47274-Image registry works with OSS storage on alibaba cloud", func() {
		exutil.SkipIfPlatformTypeNot(oc, "AlibabaCloud")

		g.By("Check OSS storage")
		output, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("config.image/cluster", "-o=jsonpath={.status.storage.oss}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("bucket"))
		o.Expect(output).To(o.ContainSubstring(`"endpointAccessibility":"Internal"`))
		o.Expect(output).To(o.ContainSubstring("region"))
		output, err = oc.WithoutNamespace().AsAdmin().Run("get").Args("config.image/cluster", "-o=jsonpath={.status.conditions[?(@.type==\"StorageEncrypted\")].message}{.status.conditions[?(@.type==\"StorageEncrypted\")].status}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Default AES256 encryption was successfully enabled on the OSS bucketTrue"))

		g.By("Check if registry operator degraded")
		registryDegrade := checkRegistryDegraded(oc)
		if registryDegrade {
			e2e.Failf("Image registry is degraded")
		}

		g.By("Check if registry works well")
		oc.SetupProject()
		checkRegistryFunctionFine(oc, "test-47274", oc.Namespace())

		g.By("Check if registry interact with OSS used the internal endpoint")
		output, err = oc.WithoutNamespace().AsAdmin().Run("logs").Args("deploy/image-registry", "--since=30s", "-n", "openshift-image-registry").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("internal.aliyuncs.com"))

	})

	// author: xiuwang@redhat.com
	g.It("NonPreRelease-Longduration-Author:xiuwang-Medium-47342-Configure image registry works with OSS parameters [Disruptive]", func() {
		exutil.SkipIfPlatformTypeNot(oc, "AlibabaCloud")
		g.By("Set status variables")
		expectedStatus1 := map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}

		g.By("Configure OSS with Public endpoint")
		defer func() {
			err := oc.AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"storage":{"oss":{"endpointAccessibility":null}}}}`, "--type=merge").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			err = waitCoBecomes(oc, "image-registry", 240, expectedStatus1)
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		_, err := oc.AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"storage":{"oss":{"endpointAccessibility":"Public"}}}}`, "--type=merge").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = wait.PollImmediate(10*time.Second, 2*time.Minute, func() (bool, error) {
			registryDegrade := checkRegistryDegraded(oc)
			if registryDegrade {
				e2e.Logf("wait for next round")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "Image registry is degraded")
		oc.SetupProject()
		checkRegistryFunctionFine(oc, "test-47342", oc.Namespace())
		output, err := oc.WithoutNamespace().AsAdmin().Run("logs").Args("deploy/image-registry", "--since=1m", "-n", "openshift-image-registry").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).NotTo(o.ContainSubstring("internal.aliyuncs.com"))

		g.By("Configure registry to use KMS encryption type")
		defer oc.AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"storage":{"oss":{"encryption":null}}}}`, "--type=merge").Execute()
		output, err = oc.AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"storage":{"oss":{"encryption":{"method":"KMS","kms":{"keyID":"invalidid"}}}}}}`, "--type=merge").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = wait.PollImmediate(10*time.Second, 2*time.Minute, func() (bool, error) {
			output, err = oc.WithoutNamespace().AsAdmin().Run("get").Args("config.image", "cluster", "-o=jsonpath={.status.conditions[?(@.type==\"StorageEncrypted\")].message}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if !strings.Contains(output, "Default KMS encryption was successfully enabled on the OSS bucket") {
				e2e.Logf("wait for next round")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "Default encryption can't be changed")
		br, err := exutil.StartBuildAndWait(oc, "test-47342")
		o.Expect(err).NotTo(o.HaveOccurred())
		br.AssertFailure()
		output, err = oc.WithoutNamespace().AsAdmin().Run("logs").Args("deploy/image-registry", "--since=1m", "-n", "openshift-image-registry").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("The specified parameter KMS keyId is not valid"))
	})

	// author: xiuwang@redhat.com
	g.It("Author:xiuwang-Critical-45345-Image registry works with ibmcos storage on IBM cloud", func() {
		exutil.SkipIfPlatformTypeNot(oc, "IBMCloud")

		g.By("Check ibmcos storage")
		output, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("config.image/cluster", "-o=jsonpath={.status.storage.ibmcos}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("bucket"))
		o.Expect(output).To(o.ContainSubstring("location"))
		o.Expect(output).To(o.ContainSubstring("resourceGroupName"))
		o.Expect(output).To(o.ContainSubstring("resourceKeyCRN"))
		o.Expect(output).To(o.ContainSubstring("serviceInstanceCRN"))

		g.By("Check if registry operator degraded")
		registryDegrade := checkRegistryDegraded(oc)
		if registryDegrade {
			e2e.Failf("Image registry is degraded")
		}

		g.By("Check if registry works well")
		oc.SetupProject()
		checkRegistryFunctionFine(oc, "test-45345", oc.Namespace())
	})

	// author: jitli@redhat.com
	g.It("NonHyperShiftHOST-Author:jitli-ConnectedOnly-Medium-41398-Users providing custom AWS tags are set with bucket creation [Disruptive]", func() {

		g.By("Check platforms")
		exutil.SkipIfPlatformTypeNot(oc, "AWS")
		g.By("Check the cluster is with resourceTags")
		output, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("infrastructure.config.openshift.io", "-o=jsonpath={..status.platformStatus.aws}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(output, "resourceTags") {
			g.Skip("Skip for no resourceTags")
		}

		g.By("Set status variables")
		expectedStatus1 := map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}

		g.By("Get bucket name")
		bucket, err := oc.AsAdmin().Run("get").Args("config.image", "-o=jsonpath={..spec.storage.s3.bucket}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(bucket).NotTo(o.BeEmpty())

		g.By("Check the tags")
		aws := getAWSClient(oc)
		tag, err := awsGetBucketTagging(aws, bucket)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(tag).To(o.ContainSubstring("customTag"))
		o.Expect(tag).To(o.ContainSubstring("installer-qe"))

		g.By("Removed managementState")
		defer func() {
			status, err := oc.AsAdmin().Run("get").Args("config.image/cluster", "-o=jsonpath={.spec.managementState}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if status != "Managed" {
				e2e.Logf("recover config.image cluster is Managed")
				err = oc.AsAdmin().Run("patch").Args("config.image/cluster", "-p", `{"spec":{"managementState": "Managed"}}`, "--type=merge").Execute()
				o.Expect(err).NotTo(o.HaveOccurred())
			} else {
				e2e.Logf("config.image cluster is Managed")
			}
			err = waitCoBecomes(oc, "image-registry", 240, expectedStatus1)
			o.Expect(err).NotTo(o.HaveOccurred())
			err = waitCoBecomes(oc, "openshift-apiserver", 480, expectedStatus1)
			o.Expect(err).NotTo(o.HaveOccurred())
			err = waitCoBecomes(oc, "kube-apiserver", 600, expectedStatus1)
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		err = oc.AsAdmin().Run("patch").Args("config.image/cluster", "-p", `{"spec":{"managementState": "Removed"}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check bucket has been deleted")
		err = wait.Poll(2*time.Second, 10*time.Second, func() (bool, error) {
			tag, err = awsGetBucketTagging(aws, bucket)
			if err != nil && strings.Contains(tag, "The specified bucket does not exist") {
				return true, nil
			}
			e2e.Logf("bucket still exist, go next round")
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "the bucket isn't been deleted")

		g.By("Managed managementState")
		err = oc.AsAdmin().Run("patch").Args("config.image/cluster", "-p", `{"spec":{"managementState": "Managed"}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Get new bucket name and check")
		err = wait.Poll(10*time.Second, 2*time.Minute, func() (bool, error) {
			bucket, _ = oc.AsAdmin().Run("get").Args("config.image", "-o=jsonpath={..spec.storage.s3.bucket}").Output()
			if strings.Compare(bucket, "") != 0 {
				return true, nil
			}
			e2e.Logf("not update")
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "Can't get bucket")

		tag, err = awsGetBucketTagging(aws, bucket)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(tag).To(o.ContainSubstring("customTag"))
		o.Expect(tag).To(o.ContainSubstring("installer-qe"))
	})

	// author: tbuskey@redhat.com
	g.It("Author:xiuwang-ROSA-OSD_CCS-ARO-High-22056-Registry operator configure prometheus metric gathering [Serial]", func() {
		var (
			authHeader         string
			after              = make(map[string]int)
			before             = make(map[string]int)
			data               prometheusImageregistryQueryHTTP
			err                error
			fails              = 0
			failItems          = ""
			l                  int
			msg                string
			prometheusURL      = "https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1"
			prometheusURLQuery string
			query              string
			token              string
			metrics            = []string{
				"imageregistry_http_request_size_bytes_count",
				"imageregistry_http_request_size_bytes_sum",
				"imageregistry_http_response_size_bytes_count",
				"imageregistry_http_requests_total",
				"imageregistry_http_response_size_bytes_sum"}
		)

		g.By("Get Prometheus token")
		token, err = getSAToken(oc, "prometheus-k8s", "openshift-monitoring")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(token).NotTo(o.BeEmpty())
		authHeader = fmt.Sprintf("Authorization: Bearer %v", token)
		checkRegistryFunctionFine(oc, "test-22056", oc.Namespace())

		g.By("Collect metrics at start")
		for _, query = range metrics {
			prometheusURLQuery = fmt.Sprintf("%v/query?query=%v", prometheusURL, query)
			err = wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 30*time.Second, false, func(ctx context.Context) (bool, error) {
				msg, _, err = oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-monitoring", "-c", "prometheus", "prometheus-k8s-0", "-i", "--", "curl", "-k", "-H", authHeader, prometheusURLQuery).Outputs()
				if err != nil || msg == "" {
					return false, nil
				}
				return true, nil
			})
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("[Before] the query %v is getting failed", query))
			json.Unmarshal([]byte(msg), &data)
			l = len(data.Data.Result) - 1
			before[query], _ = strconv.Atoi(data.Data.Result[l].Value[1].(string))
			e2e.Logf("[before] query %v ==  %v", query, before[query])

		}
		g.By("pause to get next metrics")
		time.Sleep(60 * time.Second)

		g.By("Collect metrics again")
		for _, query = range metrics {
			prometheusURLQuery = fmt.Sprintf("%v/query?query=%v", prometheusURL, query)
			err = wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 30*time.Second, false, func(ctx context.Context) (bool, error) {
				msg, _, err = oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-monitoring", "-c", "prometheus", "prometheus-k8s-0", "-i", "--", "curl", "-k", "-H", authHeader, prometheusURLQuery).Outputs()
				if err != nil || msg == "" {
					return false, nil
				}
				return true, nil
			})
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("[After] the query %v is getting failed", query))
			json.Unmarshal([]byte(msg), &data)
			l = len(data.Data.Result) - 1
			after[query], _ = strconv.Atoi(data.Data.Result[l].Value[1].(string))
			e2e.Logf("[after] query %v ==  %v", query, after[query])
		}

		g.By("results")
		for _, query = range metrics {
			if before[query] > after[query] {
				fails++
				failItems = fmt.Sprintf("%v%v ", failItems, query)
			}
			e2e.Logf("%v -> %v %v", before[query], after[query], query)
			// need to test & compare
		}
		if fails != 0 {
			e2e.Failf("\nFAIL: %v metrics decreasesd: %v\n\n", fails, failItems)
		}

		g.By("Success")
	})

	// author: xiuwang@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:xiuwang-Medium-47933-DeploymentConfigs template should respect resolve-names annotation", func() {
		var (
			imageRegistryBaseDir = exutil.FixturePath("testdata", "image_registry")
			podFile              = filepath.Join(imageRegistryBaseDir, "dc-template.yaml")
			podsrc               = podSource{
				name:      "mydc",
				namespace: "",
				image:     "myis",
				template:  podFile,
			}
		)

		g.By("Use source imagestream to create dc")
		oc.SetupProject()
		err := oc.AsAdmin().WithoutNamespace().Run("tag").Args("quay.io/openshifttest/busybox@sha256:c5439d7db88ab5423999530349d327b04279ad3161d7596d2126dfb5b02bfd1f", "myis:latest", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, oc.Namespace(), podsrc.image, "latest")
		o.Expect(err).NotTo(o.HaveOccurred())
		podsrc.namespace = oc.Namespace()
		podsrc.create(oc)
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deploymentconfig/mydc", "-o=jsonpath={..spec.containers[*].image}", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("quay.io/openshifttest/busybox"))

		g.By("Use pullthrough imagestream to create dc")
		err = oc.AsAdmin().WithoutNamespace().Run("tag").Args("quay.io/openshifttest/busybox@sha256:c5439d7db88ab5423999530349d327b04279ad3161d7596d2126dfb5b02bfd1f", "myis:latest", "--reference-policy=local", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, oc.Namespace(), podsrc.image, "latest")
		o.Expect(err).NotTo(o.HaveOccurred())
		podsrc.create(oc)
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("deploymentconfig/mydc", "-o=jsonpath={..spec.template.spec.containers[*].image}", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("image-registry.openshift-image-registry.svc:5000/" + oc.Namespace() + "/" + podsrc.image))
	})

	g.It("NonPreRelease-Longduration-Author:xiuwang-Critical-43260-Image registry pod could report to processing after openshift-apiserver reports unconnect quickly[Disruptive][Slow]", func() {
		g.By("Get one master node")
		var nodeNames []string
		firstMaster, err := exutil.GetFirstMasterNode(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		nodeNames = append(nodeNames, firstMaster)

		g.By("Get one worker node which one image registry pod scheduled in")
		names, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", "openshift-image-registry", "-l", "docker-registry=default", "-o=jsonpath={..spec.nodeName}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		workerNames := strings.Split(names, " ")
		nodeNames = append(nodeNames, workerNames[0])
		if nodeNames[0] == nodeNames[1] {
			g.Skip("This should be a SNO cluster, skip this testing")
		}
		names, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", "openshift-image-registry", "-l", "docker-registry=default", "--sort-by={.status.startTime}", "-o=jsonpath={..metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		podNames := strings.Split(names, " ")

		g.By("Make the nodes to NotReady and recover after 480s")
		for _, nodeName := range nodeNames {
			timeSleep := "480"
			var nodeStatus string
			channel := make(chan string)
			go func() {
				cmdStr := fmt.Sprintf(`systemctl stop crio; systemctl stop kubelet; sleep %s; systemctl start crio; systemctl start kubelet`, timeSleep)
				output, _ := oc.AsAdmin().WithoutNamespace().Run("debug").Args("-n", "default", fmt.Sprintf("nodes/%s", nodeName), "--", "chroot", "/host", "/bin/bash", "-c", cmdStr).Output()
				e2e.Logf("!!!!output:%s", output)
				channel <- output
			}()
			defer func() {
				receivedMsg := <-channel
				e2e.Logf("!!!!receivedMsg:%s", receivedMsg)
			}() /*
				cmdStr := fmt.Sprintf(`systemctl stop crio; systemctl stop kubelet; sleep %s; systemctl start crio; systemctl start kubelet`, timeSleep)
				cmd, _, _, err := oc.AsAdmin().WithoutNamespace().Run("debug").Args(fmt.Sprintf("nodes/%s", nodeName), "--", "chroot", "/host", "/bin/bash", "-c", cmdStr).Background()
				defer cmd.Process.Kill()
			*/
			defer func() {
				err = wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 500*time.Second, false, func(ctx context.Context) (bool, error) {
					nodeStatus, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args("node", nodeName, "--no-headers").Output()
					if !strings.Contains(nodeStatus, "NotReady") {
						return true, nil
					}
					return false, nil
				})
				exutil.AssertWaitPollNoErr(err, fmt.Sprintf("The node(%s) doesn't recover to Ready status(%s) after 500s", nodeName, nodeStatus))
			}()
			err = wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 120*time.Second, false, func(ctx context.Context) (bool, error) {
				nodeStatus, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args("node", nodeName, "--no-headers").Output()
				if strings.Contains(nodeStatus, "NotReady") {
					return true, nil
				}
				return false, nil
			})
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("The node(%s) still in Ready status(%s) after 300s", nodeName, nodeStatus))
		}
		g.By("Image registry should not affect by openshift-apiserver and reschedule to other worker")
		podNum := getImageRegistryPodNumber(oc)
		err = wait.PollUntilContextTimeout(context.TODO(), 30*time.Second, 360*time.Second, false, func(ctx context.Context) (bool, error) {
			checkPodsRunningWithLabel(oc, "openshift-image-registry", "docker-registry=default", podNum)
			names, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", "openshift-image-registry", "-l", "docker-registry=default", "--sort-by={.status.startTime}", "-o=jsonpath={..metadata.name}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			podNewNames := strings.Split(names, " ")
			if reflect.DeepEqual(podNames, podNewNames) {
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("The registry does not reschedule after nodes lost"))
	})

	g.It("Author:xiuwang-NonHyperShiftHOST-NonPreRelease-Longduration-Medium-48045-Update global pull secret for additional private registries[Disruptive]", func() {
		var (
			mc = machineConfig{
				name:     "",
				pool:     "worker",
				source:   "",
				path:     "",
				template: "",
			}
		)

		g.By("Setup a private registry")
		oc.SetupProject()
		var regUser, regPass = "testuser", getRandomString()
		tempDataDir, err := extractPullSecret(oc)
		defer os.RemoveAll(tempDataDir)
		o.Expect(err).NotTo(o.HaveOccurred())
		originAuth := filepath.Join(tempDataDir, ".dockerconfigjson")
		htpasswdFile, err := generateHtpasswdFile(tempDataDir, regUser, regPass)
		defer os.RemoveAll(htpasswdFile)
		o.Expect(err).NotTo(o.HaveOccurred())
		regRoute := setSecureRegistryEnableAuth(oc, oc.Namespace(), "myregistry", htpasswdFile, "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3")

		g.By("Push image to private registry")
		newAuthFile, err := appendPullSecretAuth(originAuth, regRoute, regUser, regPass)
		o.Expect(err).NotTo(o.HaveOccurred())
		myimage := regRoute + "/" + oc.Namespace() + "/myimage:latest"
		err = oc.AsAdmin().WithoutNamespace().Run("image").Args("mirror", "quay.io/openshifttest/busybox@sha256:c5439d7db88ab5423999530349d327b04279ad3161d7596d2126dfb5b02bfd1f", myimage, "--insecure", "-a", newAuthFile, "--keep-manifest-list=true", "--filter-by-os=.*").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Make sure the image can't be pulled without auth")
		output, _ := oc.AsAdmin().WithoutNamespace().Run("import-image").Args("firstis:latest", "--from="+myimage, "--reference-policy=local", "--import-mode=PreserveOriginal", "--insecure", "--confirm", "-n", oc.Namespace()).Output()
		o.Expect(output).To(o.ContainSubstring("Unauthorized"))

		g.By("Update pull secret")
		updatePullSecret(oc, newAuthFile)
		defer func() {
			updatePullSecret(oc, originAuth)
			// Update pull-secret will restart crio to apply change to every node
			// Need ensure master and worker update completed
			mc.waitForMCPComplete(oc)
			mc.pool = "master"
			mc.waitForMCPComplete(oc)
		}()
		err = wait.Poll(5*time.Second, 2*time.Minute, func() (bool, error) {
			podList, _ := oc.AdminKubeClient().CoreV1().Pods("openshift-apiserver").List(context.Background(), metav1.ListOptions{LabelSelector: "apiserver=true"})
			for _, pod := range podList.Items {
				output, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-apiserver", pod.Name, "--", "bash", "-c", "cat /var/lib/kubelet/config.json").Output()
				o.Expect(err).NotTo(o.HaveOccurred())
				if !strings.Contains(output, oc.Namespace()) {
					e2e.Logf("Go to next round")
					return false, nil
				}
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "Failed to update apiserver")

		g.By("Make sure the image can be pulled after add auth")
		err = oc.AsAdmin().WithoutNamespace().Run("tag").Args(myimage, "newis:latest", "--reference-policy=local", "--insecure", "--import-mode=PreserveOriginal", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, oc.Namespace(), "newis", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())

	})

	// author: wewang@redhat.com
	g.It("NonHyperShiftHOST-ARO-Author:wewang-Medium-43731-Image registry pods should have anti-affinity rules", func() {
		// When replicas=2 the image registry pods follow requiredDuringSchedulingIgnoredDuringExecution
		// anti-affinity rule on 4.11 and above version, other replicas will follow topologySpreadContraints
		g.By("Check replicas")
		podNum := getImageRegistryPodNumber(oc)
		if podNum != 2 {
			g.Skip("Skip when replicas is not 2")
		}

		g.By("Check pods anti-affinity match requiredDuringSchedulingIgnoredDuringExecution rule when replicas is 2")
		foundrequiredRules := false
		foundrequiredRules = foundAffinityRules(oc, requireRules)
		o.Expect(foundrequiredRules).To(o.BeTrue())

		/*
		   when https://bugzilla.redhat.com/show_bug.cgi?id=2000940 is fixed, will open this part
		   		g.By("Set deployment.apps replica to 0")
		   		err = oc.WithoutNamespace().AsAdmin().Run("patch").Args("deployment.apps/image-registry", "-p", `{"spec":{"replicas":0}}`, "--type=merge", "-n", "openshift-image-registry").Execute()
		   		o.Expect(err).NotTo(o.HaveOccurred())
		   		output, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("co/image-registry", "-o=jsonpath={.status.conditions[0]}").Output()
		   		o.Expect(err).NotTo(o.HaveOccurred())
		   		o.Expect(output).To(o.ContainSubstring("\"status\":\"False\""))
		   		o.Expect(output).To(o.ContainSubstring("The deployment does not have available replicas"))
		   		o.Expect(output).To(o.ContainSubstring("\"type\":\"Available\""))
		   		output, err = oc.WithoutNamespace().AsAdmin().Run("get").Args("config.imageregistry/cluster", "-o=jsonpath={.status.readyReplicas}").Output()
		   		o.Expect(err).NotTo(o.HaveOccurred())
		   		o.Expect(output).To(o.Equal("0"))
		*/
	})

	// author: jitli@redhat.com
	g.It("NonPreRelease-Longduration-Author:jitli-Critical-34895-Image registry can work well on Gov Cloud with custom endpoint defined [Disruptive]", func() {

		g.By("Check platforms")
		exutil.SkipIfPlatformTypeNot(oc, "AWS")

		g.By("Check the cluster is with us-gov")
		output, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("config.image/cluster", "-o=jsonpath={.status.storage.s3.region}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(output, "us-gov") {
			g.Skip("Skip for wrong region")
		}
		g.By("Set status variables")
		expectedStatus1 := map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}

		g.By("Set regionEndpoint if it not set")
		regionEndpoint, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("config.image/cluster", "-o=jsonpath={.status.storage.s3.regionEndpoint}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(regionEndpoint, "https://s3.us-gov-west-1.amazonaws.com") {
			defer func() {
				err = oc.AsAdmin().Run("patch").Args("config.image/cluster", "-p", `{"spec":{"storage":{"s3":{"regionEndpoint": null ,"virtualHostedStyle": null}}}}`, "--type=merge").Execute()
				o.Expect(err).NotTo(o.HaveOccurred())
				err = waitCoBecomes(oc, "image-registry", 240, expectedStatus1)
				o.Expect(err).NotTo(o.HaveOccurred())
			}()
			err = oc.AsAdmin().Run("patch").Args("config.image/cluster", "-p", `{"spec":{"storage":{"s3":{"regionEndpoint": "https://s3.us-gov-west-1.amazonaws.com" ,"virtualHostedStyle": true}}}}`, "--type=merge").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		err = wait.Poll(2*time.Second, 10*time.Second, func() (bool, error) {
			regionEndpoint, err = oc.WithoutNamespace().AsAdmin().Run("get").Args("config.image/cluster", "-o=jsonpath={.status.storage.s3.regionEndpoint}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(regionEndpoint, "https://s3.us-gov-west-1.amazonaws.com") {
				return true, nil
			}
			e2e.Logf("regionEndpoint not found, go next round")
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "regionEndpoint not found")

		g.By("Check if registry operator degraded")
		err = wait.Poll(10*time.Second, 60*time.Second, func() (bool, error) {
			registryDegrade := checkRegistryDegraded(oc)
			if registryDegrade {
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "Image registry is degraded")

		g.By("Import an image with reference-policy=local")
		oc.SetupProject()
		err = oc.WithoutNamespace().AsAdmin().Run("import-image").Args("image-34895", "--from=registry.access.redhat.com/rhel7", "--reference-policy=local", "--confirm", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Start a build")
		checkRegistryFunctionFine(oc, "test-34895", oc.Namespace())

	})

	// author: xiuwangredhat.com
	g.It("Author:xiuwang-ROSA-OSD_CCS-ARO-Critical-48744-High-18995-Pull through for images that have dots in their namespace", func() {

		g.By("Setup a private registry")
		oc.SetupProject()
		var regUser, regPass = "testuser", getRandomString()
		tempDataDir := filepath.Join("/tmp/", fmt.Sprintf("ir-%s", getRandomString()))
		defer os.RemoveAll(tempDataDir)
		err := os.Mkdir(tempDataDir, 0o755)
		if err != nil {
			e2e.Logf("Fail to create directory: %v", err)
		}
		htpasswdFile, err := generateHtpasswdFile(tempDataDir, regUser, regPass)
		defer os.RemoveAll(htpasswdFile)
		o.Expect(err).NotTo(o.HaveOccurred())
		regRoute := setSecureRegistryEnableAuth(oc, oc.Namespace(), "myregistry", htpasswdFile, "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3")

		g.By("Create secret for the private registry")
		err = oc.WithoutNamespace().AsAdmin().Run("create").Args("secret", "docker-registry", "myregistry-auth", "--docker-username="+regUser, "--docker-password="+regPass, "--docker-server="+regRoute, "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().Run("extract").Args("secret/myregistry-auth", "-n", oc.Namespace(), "--confirm", "--to="+tempDataDir).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Mirror image with dots in namespace")
		myimage := regRoute + "/" + fmt.Sprintf("48744-test.%s", getRandomString()) + ":latest"
		err = oc.AsAdmin().WithoutNamespace().Run("image").Args("mirror", "quay.io/openshifttest/busybox@sha256:c5439d7db88ab5423999530349d327b04279ad3161d7596d2126dfb5b02bfd1f", myimage, "--insecure", "-a", tempDataDir+"/.dockerconfigjson", "--keep-manifest-list=true", "--filter-by-os=.*").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		myimage1 := regRoute + "/" + fmt.Sprintf("48744-test1.%s", getRandomString()) + "/rh-test:latest"
		err = oc.AsAdmin().WithoutNamespace().Run("image").Args("mirror", myimage, myimage1, "--insecure", "-a", tempDataDir+"/.dockerconfigjson", "--keep-manifest-list=true", "--filter-by-os=.*").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create imagestream with pull through")
		err = oc.AsAdmin().WithoutNamespace().Run("import-image").Args("first:latest", "--from="+myimage, "--reference-policy=local", "--insecure", "--confirm", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, oc.Namespace(), "first", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("tag").Args(myimage1, "second:latest", "--reference-policy=local", "--insecure", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, oc.Namespace(), "second", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create pod with the imagestreams")
		err = oc.Run("set").Args("image-lookup", "--all", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		expectInfo := `Successfully pulled image "image-registry.openshift-image-registry.svc:5000/` + oc.Namespace()
		createSimpleRunPod(oc, "first:latest", expectInfo)
		createSimpleRunPod(oc, "second:latest", expectInfo)
	})

	// author: xiuwang@redhat.com
	g.It("Author:xiuwang-NonHyperShiftHOST-DisconnectedOnly-High-48739-Pull through works with icsp which source and mirror without full path [Disruptive]", func() {
		var (
			icspFile = filepath.Join(imageRegistryBaseDir, "icsp-full-mirrors.yaml")
			icspsrc  = icspSource{
				name:     "image-policy-fullmirror",
				mirrors:  "",
				source:   "registry.redhat.io",
				template: icspFile,
			}
			mc = machineConfig{
				name:     "",
				pool:     "worker",
				source:   "",
				path:     "",
				template: "",
			}
		)

		g.By("Check if image-policy-aosqe created")
		policy, dis := checkDiscPolicy(oc)
		if dis {
			output, err := oc.WithoutNamespace().AsAdmin().Run("get").Args(policy).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if !strings.Contains(output, "image-policy-aosqe") {
				e2e.Failf("image-policy-aosqe is not created in this disconnect cluster")
			}
		}
		g.By("Create imagestream using source image only match to mirrors namespace in icsp")
		oc.SetupProject()
		err := oc.WithoutNamespace().AsAdmin().Run("import-image").Args("skopeo:latest", "--from=quay.io/openshifttest/skopeo@sha256:d5f288968744a8880f983e49870c0bfcf808703fe126e4fb5fc393fb9e599f65", "--reference-policy=local", "--confirm", "--import-mode=PreserveOriginal", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, oc.Namespace(), "skopeo", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.Run("set").Args("image-lookup", "skopeo", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		expectInfo := `Successfully pulled image "image-registry.openshift-image-registry.svc:5000/` + oc.Namespace()
		createSimpleRunPod(oc, "skopeo:latest", expectInfo)

		g.By("Create imagestream using source image which use the whole mirrors")
		// Get mirror registry with 5000 port, registry.redhat.io/rhel8 images have been mirrored into it
		mirrorReg := checkMirrorRegistry(oc, "registry.redhat.io")
		o.Expect(mirrorReg).NotTo(o.BeEmpty())
		cmd := fmt.Sprintf(`echo '%v' | awk -F ':' '{print $1}'`, mirrorReg)
		mReg6001, err := exec.Command("bash", "-c", cmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		// 6001 mirror registry can't mirror images into, so make up the 5000 port and add icsp for test
		mReg := strings.TrimSuffix(string(mReg6001), "\n") + ":5000"
		icspsrc.mirrors = mReg

		g.By("Create imagecontentsourcepolicy with full mirrors for 5000 port")
		defer func() {
			icspsrc.delete(oc)
			// Update registry of icsp will restart crio to apply change to every node
			// Need ensure master and worker update completed
			mc.waitForMCPComplete(oc)
			mc.pool = "master"
			mc.waitForMCPComplete(oc)
		}()
		icspsrc.create(oc)

		// Get auth of mirror registry
		tempDataDir, err := extractPullSecret(oc)
		defer os.RemoveAll(tempDataDir)
		o.Expect(err).NotTo(o.HaveOccurred())
		// Get the manifest list image for multiarch cluster
		manifestList := getManifestList(oc, mReg+"/rhel8/mysql-80:latest", tempDataDir+"/.dockerconfigjson")
		o.Expect(manifestList).NotTo(o.BeEmpty())
		err = oc.AsAdmin().WithoutNamespace().Run("tag").Args("registry.redhat.io/rhel8/mysql-80@"+manifestList, "mysqlx:latest", "--reference-policy=local", "--import-mode=PreserveOriginal", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, oc.Namespace(), "mysqlx", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.Run("set").Args("image-lookup", "mysqlx", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		createSimpleRunPod(oc, "mysqlx:latest", expectInfo)
	})

	// author: jitli@redhat.com
	g.It("Author:jitli-ConnectedOnly-VMonly-High-48710-Should be able to deploy an existing image from private docker.io registry", func() {

		oc.SetupProject()
		g.By("Create the secret for docker private image")
		dockerConfig := filepath.Join("/home", "cloud-user", ".docker", "auto", "48710.json")
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", oc.Namespace(), "secret", "docker-registry", "secret48710", fmt.Sprintf("--from-file=.dockerconfigjson=%s", dockerConfig)).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create a imagestream with a docker private image")
		// TODO double check docker.io/irqe/busybox:latest is a manifest list built for all the arches
		output, err := oc.AsAdmin().WithoutNamespace().Run("tag").Args("docker.io/irqe/busybox:latest", "test48710:latest", "--reference-policy=local", "--import-mode=PreserveOriginal", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Tag test48710:latest set"))
		err = waitForAnImageStreamTag(oc, oc.Namespace(), "test48710", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create pod with the imagestream")
		expectInfo := `Successfully pulled image "image-registry.openshift-image-registry.svc:5000/` + oc.Namespace()
		newAppUseImageStream(oc, oc.Namespace(), "test48710:latest", expectInfo)

	})

	// author: jitli@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:jitli-LEVEL0-Critical-48959-Should be able to get public images connect to the server and have basic auth credentials", func() {

		// Skip Hypershift external OIDC clusters against which all test cases run as the same (external) user
		isExternalOIDCCluster, err := exutil.IsExternalOIDCCluster(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		if isExternalOIDCCluster {
			g.Skip("Skipping the test as we are running against a Hypershift external OIDC cluster")
		}
		g.By("Create route to expose the registry")
		routeName := getRandomString()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("route", routeName, "-n", "openshift-image-registry").Execute()
		host := exposeRouteFromSVC(oc, "reencrypt", "openshift-image-registry", routeName, "image-registry")
		waitRouteReady(host)

		g.By("Grant public access to the openshift namespace")
		defer oc.AsAdmin().WithoutNamespace().Run("policy").Args("remove-role-from-group", "system:image-puller", "system:unauthenticated", "--namespace", "openshift").Execute()
		output, err := oc.AsAdmin().WithoutNamespace().Run("policy").Args("add-role-to-group", "system:image-puller", "system:unauthenticated", "--namespace", "openshift").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("clusterrole.rbac.authorization.k8s.io/system:image-puller added: \"system:unauthenticated\""))

		g.By("Try to fetch image metadata")
		output, err = oc.AsAdmin().Run("image").Args("info", "--insecure", host+"/openshift/tools:latest", "--show-multiarch").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).NotTo(o.ContainSubstring("error: unauthorized: authentication required"))
		o.Expect(output).NotTo(o.ContainSubstring("Unable to connect to the server: no basic auth credentials"))
		o.Expect(output).To(o.ContainSubstring(host + "/openshift/tools:latest"))

	})

	// author: yyou@redhat.com
	g.It("VMonly-NonPreRelease-Longduration-Author:yyou-Critical-44037-Could configure swift authentication using application credentials [Disruptive]", func() {
		storagetype, _ := getRegistryStorageConfig(oc)
		if storagetype != "swift" {
			g.Skip("Skip for non-supported platform")
		}

		g.By("Set status variables")
		expectedStatus1 := map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}

		g.By("Configure image-registry-private-configuration secret to use new application credentials")
		defer func() {
			err := oc.AsAdmin().WithoutNamespace().Run("set").Args("data", "secret/image-registry-private-configuration", "--from-literal=REGISTRY_STORAGE_SWIFT_APPLICATIONCREDENTIALID='' ", "--from-literal=REGISTRY_STORAGE_SWIFT_APPLICATIONCREDENTIALNAME='' ", "--from-literal=REGISTRY_STORAGE_SWIFT_APPLICATIONCREDENTIALSECRET='' ", "-n", "openshift-image-registry").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			err = waitCoBecomes(oc, "image-registry", 240, expectedStatus1)
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		err := oc.AsAdmin().WithoutNamespace().Run("set").Args("data", "secret/image-registry-private-configuration", "--from-file=REGISTRY_STORAGE_SWIFT_APPLICATIONCREDENTIALID=/root/auto/44037/applicationid", "--from-file=REGISTRY_STORAGE_SWIFT_APPLICATIONCREDENTIALNAME=/root/auto/44037/applicationname", "--from-file=REGISTRY_STORAGE_SWIFT_APPLICATIONCREDENTIALSECRET=/root/auto/44037/applicationsecret", "-n", "openshift-image-registry").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check image registry pod")
		podNum := getImageRegistryPodNumber(oc)
		checkPodsRunningWithLabel(oc, "openshift-image-registry", "docker-registry=default", podNum)

		g.By("push/pull image to registry")
		oc.SetupProject()
		checkRegistryFunctionFine(oc, "test-44037", oc.Namespace())

	})

	// author: jitli@redhat.com
	// Cover test case: OCP-46069 and 49886
	g.It("NonPreRelease-Longduration-Author:jitli-Critical-46069-High-49886-Could override the default topology constraints and Topology Constraints works well in non zone cluster [Disruptive]", func() {
		g.By("Set status variables")
		expectedStatus1 := map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}

		g.By("Get image registry pod replicas num")
		podNum := getImageRegistryPodNumber(oc)

		g.By("Check cluster whose nodes have no zone label set")
		if !checkRegistryUsingFSVolume(oc) {

			g.By("Platform with zone, Check the image-registry default topology")
			output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deploy", "image-registry", "-n", "openshift-image-registry", "-o=jsonpath={.spec.template.spec.topologySpreadConstraints}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).To(o.ContainSubstring(`{"labelSelector":{"matchLabels":{"docker-registry":"default"}},"maxSkew":1,"topologyKey":"topology.kubernetes.io/zone","whenUnsatisfiable":"DoNotSchedule"}`))
			o.Expect(output).To(o.ContainSubstring(`{"labelSelector":{"matchLabels":{"docker-registry":"default"}},"maxSkew":1,"topologyKey":"kubernetes.io/hostname","whenUnsatisfiable":"DoNotSchedule"}`))
			o.Expect(output).To(o.ContainSubstring(`{"labelSelector":{"matchLabels":{"docker-registry":"default"}},"maxSkew":1,"topologyKey":"node-role.kubernetes.io/worker","whenUnsatisfiable":"DoNotSchedule"}`))

			g.By("Check whether these two registry pods are running in different workers")
			NodeList := getPodNodeListByLabel(oc, "openshift-image-registry", "docker-registry=default")
			o.Expect(NodeList[0]).NotTo(o.Equal(NodeList[1]))

			g.By("Configure topology")
			defer func() {
				err = oc.AsAdmin().Run("patch").Args("config.image/cluster", "-p", `{"spec":{"topologySpreadConstraints":null}}`, "--type=merge").Execute()
				o.Expect(err).NotTo(o.HaveOccurred())
				err = waitCoBecomes(oc, "image-registry", 240, expectedStatus1)
				o.Expect(err).NotTo(o.HaveOccurred())
			}()
			err = oc.AsAdmin().Run("patch").Args("config.image/cluster", "-p", `{"spec":{"topologySpreadConstraints":[{"labelSelector":{"matchLabels":{"docker-registry":"bar"}},"maxSkew":2,"topologyKey":"zone","whenUnsatisfiable":"ScheduleAnyway"}]}}`, "--type=merge").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Check if the topology has been override in image registry deploy")
			err = wait.Poll(3*time.Second, 9*time.Second, func() (bool, error) {
				output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("deploy", "image-registry", "-n", "openshift-image-registry", "-o=jsonpath={.spec.template.spec.topologySpreadConstraints}").Output()
				o.Expect(err).NotTo(o.HaveOccurred())
				if strings.Contains(output, `{"labelSelector":{"matchLabels":{"docker-registry":"bar"}},"maxSkew":2,"topologyKey":"zone","whenUnsatisfiable":"ScheduleAnyway"}`) {
					return true, nil
				}
				e2e.Logf("Continue to next round")
				return false, nil
			})
			exutil.AssertWaitPollNoErr(err, "The topology has not been overridden")

			g.By("Check if image registry pods go to running")
			checkPodsRunningWithLabel(oc, "openshift-image-registry", "docker-registry=default", podNum)

			g.By("check registry working well")
			oc.SetupProject()
			checkRegistryFunctionFine(oc, "test-46069", oc.Namespace())

		} else {

			g.By("Platform without zone, Check the image-registry default topology in non zone label cluster")
			output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deploy", "image-registry", "-n", "openshift-image-registry", "-o=jsonpath={.spec.template.spec.topologySpreadConstraints}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).To(o.ContainSubstring(`{"labelSelector":{"matchLabels":{"docker-registry":"default"}},"maxSkew":1,"topologyKey":"kubernetes.io/hostname","whenUnsatisfiable":"DoNotSchedule"}`))
			o.Expect(output).To(o.ContainSubstring(`{"labelSelector":{"matchLabels":{"docker-registry":"default"}},"maxSkew":1,"topologyKey":"node-role.kubernetes.io/worker","whenUnsatisfiable":"DoNotSchedule"}`))

			g.By("Check registry pod")
			checkPodsRunningWithLabel(oc, "openshift-image-registry", "docker-registry=default", podNum)

			g.By("check registry working well")
			oc.SetupProject()
			checkRegistryFunctionFine(oc, "test-49886", oc.Namespace())
		}

	})

	// author: jitli@redhat.com
	g.It("NonPreRelease-Longduration-Author:jitli-Medium-46082-Increase replicas to match one zone have one pod [Disruptive]", func() {

		g.By("Check platforms")
		if checkRegistryUsingFSVolume(oc) {
			g.Skip("Skip for fs volume")
		}

		g.By("Check the nodes with Each zone have one worker")
		workerNodes, _ := exutil.GetClusterNodesBy(oc, "worker")
		if len(workerNodes) != 3 {
			g.Skip("Skip for not three workers")
		}
		g.By("Set status variables")
		expectedStatus1 := map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}

		zone, err := oc.AsAdmin().Run("get").Args("node", "-l", "node-role.kubernetes.io/worker", `-o=jsonpath={.items[*].metadata.labels.topology\.kubernetes\.io\/zone}`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		zoneList := strings.Fields(zone)
		if strings.EqualFold(zoneList[0], zoneList[1]) || strings.EqualFold(zoneList[0], zoneList[2]) || strings.EqualFold(zoneList[1], zoneList[2]) {

			e2e.Logf("Zone: %v . Doesn't conform Each zone have one worker", zone)
			g.By("Only check pods on different worker")
			samenum, diffnum := comparePodHostIP(oc)
			e2e.Logf("%v %v", samenum, diffnum)
			o.Expect(samenum == 0).To(o.BeTrue())
			o.Expect(diffnum == 1).To(o.BeTrue())

		} else {

			e2e.Logf("Zone: %v . Each zone have one worker", zone)
			g.By("Scale registry pod to 3 then to 4")
			/*
				replicas=2 has the pod affinity configure
				When change replicas=2 to other number, the registry pods will be recreated.
				When changed replicas to 3, the Pods will be scheduled to each worker.
				Due to the RollingUpdate policy, it will also count the old pods that are running.
				So there is a certain probability that two pods are running in the same worker
				To deal with the problem, we change replicas to 3 then 4 to monitor the new pod followoing topologyspread.

				When the kubernetes issues fixed, we can update the checkpoint ,
				https://bugzilla.redhat.com/show_bug.cgi?id=2024888#c11
			*/
			defer func() {
				err = oc.AsAdmin().Run("patch").Args("config.image/cluster", "-p", `{"spec":{"replicas":2}}`, "--type=merge").Execute()
				o.Expect(err).NotTo(o.HaveOccurred())
				err = waitCoBecomes(oc, "image-registry", 240, expectedStatus1)
				o.Expect(err).NotTo(o.HaveOccurred())
			}()
			err := oc.AsAdmin().Run("patch").Args("config.image/cluster", "-p", `{"spec":{"replicas":3}}`, "--type=merge").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			checkPodsRunningWithLabel(oc, "openshift-image-registry", "docker-registry=default", 3)

			err = oc.AsAdmin().Run("patch").Args("config.image/cluster", "-p", `{"spec":{"replicas":4}}`, "--type=merge").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			checkPodsRunningWithLabel(oc, "openshift-image-registry", "docker-registry=default", 4)

			g.By("Check if image registry pods run in each zone")
			samenum, diffnum := comparePodHostIP(oc)
			e2e.Logf("%v %v", samenum, diffnum)
			o.Expect(samenum == 1).To(o.BeTrue())
			o.Expect(diffnum == 5).To(o.BeTrue())

		}

		g.By("check registry working well")
		oc.SetupProject()
		checkRegistryFunctionFine(oc, "test-46082", oc.Namespace())

	})

	// author: jitli@redhat.com
	g.It("NonPreRelease-Longduration-Author:jitli-Critical-46083-Topology Constraints works well in SNO environment [Disruptive]", func() {

		g.By("Check platforms")
		platformtype, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.spec.platformSpec.type}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		platforms := map[string]bool{
			"AWS":          true,
			"Azure":        true,
			"GCP":          true,
			"OpenStack":    true,
			"AlibabaCloud": true,
			"IBMCloud":     true,
		}
		if !platforms[platformtype] {
			g.Skip("Skip for non-supported platform")
		}

		g.By("Check whether the environment is SNO")
		// Only 1 master, 1 worker node and with the same hostname.
		masterNodes, _ := exutil.GetClusterNodesBy(oc, "master")
		workerNodes, _ := exutil.GetClusterNodesBy(oc, "worker")
		if len(masterNodes) == 1 && len(workerNodes) == 1 && masterNodes[0] == workerNodes[0] {
			e2e.Logf("This is a SNO cluster")
		} else {
			g.Skip("Not SNO cluster - skipping test ...")
		}

		g.By("Check the image-registry default topology in SNO cluster")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deploy", "image-registry", "-n", "openshift-image-registry", "-o=jsonpath={.spec.template.spec.topologySpreadConstraints}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring(`{"labelSelector":{"matchLabels":{"docker-registry":"default"}},"maxSkew":1,"topologyKey":"topology.kubernetes.io/zone","whenUnsatisfiable":"DoNotSchedule"}`))
		o.Expect(output).To(o.ContainSubstring(`{"labelSelector":{"matchLabels":{"docker-registry":"default"}},"maxSkew":1,"topologyKey":"kubernetes.io/hostname","whenUnsatisfiable":"DoNotSchedule"}`))
		o.Expect(output).To(o.ContainSubstring(`{"labelSelector":{"matchLabels":{"docker-registry":"default"}},"maxSkew":1,"topologyKey":"node-role.kubernetes.io/worker","whenUnsatisfiable":"DoNotSchedule"}`))

		g.By("Check registry pod")
		checkPodsRunningWithLabel(oc, "openshift-image-registry", "docker-registry=default", 1)

		g.By("Scale registry pod to 2")
		defer func() {
			err = oc.AsAdmin().Run("patch").Args("config.image/cluster", "-p", `{"spec":{"replicas":1}}`, "--type=merge").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			checkPodsRunningWithLabel(oc, "openshift-image-registry", "docker-registry=default", 1)
		}()
		err = oc.AsAdmin().Run("patch").Args("config.image/cluster", "-p", `{"spec":{"replicas":2}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check registry new pods")
		err = wait.Poll(20*time.Second, 1*time.Minute, func() (bool, error) {
			podsStatus, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-o", "wide", "-n", "openshift-image-registry", "-l", "docker-registry=default", "--sort-by={.status.phase}", "-o=jsonpath={.items[*].status.phase}").Output()
			if podsStatus != "Pending Pending Running" {
				e2e.Logf("the pod status is %v, continue to next round", podsStatus)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "Pods list are not one Running two Pending")

		g.By("Scale registry pod to 3")
		err = oc.AsAdmin().Run("patch").Args("config.image/cluster", "-p", `{"spec":{"replicas":3}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check if all pods are running well")
		checkPodsRunningWithLabel(oc, "openshift-image-registry", "docker-registry=default", 3)

	})

	// author: jitli@redhat.com
	g.It("NonHyperShiftHOST-Author:jitli-NonPreRelease-Longduration-Medium-22032-High-50219-Setting nodeSelector and tolerations on nodes with taints registry works well [Disruptive]", func() {
		g.By("Set status variables")
		expectedStatus1 := map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}

		g.By("Check the image-registry default topology")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deploy", "image-registry", "-n", "openshift-image-registry", "-o=jsonpath={.spec.template.spec.topologySpreadConstraints}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring(`{"labelSelector":{"matchLabels":{"docker-registry":"default"}},"maxSkew":1,"topologyKey":"kubernetes.io/hostname","whenUnsatisfiable":"DoNotSchedule"}`))
		o.Expect(output).To(o.ContainSubstring(`{"labelSelector":{"matchLabels":{"docker-registry":"default"}},"maxSkew":1,"topologyKey":"node-role.kubernetes.io/worker","whenUnsatisfiable":"DoNotSchedule"}`))

		g.By("Setting both nodeSelector and tolerations on nodes with taints")
		defer func() {
			err = oc.AsAdmin().Run("patch").Args("config.image/cluster", "-p", `{"spec":{"nodeSelector":null,"tolerations":null}}`, "--type=merge").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			err = waitCoBecomes(oc, "image-registry", 240, expectedStatus1)
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		err = oc.AsAdmin().Run("patch").Args("config.image/cluster", "-p", `{"spec":{"nodeSelector":{"node-role.kubernetes.io/master": ""},"tolerations":[{"effect":"NoSchedule","key":"node-role.kubernetes.io/master","operator":"Exists"}]}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check registry pod well")
		podNum := getImageRegistryPodNumber(oc)
		checkPodsRunningWithLabel(oc, "openshift-image-registry", "docker-registry=default", podNum)

		g.By("Check the image-registry default topology removed")
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("deploy", "image-registry", "-n", "openshift-image-registry", "-o=jsonpath={.spec.template.spec.topologySpreadConstraints}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).NotTo(o.ContainSubstring(`{"labelSelector":{"matchLabels":{"docker-registry":"default"}},"maxSkew":1,"topologyKey":"topology.kubernetes.io/zone","whenUnsatisfiable":"DoNotSchedule"}`))
		o.Expect(output).NotTo(o.ContainSubstring(`{"labelSelector":{"matchLabels":{"docker-registry":"default"}},"maxSkew":1,"topologyKey":"kubernetes.io/hostname","whenUnsatisfiable":"DoNotSchedule"}`))
		o.Expect(output).NotTo(o.ContainSubstring(`{"labelSelector":{"matchLabels":{"docker-registry":"default"}},"maxSkew":1,"topologyKey":"node-role.kubernetes.io/worker","whenUnsatisfiable":"DoNotSchedule"}`))

		g.By("check registry working well")
		oc.SetupProject()
		checkRegistryFunctionFine(oc, "test1-50219", oc.Namespace())

		g.By("Setting nodeSelector on node without taints")
		err = oc.AsAdmin().Run("patch").Args("config.image/cluster", "-p", `{"spec":{"nodeSelector":null,"tolerations":null}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().Run("patch").Args("config.image/cluster", "-p", `{"spec":{"nodeSelector":{"node-role.kubernetes.io/worker": ""}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check registry pod well")
		checkPodsRunningWithLabel(oc, "openshift-image-registry", "docker-registry=default", podNum)

		g.By("check registry working well")
		checkRegistryFunctionFine(oc, "test2-50219", oc.Namespace())
	})

	// author: xiuwang@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:xiuwang-Critical-49455-disableRedirect should work when image registry configured object storage", func() {
		// Skip Hypershift external OIDC clusters against which all test cases run as the same (external) user
		isExternalOIDCCluster, err := exutil.IsExternalOIDCCluster(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		if isExternalOIDCCluster {
			g.Skip("Skipping the test as we are running against a Hypershift external OIDC cluster")
		}
		g.By("Get registry storage info")
		storagetype, _ := getRegistryStorageConfig(oc)
		if storagetype == "pvc" || storagetype == "emptyDir" {
			g.Skip("Skip disableRedirect test for fs volume")
		}

		credentials, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret/image-registry-private-configuration", "-n", "openshift-image-registry", "-o=jsonpath={.data.REGISTRY_STORAGE_GCS_KEYFILE}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		sDec, err := base64.StdEncoding.DecodeString(credentials)
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(string(sDec), "external_account") {
			g.Skip("Skip disableRedirect test on gcp sts test, bz2111311")
		}

		g.By("Create route to expose the registry")
		routeName := getRandomString()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("route", routeName, "-n", "openshift-image-registry").Execute()
		regRoute := exposeRouteFromSVC(oc, "reencrypt", "openshift-image-registry", routeName, "image-registry")
		waitRouteReady(regRoute)

		g.By("push image to registry")
		oc.SetupProject()
		checkRegistryFunctionFine(oc, "test-49455", oc.Namespace())
		authFile, err := saveImageRegistryAuth(oc, "builder", regRoute, oc.Namespace())
		defer os.RemoveAll(authFile)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check disableRedirect function")
		// disableRedirect: Controls whether to route all data through the registry, rather than redirecting to the back end. Defaults to false.
		myimage := regRoute + "/" + oc.Namespace() + "/test-49455:latest"
		cmd := "oc image info " + myimage + " -ojson -a " + authFile + " --insecure|jq -r '.layers[0].digest'"
		imageblob, err := exec.Command("bash", "-c", cmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		token, err := getSAToken(oc, "builder", oc.Namespace())
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(token).NotTo(o.BeEmpty())
		cmd = "curl -Lks -u " + oc.Username() + ":" + token + " -I HEAD https://" + regRoute + "/v2/" + oc.Namespace() + "/test-49455/blobs/" + string(imageblob)
		err = wait.Poll(10*time.Second, 1*time.Minute, func() (bool, error) {
			curlOutput, err := exec.Command("bash", "-c", cmd).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			// When directed to backend storage, the data store format likes this
			// https://<cloud dns>/<container or bucket>/docker/registry/v2/blobs/sha256/<blob>
			if strings.Contains(string(curlOutput), "docker/registry/v2/blobs/sha256") {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "The disableRedirect doesn't work")

	})

	// author: xiuwang@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:xiuwang-Low-51055-Image pullthrough does pass 429 errors back to capable clients", func() {

		g.By("Create a registry could limit quota")
		oc.SetupProject()
		regRoute := setSecureRegistryWithoutAuth(oc, oc.Namespace(), "myregistry", "quay.io/openshifttest/registry-toomany-request@sha256:56b816ca086d714680235d0ee96320bc9b1375a8abd037839d17a8759961e842", "8080")
		o.Expect(regRoute).NotTo(o.BeEmpty())
		err := oc.Run("set").Args("resources", "deploy/myregistry", "--requests=cpu=100m,memory=128Mi").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		checkPodsRunningWithLabel(oc, oc.Namespace(), "app=myregistry", 1)

		limitURL := "curl -k -XPOST -d 'c=150' https://" + regRoute
		_, err = exec.Command("bash", "-c", limitURL).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Push image to the limit registry")
		myimage := regRoute + "/" + oc.Namespace() + "/myimage:latest"
		err = oc.AsAdmin().WithoutNamespace().Run("image").Args("mirror", "quay.io/openshifttest/busybox@sha256:fc01c814423a9fe24a0de038595eb6ed46dea56c7479ac24396ed4660ed91b1f", myimage, "--insecure", "--keep-manifest-list=true", "--filter-by-os=.*").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.Run("import-image").Args("test-51055", "--from", myimage, "--confirm", "--reference-policy=local", "--insecure").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, oc.Namespace(), "test-51055", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Limit the registry quota")
		limitURL = "curl -k -XPOST -d 'c=1' https://" + regRoute
		_, err = exec.Command("bash", "-c", limitURL).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create pod with the imagestream")
		err = oc.Run("set").Args("image-lookup", "test-51055", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		expectInfo := "got 429 Too Many Requests"
		createSimpleRunPod(oc, "test-51055:latest", expectInfo)

	})

	// author: jitli@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-Author:jitli-Medium-49747-Configure image registry to skip volume SELinuxLabel [Disruptive]", func() {
		// TODO: remove this skip when the builds v1 API will support producing manifest list images
		architecture.SkipArchitectures(oc, architecture.MULTI)
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "image_registry")
			machineConfigSource = filepath.Join(buildPruningBaseDir, "machineconfig.yaml")
			runtimeClassSource  = filepath.Join(buildPruningBaseDir, "runtimeClass.yaml")
			mc                  = machineConfig{
				name:     "49747-worker-selinux-configuration",
				pool:     "worker",
				source:   "data:text/plain;charset=utf-8;base64,W2NyaW8ucnVudGltZS5ydW50aW1lcy5zZWxpbnV4XQpydW50aW1lX3BhdGggPSAiL3Vzci9iaW4vcnVuYyIKcnVudGltZV9yb290ID0gIi9ydW4vcnVuYyIKcnVudGltZV90eXBlID0gIm9jaSIKYWxsb3dlZF9hbm5vdGF0aW9ucyA9IFsiaW8ua3ViZXJuZXRlcy5jcmktby5UcnlTa2lwVm9sdW1lU0VMaW51eExhYmVsIl0K",
				path:     "/etc/crio/crio.conf.d/01-runtime-selinux.conf",
				template: machineConfigSource,
			}

			rtc = runtimeClass{
				name:     "selinux-49747",
				handler:  "selinux",
				template: runtimeClassSource,
			}
		)

		g.By("Register defer block to delete mc and wait for mcp/worker rollback to complete")
		defer mc.delete(oc)

		g.By("Create machineconfig to add selinux cri-o config and wait for mcp update to complete")
		mc.createWithCheck(oc)

		g.By("verify new crio drop-in file exists and content is correct")
		workerNode, workerNodeErr := exutil.GetFirstWorkerNode(oc)
		o.Expect(workerNodeErr).NotTo(o.HaveOccurred())
		o.Expect(workerNode).NotTo(o.BeEmpty())
		err := wait.Poll(10*time.Second, 3*time.Minute, func() (bool, error) {
			selinuxStatus, statusErr := exutil.DebugNodeWithChroot(oc, workerNode, "cat", "/etc/crio/crio.conf.d/01-runtime-selinux.conf")
			if statusErr == nil {
				if strings.Contains(selinuxStatus, "io.kubernetes.cri-o.TrySkipVolumeSELinuxLabel") {
					e2e.Logf("runtime-selinux.conf updated")
					return true, nil
				}
			}
			e2e.Logf("runtime-selinux.conf not update, err: %v", statusErr)
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "runtime-selinux.conf not update")

		g.By("Register defer block to delete new runtime class")
		defer rtc.delete(oc)

		g.By("Create new runtimeClass from template and verify it's done")
		rtc.createWithCheck(oc)

		g.By("Override the image registry annonation and runtimeclass")
		defer oc.AsAdmin().Run("patch").Args("config.imageregistry.operator.openshift.io/cluster", "-p", `{"spec":{"unsupportedConfigOverrides":null}}`, "--type=merge").Execute()
		configPatchErr := oc.AsAdmin().Run("patch").Args("config.imageregistry.operator.openshift.io/cluster", "-p", `{"spec":{"unsupportedConfigOverrides":{"deployment":{"annotations":{"io.kubernetes.cri-o.TrySkipVolumeSELinuxLabel":"true"},"runtimeClassName":"`+rtc.name+`"}}}}`, "--type=merge").Execute()
		o.Expect(configPatchErr).NotTo(o.HaveOccurred())
		podNum := getImageRegistryPodNumber(oc)
		checkPodsRunningWithLabel(oc, "openshift-image-registry", "docker-registry=default", podNum)

		g.By("Check the registry files label")
		err = wait.Poll(10*time.Second, 3*time.Minute, func() (bool, error) {
			selinuxLabel, selinuxLabelErr := oc.AsAdmin().Run("get").Args("pod", "-n", "openshift-image-registry", "-l", "docker-registry=default", `-ojsonpath={.items..metadata.annotations.io\.kubernetes\.cri-o\.TrySkipVolumeSELinuxLabel}`).Output()
			getRuntimeClassName, runtimeClassNameErr := oc.AsAdmin().Run("get").Args("pod", "-n", "openshift-image-registry", "-l", "docker-registry=default", `-ojsonpath={.items..spec.runtimeClassName}`).Output()

			if strings.Contains(selinuxLabel, "true") && strings.Contains(getRuntimeClassName, rtc.name) {
				e2e.Logf("pod metadata updated")
				return true, nil
			}
			e2e.Logf("pod metadata not update, selinuxLabel:%v %v, runtimeClassName:%v %v", selinuxLabel, selinuxLabelErr, getRuntimeClassName, runtimeClassNameErr)
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "pod metadata not update ")

		g.By("check registry working well")
		oc.SetupProject()
		checkRegistryFunctionFine(oc, "test-49747", oc.Namespace())
	})

	// author: jitli@redhat.com
	g.It("Author:jitli-Critical-23817-Check registry operator storage setup on GCP", func() {

		if exutil.CheckPlatform(oc) != "gcp" {
			g.Skip("Skip for non-supported platform, only GCP")
		}

		g.By("Get the GCS key")
		gcsKey, gcsKeyErr := oc.WithoutNamespace().AsAdmin().Run("get").Args("secret", "-n", "openshift-image-registry", "image-registry-private-configuration", `-ojsonpath={.data.REGISTRY_STORAGE_GCS_KEYFILE}`).Output()
		o.Expect(gcsKeyErr).NotTo(o.HaveOccurred())
		o.Expect(gcsKey).NotTo(o.BeEmpty())

		g.By("Check environment variables")
		envList, envListErr := oc.AsAdmin().WithoutNamespace().Run("set").Args("env", "deployment/image-registry", "--list=true", "-n", "openshift-image-registry").Output()
		o.Expect(envListErr).NotTo(o.HaveOccurred())
		o.Expect(envList).To(o.ContainSubstring("REGISTRY_STORAGE=gcs"))
		o.Expect(envList).To(o.ContainSubstring("REGISTRY_STORAGE_GCS_BUCKET="))
		o.Expect(envList).To(o.ContainSubstring("REGISTRY_STORAGE_GCS_KEYFILE=/gcs/keyfile"))

		g.By("Check registry pod well")
		expectedStatus1 := map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}
		err := waitCoBecomes(oc, "image-registry", 240, expectedStatus1)
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	// author: jitli@redhat.com
	g.It("NonPreRelease-Longduration-Author:jitli-Medium-22031-Config CPU and memory for internal regsistry [Disruptive]", func() {

		g.By("Set up registry resources")
		podNum := getImageRegistryPodNumber(oc)
		initialConfig, err := oc.AsAdmin().Run("get").Args("config.image/cluster", "-ojsonpath={.spec.resources}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if initialConfig == "" {
			initialConfig = `null`
		}

		defer func() {
			g.By("Recover resources for imageregistry")
			err := oc.AsAdmin().Run("patch").Args("config.image/cluster", "-p", `{"spec":{"resources":`+initialConfig+`}}`, "--type=merge").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			g.By("Check registry pod well")
			checkPodsRunningWithLabel(oc, "openshift-image-registry", "docker-registry=default", podNum)
		}()

		err = oc.AsAdmin().Run("patch").Args("config.image/cluster", "-p", `{"spec":{"resources":{"limits":{"cpu":"100m","memory":"512Mi"}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check the image-registry pod resources")
		err = wait.Poll(5*time.Second, 30*time.Second, func() (bool, error) {
			resources, resourcesErr := oc.AsAdmin().Run("get").Args("pod", "-n", "openshift-image-registry", "-l", "docker-registry=default", `-ojsonpath={.items..spec..resources.limits}`).Output()
			if strings.Contains(resources, "100m") && strings.Contains(resources, "512Mi") {
				e2e.Logf("pod metadata updated")
				return true, nil
			}
			e2e.Logf("pod metadata not update, nodeSelector:%v %v", resources, resourcesErr)
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "pod metadata not update")
		checkPodsRunningWithLabel(oc, "openshift-image-registry", "docker-registry=default", podNum)
	})

	// author: jitli@redhat.com
	g.It("VMonly-Author:jitli-Critical-24133-TLS can be added to user-defined registry route [Disruptive]", func() {

		registryCrt := filepath.Join("/root", "auto", "24133", "myregistry.crt")
		registryKey := filepath.Join("/root", "auto", "24133", "myregistry.key")

		initialConfig, err := oc.AsAdmin().Run("get").Args("config.image/cluster", "-ojsonpath={.spec.routes}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if initialConfig == "" {
			initialConfig = `null`
		}
		g.By("Create secret tls")
		tls := "test24133-tls-" + exutil.GetRandomString()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", "openshift-image-registry", "secret", tls).Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", "openshift-image-registry", "secret", "tls", "--cert="+registryCrt, "--key="+registryKey, tls).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Set up the routes")
		expectedStatus1 := map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}
		defer func() {
			g.By("Recover resources for imageregistry")
			err := oc.AsAdmin().Run("patch").Args("config.image/cluster", "-p", `{"spec":{"routes":`+initialConfig+`}}`, "--type=merge").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			g.By("Check registry pod well")
			err = waitCoBecomes(oc, "image-registry", 240, expectedStatus1)
			o.Expect(err).NotTo(o.HaveOccurred())
		}()

		err = oc.AsAdmin().Run("patch").Args("config.image/cluster", "-p", `{"spec":{"routes":[{"hostname":"test24133route-image-registry.openshift.com","name":"test24133route","secretName":"`+tls+`"}]}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		err = wait.Poll(3*time.Second, 30*time.Second, func() (bool, error) {
			certificate, certificateErr := oc.AsAdmin().Run("get").Args("routes", "test24133route", "-n", "openshift-image-registry", "-ojsonpath={.spec.tls.certificate}").Output()
			key, keyErr := oc.AsAdmin().Run("get").Args("routes", "test24133route", "-n", "openshift-image-registry", "-ojsonpath={.spec.tls.key}").Output()
			if certificate != "" && key != "" {
				e2e.Logf("get certificate and key successfully")
				return true, nil
			}
			e2e.Logf("Failed to get certificate and key err:%v %v", certificateErr, keyErr)
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "Failed to get certificate and key")

	})

	// author: jitli@redhat.com
	g.It("ROSA-OSD_CCS-ARO-ConnectedOnly-Author:jitli-Medium-12766-Allow imagestream request build config triggers by different mode", func() {

		oc.SetupProject()
		g.By("Import an image to create imagestream")
		err := oc.AsAdmin().WithoutNamespace().Run("tag").Args("quay.io/openshifttest/ruby-27@sha256:8f71dd40e3f55d90662a63cb9f02b59e75ed7ac1e911c7919fd14fbfad431348", "ruby-test12766:latest", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, oc.Namespace(), "ruby-test12766", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create app with imagestream and check the build info")
		err = oc.AsAdmin().WithoutNamespace().Run("new-build").Args("--image-stream=ruby-test12766", "--code=https://github.com/sclorg/ruby-ex.git", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		pollErr := wait.Poll(6*time.Second, 30*time.Second, func() (bool, error) {
			bc, bcErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("bc", "ruby-ex", "-ojsonpath={.status.imageChangeTriggers..lastTriggeredImageID}", "-n", oc.Namespace()).Output()
			o.Expect(bcErr).NotTo(o.HaveOccurred())
			if strings.Contains(bc, "openshifttest/ruby-27") {
				return true, nil
			}
			e2e.Logf("Failed to get bc with ruby-ex, continue to next round")
			return false, nil
		})
		exutil.AssertWaitPollNoErr(pollErr, "Failed to get bc with ruby-ex")

		g.By("Import an image to create imagestream with --reference-policy=local")
		err = oc.AsAdmin().WithoutNamespace().Run("tag").Args("quay.io/openshifttest/ruby-27@sha256:8f71dd40e3f55d90662a63cb9f02b59e75ed7ac1e911c7919fd14fbfad431348", "ruby-test12766-local:latest", "--reference-policy=local", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, oc.Namespace(), "ruby-test12766-local", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create app with imagestream and check the build info")
		err = oc.AsAdmin().WithoutNamespace().Run("new-build").Args("--image-stream=ruby-test12766-local", "--code=https://github.com/sclorg/ruby-ex.git", "--name=rubyapp-12766-local", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		pollErr = wait.Poll(6*time.Second, 30*time.Second, func() (bool, error) {
			bc, bcErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("bc", "rubyapp-12766-local", "-ojsonpath={.status.imageChangeTriggers..lastTriggeredImageID}", "-n", oc.Namespace()).Output()
			o.Expect(bcErr).NotTo(o.HaveOccurred())
			if strings.Contains(bc, oc.Namespace()+"/ruby-test12766-local") {
				return true, nil
			}
			e2e.Logf("Failed to get bc with ruby-test12766-local, continue to next round")
			return false, nil
		})
		exutil.AssertWaitPollNoErr(pollErr, "Failed to get bc with ruby-test12766-local")

	})

	//author: yyou@redhat.com
	g.It("NonHyperShiftHOST-Author:yyou-Medium-50925-Add prometheusrules for image_registry_image_stream_tags_total and registry operations metrics", func() {
		var (
			operationData   prometheusImageregistryOperations
			storageTypeData prometheusImageregistryStorageType
		)
		g.By("Check no PrometheusRule/image-registry-operator-alerts in registry project")
		out, outErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("prometheusrules", "-n", "openshift-image-registry").Output()
		o.Expect(outErr).NotTo(o.HaveOccurred())
		o.Expect(out).NotTo(o.ContainSubstring("image-registry-operator-alerts"))

		g.By("Push 1 images to non-openshift project to image registry")
		oc.SetupProject()
		checkRegistryFunctionFine(oc, "test-50925", oc.Namespace())

		g.By("Collect metrics of tag")
		mo, err := exutil.NewPrometheusMonitor(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = wait.Poll(20*time.Second, 3*time.Minute, func() (bool, error) {
			tagQueryParams := exutil.MonitorInstantQueryParams{Query: "imageregistry:imagestreamtags_count:sum"}
			tagMsg, err := mo.InstantQuery(tagQueryParams)
			if err != nil {
				return false, err
			}
			if tagMsg == "" {
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "The operation metric don't get expect tag")

		g.By("Collect metrics of operations")
		opQueryParams := exutil.MonitorInstantQueryParams{Query: "imageregistry:operations_count:sum"}
		operationMsg, err := mo.InstantQuery(opQueryParams)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(operationMsg).NotTo(o.BeEmpty())
		jsonerr := json.Unmarshal([]byte(operationMsg), &storageTypeData)
		if jsonerr != nil {
			e2e.Failf("operation data is not in json format")
		}
		operationLen := len(operationData.Data.Result)
		beforeOperationData := make([]int, operationLen)
		for i := 0; i < operationLen; i++ {
			beforeOperationData[i], err = strconv.Atoi(operationData.Data.Result[i].Value[1].(string))
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("the operation array %v is %v", i, beforeOperationData[i])
		}

		g.By("Tag 2 imagestream to non-openshift project")
		err = oc.AsAdmin().Run("tag").Args("quay.io/openshifttest/busybox@sha256:c5439d7db88ab5423999530349d327b04279ad3161d7596d2126dfb5b02bfd1f", "is50925-1:latest", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, oc.Namespace(), "is50925-1", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.Run("import-image").Args("is50925-2:latest", "--from", "quay.io/openshifttest/ruby-27@sha256:8f71dd40e3f55d90662a63cb9f02b59e75ed7ac1e911c7919fd14fbfad431348", "--confirm", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, oc.Namespace(), "is50925-2", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Collect metrics of storagetype")
		fullStorageType := "S3 EmptyDir PVC Azure GCS Swift OSS IBMCOS"
		storageQuertParams := exutil.MonitorInstantQueryParams{Query: "image_registry_storage_type"}
		storageTypeMsg, err := mo.InstantQuery(storageQuertParams)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(storageTypeMsg).NotTo(o.BeEmpty())
		jsonerr = json.Unmarshal([]byte(storageTypeMsg), &storageTypeData)
		if jsonerr != nil {
			e2e.Failf("storage type data is not in json format")
		}
		storageType := storageTypeData.Data.Result[0].Metric.Storage
		o.Expect(fullStorageType).To(o.ContainSubstring(storageType))

		g.By("Collect metrics of operation again")
		err = wait.Poll(20*time.Second, 3*time.Minute, func() (bool, error) {
			opQueryParams = exutil.MonitorInstantQueryParams{Query: "imageregistry:operations_count:sum"}
			operationMsg, err = mo.InstantQuery(opQueryParams)
			o.Expect(err).NotTo(o.HaveOccurred())
			jsonerr = json.Unmarshal([]byte(operationMsg), &storageTypeData)
			if jsonerr != nil {
				e2e.Failf("operation data is not in json format")
			}
			afterOperationData := make([]int, operationLen)
			for i := 0; i < operationLen; i++ {
				afterOperationData[i], err = strconv.Atoi(operationData.Data.Result[i].Value[1].(string))
				o.Expect(err).NotTo(o.HaveOccurred())
				e2e.Logf("the operation array %v is %v", i, beforeOperationData[i])
				if afterOperationData[i] >= beforeOperationData[i] {
					e2e.Logf("%v -> %v", beforeOperationData[i], afterOperationData[i])
				} else {
					return false, nil
				}
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "The operation metric don't get expect value")
	})

	//author: xiuwang@redhat.com
	g.It("ROSA-OSD_CCS-ARO-ConnectedOnly-VMonly-Author:xiuwang-Critical-10904-Support unauthenticated with registry-admin role", func() {
		// Skip Hypershift external OIDC clusters against which all test cases run as the same (external) user
		isExternalOIDCCluster, err := exutil.IsExternalOIDCCluster(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		if isExternalOIDCCluster {
			g.Skip("Skipping the test as we are running against a Hypershift external OIDC cluster")
		}
		g.By("Add registry-admin role to a project")
		oc.SetupProject()
		defer oc.AsAdmin().WithoutNamespace().Run("policy").Args("remove-role-from-user", "registry-admin", "-z", "test-registry-admin", "-n", oc.Namespace()).Execute()
		err = oc.AsAdmin().Run("create").Args("sa", "test-registry-admin", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("policy").Args("add-role-to-user", "registry-admin", "-z", "test-registry-admin", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Get external registry host")
		routeName := getRandomString()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("route", routeName, "-n", "openshift-image-registry").Execute()
		regRoute := exposeRouteFromSVC(oc, "reencrypt", "openshift-image-registry", routeName, "image-registry")
		waitRouteReady(regRoute)

		g.By("Push a image to the project")
		checkRegistryFunctionFine(oc, "test-10904", oc.Namespace())
		err = waitForAnImageStreamTag(oc, oc.Namespace(), "test-10904", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())
		publicImageName := regRoute + "/" + oc.Namespace() + "/test-10904:latest"

		g.By("Save the external registry auth with the specific token")
		authFile, err := saveImageRegistryAuth(oc, "test-registry-admin", regRoute, oc.Namespace())
		defer os.RemoveAll(authFile)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Pull the image with registry-admin role")
		containerCLI := container.NewPodmanCLI()
		_, err = containerCLI.Run("pull").Args(publicImageName, "--authfile="+authFile, "--tls-verify=false").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer containerCLI.RemoveImage(publicImageName)

		g.By("Pag the image with another name")
		newImage := regRoute + "/" + oc.Namespace() + "/myimage:latest"
		_, err = containerCLI.Run("tag").Args(publicImageName, newImage).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Push image with registry-admin role")
		_, err = containerCLI.Run("push").Args(newImage, "--authfile="+authFile, "--tls-verify=false").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, oc.Namespace(), "myimage", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	//author: yyou@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:yyou-Critical-24160-lookupPolicy can be set by oc set image-lookup", func() {

		g.By("Import an image stream")
		operationErr := oc.WithoutNamespace().AsAdmin().Run("import-image").Args("is24160-1-lookup", "--from=quay.io/openshifttest/base-alpine@sha256:3126e4eed4a3ebd8bf972b2453fa838200988ee07c01b2251e3ea47e4b1f245c", "--confirm=true", "-n", oc.Namespace()).Execute()
		o.Expect(operationErr).NotTo(o.HaveOccurred())
		tagErr := waitForAnImageStreamTag(oc, oc.Namespace(), "is24160-1-lookup", "latest")
		o.Expect(tagErr).NotTo(o.HaveOccurred())

		g.By("Create pod from the imagestream without full repository")
		expectInfo := `Failed to pull image "is24160-1-lookup"`
		createSimpleRunPod(oc, "is24160-1-lookup", expectInfo)

		g.By("Set image-lookup and check lookupPolicy is updated ")
		lookupErr := oc.WithoutNamespace().AsAdmin().Run("set").Args("image-lookup", "is24160-1-lookup", "-n", oc.Namespace()).Execute()
		o.Expect(lookupErr).NotTo(o.HaveOccurred())
		output, updateErr := oc.WithoutNamespace().AsAdmin().Run("get").Args("imagestreams", "is24160-1-lookup", "-n", oc.Namespace(), "-o=jsonpath={.spec.lookupPolicy.local}").Output()
		o.Expect(updateErr).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("true"))

		g.By("Create pod again from the imagestream without full repository")
		expectInfo = `Successfully pulled image "quay.io/openshifttest/base-alpine@sha256:`
		createSimpleRunPod(oc, "is24160-1-lookup", expectInfo)

		g.By("Create an imagestream with pullthrough and set image-lookup")
		operationErr = oc.WithoutNamespace().AsAdmin().Run("import-image").Args("is24160-2-lookup", "--from=quay.io/openshifttest/base-alpine@sha256:3126e4eed4a3ebd8bf972b2453fa838200988ee07c01b2251e3ea47e4b1f245c", "--confirm=true", "--reference-policy=local", "-n", oc.Namespace()).Execute()
		o.Expect(operationErr).NotTo(o.HaveOccurred())
		lookupErr = oc.WithoutNamespace().AsAdmin().Run("set").Args("image-lookup", "is24160-2-lookup", "-n", oc.Namespace()).Execute()
		o.Expect(lookupErr).NotTo(o.HaveOccurred())

		g.By("Create pod and check pod could pull image")
		expectInfo = `Successfully pulled image "image-registry.openshift-image-registry.svc:5000/`
		createSimpleRunPod(oc, "is24160-2-lookup", expectInfo)

		g.By("Create another imagestream and create a deploy")
		operationErr = oc.WithoutNamespace().AsAdmin().Run("import-image").Args("is24160-3-lookup", "--from=quay.io/openshifttest/base-alpine@sha256:3126e4eed4a3ebd8bf972b2453fa838200988ee07c01b2251e3ea47e4b1f245c", "--confirm=true", "-n", oc.Namespace()).Execute()
		o.Expect(operationErr).NotTo(o.HaveOccurred())
		deployErr := oc.AsAdmin().WithoutNamespace().Run("create").Args("deploy", "deploy-lookup", "--image=is24160-3-lookup", "--port=5000", "-n", oc.Namespace()).Execute()
		o.Expect(deployErr).NotTo(o.HaveOccurred())

		g.By("Check whether the image can be pulled")
		expectInfo = `Failed to pull image "is24160-3-lookup"`
		pollErr := wait.Poll(15*time.Second, 200*time.Second, func() (bool, error) {
			output, describeErr := oc.AsAdmin().WithoutNamespace().Run("describe").Args("pod", "-l", "app=deploy-lookup", "-n", oc.Namespace()).Output()
			o.Expect(describeErr).NotTo(o.HaveOccurred())
			if strings.Contains(output, expectInfo) {
				return true, nil
			}
			e2e.Logf("Continue to next round")
			return false, nil
		})
		exutil.AssertWaitPollNoErr(pollErr, fmt.Sprintf("Pod doesn't show expected log %v", expectInfo))

		g.By("Set image-lookup for deploy")
		lookupErr = oc.WithoutNamespace().AsAdmin().Run("set").Args("image-lookup", "deploy/deploy-lookup", "-n", oc.Namespace()).Execute()
		o.Expect(lookupErr).NotTo(o.HaveOccurred())

		g.By("Check again lookupPolicy is updated")
		output, updateErr = oc.WithoutNamespace().AsAdmin().Run("get").Args("deploy", "deploy-lookup", "-n", oc.Namespace(), "-o=jsonpath={..annotations}").Output()
		o.Expect(updateErr).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring(`"alpha.image.policy.openshift.io/resolve-names":"*"`))

		g.By("Check whether the image can be pulled again")
		expectInfo = `Successfully pulled image`
		pollErr = wait.Poll(15*time.Second, 4*time.Minute, func() (bool, error) {
			output, describeErr := oc.AsAdmin().WithoutNamespace().Run("describe").Args("pod", "-l", "app=deploy-lookup", "-n", oc.Namespace()).Output()
			o.Expect(describeErr).NotTo(o.HaveOccurred())
			if strings.Contains(output, expectInfo) {
				return true, nil
			}
			e2e.Logf("Continue to next round")
			return false, nil
		})
		exutil.AssertWaitPollNoErr(pollErr, fmt.Sprintf("Pod doesn't show expected log %v", expectInfo))
	})

	//author: xiuwang@redhat.com
	g.It("ROSA-OSD_CCS-ARO-ConnectedOnly-VMonly-Author:xiuwang-Low-11314-Support unauthenticated with registry-viewer role", func() {
		// Skip Hypershift external OIDC clusters against which all test cases run as the same (external) user
		isExternalOIDCCluster, err := exutil.IsExternalOIDCCluster(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		if isExternalOIDCCluster {
			g.Skip("Skipping the test as we are running against a Hypershift external OIDC cluster")
		}
		g.By("Add registry-viewer role to a project")
		oc.SetupProject()
		defer oc.AsAdmin().WithoutNamespace().Run("policy").Args("remove-role-from-user", "registry-viewer", "-z", "test-registry-viewer", "-n", oc.Namespace()).Execute()
		err = oc.AsAdmin().Run("create").Args("sa", "test-registry-viewer", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("policy").Args("add-role-to-user", "registry-viewer", "-z", "test-registry-viewer", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Get external registry host")
		routeName := getRandomString()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("route", routeName, "-n", "openshift-image-registry").Execute()
		regRoute := exposeRouteFromSVC(oc, "reencrypt", "openshift-image-registry", routeName, "image-registry")
		waitRouteReady(regRoute)

		g.By("Push a image to the project")
		checkRegistryFunctionFine(oc, "test-11314", oc.Namespace())
		err = waitForAnImageStreamTag(oc, oc.Namespace(), "test-11314", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())
		publicImageName := regRoute + "/" + oc.Namespace() + "/test-11314:latest"

		g.By("Save the external registry auth with the specific token")
		authFile, err := saveImageRegistryAuth(oc, "test-registry-viewer", regRoute, oc.Namespace())
		defer os.RemoveAll(authFile)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Pull the image with registry-viewer role")
		containerCLI := container.NewPodmanCLI()
		_, err = containerCLI.Run("pull").Args(publicImageName, "--authfile="+authFile, "--tls-verify=false").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer containerCLI.RemoveImage(publicImageName)

		g.By("Pag the image with another name")
		newImage := regRoute + "/" + oc.Namespace() + "/myimage:latest"
		_, err = containerCLI.Run("tag").Args(publicImageName, newImage).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Can't push image with registry-view role")
		output, err := containerCLI.Run("push").Args(newImage, "--authfile="+authFile, "--tls-verify=false").Output()
		if err == nil {
			e2e.Failf("Shouldn't push image with registry-viewer role")
		}
		o.Expect(output).To(o.ContainSubstring("authentication required"))

	})

	//author: xiuwang@redhat.com
	g.It("ROSA-OSD_CCS-ARO-ConnectedOnly-Author:xiuwang-Medium-29706-Node secret takes effect when common secret is removed", func() {
		g.By("Add the secret of registry.redhat.io to project")
		tempDataDir, err := extractPullSecret(oc)
		defer os.RemoveAll(tempDataDir)
		o.Expect(err).NotTo(o.HaveOccurred())
		oc.SetupProject()
		err = oc.WithoutNamespace().AsAdmin().Run("create").Args("secret", "generic", "pj-secret", "--from-file="+tempDataDir+"/.dockerconfigjson", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Could pull image with the secret under project")
		err = oc.AsAdmin().Run("tag").Args("registry.redhat.io/ubi8/httpd-24:latest", "httpd-29706:latest", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, oc.Namespace(), "httpd-29706", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Could pull image using node secret after project secret removed")
		err = oc.WithoutNamespace().AsAdmin().Run("delete").Args("secret", "pj-secret", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().Run("import-image").Args("mysql-29706:latest", "--from=registry.redhat.io/rhel8/mysql-80:latest", "--confirm", "--reference-policy=local", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, oc.Namespace(), "mysql-29706", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.Run("set").Args("image-lookup", "mysql-29706", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("run").Args("mysql", "--image=mysql-29706:latest", `--overrides={"spec":{"securityContext":{"runAsNonRoot":true,"seccompProfile":{"type":"RuntimeDefault"}}}}`, "--env=MYSQL_ROOT_PASSWORD=test", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		checkPodsRunningWithLabel(oc, oc.Namespace(), "run=mysql", 1)
	})

	//author: xiuwang@redhat.com
	g.It("Author:xiuwang-NonHyperShiftHOST-DisconnectedOnly-Critical-29693-Import image from a secure registry using node credentials", func() {
		g.By("Check if image-policy-aosqe created")
		policy, dis := checkDiscPolicy(oc)
		if dis {
			output, err := oc.WithoutNamespace().AsAdmin().Run("get").Args(policy).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if !strings.Contains(output, "image-policy-aosqe") {
				e2e.Failf("image-policy-aosqe is not created in this disconnect cluster")
			}
		}

		g.By("Could pull image from secure registry using node credentials")
		// Get mirror registry with 5000 port, registry.redhat.io/rhel8 images have been mirrored into it
		mirrorReg := checkMirrorRegistry(oc, "registry.redhat.io")
		o.Expect(mirrorReg).NotTo(o.BeEmpty())
		cmd := fmt.Sprintf(`echo '%v' | awk -F ':' '{print $1}'`, mirrorReg)
		mReg6001, err := exec.Command("bash", "-c", cmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		// 6001 mirror registry can't mirror images into, so make up the 5000 port
		mReg := strings.TrimSuffix(string(mReg6001), "\n") + ":5000"

		err = oc.AsAdmin().Run("import-image").Args("httpd-dis:latest", "--from="+mReg+"/rhel8/httpd-24:latest", "--import-mode=PreserveOriginal", "--confirm", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, oc.Namespace(), "httpd-dis", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.Run("set").Args("image-lookup", "httpd-dis", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		expectInfo := `Successfully pulled image "` + mReg
		createSimpleRunPod(oc, "httpd-dis", expectInfo)
	})

	//author: xiuwang@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:xiuwang-Critical-29696-Use node credentials in imagestream import", func() {
		g.By("Create image stream whose auth has added to node credentials")
		dockerImage, err := exutil.GetDockerImageReference(oc.ImageClient().ImageV1().ImageStreams("openshift"), "cli", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().Run("import-image").Args("cli-29696", "--from="+dockerImage, "--confirm", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, oc.Namespace(), "cli-29696", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Could pull image")
		err = oc.AsAdmin().WithoutNamespace().Run("new-app").Args("--name=cli-pod", "-i", "cli-29696:latest", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		checkPodsRunningWithLabel(oc, oc.Namespace(), "deployment=cli-pod", 1)
	})

	// author: jitli@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:jitli-Medium-11252-ImageRegistry Check the registry-admin permission", func() {

		// Skip Hypershift external OIDC clusters against which all test cases run as the same (external) user
		isExternalOIDCCluster, err := exutil.IsExternalOIDCCluster(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		if isExternalOIDCCluster {
			g.Skip("Skipping the test as we are running against a Hypershift external OIDC cluster")
		}
		g.By("Add registry-admin role to a project")
		defer oc.AsAdmin().WithoutNamespace().Run("policy").Args("remove-role-from-user", "registry-admin", oc.Username(), "-n", oc.Namespace()).Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("policy").Args("add-role-to-user", "registry-admin", oc.Username(), "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check the user policy")
		policyOutput, err := oc.Run("auth").Args("can-i", "create", "imagestreamimages", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(policyOutput).To(o.ContainSubstring("yes"))
		policyOutput, err = oc.Run("auth").Args("can-i", "create", "imagestreamimports", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(policyOutput).To(o.ContainSubstring("yes"))
		policyOutput, _ = oc.Run("auth").Args("can-i", "list", "imagestreamimports", "-n", oc.Namespace()).Output()
		o.Expect(policyOutput).To(o.ContainSubstring("no"))
		policyOutput, err = oc.Run("auth").Args("can-i", "get", "imagestreamtags", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(policyOutput).To(o.ContainSubstring("yes"))
		policyOutput, err = oc.Run("auth").Args("can-i", "update", "imagestreams/layers", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(policyOutput).To(o.ContainSubstring("yes"))

	})

	// author: yyou@redhat.com
	g.It("Author:yyou-High-23651-oc explain work for image-registry operator", func() {

		g.By("Use oc explain to explain configs.imageregistry.operator.openshift.io")
		result, explainErr := oc.WithoutNamespace().AsAdmin().Run("explain").Args("configs", "--api-version=imageregistry.operator.openshift.io/v1").Output()
		o.Expect(explainErr).NotTo(o.HaveOccurred())
		o.Expect(result).To(o.ContainSubstring("Config is the configuration object for a registry instance managed by the"))
		o.Expect(result).To(o.ContainSubstring("registry operator"))
		o.Expect(result).To(o.ContainSubstring("ImageRegistrySpec defines the specs for the running registry."))
		o.Expect(result).To(o.ContainSubstring("ImageRegistryStatus reports image registry operational status."))

	})

	// author: jitli@redhat.com
	g.It("ConnectedOnly-Author:jitli-Medium-22596-ImageRegistry Create app with template eap74-basic-s2i with jbosseap rhel7 image", func() {

		g.By("Check if it's a https_proxy cluster")
		trustCAName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("proxy/cluster", "-o=jsonpath={.spec.trustedCA.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		// because of known issue(https://issues.redhat.com/browse/OCPBUGS-9436) to download independency when build in https_proxy cluster, so skipped the case for https_proxy cluster
		if trustCAName != "" {
			g.Skip("Skip for https_proxy platform")
		}
		architecture.SkipNonAmd64SingleArch(oc)
		// Check if openshift-sample operator installed
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co/openshift-samples").Output()
		if err != nil && strings.Contains(output, `openshift-samples" not found`) {
			g.Skip("Skip test for openshift-samples which managed templates and imagestream are not installed")
		}
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("config.samples/cluster", "-o=jsonpath={.spec.managementState}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if output == "Removed" {
			g.Skip("Skip test for openshift-samples which is removed")
		}
		g.By("Create app with template")
		newappErr := oc.AsAdmin().WithoutNamespace().Run("new-app").Args("--template=eap74-basic-s2i", "-n", oc.Namespace()).Execute()
		o.Expect(newappErr).NotTo(o.HaveOccurred())

		g.By("Check eap-app-build-artifacts status")
		waitErr := exutil.WaitForABuild(oc.BuildClient().BuildV1().Builds(oc.Namespace()), "eap-app-build-artifacts-1", nil, nil, nil)
		if waitErr != nil {
			exutil.DumpBuildLogs("eap-app-build-artifacts", oc)
		}
		o.Expect(waitErr).NotTo(o.HaveOccurred())

		waitErr = exutil.WaitForABuild(oc.BuildClient().BuildV1().Builds(oc.Namespace()), "eap-app-2", nil, nil, nil)
		if waitErr != nil {
			exutil.DumpBuildLogs("eap-app", oc)
		}
		o.Expect(waitErr).NotTo(o.HaveOccurred())

	})

	// author: yyou@redhat.com
	g.It("NonHyperShiftHOST-Author:yyou-High-27562-Recreate Rollouts for Image Registry is enabled", func() {

		g.By("Check Image registry's deployment defaults value")
		output, operationErr := oc.WithoutNamespace().AsAdmin().Run("get").Args("configs.imageregistry.operator.openshift.io/cluster", "-o=jsonpath={.spec.rolloutStrategy}").Output()
		o.Expect(operationErr).NotTo(o.HaveOccurred())
		if !strings.Contains(output, "RollingUpdate") && !strings.Contains(output, "Recreate") {
			e2e.Failf("The  rolloutStrategy of image-registry is not correct")
		}

		g.By("Check Image registry's deployment invalid value")
		output, patchErr := oc.WithoutNamespace().AsAdmin().Run("patch").Args("configs.imageregistry.operator.openshift.io/cluster", "-p", `{"spec":{"rolloutStrategy":123}}`, "--type=merge").Output()
		o.Expect(patchErr).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring(`Invalid value: "integer": spec.rolloutStrategy in body must be of type string: "integer"`))

		g.By("Check Image registry's deployment invalid value again")
		output, patchErr = oc.WithoutNamespace().AsAdmin().Run("patch").Args("configs.imageregistry.operator.openshift.io/cluster", "-p", `{"spec":{"rolloutStrategy":"test"}}`, "--type=merge").Output()
		o.Expect(patchErr).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring(`Invalid value: "test": spec.rolloutStrategy in body should match '^(RollingUpdate|Recreate)$'`))
	})

	// author: jitli@redhat.com
	g.It("Author:jitli-Critical-24100-ImageRegistry Registry OpenStack Storage [Disruptive]", func() {

		storagetype, _ := getRegistryStorageConfig(oc)
		if storagetype != "swift" {
			g.Skip("Skip for non-supported platform")
		}
		podNum := getImageRegistryPodNumber(oc)

		g.By("Check the storage swift info")
		swiftPasswd, getErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment/image-registry", "-n", "openshift-image-registry", "-o=jsonpath={.spec.template.spec.containers[0].env[?(@.name==\"REGISTRY_STORAGE_SWIFT_PASSWORD\")].valueFrom}").Output()
		o.Expect(getErr).NotTo(o.HaveOccurred())
		o.Expect(swiftPasswd).NotTo(o.BeEmpty())
		swiftName, getErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment/image-registry", "-n", "openshift-image-registry", "-o=jsonpath={.spec.template.spec.containers[0].env[?(@.name==\"REGISTRY_STORAGE_SWIFT_USERNAME\")].valueFrom}").Output()
		o.Expect(getErr).NotTo(o.HaveOccurred())
		o.Expect(swiftName).NotTo(o.BeEmpty())

		status, getErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("configs.imageregistry/cluster", "-o=jsonpath={.status.conditions}").Output()
		o.Expect(getErr).NotTo(o.HaveOccurred())
		o.Expect(status).To(o.ContainSubstring("Swift container Exists"))

		oldContainer, getErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("configs.imageregistry/cluster", "-o=jsonpath={.spec.storage.swift.container}").Output()
		o.Expect(getErr).NotTo(o.HaveOccurred())

		g.By("Reset swift storage")
		patchErr := oc.AsAdmin().Run("patch").Args("config.image/cluster", "-p", `{"spec":{"storage":{"swift":{"container":null}}}}`, "--type=merge").Execute()
		o.Expect(patchErr).NotTo(o.HaveOccurred())

		g.By("Check the storage swift info after reset")
		checkPodsRunningWithLabel(oc, "openshift-image-registry", "docker-registry=default", podNum)
		newContainer, getErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("configs.imageregistry/cluster", "-o=jsonpath={.spec.storage.swift.container}").Output()
		o.Expect(getErr).NotTo(o.HaveOccurred())
		o.Expect(newContainer).NotTo(o.Equal(oldContainer))

	})

	// author: xiuwang@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:xiuwang-Low-18994-Copy internal image to another tag via 'oc image mirror'", func() {

		g.By("Get external registry host")
		routeName := getRandomString()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("route", routeName, "-n", "openshift-image-registry").Execute()
		regRoute := exposeRouteFromSVC(oc, "reencrypt", "openshift-image-registry", routeName, "image-registry")
		waitRouteReady(regRoute)

		g.By("Save the external registry auth with the specific token")
		authFile, err := saveImageRegistryAuth(oc, "builder", regRoute, oc.Namespace())
		defer os.RemoveAll(authFile)
		o.Expect(err).NotTo(o.HaveOccurred())

		err = waitForAnImageStreamTag(oc, "openshift", "cli", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Copy internal image to another tag")
		myimage := regRoute + "/" + oc.Namespace() + "/myimage:latest"
		mirrorErr := oc.AsAdmin().WithoutNamespace().Run("image").Args("mirror", regRoute+"/openshift/cli:latest", myimage, "--insecure", "-a", authFile).Execute()
		o.Expect(mirrorErr).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, oc.Namespace(), "myimage", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())

	})

	// author: xiuwang@redhat.com
	g.It("NonHyperShiftHOST-ROSA-OSD_CCS-ARO-Author:xiuwang-Medium-18998-Mirror multiple images to another registry", func() {

		g.By("Check the cluster using architecture")
		// https://issues.redhat.com/browse/IR-192
		// Since internal registry still not supports fat manifest, so skip test except x86_64
		masterNode, _ := exutil.GetFirstMasterNode(oc)
		output, archErr := exutil.DebugNodeWithChroot(oc, masterNode, "uname", "-m")
		o.Expect(archErr).NotTo(o.HaveOccurred())
		if !strings.Contains(output, "x86_64") {
			g.Skip("Skip test for non x86_64 arch image")
		}

		g.By("Get external registry host")
		routeName := getRandomString()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("route", routeName, "-n", "openshift-image-registry").Execute()
		regRoute := exposeRouteFromSVC(oc, "reencrypt", "openshift-image-registry", routeName, "image-registry")
		waitRouteReady(regRoute)

		g.By("Save the external registry auth with the specific token")
		authFile, err := saveImageRegistryAuth(oc, "builder", regRoute, oc.Namespace())
		defer os.RemoveAll(authFile)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Copy multiple images to internal registry")
		// Using x86_64 images
		firstImagePair := "quay.io/openshifttest/alpine@sha256:9c7da0f5d0f331d8f61d125c302a07d5eecfd89a346e2f5a119b8befc994d425=" + regRoute + "/" + oc.Namespace() + "/myimage1:latest"
		secondImagePair := "quay.io/openshifttest/busybox@sha256:0415f56ccc05526f2af5a7ae8654baec97d4a614f24736e8eef41a4591f08019=" + regRoute + "/" + oc.Namespace() + "/myimage2:latest"
		thirdImagePair := "quay.io/openshifttest/registry@sha256:f4cf1bfd98c39784777f614a5d8a7bd4f2e255e87d7a28a05ff7a3e452506fdb=" + regRoute + "/" + oc.Namespace() + "/myimage3:latest"
		mirrorErr := oc.AsAdmin().WithoutNamespace().Run("image").Args("mirror", firstImagePair, secondImagePair, thirdImagePair, "--insecure", "-a", authFile).Execute()
		o.Expect(mirrorErr).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, oc.Namespace(), "myimage1", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, oc.Namespace(), "myimage2", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, oc.Namespace(), "myimage3", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())

	})
	// author: yyou@redhat.com
	g.It("NonPreRelease-Longduration-Author:yyou-High-34991-Add logLevel to registry config object [Serial]", func() {
		g.By("Check image registry config")
		podNum := getImageRegistryPodNumber(oc)
		defer checkPodsRunningWithLabel(oc, "openshift-image-registry", "docker-registry=default", podNum)
		defer oc.AsAdmin().WithoutNamespace().Run("patch").Args("configs.imageregistry.operator.openshift.io/cluster", "-p", `{"spec":{"logLevel":"Normal"}}`, "--type=merge", "-n", "openshift-image-registry").Execute()
		logLevelOutput, levelErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("configs.imageregistry.operator.openshift.io/cluster", "-o=jsonpath={.spec.logLevel}{.spec.operatorLogLevel}").Output()
		o.Expect(levelErr).NotTo(o.HaveOccurred())
		o.Expect(logLevelOutput).To(o.ContainSubstring("NormalNormal"))

		g.By("Check image registry/operator pod log")
		envOutput, envErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("deploy/image-registry", "-o=jsonpath=`{.spec.template.spec.containers[0].env[?(@.name=='REGISTRY_LOG_LEVEL')].value}`", "-n", "openshift-image-registry").Output()
		o.Expect(envErr).NotTo(o.HaveOccurred())
		o.Expect(envOutput).To(o.ContainSubstring("info"))

		g.By("Change to Debug level")
		operationErr := oc.AsAdmin().WithoutNamespace().Run("patch").Args("configs.imageregistry.operator.openshift.io/cluster", "-p", `{"spec":{"logLevel":"Debug"}}`, "--type=merge", "-n", "openshift-image-registry").Execute()
		o.Expect(operationErr).NotTo(o.HaveOccurred())
		checkPodsRunningWithLabel(oc, "openshift-image-registry", "docker-registry=default", podNum)

		g.By("Check image registry/operator pod log again")
		envOutput, envErr = oc.AsAdmin().WithoutNamespace().Run("get").Args("deploy/image-registry", "-o=jsonpath=`{.spec.template.spec.containers[0].env[?(@.name=='REGISTRY_LOG_LEVEL')].value}`", "-n", "openshift-image-registry").Output()
		o.Expect(envErr).NotTo(o.HaveOccurred())
		o.Expect(envOutput).To(o.ContainSubstring("debug"))
		changeErr := oc.AsAdmin().WithoutNamespace().Run("patch").Args("configs.imageregistry.operator.openshift.io/cluster", "-p", `{"spec":{"logLevel":"Normal"}}`, "--type=merge", "-n", "openshift-image-registry").Execute()
		o.Expect(changeErr).NotTo(o.HaveOccurred())
		checkPodsRunningWithLabel(oc, "openshift-image-registry", "docker-registry=default", podNum)

		g.By("Change spec.loglevel to Trace and check the change takes effect")
		backOutput, backErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("deploy/image-registry", "-o=jsonpath=`{.spec.template.spec.containers[0].env[?(@.name=='REGISTRY_LOG_LEVEL')].value}`", "-n", "openshift-image-registry").Output()
		o.Expect(backErr).NotTo(o.HaveOccurred())
		o.Expect(backOutput).To(o.ContainSubstring("info"))
		traceErr := oc.AsAdmin().WithoutNamespace().Run("patch").Args("configs.imageregistry.operator.openshift.io/cluster", "-p", `{"spec":{"logLevel":"Trace"}}`, "--type=merge", "-n", "openshift-image-registry").Execute()
		o.Expect(traceErr).NotTo(o.HaveOccurred())
		checkPodsRunningWithLabel(oc, "openshift-image-registry", "docker-registry=default", podNum)
		envOutput, envErr = oc.AsAdmin().WithoutNamespace().Run("get").Args("deploy/image-registry", "-o=jsonpath=`{.spec.template.spec.containers[0].env[?(@.name=='REGISTRY_LOG_LEVEL')].value}`", "-n", "openshift-image-registry").Output()
		o.Expect(envErr).NotTo(o.HaveOccurred())
		o.Expect(envOutput).To(o.ContainSubstring("debug"))
		changeErr = oc.AsAdmin().WithoutNamespace().Run("patch").Args("configs.imageregistry.operator.openshift.io/cluster", "-p", `{"spec":{"logLevel":"Normal"}}`, "--type=merge", "-n", "openshift-image-registry").Execute()
		o.Expect(changeErr).NotTo(o.HaveOccurred())
		checkPodsRunningWithLabel(oc, "openshift-image-registry", "docker-registry=default", podNum)

		g.By("Change spec.loglevel to TraceAll and check the change takes effect")
		backOutput, backErr = oc.AsAdmin().WithoutNamespace().Run("get").Args("deploy/image-registry", "-o=jsonpath=`{.spec.template.spec.containers[0].env[?(@.name=='REGISTRY_LOG_LEVEL')].value}`", "-n", "openshift-image-registry").Output()
		o.Expect(backErr).NotTo(o.HaveOccurred())
		o.Expect(backOutput).To(o.ContainSubstring("info"))
		traceAllErr := oc.AsAdmin().WithoutNamespace().Run("patch").Args("configs.imageregistry.operator.openshift.io/cluster", "-p", `{"spec":{"logLevel":"TraceAll"}}`, "--type=merge", "-n", "openshift-image-registry").Execute()
		o.Expect(traceAllErr).NotTo(o.HaveOccurred())
		checkPodsRunningWithLabel(oc, "openshift-image-registry", "docker-registry=default", podNum)
		envOutput, envErr = oc.AsAdmin().WithoutNamespace().Run("get").Args("deploy/image-registry", "-o=jsonpath=`{.spec.template.spec.containers[0].env[?(@.name=='REGISTRY_LOG_LEVEL')].value}`", "-n", "openshift-image-registry").Output()
		o.Expect(envErr).NotTo(o.HaveOccurred())
		o.Expect(envOutput).To(o.ContainSubstring("debug"))

	})
	// author: yyou@redhat.com
	g.It("Author:yyou-Medium-23030-Enable must-gather object refs in image-registry cluster", func() {
		g.By("Check if cluster operator image-registry include related Objects")
		output, checkErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("co/image-registry", "-o=jsonpath=`{.status.relatedObjects}`").Output()
		o.Expect(checkErr).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring(`"name":"system:registry"`))
		o.Expect(output).To(o.ContainSubstring(`"resource":"clusterroles"`))
		o.Expect(output).To(o.ContainSubstring(`"name":"registry-registry-role"`))
		o.Expect(output).To(o.ContainSubstring(`"resource":"clusterrolebindings"`))

		g.By("Check to get these objects")
		clusterRoles, clusterRolesErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterroles", "system:registry").Output()
		o.Expect(clusterRolesErr).NotTo(o.HaveOccurred())
		o.Expect(clusterRoles).NotTo(o.BeEmpty())
		clusterRoleBinding, BindingErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterrolebindings", "registry-registry-role").Output()
		o.Expect(BindingErr).NotTo(o.HaveOccurred())
		o.Expect(clusterRoleBinding).NotTo(o.BeEmpty())
		serviceAccount, accountErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("serviceaccounts", "registry", "-n", "openshift-image-registry").Output()
		o.Expect(accountErr).NotTo(o.HaveOccurred())
		o.Expect(serviceAccount).NotTo(o.BeEmpty())
		configmap, mapErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("configmaps", "image-registry-certificates", "-n", "openshift-image-registry").Output()
		o.Expect(mapErr).NotTo(o.HaveOccurred())
		o.Expect(configmap).NotTo(o.BeEmpty())
		secret, secretErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("secrets", "image-registry-private-configuration", "-n", "openshift-image-registry").Output()
		o.Expect(secretErr).NotTo(o.HaveOccurred())
		o.Expect(secret).NotTo(o.BeEmpty())
		nodeCa, caErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("ds", "node-ca", "-n", "openshift-image-registry").Output()
		o.Expect(caErr).NotTo(o.HaveOccurred())
		o.Expect(nodeCa).NotTo(o.BeEmpty())
		service, serviceErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "image-registry", "-n", "openshift-image-registry").Output()
		o.Expect(serviceErr).NotTo(o.HaveOccurred())
		o.Expect(service).NotTo(o.BeEmpty())
		deployment, deploymentErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployments", "image-registry", "-n", "openshift-image-registry").Output()
		o.Expect(deploymentErr).NotTo(o.HaveOccurred())
		o.Expect(deployment).NotTo(o.BeEmpty())

	})

	// author: yyou@redhat.com
	g.It("NonPreRelease-Longduration-ConnectedOnly-Author:yyou-Medium-22230-Can set the Requests values in imageregistry config [Disruptive]", func() {
		// TODO: remove this skip when the builds v1 API will support producing manifest list images
		architecture.SkipArchitectures(oc, architecture.MULTI)
		podNum := getImageRegistryPodNumber(oc)
		defer func() {
			err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("configs.imageregistry.operator.openshift.io/cluster", "-p", `{"spec": {"requests": {"read": {"maxInQueue": null,"maxRunning": null,"maxWaitInQueue": "0s"},"write": {"maxInQueue": null,"maxRunning": null,"maxWaitInQueue": "0s"}}}}`, "--type=merge").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			checkPodsRunningWithLabel(oc, "openshift-image-registry", "docker-registry=default", podNum)
		}()

		g.By("Set requests value in config")
		requestsValueErr := oc.AsAdmin().WithoutNamespace().Run("patch").Args("configs.imageregistry.operator.openshift.io/cluster", "-p", `{"spec": {"requests": {"read": {"maxInQueue": 1,"maxRunning": 1,"maxWaitInQueue": "120s"},"write": {"maxInQueue": 1,"maxRunning": 1,"maxWaitInQueue": "120s"}}}}`, "--type=merge").Execute()
		o.Expect(requestsValueErr).NotTo(o.HaveOccurred())
		checkPodsRunningWithLabel(oc, "openshift-image-registry", "docker-registry=default", podNum)
		g.By("Create the app,and trigger the build")
		createAppError := oc.WithoutNamespace().AsAdmin().Run("new-app").Args("quay.io/openshifttest/ruby-27:1.2.0~https://github.com/sclorg/ruby-ex", "--import-mode=PreserveOriginal", "-n", oc.Namespace()).Execute()
		o.Expect(createAppError).NotTo(o.HaveOccurred())
		exutil.WaitForABuild(oc.BuildClient().BuildV1().Builds(oc.Namespace()), "ruby-ex-1", nil, exutil.CheckBuildFailed, nil)
		output, _ := oc.AsAdmin().WithoutNamespace().Run("describe").Args("build/ruby-ex-1", "-n", oc.Namespace()).Output()
		o.Expect(output).To(o.ContainSubstring("too many requests to registry"))
	})

	// author: xiuwang@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:xiuwang-High-12958-Read and write image signatures with registry endpoint", func() {
		// Skip Hypershift external OIDC clusters against which all test cases run as the same (external) user
		isExternalOIDCCluster, err := exutil.IsExternalOIDCCluster(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		if isExternalOIDCCluster {
			g.Skip("Skipping the test as we are running against a Hypershift external OIDC cluster")
		}
		g.By("Create signature file")
		var signFile = `'{"schemaVersion": 2,"type":"atomic","name":"digestid","content": "MjIK"}'`
		err = oc.AsAdmin().Run("tag").Args("quay.io/openshifttest/skopeo@sha256:d5f288968744a8880f983e49870c0bfcf808703fe126e4fb5fc393fb9e599f65", "ho12958:latest", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, oc.Namespace(), "ho12958", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())
		manifest := saveImageMetadataName(oc, "openshifttest/skopeo")
		o.Expect(manifest).NotTo(o.BeEmpty())
		signContent := strings.ReplaceAll(signFile, "digestid", manifest+"@imagesignature12958test")

		g.By("Get external registry host")
		routeName := getRandomString()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("route", routeName, "-n", "openshift-image-registry").Execute()
		regRoute := exposeRouteFromSVC(oc, "reencrypt", "openshift-image-registry", routeName, "image-registry")
		waitRouteReady(regRoute)

		g.By("Add signer role")
		defer oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "remove-cluster-role-from-user", "system:image-signer", "-z", "builder", "-n", oc.Namespace()).Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "add-cluster-role-to-user", "system:image-signer", "-z", "builder", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		token, err := getSAToken(oc, "builder", oc.Namespace())
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(token).NotTo(o.BeEmpty())

		g.By("Write signuture to image")
		pushURL := "curl -Lkv -u \"" + oc.Username() + ":" + token + "\" -H  \"Content-Type: application/json\" -XPUT --data " + signContent + " https://" + regRoute + "/extensions/v2/" + oc.Namespace() + "/ho12958/signatures/" + manifest
		var curlOutput []byte
		err = wait.Poll(20*time.Second, 2*time.Minute, func() (bool, error) {
			var errCmd error
			curlOutput, errCmd = exec.Command("bash", "-c", pushURL).Output()
			if errCmd != nil {
				e2e.Logf("The signature cmd executed failed %v", errCmd)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("The signature cmd executed failed. the output:\n %s", curlOutput))

		g.By("Read signuture to image")
		getURL := "curl -Lkv -u \"" + oc.Username() + ":" + token + "\" https://" + regRoute + "/extensions/v2/" + oc.Namespace() + "/ho12958/signatures/" + manifest
		curlOutput, err = exec.Command("bash", "-c", getURL).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(curlOutput).To(o.ContainSubstring("imagesignature12958test"))

	})

	// author: wewang@redhat.com
	g.It("Author:wewang-NonHyperShiftHOST-DisconnectedOnly-High-21988-Registry can use AdditionalTrustedCA to trust an external secured registry", func() {
		g.By("Check registry-config")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap/registry-config", "-n", "openshift-config").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("registry-config"))
		g.By("Import an image from mirror registry")
		getRegistry := checkMirrorRegistry(oc, "quay.io/openshifttest")
		o.Expect(getRegistry).NotTo(o.BeEmpty())
		getRegistry = strings.ReplaceAll(getRegistry, `["`, ``)
		getRegistry = strings.ReplaceAll(getRegistry, `"]`, ``)
		mirrorImage := "--from=" + getRegistry + "/busybox@sha256:c5439d7db88ab5423999530349d327b04279ad3161d7596d2126dfb5b02bfd1f"
		err = oc.WithoutNamespace().AsAdmin().Run("import-image").Args("myimage", mirrorImage, "--confirm", "--import-mode=PreserveOriginal", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, oc.Namespace(), "myimage", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	// author: xiuwang@redhat.com
	g.It("Author:xiuwang-ROSA-OSD_CCS-ARO-Medium-10788-Medium-12059-Could import image and pull from private registry", func() {
		g.By("Setup a private registry")
		var regUser, regPass = "testuser", getRandomString()
		authFile := filepath.Join("/tmp/", fmt.Sprintf("ir-auth-%s", getRandomString()))
		defer os.RemoveAll(authFile)
		htpasswdFile, err := generateHtpasswdFile("/tmp/", regUser, regPass)
		defer os.RemoveAll(htpasswdFile)
		o.Expect(err).NotTo(o.HaveOccurred())
		regRoute := setSecureRegistryEnableAuth(oc, oc.Namespace(), "myregistry", htpasswdFile, "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3")

		g.By("Push image to private registry")
		err = oc.NotShowInfo().AsAdmin().WithoutNamespace().Run("registry").Args("login", "--registry="+regRoute, "--auth-basic="+regUser+":"+regPass, "--to="+authFile, "--insecure", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		myimage := regRoute + "/" + oc.Namespace() + "/myimage:latest"
		err = oc.AsAdmin().WithoutNamespace().Run("image").Args("mirror", "quay.io/openshifttest/busybox@sha256:c5439d7db88ab5423999530349d327b04279ad3161d7596d2126dfb5b02bfd1f", myimage, "--insecure", "-a", authFile, "--keep-manifest-list=true", "--filter-by-os=.*").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create secret for private image under project")
		err = oc.WithoutNamespace().AsAdmin().Run("create").Args("secret", "docker-registry", "mysecret", "--docker-server="+regRoute, "--docker-username="+regUser, "--docker-password="+regPass, "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Make sure the image can be pulled after add auth")
		err = oc.AsAdmin().Run("tag").Args(myimage, "authis:latest", "--reference-policy=local", "--insecure", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, oc.Namespace(), "authis", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("set").Args("image-lookup", "authis", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		expectInfo := `Successfully pulled image "image-registry.openshift-image-registry.svc:5000/` + oc.Namespace()
		createSimpleRunPod(oc, "authis:latest", expectInfo)
	})

	// author: xiuwang@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:xiuwang-Medium-10909-Add/update/remove signatures to the images", func() {
		// Skip Hypershift external OIDC clusters against which all test cases run as the same (external) user
		isExternalOIDCCluster, err := exutil.IsExternalOIDCCluster(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		if isExternalOIDCCluster {
			g.Skip("Skipping the test as we are running against a Hypershift external OIDC cluster")
		}
		var (
			signFile = filepath.Join(imageRegistryBaseDir, "imagesignature.yaml")
			signsrc  = signatureSource{
				name:     "signature-10909",
				imageid:  "",
				title:    "test10909",
				content:  "GQ==",
				template: signFile,
			}
		)
		g.By("Create imagestreamimport")
		err = oc.AsAdmin().WithoutNamespace().Run("tag").Args("quay.io/openshifttest/skopeo@sha256:d5f288968744a8880f983e49870c0bfcf808703fe126e4fb5fc393fb9e599f65", "skopeo:latest", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, oc.Namespace(), "skopeo", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())
		manifest := saveImageMetadataName(oc, "openshifttest/skopeo")
		o.Expect(manifest).NotTo(o.BeEmpty())
		signsrc.imageid = manifest

		g.By("Add signer role")
		defer oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "remove-cluster-role-from-user", "system:image-signer", "-z", "builder", "-n", oc.Namespace()).Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "add-cluster-role-to-user", "system:image-signer", "-z", "builder", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Add signature and check")
		defer oc.WithoutNamespace().AsAdmin().Run("delete").Args("imagesignature", signsrc.imageid+"@test10909", signsrc.imageid+"@newsignature10909").Execute()
		signsrc.create(oc)
		output, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("images", manifest, "-o=jsonpath={.signatures}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("test10909"))

		g.By("Could create more than one ignature and delete")
		signsrc.title = "newsignature10909"
		signsrc.content = "Dw=="
		signsrc.create(oc)
		output, err = oc.WithoutNamespace().AsAdmin().Run("get").Args("images", manifest, "-o=jsonpath={.signatures}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("newsignature10909"))
		output, err = oc.WithoutNamespace().AsAdmin().Run("delete").Args("imagesignature", signsrc.imageid+"@test10909", signsrc.imageid+"@newsignature10909").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring(`10909" deleted`))
	})

	// author: wewang@redhat.com
	g.It("Author:wewang-VMonly-Medium-23063-Check the related log from must-gather tool", func() {
		g.By("Gather registry debugging information")
		errWait := wait.Poll(15*time.Second, 2*time.Minute, func() (bool, error) {
			defer exec.Command("bash", "-c", "rm -rf ./inspect.local*").Output()
			output, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("inspect", "co/image-registry").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(output, "Gathering data for ns/openshift-image-registry") && strings.Contains(output, "Wrote inspect data to inspect.local") {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(errWait, "no useful debugging info")
	})

	//author: xiuwang@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:xiuwang-Low-10585-Low-10637-Do not create tags for imageStream if image repository does not have tags", func() {
		var (
			isFile = filepath.Join(imageRegistryBaseDir, "imagestream-notag.yaml")
			issrc  = isSource{
				name:      "is10585",
				namespace: "",
				image:     "quay.io/openshifttest/busybox",
				template:  isFile,
			}
		)
		g.By("Create imagestreamimport")
		issrc.namespace = oc.Namespace()
		issrc.create(oc)
		output, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("imagestream", issrc.name, "-o=jsonpath={.spec}", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).NotTo(o.ContainSubstring("tag"))
		output, err = oc.WithoutNamespace().AsAdmin().Run("get").Args("imagestream", issrc.name, "-o=jsonpath={.status}", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).NotTo(o.ContainSubstring("tag"))

		g.By("Import-image to a non-exist imagestream")
		output, _ = oc.WithoutNamespace().AsAdmin().Run("import-image").Args("non-exist-is", "-n", oc.Namespace()).Output()
		o.Expect(output).To(o.ContainSubstring("pass --confirm to create and import"))
	})

	// author: xiuwang@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:xiuwang-Critical-10721-Could not import the tag when reference is true", func() {
		var (
			isFile = filepath.Join(imageRegistryBaseDir, "imagestream-reference-true.yaml")
			issrc  = isSource{
				name:      "is10721",
				namespace: "",
				tagname:   "referencetrue",
				image:     "quay.io/openshifttest/busybox@sha256:c5439d7db88ab5423999530349d327b04279ad3161d7596d2126dfb5b02bfd1f",
				template:  isFile,
			}
		)
		g.By("Create imagestreamimport")
		issrc.namespace = oc.Namespace()
		issrc.create(oc)
		output, _ := oc.WithoutNamespace().AsAdmin().Run("get").Args("imagestreamtag", issrc.name+":"+issrc.tagname, "-n", oc.Namespace()).Output()
		o.Expect(output).To(o.ContainSubstring(`is10721:referencetrue" not found`))
	})

	// author: wewang@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:wewang-High-18984-Request to view all imagestreams via registry catalog api [Serial]", func() {
		// Skip Hypershift external OIDC clusters against which all test cases run as the same (external) user
		isExternalOIDCCluster, err := exutil.IsExternalOIDCCluster(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		if isExternalOIDCCluster {
			g.Skip("Skipping the test as we are running against a Hypershift external OIDC cluster")
		}
		g.By("Tag an imagestream under project")
		err = oc.AsAdmin().WithoutNamespace().Run("tag").Args("quay.io/openshifttest/busybox@sha256:c5439d7db88ab5423999530349d327b04279ad3161d7596d2126dfb5b02bfd1f", "test18984:latest", "--reference-policy=local", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, oc.Namespace(), "test18984", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Curl the registry catalog api without user")
		routeName := getRandomString()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("route", routeName, "-n", "openshift-image-registry").Execute()
		regRoute := exposeRouteFromSVC(oc, "reencrypt", "openshift-image-registry", routeName, "image-registry")
		waitRouteReady(regRoute)
		getURL := "curl -kv " + "https://" + regRoute + "/v2/_catalog?n=5"
		curlOutput, err := exec.Command("bash", "-c", getURL).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(string(curlOutput)).To(o.ContainSubstring("authentication required"))

		g.By("Curl the registry catalog api with user")
		token, err := oc.Run("whoami").Args("-t").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		getURL = "curl -kv  -u " + oc.Username() + ":" + token + " https://" + regRoute + "/v2/_catalog?n=5"
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(string(curlOutput)).To(o.ContainSubstring("authentication required"))

		g.By("Curl the registry catalog api with permission")
		defer oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "remove-cluster-role-from-user", "registry-viewer", oc.Username()).Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "add-cluster-role-to-user", "registry-viewer", oc.Username()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		curlOutput, err = exec.Command("bash", "-c", getURL).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(string(curlOutput)).To(o.ContainSubstring("test18984"))
	})

	g.It("Author:xiuwang-Low-18559-Use SAR request to access registry metrics [Serial]", func() {
		g.By("Set an registry route")
		routeName := getRandomString()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("route", routeName, "-n", "openshift-image-registry").Execute()
		regRoute := exposeRouteFromSVC(oc, "reencrypt", "openshift-image-registry", routeName, "image-registry")
		waitRouteReady(regRoute)

		g.By("Create prometheus-scraper cluster role")
		template := filepath.Join(imageRegistryBaseDir, "prometheus-role-18559.yaml")
		defer oc.AsAdmin().Run("delete").Args("-f", template, "-n", oc.Namespace()).Execute()
		templateErr := oc.AsAdmin().Run("create").Args("-f", template, "-n", oc.Namespace()).Execute()
		o.Expect(templateErr).NotTo(o.HaveOccurred())
		token, err := oc.Run("whoami").Args("-t").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Without prometheus-scraper cluster role, can't get to registry metrics")
		getURL := "curl -vs -u \"" + oc.Username() + ":" + token + "\"  https://" + regRoute + "/extensions/v2/metrics -k"
		curlOutput, err := exec.Command("bash", "-c", getURL).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(string(curlOutput)).To(o.ContainSubstring("UNAUTHORIZED"))

		g.By("With prometheus-scraper cluster role, could get to registry metrics")
		defer oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "remove-cluster-role-from-user", "prometheus-scraper", oc.Username()).Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "add-cluster-role-to-user", "prometheus-scraper", oc.Username()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		curlOutput, err = exec.Command("bash", "-c", getURL).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(string(curlOutput)).NotTo(o.ContainSubstring("UNAUTHORIZED"))
		o.Expect(string(curlOutput)).To(o.ContainSubstring("registry"))
	})

	g.It("Author:xiuwang-Critical-24878-Mount trusted CA for cluster proxies to Image Registry Operator", func() {
		g.By("Check if it's a https_proxy cluster")
		trustCAName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("proxy/cluster", "-o=jsonpath={.spec.trustedCA.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if trustCAName == "" {
			g.Skip("Skip for non https_proxy platform")
		}

		g.By("Check the cluster trusted ca")
		trustCA, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("configmap", trustCAName, `-o=jsonpath={.data.ca-bundle\.crt}`, "-n", "openshift-config").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(trustCA).NotTo(o.BeEmpty())

		certs, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("configmap", "trusted-ca", `-o=jsonpath={.data.ca-bundle\.crt}`, "-n", "openshift-image-registry").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(certs, trustCA) {
			e2e.Failf("trusted-ca doesn't contain the content of proxy ca")
		}

		g.By("Check the ca mount into registry pods")
		trustCAPath, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("deploy/image-registry", `-o=jsonpath={..volumeMounts[?(@.name=="trusted-ca")].mountPath}`, "-n", "openshift-image-registry").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		podCerts, err := oc.AsAdmin().WithoutNamespace().Run("rsh").Args("-n", "openshift-image-registry", "deploy/image-registry", "cat", trustCAPath+"/anchors/ca-bundle.crt").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(podCerts, trustCA) {
			e2e.Failf("trusted-ca doesn't mount into registry pods")
		}
	})

	// author: wewang@redhat.com
	g.It("Author:wewang-Medium-24879-Mount trusted CA for cluster proxies to Image Registry Operator with invalid setting [Disruptive]", func() {
		g.By("Check if it's a https_proxy cluster")
		output, _ := oc.WithoutNamespace().AsAdmin().Run("get").Args("proxy/cluster", "-o=jsonpath={.spec}").Output()
		if !strings.Contains(output, "httpProxy") && !strings.Contains(output, "user-ca-bundle") {
			g.Skip("Skip for non https_proxy platform")
		}

		g.By("Check the cluster trusted ca")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("proxy.config.openshift.io/cluster", "-o=jsonpath={.spec.trustedCA.name}").Output()

		o.Expect(err).NotTo(o.HaveOccurred())
		if output == "" {
			g.Skip("Skip for http_proxy platform")
		}
		o.Expect(output).To(o.Equal("user-ca-bundle"))

		g.By("Import image to internal registry")
		err = oc.WithoutNamespace().AsAdmin().Run("import-image").Args("skopeo:latest", "--from=quay.io/openshifttest/skopeo@sha256:d5f288968744a8880f983e49870c0bfcf808703fe126e4fb5fc393fb9e599f65", "--reference-policy=local", "--confirm", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, oc.Namespace(), "skopeo", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Update ca for invalid value to proxy resource")
		defer oc.AsAdmin().Run("patch").Args("proxy.config.openshift.io/cluster", "-p", `{"spec":{"trustedCA":{"name":"user-ca-bundle"}}}`, "--type=merge").Execute()
		err = oc.AsAdmin().Run("patch").Args("proxy.config.openshift.io/cluster", "-p", `{"spec":{"trustedCA":{"name":"invalid"}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = wait.Poll(25*time.Second, 2*time.Minute, func() (bool, error) {
			result, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("deployments/machine-config-operator", "-n", "openshift-machine-config-operator").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(result, "configmap \"invalid\" not found") {
				return true, nil
			}
			e2e.Logf("Continue to next round")
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "Update ca for proxy resource failed")
	})

	// author: wewang@redhat.com
	g.It("NonHyperShiftHOST-DisconnectedOnly-Author:wewang-Medium-31767-Warning appears when registry use invalid AdditionalTrustedCA [Disruptive]", func() {
		trustCAName, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("image.config", "-o=jsonpath={..spec.additionalTrustedCA.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(trustCAName).NotTo(o.BeEmpty())

		g.By("Config AdditionalTrustedCA to use a non-existent ConfigMap")
		defer func() {
			var message string
			err := oc.AsAdmin().Run("patch").Args("image.config.openshift.io/cluster", "-p", `{"spec": {"additionalTrustedCA": {"name": "`+trustCAName+`"}}}`, "--type=merge").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			waitErr := wait.Poll(10*time.Second, 1*time.Minute, func() (bool, error) {
				registryDegrade := checkRegistryDegraded(oc)
				if !registryDegrade {
					return true, nil
				}
				message, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args("co/image-registry", "-o=jsonpath={.status.conditions[?(@.type==\"Available\")].message}").Output()
				e2e.Logf("Wait for image-registry coming ready")
				return false, nil
			})
			exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("Image registry is not ready with info %s\n", message))
		}()
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("image.config.openshift.io/cluster", "-p", `{"spec": {"additionalTrustedCA": {"name": "registry-config-invalid"}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = wait.Poll(25*time.Second, 2*time.Minute, func() (bool, error) {
			result, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("deployment.apps/cluster-image-registry-operator", "-n", "openshift-image-registry").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(result, "configmap \"registry-config-invalid\" not found") {
				return true, nil
			}
			e2e.Logf("Continue to next round")
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "Update cm for image.config cluster failed")
	})

	// author: wewang@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:wewang-Critical-30231-Warning appears if use invalid value for image registry virtual-hosted buckets", func() {
		g.By("Use invalid value for image registry virtual-hosted buckets")
		output, _ := oc.AsAdmin().Run("patch").Args("config.imageregistry/cluster", "-p", `{"spec": {"storage": {"s3": {"virtualHostedStyle": "invalid"}}}}`, "--type=merge").Output()
		o.Expect(output).To(o.ContainSubstring("Invalid value: \"string\""))
	})

	// author: xiuwang@redhat.com
	g.It("NonPreRelease-Longduration-Author:xiuwang-High-29435-Set the allowed list of image registry via allowedRegistriesForImport[Disruptive]", func() {
		g.By("Set allowed list for import")
		expectedStatus1 := map[string]string{"Progressing": "True"}
		expectedStatus2 := map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}
		defer func() {
			e2e.Logf("Restoring allowedRegistriesForImport setting")
			err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("image.config/cluster", "-p", `{"spec":{"allowedRegistriesForImport":[]}}`, "--type=merge").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Check the openshift-apiserver operator status")
			err = waitCoBecomes(oc, "openshift-apiserver", 60, expectedStatus1)
			o.Expect(err).NotTo(o.HaveOccurred())
			err = waitCoBecomes(oc, "openshift-apiserver", 480, expectedStatus2)
			o.Expect(err).NotTo(o.HaveOccurred())
			exutil.AssertWaitPollNoErr(err, "openshift-apiserver operator does not become available in 480 seconds")
		}()
		err := oc.WithoutNamespace().AsAdmin().Run("patch").Args("image.config/cluster", "-p", `{"spec":{"allowedRegistriesForImport":[{"domainName":"registry.redhat.io","insecure":true},{"domainName":"quay.io","insecure":false}]}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check the openshift-apiserver operator status")
		err = waitCoBecomes(oc, "openshift-apiserver", 60, expectedStatus1)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitCoBecomes(oc, "openshift-apiserver", 480, expectedStatus2)
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.AssertWaitPollNoErr(err, "openshift-apiserver operator does not become available in 480 seconds")

		g.By("Import the not allowed image")
		output, err := oc.WithoutNamespace().AsAdmin().Run("import-image").Args("notinallowed:latest", "--from=registry.access.redhat.com/ubi8/ubi", "--import-mode=PreserveOriginal", "--confirm=true", "-n", oc.Namespace()).Output()
		if err == nil {
			e2e.Failf("allowedRegistriesForImport does not work")
		}
		o.Expect(output).To(o.ContainSubstring("Forbidden: registry \"registry.access.redhat.com\" not allowed by whitelist"))

		g.By("Import the allowed image")
		err = oc.AsAdmin().WithoutNamespace().Run("tag").Args("quay.io/openshifttest/busybox@sha256:c5439d7db88ab5423999530349d327b04279ad3161d7596d2126dfb5b02bfd1f", "allowedquay:latest", "--import-mode=PreserveOriginal", "--reference-policy=local", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, oc.Namespace(), "allowedquay", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Import the allowed image with secure")
		output, err = oc.WithoutNamespace().AsAdmin().Run("import-image").Args("secure:latest", "--from=registry.redhat.io/ubi8/ubi", "--import-mode=PreserveOriginal", "--confirm=true", "-n", oc.Namespace()).Output()
		if err == nil {
			e2e.Failf("allowedRegistriesForImport does not work")
		}
		o.Expect(output).To(o.ContainSubstring("Forbidden: registry \"registry.redhat.io\" not allowed by whitelist"))
		o.Expect(output).To(o.ContainSubstring("registry.redhat.io:80"))
	})

	// author: wewang@redhat.com
	g.It("Author:wewang-Critical-22945-Autoconfigure registry storage [Disruptive]", func() {
		exutil.SkipIfPlatformTypeNot(oc, "AWS")
		g.By("Check image-registry-private-configuration secret if created")
		output, _ := oc.AsAdmin().Run("get").Args("secret/image-registry-private-configuration", "-n", "openshift-image-registry").Output()
		o.Expect(output).To(o.ContainSubstring("image-registry-private-configuration"))

		g.By("Check Add s3 bucket to be invalid")
		defer func() {
			err := oc.WithoutNamespace().AsAdmin().Run("patch").Args("config.image/cluster", "-p", `{"spec":{"storage":{"s3":{"bucket":""}}}}`, "--type=merge").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			waitErr := wait.Poll(10*time.Second, 2*time.Minute, func() (bool, error) {
				registryDegrade := checkRegistryDegraded(oc)
				if !registryDegrade {
					return true, nil
				}
				e2e.Logf("Wait for image-registry coming ready")
				return false, nil
			})
			exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("Image registry is not ready"))
		}()
		err := oc.WithoutNamespace().AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"storage":{"s3":{"bucket":"invalid"}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		Bucket, err := oc.AsAdmin().Run("get").Args("configs.imageregistry/cluster", "-o=jsonpath={.spec.storage.s3.bucket}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(Bucket).To(o.Equal("invalid"))
		err = wait.PollImmediate(10*time.Second, 2*time.Minute, func() (bool, error) {
			registryDegrade := checkRegistryDegraded(oc)
			if registryDegrade {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "Image registry is not degraded")
	})

	// author: wewang@redhat.com
	g.It("NonHyperShiftHOST-ConnectedOnly-Author:wewang-High-26732-Increase the limit on the number of image signatures [Disruptive]", func() {
		g.By("Scale down cvo to zero")
		defer func() {
			err := oc.AsAdmin().Run("scale").Args("deployment/cluster-version-operator", "--replicas=1", "-n", "openshift-cluster-version").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			checkPodsRunningWithLabel(oc, "openshift-cluster-version", "k8s-app=cluster-version-operator", 1)
		}()
		err := oc.AsAdmin().Run("scale").Args("deployment/cluster-version-operator", "--replicas=0", "-n", "openshift-cluster-version").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		checkPodsRemovedWithLabel(oc, "openshift-cluster-version", "k8s-app=cluster-version-operator")

		g.By("Scale down ocm operator to zero")
		defer func() {
			err := oc.AsAdmin().Run("scale").Args("deployment/openshift-controller-manager-operator", "--replicas=1", "-n", "openshift-controller-manager-operator").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			checkPodsRunningWithLabel(oc, "openshift-controller-manager-operator", "app=openshift-controller-manager-operator", 1)
		}()
		err = oc.AsAdmin().Run("scale").Args("deployment/openshift-controller-manager-operator", "--replicas=0", "-n", "openshift-controller-manager-operator").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		checkPodsRemovedWithLabel(oc, "openshift-controller-manager-operator", "app=openshift-controller-manager-operator")

		g.By("Create configmap under ocm project")
		cmFile := filepath.Join(imageRegistryBaseDir, "registry.access.redhat.com.yaml")
		beforeGeneration, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment/controller-manager", "-o=jsonpath={.metadata.generation}", "-n", "openshift-controller-manager").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		beforeNum, err := strconv.Atoi(beforeGeneration)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().Run("delete").Args("cm/sigstore-config", "-n", "openshift-controller-manager").Execute()
		err = oc.AsAdmin().Run("create").Args("cm", "sigstore-config", "--from-file="+cmFile, "-n", "openshift-controller-manager").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		output, err := oc.AsAdmin().Run("get").Args("cm", "sigstore-config", "-n", "openshift-controller-manager").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("sigstore-config"))

		g.By("Configure controller-manager to load this configmap")
		defer oc.AsAdmin().Run("set").Args("volume", "deployment/controller-manager", "--remove", "--name=sigstore-config", "-n", "openshift-controller-manager").Execute()
		err = oc.AsAdmin().Run("set").Args("volume", "deployment/controller-manager", "--add", "--type=configmap", "--configmap-name=sigstore-config", "-m", "/etc/containers/registries.d/", "--name=sigstore-config", "-n", "openshift-controller-manager").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		output, err = oc.AsAdmin().Run("get").Args("deployment/controller-manager", "-o=jsonpath={.spec.template.spec.containers[0].volumeMounts[4].name}", "-n", "openshift-controller-manager").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.Equal("sigstore-config"))

		g.By("Wait cm pods restart")
		podNum, _ := oc.AsAdmin().Run("get").Args("deployment/controller-manager", "-o=jsonpath={.spec.replicas}", "-n", "openshift-controller-manager").Output()
		podNumInt, _ := strconv.Atoi(podNum)
		err = wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
			afterGeneration, _ := oc.AsAdmin().Run("get").Args("deployment/controller-manager", "-o=jsonpath={.metadata.generation}", "-n", "openshift-controller-manager").Output()
			afterNum, err := strconv.Atoi(afterGeneration)
			o.Expect(err).NotTo(o.HaveOccurred())
			if (afterNum - beforeNum) >= 1 {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("pods are not restarted"))
		checkPodsRunningWithLabel(oc, "openshift-controller-manager", "controller-manager=true", podNumInt)

		g.By("Import image")
		err = oc.AsAdmin().WithoutNamespace().Run("import-image").Args("registry.access.redhat.com/openshift3/ose", "--confirm", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, oc.Namespace(), "ose", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())
		err = wait.Poll(25*time.Second, 4*time.Minute, func() (bool, error) {
			output, err = oc.AsAdmin().WithoutNamespace().Run("describe").Args("istag", "ose:latest", "-n", oc.Namespace()).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			sigCount := strings.Count(output, "Signatures")
			if sigCount >= 3 {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("The number of image signatures are not enough!"))
	})

	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-Author:xiuwang-High-21482-Medium-21926-Set default externalRegistryHostname in image policy config globally[Disruptive]", func() {
		// OCP-21926: Check function of oc registry info command
		g.By("Check options for oc registry info")
		output, err := oc.AsAdmin().Run("registry").Args("info", "--internal=true").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("image-registry.openshift-image-registry.svc:5000"))

		output, _ = oc.AsAdmin().Run("registry").Args("info", "--internal=true", "--quiet=true").Output()
		o.Expect(output).To(o.ContainSubstring("error: registry could not be contacted"))

		output, _ = oc.AsAdmin().Run("registry").Args("info", "--internal=21926").Output()
		o.Expect(output).To(o.ContainSubstring("invalid"))

		output, _ = oc.AsAdmin().Run("registry").Args("info", "--quiet=21926").Output()
		o.Expect(output).To(o.ContainSubstring("invalid"))

		g.By("Set registry default route")
		// Enable/disable default route will update image.config CRD,
		// that will cause openshift-apiserver and kube-apiserver restart
		expectedStatus1 := map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}
		defer func() {
			g.By("Recover image registry change")
			restoreRouteExposeRegistry(oc)
			err := waitCoBecomes(oc, "image-registry", 60, expectedStatus1)
			o.Expect(err).NotTo(o.HaveOccurred())
			err = waitCoBecomes(oc, "openshift-apiserver", 480, expectedStatus1)
			o.Expect(err).NotTo(o.HaveOccurred())
			err = waitCoBecomes(oc, "kube-apiserver", 600, expectedStatus1)
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		createRouteExposeRegistry(oc)
		regRoute := getRegistryDefaultRoute(oc)
		waitRouteReady(regRoute)

		g.By("Check options --public")
		err = wait.Poll(25*time.Second, 300*time.Second, func() (bool, error) {
			publicT, _ := oc.AsAdmin().Run("registry").Args("info", "--public=true").Output()
			publicF, _ := oc.AsAdmin().Run("registry").Args("info", "--public=false").Output()
			e2e.Logf("print %s, %s", publicT, publicF)
			if strings.Contains(publicT, "default-route-openshift-image-registry") && strings.Contains(publicF, "default-route-openshift-image-registry") {
				return true, nil
			}
			e2e.Logf("Not update, Continue to next round")
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "registry configs are not changed")

		g.By("Save the external registry auth with the specific token")
		authFile, err := saveImageRegistryAuth(oc, "builder", regRoute, oc.Namespace())
		defer os.RemoveAll(authFile)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("tag").Args("quay.io/openshifttest/base-alpine@sha256:3126e4eed4a3ebd8bf972b2453fa838200988ee07c01b2251e3ea47e4b1f245c", "test21482:latest", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, oc.Namespace(), "test21482", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Copy internal image to another tag")
		myimage1 := regRoute + "/" + oc.Namespace() + "/test21482:latest"
		myimage2 := regRoute + "/" + oc.Namespace() + "/myimage:latest"
		mirrorErr := oc.AsAdmin().WithoutNamespace().Run("image").Args("mirror", myimage1, myimage2, "--insecure", "-a", authFile).Execute()
		o.Expect(mirrorErr).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, oc.Namespace(), "myimage", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	g.It("ROSA-OSD_CCS-ARO-Author:xiuwang-High-24167-Should display information about images via oc image info", func() {
		g.By("Get external registry host")
		routeName := getRandomString()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("route", routeName, "-n", "openshift-image-registry").Execute()
		regRoute := exposeRouteFromSVC(oc, "reencrypt", "openshift-image-registry", routeName, "image-registry")
		waitRouteReady(regRoute)

		g.By("Save the external registry auth with the specific token")
		authFile, err := saveImageRegistryAuth(oc, "default", regRoute, oc.Namespace())
		defer os.RemoveAll(authFile)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check a internal image info with --insecure  and --registry-config option")
		err = oc.AsAdmin().WithoutNamespace().Run("tag").Args("quay.io/openshifttest/hello-openshift@sha256:4200f438cf2e9446f6bcff9d67ceea1f69ed07a2f83363b7fb52529f7ddd8a83", "test24167:latest", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, oc.Namespace(), "test24167", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())
		info, err := oc.AsAdmin().WithoutNamespace().Run("image").Args("info", regRoute+"/"+oc.Namespace()+"/test24167:latest", "--insecure", "-a", authFile).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(info).To(o.ContainSubstring("application/vnd.docker.distribution.manifest.v2+json"))
		o.Expect(info).To(o.ContainSubstring("OS:"))
		o.Expect(info).To(o.ContainSubstring("Image Size"))
		o.Expect(info).To(o.ContainSubstring("Layers"))

		g.By("Check a manifest list image without option")
		info, err = oc.AsAdmin().WithoutNamespace().Run("image").Args("info", "quay.io/openshifttest/base-alpine@sha256:3126e4eed4a3ebd8bf972b2453fa838200988ee07c01b2251e3ea47e4b1f245c").Output()
		if err == nil || !strings.Contains(info, "use --filter-by-os to select") {
			e2e.Failf("Don't get expect info with manifest list image")
		}

		g.By("Check a manifest list image with --filter-by-os option for specific arch")
		info, err = oc.AsAdmin().WithoutNamespace().Run("image").Args("info", "quay.io/openshifttest/base-alpine@sha256:3126e4eed4a3ebd8bf972b2453fa838200988ee07c01b2251e3ea47e4b1f245c", "--filter-by-os=linux/amd64").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(info).To(o.ContainSubstring("Arch:          amd64"))

		g.By("Check a manifest list image with --show-multiarch option")
		info, err = oc.AsAdmin().WithoutNamespace().Run("image").Args("info", "quay.io/openshifttest/base-alpine@sha256:3126e4eed4a3ebd8bf972b2453fa838200988ee07c01b2251e3ea47e4b1f245c", "--show-multiarch").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(info, "amd64")).To(o.BeTrue())
		o.Expect(strings.Contains(info, "arm64")).To(o.BeTrue())
		o.Expect(strings.Contains(info, "ppc64le")).To(o.BeTrue())
		o.Expect(strings.Contains(info, "s390x")).To(o.BeTrue())

	})

	g.It("ROSA-OSD_CCS-ARO-Author:xiuwang-High-55007-ImageStreamChange triggers using annotations should work on statefulset", func() {
		var (
			statefulsetFile = filepath.Join(imageRegistryBaseDir, "statefulset-trigger-annoation.yaml")
			statefulsetsrc  = staSource{
				namespace: "",
				name:      "example-statefulset",
				image:     "",
				template:  statefulsetFile,
			}
		)
		g.By("Import an imagestream")
		err := oc.AsAdmin().WithoutNamespace().Run("import-image").Args("test:v1", "--from=quay.io/openshifttest/hello-openshift@sha256:4200f438cf2e9446f6bcff9d67ceea1f69ed07a2f83363b7fb52529f7ddd8a83", "--import-mode=PreserveOriginal", "--confirm", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, oc.Namespace(), "test", "v1")
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Tag v1 to latest tag")
		err = oc.AsAdmin().WithoutNamespace().Run("tag").Args("test:v1", "st:latest", "--import-mode=PreserveOriginal", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check the imagestream imported image")
		imagev1, err := exutil.GetDockerImageReference(oc.ImageClient().ImageV1().ImageStreams(oc.Namespace()), "st", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())
		imagev1id := strings.Split(imagev1, "@")[1]

		g.By("Create the initial statefulset")
		statefulsetsrc.namespace = oc.Namespace()
		statefulsetsrc.image = "image-registry.openshift-image-registry.svc:5000/" + statefulsetsrc.namespace + "/st:latest"
		g.By("Create statefulset")
		statefulsetsrc.create(oc)
		g.By("Check the pods are running")
		checkPodsRunningWithLabel(oc, oc.Namespace(), "app=example-statefulset", 3)
		podImage, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-l", "app=example-statefulset", "-o=jsonpath={.items[0].spec.containers[].image}", "-n", statefulsetsrc.namespace).Output()
		o.Expect(strings.Contains(podImage, imagev1id)).To(o.BeTrue())

		g.By("Import second image to the imagestream")
		err = oc.AsAdmin().WithoutNamespace().Run("import-image").Args("test:v2", "--from=quay.io/openshifttest/hello-openshift@sha256:f79669a4290b8917fc6f93eb1d2508a9517f36d8887e38745250db2ef4b0bc40", "--import-mode=PreserveOriginal", "-n", statefulsetsrc.namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, statefulsetsrc.namespace, "test", "v2")
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("tag").Args("test:v2", "st:latest", "--import-mode=PreserveOriginal", "-n", statefulsetsrc.namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check the imagestream imported image")
		imagev2, err := exutil.GetDockerImageReference(oc.ImageClient().ImageV1().ImageStreams(statefulsetsrc.namespace), "st", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check the statefulset used image have been updated")
		err = wait.Poll(10*time.Second, 2*time.Minute, func() (bool, error) {
			podImage, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("statefulset", "-o=jsonpath={..containers[0].image}", "-n", statefulsetsrc.namespace).Output()
			if strings.Contains(podImage, imagev2) {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "The imagestreamchange trigger doesn't work, not update to use new image")
	})

	g.It("NonHyperShiftHOST-ROSA-OSD_CCS-ARO-Author:xiuwang-Critical-55008-ImageStreamChange triggers using annotations should work on daemonset", func() {
		var (
			dsFile = filepath.Join(imageRegistryBaseDir, "daemonset-trigger-annoation.yaml")
			dssrc  = dsSource{
				namespace: "",
				name:      "example-daemonset",
				image:     "",
				template:  dsFile,
			}
		)
		g.By("Import an imagestream")
		err := oc.AsAdmin().WithoutNamespace().Run("import-image").Args("test:v1", "--from=quay.io/openshifttest/hello-openshift@sha256:4200f438cf2e9446f6bcff9d67ceea1f69ed07a2f83363b7fb52529f7ddd8a83", "--confirm", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, oc.Namespace(), "test", "v1")
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Tag v1 to latest tag")
		err = oc.AsAdmin().WithoutNamespace().Run("tag").Args("test:v1", "ds:latest", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check the imagestream imported image")
		imagev1, err := exutil.GetDockerImageReference(oc.ImageClient().ImageV1().ImageStreams(oc.Namespace()), "ds", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())
		imagev1id := strings.Split(imagev1, "@")[1]

		g.By("Create the initial daemonset")
		dssrc.namespace = oc.Namespace()
		dssrc.image = "image-registry.openshift-image-registry.svc:5000/" + dssrc.namespace + "/ds:latest"
		g.By("Create daemonset")
		dssrc.create(oc)
		g.By("Check the pods are running")
		err = wait.Poll(10*time.Second, 2*time.Minute, func() (bool, error) {
			podImage, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-l", "app=example-daemonset", "-o=jsonpath={.items[0].spec.containers[].image}", "-n", dssrc.namespace).Output()
			if strings.Contains(podImage, imagev1id) {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "The image is not the expect one")

		g.By("Import second image to the imagestream")
		err = oc.AsAdmin().WithoutNamespace().Run("import-image").Args("test:v2", "--from=quay.io/openshifttest/hello-openshift@sha256:f79669a4290b8917fc6f93eb1d2508a9517f36d8887e38745250db2ef4b0bc40", "-n", dssrc.namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, dssrc.namespace, "test", "v2")
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("tag").Args("test:v2", "ds:latest", "-n", dssrc.namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check the imagestream imported image")
		imagev2, err := exutil.GetDockerImageReference(oc.ImageClient().ImageV1().ImageStreams(dssrc.namespace), "ds", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check the daemonset used image have been updated")
		err = wait.Poll(15*time.Second, 3*time.Minute, func() (bool, error) {
			podImage, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("daemonset", "-o=jsonpath={..containers[0].image}", "-n", dssrc.namespace).Output()
			if strings.Contains(podImage, imagev2) {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "The imagestreamchange trigger doesn't work, not update to use new image")
	})

	g.It("ROSA-OSD_CCS-ARO-Author:wewang-High-19633-Should create a status tag after editing an image stream tag to set 'reference: true' from an invalid tag", func() {
		g.By("Import image with an invalid tag")
		output, err := oc.WithoutNamespace().AsAdmin().Run("import-image").Args("jenkins:invalid", "--from", "registry.access.redhat.com/openshift3/jenkins-2-rhel7:invalid", "--confirm", "-n", oc.Namespace()).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Import failed"))
		err = oc.WithoutNamespace().AsAdmin().Run("patch").Args("imagestream/jenkins", "-p", `{"spec":{"tags":[{"from":{"kind": "DockerImage", "name": "registry.access.redhat.com/openshift3/jenkins-2-rhel7:invalid"},"name": "invalid","reference": true}]}}`, "--type=merge", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		isOut, err := oc.WithoutNamespace().AsAdmin().Run("describe").Args("imagestream/jenkins", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(isOut).To(o.ContainSubstring("reference to registry"))
		o.Expect(isOut).NotTo(o.ContainSubstring("Import failed"))
	})

	g.It("Author:xiuwang-Low-54172-importMode negative test", func() {
		g.By("Create ImageStreamImport with negative option")
		var (
			isImportFile = filepath.Join(imageRegistryBaseDir, "imagestream-import-oci.yaml")
			isimportsrc  = isImportSource{
				namespace: "",
				name:      "54172-negative",
				image:     "quay.io/openshifttest/ociimage@sha256:d58e3e003ddec723dd14f72164beaa609d24c5e5e366579e23bc8b34b9a58324",
				policy:    "Source",
				mode:      "",
				template:  isImportFile,
			}
		)
		// PreserveOriginal is one correct value, set to an invalid value
		isimportsrc.mode = "preserveoriginal"
		isimportsrc.namespace = oc.Namespace()
		parameters := []string{"-f", isImportFile, "-p", "NAME=" + isimportsrc.name, "NAMESPACE=" + isimportsrc.namespace, "IMAGE=" + isimportsrc.image, "MODE=" + isimportsrc.mode}
		configFile := parseToJSON(oc, parameters)
		output, err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", configFile).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("invalid"))
		o.Expect(output).To(o.ContainSubstring(`valid modes are '', 'Legacy', 'PreserveOriginal'`))

		o.Expect(err).To(o.HaveOccurred())
		// Set importMode to empty, the default importMode is Legacy
		isimportsrc.mode = ""
		isimportsrc.name = "empty"
		isimportsrc.create(oc)
		importOut, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("is/empty", "-o=jsonpath={.spec.tags[0].importPolicy.importMode}", "-n", isimportsrc.namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(importOut).To(o.ContainSubstring("Legacy"))
	})

	g.It("Author:wewang-Medium-57405-Add import-mode option to oc tag command", func() {
		g.By("Tag a specific image without import-mode")
		err := oc.AsAdmin().WithoutNamespace().Run("tag").Args("quay.io/openshifttest/ruby-27@sha256:8f71dd40e3f55d90662a63cb9f02b59e75ed7ac1e911c7919fd14fbfad431348", "ruby:latest", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, oc.Namespace(), "ruby", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Check the imagstream importMode")
		output, err := oc.AsAdmin().Run("get").Args("is/ruby", "-o=jsonpath={.spec.tags[0].importPolicy.importMode}", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).Should(o.Equal("Legacy"))
		g.By("Tag a specific image with import-mode=PreserveOriginal")
		err = oc.AsAdmin().WithoutNamespace().Run("tag").Args("quay.io/openshifttest/ruby-27@sha256:8f71dd40e3f55d90662a63cb9f02b59e75ed7ac1e911c7919fd14fbfad431348", "ruby:latest", "--import-mode=PreserveOriginal", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, oc.Namespace(), "ruby", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())
		output, err = oc.AsAdmin().Run("get").Args("is/ruby", "-o=jsonpath={.spec.tags[0].importPolicy.importMode}", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).Should(o.Equal("PreserveOriginal"))
		g.By("Tag a specific image with import-mode=Legacy")
		err = oc.AsAdmin().WithoutNamespace().Run("tag").Args("quay.io/openshifttest/ruby-27@sha256:8f71dd40e3f55d90662a63cb9f02b59e75ed7ac1e911c7919fd14fbfad431348", "ruby:latest", "--import-mode=Legacy", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, oc.Namespace(), "ruby", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())
		output, err = oc.AsAdmin().Run("get").Args("is/ruby", "-o=jsonpath={.spec.tags[0].importPolicy.importMode}", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).Should(o.Equal("Legacy"))
		g.By("Tag a specific image with invalide import-mode")
		output, _ = oc.AsAdmin().WithoutNamespace().Run("tag").Args("quay.io/openshifttest/ruby-27@sha256:8f71dd40e3f55d90662a63cb9f02b59e75ed7ac1e911c7919fd14fbfad431348", "ruby:latest", "--import-mode=invalid", "-n", oc.Namespace()).Output()
		o.Expect(string(output)).To(o.ContainSubstring("valid ImportMode values are Legacy or PreserveOriginal"))
	})

	g.It("Author:xiuwang-Low-59388-Image registry is re-deployed swift storage when reconnect to openstack [Disruptive]", func() {
		g.By("Skip test for non swift storage backend")
		storagetype, _ := getRegistryStorageConfig(oc)
		if storagetype != "swift" {
			g.Skip("Skip for non-supported platform")
		}

		g.By("Get clouds.yaml file")
		tempDataDir := filepath.Join("/tmp/", fmt.Sprintf("ir-%s", getRandomString()))
		err := os.Mkdir(tempDataDir, 0o755)
		if err != nil {
			e2e.Failf("Fail to create directory: %v", err)
		}
		err = oc.AsAdmin().Run("extract").Args("secret/installer-cloud-credentials", "-n", "openshift-image-registry", "--confirm", "--to="+tempDataDir).Execute()
		defer os.RemoveAll(tempDataDir)
		o.Expect(err).NotTo(o.HaveOccurred())
		originCloudCred := filepath.Join(tempDataDir, "clouds.yaml")
		if _, err := os.Stat(originCloudCred); os.IsNotExist(err) {
			e2e.Logf("clouds credential get failed")
		}
		invalidCloudCred := filepath.Join(tempDataDir, "c-invalid.yaml")
		copyFile(originCloudCred, invalidCloudCred)

		_, err = exec.Command("bash", "-c", "sed -i '/password/d' "+invalidCloudCred).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = exec.Command("bash", "-c", "cat "+invalidCloudCred).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Set invalid credentials for openshift swift backend")
		expectedStatus1 := map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}
		defer func() {
			g.By("Set correct cloud credentials back")
			err = oc.AsAdmin().WithoutNamespace().Run("set").Args("data", "secret/installer-cloud-credentials", "-n", "openshift-image-registry", "--from-file=clouds.yaml="+originCloudCred).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			err = waitCoBecomes(oc, "image-registry", 240, expectedStatus1)
			o.Expect(err).NotTo(o.HaveOccurred())
			container2, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("config.image/cluster", "-o=jsonpath={.spec.storage.swift.container}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(container2).NotTo(o.BeEmpty())
		}()
		err = oc.AsAdmin().WithoutNamespace().Run("set").Args("data", "secret/installer-cloud-credentials", "-n", "openshift-image-registry", "--from-file=clouds.yaml="+invalidCloudCred).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.WithoutNamespace().AsAdmin().Run("patch").Args("configs.imageregistry/cluster", `--patch=[ {"op": "remove", "path": "/spec/storage"} ]`, "--type=json").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Don't set storage to use pvc, re-connect to openstack")
		output, err := oc.WithoutNamespace().AsAdmin().Run("logs").Args("deploy/cluster-image-registry-operator", "--since=30s", "-n", "openshift-image-registry").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(output, "unable to sync storage configuration") ||
			!strings.Contains(output, "failed to authenticate against OpenStack") {
			e2e.Failf("unable to sync storage configuration or failed to authenticate against OpenStack is not in logs.")
		}
		container1, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("config.image/cluster", "-o=jsonpath={.status.storage.swift.container}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(container1).NotTo(o.BeEmpty())
	})

	g.It("Author:xiuwang-High-57230-Import manifest list image using oc import-image", func() {

		manifestList := "quay.io/openshifttest/hello-openshift@sha256:4200f438cf2e9446f6bcff9d67ceea1f69ed07a2f83363b7fb52529f7ddd8a83"
		g.By("Create ImageStreamImport with manifest list image with import-mode")
		output, err := oc.AsAdmin().WithoutNamespace().Run("import-image").Args("57230:latest", "--from="+manifestList, "--import-mode=PreserveOriginal", "--reference-policy=local", "--confirm", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(output, "Manifests") ||
			!strings.Contains(output, "prefer registry pullthrough when referencing this tag") {
			e2e.Failf("import-mode with pullthrough failed to import image")
		}

		g.By("Update imagestream to update periodically with import-mode")
		output, err = oc.AsAdmin().WithoutNamespace().Run("import-image").Args("57230:latest", "--from="+manifestList, "--import-mode=PreserveOriginal", "--scheduled=true", "--insecure=true", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(output, "Manifests") ||
			!strings.Contains(output, "updates automatically from registry") ||
			!strings.Contains(output, "will use insecure HTTPS or HTTP connections") {
			e2e.Failf("Could update imagestream periodically and insecured")
		}

		g.By("Update imagestream to Legacy import Mode")
		output, err = oc.AsAdmin().WithoutNamespace().Run("import-image").Args("57230:latest", "--from="+manifestList, "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(output, "Manifests") {
			e2e.Failf("Legacy import-mode is failed to import image")
		}

		g.By("Update imagestream back to PreserveOriginal import Mode")
		output, err = oc.AsAdmin().WithoutNamespace().Run("import-image").Args("57230:latest", "--from="+manifestList, "--import-mode=PreserveOriginal", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(output, "Manifests") {
			e2e.Failf("Can't update PreserveOriginal back")
		}

		g.By("Import image with 'Legacy' import Mode")
		output, err = oc.AsAdmin().WithoutNamespace().Run("import-image").Args("57230:single", "--from="+manifestList, "--import-mode=Legacy", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(output, "Manifests") {
			e2e.Failf("Legacy import-mode is failed to import image")
		}

		g.By("Import image with empty import Mode")
		output, err = oc.AsAdmin().WithoutNamespace().Run("import-image").Args("57230:empty", "--from="+manifestList, "--import-mode=", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(output, "Manifests") {
			e2e.Failf("Legacy import-mode is failed to import image")
		}

		g.By("Import image with invalid import Mode")
		output, err = oc.AsAdmin().WithoutNamespace().Run("import-image").Args("57230:invalid", "--from="+manifestList, "--import-mode=test", "-n", oc.Namespace()).Output()
		o.Expect(err).To(o.HaveOccurred())
		if !strings.Contains(output, "error: valid ImportMode values are Legacy or PreserveOriginal") {
			e2e.Failf("invalid importMode value shouldn't work")
		}
	})

	g.It("Author:wewang-High-59399-Add OS/arch information to image stream status", func() {
		g.By("Create ImageStreamImport with multiarch image")
		var (
			isImportFile = filepath.Join(imageRegistryBaseDir, "imagestream-import-oci.yaml")
			isimportsrc  = isImportSource{
				namespace: "",
				name:      "",
				image:     "",
				policy:    "Local",
				mode:      "",
				template:  isImportFile,
			}
		)
		isarr := [2]string{"ociapp", "dockerapp"}
		imagearr := [2]string{"quay.io/openshifttest/ociimage@sha256:d58e3e003ddec723dd14f72164beaa609d24c5e5e366579e23bc8b34b9a58324", "quay.io/openshifttest/busybox@sha256:c5439d7db88ab5423999530349d327b04279ad3161d7596d2126dfb5b02bfd1f"}
		archstr := [2]string{"amd64 arm arm64 ppc64le riscv64 s390x", "amd64 arm arm arm arm64 386 mips64le ppc64le s390x"}
		isimportsrc.namespace = oc.Namespace()
		isimportsrc.mode = "PreserveOriginal"

		g.By("Get OS/architectures from an image stream tag")
		for i := 0; i < 2; i++ {
			isimportsrc.name = isarr[i]
			isimportsrc.image = imagearr[i]
			isimportsrc.create(oc)
			output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("istag/"+isarr[i]+":latest", "-o=jsonpath={range .image.dockerImageManifests[*]}{.architecture}{\" \"}{end}", "-n", oc.Namespace()).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).To(o.Equal(archstr[i]))
		}
	})

	g.It("NonHyperShiftHOST-Author:xiuwang-ConnectedOnly-Critical-59415-Low-59418-Could push manifest list image to internal registry", func() {
		var (
			internalRegistry = "image-registry.openshift-image-registry.svc:5000"
			multiArchImage   = "quay.io/openshifttest/busybox@sha256:c5439d7db88ab5423999530349d327b04279ad3161d7596d2126dfb5b02bfd1f"
			targetImage      = internalRegistry + "/" + oc.Namespace() + "/push59415:latest"
			subMan           = internalRegistry + "/" + oc.Namespace() + "/push59415@sha256:0415f56ccc05526f2af5a7ae8654baec97d4a614f24736e8eef41a4591f08019"
			expectInfo1      = `Successfully pulled image "` + subMan
			expectInfo2      = `Successfully pulled image "` + internalRegistry + "/" + oc.Namespace() + "/push59415@sha256:c5439d7db88ab5423999530349d327b04279ad3161d7596d2126dfb5b02bfd1f"
		)
		// https://docs.openshift.com/container-platform/4.13/networking/enable-cluster-wide-proxy.html#prerequisites
		// System-wide proxy affects system components only, not user workloads
		// In proxy cluster, there is no proxy set inside pod, so can't access quay.io inside pod, this causes next steps failed in the case
		g.By("Skip test on proxy cluster")
		proxySet, _ := oc.WithoutNamespace().AsAdmin().Run("get").Args("proxy/cluster", "-o=jsonpath={.spec.httpProxy}").Output()
		if proxySet != " " {
			g.Skip("Skip for proxy platform")
		}

		g.By("Get token from secret")
		token, err := getSAToken(oc, "builder", oc.Namespace())
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(token).NotTo(o.BeEmpty())

		g.By("Without correct rights can't push/pull manifest list images to/from internal registry")
		masterNode, _ := exutil.GetFirstMasterNode(oc)
		output, _ := exutil.DebugNodeWithChroot(oc, masterNode, "skopeo", "copy", "docker://"+multiArchImage, "docker://"+targetImage, "--all")
		if !strings.Contains(output, "authentication required") {
			e2e.Failf("Could push manifest list image to internal registry wihtout right")
		}

		g.By("Push manifest list image to internal registry")
		output, err = exutil.DebugNodeWithChroot(oc, masterNode, "skopeo", "copy", "docker://"+multiArchImage, "docker://"+targetImage, "--all", "--dest-creds=test:"+token)
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(output, "Writing manifest list to image destination") || !strings.Contains(output, "Storing list signatures") {
			e2e.Failf("Not copy manifest list to internal registry")
		}

		g.By("Check imagestreamtag with manifest has been created")
		err = waitForAnImageStreamTag(oc, oc.Namespace(), "push59415", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())
		createSimpleRunPod(oc, subMan, expectInfo1)
		err = oc.Run("set").Args("image-lookup", "push59415", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		createSimpleRunPod(oc, "push59415", expectInfo2)

		g.By("Can't pull image from other project without rights")
		oc.SetupProject()
		createSimpleRunPod(oc, targetImage, "authentication required")
	})

	g.It("Author:xiuwang-High-56905-Medium-56925-Retrieve and describe image manifest list", func() {
		g.By("Create manifest imagestream")
		var object imageObject
		err := oc.AsAdmin().WithoutNamespace().Run("tag").Args("quay.io/openshifttest/ruby-27@sha256:8f71dd40e3f55d90662a63cb9f02b59e75ed7ac1e911c7919fd14fbfad431348", "ruby56905:latest", "--import-mode=PreserveOriginal", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, oc.Namespace(), "ruby56905", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Retrieve and describe image manifest list")
		outputMan, err := oc.AsAdmin().WithoutNamespace().Run("describe").Args("imagestreamimage", "ruby56905@sha256:8f71dd40e3f55d90662a63cb9f02b59e75ed7ac1e911c7919fd14fbfad431348", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		object.getManifestObject(oc, "imagestreamtag", "ruby56905:latest", oc.Namespace())
		if len(object.architecture) > 0 && len(object.digest) > 0 {
			for i, dg := range object.digest {
				outputSub, err := oc.AsAdmin().WithoutNamespace().Run("describe").Args("imagestreamimage", "ruby56905@"+dg, "-n", oc.Namespace()).Output()
				o.Expect(err).NotTo(o.HaveOccurred())
				if !strings.Contains(outputSub, object.architecture[i]) || !strings.Contains(outputMan, object.architecture[i]) {
					e2e.Failf("manifest object goes wrong")
				}
			}
		} else {
			e2e.Failf("Don't get the manifest object")
		}
		g.By("Negative test")
		g.By("Check an nonexist image object")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("imagestreamimage", "ruby56905@sha256:nonexist", "-n", oc.Namespace()).Output()
		o.Expect(err).To(o.HaveOccurred())
		if !strings.Contains(output, "Error from server (NotFound)") {
			e2e.Failf("Shouldn't get the non-exist image")
		}
		// Will add user right check when https://issues.redhat.com/browse/OCPQE-10622 finished
		g.By("Check an image object without right")
		oc.SetupProject()
		outputNoRight, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("imagestreamimage", "ruby56905@sha256:8f71dd40e3f55d90662a63cb9f02b59e75ed7ac1e911c7919fd14fbfad431348", "-n", oc.Namespace()).Output()
		o.Expect(err).To(o.HaveOccurred())
		if !strings.Contains(outputNoRight, `"ruby56905" not found`) {
			e2e.Failf("Shouldn't get image without rights")
		}
	})

	g.It("NonPreRelease-Longduration-ConnectedOnly-Author:wewang-Critical-61598-Uploading large layers should success when push large size image [Serial]", func() {
		g.By("Check image registry storage type")
		out, err := oc.AsAdmin().Run("get").Args("config.image/cluster", "-o=jsonpath={.spec.storage.s3}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(out, "bucket") {
			g.Skip("Skip testing on non s3 storage cluster")
		}

		g.By("Create a build with uploading large layers image")
		localImageName := "FROM registry.fedoraproject.org/fedora:37\nRUN dd if=/dev/urandom of=/bigfile bs=1M count=10240"
		defer func() {
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("bc/fedora", "-n", oc.Namespace()).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			out, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("bc/fedora", "-n", oc.Namespace()).Output()
			o.Expect(out).To(o.ContainSubstring("fedora\" not found"))

			g.By("Get server host")
			routeName := getRandomString()
			defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("route", routeName, "-n", "openshift-image-registry").Execute()
			refRoute := exposeRouteFromSVC(oc, "reencrypt", "openshift-image-registry", routeName, "image-registry")
			waitRouteReady(refRoute)

			g.By("Prune the images")
			defer oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "remove-cluster-role-from-user", "system:image-pruner", oc.Username()).Execute()
			err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "add-cluster-role-to-user", "system:image-pruner", oc.Username()).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			token, err := oc.Run("whoami").Args("-t").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			oc.WithoutNamespace().AsAdmin().Run("adm").Args("prune", "images", "--token="+token, "--keep-younger-than=0m", "--keep-tag-revisions=0", "--registry-url="+refRoute, "--confirm").Execute()

			g.By("Check new builded large image deleted")
			imageOut, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("images", "-n", oc.Namespace()).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			imageCount := strings.Count(imageOut, oc.Namespace()+"/fedora")
			o.Expect(imageCount).To(o.Equal(0))
		}()

		err = oc.AsAdmin().WithoutNamespace().Run("new-build").Args("-D", localImageName, "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = wait.Poll(15*time.Second, 1*time.Minute, func() (bool, error) {
			output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("build", "-n", oc.Namespace()).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(output, "Running") {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "Failed to create build")

		g.By("Check the logs")
		logInfo = "Successfully pushed image-registry.openshift-image-registry.svc:5000/" + oc.Namespace() + "/fedora"
		err = wait.Poll(5*time.Minute, 30*time.Minute, func() (bool, error) {
			output, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("bc/fedora", "-n", oc.Namespace()).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(output, logInfo) {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "Failed to push large image")
	})

	g.It("Author:xiuwang-ConnectedOnly-Critical-61973-High-61974-import-mode add to new-app/new-build to support manifest list", func() {
		cmds := [2]string{"new-app", "new-build"}
		for _, cmd := range cmds {
			g.By("With bad import mode")
			defer oc.WithoutNamespace().AsAdmin().Run("delete").Args("ns", "badm"+cmd, "--ignore-not-found").Execute()
			nsError := oc.WithoutNamespace().AsAdmin().Run("create").Args("ns", "badm"+cmd).Execute()
			o.Expect(nsError).NotTo(o.HaveOccurred())
			output, err := oc.AsAdmin().WithoutNamespace().Run(cmd).Args("--image=registry.redhat.io/ubi8/httpd-24:latest~https://github.com/openshift/httpd-ex.git", "--import-mode=DoesNotExist", "--name=isi-bad-mode", "-n", "badm"+cmd).Output()
			o.Expect(err).To(o.HaveOccurred())
			if !strings.Contains(output, "error: valid ImportMode values are Legacy or PreserveOriginal") {
				e2e.Failf("The import mode shouldn't be correct")
			}
			nsError = oc.WithoutNamespace().AsAdmin().Run("delete").Args("ns", "badm"+cmd).Execute()
			o.Expect(nsError).NotTo(o.HaveOccurred())

			g.By("PreserveOriginal mode with ManifestList image")
			defer oc.WithoutNamespace().AsAdmin().Run("delete").Args("ns", "pm"+cmd, "--ignore-not-found").Execute()
			nsError = oc.WithoutNamespace().AsAdmin().Run("create").Args("ns", "pm"+cmd).Execute()
			o.Expect(nsError).NotTo(o.HaveOccurred())
			output, err = oc.AsAdmin().WithoutNamespace().Run(cmd).Args("--image=registry.redhat.io/ubi8/httpd-24:latest~https://github.com/openshift/httpd-ex.git", "--import-mode=PreserveOriginal", "--name=isi-preserve-original-mode", "-n", "pm"+cmd).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if !strings.Contains(output, `httpd-24" created`) {
				e2e.Failf("The application with PreserveOriginal of ManifestList create failed with %s", cmd)
			}
			newCheck("expect", asAdmin, withoutNamespace, contain, "httpd-24:latest", ok, []string{"istag", "-n", "pm" + cmd}).check(oc)
			output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("istag", "httpd-24:latest", "-n", "pm"+cmd, "-o=jsonpath={..importMode} {..dockerImageManifests[*].architecture}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if !strings.Contains(output, "PreserveOriginal") || !strings.Contains(output, "amd64 arm64") {
				e2e.Failf("The imagestream with PreserveOriginal of ManifestList imported error with %s", cmd)
			}
			nsError = oc.WithoutNamespace().AsAdmin().Run("delete").Args("ns", "pm"+cmd).Execute()
			o.Expect(nsError).NotTo(o.HaveOccurred())

			g.By("Legacy mode with ManifestList image")
			defer oc.WithoutNamespace().AsAdmin().Run("delete").Args("ns", "lm"+cmd, "--ignore-not-found").Execute()
			nsError = oc.WithoutNamespace().AsAdmin().Run("create").Args("ns", "lm"+cmd).Execute()
			o.Expect(nsError).NotTo(o.HaveOccurred())
			output, err = oc.AsAdmin().WithoutNamespace().Run(cmd).Args("--image=registry.redhat.io/ubi8/httpd-24:latest~https://github.com/openshift/httpd-ex.git", "--import-mode=Legacy", "--name=isi-legacy", "-n", "lm"+cmd).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if !strings.Contains(output, `httpd-24" created`) {
				e2e.Failf("The application with Legacy of ManifestList create failed with %s", cmd)
			}
			newCheck("expect", asAdmin, withoutNamespace, contain, "httpd-24:latest", ok, []string{"istag", "-n", "lm" + cmd}).check(oc)
			output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("istag", "httpd-24:latest", "-n", "lm"+cmd, "-o=jsonpath={..importMode} {..dockerImageManifests[*].architecture}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if !strings.Contains(output, "Legacy") || strings.Contains(output, "amd64 arm64") {
				e2e.Failf("The imagestream with Legacy of ManifestList imported error with %s", cmd)
			}
			nsError = oc.WithoutNamespace().AsAdmin().Run("delete").Args("ns", "lm"+cmd).Execute()
			o.Expect(nsError).NotTo(o.HaveOccurred())

			g.By("PreserveOriginal Mode without ManifestList")
			defer oc.WithoutNamespace().AsAdmin().Run("delete").Args("ns", "pwom"+cmd, "--ignore-not-found").Execute()
			nsError = oc.WithoutNamespace().AsAdmin().Run("create").Args("ns", "pwom"+cmd).Execute()
			o.Expect(nsError).NotTo(o.HaveOccurred())
			output, err = oc.AsAdmin().WithoutNamespace().Run(cmd).Args("--image=quay.io/openshifttest/httpd-24@sha256:a4d1a67d994983b38fc462f1d54361632ccc9bf7f6e88bbaed9dea40d5568e80~https://github.com/openshift/httpd-ex.git", "--import-mode=PreserveOriginal", "--name=isi-preserve-mode-non", "-n", "pwom"+cmd).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if !strings.Contains(output, `httpd-24" created`) {
				e2e.Failf("The application with Legacy of ManifestList create failed with %s", cmd)
			}
			newCheck("expect", asAdmin, withoutNamespace, contain, "httpd-24:latest", ok, []string{"istag", "-n", "pwom" + cmd}).check(oc)
			output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("istag", "httpd-24:latest", "-n", "pwom"+cmd, "-o=jsonpath={..importMode} {..dockerImageManifests[*].architecture}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if !strings.Contains(output, "PreserveOriginal") || strings.Contains(output, "amd64 arm64") {
				e2e.Failf("The imagestream with PreserveOriginal without ManifestList imported error with %s", cmd)
			}
			nsError = oc.WithoutNamespace().AsAdmin().Run("delete").Args("ns", "pwom"+cmd).Execute()
			o.Expect(nsError).NotTo(o.HaveOccurred())

			g.By("Legacy mode without ManifestList image")
			defer oc.WithoutNamespace().AsAdmin().Run("delete").Args("ns", "lwom"+cmd, "--ignore-not-found").Execute()
			nsError = oc.WithoutNamespace().AsAdmin().Run("create").Args("ns", "lwom"+cmd).Execute()
			o.Expect(nsError).NotTo(o.HaveOccurred())
			output, err = oc.AsAdmin().WithoutNamespace().Run(cmd).Args("--image=quay.io/openshifttest/httpd-24@sha256:a4d1a67d994983b38fc462f1d54361632ccc9bf7f6e88bbaed9dea40d5568e80~https://github.com/openshift/httpd-ex.git", "--import-mode=Legacy", "--name=isi-legacy-non", "-n", "lwom"+cmd).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if !strings.Contains(output, `httpd-24" created`) {
				e2e.Failf("The application with Legacy of ManifestList create failed with %s", cmd)
			}
			newCheck("expect", asAdmin, withoutNamespace, contain, "httpd-24:latest", ok, []string{"istag", "-n", "lwom" + cmd}).check(oc)
			output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("istag", "httpd-24:latest", "-n", "lwom"+cmd, "-o=jsonpath={..importMode} {..dockerImageManifests[*].architecture}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if !strings.Contains(output, "Legacy") || strings.Contains(output, "amd64 arm64") {
				e2e.Failf("The imagestream with Legacy without ManifestList imported error with %s", cmd)
			}
			nsError = oc.WithoutNamespace().AsAdmin().Run("delete").Args("ns", "lwom"+cmd).Execute()
			o.Expect(nsError).NotTo(o.HaveOccurred())
		}
	})

	g.It("Author:xiuwang-Low-22862-CloudFront storage configuration with invalid values [Disruptive]", func() {
		g.By("Skip test if it's not cloudfront setting")
		cloudFrontset, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("config.image", "cluster", "-o=jsonpath={.spec.storage.s3.cloudFront}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if cloudFrontset == "" {
			g.Skip("Skip for the aws cluster with STS credential")
		}

		g.By("Set invalid duraion")
		output, err := oc.WithoutNamespace().AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"storage":{"s3":{"cloudFront":{"duration":300}}}}}`, "--type=merge").Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring(`duration: Invalid value: "int64`))

		g.By("Set invalid privateKey")
		keyvalue, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("config.image", "cluster", "-o=jsonpath={.spec.storage.s3.cloudFront.privateKey.key}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		keyname, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("config.image", "cluster", "-o=jsonpath={.spec.storage.s3.cloudFront.privateKey.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		expectedStatus1 := map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}
		defer func() {
			g.By("Remove proxy for imageregistry cluster")
			err = oc.WithoutNamespace().AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"storage":{"s3":{"cloudFront":{"privateKey":{"key":"`+keyvalue+`","name":"`+keyname+`"}}}}}}`, "--type=merge").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			err = waitCoBecomes(oc, "image-registry", 240, expectedStatus1)
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		err = oc.WithoutNamespace().AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"storage":{"s3":{"cloudFront":{"privateKey":{"key":"invalid.pem"}}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		waitErr := wait.Poll(10*time.Second, 1*time.Minute, func() (bool, error) {
			output, err := oc.AsAdmin().WithoutNamespace().Run("describe").Args("pod", "-l", "docker-registry=default", "-n", "openshift-image-registry").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if !strings.Contains(output, "references non-existent secret key: invalid.pem") {
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "private key set error")
		err = oc.WithoutNamespace().AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"storage":{"s3":{"cloudFront":{"privateKey":{"key":"`+keyvalue+`"}}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.WithoutNamespace().AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"storage":{"s3":{"cloudFront":{"privateKey":{"name":"notexisted"}}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		waitErr = wait.Poll(10*time.Second, 2*time.Minute, func() (bool, error) {
			output, err := oc.AsAdmin().WithoutNamespace().Run("describe").Args("pod", "-l", "docker-registry=default", "-n", "openshift-image-registry").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if !strings.Contains(output, `"registry-cloudfront" : secret "notexisted" not found`) {
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "privatekey name set error")
	})

	g.It("Author:xiuwang-Medium-11569-Check the registry-editor permission [Serial]", func() {
		g.By("Add registry-eidtor role to a project")
		registry_role := [3]string{"registry-admin", "registry-editor", "registry-viewer"}
		defer func() {
			for i := 0; i < 3; i++ {
				err := oc.AsAdmin().WithoutNamespace().Run("policy").Args("remove-role-from-user", registry_role[i], "-z", "test-"+registry_role[i], "-n", oc.Namespace()).Execute()
				o.Expect(err).NotTo(o.HaveOccurred())
			}
		}()
		for i := 0; i < 3; i++ {
			err := oc.AsAdmin().Run("create").Args("sa", "test-"+registry_role[i], "-n", oc.Namespace()).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			err = oc.AsAdmin().WithoutNamespace().Run("policy").Args("add-role-to-user", registry_role[i], "-z", "test-"+registry_role[i], "-n", oc.Namespace()).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		g.By("Check the registry-editor policy")
		output, err := oc.AsAdmin().WithoutNamespace().Run("policy").Args("who-can", "create", "imagestreamimages", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(output, "test-"+registry_role[0]) || !strings.Contains(output, "test-"+registry_role[1]) || strings.Contains(output, "test-"+registry_role[2]) {
			e2e.Failf("Dreate imagestreamimages permission is not correct.")
		}
		output, err = oc.AsAdmin().WithoutNamespace().Run("policy").Args("who-can", "deletecollection", "imagestreammappings", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(output, "test-"+registry_role[0]) || !strings.Contains(output, "test-"+registry_role[1]) || strings.Contains(output, "test-"+registry_role[2]) {
			e2e.Failf("Deletecollection imagestreammappings permission is not correct.")
		}
		output, err = oc.AsAdmin().WithoutNamespace().Run("policy").Args("who-can", "list", "imagestreams/secrets", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(output, "test-"+registry_role[0]) || !strings.Contains(output, "test-"+registry_role[1]) || strings.Contains(output, "test-"+registry_role[2]) {
			e2e.Failf("List imagestreams/secrets permission is not correct.")
		}
		output, err = oc.AsAdmin().WithoutNamespace().Run("policy").Args("who-can", "patch", "imagestreamtags", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(output, "test-"+registry_role[0]) || !strings.Contains(output, "test-"+registry_role[1]) || strings.Contains(output, "test-"+registry_role[2]) {
			e2e.Failf("Patch istag permission is not correct.")
		}
		output, err = oc.AsAdmin().WithoutNamespace().Run("policy").Args("who-can", "get", "imagestreams/layers", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(output, "test-"+registry_role[0]) || !strings.Contains(output, "test-"+registry_role[1]) || !strings.Contains(output, "test-"+registry_role[2]) {
			e2e.Failf("Get is/layers permission is not correct.")
		}
	})

	g.It("ROSA-OSD_CCS-ARO-Author:xiuwang-Medium-12765-Allow imagestream request deployment config triggers by different mode('TagreferencePolicy':source/local)", func() {
		g.By("Import an image to create imagestream with source policy")
		err := oc.AsAdmin().WithoutNamespace().Run("tag").Args("quay.io/openshifttest/deployment-example@sha256:9d29ff0fdbbec33bb4eebb0dbe0d0f3860a856987e5481bb0fc39f3aba086184", "deployment-source:latest", "--import-mode=PreserveOriginal", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, oc.Namespace(), "deployment-source", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create deployment with imagestream and check the image info")
		err = oc.WithoutNamespace().AsAdmin().Run("set").Args("image-lookup", "deployment-source", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("deployment", "deployment-source", "--image=deployment-source", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check whether the image can be pulled from external registry")
		expectInfo := `Successfully pulled image "quay.io/openshifttest/deployment-example`
		pollErr := wait.Poll(15*time.Second, 200*time.Second, func() (bool, error) {
			output, describeErr := oc.AsAdmin().WithoutNamespace().Run("describe").Args("pod", "-l", "app=deployment-source", "-n", oc.Namespace()).Output()
			o.Expect(describeErr).NotTo(o.HaveOccurred())
			if strings.Contains(output, expectInfo) {
				return true, nil
			}
			e2e.Logf("Continue to next round")
			return false, nil
		})
		exutil.AssertWaitPollNoErr(pollErr, fmt.Sprintf("Pod doesn't show expected log %v", expectInfo))

		g.By("Import an image to create imagestream with local policy")
		err = oc.AsAdmin().WithoutNamespace().Run("tag").Args("quay.io/openshifttest/deployment-example@sha256:9d29ff0fdbbec33bb4eebb0dbe0d0f3860a856987e5481bb0fc39f3aba086184", "deployment-local:latest", "--import-mode=PreserveOriginal", "--reference-policy=local", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, oc.Namespace(), "deployment-local", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create deployment with imagestream and check the image info")
		err = oc.WithoutNamespace().AsAdmin().Run("set").Args("image-lookup", "deployment-local", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("deployment", "deployment-local", "--image=deployment-local", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check whether the image can be pulled from internal registry")
		expectInfo = `Successfully pulled image "image-registry.openshift-image-registry.svc:5000`
		pollErr = wait.Poll(15*time.Second, 200*time.Second, func() (bool, error) {
			output, describeErr := oc.AsAdmin().WithoutNamespace().Run("describe").Args("pod", "-l", "app=deployment-local", "-n", oc.Namespace()).Output()
			o.Expect(describeErr).NotTo(o.HaveOccurred())
			if strings.Contains(output, expectInfo) {
				return true, nil
			}
			e2e.Logf("Continue to next round")
			return false, nil
		})
		exutil.AssertWaitPollNoErr(pollErr, fmt.Sprintf("Pod doesn't show expected log %v", expectInfo))

	})

	g.It("ROSA-OSD_CCS-ARO-Author:xiuwang-High-10862-Delete spec tags using 'oc tag -d'", func() {
		g.By("Import an image to create imagestreams")
		err := oc.AsAdmin().WithoutNamespace().Run("tag").Args("quay.io/openshifttest/base-alpine@sha256:3126e4eed4a3ebd8bf972b2453fa838200988ee07c01b2251e3ea47e4b1f245c", "mystream:v1", "mystream:latest", "--import-mode=PreserveOriginal", "--source=docker", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, oc.Namespace(), "mystream", "v1")
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, oc.Namespace(), "mystream", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check the imagestream tag info")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("imagestream", "mystream", "--template={{range .spec.tags}} name: {{.name}} {{end}};{{range .status.tags}} tag: {{.tag}} {{end}}", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(output, "name: latest  name: v1 ; tag: latest  tag: v1") {
			e2e.Failf("Fail to get the imagestream info")
		}

		g.By("Delete the spec tags")
		err = oc.AsAdmin().WithoutNamespace().Run("tag").Args("mystream:v1", "mystream:latest", "--delete=true", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("imagestream", "mystream", "--template={{.spec}};{{range .status.tags}} tag: {{.tag}} {{end}}", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(output, "map[lookupPolicy:map[local:false]];") {
			e2e.Failf("Fail to get the imagestream info")
		}

		output, err = oc.AsAdmin().WithoutNamespace().Run("tag").Args("mystream:nonexist", "--delete=true", "-n", oc.Namespace()).Output()
		o.Expect(err).To(o.HaveOccurred())
		e2e.Logf("print the output", output)
		if !strings.Contains(output, `mystream:nonexist" not found`) {
			e2e.Failf("Shouldn't delete nonexist spec tag")
		}
	})

	g.It("ROSA-OSD_CCS-ARO-Author:xiuwang-High-65979-Expose registry CAs as one to mco", func() {
		g.By("Check if there is image-registry-ca cm configured")
		cmCons, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("configmap", "-n", "openshift-config-managed").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(cmCons, "image-registry-ca") {
			g.Skip("No image-registry-ca cm configured")
		}

		g.By("Check if image-registry-certificates cm contains the content of image-registry-ca cm")
		imageRegCA, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("configmap", "image-registry-ca", "-o=jsonpath={.data}", "-n", "openshift-config-managed").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		imageRegCA = strings.TrimSuffix(string(imageRegCA), "}")
		imageRegCA = strings.TrimPrefix(string(imageRegCA), "{")
		regCerts, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("configmap", "image-registry-certificates", "-o=jsonpath={.data}", "-n", "openshift-image-registry").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(regCerts, imageRegCA) {
			e2e.Failf("image-registry-certificates doesn't contain the content of image-registry-ca configmap")
		}

		g.By("image-registry-ca cm doesn't contain the content of additionalTrustedCA")
		trustCAName, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("image.config", "-o=jsonpath={..spec.additionalTrustedCA.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if trustCAName != "" {
			addTrustCA, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("configmap", trustCAName, "-o=jsonpath={.data}", "-n", "openshift-config").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			addTrustCA = strings.TrimSuffix(string(addTrustCA), "}")
			addTrustCA = strings.TrimPrefix(string(addTrustCA), "{")
			if strings.Contains(imageRegCA, addTrustCA) {
				e2e.Failf("image-registry-ca shouldn't contain the content of the additionalTrustedCA")
			}
		}
	})

	g.It("Author:xiuwang-Medium-30419-Medium-30266-Image registry pods could be running when add infra node nodeAffinity scheduler [Disruptive]", func() {
		g.By("Check if cluster is sno")
		workerNodes, _ := exutil.GetClusterNodesBy(oc, "worker")
		if len(workerNodes) == 1 {
			g.Skip("Skip for sno cluster")
		}

		g.By("Set infra node nodeAffinity when no infra node land - 30266")
		expectedStatus1 := map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}

		defer func() {
			g.By("Recover image registry change")
			err := oc.AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"affinity":null}}`, "--type=merge").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			err = waitCoBecomes(oc, "image-registry", 240, expectedStatus1)
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		err := oc.WithoutNamespace().AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"affinity":{"nodeAffinity":{"preferredDuringSchedulingIgnoredDuringExecution":[{"preference":{"matchExpressions":[{"key":"node-role.kubernetes.io/infra","operator":"In","values":[""]}]},"weight":100}]}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitCoBecomes(oc, "image-registry", 240, expectedStatus1)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Set infra node nodeAffinity when infra node set - 30419")
		infraworkers, err := oc.AsAdmin().Run("get").Args("node", "-l", "node-role.kubernetes.io/worker,kubernetes.io/os!=windows,node-role.kubernetes.io/edge!=", `-o=jsonpath={.items[*].metadata.name}`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		infraList := strings.Fields(infraworkers)
		if len(infraList) < 2 {
			g.Skip("Skip infra node test")
		}

		defer oc.AsAdmin().Run("label").Args("node", infraList[0], infraList[1], "node-role.kubernetes.io/infra-").Execute()
		err = oc.AsAdmin().Run("label").Args("node", infraList[0], infraList[1], `node-role.kubernetes.io/infra=`).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.WithoutNamespace().AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"affinity":{"nodeAffinity":{"preferredDuringSchedulingIgnoredDuringExecution":null,"requiredDuringSchedulingIgnoredDuringExecution":{"nodeSelectorTerms":[{"matchExpressions":[{"key":"node-role.kubernetes.io/infra","operator":"In","values":[""]}]}]}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitCoBecomes(oc, "image-registry", 240, expectedStatus1)
		o.Expect(err).NotTo(o.HaveOccurred())

		nodeList, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-o", "wide", "-n", "openshift-image-registry", "-l", "docker-registry=default", "-o=jsonpath={.items[*].spec.nodeName}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(nodeList, infraList[0]) || !strings.Contains(nodeList, infraList[1]) {
			e2e.Failf("The registry pods don't be scheduled to infra nodes")
		}
	})

	g.It("NonHyperShiftHOST-Author:xiuwang-Medium-24353-Registry operator storage setup - azure [Disruptive]", func() {
		exutil.SkipIfPlatformTypeNot(oc, "Azure")
		g.By("Set status variables")
		expectedStatus1 := map[string]string{"Progressing": "True"}
		expectedStatus2 := map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}

		g.By("Check image-registry-private-configuration secret if created")
		output, _ := oc.AsAdmin().Run("get").Args("secret/image-registry-private-configuration", "-n", "openshift-image-registry").Output()
		o.Expect(output).To(o.ContainSubstring("image-registry-private-configuration"))
		secretData, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("secret/installer-cloud-credentials", "-o=jsonpath={.data}", "-n", "openshift-image-registry").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(secretData, "azure_client_id") || !strings.Contains(secretData, "azure_region") || !strings.Contains(secretData, "azure_subscription_id") {
			e2e.Failf("The installer-cloud-credentials secret don't contain azure credentials for registry")
		}

		g.By("Set image registry azure storage parameters")
		azureStorageInfo, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("configs.imageregistry/cluster", "-o=jsonpath={.spec.storage.azure}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			g.By("Recover image registry change")
			patchInfo := fmt.Sprintf("{\"spec\":{\"storage\":{\"azure\":%v}}}", azureStorageInfo)
			err := oc.AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", patchInfo, "--type=merge").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			err = waitCoBecomes(oc, "image-registry", 240, expectedStatus1)
			o.Expect(err).NotTo(o.HaveOccurred())
			err = waitCoBecomes(oc, "image-registry", 240, expectedStatus2)
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		accountName, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("configs.imageregistry/cluster", "-o=jsonpath={.spec.storage.azure.accountName}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(accountName) <= 3 || len(accountName) >= 25 {
			e2e.Fail("The length of accountName should be greater than 3 and litter than 25")
		}
		accountName1 := getRandomString()
		err = oc.AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"storage":{"azure":{"accountName":"`+accountName1+`"}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitCoBecomes(oc, "image-registry", 240, expectedStatus1)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitCoBecomes(oc, "image-registry", 240, expectedStatus2)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Container could be recreated")
		err = oc.AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"storage":{"azure":{"container": null}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitCoBecomes(oc, "image-registry", 240, expectedStatus1)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitCoBecomes(oc, "image-registry", 240, expectedStatus2)
		o.Expect(err).NotTo(o.HaveOccurred())
	})
	g.It("Author:xiuwang-NonHyperShiftHOST-Critical-64796-Image Registry support azure workload identity", func() {
		exutil.SkipIfPlatformTypeNot(oc, "Azure")
		output, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("infrastructure.config.openshift.io", "-o=jsonpath={..status.platformStatus.azure}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(output, "AzureStackCloud") {
			g.Skip("Skip for AzureStackCloud, ASH is using manual credentail mode, but operators using static token")
		}
		credType, err := oc.AsAdmin().Run("get").Args("cloudcredentials.operator.openshift.io/cluster", "-o=jsonpath={.spec.credentialsMode}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(credType, "Manual") {
			g.Skip("Skip test on non azure sts cluster")
		}
		secretData, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("secret/installer-cloud-credentials", "-o=jsonpath={.data}", "-n", "openshift-image-registry").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(secretData, "azure_federated_token_file") {
			e2e.Failf("The installer-cloud-credentials secret don't use azure workload identity for registry")
		}
		envList, err := oc.WithoutNamespace().AsAdmin().Run("set").Args("env", "deploy/image-registry", "--list", "-n", "openshift-image-registry").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(envList, "AZURE_FEDERATED_TOKEN_FILE") || strings.Contains(envList, "REGISTRY_STORAGE_AZURE_ACCOUNTKEY") {
			e2e.Failf("The image registry don't use azure workload identity")
		}
		g.By("Check if could push/pull image to/from registry")
		checkRegistryFunctionFine(oc, "test-64796", oc.Namespace())
	})

	g.It("Author:wewang-NonHyperShiftHOST-High-67388-Image Registry Pull through should support idms/itms [Serial]", func() {
		//If a cluster contains any ICSP, it will skip the case
		if checkICSP(oc) {
			g.Skip("This cluster contain ICSP, skip the test.")
		}
		var (
			idmsFile = filepath.Join(imageRegistryBaseDir, "idms.yaml")
			idmssrc  = idmsSource{
				name:     "digest-mirror",
				mirrors:  "",
				source:   "quay.io/openshifttest/hello-openshift",
				template: idmsFile,
			}
			itmsFile = filepath.Join(imageRegistryBaseDir, "itms.yaml")
			itmssrc  = itmsSource{
				name:     "tag-mirror",
				mirrors:  "",
				source:   "quay.io/openshifttest/hello-openshift",
				template: itmsFile,
			}
			mc = machineConfig{
				name:     "",
				pool:     "worker",
				source:   "",
				path:     "",
				template: "",
			}
			isFile = filepath.Join(imageRegistryBaseDir, "imagestream-tag-or-digest.yaml")
			issrc  = isStruct{
				name:      "is67388",
				namespace: "",
				repo:      "",
				template:  isFile,
			}
		)
		defer func() {
			idmssrc.delete(oc)
			itmssrc.delete(oc)
			mc.waitForMCPComplete(oc)
			mc.pool = "master"
			mc.waitForMCPComplete(oc)
		}()
		g.By("Create idms and itms")
		mirror_registry := GetMirrorRegistry(oc)
		idmssrc.mirrors = mirror_registry + "/openshifttest/hello-openshift"
		itmssrc.mirrors = mirror_registry + "/openshifttest/hello-openshift"
		idmssrc.create(oc)
		itmssrc.create(oc)
		mc.waitForMCPComplete(oc)
		mc.pool = "master"
		mc.waitForMCPComplete(oc)

		g.By("Create imagestream using tag")
		issrc.name = "is67388tag"
		issrc.namespace = oc.Namespace()
		issrc.repo = "quay.io/openshifttest/hello-openshift:1.2.0"
		issrc.create(oc)
		err := waitForAnImageStreamTag(oc, oc.Namespace(), "is67388tag", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create imagestream using digest")
		issrc.name = "is67388digest"
		issrc.namespace = oc.Namespace()
		issrc.repo = "quay.io/openshifttest/hello-openshift@sha256:4200f438cf2e9446f6bcff9d67ceea1f69ed07a2f83363b7fb52529f7ddd8a83"
		issrc.create(oc)
		err = waitForAnImageStreamTag(oc, oc.Namespace(), "is67388digest", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("create a pod using the imagestream:is67388digest")
		err = oc.AsAdmin().WithoutNamespace().Run("new-app").Args("is67388digest", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		checkPodsRunningWithLabel(oc, oc.Namespace(), "deployment=is67388digest", 1)

		g.By("create a pod using the imagestream:is67388tag")
		err = oc.AsAdmin().WithoutNamespace().Run("new-app").Args("is67388tag", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		checkPodsRunningWithLabel(oc, oc.Namespace(), "deployment=is67388tag", 1)
	})

	g.It("Author:xiuwang-Low-50177-Update invalid custom CA for s3 endpoint [Disruptive]", func() {
		exutil.SkipIfPlatformTypeNot(oc, "AWS")
		g.By("Can't update longer name for trustedCA")
		originCA, err := oc.AsAdmin().Run("get").Args("config.image/cluster", "-o=jsonpath={.spec.storage.s3.trustedCA.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		expectedStatus := map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}
		defer func() {
			g.By("Recover image registry change")
			err := oc.AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"storage":{"s3":{"trustedCA":{"name":"`+originCA+`"}}}}}`, "--type=merge").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			err = waitCoBecomes(oc, "image-registry", 240, expectedStatus)
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		longName := "longlonglonglonglonglonglonglonglonglonglonglonglonglonglonglonglonglonglonglonglonglonglonglonglonglonglonglonglonglonglonglonglonglonglonglonglonglonglonglonglonglonglonglonglonglonglonglonglonglonglonglonglonglonglonglonglonglonglonglonglonglonglonglong"
		output, err := oc.WithoutNamespace().AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"storage":{"s3":{"trustedCA":{"name":"`+longName+`"}}}}}`, "--type=merge").Output()
		o.Expect(err).To(o.HaveOccurred())
		if !strings.Contains(output, "may not be longer than 253") {
			e2e.Failf("The image registry shouldn't update with name longer than 253")
		}
		err = oc.WithoutNamespace().AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"storage":{"s3":{"trustedCA":{"name":"not-exist-ca"}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("co/image-registry", "-o=jsonpath={.status.conditions[?(@.type==\"Progressing\")].message}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(output, `failed to get trusted CA "not-exist-ca": configmap "not-exist-ca" not found`) {
			e2e.Failf("The image registry should report error when trustedCA is not found")
		}
	})

	g.It("Author:xiuwang-Medium-67714-Could get the correct digest when import images with the same digest but different import mode at same time", func() {
		var (
			isFile = filepath.Join(imageRegistryBaseDir, "is-same-image-differemt-importmode.yaml")
			issrc  = isStruct{
				name:      "is67714",
				namespace: "",
				repo:      "quay.io/openshifttest/hello-openshift@sha256:4200f438cf2e9446f6bcff9d67ceea1f69ed07a2f83363b7fb52529f7ddd8a83",
				template:  isFile,
			}
		)
		issrc.namespace = oc.Namespace()
		issrc.create(oc)
		err := waitForAnImageStreamTag(oc, oc.Namespace(), "is67714", "tag-manifest")
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, oc.Namespace(), "is67714", "tag-manifest-preserve-original")
		o.Expect(err).NotTo(o.HaveOccurred())
		output, err := oc.AsAdmin().WithoutNamespace().Run("describe").Args("imagestreamtag", "is67714:tag-manifest", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(output, "Manifests:") {
			e2e.Failf("The import-mode is wrong, shoule be Legacy mode")
		}
		outputMan, err := oc.AsAdmin().WithoutNamespace().Run("describe").Args("imagestreamtag", "is67714:tag-manifest-preserve-original", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(outputMan, "Manifests:") {
			e2e.Failf("The import-mode is wrong, shoule be PreserveOriginal mode")
		}
	})

	g.It("Author:wewang-Medium-68732-image registry should use ibmcos object storage on IPI-IBM cluster", func() {
		g.By("Check platforms")
		exutil.SkipIfPlatformTypeNot(oc, "IBMCloud")
		g.By("Check if image registry managementState is Managed")
		output, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("config.image/cluster", "-o=jsonpath={.spec.managementState}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).Should(o.Equal("Managed"))
		g.By("Check if image registry used ibmcos object storage")
		output, err = oc.WithoutNamespace().AsAdmin().Run("get").Args("config.image/cluster", "-o=jsonpath={.spec.storage.ibmcos}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).Should(o.ContainSubstring("bucket"))
	})

	g.It("Author:wewang-High-6867-allow users to configure private storage accounts in Azure", func() {
		g.By("Check platforms")
		exutil.SkipIfPlatformTypeNot(oc, "Azure")
		g.By("Check enable internal image registry")
		output, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("config.image/cluster", "-o=jsonpath={.spec.storage.azure.networkAccess.type}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if output != "Internal" {
			g.Skip("intenal image registry is not enabled, so skip it")
		}
		g.By("Check the cluster's vnet and subnet")
		vnet, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("machinesets", "-n", "openshift-machine-api", "-o=jsonpath={.items[0].spec.template.spec.providerSpec.value.vnet}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		subnet, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("machinesets", "-n", "openshift-machine-api", "-o=jsonpath={.items[0].spec.template.spec.providerSpec.value.subnet}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		vnet_registry, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("config.image/cluster", "-o=jsonpath={.spec.storage.azure.networkAccess.internal.vnetName}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		subnet_registry, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("config.image/cluster", "-o=jsonpath={.spec.storage.azure.networkAccess.internal.subnetName}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		endpoint_registry, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("config.image/cluster", "-o=jsonpath={.spec.storage.azure.networkAccess.internal.privateEndpointName}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(vnet).Should(o.Equal(vnet_registry))
		o.Expect(subnet).Should(o.Equal(subnet_registry))
		o.Expect(endpoint_registry).Should(o.Equal(endpoint_registry))
		o.Expect(endpoint_registry).NotTo(o.BeEmpty(), "private endpoint name is empty")
	})

	g.It("Author:wewang-Medium-68733-image registry should use ibmcos object storage on IPI-IBM cluster", func() {
		g.By("Check platforms")
		exutil.SkipIfPlatformTypeNot(oc, "IBMCloud")
		g.By("Check image registry use ibmcos object storage")
		output, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("config.image/cluster", "-o=jsonpath={.spec.storage.ibmcos}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("bucket"))
		o.Expect(output).To(o.ContainSubstring("resourceGroupName"))
		o.Expect(output).To(o.ContainSubstring("location"))
	})

	g.It("Author:wewang-High-69008-Add IBM Cloud service endpoint override support", func() {
		g.By("Check platforms")
		exutil.SkipIfPlatformTypeNot(oc, "IBMCloud")
		g.By("Check the cluster is a disconnected private cluster")
		output, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("infrastructure", "-o=jsonpath={.items[0].status.platformStatus.ibmcloud.serviceEndpoints[?(@.name==\"IAM\")].url}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(output, "private.iam.cloud") {
			g.Skip("Skip for no disconnected private cluster")
		}
		output, err = oc.WithoutNamespace().AsAdmin().Run("get").Args("infrastructure", "-o=jsonpath={.items[0].status.platformStatus.ibmcloud.serviceEndpoints[?(@.name==\"COS\")].url}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		s3_endpoint, getErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment/image-registry", "-n", "openshift-image-registry", "-o=jsonpath={.spec.template.spec.containers[0].env[?(@.name==\"REGISTRY_STORAGE_S3_REGIONENDPOINT\")].value}").Output()
		o.Expect(getErr).NotTo(o.HaveOccurred())
		o.Expect(output).Should(o.Equal(s3_endpoint))
	})

	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-Author:wewang-Medium-68181-OpenShift APIServer should performs a GET request to valid endpoint results in API server [Disruptive]", func() {
		g.By("In default image registry cluster Managed")
		out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("configs.imageregistry/cluster", "-o=jsonpath={.spec.managementState}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(out).Should(o.Equal("Managed"))

		g.By("Check build works")
		checkRegistryFunctionFine(oc, "test-68181", oc.Namespace())

		defer func() {
			exutil.By("Recovering Internal image registry")
			output, err := oc.WithoutNamespace().AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"managementState":"Managed"}}`, "--type=merge").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(output, "patched (no change)") {
				e2e.Logf("No changes to the internal image registry.")
			} else {
				exutil.By("Waiting KAS and Image registry reboot after the Internal Image Registry was enabled")
				e2e.Logf("Checking kube-apiserver operator should be in Progressing in 100 seconds")
				expectedStatus := map[string]string{"Progressing": "True"}
				err = waitCoBecomes(oc, "kube-apiserver", 100, expectedStatus)
				exutil.AssertWaitPollNoErr(err, "kube-apiserver operator is not start progressing in 100 seconds")
				e2e.Logf("Checking kube-apiserver operator should be Available in 1500 seconds")
				expectedStatus = map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}
				err = waitCoBecomes(oc, "kube-apiserver", 1500, expectedStatus)
				exutil.AssertWaitPollNoErr(err, "kube-apiserver operator is not becomes available in 1500 seconds")
				err = waitCoBecomes(oc, "openshift-apiserver", 100, expectedStatus)
				exutil.AssertWaitPollNoErr(err, "openshift-apiserver operator is not becomes available in 100 seconds")
				err = waitCoBecomes(oc, "image-registry", 100, expectedStatus)
				exutil.AssertWaitPollNoErr(err, "image-registry operator is not becomes available in 100 seconds")
			}
		}()

		err = oc.WithoutNamespace().AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"managementState":"Removed"}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Check image-registry pods are removed")
		checkRegistrypodsRemoved(oc)

		g.By("Point build out to external registry")
		err = oc.WithoutNamespace().AsAdmin().Run("patch").Args("bc/test-68181", "-p", `{"spec":{"output":{"to":{"kind":"DockerImage","name":"quay.io/openshifttests/busybox:test"}}}}`, "--type=merge", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.Run("start-build").Args("test-68181").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = exutil.WaitForABuild(oc.BuildClient().BuildV1().Builds(oc.Namespace()), fmt.Sprintf("%s-2", "test-68181"), nil, nil, nil)
		o.Expect(err).To(o.HaveOccurred())
		g.By("Check the logs of openshift-apiserver")
		var podsOfOpenshiftApiserver []corev1.Pod
		podsOfOpenshiftApiserver = listPodStartingWith("apiserver", oc, "openshift-apiserver")
		if len(podsOfOpenshiftApiserver) == 0 {
			e2e.Failf("Error retrieving logs")
		}
		foundLog := dePodLogs(podsOfOpenshiftApiserver, oc, ApiServerInfo)
		o.Expect(foundLog).NotTo(o.BeTrue())
	})

	g.It("NonHyperShiftHOST-Author:wewang-Critical-72015-secret of Image-registry operator require more keys while using swift storage backend [Disruptive]", func() {
		g.By("Check platforms")
		exutil.SkipIfPlatformTypeNot(oc, "OpenStack")
		expectedStatus1 := map[string]string{"Progressing": "True"}
		expectedStatus2 := map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}

		g.By("Get the URL for obtaining an authentication token")
		authurl := get_osp_authurl(oc)
		if authurl == "" {
			g.Skip("no authurl for the cluster, skip test")
		}

		g.By("Add authurl to swift storage config")
		pathInfo := fmt.Sprintf("{\"spec\":{\"storage\":{\"swift\":{\"authURL\":\"%v\",\"regionName\":null, \"regionID\":null, \"domainID\":null, \"domain\":null, \"tenantID\":null}}}}", authurl)
		recoverInfo := fmt.Sprintf("{\"spec\":{\"storage\":{\"swift\":{\"authURL\":null}}}}")
		defer func() {
			g.By("recover registry storage config")
			output, err := oc.AsAdmin().Run("patch").Args("config.image/cluster", "-p", recoverInfo, "--type=merge").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).To(o.ContainSubstring("patched"))
			err = waitCoBecomes(oc, "image-registry", 100, expectedStatus2)
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		out, err := oc.AsAdmin().Run("patch").Args("config.image/cluster", "-p", pathInfo, "--type=merge").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(out).To(o.ContainSubstring("patched"))
		err = waitCoBecomes(oc, "image-registry", 100, expectedStatus2)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create a custom secret with invalid value")
		defer func() {
			g.By("Recover registry")
			err = oc.WithoutNamespace().AsAdmin().Run("delete").Args("secret/image-registry-private-configuration-user", "-n", "openshift-image-registry").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			err = waitCoBecomes(oc, "image-registry", 100, expectedStatus2)
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		err = oc.WithoutNamespace().AsAdmin().Run("create").Args("secret", "generic", "image-registry-private-configuration-user", "-n", "openshift-image-registry").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		out, err = oc.WithoutNamespace().AsAdmin().Run("get").Args("secret/image-registry-private-configuration-user", "-n", "openshift-image-registry").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(out).To(o.ContainSubstring("image-registry-private-configuration-user"))

		g.By("check the co/image-registry is degrade")
		err = waitCoBecomes(oc, "image-registry", 100, expectedStatus1)
		o.Expect(err).NotTo(o.HaveOccurred())
		message, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("co/image-registry", "-o=jsonpath={.status.conditions[?(@.type==\"Progressing\")].message}").Output()
		o.Expect(message).To(o.ContainSubstring("unable to sync storage configuration"))
	})

	g.It("NonHyperShiftHOST-Author:wewang-High-66533-Users providing custom gcp tags are set with bucket creation", func() {
		g.By("Check platforms")
		exutil.SkipIfPlatformTypeNot(oc, "GCP")

		g.By("Check the cluster is with resourceTags")
		output, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("infrastructure.config.openshift.io", "-o=jsonpath={..status.platformStatus.gcp}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(output, "resourceTags") {
			g.Skip("Skip for no resourceTags")
		}

		g.By("Get storage bucket and region")
		o.Expect(err).NotTo(o.HaveOccurred())
		bucketName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("configs.imageregistry/cluster", "-o=jsonpath={.spec.storage.gcs.bucket}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		regionName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("configs.imageregistry/cluster", "-o=jsonpath={.spec.storage.gcs.region}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check the tags")
		getResourceTags, err := getgcloudClient(oc).GetResourceTags(bucketName, regionName)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(getResourceTags).To(o.ContainSubstring("tagValues/281475558055748"))
		o.Expect(getResourceTags).To(o.ContainSubstring("tagValues/281479576182962"))
	})

	g.It("NonHyperShiftHOST-Author:wewang-Critical-72014-Image Registry Operator should short lease acquire time [Disruptive]", func() {
		g.By("Check deployment/cluster-image-registry-operator strategy is Recreate")
		expectedStatus := map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}
		out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment/cluster-image-registry-operator", "-n", "openshift-image-registry", "-o=jsonpath={.spec.strategy.type}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(out).Should(o.Equal("Recreate"))

		g.By("Set image registry to Unmanaged")
		defer func() {
			g.By("Recover image registry change")
			err := oc.AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"managementState":"Managed"}}`, "--type=merge").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			err = waitCoBecomes(oc, "image-registry", 240, expectedStatus)
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		err = oc.WithoutNamespace().AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"managementState":"Unmanaged"}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Get registry operator pod name")
		oldPod, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", "openshift-image-registry", "-l", "name=cluster-image-registry-operator", "-o", "name").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Add an annotation to the operator deployment")
		defer func() {
			g.By("Recover registry operator")
			err = oc.AsAdmin().Run("patch").Args("deploy/cluster-image-registry-operator", "-p", `{"spec":{"template":{"metadata":{"annotations":{"test":null}}}}}`, "-n", "openshift-image-registry", "--type=merge").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			out, err = oc.AsAdmin().Run("get").Args("deployment/cluster-image-registry-operator", "-n", "openshift-image-registry", "-o=jsonpath={.spec.template.metadata.annotations}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(out).ShouldNot(o.Equal("invalid"))
			err = waitCoBecomes(oc, "image-registry", 60, expectedStatus)
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		err = oc.AsAdmin().Run("patch").Args("deploy/cluster-image-registry-operator", "-p", `{"spec":{"template":{"metadata":{"annotations":{"test":"invalid"}}}}}`, "-n", "openshift-image-registry", "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		out, err = oc.AsAdmin().Run("get").Args("deployment/cluster-image-registry-operator", "-n", "openshift-image-registry", "-o=jsonpath={.spec.template.metadata.annotations.test}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(out).Should(o.Equal("invalid"))
		err = waitCoBecomes(oc, "image-registry", 60, expectedStatus)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Confirm image registry operator redeployed")
		err = wait.Poll(20*time.Second, 1*time.Minute, func() (bool, error) {
			newPod, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", "openshift-image-registry", "-l", "name=cluster-image-registry-operator", "-o", "name").Output()
			if err != nil {
				return false, err
			}
			if newPod == oldPod {
				e2e.Logf("Continue to next round")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "image registry is not redeployed")

		g.By("Check less than 1 minute waiting for the lease acquire")
		result, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("deployment.apps/cluster-image-registry-operator", "-n", "openshift-image-registry").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(result, "lease openshift-image-registry/openshift-master-controllers")).To(o.BeTrue())
		regLog, _ := regexp.Compile(".*attempting to acquire leader lease openshift-image-registry/openshift-master-controllers.*")
		requestLog := regLog.FindAllString(result, 1)
		regLog, _ = regexp.Compile(".*successfully acquired lease openshift-image-registry/openshift-master-controllers.*")
		acquiredLog := regLog.FindAllString(result, 1)

		startTime := filterTimestampFromLogs(requestLog[0], 1)
		endTime := filterTimestampFromLogs(acquiredLog[0], 1)
		o.Expect(getTimeDifferenceInMinute(startTime[0], endTime[0])).Should(o.BeNumerically("<", 0.1))
	})

	g.It("Author:xiuwang-NonHyperShiftHOST-Critical-73769-Add chunksize for s3 api compatible object storage - TP [Disruptive]", func() {
		g.By("Check if the cluster is TechPreviewNoUpgrade")
		exutil.SkipIfPlatformTypeNot(oc, "AWS")
		if !exutil.IsTechPreviewNoUpgrade(oc) {
			g.Skip("featureSet: TechPreviewNoUpgrade is required for this test")
		}
		g.By("Add chunkSizeMiB parameter")
		expectedStatus := map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}
		defer func() {
			g.By("Recover image registry change")
			err := oc.WithoutNamespace().AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"storage":{"s3":{"chunkSizeMiB":null}}}}`, "--type=merge").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			err = waitCoBecomes(oc, "image-registry", 240, expectedStatus)
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		err := oc.WithoutNamespace().AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"storage":{"s3":{"chunkSizeMiB":10}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitCoBecomes(oc, "image-registry", 240, expectedStatus)
		o.Expect(err).NotTo(o.HaveOccurred())
		validateResourceEnv(oc, "openshift-image-registry", "deployment.apps/image-registry", "REGISTRY_STORAGE_S3_CHUNKSIZE=10485760")
		g.By("Check if push successfully when push large image")
		localImageName := "FROM quay.io/openshifttest/busybox:multiarch\nRUN dd if=/dev/urandom of=/bigfile bs=1M count=1024"
		err = oc.AsAdmin().WithoutNamespace().Run("new-build").Args("-D", localImageName, "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Check the build logs")
		logInfo = "Successfully pushed image-registry.openshift-image-registry.svc:5000/" + oc.Namespace() + "/busybox"
		err = wait.Poll(2*time.Minute, 10*time.Minute, func() (bool, error) {
			output, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("bc/busybox", "-n", oc.Namespace()).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(output, logInfo) {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "Failed to push large image")
	})

	g.It("Author:xiuwang-NonHyperShiftHOST-Low-73770-chunksize neigative test - TP [Disruptive]", func() {
		g.By("Check if the cluster is TechPreviewNoUpgrade")
		exutil.SkipIfPlatformTypeNot(oc, "AWS")
		if !exutil.IsTechPreviewNoUpgrade(oc) {
			g.Skip("featureSet: TechPreviewNoUpgrade is required for this test")
		}
		g.By("Set chunkSizeMiB to not allow value ")
		expectedStatus := map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}
		defer func() {
			g.By("Recover image registry change")
			err := oc.WithoutNamespace().AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"storage":{"s3":{"chunkSizeMiB":null}}}}`, "--type=merge").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			err = waitCoBecomes(oc, "image-registry", 240, expectedStatus)
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		output, err := oc.WithoutNamespace().AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"storage":{"s3":{"chunkSizeMiB":4}}}}`, "--type=merge").Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(strings.Contains(output, "Invalid value: 4")).To(o.BeTrue())
		o.Expect(strings.Contains(output, "spec.storage.s3.chunkSizeMiB in body should be greater than or equal to 5")).To(o.BeTrue())
		output, err = oc.WithoutNamespace().AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"storage":{"s3":{"chunkSizeMiB":5125}}}}`, "--type=merge").Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(strings.Contains(output, "Invalid value: 5125")).To(o.BeTrue())
		o.Expect(strings.Contains(output, "spec.storage.s3.chunkSizeMiB in body should be less than or equal to 5120")).To(o.BeTrue())
		g.By("Add chunkSizeMiB to boundary value")
		err = oc.WithoutNamespace().AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"storage":{"s3":{"chunkSizeMiB":5}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitCoBecomes(oc, "image-registry", 240, expectedStatus)
		o.Expect(err).NotTo(o.HaveOccurred())
		validateResourceEnv(oc, "openshift-image-registry", "deployment.apps/image-registry", "REGISTRY_STORAGE_S3_CHUNKSIZE=5242880")
		err = oc.WithoutNamespace().AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"storage":{"s3":{"chunkSizeMiB":5120}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitCoBecomes(oc, "image-registry", 240, expectedStatus)
		o.Expect(err).NotTo(o.HaveOccurred())
		validateResourceEnv(oc, "openshift-image-registry", "deployment.apps/image-registry", "REGISTRY_STORAGE_S3_CHUNKSIZE=5368709120")
		err = oc.WithoutNamespace().AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"storage":{"s3":{"chunkSizeMiB":2148}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitCoBecomes(oc, "image-registry", 240, expectedStatus)
		o.Expect(err).NotTo(o.HaveOccurred())
		validateResourceEnv(oc, "openshift-image-registry", "deployment.apps/image-registry", "REGISTRY_STORAGE_S3_CHUNKSIZE=2252341248")
	})
})
