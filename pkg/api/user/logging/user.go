package user

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/ribice/gorsk"
	"github.com/ribice/gorsk/pkg/api/user"
)

// New creates new user logging service
func New(svc user.Service, logger gorsk.Logger) *LogService {
	return &LogService{
		Service: svc,
		logger:  logger,
	}
}

// LogService represents user logging service
type LogService struct {
	user.Service
	logger gorsk.Logger
}

const name = "user"

// Create logging
func (ls *LogService) Create(c *fiber.Ctx, req gorsk.User) (resp gorsk.User, err error) {
	defer func(begin time.Time) {
		req.Password = "xxx-redacted-xxx"
		ls.logger.Log(
			c,
			name, "Create user request", err,
			map[string]interface{}{
				"req":  req,
				"resp": resp,
				"took": time.Since(begin),
			},
		)
	}(time.Now())
	return ls.Service.Create(c, req)
}

// List logging
func (ls *LogService) List(c *fiber.Ctx, req gorsk.Pagination) (resp []gorsk.User, err error) {
	defer func(begin time.Time) {
		ls.logger.Log(
			c,
			name, "List user request", err,
			map[string]interface{}{
				"req":  req,
				"resp": resp,
				"took": time.Since(begin),
			},
		)
	}(time.Now())
	return ls.Service.List(c, req)
}

// View logging
func (ls *LogService) View(c *fiber.Ctx, req int) (resp gorsk.User, err error) {
	defer func(begin time.Time) {
		ls.logger.Log(
			c,
			name, "View user request", err,
			map[string]interface{}{
				"req":  req,
				"resp": resp,
				"took": time.Since(begin),
			},
		)
	}(time.Now())
	return ls.Service.View(c, req)
}

// Delete logging
func (ls *LogService) Delete(c *fiber.Ctx, req int) (err error) {
	defer func(begin time.Time) {
		ls.logger.Log(
			c,
			name, "Delete user request", err,
			map[string]interface{}{
				"req":  req,
				"took": time.Since(begin),
			},
		)
	}(time.Now())
	return ls.Service.Delete(c, req)
}

// Update logging
func (ls *LogService) Update(c *fiber.Ctx, req user.Update) (resp gorsk.User, err error) {
	defer func(begin time.Time) {
		ls.logger.Log(
			c,
			name, "Update user request", err,
			map[string]interface{}{
				"req":  req,
				"resp": resp,
				"took": time.Since(begin),
			},
		)
	}(time.Now())
	return ls.Service.Update(c, req)
}
