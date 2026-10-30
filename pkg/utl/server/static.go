package server

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

// The conditional and range request handling below follows net/http's ServeContent
// (Copyright 2009 The Go Authors, BSD-style license).

const indexPage = "index.html"

var (
	errNoOverlap  = errors.New("invalid range: failed to overlap")
	unixEpochTime = time.Unix(0, 0)
)

// Static serves the files of the root directory at prefix and below it. Directories are answered with their index page.
func Static(r fiber.Router, prefix, root string) {
	if root == "" {
		root = "." // For security we want to restrict to CWD.
	}
	h := func(c *fiber.Ctx) error {
		p, err := url.PathUnescape(c.Params("*"))
		if err != nil {
			return err
		}
		name := filepath.Join(root, path.Clean("/"+p)) // "/"+ for security
		return serveFile(c, name)
	}
	r.Add(fiber.MethodGet, prefix, h)
	if prefix == "/" {
		r.Add(fiber.MethodGet, prefix+"*", h)
		return
	}
	r.Add(fiber.MethodGet, prefix+"/*", h)
}

func serveFile(c *fiber.Ctx, file string) error {
	f, err := os.Open(file)
	if err != nil {
		return fiber.ErrNotFound
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return err
	}
	if fi.IsDir() {
		file = filepath.Join(file, indexPage)
		f, err = os.Open(file)
		if err != nil {
			return fiber.ErrNotFound
		}
		defer f.Close()
		if fi, err = f.Stat(); err != nil {
			return err
		}
	}
	return serveContent(c, fi.Name(), fi.ModTime(), fi.Size(), f)
}

func serveContent(c *fiber.Ctx, name string, modtime time.Time, size int64, content io.ReadSeeker) error {
	h := &c.Response().Header
	if !isZeroTime(modtime) {
		h.Set(fiber.HeaderLastModified, modtime.UTC().Format(http.TimeFormat))
	}
	done, rangeReq := checkPreconditions(c, modtime)
	if done {
		return nil
	}

	code := http.StatusOK

	ctype := mime.TypeByExtension(filepath.Ext(name))
	if ctype == "" {
		// read a chunk to decide between utf-8 text and binary
		var buf [512]byte
		n, _ := io.ReadFull(content, buf[:])
		ctype = http.DetectContentType(buf[:n])
		if _, err := content.Seek(0, io.SeekStart); err != nil { // rewind to output whole file
			return serveError(c, "seeker can't seek", http.StatusInternalServerError)
		}
	}
	h.SetContentType(ctype)

	// handle Content-Range header.
	ranges, err := parseRange(rangeReq, size)
	switch err {
	case nil:
	case errNoOverlap:
		if size == 0 {
			// Some clients add a Range header to all requests to
			// limit the size of the response. If the file is empty,
			// ignore the range header and respond with a 200 rather
			// than a 416.
			ranges = nil
			break
		}
		h.Set(fiber.HeaderContentRange, fmt.Sprintf("bytes */%d", size))
		fallthrough
	default:
		return serveError(c, err.Error(), http.StatusRequestedRangeNotSatisfiable)
	}

	if sumRangesSize(ranges) > size {
		// The total number of bytes in all the ranges is larger than the size of the file
		// by itself, so this is probably an attack, or a dumb client. Ignore the range request.
		ranges = nil
	}

	var body []byte
	switch {
	case len(ranges) == 1:
		ra := ranges[0]
		if _, err := content.Seek(ra.start, io.SeekStart); err != nil {
			return serveError(c, err.Error(), http.StatusRequestedRangeNotSatisfiable)
		}
		code = http.StatusPartialContent
		h.Set(fiber.HeaderContentRange, ra.contentRange(size))
		if body, err = readN(content, ra.length); err != nil {
			return err
		}
	case len(ranges) > 1:
		code = http.StatusPartialContent
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		h.SetContentType("multipart/byteranges; boundary=" + mw.Boundary())
		for _, ra := range ranges {
			part, err := mw.CreatePart(ra.mimeHeader(ctype, size))
			if err != nil {
				return err
			}
			if _, err := content.Seek(ra.start, io.SeekStart); err != nil {
				return err
			}
			if _, err := io.CopyN(part, content, ra.length); err != nil {
				return err
			}
		}
		if err := mw.Close(); err != nil {
			return err
		}
		body = buf.Bytes()
	default:
		if body, err = readN(content, size); err != nil {
			return err
		}
	}

	h.Set(fiber.HeaderAcceptRanges, "bytes")
	c.Status(code)
	if c.Method() == fiber.MethodHead {
		h.SetContentLength(len(body))
		return nil
	}
	return c.Send(body)
}

func readN(r io.Reader, n int64) ([]byte, error) {
	b := make([]byte, n)
	_, err := io.ReadFull(r, b)
	return b, err
}

// serveError answers a plain text error like net/http's Error.
func serveError(c *fiber.Ctx, text string, code int) error {
	h := &c.Response().Header
	h.Del(fiber.HeaderLastModified)
	h.Set(fiber.HeaderContentType, fiber.MIMETextPlainCharsetUTF8)
	h.Set(fiber.HeaderXContentTypeOptions, "nosniff")
	c.Status(code)
	return c.SendString(text + "\n")
}

// condResult is the result of an HTTP request precondition check.
// See https://tools.ietf.org/html/rfc7232 section 3.
type condResult int

const (
	condNone condResult = iota
	condTrue
	condFalse
)

// checkPreconditions evaluates request preconditions and reports whether a precondition
// resulted in sending StatusNotModified or StatusPreconditionFailed.
func checkPreconditions(c *fiber.Ctx, modtime time.Time) (done bool, rangeHeader string) {
	// This function carefully follows RFC 7232 section 6.
	ch := checkIfMatch(c)
	if ch == condNone {
		ch = checkIfUnmodifiedSince(c, modtime)
	}
	if ch == condFalse {
		c.Status(http.StatusPreconditionFailed)
		return true, ""
	}
	switch checkIfNoneMatch(c) {
	case condFalse:
		if c.Method() == fiber.MethodGet || c.Method() == fiber.MethodHead {
			writeNotModified(c)
			return true, ""
		}
		c.Status(http.StatusPreconditionFailed)
		return true, ""
	case condNone:
		if checkIfModifiedSince(c, modtime) == condFalse {
			writeNotModified(c)
			return true, ""
		}
	}

	rangeHeader = c.Get(fiber.HeaderRange)
	if rangeHeader != "" && checkIfRange(c, modtime) == condFalse {
		rangeHeader = ""
	}
	return false, rangeHeader
}

func checkIfMatch(c *fiber.Ctx) condResult {
	im := c.Get(fiber.HeaderIfMatch)
	if im == "" {
		return condNone
	}
	for {
		im = textproto.TrimString(im)
		if len(im) == 0 {
			break
		}
		if im[0] == ',' {
			im = im[1:]
			continue
		}
		if im[0] == '*' {
			return condTrue
		}
		etag, remain := scanETag(im)
		if etag == "" {
			break
		}
		if etagStrongMatch(etag, c.GetRespHeader(fiber.HeaderETag)) {
			return condTrue
		}
		im = remain
	}

	return condFalse
}

func checkIfUnmodifiedSince(c *fiber.Ctx, modtime time.Time) condResult {
	ius := c.Get(fiber.HeaderIfUnmodifiedSince)
	if ius == "" || isZeroTime(modtime) {
		return condNone
	}
	t, err := http.ParseTime(ius)
	if err != nil {
		return condNone
	}

	// The Last-Modified header truncates sub-second precision so
	// the modtime needs to be truncated too.
	modtime = modtime.Truncate(time.Second)
	if ret := modtime.Compare(t); ret <= 0 {
		return condTrue
	}
	return condFalse
}

func checkIfNoneMatch(c *fiber.Ctx) condResult {
	inm := c.Get(fiber.HeaderIfNoneMatch)
	if inm == "" {
		return condNone
	}
	buf := inm
	for {
		buf = textproto.TrimString(buf)
		if len(buf) == 0 {
			break
		}
		if buf[0] == ',' {
			buf = buf[1:]
			continue
		}
		if buf[0] == '*' {
			return condFalse
		}
		etag, remain := scanETag(buf)
		if etag == "" {
			break
		}
		if etagWeakMatch(etag, c.GetRespHeader(fiber.HeaderETag)) {
			return condFalse
		}
		buf = remain
	}
	return condTrue
}

func checkIfModifiedSince(c *fiber.Ctx, modtime time.Time) condResult {
	if c.Method() != fiber.MethodGet && c.Method() != fiber.MethodHead {
		return condNone
	}
	ims := c.Get(fiber.HeaderIfModifiedSince)
	if ims == "" || isZeroTime(modtime) {
		return condNone
	}
	t, err := http.ParseTime(ims)
	if err != nil {
		return condNone
	}
	// The Last-Modified header truncates sub-second precision so
	// the modtime needs to be truncated too.
	modtime = modtime.Truncate(time.Second)
	if ret := modtime.Compare(t); ret <= 0 {
		return condFalse
	}
	return condTrue
}

func checkIfRange(c *fiber.Ctx, modtime time.Time) condResult {
	if c.Method() != fiber.MethodGet && c.Method() != fiber.MethodHead {
		return condNone
	}
	ir := c.Get(fiber.HeaderIfRange)
	if ir == "" {
		return condNone
	}
	etag, _ := scanETag(ir)
	if etag != "" {
		if etagStrongMatch(etag, c.GetRespHeader(fiber.HeaderETag)) {
			return condTrue
		}
		return condFalse
	}
	// The If-Range value is typically the ETag value, but it may also be
	// the modtime date. See golang.org/issue/8367.
	if modtime.IsZero() {
		return condFalse
	}
	t, err := http.ParseTime(ir)
	if err != nil {
		return condFalse
	}
	if t.Unix() == modtime.Unix() {
		return condTrue
	}
	return condFalse
}

func writeNotModified(c *fiber.Ctx) {
	// RFC 7232 section 4.1:
	// a sender SHOULD NOT generate representation metadata other than the
	// above listed fields unless said metadata exists for the purpose of
	// guiding cache updates (e.g., Last-Modified might be useful if the
	// response does not have an ETag field).
	h := &c.Response().Header
	h.Del(fiber.HeaderContentType)
	h.Del(fiber.HeaderContentLength)
	h.Del(fiber.HeaderContentEncoding)
	if c.GetRespHeader(fiber.HeaderETag) != "" {
		h.Del(fiber.HeaderLastModified)
	}
	c.Status(http.StatusNotModified)
}

func isZeroTime(t time.Time) bool {
	return t.IsZero() || t.Equal(unixEpochTime)
}

// scanETag determines if a syntactically valid ETag is present at s. If so,
// the ETag and remaining text after consuming ETag is returned. Otherwise,
// it returns "", "".
func scanETag(s string) (etag string, remain string) {
	s = textproto.TrimString(s)
	start := 0
	if strings.HasPrefix(s, "W/") {
		start = 2
	}
	if len(s[start:]) < 2 || s[start] != '"' {
		return "", ""
	}
	// ETag is either W/"text" or "text".
	// See RFC 7232 2.3.
	for i := start + 1; i < len(s); i++ {
		c := s[i]
		switch {
		// Character values allowed in ETags.
		case c == 0x21 || c >= 0x23 && c <= 0x7E || c >= 0x80:
		case c == '"':
			return s[:i+1], s[i+1:]
		default:
			return "", ""
		}
	}
	return "", ""
}

// etagStrongMatch reports whether a and b match using strong ETag comparison.
// Assumes a and b are valid ETags.
func etagStrongMatch(a, b string) bool {
	return a == b && a != "" && a[0] == '"'
}

// etagWeakMatch reports whether a and b match using weak ETag comparison.
// Assumes a and b are valid ETags.
func etagWeakMatch(a, b string) bool {
	return strings.TrimPrefix(a, "W/") == strings.TrimPrefix(b, "W/")
}

// httpRange specifies the byte range to be sent to the client.
type httpRange struct {
	start, length int64
}

func (r httpRange) contentRange(size int64) string {
	return fmt.Sprintf("bytes %d-%d/%d", r.start, r.start+r.length-1, size)
}

func (r httpRange) mimeHeader(contentType string, size int64) textproto.MIMEHeader {
	return textproto.MIMEHeader{
		"Content-Range": {r.contentRange(size)},
		"Content-Type":  {contentType},
	}
}

// parseRange parses a Range header string as per RFC 7233.
// errNoOverlap is returned if none of the ranges overlap.
func parseRange(s string, size int64) ([]httpRange, error) {
	if s == "" {
		return nil, nil // header not present
	}
	const b = "bytes="
	if !strings.HasPrefix(s, b) {
		return nil, errors.New("invalid range")
	}
	var ranges []httpRange
	noOverlap := false
	for _, ra := range strings.Split(s[len(b):], ",") {
		ra = textproto.TrimString(ra)
		if ra == "" {
			continue
		}
		start, end, ok := strings.Cut(ra, "-")
		if !ok {
			return nil, errors.New("invalid range")
		}
		start, end = textproto.TrimString(start), textproto.TrimString(end)
		var r httpRange
		if start == "" {
			// If no start is specified, end specifies the
			// range start relative to the end of the file,
			// and we are dealing with <suffix-length>
			// which has to be a non-negative integer as per
			// RFC 7233 Section 2.1 "Byte-Ranges".
			if end == "" || end[0] == '-' {
				return nil, errors.New("invalid range")
			}
			i, err := strconv.ParseInt(end, 10, 64)
			if i < 0 || err != nil {
				return nil, errors.New("invalid range")
			}
			if i > size {
				i = size
			}
			r.start = size - i
			r.length = size - r.start
		} else {
			i, err := strconv.ParseInt(start, 10, 64)
			if err != nil || i < 0 {
				return nil, errors.New("invalid range")
			}
			if i >= size {
				// If the range begins after the size of the content,
				// then it does not overlap.
				noOverlap = true
				continue
			}
			r.start = i
			if end == "" {
				// If no end is specified, range extends to end of the file.
				r.length = size - r.start
			} else {
				i, err := strconv.ParseInt(end, 10, 64)
				if err != nil || r.start > i {
					return nil, errors.New("invalid range")
				}
				if i >= size {
					i = size - 1
				}
				r.length = i - r.start + 1
			}
		}
		ranges = append(ranges, r)
	}
	if noOverlap && len(ranges) == 0 {
		// The specified ranges did not overlap with the content.
		return nil, errNoOverlap
	}
	return ranges, nil
}

func sumRangesSize(ranges []httpRange) (size int64) {
	for _, ra := range ranges {
		size += ra.length
	}
	return
}
