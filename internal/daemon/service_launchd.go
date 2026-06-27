package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wonderjl/no-mistakes/internal/paths"
	"github.com/wonderjl/no-mistakes/internal/shellenv"
)

// Retry parameters for `launchctl bootstrap` after a preceding bootout.
// launchctl bootout is async: it SIGTERMs the existing service and gives
// launchd up to ~5s to finalize cleanup. During that window, bootstrap
// returns errno 37 EPROGRESS ("Operation already in progress") and there
// is no synchronous API to wait for the previous instance to fully detach.
// A stop+start sequence (which is exactly what `make install` does, and
// what `daemon restart` does) collides with this window unless bootstrap
// is retried. Exposed as package vars so tests can shrink the timings.
var (
	launchctlBootstrapRetryTimeout  = 10 * time.Second
	launchctlBootstrapRetryInterval = 200 * time.Millisecond
)

func installLaunchAgent(p *paths.Paths, exe string) error {
	path := launchAgentPath(p)
	home, err := serviceUserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve user home: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create launch agents directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(renderLaunchAgent(exe, p, home)), 0o644); err != nil {
		return fmt.Errorf("write launch agent: %w", err)
	}
	cleanupLegacyLaunchAgent(p)
	return nil
}

// cleanupLegacyLaunchAgent removes plists installed by an earlier binary so
// the new scoped install is the only managed daemon for this user going
// forward. Two legacy artifacts are handled:
//
//   - the globally-named (unscoped) plist a pre-scoping binary installed at
//     legacyLaunchdServiceLabel + ".plist"; and
//   - the per-install scoped plist the previous kunchenguid-labeled binary
//     installed at legacyLaunchdServiceLabel + "." + suffix + ".plist". After
//     a hard fork renames the label base, that is the label the user's
//     currently-live daemon runs under, so it must be booted out too or it is
//     orphaned in launchd alongside the new com.wonderjl daemon.
//
// For each we bootout the label before deleting so an already-loaded legacy
// daemon is released from launchd (it will exit on SIGTERM). Both are gated on
// the plist definition matching p.Root() so we never tear down a daemon owned
// by a different NM_HOME. Any error is best-effort: if there's no legacy plist
// or launchctl refuses, we proceed with the scoped install.
func cleanupLegacyLaunchAgent(p *paths.Paths) {
	cleanupLegacyLaunchAgentInstance(p, legacyLaunchdServiceLabel, legacyLaunchAgentPath())
	cleanupLegacyLaunchAgentInstance(p, suffixedLegacyLaunchdServiceLabel(p), suffixedLegacyLaunchAgentPath(p))
}

func cleanupLegacyLaunchAgentInstance(p *paths.Paths, label, path string) {
	data, err := os.ReadFile(path)
	if err != nil || !serviceDefinitionMatchesRoot(data, p) {
		return
	}
	if domain, err := launchdDomainTarget(); err == nil {
		_, _ = serviceCommandRunner("launchctl", "bootout", domain+"/"+label)
	}
	_ = os.Remove(path)
}

func startLaunchAgent(p *paths.Paths) error {
	domain, err := launchdDomainTarget()
	if err != nil {
		return err
	}
	serviceTarget := domain + "/" + launchdServiceLabel(p)
	path := launchAgentPath(p)
	_, _ = serviceCommandRunner("launchctl", "bootout", serviceTarget)
	bootstrapErr := launchctlBootstrapWithRetry(domain, path)
	_, kickstartErr := serviceCommandRunner("launchctl", "kickstart", "-k", serviceTarget)
	if kickstartErr != nil {
		if bootstrapErr != nil {
			return fmt.Errorf("launchctl bootstrap: %v; kickstart: %w", bootstrapErr, kickstartErr)
		}
		return fmt.Errorf("launchctl kickstart: %w", kickstartErr)
	}
	return nil
}

// launchctlBootstrapWithRetry runs `launchctl bootstrap` and retries on
// errno 37 EPROGRESS until the bootout-cleanup window closes. Non-busy
// failures (bad plist, bad permissions) return immediately so they can
// surface through the normal managed-start fallback path.
func launchctlBootstrapWithRetry(domain, path string) error {
	deadline := time.Now().Add(launchctlBootstrapRetryTimeout)
	var lastErr error
	for {
		_, err := serviceCommandRunner("launchctl", "bootstrap", domain, path)
		if err == nil {
			return nil
		}
		if !launchctlBootstrapBusy(err) {
			return err
		}
		lastErr = err
		if time.Now().After(deadline) {
			return lastErr
		}
		time.Sleep(launchctlBootstrapRetryInterval)
	}
}

// launchctlBootstrapBusy reports whether a bootstrap error is the
// "previous instance still unloading" race. launchctl surfaces this as
// exit 37 with stderr "Bootstrap failed: 37: Operation already in
// progress". Match both exit status and message text so we remain robust
// to launchctl output tweaks across macOS releases.
func launchctlBootstrapBusy(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "operation already in progress") ||
		strings.Contains(text, "exit status 37")
}

func stopLaunchAgent(p *paths.Paths) error {
	domain, err := launchdDomainTarget()
	if err != nil {
		return err
	}
	output, err := serviceCommandRunner("launchctl", "bootout", domain+"/"+launchdServiceLabel(p))
	if err != nil {
		if launchctlBootoutServiceNotLoaded(err, output) {
			return nil
		}
		return fmt.Errorf("launchctl bootout: %w", err)
	}
	return nil
}

func removeLaunchAgent(p *paths.Paths) error {
	err := os.Remove(launchAgentPath(p))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// launchctlBootoutServiceNotLoaded reports whether a launchctl bootout
// failure is the ESRCH case ("No such process", exit 3) that launchctl
// emits when the service label isn't currently loaded. That is semantically
// a successful stop - the service is already not running.
func launchctlBootoutServiceNotLoaded(err error, output []byte) bool {
	if err == nil {
		return false
	}
	combined := strings.ToLower(string(output) + " " + err.Error())
	return strings.Contains(combined, "no such process")
}

func launchAgentPath(p *paths.Paths) string {
	home, err := serviceUserHomeDir()
	if err != nil {
		home = ""
	}
	return filepath.Join(home, "Library", "LaunchAgents", launchdServiceLabel(p)+".plist")
}

func legacyLaunchAgentPath() string {
	home, _ := serviceUserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", legacyLaunchdServiceLabel+".plist")
}

// suffixedLegacyLaunchdServiceLabel is the per-install scoped label the
// previous kunchenguid-labeled binary ran the daemon under for this root,
// before the label base was renamed to com.wonderjl. It is the live daemon's
// label that the migration in cleanupLegacyLaunchAgent boots out.
func suffixedLegacyLaunchdServiceLabel(p *paths.Paths) string {
	return legacyLaunchdServiceLabel + "." + serviceInstanceSuffix(p)
}

func suffixedLegacyLaunchAgentPath(p *paths.Paths) string {
	home, _ := serviceUserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", suffixedLegacyLaunchdServiceLabel(p)+".plist")
}

func launchdDomainTarget() (string, error) {
	u, err := serviceCurrentUser()
	if err != nil {
		return "", fmt.Errorf("resolve current user: %w", err)
	}
	if u == nil || u.Uid == "" {
		return "", fmt.Errorf("resolve current user: empty uid")
	}
	return "gui/" + u.Uid, nil
}

func renderLaunchAgent(exe string, p *paths.Paths, home string) string {
	values := []string{exe, "daemon", "run", "--root", p.Root()}
	var args strings.Builder
	for _, value := range values {
		args.WriteString("    <string>")
		args.WriteString(xmlEscaped(value))
		args.WriteString("</string>\n")
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>%s</string>
  <key>ProgramArguments</key>
  <array>
%s  </array>
  <key>WorkingDirectory</key>
  <string>%s</string>
  <key>EnvironmentVariables</key>
  <dict>
    <key>HOME</key>
    <string>%s</string>
    <key>PATH</key>
    <string>%s</string>
  </dict>
  <key>StandardOutPath</key>
  <string>%s</string>
  <key>StandardErrorPath</key>
  <string>%s</string>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
</dict>
</plist>
`, xmlEscaped(launchdServiceLabel(p)), args.String(), xmlEscaped(p.Root()), xmlEscaped(home), xmlEscaped(managedServicePath(home)), xmlEscaped(p.DaemonLog()), xmlEscaped(p.DaemonLog()))
}

// managedServicePath returns a default PATH for daemons started by a service
// manager (launchd, systemd) that would otherwise inherit only the service
// manager's minimal PATH. Home-directory entries are interpolated here
// because neither plist nor systemd Environment= expands $HOME.
//
// Entry order: user-scoped dirs first so user-managed tools (go, cargo,
// ~/.local/bin) win over system packages, then Homebrew and distro defaults.
func managedServicePath(home string) string {
	return strings.Join(shellenv.WellKnownBinDirsForHome(home), string(os.PathListSeparator))
}
