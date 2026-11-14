package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
)

const (
	ffmpegMsg = "ffmpeg tidak ditemukan di PATH. Pasang dengan `winget install Gyan.FFmpeg` (macOS: `brew install ffmpeg`), lalu restart aplikasi."
	ytdlpMsg  = "yt-dlp tidak ditemukan di PATH. Pasang dengan `winget install yt-dlp.yt-dlp`, lalu restart aplikasi."
)

func statFile(p string) (os.FileInfo, error) { return os.Stat(p) }

// Tools says which external programs are available.
type Tools struct {
	FFmpeg bool `json:"ffmpeg"`
	YtDlp  bool `json:"ytdlp"`
}

// Downloaded is the result of fetching a URL.
type Downloaded struct {
	Path  string `json:"path"`
	Title string `json:"title"`
}

// Rokit is the service the page calls.
type Rokit struct {
	mu    sync.Mutex
	temps map[string]bool // downloads we own and may delete
}

func (r *Rokit) isTemp(p string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.temps[filepath.Clean(p)]
}

func (r *Rokit) setTemp(p string, on bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.temps == nil {
		r.temps = map[string]bool{}
	}
	if on {
		r.temps[filepath.Clean(p)] = true
	} else {
		delete(r.temps, filepath.Clean(p))
	}
}

// removeTemp deletes a download we made (never any other file).
func (r *Rokit) removeTemp(p string) {
	if p == "" || !r.isTemp(p) {
		return
	}
	_ = os.Remove(p)
	_ = os.Remove(filepath.Dir(p)) // only succeeds when empty
	r.setTemp(p, false)
}

// CheckTools reports whether ffmpeg and yt-dlp are on PATH.
func (r *Rokit) CheckTools() Tools {
	_, e1 := exec.LookPath("ffmpeg")
	_, e2 := exec.LookPath("yt-dlp")
	return Tools{FFmpeg: e1 == nil, YtDlp: e2 == nil}
}

// PickFile opens the native file dialog; "" means cancelled.
func (r *Rokit) PickFile() (string, error) {
	sel, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Title: "Pilih file audio",
		Filters: []mygo.FileFilter{
			{Name: "Audio/video", Extensions: []string{"mp3", "wav", "flac", "ogg", "m4a", "opus", "aac", "wma", "mp4", "mkv", "webm"}},
			{Name: "Semua file", Extensions: []string{"*"}},
		},
	})
	if err != nil || len(sel) == 0 {
		return "", err
	}
	return sel[0], nil
}

// PickOutput opens the native save dialog; "" means cancelled.
func (r *Rokit) PickOutput(suggested string) (string, error) {
	return mygo.Dialog.Save(mygo.SaveDialogOptions{Title: "Simpan sebagai", DefaultPath: suggested})
}

// SuggestOutput is the default output path for input in format: next to a local
// file, or a unique name in Downloads for a downloaded one.
func (r *Rokit) SuggestOutput(input, format string) string {
	if r.isTemp(input) {
		stem := strings.TrimSuffix(filepath.Base(input), filepath.Ext(input))
		return UniquePath(downloadsDir(), SanitizeFilename(stem), format)
	}
	return DefaultOutput(input, format)
}

// IsDownload says whether path is a temp download this app made.
func (r *Rokit) IsDownload(path string) bool { return r.isTemp(path) }

// Hint is the speed-inverse hint text.
func (r *Rokit) Hint(speed float64) string { return SpeedHint(speed) }

// IsURL lets the page validate with the same rule the backend enforces.
func (r *Rokit) IsURL(s string) bool { return ValidURL(s) }

func downloadsDir() string {
	if h, err := os.UserHomeDir(); err == nil {
		d := filepath.Join(h, "Downloads")
		if st, err := os.Stat(d); err == nil && st.IsDir() {
			return d
		}
	}
	return os.TempDir()
}

// Download fetches the best audio of url with yt-dlp into a temp dir.
func (r *Rokit) Download(ctx context.Context, url string) (Downloaded, error) {
	url = strings.TrimSpace(url)
	if !ValidURL(url) {
		return Downloaded{}, errors.New("URL harus diawali http:// atau https://")
	}
	dir, err := os.MkdirTemp("", "rokit_dl_")
	if err != nil {
		return Downloaded{}, err
	}
	args := []string{"-f", "bestaudio/best", "--no-playlist", "--no-simulate", "--windows-filenames",
		"-o", filepath.ToSlash(dir) + "/%(title).80s.%(ext)s", "--print", "after_move:filepath", "--", url}
	cmd := exec.CommandContext(ctx, "yt-dlp", args...)
	hideWindow(cmd)
	cmd.Env = append(os.Environ(), "PYTHONIOENCODING=utf-8")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		_ = os.RemoveAll(dir)
		if errors.Is(err, exec.ErrNotFound) {
			return Downloaded{}, errors.New(ytdlpMsg)
		}
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return Downloaded{}, errors.New(msg)
		}
		return Downloaded{}, err
	}
	path := lastLine(stdout.String())
	if st, err := os.Stat(path); err != nil || st.IsDir() {
		// Printed path mangled by console encoding: use the dir's only file.
		path = ""
		if es, _ := os.ReadDir(dir); len(es) > 0 {
			path = filepath.Join(dir, es[0].Name())
		}
	}
	if path == "" {
		_ = os.RemoveAll(dir)
		return Downloaded{}, errors.New("yt-dlp selesai tetapi tidak menghasilkan file")
	}
	r.setTemp(path, true)
	return Downloaded{Path: path, Title: strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))}, nil
}

func lastLine(s string) string {
	lines := strings.Split(strings.ReplaceAll(s, "\r", ""), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if t := strings.TrimSpace(lines[i]); t != "" {
			return t
		}
	}
	return ""
}

// Convert runs ffmpeg. A downloaded temp input is deleted after success.
func (r *Rokit) Convert(ctx context.Context, input, output string, o Opts) error {
	if err := o.Validate(); err != nil {
		return err
	}
	if input == "" || output == "" {
		return errors.New("pilih file input dan output dulu")
	}
	if samePath(input, output) {
		return errors.New("path output sama dengan file input; pilih path lain")
	}
	if st, err := os.Stat(input); err != nil || st.IsDir() {
		return fmt.Errorf("file input tidak ditemukan: %s", input)
	}
	if st, err := os.Stat(filepath.Dir(output)); err != nil || !st.IsDir() {
		return fmt.Errorf("folder output tidak ada: %s", filepath.Dir(output))
	}
	cctx, cancel := context.WithTimeout(ctx, 2*time.Hour)
	defer cancel()
	cmd := exec.CommandContext(cctx, "ffmpeg", BuildArgs(input, output, o)...)
	hideWindow(cmd)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return errors.New(ffmpegMsg)
		}
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return errors.New(msg)
		}
		return err
	}
	r.removeTemp(input)
	return nil
}
