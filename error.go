package gorsk

import (
	"errors"

	"github.com/gofiber/fiber/v2"
)

var (
	// ErrGeneric is used for testing purposes and for errors handled later in the callstack
	ErrGeneric = errors.New("generic error")

	// ErrBadRequest (400) is returned for bad request (validation)
	ErrBadRequest = fiber.NewError(400)

	// ErrUnauthorized (401) is returned when user is not authorized
	ErrUnauthorized = fiber.ErrUnauthorized
)
