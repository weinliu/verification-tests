package mco

// ign32Contents describes the "contents" field in an ignition 3.2.0 File configuration
type ign32Contents struct {
	Compression string `json:"compression,omitempty"`
	Source      string `json:"source,omitempty"`
}

type ign32FileUser struct {
	Name string `json:"name,omitempty"`
	ID   *int   `json:"id,omitempty"`
}

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
