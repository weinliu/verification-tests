package rosacli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	logger "github.com/openshift/openshift-tests-private/test/extended/util/logext"
)

var RoleTypeSuffixMap = map[string]string{
	"Installer":     "Installer-Role",
	"Support":       "Support-Role",
	"Control plane": "ControlPlane-Role",
	"Worker":        "Worker-Role",
}

type OCMResourceService interface {
	listRegion(flags ...string) (bytes.Buffer, error)
	reflectRegionList(result bytes.Buffer) (regions []*CloudRegion, err error)
	listUserRole() (bytes.Buffer, error)
	deleteUserRole(flags ...string) (bytes.Buffer, error)
	linkUserRole(flags ...string) (bytes.Buffer, error)
	unlinkUserRole(flags ...string) (bytes.Buffer, error)
	createUserRole(flags ...string) (bytes.Buffer, error)
	whoami() (bytes.Buffer, error)
	reflectAccountsInfo(result bytes.Buffer) *AccountsInfo
	reflectUserRoleList(result bytes.Buffer) (url UserRoleList, err error)
	createAccountRole(flags ...string) (bytes.Buffer, error)
	reflectAccountRoleList(result bytes.Buffer) (arl AccountRoleList, err error)
	deleteAccountRole(flags ...string) (bytes.Buffer, error)
	listAccountRole() (bytes.Buffer, error)
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

// Struct for the 'rosa list user-role' output
type UserRole struct {
	RoleName string `json:"ROLE NAME,omitempty"`
	RoleArn  string `json:"ROLE ARN,omitempty"`
	Linded   string `json:"LINKED,omitempty"`
}

type UserRoleList struct {
	UserRoleList []UserRole `json:"UserRoleList,omitempty"`
}
type AccountsInfo struct {
	AWSArn                    string `json:"AWS ARN,omitempty"`
	AWSAccountID              string `json:"AWS Account ID,omitempty"`
	AWSDefaultRegion          string `json:"AWS Default Region,omitempty"`
	OCMApi                    string `json:"OCM API,omitempty"`
	OCMAccountEmail           string `json:"OCM Account Email,omitempty"`
	OCMAccountID              string `json:"OCM Account ID,omitempty"`
	OCMAccountName            string `json:"OCM Account Name,omitempty"`
	OCMAccountUsername        string `json:"OCM Account Username,omitempty"`
	OCMOrganizationExternalID string `json:"OCM Organization External ID,omitempty"`
	OCMOrganizationID         string `json:"OCM Organization ID,omitempty"`
	OCMOrganizationName       string `json:"OCM Organization Name,omitempty"`
}

type AccountRole struct {
	RoleName         string `json:"ROLE NAME,omitempty"`
	RoleType         string `json:"ROLE TYPE,omitempty"`
	RoleArn          string `json:"ROLE ARN,omitempty"`
	OpenshiftVersion string `json:"OPENSHIFT VERSION,omitempty"`
	AWSManaged       string `json:"AWS Managed,omitempty"`
}
type AccountRoleList struct {
	AccountRoleList []AccountRole `json:"AccountRoleList,omitempty"`
}

// List region
func (ors *ocmResourceService) listRegion(flags ...string) (bytes.Buffer, error) {
	listRegion := ors.client.Runner
	listRegion = listRegion.Cmd("list", "regions").CmdFlags(flags...)
	return listRegion.Run()
}

// Pasrse the result of 'rosa regions' to the RegionInfo struct
func (ors *ocmResourceService) reflectRegionList(result bytes.Buffer) (regions []*CloudRegion, err error) {
	theMap := ors.client.Parser.tableData.Input(result).Parse().output
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

// Pasrse the result of 'rosa whoami' to the AccountsInfo struct
func (ors *ocmResourceService) reflectAccountsInfo(result bytes.Buffer) *AccountsInfo {
	res := new(AccountsInfo)
	theMap, _ := ors.client.Parser.textData.Input(result).Parse().jsonToMap()
	data, _ := json.Marshal(&theMap)
	json.Unmarshal(data, res)
	return res
}

// Pasrse the result of 'rosa list user-roles' to NodePoolList struct
func (ors *ocmResourceService) reflectUserRoleList(result bytes.Buffer) (url UserRoleList, err error) {
	url = UserRoleList{}
	theMap := ors.client.Parser.tableData.Input(result).Parse().output
	for _, userroleItem := range theMap {
		ur := &UserRole{}
		err = mapStructure(userroleItem, ur)
		if err != nil {
			return
		}
		url.UserRoleList = append(url.UserRoleList, *ur)
	}
	return
}

// run `rosa list user-role` command
func (ors *ocmResourceService) listUserRole() (bytes.Buffer, error) {
	ors.client.Runner.cmdArgs = []string{}
	listUserRole := ors.client.Runner.
		Cmd("list", "user-role")
	return listUserRole.Run()

}

// run `rosa delete user-role` command
func (ors *ocmResourceService) deleteUserRole(flags ...string) (bytes.Buffer, error) {
	deleteUserRole := ors.client.Runner
	deleteUserRole = deleteUserRole.Cmd("delete", "user-role").CmdFlags(flags...)
	return deleteUserRole.Run()
}

// run `rosa link user-role` command
func (ors *ocmResourceService) linkUserRole(flags ...string) (bytes.Buffer, error) {
	linkUserRole := ors.client.Runner
	linkUserRole = linkUserRole.Cmd("link", "user-role").CmdFlags(flags...)
	return linkUserRole.Run()
}

// run `rosa unlink user-role` command
func (ors *ocmResourceService) unlinkUserRole(flags ...string) (bytes.Buffer, error) {
	unlinkUserRole := ors.client.Runner
	unlinkUserRole = unlinkUserRole.Cmd("unlink", "user-role").CmdFlags(flags...)
	return unlinkUserRole.Run()
}

// run `rosa create user-role` command
func (ors *ocmResourceService) createUserRole(flags ...string) (bytes.Buffer, error) {
	createUserRole := ors.client.Runner
	createUserRole = createUserRole.Cmd("create", "user-role").CmdFlags(flags...)
	return createUserRole.Run()
}

// run `rosa whoami` command
func (ors *ocmResourceService) whoami() (bytes.Buffer, error) {
	ors.client.Runner.cmdArgs = []string{}
	whoami := ors.client.Runner.
		Cmd("whoami").OutputFormat()
	return whoami.Run()
}

// Get specified user-role by user-role prefix and ocmAccountUsername
func (url UserRoleList) userRole(prefix string, ocmAccountUsername string) (userRoles UserRole) {
	userRoleName := fmt.Sprintf("%s-User-%s-Role", prefix, ocmAccountUsername)
	for _, roleItme := range url.UserRoleList {
		if roleItme.RoleName == userRoleName {
			logger.Infof("Find the userRole %s ~", userRoleName)
			return roleItme
		}
	}
	return
}

// run `rosa create account-roles` command
func (ors *ocmResourceService) createAccountRole(flags ...string) (bytes.Buffer, error) {
	createAccountRole := ors.client.Runner
	createAccountRole = createAccountRole.Cmd("create", "account-roles").CmdFlags(flags...)
	return createAccountRole.Run()
}

// Pasrse the result of 'rosa list account-roles' to AccountRoleList struct
func (ors *ocmResourceService) reflectAccountRoleList(result bytes.Buffer) (arl AccountRoleList, err error) {
	arl = AccountRoleList{}
	theMap := ors.client.Parser.tableData.Input(result).Parse().output
	for _, accountRoleItem := range theMap {
		ar := &AccountRole{}
		err = mapStructure(accountRoleItem, ar)
		if err != nil {
			return
		}
		arl.AccountRoleList = append(arl.AccountRoleList, *ar)
	}
	return
}

// run `rosa delete account-roles` command
func (ors *ocmResourceService) deleteAccountRole(flags ...string) (bytes.Buffer, error) {
	deleteAccountRole := ors.client.Runner
	deleteAccountRole = deleteAccountRole.Cmd("delete", "account-roles").CmdFlags(flags...)
	return deleteAccountRole.Run()
}

// run `rosa list account-roles` command
func (ors *ocmResourceService) listAccountRole() (bytes.Buffer, error) {
	ors.client.Runner.cmdArgs = []string{}
	listAccountRole := ors.client.Runner.
		Cmd("list", "account-roles")
	return listAccountRole.Run()

}

// Get specified account roles by prefix
func (arl AccountRoleList) accountRoles(prefix string) (accountRoles []AccountRole) {

	for _, roleItme := range arl.AccountRoleList {
		if strings.Contains(roleItme.RoleName, prefix) {
			accountRoles = append(accountRoles, roleItme)
		}
	}
	return
}
