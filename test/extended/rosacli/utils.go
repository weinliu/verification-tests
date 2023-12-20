package rosacli

import (
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strings"
	"time"

	logger "github.com/openshift/openshift-tests-private/test/extended/util/logext"
	rosacli "github.com/openshift/openshift-tests-private/test/extended/util/rosacli"
)

// Get the clusterID env.
func getClusterIDENVExisted() string {
	return os.Getenv("CLUSTER_ID")
}

// Get the cluster config file
func getClusterConfigFile() string {
	sharedDir := os.Getenv("SHARED_DIR")
	return path.Join(sharedDir, "cluster-config")
}

// Get the cluster config file, for jean chen
func getClusterIDFile() string {
	sharedDir := os.Getenv("SHARED_DIR")
	return path.Join(sharedDir, "cluster-id")
}

func getClusterID() string {
	if _, err := os.Stat(getClusterIDFile()); err != nil {
		return ""
		//panic("Cluster id file not existing")
	}
	fileCont, _ := os.ReadFile(getClusterIDFile())

	return string(fileCont)
}

// Check if the cluster is hosted-cp cluster
func isHostedCPCluster(clusterID string) (bool, error) {
	var rosaClient = rosacli.NewClient()
	rosaClient.Runner.Format("json")
	clusterService := rosaClient.Cluster
	output, err := clusterService.DescribeCluster(clusterID)
	if err != nil {
		logger.Errorf("it met error when describeCluster in isHostedCPCluster is %v", err)
		return false, err
	}
	rosaClient.Runner.CloseFormat()
	jsonData := rosaClient.Parser.JsonData.Input(output).Parse()
	return jsonData.DigBool("hypershift", "enabled"), nil
}

// Check if the cluster is sts cluster. hosted-cp cluster is also treated as sts cluster
func isSTSCluster(clusterID string) (bool, error) {
	var rosaClient = rosacli.NewClient()
	rosaClient.Runner.Format("json")
	clusterService := rosaClient.Cluster
	output, err := clusterService.DescribeCluster(clusterID)
	if err != nil {
		logger.Errorf("it met error when describeCluster in isHostedCPCluster is %v", err)
		return false, err
	}
	rosaClient.Runner.CloseFormat()
	jsonData := rosaClient.Parser.JsonData.Input(output).Parse()
	return jsonData.DigBool("aws", "sts", "enabled"), nil
}

// Check if the cluster is using reusable oidc-config
func isUsingReusableOIDCConfig(clusterID string) (bool, error) {
	var rosaClient = rosacli.NewClient()
	rosaClient.Runner.Format("json")
	clusterService := rosaClient.Cluster
	output, err := clusterService.DescribeCluster(clusterID)
	if err != nil {
		logger.Errorf("it met error when describeCluster in isHostedCPCluster is %v", err)
		return false, err
	}
	rosaClient.Runner.CloseFormat()
	jsonData := rosaClient.Parser.JsonData.Input(output).Parse()
	return jsonData.DigBool("aws", "sts", "oidc_config", "reusable"), nil
}

// Check if the cluster is private cluster
func isPrivateCluster(clusterID string) (bool, error) {
	var rosaClient = rosacli.NewClient()
	rosaClient.Runner.Format("json")
	clusterService := rosaClient.Cluster
	output, err := clusterService.DescribeCluster(clusterID)
	if err != nil {
		logger.Errorf("it met error when describeCluster in isPrivateCluster is %v", err)
		return false, err
	}
	rosaClient.Runner.CloseFormat()
	jsonData := rosaClient.Parser.JsonData.Input(output).Parse()
	return jsonData.DigString("api", "listening") == "internal", nil
}

// Get installer role arn from ${SHARED_DIR}/account-roles-arns
func getInstallerRoleArn(hostedcp bool) (string, error) {
	sharedDIR := os.Getenv("SHARED_DIR")
	filePath := sharedDIR + "/account-roles-arns"
	fileContents, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(fileContents), "\n")
	for i := range lines {
		if hostedcp && strings.Contains(lines[i], "-HCP-ROSA-Installer-Role") {
			return lines[i], nil
		}
		if !hostedcp && !strings.Contains(lines[i], "-ROSA-Installer-Role") && strings.Contains(lines[i], "-Installer-Role") {
			return lines[i], nil
		}
		continue
	}
	return "", nil
}

// Split resources from the aws arn
func splitARNResources(v string) []string {
	var parts []string
	var offset int

	for offset <= len(v) {
		idx := strings.IndexAny(v[offset:], "/:")
		if idx < 0 {
			parts = append(parts, v[offset:])
			break
		}
		parts = append(parts, v[offset:idx+offset])
		offset += idx + 1
	}
	return parts
}

// Extract the oidc provider ARN from the output of `rosa create oidc-config --mode auto` and also for common message containing the arn
func extractOIDCProviderARN(output string) string {
	oidcProviderArnRE := regexp.MustCompile(`arn:aws:iam::[^']+:oidc-provider/[^']+`)
	submatchall := oidcProviderArnRE.FindAllString(output, -1)
	if len(submatchall) < 1 {
		logger.Warnf("Cannot find sub string matached %s from input string %s! Please check the matching string", oidcProviderArnRE, output)
	}
	if len(submatchall) > 1 {
		logger.Warnf("Find more than one sub string matached %s! Please check this unexpexted result then update the regex if needed.", oidcProviderArnRE)
	}
	return submatchall[0]
}

// Extract the oidc provider ARN from the output of `rosa create oidc-config --mode auto` and also for common message containing the arn
func extractOIDCProviderIDFromARN(arn string) string {
	spliptElements := splitARNResources(arn)
	return spliptElements[len(spliptElements)-1]
}

// Get the oidc id by the provider url
func getOIDCIdFromList(providerURL string) (string, error) {
	var rosaClient = rosacli.NewClient()
	ocmResourceService := rosaClient.OCMResource
	oidcConfigList, _, err := ocmResourceService.ListOIDCConfig()
	if err != nil {
		return "", err
	}
	for _, item := range oidcConfigList.OIDCConfigList {
		if strings.Contains(item.IssuerUrl, providerURL) {
			return item.ID, nil
		}
	}
	logger.Warnf("No oidc with the url %s is found.", providerURL)
	return "", nil
}

// Remove an string element from slice
func removeStringElementFromArray(slice []string, element string) []string {
	var newSlice []string
	for _, v := range slice {
		if v != element {
			newSlice = append(newSlice, v)
		}
	}
	return newSlice
}

// Write string to a file
func createFileWithContent(filename string, content string) (string, error) {
	err := os.WriteFile(filename, []byte(content), 0644)
	if err != nil {
		logger.Errorf("Failed to write to file: %s", err)
		return "", err
	}
	absPath, err := os.Getwd()
	if err != nil {
		logger.Errorf("Failed to get absolute path: %s", err)
	}
	fileAbsPath := absPath + "/" + filename
	return fileAbsPath, err
}

// Generate random string
func generateRandomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	rand.Seed(time.Now().UnixNano())

	s := make([]byte, n)
	for i := range s {
		s[i] = letters[rand.Intn(len(letters))]
	}
	return string(s)
}

// Generate htpasspwd key value pair, return with a string
func generateHtpasswdPair(user string, pass string) (string, string, string, error) {
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
func generateMultipleHtpasswdPairs(pairNum int) ([]string, error) {
	multipleuserPasswd := []string{}
	for i := 0; i < pairNum; i++ {
		userPasswdPair, _, _, err := generateHtpasswdPair(generateRandomString(6), generateRandomString(6))
		if err != nil {
			return multipleuserPasswd, err
		}
		multipleuserPasswd = append(multipleuserPasswd, userPasswdPair)
	}
	return multipleuserPasswd, nil
}

var defaultClassicWorkerPool = "worker"
