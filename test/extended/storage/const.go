package storage

import "time"

// Define the global cloudProvider
var cloudProvider, provisioner string

// Define test waiting time const
const (
	defaultMaxWaitingTime    = 300 * time.Second
	defaultIterationTimes    = 20
	longerMaxWaitingTime     = 15 * time.Minute
	moreLongerMaxWaitingTime = 30 * time.Minute
	longestMaxWaitingTime    = 1 * time.Hour
	defaultIntervalTime      = 5 * time.Second
)

const (
	// RunningStatus stands for workloads in Running status
	RunningStatus = "Running"
	// BoundStatus stands for pvc and pv in Bound status
	BoundStatus = "Bound"
	// CSINamespace is the default csi driver deployed namespace
	CSINamespace = "openshift-cluster-csi-drivers"

	// FsTypeXFS is xfs type filesystem
	FsTypeXFS = "xfs"
	// FsTypeEXT2 is xfs type filesystem
	FsTypeEXT2 = "ext2"
	// FsTypeEXT3 is xfs type filesystem
	FsTypeEXT3 = "ext3"
	// FsTypeEXT4 is xfs type filesystem
	FsTypeEXT4 = "ext4"

	// IBMPowerVST1 is ibm-powervs tier1 csi storageclass name
	IBMPowerVST1 = "ibm-powervs-tier1"
	// IBMPowerVST3 is ibm-powervs tier3 csi storageclass name
	IBMPowerVST3 = "ibm-powervs-tier3"
)

// Define CSI Driver Provisioners const
const (
	// AWS
	ebsCsiDriverProvisioner string = "ebs.csi.aws.com"
	efsCsiDriverProvisioner string = "efs.csi.aws.com"

	// Azure
	azureDiskCsiDriverProvisioner string = "disk.csi.azure.com"
	azureFileCsiDriverProvisioner string = "file.csi.azure.com"

	// GCP
	gcpPdCsiDriverProvisioner        string = "pd.csi.storage.gke.io"
	gcpFilestoreCsiDriverProvisioner string = "filestore.csi.storage.gke.io"

	// Vmware
	vmwareCsiDriverProvisioner string = "csi.vsphere.vmware.com"

	// IBM
	ibmVpcBlockCsiDriverProvisioner string = "vpc.block.csi.ibm.io"
	ibmPowervsCsiDriverProvisioner  string = "powervs.csi.ibm.com"

	// AlibabaCloud
	aliDiskpluginCsiDriverProvisioner string = "diskplugin.csi.alibabacloud.com"

	// LVM
	topolvmProvisioner string = "topolvm.io"
)

// Define the standardTopologyLabel const
const standardTopologyLabel string = "topology.kubernetes.io/zone"

// Define bytes per GiB const
const bytesPerGiB uint64 = 1 << 30 // 1 GiB = 2^30 bytes

// Define catalogsources const
const (
	qeCatalogSource          string = "qe-app-registry"
	autoReleaseCatalogSource string = "auto-release-app-registry"
	redhatCatalogSource      string = "redhat-operators"
	sourceNameSpace          string = "openshift-marketplace"
)

// Define prometheus const
const (
	prometheusQueryURL  string = "https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query?query="
	prometheusNamespace string = "openshift-monitoring"
	prometheusK8s       string = "prometheus-k8s"
)
