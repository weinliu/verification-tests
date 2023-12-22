package rosacli

import (
	"bytes"

	"github.com/openshift/openshift-tests-private/test/extended/util/logext"
)

type UserService interface {
	ResourcesCleaner

	ListUsers(clusterID string) (GroupUserList, bytes.Buffer, error)
	ReflectUsersList(result bytes.Buffer) (gul GroupUserList, err error)
	RevokeUser(clusterID string, role string, user string, flags ...string) (bytes.Buffer, error)
	GrantUser(clusterID string, role string, user string, flags ...string) (bytes.Buffer, error)
	CreateAdmin(clusterID string) (bytes.Buffer, error)
	DescribeAdmin(clusterID string) (bytes.Buffer, error)
	DeleteAdmin(clusterID string) (bytes.Buffer, error)
}

type userService struct {
	ResourcesService

	usersGranted map[string][]*userRole
	adminCreated []string
}

type userRole struct {
	user string
	role string
}

func NewUserService(client *Client) UserService {
	return &userService{
		ResourcesService: ResourcesService{
			client: client,
		},
		usersGranted: make(map[string][]*userRole),
	}
}

// Struct for the 'rosa list users' output
type GroupUser struct {
	ID     string `json:"ID,omitempty"`
	Groups string `json:"GROUPS,omitempty"`
}
type GroupUserList struct {
	GroupUsers []GroupUser `json:"GroupUsers,omitempty"`
}

// Grant user
func (us *userService) GrantUser(clusterID string, role string, user string, flags ...string) (output bytes.Buffer, err error) {
	output, err = us.grantUser(clusterID, role, user, flags...)
	if err == nil {
		createdUserRole := &userRole{
			user: user,
			role: role,
		}
		us.usersGranted[clusterID] = append(us.usersGranted[clusterID], createdUserRole)
	}
	return
}

func (us *userService) grantUser(clusterID string, role string, user string, flags ...string) (bytes.Buffer, error) {
	grantUser := us.client.Runner.
		Cmd("grant", "user", role).
		CmdFlags(append(flags, "-c", clusterID, "--user", user)...)

	return grantUser.Run()
}

// Revoke user
func (us *userService) RevokeUser(clusterID string, role string, user string, flags ...string) (output bytes.Buffer, err error) {
	output, err = us.revokeUser(clusterID, role, user, flags...)
	if err == nil {
		var newRoles []*userRole
		for _, createdUserRole := range us.usersGranted[clusterID] {
			if createdUserRole.user != user || createdUserRole.role != role {
				newRoles = append(newRoles, createdUserRole)
			}
		}
		us.usersGranted[clusterID] = newRoles
	}
	return
}

func (us *userService) revokeUser(clusterID string, role string, user string, flags ...string) (bytes.Buffer, error) {
	revokeUser := us.client.Runner.
		Cmd("revoke", "user", role).
		CmdFlags(append(flags, "-y", "-c", clusterID, "--user", user)...)

	return revokeUser.Run()
}

// List users
func (us *userService) ListUsers(clusterID string) (GroupUserList, bytes.Buffer, error) {
	listUsers := us.client.Runner.
		Cmd("list", "users").
		CmdFlags("-c", clusterID)
	output, err := listUsers.Run()
	if err != nil {
		return GroupUserList{}, output, err
	}
	gul, err := us.ReflectUsersList(output)
	return gul, output, err
}

// Pasrse the result of 'rosa list user' to  []*GroupUser struct
func (us *userService) ReflectUsersList(result bytes.Buffer) (gul GroupUserList, err error) {
	gul = GroupUserList{}
	theMap := us.client.Parser.TableData.Input(result).Parse().Output()
	for _, userItem := range theMap {
		user := &GroupUser{}
		err = MapStructure(userItem, user)
		if err != nil {
			return
		}
		gul.GroupUsers = append(gul.GroupUsers, *user)
	}
	return gul, err
}

// Get specified user by user name
func (gl GroupUserList) User(userName string) (user GroupUser, err error) {
	for _, userItem := range gl.GroupUsers {
		if userItem.ID == userName {
			user = userItem
			return
		}
	}
	return
}

// Create admin
func (us *userService) CreateAdmin(clusterID string) (output bytes.Buffer, err error) {
	createAdmin := us.client.Runner.
		Cmd("create", "admin").
		CmdFlags("-c", clusterID, "-y")

	output, err = createAdmin.Run()
	if err == nil {
		us.adminCreated = appendToStringSliceIfNotExist(us.adminCreated, clusterID)
		logext.Infof("Add admin to Cluster %v", clusterID)
		logext.Infof("Admin created =  %v", us.adminCreated)
	}
	return
}

// describe admin
func (us *userService) DescribeAdmin(clusterID string) (bytes.Buffer, error) {
	describeAdmin := us.client.Runner.
		Cmd("describe", "admin").
		CmdFlags("-c", clusterID)

	return describeAdmin.Run()
}

// delete admin
func (us *userService) DeleteAdmin(clusterID string) (output bytes.Buffer, err error) {
	deleteAdmin := us.client.Runner.
		Cmd("delete", "admin").
		CmdFlags("-c", clusterID, "-y")

	output, err = deleteAdmin.Run()
	if err == nil {
		us.adminCreated = removeFromStringSlice(us.adminCreated, clusterID)
	}
	return
}

func (us *userService) CleanResources(clusterID string) (errors []error) {
	if sliceContains(us.adminCreated, clusterID) {
		logext.Infof("Remove remaining admin")
		if _, err := us.DeleteAdmin(clusterID); err != nil {
			errors = append(errors, err)
		}
	}

	for _, grantedUserRole := range us.usersGranted[clusterID] {
		logext.Infof("Remove remaining granted user '%s' with role '%s'", grantedUserRole.user, grantedUserRole.role)
		_, err := us.RevokeUser(clusterID, grantedUserRole.role, grantedUserRole.user)
		if err != nil {
			errors = append(errors, err)
		}
	}

	return
}
