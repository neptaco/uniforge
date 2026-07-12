package cmd

import (
	"testing"

	"github.com/neptaco/uniforge/pkg/unity"
)

func TestParseCleanUnityTargetsRequiresExplicitTarget(t *testing.T) {
	_, err := parseCleanUnityTargets(nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestParseCleanUnityTargetsAcceptsLockfile(t *testing.T) {
	targets, err := parseCleanUnityTargets([]string{"LockFile"})
	if err != nil {
		t.Fatalf("parseCleanUnityTargets failed: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("targets = %d, want 1", len(targets))
	}
	if targets[0] != unity.CleanTargetLockfile {
		t.Fatalf("target = %s, want %s", targets[0], unity.CleanTargetLockfile)
	}
}

func TestParseCleanUnityTargetsRejectsUnsupportedTarget(t *testing.T) {
	_, err := parseCleanUnityTargets([]string{"library"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
