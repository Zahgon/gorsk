package server

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"

	"github.com/go-playground/validator"
	"github.com/gofiber/fiber/v2"
)

const (
	mimeApplicationJSON = "application/json"
	mimeApplicationXML  = "application/xml"
	mimeTextXML         = "text/xml"
	mimeApplicationForm = "application/x-www-form-urlencoded"
	mimeMultipartForm   = "multipart/form-data"

	// multipartMemory is the maximum amount of a multipart form kept in memory.
	multipartMemory = 32 << 20
	// maxFormSize is the maximum size of an url-encoded form body.
	maxFormSize = int64(10 << 20)
)

// errUnsupportedMediaType is returned by the default binding for request bodies of unknown content type.
var errUnsupportedMediaType = fiber.NewError(http.StatusUnsupportedMediaType)

// bindError is a 400 Bad Request answered for a request that could not be bound.
// It keeps the decoding error that caused it.
type bindError struct {
	message  string
	internal error
}

func (e *bindError) Error() string {
	return e.message
}

func (e *bindError) Unwrap() error {
	return e.internal
}

func newBindError(message string, internal error) *bindError {
	return &bindError{message: message, internal: internal}
}

// NewBinder initializes custom server binder
func NewBinder() *CustomBinder {
	return &CustomBinder{v: &CustomValidator{V: validator.New()}}
}

// CustomBinder struct
type CustomBinder struct {
	v *CustomValidator
}

var defaultBinder = NewBinder()

// Bind binds the request into i with the server binder and validates it
func Bind(c *fiber.Ctx, i interface{}) error {
	return defaultBinder.Bind(i, c)
}

// Bind tries to bind request into interface, and if it does then validate it
func (cb *CustomBinder) Bind(i interface{}, c *fiber.Ctx) error {
	if err := bind(i, c); err != nil && err != errUnsupportedMediaType {
		return err
	}
	return cb.v.Validate(i)
}

// CustomValidator holds custom validator
type CustomValidator struct {
	V *validator.Validate
}

// Validate validates the request
func (cv *CustomValidator) Validate(i interface{}) error {
	return cv.V.Struct(i)
}

// bind decodes the request into i: the query string for body-less GET and DELETE requests,
// otherwise the body according to its content type.
func bind(i interface{}, c *fiber.Ctx) error {
	req := c.Request()
	if !hasBody(c) {
		if c.Method() == fiber.MethodGet || c.Method() == fiber.MethodDelete {
			if err := bindData(i, requestURL(c).Query(), "query"); err != nil {
				return newBindError(err.Error(), err)
			}
			return nil
		}
		return fiber.NewError(http.StatusBadRequest, "Request body can't be empty")
	}
	ctype := c.Get(fiber.HeaderContentType)
	switch {
	case strings.HasPrefix(ctype, mimeApplicationJSON):
		if err := json.NewDecoder(bytes.NewReader(req.Body())).Decode(i); err != nil {
			switch e := err.(type) {
			case *json.UnmarshalTypeError:
				return newBindError(fmt.Sprintf("Unmarshal type error: expected=%v, got=%v, field=%v, offset=%v", e.Type, e.Value, e.Field, e.Offset), err)
			case *json.SyntaxError:
				return newBindError(fmt.Sprintf("Syntax error: offset=%v, error=%v", e.Offset, e.Error()), err)
			default:
				return newBindError(err.Error(), err)
			}
		}
	case strings.HasPrefix(ctype, mimeApplicationXML), strings.HasPrefix(ctype, mimeTextXML):
		if err := xml.NewDecoder(bytes.NewReader(req.Body())).Decode(i); err != nil {
			switch e := err.(type) {
			case *xml.UnsupportedTypeError:
				return newBindError(fmt.Sprintf("Unsupported type error: type=%v, error=%v", e.Type, e.Error()), err)
			case *xml.SyntaxError:
				return newBindError(fmt.Sprintf("Syntax error: line=%v, error=%v", e.Line, e.Error()), err)
			default:
				return newBindError(err.Error(), err)
			}
		}
	case strings.HasPrefix(ctype, mimeApplicationForm), strings.HasPrefix(ctype, mimeMultipartForm):
		params, err := formParams(c)
		if err != nil {
			return newBindError(err.Error(), err)
		}
		if err = bindData(i, params, "form"); err != nil {
			return newBindError(err.Error(), err)
		}
	default:
		return errUnsupportedMediaType
	}
	return nil
}

// hasBody reports whether the request announced a body, by a non-zero Content-Length or by chunked
// transfer encoding, even when the chunked body turned out empty.
func hasBody(c *fiber.Ctx) bool {
	switch c.Request().Header.ContentLength() {
	case 0:
		for _, te := range rawHeaderValues(c.Request().Header.RawHeaders(), fiber.HeaderTransferEncoding) {
			if bytes.EqualFold(te, []byte("chunked")) {
				return true
			}
		}
		return false
	case -2:
		// neither Content-Length nor Transfer-Encoding
		return false
	}
	return true
}

// formParams returns the url-encoded or multipart form values of the body followed by the query string values.
func formParams(c *fiber.Ctx) (url.Values, error) {
	ctype := c.Get(fiber.HeaderContentType)
	form, err := parseForm(c)
	if !strings.HasPrefix(ctype, mimeMultipartForm) {
		return form, err
	}

	mr, mrErr := multipartReader(c)
	if mrErr != nil {
		return nil, mrErr
	}
	f, rfErr := mr.ReadForm(multipartMemory)
	if rfErr != nil {
		return nil, rfErr
	}
	defer f.RemoveAll()
	for k, v := range f.Value {
		form[k] = append(form[k], v...)
	}
	return form, err
}

// parseForm returns the url-encoded body values of POST, PUT and PATCH requests followed by the query string values.
func parseForm(c *fiber.Ctx) (url.Values, error) {
	var (
		form = make(url.Values)
		err  error
	)
	switch c.Method() {
	case fiber.MethodPost, fiber.MethodPut, fiber.MethodPatch:
		var post url.Values
		post, err = parsePostForm(c)
		for k, v := range post {
			form[k] = append(form[k], v...)
		}
	}
	query, qErr := url.ParseQuery(requestURL(c).RawQuery)
	if err == nil {
		err = qErr
	}
	for k, v := range query {
		form[k] = append(form[k], v...)
	}
	return form, err
}

func parsePostForm(c *fiber.Ctx) (url.Values, error) {
	ct := c.Get(fiber.HeaderContentType)
	// RFC 7231, section 3.1.1.5 - empty type MAY be treated as application/octet-stream
	if ct == "" {
		ct = "application/octet-stream"
	}
	ct, _, err := mime.ParseMediaType(ct)
	if ct != mimeApplicationForm {
		return nil, err
	}
	b := c.Request().Body()
	if int64(len(b)) > maxFormSize {
		return nil, errors.New("http: POST too large")
	}
	vs, e := url.ParseQuery(string(b))
	if err == nil {
		err = e
	}
	return vs, err
}

func multipartReader(c *fiber.Ctx) (*multipart.Reader, error) {
	v := c.Get(fiber.HeaderContentType)
	if v == "" {
		return nil, http.ErrNotMultipart
	}
	d, params, err := mime.ParseMediaType(v)
	if err != nil || d != mimeMultipartForm {
		return nil, http.ErrNotMultipart
	}
	boundary, ok := params["boundary"]
	if !ok {
		return nil, http.ErrMissingBoundary
	}
	return multipart.NewReader(bytes.NewReader(c.Request().Body()), boundary), nil
}

// BindUnmarshaler is the interface used to wrap the UnmarshalParam method.
type BindUnmarshaler interface {
	// UnmarshalParam decodes and assigns a value from an form or query param.
	UnmarshalParam(param string) error
}

func bindData(ptr interface{}, data map[string][]string, tag string) error {
	typ := reflect.TypeOf(ptr).Elem()
	val := reflect.ValueOf(ptr).Elem()

	if typ.Kind() != reflect.Struct {
		return errors.New("binding element must be a struct")
	}

	for i := 0; i < typ.NumField(); i++ {
		typeField := typ.Field(i)
		structField := val.Field(i)
		if !structField.CanSet() {
			continue
		}
		structFieldKind := structField.Kind()
		inputFieldName := typeField.Tag.Get(tag)

		if inputFieldName == "" {
			inputFieldName = typeField.Name
			// If tag is nil, we inspect if the field is a struct.
			if _, ok := bindUnmarshaler(structField); !ok && structFieldKind == reflect.Struct {
				if err := bindData(structField.Addr().Interface(), data, tag); err != nil {
					return err
				}
				continue
			}
		}

		inputValue, exists := data[inputFieldName]
		if !exists {
			// Go json.Unmarshal supports case insensitive binding. However the
			// url params are bound case sensitive which is inconsistent. To
			// fix this we must check all of the map values in a
			// case-insensitive search.
			inputFieldName = strings.ToLower(inputFieldName)
			for k, v := range data {
				if strings.ToLower(k) == inputFieldName {
					inputValue = v
					exists = true
					break
				}
			}
		}

		if !exists {
			continue
		}

		// Call this first, in case we're dealing with an alias to an array type
		if ok, err := unmarshalField(typeField.Type.Kind(), inputValue[0], structField); ok {
			if err != nil {
				return err
			}
			continue
		}

		numElems := len(inputValue)
		if structFieldKind == reflect.Slice && numElems > 0 {
			sliceOf := structField.Type().Elem().Kind()
			slice := reflect.MakeSlice(structField.Type(), numElems, numElems)
			for j := 0; j < numElems; j++ {
				if err := setWithProperType(sliceOf, inputValue[j], slice.Index(j)); err != nil {
					return err
				}
			}
			val.Field(i).Set(slice)
		} else if err := setWithProperType(typeField.Type.Kind(), inputValue[0], structField); err != nil {
			return err
		}
	}
	return nil
}

func setWithProperType(valueKind reflect.Kind, val string, structField reflect.Value) error {
	// But also call it here, in case we're dealing with an array of BindUnmarshalers
	if ok, err := unmarshalField(valueKind, val, structField); ok {
		return err
	}

	switch valueKind {
	case reflect.Ptr:
		return setWithProperType(structField.Elem().Kind(), val, structField.Elem())
	case reflect.Int:
		return setIntField(val, 0, structField)
	case reflect.Int8:
		return setIntField(val, 8, structField)
	case reflect.Int16:
		return setIntField(val, 16, structField)
	case reflect.Int32:
		return setIntField(val, 32, structField)
	case reflect.Int64:
		return setIntField(val, 64, structField)
	case reflect.Uint:
		return setUintField(val, 0, structField)
	case reflect.Uint8:
		return setUintField(val, 8, structField)
	case reflect.Uint16:
		return setUintField(val, 16, structField)
	case reflect.Uint32:
		return setUintField(val, 32, structField)
	case reflect.Uint64:
		return setUintField(val, 64, structField)
	case reflect.Bool:
		return setBoolField(val, structField)
	case reflect.Float32:
		return setFloatField(val, 32, structField)
	case reflect.Float64:
		return setFloatField(val, 64, structField)
	case reflect.String:
		structField.SetString(val)
	default:
		return errors.New("unknown type")
	}
	return nil
}

func unmarshalField(valueKind reflect.Kind, val string, field reflect.Value) (bool, error) {
	switch valueKind {
	case reflect.Ptr:
		return unmarshalFieldPtr(val, field)
	default:
		return unmarshalFieldNonPtr(val, field)
	}
}

// bindUnmarshaler attempts to unmarshal a reflect.Value into a BindUnmarshaler
func bindUnmarshaler(field reflect.Value) (BindUnmarshaler, bool) {
	ptr := reflect.New(field.Type())
	if ptr.CanInterface() {
		iface := ptr.Interface()
		if unmarshaler, ok := iface.(BindUnmarshaler); ok {
			return unmarshaler, ok
		}
	}
	return nil, false
}

func unmarshalFieldNonPtr(value string, field reflect.Value) (bool, error) {
	if unmarshaler, ok := bindUnmarshaler(field); ok {
		err := unmarshaler.UnmarshalParam(value)
		field.Set(reflect.ValueOf(unmarshaler).Elem())
		return true, err
	}
	return false, nil
}

func unmarshalFieldPtr(value string, field reflect.Value) (bool, error) {
	if field.IsNil() {
		// Initialize the pointer to a nil value
		field.Set(reflect.New(field.Type().Elem()))
	}
	return unmarshalFieldNonPtr(value, field.Elem())
}

func setIntField(value string, bitSize int, field reflect.Value) error {
	if value == "" {
		value = "0"
	}
	intVal, err := strconv.ParseInt(value, 10, bitSize)
	if err == nil {
		field.SetInt(intVal)
	}
	return err
}

func setUintField(value string, bitSize int, field reflect.Value) error {
	if value == "" {
		value = "0"
	}
	uintVal, err := strconv.ParseUint(value, 10, bitSize)
	if err == nil {
		field.SetUint(uintVal)
	}
	return err
}

func setBoolField(value string, field reflect.Value) error {
	if value == "" {
		value = "false"
	}
	boolVal, err := strconv.ParseBool(value)
	if err == nil {
		field.SetBool(boolVal)
	}
	return err
}

func setFloatField(value string, bitSize int, field reflect.Value) error {
	if value == "" {
		value = "0.0"
	}
	floatVal, err := strconv.ParseFloat(value, bitSize)
	if err == nil {
		field.SetFloat(floatVal)
	}
	return err
}
