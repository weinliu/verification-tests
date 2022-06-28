package storage

// GenericEphemeralVolume struct deination
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
	VolumeDefination interface{}
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
