package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Auto-update against GitHub Releases of the (private) source repo.
//
// Flow: check -> download DMG -> verify SHA-256 -> extract .app ->
// re-exec the new binary with --apply-update, which swaps the bundle
// and relaunches (see applyupdate_darwin.go). The pattern mirrors the
// magpiedb updater: version source of truth, digest verification, a
// helper swap with a kept backup for rollback.
//
// The repo is private, so release API calls and asset downloads need a
// token. For this self-use app the token is NOT embedded in the binary;
// it is read, in order, from:
//   1. env DESKCANVAS_UPDATE_TOKEN
//   2. <UserConfigDir>/DeskCanvas/update-token  (one line, a fine-grained
//      read-only PAT scoped to this repo only)
// Without a token the check still works if the repo ever goes public.

const (
	updateOwner = "sugaryiiy"
	updateRepo  = "deskcanvas"
	dmgAsset    = "DeskCanvas-M0-macOS-arm64.dmg"
)

type ghRelease struct {
	TagName    string `json:"tag_name"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
	Assets     []struct {
		ID     int64  `json:"id"`
		Name   string `json:"name"`
		Size   int64  `json:"size"`
		Digest string `json:"digest"` // "sha256:<hex>", may be empty on old assets
	} `json:"assets"`
}

// updateInfo is what the frontend polls.
type updateInfo struct {
	Current   string `json:"current"`
	Latest    string `json:"latest,omitempty"`
	Available bool   `json:"available"`
	Status    string `json:"status"` // idle|checking|downloading|ready|applying|error|uptodate
	Detail    string `json:"detail,omitempty"`
}

func updateToken() string {
	if t := strings.TrimSpace(os.Getenv("DESKCANVAS_UPDATE_TOKEN")); t != "" {
		return t
	}
	cfg, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	b, err := os.ReadFile(filepath.Join(cfg, "DeskCanvas", "update-token"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func ghRequest(ctx context.Context, url, accept string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if tok := updateToken(); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	return http.DefaultClient.Do(req)
}

// latestRelease returns the newest non-draft release (prereleases
// included — /releases/latest would skip them, and this project ships
// prereleases during development).
func latestRelease(ctx context.Context) (*ghRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases?per_page=10", updateOwner, updateRepo)
	resp, err := ghRequest(ctx, url, "application/vnd.github+json")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github releases list: %s", resp.Status)
	}
	var rels []ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&rels); err != nil {
		return nil, err
	}
	for i := range rels {
		if !rels[i].Draft {
			return &rels[i], nil
		}
	}
	return nil, fmt.Errorf("no published release found")
}

// compareVersions compares dotted numeric versions with an optional
// leading "v" and an optional "-suffix" (suffixes are ignored except
// that a suffixed build sorts before the same version without one,
// e.g. v0.1.1-m0 < v0.1.1). Returns -1, 0 or +1.
func compareVersions(a, b string) int {
	pa, pb := parseVersion(a), parseVersion(b)
	for i := 0; i < len(pa.nums) || i < len(pb.nums); i++ {
		var x, y int
		if i < len(pa.nums) {
			x = pa.nums[i]
		}
		if i < len(pb.nums) {
			y = pb.nums[i]
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	if pa.suffix == pb.suffix {
		return 0
	}
	if pa.suffix == "" {
		return 1
	}
	if pb.suffix == "" {
		return -1
	}
	if pa.suffix < pb.suffix {
		return -1
	}
	return 1
}

type parsedVersion struct {
	nums   []int
	suffix string
}

func parseVersion(v string) parsedVersion {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	core, suffix, _ := strings.Cut(v, "-")
	var nums []int
	for _, part := range strings.Split(core, ".") {
		n, err := strconv.Atoi(part)
		if err != nil {
			n = 0
		}
		nums = append(nums, n)
	}
	return parsedVersion{nums: nums, suffix: suffix}
}

// updatesDir is where downloaded DMGs and staged apps live.
func updatesDir() (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(cache, "DeskCanvas", "updates")
	return dir, os.MkdirAll(dir, 0o755)
}

// downloadAsset streams one release asset to dest, verifying its size,
// and returns the hex SHA-256 of what landed on disk.
func downloadAsset(ctx context.Context, assetID int64, size int64, dest string) (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/assets/%d", updateOwner, updateRepo, assetID)
	resp, err := ghRequest(ctx, url, "application/octet-stream")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("asset download: %s", resp.Status)
	}
	f, err := os.Create(dest)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), resp.Body)
	if err != nil {
		return "", err
	}
	if size > 0 && n != size {
		return "", fmt.Errorf("asset size mismatch: got %d, want %d", n, size)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// --- App-side orchestration ----------------------------------------------

func (a *App) setUpdateStatus(status, detail string) {
	a.mu.Lock()
	a.update.Status = status
	a.update.Detail = detail
	a.mu.Unlock()
	log.Printf("deskcanvas update: %s %s", status, detail)
}

// checkUpdate refreshes a.update from GitHub. Safe to call repeatedly.
func (a *App) checkUpdate(ctx context.Context) updateInfo {
	a.mu.Lock()
	a.update.Current = Version
	if Version == "dev" {
		a.update.Status = "idle"
		a.update.Detail = "dev build — auto-update disabled"
	}
	a.mu.Unlock()
	if Version == "dev" {
		return a.snapshotUpdate()
	}

	a.setUpdateStatus("checking", "")
	rel, err := latestRelease(ctx)
	if err != nil {
		a.setUpdateStatus("error", err.Error())
		return a.snapshotUpdate()
	}
	a.mu.Lock()
	a.update.Latest = rel.TagName
	a.update.Available = compareVersions(Version, rel.TagName) < 0
	if a.update.Available {
		a.update.Status = "idle"
		a.update.Detail = "new version available"
	} else {
		a.update.Status = "uptodate"
		a.update.Detail = ""
	}
	a.mu.Unlock()
	return a.snapshotUpdate()
}

func (a *App) snapshotUpdate() updateInfo {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.update
}

// downloadAndStage fetches the DMG of the latest release, verifies it
// against the digest GitHub reports for the asset, extracts the .app
// and remembers where it is for the apply step.
func (a *App) downloadAndStage(ctx context.Context) error {
	rel, err := latestRelease(ctx)
	if err != nil {
		return err
	}
	var assetID, assetSize int64
	var digest string
	for _, as := range rel.Assets {
		if as.Name == dmgAsset {
			assetID, assetSize, digest = as.ID, as.Size, as.Digest
		}
	}
	if assetID == 0 {
		return fmt.Errorf("release %s has no %s asset", rel.TagName, dmgAsset)
	}
	dir, err := updatesDir()
	if err != nil {
		return err
	}
	dmgPath := filepath.Join(dir, rel.TagName+"-"+dmgAsset)
	a.setUpdateStatus("downloading", rel.TagName)
	ctx2, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	sum, err := downloadAsset(ctx2, assetID, assetSize, dmgPath)
	if err != nil {
		return err
	}
	if strings.HasPrefix(digest, "sha256:") {
		want := strings.TrimPrefix(digest, "sha256:")
		if !strings.EqualFold(sum, want) {
			return fmt.Errorf("sha256 mismatch: got %s, want %s", sum, want)
		}
	}
	appPath, err := extractAppFromDMG(dmgPath, filepath.Join(dir, rel.TagName+"-staged"))
	if err != nil {
		return err
	}
	a.mu.Lock()
	a.update.Latest = rel.TagName
	a.update.Available = true
	a.stagedApp = appPath
	a.mu.Unlock()
	a.setUpdateStatus("ready", rel.TagName)
	return nil
}

// applyUpdate downloads (if needed) and hands over to the new binary,
// which performs the bundle swap and relaunch. This process exits as
// part of the handover.
func (a *App) applyUpdate(ctx context.Context) error {
	info := a.snapshotUpdate()
	if !info.Available {
		info = a.checkUpdate(ctx)
	}
	if !info.Available {
		return fmt.Errorf("no update available")
	}
	a.mu.Lock()
	staged := a.stagedApp
	a.mu.Unlock()
	if staged == "" {
		if err := a.downloadAndStage(ctx); err != nil {
			a.setUpdateStatus("error", err.Error())
			return err
		}
		a.mu.Lock()
		staged = a.stagedApp
		a.mu.Unlock()
	}
	a.setUpdateStatus("applying", info.Latest)
	return launchApplyHelper(staged) // darwin: re-exec + quit; other: logs
}
