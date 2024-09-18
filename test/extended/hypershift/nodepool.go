package hypershift

import (
	"fmt"
	"strings"

	o "github.com/onsi/gomega"
)

type NodePool struct {
	ClusterName     string `param:"cluster-name"`
	Name            string `param:"name"`
	Namespace       string `param:"namespace"`
	NodeCount       *int   `param:"node-count"`
	NodeUpgradeType string `param:"node-upgrade-type"`
	ReleaseImage    string `param:"release-image"`
}

func NewNodePool(clusterName, name, namespace string) *NodePool {
	return &NodePool{
		ClusterName: clusterName,
		Name:        name,
		Namespace:   namespace,
	}
}

func (np *NodePool) WithName(name string) *NodePool {
	np.Name = name
	return np
}

func (np *NodePool) WithNodeCount(nodeCount *int) *NodePool {
	np.NodeCount = nodeCount
	return np
}

func (np *NodePool) WithNodeUpgradeType(nodeUpgradeType string) *NodePool {
	np.NodeUpgradeType = nodeUpgradeType
	return np
}

func (np *NodePool) WithReleaseImage(releaseImage string) *NodePool {
	np.ReleaseImage = releaseImage
	return np
}

type AWSNodePool struct {
	NodePool
	InstanceProfile string `param:"instance-profile"`
	InstanceType    string `param:"instance-type"`
	RootVolumeIOPS  *int64 `param:"root-volume-iops"`
	RootVolumeSize  *int64 `param:"root-volume-size"`
	RootVolumeType  string `param:"root-volume-type"`
	SecurityGroupID string `param:"securitygroup-id"`
	SubnetID        string `param:"subnet-id"`
}

func NewAWSNodePool(name, clusterName, namespace string) *AWSNodePool {
	return &AWSNodePool{
		NodePool: NodePool{
			Name:        name,
			Namespace:   namespace,
			ClusterName: clusterName,
		},
	}
}

func (a *AWSNodePool) WithInstanceProfile(profile string) *AWSNodePool {
	a.InstanceProfile = profile
	return a
}

func (a *AWSNodePool) WithInstanceType(instanceType string) *AWSNodePool {
	a.InstanceType = instanceType
	return a
}

func (a *AWSNodePool) WithNodeCount(nodeCount *int) *AWSNodePool {
	a.NodeCount = nodeCount
	return a
}

func (a *AWSNodePool) WithNodeUpgradeType(nodeUpgradeType string) *AWSNodePool {
	a.NodeUpgradeType = nodeUpgradeType
	return a
}

func (a *AWSNodePool) WithReleaseImage(releaseImage string) *AWSNodePool {
	a.ReleaseImage = releaseImage
	return a
}

func (a *AWSNodePool) WithRootVolumeIOPS(rootVolumeIOPS *int64) *AWSNodePool {
	a.RootVolumeIOPS = rootVolumeIOPS
	return a
}

func (a *AWSNodePool) WithRootVolumeSize(rootVolumeSize *int64) *AWSNodePool {
	a.RootVolumeSize = rootVolumeSize
	return a
}
func (a *AWSNodePool) WithRootVolumeType(rootVolumeType string) *AWSNodePool {
	a.RootVolumeType = rootVolumeType
	return a
}

func (a *AWSNodePool) WithSecurityGroupID(securityGroupID string) *AWSNodePool {
	a.SecurityGroupID = securityGroupID
	return a
}

func (a *AWSNodePool) WithSubnetID(subnetID string) *AWSNodePool {
	a.SubnetID = subnetID
	return a
}

func (a *AWSNodePool) CreateAWSNodePool() {
	gCreateNodePool[*AWSNodePool]("aws", a)
}

type AzureNodePool struct {
	NodePool
	MarketplaceImage *azureMarketplaceImage
	ImageId          string `param:"image-id"`
	RootDiskSize     *int   `param:"root-disk-size"`
	SubnetId         string `param:"nodepool-subnet-id"`
}

func NewAzureNodePool(name, clusterName, namespace string) *AzureNodePool {
	return &AzureNodePool{
		NodePool: NodePool{
			Name:        name,
			Namespace:   namespace,
			ClusterName: clusterName,
		},
	}
}

func (a *AzureNodePool) WithImageId(imageId string) *AzureNodePool {
	a.ImageId = imageId
	return a
}

func (a *AzureNodePool) WithSubnetId(subnetId string) *AzureNodePool {
	a.SubnetId = subnetId
	return a
}

func (a *AzureNodePool) WithRootDiskSize(rootDiskSize *int) *AzureNodePool {
	a.RootDiskSize = rootDiskSize
	return a
}

func (a *AzureNodePool) WithNodeCount(nodeCount *int) *AzureNodePool {
	a.NodeCount = nodeCount
	return a
}

func (a *AzureNodePool) WithMarketplaceImage(marketplaceImage *azureMarketplaceImage) *AzureNodePool {
	a.MarketplaceImage = marketplaceImage
	return a
}

func (a *AzureNodePool) CreateAzureNodePool() {
	gCreateNodePool[*AzureNodePool]("azure", a)
}

// gCreateNodePool creates a nodepool for different platforms,
// nodepool C should be one kind of nodepools. e.g. *AWSNodePool *AzureNodePool
func gCreateNodePool[C any](platform string, nodepool C) {
	vars, err := parse(nodepool)
	o.Expect(err).ShouldNot(o.HaveOccurred())
	o.Expect(vars).ShouldNot(o.BeEmpty())

	var bashClient = NewCmdClient().WithShowInfo(true)
	cmd := fmt.Sprintf("hypershift create nodepool %s %s", platform, strings.Join(vars, " "))
	_, err = bashClient.Run(cmd).Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
}
