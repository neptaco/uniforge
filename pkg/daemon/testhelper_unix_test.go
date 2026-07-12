//go:build !windows

package daemon

import (
	"net"
	"testing"
)

// testDial connects to the daemon listener using the platform-appropriate transport.
func testDial(t *testing.T, endpoint string) net.Conn {
	t.Helper()
	conn, err := net.Dial("unix", endpoint)
	if err != nil {
		t.Fatalf("dial unix: %v", err)
	}
	return conn
}

// expectedTransport returns the expected transport type for this platform.
func expectedTransport() Transport {
	return TransportUnix
}
