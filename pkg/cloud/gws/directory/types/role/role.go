package role

import "github.com/altshiftab/utils_go/pkg/cloud/gws/directory/types/role/role_privilege"

type Role struct {
	Kind string `json:"kind,omitempty"`
	Etag string `json:"etag,omitempty"`

	RoleId          string `json:"roleId,omitempty"`
	RoleName        string `json:"roleName,omitempty"`
	RoleDescription string `json:"roleDescription,omitempty"`

	RolePrivileges []*role_privilege.RolePrivilege `json:"rolePrivileges,omitempty"`

	IsSystemRole     bool `json:"isSystemRole,omitempty"`
	IsSuperAdminRole bool `json:"isSuperAdminRole,omitempty"`
}
