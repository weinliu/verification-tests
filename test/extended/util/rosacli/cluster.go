package rosacli

import (
	"bytes"

	"gopkg.in/yaml.v3"
)

type ClusterService interface {
	DescribeCluster(clusterID string) (bytes.Buffer, error)
	ReflectClusterDescription(result bytes.Buffer) *ClusterDescription
	List() (bytes.Buffer, error)
	CreateDryRun(clusterID string, flags ...string) (bytes.Buffer, error)
}

var _ ClusterService = &clusterService{}

type clusterService Service

// Struct for the 'rosa describe cluster' output
type ClusterDescription struct {
	Name             string              `yaml:"Name,omitempty"`
	ID               string              `yaml:"ID,omitempty"`
	ExternalID       string              `yaml:"External ID,omitempty"`
	OpenshiftVersion string              `yaml:"OpenShift Version,omitempty"`
	ChannelGroup     string              `yaml:"Channel Group,omitempty"`
	DNS              string              `yaml:"DNS,omitempty"`
	AWSAccount       string              `yaml:"AWS Account,omitempty"`
	APIURL           string              `yaml:"API URL,omitempty"`
	ConsoleURL       string              `yaml:"Console URL,omitempty"`
	Region           string              `yaml:"Region,omitempty"`
	MultiAZ          string              `yaml:"Multi-AZ,omitempty"`
	State            string              `yaml:"State,omitempty"`
	Private          string              `yaml:"Private,omitempty"`
	Created          string              `yaml:"Created,omitempty"`
	DetailsPage      string              `yaml:"Details Page,omitempty"`
	ControlPlane     string              `yaml:"Control Plane,omitempty"`
	ScheduledUpgrade string              `yaml:"Scheduled Upgrade,omitempty"`
	InfraID          string              `yaml:"Infra ID,omitempty"`
	Availability     []map[string]string `yaml:"Availability,omitempty"`
	Nodes            []map[string]string `yaml:"Nodes,omitempty"`
	Network          []map[string]string `yaml:"Network,omitempty"`
}

func (c *clusterService) DescribeCluster(clusterID string) (bytes.Buffer, error) {
	describe := c.Client.Runner.
		Cmd("describe", "cluster").
		CmdFlags("-c", clusterID).
		OutputFormat()
	// if jsonOutput {
	// 	describe = describe.JsonFormat(jsonOutput)
	// }

	return describe.Run()
}

// Pasrse the result of 'rosa describe cluster' to the RosaClusterDescription struct
func (c *clusterService) ReflectClusterDescription(result bytes.Buffer) *ClusterDescription {
	res := new(ClusterDescription)
	theMap, _ := c.Client.Parser.TextData.Input(result).Parse().YamlToMap()
	data, _ := yaml.Marshal(&theMap)
	yaml.Unmarshal(data, res)
	return res
}

func (c *clusterService) List() (bytes.Buffer, error) {
	list := c.Client.Runner.Cmd("list", "cluster")
	return list.Run()
}

func (c *clusterService) CreateDryRun(clusterID string, flags ...string) (bytes.Buffer, error) {
	combflags := append([]string{"-c", clusterID, "--dry-run"}, flags...)
	createDryRun := c.Client.Runner.
		Cmd("create", "cluster").
		CmdFlags(combflags...)
	return createDryRun.Run()
}
