package rosacli

type Client struct {
	// Clients
	Runner *runner
	Parser *Parser

	// services
	Cluster       ClusterService
	IDP           IDPService
	OCMResource   OCMResourceService
	User          UserService
	MachinePool   MachinePoolService
	KubeletConfig KubeletConfigService
	// Addon AddonService
	// IDP IDPService
	// Network NetworkService
	// MachinePool MachinepPoolService
	// OCMRole OCMRoleService
	// OCMResource OCMResourceService
}

func NewClient() *Client {
	runner := NewRunner()
	parser := NewParser()

	client := &Client{
		Runner: runner,
		Parser: parser,
	}

	client.Cluster = &clusterService{Client: client}
	client.IDP = &idpService{Client: client}
	client.OCMResource = &ocmResourceService{Client: client}
	client.User = &userService{Client: client}
	client.MachinePool = &machinepoolService{Client: client}
	client.KubeletConfig = &kubeletConfigService{Client: client}

	return client
}

type Service struct {
	Client *Client
}
