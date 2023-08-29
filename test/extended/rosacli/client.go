package rosacli

type Client struct {
	// Clients
	Runner *runner
	Parser *parser

	// services
	Cluster     ClusterService
	IDP         IDPService
	OCMResource OCMResourceService
	User        UserService
	MachinePool MachinePoolService
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

	client.Cluster = &clusterService{client: client}
	client.IDP = &idpService{client: client}
	client.OCMResource = &ocmResourceService{client: client}
	client.User = &userService{client: client}
	client.MachinePool = &machinepoolService{client: client}

	return client
}

type service struct {
	client *Client
}
