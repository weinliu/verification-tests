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
	awsCred "github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3Types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"github.com/openshift/openshift-tests-private/test/extended/util/clusterinfra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

// new AWS STS client
func newStsClient(cfg aws.Config) *sts.Client {
	if !checkAWSCredentials() {
		g.Skip("Skip since no AWS credetial! No Env AWS_SHARED_CREDENTIALS_FILE, Env CLUSTER_PROFILE_DIR  or $HOME/.aws/credentials file")
	}
	return sts.NewFromConfig(cfg)
}

// Create AWS IAM client
func newIamClient(cfg aws.Config) *iam.Client {
	if !checkAWSCredentials() {
		g.Skip("Skip since no AWS credetial! No Env AWS_SHARED_CREDENTIALS_FILE, Env CLUSTER_PROFILE_DIR  or $HOME/.aws/credentials file")
	}
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
		e2e.Logf("Couldn't delete role %v: %v\n", roleName, err)
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
		e2e.Logf("Error listing attached policies of IAM role " + roleName)
	}

	if len(listAttachedPoliciesOutput.AttachedPolicies) == 0 {
		e2e.Logf("No attached policies under IAM role: " + roleName)
	}

	if len(listAttachedPoliciesOutput.AttachedPolicies) != 0 {
		// Detach attached policy from the IAM role
		for _, policy := range listAttachedPoliciesOutput.AttachedPolicies {
			_, err := iamClient.DetachRolePolicy(context.TODO(), &iam.DetachRolePolicyInput{
				RoleName:  aws.String(roleName),
				PolicyArn: policy.PolicyArn,
			})
			if err != nil {
				e2e.Logf("Error detaching policy: " + *policy.PolicyName)
			} else {
				e2e.Logf("Detached policy: " + *policy.PolicyName)
			}
		}
	}

	// Delete the IAM role
	_, err = iamClient.DeleteRole(context.TODO(), &iam.DeleteRoleInput{
		RoleName: aws.String(roleName),
	})
	if err != nil {
		e2e.Logf("Error deleting IAM role: " + roleName)
	} else {
		e2e.Logf("IAM role deleted successfully: " + roleName)
	}
}

// Create role_arn required for Loki deployment on STS clusters
func createIAMRoleForLokiSTSDeployment(iamClient *iam.Client, oidcName, awsAccountID, lokiNamespace, lokiStackName, roleName string) string {
	policyArn := "arn:aws:iam::aws:policy/AmazonS3FullAccess"

	lokiTrustPolicy := `{
		"Version": "2012-10-17",
		"Statement": [
			{
				"Effect": "Allow",
				"Principal": {
					"Federated": "arn:aws:iam::%s:oidc-provider/%s"
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
	lokiTrustPolicy = fmt.Sprintf(lokiTrustPolicy, awsAccountID, oidcName, oidcName, lokiNamespace, lokiStackName, lokiNamespace, lokiStackName)
	roleArn := createIAMRoleOnAWS(iamClient, lokiTrustPolicy, roleName, policyArn)
	return roleArn
}

// Creates S3 bucket using STS AssumeRole operation with Temporary credentials and session token
func createObjectStorageSecretWithS3OnSTS(cfg aws.Config, stsClient *sts.Client, s3roleArn, bucketName string) {
	// Check if bucket exists, if yes delete it
	if checkIfS3bucketExistsWithSTS(cfg, stsClient, s3roleArn, bucketName) {
		e2e.Logf("Bucket already exists. Deleting bucket")
		deleteS3bucketWithSTS(cfg, stsClient, s3roleArn, bucketName)
	}

	result, err := stsClient.AssumeRole(context.TODO(), &sts.AssumeRoleInput{
		RoleArn:         &s3roleArn,
		RoleSessionName: aws.String("logging_loki_sts_s3flow"),
	})
	o.Expect(err).NotTo(o.HaveOccurred())

	// Creating S3 client with assumed role
	s3Client := s3.NewFromConfig(cfg, func(options *s3.Options) {
		options.Credentials = awsCred.NewStaticCredentialsProvider(
			*result.Credentials.AccessKeyId,
			*result.Credentials.SecretAccessKey,
			*result.Credentials.SessionToken,
		)
	})

	// Define the input for CreateBucket operation
	createBucketInput := &s3.CreateBucketInput{
		Bucket: aws.String(bucketName),
	}

	// Check if the region is us-east-1, do not specify a location constraint
	if cfg.Region != "us-east-1" {
		createBucketInput.CreateBucketConfiguration = &s3Types.CreateBucketConfiguration{
			LocationConstraint: s3Types.BucketLocationConstraint(cfg.Region),
		}
	}
	_, err = s3Client.CreateBucket(context.TODO(), createBucketInput)
	o.Expect(err).NotTo(o.HaveOccurred(), "Failed to create bucket: "+bucketName)
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
					e2e.Logf("Logs " + *object.Key + " found under the bucket: " + bucketName)
					return true, nil
				}
			}
		}
		e2e.Logf("Waiting for data to be available under bucket: " + bucketName)
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "Timed out...No data is available under the bucket: "+bucketName)
}

// This is an API operation that allows you to request temporary security credentials for accessing AWS
// resources by assuming an IAM (Identity and Access Management) role
func createS3AssumeRole(stsClient *sts.Client, iamClient *iam.Client, lokistackName string) (string, string) {
	_, AWS_USER_ARN := getAwsAccount(stsClient)
	assumeRoleName := lokistackName + "-" + exutil.GetRandomString()

	policyArn := "arn:aws:iam::aws:policy/AmazonS3FullAccess"

	s3trustPolicy := `{
		"Version": "2012-10-17",
		"Statement": [
			{
				"Effect": "Allow",
				"Principal": {
					"Service": "s3.amazonaws.com",
					"AWS": "%s"
				},
				"Action": "sts:AssumeRole"
			}
		]
	}`
	s3trustPolicy = fmt.Sprintf(s3trustPolicy, AWS_USER_ARN)

	s3roleArn := createIAMRoleOnAWS(iamClient, s3trustPolicy, assumeRoleName, policyArn)
	// Waiting for a bit since it takes some time for the role creation to be propogated on AWS env.
	time.Sleep(10 * time.Second)
	return s3roleArn, assumeRoleName
}

func patchLokiOperatorWithAWSRoleArn(oc *exutil.CLI, packageName, lokiNamespace, roleArn string) {
	roleArnPatchConfig := `{
		"spec": {
		  "config": {
			"env": [
			  {
				"name": "ROLEARN",
				"value": "%s"
			  }
			]
		  }
		}
	  }`

	oc.NotShowInfo().AsAdmin().WithoutNamespace().Run("patch").Args("sub", packageName, "-n", lokiNamespace, "-p", fmt.Sprintf(roleArnPatchConfig, roleArn), "--type=merge").Execute()
	waitForPodReadyWithLabel(oc, loNS, "name=loki-operator-controller-manager")
}

// Checks if a specific bucket exists under AWS S3 service
func checkIfS3bucketExistsWithSTS(cfg aws.Config, stsClient *sts.Client, s3assumeRoleArn string, bucketName string) bool {
	assumeRoleInput := &sts.AssumeRoleInput{
		RoleArn:         &s3assumeRoleArn,
		RoleSessionName: aws.String("checks3WithAssumeRole"),
	}

	assumeRoleOutput, err := stsClient.AssumeRole(context.Background(), assumeRoleInput)
	o.Expect(err).NotTo(o.HaveOccurred())

	// Creating S3 client with assumed role
	s3Client := s3.NewFromConfig(cfg, func(options *s3.Options) {
		options.Credentials = awsCred.NewStaticCredentialsProvider(
			*assumeRoleOutput.Credentials.AccessKeyId,
			*assumeRoleOutput.Credentials.SecretAccessKey,
			*assumeRoleOutput.Credentials.SessionToken,
		)
	})

	// Check if the bucket exists
	_, err = s3Client.HeadBucket(context.TODO(), &s3.HeadBucketInput{
		Bucket: &bucketName,
	})
	if err != nil {
		if msg, errCode := err.(*s3Types.NoSuchBucket); errCode {
			// Bucket does not exist
			e2e.Logf("Bucket does not exist:", msg)
			return false
		} else {
			e2e.Logf("Some other error accessing S3..Going ahead with bucket creation")
			return false
		}
	}
	// Bucket exists
	return true
}

// Deletes a specific bucket's contents (if any) and the bucket itself within AWS S3
func deleteS3bucketWithSTS(cfg aws.Config, stsClient *sts.Client, s3assumeRoleArn string, bucketName string) {
	assumeRoleOutput, err := stsClient.AssumeRole(context.TODO(), &sts.AssumeRoleInput{
		RoleArn:         aws.String(s3assumeRoleArn),
		RoleSessionName: aws.String("AssumeRoleSession"),
	})
	o.Expect(err).NotTo(o.HaveOccurred())

	// Creating S3 client with assumed role
	s3Client := s3.NewFromConfig(cfg, func(options *s3.Options) {
		options.Credentials = awsCred.NewStaticCredentialsProvider(
			*assumeRoleOutput.Credentials.AccessKeyId,
			*assumeRoleOutput.Credentials.SecretAccessKey,
			*assumeRoleOutput.Credentials.SessionToken,
		)
	})

	// List objects in the bucket
	listObjectsOutput, err := s3Client.ListObjectsV2(context.TODO(), &s3.ListObjectsV2Input{
		Bucket: aws.String(bucketName),
	})
	o.Expect(err).NotTo(o.HaveOccurred(), "Error listing bucket objects...")

	if len(listObjectsOutput.Contents) != 0 {
		// Delete each object individually
		for _, obj := range listObjectsOutput.Contents {
			_, err := s3Client.DeleteObject(context.TODO(), &s3.DeleteObjectInput{
				Bucket: &bucketName,
				Key:    obj.Key,
			})
			o.Expect(err).NotTo(o.HaveOccurred(), "Error deleting bucket objects...")
		}
	}

	// Delete the bucket
	_, err = s3Client.DeleteBucket(context.TODO(), &s3.DeleteBucketInput{
		Bucket: &bucketName,
	})
	if err != nil {
		// If the bucket isn't empty, it will return an error
		if msg, errCode := err.(*s3Types.NoSuchBucket); errCode {
			e2e.Logf("Bucket does not exist:", msg)
		} else {
			e2e.Failf("Some Error deleting the bucket" + bucketName)
		}
	} else {
		e2e.Logf("Bucket deleted successfully")
	}

}

// cloudWatchSpec the basic object which describe all common test options
type cloudwatchSpec struct {
	clfAccountName    string // ServicAccountName
	secretName        string // `default: "cw-secret"`, the name of the secret for the collector to use
	secretNamespace   string // `default: "openshift-logging"`, the namespace where the clusterloggingfoward deployed
	stsEnabled        bool   //  Is sts Enabled
	stsSecretType     string //  CredentialsCreate|CredentialsRequest|CredentialsCreateSimple, refer to functon createStsSecret
	awsRoleName       string // aws_access_key file
	awsRoleArn        string // aws_access_key file
	awsRegion         string
	awsPolicyName     string
	awsPolicyArn      string
	groupPrefix       string   // the prefix of the cloudwatch group, the default values is the cluster infrastructureName. For example: anli23ovn-fwm5l
	groupType         string   // `default: "logType"`, the group type to classify logs. logType,namespaceName,namespaceUUID
	selNamespacesUUID []string // The UUIDs of all app namespaces should be collected
	//disNamespacesUUID []string // The app namespaces should not be collected
	nodes            []string // Cluster Nodes Names
	ovnEnabled       bool     //`default: "false"`//  if ovn is enabled. default: false
	hasMaster        bool     // if master is enabled.
	logTypes         []string //`default: "['infrastructure','application', 'audit']"` // logTypes in {"application","infrastructure","audit"}
	selAppNamespaces []string //The app namespaces should be collected and verified
	//selInfraNamespaces []string //The infra namespaces should be collected and verified
	disAppNamespaces []string //The namespaces should not be collected and verified
	//selInfraPods       []string // The infra pods should be collected and verified.
	//selAppPods         []string // The app pods should be collected and verified
	//disAppPods         []string // The pods shouldn't be collected and verified
	//selInfraContainres []string // The infra containers should be collected and verified
	//selAppContainres   []string // The app containers should be collected and verified
	//disAppContainers   []string // The containers shouldn't be collected verified
	iamClient *iam.Client
	cwClient  *cloudwatchlogs.Client
}

// The stream present status
type cloudwatchStreamResult struct {
	pattern string
	logType string //container,journal, audit
	exist   bool
}

// Set the default values to the cloudwatchSpec Object, you need to change the default in It if needs
func (cw *cloudwatchSpec) init(oc *exutil.CLI) {
	var err error
	if cw.secretName == "" {
		cw.secretName = "clf-" + getRandomString()
	}
	if cw.secretNamespace == "" {
		cw.secretNamespace = loggingNS
	}
	if cw.clfAccountName == "" {
		cw.clfAccountName = cw.secretName
	}
	if len(cw.nodes) == 0 {
		cw.nodes = getNodeNames(oc, "kubernetes.io/os=linux")
	}
	cw.ovnEnabled = false
	//TBD enable OVN audit Logs
	//if checkNetworkType(oc) == "ovnkubernetes" {
	//	cw.ovnEnabled = true
	//}
	cw.hasMaster = false
	masterNodes, err := oc.AdminKubeClient().CoreV1().Nodes().List(context.Background(), metav1.ListOptions{LabelSelector: "node-role.kubernetes.io/master="})
	if err == nil && len(masterNodes.Items) > 0 {
		cw.hasMaster = true
	} else {
		e2e.Logf("Warning: we can not get the master node status, assume that is a hypershift hosted cluster")
	}
	cw.stsEnabled = exutil.IsSTSCluster(oc)
	if cw.stsEnabled {
		if !checkAWSCredentials() {
			g.Skip("Skip since no AWS credetial! No Env AWS_SHARED_CREDENTIALS_FILE, Env CLUSTER_PROFILE_DIR  or $HOME/.aws/credentials file")
		}
	} else {
		clusterinfra.GetAwsCredentialFromCluster(oc)
	}
	if cw.awsRegion == "" {
		cw.awsRegion, err = exutil.GetAWSClusterRegion(oc)
		if err != nil {
			// use us-east-2 as default region
			cw.awsRegion = "us-east-2"
		}
	}

	if cw.stsSecretType == "" {
		cw.stsSecretType = "CredentialsCreate"
	}
	if cw.stsEnabled {
		//Create IAM roles for cloudwatch
		cw.createIAMCloudwatchRole(oc)
	}
	cw.newCloudwatchClient()
	e2e.Logf("Init cloudwatchSpec done ")
}

func (cw *cloudwatchSpec) setGroupPrefix(groupPrefix string) {
	cw.groupPrefix = groupPrefix
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
	if !checkAWSCredentials() {
		g.Skip("Skip since no AWS credetial! No Env AWS_SHARED_CREDENTIALS_FILE, Env CLUSTER_PROFILE_DIR  or $HOME/.aws/credentials file")
	}
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
       "Federated": "arn:aws:iam::%s:oidc-provider/%s"
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
	trustPolicy = fmt.Sprintf(trustPolicy, accountID, oidcProvider, oidcProvider, cw.secretNamespace, cw.clfAccountName)
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
         "Resource": "arn:aws:logs:*:*:*"
     }
   ]
}`
	cw.awsPolicyArn = iamCreatePolicy(cw.iamClient, mgmtPolicy, cw.awsPolicyName)
}

func (cw *cloudwatchSpec) createIAMCloudwatchRole(oc *exutil.CLI) {
	if os.Getenv("AWS_CLOUDWATCH_ROLE_ARN") != "" {
		cw.awsRoleArn = os.Getenv("AWS_CLOUDWATCH_ROLE_ARN")
		return
	}
	cw.awsRoleName = "logcw-" + getInfrastructureName(oc) + cw.secretName + "-" + getRandomString()
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

func (cw *cloudwatchSpec) createStsSecret(oc *exutil.CLI) error {
	switch cw.stsSecretType {
	case "CredentialsCreate":
		credentialsData := `[default]
sts_regional_endpoints = regional
role_arn = %s
web_identity_token_file = /var/run/secrets/openshift/serviceaccount/token`
		credentialsData = fmt.Sprintf(credentialsData, cw.awsRoleArn)
		return oc.NotShowInfo().AsAdmin().WithoutNamespace().Run("create").Args("secret", "generic", cw.secretName, "--from-literal=credentials="+credentialsData, "-n", cw.secretNamespace).Execute()
	case "CredentialsCreateSimple":
		return oc.NotShowInfo().AsAdmin().WithoutNamespace().Run("create").Args("secret", "generic", cw.secretName, "--from-literal=role_arn="+cw.awsRoleArn, "-n", cw.secretNamespace).Execute()
	case "CredentialsRequest":
		return fmt.Errorf("TBD: Create sts secret via CredentialsRequest")
	default:
		return fmt.Errorf("unsupported stsSecretType : %s", cw.stsSecretType)
	}
}

// Create Cloudwatch Secret. note: use credential files can avoid leak in output
func (cw *cloudwatchSpec) createClfSecret(oc *exutil.CLI) {
	var err error
	if cw.stsEnabled {
		err = cw.createStsSecret(oc)
	} else {
		err = oc.NotShowInfo().AsAdmin().WithoutNamespace().Run("create").Args("secret", "generic", cw.secretName, "--from-literal=aws_access_key_id="+os.Getenv("AWS_ACCESS_KEY_ID"), "--from-literal=aws_secret_access_key="+os.Getenv("AWS_SECRET_ACCESS_KEY"), "-n", cw.secretNamespace).Execute()
	}
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Return Cloudwatch GroupNames
func (cw cloudwatchSpec) getCloudwatchLogGroupNames(groupPrefix string) []string {
	var groupNames []string
	if groupPrefix == "" {
		groupPrefix = cw.groupPrefix
	}
	logGroupDesc, err := cw.cwClient.DescribeLogGroups(context.TODO(), &cloudwatchlogs.DescribeLogGroupsInput{
		LogGroupNamePrefix: &groupPrefix,
	})

	if err != nil {
		e2e.Logf("Warn: DescribeLogGroups failed \n %v", err)
		return groupNames
	}
	for _, group := range logGroupDesc.LogGroups {
		groupNames = append(groupNames, *group.LogGroupName)
	}
	e2e.Logf("Found cloudWatchLog groupNames %v", groupNames)
	return groupNames
}

// trigger DeleteLogGroup. sometimes, the api return success, but the resource are still there. now wait up to 3 minutes to make the delete success as more as possible.
func (cw *cloudwatchSpec) deleteGroups() {
	wait.PollUntilContextTimeout(context.Background(), 30*time.Second, 90*time.Second, true, func(context.Context) (done bool, err error) {
		logGroupNames := cw.getCloudwatchLogGroupNames(cw.groupPrefix)
		if len(logGroupNames) == 0 {
			return true, nil
		}
		for _, name := range logGroupNames {
			e2e.Logf("Delete LogGroup %s", name)
			cw.cwClient.DeleteLogGroup(context.TODO(), &cloudwatchlogs.DeleteLogGroupInput{LogGroupName: &name})
		}
		return false, nil
	})
}

// clean the Cloudwatch resources
func (cw *cloudwatchSpec) deleteResources() {
	cw.deleteGroups()
	//delete roles when the role is created in case
	if cw.stsEnabled && os.Getenv("AWS_CLOUDWATCH_ROLE_ARN") == "" {
		cw.deleteIAMCloudwatchRole()
	}
}

// Get Stream names matching the logTypes and project names.
func (cw *cloudwatchSpec) getCloudwatchLogStreamNames(groupName string, streamPrefix string, projectNames ...string) []string {
	var logStreamNames []string
	var err error
	var logStreamDesc *cloudwatchlogs.DescribeLogStreamsOutput

	if streamPrefix == "" {
		e2e.Logf("list logStream in logGroup %s", groupName)
		logStreamDesc, err = cw.cwClient.DescribeLogStreams(context.TODO(), &cloudwatchlogs.DescribeLogStreamsInput{
			LogGroupName: &groupName,
		})
	} else {
		e2e.Logf("search logStream %s in logGroup %s", streamPrefix, groupName)
		logStreamDesc, err = cw.cwClient.DescribeLogStreams(context.TODO(), &cloudwatchlogs.DescribeLogStreamsInput{
			LogGroupName:        &groupName,
			LogStreamNamePrefix: &streamPrefix,
		})
	}
	if err != nil {
		e2e.Logf("Warn: DescribeLogStreams failed \n %v", err)
		return logStreamNames
	}

	if len(projectNames) == 0 {
		for _, stream := range logStreamDesc.LogStreams {
			logStreamNames = append(logStreamNames, *stream.LogStreamName)
		}
	} else {
		for _, proj := range projectNames {
			for _, stream := range logStreamDesc.LogStreams {
				if strings.Contains(*stream.LogStreamName, proj) {
					logStreamNames = append(logStreamNames, *stream.LogStreamName)
				}
			}
		}
	}
	return logStreamNames
}

// In this function, verify if infra pod logstreams exist in Cloudwatch
func (cw *cloudwatchSpec) infrastructurePodLogsFound(strict bool) bool {
	var logFoundAll = true
	var logFoundOne = false

	//Find infrastructure Log Group
	var infraLogGroupNames []string
	logGroupNames := cw.getCloudwatchLogGroupNames(cw.groupPrefix)
	for _, e := range logGroupNames {
		r, _ := regexp.Compile(`.*\.infrastructure$`)
		match := r.MatchString(e)
		if match {
			infraLogGroupNames = append(infraLogGroupNames, e)
		}
	}
	if len(infraLogGroupNames) == 0 {
		return false
	}

	//Construct the stream search pattern.
	//podLogStream:  ip-10-0-152-69.us-east-2.compute.internal.kubernetes.var.log.pods.openshift-image-registry_image-registry-5dccc4f469-fnbbm_ae43a304-b972-4427-8333-359672194daa.registry.0.log
	var streamsToVerify []*cloudwatchStreamResult
	for _, e := range cw.nodes {
		streamsToVerify = append(streamsToVerify, &cloudwatchStreamResult{pattern: e + ".kubernetes.var.log.pods", logType: "container", exist: false})
		streamsToVerify = append(streamsToVerify, &cloudwatchStreamResult{pattern: e + ".var.log.pods", logType: "container", exist: false})
	}

	// Check if logstream can be found
	for _, streamI := range streamsToVerify {
		logStreams := cw.getCloudwatchLogStreamNames(infraLogGroupNames[0], streamI.pattern)
		if len(logStreams) > 0 {
			streamI.exist = true
			logFoundOne = true
		}
	}
	for _, streamI := range streamsToVerify {
		if !streamI.exist {
			e2e.Logf("can not find the stream matching " + streamI.pattern)
			logFoundAll = false
		}
	}

	// when strict=true, return ture if we can find podLogStream for all nodes
	if strict {
		return logFoundAll
	}
	// when strict=false, return ture if any pod logstream exists
	return logFoundOne
}

// In this function, verify the system logs present on Cloudwatch
func (cw *cloudwatchSpec) infrastructureSystemLogsFound(strict bool) bool {
	var infraLogGroupNames []string
	var logFoundAll = true
	var logFoundOne = false

	//Find infrastructure Log Group
	logGroupNames := cw.getCloudwatchLogGroupNames(cw.groupPrefix)
	for _, e := range logGroupNames {
		r, _ := regexp.Compile(`.*\.infrastructure$`)
		match := r.MatchString(e)
		if match {
			infraLogGroupNames = append(infraLogGroupNames, e)
		}
	}
	if len(infraLogGroupNames) == 0 {
		return false
	}

	//Construct the stream search pattern.
	//journalStream: ip-10-0-152-69.journal.system
	var streamsToVerify []*cloudwatchStreamResult
	for _, e := range cw.nodes {
		streamsToVerify = append(streamsToVerify, &cloudwatchStreamResult{pattern: strings.Split(e, ".")[0] + ".journal.system", logType: "journal", exist: false})
	}

	// Check if logstream can be found
	for _, streamI := range streamsToVerify {
		logStreams := cw.getCloudwatchLogStreamNames(infraLogGroupNames[0], streamI.pattern)
		if len(logStreams) > 0 {
			streamI.exist = true
			logFoundOne = true
		}
	}
	for _, streamI := range streamsToVerify {
		if !streamI.exist {
			e2e.Logf("can not find the stream matching the pattern " + streamI.pattern)
			logFoundAll = false
		}
	}

	// when strict=true, return ture if we can find system LogStream for all nodes
	if strict {
		return logFoundAll
	}
	// when strict=false, return ture if any system logstream exists
	return logFoundOne
}

// In this function, verify the system logs present on Cloudwatch
func (cw *cloudwatchSpec) infrastructureLogsFound(strict bool) bool {
	if strict {
		return cw.infrastructureSystemLogsFound(false) && cw.infrastructurePodLogsFound(false)
	}
	return cw.infrastructureSystemLogsFound(false) || cw.infrastructurePodLogsFound(false)
}

// In this function, verify all type of audit logs can be found.
// LogStream Example:
//
//		anli48022-gwbb4-master-2.k8s-audit.log
//		anli48022-gwbb4-master-2.openshift-audit.log
//		anli48022-gwbb4-master-1.k8s-audit.log
//		ip-10-0-136-31.us-east-2.compute.internal.linux-audit.log
//	  when strict=false, test pass when all type of audit logs are found
//	  when strict=true,  test pass if any audit log is found.
func (cw *cloudwatchSpec) auditLogsFound(strict bool) bool {
	var auditLogGroupNames []string
	var streamsToVerify []*cloudwatchStreamResult

	for _, e := range cw.getCloudwatchLogGroupNames(cw.groupPrefix) {
		r, _ := regexp.Compile(`.*\.audit$`)
		match := r.MatchString(e)
		//match1, _ := regexp.MatchString(".*\\.audit$", e)
		if match {
			auditLogGroupNames = append(auditLogGroupNames, e)
		}
	}

	if len(auditLogGroupNames) == 0 {
		return false
	}

	if cw.hasMaster {
		streamsToVerify = append(streamsToVerify, &cloudwatchStreamResult{pattern: ".k8s-audit.log$", logType: "k8saudit", exist: false})
		streamsToVerify = append(streamsToVerify, &cloudwatchStreamResult{pattern: ".openshift-audit.log$", logType: "ocpaudit", exist: false})
	}
	if cw.ovnEnabled {
		streamsToVerify = append(streamsToVerify, &cloudwatchStreamResult{pattern: ".ovn-audit.log", logType: "ovnaudit", exist: false})
	}
	streamsToVerify = append(streamsToVerify, &cloudwatchStreamResult{pattern: ".linux-audit.log$", logType: "linuxaudit", exist: false})

	//Only search the logstream in which the suffix is audit.log. logStreams can not fetch all records in one batch in larger cluster
	logFoundOne := false
	logStreams := cw.getCloudwatchLogStreamNames(auditLogGroupNames[0], "")
	for _, streamI := range streamsToVerify {
		for _, streamName := range logStreams {
			match, _ := regexp.MatchString(streamI.pattern, streamName)
			if match {
				streamI.exist = true
				logFoundOne = true
			}
		}
	}

	logFoundAll := true
	for _, streamI := range streamsToVerify {
		if !streamI.exist {
			e2e.Logf("warning: can not find stream matching the pattern : " + streamI.pattern)
			logFoundAll = false
		}
	}
	if strict {
		return logFoundAll
	}
	return logFoundOne
}

// In this function, verify the pod's groupNames can be found in cloudwatch
// GroupName example:
//
//	uuid-.0471c739-e38c-4590-8a96-fdd5298d47ae,uuid-.audit,uuid-.infrastructure
func (cw *cloudwatchSpec) applicationLogsFoundUUID() bool {
	var appLogGroupNames []string
	if len(cw.selNamespacesUUID) == 0 {
		logGroupNames := cw.getCloudwatchLogGroupNames(cw.groupPrefix)
		for _, e := range logGroupNames {
			r1, _ := regexp.Compile(`.*\.infrastructure$`)
			match1 := r1.MatchString(e)
			//match1, _ := regexp.MatchString(".*\\.infrastructure$", e)
			if match1 {
				continue
			}
			r2, _ := regexp.Compile(`.*\.audit$`)
			match2 := r2.MatchString(e)
			//match2, _ := regexp.MatchString(".*\\.audit$", e)
			if match2 {
				continue
			}
			appLogGroupNames = append(appLogGroupNames, e)
		}
		return len(appLogGroupNames) > 0
	}

	for _, projectUUID := range cw.selNamespacesUUID {
		logGroupNames := cw.getCloudwatchLogGroupNames(cw.groupPrefix + "." + projectUUID)
		if len(logGroupNames) == 0 {
			e2e.Logf("Can not find groupnames for project " + projectUUID)
			return false
		}
	}
	return true
}

// In this function, we verify the pod's groupNames can be found in cloudwatch
// GroupName:
//
//	prefix.aosqe-log-json-1638788875,prefix.audit,prefix.infrastructure
func (cw *cloudwatchSpec) applicationLogsFoundNamespaceName() bool {
	if len(cw.selAppNamespaces) == 0 {
		var appLogGroupNames []string
		logGroupNames := cw.getCloudwatchLogGroupNames(cw.groupPrefix)
		for _, e := range logGroupNames {
			r1, _ := regexp.Compile(`.*\.infrastructure$`)
			match1 := r1.MatchString(e)
			//match1, _ := regexp.MatchString(".*\\.infrastructure$", e)
			if match1 {
				continue
			}
			r2, _ := regexp.Compile(`.*\.audit$`)
			match2 := r2.MatchString(e)
			//match2, _ := regexp.MatchString(".*\\.audit$", e)
			if match2 {
				continue
			}
			appLogGroupNames = append(appLogGroupNames, e)
		}
		return len(appLogGroupNames) > 0
	}

	for _, projectName := range cw.selAppNamespaces {
		logGroupNames := cw.getCloudwatchLogGroupNames(cw.groupPrefix + "." + projectName)
		if len(logGroupNames) == 0 {
			e2e.Logf("Can not find groupnames for project " + projectName)
			return false
		}
	}
	return true
}

// In this function, verify the logStream can be found under application groupName
// GroupName Example:
//
//	anli48022-gwbb4.application
//
// logStream Example:
//
//	kubernetes.var.log.containers.centos-logtest-tvffh_aosqe-log-json-1638427743_centos-logtest-56a00a8f6a2e43281bce6d44d33e93b600352f2234610a093c4d254a49d9bf4e.log
//	kubernetes.var.log.containers.loki-server-6f8485b8ff-b4p8w_loki-aosqe_loki-c7a4e4fa4370062e53803ac5acecc57f6217eb2bb603143ac013755819ed5fdb.log
//	The stream name changed from containers to pods
//	kubernetes.var.log.pods.openshift-image-registry_image-registry-7f5dbdbc69-vwddg_425a4fbc-6a20-4919-8cd2-8bebd5d9b5cd.registry.0.log
//	pods.
func (cw *cloudwatchSpec) applicationLogsFoundLogType() bool {
	var appLogGroupNames []string

	logGroupNames := cw.getCloudwatchLogGroupNames(cw.groupPrefix)
	for _, e := range logGroupNames {
		r, _ := regexp.Compile(`.*\.application$`)
		match := r.MatchString(e)
		//match1, _ := regexp.MatchString(".*\\.application$", e)
		if match {
			appLogGroupNames = append(appLogGroupNames, e)
		}
	}
	// Return false if can not find app group
	if len(appLogGroupNames) == 0 {
		return false
	}

	if len(appLogGroupNames) > 1 {
		e2e.Logf("multiple App GroupNames found %v, Please clean up LogGroup in Cloudwatch", appLogGroupNames)
		return false
	}
	e2e.Logf("find logGroup %v", appLogGroupNames[0])

	logStreams := cw.getCloudwatchLogStreamNames(appLogGroupNames[0], "")
	e2e.Logf("The log Streams are: %v", logStreams)

	//Return true if no selNamespaces is pre-defined, else search the defined namespaces
	if len(cw.selAppNamespaces) == 0 {
		return true
	}

	var projects []string
	for i := 0; i < len(logStreams); i++ {
		// kubernetes.var.log.pods.e2e-test-vector-cloudwatch-9vvg5_logging-centos-logtest-xwzb5_b437565e-e60b-471a-a5f8-0d1bf72d6206.logging-centos-logtest.0.log
		streamFields := strings.Split(strings.Split(logStreams[i], "_")[0], ".")
		projects = append(projects, streamFields[len(streamFields)-1])
	}
	for _, appProject := range cw.selAppNamespaces {
		if !contain(projects, appProject) {
			e2e.Logf("Can not find the logStream for project %s, found projects %v", appProject, projects)
			return false
		}
	}

	//disSelAppNamespaces, select by pod, containers ....
	for i := 0; i < len(cw.disAppNamespaces); i++ {
		if contain(projects, cw.disAppNamespaces[i]) {
			e2e.Logf("Find logs from project %s, logs from this project shouldn't be collected!!!", cw.disAppNamespaces[i])
			return false
		}
	}
	return true
}

// The index to find application logs
// GroupType
//
//	logType: anli48022-gwbb4.application
//	namespaceName:  anli48022-gwbb4.aosqe-log-json-1638788875
//	namespaceUUID:   anli48022-gwbb4.0471c739-e38c-4590-8a96-fdd5298d47ae,uuid.audit,uuid.infrastructure
func (cw *cloudwatchSpec) applicationLogsFound() bool {
	switch cw.groupType {
	case "logType":
		return cw.applicationLogsFoundLogType()
	case "namespaceName":
		return cw.applicationLogsFoundNamespaceName()
	case "namespaceUUID":
		return cw.applicationLogsFoundUUID()
	default:
		return false
	}
}

// The common function to verify if logs can be found or not. In general, customized the cloudwatchSpec before call this function
func (cw *cloudwatchSpec) logsFound() bool {
	var appLogSuccess = true
	var infraLogSuccess = true
	var auditLogSuccess = true

	for _, logType := range cw.logTypes {
		if logType == "infrastructure" {
			infraLogSuccess = false
			err1 := wait.PollUntilContextTimeout(context.Background(), 30*time.Second, 180*time.Second, true, func(context.Context) (done bool, err error) {
				return cw.infrastructureLogsFound(true), nil
			})
			if err1 != nil {
				e2e.Logf("Failed to find infrastructure in given time\n %v", err1)
			} else {
				infraLogSuccess = true
				e2e.Logf("infraLogSuccess: %t", infraLogSuccess)
			}
		}
		if logType == "audit" {
			auditLogSuccess = false
			err2 := wait.PollUntilContextTimeout(context.Background(), 30*time.Second, 180*time.Second, true, func(context.Context) (done bool, err error) {
				return cw.auditLogsFound(true), nil
			})
			if err2 != nil {
				e2e.Logf("Failed to find auditLogs in given time\n %v", err2)
			} else {
				auditLogSuccess = true
				e2e.Logf("auditLogSuccess: %t", auditLogSuccess)
			}
		}
		if logType == "application" {
			appLogSuccess = false
			err3 := wait.PollUntilContextTimeout(context.Background(), 30*time.Second, 180*time.Second, true, func(context.Context) (done bool, err error) {
				return cw.applicationLogsFound(), nil
			})
			if err3 != nil {
				e2e.Logf("Failed to find AppLogs in given time\n %v", err3)
			} else {
				appLogSuccess = true
				e2e.Logf("appFound: %t", auditLogSuccess)
			}
		}
	}

	if infraLogSuccess && auditLogSuccess && appLogSuccess {
		e2e.Logf("Found all expected logs")
		return true
	}
	e2e.Logf("Error: couldn't find some type of logs. Possible reason: logs weren't generated; connect to AWS failure/timeout; Logging Bugs")
	return false
}

func (cw *cloudwatchSpec) getLogRecordsFromCloudwatchByNamespace(limit int32, logGroupName string, namespaceName string) ([]LogEntity, error) {
	var (
		output *cloudwatchlogs.FilterLogEventsOutput
		logs   []LogEntity
	)

	streamNames := cw.getCloudwatchLogStreamNames(logGroupName, "", namespaceName)
	e2e.Logf("the log streams are: %v", streamNames)
	err := wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 300*time.Second, true, func(context.Context) (done bool, err error) {
		output, err = cw.filterLogEventsFromCloudwatch(limit, logGroupName, "", streamNames...)
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
func (cw *cloudwatchSpec) filterLogEventsFromCloudwatch(limit int32, logGroupName, logStreamNamePrefix string, logStreamNames ...string) (*cloudwatchlogs.FilterLogEventsOutput, error) {
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
