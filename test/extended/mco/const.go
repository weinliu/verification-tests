// Package mco stores all MCO automated test cases
package mco

import (
	"time"
)

const (
	// MachineConfigNamespace mco namespace
	MachineConfigNamespace = "openshift-machine-config-operator"
	// MachineConfigDaemon mcd container name
	MachineConfigDaemon = "machine-config-daemon"
	// MachineConfigOperator mco container name
	MachineConfigOperator = "machine-config-operator"
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

	// TmplAddSSHAuthorizedKeyForWorker template file name: change-worker-add-ssh-authorized-key
	TmplAddSSHAuthorizedKeyForWorker = "change-worker-add-ssh-authorized-key"

	// EnvVarLayeringTestImageRepository environment variable to define the image repository used by layering test cases
	EnvVarLayeringTestImageRepository = "LAYERING_TEST_IMAGE_REPOSITORY"

	// DefaultLayeringQuayRepository the quay repository that will be used by default to push auxiliary layering images
	DefaultLayeringQuayRepository = "quay.io/mcoqe/layering"
	// InternalRegistrySvcURL is the url to reach the internal registry service from inside a cluster
	InternalRegistrySvcURL = "image-registry.openshift-image-registry.svc:5000"

	// LayeringBaseImageReleaseInfo is the name of the layering base image in release info
	LayeringBaseImageReleaseInfo = "rhel-coreos"
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

	// ExpirationDokerfileLabel Expiration label in Dockerfile
	ExpirationDokerfileLabel = `LABEL maintainer="mco-qe-team" quay.expires-after=2h`

	layeringTestsTmpNamespace   = "layering-tests-imagestreams"
	layeringRegistryAdminSAName = "test-registry-sa"

	// DefaultExpectTimeout is the default tiemout for expect commands
	DefaultExpectTimeout = 10 * time.Second

	// DefaultMinutesWaitingPerNode is the  number of minutes per node that the MCPs will wait to become updated
	DefaultMinutesWaitingPerNode = 10

	// RTKernelIncWait exta minutes that MCPs will wait per node if we use a rt-kernel configuration
	RTKernelIncWait = 5

	// ImageRegistryCertificatesDir is the path were the image registry certificates will be stored in a node. Example: /etc/docker/certs.d/mycertname/ca.crt
	ImageRegistryCertificatesDir = "/etc/docker/certs.d"

	// ImageRegistryCertificatesFileName is the name of the image registry certificates. Example: /etc/docker/certs.d/mycertname/ca.crt
	ImageRegistryCertificatesFileName = "ca.crt"

	// OCBMachineConfigPoolLabel the label used to enable and disable the on-cluster build functionality in MCPs
	OCBMachineConfigPoolLabel = "machineconfiguration.openshift.io/layering-enabled"

	// OCBMachineOsBuilderLabel the label to identify the machine-os-builder pod
	OCBMachineOsBuilderLabel = "k8s-app=machine-os-builder"

	// OCBMachineOsBuilderContainer the name of the container running the controller in the machine-os-builder pod
	OCBMachineOsBuilderContainer = "machine-os-builder"

	// OCBConfigmapName is the name of the on-cluster-build-config configmap
	OCBConfigmapName = "on-cluster-build-config"

	// OCBDefaultBaseImagePullSecretName default value for the OCB image pull secret name
	OCBDefaultBaseImagePullSecretName = "mco-global-pull-secret"

	// OCBDefaultFinalImagePushSecretName default value for the OCB image pull secret name
	OCBDefaultFinalImagePushSecretName = "mco-test-push-secret"

	// SecurePort is the tls secured port to serve ignition configs
	IgnitionSecurePort = 22623
	// InsecurePort is the port to serve ignition configs w/o tls
	IgnitionInsecurePort = 22624
)
