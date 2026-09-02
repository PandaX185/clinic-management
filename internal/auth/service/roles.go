package service

// StandardRoles returns the set of roles seeded into every clinic schema on
// provision. Tenant (schema provisioner) and directory (role validation)
// both consult this set so the role vocabulary lives in exactly one place.
func StandardRoles() map[string]bool {
	return map[string]bool{
		string(RoleAdmin): true, string(RoleStaff): true, string(RoleDoctor): true,
		string(RoleNurse): true, string(RoleManager): true, string(RolePatient): true,
	}
}
