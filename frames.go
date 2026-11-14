package main

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// FrameOpts configure video -> images extraction.
type FrameOpts struct {
	Format    string  `json:"format"`    // png | jpg
	Quality   int     `json:"quality"`   // 1..100, jpg only
	FPS       float64 `json:"fps"`       // 0.5..60
	AllFrames bool    `json:"allFrames"` // every frame, ignores FPS
}

// FrameProgress is streamed while ffmpeg extracts.
type FrameProgress struct {
	Done  int `json:"done"`
	Total int `json:"total"` // estimate: duration x fps; 0 if unknown
}

// FramesResult is what the page shows afterwards.
type FramesResult struct {
	Dir   string `json:"dir"`
	Count int    `json:"count"`
}

func (o FrameOpts) Validate() error {
	if o.Format != "png" && o.Format != "jpg" {
		return fmt.Errorf("format gambar %q tidak dikenal", o.Format)
	}
	if o.Format == "jpg" && (o.Quality < 1 || o.Quality > 100) {
		return fmt.Errorf("kualitas %d di luar 1..100", o.Quality)
	}
	if !o.AllFrames && (o.FPS < 0.5 || o.FPS > 60) {
		return fmt.Errorf("fps %v di luar 0.5..60", o.FPS)
	}
	return nil
}

// JpegQ maps quality 100 (best) .. 1 (worst) onto ffmpeg's -q:v 2 (best) .. 31 (worst).
func JpegQ(quality int) int {
	q := min(max(quality, 1), 100)
	return 2 + int(math.Round(float64(100-q)*29/100))
}

// BuildFrameArgs returns the ffmpeg args that write frame_%05d.<ext> into outDir.
func BuildFrameArgs(input, outDir string, o FrameOpts) []string {
	a := []string{"-hide_banner", "-nostdin", "-y", "-i", input, "-an", "-sn"}
	if o.AllFrames {
		a = append(a, "-fps_mode", "passthrough")
	} else {
		a = append(a, "-vf", "fps="+num(o.FPS))
	}
	if o.Format == "jpg" {
		a = append(a, "-q:v", strconv.Itoa(JpegQ(o.Quality)))
	}
	return append(a, "-progress", "pipe:1", "-nostats", filepath.Join(outDir, "frame_%05d."+o.Format))
}

// EstimateFrames is duration x fps (or the stream's own rate for all frames); 0 if unknown.
func EstimateFrames(v VideoInfo, o FrameOpts) int {
	rate := o.FPS
	if o.AllFrames {
		rate = v.FPS
	}
	if v.Duration <= 0 || rate <= 0 {
		return 0
	}
	return max(1, int(math.Ceil(v.Duration*rate)))
}

// ParseFFmpegProgress reads the "frame=N" line of `-progress` output.
func ParseFFmpegProgress(line string) (int, bool) {
	v, ok := strings.CutPrefix(strings.TrimSpace(line), "frame=")
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	return n, err == nil
}

// ParseOutTime reads the "out_time_us=N" line of `-progress` output as seconds.
func ParseOutTime(line string) (float64, bool) {
	v, ok := strings.CutPrefix(strings.TrimSpace(line), "out_time_us=")
	if !ok {
		return 0, false
	}
	n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	return float64(n) / 1e6, err == nil && n >= 0 // ffmpeg prints N/A or a negative before the first packet
}

// UniqueDirPath returns parent/name, or parent/name (1), (2)... that does not exist yet.
func UniqueDirPath(parent, name string) string {
	p := filepath.Join(parent, name)
	for n := 1; exists(p); n++ {
		p = filepath.Join(parent, fmt.Sprintf("%s (%d)", name, n))
	}
	return p
}

// ResolveOutDir creates and returns the folder to write into without ever
// touching existing content: a missing or empty folder is used as is, anything
// else gets a "name (N)" sibling.
func ResolveOutDir(p string) (string, error) {
	if st, err := os.Stat(p); err == nil {
		if st.IsDir() {
			if es, err := os.ReadDir(p); err == nil && len(es) == 0 {
				return p, nil
			}
		}
		p = UniqueDirPath(filepath.Dir(p), filepath.Base(p))
	}
	return p, os.MkdirAll(p, 0o755)
}

func countFrames(dir, ext string) int {
	m, _ := filepath.Glob(filepath.Join(dir, "frame_*."+ext))
	return len(m)
}

var errNoFrames = errors.New("tidak ada frame yang dihasilkan (video tanpa gambar?)")

// ConvProgress is streamed while an audio conversion runs.
type ConvProgress struct {
	Percent float64 `json:"percent"` // 0..100
}
