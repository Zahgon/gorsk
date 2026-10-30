package rbac

import (
	"github.com/gofiber/fiber/v2"

	"github.com/ribice/gorsk"
)

// Service is RBAC application service
type Service struct{}

func checkBool(b bool) error {
	if b {
		return nil
	}
	return fiber.ErrForbidden
}

// User returns user data stored in jwt token
func (s Service) User(c *fiber.Ctx) gorsk.AuthUser {
	id := c.Locals("id").(int)
	companyID := c.Locals("company_id").(int)
	locationID := c.Locals("location_id").(int)
	user := c.Locals("username").(string)
	email := c.Locals("email").(string)
	role := c.Locals("role").(gorsk.AccessRole)
	return gorsk.AuthUser{
		ID:         id,
		Username:   user,
		CompanyID:  companyID,
		LocationID: locationID,
		Email:      email,
		Role:       role,
	}
}

// EnforceRole authorizes request by AccessRole
func (s Service) EnforceRole(c *fiber.Ctx, r gorsk.AccessRole) error {
	return checkBool(!(c.Locals("role").(gorsk.AccessRole) > r))
}

// EnforceUser checks whether the request to change user data is done by the same user
func (s Service) EnforceUser(c *fiber.Ctx, ID int) error {
	// TODO: Implement querying db and checking the requested user's company_id/location_id
	// to allow company/location admins to view the user
	if s.isAdmin(c) {
		return nil
	}
	return checkBool(c.Locals("id").(int) == ID)
}

// EnforceCompany checks whether the request to apply change to company data
// is done by the user belonging to the that company and that the user has role CompanyAdmin.
// If user has admin role, the check for company doesnt need to pass.
func (s Service) EnforceCompany(c *fiber.Ctx, ID int) error {
	if s.isAdmin(c) {
		return nil
	}
	if err := s.EnforceRole(c, gorsk.CompanyAdminRole); err != nil {
		return err
	}
	return checkBool(c.Locals("company_id").(int) == ID)
}

// EnforceLocation checks whether the request to change location data
// is done by the user belonging to the requested location
func (s Service) EnforceLocation(c *fiber.Ctx, ID int) error {
	if s.isCompanyAdmin(c) {
		return nil
	}
	if err := s.EnforceRole(c, gorsk.LocationAdminRole); err != nil {
		return err
	}
	return checkBool(c.Locals("location_id").(int) == ID)
}

func (s Service) isAdmin(c *fiber.Ctx) bool {
	return !(c.Locals("role").(gorsk.AccessRole) > gorsk.AdminRole)
}

func (s Service) isCompanyAdmin(c *fiber.Ctx) bool {
	// Must query company ID in database for the given user
	return !(c.Locals("role").(gorsk.AccessRole) > gorsk.CompanyAdminRole)
}

// AccountCreate performs auth check when creating a new account
// Location admin cannot create accounts, needs to be fixed on EnforceLocation function
func (s Service) AccountCreate(c *fiber.Ctx, roleID gorsk.AccessRole, companyID, locationID int) error {
	if err := s.EnforceLocation(c, locationID); err != nil {
		return err
	}
	return s.IsLowerRole(c, roleID)
}

// IsLowerRole checks whether the requesting user has higher role than the user it wants to change
// Used for account creation/deletion
func (s Service) IsLowerRole(c *fiber.Ctx, r gorsk.AccessRole) error {
	return checkBool(c.Locals("role").(gorsk.AccessRole) < r)
}
