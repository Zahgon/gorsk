package transport_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/ribice/gorsk"
	"github.com/ribice/gorsk/pkg/api/auth"
	"github.com/ribice/gorsk/pkg/api/auth/transport"
	"github.com/ribice/gorsk/pkg/utl/jwt"
	authMw "github.com/ribice/gorsk/pkg/utl/middleware/auth"
	"github.com/ribice/gorsk/pkg/utl/mock"
	"github.com/ribice/gorsk/pkg/utl/mock/mockdb"
	"github.com/ribice/gorsk/pkg/utl/server"

	"github.com/go-pg/pg/v9/orm"
	"github.com/stretchr/testify/assert"
)

func TestLogin(t *testing.T) {
	cases := []struct {
		name       string
		req        string
		wantStatus int
		wantResp   *gorsk.AuthToken
		udb        *mockdb.User
		jwt        *mock.JWT
		sec        *mock.Secure
	}{
		{
			name:       "Invalid request",
			req:        `{"username":"juzernejm"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Fail on FindByUsername",
			req:        `{"username":"juzernejm","password":"hunter123"}`,
			wantStatus: http.StatusInternalServerError,
			udb: &mockdb.User{
				FindByUsernameFn: func(orm.DB, string) (gorsk.User, error) {
					return gorsk.User{}, gorsk.ErrGeneric
				},
			},
		},
		{
			name:       "Success",
			req:        `{"username":"juzernejm","password":"hunter123"}`,
			wantStatus: http.StatusOK,
			udb: &mockdb.User{
				FindByUsernameFn: func(orm.DB, string) (gorsk.User, error) {
					return gorsk.User{
						Password: "hunter123",
						Active:   true,
					}, nil
				},
				UpdateFn: func(db orm.DB, u gorsk.User) error {
					return nil
				},
			},
			jwt: &mock.JWT{
				GenerateTokenFn: func(gorsk.User) (string, error) {
					return "jwttokenstring", nil
				},
			},
			sec: &mock.Secure{
				HashMatchesPasswordFn: func(string, string) bool {
					return true
				},
				TokenFn: func(string) string {
					return "refreshtoken"
				},
			},
			wantResp: &gorsk.AuthToken{Token: "jwttokenstring", RefreshToken: "refreshtoken"},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			r := server.New(&server.Config{})
			transport.NewHTTP(auth.New(nil, tt.udb, tt.jwt, tt.sec, nil), r, nil)
			path := "/login"
			req := httptest.NewRequest("POST", path, bytes.NewBufferString(tt.req))
			req.Header.Set("Content-Type", "application/json")
			res, err := r.Test(req, -1)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			if tt.wantResp != nil {
				response := new(gorsk.AuthToken)
				if err := json.NewDecoder(res.Body).Decode(response); err != nil {
					t.Fatal(err)
				}
				tt.wantResp.RefreshToken = response.RefreshToken
				assert.Equal(t, tt.wantResp, response)
			}
			assert.Equal(t, tt.wantStatus, res.StatusCode)
		})
	}
}

func TestRefresh(t *testing.T) {
	cases := []struct {
		name       string
		req        string
		wantStatus int
		wantResp   *gorsk.RefreshToken
		udb        *mockdb.User
		jwt        *mock.JWT
	}{
		{
			name:       "Fail on FindByToken",
			req:        "refreshtoken",
			wantStatus: http.StatusInternalServerError,
			udb: &mockdb.User{
				FindByTokenFn: func(orm.DB, string) (gorsk.User, error) {
					return gorsk.User{}, gorsk.ErrGeneric
				},
			},
		},
		{
			name:       "Success",
			req:        "refreshtoken",
			wantStatus: http.StatusOK,
			udb: &mockdb.User{
				FindByTokenFn: func(orm.DB, string) (gorsk.User, error) {
					return gorsk.User{
						Username: "johndoe",
						Active:   true,
					}, nil
				},
			},
			jwt: &mock.JWT{
				GenerateTokenFn: func(gorsk.User) (string, error) {
					return "jwttokenstring", nil
				},
			},
			wantResp: &gorsk.RefreshToken{Token: "jwttokenstring"},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			r := server.New(&server.Config{})
			transport.NewHTTP(auth.New(nil, tt.udb, tt.jwt, nil, nil), r, nil)
			path := "/refresh/" + tt.req
			res, err := r.Test(httptest.NewRequest("GET", path, nil), -1)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			if tt.wantResp != nil {
				response := new(gorsk.RefreshToken)
				if err := json.NewDecoder(res.Body).Decode(response); err != nil {
					t.Fatal(err)
				}
				assert.Equal(t, tt.wantResp, response)
			}
			assert.Equal(t, tt.wantStatus, res.StatusCode)
		})
	}
}

func TestMe(t *testing.T) {
	cases := []struct {
		name       string
		wantStatus int
		wantResp   gorsk.User
		header     string
		udb        *mockdb.User
		rbac       *mock.RBAC
	}{
		{
			name:       "Fail on user view",
			wantStatus: http.StatusInternalServerError,
			udb: &mockdb.User{
				ViewFn: func(orm.DB, int) (gorsk.User, error) {
					return gorsk.User{}, gorsk.ErrGeneric
				},
			},
			rbac: &mock.RBAC{
				UserFn: func(*fiber.Ctx) gorsk.AuthUser {
					return gorsk.AuthUser{ID: 1}
				},
			},
			header: mock.HeaderValid(),
		},
		{
			name:       "Success",
			wantStatus: http.StatusOK,
			udb: &mockdb.User{
				ViewFn: func(db orm.DB, i int) (gorsk.User, error) {
					return gorsk.User{
						Base: gorsk.Base{
							ID: i,
						},
						CompanyID:  2,
						LocationID: 3,
						Email:      "john@mail.com",
						FirstName:  "John",
						LastName:   "Doe",
					}, nil
				},
			},
			rbac: &mock.RBAC{
				UserFn: func(*fiber.Ctx) gorsk.AuthUser {
					return gorsk.AuthUser{ID: 1}
				},
			},
			header: mock.HeaderValid(),
			wantResp: gorsk.User{
				Base: gorsk.Base{
					ID: 1,
				},
				CompanyID:  2,
				LocationID: 3,
				Email:      "john@mail.com",
				FirstName:  "John",
				LastName:   "Doe",
			},
		},
	}

	jwt, err := jwt.New("HS256", "jwtsecret123", 60, 4)
	if err != nil {
		t.Fatal(err)
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			r := server.New(&server.Config{})
			transport.NewHTTP(auth.New(nil, tt.udb, nil, nil, tt.rbac), r, authMw.Middleware(jwt))
			path := "/me"
			req := httptest.NewRequest("GET", path, nil)
			req.Header.Set("Authorization", tt.header)
			res, err := r.Test(req, -1)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			if tt.wantResp.ID != 0 {
				var response gorsk.User
				if err := json.NewDecoder(res.Body).Decode(&response); err != nil {
					t.Fatal(err)
				}
				assert.Equal(t, tt.wantResp, response)
			}
			assert.Equal(t, tt.wantStatus, res.StatusCode)
		})
	}
}
