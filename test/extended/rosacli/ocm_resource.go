package rosacli

import (
	"bytes"
)

type OCMResourceService interface {
	listRegion(flags ...string) (bytes.Buffer, error)
	reflectRegionList(result bytes.Buffer) (regions []*CloudRegion, err error)
}

var _ OCMResourceService = &ocmResourceService{}

type ocmResourceService service

// Struct for the 'rosa list region' output
type CloudRegion struct {
	ID                  string `json:"ID,omitempty"`
	Name                string `json:"NAME,omitempty"`
	MultiAZSupported    string `json:"MULTI-AZ SUPPORT,omitempty"`
	HypershiftSupported string `json:"HOSTED-CP SUPPORT,omitempty"`
}

// List region
func (c *ocmResourceService) listRegion(flags ...string) (bytes.Buffer, error) {
	listRegion := c.client.Runner
	listRegion = listRegion.Cmd("list", "regions").CmdFlags(flags...)
	return listRegion.Run()
}

// Pasrse the result of 'rosa regions' to the RegionInfo struct
func (c *ocmResourceService) reflectRegionList(result bytes.Buffer) (regions []*CloudRegion, err error) {
	theMap := c.client.Parser.tableData.Input(result).Parse().output
	for _, regionItem := range theMap {
		region := &CloudRegion{}
		err = mapStructure(regionItem, region)
		if err != nil {
			return
		}
		regions = append(regions, region)
	}
	return
}
