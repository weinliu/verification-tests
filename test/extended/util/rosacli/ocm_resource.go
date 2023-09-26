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
	ListRegion(flags ...string) (bytes.Buffer, error)
	ReflectRegionList(result bytes.Buffer) (regions []*CloudRegion, err error)
	ListUserRole() (bytes.Buffer, error)
	DeleteUserRole(flags ...string) (bytes.Buffer, error)
	LinkUserRole(flags ...string) (bytes.Buffer, error)
	UnlinkUserRole(flags ...string) (bytes.Buffer, error)
	CreateUserRole(flags ...string) (bytes.Buffer, error)
	Whoami() (bytes.Buffer, error)
	ReflectAccountsInfo(result bytes.Buffer) *AccountsInfo
	ReflectUserRoleList(result bytes.Buffer) (url UserRoleList, err error)
	CreateAccountRole(flags ...string) (bytes.Buffer, error)
	ReflectAccountRoleList(result bytes.Buffer) (arl AccountRoleList, err error)
	DeleteAccountRole(flags ...string) (bytes.Buffer, error)
	ListAccountRole() (bytes.Buffer, error)
}

var _ OCMResourceService = &ocmResourceService{}

type ocmResourceService Service

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
func (ors *ocmResourceService) ListRegion(flags ...string) (bytes.Buffer, error) {
	listRegion := ors.Client.Runner
	listRegion = listRegion.Cmd("list", "regions").CmdFlags(flags...)
	return listRegion.Run()
}

// Pasrse the result of 'rosa regions' to the RegionInfo struct
func (ors *ocmResourceService) ReflectRegionList(result bytes.Buffer) (regions []*CloudRegion, err error) {
	theMap := ors.Client.Parser.TableData.Input(result).Parse().Output()
	for _, regionItem := range theMap {
		region := &CloudRegion{}
		err = MapStructure(regionItem, region)
		if err != nil {
			return
		}
		regions = append(regions, region)
	}
	return
}

// Pasrse the result of 'rosa whoami' to the AccountsInfo struct
func (ors *ocmResourceService) ReflectAccountsInfo(result bytes.Buffer) *AccountsInfo {
	res := new(AccountsInfo)
	theMap, _ := ors.Client.Parser.TextData.Input(result).Parse().JsonToMap()
	data, _ := json.Marshal(&theMap)
	json.Unmarshal(data, res)
	return res
}

// Pasrse the result of 'rosa list user-roles' to NodePoolList struct
func (ors *ocmResourceService) ReflectUserRoleList(result bytes.Buffer) (url UserRoleList, err error) {
	url = UserRoleList{}
	theMap := ors.Client.Parser.TableData.Input(result).Parse().Output()
	for _, userroleItem := range theMap {
		ur := &UserRole{}
		err = MapStructure(userroleItem, ur)
		if err != nil {
			return
		}
		url.UserRoleList = append(url.UserRoleList, *ur)
	}
	return
}

// run `rosa list user-role` command
func (ors *ocmResourceService) ListUserRole() (bytes.Buffer, error) {
	ors.Client.Runner.cmdArgs = []string{}
	listUserRole := ors.Client.Runner.
		Cmd("list", "user-role")
	return listUserRole.Run()

}

// run `rosa delete user-role` command
func (ors *ocmResourceService) DeleteUserRole(flags ...string) (bytes.Buffer, error) {
	deleteUserRole := ors.Client.Runner
	deleteUserRole = deleteUserRole.Cmd("delete", "user-role").CmdFlags(flags...)
	return deleteUserRole.Run()
}

// run `rosa link user-role` command
func (ors *ocmResourceService) LinkUserRole(flags ...string) (bytes.Buffer, error) {
	linkUserRole := ors.Client.Runner
	linkUserRole = linkUserRole.Cmd("link", "user-role").CmdFlags(flags...)
	return linkUserRole.Run()
}

// run `rosa unlink user-role` command
func (ors *ocmResourceService) UnlinkUserRole(flags ...string) (bytes.Buffer, error) {
	unlinkUserRole := ors.Client.Runner
	unlinkUserRole = unlinkUserRole.Cmd("unlink", "user-role").CmdFlags(flags...)
	return unlinkUserRole.Run()
}

// run `rosa create user-role` command
func (ors *ocmResourceService) CreateUserRole(flags ...string) (bytes.Buffer, error) {
	createUserRole := ors.Client.Runner
	createUserRole = createUserRole.Cmd("create", "user-role").CmdFlags(flags...)
	return createUserRole.Run()
}

// run `rosa whoami` command
func (ors *ocmResourceService) Whoami() (bytes.Buffer, error) {
	ors.Client.Runner.cmdArgs = []string{}
	whoami := ors.Client.Runner.
		Cmd("whoami").OutputFormat()
	return whoami.Run()
}

// Get specified user-role by user-role prefix and ocmAccountUsername
func (url UserRoleList) UserRole(prefix string, ocmAccountUsername string) (userRoles UserRole) {
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
func (ors *ocmResourceService) CreateAccountRole(flags ...string) (bytes.Buffer, error) {
	createAccountRole := ors.Client.Runner
	createAccountRole = createAccountRole.Cmd("create", "account-roles").CmdFlags(flags...)
	return createAccountRole.Run()
}

// Pasrse the result of 'rosa list account-roles' to AccountRoleList struct
func (ors *ocmResourceService) ReflectAccountRoleList(result bytes.Buffer) (arl AccountRoleList, err error) {
	arl = AccountRoleList{}
	theMap := ors.Client.Parser.TableData.Input(result).Parse().Output()
	for _, accountRoleItem := range theMap {
		ar := &AccountRole{}
		err = MapStructure(accountRoleItem, ar)
		if err != nil {
			return
		}
		arl.AccountRoleList = append(arl.AccountRoleList, *ar)
	}
	return
}

// run `rosa delete account-roles` command
func (ors *ocmResourceService) DeleteAccountRole(flags ...string) (bytes.Buffer, error) {
	deleteAccountRole := ors.Client.Runner
	deleteAccountRole = deleteAccountRole.Cmd("delete", "account-roles").CmdFlags(flags...)
	return deleteAccountRole.Run()
}

// run `rosa list account-roles` command
func (ors *ocmResourceService) ListAccountRole() (bytes.Buffer, error) {
	ors.Client.Runner.cmdArgs = []string{}
	listAccountRole := ors.Client.Runner.
		Cmd("list", "account-roles")
	return listAccountRole.Run()

}

// Get specified account roles by prefix
func (arl AccountRoleList) AccountRoles(prefix string) (accountRoles []AccountRole) {

	for _, roleItme := range arl.AccountRoleList {
		if strings.Contains(roleItme.RoleName, prefix) {
			accountRoles = append(accountRoles, roleItme)
		}
	}
	return
}
