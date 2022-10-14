package mco

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	container "github.com/openshift/openshift-tests-private/test/extended/util/container"
	logger "github.com/openshift/openshift-tests-private/test/extended/util/logext"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

func prepareDockerfileDirectory(dockerFileContent string) (string, error) {
	layout := "2006_01_02T15-04-05Z"

	directory := filepath.Join(e2e.TestContext.OutputDir, fmt.Sprintf("containerbuild-%s", time.Now().Format(layout)))
	if err := os.Mkdir(directory, os.ModePerm); err != nil {
		return "", err
	}

	dockerFile := filepath.Join(directory, "Dockerfile")
	if err := os.WriteFile(dockerFile, []byte(dockerFileContent), 0o644); err != nil {
		return "", err
	}

	return directory, nil
}

func buildPushImage(architecture, buildPath, imageTag, tokenDir string) error {

	podmanCLI := container.NewPodmanCLI()
	podmanCLI.ExecCommandPath = buildPath
	switch architecture {
	case "arm64", "amd64":
		output, err := podmanCLI.Run("build").Args(buildPath, "--arch", architecture, "--tag", imageTag, "--authfile", fmt.Sprintf("%s/.dockerconfigjson", tokenDir)).Output()
		if err != nil {
			logger.Errorf("Podman failed building image %s with architecture %s:\n%s", imageTag, architecture, output)
			return err
		}

		logger.Debugf(output)
	default:
		msg := fmt.Sprintf("Architecture '%s' is not supported. Oly 'arm64' and 'amd64' architectures are supported", architecture)
		logger.Errorf(msg)
		return fmt.Errorf(msg)
	}

	output, err := podmanCLI.Run("push").Args(imageTag, "--authfile", fmt.Sprintf("%s/.dockerconfigjson", tokenDir)).Output()
	if err != nil {
		logger.Errorf("Podman failed pushing image %s:\n%s", imageTag, output)
		return err
	}

	logger.Debugf(output)

	logger.Infof("Image %s was built with architecture %s and pushed properly", imageTag, architecture)

	return nil
}

func getImageFromRelaseInfo(oc *exutil.CLI, imageName, tokenDir string) (string, error) {

	stdout, stderr, err := oc.Run("adm").Args("release", "info", "-o", fmt.Sprintf(`jsonpath={.references.spec.tags[?(@.name=="%s")].from.name}`, imageName),
		"--registry-config", fmt.Sprintf("%s/.dockerconfigjson", tokenDir)).Outputs()
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

func skopeoInspect(image, tokenDir string) (string, string, error) {
	var (
		stdout, stderr []byte
		exitCode       int
		err            error
	)

	argsString := fmt.Sprintf("inspect --tls-verify=false --config docker://%s --authfile %s/.dockerconfigjson", image, tokenDir)

	logger.Infof("Executing command: skopeo %s", argsString)
	args := strings.Split(argsString, " ")
	stdout, err = exec.Command("skopeo", args...).Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr = exitErr.Stderr
			exitCode = exitErr.ExitCode()
			return string(stdout), string(stderr), fmt.Errorf("Skopeo command [skopeo %s] failed with return code %d.\nStdout: %s\nStderr: %s",
				argsString, exitCode, stdout, stderr)
		}
		return string(stdout), "", err
	}

	return string(stdout), string(stderr), nil
}
