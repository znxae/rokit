package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// External tools: ffmpeg, ffprobe and yt-dlp ship in a "bin" folder next to the app exe (see scripts/).
// Lookup order: <exe dir>\bin first, then PATH.

const (
	toolBundled = "bawaan"
	toolOnPath  = "PATH"
)

// ToolInfo says which copy of a tool is in use.
type ToolInfo struct {
	Found   bool   `json:"found"`
	Source  string `json:"source"` // "bawaan" or "PATH"
	Version string `json:"version"`
}

func binDir() string {
	p, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Join(filepath.Dir(p), "bin")
}

// resolveTool finds a tool: the bundled copy first, then PATH. path is "" when neither exists.
func resolveTool(name string) (path, source string) {
	if d := binDir(); d != "" {
		p := filepath.Join(d, name+".exe")
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, toolBundled
		}
	}
	if p, err := exec.LookPath(name); err == nil {
		return p, toolOnPath
	}
	return "", ""
}

// toolCmd builds the command for a tool. yt-dlp always gets --ffmpeg-location pointing at the ffmpeg in use.
// When the tool is missing the command keeps the bare name, so running it fails with exec.ErrNotFound.
func toolCmd(ctx context.Context, name string, args ...string) *exec.Cmd {
	path, _ := resolveTool(name)
	if path == "" {
		path = name
	}
	if name == "yt-dlp" {
		if ff, _ := resolveTool("ffmpeg"); ff != "" {
			args = append([]string{"--ffmpeg-location", filepath.Dir(ff)}, args...)
		}
	}
	return exec.CommandContext(ctx, path, args...)
}

// ParseToolVersion: "ffmpeg version n8.1.3-35-gf1acb0a266-20261114 Copyright ..." gives "n8.1.3-35-gf1acb0a266-20261114";
// a bare yt-dlp "2026.10.26" is returned as is.
func ParseToolVersion(out string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(out), "\n")
	f := strings.Fields(line)
	if len(f) >= 3 && f[1] == "version" {
		return f[2]
	}
	if len(f) >= 1 {
		return f[0]
	}
	return ""
}

func toolInfo(name, versionArg string) ToolInfo {
	p, src := resolveTool(name)
	if p == "" {
		return ToolInfo{}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, p, versionArg)
	hideWindow(cmd)
	out, _ := cmd.Output()
	return ToolInfo{Found: true, Source: src, Version: ParseToolVersion(string(out))}
}

var ytdlpTagRe = regexp.MustCompile(`^[0-9]{4}\.[0-9]{2}\.[0-9]{2}(\.[0-9]{1,6})?$`)

// ParseSums finds the SHA-256 for file in a "hash  name" checksum list (the yt-dlp SHA2-256SUMS format).
func ParseSums(text, file string) (string, bool) {
	for _, l := range strings.Split(text, "\n") {
		f := strings.Fields(l)
		if len(f) == 2 && strings.TrimPrefix(f[1], "*") == file && len(f[0]) == 64 {
			if _, err := hex.DecodeString(f[0]); err == nil {
				return strings.ToLower(f[0]), true
			}
		}
	}
	return "", false
}

// updateHostOK: where yt-dlp updates may come from, including redirect hops.
func updateHostOK(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || (u.Port() != "" && u.Port() != "443") {
		return false
	}
	h := strings.ToLower(u.Hostname())
	return h == "github.com" || h == "api.github.com" || strings.HasSuffix(h, ".githubusercontent.com")
}

var updateClient = &http.Client{
	Timeout: 5 * time.Minute,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 4 || !updateHostOK(req.URL.String()) {
			return fmt.Errorf("pengalihan ditolak (host: %s)", SafeHost(req.URL.String()))
		}
		return nil
	},
}

func updateGet(ctx context.Context, rawURL string, limit int64) ([]byte, error) {
	if !updateHostOK(rawURL) {
		return nil, fmt.Errorf("alamat pembaruan tidak diizinkan (host: %s)", SafeHost(rawURL))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Rokit-Updater/1.0")
	resp, err := updateClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tidak bisa menghubungi GitHub: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub menjawab HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, errors.New("file pembaruan terlalu besar")
	}
	return b, nil
}

// UpdateYtDlp downloads the latest official yt-dlp release into bin\, checks its SHA-256 against the
// release's own SHA2-256SUMS, runs --version on the new file, and only then replaces the old one.
func (r *Rokit) UpdateYtDlp(ctx context.Context) (ToolInfo, error) {
	dir := binDir()
	if st, err := os.Stat(dir); dir == "" || err != nil || !st.IsDir() {
		return ToolInfo{}, errors.New("folder bin bawaan tidak ada; pasang ulang aplikasi")
	}
	cctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	b, err := updateGet(cctx, "https://api.github.com/repos/yt-dlp/yt-dlp/releases/latest", 1<<20)
	if err != nil {
		return ToolInfo{}, err
	}
	var rel struct {
		Tag string `json:"tag_name"`
	}
	if json.Unmarshal(b, &rel) != nil || !ytdlpTagRe.MatchString(rel.Tag) {
		return ToolInfo{}, errors.New("jawaban GitHub tidak bisa dibaca")
	}
	dst := filepath.Join(dir, "yt-dlp.exe")
	if cur := toolInfo("yt-dlp", "--version"); cur.Source == toolBundled && cur.Version == rel.Tag {
		return cur, nil // already the latest
	}
	base := "https://github.com/yt-dlp/yt-dlp/releases/download/" + rel.Tag + "/"
	sums, err := updateGet(cctx, base+"SHA2-256SUMS", 1<<20)
	if err != nil {
		return ToolInfo{}, err
	}
	want, ok := ParseSums(string(sums), "yt-dlp.exe")
	if !ok {
		return ToolInfo{}, errors.New("daftar checksum rilis tidak memuat yt-dlp.exe")
	}
	exe, err := updateGet(cctx, base+"yt-dlp.exe", 200<<20)
	if err != nil {
		return ToolInfo{}, err
	}
	if sum := sha256.Sum256(exe); hex.EncodeToString(sum[:]) != want {
		return ToolInfo{}, errors.New("checksum yt-dlp.exe tidak cocok; pembaruan dibatalkan")
	}
	tmp := dst + ".new"
	defer os.Remove(tmp)
	if err := os.WriteFile(tmp, exe, 0o755); err != nil {
		return ToolInfo{}, fmt.Errorf("tidak bisa menulis ke folder bin: %w", err)
	}
	vcmd := exec.CommandContext(cctx, tmp, "--version") // the only run, after the hash matched
	hideWindow(vcmd)
	var out bytes.Buffer
	vcmd.Stdout = &out
	if err := vcmd.Run(); err != nil || ParseToolVersion(out.String()) != rel.Tag {
		return ToolInfo{}, errors.New("yt-dlp baru tidak bisa dijalankan; pembaruan dibatalkan")
	}
	if err := os.Rename(tmp, dst); err != nil {
		return ToolInfo{}, fmt.Errorf("tidak bisa mengganti yt-dlp.exe (sedang dipakai?): %w", err)
	}
	return ToolInfo{Found: true, Source: toolBundled, Version: rel.Tag}, nil
}
