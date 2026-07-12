package unity

import (
	"testing"
)

// TestGetChangesetForVersion_Integration tests the actual Unity Release API
// through the pkg/hub shared client.
// Skip in CI, run manually with: go test -run Integration -v ./pkg/unity/
func TestGetChangesetForVersion_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// 2022.3.62f3 is a published release whose changeset never changes
	changeset, err := GetChangesetForVersion("2022.3.62f3")
	if err != nil {
		t.Fatalf("GetChangesetForVersion failed: %v", err)
	}

	t.Logf("Changeset: %s", changeset)

	if changeset != "96770f904ca7" {
		t.Errorf("changeset = %q, want %q", changeset, "96770f904ca7")
	}
}
