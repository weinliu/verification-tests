package hypershift

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"io/ioutil"
	e2e "k8s.io/kubernetes/test/e2e/framework"
	"strings"
)

type installHelper struct {
	oc         *exutil.CLI
	bucketName string
	region     string
	dir        string
	awsClient  *s3.Client
}

type createCluster struct {
	PullSecret       string `param:"pull-secret"`
	AWSCreds         string `param:"aws-creds"`
	Name             string `param:"name"`
	BaseDomain       string `param:"base-domain"`
	Namespace        string `param:"namespace"`
	NodePoolReplicas *int   `param:"node-pool-replicas"`
	Region           string `param:"region"`
}

func (c *createCluster) withName(name string) *createCluster {
	c.Name = name
	return c
}

func (c *createCluster) withNamespace(Namespace string) *createCluster {
	c.Namespace = Namespace
	return c
}

func (c *createCluster) withNodePoolReplicas(NodePoolReplicas int) *createCluster {
	c.NodePoolReplicas = &NodePoolReplicas
	return c
}

func (receiver *installHelper) createClusterAWSCommonBuilder() *createCluster {
	nodePoolReplicas := 3
	return &createCluster{
		PullSecret:       receiver.dir + "/.dockerconfigjson",
		AWSCreds:         receiver.dir + "/credentials",
		BaseDomain:       "qe.devcluster.openshift.com",
		Region:           receiver.region,
		NodePoolReplicas: &nodePoolReplicas,
	}
}

func (receiver *installHelper) newAWSS3Client() {
	accessKeyID, secureKey, err := getAWSKey(receiver.oc)
	o.Expect(err).NotTo(o.HaveOccurred())
	region, err := getClusterRegion(receiver.oc)
	o.Expect(err).NotTo(o.HaveOccurred())
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKeyID, secureKey, "")),
		config.WithRegion(region))
	o.Expect(err).NotTo(o.HaveOccurred())
	content := "[default]\naws_access_key_id=" + accessKeyID + "\naws_secret_access_key=" + secureKey
	filePath := receiver.dir + "/credentials"
	err = ioutil.WriteFile(filePath, []byte(content), 0644)
	o.Expect(err).NotTo(o.HaveOccurred())
	receiver.awsClient = s3.NewFromConfig(cfg)
	receiver.region = region
}

func (receiver *installHelper) createAWSS3Bucket() {
	region, err := getClusterRegion(receiver.oc)
	o.Expect(err).NotTo(o.HaveOccurred())
	exist := false
	buckets, err := receiver.awsClient.ListBuckets(context.TODO(), &s3.ListBucketsInput{})
	o.Expect(err).NotTo(o.HaveOccurred())
	for _, bu := range buckets.Buckets {
		if *bu.Name == receiver.bucketName {
			exist = true
			break
		}
	}
	if exist {
		return
	}
	_, err = receiver.awsClient.CreateBucket(context.TODO(), &s3.CreateBucketInput{Bucket: &receiver.bucketName, CreateBucketConfiguration: &types.CreateBucketConfiguration{LocationConstraint: types.BucketLocationConstraint(region)}})
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (receiver *installHelper) deleteAWSS3Bucket() {
	_, err := receiver.awsClient.DeleteBucket(context.TODO(), &s3.DeleteBucketInput{Bucket: &receiver.bucketName})
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (receiver *installHelper) hyperShiftInstall() {
	region, err := getClusterRegion(receiver.oc)
	o.Expect(err).ShouldNot(o.HaveOccurred())
	var bashClient = NewCmdClient().WithShowInfo(true)
	cmd := fmt.Sprintf("hypershift install --oidc-storage-provider-s3-bucket-name %s --oidc-storage-provider-s3-credentials %s --oidc-storage-provider-s3-region %s", receiver.bucketName, receiver.dir+"/credentials", region)
	_, err = bashClient.Run(cmd).Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
	e2e.Logf("check hyperShift operator install")
	o.Eventually(func() string {
		value, er := receiver.oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", "hypershift", "--ignore-not-found", "-l", "app=operator", `-ojsonpath='{.items[].status.conditions[?(@.type=="Ready")].status}`).Output()
		if er != nil {
			e2e.Logf("error occurred: %v, try next round", er)
			return ""
		}
		return value
	}, DefaultTimeout, DefaultTimeout/10).Should(o.ContainSubstring("True"), "hyperShift operator install error")
}

func (receiver *installHelper) hyperShiftUninstall() {
	var bashClient = NewCmdClient().WithShowInfo(true)
	_, err := bashClient.Run("hypershift install render --format=yaml | oc delete -f -").Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
	e2e.Logf("check hyperShift operator uninstall")
	o.Eventually(func() string {
		value, er := receiver.oc.AsAdmin().WithoutNamespace().Run("get").Args("all", "-n", "hypershift").Output()
		if er != nil {
			e2e.Logf("error occurred: %v, try next round", er)
			return ""
		}
		return value
	}, ShortTimeout, ShortTimeout/10).Should(o.ContainSubstring("No resources found"), "hyperShift operator uninstall error")
}

func (receiver *installHelper) extractPullSecret() {
	err := receiver.oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", "--to="+receiver.dir, "--confirm").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (receiver *installHelper) createAWSHostedClusters(createCluster *createCluster) {
	vars, err := parse(createCluster)
	o.Expect(err).ShouldNot(o.HaveOccurred())
	var bashClient = NewCmdClient().WithShowInfo(true)
	cmd := fmt.Sprintf("hypershift create cluster aws %s", strings.Join(vars, " "))
	_, err = bashClient.Run(cmd).Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
	e2e.Logf("check AWS HostedClusters")
	o.Eventually(func() string {
		value, er := receiver.oc.AsAdmin().WithoutNamespace().Run("get").Args("hostedclusters", "-n", createCluster.Namespace, "--ignore-not-found", createCluster.Name, `-ojsonpath='{.status.conditions[?(@.type=="Available")].status}'`).Output()
		if er != nil {
			e2e.Logf("error occurred: %v, try next round", er)
			return ""
		}
		return value
	}, ClusterInstallTimeout, ClusterInstallTimeout/10).Should(o.ContainSubstring("True"), "AWS HostedClusters install error")
}

func (receiver *installHelper) destroyAWSHostedClusters(createCluster *createCluster) {
	var bashClient = NewCmdClient().WithShowInfo(true)
	cmd := fmt.Sprintf("hypershift destroy cluster aws --aws-creds %s --namespace %s --name %s --region %s", receiver.dir+"/credentials", createCluster.Namespace, createCluster.Name, createCluster.Region)
	_, err := bashClient.Run(cmd).Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
	e2e.Logf("check destroy AWS HostedClusters")
	o.Eventually(func() string {
		value, er := receiver.oc.AsAdmin().WithoutNamespace().Run("get").Args("hostedclusters", "-n", createCluster.Namespace).Output()
		if er != nil {
			e2e.Logf("error occurred: %v, try next round", er)
			return ""
		}
		return value
	}, ShortTimeout, ShortTimeout/10).ShouldNot(o.ContainSubstring(createCluster.Name), "destroy AWS HostedClusters error")
}

func (receiver *installHelper) createHostedClusterKubeconfig(createCluster *createCluster) string {
	var bashClient = NewCmdClient()
	hostedClustersKubeconfigFile := receiver.dir + "/guestcluster-kubeconfig"
	_, err := bashClient.Run(fmt.Sprintf("hypershift create kubeconfig --namespace %s --name %s > %s", createCluster.Namespace, createCluster.Name, hostedClustersKubeconfigFile)).Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
	return hostedClustersKubeconfigFile
}
