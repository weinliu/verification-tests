package hypershift

import (
	"fmt"
	"strings"

	o "github.com/onsi/gomega"
)

type NodePool struct {
	Name        string `param:"name"`
	ClusterName string `param:"cluster-name"`
	Namespace   string `param:"namespace"`
	//root-disk-size is for azure only, will op it later and move it to azureNodePool
	RootDiskSize *int   `param:"root-disk-size"`
	NodeCount    *int   `param:"node-count"`
	ReleaseImage string `param:"release-image"`
}

func (c *NodePool) WithName(name string) *NodePool {
	c.Name = name
	return c
}

func (c *NodePool) WithRootDiskSize(RootDiskSize int) *NodePool {
	c.RootDiskSize = &RootDiskSize
	return c
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

func (a *AWSNodePool) WithNodeCount(nodeCount *int) *AWSNodePool {
	a.NodeCount = nodeCount
	return a
}

func (a *AWSNodePool) WithReleaseImage(releaseImage string) *AWSNodePool {
	a.ReleaseImage = releaseImage
	return a
}

func (a *AWSNodePool) CreateAWSNodePool() {
	gCreateNodePool[*AWSNodePool]("aws", a)
}

// gCreateNodePool creates a nodepool for different platforms,
// nodepool C should be one kind of nodepools. e.g. *AWSNodePool *azureNodePool
func gCreateNodePool[C any](platform string, nodepool C) {
	vars, err := parse(nodepool)
	o.Expect(err).ShouldNot(o.HaveOccurred())
	o.Expect(vars).ShouldNot(o.BeEmpty())

	var bashClient = NewCmdClient().WithShowInfo(true)
	cmd := fmt.Sprintf("hypershift create nodepool %s %s", platform, strings.Join(vars, " "))
	_, err = bashClient.Run(cmd).Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
}
