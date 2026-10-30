package server_test

import (
	"bytes"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/ribice/gorsk/pkg/utl/server"
	"github.com/stretchr/testify/assert"
)

type Req struct {
	Name string `json:"name" validate:"required"`
}

func TestBind(t *testing.T) {
	cases := []struct {
		name     string
		req      string
		wantErr  bool
		wantData *Req
	}{
		{
			name:     "Fail on binding",
			wantErr:  true,
			req:      `"bleja"`,
			wantData: &Req{Name: ""},
		},
		{
			name:     "Fail on validation",
			wantErr:  true,
			wantData: &Req{Name: ""},
		},
		{
			name:     "Success",
			req:      `{"name":"John"}`,
			wantData: &Req{Name: "John"},
		},
	}
	b := server.NewBinder()
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/", bytes.NewBufferString(tt.req))
			req.Header.Set("Content-Type", "application/json")
			app := fiber.New()
			app.Post("/", func(c *fiber.Ctx) error {
				r := new(Req)
				err := b.Bind(r, c)
				assert.Equal(t, tt.wantData, r)
				assert.Equal(t, tt.wantErr, err != nil)
				return nil
			})
			res, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, fiber.StatusOK, res.StatusCode)
		})
	}

}
