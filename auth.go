package gorsk

import (
	"github.com/gofiber/fiber/v2"
)

// AuthToken holds authentication token details with refresh token
type AuthToken struct {
	Token        string `json:"token"`
	RefreshToken string `json:"refresh_token"`
}

// RefreshToken holds authentication token details
type RefreshToken struct {
	Token string `json:"token"`
}

// RBACService represents role-based access control service interface
type RBACService interface {
	User(*fiber.Ctx) AuthUser
	EnforceRole(*fiber.Ctx, AccessRole) error
	EnforceUser(*fiber.Ctx, int) error
	EnforceCompany(*fiber.Ctx, int) error
	EnforceLocation(*fiber.Ctx, int) error
	AccountCreate(*fiber.Ctx, AccessRole, int, int) error
	IsLowerRole(*fiber.Ctx, AccessRole) error
}
