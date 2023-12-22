package rosacli

import (
	"bytes"

	"github.com/openshift/openshift-tests-private/test/extended/util/logext"
)

type IDPService interface {
	ResourcesCleaner

	ReflectIDPList(result bytes.Buffer) (idplist IDPList, err error)
	CreateIDP(clusterID string, idpName string, idflags ...string) (bytes.Buffer, error)
	ListIDP(clusterID string) (IDPList, bytes.Buffer, error)
	DeleteIDP(clusterID string, idpName string) (bytes.Buffer, error)
}

type idpService struct {
	ResourcesService

	idps map[string][]string
}

func NewIDPService(client *Client) IDPService {
	return &idpService{
		ResourcesService: ResourcesService{
			client: client,
		},
		idps: make(map[string][]string),
	}
}

// Struct for the 'rosa list idp' output
type IDP struct {
	Name    string `json:"NAME,omitempty"`
	Type    string `json:"TYPE,omitempty"`
	AuthURL string `json:"AUTH URL,omitempty"`
}
type IDPList struct {
	IDPs []IDP `json:"IDPs,omitempty"`
}

// Pasrse the result of 'rosa list idp' to the IDPList struct
func (is *idpService) ReflectIDPList(result bytes.Buffer) (idplist IDPList, err error) {
	idplist = IDPList{}
	theMap := is.client.Parser.TableData.Input(result).Parse().Output()
	for _, idpItem := range theMap {
		idp := &IDP{}
		err = MapStructure(idpItem, idp)
		if err != nil {
			return
		}
		idplist.IDPs = append(idplist.IDPs, *idp)
	}
	return idplist, err
}

// Check the idp with the name exists in the IDPLIST
func (idps IDPList) IsExist(idpName string) (existed bool) {
	existed = false
	for _, idp := range idps.IDPs {
		if idp.Name == idpName {
			existed = true
			break
		}
	}
	return
}

// Get specified machinepool by IDP NAME
func (idps IDPList) Idp(idpName string) (idp IDP) {
	for _, idp := range idps.IDPs {
		if idp.Name == idpName {
			return idp
		}
	}
	return
}

// Create idp
func (is *idpService) CreateIDP(clusterID string, name string, flags ...string) (output bytes.Buffer, err error) {
	output, err = is.create(clusterID, name, flags...)
	if err == nil {
		is.idps[clusterID] = append(is.idps[clusterID], name)
	}
	return
}

func (is *idpService) create(clusterID string, name string, flags ...string) (bytes.Buffer, error) {
	combflags := append(flags, "-c", clusterID, "--name", name)
	createIDP := is.client.Runner.
		Cmd("create", "idp").
		CmdFlags(combflags...)

	return createIDP.Run()
}

// Delete idp
func (is *idpService) DeleteIDP(clusterID string, idpName string) (output bytes.Buffer, err error) {
	output, err = is.delete(clusterID, idpName)
	if err == nil {
		is.idps[clusterID] = removeFromStringSlice(is.idps[clusterID], idpName)
	}
	return
}

func (is *idpService) delete(clusterID string, name string) (bytes.Buffer, error) {
	deleteIDP := is.client.Runner.
		Cmd("delete", "idp", name).
		CmdFlags("-c", clusterID, "-y")

	return deleteIDP.Run()
}

// list idp
func (is *idpService) ListIDP(clusterID string) (IDPList, bytes.Buffer, error) {
	listIDP := is.client.Runner.
		Cmd("list", "idp").
		CmdFlags("-c", clusterID)

	output, err := listIDP.Run()
	if err != nil {
		return IDPList{}, output, err
	}
	idpList, err := is.ReflectIDPList(output)
	return idpList, output, err
}

func (is *idpService) CleanResources(clusterID string) (errors []error) {
	for _, idpName := range is.idps[clusterID] {
		logext.Infof("Remove remaining idp '%s'", idpName)
		_, err := is.delete(clusterID, idpName)
		if err != nil {
			errors = append(errors, err)
		}
	}

	return
}
