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
	ListRegion(flags ...string) ([]*CloudRegion, bytes.Buffer, error)
	ReflectRegionList(result bytes.Buffer) (regions []*CloudRegion, err error)
	ListUserRole() (UserRoleList, bytes.Buffer, error)
	DeleteUserRole(flags ...string) (bytes.Buffer, error)
	LinkUserRole(flags ...string) (bytes.Buffer, error)
	UnlinkUserRole(flags ...string) (bytes.Buffer, error)
	CreateUserRole(flags ...string) (bytes.Buffer, error)
	ReflectUserRoleList(result bytes.Buffer) (url UserRoleList, err error)
	Whoami() (bytes.Buffer, error)
	ReflectAccountsInfo(result bytes.Buffer) *AccountsInfo
	CreateAccountRole(flags ...string) (bytes.Buffer, error)
	ReflectAccountRoleList(result bytes.Buffer) (arl AccountRoleList, err error)
	DeleteAccountRole(flags ...string) (bytes.Buffer, error)
	ListAccountRole() (AccountRoleList, bytes.Buffer, error)
	ListOCMRole() (OCMRoleList, bytes.Buffer, error)
	DeleteOCMRole(flags ...string) (bytes.Buffer, error)
	LinkOCMRole(flags ...string) (bytes.Buffer, error)
	UnlinkOCMRole(flags ...string) (bytes.Buffer, error)
	CreateOCMRole(flags ...string) (bytes.Buffer, error)
	ReflectOCMRoleList(result bytes.Buffer) (orl OCMRoleList, err error)

	ListOIDCConfig() (OIDCConfigList, bytes.Buffer, error)
	DeleteOIDCConfig(flags ...string) (bytes.Buffer, error)
	CreateOIDCConfig(flags ...string) (bytes.Buffer, error)
	ReflectOIDCConfigList(result bytes.Buffer) (oidclist OIDCConfigList, err error)

	DeleteOperatorRoles(flags ...string) (bytes.Buffer, error)
	CreateOperatorRoles(flags ...string) (bytes.Buffer, error)

	CreateOIDCProvider(flags ...string) (bytes.Buffer, error)
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

// Struct for the 'rosa list ocm-role' output
type OCMRole struct {
	RoleName   string `json:"ROLE NAME,omitempty"`
	RoleArn    string `json:"ROLE ARN,omitempty"`
	Linded     string `json:"LINKED,omitempty"`
	Admin      string `json:"ADMIN,omitempty"`
	AwsManaged string `json:"AWS MANAGED,omitempty"`
}

type OCMRoleList struct {
	OCMRoleList []OCMRole `json:"OCMRoleList,omitempty"`
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

type OIDCConfig struct {
	ID        string `json:"ID,omitempty"`
	Managed   string `json:"MANAGED,omitempty"`
	IssuerUrl string `json:"ISSUER URL,omitempty"`
	SecretArn string `json:"SECRET ARN,omitempty"`
}
type OIDCConfigList struct {
	OIDCConfigList []OIDCConfig `json:"OIDCConfigList,omitempty"`
}

// List region
func (ors *ocmResourceService) ListRegion(flags ...string) ([]*CloudRegion, bytes.Buffer, error) {
	listRegion := ors.Client.Runner
	listRegion = listRegion.Cmd("list", "regions").CmdFlags(flags...)
	output, err := listRegion.Run()
	if err != nil {
		return []*CloudRegion{}, output, err
	}
	rList, err := ors.ReflectRegionList(output)
	return rList, output, err
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
func (ors *ocmResourceService) ListUserRole() (UserRoleList, bytes.Buffer, error) {
	ors.Client.Runner.cmdArgs = []string{}
	listUserRole := ors.Client.Runner.
		Cmd("list", "user-role")
	output, err := listUserRole.Run()
	if err != nil {
		return UserRoleList{}, output, err
	}
	uList, err := ors.ReflectUserRoleList(output)
	return uList, output, err

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
func (ors *ocmResourceService) ListAccountRole() (AccountRoleList, bytes.Buffer, error) {
	ors.Client.Runner.cmdArgs = []string{}
	listAccountRole := ors.Client.Runner.
		Cmd("list", "account-roles")
	output, err := listAccountRole.Run()
	if err != nil {
		return AccountRoleList{}, output, err
	}
	arl, err := ors.ReflectAccountRoleList(output)
	return arl, output, err

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

// run `rosa create ocm-role` command
func (ors *ocmResourceService) CreateOCMRole(flags ...string) (bytes.Buffer, error) {
	createOCMRole := ors.Client.Runner
	createOCMRole = createOCMRole.Cmd("create", "ocm-role").CmdFlags(flags...)
	return createOCMRole.Run()
}

// run `rosa list ocm-role` command
func (ors *ocmResourceService) ListOCMRole() (OCMRoleList, bytes.Buffer, error) {
	ors.Client.Runner.cmdArgs = []string{}
	listOCMRole := ors.Client.Runner.
		Cmd("list", "ocm-role")
	output, err := listOCMRole.Run()
	if err != nil {
		return OCMRoleList{}, output, err
	}
	orl, err := ors.ReflectOCMRoleList(output)
	return orl, output, err
}

// run `rosa delete ocm-role` command
func (ors *ocmResourceService) DeleteOCMRole(flags ...string) (bytes.Buffer, error) {
	deleteOCMRole := ors.Client.Runner
	deleteOCMRole = deleteOCMRole.Cmd("delete", "ocm-role").CmdFlags(flags...)
	return deleteOCMRole.Run()
}

// run `rosa link ocm-role` command
func (ors *ocmResourceService) LinkOCMRole(flags ...string) (bytes.Buffer, error) {
	linkOCMRole := ors.Client.Runner
	linkOCMRole = linkOCMRole.Cmd("link", "ocm-role").CmdFlags(flags...)
	return linkOCMRole.Run()
}

// run `rosa unlink ocm-role` command
func (ors *ocmResourceService) UnlinkOCMRole(flags ...string) (bytes.Buffer, error) {
	unlinkOCMRole := ors.Client.Runner
	unlinkOCMRole = unlinkOCMRole.Cmd("unlink", "ocm-role").CmdFlags(flags...)
	return unlinkOCMRole.Run()
}

// Pasrse the result of 'rosa list user-ocm' to NodePoolList struct
func (ors *ocmResourceService) ReflectOCMRoleList(result bytes.Buffer) (orl OCMRoleList, err error) {
	orl = OCMRoleList{}
	theMap := ors.Client.Parser.TableData.Input(result).Parse().Output()
	for _, ocmRoleItem := range theMap {
		or := &OCMRole{}
		err = MapStructure(ocmRoleItem, or)
		if err != nil {
			return
		}
		orl.OCMRoleList = append(orl.OCMRoleList, *or)
	}
	return
}

// Get specified ocm-role by ocm-role prefix and ocmOUsername
func (url OCMRoleList) OCMRole(prefix string, ocmOrganizationExternalID string) (userRoles OCMRole) {
	ocmRoleName := fmt.Sprintf("%s-OCM-Role-%s", prefix, ocmOrganizationExternalID)
	for _, roleItme := range url.OCMRoleList {
		if roleItme.RoleName == ocmRoleName {
			logger.Infof("Find the ocm Role %s ~", ocmRoleName)
			return roleItme
		}
	}
	return
}

// run `rosa create oidc-config` command
func (ors *ocmResourceService) CreateOIDCConfig(flags ...string) (bytes.Buffer, error) {
	createOIDCConfig := ors.Client.Runner
	createOIDCConfig = createOIDCConfig.Cmd("create", "oidc-config").CmdFlags(flags...)
	return createOIDCConfig.Run()
}

// run `rosa list oidc-config` command
func (ors *ocmResourceService) ListOIDCConfig() (OIDCConfigList, bytes.Buffer, error) {
	ors.Client.Runner.cmdArgs = []string{}
	listOIDCConfig := ors.Client.Runner.
		Cmd("list", "oidc-config")
	output, err := listOIDCConfig.Run()
	if err != nil {
		return OIDCConfigList{}, output, err
	}
	oidcl, err := ors.ReflectOIDCConfigList(output)
	return oidcl, output, err

}

// run `rosa delete oidc-config` command
func (ors *ocmResourceService) DeleteOIDCConfig(flags ...string) (bytes.Buffer, error) {
	deleteOIDCConfig := ors.Client.Runner
	deleteOIDCConfig = deleteOIDCConfig.Cmd("delete", "oidc-config").CmdFlags(flags...)
	return deleteOIDCConfig.Run()
}

// Pasrse the result of 'rosa list oidc-config' to OIDCConfigList struct
func (ors *ocmResourceService) ReflectOIDCConfigList(result bytes.Buffer) (oidcl OIDCConfigList, err error) {
	oidcl = OIDCConfigList{}
	theMap := ors.Client.Parser.TableData.Input(result).Parse().Output()
	for _, oidcConfigItem := range theMap {
		oidc := &OIDCConfig{}
		err = MapStructure(oidcConfigItem, oidc)
		if err != nil {
			return
		}
		oidcl.OIDCConfigList = append(oidcl.OIDCConfigList, *oidc)
	}
	return
}

// Get specified oidc-config by oidc-config-id
func (oidcl OIDCConfigList) OIDCConfig(id string) (oidc OIDCConfig) {
	for _, item := range oidcl.OIDCConfigList {
		if item.ID == id {
			return item
		}
	}
	return
}

// run `rosa create operator-roles` command
func (ors *ocmResourceService) CreateOperatorRoles(flags ...string) (bytes.Buffer, error) {
	createOperatorRoles := ors.Client.Runner
	createOperatorRoles = createOperatorRoles.Cmd("create", "operator-roles").CmdFlags(flags...)
	return createOperatorRoles.Run()
}

// run `rosa delete operator-roles` command
func (ors *ocmResourceService) DeleteOperatorRoles(flags ...string) (bytes.Buffer, error) {
	deleteOperatorRoles := ors.Client.Runner
	deleteOperatorRoles = deleteOperatorRoles.Cmd("delete", "operator-roles").CmdFlags(flags...)
	return deleteOperatorRoles.Run()
}

// run `rosa create oidc-proviedr` command
func (ors *ocmResourceService) CreateOIDCProvider(flags ...string) (bytes.Buffer, error) {
	createODICProvider := ors.Client.Runner
	createODICProvider = createODICProvider.Cmd("create", "oidc-provider").CmdFlags(flags...)
	return createODICProvider.Run()
}
