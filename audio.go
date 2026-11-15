package main

import (
	"fmt"
	"math"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
)

// Opts are the conversion settings sent by the page.
type Opts struct {
	Format   string  `json:"format"`   // mp3 wav flac ogg m4a opus
	Bitrate  int     `json:"bitrate"`  // kbps, ignored for lossless
	Speed    float64 `json:"speed"`    // 0.25..4
	GainDB   float64 `json:"gainDb"`   // -20..20
	Channels string  `json:"channels"` // keep | mono | stereo
	// SpeedMode: "" or "tempo" keeps the pitch (atempo); "pitch" resamples, so speed and pitch change together
	// (what Roblox's Sound.PlaybackSpeed does).
	SpeedMode string `json:"speedMode"`
	// SampleRate of the input in Hz, set by Convert from ffprobe (never by the page); 0 = unknown.
	SampleRate int `json:"-"`
}

const (
	modeTempo = "tempo"
	modePitch = "pitch"
	// fallbackRate is used by the pitch mode when the input's sample rate is unknown.
	fallbackRate = 44100
)

// PitchFilters changes speed and pitch together: play the samples at rate*speed, then resample back to rate.
// With an unknown rate the input is first resampled to fallbackRate.
func PitchFilters(speed float64, rate int) []string {
	var f []string
	if rate <= 0 {
		rate = fallbackRate
		f = append(f, "aresample="+strconv.Itoa(rate))
	}
	return append(f, "asetrate="+strconv.Itoa(int(math.Round(float64(rate)*speed))), "aresample="+strconv.Itoa(rate))
}

var codecs = map[string]string{
	"mp3": "libmp3lame", "wav": "pcm_s16le", "flac": "flac",
	"ogg": "libvorbis", "m4a": "aac", "opus": "libopus",
}

func isLossless(f string) bool { return f == "wav" || f == "flac" }

func num(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

// AtempoChain splits a speed into atempo filters; each accepts only 0.5..2.0.
func AtempoChain(speed float64) []string {
	var v []string
	for speed > 2.0 {
		v = append(v, "atempo=2")
		speed /= 2.0
	}
	for speed < 0.5 {
		v = append(v, "atempo=0.5")
		speed /= 0.5
	}
	return append(v, "atempo="+num(speed))
}

// BuildArgs returns the full ffmpeg argument list (pure; stdin closed via -nostdin).
func BuildArgs(input, output string, o Opts) []string {
	a := []string{"-hide_banner", "-nostdin", "-y", "-i", input, "-vn"}
	var filters []string
	if math.Abs(o.Speed-1) > 1e-9 {
		if o.SpeedMode == modePitch {
			filters = append(filters, PitchFilters(o.Speed, o.SampleRate)...)
		} else {
			filters = append(filters, AtempoChain(o.Speed)...)
		}
	}
	if math.Abs(o.GainDB) > 1e-9 {
		filters = append(filters, "volume="+num(o.GainDB)+"dB")
	}
	if len(filters) > 0 {
		a = append(a, "-af", strings.Join(filters, ","))
	}
	switch o.Channels {
	case "mono":
		a = append(a, "-ac", "1")
	case "stereo":
		a = append(a, "-ac", "2")
	}
	a = append(a, "-c:a", codecs[o.Format])
	if !isLossless(o.Format) {
		a = append(a, "-b:a", fmt.Sprintf("%dk", o.Bitrate))
	}
	return append(a, output)
}

// Validate rejects anything the UI could not have produced (trust boundary).
func (o Opts) Validate() error {
	if _, ok := codecs[o.Format]; !ok {
		return fmt.Errorf("unknown format %q", o.Format)
	}
	if o.Speed < 0.25 || o.Speed > 4 {
		return fmt.Errorf("speed %v out of range 0.25..4", o.Speed)
	}
	switch o.SpeedMode {
	case "", modeTempo, modePitch:
	default:
		return fmt.Errorf("unknown speed mode %q", o.SpeedMode)
	}
	if o.GainDB < -20 || o.GainDB > 20 {
		return fmt.Errorf("gain %v out of range -20..20", o.GainDB)
	}
	switch o.Channels {
	case "keep", "mono", "stereo":
	default:
		return fmt.Errorf("unknown channels %q", o.Channels)
	}
	if !isLossless(o.Format) && (o.Bitrate < 32 || o.Bitrate > 512) {
		return fmt.Errorf("bitrate %d out of range", o.Bitrate)
	}
	return nil
}

// SpeedHint explains the chosen mode. mode is "pitch" (Roblox) or anything else (Normal, pitch kept).
func SpeedHint(speed float64, mode string) string {
	if math.Abs(speed-1) < 5e-4 {
		return "Speed normal (1x)"
	}
	if mode != modePitch {
		return "Nada tetap, hanya kecepatan yang berubah."
	}
	inv := 1 / speed
	dir := "lebih lambat"
	if inv > 1 {
		dir = "lebih cepat"
	}
	return fmt.Sprintf("Untuk kembali ke suara asli di Roblox Studio, setel PlaybackSpeed ke %.3f (%s).", inv, dir)
}

// ValidURL accepts only plain http(s) URLs without whitespace/control chars.
func ValidURL(s string) bool {
	s = strings.TrimSpace(s)
	if !(strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")) || len(s) <= 8 {
		return false
	}
	for _, r := range s {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// SanitizeFilename makes a title safe as a Windows file name (max 80 runes).
// shortcut: reserved names (CON, NUL, ...) are not special-cased; add if titles hit them.
func SanitizeFilename(s string) string {
	r := []rune(strings.Map(func(c rune) rune {
		if unicode.IsControl(c) || strings.ContainsRune(`<>:"/\|?*`, c) {
			return '_'
		}
		return c
	}, s))
	t := strings.Trim(strings.TrimSpace(string(r)), ".")
	if rs := []rune(t); len(rs) > 80 {
		t = string(rs[:80])
	}
	t = strings.TrimRight(strings.TrimSpace(t), ".")
	if t == "" {
		return "audio"
	}
	return t
}

// exists is a variable so tests can fake the filesystem.
var exists = func(p string) bool { _, err := statFile(p); return err == nil }

// UniquePath returns dir/stem.ext, or dir/stem (1).ext, (2)... so nothing is overwritten.
func UniquePath(dir, stem, ext string) string {
	p := filepath.Join(dir, stem+"."+ext)
	for n := 1; exists(p); n++ {
		p = filepath.Join(dir, fmt.Sprintf("%s (%d).%s", stem, n, ext))
	}
	return p
}

// DefaultOutput is input with a new extension; never equal to the input.
func DefaultOutput(input, format string) string {
	ext := filepath.Ext(input)
	base := strings.TrimSuffix(input, ext)
	p := base + "." + format
	if samePath(p, input) {
		p = base + "_out." + format
	}
	return p
}

func samePath(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}
