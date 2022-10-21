package mco

const (
	// MachineConfigNamespace mco namespace
	MachineConfigNamespace = "openshift-machine-config-operator"
	// MachineConfigDaemon mcd container name
	MachineConfigDaemon = "machine-config-daemon"
	// MachineConfigDaemonEvents cluster role binding
	MachineConfigDaemonEvents = "machine-config-daemon-events"

	// MachineConfigPoolMaster master pool name
	MachineConfigPoolMaster = "master"
	// MachineConfigPoolWorker worker pool name
	MachineConfigPoolWorker = "worker"

	// ControllerDeployment name of the deployment deploying the machine config controller
	ControllerDeployment = "machine-config-controller"
	// ControllerContainer name of the controller container in the controller pod
	ControllerContainer = "machine-config-controller"
	// ControllerLabel label used to identify the controller pod
	ControllerLabel = "k8s-app"
	// ControllerLabelValue value used to identify the controller pod
	ControllerLabelValue = "machine-config-controller"

	// ArchitectureARM64 value used to identify arm64 architecture
	ArchitectureARM64 = "arm64"
	// ArchitectureAMD64 value used to identify amd64 architecture
	ArchitectureAMD64 = "amd64"

	// TmplAddSSHAuthorizedKeyForWorker template file name: change-worker-add-ssh-authorized-key
	TmplAddSSHAuthorizedKeyForWorker = "change-worker-add-ssh-authorized-key"

	// EnvVarLayeringTestImageRepository environment variable to define the image repository used by layering test cases
	EnvVarLayeringTestImageRepository = "LAYERING_TEST_IMAGE_REPOSITORY"

	// DefaultLayeringQuayRepository the quay repository that will be used by default to push auxiliary layering images
	DefaultLayeringQuayRepository = "quay.io/mcoqe/layering"

	// LayeringBaseImageReleaseInfo is the name of the layering base image in release info
	LayeringBaseImageReleaseInfo = "rhel-coreos-8"
	// TmplHypershiftMcConfigMap template file name:hypershift-cluster-mc-configmap.yaml, it's used to create mc for hosted cluster
	TmplHypershiftMcConfigMap = "hypershift-cluster-mc-configmap.yaml"
	// GenericMCTemplate is the name of a MachineConfig template that can be fully configured by parameters
	GenericMCTemplate = "generic-machine-config-template.yml"

	// HypershiftCrNodePool keyword: nodepool
	HypershiftCrNodePool = "nodepool"
	// HypershiftHostedCluster keyword: hostedcluster
	HypershiftHostedCluster = "hostedcluster"
	// HypershiftNsClusters namespace: clusters
	HypershiftNsClusters = "clusters"
	// HypershiftNs operator namespace: hypershift
	HypershiftNs = "hypershift"
	// HypershiftAwsMachine keyword: awsmachine
	HypershiftAwsMachine = "awsmachine"

	// NodeAnnotationCurrentConfig current config
	NodeAnnotationCurrentConfig = "machineconfiguration.openshift.io/currentConfig"
	// NodeAnnotationDesiredConfig desired config
	NodeAnnotationDesiredConfig = "machineconfiguration.openshift.io/desiredConfig"
	// NodeAnnotationDesiredDrain desired drain id
	NodeAnnotationDesiredDrain = "machineconfiguration.openshift.io/desiredDrain"
	// NodeAnnotationLastAppliedDrain last applied drain id
	NodeAnnotationLastAppliedDrain = "machineconfiguration.openshift.io/lastAppliedDrain"
	// NodeAnnotationReason failure reason
	NodeAnnotationReason = "machineconfiguration.openshift.io/reason"
	// NodeAnnotationState state of the mc
	NodeAnnotationState = "machineconfiguration.openshift.io/state"

	// TestCtxKeyBucket hypershift test s3 bucket name
	TestCtxKeyBucket = "bucket"
	// TestCtxKeyNodePool hypershift test node pool name
	TestCtxKeyNodePool = "nodepool"
	// TestCtxKeyCluster hypershift test hosted cluster name
	TestCtxKeyCluster = "cluster"
	// TestCtxKeyConfigMap hypershift test config map name
	TestCtxKeyConfigMap = "configmap"
	// TestCtxKeyKubeConfig hypershift test kubeconfig of hosted cluster
	TestCtxKeyKubeConfig = "kubeconfig"
	// TestCtxKeyFilePath hypershift test filepath in machine config
	TestCtxKeyFilePath = "filepath"
	// TestCtxKeySkipCleanUp indicates whether clean up should be skipped
	TestCtxKeySkipCleanUp = "skipCleanUp"

	// AWSPlatform value used to identify aws infrastructure
	AWSPlatform = "aws"
	// GCPPlatform value used to identify gcp infrastructure
	GCPPlatform = "gcp"
	// AzurePlatform value used to identify azure infrastructure
	AzurePlatform = "azure"
)
