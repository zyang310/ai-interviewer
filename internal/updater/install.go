//go:build darwin

package updater

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
)

// Install downloads the release at downloadURL, verifies it is a genuine,
// notarized Mogi build, and hands off to a detached helper that waits for this
// process to exit, swaps the new .app into the current install location, and
// relaunches it. It returns once the helper is confirmed running — the caller
// (App.InstallUpdate) is expected to quit the Wails runtime immediately after,
// which lets the swap proceed.
//
// Only signed, notarized downloads are ever installed: a download that fails
// codesign/spctl verification is rejected outright, the same bar release.yml
// itself enforces before publishing (see docs/ci-cd-and-auto-update.md).
func Install(ctx context.Context, downloadURL string) error {
	if downloadURL == "" {
		return fmt.Errorf("updater: no download URL available")
	}

	installPath, err := currentAppBundle()
	if err != nil {
		return fmt.Errorf("updater: locate running app: %w", err)
	}

	workDir, err := os.MkdirTemp("", "mogi-update-*")
	if err != nil {
		return fmt.Errorf("updater: create work dir: %w", err)
	}

	zipPath := filepath.Join(workDir, "update.zip")
	if err := downloadFile(ctx, downloadURL, zipPath); err != nil {
		os.RemoveAll(workDir)
		return err
	}

	extractDir := filepath.Join(workDir, "extracted")
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		os.RemoveAll(workDir)
		return fmt.Errorf("updater: create extract dir: %w", err)
	}
	// ditto, not archive/zip: it preserves symlinks, resource forks, and
	// extended attributes, all of which archive/zip drops or mangles and any
	// of which invalidates the bundle's code signature.
	if out, err := exec.CommandContext(ctx, "ditto", "-x", "-k", zipPath, extractDir).CombinedOutput(); err != nil {
		os.RemoveAll(workDir)
		return fmt.Errorf("updater: extract update: %w: %s", err, out)
	}

	newApp, err := findAppBundle(extractDir)
	if err != nil {
		os.RemoveAll(workDir)
		return err
	}

	if err := verifySignedAndNotarized(ctx, newApp); err != nil {
		os.RemoveAll(workDir)
		return fmt.Errorf("updater: downloaded build failed verification, refusing to install: %w", err)
	}

	if err := spawnSwapHelper(installPath, newApp, workDir); err != nil {
		os.RemoveAll(workDir)
		return fmt.Errorf("updater: start install helper: %w", err)
	}
	return nil
}

// currentAppBundle resolves the running executable's .app bundle root, e.g.
// "/Applications/Mogi.app" from ".../Mogi.app/Contents/MacOS/Mogi".
func currentAppBundle() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve running executable: %w", err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return "", fmt.Errorf("resolve executable symlinks: %w", err)
	}
	return bundleFromExecutable(exe)
}

// bundleFromExecutable computes the .app bundle root from a path inside its
// Contents/MacOS directory. Split out from currentAppBundle so the path
// arithmetic is testable without a real running .app.
func bundleFromExecutable(exe string) (string, error) {
	bundle := filepath.Dir(filepath.Dir(filepath.Dir(exe)))
	if filepath.Ext(bundle) != ".app" {
		return "", fmt.Errorf("running binary is not inside a .app bundle (got %s)", bundle)
	}
	return bundle, nil
}

// findAppBundle returns the single top-level .app directory inside dir — the
// same "first .app wins" rule release.yml itself uses when packaging.
func findAppBundle(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("updater: read extracted contents: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() && filepath.Ext(e.Name()) == ".app" {
			return filepath.Join(dir, e.Name()), nil
		}
	}
	return "", fmt.Errorf("updater: no .app bundle in downloaded release")
}

// verifySignedAndNotarized runs the same two checks release.yml runs on a
// build before publishing it: a valid Developer ID signature, and Gatekeeper's
// own verdict on whether it would launch without warnings. Either failing means
// the download does not match what we published, and it must not be installed.
func verifySignedAndNotarized(ctx context.Context, appPath string) error {
	if out, err := exec.CommandContext(ctx, "codesign", "--verify", "--deep", "--strict", appPath).CombinedOutput(); err != nil {
		return fmt.Errorf("codesign verify: %w: %s", err, out)
	}
	if out, err := exec.CommandContext(ctx, "spctl", "--assess", "--type", "execute", appPath).CombinedOutput(); err != nil {
		return fmt.Errorf("gatekeeper assessment: %w: %s", err, out)
	}
	return nil
}

// spawnSwapHelper starts a detached shell process that waits for the current
// process (by pid) to exit, then swaps newApp into installPath and relaunches
// it. Deliberately NOT started with the request's context: that context dies
// with this process during Wails shutdown, and exec.CommandContext SIGKILLs its
// child the instant its context is cancelled — which would kill the helper
// before it ever gets to run. Setsid detaches it into its own session so it
// survives the parent exiting at all. Paths travel as env vars, never
// interpolated into the script text, so unusual paths can't be read as shell
// syntax.
func spawnSwapHelper(installPath, newApp, cleanupDir string) error {
	const script = `
set -e
while kill -0 "$MOGI_PID" 2>/dev/null; do sleep 0.2; done
old="$MOGI_INSTALL_PATH.old"
rm -rf "$old"
mv "$MOGI_INSTALL_PATH" "$old"
mv "$MOGI_NEW_APP" "$MOGI_INSTALL_PATH"
rm -rf "$old"
open "$MOGI_INSTALL_PATH"
rm -rf "$MOGI_CLEANUP_DIR"
`
	cmd := exec.Command("/bin/sh", "-c", script)
	cmd.Env = append(os.Environ(),
		"MOGI_PID="+strconv.Itoa(os.Getpid()),
		"MOGI_INSTALL_PATH="+installPath,
		"MOGI_NEW_APP="+newApp,
		"MOGI_CLEANUP_DIR="+cleanupDir,
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd.Start()
}

// downloadFile streams url to destPath. Separate from Check's client: this
// transfers megabytes rather than a small JSON body, needs no GitHub API
// headers, and deliberately has no fixed timeout — ctx (the app's lifetime
// context) is the only cutoff, so a slow connection is never treated as failure.
func downloadFile(ctx context.Context, url, destPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("updater: build download request: %w", err)
	}
	req.Header.Set("User-Agent", "mogi")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("updater: download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("updater: download returned %d", resp.StatusCode)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("updater: create download file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("updater: write download: %w", err)
	}
	return nil
}
