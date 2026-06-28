package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// After the rip, no Go source under the module may reference a telemetry sink,
// the tracking wrappers, or the telemetry host/env. Freezes the removal.
func TestNoTelemetryReferencesRemain(t *testing.T) {
	root := "../.."
	banned := []string{
		"internal/telemetry",
		"trackCommand",
		"trackAxiSurface",
		"a.kunchenguid.com",
		"NO_MISTAKES_UMAMI",
	}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		if strings.HasSuffix(path, "telemetry_guard_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, b := range banned {
			if strings.Contains(string(data), b) {
				t.Errorf("%s still references %q", path, b)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
