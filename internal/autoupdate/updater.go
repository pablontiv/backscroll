// Package autoupdate implements Backscroll's private release-update contract.
// See README.md for provenance and why the upstream updater cannot be used.
package autoupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/pablontiv/picokit/hashfile"
)

// Options supplies explicit boundaries for deterministic, installation-safe
// tests. Zero values use the production OS and HTTP implementations.
type Options struct {
	HTTPClient *http.Client
	CacheDir   func() (string, error)
	Executable func() (string, error)
	Replace    func(string, string) error
	Exec       func(string) error
}

type Updater struct {
	Repo, Binary, CurrentVersion string
	httpClient                   *http.Client
	cacheDir                     func() (string, error)
	executable                   func() (string, error)
	replace                      func(string, string) error
	exec                         func(string) error
	githubAPI, githubDLBase      string
}

func New(repo, binary string, options ...Options) *Updater {
	var o Options
	if len(options) > 0 {
		o = options[0]
	}
	if o.HTTPClient == nil {
		o.HTTPClient = &http.Client{Timeout: 60 * time.Second}
	}
	if o.CacheDir == nil {
		o.CacheDir = os.UserCacheDir
	}
	if o.Executable == nil {
		o.Executable = os.Executable
	}
	if o.Replace == nil {
		o.Replace = atomicReplace
	}
	if o.Exec == nil {
		o.Exec = platformExec
	}
	return &Updater{Repo: repo, Binary: binary, httpClient: o.HTTPClient, cacheDir: o.CacheDir, executable: o.Executable, replace: o.Replace, exec: o.Exec,
		githubAPI: "https://api.github.com/repos/" + repo + "/releases/latest", githubDLBase: "https://github.com/" + repo + "/releases/download/"}
}

func (u *Updater) binaryName() string {
	if runtime.GOOS == "windows" {
		return u.Binary + ".exe"
	}
	return u.Binary
}
func (u *Updater) archiveName(tag string) string {
	ext := ".tar.gz"
	if runtime.GOOS == "windows" {
		ext = ".zip"
	}
	return fmt.Sprintf("%s_%s_%s_%s%s", u.Binary, strings.TrimPrefix(tag, "v"), runtime.GOOS, runtime.GOARCH, ext)
}

func (u *Updater) download(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := u.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("download %s: status %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	closeErr := resp.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", url, err)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close download %s: %w", url, closeErr)
	}
	return body, nil
}

func (u *Updater) FetchAndStage(ctx context.Context, current string) error {
	if current == "dev" {
		return nil
	}
	body, err := u.download(ctx, u.githubAPI)
	if err != nil {
		return err
	}
	var release struct {
		Tag string `json:"tag_name"`
	}
	if err = json.Unmarshal(body, &release); err != nil {
		return fmt.Errorf("release response: %w", err)
	}
	if release.Tag == "" {
		return fmt.Errorf("empty tag_name in github response")
	}
	if !isNewer(release.Tag, current) {
		return nil
	}
	// Tags become directory names: reject path traversal without changing release names.
	if filepath.Base(release.Tag) != release.Tag || strings.ContainsAny(release.Tag, "/\\") {
		return fmt.Errorf("invalid release tag %q", release.Tag)
	}
	cache, err := u.cacheDir()
	if err != nil {
		return fmt.Errorf("cache directory: %w", err)
	}
	dest := filepath.Join(cache, u.Binary, "staged", release.Tag, u.binaryName())
	if info, statErr := os.Stat(dest); statErr == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("staged binary is not a regular file: %s", dest)
		}
		return nil
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return fmt.Errorf("staged binary: %w", statErr)
	}
	archive := u.archiveName(release.Tag)
	base := u.githubDLBase + release.Tag + "/"
	data, err := u.download(ctx, base+archive)
	if err != nil {
		return err
	}
	checksums, err := u.download(ctx, base+"checksums.txt")
	if err != nil {
		return err
	}
	expected := ""
	for _, line := range strings.Split(string(checksums), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == archive {
			expected = fields[0]
			break
		}
	}
	if expected == "" {
		return fmt.Errorf("checksum not found for %s", archive)
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != expected {
		return fmt.Errorf("SHA256 mismatch for %s", archive)
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return fmt.Errorf("staging directory: %w", err)
	}
	if runtime.GOOS == "windows" {
		return u.extractZip(data, dest)
	}
	return u.extractTar(data, dest)
}

func (u *Updater) extractTar(data []byte, dest string) error {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("gzip: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar: %w", err)
		}
		if filepath.Base(h.Name) == u.binaryName() {
			if h.Typeflag != tar.TypeReg && h.Typeflag != tar.TypeRegA {
				return fmt.Errorf("binary is not a regular archive member")
			}
			payload, err := io.ReadAll(tr)
			if err != nil {
				return fmt.Errorf("tar binary: %w", err)
			}
			// Validate the gzip footer before publishing a staged executable.
			if _, err := io.Copy(io.Discard, gz); err != nil {
				return fmt.Errorf("gzip payload: %w", err)
			}
			return hashfile.WriteAtomic(dest, bytes.NewReader(payload), 0755)
		}
	}
	return fmt.Errorf("binary %s not found in archive", u.binaryName())
}
func (u *Updater) extractZip(data []byte, dest string) error {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("zip: %w", err)
	}
	for _, f := range zr.File {
		if filepath.Base(f.Name) != u.binaryName() {
			continue
		}
		if !f.Mode().IsRegular() {
			return fmt.Errorf("binary is not a regular zip member")
		}
		reader, err := f.Open()
		if err != nil {
			return err
		}
		err = hashfile.WriteAtomic(dest, reader, 0755)
		closeErr := reader.Close()
		if err != nil {
			return err
		}
		return closeErr
	}
	return fmt.Errorf("binary %s not found in zip", u.binaryName())
}

// The release lineage uses numeric major/minor/patch comparison, ignoring
// prerelease/build suffixes exactly as the pinned release updater did.
func parseVersion(v string) ([3]int, bool) {
	v = strings.TrimPrefix(v, "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	parts := strings.SplitN(v, ".", 3)
	var result [3]int
	if len(parts) != 3 {
		return result, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return result, false
		}
		result[i] = n
	}
	return result, true
}
func isNewer(candidate, current string) bool {
	a, ok := parseVersion(candidate)
	b, bok := parseVersion(current)
	if !ok || !bok {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return false
}
