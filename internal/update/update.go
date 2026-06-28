// Package update previously implemented in-binary self-update by polling the
// upstream GitHub releases. This fork is distributed and updated through its
// installer / `go install` pin, so self-update is disabled: the binary no
// longer contacts any remote to check for or download new versions.
package update

import (
	"fmt"
	"io"
)

// Run reports that built-in self-update is disabled. It performs no network
// access and always succeeds, so `no-mistakes update` exits 0 with guidance to
// update through the install method instead.
func Run(out io.Writer) error {
	fmt.Fprintln(out, "self-update is disabled in this build.")
	fmt.Fprintln(out, "Update through your install method, e.g. re-run your installer or:")
	fmt.Fprintln(out, "  go install github.com/wonderjl/no-mistakes/cmd/no-mistakes@<tag>")
	return nil
}

// CachedLatestVersion reports the newest known version for the TUI update
// banner. Self-update is disabled, so there is never a newer version to show.
func CachedLatestVersion() string {
	return ""
}
