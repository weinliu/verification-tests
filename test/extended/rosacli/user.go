package rosacli

import (
	"bytes"

	logger "github.com/openshift/openshift-tests-private/test/extended/util/logext"
)

type UserService interface {
	listUsers(clusterID string) (bytes.Buffer, error)
	reflectUsersList(result bytes.Buffer) (gul GroupUserList, err error)
	revokeUser(clusterID string, flags ...string) (bytes.Buffer, error)
	grantUser(clusterID string, flags ...string) (bytes.Buffer, error)
	removeAllUsers(clusterID string) (err error)
	createAdmin(clusterID string) (bytes.Buffer, error)
	describeAdmin(clusterID string) (bytes.Buffer, error)
	deleteAdmin(clusterID string) (bytes.Buffer, error)
}

var _ UserService = &userService{}

type userService service

// Struct for the 'rosa list users' output
type GroupUser struct {
	ID     string `json:"ID,omitempty"`
	Groups string `json:"GROUPS,omitempty"`
}
type GroupUserList struct {
	GroupUsers []GroupUser `json:"GroupUsers,omitempty"`
}

// Grant user
func (c *userService) grantUser(clusterID string, flags ...string) (bytes.Buffer, error) {
	grantUser := c.client.Runner.
		Cmd("grant", "user").
		CmdFlags(append([]string{"-c", clusterID}, flags...)...)

	return grantUser.Run()
}

// Revoke user
func (c *userService) revokeUser(clusterID string, flags ...string) (bytes.Buffer, error) {
	combflags := append([]string{"-c", clusterID}, flags...)
	revokeUser := c.client.Runner.
		Cmd("revoke", "user").
		CmdFlags(combflags...)

	return revokeUser.Run()
}

// List users
func (c *userService) listUsers(clusterID string) (bytes.Buffer, error) {
	listUsers := c.client.Runner.
		Cmd("list", "users").
		CmdFlags("-c", clusterID)
	return listUsers.Run()
}

// Pasrse the result of 'rosa list user' to  []*GroupUser struct
func (c *userService) reflectUsersList(result bytes.Buffer) (gul GroupUserList, err error) {
	gul = GroupUserList{}
	theMap := c.client.Parser.tableData.Input(result).Parse().output
	for _, userItem := range theMap {
		user := &GroupUser{}
		err = mapStructure(userItem, user)
		if err != nil {
			return
		}
		gul.GroupUsers = append(gul.GroupUsers, *user)
	}
	return gul, err
}

// Delete all users
func (c *userService) removeAllUsers(clusterID string) (err error) {
	out, err := c.listUsers(clusterID)
	gul, err := c.reflectUsersList(out)
	if err != nil {
		return err
	}
	if len(gul.GroupUsers) != 0 {
		for _, uitem := range gul.GroupUsers {
			_, err = c.revokeUser(clusterID,
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
func (gl GroupUserList) user(userName string) (user GroupUser, err error) {
	for _, userItem := range gl.GroupUsers {
		if userItem.ID == userName {
			user = userItem
			return
		}
	}
	return
}

// Create admin
func (c *userService) createAdmin(clusterID string) (bytes.Buffer, error) {
	createAdmin := c.client.Runner.
		Cmd("create", "admin").
		CmdFlags("-c", clusterID, "-y")

	return createAdmin.Run()
}

// describe admin
func (c *userService) describeAdmin(clusterID string) (bytes.Buffer, error) {
	describeAdmin := c.client.Runner.
		Cmd("describe", "admin").
		CmdFlags("-c", clusterID)

	return describeAdmin.Run()
}

// delete admin
func (c *userService) deleteAdmin(clusterID string) (bytes.Buffer, error) {
	deleteAdmin := c.client.Runner.
		Cmd("delete", "admin").
		CmdFlags("-c", clusterID, "-y")

	return deleteAdmin.Run()
}
