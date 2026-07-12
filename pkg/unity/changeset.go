package unity

import (
	"github.com/neptaco/uniforge/pkg/hub"
)

// GetChangesetForVersion fetches the changeset for a specific Unity version.
// Resolution lives in pkg/hub so that every Unity Release API request in the
// process shares the same rate limiter and retry handling.
func GetChangesetForVersion(version string) (string, error) {
	return hub.ResolveChangeset(version)
}
