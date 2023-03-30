package mco

// ign32Contents describes the "contents" field in an ignition 3.2.0 File configuration
type ign32Contents struct {
	Compression string `json:"compression,omitempty"`
	Source      string `json:"source,omitempty"`
}

// ign32FileUser describes the user that will own a given file
type ign32FileUser struct {
	Name string `json:"name,omitempty"`
	ID   *int   `json:"id,omitempty"`
}

// ign32FileUser describes the group that will own a given file
type ign32FileGroup struct {
	Name string `json:"name,omitempty"`
	ID   *int   `json:"id,omitempty"`
}

// ign32File describes the configuration of a File in ignition 3.2.0
type ign32File struct {
	Path     string         `json:"path,omitempty"`
	Contents ign32Contents  `json:"contents,omitempty"`
	Mode     *int           `json:"mode,omitempty"`
	User     ign32FileUser  `json:"user,omitempty"`
	Group    ign32FileGroup `json:"group,omitempty"`
}

// ign32PaswdUser describes the passwd data regarding a given user
type ign32PaswdUser struct {
	Name              string   `json:"name,omitempty"`
	SSHAuthorizedKeys []string `json:"sshAuthorizedKeys,omitempty"`
	PasswordHash      string   `json:"passwordHash,omitempty"`
}
