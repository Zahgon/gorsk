package server

import (
	"fmt"
	"net/http"

	"github.com/go-playground/validator"
	"github.com/gofiber/fiber/v2"
)

type customErrHandler struct {
	debug bool
}

var validationErrors = map[string]string{
	"required": " is required, but was not received",
	"min":      "'s value or length is less than allowed",
	"max":      "'s value or length is bigger than allowed",
}

func getVldErrorMsg(s string) string {
	if v, ok := validationErrors[s]; ok {
		return v
	}
	return " failed on " + s + " validation"
}

// errorText describes err the way it is reported in debug responses and request logs:
// HTTP errors as "code=<status>, message=<message>", any other error by its own text.
func errorText(err error) string {
	switch e := err.(type) {
	case *fiber.Error:
		return fmt.Sprintf("code=%d, message=%s", e.Code, e.Message)
	case *bindError:
		return fmt.Sprintf("code=%d, message=%s", http.StatusBadRequest, e.message)
	}
	return err.Error()
}

func (ce *customErrHandler) handler(c *fiber.Ctx, err error) error {
	if c.Locals(requestURLKey) == nil {
		// the server rejected the request before handling it
		code := http.StatusBadRequest
		if e, ok := err.(*fiber.Error); ok {
			code = e.Code
		}
		return plainError(c, code, "")
	}

	var (
		code = http.StatusInternalServerError
		msg  interface{}
	)

	type resp struct {
		Message interface{} `json:"message"`
	}

	if ce.debug {
		msg = errorText(err)
		switch e := err.(type) {
		case *fiber.Error:
			code = e.Code
		case *bindError, validator.ValidationErrors:
			code = http.StatusBadRequest
		}
	} else {
		switch e := err.(type) {
		case *fiber.Error:
			code = e.Code
			msg = e.Message
		case *bindError:
			code = http.StatusBadRequest
			msg = fmt.Sprintf("%v, %v", errorText(e), e.internal)
		case validator.ValidationErrors:
			var errMsg []string
			for _, v := range e {
				errMsg = append(errMsg, fmt.Sprintf("%s%s", v.Field(), getVldErrorMsg(v.ActualTag())))
			}
			msg = resp{Message: errMsg}
			code = http.StatusBadRequest
		default:
			msg = http.StatusText(code)
		}
		if _, ok := msg.(string); ok {
			msg = resp{Message: msg}
		}
	}

	// Send response
	if c.Method() == fiber.MethodHead {
		c.Status(code)
		return nil
	}
	return JSON(c, code, msg)
}
