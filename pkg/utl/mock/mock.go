package mock

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"
)

// TestTime is used for testing time fields
func TestTime(year int) time.Time {
	return time.Date(year, time.May, 19, 1, 2, 3, 4, time.UTC)
}

// HeaderValid is used for jwt testing
func HeaderValid() string {
	return "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpZCI6MSwidSI6ImpvaG5kb2UiLCJlIjoiam9obmRvZUBtYWlsLmNvbSIsInIiOjEsImMiOjEsImwiOjEsImV4cCI6NDEwOTMyMDg5NCwiaWF0IjoxNTE2MjM5MDIyfQ.FZYfTeKNulzfMM6IMSOHJKokgFACtL_e-MW7HOLLm1k"
}

// FiberCtxWithKeys returns new Fiber context with keys
func FiberCtxWithKeys(keys []string, values ...interface{}) *fiber.Ctx {
	app := fiber.New()
	c := app.AcquireCtx(&fasthttp.RequestCtx{})
	for i, k := range keys {
		c.Locals(k, values[i])
	}
	return c
}
