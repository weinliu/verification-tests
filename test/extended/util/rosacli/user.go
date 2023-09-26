package rosacli

import (
	"bytes"

	logger "github.com/openshift/openshift-tests-private/test/extended/util/logext"
)

type UserService interface {
	ListUsers(clusterID string) (bytes.Buffer, error)
	ReflectUsersList(result bytes.Buffer) (gul GroupUserList, err error)
	RevokeUser(clusterID string, flags ...string) (bytes.Buffer, error)
	GrantUser(clusterID string, flags ...string) (bytes.Buffer, error)
	RemoveAllUsers(clusterID string) (err error)
	CreateAdmin(clusterID string) (bytes.Buffer, error)
	DescribeAdmin(clusterID string) (bytes.Buffer, error)
	DeleteAdmin(clusterID string) (bytes.Buffer, error)
}

var _ UserService = &userService{}

type userService Service

// Struct for the 'rosa list users' output
type GroupUser struct {
	ID     string `json:"ID,omitempty"`
	Groups string `json:"GROUPS,omitempty"`
}
type GroupUserList struct {
	GroupUsers []GroupUser `json:"GroupUsers,omitempty"`
}

// Grant user
func (c *userService) GrantUser(clusterID string, flags ...string) (bytes.Buffer, error) {
	grantUser := c.Client.Runner.
		Cmd("grant", "user").
		CmdFlags(append([]string{"-c", clusterID}, flags...)...)

	return grantUser.Run()
}

// Revoke user
func (c *userService) RevokeUser(clusterID string, flags ...string) (bytes.Buffer, error) {
	combflags := append([]string{"-c", clusterID}, flags...)
	revokeUser := c.Client.Runner.
		Cmd("revoke", "user").
		CmdFlags(combflags...)

	return revokeUser.Run()
}

// List users
func (c *userService) ListUsers(clusterID string) (bytes.Buffer, error) {
	listUsers := c.Client.Runner.
		Cmd("list", "users").
		CmdFlags("-c", clusterID)
	return listUsers.Run()
}

// Pasrse the result of 'rosa list user' to  []*GroupUser struct
func (c *userService) ReflectUsersList(result bytes.Buffer) (gul GroupUserList, err error) {
	gul = GroupUserList{}
	theMap := c.Client.Parser.TableData.Input(result).Parse().Output()
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

// Delete all users
func (c *userService) RemoveAllUsers(clusterID string) (err error) {
	out, err := c.ListUsers(clusterID)
	gul, err := c.ReflectUsersList(out)
	if err != nil {
		return err
	}
	if len(gul.GroupUsers) != 0 {
		for _, uitem := range gul.GroupUsers {
			_, err = c.RevokeUser(clusterID,
				uitem.Groups,
				"--user", uitem.ID,
				"-y",
			)
			if err != nil {
				return err
			}
		}
	} else {
		logger.Infof("There is no user existed on cluster %s ~", clusterID)
		return nil
	}
	return err
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
func (c *userService) CreateAdmin(clusterID string) (bytes.Buffer, error) {
	createAdmin := c.Client.Runner.
		Cmd("create", "admin").
		CmdFlags("-c", clusterID, "-y")

	return createAdmin.Run()
}

// describe admin
func (c *userService) DescribeAdmin(clusterID string) (bytes.Buffer, error) {
	describeAdmin := c.Client.Runner.
		Cmd("describe", "admin").
		CmdFlags("-c", clusterID)

	return describeAdmin.Run()
}

// delete admin
func (c *userService) DeleteAdmin(clusterID string) (bytes.Buffer, error) {
	deleteAdmin := c.Client.Runner.
		Cmd("delete", "admin").
		CmdFlags("-c", clusterID, "-y")

	return deleteAdmin.Run()
}
