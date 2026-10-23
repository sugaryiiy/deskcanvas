//go:build darwin

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// macOS implementation of the updater's platform hooks: DMG mounting,
// bundle discovery, and the --apply-update swap helper. The helper is
// the NEW app's own binary re-executed: it waits for the old process to
// exit, swaps the bundles with ditto, strips the quarantine attribute,
// and relaunches. The old bundle is kept as DeskCanvas.previous.app so
// a bad update can be rolled back by renaming it back by hand; a
// healthy new start deletes it (see cleanupPreviousBundle in app.go).

func extractAppFromDMG(dmgPath, stagedDir string) (string, error) {
	mount := stagedDir + "-mount"
	if err := os.MkdirAll(mount, 0o755); err != nil {
		return "", err
	}
	if out, err := exec.Command("/usr/bin/hdiutil", "attach", "-nobrowse", "-readonly",
		"-mountpoint", mount, dmgPath).CombinedOutput(); err != nil {
		return "", fmt.Errorf("hdiutil attach: %v: %s", err, out)
	}
	defer exec.Command("/usr/bin/hdiutil", "detach", "-quiet", mount).Run()

	entries, err := os.ReadDir(mount)
	if err != nil {
		return "", err
	}
	var appName string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".app") {
			appName = e.Name()
		}
	}
	if appName == "" {
		return "", fmt.Errorf("no .app inside DMG")
	}
	dest := filepath.Join(stagedDir, appName)
	os.RemoveAll(stagedDir)
	if err := os.MkdirAll(stagedDir, 0o755); err != nil {
		return "", err
	}
	if out, err := exec.Command("/usr/bin/ditto", filepath.Join(mount, appName), dest).CombinedOutput(); err != nil {
		return "", fmt.Errorf("ditto app: %v: %s", err, out)
	}
	return dest, nil
}

// currentBundlePath returns the .app bundle this process runs from,
// or "" when it is not inside a bundle (dev binary).
func currentBundlePath() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if i := strings.Index(exe, ".app/Contents/MacOS/"); i >= 0 {
		return exe[:i+len(".app")]
	}
	return ""
}

func launchApplyHelper(stagedApp string) error {
	target := currentBundlePath()
	if target == "" {
		return fmt.Errorf("not running from an .app bundle; cannot self-update")
	}
	exeName := filepath.Base(mustExecutable())
	helper := filepath.Join(stagedApp, "Contents", "MacOS", exeName)
	cmd := exec.Command(helper, "--apply-update", stagedApp, target, strconv.Itoa(os.Getpid()))
	cmd.Stdout, cmd.Stderr = applyLogFile(), applyLogFile()
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start apply helper: %w", err)
	}
	// Give the helper a moment to start waiting on our PID, then exit
	// so it can perform the swap.
	go func() {
		time.Sleep(300 * time.Millisecond)
		os.Exit(0)
	}()
	return nil
}

func mustExecutable() string {
	exe, _ := os.Executable()
	return exe
}

func applyLogFile() *os.File {
	dir, err := updatesDir()
	if err != nil {
		return nil
	}
	f, _ := os.OpenFile(filepath.Join(dir, "apply.log"),
		os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	return f
}

// runApplyUpdateIfRequested executes the swap when this process was
// launched as the update helper. Returns true when it handled the run.
func runApplyUpdateIfRequested() bool {
	if len(os.Args) < 5 || os.Args[1] != "--apply-update" {
		return false
	}
	source, target := os.Args[2], os.Args[3]
	pid, _ := strconv.Atoi(os.Args[4])
	logf := applyLogFile()
	if logf != nil {
		defer logf.Close()
		fmt.Fprintf(logf, "apply-update: source=%s target=%s waitPid=%d\n", source, target, pid)
	}

	// Wait for the old process to die (up to 30s).
	for i := 0; i < 300; i++ {
		if err := syscall.Kill(pid, 0); err != nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	previous := filepath.Join(filepath.Dir(target), "DeskCanvas.previous.app")
	os.RemoveAll(previous)
	if err := os.Rename(target, previous); err != nil {
		// Nowhere safe to swap into; relaunch what is still there.
		exec.Command("/usr/bin/open", target).Run()
		return true
	}
	if out, err := exec.Command("/usr/bin/ditto", source, target).CombinedOutput(); err != nil {
		// Roll back: restore the previous bundle and launch it.
		os.RemoveAll(target)
		os.Rename(previous, target)
		if logf != nil {
			fmt.Fprintf(logf, "apply-update: ditto failed, rolled back: %v: %s\n", err, out)
		}
	}
	exec.Command("/usr/bin/xattr", "-dr", "com.apple.quarantine", target).Run()
	exec.Command("/usr/bin/open", target).Run()
	return true
}

// cleanupPreviousBundle removes the rollback bundle after a healthy
// start of the updated app.
func cleanupPreviousBundle() {
	target := currentBundlePath()
	if target == "" {
		return
	}
	previous := filepath.Join(filepath.Dir(target), "DeskCanvas.previous.app")
	if previous != target {
		os.RemoveAll(previous)
	}
}
