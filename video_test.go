package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestJpegQ(t *testing.T) {
	for q, want := range map[int]int{100: 2, 1: 31, 0: 31, 500: 2, 50: 17} {
		if got := JpegQ(q); got != want {
			t.Errorf("quality %d: got %d want %d", q, got, want)
		}
	}
}

func TestBuildFrameArgs(t *testing.T) {
	a := BuildFrameArgs("v.mp4", "out", FrameOpts{Format: "png", FPS: 5})
	if !hasPair(a, "-vf", "fps=5") || slices.Contains(a, "-q:v") || !slices.Contains(a, "-an") {
		t.Errorf("png: %v", a)
	}
	if last := a[len(a)-1]; last != filepath.Join("out", "frame_%05d.png") {
		t.Errorf("pattern %q", last)
	}
	j := BuildFrameArgs("v.mp4", "out", FrameOpts{Format: "jpg", Quality: 90, FPS: 0.5})
	if !hasPair(j, "-vf", "fps=0.5") || !hasPair(j, "-q:v", "5") || !strings.HasSuffix(j[len(j)-1], "frame_%05d.jpg") {
		t.Errorf("jpg: %v", j)
	}
	all := BuildFrameArgs("v.mp4", "out", FrameOpts{Format: "png", AllFrames: true, FPS: 30})
	if slices.Contains(all, "-vf") || !hasPair(all, "-fps_mode", "passthrough") {
		t.Errorf("all frames must not use fps filter: %v", all)
	}
	if !hasPair(a, "-progress", "pipe:1") {
		t.Error("progress missing")
	}
}

func TestFrameOptsValidate(t *testing.T) {
	good := []FrameOpts{{Format: "png", FPS: 1}, {Format: "jpg", Quality: 80, FPS: 60}, {Format: "png", AllFrames: true}}
	for _, o := range good {
		if err := o.Validate(); err != nil {
			t.Errorf("%+v: %v", o, err)
		}
	}
	bad := []FrameOpts{{Format: "gif", FPS: 1}, {Format: "png", FPS: 0.1}, {Format: "png", FPS: 61}, {Format: "jpg", Quality: 0, FPS: 1}}
	for _, o := range bad {
		if o.Validate() == nil {
			t.Errorf("%+v should be rejected", o)
		}
	}
}

func TestEstimateFrames(t *testing.T) {
	v := VideoInfo{Duration: 10, FPS: 29.97}
	if got := EstimateFrames(v, FrameOpts{FPS: 5}); got != 50 {
		t.Error(got)
	}
	if got := EstimateFrames(v, FrameOpts{AllFrames: true}); got != 300 {
		t.Error(got)
	}
	if EstimateFrames(VideoInfo{}, FrameOpts{FPS: 5}) != 0 {
		t.Error("unknown duration must give 0")
	}
}

func TestParseFFmpegProgress(t *testing.T) {
	if n, ok := ParseFFmpegProgress("frame=123\r"); !ok || n != 123 {
		t.Error(n, ok)
	}
	for _, s := range []string{"fps=30", "progress=end", "frame=x", ""} {
		if _, ok := ParseFFmpegProgress(s); ok {
			t.Errorf("%q", s)
		}
	}
}

func TestResolveOutDirNeverTouchesExisting(t *testing.T) {
	base := t.TempDir()
	p := filepath.Join(base, "clip")
	got, err := ResolveOutDir(p)
	if err != nil || got != p {
		t.Fatal(got, err)
	}
	// existing but empty: reuse
	if got, _ := ResolveOutDir(p); got != p {
		t.Error("empty folder should be reused", got)
	}
	os.WriteFile(filepath.Join(p, "frame_00001.png"), []byte("x"), 0o644)
	got, err = ResolveOutDir(p)
	if err != nil || got != filepath.Join(base, "clip (1)") {
		t.Fatal(got, err)
	}
	if _, err := os.Stat(got); err != nil {
		t.Error("new folder must be created")
	}
	if b, _ := os.ReadFile(filepath.Join(p, "frame_00001.png")); string(b) != "x" {
		t.Error("existing file touched")
	}
}

func TestParseProbe(t *testing.T) {
	js := `{"streams":[{"codec_type":"video","avg_frame_rate":"30000/1001","r_frame_rate":"30/1","width":1280,"height":720},
	  {"codec_type":"audio"}],"format":{"duration":"12.5"}}`
	v, err := ParseProbe([]byte(js))
	if err != nil || !v.HasVideo || !v.HasAudio || v.Duration != 12.5 || v.Width != 1280 || v.FPS < 29.9 || v.FPS > 30 {
		t.Fatalf("%+v %v", v, err)
	}
	v, _ = ParseProbe([]byte(`{"streams":[{"codec_type":"video","avg_frame_rate":"0/0","r_frame_rate":"25/1"}],"format":{}}`))
	if v.HasAudio || v.FPS != 25 {
		t.Errorf("silent video: %+v", v)
	}
	if _, err := ParseProbe([]byte("nope")); err == nil {
		t.Error("garbage must fail")
	}
}

func TestNoAudioMapping(t *testing.T) {
	if !NoAudioStderr("Output file #0 does not contain any stream") {
		t.Error("should map")
	}
	if NoAudioStderr("Invalid data found when processing input") {
		t.Error("must not map other errors")
	}
}

func TestFormatSelector(t *testing.T) {
	for q, want := range map[string]string{
		"best": "bv*+ba/b",
		"1080": "bv*[height<=1080]+ba/b[height<=1080]",
		"720":  "bv*[height<=720]+ba/b[height<=720]",
		"480":  "bv*[height<=480]+ba/b[height<=480]",
		"360":  "bv*[height<=360]+ba/b[height<=360]",
	} {
		if got, err := FormatSelector(q); err != nil || got != want {
			t.Errorf("%s: %q %v", q, got, err)
		}
	}
	for _, bad := range []string{"", "144", "720; rm", "bv*"} {
		if _, err := FormatSelector(bad); err == nil {
			t.Errorf("%q should be rejected", bad)
		}
	}
}

func TestBuildDownloadArgs(t *testing.T) {
	a, err := BuildDownloadArgs("https://youtu.be/x", `C:\tmp\d`, DLOpts{Quality: "720", Container: "mp4"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(a, "--no-playlist") || !hasPair(a, "--merge-output-format", "mp4") ||
		!hasPair(a, "-f", "bv*[height<=720]+ba/b[height<=720]") || !hasPair(a, "-o", "C:/tmp/d/%(title).80s.%(ext)s") {
		t.Errorf("%v", a)
	}
	if n := len(a); a[n-2] != "--" || a[n-1] != "https://youtu.be/x" {
		t.Errorf("url must follow --: %v", a[len(a)-3:])
	}
	asis, _ := BuildDownloadArgs("https://youtu.be/x", "d", DLOpts{Quality: "best", Container: "asis"})
	if slices.Contains(asis, "--merge-output-format") || slices.Contains(asis, "-S") {
		t.Errorf("as-is must not merge/sort: %v", asis)
	}
	for _, u := range []string{"-o evil", "file:///x", "", "https://a b"} {
		if _, err := BuildDownloadArgs(u, "d", DLOpts{Quality: "best", Container: "mp4"}); err == nil {
			t.Errorf("%q should be rejected", u)
		}
	}
	if _, err := BuildDownloadArgs("https://a.com/x", "d", DLOpts{Quality: "best", Container: "avi"}); err == nil {
		t.Error("unknown container accepted")
	}
}

func TestParseProgress(t *testing.T) {
	p, ok := ParseProgress("RKT|  12.3%|1.20MiB/s|00:12")
	if !ok || p.Percent != 12.3 || p.Speed != "1.20MiB/s" || p.ETA != "00:12" {
		t.Errorf("%+v %v", p, ok)
	}
	p, ok = ParseProgress("RKT|100%|N/A|Unknown")
	if !ok || p.Percent != 100 || p.Speed != "" || p.ETA != "" {
		t.Errorf("%+v %v", p, ok)
	}
	for _, s := range []string{"[download] 10%", "RKT|x|y|z", "RKT|5%"} {
		if _, ok := ParseProgress(s); ok {
			t.Errorf("%q", s)
		}
	}
}

func TestParseMeta(t *testing.T) {
	m, thumb, err := ParseMeta([]byte(`{"title":"Halo","uploader":"A","duration":61.5,"thumbnail":"https://i/x.jpg"}`))
	if err != nil || m.Title != "Halo" || m.Duration != 61.5 || thumb != "https://i/x.jpg" {
		t.Fatalf("%+v %q %v", m, thumb, err)
	}
	for _, js := range []string{`{"_type":"playlist","title":"x"}`, `{"title":"x","entries":[{}]}`, "nope"} {
		if _, _, err := ParseMeta([]byte(js)); err == nil {
			t.Errorf("%s should fail", js)
		}
	}
}
