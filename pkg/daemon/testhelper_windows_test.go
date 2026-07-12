//go:build windows

package daemon

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/Microsoft/go-winio"
)

// testDial connects to the daemon listener using the platform-appropriate transport.
func testDial(t *testing.T, endpoint string) net.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := winio.DialPipeContext(ctx, endpoint)
	if err != nil {
		t.Fatalf("dial named pipe: %v", err)
	}
	return conn
}

// expectedTransport returns the expected transport type for this platform.
func expectedTransport() Transport {
	return TransportNamedPipe
}
