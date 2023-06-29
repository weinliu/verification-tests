package rosacli

type Client struct {
	// Clients
	Runner *runner
	Parser *parser

	// services
	Cluster ClusterService
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

	return client
}

type service struct {
	client *Client
}
