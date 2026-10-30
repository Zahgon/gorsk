package auth

import (
	"net/http"

	"github.com/dgrijalva/jwt-go"
	"github.com/gofiber/fiber/v2"

	"github.com/ribice/gorsk"
)

// TokenParser represents JWT token parser
type TokenParser interface {
	ParseToken(string) (*jwt.Token, error)
}

// Middleware makes JWT implement the Middleware interface.
func Middleware(tokenParser TokenParser) fiber.Handler {
	return func(c *fiber.Ctx) error {
		token, err := tokenParser.ParseToken(c.Get(fiber.HeaderAuthorization))
		if err != nil || !token.Valid {
			c.Status(http.StatusUnauthorized)
			return nil
		}

		claims := token.Claims.(jwt.MapClaims)

		id := int(claims["id"].(float64))
		companyID := int(claims["c"].(float64))
		locationID := int(claims["l"].(float64))
		username := claims["u"].(string)
		email := claims["e"].(string)
		role := gorsk.AccessRole(claims["r"].(float64))

		c.Locals("id", id)
		c.Locals("company_id", companyID)
		c.Locals("location_id", locationID)
		c.Locals("username", username)
		c.Locals("email", email)
		c.Locals("role", role)

		return c.Next()
	}
}
