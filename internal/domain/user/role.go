package user

type Role string

const (
	RoleNormal Role = "normal"
	RoleBanned Role = "banned"
)

func (r Role) Valid() bool {
	switch r {
	case RoleNormal, RoleBanned:
		return true
	default:
		return false
	}
}

func (r Role) CanAccess() bool {
	return r == RoleNormal
}
