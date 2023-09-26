package rosacli

import (
	"bytes"
)

type IDPService interface {
	ReflectIDPList(result bytes.Buffer) (idplist IDPList, err error)
	CreateIDP(clusterID string, flags ...string) (bytes.Buffer, error)
	ListIDP(clusterID string) (bytes.Buffer, error)
	DeleteIDP(clusterID string, idpName string) (bytes.Buffer, error)
}

var _ IDPService = &idpService{}

type idpService Service

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
func (c *idpService) ReflectIDPList(result bytes.Buffer) (idplist IDPList, err error) {
	idplist = IDPList{}
	theMap := c.Client.Parser.TableData.Input(result).Parse().Output()
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

// Create idp
func (c *idpService) CreateIDP(clusterID string, flags ...string) (bytes.Buffer, error) {
	combflags := append([]string{"-c", clusterID}, flags...)
	createIDP := c.Client.Runner.
		Cmd("create", "idp").
		CmdFlags(combflags...)

	return createIDP.Run()
}

// Delete idp
func (c *idpService) DeleteIDP(clusterID string, idpName string) (bytes.Buffer, error) {
	deleteIDP := c.Client.Runner.
		Cmd("delete", "idp").
		CmdFlags("-c", clusterID, idpName, "-y")

	return deleteIDP.Run()
}

// list idp
func (c *idpService) ListIDP(clusterID string) (bytes.Buffer, error) {
	listIDP := c.Client.Runner.
		Cmd("list", "idp").
		CmdFlags("-c", clusterID)

	return listIDP.Run()
}
