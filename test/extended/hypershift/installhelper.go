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
	"github.com/tidwall/gjson"
	"io/ioutil"
	e2e "k8s.io/kubernetes/test/e2e/framework"
	"os"
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
	InfraJSON        string `param:"infra-json"`
	IamJSON          string `param:"iam-json"`
	InfraID          string `param:"infra-id"`
}

type infra struct {
	AWSCreds   string `param:"aws-creds"`
	Name       string `param:"name"`
	BaseDomain string `param:"base-domain"`
	InfraID    string `param:"infra-id"`
	Region     string `param:"region"`
	Zones      string `param:"zones"`
	OutputFile string `param:"output-file"`
}

type iam struct {
	AWSCreds      string `param:"aws-creds"`
	InfraID       string `param:"infra-id"`
	LocalZoneID   string `param:"local-zone-id"`
	PrivateZoneID string `param:"private-zone-id"`
	PublicZoneID  string `param:"public-zone-id"`
	Region        string `param:"region"`
	OutputFile    string `param:"output-file"`
}

func (c *createCluster) withName(name string) *createCluster {
	c.Name = name
	return c
}

func (c *createCluster) withNodePoolReplicas(NodePoolReplicas int) *createCluster {
	c.NodePoolReplicas = &NodePoolReplicas
	return c
}

func (c *createCluster) withInfraJSON(InfraJSON string) *createCluster {
	c.InfraJSON = InfraJSON
	return c
}

func (c *createCluster) withIamJSON(IamJSON string) *createCluster {
	c.IamJSON = IamJSON
	return c
}

func (i *infra) withInfraID(InfraID string) *infra {
	i.InfraID = InfraID
	return i
}

func (i *infra) withOutputFile(OutputFile string) *infra {
	i.OutputFile = OutputFile
	return i
}

func (i *infra) withName(Name string) *infra {
	i.Name = Name
	return i
}

func (i *iam) withInfraID(InfraID string) *iam {
	i.InfraID = InfraID
	return i
}

func (i *iam) withOutputFile(OutputFile string) *iam {
	i.OutputFile = OutputFile
	return i
}

func (receiver *installHelper) createClusterAWSCommonBuilder() *createCluster {
	nodePoolReplicas := 3
	baseDomain, err := getBaseDomain(receiver.oc)
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("current baseDomain %s", baseDomain)
	return &createCluster{
		PullSecret:       receiver.dir + "/.dockerconfigjson",
		AWSCreds:         receiver.dir + "/credentials",
		BaseDomain:       baseDomain,
		Region:           receiver.region,
		Namespace:        receiver.oc.Namespace(),
		NodePoolReplicas: &nodePoolReplicas,
	}
}

func (receiver *installHelper) createInfraCommonBuilder() *infra {
	baseDomain, err := getBaseDomain(receiver.oc)
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("current baseDomain %s", baseDomain)
	return &infra{
		AWSCreds:   receiver.dir + "/credentials",
		BaseDomain: baseDomain,
		Region:     receiver.region,
	}
}

func (receiver *installHelper) createIamCommonBuilder(infraFile string) *iam {
	file, err := os.Open(infraFile)
	o.Expect(err).ShouldNot(o.HaveOccurred())
	defer file.Close()
	con, err := ioutil.ReadAll(file)

	return &iam{
		AWSCreds:      receiver.dir + "/credentials",
		Region:        receiver.region,
		PublicZoneID:  gjson.Get(string(con), "publicZoneID").Str,
		PrivateZoneID: gjson.Get(string(con), "privateZoneID").Str,
		LocalZoneID:   gjson.Get(string(con), "localZoneID").Str,
	}
}

func (receiver *installHelper) newAWSS3Client() {
	accessKeyID, secureKey, err := getAWSKey(receiver.oc)
	o.Expect(err).NotTo(o.HaveOccurred())
	region, err := getClusterRegion(receiver.oc)
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("current region %s", region)
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
	if strings.Compare("us-east-1", receiver.region) == 0 {
		_, err = receiver.awsClient.CreateBucket(context.TODO(), &s3.CreateBucketInput{Bucket: &receiver.bucketName})
	} else {
		_, err = receiver.awsClient.CreateBucket(context.TODO(), &s3.CreateBucketInput{Bucket: &receiver.bucketName, CreateBucketConfiguration: &types.CreateBucketConfiguration{LocationConstraint: types.BucketLocationConstraint(receiver.region)}})
	}
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (receiver *installHelper) deleteAWSS3Bucket() {
	_, err := receiver.awsClient.DeleteBucket(context.TODO(), &s3.DeleteBucketInput{Bucket: &receiver.bucketName})
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (receiver *installHelper) hyperShiftInstall() {
	var bashClient = NewCmdClient().WithShowInfo(true)
	cmd := fmt.Sprintf("hypershift install --oidc-storage-provider-s3-bucket-name %s --oidc-storage-provider-s3-credentials %s --oidc-storage-provider-s3-region %s", receiver.bucketName, receiver.dir+"/credentials", receiver.region)
	_, err := bashClient.Run(cmd).Output()
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

func (receiver *installHelper) createAWSHostedClusters(createCluster *createCluster) *hostedCluster {
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
	os.Remove(createCluster.InfraJSON)
	os.Remove(createCluster.IamJSON)
	cluster := newHostedCluster(receiver.oc, createCluster.Namespace, createCluster.Name)
	infraID, err := cluster.getInfraID()
	o.Expect(err).ShouldNot(o.HaveOccurred())
	createCluster.InfraID = infraID
	return cluster
}

func (receiver *installHelper) destroyAWSHostedClusters(createCluster *createCluster) {
	var bashClient = NewCmdClient().WithShowInfo(true)
	cmd := fmt.Sprintf("hypershift destroy cluster aws --aws-creds %s --namespace %s --name %s --region %s", createCluster.AWSCreds, createCluster.Namespace, createCluster.Name, createCluster.Region)
	_, err := bashClient.Run(cmd).Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
	e2e.Logf("check destroy AWS HostedClusters")
	o.Eventually(pollGetHostedClusters(receiver.oc, receiver.oc.Namespace()), ShortTimeout, ShortTimeout/10).ShouldNot(o.ContainSubstring(createCluster.Name), "destroy AWS HostedClusters error")
}

func (receiver *installHelper) deleteHostedClustersManual(createCluster *createCluster) {
	hostedClustersNames, err := getHostedClusters(receiver.oc, receiver.oc.Namespace())
	o.Expect(err).ShouldNot(o.HaveOccurred())
	if strings.Contains(hostedClustersNames, createCluster.Name) {
		err = receiver.oc.AsAdmin().WithoutNamespace().Run("delete").Args("hostedcluster", "-n", receiver.oc.Namespace(), createCluster.Name).Execute()
		o.Expect(err).ShouldNot(o.HaveOccurred())
	}
	receiver.destroyAWSIam(&iam{AWSCreds: createCluster.AWSCreds, Region: createCluster.Region, InfraID: createCluster.InfraID})
	receiver.destroyAWSInfra(&infra{AWSCreds: createCluster.AWSCreds, Region: createCluster.Region, InfraID: createCluster.InfraID, BaseDomain: createCluster.BaseDomain})
}

func (receiver *installHelper) createHostedClusterKubeconfig(createCluster *createCluster, cluster *hostedCluster) {
	var bashClient = NewCmdClient().WithShowInfo(true)
	hostedClustersKubeconfigFile := receiver.dir + "/guestcluster-kubeconfig-" + createCluster.Name
	_, err := bashClient.Run(fmt.Sprintf("hypershift create kubeconfig --namespace %s --name %s > %s", createCluster.Namespace, createCluster.Name, hostedClustersKubeconfigFile)).Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
	cluster.hostedClustersKubeconfigFile = hostedClustersKubeconfigFile
}

func (receiver *installHelper) createAWSInfra(infra *infra) {
	vars, err := parse(infra)
	o.Expect(err).ShouldNot(o.HaveOccurred())
	var bashClient = NewCmdClient().WithShowInfo(true)
	cmd := fmt.Sprintf("hypershift create infra aws %s", strings.Join(vars, " "))
	_, err = bashClient.Run(cmd).Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
}

func (receiver *installHelper) destroyAWSInfra(infra *infra) {
	e2e.Logf("destroy AWS infrastructure")
	var bashClient = NewCmdClient().WithShowInfo(true)
	cmd := fmt.Sprintf("hypershift destroy infra aws --infra-id %s --aws-creds %s --base-domain %s --region %s", infra.InfraID, infra.AWSCreds, infra.BaseDomain, infra.Region)
	_, err := bashClient.Run(cmd).Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
}

func (receiver *installHelper) createAWSIam(iam *iam) {
	vars, err := parse(iam)
	o.Expect(err).ShouldNot(o.HaveOccurred())
	var bashClient = NewCmdClient().WithShowInfo(true)
	cmd := fmt.Sprintf("hypershift create iam aws %s", strings.Join(vars, " "))
	_, err = bashClient.Run(cmd).Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
}

func (receiver *installHelper) destroyAWSIam(iam *iam) {
	e2e.Logf("destroy AWS iam")
	var bashClient = NewCmdClient().WithShowInfo(true)
	cmd := fmt.Sprintf("hypershift destroy iam aws --infra-id %s --aws-creds %s --region %s", iam.InfraID, iam.AWSCreds, iam.Region)
	_, err := bashClient.Run(cmd).Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
}

func (receiver *installHelper) deleteHostedClustersCRAllBackground() {
	_, _, _, err := receiver.oc.AsAdmin().WithoutNamespace().Run("delete").Args("hostedcluster", "--all", "-n", receiver.oc.Namespace()).Background()
	o.Expect(err).NotTo(o.HaveOccurred())
}
