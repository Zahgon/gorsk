package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"golang.org/x/net/http/httpguts"

	"github.com/ribice/gorsk/pkg/utl/middleware/secure"
)

const (
	// MIMEApplicationJSONCharsetUTF8 is the content type of JSON responses
	MIMEApplicationJSONCharsetUTF8 = "application/json; charset=UTF-8"

	methodPropfind = "PROPFIND"
	defaultIndent  = "  "
	// maxHeaderBytes is the maximum size of the request line and headers.
	maxHeaderBytes = 1<<20 + 4096
)

type ctxKey int

const requestURLKey ctxKey = iota

// New instantates new Fiber server
func New(cfg *Config) *fiber.App {
	indent := ""
	if cfg.Debug {
		indent = defaultIndent
	}
	custErr := &customErrHandler{debug: cfg.Debug}
	app := fiber.New(fiber.Config{
		CaseSensitive:                true,
		StrictRouting:                true,
		ReadTimeout:                  time.Duration(cfg.ReadTimeoutSeconds) * time.Second,
		WriteTimeout:                 time.Duration(cfg.WriteTimeoutSeconds) * time.Second,
		ReadBufferSize:               maxHeaderBytes,
		ErrorHandler:                 custErr.handler,
		JSONEncoder:                  func(v interface{}) ([]byte, error) { return encodeJSON(v, indent) },
		RequestMethods:               append(append([]string{}, fiber.DefaultMethods...), methodPropfind),
		DisablePreParseMultipartForm: true,
		DisableStartupMessage:        true,
	})
	app.Use(parseRequestURL, sortHeaders, requestLogger(), recoverPanic,
		secure.CORS(), secure.Headers())
	app.Add(fiber.MethodGet, "/", healthCheck)
	return app
}

func healthCheck(c *fiber.Ctx) error {
	return JSON(c, http.StatusOK, "OK")
}

// NotFound answers the requests no route handled: 405 Method Not Allowed when the path is routed
// for other methods, 404 Not Found otherwise. It has to be registered after all routes.
func NotFound(c *fiber.Ctx) error {
	err := c.Next()
	if e, ok := err.(*fiber.Error); ok {
		switch e.Code {
		case fiber.StatusNotFound:
			return fiber.ErrNotFound
		case fiber.StatusMethodNotAllowed:
			c.Response().Header.Del(fiber.HeaderAllow)
		}
	}
	return err
}

// NotFoundHandler answers 404 Not Found
func NotFoundHandler(*fiber.Ctx) error {
	return fiber.ErrNotFound
}

// MethodNotAllowedHandler answers 405 Method Not Allowed
func MethodNotAllowedHandler(*fiber.Ctx) error {
	return fiber.ErrMethodNotAllowed
}

// JSON sends i encoded as JSON with the given status code. The encoding ends with a newline and
// is indented in debug mode or when the request has a "pretty" query parameter.
func JSON(c *fiber.Ctx, code int, i interface{}) error {
	c.Status(code)
	if _, pretty := requestURL(c).Query()["pretty"]; pretty {
		b, err := encodeJSON(i, defaultIndent)
		if err != nil {
			return err
		}
		c.Set(fiber.HeaderContentType, MIMEApplicationJSONCharsetUTF8)
		return c.Send(b)
	}
	return c.JSON(i, MIMEApplicationJSONCharsetUTF8)
}

func encodeJSON(v interface{}, indent string) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	if indent != "" {
		enc.SetIndent("", indent)
	}
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// parseRequestURL checks the request line and the Content-Length and Host headers like net/http does, answering
// net/http's plain-text error responses, and answers "OPTIONS *" itself. Responses to HTTP/1.0
// requests are HTTP/1.0 responses. Routing uses the raw path unless the decoded path escapes back
// to it, in which case the decoded path is used.
func parseRequestURL(c *fiber.Ctx) error {
	h := &c.Request().Header
	major, minor, ok := http.ParseHTTPVersion(string(h.Protocol()))
	uri := string(h.RequestURI())
	if !ok || strings.Contains(uri, " ") {
		return plainError(c, http.StatusBadRequest, "")
	}
	if major != 1 {
		return plainError(c, http.StatusHTTPVersionNotSupported, "unsupported protocol version")
	}
	u, err := url.ParseRequestURI(uri)
	if err != nil {
		return plainError(c, http.StatusBadRequest, "")
	}
	lengths := rawHeaderValues(h.RawHeaders(), fiber.HeaderContentLength)
	for _, l := range lengths {
		if !bytes.Equal(l, lengths[0]) {
			return plainError(c, http.StatusBadRequest, "")
		}
	}
	hosts := rawHeaderValues(h.RawHeaders(), fiber.HeaderHost)
	switch {
	case len(hosts) > 1:
		return plainError(c, http.StatusBadRequest, "")
	case len(hosts) == 0 && minor > 0 && c.Method() != fiber.MethodConnect:
		return plainError(c, http.StatusBadRequest, "missing required Host header")
	case len(hosts) == 1 && !httpguts.ValidHostHeader(string(hosts[0])):
		return plainError(c, http.StatusBadRequest, "malformed Host header")
	}
	if minor == 0 {
		c.Response().Header.SetProtocol(h.Protocol())
	}
	if uri == "*" && c.Method() == fiber.MethodOptions {
		c.Set(fiber.HeaderContentLength, "0")
		return nil
	}
	c.Locals(requestURLKey, u)
	if u.RawPath != "" {
		c.Path(u.RawPath)
	} else {
		c.Path(u.Path)
	}
	return c.Next()
}

// plainError answers a request the way net/http answers requests it cannot handle: the status,
// optionally followed by a detail, as status text and plain-text body, and the connection is closed.
func plainError(c *fiber.Ctx, code int, detail string) error {
	text := http.StatusText(code)
	if detail != "" {
		text += ": " + detail
	}
	c.Context().SetConnectionClose()
	c.Status(code)
	c.Response().Header.SetStatusMessage([]byte(text))
	c.Set(fiber.HeaderContentType, fiber.MIMETextPlainCharsetUTF8)
	return c.SendString(strconv.Itoa(code) + " " + text)
}

// rawHeaderValues returns the values of all request header lines with the given name.
func rawHeaderValues(rawHeaders []byte, name string) [][]byte {
	var values [][]byte
	for _, line := range bytes.Split(rawHeaders, []byte("\r\n")) {
		k, v, ok := bytes.Cut(line, []byte(":"))
		if ok && bytes.EqualFold(k, []byte(name)) {
			values = append(values, bytes.TrimSpace(v))
		}
	}
	return values
}

// requestURL returns the parsed request target.
func requestURL(c *fiber.Ctx) *url.URL {
	if u, ok := c.Locals(requestURLKey).(*url.URL); ok {
		return u
	}
	u, err := url.ParseRequestURI(string(c.Request().Header.RequestURI()))
	if err != nil {
		return &url.URL{}
	}
	return u
}

// fasthttp writes these headers itself, ahead of all others.
var serverManagedHeaders = map[string]bool{
	fiber.HeaderContentType:      true,
	fiber.HeaderContentLength:    true,
	fiber.HeaderContentEncoding:  true,
	fiber.HeaderServer:           true,
	fiber.HeaderSetCookie:        true,
	fiber.HeaderConnection:       true,
	fiber.HeaderTransferEncoding: true,
	fiber.HeaderDate:             true,
	fiber.HeaderTrailer:          true,
}

// sortHeaders orders the response headers by name, keeping the order of the values of each header.
func sortHeaders(c *fiber.Ctx) error {
	err := c.Next()
	h := &c.Response().Header
	type field struct{ key, value string }
	var fields []field
	h.VisitAll(func(k, v []byte) {
		if !serverManagedHeaders[string(k)] {
			fields = append(fields, field{string(k), string(v)})
		}
	})
	sort.SliceStable(fields, func(i, j int) bool { return fields[i].key < fields[j].key })
	for _, f := range fields {
		h.Del(f.key)
	}
	for _, f := range fields {
		h.Add(f.key, f.value)
	}
	return err
}

const logFormat = `{"time":"${time_rfc3339_nano}","id":"${id}","remote_ip":"${remote_ip}",` +
	`"host":"${host}","method":"${method}","uri":"${uri}","user_agent":"${user_agent}",` +
	`"status":${status},"error":"${error}","latency":${latency},"latency_human":"${latency_human}"` +
	`,"bytes_in":${bytes_in},"bytes_out":${bytes_out}}` + "\n"

// requestLogger logs every request to stdout as a JSON line, after the error handler answered a failed request.
func requestLogger() fiber.Handler {
	tag := func(f func(c *fiber.Ctx, data *logger.Data) string) logger.LogFunc {
		return func(output logger.Buffer, c *fiber.Ctx, data *logger.Data, _ string) (int, error) {
			return output.WriteString(f(c, data))
		}
	}
	return logger.New(logger.Config{
		Format: logFormat,
		Output: os.Stdout,
		CustomTags: map[string]logger.LogFunc{
			"time_rfc3339_nano": tag(func(*fiber.Ctx, *logger.Data) string {
				return time.Now().Format(time.RFC3339Nano)
			}),
			"id": tag(func(c *fiber.Ctx, _ *logger.Data) string {
				if id := c.Get(fiber.HeaderXRequestID); id != "" {
					return id
				}
				return c.GetRespHeader(fiber.HeaderXRequestID)
			}),
			"remote_ip": tag(func(c *fiber.Ctx, _ *logger.Data) string { return realIP(c) }),
			"host": tag(func(c *fiber.Ctx, _ *logger.Data) string {
				if u := requestURL(c); u.Host != "" {
					return u.Host
				}
				return string(c.Request().Header.Host())
			}),
			"method": tag(func(c *fiber.Ctx, _ *logger.Data) string { return c.Method() }),
			"uri": tag(func(c *fiber.Ctx, _ *logger.Data) string {
				return string(c.Request().Header.RequestURI())
			}),
			"user_agent": tag(func(c *fiber.Ctx, _ *logger.Data) string { return c.Get(fiber.HeaderUserAgent) }),
			"status": tag(func(c *fiber.Ctx, _ *logger.Data) string {
				return strconv.Itoa(c.Response().StatusCode())
			}),
			"error": tag(func(_ *fiber.Ctx, data *logger.Data) string {
				if data.ChainErr != nil {
					return errorText(data.ChainErr)
				}
				return ""
			}),
			"latency": tag(func(_ *fiber.Ctx, data *logger.Data) string {
				return strconv.FormatInt(int64(data.Stop.Sub(data.Start)), 10)
			}),
			"latency_human": tag(func(_ *fiber.Ctx, data *logger.Data) string {
				return data.Stop.Sub(data.Start).String()
			}),
			"bytes_in": tag(func(c *fiber.Ctx, _ *logger.Data) string {
				if cl := c.Get(fiber.HeaderContentLength); cl != "" {
					return cl
				}
				return "0"
			}),
			"bytes_out": tag(func(c *fiber.Ctx, _ *logger.Data) string {
				return strconv.Itoa(len(c.Response().Body()))
			}),
		},
	})
}

// realIP returns the client address: the first X-Forwarded-For entry, X-Real-IP or the peer address.
func realIP(c *fiber.Ctx) string {
	if ip := c.Get(fiber.HeaderXForwardedFor); ip != "" {
		return strings.Split(ip, ", ")[0]
	}
	if ip := c.Get("X-Real-IP"); ip != "" {
		return ip
	}
	ra, _, _ := net.SplitHostPort(c.Context().RemoteAddr().String())
	return ra
}

// Config represents server specific config
type Config struct {
	Port                string
	ReadTimeoutSeconds  int
	WriteTimeoutSeconds int
	Debug               bool
}

// Start starts Fiber server
func Start(app *fiber.App, cfg *Config) {
	// Start server
	go func() {
		lc := net.ListenConfig{KeepAlive: 3 * time.Minute}
		ln, err := lc.Listen(context.Background(), "tcp", cfg.Port)
		if err == nil {
			fmt.Printf("⇨ http server started on %s\n", ln.Addr())
			err = app.Listener(ln)
		}
		if err != nil {
			logLine("INFO", "Shutting down the server")
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server with
	// a timeout of 10 seconds.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit
	if err := app.ShutdownWithTimeout(10 * time.Second); err != nil {
		logLine("FATAL", err.Error())
		os.Exit(1)
	}
}
