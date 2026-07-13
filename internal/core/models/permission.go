package domain

type Permission struct {
	ID     string `json:"id"`
	RoleID string `json:"role_id"`
	Name   string `json:"name"`
}

func NewPermission(id, roleID, name string) *Permission {
	return &Permission{
		ID:     id,
		RoleID: roleID,
		Name:   name,
	}
}
