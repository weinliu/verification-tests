package mco

import (
	"fmt"
	"path/filepath"

	g "github.com/onsi/ginkgo"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	container "github.com/openshift/openshift-tests-private/test/extended/util/container"
	logger "github.com/openshift/openshift-tests-private/test/extended/util/logext"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// OsImageBuilderInNode encapsulates the functionality to build custom osImages inside a cluster node
type OsImageBuilderInNode struct {
	node Node
	architecture,
	osImage,
	dockerFileCommands, // Full docker file but the "FROM basOsImage..." that will be calculated
	dockerConfig,
	remoteTmpDir,
	remoteKubeconfig,
	remoteDockerConfig,
	remoteDockerfile,
	saRegistryName string
}

func (b *OsImageBuilderInNode) prepareEnvironment() error {
	if b.dockerConfig == "" {
		logger.Infof("No docker config file was provided to the osImage builder. Generating a new docker config file")
		g.By("Extract pull-secret")
		pullSecret := GetPullSecret(b.node.oc.AsAdmin())
		tokenDir, err := pullSecret.Extract()
		if err != nil {
			return fmt.Errorf("Error extracting pull-secret. Error: %s", err)
		}
		logger.Infof("Pull secret has been extracted to: %s\n", tokenDir)
		b.dockerConfig = filepath.Join(tokenDir, ".dockerconfigjson")
	}
	logger.Infof("Using docker config file: %s\n", b.dockerConfig)

	if b.architecture == "" {
		b.architecture = exutil.GetClusterArchitecture(b.node.oc)
		logger.Infof("Building using architecture: %s", b.architecture)
	}
	if b.osImage == "" {
		b.osImage = getLayeringTestImageRepository()
		logger.Infof("Building image: %s", b.osImage)
	}

	b.remoteTmpDir = filepath.Join("/root", e2e.TestContext.OutputDir, fmt.Sprintf("mco-test-%s", exutil.GetRandomString()))
	_, err := b.node.DebugNodeWithChroot("mkdir", "-p", b.remoteTmpDir)
	if err != nil {
		return fmt.Errorf("Error creating tmp dir %s in node %s. Error: %s", b.remoteTmpDir, b.node.GetName(), err)
	}

	if b.remoteKubeconfig == "" {
		b.remoteKubeconfig = filepath.Join(b.remoteTmpDir, "kubeconfig")
	}
	if b.remoteDockerConfig == "" {
		b.remoteDockerConfig = filepath.Join(b.remoteTmpDir, ".dockerconfigjson")
	}
	if b.remoteDockerfile == "" {
		b.remoteDockerfile = filepath.Join(b.remoteTmpDir, "Dockerfile")
	}
	// It is not needed to create a new SA, but we will need it as soon as we support the internal registry
	if b.saRegistryName == "" {
		b.saRegistryName = "test-registry-sa"
		if err := b.createRegistryAdminSA(); err != nil {
			return err
		}
	}

	g.By("Prepare remote docker config file")
	logger.Infof("Copy cluster config.json file")
	_, cpErr := b.node.DebugNodeWithChroot("cp", "/var/lib/kubelet/config.json", b.remoteDockerConfig)
	if cpErr != nil {
		logger.Errorf("Error copying cluster config.json file to a temporary directory")
		return cpErr
	}

	logger.Infof("OK!\n")

	return nil
}

// createRegistryAdminSA creates an auxiliary service account with registry-admin permissions
func (b *OsImageBuilderInNode) createRegistryAdminSA() error {
	cErr := b.node.oc.Run("create").Args("serviceaccount", b.saRegistryName).Execute()
	if cErr != nil {
		return fmt.Errorf("Error creating ServiceAccount %s: %s", b.saRegistryName, cErr)
	}

	err := b.node.oc.Run("adm").Args("policy", "add-cluster-role-to-user", "registry-admin", "-z", b.saRegistryName).Execute()
	if err != nil {
		return fmt.Errorf("Error creating ServiceAccount %s: %s", b.saRegistryName, err)
	}
	return nil
}

// DeleteRegistryAdminSA delete the auxiliary service account
func (b *OsImageBuilderInNode) DeleteRegistryAdminSA() error {
	cErr := b.node.oc.Run("delete").Args("serviceaccount", b.saRegistryName, "--ignore-not-found=true").Execute()
	if cErr != nil {
		return fmt.Errorf("Error creating ServiceAccount %s: %s", b.saRegistryName, cErr)
	}

	return nil
}

func (b *OsImageBuilderInNode) buildImage() error {
	g.By("Get base osImage locally")
	baseImage, err := getImageFromReleaseInfo(b.node.oc.AsAdmin(), LayeringBaseImageReleaseInfo, b.dockerConfig)
	if err != nil {
		return fmt.Errorf("Error getting the base image to build new osImages. Error: %s", err)
	}

	logger.Infof("Base image: %s\n", baseImage)

	g.By("Prepare remote dockerFile directory")
	dockerFile := "FROM " + baseImage + "\n" + b.dockerFileCommands
	logger.Infof(" Using Dockerfile:\n%s", dockerFile)

	dfRemote := NewRemoteFile(b.node, b.remoteDockerfile)
	rfErr := dfRemote.PushNewTextContent(dockerFile)
	if rfErr != nil {
		return fmt.Errorf("Error creating the Dockerfile in the remote node. Error: %s", rfErr)
	}
	logger.Infof("OK!\n")

	g.By("Build osImage")
	podmanCLI := container.NewPodmanCLI()
	buildPath := filepath.Dir(b.remoteDockerfile)
	podmanCLI.ExecCommandPath = buildPath
	switch b.architecture {
	case ArchitectureARM64, ArchitectureAMD64:
		output, err := b.node.DebugNodeWithChroot("podman", "build", buildPath, "--arch", b.architecture, "--tag", b.osImage, "--authfile", b.remoteDockerConfig)
		if err != nil {
			msg := fmt.Sprintf("Podman failed building image %s with architecture %s:\n%s\n%s", b.osImage, b.architecture, output, err)
			logger.Errorf(msg)
			return fmt.Errorf(msg)
		}

		logger.Debugf(output)
	default:
		msg := fmt.Sprintf("Architecture '%s' is not supported. Oly 'arm64' and 'amd64' architectures are supported", b.architecture)
		logger.Errorf(msg)
		return fmt.Errorf(msg)
	}
	logger.Infof("OK!\n")

	return nil
}

func (b *OsImageBuilderInNode) pushImage() error {
	g.By("Push osImage")
	output, err := b.node.DebugNodeWithChroot("podman", "push", b.osImage, "--authfile", b.remoteDockerConfig)
	if err != nil {
		msg := fmt.Sprintf("Podman failed pushing image %s:\n%s\n%s", b.osImage, output, err)
		logger.Errorf(msg)
		return fmt.Errorf(msg)
	}

	logger.Debugf(output)
	logger.Infof("OK!\n")
	return nil
}

func (b *OsImageBuilderInNode) removeImage() error {
	g.By("Remove osImage")
	rmOutput, err := b.node.DebugNodeWithChroot("podman", "rmi", b.osImage)
	if err != nil {
		msg := fmt.Sprintf("Podman failed removing image %s:\n%s\n%s", b.osImage, rmOutput, err)
		logger.Errorf(msg)
		return fmt.Errorf(msg)
	}

	logger.Debugf(rmOutput)
	logger.Infof("OK!\n")
	return nil
}

func (b *OsImageBuilderInNode) digestImage() (string, error) {
	g.By("Digest osImage")
	inspectInfo, _, err := b.node.DebugNodeWithChrootStd("skopeo", "inspect", "docker://"+b.osImage, "--authfile", b.remoteDockerConfig)
	if err != nil {
		msg := fmt.Sprintf("Skopeo failed inspecting image %s:\n%s\n%s", b.osImage, inspectInfo, err)
		logger.Errorf(msg)
		return "", fmt.Errorf(msg)
	}

	logger.Debugf(inspectInfo)

	inspectJSON := JSON(inspectInfo)
	digestedImage := inspectJSON.Get("Name").ToString() + "@" + inspectJSON.Get("Digest").ToString()

	logger.Infof("Image %s was built with architecture %s and pushed properly", b.osImage, b.architecture)
	logger.Infof("Image %s was digested as %s", b.osImage, digestedImage)

	logger.Infof("OK!\n")
	return digestedImage, nil
}

// CreateAndDigestOsImage create the osImage and returns the image digested
func (b *OsImageBuilderInNode) CreateAndDigestOsImage() (string, error) {
	if err := b.prepareEnvironment(); err != nil {
		return "", err
	}
	if err := b.buildImage(); err != nil {
		return "", err
	}
	if err := b.pushImage(); err != nil {
		return "", err
	}
	if err := b.removeImage(); err != nil {
		return "", err
	}
	return b.digestImage()
}
