package netobserv

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"github.com/tidwall/gjson"
	"google.golang.org/api/cloudresourcemanager/v1"
	"google.golang.org/api/iam/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

const (
	minioNS        = "minio-aosqe"
	minioSecret    = "minio-creds"
	apiPath        = "/api/logs/v1/"
	queryRangePath = "/loki/api/v1/query_range"
)

// s3Credential defines the s3 credentials
type s3Credential struct {
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	Endpoint        string //the endpoint of s3 service
}

type resource struct {
	kind      string
	name      string
	namespace string
}

func getAWSCredentialFromCluster(oc *exutil.CLI) s3Credential {
	region, err := exutil.GetAWSClusterRegion(oc)
	o.Expect(err).NotTo(o.HaveOccurred())

	dirname := "/tmp/" + oc.Namespace() + "-creds"
	defer os.RemoveAll(dirname)
	err = os.MkdirAll(dirname, 0777)
	o.Expect(err).NotTo(o.HaveOccurred())

	_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/aws-creds", "-n", "kube-system", "--confirm", "--to="+dirname).Output()
	o.Expect(err).NotTo(o.HaveOccurred())

	accessKeyID, err := os.ReadFile(dirname + "/aws_access_key_id")
	o.Expect(err).NotTo(o.HaveOccurred())
	secretAccessKey, err := os.ReadFile(dirname + "/aws_secret_access_key")
	o.Expect(err).NotTo(o.HaveOccurred())

	cred := s3Credential{Region: region, AccessKeyID: string(accessKeyID), SecretAccessKey: string(secretAccessKey)}
	return cred
}

// get azure storage account from image registry
// TODO: create a storage account and use that accout to manage azure container
func getAzureStorageAccount(oc *exutil.CLI) (string, string) {
	var accountName string
	imageRegistry, err := oc.AdminKubeClient().AppsV1().Deployments("openshift-image-registry").Get(context.Background(), "image-registry", metav1.GetOptions{})
	o.Expect(err).NotTo(o.HaveOccurred())
	for _, container := range imageRegistry.Spec.Template.Spec.Containers {
		for _, env := range container.Env {
			if env.Name == "REGISTRY_STORAGE_AZURE_ACCOUNTNAME" {
				accountName = env.Value
				break
			}
		}
	}

	dirname := "/tmp/" + oc.Namespace() + "-creds"
	defer os.RemoveAll(dirname)
	err = os.MkdirAll(dirname, 0777)
	o.Expect(err).NotTo(o.HaveOccurred())
	_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/image-registry-private-configuration", "-n", "openshift-image-registry", "--confirm", "--to="+dirname).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	accountKey, err := os.ReadFile(dirname + "/REGISTRY_STORAGE_AZURE_ACCOUNTKEY")
	o.Expect(err).NotTo(o.HaveOccurred())
	return accountName, string(accountKey)
}

func createSecretForODFBucket(oc *exutil.CLI, bucketName, secretName, ns string) error {
	if len(secretName) == 0 {
		return fmt.Errorf("secret name shouldn't be empty")
	}
	dirname := "/tmp/" + oc.Namespace() + "-creds"
	err := os.MkdirAll(dirname, 0777)
	o.Expect(err).NotTo(o.HaveOccurred())
	defer os.RemoveAll(dirname)
	_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/noobaa-admin", "-n", "openshift-storage", "--confirm", "--to="+dirname).Output()
	o.Expect(err).NotTo(o.HaveOccurred())

	endpoint := "http://s3.openshift-storage.svc"
	return oc.AsAdmin().WithoutNamespace().Run("create").Args("secret", "generic", secretName, "--from-file=access_key_id="+dirname+"/AWS_ACCESS_KEY_ID", "--from-file=access_key_secret="+dirname+"/AWS_SECRET_ACCESS_KEY", "--from-literal=bucketnames="+bucketName, "--from-literal=endpoint="+endpoint, "-n", ns).Execute()
}

func createSecretForMinIOBucket(oc *exutil.CLI, bucketName, secretName, ns, minIONS string) error {
	if len(secretName) == 0 {
		return fmt.Errorf("secret name shouldn't be empty")
	}
	dirname := "/tmp/" + oc.Namespace() + "-creds"
	defer os.RemoveAll(dirname)
	err := os.MkdirAll(dirname, 0777)
	o.Expect(err).NotTo(o.HaveOccurred())

	_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/"+minioSecret, "-n", minIONS, "--confirm", "--to="+dirname).Output()
	o.Expect(err).NotTo(o.HaveOccurred())

	endpoint := "http://minio." + minIONS + ".svc"
	return oc.AsAdmin().WithoutNamespace().Run("create").Args("secret", "generic", secretName, "--from-file=access_key_id="+dirname+"/access_key_id", "--from-file=access_key_secret="+dirname+"/secret_access_key", "--from-literal=bucketnames="+bucketName, "--from-literal=endpoint="+endpoint, "-n", ns).Execute()
}

func createS3Bucket(client *s3.Client, bucketName string, cred s3Credential) error {
	// check if the bucket exists or not
	// if exists, clear all the objects in the bucket
	// if not, create the bucket
	exist := false
	buckets, err := client.ListBuckets(context.TODO(), &s3.ListBucketsInput{})
	if err != nil {
		return err
	}
	for _, bu := range buckets.Buckets {
		if *bu.Name == bucketName {
			exist = true
			break
		}
	}
	// clear all the objects in the bucket
	if exist {
		return emptyS3Bucket(client, bucketName)
	}

	/*
		Per https://docs.aws.amazon.com/AmazonS3/latest/API/API_CreateBucket.html#API_CreateBucket_RequestBody,
		us-east-1 is the default region and it's not a valid value of LocationConstraint,
		using `LocationConstraint: types.BucketLocationConstraint("us-east-1")` gets error `InvalidLocationConstraint`.
		Here remove the configration when the region is us-east-1
	*/
	if len(cred.Region) == 0 || cred.Region == "us-east-1" {
		_, err = client.CreateBucket(context.TODO(), &s3.CreateBucketInput{Bucket: &bucketName})
		return err
	}
	_, err = client.CreateBucket(context.TODO(), &s3.CreateBucketInput{Bucket: &bucketName, CreateBucketConfiguration: &types.CreateBucketConfiguration{LocationConstraint: types.BucketLocationConstraint(cred.Region)}})
	return err
}

func deleteS3Bucket(client *s3.Client, bucketName string) error {
	// empty bucket
	err := emptyS3Bucket(client, bucketName)
	if err != nil {
		return err
	}
	// delete bucket
	_, err = client.DeleteBucket(context.TODO(), &s3.DeleteBucketInput{Bucket: &bucketName})
	return err
}

func emptyS3Bucket(client *s3.Client, bucketName string) error {
	// List objects in the bucket
	objects, err := client.ListObjectsV2(context.TODO(), &s3.ListObjectsV2Input{
		Bucket: &bucketName,
	})
	if err != nil {
		return err
	}

	// Delete objects in the bucket
	if len(objects.Contents) > 0 {
		objectIdentifiers := make([]types.ObjectIdentifier, len(objects.Contents))
		for i, object := range objects.Contents {
			objectIdentifiers[i] = types.ObjectIdentifier{Key: object.Key}
		}

		quiet := true
		_, err = client.DeleteObjects(context.TODO(), &s3.DeleteObjectsInput{
			Bucket: &bucketName,
			Delete: &types.Delete{
				Objects: objectIdentifiers,
				Quiet:   &quiet,
			},
		})
		if err != nil {
			return err
		}
	}

	// Check if there are more objects to delete and handle pagination
	if *objects.IsTruncated {
		return emptyS3Bucket(client, bucketName)
	}

	return nil
}

// CreateSecretForAWSS3Bucket creates a secret for Loki to connect to s3 bucket
func createSecretForAWSS3Bucket(oc *exutil.CLI, bucketName, secretName, ns string) error {
	if len(secretName) == 0 {
		return fmt.Errorf("secret name shouldn't be empty")
	}
	cred := getAWSCredentialFromCluster(oc)
	dirname := "/tmp/" + oc.Namespace() + "-creds"
	err := os.MkdirAll(dirname, 0777)
	o.Expect(err).NotTo(o.HaveOccurred())
	defer os.RemoveAll(dirname)
	f1, err1 := os.Create(dirname + "/aws_access_key_id")
	o.Expect(err1).NotTo(o.HaveOccurred())
	defer f1.Close()
	_, err = f1.WriteString(cred.AccessKeyID)
	o.Expect(err).NotTo(o.HaveOccurred())
	f2, err2 := os.Create(dirname + "/aws_secret_access_key")
	o.Expect(err2).NotTo(o.HaveOccurred())
	defer f2.Close()
	_, err = f2.WriteString(cred.SecretAccessKey)
	o.Expect(err).NotTo(o.HaveOccurred())

	endpoint := "https://s3." + cred.Region + ".amazonaws.com"
	return oc.AsAdmin().WithoutNamespace().Run("create").Args("secret", "generic", secretName, "--from-file=access_key_id="+dirname+"/aws_access_key_id", "--from-file=access_key_secret="+dirname+"/aws_secret_access_key", "--from-literal=region="+cred.Region, "--from-literal=bucketnames="+bucketName, "--from-literal=endpoint="+endpoint, "-n", ns).Execute()
}

// Creates a secret for Loki to connect to gcs bucket
func createSecretForGCSBucket(oc *exutil.CLI, bucketName, secretName, ns string) error {
	if len(secretName) == 0 {
		return fmt.Errorf("secret name shouldn't be empty")
	}
	dirname := "/tmp/" + oc.Namespace() + "-creds"
	defer os.RemoveAll(dirname)
	err := os.MkdirAll(dirname, 0777)
	o.Expect(err).NotTo(o.HaveOccurred())

	// for GCP STS clusters, get gcp-credentials from env var GOOGLE_APPLICATION_CREDENTIALS
	// TODO: support using STS token to create the secret
	_, err = oc.AdminKubeClient().CoreV1().Secrets("kube-system").Get(context.Background(), "gcp-credentials", metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		gcsCred := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
		return oc.AsAdmin().WithoutNamespace().Run("create").Args("secret", "generic", secretName, "-n", ns, "--from-literal=bucketname="+bucketName, "--from-file=key.json="+gcsCred).Execute()
	}
	_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/gcp-credentials", "-n", "kube-system", "--confirm", "--to="+dirname).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return oc.AsAdmin().WithoutNamespace().Run("create").Args("secret", "generic", secretName, "-n", ns, "--from-literal=bucketname="+bucketName, "--from-file=key.json="+dirname+"/service_account.json").Execute()
}

func createSecretForSwiftContainer(oc *exutil.CLI, containerName, secretName, ns string, cred *exutil.OpenstackCredentials) error {
	userID, domainID := exutil.GetOpenStackUserIDAndDomainID(cred)
	err := oc.AsAdmin().WithoutNamespace().Run("create").Args("secret", "generic", "-n", ns, secretName,
		"--from-literal=auth_url="+cred.Clouds.Openstack.Auth.AuthURL,
		"--from-literal=username="+cred.Clouds.Openstack.Auth.Username,
		"--from-literal=user_domain_name="+cred.Clouds.Openstack.Auth.UserDomainName,
		"--from-literal=user_domain_id="+domainID,
		"--from-literal=user_id="+userID,
		"--from-literal=password="+cred.Clouds.Openstack.Auth.Password,
		"--from-literal=domain_id="+domainID,
		"--from-literal=domain_name="+cred.Clouds.Openstack.Auth.UserDomainName,
		"--from-literal=container_name="+containerName,
		"--from-literal=project_id="+cred.Clouds.Openstack.Auth.ProjectID,
		"--from-literal=project_name="+cred.Clouds.Openstack.Auth.ProjectName,
		"--from-literal=project_domain_id="+domainID,
		"--from-literal=project_domain_name="+cred.Clouds.Openstack.Auth.UserDomainName).Execute()
	return err
}

// creates a secret for Loki to connect to azure container
func createSecretForAzureContainer(oc *exutil.CLI, bucketName, secretName, ns string) error {
	environment := "AzureGlobal"
	accountName, accountKey := getAzureStorageAccount(oc)
	err := oc.AsAdmin().WithoutNamespace().Run("create").Args("secret", "generic", "-n", ns, secretName, "--from-literal=environment="+environment, "--from-literal=container="+bucketName, "--from-literal=account_name="+accountName, "--from-literal=account_key="+accountKey).Execute()
	return err
}

func getODFCreds(oc *exutil.CLI) s3Credential {
	dirname := "/tmp/" + oc.Namespace() + "-creds"
	defer os.RemoveAll(dirname)
	err := os.MkdirAll(dirname, 0777)
	o.Expect(err).NotTo(o.HaveOccurred())

	_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/noobaa-admin", "-n", "openshift-storage", "--confirm", "--to="+dirname).Output()
	o.Expect(err).NotTo(o.HaveOccurred())

	accessKeyID, err := os.ReadFile(dirname + "/AWS_ACCESS_KEY_ID")
	o.Expect(err).NotTo(o.HaveOccurred())
	secretAccessKey, err := os.ReadFile(dirname + "/AWS_SECRET_ACCESS_KEY")
	o.Expect(err).NotTo(o.HaveOccurred())

	endpoint := "http://" + getRouteAddress(oc, "openshift-storage", "s3")
	return s3Credential{Endpoint: endpoint, AccessKeyID: string(accessKeyID), SecretAccessKey: string(secretAccessKey)}
}

func getMinIOCreds(oc *exutil.CLI, ns string) s3Credential {
	dirname := "/tmp/" + oc.Namespace() + "-creds"
	defer os.RemoveAll(dirname)
	err := os.MkdirAll(dirname, 0777)
	o.Expect(err).NotTo(o.HaveOccurred())

	_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/"+minioSecret, "-n", ns, "--confirm", "--to="+dirname).Output()
	o.Expect(err).NotTo(o.HaveOccurred())

	accessKeyID, err := os.ReadFile(dirname + "/access_key_id")
	o.Expect(err).NotTo(o.HaveOccurred())
	secretAccessKey, err := os.ReadFile(dirname + "/secret_access_key")
	o.Expect(err).NotTo(o.HaveOccurred())

	endpoint := "http://" + getRouteAddress(oc, ns, "minio")
	return s3Credential{Endpoint: endpoint, AccessKeyID: string(accessKeyID), SecretAccessKey: string(secretAccessKey)}
}

// initialize a s3 client with credential
// TODO: add an option to initialize a new client with STS
func newS3Client(cred s3Credential) *s3.Client {
	var err error
	var cfg aws.Config
	if len(cred.Endpoint) > 0 {
		customResolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
			return aws.Endpoint{
				URL:               cred.Endpoint,
				HostnameImmutable: true,
				Source:            aws.EndpointSourceCustom,
			}, nil
		})
		// For ODF and Minio, they're deployed in OCP clusters
		// In some clusters, we can't connect it without proxy, here add proxy settings to s3 client when there has http_proxy or https_proxy in the env var
		httpClient := awshttp.NewBuildableClient().WithTransportOptions(func(tr *http.Transport) {
			proxy := getProxyFromEnv()
			if len(proxy) > 0 {
				proxyURL, err := url.Parse(proxy)
				o.Expect(err).NotTo(o.HaveOccurred())
				tr.Proxy = http.ProxyURL(proxyURL)
			}
			tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		})
		cfg, err = config.LoadDefaultConfig(context.TODO(),
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cred.AccessKeyID, cred.SecretAccessKey, "")),
			config.WithEndpointResolverWithOptions(customResolver),
			config.WithHTTPClient(httpClient),
			config.WithRegion("auto"))
	} else {
		// aws s3
		cfg, err = config.LoadDefaultConfig(context.TODO(),
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cred.AccessKeyID, cred.SecretAccessKey, "")),
			config.WithRegion(cred.Region))
	}
	o.Expect(err).NotTo(o.HaveOccurred())
	return s3.NewFromConfig(cfg)
}

func getStorageClassName(oc *exutil.CLI) (string, error) {
	scs, err := oc.AdminKubeClient().StorageV1().StorageClasses().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return "", err
	}
	if len(scs.Items) == 0 {
		return "", fmt.Errorf("there is no storageclass in the cluster")
	}
	for _, sc := range scs.Items {
		if sc.ObjectMeta.Annotations["storageclass.kubernetes.io/is-default-class"] == "true" {
			return sc.Name, nil
		}
	}
	return scs.Items[0].Name, nil
}

func getSATokenFromSecret(oc *exutil.CLI, name, ns string) string {
	secrets, err := oc.AdminKubeClient().CoreV1().Secrets(ns).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return ""
	}
	var secret string
	for _, s := range secrets.Items {
		if strings.Contains(s.Name, name+"-token") {
			secret = s.Name
			break
		}
	}
	dirname := "/tmp/" + oc.Namespace() + "-sa"
	defer os.RemoveAll(dirname)
	err = os.MkdirAll(dirname, 0777)
	o.Expect(err).NotTo(o.HaveOccurred())
	_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/"+secret, "-n", ns, "--confirm", "--to="+dirname).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	bearerToken, err := os.ReadFile(dirname + "/token")
	o.Expect(err).NotTo(o.HaveOccurred())
	return string(bearerToken)
}

// PrepareResourcesForLokiStack creates buckets/containers in backend storage provider, and creates the secret for Loki to use
func (l lokiStack) prepareResourcesForLokiStack(oc *exutil.CLI) error {
	var err error
	if len(l.BucketName) == 0 {
		return fmt.Errorf("the bucketName should not be empty")
	}
	switch l.StorageType {
	case "s3":
		{
			if exutil.IsWorkloadIdentityCluster(oc) {
				region, err := exutil.GetAWSClusterRegion(oc)
				o.Expect(err).NotTo(o.HaveOccurred())
				cfg := readDefaultSDKExternalConfigurations(context.TODO(), region)
				iamClient := newIamClient(cfg)
				stsClient := newStsClient(cfg)
				awsAccountID, _ := getAwsAccount(stsClient)
				oidcName, err := getOIDC(oc)
				o.Expect(err).NotTo(o.HaveOccurred())
				lokiIAMRoleName := l.Name + "-" + exutil.GetRandomString()
				roleArn := createIAMRoleForLokiSTSDeployment(iamClient, oidcName, awsAccountID, l.Namespace, l.Name, lokiIAMRoleName)
				os.Setenv("LOKI_ROLE_NAME_ON_STS", lokiIAMRoleName)
				patchLokiOperatorWithAWSRoleArn(oc, "loki-operator", "openshift-operators-redhat", roleArn)
				var s3AssumeRoleName string
				defer func() {
					deleteIAMroleonAWS(iamClient, s3AssumeRoleName)
				}()
				s3AssumeRoleArn, s3AssumeRoleName := createS3AssumeRole(stsClient, iamClient, l.Name)
				createS3ObjectStorageBucketWithSTS(cfg, stsClient, s3AssumeRoleArn, l.BucketName)
				createObjectStorageSecretOnAWSSTSCluster(oc, region, l.StorageSecret, l.BucketName, l.Namespace)
			} else {
				cred := getAWSCredentialFromCluster(oc)
				client := newS3Client(cred)
				err = createS3Bucket(client, l.BucketName, cred)
				if err != nil {
					return err
				}
				err = createSecretForAWSS3Bucket(oc, l.BucketName, l.StorageSecret, l.Namespace)
			}
		}
	case "azure":
		{
			if exutil.IsWorkloadIdentityCluster(oc) {
				if !readAzureCredentials() {
					g.Skip("Azure Credentials not found. Skip case!")
				} else {
					performManagedIdentityAndSecretSetupForAzureWIF(oc, l.Name, l.Namespace, l.BucketName, l.StorageSecret)
				}
			} else {
				accountName, accountKey, err1 := exutil.GetAzureStorageAccountFromCluster(oc)
				if err1 != nil {
					return fmt.Errorf("can't get azure storage account from cluster: %v", err1)
				}
				client, err2 := exutil.NewAzureContainerClient(oc, accountName, accountKey, l.BucketName)
				if err2 != nil {
					return err2
				}
				err = exutil.CreateAzureStorageBlobContainer(client)
				if err != nil {
					return err
				}
				err = createSecretForAzureContainer(oc, l.BucketName, l.StorageSecret, l.Namespace)
			}
		}
	case "gcs":
		{
			projectID, errGetID := exutil.GetGcpProjectID(oc)
			o.Expect(errGetID).NotTo(o.HaveOccurred())
			err = exutil.CreateGCSBucket(projectID, l.BucketName)
			if err != nil {
				return err
			}
			if exutil.IsWorkloadIdentityCluster(oc) {
				clusterName := getInfrastructureName(oc)
				gcsSAName := generateServiceAccountNameForGCS(clusterName)
				os.Setenv("LOGGING_GCS_SERVICE_ACCOUNT_NAME", gcsSAName)
				projectNumber, err1 := getGCPProjectNumber(projectID)
				if err1 != nil {
					return fmt.Errorf("can't get GCP project number: %v", err1)
				}
				poolID, err2 := getPoolID(oc)
				if err2 != nil {
					return fmt.Errorf("can't get pool ID: %v", err2)
				}
				sa, err3 := createServiceAccountOnGCP(projectID, gcsSAName)
				if err3 != nil {
					return fmt.Errorf("can't create service account: %v", err3)
				}
				os.Setenv("LOGGING_GCS_SERVICE_ACCOUNT_EMAIL", sa.Email)
				err4 := grantPermissionsToGCPServiceAccount(poolID, projectID, projectNumber, l.Namespace, l.Name, sa.Email)
				if err4 != nil {
					return fmt.Errorf("can't add roles to the serviceaccount: %v", err4)
				}
				err = createSecretForGCSBucketWithSTS(oc, projectNumber, poolID, sa.Email, l.Namespace, l.StorageSecret, l.BucketName)
			} else {
				err = createSecretForGCSBucket(oc, l.BucketName, l.StorageSecret, l.Namespace)
			}
		}
	case "swift":
		{
			cred, err1 := exutil.GetOpenStackCredentials(oc)
			o.Expect(err1).NotTo(o.HaveOccurred())
			client := exutil.NewOpenStackClient(cred, "object-store")
			err = exutil.CreateOpenStackContainer(client, l.BucketName)
			if err != nil {
				return err
			}
			err = createSecretForSwiftContainer(oc, l.BucketName, l.StorageSecret, l.Namespace, cred)
		}
	case "odf":
		{
			err = createObjectBucketClaim(oc, l.Namespace, l.BucketName)
			if err != nil {
				return err
			}
			err = createSecretForODFBucket(oc, l.BucketName, l.StorageSecret, l.Namespace)
		}
	case "minio":
		{
			cred := getMinIOCreds(oc, minioNS)
			client := newS3Client(cred)
			err = createS3Bucket(client, l.BucketName, cred)
			if err != nil {
				return err
			}
			err = createSecretForMinIOBucket(oc, l.BucketName, l.StorageSecret, l.Namespace, minioNS)
		}
	}
	return err
}

func (l lokiStack) removeObjectStorage(oc *exutil.CLI) {
	resource{"secret", l.StorageSecret, l.Namespace}.clear(oc)
	var err error
	switch l.StorageType {
	case "s3":
		{
			if exutil.IsWorkloadIdentityCluster(oc) {
				region, err := exutil.GetAWSClusterRegion(oc)
				o.Expect(err).NotTo(o.HaveOccurred())
				cfg := readDefaultSDKExternalConfigurations(context.TODO(), region)
				iamClient := newIamClient(cfg)
				stsClient := newStsClient(cfg)
				var s3AssumeRoleName string
				defer func() {
					deleteIAMroleonAWS(iamClient, s3AssumeRoleName)
				}()
				s3AssumeRoleArn, s3AssumeRoleName := createS3AssumeRole(stsClient, iamClient, l.Name)
				if checkIfS3bucketExistsWithSTS(cfg, stsClient, s3AssumeRoleArn, l.BucketName) {
					deleteS3bucketWithSTS(cfg, stsClient, s3AssumeRoleArn, l.BucketName)
				}
				deleteIAMroleonAWS(iamClient, os.Getenv("LOKI_ROLE_NAME_ON_STS"))
				os.Unsetenv("LOKI_ROLE_NAME_ON_STS")
			} else {
				cred := getAWSCredentialFromCluster(oc)
				client := newS3Client(cred)
				err = deleteS3Bucket(client, l.BucketName)
			}
		}
	case "azure":
		{
			if exutil.IsWorkloadIdentityCluster(oc) {
				resourceGroup, err := getResourceGroupOnAzure(oc)
				o.Expect(err).NotTo(o.HaveOccurred())
				azureSubscriptionID := os.Getenv("AZURE_SUBSCRIPTION_ID")
				cred := createNewDefaultAzureCredential()
				deleteManagedIdentityOnAzure(cred, azureSubscriptionID, resourceGroup, l.Name)
				deleteAzureStorageAccount(cred, azureSubscriptionID, resourceGroup, os.Getenv("LOKI_OBJECT_STORAGE_STORAGE_ACCOUNT"))
				os.Unsetenv("LOKI_OBJECT_STORAGE_STORAGE_ACCOUNT")
			} else {
				accountName, accountKey, err1 := exutil.GetAzureStorageAccountFromCluster(oc)
				o.Expect(err1).NotTo(o.HaveOccurred())
				client, err2 := exutil.NewAzureContainerClient(oc, accountName, accountKey, l.BucketName)
				o.Expect(err2).NotTo(o.HaveOccurred())
				err = exutil.DeleteAzureStorageBlobContainer(client)
			}
		}
	case "gcs":
		{
			if exutil.IsWorkloadIdentityCluster(oc) {
				sa := os.Getenv("LOGGING_GCS_SERVICE_ACCOUNT_NAME")
				if sa == "" {
					e2e.Logf("LOGGING_GCS_SERVICE_ACCOUNT_NAME is not set, no need to delete the serviceaccount")
				} else {
					os.Unsetenv("LOGGING_GCS_SERVICE_ACCOUNT_NAME")
					email := os.Getenv("LOGGING_GCS_SERVICE_ACCOUNT_EMAIL")
					if email == "" {
						e2e.Logf("LOGGING_GCS_SERVICE_ACCOUNT_EMAIL is not set, no need to delete the policies")
					} else {
						os.Unsetenv("LOGGING_GCS_SERVICE_ACCOUNT_EMAIL")
						projectID, errGetID := exutil.GetGcpProjectID(oc)
						o.Expect(errGetID).NotTo(o.HaveOccurred())
						projectNumber, _ := getGCPProjectNumber(projectID)
						poolID, _ := getPoolID(oc)
						err = removePermissionsFromGCPServiceAccount(poolID, projectID, projectNumber, l.Namespace, l.Name, email)
						o.Expect(err).NotTo(o.HaveOccurred())
						err = removeServiceAccountFromGCP("projects/" + projectID + "/serviceAccounts/" + email)
						o.Expect(err).NotTo(o.HaveOccurred())
					}
				}
			}
			err = exutil.DeleteGCSBucket(l.BucketName)
		}
	case "swift":
		{
			cred, err1 := exutil.GetOpenStackCredentials(oc)
			o.Expect(err1).NotTo(o.HaveOccurred())
			client := exutil.NewOpenStackClient(cred, "object-store")
			err = exutil.DeleteOpenStackContainer(client, l.BucketName)
		}
	case "odf":
		{
			err = deleteObjectBucketClaim(oc, l.Namespace, l.BucketName)
		}
	case "minio":
		{
			cred := getMinIOCreds(oc, minioNS)
			client := newS3Client(cred)
			err = deleteS3Bucket(client, l.BucketName)
		}
	}
	o.Expect(err).NotTo(o.HaveOccurred())
}

func useExtraObjectStorage(oc *exutil.CLI) string {
	if checkODF(oc) {
		e2e.Logf("use the existing ODF storage service")
		return "odf"
	}
	ready, err := checkMinIO(oc, minioNS)
	if ready {
		e2e.Logf("use existing MinIO storage service")
		return "minio"
	}
	if strings.Contains(err.Error(), "No resources found") || strings.Contains(err.Error(), "not found") {
		e2e.Logf("deploy MinIO and use this MinIO as storage service")
		deployMinIO(oc)
		return "minio"
	}
	return ""
}

// return the storage type per different platform
func getStorageType(oc *exutil.CLI) string {
	platform := exutil.CheckPlatform(oc)
	switch platform {
	case "aws":
		{
			return "s3"
		}
	case "gcp":
		{
			return "gcs"
		}
	case "azure":
		{
			return "azure"
		}
	case "openstack":
		{
			return "swift"
		}
	default:
		{
			return useExtraObjectStorage(oc)
		}
	}
}

// checkODF check if the ODF is installed in the cluster or not
// here only checks the sc/ocs-storagecluster-ceph-rbd and svc/s3
func checkODF(oc *exutil.CLI) bool {
	svcFound := false
	expectedSC := []string{"openshift-storage.noobaa.io", "ocs-storagecluster-ceph-rbd", "ocs-storagecluster-cephfs"}
	var scInCluster []string
	scs, err := oc.AdminKubeClient().StorageV1().StorageClasses().List(context.Background(), metav1.ListOptions{})
	o.Expect(err).NotTo(o.HaveOccurred())

	for _, sc := range scs.Items {
		scInCluster = append(scInCluster, sc.Name)
	}

	for _, s := range expectedSC {
		if !contain(scInCluster, s) {
			return false
		}
	}

	_, err = oc.AdminKubeClient().CoreV1().Services("openshift-storage").Get(context.Background(), "s3", metav1.GetOptions{})
	if err == nil {
		svcFound = true
	}
	return svcFound
}

// checkMinIO
func checkMinIO(oc *exutil.CLI, ns string) (bool, error) {
	podReady, svcFound := false, false
	pod, err := oc.AdminKubeClient().CoreV1().Pods(ns).List(context.Background(), metav1.ListOptions{LabelSelector: "app=minio"})
	if err != nil {
		return false, err
	}
	if len(pod.Items) > 0 && pod.Items[0].Status.Phase == "Running" {
		podReady = true
	}
	_, err = oc.AdminKubeClient().CoreV1().Services(ns).Get(context.Background(), "minio", metav1.GetOptions{})
	if err == nil {
		svcFound = true
	}
	return podReady && svcFound, err
}

func deployMinIO(oc *exutil.CLI) {
	// create namespace
	_, err := oc.AdminKubeClient().CoreV1().Namespaces().Get(context.Background(), minioNS, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("namespace", minioNS).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	}
	// create secret
	_, err = oc.AdminKubeClient().CoreV1().Secrets(minioNS).Get(context.Background(), minioSecret, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("secret", "generic", minioSecret, "-n", minioNS, "--from-literal=access_key_id="+getRandomString(), "--from-literal=secret_access_key=passwOOrd"+getRandomString()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	}
	// deploy minIO
	deployTemplate := exutil.FixturePath("testdata", "logging", "minIO", "deploy.yaml")
	deployFile, err := processTemplate(oc, "-n", minioNS, "-f", deployTemplate, "-p", "NAMESPACE="+minioNS, "NAME=minio", "SECRET_NAME="+minioSecret)
	defer os.Remove(deployFile)
	o.Expect(err).NotTo(o.HaveOccurred())
	err = oc.AsAdmin().Run("apply").Args("-f", deployFile, "-n", minioNS).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	// wait for minio to be ready
	for _, rs := range []string{"deployment", "svc", "route"} {
		resource{rs, "minio", minioNS}.WaitForResourceToAppear(oc)
	}
	WaitForDeploymentPodsToBeReady(oc, minioNS, "minio")
}

func removeMinIO(oc *exutil.CLI) {
	deleteNamespace(oc, minioNS)
}

// WaitForDeploymentPodsToBeReady waits for the specific deployment to be ready
func WaitForDeploymentPodsToBeReady(oc *exutil.CLI, namespace string, name string) {
	var selectors map[string]string
	err := wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 180*time.Second, true, func(context.Context) (done bool, err error) {
		deployment, err := oc.AdminKubeClient().AppsV1().Deployments(namespace).Get(context.Background(), name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				e2e.Logf("Waiting for deployment/%s to appear\n", name)
				return false, nil
			}
			return false, err
		}
		selectors = deployment.Spec.Selector.MatchLabels
		if deployment.Status.AvailableReplicas == *deployment.Spec.Replicas && deployment.Status.UpdatedReplicas == *deployment.Spec.Replicas {
			e2e.Logf("Deployment %s available (%d/%d)\n", name, deployment.Status.AvailableReplicas, *deployment.Spec.Replicas)
			return true, nil
		}
		e2e.Logf("Waiting for full availability of %s deployment (%d/%d)\n", name, deployment.Status.AvailableReplicas, *deployment.Spec.Replicas)
		return false, nil
	})
	if err != nil && len(selectors) > 0 {
		var labels []string
		for k, v := range selectors {
			labels = append(labels, k+"="+v)
		}
		label := strings.Join(labels, ",")
		_ = oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", namespace, "-l", label).Execute()
		podStatus, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", namespace, "-l", label, "-ojsonpath={.items[*].status.conditions}").Output()
		containerStatus, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", namespace, "-l", label, "-ojsonpath={.items[*].status.containerStatuses}").Output()
		e2e.Failf("deployment %s is not ready:\nconditions: %s\ncontainer status: %s", name, podStatus, containerStatus)
	}
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("deployment %s is not available", name))
}

func (r resource) WaitForResourceToAppear(oc *exutil.CLI) {
	err := wait.PollUntilContextTimeout(context.Background(), 3*time.Second, 180*time.Second, true, func(context.Context) (done bool, err error) {
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", r.namespace, r.kind, r.name).Output()
		if err != nil {
			msg := fmt.Sprintf("%v", output)
			if strings.Contains(msg, "NotFound") {
				return false, nil
			}
			return false, err
		}
		e2e.Logf("Find %s %s", r.kind, r.name)
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("resource %s/%s is not appear", r.kind, r.name))
}

func generateServiceAccountNameForGCS(clusterName string) string {
	// Service Account should be between 6-30 characters long
	name := clusterName + getRandomString()
	return name
}

// To read Azure subscription json file from local disk.
// Also injects ENV vars needed to perform certain operations on Managed Identities.
func readAzureCredentials() bool {
	var azureCredFile string
	envDir, present := os.LookupEnv("CLUSTER_PROFILE_DIR")
	if present {
		azureCredFile = filepath.Join(envDir, "osServicePrincipal.json")
	} else {
		authFileLocation, present := os.LookupEnv("AZURE_AUTH_LOCATION")
		if present {
			azureCredFile = authFileLocation
		}
	}
	if len(azureCredFile) > 0 {
		fileContent, err := os.ReadFile(azureCredFile)
		o.Expect(err).NotTo(o.HaveOccurred())

		subscriptionID := gjson.Get(string(fileContent), `azure_subscription_id`).String()
		if subscriptionID == "" {
			subscriptionID = gjson.Get(string(fileContent), `subscriptionId`).String()
		}
		os.Setenv("AZURE_SUBSCRIPTION_ID", subscriptionID)

		tenantID := gjson.Get(string(fileContent), `azure_tenant_id`).String()
		if tenantID == "" {
			tenantID = gjson.Get(string(fileContent), `tenantId`).String()
		}
		os.Setenv("AZURE_TENANT_ID", tenantID)

		clientID := gjson.Get(string(fileContent), `azure_client_id`).String()
		if clientID == "" {
			clientID = gjson.Get(string(fileContent), `clientId`).String()
		}
		os.Setenv("AZURE_CLIENT_ID", clientID)

		clientSecret := gjson.Get(string(fileContent), `azure_client_secret`).String()
		if clientSecret == "" {
			clientSecret = gjson.Get(string(fileContent), `clientSecret`).String()
		}
		os.Setenv("AZURE_CLIENT_SECRET", clientSecret)
		return true
	}
	return false
}

func getGCPProjectNumber(projectID string) (string, error) {
	crmService, err := cloudresourcemanager.NewService(context.Background())
	if err != nil {
		return "", err
	}

	project, err := crmService.Projects.Get(projectID).Do()
	if err != nil {
		return "", err
	}
	return strconv.FormatInt(project.ProjectNumber, 10), nil
}

func getPoolID(oc *exutil.CLI) (string, error) {
	// pool_id="$(oc get authentication cluster -o json | jq -r .spec.serviceAccountIssuer | sed 's/.*\/\([^\/]*\)-oidc/\1/')"
	issuer, err := getOIDC(oc)
	if err != nil {
		return "", err
	}

	return strings.Split(strings.Split(issuer, "/")[1], "-oidc")[0], nil
}

func createServiceAccountOnGCP(projectID, name string) (*iam.ServiceAccount, error) {
	e2e.Logf("start to creating serviceaccount on GCP")
	ctx := context.Background()
	service, err := iam.NewService(ctx)
	if err != nil {
		return nil, fmt.Errorf("iam.NewService: %w", err)
	}

	request := &iam.CreateServiceAccountRequest{
		AccountId: name,
		ServiceAccount: &iam.ServiceAccount{
			DisplayName: "Service Account for " + name,
		},
	}
	account, err := service.Projects.ServiceAccounts.Create("projects/"+projectID, request).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to create serviceaccount: %w", err)
	}
	e2e.Logf("Created service account: %v", account)
	return account, nil
}

func grantPermissionsToGCPServiceAccount(poolID, projectID, projectNumber, lokiNS, lokiStackName, serviceAccountEmail string) error {
	gcsRoles := []string{
		"roles/iam.workloadIdentityUser",
		"roles/storage.objectAdmin",
	}
	subjects := []string{
		"system:serviceaccount:" + lokiNS + ":" + lokiStackName,
		"system:serviceaccount:" + lokiNS + ":" + lokiStackName + "-ruler",
	}

	for _, role := range gcsRoles {
		err := addBinding(projectID, "serviceAccount:"+serviceAccountEmail, role)
		if err != nil {
			return fmt.Errorf("error adding role %s to %s: %v", role, serviceAccountEmail, err)
		}
		for _, sub := range subjects {
			err := addBinding(projectID, "principal://iam.googleapis.com/projects/"+projectNumber+"/locations/global/workloadIdentityPools/"+poolID+"/subject/"+sub, role)
			if err != nil {
				return fmt.Errorf("error adding role %s to %s: %v", role, sub, err)
			}
		}
	}
	return nil
}

// ref: https://github.com/GoogleCloudPlatform/golang-samples/blob/main/iam/quickstart/quickstart.go
func addBinding(projectID, member, role string) error {
	crmService, err := cloudresourcemanager.NewService(context.Background())
	if err != nil {
		return fmt.Errorf("cloudresourcemanager.NewService: %v", err)
	}

	policy, err := getPolicy(crmService, projectID)
	if err != nil {
		return fmt.Errorf("error getting policy: %v", err)
	}

	// Find the policy binding for role. Only one binding can have the role.
	var binding *cloudresourcemanager.Binding
	for _, b := range policy.Bindings {
		if b.Role == role {
			binding = b
			break
		}
	}

	if binding != nil {
		// If the binding exists, adds the member to the binding
		binding.Members = append(binding.Members, member)
	} else {
		// If the binding does not exist, adds a new binding to the policy
		binding = &cloudresourcemanager.Binding{
			Role:    role,
			Members: []string{member},
		}
		policy.Bindings = append(policy.Bindings, binding)
	}
	return setPolicy(crmService, projectID, policy)
}

// getPolicy gets the project's IAM policy
func getPolicy(crmService *cloudresourcemanager.Service, projectID string) (*cloudresourcemanager.Policy, error) {
	request := new(cloudresourcemanager.GetIamPolicyRequest)
	policy, err := crmService.Projects.GetIamPolicy(projectID, request).Do()
	if err != nil {
		return nil, err
	}
	return policy, nil
}

// setPolicy sets the project's IAM policy
func setPolicy(crmService *cloudresourcemanager.Service, projectID string, policy *cloudresourcemanager.Policy) error {
	request := new(cloudresourcemanager.SetIamPolicyRequest)
	request.Policy = policy
	_, err := crmService.Projects.SetIamPolicy(projectID, request).Do()
	return err
}

func createObjectBucketClaim(oc *exutil.CLI, ns, name string) error {
	template := exutil.FixturePath("testdata", "logging", "odf", "objectBucketClaim.yaml")
	obc := resource{"objectbucketclaims", name, ns}

	err := obc.applyFromTemplate(oc, "-f", template, "-n", ns, "-p", "NAME="+name, "NAMESPACE="+ns)
	if err != nil {
		return err
	}
	obc.WaitForResourceToAppear(oc)
	resource{"objectbuckets", "obc-" + ns + "-" + name, ns}.WaitForResourceToAppear(oc)
	assertResourceStatus(oc, "objectbucketclaims", name, ns, "{.status.phase}", "Bound")
	return nil
}

func (r resource) applyFromTemplate(oc *exutil.CLI, parameters ...string) error {
	parameters = append(parameters, "-n", r.namespace)
	file, err := processTemplate(oc, parameters...)
	defer os.Remove(file)
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Can not process %v", parameters))
	output, err := oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", file, "-n", r.namespace).Output()
	if err != nil {
		return fmt.Errorf(output)
	}
	r.WaitForResourceToAppear(oc)
	return nil
}

// Assert the status of a resource
func assertResourceStatus(oc *exutil.CLI, kind, name, namespace, jsonpath, exptdStatus string) {
	parameters := []string{kind, name, "-o", "jsonpath=" + jsonpath}
	if namespace != "" {
		parameters = append(parameters, "-n", namespace)
	}
	err := wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 180*time.Second, true, func(context.Context) (done bool, err error) {
		status, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(parameters...).Output()
		if err != nil {
			return false, err
		}
		if strings.Compare(status, exptdStatus) != 0 {
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("%s/%s value for %s is not %s", kind, name, jsonpath, exptdStatus))
}

func createSecretForGCSBucketWithSTS(oc *exutil.CLI, projectNumber, poolID, serviceAccountEmail, ns, name, bucketName string) error {
	providerName := "projects/" + projectNumber + "/locations/global/workloadIdentityPools/" + poolID + "/providers/" + poolID
	audience, err := getGCPAudience(providerName)
	if err != nil {
		return err
	}
	key := `{
		"universe_domain": "googleapis.com",
		"type": "external_account",
		"audience": "//iam.googleapis.com/` + providerName + `",
		"subject_token_type": "urn:ietf:params:oauth:token-type:jwt",
		"token_url": "https://sts.googleapis.com/v1/token",
		"credential_source": {
			"file": "/var/run/secrets/storage/serviceaccount/token",
			"format": {
				"type": "text"
			}
		},
		"service_account_impersonation_url": "https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/` + serviceAccountEmail + `:generateAccessToken"
	}`

	return oc.NotShowInfo().AsAdmin().WithoutNamespace().Run("create").Args("secret", "generic", "-n", ns, name,
		"--from-literal=bucketname="+bucketName, "--from-literal=audience="+audience, "--from-literal=key.json="+key).Execute()
}

func getGCPAudience(providerName string) (string, error) {
	ctx := context.Background()
	service, err := iam.NewService(ctx)

	if err != nil {
		return "", fmt.Errorf("iam.NewService: %w", err)
	}
	audience, err := service.Projects.Locations.WorkloadIdentityPools.Providers.Get(providerName).Do()
	if err != nil {
		return "", fmt.Errorf("can't get audience: %v", err)
	}
	return audience.Oidc.AllowedAudiences[0], nil

}

// delete the objects in the cluster
func (r resource) clear(oc *exutil.CLI) error {
	msg, err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", r.namespace, r.kind, r.name).Output()
	if err != nil {
		errstring := fmt.Sprintf("%v", msg)
		if strings.Contains(errstring, "NotFound") || strings.Contains(errstring, "the server doesn't have a resource type") {
			return nil
		}
		return err
	}
	err = r.WaitUntilResourceIsGone(oc)
	return err
}

func (r resource) WaitUntilResourceIsGone(oc *exutil.CLI) error {
	err := wait.PollUntilContextTimeout(context.Background(), 3*time.Second, 180*time.Second, true, func(context.Context) (done bool, err error) {
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", r.namespace, r.kind, r.name).Output()
		if err != nil {
			errstring := fmt.Sprintf("%v", output)
			if strings.Contains(errstring, "NotFound") || strings.Contains(errstring, "the server doesn't have a resource type") {
				return true, nil
			}
			return true, err
		}
		return false, nil
	})
	if err != nil {
		return fmt.Errorf("can't remove %s/%s in %s project", r.kind, r.name, r.namespace)
	}
	return nil
}

func removePermissionsFromGCPServiceAccount(poolID, projectID, projectNumber, lokiNS, lokiStackName, serviceAccountEmail string) error {
	gcsRoles := []string{
		"roles/iam.workloadIdentityUser",
		"roles/storage.objectAdmin",
	}
	subjects := []string{
		"system:serviceaccount:" + lokiNS + ":" + lokiStackName,
		"system:serviceaccount:" + lokiNS + ":" + lokiStackName + "-ruler",
	}

	for _, role := range gcsRoles {
		err := removeMember(projectID, "serviceAccount:"+serviceAccountEmail, role)
		if err != nil {
			return fmt.Errorf("error removing role %s from %s: %v", role, serviceAccountEmail, err)
		}
		for _, sub := range subjects {
			err := removeMember(projectID, "principal://iam.googleapis.com/projects/"+projectNumber+"/locations/global/workloadIdentityPools/"+poolID+"/subject/"+sub, role)
			if err != nil {
				return fmt.Errorf("error removing role %s from %s: %v", role, sub, err)
			}
		}
	}
	return nil
}

func removeServiceAccountFromGCP(name string) error {
	ctx := context.Background()
	service, err := iam.NewService(ctx)
	if err != nil {
		return fmt.Errorf("iam.NewService: %w", err)
	}
	_, err = service.Projects.ServiceAccounts.Delete(name).Do()
	if err != nil {
		return fmt.Errorf("can't remove service account: %v", err)
	}
	return nil
}

// removeMember removes the member from the project's IAM policy
func removeMember(projectID, member, role string) error {
	crmService, err := cloudresourcemanager.NewService(context.Background())
	if err != nil {
		return fmt.Errorf("cloudresourcemanager.NewService: %v", err)
	}
	policy, err := getPolicy(crmService, projectID)
	if err != nil {
		return fmt.Errorf("error getting policy: %v", err)
	}
	// Find the policy binding for role. Only one binding can have the role.
	var binding *cloudresourcemanager.Binding
	var bindingIndex int
	for i, b := range policy.Bindings {
		if b.Role == role {
			binding = b
			bindingIndex = i
			break
		}
	}

	if len(binding.Members) == 1 && binding.Members[0] == member {
		// If the member is the only member in the binding, removes the binding
		last := len(policy.Bindings) - 1
		policy.Bindings[bindingIndex] = policy.Bindings[last]
		policy.Bindings = policy.Bindings[:last]
	} else {
		// If there is more than one member in the binding, removes the member
		var memberIndex int
		var exist bool
		for i, mm := range binding.Members {
			if mm == member {
				memberIndex = i
				exist = true
				break
			}
		}
		if exist {
			last := len(policy.Bindings[bindingIndex].Members) - 1
			binding.Members[memberIndex] = binding.Members[last]
			binding.Members = binding.Members[:last]
		}
	}

	return setPolicy(crmService, projectID, policy)
}

func deleteObjectBucketClaim(oc *exutil.CLI, ns, name string) error {
	obc := resource{"objectbucketclaims", name, ns}
	err := obc.clear(oc)
	if err != nil {
		return err
	}
	return obc.WaitUntilResourceIsGone(oc)
}
