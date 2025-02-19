package logging

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"github.com/openshift/openshift-tests-private/test/extended/util/clusterinfra"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// Check if credentials exist for STS clusters
func checkAWSCredentials() bool {
	//set AWS_SHARED_CREDENTIALS_FILE from CLUSTER_PROFILE_DIR as the first priority"
	prowConfigDir, present := os.LookupEnv("CLUSTER_PROFILE_DIR")
	if present {
		awsCredFile := filepath.Join(prowConfigDir, ".awscred")
		if _, err := os.Stat(awsCredFile); err == nil {
			err := os.Setenv("AWS_SHARED_CREDENTIALS_FILE", awsCredFile)
			if err == nil {
				e2e.Logf("use CLUSTER_PROFILE_DIR/.awscred")
				return true
			}
		}
	}

	// check if AWS_SHARED_CREDENTIALS_FILE exist
	_, present = os.LookupEnv("AWS_SHARED_CREDENTIALS_FILE")
	if present {
		e2e.Logf("use Env AWS_SHARED_CREDENTIALS_FILE")
		return true
	}

	// check if AWS_SECRET_ACCESS_KEY exist
	_, keyIDPresent := os.LookupEnv("AWS_ACCESS_KEY_ID")
	_, keyPresent := os.LookupEnv("AWS_SECRET_ACCESS_KEY")
	if keyIDPresent && keyPresent {
		e2e.Logf("use Env AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY")
		return true
	}
	// check if $HOME/.aws/credentials exist
	home, _ := os.UserHomeDir()
	if _, err := os.Stat(home + "/.aws/credentials"); err == nil {
		e2e.Logf("use HOME/.aws/credentials")
		return true
	}
	return false
}

func getAWSCredentialFromFile(file string) error {
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	s := strings.Split(string(data), "\n")
	for i := 0; i < len(s); i++ {
		if strings.Contains(s[i], "aws_access_key_id") {
			aws_access_key_id := strings.TrimSpace(strings.Split(s[i], "=")[1])
			os.Setenv("AWS_ACCESS_KEY_ID", aws_access_key_id)
		}
		if strings.Contains(s[i], "aws_secret_access_key") {
			aws_secret_access_key := strings.TrimSpace(strings.Split(s[i], "=")[1])
			os.Setenv("AWS_SECRET_ACCESS_KEY", aws_secret_access_key)
		}
	}
	return nil
}

// get AWS Account ID
func getAwsAccount(stsClient *sts.Client) (string, string) {
	result, err := stsClient.GetCallerIdentity(context.TODO(), &sts.GetCallerIdentityInput{})
	o.Expect(err).NotTo(o.HaveOccurred())
	awsAccount := aws.ToString(result.Account)
	awsUserArn := aws.ToString(result.Arn)
	return awsAccount, awsUserArn
}

func readDefaultSDKExternalConfigurations(ctx context.Context, region string) aws.Config {
	cfg, err := awsConfig.LoadDefaultConfig(ctx,
		awsConfig.WithRegion(region),
	)
	o.Expect(err).NotTo(o.HaveOccurred())
	return cfg
}

// initialize a s3 client with credential
func newS3Client(cfg aws.Config) *s3.Client {
	return s3.NewFromConfig(cfg)
}

// new AWS STS client
func newStsClient(cfg aws.Config) *sts.Client {
	return sts.NewFromConfig(cfg)
}

// Create AWS IAM client
func newIamClient(cfg aws.Config) *iam.Client {
	return iam.NewFromConfig(cfg)
}

// aws iam create-role
func iamCreateRole(iamClient *iam.Client, trustPolicy string, roleName string) string {
	e2e.Logf("Create iam role %v", roleName)
	result, err := iamClient.CreateRole(context.TODO(), &iam.CreateRoleInput{
		AssumeRolePolicyDocument: aws.String(trustPolicy),
		RoleName:                 aws.String(roleName),
	})
	o.Expect(err).NotTo(o.HaveOccurred(), "couldn't create role "+roleName)
	roleArn := aws.ToString(result.Role.Arn)
	return roleArn
}

// aws iam delete-role
func iamDeleteRole(iamClient *iam.Client, roleName string) {
	_, err := iamClient.DeleteRole(context.TODO(), &iam.DeleteRoleInput{
		RoleName: aws.String(roleName),
	})
	if err != nil {
		e2e.Logf("Couldn't delete role %s: %v", roleName, err)
	}
}

// aws iam create-policy
func iamCreatePolicy(iamClient *iam.Client, mgmtPolicy string, policyName string) string {
	e2e.Logf("Create iam policy %v", policyName)
	result, err := iamClient.CreatePolicy(context.TODO(), &iam.CreatePolicyInput{
		PolicyDocument: aws.String(mgmtPolicy),
		PolicyName:     aws.String(policyName),
	})
	o.Expect(err).NotTo(o.HaveOccurred(), "Couldn't create policy"+policyName)
	policyArn := aws.ToString(result.Policy.Arn)
	return policyArn
}

// aws iam delete-policy
func iamDeletePolicy(iamClient *iam.Client, policyArn string) {
	_, err := iamClient.DeletePolicy(context.TODO(), &iam.DeletePolicyInput{
		PolicyArn: aws.String(policyArn),
	})
	if err != nil {
		e2e.Logf("Couldn't delete policy %v: %v", policyArn, err)
	}
}

// This func creates a IAM role, attaches custom trust policy and managed permission policy
func createIAMRoleOnAWS(iamClient *iam.Client, trustPolicy string, roleName string, policyArn string) string {
	result, err := iamClient.CreateRole(context.TODO(), &iam.CreateRoleInput{
		AssumeRolePolicyDocument: aws.String(trustPolicy),
		RoleName:                 aws.String(roleName),
	})
	o.Expect(err).NotTo(o.HaveOccurred(), "Couldn't create role %v", roleName)
	roleArn := aws.ToString(result.Role.Arn)

	//Adding managed permission policy if provided
	if policyArn != "" {
		_, err = iamClient.AttachRolePolicy(context.TODO(), &iam.AttachRolePolicyInput{
			PolicyArn: aws.String(policyArn),
			RoleName:  aws.String(roleName),
		})
		o.Expect(err).NotTo(o.HaveOccurred())
	}
	return roleArn
}

// Deletes IAM role and attached policies
func deleteIAMroleonAWS(iamClient *iam.Client, roleName string) {
	// List attached policies of the IAM role
	listAttachedPoliciesOutput, err := iamClient.ListAttachedRolePolicies(context.TODO(), &iam.ListAttachedRolePoliciesInput{
		RoleName: aws.String(roleName),
	})
	if err != nil {
		e2e.Logf("Error listing attached policies of IAM role %s", roleName)
	}

	if len(listAttachedPoliciesOutput.AttachedPolicies) == 0 {
		e2e.Logf("No attached policies under IAM role: %s", roleName)
	}

	if len(listAttachedPoliciesOutput.AttachedPolicies) != 0 {
		// Detach attached policy from the IAM role
		for _, policy := range listAttachedPoliciesOutput.AttachedPolicies {
			_, err := iamClient.DetachRolePolicy(context.TODO(), &iam.DetachRolePolicyInput{
				RoleName:  aws.String(roleName),
				PolicyArn: policy.PolicyArn,
			})
			if err != nil {
				e2e.Logf("Error detaching policy: %s", *policy.PolicyName)
			} else {
				e2e.Logf("Detached policy: %s", *policy.PolicyName)
			}
		}
	}

	// Delete the IAM role
	_, err = iamClient.DeleteRole(context.TODO(), &iam.DeleteRoleInput{
		RoleName: aws.String(roleName),
	})
	if err != nil {
		e2e.Logf("Error deleting IAM role: %s", roleName)
	} else {
		e2e.Logf("IAM role deleted successfully: %s", roleName)
	}
}

// Create role_arn required for Loki deployment on STS clusters
func createIAMRoleForLokiSTSDeployment(iamClient *iam.Client, oidcName, awsAccountID, partition, lokiNamespace, lokiStackName, roleName string) string {
	policyArn := "arn:" + partition + ":iam::aws:policy/AmazonS3FullAccess"

	lokiTrustPolicy := `{
		"Version": "2012-10-17",
		"Statement": [
			{
				"Effect": "Allow",
				"Principal": {
					"Federated": "arn:%s:iam::%s:oidc-provider/%s"
				},
				"Action": "sts:AssumeRoleWithWebIdentity",
				"Condition": {
					"StringEquals": {
						"%s:sub": [
							"system:serviceaccount:%s:%s",
							"system:serviceaccount:%s:%s-ruler"
						]
					}
				}
			}
		]
	}`
	lokiTrustPolicy = fmt.Sprintf(lokiTrustPolicy, partition, awsAccountID, oidcName, oidcName, lokiNamespace, lokiStackName, lokiNamespace, lokiStackName)
	roleArn := createIAMRoleOnAWS(iamClient, lokiTrustPolicy, roleName, policyArn)
	return roleArn
}

// Creates Loki object storage secret on AWS STS cluster
func createObjectStorageSecretOnAWSSTSCluster(oc *exutil.CLI, region, storageSecret, bucketName, namespace string) {
	err := oc.NotShowInfo().AsAdmin().WithoutNamespace().Run("create").Args("secret", "generic", storageSecret, "--from-literal=region="+region, "--from-literal=bucketnames="+bucketName, "--from-literal=audience=openshift", "-n", namespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Function to check if tenant logs are present under the S3 bucket.
// Returns success if any one of the tenants under tenants[] are found.
func validatesIfLogsArePushedToS3Bucket(s3Client *s3.Client, bucketName string, tenants []string) {
	// Poll to check contents of the s3 bucket
	err := wait.PollUntilContextTimeout(context.Background(), 30*time.Second, 300*time.Second, true, func(context.Context) (done bool, err error) {
		listObjectsOutput, err := s3Client.ListObjectsV2(context.TODO(), &s3.ListObjectsV2Input{
			Bucket: aws.String(bucketName),
		})
		if err != nil {
			return false, err
		}

		for _, object := range listObjectsOutput.Contents {
			for _, tenantName := range tenants {
				if strings.Contains(*object.Key, tenantName) {
					e2e.Logf("Logs %s found under the bucket: %s", *object.Key, bucketName)
					return true, nil
				}
			}
		}
		e2e.Logf("Waiting for data to be available under bucket: %s", bucketName)
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "Timed out...No data is available under the bucket: "+bucketName)
}

// cloudWatchSpec the basic object which describe all common test options
type cloudwatchSpec struct {
	awsRoleName         string
	awsRoleArn          string
	awsRegion           string
	awsPolicyName       string
	awsPolicyArn        string
	awsPartition        string //The partition in which the resource is located, valid when the cluster is STS, ref: https://docs.aws.amazon.com/IAM/latest/UserGuide/reference-arns.html#arns-syntax
	clusterPlatformType string
	collectorSAName     string // the service account for collector pod to use
	cwClient            *cloudwatchlogs.Client
	groupName           string // the strategy for grouping logstreams, for example: '{.log_type||"none"}'
	hasMaster           bool   // wether the cluster has master nodes or not
	iamClient           *iam.Client
	logTypes            []string //default: "['infrastructure','application', 'audit']"
	nodes               []string // Cluster Nodes Names, required when checking infrastructure/audit logs and strict=true
	ovnEnabled          bool     // if ovn is enabled
	secretName          string   // the name of the secret for the collector to use
	secretNamespace     string   // the namespace where the collector pods to be deployed
	stsEnabled          bool     // Is sts enabled on the cluster
	selAppNamespaces    []string //The app namespaces should be collected and verified
	selNamespacesID     []string // The UUIDs of all app namespaces should be collected
	disAppNamespaces    []string //The namespaces should not be collected and verified
}

// Set the default values to the cloudwatchSpec Object, you need to change the default in It if needs
func (cw *cloudwatchSpec) init(oc *exutil.CLI) {
	if checkNetworkType(oc) == "ovnkubernetes" {
		cw.ovnEnabled = true
	}
	cw.hasMaster = hasMaster(oc)
	cw.clusterPlatformType = exutil.CheckPlatform(oc)
	if cw.clusterPlatformType == "aws" {
		if exutil.IsSTSCluster(oc) {
			if !checkAWSCredentials() {
				g.Skip("Skip since no AWS credetials.")
			}
			cw.stsEnabled = true
		} else {
			clusterinfra.GetAwsCredentialFromCluster(oc)
		}
	} else {
		credFile, filePresent := os.LookupEnv("AWS_SHARED_CREDENTIALS_FILE")
		if filePresent {
			err := getAWSCredentialFromFile(credFile)
			if err != nil {
				g.Skip("Skip for the platform is not AWS and can't get credentials from file " + credFile)
			}
		} else {
			_, keyIDPresent := os.LookupEnv("AWS_ACCESS_KEY_ID")
			_, secretKeyPresent := os.LookupEnv("AWS_SECRET_ACCESS_KEY")
			if !keyIDPresent || !secretKeyPresent {
				g.Skip("Skip for the platform is not AWS and there is no AWS credentials set")
			}
		}
	}
	if cw.awsRegion == "" {
		region, _ := exutil.GetAWSClusterRegion(oc)
		if region != "" {
			cw.awsRegion = region
		} else {
			// use us-east-2 as default region
			cw.awsRegion = "us-east-2"
		}
	}
	if cw.stsEnabled {
		//Note: AWS China is not added, and the partition is `aws-cn`.
		if strings.HasPrefix(cw.awsRegion, "us-gov") {
			cw.awsPartition = "aws-us-gov"
		} else {
			cw.awsPartition = "aws"
		}
		//Create IAM roles for cloudwatch
		cw.createIAMCloudwatchRole(oc)
	}
	cw.newCloudwatchClient()
	e2e.Logf("Init cloudwatchSpec done")
}

func (cw *cloudwatchSpec) setGroupName(groupName string) {
	cw.groupName = groupName
}

func (cw *cloudwatchSpec) newCloudwatchClient() {
	cfg, err := awsConfig.LoadDefaultConfig(context.TODO(), awsConfig.WithRegion(cw.awsRegion))
	o.Expect(err).NotTo(o.HaveOccurred())
	// Create a Cloudwatch service client
	cw.cwClient = cloudwatchlogs.NewFromConfig(cfg)
}

func (cw *cloudwatchSpec) newIamClient() {
	cfg, err := awsConfig.LoadDefaultConfig(context.TODO(), awsConfig.WithRegion(cw.awsRegion))
	o.Expect(err).NotTo(o.HaveOccurred())
	cw.iamClient = iam.NewFromConfig(cfg)
}

func (cw *cloudwatchSpec) newIamRole(oc *exutil.CLI) {
	oidcProvider, e := getOIDC(oc)
	o.Expect(e).NotTo(o.HaveOccurred())
	awscfg, err := awsConfig.LoadDefaultConfig(context.TODO(), awsConfig.WithRegion(cw.awsRegion))
	o.Expect(err).NotTo(o.HaveOccurred(), "failed to load AWS configuration")
	stsClient := sts.NewFromConfig(awscfg)
	accountID, _ := getAwsAccount(stsClient)
	trustPolicy := `{
"Version": "2012-10-17",
 "Statement": [
   {
     "Effect": "Allow",
     "Principal": {
       "Federated": "arn:%s:iam::%s:oidc-provider/%s"
     },
     "Action": "sts:AssumeRoleWithWebIdentity",
     "Condition": {
       "StringEquals": {
         "%s:sub": "system:serviceaccount:%s:%s"
       }
     }
   }
 ]
}`
	trustPolicy = fmt.Sprintf(trustPolicy, cw.awsPartition, accountID, oidcProvider, oidcProvider, cw.secretNamespace, cw.collectorSAName)
	cw.awsRoleArn = iamCreateRole(cw.iamClient, trustPolicy, cw.awsRoleName)
}

func (cw *cloudwatchSpec) newIamPolicy() {
	mgmtPolicy := `{
"Version": "2012-10-17",
"Statement": [
     {
         "Effect": "Allow",
         "Action": [
            "logs:CreateLogGroup",
            "logs:CreateLogStream",
            "logs:DescribeLogGroups",
            "logs:DescribeLogStreams",
            "logs:PutLogEvents",
            "logs:PutRetentionPolicy"
         ],
         "Resource": "arn:%s:logs:*:*:*"
     }
   ]
}`
	cw.awsPolicyArn = iamCreatePolicy(cw.iamClient, fmt.Sprintf(mgmtPolicy, cw.awsPartition), cw.awsPolicyName)
}

func (cw *cloudwatchSpec) createIAMCloudwatchRole(oc *exutil.CLI) {
	if os.Getenv("AWS_CLOUDWATCH_ROLE_ARN") != "" {
		cw.awsRoleArn = os.Getenv("AWS_CLOUDWATCH_ROLE_ARN")
		return
	}
	cw.awsRoleName = cw.secretName + "-" + getInfrastructureName(oc)
	cw.awsPolicyName = cw.awsRoleName
	cw.newIamClient()
	e2e.Logf("Created aws iam role: %v", cw.awsRoleName)
	cw.newIamRole(oc)
	cw.newIamPolicy()
	_, err := cw.iamClient.AttachRolePolicy(context.TODO(), &iam.AttachRolePolicyInput{
		PolicyArn: &cw.awsPolicyArn,
		RoleName:  &cw.awsRoleName,
	})
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (cw *cloudwatchSpec) deleteIAMCloudwatchRole() {
	cw.iamClient.DetachRolePolicy(context.TODO(), &iam.DetachRolePolicyInput{
		PolicyArn: aws.String(cw.awsPolicyArn),
		RoleName:  aws.String(cw.awsRoleName),
	},
	)
	iamDeleteRole(cw.iamClient, cw.awsRoleName)
	iamDeletePolicy(cw.iamClient, cw.awsPolicyArn)
}

// Create Cloudwatch Secret. note: use credential files can avoid leak in output
func (cw *cloudwatchSpec) createClfSecret(oc *exutil.CLI) {
	var err error
	if cw.stsEnabled {
		token, _ := oc.AsAdmin().WithoutNamespace().Run("create").Args("token", cw.collectorSAName, "--audience=openshift", "--duration=24h", "-n", cw.secretNamespace).Output()
		err = oc.NotShowInfo().AsAdmin().WithoutNamespace().Run("create").Args("secret", "generic", cw.secretName, "--from-literal=role_arn="+cw.awsRoleArn, "--from-literal=token="+token, "-n", cw.secretNamespace).Execute()
	} else {
		err = oc.NotShowInfo().AsAdmin().WithoutNamespace().Run("create").Args("secret", "generic", cw.secretName, "--from-literal=aws_access_key_id="+os.Getenv("AWS_ACCESS_KEY_ID"), "--from-literal=aws_secret_access_key="+os.Getenv("AWS_SECRET_ACCESS_KEY"), "-n", cw.secretNamespace).Execute()
	}
	o.Expect(err).NotTo(o.HaveOccurred())
}

// trigger DeleteLogGroup. sometimes, the api return success, but the resource are still there. now wait up to 3 minutes to make the delete success as more as possible.
func (cw *cloudwatchSpec) deleteGroups(groupPrefix string) {
	wait.PollUntilContextTimeout(context.Background(), 30*time.Second, 90*time.Second, true, func(context.Context) (done bool, err error) {
		logGroupNames, _ := cw.getLogGroupNames(groupPrefix)
		if len(logGroupNames) == 0 {
			return true, nil
		}
		for _, name := range logGroupNames {
			_, err := cw.cwClient.DeleteLogGroup(context.TODO(), &cloudwatchlogs.DeleteLogGroupInput{LogGroupName: &name})
			if err != nil {
				e2e.Logf("Can't delete log group: %s", name)
			} else {
				e2e.Logf("Log group %s is deleted", name)
			}
		}
		return false, nil
	})
}

// clean the Cloudwatch resources
func (cw *cloudwatchSpec) deleteResources(oc *exutil.CLI) {
	resource{"secret", cw.secretName, cw.secretNamespace}.clear(oc)
	cw.deleteGroups("")
	//delete roles when the role is created in case
	if cw.stsEnabled && os.Getenv("AWS_CLOUDWATCH_ROLE_ARN") == "" {
		cw.deleteIAMCloudwatchRole()
	}
}

// Return Cloudwatch GroupNames
func (cw cloudwatchSpec) getLogGroupNames(groupPrefix string) ([]string, error) {
	var (
		groupNames []string
	)
	if groupPrefix == "" {
		if strings.Contains(cw.groupName, "{") {
			groupPrefix = strings.Split(cw.groupName, "{")[0]
		} else {
			groupPrefix = cw.groupName
		}
	}
	logGroupDesc, err := cw.cwClient.DescribeLogGroups(context.TODO(), &cloudwatchlogs.DescribeLogGroupsInput{
		LogGroupNamePrefix: &groupPrefix,
	})
	if err != nil {
		return groupNames, fmt.Errorf("can't get log groups from cloudwatch: %v", err)
	}
	for _, group := range logGroupDesc.LogGroups {
		groupNames = append(groupNames, *group.LogGroupName)
	}

	nextToken := logGroupDesc.NextToken
	for nextToken != nil {
		logGroupDesc, err = cw.cwClient.DescribeLogGroups(context.TODO(), &cloudwatchlogs.DescribeLogGroupsInput{
			LogGroupNamePrefix: &groupPrefix,
			NextToken:          nextToken,
		})
		if err != nil {
			return groupNames, fmt.Errorf("can't get log groups from cloudwatch: %v", err)
		}
		for _, group := range logGroupDesc.LogGroups {
			groupNames = append(groupNames, *group.LogGroupName)
		}
		nextToken = logGroupDesc.NextToken
	}
	return groupNames, nil
}

func (cw *cloudwatchSpec) waitForLogGroupsAppear(groupPrefix, keyword string) error {
	if groupPrefix == "" {
		if strings.Contains(cw.groupName, "{") {
			groupPrefix = strings.Split(cw.groupName, "{")[0]
		} else {
			groupPrefix = cw.groupName
		}
	}
	err := wait.PollUntilContextTimeout(context.Background(), 30*time.Second, 300*time.Second, true, func(context.Context) (done bool, err error) {
		groups, err := cw.getLogGroupNames(groupPrefix)
		if err != nil {
			e2e.Logf("error getting log groups: %v", err)
			return false, nil
		}
		if len(groups) == 0 {
			e2e.Logf("no log groups match the prefix: %s", groupPrefix)
			return false, nil
		}
		e2e.Logf("the log group names %v", groups)
		if keyword != "" {
			return containSubstring(groups, keyword), nil
		}
		return true, nil
	})
	if err != nil {
		return fmt.Errorf("can't find log groups with prefix: %s", groupPrefix)
	}
	return nil
}

// Get Stream names matching the logTypes and project names.
func (cw *cloudwatchSpec) getLogStreamNames(groupName string, streamPrefix string) ([]string, error) {
	var (
		logStreamNames  []string
		err             error
		logStreamDesc   *cloudwatchlogs.DescribeLogStreamsOutput
		logStreamsInput cloudwatchlogs.DescribeLogStreamsInput
	)

	if streamPrefix == "" {
		logStreamsInput = cloudwatchlogs.DescribeLogStreamsInput{
			LogGroupName: &groupName,
		}
	} else {
		logStreamsInput = cloudwatchlogs.DescribeLogStreamsInput{
			LogGroupName:        &groupName,
			LogStreamNamePrefix: &streamPrefix,
		}
	}
	logStreamDesc, err = cw.cwClient.DescribeLogStreams(context.TODO(), &logStreamsInput)
	if err != nil {
		return logStreamNames, fmt.Errorf("can't get log streams: %v", err)
	}
	for _, stream := range logStreamDesc.LogStreams {
		logStreamNames = append(logStreamNames, *stream.LogStreamName)
	}

	nextToken := logStreamDesc.NextToken
	for nextToken != nil {
		if streamPrefix == "" {
			logStreamsInput = cloudwatchlogs.DescribeLogStreamsInput{
				LogGroupName: &groupName,
				NextToken:    nextToken,
			}
		} else {
			logStreamsInput = cloudwatchlogs.DescribeLogStreamsInput{
				LogGroupName:        &groupName,
				LogStreamNamePrefix: &streamPrefix,
				NextToken:           nextToken,
			}
		}
		logStreamDesc, err = cw.cwClient.DescribeLogStreams(context.TODO(), &logStreamsInput)
		if err != nil {
			return logStreamNames, fmt.Errorf("can't get log streams from cloudwatch: %v", err)
		}
		for _, stream := range logStreamDesc.LogStreams {
			logStreamNames = append(logStreamNames, *stream.LogStreamName)
		}
		nextToken = logStreamDesc.NextToken
	}
	return logStreamNames, nil
}

// In this function, verify if the infra container logs are forwarded to Cloudwatch or not
func (cw *cloudwatchSpec) checkInfraContainerLogs(strict bool) bool {
	var (
		infraLogGroupNames []string
		logStreams         []string
	)
	logGroupNames, err := cw.getLogGroupNames("")
	o.Expect(err).NotTo(o.HaveOccurred())
	if len(logGroupNames) == 0 {
		return false
	}
	if strings.Contains(cw.groupName, "{.log_type") {
		for _, e := range logGroupNames {
			r, _ := regexp.Compile(`.*\.infrastructure$`)
			match := r.MatchString(e)
			if match {
				infraLogGroupNames = append(infraLogGroupNames, e)
			}
		}
	}
	if len(infraLogGroupNames) == 0 {
		infraLogGroupNames = logGroupNames
	}
	e2e.Logf("the log group names for infra container logs are %v", infraLogGroupNames)

	// get all the log streams under the log groups
	for _, group := range infraLogGroupNames {
		streams, _ := cw.getLogStreamNames(group, "")
		for _, stream := range streams {
			if strings.Contains(stream, ".openshift-") {
				logStreams = append(logStreams, stream)
			}
		}
	}

	// when strict=true, return ture if we can find podLogStream for all nodes
	if strict {
		if len(cw.nodes) == 0 {
			e2e.Logf("node name is empty, please get node names at first")
			return false
		}
		for _, node := range cw.nodes {
			if !containSubstring(logStreams, node+".openshift-") {
				e2e.Logf("can't find log stream %s", node+".openshift-")
				return false
			}
		}
		return true
	} else {
		return len(logStreams) > 0
	}
}

// list streams, check streams, provide the log streams in this function?

// In this function, verify the system logs present on Cloudwatch
func (cw *cloudwatchSpec) checkInfraNodeLogs(strict bool) bool {
	var (
		infraLogGroupNames []string
		logStreams         []string
	)
	logGroupNames, err := cw.getLogGroupNames("")
	if err != nil || len(logGroupNames) == 0 {
		return false
	}
	for _, group := range logGroupNames {
		r, _ := regexp.Compile(`.*\.infrastructure$`)
		match := r.MatchString(group)
		if match {
			infraLogGroupNames = append(infraLogGroupNames, group)
		}
	}
	if len(infraLogGroupNames) == 0 {
		infraLogGroupNames = logGroupNames
	}
	e2e.Logf("the infra node log group names are %v", infraLogGroupNames)

	// get all the log streams under the log groups
	for _, group := range infraLogGroupNames {
		streams, _ := cw.getLogStreamNames(group, "")
		for _, stream := range streams {
			if strings.Contains(stream, ".journal.system") {
				logStreams = append(logStreams, stream)
			}
		}
	}
	e2e.Logf("the infrastructure node log streams: %v", logStreams)
	// when strict=true, return ture if we can find log streams from all nodes
	if strict {
		var expectedStreamNames []string
		if len(cw.nodes) == 0 {
			e2e.Logf("node name is empty, please get node names at first")
			return false
		}
		//stream name: ip-10-0-152-69.journal.system
		if cw.clusterPlatformType == "aws" {
			for _, node := range cw.nodes {
				expectedStreamNames = append(expectedStreamNames, strings.Split(node, ".")[0])
			}
		} else {
			expectedStreamNames = append(expectedStreamNames, cw.nodes...)
		}
		for _, name := range expectedStreamNames {
			streamName := name + ".journal.system"
			if !contain(logStreams, streamName) {
				e2e.Logf("can't find log stream %s", streamName)
				return false
			}
		}
		return true
	} else {
		return len(logStreams) > 0
	}
}

// In this function, verify the system logs present on Cloudwatch
func (cw *cloudwatchSpec) infrastructureLogsFound(strict bool) bool {
	return cw.checkInfraContainerLogs(strict) && cw.checkInfraNodeLogs(strict)
}

/*
In this function, verify all type of audit logs can be found.
when strict=false, test pass when all type of audit logs are found
when strict=true,  test pass if any audit log is found.
stream:
ip-10-0-90-156.us-east-2.compute.internal
*/
func (cw *cloudwatchSpec) auditLogsFound(strict bool) bool {
	var (
		auditLogGroupNames []string
		logStreams         []string
	)

	logGroupNames, err := cw.getLogGroupNames("")
	if err != nil || len(logGroupNames) == 0 {
		return false
	}
	for _, e := range logGroupNames {
		r, _ := regexp.Compile(`.*\.audit$`)
		match := r.MatchString(e)
		if match {
			auditLogGroupNames = append(auditLogGroupNames, e)
		}
	}
	if len(auditLogGroupNames) == 0 {
		auditLogGroupNames = logGroupNames
	}
	e2e.Logf("the log group names for audit logs are %v", auditLogGroupNames)

	// stream name: ip-10-0-74-46.us-east-2.compute.internal
	// get all the log streams under the log groups
	for _, group := range auditLogGroupNames {
		streams, _ := cw.getLogStreamNames(group, "")
		logStreams = append(logStreams, streams...)
	}

	// when strict=true, return ture if we can find podLogStream for all nodes
	if strict {
		if len(cw.nodes) == 0 {
			e2e.Logf("node name is empty, please get node names at first")
			return false
		}
		for _, node := range cw.nodes {
			if !containSubstring(logStreams, node) {
				e2e.Logf("can't find log stream from node: %s", node)
				return false
			}
		}
		return true
	} else {
		return len(logStreams) > 0
	}

}

// check if the container logs are grouped by namespace_id
func (cw *cloudwatchSpec) checkLogGroupByNamespaceID() bool {
	var (
		groupPrefix string
	)

	if strings.Contains(cw.groupName, ".kubernetes.namespace_id") {
		groupPrefix = strings.Split(cw.groupName, "{")[0]
	} else {
		e2e.Logf("the group name doesn't contain .kubernetes.namespace_id, no need to call this function")
		return false
	}
	for _, namespaceID := range cw.selNamespacesID {
		groupErr := cw.waitForLogGroupsAppear(groupPrefix, namespaceID)
		if groupErr != nil {
			e2e.Logf("can't find log group named %s", namespaceID)
			return false
		}
	}
	return true
}

// check if the container logs are grouped by namespace_name
func (cw *cloudwatchSpec) checkLogGroupByNamespaceName() bool {
	var (
		groupPrefix string
	)
	if strings.Contains(cw.groupName, ".kubernetes.namespace_name") {
		groupPrefix = strings.Split(cw.groupName, "{")[0]
	} else {
		e2e.Logf("the group name doesn't contain .kubernetes.namespace_name, no need to call this function")
		return false
	}
	for _, namespaceName := range cw.selAppNamespaces {
		groupErr := cw.waitForLogGroupsAppear(groupPrefix, namespaceName)
		if groupErr != nil {
			e2e.Logf("can't find log group named %s", namespaceName)
			return false
		}
	}
	for _, ns := range cw.disAppNamespaces {
		groups, err := cw.getLogGroupNames(groupPrefix)
		if err != nil {
			return false
		}
		if containSubstring(groups, ns) {
			return false
		}
	}
	return true
}

func (cw *cloudwatchSpec) getApplicationLogStreams() ([]string, error) {
	var (
		appLogGroupNames []string
		logStreams       []string
	)

	logGroupNames, err := cw.getLogGroupNames("")
	if err != nil || len(logGroupNames) == 0 {
		return logStreams, err
	}
	for _, e := range logGroupNames {
		r, _ := regexp.Compile(`.*\.application$`)
		match := r.MatchString(e)
		if match {
			appLogGroupNames = append(appLogGroupNames, e)
		}
	}
	if len(appLogGroupNames) == 0 {
		appLogGroupNames = logGroupNames
	}
	e2e.Logf("the log group names for application logs are %v", appLogGroupNames)

	for _, group := range appLogGroupNames {
		streams, _ := cw.getLogStreamNames(group, "")
		for _, stream := range streams {
			if !strings.Contains(stream, "ip-10-0") {
				logStreams = append(logStreams, stream)
			}
		}
	}
	return logStreams, nil
}

// The index to find application logs
// GroupType
//
//	logType: anli48022-gwbb4.application
//	namespaceName:  anli48022-gwbb4.aosqe-log-json-1638788875
//	namespaceUUID:   anli48022-gwbb4.0471c739-e38c-4590-8a96-fdd5298d47ae,uuid.audit,uuid.infrastructure
func (cw *cloudwatchSpec) applicationLogsFound() bool {
	if (len(cw.selAppNamespaces) > 0 || len(cw.disAppNamespaces) > 0) && strings.Contains(cw.groupName, ".kubernetes.namespace_id") {
		return cw.checkLogGroupByNamespaceName()
	}
	if len(cw.selNamespacesID) > 0 {
		return cw.checkLogGroupByNamespaceID()
	}

	logStreams, err := cw.getApplicationLogStreams()
	if err != nil || len(logStreams) == 0 {
		return false
	}
	for _, ns := range cw.selAppNamespaces {
		if !containSubstring(logStreams, ns) {
			e2e.Logf("can't find logs from project %s", ns)
			return false
		}
	}
	for _, ns := range cw.disAppNamespaces {
		if containSubstring(logStreams, ns) {
			e2e.Logf("find logs from project %s, this is not expected", ns)
			return false
		}
	}
	return true
}

// The common function to verify if logs can be found or not. In general, customized the cloudwatchSpec before call this function
func (cw *cloudwatchSpec) logsFound() bool {
	var (
		appLogSuccess   = true
		infraLogSuccess = true
		auditLogSuccess = true
	)

	for _, logType := range cw.logTypes {
		switch logType {
		case "infrastructure":
			err := wait.PollUntilContextTimeout(context.Background(), 30*time.Second, 180*time.Second, true, func(context.Context) (done bool, err error) {
				return cw.infrastructureLogsFound(true), nil
			})
			if err != nil {
				e2e.Logf("can't find infrastructure in given time")
				infraLogSuccess = false
			}
		case "audit":
			err := wait.PollUntilContextTimeout(context.Background(), 30*time.Second, 180*time.Second, true, func(context.Context) (done bool, err error) {
				return cw.auditLogsFound(false), nil
			})
			if err != nil {
				e2e.Logf("can't find audit logs in given time")
				auditLogSuccess = false
			}
		case "application":
			err := wait.PollUntilContextTimeout(context.Background(), 30*time.Second, 180*time.Second, true, func(context.Context) (done bool, err error) {
				return cw.applicationLogsFound(), nil
			})
			if err != nil {
				e2e.Logf("can't find application logs in given time")
				appLogSuccess = false
			}
		}
	}
	return infraLogSuccess && auditLogSuccess && appLogSuccess
}

func (cw *cloudwatchSpec) getLogRecordsByNamespace(limit int32, logGroupName string, namespaceName string) ([]LogEntity, error) {
	var (
		output *cloudwatchlogs.FilterLogEventsOutput
		logs   []LogEntity
	)

	streamNames, streamErr := cw.getLogStreamNames(logGroupName, namespaceName)
	if streamErr != nil {
		return logs, streamErr
	}
	e2e.Logf("the log streams: %v", streamNames)
	err := wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 300*time.Second, true, func(context.Context) (done bool, err error) {
		output, err = cw.filterLogEvents(limit, logGroupName, "", streamNames...)
		if err != nil {
			e2e.Logf("get error when filter events in cloudwatch, try next time")
			return false, nil
		}
		if len(output.Events) == 0 {
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return nil, fmt.Errorf("the query is not completed in 5 minutes or there is no log record matches the query: %v", err)
	}
	for _, event := range output.Events {
		var log LogEntity
		json.Unmarshal([]byte(*event.Message), &log)
		logs = append(logs, log)
	}

	return logs, nil
}

// aws logs filter-log-events --log-group-name logging-47052-qitang-fips-zfpgd.application --log-stream-name-prefix=var.log.pods.e2e-test-logfwd-namespace-x8mzw
func (cw *cloudwatchSpec) filterLogEvents(limit int32, logGroupName, logStreamNamePrefix string, logStreamNames ...string) (*cloudwatchlogs.FilterLogEventsOutput, error) {
	if len(logStreamNamePrefix) > 0 && len(logStreamNames) > 0 {
		return nil, fmt.Errorf("invalidParameterException: logStreamNamePrefix and logStreamNames are specified")
	}
	var (
		err    error
		output *cloudwatchlogs.FilterLogEventsOutput
	)

	if len(logStreamNamePrefix) > 0 {
		output, err = cw.cwClient.FilterLogEvents(context.TODO(), &cloudwatchlogs.FilterLogEventsInput{
			LogGroupName:        &logGroupName,
			LogStreamNamePrefix: &logStreamNamePrefix,
			Limit:               &limit,
		})
	} else if len(logStreamNames) > 0 {
		output, err = cw.cwClient.FilterLogEvents(context.TODO(), &cloudwatchlogs.FilterLogEventsInput{
			LogGroupName:   &logGroupName,
			LogStreamNames: logStreamNames,
			Limit:          &limit,
		})
	}
	return output, err
}
