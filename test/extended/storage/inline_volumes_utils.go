package storage

// GenericEphemeralVolume struct definition
type GenericEphemeralVolume struct {
	VolumeClaimTemplate struct {
		Metadata struct {
			Labels struct {
				WorkloadName string `json:"workloadName"`
			} `json:"labels"`
		} `json:"metadata"`
		Spec struct {
			AccessModes      []string `json:"accessModes"`
			StorageClassName string   `json:"storageClassName"`
			Resources        struct {
				Requests struct {
					Storage string `json:"storage"`
				} `json:"requests"`
			} `json:"resources"`
		} `json:"spec"`
	} `json:"volumeClaimTemplate"`
}

// InlineVolume Object defination
type InlineVolume struct {
	Kind             string
	VolumeDefinition interface{}
	StorageClass     string
}

// GenericEphemeralVolumeOption function mode options
type GenericEphemeralVolumeOption func(*GenericEphemeralVolume)

// Replace the default value of GenericEphemeralVolume workload labelValue parameter
func setGenericEphemeralVolumeWorkloadLabel(labelValue string) GenericEphemeralVolumeOption {
	return func(this *GenericEphemeralVolume) {
		this.VolumeClaimTemplate.Metadata.Labels.WorkloadName = labelValue
	}
}

// Replace the default value of GenericEphemeralVolume accessModes parameter
func setGenericEphemeralVolumeAccessModes(accessModes []string) GenericEphemeralVolumeOption {
	return func(this *GenericEphemeralVolume) {
		this.VolumeClaimTemplate.Spec.AccessModes = accessModes
	}
}

// Replace the default value of GenericEphemeralVolume storageClass parameter
func setGenericEphemeralVolumeStorageClassName(storageClass string) GenericEphemeralVolumeOption {
	return func(this *GenericEphemeralVolume) {
		this.VolumeClaimTemplate.Spec.StorageClassName = storageClass
	}
}

// Replace the default value of GenericEphemeralVolume size parameter
func setGenericEphemeralVolume(size string) GenericEphemeralVolumeOption {
	return func(this *GenericEphemeralVolume) {
		this.VolumeClaimTemplate.Spec.Resources.Requests.Storage = size
	}
}

func newGenericEphemeralVolume(opts ...GenericEphemeralVolumeOption) GenericEphemeralVolume {
	var defaultGenericEphemeralVolume GenericEphemeralVolume
	defaultGenericEphemeralVolume.VolumeClaimTemplate.Spec.AccessModes = []string{"ReadWriteOnce"}
	defaultGenericEphemeralVolume.VolumeClaimTemplate.Spec.StorageClassName = getClusterDefaultStorageclassByPlatform(cloudProvider)
	defaultGenericEphemeralVolume.VolumeClaimTemplate.Spec.Resources.Requests.Storage = getValidVolumeSize()
	for _, o := range opts {
		o(&defaultGenericEphemeralVolume)
	}
	return defaultGenericEphemeralVolume
}

// CsiSharedresourceInlineVolume struct definiti
type CsiSharedresourceInlineVolume struct {
	ReadOnly         bool   `json:"readOnly"`
	Driver           string `json:"driver"`
	VolumeAttributes struct {
		SharedConfigMap string `json:"sharedConfigMap,omitempty"`
		SharedSecret    string `json:"sharedSecret,omitempty"`
	} `json:"volumeAttributes"`
}

// CsiSharedresourceInlineVolumeOption function mode options
type CsiSharedresourceInlineVolumeOption func(*CsiSharedresourceInlineVolume)

// Replace the default value of CsiSharedresourceInlineVolume shared configMap
func setCsiSharedresourceInlineVolumeSharedCM(cmName string) CsiSharedresourceInlineVolumeOption {
	return func(this *CsiSharedresourceInlineVolume) {
		this.VolumeAttributes.SharedConfigMap = cmName
	}
}

// Create a new csi shared resource inline volume
func newCsiSharedresourceInlineVolume(opts ...CsiSharedresourceInlineVolumeOption) CsiSharedresourceInlineVolume {
	var defaultCsiSharedresourceInlineVolume CsiSharedresourceInlineVolume
	defaultCsiSharedresourceInlineVolume.Driver = "csi.sharedresource.openshift.io"
	defaultCsiSharedresourceInlineVolume.ReadOnly = true
	for _, o := range opts {
		o(&defaultCsiSharedresourceInlineVolume)
	}
	return defaultCsiSharedresourceInlineVolume
}
