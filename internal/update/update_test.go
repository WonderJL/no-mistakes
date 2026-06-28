package update

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunReportsDisabled(t *testing.T) {
	var buf bytes.Buffer
	if err := Run(&buf); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "self-update is disabled") {
		t.Fatalf("expected disabled message, got: %q", out)
	}
	// Guidance must point at the fork's install path, not any upstream repo.
	if !strings.Contains(out, "github.com/wonderjl/no-mistakes") {
		t.Fatalf("expected fork install guidance, got: %q", out)
	}
}

func TestCachedLatestVersionReturnsEmpty(t *testing.T) {
	if got := CachedLatestVersion(); got != "" {
		t.Fatalf("CachedLatestVersion() = %q, want empty (self-update disabled)", got)
	}
}
