package server

import (
	"fmt"
	"os"
	"path"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
)

// stackSize is the maximum size of the stack trace printed for a recovered panic.
const stackSize = 4 << 10

// recoverPanic recovers from panics in the rest of the chain, prints the panic with the stack of all
// goroutines and answers the request through the error handler.
func recoverPanic(c *fiber.Ctx) (err error) {
	defer func() {
		if r := recover(); r != nil {
			perr, ok := r.(error)
			if !ok {
				perr = fmt.Errorf("%v", r)
			}
			stack := make([]byte, stackSize)
			length := runtime.Stack(stack, true)
			logLine("-", fmt.Sprintf("[PANIC RECOVER] %v %s\n", perr, stack[:length]))
			err = c.App().ErrorHandler(c, perr)
		}
	}()
	return c.Next()
}

var logMu sync.Mutex

// logLine prints a server message to stdout as a JSON line, in the format of the service's
// original (Echo) logger, prefix included, so log consumers keep working.
func logLine(level, message string) {
	_, file, line, _ := runtime.Caller(1)
	logMu.Lock()
	defer logMu.Unlock()
	fmt.Fprintf(os.Stdout, `{"time":"%s","level":"%s","prefix":"echo","file":"%s","line":"%d","message":%s}`+"\n",
		time.Now().Format(time.RFC3339Nano), level, path.Base(file), line, strconv.Quote(message))
}
