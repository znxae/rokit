package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// DLOpts configure a video download.
type DLOpts struct {
	Quality   string `json:"quality"`   // best | 1080 | 720 | 480 | 360
	Container string `json:"container"` // mp4 | asis
}

// DLProgress is streamed while yt-dlp downloads.
type DLProgress struct {
	Percent float64 `json:"percent"`
	Speed   string  `json:"speed"`
	ETA     string  `json:"eta"`
}

// VideoMeta is what "Cek" shows.
type VideoMeta struct {
	Title     string  `json:"title"`
	Uploader  string  `json:"uploader"`
	Duration  float64 `json:"duration"`
	Thumbnail string  `json:"thumbnail"` // data URL, "" if unavailable
}

// Saved is the finished download.
type Saved struct {
	Path string `json:"path"`
}

const progressPrefix = "RKT|"

// FormatSelector builds the yt-dlp -f value for a quality choice.
func FormatSelector(q string) (string, error) {
	switch q {
	case "best":
		return "bv*+ba/b", nil
	case "1080", "720", "480", "360":
		return fmt.Sprintf("bv*[height<=%[1]s]+ba/b[height<=%[1]s]", q), nil
	}
	return "", fmt.Errorf("kualitas %q tidak dikenal", q)
}

// BuildDownloadArgs returns the yt-dlp args (never run through a shell). dir is
// the temp folder; the file is moved to Downloads only after success.
func BuildDownloadArgs(url, dir string, o DLOpts) ([]string, error) {
	url = strings.TrimSpace(url)
	if !ValidURL(url) {
		return nil, errors.New("URL harus diawali http:// atau https://")
	}
	sel, err := FormatSelector(o.Quality)
	if err != nil {
		return nil, err
	}
	if o.Container != "mp4" && o.Container != "asis" {
		return nil, fmt.Errorf("container %q tidak dikenal", o.Container)
	}
	a := []string{"-f", sel, "--no-playlist", "--no-colors", "--newline", "--progress", "--no-simulate", "--windows-filenames",
		"--progress-template", "download:" + progressPrefix + "%(progress._percent_str)s|%(progress._speed_str)s|%(progress._eta_str)s",
		"-o", dirSlash(dir) + "/%(title).80s.%(ext)s", "--print", "after_move:filepath"}
	if o.Container == "mp4" {
		// Prefer h264/aac so the merge into mp4 is a fast remux, not a recode.
		a = append(a, "--merge-output-format", "mp4", "-S", "vcodec:h264,res,acodec:m4a")
	}
	return append(a, "--", url), nil
}

func dirSlash(d string) string { return strings.ReplaceAll(d, `\`, "/") }

// ParseProgress reads one "RKT|12.3%|1.2MiB/s|00:12" line.
func ParseProgress(line string) (DLProgress, bool) {
	_, rest, ok := strings.Cut(line, progressPrefix)
	if !ok {
		return DLProgress{}, false
	}
	f := strings.Split(rest, "|")
	if len(f) < 3 {
		return DLProgress{}, false
	}
	p, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(f[0]), "%"), 64)
	if err != nil {
		return DLProgress{}, false
	}
	clean := func(s string) string {
		s = strings.TrimSpace(s)
		if s == "N/A" || s == "Unknown" {
			return ""
		}
		return s
	}
	return DLProgress{Percent: p, Speed: clean(f[1]), ETA: clean(f[2])}, true
}

// ParseMeta reads `yt-dlp -J` output; playlists are rejected. The second result
// is the thumbnail URL for the caller to fetch.
func ParseMeta(b []byte) (VideoMeta, string, error) {
	var raw struct {
		Type      string          `json:"_type"`
		Entries   json.RawMessage `json:"entries"`
		Title     string          `json:"title"`
		Uploader  string          `json:"uploader"`
		Duration  float64         `json:"duration"`
		Thumbnail string          `json:"thumbnail"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return VideoMeta{}, "", fmt.Errorf("jawaban yt-dlp tidak terbaca: %w", err)
	}
	if raw.Type == "playlist" || len(raw.Entries) > 0 {
		return VideoMeta{}, "", errors.New("ini playlist; tempel URL satu video")
	}
	return VideoMeta{Title: raw.Title, Uploader: raw.Uploader, Duration: raw.Duration}, raw.Thumbnail, nil
}
