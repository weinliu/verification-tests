package logging

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

func readDefaultSDKExternalConfigurations(ctx context.Context) (aws.Config, error) {
	cfg, err := awsConfig.LoadDefaultConfig(ctx,
		awsConfig.WithRegion(os.Getenv("AWS_CLUSTER_REGION")),
	)
	return cfg, err
}

// new AWS STS client
func newStsClient() *sts.Client {
	checkAWSCredentials()
	cfg, err := readDefaultSDKExternalConfigurations(context.TODO())
	if err != nil {
		e2e.Logf("failed to load AWS configuration, %v", err)
	}
	o.Expect(err).NotTo(o.HaveOccurred())
	return sts.NewFromConfig(cfg)
}

// get AWS Account ID
func getAwsAccount(stsClient *sts.Client) (string, string) {
	result, err := stsClient.GetCallerIdentity(context.TODO(), &sts.GetCallerIdentityInput{})
	if err != nil {
		e2e.Logf("Couldn't get AWS caller identity. Here's why: %v\n", err)
	}
	o.Expect(err).NotTo(o.HaveOccurred())
	awsAccount := aws.ToString(result.Account)
	awsUserArn := aws.ToString(result.Arn)
	e2e.Logf("The current AWS account is: %v\n", awsAccount)
	return awsAccount, awsUserArn
}

// Create AWS IAM client
func newIamClient() *iam.Client {
	checkAWSCredentials()
	cfg, err := readDefaultSDKExternalConfigurations(context.TODO())
	if err != nil {
		e2e.Logf("failed to load AWS configuration, %v", err)
	}
	o.Expect(err).NotTo(o.HaveOccurred())
	return iam.NewFromConfig(cfg)
}

// Check if credentials exist for STS clusters
func checkAWSCredentials() {
	envAws, present := os.LookupEnv("AWS_SHARED_CREDENTIALS_FILE")
	if present {
		e2e.Logf("The env AWS_SHARED_CREDENTIALS_FILE has been set: %v", envAws)
	} else {
		e2e.Logf("The env AWS_SHARED_CREDENTIALS_FILE is not set")
		envDir, present := os.LookupEnv("CLUSTER_PROFILE_DIR")
		if present {
			e2e.Logf("The env CLUSTER_PROFILE_DIR has been set: %v", envDir)
			awsCredFile := filepath.Join(envDir, ".awscred")
			if _, err := os.Stat(awsCredFile); err == nil {
				e2e.Logf("The .awscred file exists, export env AWS_SHARED_CREDENTIALS_FILE...")
				err := os.Setenv("AWS_SHARED_CREDENTIALS_FILE", awsCredFile)
				o.Expect(err).NotTo(o.HaveOccurred())
			} else {
				e2e.Logf("Error: %v", err)
				g.Skip("The .awscred file does not exist\n")
			}
		} else {
			g.Skip("Skip since env CLUSTER_PROFILE_DIR is not set either, no valid aws credential")
		}
	}
}

// This func creates a IAM role, attaches custom trust policy and managed permission policy
func createIAMRoleOnAWS(iamClient *iam.Client, trustPolicy string, roleName string, policyArn string) string {
	result, err := iamClient.CreateRole(context.TODO(), &iam.CreateRoleInput{
		AssumeRolePolicyDocument: aws.String(trustPolicy),
		RoleName:                 aws.String(roleName),
	})
	if err != nil {
		e2e.Logf("Couldn't create role %v. Here's why: %v\n", roleName, err)
	}
	o.Expect(err).NotTo(o.HaveOccurred())
	roleArn := aws.ToString(result.Role.Arn)
	e2e.Logf("The created role ARN is: %v\n", roleArn)

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
func deleteIAMroleonAWS(roleName string) {
	cfg, err := readDefaultSDKExternalConfigurations(context.Background())
	o.Expect(err).NotTo(o.HaveOccurred())

	// Create an IAM client
	iamClient := iam.NewFromConfig(cfg)

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
func createIAMRoleForLokiSTSDeployment(oc *exutil.CLI, lokiNamespace string, lokiStackName string, roleName string) string {
	iamClient := newIamClient()
	stsClient := newStsClient()
	oidcName := getOIDC(oc)
	AWSAccountID, _ := getAwsAccount(stsClient)
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

	lokiTrustPolicy = fmt.Sprintf(lokiTrustPolicy, AWSAccountID, oidcName, oidcName, lokiNamespace, lokiStackName, lokiNamespace, lokiStackName)
	roleArn := createIAMRoleOnAWS(iamClient, lokiTrustPolicy, roleName, policyArn)
	return roleArn
}

// Creates S3 bucket and Object Storage required for Loki with that bucket and region.
func createObjectStorageSecretWithS3OnSTS(oc *exutil.CLI, stsClient *sts.Client, s3roleArn string, ls lokiStack) {

	// Check if bucket exists, if yes delete it
	if checkIfS3bucketExistsWithSTS(stsClient, s3roleArn, ls.bucketName) {
		e2e.Logf("Bucket already exists. Deleting bucket")
		deleteS3bucketWithSTS(stsClient, s3roleArn, ls.bucketName)
	}
	cfg, err := readDefaultSDKExternalConfigurations(context.Background())
	o.Expect(err).NotTo(o.HaveOccurred())

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
		Bucket: aws.String(ls.bucketName),
	}

	// Check if the region is us-east-1, do not specify a location constraint
	if cfg.Region != "us-east-1" {
		createBucketInput.CreateBucketConfiguration = &s3Types.CreateBucketConfiguration{
			LocationConstraint: s3Types.BucketLocationConstraint(cfg.Region),
		}
	}

	_, err = s3Client.CreateBucket(context.TODO(), createBucketInput)
	o.Expect(err).NotTo(o.HaveOccurred(), "Failed to create bucket: "+ls.bucketName)

	// Creating object storage secret
	err = oc.AsAdmin().WithoutNamespace().Run("create").Args("secret", "generic", ls.storageSecret, "--from-literal=region="+cfg.Region, "--from-literal=bucketnames="+ls.bucketName, "-n", ls.namespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Function to check S3 bucket contents on STS cluster
func validateS3contentsWithSTS(bucketName, s3roleArn string) {
	stsClient := newStsClient()
	cfg, err := readDefaultSDKExternalConfigurations(context.Background())
	o.Expect(err).NotTo(o.HaveOccurred())

	// Assume the role to get temporary credentials
	assumeRoleOutput, err := stsClient.AssumeRole(context.TODO(), &sts.AssumeRoleInput{
		RoleArn:         aws.String(s3roleArn),
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
	// Poll to check contents of the s3 bucket
	err = wait.PollUntilContextTimeout(context.Background(), 30*time.Second, 300*time.Second, true, func(context.Context) (done bool, err error) {
		listObjectsOutput, err := s3Client.ListObjectsV2(context.TODO(), &s3.ListObjectsV2Input{
			Bucket: aws.String(bucketName),
		})
		if err != nil {
			return false, err
		}

		for _, object := range listObjectsOutput.Contents {
			if strings.Contains(*object.Key, "application") || strings.Contains(*object.Key, "infrastructure") || strings.Contains(*object.Key, "audit") || strings.Contains(*object.Key, "index") {
				e2e.Logf("Logs " + *object.Key + " found under the bucket: " + bucketName)
				return true, nil
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

	oc.AsAdmin().WithoutNamespace().Run("patch").Args("sub", packageName, "-n", lokiNamespace, "-p", fmt.Sprintf(roleArnPatchConfig, roleArn), "--type=merge").Execute()
	waitForPodReadyWithLabel(oc, loNS, "name=loki-operator-controller-manager")
}

// Checks if a specific bucket exists under AWS S3 service
func checkIfS3bucketExistsWithSTS(stsClient *sts.Client, s3assumeRoleArn string, bucketName string) bool {
	cfg, err := readDefaultSDKExternalConfigurations(context.Background())
	o.Expect(err).NotTo(o.HaveOccurred())

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
func deleteS3bucketWithSTS(stsClient *sts.Client, s3assumeRoleArn string, bucketName string) {
	cfg, err := readDefaultSDKExternalConfigurations(context.Background())
	o.Expect(err).NotTo(o.HaveOccurred())

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
