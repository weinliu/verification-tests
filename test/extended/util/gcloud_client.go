package util

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	o "github.com/onsi/gomega"
	"google.golang.org/api/iterator"
)

// Gcloud struct
type Gcloud struct {
	ProjectID string
}

// Login logins to the gcloud. This function needs to be used only once to login into the GCP.
// the gcloud client is only used for the cluster which is on gcp platform.
func (gcloud *Gcloud) Login() *Gcloud {
	checkCred, err := exec.Command("bash", "-c", `gcloud auth list --format="value(account)"`).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if string(checkCred) != "" {
		return gcloud
	}
	credErr := exec.Command("bash", "-c", "gcloud auth login --cred-file=$GOOGLE_APPLICATION_CREDENTIALS").Run()
	o.Expect(credErr).NotTo(o.HaveOccurred())
	projectErr := exec.Command("bash", "-c", fmt.Sprintf("gcloud config set project %s", gcloud.ProjectID)).Run()
	o.Expect(projectErr).NotTo(o.HaveOccurred())
	return gcloud
}

// GetIntSvcExternalIP returns the int svc external IP
func (gcloud *Gcloud) GetIntSvcExternalIP(infraID string) (string, error) {
	externalIP, err := exec.Command("bash", "-c", fmt.Sprintf(`gcloud compute instances list --filter="%s-int-svc"  --format="value(EXTERNAL_IP)"`, infraID)).Output()
	if string(externalIP) == "" {
		return "", errors.New("additional VM is not found")
	}
	return strings.Trim(string(externalIP), "\n"), err
}

// GetIntSvcInternalIP returns the int svc internal IP
func (gcloud *Gcloud) GetIntSvcInternalIP(infraID string) (string, error) {
	internalIP, err := exec.Command("bash", "-c", fmt.Sprintf(`gcloud compute instances list --filter="%s-int-svc"  --format="value(networkInterfaces.networkIP)"`, infraID)).Output()
	if string(internalIP) == "" {
		return "", errors.New("additional VM is not found")
	}
	return strings.Trim(string(internalIP), "\n"), err
}

// GetFirewallAllowPorts returns firewall allow ports
func (gcloud *Gcloud) GetFirewallAllowPorts(ruleName string) (string, error) {
	ports, err := exec.Command("bash", "-c", fmt.Sprintf(`gcloud compute firewall-rules list --filter="name=(%s)" --format="value(ALLOW)"`, ruleName)).Output()
	return strings.Trim(string(ports), "\n"), err
}

// UpdateFirewallAllowPorts updates the firewall allow ports
func (gcloud *Gcloud) UpdateFirewallAllowPorts(ruleName string, ports string) error {
	return exec.Command("bash", "-c", fmt.Sprintf(`gcloud compute firewall-rules update %s --allow %s`, ruleName, ports)).Run()
}

// GetZone get zone information for an instance
func (gcloud *Gcloud) GetZone(infraID string, workerName string) (string, error) {
	output, err := exec.Command("bash", "-c", fmt.Sprintf(`gcloud compute instances list --filter="%s" --format="value(ZONE)"`, workerName)).Output()
	if string(output) == "" {
		return "", errors.New("Zone info for the instance is not found")
	}
	return string(output), err
}

// StartInstance Bring GCP node/instance back up
func (gcloud *Gcloud) StartInstance(nodeName string, zoneName string) error {
	return exec.Command("bash", "-c", fmt.Sprintf(`gcloud compute instances start %s --zone=%s`, nodeName, zoneName)).Run()
}

// StopInstance Shutdown GCP node/instance
func (gcloud *Gcloud) StopInstance(nodeName string, zoneName string) error {
	return exec.Command("bash", "-c", fmt.Sprintf(`gcloud compute instances stop %s --zone=%s`, nodeName, zoneName)).Run()
}

// GetGcpInstanceByNode returns the instance name
func (gcloud *Gcloud) GetGcpInstanceByNode(nodeIdentity string) (string, error) {
	instanceID, err := exec.Command("bash", "-c", fmt.Sprintf(`gcloud compute instances list --filter="%s" --format="value(name)"`, nodeIdentity)).Output()
	if string(instanceID) == "" {
		return "", fmt.Errorf("VM is not found")
	}
	return strings.Trim(string(instanceID), "\n"), err
}

// GetGcpInstanceStateByNode returns the instance state
func (gcloud *Gcloud) GetGcpInstanceStateByNode(nodeIdentity string) (string, error) {
	instanceState, err := exec.Command("bash", "-c", fmt.Sprintf(`gcloud compute instances list --filter="%s" --format="value(status)"`, nodeIdentity)).Output()
	if string(instanceState) == "" {
		return "", fmt.Errorf("Not able to get instance state")
	}
	return strings.Trim(string(instanceState), "\n"), err
}

// StopInstanceAsync Shutdown GCP node/instance with async
func (gcloud *Gcloud) StopInstanceAsync(nodeName string, zoneName string) error {
	return exec.Command("bash", "-c", fmt.Sprintf(`gcloud compute instances stop %s --async --zone=%s`, nodeName, zoneName)).Run()
}

// CreateGCSBucket creates a GCS bucket in a project
func CreateGCSBucket(projectID, bucketName string) error {
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
	buckets, err := ListGCSBuckets(*client, projectID)
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
		return EmptyGCSBucket(*client, bucketName)
	}

	bucket := client.Bucket(bucketName)
	if err := bucket.Create(ctx, projectID, &storage.BucketAttrs{}); err != nil {
		return fmt.Errorf("Bucket(%q).Create: %v", bucketName, err)
	}
	fmt.Printf("Created bucket %v\n", bucketName)
	return nil
}

// ListGCSBuckets gets all the bucket names under the projectID
func ListGCSBuckets(client storage.Client, projectID string) ([]string, error) {
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

// ListObjestsInGCSBucket gets all the objects in a bucket
func ListObjestsInGCSBucket(client storage.Client, bucket string) ([]string, error) {
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

// DeleteFilesInGCSBucket removes the object in the bucket
func DeleteFilesInGCSBucket(client storage.Client, object, bucket string) error {
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

// EmptyGCSBucket removes all the objects in the bucket
func EmptyGCSBucket(client storage.Client, bucket string) error {
	objects, err := ListObjestsInGCSBucket(client, bucket)
	if err != nil {
		return err
	}

	for _, object := range objects {
		err = DeleteFilesInGCSBucket(client, object, bucket)
		if err != nil {
			return err
		}
	}
	return nil
}

// DeleteGCSBucket deletes the GCS bucket
func DeleteGCSBucket(bucketName string) error {
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("storage.NewClient: %v", err)
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(ctx, time.Second*60)
	defer cancel()

	// remove objects
	err = EmptyGCSBucket(*client, bucketName)
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
