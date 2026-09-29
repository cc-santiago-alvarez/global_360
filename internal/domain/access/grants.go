package access

import "sort"

// Grants is the effective set of permissions of a user, resolved from its
// active role assignments.
type Grants struct {
	global    map[string]bool
	byCompany map[string]map[string]bool
}

// BuildGrants resolves the permissions granted by active assignments of active roles.
func BuildGrants(assignments []*RoleAssignment, roles []*Role) *Grants {
	g := &Grants{global: map[string]bool{}, byCompany: map[string]map[string]bool{}}
	byID := make(map[string]*Role, len(roles))
	for _, r := range roles {
		byID[r.ID] = r
	}
	for _, a := range assignments {
		role, ok := byID[a.RoleID]
		if !a.Active || !ok || !role.Active {
			continue
		}
		target := g.global
		if a.CompanyID != nil {
			target = g.byCompany[*a.CompanyID]
			if target == nil {
				target = map[string]bool{}
				g.byCompany[*a.CompanyID] = target
			}
		}
		for _, p := range role.Permissions {
			target[p] = true
		}
	}
	return g
}

// Can reports whether code is granted globally or, when companyID is set, within that company.
func (g *Grants) Can(code string, companyID *string) bool {
	if g == nil {
		return false
	}
	if g.global[code] {
		return true
	}
	return companyID != nil && g.byCompany[*companyID][code]
}

// CanGlobally reports whether code is granted without company restriction.
func (g *Grants) CanGlobally(code string) bool { return g != nil && g.global[code] }

// HasAny reports whether code is granted globally or in at least one company.
func (g *Grants) HasAny(code string) bool {
	if g.CanGlobally(code) {
		return true
	}
	if g == nil {
		return false
	}
	for _, perms := range g.byCompany {
		if perms[code] {
			return true
		}
	}
	return false
}

// CompaniesWith returns the company IDs where code is granted (excluding global grants).
func (g *Grants) CompaniesWith(code string) []string {
	if g == nil {
		return nil
	}
	var ids []string
	for id, perms := range g.byCompany {
		if perms[code] {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

// GlobalPermissions returns the sorted globally granted codes.
func (g *Grants) GlobalPermissions() []string {
	if g == nil {
		return []string{}
	}
	return sortedKeys(g.global)
}

// CompanyPermissions returns the sorted codes granted per company.
func (g *Grants) CompanyPermissions() map[string][]string {
	out := map[string][]string{}
	if g == nil {
		return out
	}
	for id, perms := range g.byCompany {
		out[id] = sortedKeys(perms)
	}
	return out
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
