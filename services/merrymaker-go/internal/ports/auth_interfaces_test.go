package ports_test

import (
	"testing"

	mocks "github.com/target/mmk-ui-api/internal/mocks/auth"
	"github.com/target/mmk-ui-api/internal/ports"
)

// This test only verifies that our mocks conform to the ports at compile time.
func TestMocksImplementPorts(t *testing.T) {
	t.Helper()

	var _ ports.AuthProvider = (*mocks.MockAuthProvider)(nil)
	var _ ports.SessionStore = (*mocks.MemorySessionStore)(nil)
	var _ ports.RoleMapper = (*mocks.StaticRoleMapper)(nil)
}
