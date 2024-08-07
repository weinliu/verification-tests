package util

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"

	"github.com/tidwall/gjson"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

const (
	ReleaseImageLatestEnv = "RELEASE_IMAGE_LATEST"
)

func GetLatestReleaseImageFromEnv() string {
	return os.Getenv(ReleaseImageLatestEnv)
}

// GetLatest4StableImage to get the latest 4-stable OCP image from releasestream link
// Return OCP image for sample quay.io/openshift-release-dev/ocp-release:4.11.0-fc.0-x86_64
func GetLatest4StableImage() (string, error) {
	outputCmd, err := exec.Command("bash", "-c", "curl -s -k https://amd64.ocp.releases.ci.openshift.org/api/v1/releasestream/4-stable/latest").Output()
	if err != nil {
		e2e.Logf("Encountered err: %v when trying to curl the releasestream page", err)
		return "", err
	}
	latestImage := gjson.Get(string(outputCmd), `pullSpec`).String()
	e2e.Logf("The latest 4-stable OCP image is %s", latestImage)
	return latestImage, nil
}

func GetLatest4PreviewImage(arch string) (latestImage string, err error) {
	url := map[string]string{
		"amd64": "https://amd64.ocp.releases.ci.openshift.org/api/v1/releasestream/4-dev-preview/latest",
		"arm64": "https://arm64.ocp.releases.ci.openshift.org/api/v1/releasestream/4-dev-preview-arm64/latest",
		"multi": "https://multi.ocp.releases.ci.openshift.org/api/v1/releasestream/4-dev-preview-multi/latest",
	}
	var resp *http.Response
	var body []byte
	resp, err = http.Get(url[arch])
	if err != nil {
		err = fmt.Errorf("fail to get url %v, error: %v", url[arch], err)
		return "", err
	}
	body, err = io.ReadAll(resp.Body)
	defer resp.Body.Close()
	if err != nil {
		err = fmt.Errorf("fail to parse the result, error: %v", err)
	}
	latestImage = gjson.Get(string(body), `pullSpec`).String()
	return latestImage, err
}

// GetLatestNightlyImage to get the latest nightly OCP image from releasestream link
// Input parameter release: OCP release version such as 4.11, 4.9, ..., 4.6
// Return OCP image
func GetLatestNightlyImage(release string) (string, error) {
	var url string
	switch release {
	case "4.16", "4.15", "4.14", "4.13", "4.12", "4.11", "4.10", "4.9", "4.8", "4.7", "4.6":
		url = "https://amd64.ocp.releases.ci.openshift.org/api/v1/releasestream/" + release + ".0-0.nightly/latest"
	default:
		e2e.Logf("Inputted release version %s is not supported. Only versions from 4.16 to 4.6 are supported.", release)
		return "", errors.New("not supported version of payload")
	}
	outputCmd, err := exec.Command("bash", "-c", "curl -s -k "+url).Output()
	if err != nil {
		e2e.Logf("Encountered err: %v when trying to curl the releasestream page", err)
		return "", err
	}
	latestImage := gjson.Get(string(outputCmd), `pullSpec`).String()
	e2e.Logf("The latest nightly OCP image for %s is: %s", release, latestImage)
	return latestImage, nil
}

// GetLatestImage retrieves the pull spec of the latest image satisfying the arch - product - stream combination.
// arch = "amd64", "arm64", "ppc64le", "s390x", "multi"
// product = "ocp", "origin" (i.e. okd, which only supports the amd64 architecture)
// Possible values for the stream parameter depend on arch and product.
// See https://docs.ci.openshift.org/docs/getting-started/useful-links/#services for relevant release status pages.
//
// Examples:
// GetLatestImage("amd64", "ocp", "4.14.0-0.nightly")
// GetLatestImage("arm64", "ocp", "4.14.0-0.nightly-arm64")
// GetLatestImage("amd64", "origin", "4.14.0-0.okd")
func GetLatestImage(arch, product, stream string) (string, error) {
	switch arch {
	case "amd64", "arm64", "ppc64le", "s390x", "multi":
	default:
		return "", fmt.Errorf("unsupported architecture %v", arch)
	}

	switch product {
	case "ocp", "origin":
	default:
		return "", fmt.Errorf("unsupported product %v", product)
	}

	switch {
	case product == "ocp":
	case product == "origin" && arch == "amd64":
	default:
		return "", fmt.Errorf("the product - architecture combination: %v - %v is not supported", product, arch)
	}

	url := fmt.Sprintf("https://%v.%v.releases.ci.openshift.org/api/v1/releasestream/%v/latest",
		arch, product, stream)
	stdout, err := exec.Command("bash", "-c", "curl -s -k "+url).Output()
	if err != nil {
		return "", err
	}
	if !gjson.ValidBytes(stdout) {
		return "", errors.New("curl does not return a valid json")
	}
	latestImage := gjson.GetBytes(stdout, "pullSpec").String()
	e2e.Logf("Found latest image %v for architecture %v, product %v and stream %v", latestImage, arch, product, stream)
	return latestImage, nil
}
