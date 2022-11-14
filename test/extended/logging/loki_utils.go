package logging

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"reflect"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"github.com/Azure/azure-storage-blob-go/azblob"
	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	tokens3 "github.com/gophercloud/gophercloud/openstack/identity/v3/tokens"
	"github.com/gophercloud/gophercloud/openstack/identity/v3/users"
	"github.com/gophercloud/gophercloud/openstack/objectstorage/v1/containers"
	"github.com/gophercloud/gophercloud/openstack/objectstorage/v1/objects"
	"github.com/gophercloud/gophercloud/pagination"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"google.golang.org/api/iterator"
	yamlv3 "gopkg.in/yaml.v3"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	k8sresource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

const (
	apiPath         = "/api/logs/v1/"
	queryPath       = "/loki/api/v1/query"
	queryRangePath  = "/loki/api/v1/query_range"
	labelsPath      = "/loki/api/v1/labels"
	labelValuesPath = "/loki/api/v1/label/%s/values"
	seriesPath      = "/loki/api/v1/series"
	tailPath        = "/loki/api/v1/tail"
	minioNS         = "minio-aosqe"
	minioSecret     = "minio-creds"
)

// s3Credential defines the s3 credentials
type s3Credential struct {
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	Endpoint        string //the endpoint of s3 service
}

func getAWSClusterRegion(oc *exutil.CLI) (string, error) {
	region, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.aws.region}").Output()
	return region, err
}

func getAWSCredentialFromCluster(oc *exutil.CLI) s3Credential {
	region, err := getAWSClusterRegion(oc)
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

// initialize an aws s3 client with aws credential
// TODO: add an option to initialize a new client with STS
func newAWSS3Client(oc *exutil.CLI, cred s3Credential) *s3.Client {
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

func createAWSS3Bucket(oc *exutil.CLI, client *s3.Client, bucketName string, cred s3Credential) error {
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
		return emptyAWSS3Bucket(client, bucketName)
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

func deleteAWSS3Bucket(client *s3.Client, bucketName string) error {
	// empty bucket
	err := emptyAWSS3Bucket(client, bucketName)
	if err != nil {
		return err
	}
	// delete bucket
	_, err = client.DeleteBucket(context.TODO(), &s3.DeleteBucketInput{Bucket: &bucketName})
	return err
}

func emptyAWSS3Bucket(client *s3.Client, bucketName string) error {
	// list objects in the bucket
	objects, err := client.ListObjects(context.TODO(), &s3.ListObjectsInput{Bucket: &bucketName})
	o.Expect(err).NotTo(o.HaveOccurred())
	// remove objects in the bucket
	newObjects := []types.ObjectIdentifier{}
	for _, object := range objects.Contents {
		newObjects = append(newObjects, types.ObjectIdentifier{Key: object.Key})
	}
	if len(newObjects) > 0 {
		_, err = client.DeleteObjects(context.TODO(), &s3.DeleteObjectsInput{Bucket: &bucketName, Delete: &types.Delete{Quiet: true, Objects: newObjects}})
		return err
	}
	return nil
}

// createSecretForAWSS3Bucket creates a secret for Loki to connect to s3 bucket
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

func getGCPProjectID(oc *exutil.CLI) string {
	projectID, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructures.config.openshift.io", "cluster", "-ojsonpath={.status.platformStatus.gcp.projectID}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return projectID
}

func createGCSBucket(projectID, bucketName string) error {
	ctx := context.Background()
	// initialize the GCS client, the credentials are got from the env var GOOGLE_APPLICATION_CREDENTIALS
	client, err := storage.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("storage.NewClient: %v", err)
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(ctx, time.Second*10)
	defer cancel()

	// check if the bucket exists or not
	// if exists, clear all the objects in the bucket
	// if not, create the bucket
	exist := false
	buckets, err := listGCSBuckets(*client, projectID)
	if err != nil {
		return err
	}
	for _, bu := range buckets {
		if bu == bucketName {
			exist = true
			break
		}
	}
	if exist {
		return emptyGCSBucket(*client, bucketName)
	}

	bucket := client.Bucket(bucketName)
	if err := bucket.Create(ctx, projectID, &storage.BucketAttrs{}); err != nil {
		return fmt.Errorf("Bucket(%q).Create: %v", bucketName, err)
	}
	fmt.Printf("Created bucket %v\n", bucketName)
	return nil
}

// listGCSBuckets gets all the bucket names under the projectID
func listGCSBuckets(client storage.Client, projectID string) ([]string, error) {
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, time.Second*30)
	defer cancel()

	var buckets []string
	it := client.Buckets(ctx, projectID)
	for {
		battrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		buckets = append(buckets, battrs.Name)
	}
	return buckets, nil
}

// listObjestsInGCSBucket gets all the objects in a bucket
func listObjestsInGCSBucket(client storage.Client, bucket string) ([]string, error) {
	files := []string{}
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, time.Second*10)
	defer cancel()

	it := client.Bucket(bucket).Objects(ctx, nil)
	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return files, fmt.Errorf("Bucket(%q).Objects: %v", bucket, err)
		}
		files = append(files, attrs.Name)
	}
	return files, nil
}

// deleteFilesInGCSBucket removes the object in the bucket
func deleteFilesInGCSBucket(client storage.Client, object, bucket string) error {
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, time.Second*10)
	defer cancel()

	o := client.Bucket(bucket).Object(object)

	// Optional: set a generation-match precondition to avoid potential race
	// conditions and data corruptions. The request to upload is aborted if the
	// object's generation number does not match your precondition.
	attrs, err := o.Attrs(ctx)
	if err != nil {
		return fmt.Errorf("object.Attrs: %v", err)
	}
	o = o.If(storage.Conditions{GenerationMatch: attrs.Generation})

	if err := o.Delete(ctx); err != nil {
		return fmt.Errorf("Object(%q).Delete: %v", object, err)
	}
	return nil
}

// emptyGCSBucket removes all the objects in the bucket
func emptyGCSBucket(client storage.Client, bucket string) error {
	objects, err := listObjestsInGCSBucket(client, bucket)
	o.Expect(err).NotTo(o.HaveOccurred())

	for _, object := range objects {
		err = deleteFilesInGCSBucket(client, object, bucket)
		if err != nil {
			return err
		}
	}
	return nil
}

func deleteGCSBucket(bucketName string) error {
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("storage.NewClient: %v", err)
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(ctx, time.Second*60)
	defer cancel()

	// remove objects
	err = emptyGCSBucket(*client, bucketName)
	if err != nil {
		return err
	}
	bucket := client.Bucket(bucketName)
	if err := bucket.Delete(ctx); err != nil {
		return fmt.Errorf("Bucket(%q).Delete: %v", bucketName, err)
	}
	fmt.Printf("Bucket %v deleted\n", bucketName)
	return nil
}

// creates a secret for Loki to connect to gcs bucket
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

// initialize a new azure blob container client
func newAzureContainerClient(oc *exutil.CLI, name string) azblob.ContainerURL {
	accountName, accountKey := getAzureStorageAccount(oc)
	credential, err := azblob.NewSharedKeyCredential(accountName, accountKey)
	o.Expect(err).NotTo(o.HaveOccurred())
	p := azblob.NewPipeline(credential, azblob.PipelineOptions{})
	u, _ := url.Parse(fmt.Sprintf("https://%s.blob.core.windows.net", accountName))
	serviceURL := azblob.NewServiceURL(*u, p)
	return serviceURL.NewContainerURL(name)
}

// create azure storage container
func createAzureStorageBlobContainer(container azblob.ContainerURL) error {
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, time.Second*60)
	defer cancel()

	// check if the container exists or not
	// if exists, then remove the blobs in the container, if not, create the container
	_, err := container.GetProperties(ctx, azblob.LeaseAccessConditions{})
	message := fmt.Sprintf("%v", err)
	if strings.Contains(message, "ContainerNotFound") {
		_, err = container.Create(ctx, azblob.Metadata{}, azblob.PublicAccessNone)
		return err
	}
	return emptyAzureBlobContainer(container)
}

// delete azure storage container
func deleteAzureStorageBlobContainer(container azblob.ContainerURL) error {
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, time.Second*60)
	defer cancel()

	err := emptyAzureBlobContainer(container)
	if err != nil {
		return err
	}
	_, err = container.Delete(ctx, azblob.ContainerAccessConditions{})
	return err
}

// list files in azure storage container
func listBlobsInAzureContainer(container azblob.ContainerURL) []string {
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, time.Second*60)
	defer cancel()

	blobNames := []string{}
	for marker := (azblob.Marker{}); marker.NotDone(); { // The parens around Marker{} are required to avoid compiler error.
		// Get a result segment starting with the blob indicated by the current Marker.
		listBlob, err := container.ListBlobsFlatSegment(ctx, marker, azblob.ListBlobsSegmentOptions{})
		o.Expect(err).NotTo(o.HaveOccurred())

		// IMPORTANT: ListBlobs returns the start of the next segment; you MUST use this to get
		// the next segment (after processing the current result segment).
		marker = listBlob.NextMarker

		// Process the blobs returned in this result segment (if the segment is empty, the loop body won't execute)
		for _, blobInfo := range listBlob.Segment.BlobItems {
			blobNames = append(blobNames, blobInfo.Name)
		}
	}
	return blobNames
}

// delete file from azure storage container
func deleteAzureBlob(container azblob.ContainerURL, blobName string) error {
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, time.Second*60)
	defer cancel()

	blobURL := container.NewBlockBlobURL(blobName)
	_, err := blobURL.Delete(ctx, azblob.DeleteSnapshotsOptionNone, azblob.BlobAccessConditions{})
	return err
}

// remove all the files in azure storage container
func emptyAzureBlobContainer(container azblob.ContainerURL) error {
	blobNames := listBlobsInAzureContainer(container)
	for _, blob := range blobNames {
		err := deleteAzureBlob(container, blob)
		if err != nil {
			return err
		}
	}
	return nil
}

// creates a secret for Loki to connect to azure container
func createSecretForAzureContainer(oc *exutil.CLI, bucketName, secretName, ns string) error {
	environment := "AzureGlobal"
	accountName, accountKey := getAzureStorageAccount(oc)
	err := oc.AsAdmin().WithoutNamespace().Run("create").Args("secret", "generic", "-n", ns, secretName, "--from-literal=environment="+environment, "--from-literal=container="+bucketName, "--from-literal=account_name="+accountName, "--from-literal=account_key="+accountKey).Execute()
	return err
}

type openstackCredentials struct {
	Clouds struct {
		Openstack struct {
			Auth struct {
				AuthURL        string `yaml:"auth_url"`
				Password       string `yaml:"password"`
				ProjectID      string `yaml:"project_id"`
				ProjectName    string `yaml:"project_name"`
				UserDomainName string `yaml:"user_domain_name"`
				Username       string `yaml:"username"`
			} `yaml:"auth"`
			EndpointType       string `yaml:"endpoint_type"`
			IdentityAPIVersion string `yaml:"identity_api_version"`
			RegionName         string `yaml:"region_name"`
			Verify             bool   `yaml:"verify"`
		} `yaml:"openstack"`
	} `yaml:"clouds"`
}

func getOpenStackCredentials(oc *exutil.CLI) (*openstackCredentials, error) {
	cred := &openstackCredentials{}
	dirname := "/tmp/" + oc.Namespace() + "-creds"
	defer os.RemoveAll(dirname)
	err := os.MkdirAll(dirname, 0777)
	o.Expect(err).NotTo(o.HaveOccurred())

	_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/openstack-credentials", "-n", "kube-system", "--confirm", "--to="+dirname).Output()
	if err != nil {
		return cred, err
	}

	confFile, err := os.ReadFile(dirname + "/clouds.yaml")
	if err == nil {
		err = yamlv3.Unmarshal(confFile, cred)
	}
	return cred, err
}

// newOpenStackClient initializes an openstack client
// serviceType the type of the client, currently only supports indentity and object-store
func newOpenStackClient(cred *openstackCredentials, serviceType string) *gophercloud.ServiceClient {
	var client *gophercloud.ServiceClient
	opts := gophercloud.AuthOptions{
		IdentityEndpoint: cred.Clouds.Openstack.Auth.AuthURL,
		Username:         cred.Clouds.Openstack.Auth.Username,
		Password:         cred.Clouds.Openstack.Auth.Password,
		TenantID:         cred.Clouds.Openstack.Auth.ProjectID,
		DomainName:       cred.Clouds.Openstack.Auth.UserDomainName,
	}
	provider, err := openstack.AuthenticatedClient(opts)
	o.Expect(err).NotTo(o.HaveOccurred())
	switch serviceType {
	case "identity":
		{
			client, err = openstack.NewIdentityV3(provider, gophercloud.EndpointOpts{Region: cred.Clouds.Openstack.RegionName})
		}
	case "object-store":
		{
			client, err = openstack.NewObjectStorageV1(provider, gophercloud.EndpointOpts{Region: cred.Clouds.Openstack.RegionName})
		}
	}
	o.Expect(err).NotTo(o.HaveOccurred())
	return client
}

func getAuthenticatedUserID(providerClient *gophercloud.ProviderClient) (string, error) {
	//copied from https://github.com/gophercloud/gophercloud/blob/master/auth_result.go
	res := providerClient.GetAuthResult()
	if res == nil {
		//ProviderClient did not use openstack.Authenticate(), e.g. because token
		//was set manually with ProviderClient.SetToken()
		return "", fmt.Errorf("no AuthResult available")
	}
	switch r := res.(type) {
	case tokens3.CreateResult:
		u, err := r.ExtractUser()
		if err != nil {
			return "", err
		}
		return u.ID, nil
	default:
		return "", fmt.Errorf("got unexpected AuthResult type %t", r)
	}
}

func getOpenStackUserIDAndDomainID(cred *openstackCredentials) (string, string) {
	//normal users don't have permisson to list users, to get user info, must provide user ID
	//here gets user ID from auth result
	client := newOpenStackClient(cred, "identity")
	userID, err := getAuthenticatedUserID(client.ProviderClient)
	o.Expect(err).NotTo(o.HaveOccurred())
	user, err := users.Get(client, userID).Extract()
	o.Expect(err).NotTo(o.HaveOccurred())
	return userID, user.DomainID
}

func createOpenStackContainer(client *gophercloud.ServiceClient, name string) error {
	pager := containers.List(client, &containers.ListOpts{Full: true, Prefix: name})
	exist := false
	// check if the container exists or not
	err := pager.EachPage(func(page pagination.Page) (bool, error) {
		containerNames, err := containers.ExtractNames(page)
		o.Expect(err).NotTo(o.HaveOccurred())
		for _, n := range containerNames {
			if n == name {
				exist = true
				break
			}
		}
		return true, nil
	})
	if err != nil {
		return err
	}
	if exist {
		err = emptyOpenStackContainer(client, name)
		o.Expect(err).NotTo(o.HaveOccurred())
	}
	// create the container
	res := containers.Create(client, name, containers.CreateOpts{})
	_, err = res.Extract()
	return err
}

func deleteOpenStackContainer(client *gophercloud.ServiceClient, name string) error {
	err := emptyOpenStackContainer(client, name)
	o.Expect(err).NotTo(o.HaveOccurred())
	response := containers.Delete(client, name)
	_, err = response.Extract()
	return err
}

func emptyOpenStackContainer(client *gophercloud.ServiceClient, name string) error {
	objectNames, err := listOpenStackContainerObjects(client, name)
	o.Expect(err).NotTo(o.HaveOccurred())
	for _, obj := range objectNames {
		err = deleteObjectsFromOpenStackContainer(client, name, obj)
	}
	return err
}

func listOpenStackContainerObjects(client *gophercloud.ServiceClient, name string) ([]string, error) {
	pager := objects.List(client, name, &objects.ListOpts{Full: true})
	names := []string{}
	err := pager.EachPage(func(page pagination.Page) (bool, error) {
		objectNames, err := objects.ExtractNames(page)
		names = append(names, objectNames...)
		return true, err
	})
	return names, err
}

func deleteObjectsFromOpenStackContainer(client *gophercloud.ServiceClient, containerName, objectName string) error {
	result := objects.Delete(client, containerName, objectName, objects.DeleteOpts{})
	_, err := result.Extract()
	return err
}

func createSecretForSwiftContainer(oc *exutil.CLI, containerName, secretName, ns string, cred *openstackCredentials) error {
	userID, domainID := getOpenStackUserIDAndDomainID(cred)
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

// checkODF check if the ODF is installed in the cluster or not
// here only checks the sc/ocs-storagecluster-ceph-rbd and svc/s3
func checkODF(oc *exutil.CLI) bool {
	scFound, svcFound := false, false
	scs, err := oc.AdminKubeClient().StorageV1().StorageClasses().List(context.Background(), metav1.ListOptions{})
	o.Expect(err).NotTo(o.HaveOccurred())
	for _, sc := range scs.Items {
		if sc.Name == "ocs-storagecluster-ceph-rbd" {
			scFound = true
			break
		}
	}
	_, err = oc.AdminKubeClient().CoreV1().Services("openshift-storage").Get(context.Background(), "s3", metav1.GetOptions{})
	if err == nil {
		svcFound = true
	}
	return scFound && svcFound
}

// checkMinIO
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

// lokiStack contains the configurations of loki stack
type lokiStack struct {
	name          string // lokiStack name
	namespace     string // lokiStack namespace
	tSize         string // size
	storageType   string // the backend storage type, currently support s3, gcs, azure, swift, ODF and minIO
	storageSecret string // the secret name for loki to use to connect to backend storage
	storageClass  string // storage class name
	bucketName    string // the butcket or the container name where loki stores it's data in
	template      string // the file used to create the loki stack
}

func (l lokiStack) setTSize(size string) lokiStack {
	l.tSize = size
	return l
}

// prepareResourcesForLokiStack creates buckets/containers in backend storage provider, and creates the secret for Loki to use
func (l lokiStack) prepareResourcesForLokiStack(oc *exutil.CLI) error {
	var err error
	if len(l.bucketName) == 0 {
		return fmt.Errorf("the bucketName should not be empty")
	}
	switch l.storageType {
	case "s3":
		{
			cred := getAWSCredentialFromCluster(oc)
			client := newAWSS3Client(oc, cred)
			err = createAWSS3Bucket(oc, client, l.bucketName, cred)
			if err != nil {
				return err
			}
			err = createSecretForAWSS3Bucket(oc, l.bucketName, l.storageSecret, l.namespace)
		}
	case "azure":
		{
			client := newAzureContainerClient(oc, l.bucketName)
			err = createAzureStorageBlobContainer(client)
			if err != nil {
				return err
			}
			err = createSecretForAzureContainer(oc, l.bucketName, l.storageSecret, l.namespace)
		}
	case "gcs":
		{
			err = createGCSBucket(getGCPProjectID(oc), l.bucketName)
			if err != nil {
				return err
			}
			err = createSecretForGCSBucket(oc, l.bucketName, l.storageSecret, l.namespace)
		}
	case "swift":
		{
			cred, err1 := getOpenStackCredentials(oc)
			o.Expect(err1).NotTo(o.HaveOccurred())
			client := newOpenStackClient(cred, "object-store")
			err = createOpenStackContainer(client, l.bucketName)
			if err != nil {
				return err
			}
			err = createSecretForSwiftContainer(oc, l.bucketName, l.storageSecret, l.namespace, cred)
		}
	case "odf":
		{
			cred := getODFCreds(oc)
			client := newAWSS3Client(oc, cred)
			err = createAWSS3Bucket(oc, client, l.bucketName, cred)
			if err != nil {
				return err
			}
			err = createSecretForODFBucket(oc, l.bucketName, l.storageSecret, l.namespace)
		}
	case "minio":
		{
			cred := getMinIOCreds(oc, minioNS)
			client := newAWSS3Client(oc, cred)
			err = createAWSS3Bucket(oc, client, l.bucketName, cred)
			if err != nil {
				return err
			}
			err = createSecretForMinIOBucket(oc, l.bucketName, l.storageSecret, l.namespace, minioNS)
		}
	}
	return err
}

// deployLokiStack creates the lokiStack CR with basic settings: name, namespace, size, storage.secret.name, storage.secret.type, storageClassName
// optionalParameters is designed for adding parameters to deploy lokiStack with different tenants or some other settings
func (l lokiStack) deployLokiStack(oc *exutil.CLI, optionalParameters ...string) error {
	var storage string
	if l.storageType == "odf" || l.storageType == "minio" {
		storage = "s3"
	} else {
		storage = l.storageType
	}
	parameters := []string{"-f", l.template, "-n", l.namespace, "-p", "NAME=" + l.name, "NAMESPACE=" + l.namespace, "SIZE=" + l.tSize, "SECRET_NAME=" + l.storageSecret, "STORAGE_TYPE=" + storage, "STORAGE_CLASS=" + l.storageClass}
	if len(optionalParameters) != 0 {
		parameters = append(parameters, optionalParameters...)
	}
	file, err := processTemplate(oc, parameters...)
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Can not process %v", parameters))
	err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", file, "-n", l.namespace).Execute()
	ls := resource{"lokistack", l.name, l.namespace}
	ls.WaitForResourceToAppear(oc)
	return err
}

func (l lokiStack) waitForLokiStackToBeReady(oc *exutil.CLI) {
	for _, deploy := range []string{l.name + "-distributor", l.name + "-gateway", l.name + "-querier", l.name + "-query-frontend"} {
		WaitForDeploymentPodsToBeReady(oc, l.namespace, deploy)
	}
	for _, ss := range []string{l.name + "-compactor", l.name + "-index-gateway", l.name + "-ingester"} {
		waitForStatefulsetReady(oc, l.namespace, ss)
	}
}

func (l lokiStack) removeLokiStack(oc *exutil.CLI) {
	resource{"lokistack", l.name, l.namespace}.clear(oc)
	_ = oc.AsAdmin().WithoutNamespace().Run("delete").Args("pvc", "-n", l.namespace, "-l", "app.kubernetes.io/instance="+l.name).Execute()
}

func (l lokiStack) removeObjectStorage(oc *exutil.CLI) {
	resource{"secret", l.storageSecret, l.namespace}.clear(oc)
	var err error
	switch l.storageType {
	case "s3":
		{
			cred := getAWSCredentialFromCluster(oc)
			client := newAWSS3Client(oc, cred)
			err = deleteAWSS3Bucket(client, l.bucketName)
		}
	case "azure":
		{
			client := newAzureContainerClient(oc, l.bucketName)
			err = deleteAzureStorageBlobContainer(client)
		}
	case "gcs":
		{
			err = deleteGCSBucket(l.bucketName)
		}
	case "swift":
		{
			cred, err1 := getOpenStackCredentials(oc)
			o.Expect(err1).NotTo(o.HaveOccurred())
			client := newOpenStackClient(cred, "object-store")
			err = deleteOpenStackContainer(client, l.bucketName)
		}
	case "odf":
		{
			cred := getODFCreds(oc)
			client := newAWSS3Client(oc, cred)
			err = deleteAWSS3Bucket(client, l.bucketName)
		}
	case "minio":
		{
			cred := getMinIOCreds(oc, minioNS)
			client := newAWSS3Client(oc, cred)
			err = deleteAWSS3Bucket(client, l.bucketName)
		}
	}
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (l lokiStack) createPipelineSecret(oc *exutil.CLI, name, namespace, token string) {
	dirname := "/tmp/" + oc.Namespace() + getRandomString()
	defer os.RemoveAll(dirname)
	err := os.MkdirAll(dirname, 0777)
	o.Expect(err).NotTo(o.HaveOccurred())

	err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("cm/"+l.name+"-ca-bundle", "-n", l.namespace, "--keys=service-ca.crt", "--confirm", "--to="+dirname).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())

	if token != "" {
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("secret", "generic", name, "-n", namespace, "--from-file=ca-bundle.crt="+dirname+"/service-ca.crt", "--from-literal=token="+token).Execute()
	} else {
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("secret", "generic", name, "-n", namespace, "--from-file=ca-bundle.crt="+dirname+"/service-ca.crt").Execute()
	}
	o.Expect(err).NotTo(o.HaveOccurred())

}

func grantLokiPermissionsToSA(oc *exutil.CLI, rbacName, sa, ns string) {
	rbac := exutil.FixturePath("testdata", "logging", "lokistack", "loki-rbac.yaml")
	file, err := processTemplate(oc, "-f", rbac, "-p", "NAME="+rbacName, "-p", "SA="+sa, "NAMESPACE="+ns)
	o.Expect(err).NotTo(o.HaveOccurred())
	err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", file).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func removeLokiStackPermissionFromSA(oc *exutil.CLI, rbacName string) {
	err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("clusterrole/"+rbacName, "clusterrolebinding/"+rbacName).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

// TODO: add an option to provide TLS config
type lokiClient struct {
	username        string //Username for HTTP basic auth.
	password        string //Password for HTTP basic auth
	address         string //Server address.
	orgID           string //adds X-Scope-OrgID to API requests for representing tenant ID. Useful for requesting tenant data when bypassing an auth gateway.
	bearerToken     string //adds the Authorization header to API requests for authentication purposes.
	bearerTokenFile string //adds the Authorization header to API requests for authentication purposes.
	retries         int    //How many times to retry each query when getting an error response from Loki.
	queryTags       string //adds X-Query-Tags header to API requests.
	quiet           bool   //Suppress query metadata.
}

// newLokiClient initializes a lokiClient with server address
func newLokiClient(routeAddress string) *lokiClient {
	client := &lokiClient{}
	client.address = routeAddress
	client.retries = 5
	client.quiet = true
	return client
}

// retry sets how many times to retry each query
func (c *lokiClient) retry(retry int) *lokiClient {
	nc := *c
	nc.retries = retry
	return &nc
}

// withToken sets the token used to do query
func (c *lokiClient) withToken(bearerToken string) *lokiClient {
	nc := *c
	nc.bearerToken = bearerToken
	return &nc
}

/*
func (c *lokiClient) withTokenFile(bearerTokenFile string) *lokiClient {
	nc := *c
	nc.bearerTokenFile = bearerTokenFile
	return &nc
}
*/

func (c *lokiClient) getHTTPRequestHeader() (http.Header, error) {
	h := make(http.Header)
	if c.username != "" && c.password != "" {
		h.Set(
			"Authorization",
			"Basic "+base64.StdEncoding.EncodeToString([]byte(c.username+":"+c.password)),
		)
	}
	h.Set("User-Agent", "loki-logcli")

	if c.orgID != "" {
		h.Set("X-Scope-OrgID", c.orgID)
	}

	if c.queryTags != "" {
		h.Set("X-Query-Tags", c.queryTags)
	}

	if (c.username != "" || c.password != "") && (len(c.bearerToken) > 0 || len(c.bearerTokenFile) > 0) {
		return nil, fmt.Errorf("at most one of HTTP basic auth (username/password), bearer-token & bearer-token-file is allowed to be configured")
	}

	if len(c.bearerToken) > 0 && len(c.bearerTokenFile) > 0 {
		return nil, fmt.Errorf("at most one of the options bearer-token & bearer-token-file is allowed to be configured")
	}

	if c.bearerToken != "" {
		h.Set("Authorization", "Bearer "+c.bearerToken)
	}

	if c.bearerTokenFile != "" {
		b, err := os.ReadFile(c.bearerTokenFile)
		if err != nil {
			return nil, fmt.Errorf("unable to read authorization credentials file %s: %s", c.bearerTokenFile, err)
		}
		bearerToken := strings.TrimSpace(string(b))
		h.Set("Authorization", "Bearer "+bearerToken)
	}
	return h, nil
}

func (c *lokiClient) doRequest(path, query string, quiet bool, out interface{}) error {
	us, err := buildURL(c.address, path, query)
	if err != nil {
		return err
	}
	if !quiet {
		e2e.Logf(us)
	}

	req, err := http.NewRequest("GET", us, nil)
	if err != nil {
		return err
	}

	h, err := c.getHTTPRequestHeader()
	if err != nil {
		return err
	}
	req.Header = h

	var tr *http.Transport
	proxy := getProxyFromEnv()
	if len(proxy) > 0 {
		proxyURL, err := url.Parse(proxy)
		o.Expect(err).NotTo(o.HaveOccurred())
		tr = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			Proxy:           http.ProxyURL(proxyURL),
		}
	} else {
		tr = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}

	client := &http.Client{Transport: tr}

	var resp *http.Response
	attempts := c.retries + 1
	success := false

	for attempts > 0 {
		attempts--

		resp, err = client.Do(req)
		if err != nil {
			e2e.Logf("error sending request %v", err)
			continue
		}
		if resp.StatusCode/100 != 2 {
			buf, _ := io.ReadAll(resp.Body) // nolint
			e2e.Logf("Error response from server: %s (%v) attempts remaining: %d", string(buf), err, attempts)
			if err := resp.Body.Close(); err != nil {
				e2e.Logf("error closing body", err)
			}
			continue
		}
		success = true
		break
	}
	if !success {
		return fmt.Errorf("run out of attempts while querying the server")
	}

	defer func() {
		if err := resp.Body.Close(); err != nil {
			e2e.Logf("error closing body", err)
		}
	}()
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *lokiClient) doQuery(path string, query string, quiet bool) (*lokiQueryResponse, error) {
	var err error
	var r lokiQueryResponse

	if err = c.doRequest(path, query, quiet, &r); err != nil {
		return nil, err
	}

	return &r, nil
}

/*
//query uses the /api/v1/query endpoint to execute an instant query
func (c *lokiClient) query(logType string, queryStr string, limit int, forward bool, time time.Time) (*lokiQueryResponse, error) {
	direction := func() string {
		if forward {
			return "FORWARD"
		}
		return "BACKWARD"
	}
	qsb := newQueryStringBuilder()
	qsb.setString("query", queryStr)
	qsb.setInt("limit", int64(limit))
	qsb.setInt("time", time.UnixNano())
	qsb.setString("direction", direction())
	logPath := apiPath + logType + queryPath
	return c.doQuery(logPath, qsb.encode(), c.quiet)
}
*/

// queryRange uses the /api/v1/query_range endpoint to execute a range query
// logType: application, infrastructure, audit
// queryStr: string to filter logs, for example: "{kubernetes_namespace_name="test"}"
// limit: max log count
// start: Start looking for logs at this absolute time(inclusive), e.g.: time.Now().Add(time.Duration(-1)*time.Hour) means 1 hour ago
// end: Stop looking for logs at this absolute time (exclusive)
// forward: true means scan forwards through logs, false means scan backwards through logs
func (c *lokiClient) queryRange(logType string, queryStr string, limit int, start, end time.Time, forward bool) (*lokiQueryResponse, error) {
	direction := func() string {
		if forward {
			return "FORWARD"
		}
		return "BACKWARD"
	}
	params := newQueryStringBuilder()
	params.setString("query", queryStr)
	params.setInt32("limit", limit)
	params.setInt("start", start.UnixNano())
	params.setInt("end", end.UnixNano())
	params.setString("direction", direction())
	logPath := ""
	if len(logType) > 0 {
		logPath = apiPath + logType + queryRangePath
	} else {
		logPath = queryRangePath
	}

	return c.doQuery(logPath, params.encode(), c.quiet)
}

func (c *lokiClient) searchLogsInLoki(logType, query string) (*lokiQueryResponse, error) {
	res, err := c.queryRange(logType, query, 5, time.Now().Add(time.Duration(-1)*time.Hour), time.Now(), false)
	return res, err
}

func (c *lokiClient) searchByKey(logType, key, value string) (*lokiQueryResponse, error) {
	res, err := c.searchLogsInLoki(logType, "{"+key+"=\""+value+"\"}")
	return res, err
}

func (c *lokiClient) searchByNamespace(logType, projectName string) (*lokiQueryResponse, error) {
	res, err := c.searchLogsInLoki(logType, "{kubernetes_namespace_name=\""+projectName+"\"}")
	return res, err
}

// extractLogEntities extract the log entities from loki query response, designed for checking the content of log data in Loki
func extractLogEntities(lokiQueryResult *lokiQueryResponse) []LogEntity {
	var lokiLogs []LogEntity
	// convert interface{} to []string
	convert := func(t interface{}) []string {
		var data []string
		switch reflect.TypeOf(t).Kind() {
		case reflect.Slice, reflect.Array:
			s := reflect.ValueOf(t)
			for i := 0; i < s.Len(); i++ {
				data = append(data, fmt.Sprintln(s.Index(i)))
			}
		}
		return data
	}
	/*
		value example:
		  [
		    timestamp,
			log data
		  ]
	*/
	for _, res := range lokiQueryResult.Data.Result {
		for _, value := range res.Values {
			lokiLog := LogEntity{}
			// only process log data, drop timestamp
			json.Unmarshal([]byte(convert(value)[1]), &lokiLog)
			lokiLogs = append(lokiLogs, lokiLog)
		}
	}
	return lokiLogs
}

// listLabelValues uses the /api/v1/label endpoint to list label values
func (c *lokiClient) listLabelValues(logType, name string, start, end time.Time) (*labelResponse, error) {
	lpath := fmt.Sprintf(labelValuesPath, url.PathEscape(name))
	var labelResponse labelResponse
	params := newQueryStringBuilder()
	params.setInt("start", start.UnixNano())
	params.setInt("end", end.UnixNano())

	path := ""
	if len(logType) > 0 {
		path = apiPath + logType + lpath
	} else {
		path = lpath
	}

	if err := c.doRequest(path, params.encode(), c.quiet, &labelResponse); err != nil {
		return nil, err
	}
	return &labelResponse, nil
}

// listLabelNames uses the /api/v1/label endpoint to list label names
func (c *lokiClient) listLabelNames(logType string, start, end time.Time) (*labelResponse, error) {
	var labelResponse labelResponse
	params := newQueryStringBuilder()
	params.setInt("start", start.UnixNano())
	params.setInt("end", end.UnixNano())
	path := ""
	if len(logType) > 0 {
		path = apiPath + logType + labelsPath
	} else {
		path = labelsPath
	}

	if err := c.doRequest(path, params.encode(), c.quiet, &labelResponse); err != nil {
		return nil, err
	}
	return &labelResponse, nil
}

// listLabels gets the label names or values
func (c *lokiClient) listLabels(logType, labelName string, start, end time.Time) []string {
	var labelResponse *labelResponse
	var err error
	if len(labelName) > 0 {
		labelResponse, err = c.listLabelValues(logType, labelName, start, end)
	} else {
		labelResponse, err = c.listLabelNames(logType, start, end)
	}
	if err != nil {
		e2e.Failf("Error doing request: %+v", err)
	}
	return labelResponse.Data
}

// buildURL concats a url `http://foo/bar` with a path `/buzz`.
func buildURL(u, p, q string) (string, error) {
	url, err := url.Parse(u)
	if err != nil {
		return "", err
	}
	url.Path = path.Join(url.Path, p)
	url.RawQuery = q
	return url.String(), nil
}

type queryStringBuilder struct {
	values url.Values
}

func newQueryStringBuilder() *queryStringBuilder {
	return &queryStringBuilder{
		values: url.Values{},
	}
}

func (b *queryStringBuilder) setString(name, value string) {
	b.values.Set(name, value)
}

func (b *queryStringBuilder) setInt(name string, value int64) {
	b.setString(name, strconv.FormatInt(value, 10))
}

func (b *queryStringBuilder) setInt32(name string, value int) {
	b.setString(name, strconv.Itoa(value))
}

/*
func (b *queryStringBuilder) setStringArray(name string, values []string) {
	for _, v := range values {
		b.values.Add(name, v)
	}
}
func (b *queryStringBuilder) setFloat32(name string, value float32) {
	b.setString(name, strconv.FormatFloat(float64(value), 'f', -1, 32))
}
func (b *queryStringBuilder) setFloat(name string, value float64) {
	b.setString(name, strconv.FormatFloat(value, 'f', -1, 64))
}
*/

// encode returns the URL-encoded query string based on key-value
// parameters added to the builder calling Set functions.
func (b *queryStringBuilder) encode() string {
	return b.values.Encode()
}

// getSchedulableLinuxWorkerNodes returns a group of nodes that match the requirements:
// os: linux, role: worker, status: ready, schedulable
func getSchedulableLinuxWorkerNodes(oc *exutil.CLI) ([]v1.Node, error) {
	var nodes, workers []v1.Node
	linuxNodes, err := oc.AdminKubeClient().CoreV1().Nodes().List(context.Background(), metav1.ListOptions{LabelSelector: "kubernetes.io/os=linux"})
	// get schedulable linux worker nodes
	for _, node := range linuxNodes.Items {
		if _, ok := node.Labels["node-role.kubernetes.io/worker"]; ok && !node.Spec.Unschedulable {
			workers = append(workers, node)
		}
	}
	// get ready nodes
	for _, worker := range workers {
		for _, con := range worker.Status.Conditions {
			if con.Type == "Ready" && con.Status == "True" {
				nodes = append(nodes, worker)
				break
			}
		}
	}
	return nodes, err
}

// getPodsNodesMap returns all the running pods in each node
func getPodsNodesMap(oc *exutil.CLI, nodes []v1.Node) map[string][]v1.Pod {
	podsMap := make(map[string][]v1.Pod)
	projects, err := oc.AdminKubeClient().CoreV1().Namespaces().List(context.Background(), metav1.ListOptions{})
	o.Expect(err).NotTo(o.HaveOccurred())

	// get pod list in each node
	for _, project := range projects.Items {
		pods, err := oc.AdminKubeClient().CoreV1().Pods(project.Name).List(context.Background(), metav1.ListOptions{})
		o.Expect(err).NotTo(o.HaveOccurred())
		for _, pod := range pods.Items {
			if pod.Status.Phase != "Failed" && pod.Status.Phase != "Succeeded" {
				podsMap[pod.Spec.NodeName] = append(podsMap[pod.Spec.NodeName], pod)
			}
		}
	}

	var nodeNames []string
	for _, node := range nodes {
		nodeNames = append(nodeNames, node.Name)
	}
	// if the key is not in nodes list, remove the element from the map
	for podmap := range podsMap {
		if !contain(nodeNames, podmap) {
			delete(podsMap, podmap)
		}
	}
	return podsMap
}

type resList struct {
	cpu    int64
	memory int64
}

// getRequestedResourcesNodesMap returns the requested CPU and Memory in each node
func getRequestedResourcesNodesMap(oc *exutil.CLI, nodes []v1.Node) map[string]resList {
	rmap := make(map[string]resList)
	podsMap := getPodsNodesMap(oc, nodes)
	for nodeName := range podsMap {
		var totalRequestedCPU, totalRequestedMemory int64
		for _, pod := range podsMap[nodeName] {
			for _, container := range pod.Spec.Containers {
				totalRequestedCPU += container.Resources.Requests.Cpu().MilliValue()
				totalRequestedMemory += container.Resources.Requests.Memory().MilliValue()
			}
		}
		rmap[nodeName] = resList{totalRequestedCPU, totalRequestedMemory}
	}
	return rmap
}

// getAllocatableResourcesNodesMap returns the allocatable CPU and Memory in each node
func getAllocatableResourcesNodesMap(nodes []v1.Node) map[string]resList {
	rmap := make(map[string]resList)
	for _, node := range nodes {
		rmap[node.Name] = resList{node.Status.Allocatable.Cpu().MilliValue(), node.Status.Allocatable.Memory().MilliValue()}
	}
	return rmap
}

// getRemainingResourcesNodesMap returns the remaning CPU and Memory in each node
func getRemainingResourcesNodesMap(oc *exutil.CLI, nodes []v1.Node) map[string]resList {
	rmap := make(map[string]resList)
	requested := getRequestedResourcesNodesMap(oc, nodes)
	allocatable := getAllocatableResourcesNodesMap(nodes)

	for _, node := range nodes {
		rmap[node.Name] = resList{allocatable[node.Name].cpu - requested[node.Name].cpu, allocatable[node.Name].memory - requested[node.Name].memory}
	}
	return rmap
}

// compareClusterResources compares the remaning resource with the requested resource provide by user
func compareClusterResources(oc *exutil.CLI, cpu, memory string) bool {
	nodes, err := getSchedulableLinuxWorkerNodes(oc)
	o.Expect(err).NotTo(o.HaveOccurred())
	var remainingCPU, remainingMemory int64
	re := getRemainingResourcesNodesMap(oc, nodes)
	for _, node := range nodes {
		remainingCPU += re[node.Name].cpu
		remainingMemory += re[node.Name].memory
	}

	requiredCPU, _ := k8sresource.ParseQuantity(cpu)
	requiredMemory, _ := k8sresource.ParseQuantity(memory)
	e2e.Logf("the required cpu is: %d, and the required memory is: %d", requiredCPU.MilliValue(), requiredMemory.MilliValue())
	e2e.Logf("the remaining cpu is: %d, and the remaning memory is: %d", remainingCPU, remainingMemory)
	return remainingCPU > requiredCPU.MilliValue() && remainingMemory > requiredMemory.MilliValue()
}

// validateInfraAndResourcesForLoki checks cluster remaning resources and platform type
// supportedPlatforms the platform types which the case can be executed on, if it's empty, then skip this check
func validateInfraAndResourcesForLoki(oc *exutil.CLI, supportedPlatforms []string, reqMemory, reqCPU string) bool {
	currentPlatform := exutil.CheckPlatform(oc)
	if currentPlatform == "aws" {
		// skip the case on aws sts clusters
		_, err := oc.AdminKubeClient().CoreV1().Secrets("kube-system").Get(context.Background(), "aws-creds", metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return false
		}
	}
	if len(supportedPlatforms) > 0 {
		return contain(supportedPlatforms, currentPlatform) && compareClusterResources(oc, reqCPU, reqMemory)
	}
	return compareClusterResources(oc, reqCPU, reqMemory)
}

type externalLoki struct {
	name      string
	namespace string
}

func (l externalLoki) deployLoki(oc *exutil.CLI) {
	//Create configmap for Loki
	CMTemplate := exutil.FixturePath("testdata", "logging", "external-log-stores", "loki", "loki-configmap.yaml")
	lokiCM := resource{"configmap", l.name, l.namespace}
	err := lokiCM.applyFromTemplate(oc, "-n", l.namespace, "-f", CMTemplate, "-p", "LOKINAMESPACE="+l.namespace, "-p", "LOKICMNAME="+l.name)
	o.Expect(err).NotTo(o.HaveOccurred())

	//Create Deployment for Loki
	deployTemplate := exutil.FixturePath("testdata", "logging", "external-log-stores", "loki", "loki-deployment.yaml")
	lokiDeploy := resource{"deployment", l.name, l.namespace}
	err = lokiDeploy.applyFromTemplate(oc, "-n", l.namespace, "-f", deployTemplate, "-p", "LOKISERVERNAME="+l.name, "-p", "LOKINAMESPACE="+l.namespace, "-p", "LOKICMNAME="+l.name)
	o.Expect(err).NotTo(o.HaveOccurred())

	//Expose Loki as a Service
	WaitForDeploymentPodsToBeReady(oc, l.namespace, l.name)
	err = oc.AsAdmin().WithoutNamespace().Run("expose").Args("-n", l.namespace, "deployment", l.name).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())

	// expose loki route
	err = oc.AsAdmin().WithoutNamespace().Run("expose").Args("-n", l.namespace, "svc", l.name).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (l externalLoki) remove(oc *exutil.CLI) {
	resource{"configmap", l.name, l.namespace}.clear(oc)
	resource{"deployment", l.name, l.namespace}.clear(oc)
	resource{"svc", l.name, l.namespace}.clear(oc)
	resource{"route", l.name, l.namespace}.clear(oc)
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
