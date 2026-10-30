package zlog

import (
	"fmt"
	"os"

	"github.com/gofiber/fiber/v2"

	"github.com/rs/zerolog"
)

// Log represents zerolog logger
type Log struct {
	logger *zerolog.Logger
}

// New instantiates new zero logger
func New() *Log {
	z := zerolog.New(os.Stdout)
	return &Log{
		logger: &z,
	}
}

// Log logs using zerolog
func (z *Log) Log(ctx *fiber.Ctx, source, msg string, err error, params map[string]interface{}) {

	if params == nil {
		params = make(map[string]interface{})
	}

	params["source"] = source

	if id, ok := ctx.Locals("id").(int); ok {
		params["id"] = id
		params["user"] = ctx.Locals("username").(string)
	}

	if err != nil {
		params["error"] = err
		if e, ok := err.(*fiber.Error); ok {
			params["error"] = fmt.Sprintf("code=%d, message=%s", e.Code, e.Message)
		}
		z.logger.Error().Fields(params).Msg(msg)
		return
	}

	z.logger.Info().Fields(params).Msg(msg)
}
