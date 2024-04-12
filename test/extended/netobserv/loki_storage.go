package netobserv

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	minioNS        = "minio-aosqe"
	minioSecret    = "minio-creds"
	apiPath        = "/api/logs/v1/"
	queryRangePath = "/loki/api/v1/query_range"
)

// S3Credential defines the s3 credentials
type S3Credential struct {
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	Endpoint        string //the endpoint of s3 service
}

func getAWSCredentialFromCluster(oc *exutil.CLI) S3Credential {
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

	cred := S3Credential{Region: region, AccessKeyID: string(accessKeyID), SecretAccessKey: string(secretAccessKey)}
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

func createS3Bucket(client *s3.Client, bucketName string, cred S3Credential) error {
	// check if the bucket exists or not
	// if exists, clear all the objects in the bucket
	// if not, create the bucket
	exist := false
	buckets, err := client.ListBuckets(context.TODO(), &s3.ListBucketsInput{})
	o.Expect(err).NotTo(o.HaveOccurred())
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

func getODFCreds(oc *exutil.CLI) S3Credential {
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
	return S3Credential{Endpoint: endpoint, AccessKeyID: string(accessKeyID), SecretAccessKey: string(secretAccessKey)}
}

func getMinIOCreds(oc *exutil.CLI, ns string) S3Credential {
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
	return S3Credential{Endpoint: endpoint, AccessKeyID: string(accessKeyID), SecretAccessKey: string(secretAccessKey)}
}

// initialize an aws s3 client with aws credential
// TODO: add an option to initialize a new client with STS
func newAWSS3Client(cred S3Credential) *s3.Client {
	var err error
	var cfg aws.Config
	if len(cred.Endpoint) > 0 {
		customResolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
			return aws.Endpoint{
				PartitionID:       "aws",
				URL:               cred.Endpoint,
				HostnameImmutable: true,
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
			config.WithHTTPClient(httpClient))
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
			cred := getAWSCredentialFromCluster(oc)
			client := newAWSS3Client(cred)
			err = createS3Bucket(client, l.BucketName, cred)
			if err != nil {
				return err
			}
			err = createSecretForAWSS3Bucket(oc, l.BucketName, l.StorageSecret, l.Namespace)
		}
	case "azure":
		{
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
	case "gcs":
		{
			projectID, errGetID := exutil.GetGcpProjectID(oc)
			o.Expect(errGetID).NotTo(o.HaveOccurred())
			err = exutil.CreateGCSBucket(projectID, l.BucketName)
			if err != nil {
				return err
			}
			err = createSecretForGCSBucket(oc, l.BucketName, l.StorageSecret, l.Namespace)
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
			cred := getODFCreds(oc)
			client := newAWSS3Client(cred)
			err = createS3Bucket(client, l.BucketName, cred)
			if err != nil {
				return err
			}
			err = createSecretForODFBucket(oc, l.BucketName, l.StorageSecret, l.Namespace)
		}
	case "minio":
		{
			cred := getMinIOCreds(oc, minioNS)
			client := newAWSS3Client(cred)
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
			cred := getAWSCredentialFromCluster(oc)
			client := newAWSS3Client(cred)
			err = deleteS3Bucket(client, l.BucketName)
		}
	case "azure":
		{
			accountName, accountKey, err1 := exutil.GetAzureStorageAccountFromCluster(oc)
			o.Expect(err1).NotTo(o.HaveOccurred())
			client, err2 := exutil.NewAzureContainerClient(oc, accountName, accountKey, l.BucketName)
			o.Expect(err2).NotTo(o.HaveOccurred())
			err = exutil.DeleteAzureStorageBlobContainer(client)
		}
	case "gcs":
		{
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
			cred := getODFCreds(oc)
			client := newAWSS3Client(cred)
			err = deleteS3Bucket(client, l.BucketName)
		}
	case "minio":
		{
			cred := getMinIOCreds(oc, minioNS)
			client := newAWSS3Client(cred)
			err = deleteS3Bucket(client, l.BucketName)
		}
	}
	o.Expect(err).NotTo(o.HaveOccurred())
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
			if checkODF(oc) {
				return "odf"
			}
			if checkMinIO(oc, minioNS) {
				return "minio"
			}
			return ""
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

func checkMinIO(oc *exutil.CLI, ns string) bool {
	podReady, svcFound := false, false
	pod, err := oc.AdminKubeClient().CoreV1().Pods(ns).List(context.Background(), metav1.ListOptions{LabelSelector: "app=minio"})
	o.Expect(err).NotTo(o.HaveOccurred())
	if len(pod.Items) > 0 && pod.Items[0].Status.Phase == "Running" {
		podReady = true
	}
	_, err = oc.AdminKubeClient().CoreV1().Services(ns).Get(context.Background(), "minio", metav1.GetOptions{})
	if err == nil {
		svcFound = true
	}
	return podReady && svcFound
}
