package transport

import (
	"net/http"

	"github.com/ribice/gorsk/pkg/api/auth"
	"github.com/ribice/gorsk/pkg/utl/server"

	"github.com/gofiber/fiber/v2"
)

// HTTP represents auth http service
type HTTP struct {
	svc auth.Service
}

// NewHTTP creates new auth http service
func NewHTTP(svc auth.Service, r fiber.Router, mw fiber.Handler) {
	h := HTTP{svc}
	// swagger:route POST /login auth login
	// Logs in user by username and password.
	// responses:
	//  200: loginResp
	//  400: errMsg
	//  401: errMsg
	// 	403: err
	//  404: errMsg
	//  500: err
	r.Post("/login", h.login)
	// swagger:operation GET /refresh/{token} auth refresh
	// ---
	// summary: Refreshes jwt token.
	// description: Refreshes jwt token by checking at database whether refresh token exists.
	// parameters:
	// - name: token
	//   in: path
	//   description: refresh token
	//   type: string
	//   required: true
	// responses:
	//   "200":
	//     "$ref": "#/responses/refreshResp"
	//   "400":
	//     "$ref": "#/responses/errMsg"
	//   "401":
	//     "$ref": "#/responses/err"
	//   "500":
	//     "$ref": "#/responses/err"
	r.Add(fiber.MethodGet, "/refresh/:token", h.refresh)

	// swagger:route GET /me auth meReq
	// Gets user's info from session.
	// responses:
	//  200: userResp
	//  500: err
	r.Add(fiber.MethodGet, "/me", mw, h.me)
}

type credentials struct {
	Username string `json:"username" validate:"required"`
	Password string `json:"password" validate:"required"`
}

func (h *HTTP) login(c *fiber.Ctx) error {
	cred := new(credentials)
	if err := server.Bind(c, cred); err != nil {
		return err
	}
	r, err := h.svc.Authenticate(c, cred.Username, cred.Password)
	if err != nil {
		return err
	}
	return server.JSON(c, http.StatusOK, r)
}

func (h *HTTP) refresh(c *fiber.Ctx) error {
	token, err := h.svc.Refresh(c, c.Params("token"))
	if err != nil {
		return err
	}
	return server.JSON(c, http.StatusOK, map[string]string{
		"token": token,
	})
}

func (h *HTTP) me(c *fiber.Ctx) error {
	user, err := h.svc.Me(c)
	if err != nil {
		return err
	}
	return server.JSON(c, http.StatusOK, user)
}
