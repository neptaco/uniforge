package cmd

import (
	"testing"

	"github.com/neptaco/uniforge/pkg/bridge"
)

func TestNewDaemonMetaUsesCLIReleaseVersion(t *testing.T) {
	meta := newDaemonMeta("0.9.1")

	if meta.ProtocolVersion != bridge.ProtocolVersion {
		t.Fatalf("protocol version = %d, want %d", meta.ProtocolVersion, bridge.ProtocolVersion)
	}
	if meta.Version != "0.9.1" {
		t.Fatalf("version = %q, want %q", meta.Version, "0.9.1")
	}
}
