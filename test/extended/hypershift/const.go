package hypershift

import "time"

// OcpClientVerb is the oc verb operation of OCP
type OcpClientVerb = string

/*
oc <OcpClientVerb> resources
*/
const (
	OcpGet      OcpClientVerb = "get"
	OcpPatch    OcpClientVerb = "patch"
	OcpWhoami   OcpClientVerb = "whoami"
	OcpDelete   OcpClientVerb = "delete"
	OcpAnnotate OcpClientVerb = "annotate"
	OcpDebug    OcpClientVerb = "debug"
	OcpExec     OcpClientVerb = "exec"
	OcpScale    OcpClientVerb = "scale"
	OcpAdm      OcpClientVerb = "adm"
	OcpApply    OcpClientVerb = "apply"
	OcpCreate   OcpClientVerb = "create"
	OcpLabel    OcpClientVerb = "label"

	//NodepoolNameSpace is the namespace where the nodepool CR is always created
	NodepoolNameSpace = "clusters"

	ClusterInstallTimeout = 3600 * time.Second
	DoubleLongTimeout     = 1800 * time.Second
	LongTimeout           = 900 * time.Second
	DefaultTimeout        = 300 * time.Second
	ShortTimeout          = 50 * time.Second
)

const (
	HyperShiftResourceTagKeyPrefix = "kubernetes.io/cluster/"
	HyperShiftResourceTagKeyValue  = "owned"
	SupportedPreviousMinorVersions = 2
)

type PlatformType = string

const (
	// AWSPlatform represents Amazon Web Services infrastructure.
	AWSPlatform PlatformType = "AWS"

	// NonePlatform represents user supplied (e.g. bare metal) infrastructure.
	NonePlatform PlatformType = "None"

	// IBMCloudPlatform represents IBM Cloud infrastructure.
	IBMCloudPlatform PlatformType = "IBMCloud"

	// AgentPlatform represents user supplied insfrastructure booted with agents.
	AgentPlatform PlatformType = "Agent"

	// KubevirtPlatform represents Kubevirt infrastructure.
	KubevirtPlatform PlatformType = "KubeVirt"

	// AzurePlatform represents Azure infrastructure.
	AzurePlatform PlatformType = "Azure"

	// PowerVSPlatform represents PowerVS infrastructure.
	PowerVSPlatform PlatformType = "PowerVS"
)

type AvailabilityPolicy = string

const (
	// HighlyAvailable means components should be resilient to problems across
	// fault boundaries as defined by the component to which the policy is
	// attached. This usually means running critical workloads with 3 replicas and
	// with little or no toleration of disruption of the component.
	HighlyAvailable AvailabilityPolicy = "HighlyAvailable"

	// SingleReplica means components are not expected to be resilient to problems
	// across most fault boundaries associated with high availability. This
	// usually means running critical workloads with just 1 replica and with
	// toleration of full disruption of the component.
	SingleReplica AvailabilityPolicy = "SingleReplica"
)

// AWSEndpointAccessType specifies the publishing scope of cluster endpoints.
type AWSEndpointAccessType = string

const (
	// Public endpoint access allows public API server access and public node
	// communication with the control plane.
	Public AWSEndpointAccessType = "Public"

	// PublicAndPrivate endpoint access allows public API server access and
	// private node communication with the control plane.
	PublicAndPrivate AWSEndpointAccessType = "PublicAndPrivate"

	// Private endpoint access allows only private API server access and private
	// node communication with the control plane.
	Private AWSEndpointAccessType = "Private"
)

type IdentityProviderType = string

const (
	// IdentityProviderTypeBasicAuth provides identities for users authenticating with HTTP Basic Auth
	IdentityProviderTypeBasicAuth IdentityProviderType = "BasicAuth"

	// IdentityProviderTypeGitHub provides identities for users authenticating using GitHub credentials
	IdentityProviderTypeGitHub IdentityProviderType = "GitHub"

	// IdentityProviderTypeGitLab provides identities for users authenticating using GitLab credentials
	IdentityProviderTypeGitLab IdentityProviderType = "GitLab"

	// IdentityProviderTypeGoogle provides identities for users authenticating using Google credentials
	IdentityProviderTypeGoogle IdentityProviderType = "Google"

	// IdentityProviderTypeHTPasswd provides identities from an HTPasswd file
	IdentityProviderTypeHTPasswd IdentityProviderType = "HTPasswd"

	// IdentityProviderTypeKeystone provides identitities for users authenticating using keystone password credentials
	IdentityProviderTypeKeystone IdentityProviderType = "Keystone"

	// IdentityProviderTypeLDAP provides identities for users authenticating using LDAP credentials
	IdentityProviderTypeLDAP IdentityProviderType = "LDAP"
)

const (
	//default private secret credential file location, in prow the private credential will be mount to this file
	DefaultAWSHyperShiftPrivateSecretFile = "/etc/hypershift-pool-aws-credentials/awsprivatecred"
	//we can overwrite the default secret credential file by this env
	AWS_HYPERSHIFT_PRIVATE_SECRET_FILE = "AWS_HYPERSHIFT_PRIVATE_SECRET_FILE"
)

// external dns for qe
const (
	HyperShiftExternalDNSBaseDomain = "hypershift-ci.qe.devcluster.openshift.com"
	HyperShiftExternalDNS           = "hypershift-ext.qe.devcluster.openshift.com"
)

const (
	machineAPINamespace      = "openshift-machine-api"
	clusterAPINamespace      = "openshift-cluster-api"
	machineApproverNamespace = "openshift-cluster-machine-approver"
	mapiMachineset           = "machinesets.machine.openshift.io"
	mapiMachine              = "machines.machine.openshift.io"
	mapiMHC                  = "machinehealthchecks.machine.openshift.io"
	capiMachineset           = "machinesets.cluster.x-k8s.io"
	capiMachine              = "machines.cluster.x-k8s.io"
	defaultTimeout           = 300 * time.Second
)

// node isolation
const (
	servingComponentNodesTaintKey = "hypershift.openshift.io/request-serving-component"
	servingComponentNodesLabelKey = "hypershift.openshift.io/request-serving-component"
	nonServingComponentLabelKey   = "hypershift.openshift.io/control-plane"
	nonServingComponentTaintKey   = nonServingComponentLabelKey

	servingComponentNodesTaint = servingComponentNodesTaintKey + "=true:NoSchedule"
	servingComponentNodesLabel = servingComponentNodesLabelKey + "=true"
	nonServingComponentLabel   = nonServingComponentLabelKey + "=true"
	nonServingComponentTaint   = nonServingComponentTaintKey + "=true:NoSchedule"
)

type K8SResource string

const (
	Service K8SResource = "services"
)
