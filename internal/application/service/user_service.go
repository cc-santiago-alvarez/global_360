package service

import (
	"context"
	"errors"

	"global_360/internal/application/actor"
	"global_360/internal/application/dto"
	"global_360/internal/application/port"
	"global_360/internal/domain/access"
	"global_360/internal/domain/audit"
	"global_360/internal/domain/identity"
	"global_360/internal/domain/shared"
)

// UserService manages persons, users and their role assignments.
type UserService struct {
	users       port.UserRepository
	persons     port.PersonRepository
	companies   port.CompanyRepository
	countries   port.CountryRepository
	roles       port.RoleRepository
	assignments port.RoleAssignmentRepository
	tokens      port.RefreshTokenRepository
	hasher      port.PasswordHasher
	d           Deps
}

func NewUserService(
	users port.UserRepository, persons port.PersonRepository, companies port.CompanyRepository,
	countries port.CountryRepository, roles port.RoleRepository, assignments port.RoleAssignmentRepository,
	tokens port.RefreshTokenRepository, hasher port.PasswordHasher, d Deps,
) *UserService {
	return &UserService{
		users: users, persons: persons, companies: companies, countries: countries, roles: roles,
		assignments: assignments, tokens: tokens, hasher: hasher, d: d,
	}
}

// Create registers a person (or reuses an existing one) and a new user for it.
func (s *UserService) Create(ctx context.Context, cmd dto.CreateUser) (dto.User, error) {
	a, err := authorize(ctx, access.PermUserManage, cmd.CompanyID)
	if err != nil {
		return dto.User{}, err
	}
	if err := identity.ValidatePassword(cmd.Password); err != nil {
		return dto.User{}, err
	}
	if err := s.ensureCompanyAcceptsUsers(ctx, cmd.CompanyID); err != nil {
		return dto.User{}, err
	}
	country, err := s.countries.FindByID(ctx, cmd.Person.DocumentCountryID)
	if err != nil {
		return dto.User{}, refErr(err, "document_country_id")
	}
	if !country.Active {
		return dto.User{}, shared.Validation("document_country_id refers to an inactive country")
	}

	now := s.d.Clock.Now()
	newPerson, err := identity.NewPerson(s.d.IDs.NewID(), identity.PersonData(cmd.Person), now)
	if err != nil {
		return dto.User{}, err
	}
	hash, err := s.hasher.Hash(cmd.Password)
	if err != nil {
		return dto.User{}, err
	}

	var result dto.User
	err = s.d.Tx.WithinTransaction(ctx, func(ctx context.Context) error {
		p, err := s.persons.FindByDocument(ctx, newPerson.DocumentType, newPerson.DocumentNumber, newPerson.DocumentCountryID)
		switch {
		case err == nil:
			// Linking an existing person exposes its data; only global user managers may do it.
			if !a.Grants.CanGlobally(access.PermUserManage) {
				return shared.Conflict("a person with this document already exists")
			}
		case errors.Is(err, shared.ErrNotFound):
			p = newPerson
			if err := s.persons.Create(ctx, p); err != nil {
				return err
			}
			if err := s.d.Audit.Record(ctx, audit.EntityPerson, p.ID, audit.ActionCreate, nil, dto.NewPerson(p)); err != nil {
				return err
			}
		default:
			return err
		}

		u, err := identity.NewUser(s.d.IDs.NewID(), p.ID, cmd.CompanyID, cmd.Email, hash, now)
		if err != nil {
			return err
		}
		if err := s.users.Create(ctx, u); err != nil {
			return err
		}
		result = dto.NewUser(u, p)
		return s.d.Audit.Record(ctx, audit.EntityUser, u.ID, audit.ActionCreate, nil, dto.NewUser(u, nil))
	})
	return result, err
}

// Get returns a user with its person. Users can always read themselves.
func (s *UserService) Get(ctx context.Context, id string) (dto.User, error) {
	u, err := s.users.FindByID(ctx, id)
	if err != nil {
		return dto.User{}, err
	}
	if err := s.authorizeOnUser(ctx, access.PermUserRead, u, true); err != nil {
		return dto.User{}, err
	}
	p, err := s.persons.FindByID(ctx, u.PersonID)
	if err != nil {
		return dto.User{}, err
	}
	return dto.NewUser(u, p), nil
}

// List returns users visible to the actor: all with global user.read, otherwise
// only users of the companies where it holds user.read.
func (s *UserService) List(ctx context.Context, q dto.ListUsers) (dto.Page[dto.User], error) {
	a, err := authorizeAny(ctx, access.PermUserRead)
	if err != nil {
		return dto.Page[dto.User]{}, err
	}
	if q.Status != nil && !q.Status.Valid() {
		return dto.Page[dto.User]{}, shared.Validation("invalid user status %q", *q.Status)
	}
	page := q.Page.Normalize()
	filter := port.UserFilter{Status: q.Status, Page: page}
	all, ids := companyScope(a.Grants, access.PermUserRead)
	switch {
	case q.CompanyID != nil:
		if !all && !contains(ids, *q.CompanyID) {
			return dto.Page[dto.User]{}, shared.Forbidden("missing permission %s", access.PermUserRead)
		}
		filter.RestrictToCompanies, filter.CompanyIDs = true, []string{*q.CompanyID}
	case !all:
		filter.RestrictToCompanies, filter.CompanyIDs = true, ids
	}
	users, total, err := s.users.List(ctx, filter)
	if err != nil {
		return dto.Page[dto.User]{}, err
	}
	items := make([]dto.User, 0, len(users))
	for _, u := range users {
		items = append(items, dto.NewUser(u, nil))
	}
	return dto.Page[dto.User]{Items: items, Total: total, Page: page.Page, PageSize: page.PageSize}, nil
}

// Update changes login email and person contact data.
func (s *UserService) Update(ctx context.Context, id string, cmd dto.UpdateUser) (dto.User, error) {
	u, err := s.users.FindByID(ctx, id)
	if err != nil {
		return dto.User{}, err
	}
	if err := s.authorizeOnUser(ctx, access.PermUserManage, u, true); err != nil {
		return dto.User{}, err
	}
	p, err := s.persons.FindByID(ctx, u.PersonID)
	if err != nil {
		return dto.User{}, err
	}
	now := s.d.Clock.Now()
	userBefore, personBefore := dto.NewUser(u, nil), dto.NewPerson(p)

	personChanged := cmd.FirstNames != nil || cmd.LastNames != nil || cmd.PersonEmail != nil || cmd.Phone != nil
	if personChanged {
		if err := p.Update(cmd.FirstNames, cmd.LastNames, cmd.PersonEmail, cmd.Phone, now); err != nil {
			return dto.User{}, err
		}
	}
	if cmd.Email != nil {
		if err := u.ChangeEmail(*cmd.Email, now); err != nil {
			return dto.User{}, err
		}
	}
	err = s.d.Tx.WithinTransaction(ctx, func(ctx context.Context) error {
		if personChanged {
			if err := s.persons.Update(ctx, p); err != nil {
				return err
			}
			if err := s.d.Audit.Record(ctx, audit.EntityPerson, p.ID, audit.ActionUpdate, personBefore, dto.NewPerson(p)); err != nil {
				return err
			}
		}
		if cmd.Email != nil {
			if err := s.users.Update(ctx, u); err != nil {
				return err
			}
			return s.d.Audit.Record(ctx, audit.EntityUser, u.ID, audit.ActionUpdate, userBefore, dto.NewUser(u, nil))
		}
		return nil
	})
	if err != nil {
		return dto.User{}, err
	}
	return dto.NewUser(u, p), nil
}

// ChangeStatus activates, deactivates or blocks a user. Leaving the active state
// revokes every open session.
func (s *UserService) ChangeStatus(ctx context.Context, id string, status identity.UserStatus) (dto.User, error) {
	u, err := s.users.FindByID(ctx, id)
	if err != nil {
		return dto.User{}, err
	}
	if err := s.authorizeOnUser(ctx, access.PermUserManage, u, false); err != nil {
		return dto.User{}, err
	}
	if actor.FromContext(ctx).UserID == u.ID {
		return dto.User{}, shared.Validation("users cannot change their own status")
	}
	before := dto.NewUser(u, nil)
	now := s.d.Clock.Now()
	if err := u.ChangeStatus(status, now); err != nil {
		return dto.User{}, err
	}
	err = s.d.Tx.WithinTransaction(ctx, func(ctx context.Context) error {
		if err := s.users.Update(ctx, u); err != nil {
			return err
		}
		if status != identity.UserActive {
			if err := s.tokens.RevokeAllForUser(ctx, u.ID, now); err != nil {
				return err
			}
		}
		return s.d.Audit.Record(ctx, audit.EntityUser, u.ID, audit.ActionUpdate, before, dto.NewUser(u, nil))
	})
	if err != nil {
		return dto.User{}, err
	}
	return dto.NewUser(u, nil), nil
}

// ChangePassword sets a new password. Users changing their own password must
// provide the current one. All sessions are revoked afterwards.
func (s *UserService) ChangePassword(ctx context.Context, id string, cmd dto.ChangePassword) error {
	u, err := s.users.FindByID(ctx, id)
	if err != nil {
		return err
	}
	self := actor.FromContext(ctx).UserID == u.ID
	if self {
		if !s.hasher.Compare(u.PasswordHash, cmd.CurrentPassword) {
			return shared.Validation("current_password is incorrect")
		}
	} else if err := s.authorizeOnUser(ctx, access.PermUserManage, u, false); err != nil {
		return err
	}
	if err := identity.ValidatePassword(cmd.NewPassword); err != nil {
		return err
	}
	hash, err := s.hasher.Hash(cmd.NewPassword)
	if err != nil {
		return err
	}
	now := s.d.Clock.Now()
	u.ChangePasswordHash(hash, now)
	return s.d.Tx.WithinTransaction(ctx, func(ctx context.Context) error {
		if err := s.users.Update(ctx, u); err != nil {
			return err
		}
		if err := s.tokens.RevokeAllForUser(ctx, u.ID, now); err != nil {
			return err
		}
		return s.d.Audit.Record(ctx, audit.EntityUser, u.ID, audit.ActionUpdate, nil, map[string]any{"password_changed": true})
	})
}

// ListRoles returns the active role assignments of a user.
func (s *UserService) ListRoles(ctx context.Context, userID string) ([]dto.RoleAssignment, error) {
	u, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeOnUser(ctx, access.PermUserRead, u, true); err != nil {
		return nil, err
	}
	assignments, err := s.assignments.ListActiveByUser(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(assignments))
	for _, a := range assignments {
		ids = append(ids, a.RoleID)
	}
	roles, err := s.roles.FindByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	return assignmentResults(assignments, roles), nil
}

// AssignRole grants a role to a user, globally or within a company.
func (s *UserService) AssignRole(ctx context.Context, userID string, cmd dto.AssignRole) (dto.RoleAssignment, error) {
	a, err := currentActor(ctx)
	if err != nil {
		return dto.RoleAssignment{}, err
	}
	u, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return dto.RoleAssignment{}, err
	}
	role, err := s.roles.FindByID(ctx, cmd.RoleID)
	if err != nil {
		return dto.RoleAssignment{}, refErr(err, "role_id")
	}
	if err := authorizeRoleGrant(a, role, cmd.CompanyID, u); err != nil {
		return dto.RoleAssignment{}, err
	}
	if err := s.ensureCompanyAcceptsUsers(ctx, cmd.CompanyID); err != nil {
		return dto.RoleAssignment{}, err
	}
	assignment, err := access.NewRoleAssignment(s.d.IDs.NewID(), u.ID, role, cmd.CompanyID, a.UserIDPtr(), s.d.Clock.Now())
	if err != nil {
		return dto.RoleAssignment{}, err
	}
	result := dto.NewRoleAssignment(assignment, role.Name)
	err = s.d.Tx.WithinTransaction(ctx, func(ctx context.Context) error {
		if err := s.assignments.Create(ctx, assignment); err != nil {
			if errors.Is(err, shared.ErrConflict) {
				return shared.Conflict("the user already has role %s in this scope", role.Name)
			}
			return err
		}
		return s.d.Audit.Record(ctx, audit.EntityRoleAssignment, assignment.ID, audit.ActionCreate, nil, result)
	})
	return result, err
}

// RevokeRole deactivates a role assignment (it is never deleted).
func (s *UserService) RevokeRole(ctx context.Context, userID, assignmentID string) error {
	a, err := currentActor(ctx)
	if err != nil {
		return err
	}
	assignment, err := s.assignments.FindByID(ctx, assignmentID)
	if err != nil {
		return err
	}
	if assignment.UserID != userID {
		return shared.NotFound("role assignment not found")
	}
	if assignment.UserID == a.UserID {
		return shared.Validation("users cannot revoke their own roles")
	}
	u, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	role, err := s.roles.FindByID(ctx, assignment.RoleID)
	if err != nil {
		return err
	}
	if err := authorizeRoleGrant(a, role, assignment.CompanyID, u); err != nil {
		return err
	}
	before := dto.NewRoleAssignment(assignment, role.Name)
	if err := assignment.Revoke(a.UserIDPtr(), s.d.Clock.Now()); err != nil {
		return err
	}
	return s.d.Tx.WithinTransaction(ctx, func(ctx context.Context) error {
		if err := s.assignments.Update(ctx, assignment); err != nil {
			return err
		}
		return s.d.Audit.Record(ctx, audit.EntityRoleAssignment, assignment.ID, audit.ActionUpdate, before, dto.NewRoleAssignment(assignment, role.Name))
	})
}

// authorizeOnUser checks code on the target user's company (or globally for internal users).
func (s *UserService) authorizeOnUser(ctx context.Context, code string, u *identity.User, allowSelf bool) error {
	a, err := currentActor(ctx)
	if err != nil {
		return err
	}
	if allowSelf && a.UserID == u.ID {
		return nil
	}
	if !a.Grants.Can(code, u.CompanyID) {
		return shared.Forbidden("missing permission %s", code)
	}
	return nil
}

func (s *UserService) ensureCompanyAcceptsUsers(ctx context.Context, companyID *string) error {
	if companyID == nil {
		return nil
	}
	c, err := s.companies.FindByID(ctx, *companyID)
	if err != nil {
		return refErr(err, "company_id")
	}
	if !c.AcceptsUsers() {
		return shared.Validation("company %s is inactive", c.ID)
	}
	return nil
}

// authorizeRoleGrant decides whether actor may assign or revoke role in companyID for user u.
//   - Global role.manage may grant any role anywhere.
//   - Otherwise only company roles can be granted, within a company where the actor
//     holds user.manage, to users of that same company, and only when the actor
//     itself holds every permission of the role there (no privilege escalation).
func authorizeRoleGrant(a actor.Actor, role *access.Role, companyID *string, u *identity.User) error {
	if a.Grants.CanGlobally(access.PermRoleManage) {
		return nil
	}
	forbidden := shared.Forbidden("not allowed to manage role %s in this scope", role.Name)
	if role.Scope != access.ScopeCompany || companyID == nil {
		return forbidden
	}
	if !a.Grants.Can(access.PermUserManage, companyID) {
		return forbidden
	}
	if u.CompanyID == nil || *u.CompanyID != *companyID {
		return forbidden
	}
	for _, p := range role.Permissions {
		if !a.Grants.Can(p, companyID) {
			return forbidden
		}
	}
	return nil
}

func assignmentResults(assignments []*access.RoleAssignment, roles []*access.Role) []dto.RoleAssignment {
	names := make(map[string]string, len(roles))
	for _, r := range roles {
		names[r.ID] = r.Name
	}
	out := make([]dto.RoleAssignment, 0, len(assignments))
	for _, a := range assignments {
		out = append(out, dto.NewRoleAssignment(a, names[a.RoleID]))
	}
	return out
}

func contains(values []string, v string) bool {
	for _, x := range values {
		if x == v {
			return true
		}
	}
	return false
}
