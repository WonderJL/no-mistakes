package cli

import (
	"strings"
	"testing"
)

func TestUpdateCommandDisabled(t *testing.T) {
	out, err := executeCmd("update")
	if err != nil {
		t.Fatalf("update failed: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, "self-update is disabled") {
		t.Fatalf("expected disabled message, got: %s", out)
	}
	if !strings.Contains(out, "github.com/wonderjl/no-mistakes") {
		t.Fatalf("expected fork install guidance, got: %s", out)
	}
}
