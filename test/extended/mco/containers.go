package mco

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	g "github.com/onsi/ginkgo"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	container "github.com/openshift/openshift-tests-private/test/extended/util/container"
	logger "github.com/openshift/openshift-tests-private/test/extended/util/logext"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// OsImageBuilder encapsulates the functionality to build custom osImage in the machine running the testcase
type OsImageBuilder struct {
	oc *exutil.CLI
	architecture,
	osImage,
	dockerFileCommands, // Full docker file but the "FROM basOsImage..." that will be calculated
	dockerConfig,
	tmpDir,
	saRegistryName string
}

func (b *OsImageBuilder) prepareEnvironment() error {
	if b.dockerConfig == "" {
		logger.Infof("No docker config file was provided to the osImage builder. Generating a new docker config file")
		g.By("Extract pull-secret")
		pullSecret := GetPullSecret(b.oc.AsAdmin())
		tokenDir, err := pullSecret.Extract()
		if err != nil {
			return fmt.Errorf("Error extracting pull-secret. Error: %s", err)
		}
		logger.Infof("Pull secret has been extracted to: %s\n", tokenDir)
		b.dockerConfig = filepath.Join(tokenDir, ".dockerconfigjson")
	}
	logger.Infof("Using docker config file: %s\n", b.dockerConfig)

	if b.architecture == "" {
		b.architecture = exutil.GetClusterArchitecture(b.oc)
		logger.Infof("Building using architecture: %s", b.architecture)
	}
	if b.osImage == "" {
		b.osImage = getLayeringTestImageRepository()
		logger.Infof("Building image: %s", b.osImage)
	}
	if b.tmpDir == "" {
		b.tmpDir = e2e.TestContext.OutputDir
	}

	if b.saRegistryName == "" {
		b.saRegistryName = "test-registry-sa"
		if err := b.createRegistryAdminSA(); err != nil {
			return err
		}
	}

	return nil
}

func (b *OsImageBuilder) buildImage() error {
	g.By("Build image locally")
	baseImage, err := getImageFromReleaseInfo(b.oc.AsAdmin(), LayeringBaseImageReleaseInfo, b.dockerConfig)
	if err != nil {
		return fmt.Errorf("Error getting the base image to build new osImages. Error: %s", err)
	}

	logger.Infof("Base image: %s\n", baseImage)

	dockerFile := "FROM " + baseImage + "\n" + b.dockerFileCommands
	logger.Infof(" Using Dockerfile:\n%s", dockerFile)

	buildDir, err := prepareDockerfileDirectory(b.tmpDir, dockerFile)
	if err != nil {
		return fmt.Errorf("Error creating the build directory with the Dockerfile. Error: %s", err)
	}

	podmanCLI := container.NewPodmanCLI()
	podmanCLI.ExecCommandPath = buildDir
	switch b.architecture {
	case ArchitectureARM64, ArchitectureAMD64:
		output, err := podmanCLI.Run("build").Args(buildDir, "--arch", b.architecture, "--tag", b.osImage, "--authfile", b.dockerConfig).Output()
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

func (b *OsImageBuilder) pushImage() error {
	if b.osImage == "" {
		return fmt.Errorf("There is no image to be pushed. Wast the osImage built?")
	}
	g.By("Push osImage")
	logger.Infof("Pushing image %s", b.osImage)
	podmanCLI := container.NewPodmanCLI()
	output, err := podmanCLI.Run("push").Args(b.osImage, "--authfile", b.dockerConfig).Output()
	if err != nil {
		msg := fmt.Sprintf("Podman failed pushing image %s:\n%s\n%s", b.osImage, output, err)
		logger.Errorf(msg)
		return fmt.Errorf(msg)
	}

	logger.Debugf(output)
	logger.Infof("OK!\n")
	return nil
}

func (b *OsImageBuilder) removeImage() error {
	if b.osImage == "" {
		return fmt.Errorf("There is no image to be removed. Wast the osImage built?")
	}
	logger.Infof("Removing image %s", b.osImage)

	podmanCLI := container.NewPodmanCLI()
	rmOutput, err := podmanCLI.Run("rmi").Args(b.osImage).Output()
	if err != nil {
		msg := fmt.Sprintf("Podman failed removing image %s:\n%s\n%s", b.osImage, rmOutput, err)
		logger.Errorf(msg)
		return fmt.Errorf(msg)
	}

	logger.Debugf(rmOutput)
	logger.Infof("OK!\n")
	return nil
}

func (b *OsImageBuilder) digestImage() (string, error) {
	if b.osImage == "" {
		return "", fmt.Errorf("There is no image to be digested. Wast the osImage built?")
	}
	skopeoCLI := NewSkopeoCLI().SetAuthFile(b.dockerConfig)
	inspectInfo, err := skopeoCLI.Run("inspect").Args("--tls-verify=false", "docker://"+b.osImage).Output()
	if err != nil {
		msg := fmt.Sprintf("Skopeo failed inspecting image %s:\n%s\n%s", b.osImage, inspectInfo, err)
		logger.Errorf(msg)
		return "", fmt.Errorf(msg)
	}

	logger.Debugf(inspectInfo)

	inspectJSON := JSON(inspectInfo)
	digestedImage := inspectJSON.Get("Name").ToString() + "@" + inspectJSON.Get("Digest").ToString()

	logger.Infof("Image %s was digested as %s", b.osImage, digestedImage)

	return digestedImage, nil
}

// CreateAndDigestOsImage create the osImage and returns the image digested
func (b *OsImageBuilder) CreateAndDigestOsImage() (string, error) {
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

// createRegistryAdminSA creates an auxiliary service account with registry-admin permissions
func (b *OsImageBuilder) createRegistryAdminSA() error {
	cErr := b.oc.Run("create").Args("serviceaccount", b.saRegistryName).Execute()
	if cErr != nil {
		return fmt.Errorf("Error creating ServiceAccount %s: %s", b.saRegistryName, cErr)
	}

	err := b.oc.Run("adm").Args("policy", "add-cluster-role-to-user", "registry-admin", "-z", b.saRegistryName).Execute()
	if err != nil {
		return fmt.Errorf("Error creating ServiceAccount %s: %s", b.saRegistryName, err)
	}
	return nil
}

// DeleteRegistryAdminSA delete the auxiliary service account
func (b *OsImageBuilder) DeleteRegistryAdminSA() error {
	cErr := b.oc.Run("delete").Args("serviceaccount", b.saRegistryName, "--ignore-not-found=true").Execute()
	if cErr != nil {
		return fmt.Errorf("Error creating ServiceAccount %s: %s", b.saRegistryName, cErr)
	}

	return nil
}

func prepareDockerfileDirectory(baseDir, dockerFileContent string) (string, error) {
	layout := "2006_01_02T15-04-05Z"

	directory := filepath.Join(baseDir, fmt.Sprintf("containerbuild-%s", time.Now().Format(layout)))
	if err := os.Mkdir(directory, os.ModePerm); err != nil {
		return "", err
	}

	dockerFile := filepath.Join(directory, "Dockerfile")
	if err := os.WriteFile(dockerFile, []byte(dockerFileContent), 0o644); err != nil {
		return "", err
	}

	return directory, nil
}

func getImageFromReleaseInfo(oc *exutil.CLI, imageName, dockerConfigFile string) (string, error) {
	stdout, stderr, err := oc.Run("adm").Args("release", "info", "--image-for", imageName,
		"--registry-config", dockerConfigFile).Outputs()
	if err != nil {
		logger.Errorf("STDOUT: %s", stdout)
		logger.Errorf("STDERR: %s", stderr)
		return "", err
	}

	return stdout, nil
}

func getLayeringTestImageRepository() string {
	layeringImageRepo, exists := os.LookupEnv(EnvVarLayeringTestImageRepository)

	if !exists {
		layeringImageRepo = DefaultLayeringQuayRepository
	}

	return layeringImageRepo
}
