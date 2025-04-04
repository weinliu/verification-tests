package rosacli

import (
	"fmt"
	"os/exec"
	"strings"

	logger "github.com/openshift/openshift-tests-private/test/extended/util/logext"
)

// Generate htpasspwd key value pair, return with a string
func GenerateHtpasswdPair(user string, pass string) (string, string, string, error) {
	generateCMD := fmt.Sprintf("htpasswd -Bbn %s %s", user, pass)
	output, err := exec.Command("bash", "-c", generateCMD).Output()
	htpasswdPair := strings.TrimSpace(string(output))
	parts := strings.SplitN(htpasswdPair, ":", 2)
	if err != nil {
		logger.Errorf("Fail to generate htpasswd file: %v", err)
		return "", "", "", err
	}
	return htpasswdPair, parts[0], parts[1], nil
}

// generate Htpasswd user-password Pairs
func GenerateMultipleHtpasswdPairs(pairNum int) ([]string, error) {
	multipleuserPasswd := []string{}
	for i := 0; i < pairNum; i++ {
		userPasswdPair, _, _, err := GenerateHtpasswdPair(GenerateRandomString(6), GenerateRandomString(6))
		if err != nil {
			return multipleuserPasswd, err
		}
		multipleuserPasswd = append(multipleuserPasswd, userPasswdPair)
	}
	return multipleuserPasswd, nil
}
