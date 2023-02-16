package util

import (
	"errors"
	"os/exec"

	"github.com/tidwall/gjson"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

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

// GetLatestNightlyImage to get the latest nightly OCP image from releasestream link
// Input parameter release: OCP release version such as 4.11, 4.9, ..., 4.6
// Return OCP image
func GetLatestNightlyImage(release string) (string, error) {
	var url string
	switch release {
	case "4.13", "4.12", "4.11", "4.10", "4.9", "4.8", "4.7", "4.6":
		url = "https://amd64.ocp.releases.ci.openshift.org/api/v1/releasestream/" + release + ".0-0.nightly/latest"
	default:
		e2e.Logf("Inputted release version %s is not supported. Only versions from 4.13 to 4.6 are supported.", release)
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
