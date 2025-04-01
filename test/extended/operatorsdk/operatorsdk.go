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
	var oc = exutil.NewCLIWithoutNamespace("default")
	var ocpversion = "4.19"
	var upstream = true
	var upstreamversion = "1.39.1"

	/*
		g.BeforeEach(func() {
			g.Skip("OperatorSDK is deprecated since OCP 4.16, so skip it")
		})
	*/

	// author: jitli@redhat.com
	g.It("Author:jitli-VMonly-Medium-34945-ansible Add flag metricsaddr for ansible operator", func() {
		if os.Getenv("HTTP_PROXY") != "" || os.Getenv("http_proxy") != "" {
			g.Skip("HTTP_PROXY is not empty - skipping test ...")
		}
		imageTag := "registry-proxy.engineering.redhat.com/rh-osbs/openshift-ose-ansible-rhel9-operator:v" + ocpversion
		containerCLI := container.NewPodmanCLI()
		e2e.Logf("create container with image %s", imageTag)
		id, err := containerCLI.ContainerCreate(imageTag, "test-34945", "/bin/sh", true)
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

		commandStr := []string{"ansible-operator", "run", "--help"}
		output, err := containerCLI.Exec(id, commandStr)
		if err != nil {
			e2e.Failf("command %s: %s", commandStr, output)
		}
		o.Expect(output).To(o.ContainSubstring("--metrics-bind-address"))

	})

	// author: jitli@redhat.com
	g.It("Author:jitli-VMonly-Medium-77166-Check the ansible-operator-plugins version info", func() {
		if os.Getenv("HTTP_PROXY") != "" || os.Getenv("http_proxy") != "" {
			g.Skip("HTTP_PROXY is not empty - skipping test ...")
		}
		imageTag := "registry-proxy.engineering.redhat.com/rh-osbs/openshift-ose-ansible-rhel9-operator:v" + ocpversion
		containerCLI := container.NewPodmanCLI()
		e2e.Logf("create container with image %s", imageTag)
		id, err := containerCLI.ContainerCreate(imageTag, "test-77166", "/bin/sh", true)
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

		commandStr := []string{"ansible-operator", "version"}
		output, err := containerCLI.Exec(id, commandStr)
		if err != nil {
			e2e.Failf("command %s: %s", commandStr, output)
		}
		o.Expect(output).To(o.ContainSubstring("v1.36."))

	})

	// author: chuo@redhat.com
	g.It("Author:chuo-ConnectedOnly-VMonly-Author:chuo-High-34427-Ensure that Ansible Based Operators creation is working", func() {
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

		dockerFile := filepath.Join(tmpPath, "Dockerfile")
		if !upstream {
			exutil.By("step: OCP-52625 operatorsdk generate operator base image match the release version.")
			content := getContent(dockerFile)
			o.Expect(content).To(o.ContainSubstring("registry.redhat.io/openshift4/ose-ansible-rhel9-operator:v" + ocpversion))
			replaceContent(dockerFile, "registry.redhat.io/openshift4/ose-ansible-rhel9-operator:v"+ocpversion, "brew.registry.redhat.io/rh-osbs/openshift-ose-ansible-rhel9-operator:v"+ocpversion)
		} else {
			if os.Getenv("AnsiblePremergeTest") == "false" {
				replaceContent(dockerFile, "quay.io/operator-framework/ansible-operator:v"+upstreamversion, "brew.registry.redhat.io/rh-osbs/openshift-ose-ansible-rhel9-operator:v"+ocpversion)
			} else {
				replaceContent(dockerFile, "quay.io/operator-framework/ansible-operator:v"+upstreamversion, "quay.io/olmqe/ansible-operator-base:premergetest")
			}
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
	g.It("Author:chuo-ConnectedOnly-VMonly-Medium-34366-change ansible operator flags from maxWorkers using env MAXCONCURRENTRECONCILES ", func() {
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

		dockerFile := filepath.Join(tmpPath, "Dockerfile")
		if !upstream {
			exutil.By("step: modify Dockerfile.")
			content := getContent(dockerFile)
			o.Expect(content).To(o.ContainSubstring("registry.redhat.io/openshift4/ose-ansible-rhel9-operator:v" + ocpversion))
			replaceContent(dockerFile, "registry.redhat.io/openshift4/ose-ansible-rhel9-operator:v"+ocpversion, "brew.registry.redhat.io/rh-osbs/openshift-ose-ansible-rhel9-operator:v"+ocpversion)
		} else {
			if os.Getenv("AnsiblePremergeTest") == "false" {
				replaceContent(dockerFile, "quay.io/operator-framework/ansible-operator:v"+upstreamversion, "brew.registry.redhat.io/rh-osbs/openshift-ose-ansible-rhel9-operator:v"+ocpversion)
			} else {
				replaceContent(dockerFile, "quay.io/operator-framework/ansible-operator:v"+upstreamversion, "quay.io/olmqe/ansible-operator-base:premergetest")
			}
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

	// author: chuo@redhat.com
	g.It("Author:chuo-ConnectedOnly-VMonly-High-34426-Critical-52625-Medium-37142-ensure that Helm Based Operators creation and cr create delete ", func() {
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

		dockerFile := filepath.Join(tmpPath, "Dockerfile")
		if !upstream {
			exutil.By("step: OCP-52625 operatorsdk generate operator base image match the release version	.")
			content := getContent(dockerFile)
			o.Expect(content).To(o.ContainSubstring("registry.redhat.io/openshift4/ose-helm-rhel9-operator:v" + ocpversion))
			replaceContent(dockerFile, "registry.redhat.io/openshift4/ose-helm-rhel9-operator:v"+ocpversion, "brew.registry.redhat.io/rh-osbs/openshift-ose-helm-operator-rhel9:v"+ocpversion)
		} else {
			if os.Getenv("HelmPremergeTest") == "false" {
				replaceContent(dockerFile, "quay.io/operator-framework/helm-operator:v"+upstreamversion, "brew.registry.redhat.io/rh-osbs/openshift-ose-helm-rhel9-operator:v"+ocpversion)
			} else {
				replaceContent(dockerFile, "quay.io/operator-framework/helm-operator:v"+upstreamversion, "quay.io/olmqe/helm-operator-base:premergetest")
			}
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

	// author: jitli@redhat.com
	g.It("Author:jitli-VMonly-ConnectedOnly-High-42028-Check python kubernetes package", func() {
		if os.Getenv("HTTP_PROXY") != "" || os.Getenv("http_proxy") != "" {
			g.Skip("HTTP_PROXY is not empty - skipping test ...")
		}
		imageTag := "registry-proxy.engineering.redhat.com/rh-osbs/openshift-ose-ansible-rhel9-operator:v" + ocpversion
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
		o.Expect(output).To(o.ContainSubstring("30.1."))

		e2e.Logf("OCP 42028 SUCCESS")
	})

	// author: chuo@redhat.com
	g.It("Author:chuo-VMonly-ConnectedOnly-High-40341-Ansible operator needs a way to pass vars as unsafe ", func() {
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

		dockerFile := filepath.Join(tmpPath, "Dockerfile")
		if !upstream {
			exutil.By("step: modify Dockerfile.")
			content := getContent(dockerFile)
			o.Expect(content).To(o.ContainSubstring("registry.redhat.io/openshift4/ose-ansible-rhel9-operator:v" + ocpversion))
			replaceContent(dockerFile, "registry.redhat.io/openshift4/ose-ansible-rhel9-operator:v"+ocpversion, "brew.registry.redhat.io/rh-osbs/openshift-ose-ansible-rhel9-operator:v"+ocpversion)
		} else {
			if os.Getenv("AnsiblePremergeTest") == "false" {
				replaceContent(dockerFile, "quay.io/operator-framework/ansible-operator:v"+upstreamversion, "brew.registry.redhat.io/rh-osbs/openshift-ose-ansible-rhel9-operator:v"+ocpversion)
			} else {
				replaceContent(dockerFile, "quay.io/operator-framework/ansible-operator:v"+upstreamversion, "quay.io/olmqe/ansible-operator-base:premergetest")
			}
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
	g.It("Author:jfan-VMonly-ConnectedOnly-High-44550-SDK support ansible type operator for http_proxy env", func() {
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
		if os.Getenv("AnsiblePremergeTest") == "false" {
			replaceContent(dockerFile, "quay.io/operator-framework/ansible-operator:v"+upstreamversion, "brew.registry.redhat.io/rh-osbs/openshift-ose-ansible-rhel9-operator:v"+ocpversion)
		} else {
			replaceContent(dockerFile, "quay.io/operator-framework/ansible-operator:v"+upstreamversion, "quay.io/olmqe/ansible-operator-base:premergetest")
		}
		replaceContent(dockerFile, "install -r ${HOME}/requirements.yml", "install -r ${HOME}/requirements.yml --force")

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
					if strings.Contains(line, "1/1") {
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
	g.It("Author:jfan-VMonly-ConnectedOnly-High-44551-SDK support helm type operator for http_proxy env", func() {
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
		if os.Getenv("HelmPremergeTest") == "false" {
			replaceContent(dockerFile, "quay.io/operator-framework/helm-operator:v"+upstreamversion, "brew.registry.redhat.io/rh-osbs/openshift-ose-helm-rhel9-operator:v"+ocpversion)
		} else {
			replaceContent(dockerFile, "quay.io/operator-framework/helm-operator:v"+upstreamversion, "quay.io/olmqe/helm-operator-base:premergetest")
		}
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
					if strings.Contains(line, "1/1") {
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
	g.It("Author:jitli-VMonly-DisconnectedOnly-High-52571-Disconnected test for ansible type operator", func() {
		exutil.SkipOnProxyCluster(oc)
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
		if os.Getenv("AnsiblePremergeTest") == "false" {
			replaceContent(filepath.Join(tmpPath, "Dockerfile"), "quay.io/operator-framework/ansible-operator:v"+upstreamversion, "brew.registry.redhat.io/rh-osbs/openshift-ose-ansible-rhel9-operator:v"+ocpversion)
		} else {
			replaceContent(filepath.Join(tmpPath, "Dockerfile"), "quay.io/operator-framework/ansible-operator:v"+upstreamversion, "quay.io/olmqe/ansible-operator-base:premergetest")
		}
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
		if !strings.Contains(content, "quay.io/olmqe/memcached@sha256:") || !strings.Contains(content, "quay.io/olmqe/memcached-operator@sha256:") {
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
	g.It("Author:jitli-VMonly-DisconnectedOnly-High-52572-Disconnected test for helm type operator", func() {
		exutil.SkipOnProxyCluster(oc)
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
		if os.Getenv("HelmPremergeTest") == "false" {
			replaceContent(dockerFile, "quay.io/operator-framework/helm-operator:v"+upstreamversion, "brew.registry.redhat.io/rh-osbs/openshift-ose-helm-rhel9-operator:v"+ocpversion)
		} else {
			replaceContent(dockerFile, "quay.io/operator-framework/helm-operator:v"+upstreamversion, "quay.io/olmqe/helm-operator-base:premergetest")
		}
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
		if !strings.Contains(content, "quay.io/olmqe/nginx@sha256:") || !strings.Contains(content, "quay.io/olmqe/memcached-operator@sha256:") {
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

	// author: jfan@redhat.com
	g.It("Author:jfan-VMonly-ConnectedOnly-High-45141-High-41497-High-34292-High-29374-High-28157-High-27977-ansible k8sevent k8sstatus maxConcurrentReconciles modules to a collect blacklist [Slow]", func() {
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
		if os.Getenv("AnsiblePremergeTest") == "false" {
			replaceContent(filepath.Join(tmpPath, "Dockerfile"), "brew.registry.redhat.io/rh-osbs/openshift-ose-ansible-operator:vocpversion", "brew.registry.redhat.io/rh-osbs/openshift-ose-ansible-rhel9-operator:v"+ocpversion)
		} else {
			replaceContent(filepath.Join(tmpPath, "Dockerfile"), "brew.registry.redhat.io/rh-osbs/openshift-ose-ansible-operator:vocpversion", "quay.io/olmqe/ansible-operator-base:premergetest")
		}
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
					if strings.Contains(line, "1/1") {
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
	g.It("Author:jfan-VMonly-ConnectedOnly-High-28586-ansible Content Collections Support in watches.yaml", func() {
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
		if os.Getenv("AnsiblePremergeTest") == "false" {
			replaceContent(filepath.Join(tmpPath, "Dockerfile"), "brew.registry.redhat.io/rh-osbs/openshift-ose-ansible-operator:vocpversion", "brew.registry.redhat.io/rh-osbs/openshift-ose-ansible-rhel9-operator:v"+ocpversion)
		} else {
			replaceContent(filepath.Join(tmpPath, "Dockerfile"), "brew.registry.redhat.io/rh-osbs/openshift-ose-ansible-operator:vocpversion", "quay.io/olmqe/ansible-operator-base:premergetest")
		}
		// copy the watches.yaml
		err = copy(filepath.Join(dataPath, "watches.yaml"), filepath.Join(tmpPath, "watches.yaml"))
		o.Expect(err).NotTo(o.HaveOccurred())

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
	g.It("Author:jfan-VMonly-ConnectedOnly-High-48366-add ansible prometheus metrics", func() {
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
		if os.Getenv("AnsiblePremergeTest") == "false" {
			replaceContent(dockerfileFilePath, "brew.registry.redhat.io/rh-osbs/openshift-ose-ansible-operator:vocpversion", "brew.registry.redhat.io/rh-osbs/openshift-ose-ansible-rhel9-operator:v"+ocpversion)
		} else {
			replaceContent(dockerfileFilePath, "brew.registry.redhat.io/rh-osbs/openshift-ose-ansible-operator:vocpversion", "quay.io/olmqe/ansible-operator-base:premergetest")
		}
		// copy the roles/testmetrics/tasks/main.yml
		err = copy(filepath.Join(dataPath, "main.yml"), filepath.Join(tmpPath, "roles", "testmetrics", "tasks", "main.yml"))
		o.Expect(err).NotTo(o.HaveOccurred())

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
		//Depending on the environment, the IP address may sometimes switch between ipv4 and ipv6.
		if err != nil {
			metricsMsg, err = exec.Command("bash", "-c", "oc exec deployment/ansiblemetrics-controller-manager -n "+nsOperator+" -- curl -k -H \"Authorization: Bearer "+metricsToken+"\" 'https://"+promeEp+":8443/metrics'").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
		}
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

	// author: jitli@redhat.com
	g.It("Author:jitli-ConnectedOnly-VMonly-High-69005-helm operator recoilne the different namespaces", func() {
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

		dockerFile := filepath.Join(tmpPath, "Dockerfile")
		if !upstream {
			content := getContent(dockerFile)
			o.Expect(content).To(o.ContainSubstring("registry.redhat.io/openshift4/ose-helm-rhel9-operator:v" + ocpversion))
			replaceContent(dockerFile, "registry.redhat.io/openshift4/ose-helm-rhel9-operator:v"+ocpversion, "brew.registry.redhat.io/rh-osbs/openshift-ose-helm-operator-rhel9:v"+ocpversion)
		} else {
			if os.Getenv("HelmPremergeTest") == "false" {
				replaceContent(dockerFile, "quay.io/operator-framework/helm-operator:v"+upstreamversion, "brew.registry.redhat.io/rh-osbs/openshift-ose-helm-rhel9-operator:v"+ocpversion)
			} else {
				replaceContent(dockerFile, "quay.io/operator-framework/helm-operator:v"+upstreamversion, "quay.io/olmqe/helm-operator-base:premergetest")
			}
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
		o.Expect(podLogs).To(o.ContainSubstring(`"msg":"Watching namespaces","namespaces":["default","nginx-operator-69005-system"]`))

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
		o.Expect(podLogs).To(o.ContainSubstring(`"msg":"Watching namespaces","namespaces":["nginx-operator-69005-system"]`))
	})
})
