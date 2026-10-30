package secure

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// Headers adds general security headers for basic security measures
func Headers() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Protects from MimeType Sniffing
		c.Set("X-Content-Type-Options", "nosniff")
		// Prevents browser from prefetching DNS
		c.Set("X-DNS-Prefetch-Control", "off")
		// Denies website content to be served in an iframe
		c.Set("X-Frame-Options", "DENY")
		c.Set("Strict-Transport-Security", "max-age=5184000; includeSubDomains")
		// Prevents Internet Explorer from executing downloads in site's context
		c.Set("X-Download-Options", "noopen")
		// Minimal XSS protection
		c.Set("X-XSS-Protection", "1; mode=block")
		return c.Next()
	}
}

// CORSConfig defines the config for CORS middleware
type CORSConfig struct {
	AllowOrigins     []string
	AllowMethods     []string
	AllowHeaders     []string
	ExposeHeaders    []string
	AllowCredentials bool
	MaxAge           int
}

// CORS adds Cross-Origin Resource Sharing support
func CORS() fiber.Handler {
	return CORSWithConfig(CORSConfig{
		AllowOrigins:     []string{"*"},
		MaxAge:           86400,
		AllowMethods:     []string{"POST", "GET", "PUT", "DELETE", "PATCH", "HEAD"},
		AllowHeaders:     []string{"*"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
	})
}

// CORSWithConfig returns a CORS middleware for the given config.
// Every OPTIONS request is answered as a preflight request with 204 No Content.
func CORSWithConfig(config CORSConfig) fiber.Handler {
	allowMethods := strings.Join(config.AllowMethods, ",")
	allowHeaders := strings.Join(config.AllowHeaders, ",")
	exposeHeaders := strings.Join(config.ExposeHeaders, ",")
	maxAge := strconv.Itoa(config.MaxAge)

	return func(c *fiber.Ctx) error {
		h := &c.Response().Header
		origin := c.Get(fiber.HeaderOrigin)
		allowOrigin := ""

		// Check allowed origins
		for _, o := range config.AllowOrigins {
			if o == "*" && config.AllowCredentials {
				allowOrigin = origin
				break
			}
			if o == "*" || o == origin {
				allowOrigin = o
				break
			}
		}

		// Simple request
		if c.Method() != fiber.MethodOptions {
			h.Add(fiber.HeaderVary, fiber.HeaderOrigin)
			h.Set(fiber.HeaderAccessControlAllowOrigin, allowOrigin)
			if config.AllowCredentials {
				h.Set(fiber.HeaderAccessControlAllowCredentials, "true")
			}
			if exposeHeaders != "" {
				h.Set(fiber.HeaderAccessControlExposeHeaders, exposeHeaders)
			}
			return c.Next()
		}

		// Preflight request
		h.Add(fiber.HeaderVary, fiber.HeaderOrigin)
		h.Add(fiber.HeaderVary, fiber.HeaderAccessControlRequestMethod)
		h.Add(fiber.HeaderVary, fiber.HeaderAccessControlRequestHeaders)
		h.Set(fiber.HeaderAccessControlAllowOrigin, allowOrigin)
		h.Set(fiber.HeaderAccessControlAllowMethods, allowMethods)
		if config.AllowCredentials {
			h.Set(fiber.HeaderAccessControlAllowCredentials, "true")
		}
		if allowHeaders != "" {
			h.Set(fiber.HeaderAccessControlAllowHeaders, allowHeaders)
		} else if rh := c.Get(fiber.HeaderAccessControlRequestHeaders); rh != "" {
			h.Set(fiber.HeaderAccessControlAllowHeaders, rh)
		}
		if config.MaxAge > 0 {
			h.Set(fiber.HeaderAccessControlMaxAge, maxAge)
		}
		c.Status(http.StatusNoContent)
		return nil
	}
}
