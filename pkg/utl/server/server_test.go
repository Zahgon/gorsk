package server_test

import (
	"testing"

	"github.com/ribice/gorsk/pkg/utl/server"
)

// Improve tests
func TestNew(t *testing.T) {
	e := server.New(&server.Config{})
	if e == nil {
		t.Errorf("Server should not be nil")
	}
}
