package opm

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	container "github.com/openshift/openshift-tests-private/test/extended/util/container"
	db "github.com/openshift/openshift-tests-private/test/extended/util/db"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-operators] OLM opm should", func() {
	defer g.GinkgoRecover()

	var opmCLI = NewOpmCLI()

	// author: scolange@redhat.com
	g.It("Author:scolange-Medium-43769-Remove opm alpha add command", func() {

		exutil.By("step: opm alpha --help")
		output1, err := opmCLI.Run("alpha").Args("--help").Output()
		e2e.Logf(output1)

		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output1).NotTo(o.ContainSubstring("add"))
		exutil.By("test case 43769 SUCCESS")

	})

	// author: kuiwang@redhat.com
	g.It("Author:kuiwang-Medium-43185-DC based opm subcommands out of alpha", func() {
		exutil.By("check init, serve, render and validate under opm")
		output, err := opmCLI.Run("").Args("--help").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf(output)
		o.Expect(output).To(o.ContainSubstring("init "))
		o.Expect(output).To(o.ContainSubstring("serve "))
		o.Expect(output).To(o.ContainSubstring("render "))
		o.Expect(output).To(o.ContainSubstring("validate "))

		exutil.By("check init, serve, render and validate not under opm alpha")
		output, err = opmCLI.Run("alpha").Args("--help").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf(output)
		o.Expect(output).NotTo(o.ContainSubstring("init "))
		o.Expect(output).NotTo(o.ContainSubstring("serve "))
		o.Expect(output).NotTo(o.ContainSubstring("render "))
		o.Expect(output).NotTo(o.ContainSubstring("validate "))
	})

	// author: kuiwang@redhat.com
	g.It("ConnectedOnly-Author:kuiwang-Medium-43171-opm render blob from bundle, db based index, dc based index, db file and directory", func() {
		exutil.By("render db-based index image")
		output, err := opmCLI.Run("render").Args("quay.io/olmqe/olm-index:OLM-2199").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("\"name\": \"cockroachdb\""))
		o.Expect(output).To(o.ContainSubstring("\"name\": \"cockroachdb.v5.0.3\""))
		o.Expect(output).To(o.ContainSubstring("\"image\": \"quay.io/olmqe/cockroachdb-operator:5.0.3-2199\""))
		o.Expect(output).To(o.ContainSubstring("quay.io/helmoperators/cockroachdb:v5.0.3"))
		o.Expect(output).To(o.ContainSubstring("\"name\": \"cockroachdb.v5.0.4\""))
		o.Expect(output).To(o.ContainSubstring("\"replaces\": \"cockroachdb.v5.0.3\""))
		o.Expect(output).To(o.ContainSubstring("quay.io/helmoperators/cockroachdb:v5.0.4"))
		o.Expect(output).To(o.ContainSubstring("\"name\": \"windup-operator.0.0.4\""))
		o.Expect(output).To(o.ContainSubstring("quay.io/windupeng/windup-operator-native:0.0.4"))
		o.Expect(output).To(o.ContainSubstring("\"name\": \"windup-operator.0.0.5\""))
		o.Expect(output).To(o.ContainSubstring("quay.io/windupeng/windup-operator-native:0.0.5"))

		exutil.By("render dc-based index image with one file")
		output, err = opmCLI.Run("render").Args("quay.io/olmqe/olm-index:OLM-2199-DC-example").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("\"name\": \"cockroachdb\""))
		o.Expect(output).To(o.ContainSubstring("\"name\": \"cockroachdb.v5.0.3\""))
		o.Expect(output).To(o.ContainSubstring("\"image\": \"quay.io/olmqe/cockroachdb-operator:5.0.3-2199\""))
		o.Expect(output).To(o.ContainSubstring("quay.io/helmoperators/cockroachdb:v5.0.3"))
		o.Expect(output).To(o.ContainSubstring("\"name\": \"cockroachdb.v5.0.4\""))
		o.Expect(output).To(o.ContainSubstring("\"replaces\": \"cockroachdb.v5.0.3\""))
		o.Expect(output).To(o.ContainSubstring("quay.io/helmoperators/cockroachdb:v5.0.4"))
		o.Expect(output).To(o.ContainSubstring("\"name\": \"windup-operator.0.0.4\""))
		o.Expect(output).To(o.ContainSubstring("quay.io/windupeng/windup-operator-native:0.0.4"))
		o.Expect(output).To(o.ContainSubstring("\"name\": \"windup-operator.0.0.5\""))
		o.Expect(output).To(o.ContainSubstring("quay.io/windupeng/windup-operator-native:0.0.5"))

		exutil.By("render dc-based index image with different files")
		output, err = opmCLI.Run("render").Args("quay.io/olmqe/olm-index:OLM-2199-DC-example-Df").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("\"name\": \"cockroachdb\""))
		o.Expect(output).To(o.ContainSubstring("\"name\": \"cockroachdb.v5.0.3\""))
		o.Expect(output).To(o.ContainSubstring("\"image\": \"quay.io/olmqe/cockroachdb-operator:5.0.3-2199\""))
		o.Expect(output).To(o.ContainSubstring("quay.io/helmoperators/cockroachdb:v5.0.3"))
		o.Expect(output).To(o.ContainSubstring("\"name\": \"cockroachdb.v5.0.4\""))
		o.Expect(output).To(o.ContainSubstring("\"replaces\": \"cockroachdb.v5.0.3\""))
		o.Expect(output).To(o.ContainSubstring("quay.io/helmoperators/cockroachdb:v5.0.4"))
		o.Expect(output).To(o.ContainSubstring("\"name\": \"windup-operator.0.0.4\""))
		o.Expect(output).To(o.ContainSubstring("quay.io/windupeng/windup-operator-native:0.0.4"))
		o.Expect(output).To(o.ContainSubstring("\"name\": \"windup-operator.0.0.5\""))
		o.Expect(output).To(o.ContainSubstring("quay.io/windupeng/windup-operator-native:0.0.5"))

		exutil.By("render dc-based index image with different directory")
		output, err = opmCLI.Run("render").Args("quay.io/olmqe/olm-index:OLM-2199-DC-example-Dd").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("\"name\": \"cockroachdb\""))
		o.Expect(output).To(o.ContainSubstring("\"name\": \"cockroachdb.v5.0.3\""))
		o.Expect(output).To(o.ContainSubstring("\"image\": \"quay.io/olmqe/cockroachdb-operator:5.0.3-2199\""))
		o.Expect(output).To(o.ContainSubstring("quay.io/helmoperators/cockroachdb:v5.0.3"))
		o.Expect(output).To(o.ContainSubstring("\"name\": \"cockroachdb.v5.0.4\""))
		o.Expect(output).To(o.ContainSubstring("\"replaces\": \"cockroachdb.v5.0.3\""))
		o.Expect(output).To(o.ContainSubstring("quay.io/helmoperators/cockroachdb:v5.0.4"))
		o.Expect(output).To(o.ContainSubstring("\"name\": \"windup-operator.0.0.4\""))
		o.Expect(output).To(o.ContainSubstring("quay.io/windupeng/windup-operator-native:0.0.4"))
		o.Expect(output).To(o.ContainSubstring("\"name\": \"windup-operator.0.0.5\""))
		o.Expect(output).To(o.ContainSubstring("quay.io/windupeng/windup-operator-native:0.0.5"))

		exutil.By("render bundle image")
		output, err = opmCLI.Run("render").Args("quay.io/olmqe/cockroachdb-operator:5.0.4-2199", "quay.io/olmqe/cockroachdb-operator:5.0.3-2199").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).NotTo(o.ContainSubstring("\"name\": \"cockroachdb\""))
		o.Expect(output).To(o.ContainSubstring("\"name\": \"cockroachdb.v5.0.4\""))
		o.Expect(output).To(o.ContainSubstring("\"name\": \"cockroachdb.v5.0.3\""))
		o.Expect(output).To(o.ContainSubstring("\"package\": \"cockroachdb\""))
		o.Expect(output).To(o.ContainSubstring("quay.io/helmoperators/cockroachdb:v5.0.4"))
		o.Expect(output).To(o.ContainSubstring("quay.io/helmoperators/cockroachdb:v5.0.3"))
		o.Expect(output).To(o.ContainSubstring("\"group\": \"charts.operatorhub.io\""))
		o.Expect(output).To(o.ContainSubstring("\"version\": \"5.0.4\""))
		o.Expect(output).To(o.ContainSubstring("\"version\": \"5.0.3\""))

		exutil.By("render directory")
		opmBaseDir := exutil.FixturePath("testdata", "opm")
		configDir := filepath.Join(opmBaseDir, "render", "configs")
		output, err = opmCLI.Run("render").Args(configDir).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("\"name\": \"cockroachdb\""))
		o.Expect(output).To(o.ContainSubstring("\"name\": \"cockroachdb.v5.0.3\""))
		o.Expect(output).To(o.ContainSubstring("\"image\": \"quay.io/olmqe/cockroachdb-operator:5.0.3-2199\""))
		o.Expect(output).To(o.ContainSubstring("quay.io/helmoperators/cockroachdb:v5.0.3"))
		o.Expect(output).To(o.ContainSubstring("\"name\": \"cockroachdb.v5.0.4\""))
		o.Expect(output).To(o.ContainSubstring("\"replaces\": \"cockroachdb.v5.0.3\""))
		o.Expect(output).To(o.ContainSubstring("quay.io/helmoperators/cockroachdb:v5.0.4"))
		o.Expect(output).To(o.ContainSubstring("\"name\": \"windup-operator.0.0.4\""))
		o.Expect(output).To(o.ContainSubstring("quay.io/windupeng/windup-operator-native:0.0.4"))
		o.Expect(output).To(o.ContainSubstring("\"name\": \"windup-operator.0.0.5\""))
		o.Expect(output).To(o.ContainSubstring("quay.io/windupeng/windup-operator-native:0.0.5"))

	})

	// author: kuiwang@redhat.com
	g.It("Author:kuiwang-Medium-43180-opm init dc configuration package", func() {
		exutil.By("init package")
		opmBaseDir := exutil.FixturePath("testdata", "opm")
		readme := filepath.Join(opmBaseDir, "render", "init", "readme.md")
		testpng := filepath.Join(opmBaseDir, "render", "init", "test.png")

		output, err := opmCLI.Run("init").Args("--default-channel=alpha", "-d", readme, "-i", testpng, "mta-operator").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf(output)
		o.Expect(output).To(o.ContainSubstring("\"schema\": \"olm.package\""))
		o.Expect(output).To(o.ContainSubstring("\"name\": \"mta-operator\""))
		o.Expect(output).To(o.ContainSubstring("\"defaultChannel\": \"alpha\""))
		o.Expect(output).To(o.ContainSubstring("zcfHkVw9GfpbJmeev9F08WW8uDkaslwX6avlWGU6N"))
		o.Expect(output).To(o.ContainSubstring("\"description\": \"it is testing\""))

	})

	// author: kuiwang@redhat.com
	g.It("Author:kuiwang-Medium-43248-Support ignoring files when loading declarative configs", func() {

		opmBaseDir := exutil.FixturePath("testdata", "opm")
		correctIndex := path.Join(opmBaseDir, "render", "validate", "configs")
		wrongIndex := path.Join(opmBaseDir, "render", "validate", "configs-wrong")
		wrongIgnoreIndex := path.Join(opmBaseDir, "render", "validate", "configs-wrong-ignore")

		exutil.By("validate correct index")
		output, err := opmCLI.Run("validate").Args(correctIndex).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf(output)

		exutil.By("validate wrong index")
		output, err = opmCLI.Run("validate").Args(wrongIndex).Output()
		o.Expect(err).To(o.HaveOccurred())
		e2e.Logf(output)

		exutil.By("validate index with ignore wrong json")
		output, err = opmCLI.Run("validate").Args(wrongIgnoreIndex).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf(output)

	})

	// author: jitli@redhat.com
	g.It("Author:jitli-Medium-43768-Improve formatting of opm alpha validate", func() {

		opmBase := exutil.FixturePath("testdata", "opm")
		catalogdir := path.Join(opmBase, "render", "validate", "catalog")
		catalogerrdir := path.Join(opmBase, "render", "validate", "catalog-error")

		exutil.By("step: opm validate -h")
		output1, err := opmCLI.Run("validate").Args("--help").Output()
		e2e.Logf(output1)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output1).To(o.ContainSubstring("opm validate "))

		exutil.By("opm validate catalog")
		output, err := opmCLI.Run("validate").Args(catalogdir).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.BeEmpty())

		exutil.By("opm validate catalog-error")
		output, err = opmCLI.Run("validate").Args(catalogerrdir).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("invalid package \\\"operator-1\\\""))
		o.Expect(output).To(o.ContainSubstring("invalid channel \\\"alpha\\\""))
		o.Expect(output).To(o.ContainSubstring("invalid bundle \\\"operator-1.v0.3.0\\\""))
		e2e.Logf(output)

	})

	// author: xzha@redhat.com
	g.It("Author:xzha-Medium-45401-opm validate should detect cycles in channels", func() {
		opmBase := exutil.FixturePath("testdata", "opm")
		catalogerrdir := path.Join(opmBase, "render", "validate", "catalog-error", "operator-1")

		exutil.By("opm validate catalog-error/operator-1")
		output, err := opmCLI.Run("validate").Args(catalogerrdir).Output()
		if err != nil {
			e2e.Logf(output)
		}
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("invalid channel \\\"45401-1\\\""))
		o.Expect(output).To(o.ContainSubstring("invalid channel \\\"45401-2\\\""))
		o.Expect(output).To(o.ContainSubstring("invalid channel \\\"45401-3\\\""))
		channelInfoList := strings.Split(output, "invalid channel")
		for _, channelInfo := range channelInfoList {
			if strings.Contains(channelInfo, "45401-1") {
				o.Expect(channelInfo).To(o.ContainSubstring("detected cycle in replaces chain of upgrade graph"))
			}
			if strings.Contains(channelInfo, "45401-2") {
				o.Expect(output).To(o.ContainSubstring("multiple channel heads found in graph"))
			}
			if strings.Contains(channelInfo, "45401-3") {
				o.Expect(output).To(o.ContainSubstring("no channel head found in graph"))
			}
		}
		exutil.By("45401 SUCCESS")
	})

	// author: xzha@redhat.com
	g.It("ConnectedOnly-Author:xzha-Medium-45402-opm render should automatically pulling in the image(s) used in the deployments", func() {
		exutil.By("render bundle image")
		output, err := opmCLI.Run("render").Args("quay.io/olmqe/mta-operator:v0.0.4-45402", "quay.io/olmqe/eclipse-che:7.32.2-45402", "-oyaml").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("---"))
		bundleConfigBlobs := strings.Split(output, "---")
		for _, bundleConfigBlob := range bundleConfigBlobs {
			if strings.Contains(bundleConfigBlob, "packageName: mta-operator") {
				exutil.By("check putput of render bundle image which has no relatedimages defined in csv")
				o.Expect(bundleConfigBlob).To(o.ContainSubstring("relatedImages"))
				relatedImages := strings.Split(bundleConfigBlob, "relatedImages")[1]
				o.Expect(relatedImages).To(o.ContainSubstring("quay.io/olmqe/mta-operator:v0.0.4-45402"))
				o.Expect(relatedImages).To(o.ContainSubstring("quay.io/windupeng/windup-operator-native:0.0.4"))
				continue
			}
			if strings.Contains(bundleConfigBlob, "packageName: eclipse-che") {
				exutil.By("check putput of render bundle image which has relatedimages defined in csv")
				o.Expect(bundleConfigBlob).To(o.ContainSubstring("relatedImages"))
				relatedImages := strings.Split(bundleConfigBlob, "relatedImages")[1]
				o.Expect(relatedImages).To(o.ContainSubstring("index.docker.io/codercom/code-server"))
				o.Expect(relatedImages).To(o.ContainSubstring("quay.io/olmqe/eclipse-che:7.32.2-45402"))
			}
		}
	})

	// author: xzha@redhat.com
	g.It("ConnectedOnly-Author:xzha-Medium-48438-opm render should support olm.constraint which is defined in dependencies", func() {
		exutil.By("render bundle image")
		output, err := opmCLI.Run("render").Args("quay.io/olmqe/etcd-bundle:v0.9.2-48438", "-oyaml").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("check output of render bundle image contain olm.constraint which is defined in dependencies.yaml")
		if !strings.Contains(output, "olm.constraint") {
			e2e.Failf("output doesn't contain olm.constraint")
		}

		//exutil.By("check output of render bundle image contain olm.csv.metadata")
		//if !strings.Contains(output, "olm.csv.metadata") {
		//	e2e.Failf("output doesn't contain olm.csv.metadata")
		//}
	})

	// author: xzha@redhat.com
	g.It("ConnectedOnly-VMonly-Author:xzha-High-30189-OPM can pull and unpack bundle images in a container", func() {
		imageTag := "quay.io/openshift/origin-operator-registry"
		containerCLI := container.NewPodmanCLI()
		containerName := "test-30189-" + getRandomString()
		e2e.Logf("create container with image %s", imageTag)
		id, err := containerCLI.ContainerCreate(imageTag, containerName, "/bin/sh", true)
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

		e2e.Logf("get grpcurl")
		_, err = containerCLI.Exec(id, []string{"wget", "https://github.com/fullstorydev/grpcurl/releases/download/v1.6.0/grpcurl_1.6.0_linux_x86_64.tar.gz"})
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = containerCLI.Exec(id, []string{"tar", "xzf", "grpcurl_1.6.0_linux_x86_64.tar.gz"})
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = containerCLI.Exec(id, []string{"chmod", "a+rx", "grpcurl"})
		o.Expect(err).NotTo(o.HaveOccurred())

		opmPath, err := exec.LookPath("opm")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(opmPath).NotTo(o.BeEmpty())
		err = containerCLI.CopyFile(id, opmPath, "/tmp/opm")
		o.Expect(err).NotTo(o.HaveOccurred())
		commandStr := []string{"/tmp/opm", "version"}
		e2e.Logf("run command %s", commandStr)
		output, err := containerCLI.Exec(id, commandStr)
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("opm version is: %s", output)

		commandStr = []string{"/tmp/opm", "index", "add", "--bundles", "quay.io/olmqe/ditto-operator:0.1.0", "--from-index", "quay.io/olmqe/etcd-index:30189", "--generate"}
		e2e.Logf("run command %s", commandStr)
		output, err = containerCLI.Exec(id, commandStr)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Pulling previous image"))
		o.Expect(output).To(o.ContainSubstring("writing dockerfile: index.Dockerfile"))

		commandStr = []string{"ls"}
		e2e.Logf("run command %s", commandStr)
		output, err = containerCLI.Exec(id, commandStr)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("database"))
		o.Expect(output).To(o.ContainSubstring("index.Dockerfile"))

		commandStr = []string{"/tmp/opm", "index", "export", "-i", "quay.io/olmqe/etcd-index:0.9.0-30189", "-f", "tmp", "-o", "etcd"}
		e2e.Logf("run command %s", commandStr)
		output, err = containerCLI.Exec(id, commandStr)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Pulling previous image"))
		o.Expect(output).To(o.ContainSubstring("Preparing to pull bundles map"))

		commandStr = []string{"mv", "tmp/etcd", "."}
		e2e.Logf("run command %s", commandStr)
		_, err = containerCLI.Exec(id, commandStr)
		o.Expect(err).NotTo(o.HaveOccurred())

		commandStr = []string{"ls", "-R", "etcd"}
		e2e.Logf("run command %s", commandStr)
		output, err = containerCLI.Exec(id, commandStr)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("etcdoperator.v0.9.0.clusterserviceversion.yaml"))

		commandStr = []string{"mkdir", "test"}
		e2e.Logf("run command %s", commandStr)
		_, err = containerCLI.Exec(id, commandStr)
		o.Expect(err).NotTo(o.HaveOccurred())

		commandStr = []string{"/tmp/opm", "alpha", "bundle", "generate", "--directory", "etcd", "--package", "test-operator", "--channels", "stable,beta", "-u", "test"}
		e2e.Logf("run command %s", commandStr)
		output, err = containerCLI.Exec(id, commandStr)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Writing bundle.Dockerfile"))

		commandStr = []string{"ls", "-R", "test"}
		e2e.Logf("run command %s", commandStr)
		output, err = containerCLI.Exec(id, commandStr)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("annotations.yaml"))
		o.Expect(output).To(o.ContainSubstring("etcdoperator.v0.9.0.clusterserviceversion.yaml"))

		commandStr = []string{"initializer", "-m", "test"}
		e2e.Logf("run command %s", commandStr)
		output, err = containerCLI.Exec(id, commandStr)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("loading Packages and Entries"))

		commandStr = []string{"/tmp/opm", "registry", "serve", "-p", "50050"}
		e2e.Logf("run command %s", commandStr)
		_, err = containerCLI.ExecBackgroud(id, commandStr)
		o.Expect(err).NotTo(o.HaveOccurred())

		commandGRP := "podman exec " + id + " ./grpcurl -plaintext localhost:50050 api.Registry/ListBundles | jq '{csvName}'"
		outputGRP, err := exec.Command("bash", "-c", commandGRP).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(outputGRP).NotTo(o.ContainSubstring("etcdoperator.v0.9.2"))
		o.Expect(outputGRP).To(o.ContainSubstring("etcdoperator.v0.9.0"))

		commandStr = []string{"/tmp/opm", "registry", "add", "-b", "quay.io/olmqe/etcd-bundle:0.9.2"}
		e2e.Logf("run command %s", commandStr)
		output, err = containerCLI.Exec(id, commandStr)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("adding to the registry"))

		commandStr = []string{"/tmp/opm", "registry", "serve", "-p", "50051"}
		e2e.Logf("run command %s", commandStr)
		_, err = containerCLI.ExecBackgroud(id, commandStr)
		o.Expect(err).NotTo(o.HaveOccurred())

		commandGRP = "podman exec " + id + " ./grpcurl -plaintext localhost:50051 api.Registry/ListBundles | jq '{csvName}'"
		outputGRP, err = exec.Command("bash", "-c", commandGRP).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(outputGRP).To(o.ContainSubstring("etcdoperator.v0.9.2"))
		o.Expect(outputGRP).To(o.ContainSubstring("etcdoperator.v0.9.0"))

		commandStr = []string{"/tmp/opm", "registry", "rm", "-o", "etcd"}
		e2e.Logf("run command %s", commandStr)
		output, err = containerCLI.Exec(id, commandStr)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("removing from the registry"))

		commandStr = []string{"/tmp/opm", "registry", "serve", "-p", "50052"}
		e2e.Logf("run command %s", commandStr)
		_, err = containerCLI.ExecBackgroud(id, commandStr)
		o.Expect(err).NotTo(o.HaveOccurred())

		commandGRP = "podman exec " + id + " ./grpcurl -plaintext localhost:50052 api.Registry/ListBundles | jq '{csvName}'"
		outputGRP, err = exec.Command("bash", "-c", commandGRP).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(outputGRP).NotTo(o.ContainSubstring("etcd"))

		e2e.Logf("OCP 30189 SUCCESS")
	})

	// author: xzha@redhat.com
	g.It("ConnectedOnly-VMonly-Author:xzha-Medium-47335-opm should validate the constraint type for bundle", func() {
		opmBaseDir := exutil.FixturePath("testdata", "opm")
		tmpPath := filepath.Join(opmBaseDir, "temp"+getRandomString())
		defer DeleteDir(tmpPath, "fixture-testdata")
		exutil.By("step: mkdir with mode 0755")
		err := os.MkdirAll(tmpPath, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		opmCLI.ExecCommandPath = tmpPath

		exutil.By("opm validate quay.io/olmqe/etcd-bundle:v0.9.2-47335-1")
		output, err := opmCLI.Run("alpha").Args("bundle", "validate", "-t", "quay.io/olmqe/etcd-bundle:v0.9.2-47335-1", "-b", "podman").Output()
		if err != nil {
			e2e.Logf(output)
		}
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("opm validate quay.io/olmqe/etcd-bundle:v0.9.2-47335-2")
		output, err = opmCLI.Run("alpha").Args("bundle", "validate", "-t", "quay.io/olmqe/etcd-bundle:v0.9.2-47335-2", "-b", "podman").Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(string(output)).To(o.ContainSubstring("Bundle validation errors: Invalid CEL expression: ERROR"))
		o.Expect(string(output)).To(o.ContainSubstring("Syntax error: missing"))

		exutil.By("opm validate quay.io/olmqe/etcd-bundle:v0.9.2-47335-3")
		output, err = opmCLI.Run("alpha").Args("bundle", "validate", "-t", "quay.io/olmqe/etcd-bundle:v0.9.2-47335-3", "-b", "podman").Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(string(output)).To(o.ContainSubstring("Bundle validation errors: The CEL expression is missing"))

		exutil.By("opm validate quay.io/olmqe/etcd-bundle:v0.9.2-47335-4")
		output, err = opmCLI.Run("alpha").Args("bundle", "validate", "-t", "quay.io/olmqe/etcd-bundle:v0.9.2-47335-4", "-b", "podman").Output()
		if err != nil {
			e2e.Logf(output)
		}
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("opm validate quay.io/olmqe/etcd-bundle:v0.9.2-47335-5")
		output, err = opmCLI.Run("alpha").Args("bundle", "validate", "-t", "quay.io/olmqe/etcd-bundle:v0.9.2-47335-5", "-b", "podman").Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(string(output)).To(o.ContainSubstring("Bundle validation errors: Invalid CEL expression: ERROR"))
		o.Expect(string(output)).To(o.ContainSubstring("undeclared reference to 'semver_compares'"))

		exutil.By("opm validate quay.io/olmqe/etcd-bundle:v0.9.2-47335-6")
		output, err = opmCLI.Run("alpha").Args("bundle", "validate", "-t", "quay.io/olmqe/etcd-bundle:v0.9.2-47335-6", "-b", "podman").Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(string(output)).To(o.ContainSubstring("Bundle validation errors: Invalid CEL expression: cel expressions must have type Bool"))

		exutil.By("47335 SUCCESS")
	})

	// author: xzha@redhat.com
	g.It("ConnectedOnly-Author:xzha-Medium-70013-opm support deprecated channel", func() {
		opmBaseDir := exutil.FixturePath("testdata", "opm", "70013")
		opmCLI.ExecCommandPath = opmBaseDir

		exutil.By("opm validate catalog")
		output, err := opmCLI.Run("validate").Args("catalog-valid").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.BeEmpty())

		exutil.By("opm validate catalog")
		output, err = opmCLI.Run("validate").Args("catalog-invalid").Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(string(output)).To(o.ContainSubstring("message must be set"))

		exutil.By("opm render")
		output, err = opmCLI.Run("render").Args("quay.io/olmqe/olmtest-operator-index:nginx70050", "-o", "yaml").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(string(output), "schema: olm.deprecations")).To(o.BeTrue())

		exutil.By("70013 SUCCESS")
	})

	// author: bandrade@redhat.com
	g.It("Author:bandrade-Medium-34016-opm can prune operators from catalog", func() {
		opmBaseDir := exutil.FixturePath("testdata", "opm")
		indexDB := filepath.Join(opmBaseDir, "index_34016.db")
		output, err := opmCLI.Run("registry").Args("prune", "-d", indexDB, "-p", "lib-bucket-provisioner").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(output, "deleting packages") || !strings.Contains(output, "pkg=planetscale") {
			e2e.Failf(fmt.Sprintf("Failed to obtain the removed packages from prune : %s", output))
		}
	})

	// author: bandrade@redhat.com
	g.It("ConnectedOnly-Author:bandrade-Medium-54168-opm support '--use-http' global flag", func() {
		if os.Getenv("HTTP_PROXY") != "" || os.Getenv("http_proxy") != "" {
			g.Skip("HTTP_PROXY is not empty - skipping test ...")
		}
		opmBaseDir := exutil.FixturePath("testdata", "opm", "53869")
		opmCLI.ExecCommandPath = opmBaseDir
		defer DeleteDir(opmBaseDir, "fixture-testdata")

		exutil.By("1) checking alpha list")
		output, err := opmCLI.Run("alpha").Args("list", "bundles", "quay.io/openshifttest/nginxolm-operator-index:v1", "--use-http").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(output, "nginx-operator") {
			e2e.Failf(fmt.Sprintf("Failed to obtain the packages from alpha list : %s", output))
		}

		exutil.By("2) checking render")
		output, err = opmCLI.Run("render").Args("quay.io/openshifttest/nginxolm-operator-index:v1", "--use-http").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(output, "nginx-operator") {
			e2e.Failf(fmt.Sprintf("Failed run render command : %s", output))
		}

		exutil.By("3) checking index add")
		output, err = opmCLI.Run("index").Args("add", "-b", "quay.io/openshifttest/nginxolm-operator-bundle:v0.0.1", "-t", "quay.io/olmqe/nginxolm-operator-index:v54168", "--use-http", "--generate").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(output, "writing dockerfile") {
			e2e.Failf(fmt.Sprintf("Failed run render command : %s", output))
		}

		exutil.By("4) checking render-veneer semver")
		output, err = opmCLI.Run("alpha").Args("render-template", "--use-http", "basic", filepath.Join(opmBaseDir, "catalog-basic-template.yaml"), "-o", "yaml").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(output, "nginx-operator") {
			e2e.Failf(fmt.Sprintf("Failed run render command : %s", output))
		}

		exutil.By("5) checking render-graph")
		output, err = opmCLI.Run("alpha").Args("render-graph", "quay.io/openshifttest/nginxolm-operator-index:v1", "--use-http").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(output, "nginx-operator") {
			e2e.Failf(fmt.Sprintf("Failed run render-graph command : %s", output))
		}
	})

	// author: bandrade@redhat.com
	g.It("Author:bandrade-VMonly-Low-30318-Bundle build understands packages", func() {
		opmBaseDir := exutil.FixturePath("testdata", "opm")
		testDataPath := filepath.Join(opmBaseDir, "learn_operator")
		opmCLI.ExecCommandPath = testDataPath
		defer DeleteDir(testDataPath, "fixture-testdata")

		exutil.By("step: opm alpha bundle generate")
		output, err := opmCLI.Run("alpha").Args("bundle", "generate", "-d", "package/0.0.1", "-p", "25955-operator", "-c", "alpha", "-e", "alpha").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf(output)
		if !strings.Contains(output, "Writing annotations.yaml") || !strings.Contains(output, "Writing bundle.Dockerfile") {
			e2e.Failf("Failed to execute opm alpha bundle generate : %s", output)
		}
	})
})

var _ = g.Describe("[sig-operators] OLM opm with podman", func() {
	defer g.GinkgoRecover()

	var opmCLI = NewOpmCLI()
	var oc = exutil.NewCLI("vmonly-"+getRandomString(), exutil.KubeConfigPath())

	// OCP-43641 author: jitli@redhat.com
	g.It("Author:jitli-ConnectedOnly-VMonly-Medium-43641-opm index add fails during image extraction", func() {
		bundleImage := "quay.io/olmqe/etcd:0.9.4-43641"
		indexImage := "quay.io/olmqe/etcd-index:v1-4.8"
		opmBaseDir := exutil.FixturePath("testdata", "opm")
		TestDataPath := filepath.Join(opmBaseDir, "temp")
		opmCLI.ExecCommandPath = TestDataPath
		defer DeleteDir(TestDataPath, "fixture-testdata")
		err := os.Mkdir(TestDataPath, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("step: checking user account is no-root")
		user, err := exec.Command("bash", "-c", "whoami").Output()
		e2e.Logf("User:%s", user)
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Compare(string(user), "root") == -1 {
			exutil.By("step: opm index add")
			output1, err := opmCLI.Run("index").Args("add", "--generate", "--bundles", bundleImage, "--from-index", indexImage, "--overwrite-latest").Output()
			e2e.Logf(output1)
			o.Expect(err).NotTo(o.HaveOccurred())
			exutil.By("test case 43641 SUCCESS")
		} else {
			e2e.Logf("User is %s. the case should login as no-root account", user)
		}
	})

	// author: xzha@redhat.com
	g.It("Author:xzha-ConnectedOnly-VMonly-Medium-25955-opm Ability to generate scaffolding for Operator Bundle", func() {
		var podmanCLI = container.NewPodmanCLI()
		opmBaseDir := exutil.FixturePath("testdata", "opm")
		TestDataPath := filepath.Join(opmBaseDir, "learn_operator")
		opmCLI.ExecCommandPath = TestDataPath
		defer DeleteDir(TestDataPath, "fixture-testdata")
		imageTag := "quay.io/olmqe/25955-operator-" + getRandomString() + ":v0.0.1"

		exutil.By("step: opm alpha bundle generate")
		output, err := opmCLI.Run("alpha").Args("bundle", "generate", "-d", "package/0.0.1", "-p", "25955-operator", "-c", "alpha", "-e", "alpha").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf(output)
		if !strings.Contains(output, "Writing annotations.yaml") || !strings.Contains(output, "Writing bundle.Dockerfile") {
			e2e.Failf("Failed to execute opm alpha bundle generate : %s", output)
		}

		exutil.By("step: opm alpha bundle build")
		e2e.Logf("clean test data")
		DeleteDir(TestDataPath, "fixture-testdata")
		opmBaseDir = exutil.FixturePath("testdata", "opm")
		TestDataPath = filepath.Join(opmBaseDir, "learn_operator")
		opmCLI.ExecCommandPath = TestDataPath
		_, err = podmanCLI.RemoveImage(imageTag)
		o.Expect(err).NotTo(o.HaveOccurred())

		e2e.Logf("run opm alpha bundle build")
		defer podmanCLI.RemoveImage(imageTag)
		output, _ = opmCLI.Run("alpha").Args("bundle", "build", "-d", "package/0.0.1", "-b", "podman", "--tag", imageTag, "-p", "25955-operator", "-c", "alpha", "-e", "alpha", "--overwrite").Output()
		e2e.Logf(output)
		if !strings.Contains(output, "COMMIT "+imageTag) {
			e2e.Failf("Failed to execute opm alpha bundle build : %s", output)
		}

		e2e.Logf("step: check image %s exist", imageTag)
		existFlag, err := podmanCLI.CheckImageExist(imageTag)
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("check image exist is %v", existFlag)
		o.Expect(existFlag).To(o.BeTrue())
	})

	// author: xzha@redhat.com
	g.It("Author:xzha-ConnectedOnly-VMonly-Medium-37294-OPM can strand packages with prune stranded", func() {
		var sqlit = db.NewSqlit()
		quayCLI := container.NewQuayCLI()
		containerTool := "podman"
		containerCLI := container.NewPodmanCLI()
		opmBaseDir := exutil.FixturePath("testdata", "opm")
		TestDataPath := filepath.Join(opmBaseDir, "temp")
		opmCLI.ExecCommandPath = TestDataPath
		defer DeleteDir(TestDataPath, "fixture-testdata")
		indexImage := "quay.io/olmqe/etcd-index:test-37294"
		indexImageSemver := "quay.io/olmqe/etcd-index:test-37294-semver"

		exutil.By("step: check etcd-index:test-37294, operatorbundle has two records, channel_entry has one record")
		indexdbpath1 := filepath.Join(TestDataPath, getRandomString())
		err := os.Mkdir(TestDataPath, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = os.Mkdir(indexdbpath1, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = oc.AsAdmin().WithoutNamespace().Run("image").Args("extract", indexImage, "--path", "/database/index.db:"+indexdbpath1).Output()
		e2e.Logf("get index.db SUCCESS, path is %s", path.Join(indexdbpath1, "index.db"))
		o.Expect(err).NotTo(o.HaveOccurred())
		result, err := sqlit.DBMatch(path.Join(indexdbpath1, "index.db"), "operatorbundle", "name", []string{"etcdoperator.v0.9.0", "etcdoperator.v0.9.2"})
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(result).To(o.BeTrue())
		result, err = sqlit.DBMatch(path.Join(indexdbpath1, "index.db"), "channel_entry", "operatorbundle_name", []string{"etcdoperator.v0.9.2"})
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(result).To(o.BeTrue())

		exutil.By("step: prune-stranded this index image")
		indexImageTmp1 := indexImage + getRandomString()
		defer containerCLI.RemoveImage(indexImageTmp1)
		output, err := opmCLI.Run("index").Args("prune-stranded", "-f", indexImage, "--tag", indexImageTmp1, "-c", containerTool).Output()
		if err != nil {
			e2e.Logf(output)
		}
		o.Expect(err).NotTo(o.HaveOccurred())
		output, err = containerCLI.Run("push").Args(indexImageTmp1).Output()
		if err != nil {
			e2e.Logf(output)
		}
		defer quayCLI.DeleteTag(strings.Replace(indexImageTmp1, "quay.io/", "", 1))
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("step: check index image operatorbundle has one record")
		indexdbpath2 := filepath.Join(TestDataPath, getRandomString())
		err = os.Mkdir(indexdbpath2, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = oc.AsAdmin().WithoutNamespace().Run("image").Args("extract", indexImageTmp1, "--path", "/database/index.db:"+indexdbpath2).Output()
		e2e.Logf("get index.db SUCCESS, path is %s", path.Join(indexdbpath2, "index.db"))
		o.Expect(err).NotTo(o.HaveOccurred())
		result, err = sqlit.DBMatch(path.Join(indexdbpath2, "index.db"), "operatorbundle", "name", []string{"etcdoperator.v0.9.2"})
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(result).To(o.BeTrue())
		result, err = sqlit.DBMatch(path.Join(indexdbpath2, "index.db"), "channel_entry", "operatorbundle_name", []string{"etcdoperator.v0.9.2"})
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(result).To(o.BeTrue())

		exutil.By("test 2")
		exutil.By("step: step: check etcd-index:test-37294-semver, operatorbundle has two records, channel_entry has two records")
		indexdbpath3 := filepath.Join(TestDataPath, getRandomString())
		err = os.Mkdir(indexdbpath3, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = oc.AsAdmin().WithoutNamespace().Run("image").Args("extract", indexImageSemver, "--path", "/database/index.db:"+indexdbpath3).Output()
		e2e.Logf("get index.db SUCCESS, path is %s", path.Join(indexdbpath3, "index.db"))
		o.Expect(err).NotTo(o.HaveOccurred())
		result, err = sqlit.DBMatch(path.Join(indexdbpath3, "index.db"), "operatorbundle", "name", []string{"etcdoperator.v0.9.0", "etcdoperator.v0.9.2"})
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(result).To(o.BeTrue())
		result, err = sqlit.DBMatch(path.Join(indexdbpath3, "index.db"), "channel_entry", "operatorbundle_name", []string{"etcdoperator.v0.9.0", "etcdoperator.v0.9.2"})
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(result).To(o.BeTrue())

		exutil.By("step: prune-stranded this index image")
		indexImageTmp2 := indexImage + getRandomString()
		defer containerCLI.RemoveImage(indexImageTmp2)
		output, err = opmCLI.Run("index").Args("prune-stranded", "-f", indexImageSemver, "--tag", indexImageTmp2, "-c", containerTool).Output()
		if err != nil {
			e2e.Logf(output)
		}
		o.Expect(err).NotTo(o.HaveOccurred())
		output, err = containerCLI.Run("push").Args(indexImageTmp2).Output()
		if err != nil {
			e2e.Logf(output)
		}
		defer quayCLI.DeleteTag(strings.Replace(indexImageTmp2, "quay.io/", "", 1))
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("step: check index image has both v0.9.2 and v0.9.2")
		indexdbpath4 := filepath.Join(TestDataPath, getRandomString())
		err = os.Mkdir(indexdbpath4, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = oc.AsAdmin().WithoutNamespace().Run("image").Args("extract", indexImageTmp2, "--path", "/database/index.db:"+indexdbpath4).Output()
		e2e.Logf("get index.db SUCCESS, path is %s", path.Join(indexdbpath4, "index.db"))
		o.Expect(err).NotTo(o.HaveOccurred())
		result, err = sqlit.DBMatch(path.Join(indexdbpath4, "index.db"), "operatorbundle", "name", []string{"etcdoperator.v0.9.0", "etcdoperator.v0.9.2"})
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(result).To(o.BeTrue())
		result, err = sqlit.DBMatch(path.Join(indexdbpath4, "index.db"), "channel_entry", "operatorbundle_name", []string{"etcdoperator.v0.9.0", "etcdoperator.v0.9.2"})
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(result).To(o.BeTrue())
		e2e.Logf("step: check index image has both v0.9.2 and v0.9.2 SUCCESS")
	})

	// author: kuiwang@redhat.com
	g.It("Author:kuiwang-ConnectedOnly-VMonly-Medium-40167-bundle image is missed in index db of index image", func() {
		var (
			opmBaseDir    = exutil.FixturePath("testdata", "opm")
			TestDataPath  = filepath.Join(opmBaseDir, "temp")
			opmCLI        = NewOpmCLI()
			quayCLI       = container.NewQuayCLI()
			sqlit         = db.NewSqlit()
			containerTool = "podman"
			containerCLI  = container.NewPodmanCLI()

			// it is shared image. could not need to remove it.
			indexImage = "quay.io/olmqe/cockroachdb-index:2.1.11-40167"
			// it is generated by case. need to remove it after case exist normally or abnormally
			customIndexImage = "quay.io/olmqe/cockroachdb-index:2.1.11-40167-custome-" + getRandomString()
		)
		defer DeleteDir(TestDataPath, "fixture-testdata")
		defer containerCLI.RemoveImage(customIndexImage)
		defer quayCLI.DeleteTag(strings.Replace(customIndexImage, "quay.io/", "", 1))
		err := os.Mkdir(TestDataPath, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		opmCLI.ExecCommandPath = TestDataPath

		exutil.By("prune redhat index image to get custom index image")
		if output, err := opmCLI.Run("index").Args("prune", "-f", indexImage, "-p", "cockroachdb", "-t", customIndexImage, "-c", containerTool).Output(); err != nil {
			e2e.Logf(output)
			if strings.Contains(output, "error unmounting container") {
				g.Skip("skip case because we can not prepare data")
			}
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		if output, err := containerCLI.Run("push").Args(customIndexImage).Output(); err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		exutil.By("extract db file")
		indexdbpath1 := filepath.Join(TestDataPath, getRandomString())
		err = os.Mkdir(indexdbpath1, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = oc.AsAdmin().WithoutNamespace().Run("image").Args("extract", customIndexImage, "--path", "/database/index.db:"+indexdbpath1).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("get index.db SUCCESS, path is %s", path.Join(indexdbpath1, "index.db"))

		exutil.By("check if the bunld image is in db index")
		rows, err := sqlit.QueryDB(path.Join(indexdbpath1, "index.db"), "select image from related_image where operatorbundle_name like 'cockroachdb%';")
		o.Expect(err).NotTo(o.HaveOccurred())
		defer rows.Close()
		var imageList string
		var image string
		for rows.Next() {
			rows.Scan(&image)
			imageList = imageList + image
		}
		e2e.Logf("imageList is %v", imageList)
		o.Expect(imageList).To(o.ContainSubstring("cockroachdb-operator"))

	})

	// author: xzha@redhat.com
	g.It("Author:xzha-ConnectedOnly-VMonly-Medium-40530-The index image generated by opm index prune should not leave unrelated images", func() {
		quayCLI := container.NewQuayCLI()
		var sqlit = db.NewSqlit()
		containerCLI := container.NewPodmanCLI()
		containerTool := "podman"
		opmBaseDir := exutil.FixturePath("testdata", "opm")
		TestDataPath := filepath.Join(opmBaseDir, "temp")
		opmCLI.ExecCommandPath = TestDataPath
		defer DeleteDir(TestDataPath, "fixture-testdata")
		indexImage := "quay.io/olmqe/redhat-operator-index:40530"
		defer containerCLI.RemoveImage(indexImage)

		exutil.By("step: check the index image has other bundles except cluster-logging")
		indexTmpPath1 := filepath.Join(TestDataPath, getRandomString())
		err := os.MkdirAll(indexTmpPath1, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = oc.AsAdmin().WithoutNamespace().Run("image").Args("extract", indexImage, "--path", "/database/index.db:"+indexTmpPath1).Output()
		e2e.Logf("get index.db SUCCESS, path is %s", path.Join(indexTmpPath1, "index.db"))
		o.Expect(err).NotTo(o.HaveOccurred())

		rows, err := sqlit.QueryDB(path.Join(indexTmpPath1, "index.db"), "select distinct(operatorbundle_name) from related_image where operatorbundle_name not in (select operatorbundle_name from channel_entry)")
		o.Expect(err).NotTo(o.HaveOccurred())
		defer rows.Close()
		var OperatorBundles []string
		var name string
		for rows.Next() {
			rows.Scan(&name)
			OperatorBundles = append(OperatorBundles, name)
		}
		o.Expect(OperatorBundles).NotTo(o.BeEmpty())

		exutil.By("step: Prune the index image to keep cluster-logging only")
		indexImage1 := indexImage + getRandomString()
		defer containerCLI.RemoveImage(indexImage1)
		output, err := opmCLI.Run("index").Args("prune", "-f", indexImage, "-p", "cluster-logging", "-t", indexImage1, "-c", containerTool).Output()
		if err != nil {
			e2e.Logf(output)
		}
		o.Expect(err).NotTo(o.HaveOccurred())
		output, err = containerCLI.Run("push").Args(indexImage1).Output()
		if err != nil {
			e2e.Logf(output)
		}
		defer quayCLI.DeleteTag(strings.Replace(indexImage1, "quay.io/", "", 1))
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("step: check database, there is no related images")
		indexTmpPath2 := filepath.Join(TestDataPath, getRandomString())
		err = os.MkdirAll(indexTmpPath2, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = oc.AsAdmin().WithoutNamespace().Run("image").Args("extract", indexImage1, "--path", "/database/index.db:"+indexTmpPath2).Output()
		e2e.Logf("get index.db SUCCESS, path is %s", path.Join(indexTmpPath2, "index.db"))
		o.Expect(err).NotTo(o.HaveOccurred())

		rows2, err := sqlit.QueryDB(path.Join(indexTmpPath2, "index.db"), "select distinct(operatorbundle_name) from related_image where operatorbundle_name not in (select operatorbundle_name from channel_entry)")
		o.Expect(err).NotTo(o.HaveOccurred())
		OperatorBundles = nil
		defer rows2.Close()
		for rows2.Next() {
			rows2.Scan(&name)
			OperatorBundles = append(OperatorBundles, name)
		}
		o.Expect(OperatorBundles).To(o.BeEmpty())

		exutil.By("step: check the image mirroring mapping")
		manifestsPath := filepath.Join(TestDataPath, getRandomString())
		output, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("catalog", "mirror", indexImage1, "localhost:5000", "--manifests-only", "--to-manifests="+manifestsPath).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("/database/index.db"))

		result, err := exec.Command("bash", "-c", "cat "+manifestsPath+"/mapping.txt").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(result).NotTo(o.BeEmpty())

		result, _ = exec.Command("bash", "-c", "cat "+manifestsPath+"/mapping.txt|grep -v ose-cluster-logging|grep -v ose-logging|grep -v redhat-operator-index:40530").Output()
		o.Expect(result).To(o.BeEmpty())
		exutil.By("step: 40530 SUCCESS")

	})

	// author: bandrade@redhat.com
	g.It("Author:bandrade-ConnectedOnly-VMonly-Medium-34049-opm can prune operators from index", func() {
		var sqlit = db.NewSqlit()
		quayCLI := container.NewQuayCLI()
		podmanCLI := container.NewPodmanCLI()
		opmBaseDir := exutil.FixturePath("testdata", "opm")
		TestDataPath := filepath.Join(opmBaseDir, "temp")
		indexTmpPath := filepath.Join(TestDataPath, getRandomString())
		defer DeleteDir(TestDataPath, indexTmpPath)
		err := os.MkdirAll(indexTmpPath, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		containerCLI := container.NewPodmanCLI()
		containerTool := "podman"
		sourceImageTag := "quay.io/olmqe/multi-index:2.0"
		imageTag := "quay.io/olmqe/multi-index:3.0." + getRandomString()
		defer podmanCLI.RemoveImage(imageTag)
		defer podmanCLI.RemoveImage(sourceImageTag)
		output, err := opmCLI.Run("index").Args("prune", "-f", sourceImageTag, "-p", "planetscale", "-t", imageTag, "-c", containerTool).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(output, "deleting packages") || !strings.Contains(output, "pkg=lib-bucket-provisioner") {
			e2e.Failf(fmt.Sprintf("Failed to obtain the removed packages from prune : %s", output))
		}

		output, err = containerCLI.Run("push").Args(imageTag).Output()
		if err != nil {
			e2e.Logf(output)
		}
		defer quayCLI.DeleteTag(strings.Replace(imageTag, "quay.io/", "", 1))
		o.Expect(err).NotTo(o.HaveOccurred())

		_, err = oc.AsAdmin().WithoutNamespace().Run("image").Args("extract", imageTag, "--path", "/database/index.db:"+indexTmpPath).Output()
		e2e.Logf("get index.db SUCCESS, path is %s", path.Join(indexTmpPath, "index.db"))
		o.Expect(err).NotTo(o.HaveOccurred())

		result, err := sqlit.DBMatch(path.Join(indexTmpPath, "index.db"), "channel_entry", "operatorbundle_name", []string{"lib-bucket-provisioner.v1.0.0"})
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(result).To(o.BeFalse())

	})

	g.It("Author:xzha-ConnectedOnly-VMonly-Medium-26594-Related Images", func() {
		var sqlit = db.NewSqlit()
		quayCLI := container.NewQuayCLI()
		containerCLI := container.NewPodmanCLI()
		containerTool := "podman"
		opmBaseDir := exutil.FixturePath("testdata", "opm")
		TestDataPath := filepath.Join(opmBaseDir, "eclipse-che")
		TmpDataPath := filepath.Join(opmBaseDir, "tmp")
		err := os.MkdirAll(TmpDataPath, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		bundleImageTag := "quay.io/olmqe/eclipse-che:7.32.2-" + getRandomString()

		defer exec.Command("kill", "-9", "$(lsof -t -i:26594)").Output()
		defer DeleteDir(TestDataPath, "fixture-testdata")
		defer containerCLI.RemoveImage(bundleImageTag)
		defer quayCLI.DeleteTag(strings.Replace(bundleImageTag, "quay.io/", "", 1))

		exutil.By("step: build bundle image ")
		opmCLI.ExecCommandPath = TestDataPath
		output, err := opmCLI.Run("alpha").Args("bundle", "build", "-d", "7.32.2", "-b", containerTool, "-t", bundleImageTag, "-p", "eclipse-che", "-c", "alpha", "-e", "alpha", "--overwrite").Output()
		if err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		o.Expect(string(output)).To(o.ContainSubstring("Writing annotations.yaml"))
		o.Expect(string(output)).To(o.ContainSubstring("Writing bundle.Dockerfile"))

		if output, err = containerCLI.Run("push").Args(bundleImageTag).Output(); err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		exutil.By("step: build bundle.db")
		dbFilePath := TmpDataPath + "bundles.db"
		if output, err := opmCLI.Run("registry").Args("add", "-b", bundleImageTag, "-d", dbFilePath, "-c", containerTool, "--mode", "semver").Output(); err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		exutil.By("step: Check if the related images stores in this database")
		image := "quay.io/che-incubator/configbump@sha256:175ff2ba1bd74429de192c0a9facf39da5699c6da9f151bd461b3dc8624dd532"

		result, err := sqlit.DBMatch(dbFilePath, "package", "name", []string{"eclipse-che"})
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(result).To(o.BeTrue())
		result, err = sqlit.DBHas(dbFilePath, "related_image", "image", []string{image})
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(result).To(o.BeTrue())

		exutil.By("step: Run the opm registry server binary to load manifest and serves a grpc API to query it.")
		e2e.Logf("step: Run the registry-server ")
		cmd := exec.Command("opm", "registry", "serve", "-d", dbFilePath, "-t", filepath.Join(TmpDataPath, "26594.log"), "-p", "26594")
		cmd.Dir = TmpDataPath
		err = cmd.Start()
		o.Expect(err).NotTo(o.HaveOccurred())
		time.Sleep(time.Second * 1)
		e2e.Logf("step: check api.Registry/ListPackages")
		outputCurl, err := exec.Command("grpcurl", "-plaintext", "localhost:26594", "api.Registry/ListPackages").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(string(outputCurl)).To(o.ContainSubstring("eclipse-che"))
		e2e.Logf("step: check api.Registry/GetBundleForChannel")
		outputCurl, err = exec.Command("grpcurl", "-plaintext", "-d", "{\"pkgName\":\"eclipse-che\",\"channelName\":\"alpha\"}", "localhost:26594", "api.Registry/GetBundleForChannel").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(string(outputCurl)).To(o.ContainSubstring(image))
		cmd.Process.Kill()
		exutil.By("step: SUCCESS")

	})

	g.It("Author:xzha-ConnectedOnly-Medium-43409-opm can list catalog contents", func() {
		dbimagetag := "quay.io/olmqe/community-operator-index:v4.8"
		dcimagetag := "quay.io/olmqe/community-operator-index:v4.8-dc"

		exutil.By("1, testing with index.db image ")
		exutil.By("1.1 list packages")
		output, err := opmCLI.Run("alpha").Args("list", "packages", dbimagetag).Output()
		if err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		o.Expect(string(output)).To(o.ContainSubstring("3scale-community-operator"))
		o.Expect(string(output)).To(o.ContainSubstring("3scale API Management"))
		o.Expect(string(output)).To(o.ContainSubstring("threescale-2.10"))

		exutil.By("1.2 list channels")
		output, err = opmCLI.Run("alpha").Args("list", "channels", dbimagetag).Output()
		if err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		o.Expect(string(output)).To(o.ContainSubstring("3scale-community-operator"))
		o.Expect(string(output)).To(o.ContainSubstring("threescale-2.10"))
		o.Expect(string(output)).To(o.ContainSubstring("3scale-community-operator.v0.7.0"))

		exutil.By("1.3 list channels in a package")
		output, err = opmCLI.Run("alpha").Args("list", "channels", dbimagetag, "3scale-community-operator").Output()
		if err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		o.Expect(string(output)).To(o.ContainSubstring("threescale-2.10"))
		o.Expect(string(output)).To(o.ContainSubstring("threescale-2.9"))

		exutil.By("1.4 list bundles")
		output, err = opmCLI.Run("alpha").Args("list", "bundles", dbimagetag).Output()
		if err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		o.Expect(string(output)).To(o.ContainSubstring("3scale-community-operator.v0.6.0"))
		o.Expect(string(output)).To(o.ContainSubstring("3scale-community-operator.v0.7.0"))

		exutil.By("1.5 list bundles in a package")
		output, err = opmCLI.Run("alpha").Args("list", "bundles", dbimagetag, "wso2am-operator").Output()
		if err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		o.Expect(string(output)).To(o.ContainSubstring("wso2am-operator.v1.0.0"))
		o.Expect(string(output)).To(o.ContainSubstring("wso2am-operator.v1.0.1"))
		o.Expect(string(output)).To(o.ContainSubstring("wso2am-operator.v1.1.0"))

		exutil.By("2, testing with dc format index image")
		exutil.By("2.1 list packages")
		output, err = opmCLI.Run("alpha").Args("list", "packages", dcimagetag).Output()
		if err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		o.Expect(string(output)).To(o.ContainSubstring("3scale-community-operator"))
		o.Expect(string(output)).To(o.ContainSubstring("3scale API Management"))
		o.Expect(string(output)).To(o.ContainSubstring("threescale-2.10"))

		exutil.By("2.2 list channels")
		output, err = opmCLI.Run("alpha").Args("list", "channels", dcimagetag).Output()
		if err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		o.Expect(string(output)).To(o.ContainSubstring("3scale-community-operator"))
		o.Expect(string(output)).To(o.ContainSubstring("threescale-2.10"))
		o.Expect(string(output)).To(o.ContainSubstring("3scale-community-operator.v0.7.0"))

		exutil.By("2.3 list channels in a package")
		output, err = opmCLI.Run("alpha").Args("list", "channels", dcimagetag, "3scale-community-operator").Output()
		if err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		o.Expect(string(output)).To(o.ContainSubstring("threescale-2.10"))
		o.Expect(string(output)).To(o.ContainSubstring("threescale-2.9"))

		exutil.By("2.4 list bundles")
		output, err = opmCLI.Run("alpha").Args("list", "bundles", dcimagetag).Output()
		if err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		o.Expect(string(output)).To(o.ContainSubstring("3scale-community-operator.v0.6.0"))
		o.Expect(string(output)).To(o.ContainSubstring("3scale-community-operator.v0.7.0"))

		exutil.By("2.5 list bundles in a package")
		output, err = opmCLI.Run("alpha").Args("list", "bundles", dcimagetag, "wso2am-operator").Output()
		if err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		o.Expect(string(output)).To(o.ContainSubstring("wso2am-operator.v1.0.0"))
		o.Expect(string(output)).To(o.ContainSubstring("wso2am-operator.v1.0.1"))
		o.Expect(string(output)).To(o.ContainSubstring("wso2am-operator.v1.1.0"))

		exutil.By("3, testing with index.db file")
		opmBaseDir := exutil.FixturePath("testdata", "opm")
		TmpDataPath := filepath.Join(opmBaseDir, "tmp")
		indexdbFilePath := filepath.Join(TmpDataPath, "index.db")
		err = os.MkdirAll(TmpDataPath, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("get index.db")
		_, err = oc.AsAdmin().WithoutNamespace().Run("image").Args("extract", dbimagetag, "--path", "/database/index.db:"+TmpDataPath).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("get index.db SUCCESS, path is %s", indexdbFilePath)
		if _, err := os.Stat(indexdbFilePath); os.IsNotExist(err) {
			e2e.Logf("get index.db Failed")
		}

		exutil.By("3.1 list packages")
		output, err = opmCLI.Run("alpha").Args("list", "packages", indexdbFilePath).Output()
		if err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		o.Expect(string(output)).To(o.ContainSubstring("3scale-community-operator"))
		o.Expect(string(output)).To(o.ContainSubstring("3scale API Management"))
		o.Expect(string(output)).To(o.ContainSubstring("threescale-2.10"))

		exutil.By("3.2 list channels")
		output, err = opmCLI.Run("alpha").Args("list", "channels", indexdbFilePath).Output()
		if err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		o.Expect(string(output)).To(o.ContainSubstring("3scale-community-operator"))
		o.Expect(string(output)).To(o.ContainSubstring("threescale-2.10"))
		o.Expect(string(output)).To(o.ContainSubstring("3scale-community-operator.v0.7.0"))

		exutil.By("3.3 list channels in a package")
		output, err = opmCLI.Run("alpha").Args("list", "channels", indexdbFilePath, "3scale-community-operator").Output()
		if err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		o.Expect(string(output)).To(o.ContainSubstring("threescale-2.10"))
		o.Expect(string(output)).To(o.ContainSubstring("threescale-2.9"))

		exutil.By("3.4 list bundles")
		output, err = opmCLI.Run("alpha").Args("list", "bundles", indexdbFilePath).Output()
		if err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		o.Expect(string(output)).To(o.ContainSubstring("3scale-community-operator.v0.6.0"))
		o.Expect(string(output)).To(o.ContainSubstring("3scale-community-operator.v0.7.0"))

		exutil.By("3.5 list bundles in a package")
		output, err = opmCLI.Run("alpha").Args("list", "bundles", indexdbFilePath, "wso2am-operator").Output()
		if err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		o.Expect(string(output)).To(o.ContainSubstring("wso2am-operator.v1.0.0"))
		o.Expect(string(output)).To(o.ContainSubstring("wso2am-operator.v1.0.1"))
		o.Expect(string(output)).To(o.ContainSubstring("wso2am-operator.v1.1.0"))

		exutil.By("step: SUCCESS")
	})

	// author: xzha@redhat.com
	g.It("Author:xzha-ConnectedOnly-VMonly-Medium-43147-opm support rebuild index if any bundles have been truncated", func() {
		quayCLI := container.NewQuayCLI()
		containerCLI := container.NewPodmanCLI()
		containerTool := "podman"
		indexImage := "quay.io/olmqe/ditto-index:43147"
		indexImageDep := "quay.io/olmqe/ditto-index:43147-dep" + getRandomString()
		indexImageOW := "quay.io/olmqe/ditto-index:43147-ow" + getRandomString()

		defer containerCLI.RemoveImage(indexImage)
		defer containerCLI.RemoveImage(indexImageDep)
		defer containerCLI.RemoveImage(indexImageOW)
		defer quayCLI.DeleteTag(strings.Replace(indexImageDep, "quay.io/", "", 1))

		exutil.By("step: run deprecatetruncate")
		output, err := opmCLI.Run("index").Args("deprecatetruncate", "-b", "quay.io/olmqe/ditto-operator:0.1.1", "-f", indexImage, "-t", indexImageDep, "-c", containerTool).Output()
		if err != nil {
			e2e.Logf(output)
		}
		o.Expect(err).NotTo(o.HaveOccurred())
		output, err = containerCLI.Run("push").Args(indexImageDep).Output()
		if err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		exutil.By("check there is no channel alpha")
		output, err = opmCLI.Run("alpha").Args("list", "channels", indexImageDep).Output()
		if err != nil {
			e2e.Logf(output)
		}
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(string(output)).NotTo(o.ContainSubstring("alpha"))
		o.Expect(string(output)).To(o.ContainSubstring("beta"))
		o.Expect(string(output)).NotTo(o.ContainSubstring("ditto-operator.v0.1.0"))

		exutil.By("re-adding the bundle")
		output, err = opmCLI.Run("index").Args("add", "-b", "quay.io/olmqe/ditto-operator:0.2.0-43147", "-f", indexImageDep, "-t", indexImageOW, "--overwrite-latest", "-c", containerTool).Output()
		if err != nil {
			e2e.Logf(output)
		}
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(string(output)).NotTo(o.ContainSubstring("ERRO"))

		exutil.By("step: 43147 SUCCESS")
	})

	// author: xzha@redhat.com
	g.It("Author:xzha-ConnectedOnly-VMonly-Medium-43562-opm should raise error when adding an bundle whose version is higher than the bundle being added", func() {
		containerCLI := container.NewPodmanCLI()
		containerTool := "podman"
		indexImage := "quay.io/olmqe/ditto-index:43562"
		indexImage1 := "quay.io/olmqe/ditto-index:43562-1" + getRandomString()
		indexImage2 := "quay.io/olmqe/ditto-index:43562-2" + getRandomString()

		defer containerCLI.RemoveImage(indexImage)
		defer containerCLI.RemoveImage(indexImage1)
		defer containerCLI.RemoveImage(indexImage2)

		exutil.By("step: run add ditto-operator.v0.1.0 replace ditto-operator.v0.1.1")
		output1, err := opmCLI.Run("index").Args("add", "-b", "quay.io/olmqe/ditto-operator:43562-0.1.0", "-f", indexImage, "-t", indexImage1, "-c", containerTool).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(string(output1)).To(o.ContainSubstring("error"))
		o.Expect(string(output1)).To(o.ContainSubstring("permissive mode disabled"))
		o.Expect(string(output1)).To(o.ContainSubstring("this may be due to incorrect channel head"))

		output2, err := opmCLI.Run("index").Args("add", "-b", "quay.io/olmqe/ditto-operator:43562-0.1.2", "-f", indexImage, "-t", indexImage1, "-c", containerTool).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(string(output2)).To(o.ContainSubstring("error"))
		o.Expect(string(output2)).To(o.ContainSubstring("permissive mode disabled"))
		o.Expect(string(output2)).To(o.ContainSubstring("this may be due to incorrect channel head"))

		exutil.By("test case 43562 SUCCESS")
	})

	// author: tbuskey@redhat.com
	g.It("ConnectedOnly-Author:xzha-VMonly-High-30786-Bundle addition commutativity", func() {
		var sqlit = db.NewSqlit()
		opmBaseDir := exutil.FixturePath("testdata", "opm")
		defer DeleteDir(opmBaseDir, "fixture-testdata")
		TestDataPath := filepath.Join(opmBaseDir, "temp")
		opmCLI.ExecCommandPath = TestDataPath

		var (
			bundles    [3]string
			bundleName [3]string
			indexName  = "index30786"
			matched    bool
			sqlResults []db.Channel
		)

		exutil.By("Setup environment")
		// see OCP-30786 for creation of these images
		bundles[0] = "quay.io/olmqe/etcd-bundle:0.9.0-39795"
		bundles[1] = "quay.io/olmqe/etcd-bundle:0.9.2-39795"
		bundles[2] = "quay.io/olmqe/etcd-bundle:0.9.4-39795"
		bundleName[0] = "etcdoperator.v0.9.0"
		bundleName[1] = "etcdoperator.v0.9.2"
		bundleName[2] = "etcdoperator.v0.9.4"
		podmanCLI := container.NewPodmanCLI()
		containerCLI := podmanCLI

		indexTmpPath1 := filepath.Join(TestDataPath, "database")
		err := os.MkdirAll(indexTmpPath1, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create index image with a,b")
		index := 1
		a := 0
		b := 1
		order := "a,b"
		s := fmt.Sprintf("%v,%v", bundles[a], bundles[b])
		t1 := fmt.Sprintf("%v:%v", indexName, index)
		defer podmanCLI.RemoveImage(t1)
		msg, err := opmCLI.Run("index").Args("add", "-b", s, "-t", t1).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf(msg)
		matched, _ = regexp.MatchString(fmt.Sprintf("bundles=.*%v %v", bundles[a], bundles[b]), msg)
		o.Expect(matched).To(o.BeTrue())

		msg, err = containerCLI.Run("images").Args("-n", t1).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("IMAGES in %v: %v", order, msg)
		o.Expect(msg).NotTo(o.BeEmpty())
		podmanCLI.RemoveImage(t1)

		exutil.By("Generate db with a,b & check with sqlite")
		msg, err = opmCLI.Run("index").Args("add", "-b", s, "--generate").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf(msg)
		matched, _ = regexp.MatchString(fmt.Sprintf("bundles=.*%v %v", bundles[a], bundles[b]), msg)
		o.Expect(matched).To(o.BeTrue())

		sqlResults, err = sqlit.QueryOperatorChannel(path.Join(indexTmpPath1, "index.db"))
		// force string compare
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("sqlite contents %v: %v", order, sqlResults)
		o.Expect(fmt.Sprintf("%v", sqlResults[0])).To(o.ContainSubstring(bundleName[1]))
		o.Expect(fmt.Sprintf("%v", sqlResults[1])).To(o.ContainSubstring(bundleName[0]))
		os.Remove(path.Join(indexTmpPath1, "index.db"))

		exutil.By("Create index image with b,a")
		index++
		a = 1
		b = 0
		order = "b,a"
		s = fmt.Sprintf("%v,%v", bundles[a], bundles[b])
		t2 := fmt.Sprintf("%v:%v", indexName, index)
		defer podmanCLI.RemoveImage(t2)
		msg, err = opmCLI.Run("index").Args("add", "-b", s, "-t", t2).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf(msg)
		matched, _ = regexp.MatchString(fmt.Sprintf("bundles=.*%v %v", bundles[a], bundles[b]), msg)
		o.Expect(matched).To(o.BeTrue())

		msg, err = containerCLI.Run("images").Args("-n", t2).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("IMAGES in %v: %v", order, msg)
		o.Expect(msg).NotTo(o.BeEmpty())
		podmanCLI.RemoveImage(t2)

		exutil.By("Generate db with b,a & check with sqlite")
		msg, err = opmCLI.Run("index").Args("add", "-b", s, "--generate").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf(msg)
		matched, _ = regexp.MatchString(fmt.Sprintf("bundles=.*%v %v", bundles[a], bundles[b]), msg)
		o.Expect(matched).To(o.BeTrue())

		sqlResults, err = sqlit.QueryOperatorChannel(path.Join(indexTmpPath1, "index.db"))
		// force string compare
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("sqlite contents %v: %v", order, sqlResults)
		o.Expect(fmt.Sprintf("%v", sqlResults[0])).To(o.ContainSubstring(bundleName[1]))
		o.Expect(fmt.Sprintf("%v", sqlResults[1])).To(o.ContainSubstring(bundleName[0]))
		os.Remove(path.Join(indexTmpPath1, "index.db"))

		exutil.By("Create index image with a,b,c")
		index++
		a = 0
		b = 1
		c := 2
		order = "a,b,c"
		s = fmt.Sprintf("%v,%v,%v", bundles[a], bundles[b], bundles[c])
		t3 := fmt.Sprintf("%v:%v", indexName, index)
		defer podmanCLI.RemoveImage(t3)
		msg, err = opmCLI.Run("index").Args("add", "-b", s, "-t", t3).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf(msg)
		matched, _ = regexp.MatchString(fmt.Sprintf("bundles=.*%v %v %v", bundles[a], bundles[b], bundles[c]), msg)
		o.Expect(matched).To(o.BeTrue())

		msg, err = containerCLI.Run("images").Args("-n", t3).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("IMAGES in %v: %v", order, msg)
		o.Expect(msg).NotTo(o.BeEmpty())
		podmanCLI.RemoveImage(t3)

		exutil.By("Generate db with a,b,c & check with sqlite")
		msg, err = opmCLI.Run("index").Args("add", "-b", s, "--generate").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf(msg)
		matched, _ = regexp.MatchString(fmt.Sprintf("bundles=.*%v %v %v", bundles[a], bundles[b], bundles[c]), msg)
		o.Expect(matched).To(o.BeTrue())

		sqlResults, err = sqlit.QueryOperatorChannel(path.Join(indexTmpPath1, "index.db"))
		// force string compare
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("sqlite contents %v: %v", order, sqlResults)
		o.Expect(fmt.Sprintf("%v", sqlResults[0])).To(o.ContainSubstring(bundleName[2]))
		o.Expect(fmt.Sprintf("%v", sqlResults[1])).To(o.ContainSubstring(bundleName[1]))
		o.Expect(fmt.Sprintf("%v", sqlResults[2])).To(o.ContainSubstring(bundleName[0]))
		os.Remove(path.Join(indexTmpPath1, "index.db"))

		exutil.By("Create index image with b,c,a")
		index++
		a = 1
		b = 2
		c = 0
		order = "b,c,a"
		s = fmt.Sprintf("%v,%v,%v", bundles[a], bundles[b], bundles[c])
		t4 := fmt.Sprintf("%v:%v", indexName, index)
		defer podmanCLI.RemoveImage(t4)
		msg, err = opmCLI.Run("index").Args("add", "-b", s, "-t", t4).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf(msg)
		matched, _ = regexp.MatchString(fmt.Sprintf("bundles=.*%v %v %v", bundles[a], bundles[b], bundles[c]), msg)
		o.Expect(matched).To(o.BeTrue())

		msg, err = containerCLI.Run("images").Args("-n", t4).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("IMAGES in %v: %v", order, msg)
		o.Expect(msg).NotTo(o.BeEmpty())
		podmanCLI.RemoveImage(t4)
		// no db check

		exutil.By("Create index image with c,a,b")
		index++
		a = 2
		b = 0
		c = 1
		order = "c,a,b"
		s = fmt.Sprintf("%v,%v,%v", bundles[a], bundles[b], bundles[c])
		t5 := fmt.Sprintf("%v:%v", indexName, index)
		defer podmanCLI.RemoveImage(t5)
		msg, err = opmCLI.Run("index").Args("add", "-b", s, "-t", t5).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf(msg)
		matched, _ = regexp.MatchString(fmt.Sprintf("bundles=.*%v %v %v", bundles[a], bundles[b], bundles[c]), msg)
		o.Expect(matched).To(o.BeTrue())

		msg, err = containerCLI.Run("images").Args("-n", t5).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("IMAGES in %v: %v", order, msg)
		o.Expect(msg).NotTo(o.BeEmpty())
		podmanCLI.RemoveImage(t5)
		// no db check

		exutil.By("Generate db with b,a,c & check with sqlite")
		a = 1
		b = 0
		c = 2
		order = "b,a,c"
		s = fmt.Sprintf("%v,%v,%v", bundles[a], bundles[b], bundles[c])
		// no image check, just db

		msg, err = opmCLI.Run("index").Args("add", "-b", s, "--generate").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf(msg)
		matched, _ = regexp.MatchString(fmt.Sprintf("bundles=.*%v %v %v", bundles[a], bundles[b], bundles[c]), msg)
		o.Expect(matched).To(o.BeTrue())

		sqlResults, err = sqlit.QueryOperatorChannel(path.Join(indexTmpPath1, "index.db"))
		// force string compare
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("sqlite contents %v: %v", order, sqlResults)
		o.Expect(fmt.Sprintf("%v", sqlResults[0])).To(o.ContainSubstring(bundleName[2]))
		o.Expect(fmt.Sprintf("%v", sqlResults[1])).To(o.ContainSubstring(bundleName[1]))
		o.Expect(fmt.Sprintf("%v", sqlResults[2])).To(o.ContainSubstring(bundleName[0]))
		os.Remove(path.Join(indexTmpPath1, "index.db"))

		exutil.By("Finished")
	})

	// author: scolange@redhat.com
	g.It("ConnectedOnly-Author:scolange-VMonly-Medium-25935-Ability to modify the contents of an existing registry database", func() {
		containerCLI := container.NewPodmanCLI()
		containerTool := "podman"
		opmBaseDir := exutil.FixturePath("testdata", "opm")
		TmpDataPath := filepath.Join(opmBaseDir, "tmp")

		defer DeleteDir(TmpDataPath, "fixture-testdata")

		bundleImageTag1 := "quay.io/operator-framework/operator-bundle-prometheus:0.14.0"
		bundleImageTag2 := "quay.io/operator-framework/operator-bundle-prometheus:0.15.0"
		bundleImageTag3 := "quay.io/operator-framework/operator-bundle-prometheus:0.22.2"
		defer containerCLI.RemoveImage(bundleImageTag1)
		defer containerCLI.RemoveImage(bundleImageTag2)
		defer containerCLI.RemoveImage(bundleImageTag3)

		exutil.By("step: build bundle.db")
		dbFilePath := TmpDataPath + "bundles.db"
		if output, err := opmCLI.Run("registry").Args("add", "-b", bundleImageTag1, "-d", dbFilePath, "-c", containerTool, "--mode", "semver").Output(); err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		exutil.By("step1: modified the bundle.db already created")
		if output, err := opmCLI.Run("registry").Args("add", "-b", bundleImageTag2, "-d", dbFilePath, "-c", containerTool, "--mode", "semver").Output(); err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		exutil.By("step2: modified the bundle.db already created")
		if output, err := opmCLI.Run("registry").Args("add", "-b", bundleImageTag3, "-d", dbFilePath, "-c", containerTool, "--mode", "semver").Output(); err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		exutil.By("step: SUCCESS 25935")

	})

	// author: xzha@redhat.com
	g.It("Author:xzha-Medium-45407-opm and oc should print sqlite deprecation warnings", func() {
		exutil.By("opm render --help")
		output, err := opmCLI.Run("render").Args("--help").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("DEPRECATION NOTICE:"))

		exutil.By("opm index --help")
		output, err = opmCLI.Run("index").Args("--help").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("DEPRECATION NOTICE:"))

		exutil.By("opm registry --help")
		output, err = opmCLI.Run("registry").Args("--help").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("DEPRECATION NOTICE:"))

		exutil.By("oc adm catalog mirror --help")
		output, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("catalog", "mirror", "--help").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("DEPRECATION NOTICE:"))

		exutil.By("45407 SUCCESS")
	})

	// author: xzha@redhat.com
	g.It("Author:xzha-ConnectedOnly-VMonly-Medium-45403-opm index prune should report error if the working directory does not have write permissions", func() {
		podmanCLI := container.NewPodmanCLI()
		opmBaseDir := exutil.FixturePath("testdata", "opm")
		tmpPath := filepath.Join(opmBaseDir, "temp"+getRandomString())
		defer DeleteDir(tmpPath, "fixture-testdata")
		exutil.By("step: mkdir with mode 0555")
		err := os.MkdirAll(tmpPath, 0555)
		o.Expect(err).NotTo(o.HaveOccurred())
		opmCLI.ExecCommandPath = tmpPath

		exutil.By("step: opm index prune")
		containerTool := "podman"
		sourceImageTag := "quay.io/olmqe/multi-index:2.0"
		imageTag := "quay.io/olmqe/multi-index:45403-" + getRandomString()
		defer podmanCLI.RemoveImage(imageTag)
		output, err := opmCLI.Run("index").Args("prune", "-f", sourceImageTag, "-p", "planetscale", "-t", imageTag, "-c", containerTool).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(output).To(o.MatchRegexp("(?i)mkdir .* permission denied(?i)"))
		exutil.By("45403 SUCCESS")
	})

	// author: xzha@redhat.com
	g.It("Author:xzha-ConnectedOnly-Medium-53869-opm supports creating a catalog using basic veneer", func() {
		if os.Getenv("HTTP_PROXY") != "" || os.Getenv("http_proxy") != "" {
			g.Skip("HTTP_PROXY is not empty - skipping test ...")
		}
		opmBaseDir := exutil.FixturePath("testdata", "opm", "53869")
		opmCLI.ExecCommandPath = opmBaseDir
		defer DeleteDir(opmBaseDir, "fixture-testdata")

		exutil.By("step: create dir catalog")
		catsrcPathYaml := filepath.Join(opmBaseDir, "catalog-yaml")
		err := os.MkdirAll(catsrcPathYaml, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("step: create a catalog using basic veneer with yaml format")
		output, err := opmCLI.Run("alpha").Args("render-template", "basic", filepath.Join(opmBaseDir, "catalog-basic-template.yaml"), "-o", "yaml").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(string(output)).To(o.ContainSubstring("nginx-operator"))

		indexFilePath := filepath.Join(catsrcPathYaml, "index.yaml")
		if err = ioutil.WriteFile(indexFilePath, []byte(output), 0644); err != nil {
			e2e.Failf(fmt.Sprintf("Writefile %s Error: %v", indexFilePath, err))
		}
		output, err = opmCLI.Run("validate").Args(catsrcPathYaml).Output()
		if err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		output, err = opmCLI.Run("alpha").Args("list", "bundles", catsrcPathYaml).Output()
		if err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		o.Expect(string(output)).To(o.ContainSubstring("quay.io/olmqe/nginxolm-operator-bundle:v0.0.1"))
		o.Expect(string(output)).To(o.ContainSubstring("quay.io/olmqe/nginxolm-operator-bundle:v1.0.1"))

		exutil.By("step: create dir catalog")
		catsrcPathJSON := filepath.Join(opmBaseDir, "catalog-json")
		err = os.MkdirAll(catsrcPathJSON, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("step: create a catalog using basic veneer with json format")
		output, err = opmCLI.Run("alpha").Args("render-template", "basic", filepath.Join(opmBaseDir, "catalog-basic-template.yaml"), "-o", "json").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		indexFilePath = filepath.Join(catsrcPathJSON, "index.json")
		if err = ioutil.WriteFile(indexFilePath, []byte(output), 0644); err != nil {
			e2e.Failf(fmt.Sprintf("Writefile %s Error: %v", indexFilePath, err))
		}
		output, err = opmCLI.Run("validate").Args(catsrcPathJSON).Output()
		if err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		output, err = opmCLI.Run("alpha").Args("list", "bundles", catsrcPathJSON, "nginx-operator").Output()
		if err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		o.Expect(string(output)).To(o.ContainSubstring("quay.io/olmqe/nginxolm-operator-bundle:v0.0.1"))
		o.Expect(string(output)).To(o.ContainSubstring("quay.io/olmqe/nginxolm-operator-bundle:v1.0.1"))
	})

	// author: xzha@redhat.com
	g.It("Author:xzha-ConnectedOnly-Medium-53871-Medium-53915-Medium-53996-opm supports creating a catalog using semver veneer", func() {
		if os.Getenv("HTTP_PROXY") != "" || os.Getenv("http_proxy") != "" {
			g.Skip("HTTP_PROXY is not empty - skipping test ...")
		}
		opmBaseDir := exutil.FixturePath("testdata", "opm", "53871")
		opmCLI.ExecCommandPath = opmBaseDir
		defer DeleteDir(opmBaseDir, "fixture-testdata")

		exutil.By("step: create dir catalog-1")
		catsrcPath1 := filepath.Join(opmBaseDir, "catalog-1")
		err := os.MkdirAll(catsrcPath1, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("step: GenerateMajorChannels: true GenerateMinorChannels: false")
		output, err := opmCLI.Run("alpha").Args("render-template", "semver", filepath.Join(opmBaseDir, "catalog-semver-veneer-1.yaml"), "-o", "yaml").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		indexFilePath := filepath.Join(catsrcPath1, "index.yaml")
		if err = ioutil.WriteFile(indexFilePath, []byte(output), 0644); err != nil {
			e2e.Failf(fmt.Sprintf("Writefile %s Error: %v", indexFilePath, err))
		}
		output, err = opmCLI.Run("validate").Args(catsrcPath1).Output()
		if err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		output, err = opmCLI.Run("alpha").Args("list", "channels", catsrcPath1, "nginx-operator").Output()
		if err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		o.Expect(string(output)).To(o.ContainSubstring("candidate-v0  nginx-operator.v0.0.1"))
		o.Expect(string(output)).To(o.ContainSubstring("candidate-v1  nginx-operator.v1.0.2"))
		o.Expect(string(output)).To(o.ContainSubstring("candidate-v2  nginx-operator.v2.1.0"))
		o.Expect(string(output)).To(o.ContainSubstring("fast-v0       nginx-operator.v0.0.1"))
		o.Expect(string(output)).To(o.ContainSubstring("fast-v2       nginx-operator.v2.1.0"))
		o.Expect(string(output)).To(o.ContainSubstring("stable-v1     nginx-operator.v1.0.2"))
		o.Expect(string(output)).To(o.ContainSubstring("stable-v2     nginx-operator.v2.1.0"))

		exutil.By("step: create dir catalog-2")
		catsrcPath2 := filepath.Join(opmBaseDir, "catalog-2")
		err = os.MkdirAll(catsrcPath2, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("step: GenerateMajorChannels: true GenerateMinorChannels: true")
		output, err = opmCLI.Run("alpha").Args("render-template", "semver", filepath.Join(opmBaseDir, "catalog-semver-veneer-2.yaml"), "-o", "yaml").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		indexFilePath = filepath.Join(catsrcPath2, "index.yaml")
		if err = ioutil.WriteFile(indexFilePath, []byte(output), 0644); err != nil {
			e2e.Failf(fmt.Sprintf("Writefile %s Error: %v", indexFilePath, err))
		}
		output, err = opmCLI.Run("validate").Args(catsrcPath2).Output()
		if err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		output, err = opmCLI.Run("alpha").Args("list", "channels", catsrcPath2, "nginx-operator").Output()
		if err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		o.Expect(string(output)).To(o.ContainSubstring("candidate-v0    nginx-operator.v0.0.1"))
		o.Expect(string(output)).To(o.ContainSubstring("candidate-v0.0  nginx-operator.v0.0.1"))
		o.Expect(string(output)).To(o.ContainSubstring("candidate-v1    nginx-operator.v1.0.1"))
		o.Expect(string(output)).To(o.ContainSubstring("candidate-v1.0  nginx-operator.v1.0.1"))
		o.Expect(string(output)).To(o.ContainSubstring("fast-v1         nginx-operator.v1.0.1-beta"))
		o.Expect(string(output)).To(o.ContainSubstring("fast-v1.0       nginx-operator.v1.0.1-beta"))
		o.Expect(string(output)).To(o.ContainSubstring("stable-v1       nginx-operator.v1.0.1"))
		o.Expect(string(output)).To(o.ContainSubstring("stable-v1.0     nginx-operator.v1.0.1"))

		exutil.By("step: create dir catalog-3")
		catsrcPath3 := filepath.Join(opmBaseDir, "catalog-3")
		err = os.MkdirAll(catsrcPath3, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("step: not set GenerateMajorChannels and GenerateMinorChannels")
		output, err = opmCLI.Run("alpha").Args("render-template", "semver", filepath.Join(opmBaseDir, "catalog-semver-veneer-3.yaml")).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		indexFilePath = filepath.Join(catsrcPath3, "index.json")
		if err = ioutil.WriteFile(indexFilePath, []byte(output), 0644); err != nil {
			e2e.Failf(fmt.Sprintf("Writefile %s Error: %v", indexFilePath, err))
		}
		output, err = opmCLI.Run("validate").Args(catsrcPath3).Output()
		if err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		output, err = opmCLI.Run("alpha").Args("list", "channels", catsrcPath3, "nginx-operator").Output()
		if err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		o.Expect(string(output)).NotTo(o.ContainSubstring("candidate-v0 "))
		o.Expect(string(output)).To(o.ContainSubstring("candidate-v0.0  nginx-operator.v0.0.1"))
		o.Expect(string(output)).NotTo(o.ContainSubstring("candidate-v1 "))
		o.Expect(string(output)).To(o.ContainSubstring("candidate-v1.0  nginx-operator.v1.0.2"))
		o.Expect(string(output)).NotTo(o.ContainSubstring("fast-v0 "))
		o.Expect(string(output)).To(o.ContainSubstring("fast-v0.0       nginx-operator.v0.0.1"))
		o.Expect(string(output)).NotTo(o.ContainSubstring("fast-v2 "))
		o.Expect(string(output)).To(o.ContainSubstring("fast-v2.0       nginx-operator.v2.0.1"))
		o.Expect(string(output)).To(o.ContainSubstring("fast-v2.1       nginx-operator.v2.1.0"))
		o.Expect(string(output)).NotTo(o.ContainSubstring("stable-v2 "))
		o.Expect(string(output)).To(o.ContainSubstring("stable-v2.1     nginx-operator.v2.1.0"))

		exutil.By("step: generate mermaid graph data for generated-channels")
		output, err = opmCLI.Run("alpha").Args("render-template", "semver", filepath.Join(opmBaseDir, "catalog-semver-veneer-4.yaml"), "-o", "mermaid").Output()
		if err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		o.Expect(string(output)).To(o.ContainSubstring("channel \"fast-v2.0\""))
		o.Expect(string(output)).To(o.ContainSubstring("subgraph nginx-operator-fast-v2.0[\"fast-v2.0\"]"))
		o.Expect(string(output)).To(o.ContainSubstring("nginx-operator-fast-v2.0-nginx-operator.v2.0.1[\"nginx-operator.v2.0.1\"]"))

		exutil.By("step: semver veneer should validate bundle versions")
		output, err = opmCLI.Run("alpha").Args("render-template", "semver", filepath.Join(opmBaseDir, "catalog-semver-veneer-5.yaml")).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(string(output)).To(o.ContainSubstring("encountered bundle versions which differ only by build metadata, which cannot be ordered"))
		o.Expect(string(output)).To(o.ContainSubstring("cannot be compared to \"1.0.1-alpha\""))

		exutil.By("OCP-53996")
		filePath := filepath.Join(opmBaseDir, "catalog-semver-veneer-1.yaml")
		exutil.By("step: create dir catalog")
		catsrcPath53996 := filepath.Join(opmBaseDir, "catalog-53996")
		err = os.MkdirAll(catsrcPath53996, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("step: generate index.yaml with yaml format")
		command := "cat " + filePath + "| opm alpha render-template semver -o yaml - "
		contentByte, err := exec.Command("bash", "-c", command).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		indexFilePath = filepath.Join(catsrcPath53996, "index.yaml")
		if err = ioutil.WriteFile(indexFilePath, contentByte, 0644); err != nil {
			e2e.Failf(fmt.Sprintf("Writefile %s Error: %v", indexFilePath, err))
		}
		output, err = opmCLI.Run("validate").Args(catsrcPath53996).Output()
		if err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		output, err = opmCLI.Run("alpha").Args("list", "channels", catsrcPath53996, "nginx-operator").Output()
		if err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		o.Expect(string(output)).To(o.ContainSubstring("candidate-v0  nginx-operator.v0.0.1"))
		o.Expect(string(output)).To(o.ContainSubstring("candidate-v1  nginx-operator.v1.0.2"))
		o.Expect(string(output)).To(o.ContainSubstring("candidate-v2  nginx-operator.v2.1.0"))
		o.Expect(string(output)).To(o.ContainSubstring("fast-v0       nginx-operator.v0.0.1"))
		o.Expect(string(output)).To(o.ContainSubstring("fast-v2       nginx-operator.v2.1.0"))
		o.Expect(string(output)).To(o.ContainSubstring("stable-v1     nginx-operator.v1.0.2"))
		o.Expect(string(output)).To(o.ContainSubstring("stable-v2     nginx-operator.v2.1.0"))

		exutil.By("step: generate json format file")
		command = "cat " + filePath + `| opm alpha render-template semver  - | jq 'select(.schema=="olm.channel")'| jq '{name,entries}'`
		contentByte, err = exec.Command("bash", "-c", command).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(string(contentByte)).To(o.ContainSubstring("nginx-operator.v1.0.2"))
		o.Expect(string(contentByte)).To(o.ContainSubstring("candidate-v1"))

		exutil.By("step: generate mermaid graph data for generated-channels")
		command = "cat " + filePath + "| opm alpha render-template semver -o mermaid -"
		contentByte, err = exec.Command("bash", "-c", command).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(string(contentByte)).To(o.ContainSubstring("package \"nginx-operator\""))
		o.Expect(string(contentByte)).To(o.ContainSubstring("nginx-operator-candidate-v1-nginx-operator.v1.0.1"))
	})

	// author: xzha@redhat.com
	g.It("Author:xzha-ConnectedOnly-Medium-53917-opm can visualize the update graph for a given Operator from an arbitrary version", func() {
		if os.Getenv("HTTP_PROXY") != "" || os.Getenv("http_proxy") != "" {
			g.Skip("HTTP_PROXY is not empty - skipping test ...")
		}

		exutil.By("step: check help message")
		output, err := opmCLI.Run("alpha").Args("render-graph", "-h").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(string(output)).To(o.ContainSubstring("--minimum-edge"))
		o.Expect(string(output)).To(o.ContainSubstring("--use-http"))

		exutil.By("step: opm alpha render-graph index-image")
		output, err = opmCLI.Run("alpha").Args("render-graph", "quay.io/olmqe/nginxolm-operator-index:v1").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(string(output)).To(o.ContainSubstring("package \"nginx-operator\""))
		o.Expect(string(output)).To(o.ContainSubstring("subgraph nginx-operator-alpha[\"alpha\"]"))

		exutil.By("step: opm alpha render-graph index-image with --minimum-edge")
		output, err = opmCLI.Run("alpha").Args("render-graph", "quay.io/olmqe/nginxolm-operator-index:v1", "--minimum-edge", "nginx-operator.v1.0.1").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(string(output)).To(o.ContainSubstring("package \"nginx-operator\""))
		o.Expect(string(output)).To(o.ContainSubstring("nginx-operator.v1.0.1"))
		o.Expect(string(output)).NotTo(o.ContainSubstring("nginx-operator.v0.0.1"))

		exutil.By("step: create dir catalog")
		catsrcPath := filepath.Join("/tmp", "53917-catalog")
		defer os.RemoveAll(catsrcPath)
		errCreateDir := os.MkdirAll(catsrcPath, 0755)
		o.Expect(errCreateDir).NotTo(o.HaveOccurred())

		exutil.By("step: opm alpha render-graph fbc-dir")
		output, err = opmCLI.Run("render").Args("quay.io/olmqe/nginxolm-operator-index:v1", "-o", "json").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		indexFilePath := filepath.Join(catsrcPath, "index.json")
		if err = ioutil.WriteFile(indexFilePath, []byte(output), 0644); err != nil {
			e2e.Failf(fmt.Sprintf("Writefile %s Error: %v", indexFilePath, err))
		}
		output, err = opmCLI.Run("alpha").Args("render-graph", catsrcPath).Output()
		if err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		o.Expect(string(output)).To(o.ContainSubstring("package \"nginx-operator\""))
		o.Expect(string(output)).To(o.ContainSubstring("subgraph nginx-operator-alpha[\"alpha\"]"))

		exutil.By("step: opm alpha render-graph sqlit-based catalog image")
		output, err = opmCLI.Run("alpha").Args("render-graph", "quay.io/olmqe/ditto-index:v1beta1").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(string(output)).To(o.ContainSubstring("subgraph \"ditto-operator\""))
		o.Expect(string(output)).To(o.ContainSubstring("subgraph ditto-operator-alpha[\"alpha\"]"))
		o.Expect(string(output)).To(o.ContainSubstring("ditto-operator.v0.2.0"))
	})

	// author: scolange@redhat.com
	g.It("ConnectedOnly-Author:scolange-VMonly-Medium-25934-Reference non-latest versions of bundles by image digests", func() {
		containerCLI := container.NewPodmanCLI()
		containerTool := "podman"
		opmBaseDir := exutil.FixturePath("testdata", "opm")
		TmpDataPath := filepath.Join(opmBaseDir, "tmp")
		err := os.MkdirAll(TmpDataPath, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		bundleImageTag1 := "quay.io/olmqe/etcd-bundle:0.9.0"
		bundleImageTag2 := "quay.io/olmqe/etcd-bundle:0.9.0-25934"
		defer DeleteDir(TmpDataPath, "fixture-testdata")
		defer containerCLI.RemoveImage(bundleImageTag2)
		defer containerCLI.RemoveImage(bundleImageTag1)
		defer exec.Command("kill", "-9", "$(lsof -t -i:25934)").Output()

		if output, err := containerCLI.Run("pull").Args(bundleImageTag1).Output(); err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		if output, err := containerCLI.Run("tag").Args(bundleImageTag1, bundleImageTag2).Output(); err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		if output, err := containerCLI.Run("push").Args(bundleImageTag2).Output(); err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		exutil.By("step: build bundle.db")
		dbFilePath := TmpDataPath + "bundles.db"
		if output, err := opmCLI.Run("registry").Args("add", "-b", bundleImageTag2, "-d", dbFilePath, "-c", containerTool, "--mode", "semver").Output(); err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		exutil.By("step: Run the opm registry server binary to load manifest and serves a grpc API to query it.")
		e2e.Logf("step: Run the registry-server ")
		cmd := exec.Command("opm", "registry", "serve", "-d", dbFilePath, "-p", "25934")
		e2e.Logf("cmd %v:", cmd)
		cmd.Dir = TmpDataPath
		err = cmd.Start()
		e2e.Logf("cmd %v raise error: %v", cmd, err)
		o.Expect(err).NotTo(o.HaveOccurred())

		e2e.Logf("step: check api.Registry/ListPackages")
		err = wait.PollUntilContextTimeout(context.TODO(), 20*time.Second, 240*time.Second, false, func(ctx context.Context) (bool, error) {
			outputCurl, err := exec.Command("grpcurl", "-plaintext", "localhost:25934", "api.Registry/ListPackages").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(string(outputCurl), "etcd") {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("grpcurl %s not listet package", dbFilePath))

		e2e.Logf("step: check api.Registry/GetBundleForChannel")
		outputCurl, err := exec.Command("grpcurl", "-plaintext", "localhost:25934", "api.Registry/ListBundles").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(string(outputCurl)).To(o.ContainSubstring("bundlePath"))
		cmd.Process.Kill()
		exutil.By("step: SUCCESS 25934")

	})

	// author: scolange@redhat.com
	g.It("ConnectedOnly-Author:scolange-VMonly-Medium-47222-can't remove package from index: database is locked", func() {
		podmanCLI := container.NewPodmanCLI()
		baseDir := exutil.FixturePath("testdata", "olm")
		TestDataPath := filepath.Join(baseDir, "temp")
		indexTmpPath := filepath.Join(TestDataPath, getRandomString())
		defer DeleteDir(TestDataPath, indexTmpPath)
		err := os.MkdirAll(indexTmpPath, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		indexImage := "registry.redhat.io/redhat/certified-operator-index:v4.7"

		exutil.By("remove package from index")
		dockerconfigjsonpath := filepath.Join(indexTmpPath, ".dockerconfigjson")
		defer exec.Command("rm", "-f", dockerconfigjsonpath).Output()
		_, err = oc.AsAdmin().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", "--confirm", "--to="+indexTmpPath).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		opmCLI.SetAuthFile(dockerconfigjsonpath)
		defer podmanCLI.RemoveImage(indexImage)
		output, err := opmCLI.Run("index").Args("rm", "--generate", "--binary-image", "registry.redhat.io/openshift4/ose-operator-registry:v4.7", "--from-index", indexImage, "--operators", "cert-manager-operator", "--pull-tool=podman").Output()
		e2e.Logf(output)
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.By("test case 47222 SUCCESS")
	})

	// author: jitli@redhat.com
	g.It("ConnectedOnly-Author:jitli-Medium-60573-opm exclude bundles with olm.deprecated property when rendering", func() {

		exutil.By("opm render the sqlite index image message")
		msg, err := opmCLI.Run("render").Args("quay.io/olmqe/catalogtest-index:v4.12depre", "-oyaml").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(msg, "olm.deprecated") {
			e2e.Failf("opm render the sqlite index image message, doesn't show the bundle with olm.dreprecated label")
		}

		exutil.By("opm render the fbc index image message")
		msg, err = opmCLI.Run("render").Args("quay.io/olmqe/test-index:mix", "-oyaml").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(msg, "olm.deprecated") {
			e2e.Logf("opm render the fbc index image message, still show the bundle with olm.dreprecated label")
		} else {
			e2e.Failf("opm render the fbc index image message, should show the bundle with olm.dreprecated label")
		}

	})

	// author: jitli@redhat.com
	g.It("Author:jitli-ConnectedOnly-Medium-73218-opm alpha render-graph indicate deprecated graph content", func() {
		if os.Getenv("HTTP_PROXY") != "" || os.Getenv("http_proxy") != "" {
			g.Skip("HTTP_PROXY is not empty - skipping test ...")
		}

		exutil.By("step: opm alpha render-graph index-image with deprecated label")
		output, err := opmCLI.Run("alpha").Args("render-graph", "quay.io/olmqe/olmtest-operator-index:nginxolm73218").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(string(output)).To(o.ContainSubstring("classDef deprecated fill:#E8960F"))
		o.Expect(string(output)).To(o.ContainSubstring("nginx73218-candidate-v1.0-nginx73218.v1.0.1[\"nginx73218.v1.0.1\"]:::deprecated"))
		o.Expect(string(output)).To(o.ContainSubstring("nginx73218-candidate-v1.0-nginx73218.v1.0.1[\"nginx73218.v1.0.1\"]-- skip --> nginx73218-candidate-v1.0-nginx73218.v1.0.3[\"nginx73218.v1.0.3\"]"))

	})

	// author: jitli@redhat.com
	g.It("Author:jitli-VMonly-ConnectedOnly-High-75148-opm generate binary-less dockerfiles", func() {

		tmpBasePath := "/tmp/ocp-75148-" + getRandomString()
		tmpPath := filepath.Join(tmpBasePath, "catalog")
		dockerFile := filepath.Join(tmpBasePath, "catalog.Dockerfile")

		err := os.MkdirAll(tmpPath, 0o755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(tmpBasePath)
		opmCLI.ExecCommandPath = tmpBasePath

		exutil.By("Check flag --binary-image has been deprecated")
		output, err := opmCLI.Run("generate").Args("dockerfile", "--binary-image", "quay.io/operator-framework/opm:latest", "catalog").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("--binary-image has been deprecated, use --base-image instead"))

		err = os.Remove(dockerFile)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Check parameter --base-image and --builder-image")
		_, err = opmCLI.Run("generate").Args("dockerfile", "catalog", "-i", "quay.io/openshifttest/opm:v1", "-b", "quay.io/openshifttest/opm:v2").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Check the custom images in Dockerfile")
		waitErr := wait.PollUntilContextTimeout(context.TODO(), 2*time.Second, 6*time.Second, false, func(ctx context.Context) (bool, error) {
			if _, err := os.Stat(dockerFile); os.IsNotExist(err) {
				e2e.Logf("get catalog.Dockerfile Failed")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "get catalog.Dockerfile Failed")

		content, err := ioutil.ReadFile(dockerFile)
		o.Expect(err).NotTo(o.HaveOccurred())

		if !strings.Contains(string(content), "FROM quay.io/openshifttest/opm:v2 as builder") || !strings.Contains(string(content), "FROM quay.io/openshifttest/opm:v1") {
			e2e.Failf("Fail to get the custom images in Dockerfile")
		}

		err = os.Remove(dockerFile)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create a binary-less dockerfile")
		_, err = opmCLI.Run("generate").Args("dockerfile", "catalog", "--base-image=scratch").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Check the custom images in Dockerfile")
		waitErr = wait.PollUntilContextTimeout(context.TODO(), 2*time.Second, 6*time.Second, false, func(ctx context.Context) (bool, error) {
			if _, err := os.Stat(dockerFile); os.IsNotExist(err) {
				e2e.Logf("get catalog.Dockerfile Failed")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "get catalog.Dockerfile Failed")

		content, err = ioutil.ReadFile(dockerFile)
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(string(content), "OLMv0 CatalogSources that use binary-less images must set:") || !strings.Contains(string(content), "extractContent:") || !strings.Contains(string(content), "FROM scratch") {
			e2e.Failf("Fail to get the scratch in Dockerfile")
		}

		err = os.Remove(dockerFile)
		o.Expect(err).NotTo(o.HaveOccurred())

	})

})
