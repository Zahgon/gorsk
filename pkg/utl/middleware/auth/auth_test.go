package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dgrijalva/jwt-go"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"

	"github.com/ribice/gorsk"
	"github.com/ribice/gorsk/pkg/utl/middleware/auth"
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

type tokenParser struct {
	ParseTokenFn func(string) (*jwt.Token, error)
}

func (t tokenParser) ParseToken(s string) (*jwt.Token, error) {
	if s == "" {
		return nil, gorsk.ErrGeneric
	}
	return &jwt.Token{
		Raw:    "abcd",
		Method: jwt.SigningMethodHS256,
		Claims: jwt.MapClaims{
			"c":   1.0,
			"e":   "johndoe@mail.com",
			"exp": 1581773411,
			"id":  1.0,
			"l":   1.0,
			"r":   100.0,
			"u":   "johndoe",
		},
		Valid: true,
	}, nil
}

func TestMWFunc(t *testing.T) {
	cases := map[string]struct {
		wantStatus int
		header     string
		signMethod string
	}{
		"Empty header": {
			wantStatus: http.StatusUnauthorized,
		},
		"Success": {
			header:     "Bearer 123",
			wantStatus: http.StatusOK,
		},
	}
	app := fiberHandler(auth.Middleware(tokenParser{}))

	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/hello", nil)
			req.Header.Set("Authorization", tt.header)
			res, err := app.Test(req)
			if err != nil {
				t.Fatal("Cannot create http request")
			}
			assert.Equal(t, tt.wantStatus, res.StatusCode)
		})
	}
}
