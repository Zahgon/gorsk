package secure_test

import (
	"net/http/httptest"
	"testing"

	"github.com/ribice/gorsk/pkg/utl/middleware/secure"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
)

func fiberHandler(mw ...fiber.Handler) *fiber.App {
	app := fiber.New()
	for _, v := range mw {
		app.Use(v)
	}
	app.Get("/hello", hwHandler)
	return app
}

func hwHandler(c *fiber.Ctx) error {
	return c.Status(200).SendString("Hello World")
}

func TestSecureHeaders(t *testing.T) {
	app := fiberHandler(secure.Headers())
	resp, err := app.Test(httptest.NewRequest("GET", "/hello", nil))
	if err != nil {
		t.Fatal("Did not expect http.Get to fail")
	}
	assert.Equal(t, "nosniff", resp.Header.Get("X-Content-Type-Options"))
	assert.Equal(t, "off", resp.Header.Get("X-DNS-Prefetch-Control"))
	assert.Equal(t, "DENY", resp.Header.Get("X-Frame-Options"))
	assert.Equal(t, "max-age=5184000; includeSubDomains", resp.Header.Get("Strict-Transport-Security"))
	assert.Equal(t, "noopen", resp.Header.Get("X-Download-Options"))
	assert.Equal(t, "1; mode=block", resp.Header.Get("X-XSS-Protection"))
}

func TestCORS(t *testing.T) {
	app := fiberHandler(secure.CORS())
	req := httptest.NewRequest("OPTIONS", "/hello", nil)
	resp, _ := app.Test(req)
	assert.Equal(t, "86400", resp.Header.Get("Access-Control-Max-Age"))
	assert.Equal(t, "POST,GET,PUT,DELETE,PATCH,HEAD", resp.Header.Get("Access-Control-Allow-Methods"))
	assert.Equal(t, "*", resp.Header.Get("Access-Control-Allow-Headers"))
	assert.Equal(t, "true", resp.Header.Get("Access-Control-Allow-Credentials"))
	// assert.Equal(t, "Content-Length", resp.Header.Get("Access-Control-Expose-Headers"))
	// assert.Equal(t, "*", resp.Header.Get("Access-Control-Allow-Origin"))
}
