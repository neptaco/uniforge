package mcp

import (
	"testing"
	"time"

	"github.com/neptaco/uniforge/pkg/bridge"
	"github.com/neptaco/uniforge/pkg/daemon"
)

func TestDaemonClientOptionsPreserveRuntimeConfig(t *testing.T) {
	config := daemon.Config{Name: "custom-uniforge", RuntimeDir: t.TempDir()}
	timeout := 17 * time.Second
	runtime := NewBridgeRuntime(BridgeRuntimeOptions{
		DaemonConfig:    config,
		AutoStartDaemon: true,
	})

	got := runtime.daemonClientOptions(timeout)
	want := bridge.ClientOptions{
		DaemonConfig:    config,
		AutoStartDaemon: true,
		RequestTimeout:  timeout,
	}
	if got.DaemonConfig != want.DaemonConfig ||
		got.AutoStartDaemon != want.AutoStartDaemon ||
		got.RequestTimeout != want.RequestTimeout {
		t.Fatalf("daemon client options = %+v, want %+v", got, want)
	}
}
