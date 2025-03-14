package oap

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

const (
	esoSubName   = "external-secrets-operator"
	esoNamespace = "external-secrets-operator"
	//operatorLabel          = "name=cert-manager-operator"
	esoDeploymentName = "external-secrets-operator-controller-manager"
	//controllerNamespace    = "cert-manager"
	cesLabel     = "app.kubernetes.io/name=external-secrets"
	webhookLabel = "app.kubernetes.io/name=external-secrets-webhook"
	//operandNamespace       = "cert-manager"
	//operandLabel           = "app.kubernetes.io/instance=cert-manager"
	//defaultOperandPodNum = 1
)

// Create External Secrets Operator
func createExternalSecretsOperator(oc *exutil.CLI) {
	var (
		testMode    = "PROD"
		channelName = "stable"
	)
	e2e.Logf("=> create the operator namespace")
	buildPruningBaseDir := exutil.FixturePath("testdata", "oap/eso")
	namespaceFile := filepath.Join(buildPruningBaseDir, "namespace.yaml")
	output, err := oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", namespaceFile).Output()
	// skip the install process to mitigate the namespace deletion terminating issue caused by case 62006
	// the full message is 'Detected changes to resource external-secrets-operator which is currently being deleted'
	if strings.Contains(output, "being deleted") {
		g.Skip("skip the install process as the external-secrets-operator namespace is being terminated due to other env issue e.g. we ever hit such failures caused by OCPBUGS-31443")
	}
	if err != nil && !strings.Contains(output, "AlreadyExists") {
		e2e.Failf("Failed to apply namespace external-secrets-operator: %v", err)
	}

	// use the default catalogsource if 'qe-app-registry' doesn't exists
	output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-marketplace", "catalogsource", "qe-app-registry").Output()
	if strings.Contains(output, "NotFound") || err != nil {
		testMode = "PROD"
	}

	var catalogSourceName, catalogSourceNamespace string
	if testMode == "PROD" {
		catalogSourceName = "community-operators"
		catalogSourceNamespace = "openshift-marketplace"

		// skip if the default catalogsource doesn't contain subscription's packagemanifest
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("packagemanifest", "-n", catalogSourceNamespace, "-l", "catalog="+catalogSourceName, "--field-selector", "metadata.name="+esoSubName).Output()
		if !strings.Contains(output, esoSubName) || err != nil {
			g.Skip("skip since no available packagemanifest was found")
		}
	} else if testMode == "STAGE" {
		// TODO: until the Konflux pipeline is configured to add a deterministic tag, update this pull spec whenever a new stage build is out
		fbcImage := "quay.io/redhat-user-workloads/cert-manager-oape-tenant/cert-manager-operator-1-15/cert-manager-operator-fbc-1-15:edb257f20c2c0261200e2f0f7bf8118099567745"
		catalogSourceName = "konflux-fbc-external-secrets"
		catalogSourceNamespace = esoNamespace
		e2e.Logf("=> create the file-based catalog")
		createFBC(oc, catalogSourceName, catalogSourceNamespace, fbcImage)
	} else {
		e2e.Failf("Invalid Test Mode, it should be 'PROD' or 'STAGE'")
	}
	e2e.Logf("=> using catalogsource '%s' from namespace '%s'", catalogSourceName, catalogSourceNamespace)

	e2e.Logf("=> create the operatorgroup")
	operatorGroupFile := filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
	err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", operatorGroupFile).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())

	e2e.Logf("=> create the subscription")
	subscriptionTemplate := filepath.Join(buildPruningBaseDir, "subscription.yaml")
	params := []string{"-f", subscriptionTemplate, "-p", "NAME=" + esoSubName, "SOURCE=" + catalogSourceName, "SOURCE_NAMESPACE=" + catalogSourceNamespace, "CHANNEL=" + channelName}
	exutil.ApplyNsResourceFromTemplate(oc, esoNamespace, params...)
	// Wait for subscription state to become AtLatestKnown
	err = wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 180*time.Second, true, func(context.Context) (bool, error) {
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", esoSubName, "-n", esoNamespace, "-o=jsonpath={.status.state}").Output()
		if strings.Contains(output, "AtLatestKnown") {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		dumpResource(oc, esoNamespace, "sub", esoSubName, "-o=jsonpath={.status}")
	}
	exutil.AssertWaitPollNoErr(err, "timeout waiting for subscription state to become AtLatestKnown")

	e2e.Logf("Create OperatorConfig")
	operatorConfig := filepath.Join(buildPruningBaseDir, "operatorconfig.yaml")
	err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", operatorConfig, "-n", "external-secrets-operator").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())

	e2e.Logf("=> retrieve the installed CSV name")
	csvName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", esoSubName, "-n", esoNamespace, "-o=jsonpath={.status.installedCSV}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(csvName).NotTo(o.BeEmpty())
	// Wait for csv phase to become Succeeded
	err = wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 180*time.Second, true, func(context.Context) (bool, error) {
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", csvName, "-n", esoNamespace, "-o=jsonpath={.status.phase}").Output()
		if strings.Contains(output, "Succeeded") {
			e2e.Logf("csv '%s' installed successfully", csvName)
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		dumpResource(oc, esoNamespace, "csv", csvName, "-o=jsonpath={.status}")
	}
	exutil.AssertWaitPollNoErr(err, "timeout waiting for csv phase to become Succeeded")

	//Wait for pods phase to become Running
	err = wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 300*time.Second, true, func(context.Context) (bool, error) {
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", esoNamespace, "-l", cesLabel, `-o=jsonpath={..status.conditions[?(@.type=="Ready")].status}`).Output()
		if output == "True" {
			e2e.Logf("cluster-external-secrets pod is running!")
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", esoNamespace, "-l", cesLabel).Execute()
	}
	exutil.AssertWaitPollNoErr(err, "timeout waiting for cluster-external-secrets pod phase to become Running")

	err = wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 300*time.Second, true, func(context.Context) (bool, error) {
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", esoNamespace, "-l", webhookLabel, `-o=jsonpath={..status.conditions[?(@.type=="Ready")].status}`).Output()
		if output == "True" {
			e2e.Logf("cluster-external-secrets-webhook pod is running!")
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", esoNamespace, "-l", webhookLabel).Execute()
	}
	exutil.AssertWaitPollNoErr(err, "timeout waiting for cluster-external-secrets-webhook pod phase to become Running")

}

// GetAWSSecret retrieves a secret from AWS Secrets Manager
func GetSecretAWS(accessKeyID, secureKey, region, secretName string) (string, error) {

	awsConfig, err := config.LoadDefaultConfig(
		context.TODO(),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKeyID, secureKey, "")),
		config.WithRegion(region),
	)
	if err != nil {
		return "", fmt.Errorf("failed to load AWS config: %v", err)
	}

	svc := secretsmanager.NewFromConfig(awsConfig)
	result, err := svc.GetSecretValue(context.TODO(), &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(secretName),
	})
	if err != nil {
		return "", fmt.Errorf("failed to get secret: %v", err)
	}

	if result.SecretString == nil {
		return "", fmt.Errorf("secret value is nil")
	}

	return *result.SecretString, nil
}

// UpdateSecret updates the value of a secret in AWS Secrets Manager
func UpdateSecretAWS(accessKeyID, secureKey, region, secretName, newSecretValue string) error {

	awsConfig, err := config.LoadDefaultConfig(
		context.TODO(),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKeyID, secureKey, "")),
		config.WithRegion(region),
	)
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %v", err)
	}

	svc := secretsmanager.NewFromConfig(awsConfig)
	_, err = svc.UpdateSecret(context.TODO(), &secretsmanager.UpdateSecretInput{
		SecretId:     aws.String(secretName),
		SecretString: aws.String(newSecretValue),
	})
	if err != nil {
		return fmt.Errorf("failed to update secret: %v", err)
	}
	e2e.Logf("Secret updated successfully!")
	return nil
}

// GetSecretValueByKeyAWS retrieve a specific secret value from AWS Secrets Manager
func GetSecretValueByKeyAWS(accessKeyID, secureKey, region, secretName, key string) (string, error) {

	secretValue, err := GetSecretAWS(accessKeyID, secureKey, region, secretName)
	if err != nil {
		return "", err
	}
	e2e.Logf("Secret Value: %v", secretValue)

	var secretData map[string]string
	if err := json.Unmarshal([]byte(secretValue), &secretData); err != nil {
		return "", fmt.Errorf("failed to parse secret JSON: %v", err)
	}

	// Extract the value of the specified Key
	value, exists := secretData[key]
	if !exists {
		return "", fmt.Errorf("key %v not found in secret", key)
	}

	return value, nil
}

// UpdateSecretValueByKeyAWS update specific fields in AWS Secrets Manager
func UpdateSecretValueByKeyAWS(accessKeyID, secureKey, region, secretName, key, newValue string) error {

	secretValue, err := GetSecretAWS(accessKeyID, secureKey, region, secretName)
	if err != nil {
		return fmt.Errorf("failed to get secret: %v", err)
	}

	var secretData map[string]string
	if err := json.Unmarshal([]byte(secretValue), &secretData); err != nil {
		return fmt.Errorf("failed to parse secret JSON: %v", err)
	}

	secretData[key] = newValue
	updatedSecretValue, err := json.Marshal(secretData)
	if err != nil {
		return fmt.Errorf("failed to encode updated secret JSON: %v", err)
	}

	return UpdateSecretAWS(accessKeyID, secureKey, region, secretName, string(updatedSecretValue))
}
