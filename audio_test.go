package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func opts(format string) Opts {
	return Opts{Format: format, Bitrate: 128, Speed: 1, Channels: "keep"}
}

func hasPair(a []string, k, v string) bool {
	for i := 0; i+1 < len(a); i++ {
		if a[i] == k && a[i+1] == v {
			return true
		}
	}
	return false
}

func TestCodecsPerFormat(t *testing.T) {
	for f, c := range map[string]string{"mp3": "libmp3lame", "wav": "pcm_s16le", "flac": "flac", "ogg": "libvorbis", "m4a": "aac", "opus": "libopus"} {
		if !hasPair(BuildArgs("i", "o", opts(f)), "-c:a", c) {
			t.Errorf("%s: want codec %s", f, c)
		}
	}
}

// Video inputs must yield audio only: -vn comes after -i and before the codec, for every format.
func TestVideoInputDropsVideoStream(t *testing.T) {
	for f := range codecs {
		a := BuildArgs("clip.mp4", "o."+f, opts(f))
		vn, in, codec := slices.Index(a, "-vn"), slices.Index(a, "-i"), slices.Index(a, "-c:a")
		if vn < 0 || in < 0 || !(in < vn && vn < codec) {
			t.Errorf("%s: bad -vn position in %v", f, a)
		}
	}
}

func TestBitrateOmittedForLossless(t *testing.T) {
	for _, f := range []string{"wav", "flac"} {
		if slices.Contains(BuildArgs("i", "o", opts(f)), "-b:a") {
			t.Errorf("%s must not have -b:a", f)
		}
	}
	if !hasPair(BuildArgs("i", "o", opts("mp3")), "-b:a", "128k") {
		t.Error("mp3 needs -b:a 128k")
	}
}

func TestAtempoChaining(t *testing.T) {
	for speed, want := range map[float64][]string{
		1.5:  {"atempo=1.5"},
		4:    {"atempo=2", "atempo=2"},
		3:    {"atempo=2", "atempo=1.5"},
		0.25: {"atempo=0.5", "atempo=0.5"},
	} {
		if got := AtempoChain(speed); !slices.Equal(got, want) {
			t.Errorf("%v: got %v want %v", speed, got, want)
		}
	}
	o := opts("mp3")
	o.Speed = 0.25
	if !hasPair(BuildArgs("i", "o", o), "-af", "atempo=0.5,atempo=0.5") {
		t.Error("0.25x chain missing")
	}
}

func TestVolumeAndNoFilterByDefault(t *testing.T) {
	if slices.Contains(BuildArgs("i", "o", opts("mp3")), "-af") {
		t.Error("no filter expected at defaults")
	}
	o := opts("mp3")
	o.GainDB, o.Speed = -6.5, 0.5
	if !hasPair(BuildArgs("i", "o", o), "-af", "atempo=0.5,volume=-6.5dB") {
		t.Errorf("got %v", BuildArgs("i", "o", o))
	}
}

func TestChannels(t *testing.T) {
	if slices.Contains(BuildArgs("i", "o", opts("mp3")), "-ac") {
		t.Error("keep must omit -ac")
	}
	o := opts("mp3")
	o.Channels = "mono"
	if !hasPair(BuildArgs("i", "o", o), "-ac", "1") {
		t.Error("mono")
	}
	o.Channels = "stereo"
	if !hasPair(BuildArgs("i", "o", o), "-ac", "2") {
		t.Error("stereo")
	}
}

func TestSpeedHint(t *testing.T) {
	const tempo15 = "Pitch tetap. Roblox PlaybackSpeed mengubah pitch, jadi jangan pakai mode ini kalau Anda akan memperlambat di Roblox. Di alat lain yang menjaga pitch: ubah speed ke 0.667x (lebih lambat)"
	for _, k := range []struct {
		speed      float64
		mode, want string
	}{
		{1, "tempo", "Speed normal (1x)"},
		{1, "pitch", "Speed normal (1x)"},
		{1.5, "tempo", tempo15},
		{1.5, "", tempo15},
		{0.5, "tempo", "Pitch tetap. Roblox PlaybackSpeed mengubah pitch, jadi jangan pakai mode ini kalau Anda akan memperlambat di Roblox. Di alat lain yang menjaga pitch: ubah speed ke 2.000x (lebih cepat)"},
		{2, "pitch", "Di Roblox Studio, setel PlaybackSpeed ke 0.500 untuk kembali ke suara asli (pitch dan kecepatan kembali normal)"},
		{4, "pitch", "Di Roblox Studio, setel PlaybackSpeed ke 0.250 untuk kembali ke suara asli (pitch dan kecepatan kembali normal)"},
		{0.25, "pitch", "Di Roblox Studio, setel PlaybackSpeed ke 4.000 untuk kembali ke suara asli (pitch dan kecepatan kembali normal)"},
	} {
		if got := SpeedHint(k.speed, k.mode); got != k.want {
			t.Errorf("%v %s: got %q", k.speed, k.mode, got)
		}
	}
}

func TestPitchModeFilters(t *testing.T) {
	o := opts("wav")
	o.SpeedMode, o.Speed, o.SampleRate, o.GainDB = "pitch", 2, 44100, -6
	a := BuildArgs("i", "o", o)
	if !hasPair(a, "-af", "asetrate=88200,aresample=44100,volume=-6dB") {
		t.Errorf("pitch filter: %v", a)
	}
	for speed, want := range map[float64]string{0.25: "asetrate=11025,aresample=44100", 4: "asetrate=176400,aresample=44100", 1.5: "asetrate=66150,aresample=44100"} {
		o.GainDB, o.Speed = 0, speed
		if !hasPair(BuildArgs("i", "o", o), "-af", want) {
			t.Errorf("%vx: want %s in %v", speed, want, BuildArgs("i", "o", o))
		}
	}
	// 48 kHz input keeps its own rate
	o.Speed, o.SampleRate = 2, 48000
	if !hasPair(BuildArgs("i", "o", o), "-af", "asetrate=96000,aresample=48000") {
		t.Error("48 kHz")
	}
	// unknown rate: resample to 44100 first, say so
	o.SampleRate = 0
	if !hasPair(BuildArgs("i", "o", o), "-af", "aresample=44100,asetrate=88200,aresample=44100") {
		t.Error("unknown rate fallback")
	}
	for _, mode := range []string{"pitch"} {
		o.SpeedMode = mode
		for _, x := range BuildArgs("i", "o", o) {
			if strings.Contains(x, "atempo") {
				t.Errorf("atempo must not appear in pitch mode: %v", x)
			}
		}
	}
	// the default and "tempo" keep atempo and never asetrate
	for _, mode := range []string{"", "tempo"} {
		o.SpeedMode = mode
		if !hasPair(BuildArgs("i", "o", o), "-af", "atempo=2") {
			t.Errorf("mode %q must use atempo", mode)
		}
	}
	// speed 1 adds no filter in either mode
	o.Speed, o.SpeedMode = 1, "pitch"
	if slices.Contains(BuildArgs("i", "o", o), "-af") {
		t.Error("1x must not filter")
	}
	o.Speed, o.SpeedMode = 2, "x"
	if o.Validate() == nil {
		t.Error("unknown mode must be rejected")
	}
}

func TestProbeSampleRate(t *testing.T) {
	v, err := ParseProbe([]byte(`{"streams":[{"codec_type":"video","width":1,"height":1},{"codec_type":"audio","sample_rate":"48000"}],"format":{"duration":"1"}}`))
	if err != nil || v.SampleRate != 48000 || !v.HasAudio {
		t.Fatalf("%+v %v", v, err)
	}
}

func TestValidURL(t *testing.T) {
	for _, ok := range []string{"https://www.youtube.com/watch?v=abc", "  http://example.com/a  "} {
		if !ValidURL(ok) {
			t.Errorf("%q should be valid", ok)
		}
	}
	for _, bad := range []string{"", "https://", "ftp://x.com/a", "javascript:alert(1)", "https://a.com/x y", "file:///C:/a.mp3", "www.youtube.com", "-o evil"} {
		if ValidURL(bad) {
			t.Errorf("%q should be invalid", bad)
		}
	}
}

func TestSanitizeFilename(t *testing.T) {
	for in, want := range map[string]string{
		`a<b>:c"d/e\f|g?h*i`: "a_b__c_d_e_f_g_h_i",
		"  ..Title.. ":       "Title",
		"???":                "___",
		"  . ":               "audio",
		"tab\there":          "tab_here",
		"Lagu — Ünïcode 音楽":  "Lagu — Ünïcode 音楽",
	} {
		if got := SanitizeFilename(in); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
	if n := len([]rune(SanitizeFilename(strings.Repeat("x", 200)))); n != 80 {
		t.Errorf("length %d", n)
	}
}

func TestUniquePathNeverExists(t *testing.T) {
	d := t.TempDir()
	if got := UniquePath(d, "t", "mp3"); got != filepath.Join(d, "t.mp3") {
		t.Fatal(got)
	}
	os.WriteFile(filepath.Join(d, "t.mp3"), []byte("x"), 0o644)
	if got := UniquePath(d, "t", "mp3"); got != filepath.Join(d, "t (1).mp3") {
		t.Fatal(got)
	}
	os.WriteFile(filepath.Join(d, "t (1).mp3"), []byte("x"), 0o644)
	if got := UniquePath(d, "t", "mp3"); got != filepath.Join(d, "t (2).mp3") {
		t.Fatal(got)
	}
}

func TestDefaultOutputNeverInput(t *testing.T) {
	if got := DefaultOutput(filepath.Join("a", "b.mp3"), "wav"); got != filepath.Join("a", "b.wav") {
		t.Error(got)
	}
	if got := DefaultOutput(filepath.Join("a", "b.mp3"), "mp3"); got != filepath.Join("a", "b_out.mp3") {
		t.Error(got)
	}
}

func TestValidate(t *testing.T) {
	if err := opts("mp3").Validate(); err != nil {
		t.Fatal(err)
	}
	bad := []Opts{{Format: "exe", Speed: 1, Channels: "keep"}, {Format: "wav", Speed: 9, Channels: "keep"},
		{Format: "wav", Speed: 1, GainDB: 99, Channels: "keep"}, {Format: "wav", Speed: 1, Channels: "x"},
		{Format: "mp3", Bitrate: 1, Speed: 1, Channels: "keep"}}
	for _, o := range bad {
		if o.Validate() == nil {
			t.Errorf("%+v should be rejected", o)
		}
	}
}

func TestRemoveTempOnlyOwnFiles(t *testing.T) {
	d := t.TempDir()
	own, other := filepath.Join(d, "own.m4a"), filepath.Join(d, "other.m4a")
	os.WriteFile(own, []byte("x"), 0o644)
	os.WriteFile(other, []byte("x"), 0o644)
	r := &Rokit{}
	r.setTemp(own, true)
	r.removeTemp(other)
	r.removeTemp(own)
	if _, err := os.Stat(other); err != nil {
		t.Error("non-temp file must survive")
	}
	if _, err := os.Stat(own); err == nil {
		t.Error("temp file must be deleted")
	}
}

// Needs ffmpeg/ffprobe: AE_TEST_INPUT=file go test -run Real -v
func TestRealFFmpegPipeline(t *testing.T) {
	input := os.Getenv("AE_TEST_INPUT")
	if input == "" {
		t.Skip("set AE_TEST_INPUT")
	}
	probe := func(path, sel string) float64 {
		out, err := exec.Command("ffprobe", "-v", "error", "-select_streams", "a:0", "-show_entries", sel, "-of", "csv=p=0", path).Output()
		if err != nil {
			t.Fatal(err)
		}
		f, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
		if err != nil {
			t.Fatalf("probe %s: %q", path, out)
		}
		return f
	}
	src := probe(input, "format=duration")
	type c struct {
		f        string
		speed, g float64
		ch       string
		wantCh   float64
	}
	dir := t.TempDir()
	for i, k := range []c{{"mp3", 1, 0, "keep", 2}, {"wav", 1, 0, "keep", 2}, {"flac", 1, 0, "keep", 2}, {"ogg", 1, 0, "keep", 2},
		{"m4a", 1, 0, "keep", 2}, {"opus", 1, 0, "keep", 2}, {"mp3", 0.25, 0, "keep", 2}, {"wav", 4, -6, "keep", 2}, {"mp3", 1.5, 3, "mono", 1}} {
		out := filepath.Join(dir, "case"+strconv.Itoa(i)+"."+k.f)
		r := &Rokit{}
		if err := r.Convert(t.Context(), input, out, Opts{Format: k.f, Bitrate: 128, Speed: k.speed, GainDB: k.g, Channels: k.ch}, nil); err != nil {
			t.Fatalf("%s x%v: %v", k.f, k.speed, err)
		}
		if d := probe(out, "format=duration"); d < src/k.speed-0.4 || d > src/k.speed+0.4 {
			t.Errorf("%s x%v: duration %v want %v", k.f, k.speed, d, src/k.speed)
		}
		if ch := probe(out, "stream=channels"); ch != k.wantCh {
			t.Errorf("%s channels %v", k.f, ch)
		}
	}
}
