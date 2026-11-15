package main

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// Real ffmpeg proof (skipped without ffmpeg): in "pitch" mode speed and pitch move together, so the inverse
// resample (what Roblox's PlaybackSpeed = 1/speed does) restores the original; with atempo it does not.
func TestRealSpeedModeRoundTrip(t *testing.T) {
	if p, _ := resolveTool("ffmpeg"); p == "" {
		t.Skip("no ffmpeg")
	}
	dir := t.TempDir()
	ctx := t.Context()
	src := filepath.Join(dir, "sine.wav")
	if out, err := toolCmd(ctx, "ffmpeg", "-v", "error", "-y", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=44100:duration=3", src).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	// duration in seconds and frequency from positive-going zero crossings of the decoded mono samples
	measure := func(path string) (dur, hz float64) {
		raw, err := toolCmd(ctx, "ffmpeg", "-v", "error", "-i", path, "-f", "s16le", "-ac", "1", "-ar", "44100", "-").Output()
		if err != nil {
			t.Fatal(err)
		}
		n := len(raw) / 2
		s := make([]int16, n)
		if err := binary.Read(bytes.NewReader(raw), binary.LittleEndian, s); err != nil {
			t.Fatal(err)
		}
		cross := 0
		for i := 1; i < n; i++ {
			if s[i-1] < 0 && s[i] >= 0 {
				cross++
			}
		}
		dur = float64(n) / 44100
		return dur, float64(cross) / dur
	}
	conv := func(in, name string, speed float64, mode string) string {
		out := filepath.Join(dir, name)
		if err := (&Rokit{}).Convert(ctx, in, out, Opts{Format: "wav", Speed: speed, Channels: "keep", SpeedMode: mode}, nil); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return out
	}
	near := func(label string, got, want, tol float64) {
		t.Logf("%-28s %.3f (want ~%.1f)", label, got, want)
		if got < want-tol || got > want+tol {
			t.Errorf("%s: %.3f, want %.1f ±%.1f", label, got, want, tol)
		}
	}
	d0, f0 := measure(src)
	near("original duration s", d0, 3, 0.05)
	near("original Hz", f0, 440, 4)

	fast := conv(src, "fast.wav", 2, "pitch")
	d1, f1 := measure(fast)
	near("pitch 2x duration s", d1, 1.5, 0.05)
	near("pitch 2x Hz", f1, 880, 8)
	back := conv(fast, "back.wav", 0.5, "pitch")
	d2, f2 := measure(back)
	near("pitch 2x then 0.5 duration s", d2, 3, 0.06)
	near("pitch 2x then 0.5 Hz", f2, 440, 5)

	tempo := conv(src, "tempo.wav", 2, "tempo")
	d3, f3 := measure(tempo)
	near("atempo 2x duration s", d3, 1.5, 0.05)
	near("atempo 2x Hz (pitch kept)", f3, 440, 6)
	tb := conv(tempo, "tempo_back.wav", 0.5, "pitch")
	d4, f4 := measure(tb)
	near("atempo 2x then 0.5 duration s", d4, 3, 0.06)
	near("atempo 2x then 0.5 Hz", f4, 220, 4)
	if _, err := os.Stat(back); err != nil {
		t.Fatal(err)
	}
}
