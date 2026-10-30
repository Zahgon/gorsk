package mock

import (
	"github.com/gofiber/fiber/v2"

	"github.com/ribice/gorsk"
)

// RBAC Mock
type RBAC struct {
	UserFn            func(*fiber.Ctx) gorsk.AuthUser
	EnforceRoleFn     func(*fiber.Ctx, gorsk.AccessRole) error
	EnforceUserFn     func(*fiber.Ctx, int) error
	EnforceCompanyFn  func(*fiber.Ctx, int) error
	EnforceLocationFn func(*fiber.Ctx, int) error
	AccountCreateFn   func(*fiber.Ctx, gorsk.AccessRole, int, int) error
	IsLowerRoleFn     func(*fiber.Ctx, gorsk.AccessRole) error
}

// User mock
func (a RBAC) User(c *fiber.Ctx) gorsk.AuthUser {
	return a.UserFn(c)
}

// EnforceRole mock
func (a RBAC) EnforceRole(c *fiber.Ctx, role gorsk.AccessRole) error {
	return a.EnforceRoleFn(c, role)
}

// EnforceUser mock
func (a RBAC) EnforceUser(c *fiber.Ctx, id int) error {
	return a.EnforceUserFn(c, id)
}

// EnforceCompany mock
func (a RBAC) EnforceCompany(c *fiber.Ctx, id int) error {
	return a.EnforceCompanyFn(c, id)
}

// EnforceLocation mock
func (a RBAC) EnforceLocation(c *fiber.Ctx, id int) error {
	return a.EnforceLocationFn(c, id)
}

// AccountCreate mock
func (a RBAC) AccountCreate(c *fiber.Ctx, roleID gorsk.AccessRole, companyID, locationID int) error {
	return a.AccountCreateFn(c, roleID, companyID, locationID)
}

// IsLowerRole mock
func (a RBAC) IsLowerRole(c *fiber.Ctx, role gorsk.AccessRole) error {
	return a.IsLowerRoleFn(c, role)
}
