package rosacli

import (
	"bytes"

	"gopkg.in/yaml.v3"
)

type KubeletConfigService interface {
	DescribeKubeletConfig(clusterID string) (bytes.Buffer, error)
	ReflectKubeletConfigDescription(result bytes.Buffer) *KubeletConfigDescription
	EditKubeletConfig(clusterID string, flags ...string) (bytes.Buffer, error)
	DeleteKubeletConfig(clusterID string, flags ...string) (bytes.Buffer, error)
	CreateKubeletConfig(clusterID string, flags ...string) (bytes.Buffer, error)
}

var _ KubeletConfigService = &kubeletConfigService{}

type kubeletConfigService Service

// Struct for the 'rosa describe kubeletconfig' output
type KubeletConfigDescription struct {
	PodPidsLimit int `yaml:"Pod Pids Limit,omitempty"`
}

// Describe Kubeletconfig
func (k *kubeletConfigService) DescribeKubeletConfig(clusterID string) (bytes.Buffer, error) {
	describe := k.Client.Runner.
		Cmd("describe", "kubeletconfig").
		CmdFlags("-c", clusterID).
		OutputFormat()
	// if jsonOutput {
	// 	describe = describe.JsonFormat(jsonOutput)
	// }

	return describe.Run()
}

// Pasrse the result of 'rosa describe kubeletconfig' to the KubeletConfigDescription struct
func (k *kubeletConfigService) ReflectKubeletConfigDescription(result bytes.Buffer) *KubeletConfigDescription {
	res := new(KubeletConfigDescription)
	theMap, _ := k.Client.Parser.TextData.Input(result).Parse().YamlToMap()
	data, _ := yaml.Marshal(&theMap)
	yaml.Unmarshal(data, res)
	return res
}

// Edit the kubeletconfig
func (k *kubeletConfigService) EditKubeletConfig(clusterID string, flags ...string) (bytes.Buffer, error) {
	combflags := append([]string{"-c", clusterID}, flags...)
	editCluster := k.Client.Runner.
		Cmd("edit", "kubeletconfig").
		CmdFlags(combflags...)
	return editCluster.Run()
}

// Delete the kubeletconfig
func (k *kubeletConfigService) DeleteKubeletConfig(clusterID string, flags ...string) (bytes.Buffer, error) {
	combflags := append([]string{"-c", clusterID}, flags...)
	editCluster := k.Client.Runner.
		Cmd("delete", "kubeletconfig").
		CmdFlags(combflags...)
	return editCluster.Run()
}

// Create the kubeletconfig
func (k *kubeletConfigService) CreateKubeletConfig(clusterID string, flags ...string) (bytes.Buffer, error) {
	combflags := append([]string{"-c", clusterID}, flags...)
	editCluster := k.Client.Runner.
		Cmd("create", "kubeletconfig").
		CmdFlags(combflags...)
	return editCluster.Run()
}
