package hypershift

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"regexp"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	"github.com/onsi/ginkgo/v2/types"
	o "github.com/onsi/gomega"
	"github.com/tidwall/gjson"

	e2e "k8s.io/kubernetes/test/e2e/framework"
	"k8s.io/utils/ptr"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

type installHelper struct {
	oc           *exutil.CLI
	bucketName   string
	region       string
	artifactDir  string
	dir          string
	s3Client     *exutil.S3Client
	iaasPlatform string
	installType  AWSEndpointAccessType
	externalDNS  bool
}

type createCluster struct {
	PullSecret                     string                `param:"pull-secret"`
	AWSCreds                       string                `param:"aws-creds"`
	AzureCreds                     string                `param:"azure-creds"`
	Name                           string                `param:"name"`
	BaseDomain                     string                `param:"base-domain"`
	Namespace                      string                `param:"namespace"`
	NodePoolReplicas               *int                  `param:"node-pool-replicas"`
	Region                         string                `param:"region"`
	Location                       string                `param:"location"`
	InfraJSON                      string                `param:"infra-json"`
	IamJSON                        string                `param:"iam-json"`
	InfraID                        string                `param:"infra-id"`
	RootDiskSize                   *int                  `param:"root-disk-size"`
	AdditionalTags                 string                `param:"additional-tags"`
	ControlPlaneAvailabilityPolicy string                `param:"control-plane-availability-policy"`
	InfraAvailabilityPolicy        string                `param:"infra-availability-policy"`
	Zones                          string                `param:"zones"`
	SSHKey                         string                `param:"ssh-key"`
	GenerateSSH                    bool                  `param:"generate-ssh"`
	OLMCatalogPlacement            string                `param:"olm-catalog-placement"`
	FIPS                           bool                  `param:"fips"`
	Annotations                    map[string]string     `param:"annotations"`
	EndpointAccess                 AWSEndpointAccessType `param:"endpoint-access"`
	ExternalDnsDomain              string                `param:"external-dns-domain"`
	ReleaseImage                   string                `param:"release-image"`
	ResourceGroupTags              string                `param:"resource-group-tags"`
	EncryptionKeyId                string                `param:"encryption-key-id"`
}

type infra struct {
	AWSCreds   string `param:"aws-creds"`
	AzureCreds string `param:"azure-creds"`
	Name       string `param:"name"`
	BaseDomain string `param:"base-domain"`
	InfraID    string `param:"infra-id"`
	Location   string `param:"location"`
	Region     string `param:"region"`
	RHCOSImage string `param:"rhcos-image"`
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

type bastion struct {
	Region     string `param:"region"`
	InfraID    string `param:"infra-id"`
	SSHKeyFile string `param:"ssh-key-file"`
	AWSCreds   string `param:"aws-creds"`
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

func (c *createCluster) withRootDiskSize(RootDiskSize int) *createCluster {
	c.RootDiskSize = &RootDiskSize
	return c
}

func (c *createCluster) withAdditionalTags(AdditionalTags string) *createCluster {
	c.AdditionalTags = AdditionalTags
	return c
}

func (c *createCluster) withInfraAvailabilityPolicy(InfraAvailabilityPolicy string) *createCluster {
	c.InfraAvailabilityPolicy = InfraAvailabilityPolicy
	return c
}

func (c *createCluster) withControlPlaneAvailabilityPolicy(ControlPlaneAvailabilityPolicy string) *createCluster {
	c.ControlPlaneAvailabilityPolicy = ControlPlaneAvailabilityPolicy
	return c
}

func (c *createCluster) withZones(Zones string) *createCluster {
	c.Zones = Zones
	return c
}

func (c *createCluster) withSSHKey(SSHKey string) *createCluster {
	c.SSHKey = SSHKey
	return c
}

func (c *createCluster) withInfraID(InfraID string) *createCluster {
	c.InfraID = InfraID
	return c
}

func (c *createCluster) withEncryptionKeyId(encryptionKeyId string) *createCluster {
	c.EncryptionKeyId = encryptionKeyId
	return c
}

func (c *createCluster) withReleaseImage(releaseImage string) *createCluster {
	c.ReleaseImage = releaseImage
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

func (i *infra) withRHCOSImage(rhcosImage string) *infra {
	i.RHCOSImage = rhcosImage
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

func (c *createCluster) withEndpointAccess(endpointAccess AWSEndpointAccessType) *createCluster {
	c.EndpointAccess = endpointAccess
	return c
}

func (c *createCluster) withAnnotation(key, value string) *createCluster {
	if c.Annotations == nil {
		c.Annotations = make(map[string]string)
	}
	c.Annotations[key] = value
	return c
}

func (c *createCluster) withAnnotationMap(annotations map[string]string) *createCluster {
	if c.Annotations == nil {
		c.Annotations = make(map[string]string)
	}
	for key, value := range annotations {
		c.Annotations[key] = value
	}
	return c
}

func (c *createCluster) withExternalDnsDomain(externalDnsDomain string) *createCluster {
	c.ExternalDnsDomain = externalDnsDomain
	return c
}

func (c *createCluster) withBaseDomain(baseDomain string) *createCluster {
	c.BaseDomain = baseDomain
	return c
}

func (c *createCluster) withResourceGroupTags(rgTags string) *createCluster {
	c.ResourceGroupTags = rgTags
	return c
}

func (receiver *installHelper) createClusterAWSCommonBuilder() *createCluster {
	nodePoolReplicas := 3
	baseDomain, err := getBaseDomain(receiver.oc)
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("current baseDomain %s", baseDomain)
	e2e.Logf("extract secret/pull-secret")
	receiver.extractPullSecret()
	return &createCluster{
		PullSecret:       receiver.dir + "/.dockerconfigjson",
		AWSCreds:         receiver.dir + "/credentials",
		BaseDomain:       baseDomain,
		Region:           receiver.region,
		Namespace:        receiver.oc.Namespace(),
		NodePoolReplicas: &nodePoolReplicas,
	}
}

func (receiver *installHelper) createClusterAzureCommonBuilder() *createCluster {
	nodePoolReplicas := 3
	baseDomain, err := getBaseDomain(receiver.oc)
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("current baseDomain:%s", baseDomain)
	location, err := getClusterRegion(receiver.oc)
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("current location:%s", location)
	e2e.Logf("extract secret/pull-secret")
	receiver.extractPullSecret()
	return &createCluster{
		PullSecret:       receiver.dir + "/.dockerconfigjson",
		AzureCreds:       receiver.dir + "/credentials",
		BaseDomain:       baseDomain,
		Location:         location,
		Namespace:        receiver.oc.Namespace(),
		NodePoolReplicas: &nodePoolReplicas,
	}
}

func (receiver *installHelper) createClusterAROCommonBuilder() *createCluster {
	location, err := getClusterRegion(receiver.oc)
	o.Expect(err).NotTo(o.HaveOccurred(), "failed to get cluster location")
	return &createCluster{
		Annotations:         map[string]string{podSecurityAdmissionOverrideLabelKey: string(podSecurityBaseline)},
		AzureCreds:          exutil.MustGetAzureCredsLocation(),
		BaseDomain:          hypershiftBaseDomainAzure,
		ExternalDnsDomain:   hypershiftExternalDNSDomainAzure,
		FIPS:                true,
		GenerateSSH:         true,
		Location:            location,
		Namespace:           receiver.oc.Namespace(),
		NodePoolReplicas:    ptr.To(2),
		OLMCatalogPlacement: olmCatalogPlacementGuest,
		PullSecret:          exutil.GetTestEnv().PullSecretLocation,
		ReleaseImage:        exutil.GetLatestReleaseImageFromEnv(),
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

func (receiver *installHelper) createInfraAROCommonBuilder() *infra {
	location, err := getClusterRegion(receiver.oc)
	o.Expect(err).NotTo(o.HaveOccurred(), "failed to get cluster location")
	return &infra{
		AzureCreds: exutil.MustGetAzureCredsLocation(),
		BaseDomain: hypershiftBaseDomainAzure,
		Location:   location,
	}
}

func (receiver *installHelper) createIamCommonBuilder(infraFile string) *iam {
	file, err := os.Open(infraFile)
	o.Expect(err).ShouldNot(o.HaveOccurred())
	defer file.Close()
	con, err := ioutil.ReadAll(file)
	o.Expect(err).NotTo(o.HaveOccurred())

	return &iam{
		AWSCreds:      receiver.dir + "/credentials",
		Region:        receiver.region,
		PublicZoneID:  gjson.Get(string(con), "publicZoneID").Str,
		PrivateZoneID: gjson.Get(string(con), "privateZoneID").Str,
		LocalZoneID:   gjson.Get(string(con), "localZoneID").Str,
	}
}

func (receiver *installHelper) createNodePoolAzureCommonBuilder(clusterName string) *NodePool {
	nodeCount := 1
	return &NodePool{
		Namespace:   receiver.oc.Namespace(),
		ClusterName: clusterName,
		NodeCount:   &nodeCount,
	}
}

func (receiver *installHelper) newAWSS3Client() string {
	accessKeyID, secureKey, err := getAWSKey(receiver.oc)
	o.Expect(err).NotTo(o.HaveOccurred())
	region, err := getClusterRegion(receiver.oc)
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("current region %s", region)
	content := "[default]\naws_access_key_id=" + accessKeyID + "\naws_secret_access_key=" + secureKey
	filePath := receiver.dir + "/credentials"
	err = ioutil.WriteFile(filePath, []byte(content), 0644)
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("extract AWS Credentials")

	receiver.s3Client = exutil.NewS3ClientFromCredFile(filePath, "default", region)
	receiver.region = region
	return filePath
}

func (receiver *installHelper) createAWSS3Bucket() {
	o.Expect(receiver.s3Client.HeadBucket(receiver.bucketName)).Should(o.HaveOccurred())
	o.Expect(receiver.s3Client.CreateBucket(receiver.bucketName)).ShouldNot(o.HaveOccurred())

	bucketPolicyTemplate := `{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": "*",
      "Action": "s3:GetObject",
      "Resource": "arn:aws:s3:::%s/*"
    }
  ]
}`
	policy := fmt.Sprintf(bucketPolicyTemplate, receiver.bucketName)
	o.Expect(receiver.s3Client.PutBucketPolicy(receiver.bucketName, policy)).To(o.Succeed(), "an error happened while adding a policy to the bucket")
}

func (receiver *installHelper) deleteAWSS3Bucket() {
	o.Expect(receiver.s3Client.DeleteBucket(receiver.bucketName)).ShouldNot(o.HaveOccurred())
}

func (receiver *installHelper) extractAzureCredentials() {
	clientID, clientSecret, subscriptionID, tenantID, err := getAzureKey(receiver.oc)
	o.Expect(err).NotTo(o.HaveOccurred())
	content := "subscriptionId: " + subscriptionID + "\ntenantId: " + tenantID + "\nclientId: " + clientID + "\nclientSecret: " + clientSecret
	filePath := receiver.dir + "/credentials"
	err = ioutil.WriteFile(filePath, []byte(content), 0644)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (receiver *installHelper) hyperShiftInstall() {
	// Enable defaulting webhook since 4.14
	// Deploy latest hypershift operator since 4.16
	// Wait until the hypershift operator has been rolled out and its webhook service is available
	cmd := "hypershift install --enable-defaulting-webhook=true --wait-until-available "

	// Build up the platform-related part of the installation command
	switch receiver.iaasPlatform {
	case "aws":
		e2e.Logf("Config AWS Bucket")
		credsPath := receiver.newAWSS3Client()
		receiver.createAWSS3Bucket()

		// OIDC
		cmd += fmt.Sprintf("--oidc-storage-provider-s3-bucket-name %s --oidc-storage-provider-s3-credentials %s --oidc-storage-provider-s3-region %s ", receiver.bucketName, credsPath, receiver.region)

		// Private clusters
		if receiver.installType == PublicAndPrivate || receiver.installType == Private {
			privateCred := getAWSPrivateCredentials(credsPath)
			cmd += fmt.Sprintf(" --private-platform AWS --aws-private-creds %s --aws-private-region=%s ", privateCred, receiver.region)
		}

		if receiver.externalDNS {
			cmd += fmt.Sprintf(" --external-dns-provider=aws --external-dns-credentials=%s --external-dns-domain-filter=%s ", receiver.dir+"/credentials", hypershiftExternalDNSDomainAWS)
		}
	case "azure":
		e2e.Logf("extract Azure Credentials")
		receiver.extractAzureCredentials()
	}

	e2e.Logf("run hypershift install command: %s", cmd)
	_, err := NewCmdClient().WithShowInfo(true).Run(cmd).Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
}

func (receiver *installHelper) hyperShiftUninstall() {
	// hypershift install renders crds before resources by default.
	// Delete resources before crds to avoid unrecognized resource failure.
	e2e.Logf("Uninstalling the Hypershift operator and relevant resources")
	var bashClient = NewCmdClient().WithShowInfo(true)
	_, err := bashClient.Run("hypershift install render --enable-defaulting-webhook=true --format=yaml --outputs resources | oc delete -f -").Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
	_, err = bashClient.Run("hypershift install render --enable-defaulting-webhook=true --format=yaml --outputs crds | oc delete -f -").Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())

	e2e.Logf("Waiting until the Hypershift operator and relevant resources are uninstalled")
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
	cmd := fmt.Sprintf("hypershift create cluster aws %s %s", strings.Join(vars, " "), ` --annotations=hypershift.openshift.io/cleanup-cloud-resources="true"`)
	e2e.Logf("run hypershift create command: %s", cmd)
	_, err = bashClient.Run(cmd).Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
	e2e.Logf("check AWS HostedClusters ready")
	cluster := newHostedCluster(receiver.oc, createCluster.Namespace, createCluster.Name)
	o.Eventually(cluster.pollHostedClustersReady(), ClusterInstallTimeout, ClusterInstallTimeout/20).Should(o.BeTrue(), "AWS HostedClusters install error")
	infraID, err := cluster.getInfraID()
	o.Expect(err).ShouldNot(o.HaveOccurred())
	createCluster.InfraID = infraID
	return cluster
}

func (receiver *installHelper) createAWSHostedClusterWithoutCheck(createCluster *createCluster) *hostedCluster {
	vars, err := parse(createCluster)
	o.Expect(err).ShouldNot(o.HaveOccurred())
	var bashClient = NewCmdClient().WithShowInfo(true)
	cmd := fmt.Sprintf("hypershift create cluster aws %s", strings.Join(vars, " "))
	e2e.Logf("run hypershift create command: %s", cmd)
	_, err = bashClient.Run(cmd).Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
	return newHostedCluster(receiver.oc, createCluster.Namespace, createCluster.Name)
}

func (receiver *installHelper) createAzureHostedClusters(createCluster *createCluster) *hostedCluster {
	cluster := receiver.createAzureHostedClusterWithoutCheck(createCluster)
	o.Eventually(cluster.pollHostedClustersReady(), ClusterInstallTimeoutAzure, ClusterInstallTimeoutAzure/20).Should(o.BeTrue(), "azure HostedClusters install error")
	infraID, err := cluster.getInfraID()
	o.Expect(err).ShouldNot(o.HaveOccurred())
	createCluster.InfraID = infraID
	return cluster
}

func (receiver *installHelper) createAzureHostedClusterWithoutCheck(createCluster *createCluster) *hostedCluster {
	vars, err := parse(createCluster)
	o.Expect(err).ShouldNot(o.HaveOccurred())
	cmd := fmt.Sprintf("hypershift create cluster azure %s", strings.Join(vars, " "))
	_, err = NewCmdClient().WithShowInfo(true).Run(cmd).Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
	return newHostedCluster(receiver.oc, createCluster.Namespace, createCluster.Name)
}

func (receiver *installHelper) createAWSHostedClustersRender(createCluster *createCluster, exec func(filename string) error) *hostedCluster {
	vars, err := parse(createCluster)
	o.Expect(err).ShouldNot(o.HaveOccurred())
	var bashClient = NewCmdClient().WithShowInfo(true)

	yamlFile := fmt.Sprintf("%s/%s.yaml", receiver.dir, createCluster.Name)
	_, err = bashClient.Run(fmt.Sprintf("hypershift create cluster aws %s --render > %s", strings.Join(vars, " "), yamlFile)).Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
	e2e.Logf("exec call-back func")
	err = exec(yamlFile)
	o.Expect(err).ShouldNot(o.HaveOccurred())
	e2e.Logf("apply -f Render...")
	err = receiver.oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", yamlFile).Execute()
	o.Expect(err).ShouldNot(o.HaveOccurred())

	e2e.Logf("check AWS HostedClusters ready")
	cluster := newHostedCluster(receiver.oc, createCluster.Namespace, createCluster.Name)
	o.Eventually(cluster.pollHostedClustersReady(), ClusterInstallTimeout, ClusterInstallTimeout/20).Should(o.BeTrue(), "AWS HostedClusters install error")
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

func (receiver *installHelper) destroyAzureHostedClusters(createCluster *createCluster) {
	e2e.Logf("Destroying Azure HC")
	var bashClient = NewCmdClient().WithShowInfo(true)
	cmd := fmt.Sprintf("hypershift destroy cluster azure --azure-creds %s --namespace %s --name %s --location %s", createCluster.AzureCreds, createCluster.Namespace, createCluster.Name, createCluster.Location)
	out, err := bashClient.Run(cmd).Output()
	o.Expect(err).ShouldNot(o.HaveOccurred(), "error destroying Azure HC")
	e2e.Logf("hypershift destroy output:\n%v", out)

	e2e.Logf("Making sure that the HC is gone")
	o.Expect(getHostedClusters(receiver.oc, receiver.oc.Namespace())).ShouldNot(o.ContainSubstring(createCluster.Name), "HC persists even after deletion")
}

func (receiver *installHelper) dumpHostedCluster(createCluster *createCluster) error {
	// Ensure dump dir exists
	dumpDir := path.Join(receiver.dir, createCluster.Name)
	if err := os.MkdirAll(dumpDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dumpDir, err)
	}

	// Dump HC
	cmd := fmt.Sprintf("hypershift dump cluster --artifact-dir %s --dump-guest-cluster --name %s --namespace %s", dumpDir, createCluster.Name, createCluster.Namespace)
	_ = NewCmdClient().WithShowInfo(true).Run(cmd).Execute()

	// Ensure dump artifact dir exists
	dumpArtifactDir := path.Join(receiver.artifactDir, createCluster.Name)
	if err := os.MkdirAll(dumpArtifactDir, 0755); err != nil {
		return fmt.Errorf("failed to create artifact directory %s: %w", dumpArtifactDir, err)
	}

	// Move dump archive to artifact dir
	exutil.MoveFileToPath(path.Join(dumpDir, dumpArchiveName), path.Join(dumpArtifactDir, dumpArchiveName))
	e2e.Logf("Dump archive saved to %s", dumpArtifactDir)
	return nil
}

func (receiver *installHelper) dumpAROHostedCluster(createCluster *createCluster) error {
	if err := os.Setenv(managedServiceKey, managedServiceAROHCP); err != nil {
		e2e.Logf("Error setting env %s to %s: %v", managedServiceKey, managedServiceAROHCP, err)
	}
	return receiver.dumpHostedCluster(createCluster)
}

func (receiver *installHelper) dumpDestroyAROHostedCluster(createCluster *createCluster) {
	if g.GetFailer().GetState().Is(types.SpecStateFailureStates) {
		if err := receiver.dumpAROHostedCluster(createCluster); err != nil {
			e2e.Logf("Error dumping ARO hosted cluster %s: %v", createCluster.Name, err)
		}
	}
	receiver.destroyAzureHostedClusters(createCluster)
}

func (receiver *installHelper) dumpDeleteAROHostedCluster(createCluster *createCluster) {
	if g.GetFailer().GetState().Is(types.SpecStateFailureStates) {
		if err := receiver.dumpAROHostedCluster(createCluster); err != nil {
			e2e.Logf("Error dumping ARO hosted cluster %s: %v", createCluster.Name, err)
		}
	}
	doOcpReq(receiver.oc, OcpDelete, true, "hc", createCluster.Name, "-n", createCluster.Namespace)
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

func (receiver *installHelper) createAzureInfra(infra *infra) {
	vars, err := parse(infra)
	o.Expect(err).ShouldNot(o.HaveOccurred())
	var bashClient = NewCmdClient().WithShowInfo(true)
	cmd := fmt.Sprintf("hypershift create infra azure %s", strings.Join(vars, " "))
	_, err = bashClient.Run(cmd).Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
}

func (receiver *installHelper) destroyAzureInfra(infra *infra) {
	var bashClient = NewCmdClient().WithShowInfo(true)
	cmd := fmt.Sprintf("hypershift destroy infra azure --infra-id %s --azure-creds %s --location %s --name %s", infra.InfraID, infra.AzureCreds, infra.Location, infra.Name)
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

func (receiver *installHelper) createAzureNodePool(nodePool *NodePool) {
	vars, err := parse(nodePool)
	o.Expect(err).ShouldNot(o.HaveOccurred())
	var bashClient = NewCmdClient().WithShowInfo(true)
	cmd := fmt.Sprintf("hypershift create nodepool azure %s", strings.Join(vars, " "))
	_, err = bashClient.Run(cmd).Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
}

func (receiver *installHelper) createAWSBastion(bastion *bastion) string {
	vars, err := parse(bastion)
	o.Expect(err).ShouldNot(o.HaveOccurred())
	var bashClient = NewCmdClient().WithShowInfo(true)
	cmd := fmt.Sprintf("hypershift create bastion aws %s", strings.Join(vars, " "))
	log, err := bashClient.Run(cmd).Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
	numBlock := "(25[0-5]|2[0-4][0-9]|1[0-9][0-9]|[1-9]?[0-9])"
	regexPattern := numBlock + "\\." + numBlock + "\\." + numBlock + "\\." + numBlock
	regEx := regexp.MustCompile(regexPattern)
	return regEx.FindString(log)
}

func (receiver *installHelper) destroyAWSBastion(bastion *bastion) {
	e2e.Logf("destroy AWS bastion")
	var bashClient = NewCmdClient().WithShowInfo(true)
	cmd := fmt.Sprintf("hypershift destroy bastion aws --infra-id %s --aws-creds %s --region %s", bastion.InfraID, bastion.AWSCreds, bastion.Region)
	_, err := bashClient.Run(cmd).Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
}
