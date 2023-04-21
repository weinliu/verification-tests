package mco

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/openshift/openshift-tests-private/test/extended/util/architecture"

	g "github.com/onsi/ginkgo/v2"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	container "github.com/openshift/openshift-tests-private/test/extended/util/container"
	logger "github.com/openshift/openshift-tests-private/test/extended/util/logext"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// OsImageBuilderInNode encapsulates the functionality to build custom osImages inside a cluster node
type OsImageBuilderInNode struct {
	node         Node
	architecture architecture.Architecture
	osImage,
	dockerFileCommands, // Full docker file but the "FROM basOsImage..." that will be calculated
	dockerConfig,
	tmpDir,
	remoteTmpDir,
	remoteKubeconfig,
	remoteDockerConfig,
	remoteDockerfile string
	UseInternalRegistry bool
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

	b.architecture = architecture.ClusterArchitecture(b.node.oc)
	logger.Infof("Building using architecture: %s", b.architecture)

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

	g.By("Prepare remote docker config file")
	logger.Infof("Copy cluster config.json file")
	_, cpErr := b.node.DebugNodeWithChroot("cp", "/var/lib/kubelet/config.json", b.remoteDockerConfig)
	if cpErr != nil {
		logger.Errorf("Error copying cluster config.json file to a temporary directory")
		return cpErr
	}

	if b.UseInternalRegistry {
		b.osImage = fmt.Sprintf("%s/%s/%s", InternalRegistrySvcURL, layeringImagestreamNamespace, "layering")
		if err := b.preparePushToInternalRegistry(); err != nil {
			return err
		}
	} else if b.osImage == "" {
		b.osImage = getLayeringTestImageRepository()
	}
	logger.Infof("Building image: %s", b.osImage)

	logger.Infof("OK!\n")

	return nil
}

func (b *OsImageBuilderInNode) preparePushToInternalRegistry() error {
	logger.Infof("Create namespace to store the imagestream")
	nsExistsErr := b.node.oc.Run("get").Args("namespace", layeringImagestreamNamespace).Execute()
	if nsExistsErr != nil {
		err := b.node.oc.Run("create").Args("namespace", layeringImagestreamNamespace).Execute()
		if err != nil {
			return fmt.Errorf("Error creating namespace %s to store the layering imagestreams. Error: %s",
				layeringImagestreamNamespace, err)
		}
	} else {
		logger.Infof("Namespace %s already exists. Skip namespace creation", layeringImagestreamNamespace)
	}

	logger.Infof("Create registry admin service account to store the imagestream")
	saExistsErr := b.node.oc.Run("get").Args("-n", layeringImagestreamNamespace, "serviceaccount", layeringRegistryAdminSAName).Execute()
	if saExistsErr != nil {
		cErr := b.node.oc.Run("create").Args("-n", layeringImagestreamNamespace, "serviceaccount", layeringRegistryAdminSAName).Execute()
		if cErr != nil {
			return fmt.Errorf("Error creating ServiceAccount %s/%s: %s", layeringImagestreamNamespace, layeringRegistryAdminSAName, cErr)
		}
	} else {
		logger.Infof("SA %s/%s already exists. Skip SA creation", layeringImagestreamNamespace, layeringRegistryAdminSAName)
	}

	admErr := b.node.oc.Run("adm").Args("-n", layeringImagestreamNamespace, "policy", "add-cluster-role-to-user", "registry-admin", "-z", layeringRegistryAdminSAName).Execute()
	if admErr != nil {
		return fmt.Errorf("Error creating ServiceAccount %s: %s", layeringRegistryAdminSAName, admErr)
	}

	logger.Infof("Get SA token")
	saToken, err := b.node.oc.Run("create").Args("-n", layeringImagestreamNamespace, "token", layeringRegistryAdminSAName).Output()
	if err != nil {
		logger.Errorf("Error getting token for SA %s", layeringRegistryAdminSAName)
		return err
	}
	logger.Debugf("SA TOKEN: %s", saToken)
	logger.Infof("OK!\n")

	logger.Infof("Loging as registry admin to internal registry")
	loginOut, loginErr := b.node.DebugNodeWithChroot("podman", "login", InternalRegistrySvcURL, "-u", layeringRegistryAdminSAName, "-p", saToken, "--authfile", b.remoteDockerConfig)
	if loginErr != nil {
		return fmt.Errorf("Error trying to login to internal registry:\nOutput:%s\nError:%s", loginOut, loginErr)
	}
	logger.Infof("OK!\n")

	return nil
}

// CleanUp will clean up all the helper resources created by the builder
func (b *OsImageBuilderInNode) CleanUp() error {
	logger.Infof("Cleanup image builder resources")
	if b.UseInternalRegistry {
		logger.Infof("Removing namespace %s", layeringImagestreamNamespace)
		err := b.node.oc.Run("delete").Args("namespace", layeringImagestreamNamespace, "--ignore-not-found").Execute()
		if err != nil {
			return fmt.Errorf("Error creating namespace %s to store the layering imagestreams. Error: %s",
				layeringImagestreamNamespace, err)
		}

	} else {
		logger.Infof("Not using internal registry, nothing to clean")
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
	dockerFile := "FROM " + baseImage + "\n" + b.dockerFileCommands + "\n" + ExpirationDokerfileLabel
	logger.Infof(" Using Dockerfile:\n%s", dockerFile)

	localBuildDir, err := prepareDockerfileDirectory(b.tmpDir, dockerFile)
	if err != nil {
		return fmt.Errorf("Error creating the build directory with the Dockerfile. Error: %s", err)
	}

	cpErr := b.node.CopyFromLocal(filepath.Join(localBuildDir, "Dockerfile"), b.remoteDockerfile)
	if cpErr != nil {
		return fmt.Errorf("Error creating the Dockerfile in the remote node. Error: %s", cpErr)
	}
	logger.Infof("OK!\n")

	g.By("Build osImage")
	podmanCLI := container.NewPodmanCLI()
	buildPath := filepath.Dir(b.remoteDockerfile)
	podmanCLI.ExecCommandPath = buildPath
	switch b.architecture {
	case architecture.AMD64, architecture.ARM64, architecture.PPC64LE, architecture.S390X:
		output, err := b.node.DebugNodeWithChroot("podman", "build", buildPath, "--arch", b.architecture.String(), "--tag", b.osImage, "--authfile", b.remoteDockerConfig)
		if err != nil {
			msg := fmt.Sprintf("Podman failed building image %s with architecture %s:\n%s\n%s", b.osImage, b.architecture, output, err)
			logger.Errorf(msg)
			return fmt.Errorf(msg)
		}

		logger.Debugf(output)
	default:
		msg := fmt.Sprintf("Architecture '%s' is not supported. Oly 'arm64' and 'amd64' architectures are supported", b.architecture.String())
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

	// If we don't have permissions to push the image, the `oc debug` command will not return an error
	//  so we need to check manually that there is no unauthorized error
	if strings.Contains(output, "unauthorized: access to the requested resource is not authorized") {
		msg := fmt.Sprintf("Podman was not authorized to push the image %s:\n%s\n", b.osImage, output)
		logger.Errorf(msg)
		return fmt.Errorf(msg)
	}

	logger.Debugf(output)
	logger.Infof("OK!\n")
	return nil
}

func (b *OsImageBuilderInNode) removeImage() error {
	g.By("Remove osImage")
	rmOutput, err := b.node.DebugNodeWithChroot("podman", "rmi", "-i", b.osImage)
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
