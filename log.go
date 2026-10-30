package gorsk

import "github.com/gofiber/fiber/v2"

// Logger represents logging interface
type Logger interface {
	// source, msg, error, params
	Log(*fiber.Ctx, string, string, error, map[string]interface{})
}
