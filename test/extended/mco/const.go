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

	// TmplAddSSHAuthorizedKeyForWorker template file name: change-worker-add-ssh-authorized-key
	TmplAddSSHAuthorizedKeyForWorker = "change-worker-add-ssh-authorized-key"
)
