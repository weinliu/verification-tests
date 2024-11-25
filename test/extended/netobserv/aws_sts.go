package netobserv

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	awsCred "github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3Types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
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

// Creates S3 bucket storage with STS
func createS3ObjectStorageBucketWithSTS(cfg aws.Config, stsClient *sts.Client, s3roleArn, bucketName string) {
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
	WaitForPodsReadyWithLabel(oc, "openshift-operators-redhat", "name=loki-operator-controller-manager")
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
			e2e.Logf("Bucket does not exist: %s", msg)
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
			e2e.Logf("Bucket does not exist: %s", msg)
		} else {
			e2e.Failf("Some Error deleting the bucket" + bucketName)
		}
	} else {
		e2e.Logf("Bucket deleted successfully")
	}

}
