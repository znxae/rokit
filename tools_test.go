package main

import "testing"

func TestParseToolVersion(t *testing.T) {
	for in, want := range map[string]string{
		"ffmpeg version n8.1.3-35-gf1acb0a266-20261114 Copyright (c) 2000-2026 the FFmpeg developers\nbuilt with gcc": "n8.1.3-35-gf1acb0a266-20261114",
		"2026.10.26\r\n": "2026.10.26", "": "", "ffprobe version 7.0 Copyright": "7.0",
	} {
		if got := ParseToolVersion(in); got != want {
			t.Errorf("%q: %q want %q", in, got, want)
		}
	}
}

func TestParseSums(t *testing.T) {
	h := "f5389d3698ecde4ef06f4e6b1ce0294fe9ac1396228c8990b8d3416d6d177a7d"
	sums := "aaaa  other\n" + h + "  yt-dlp.exe\n" + h + "  yt-dlp.exe.sig\n"
	if got, ok := ParseSums(sums, "yt-dlp.exe"); !ok || got != h {
		t.Errorf("%q %v", got, ok)
	}
	if got, ok := ParseSums("ABCDEF"+h[6:]+" *yt-dlp.exe", "yt-dlp.exe"); !ok || got != "abcdef"+h[6:] {
		t.Errorf("star/upper: %q %v", got, ok)
	}
	for _, bad := range []string{"", "short  yt-dlp.exe", "zz" + h[2:] + "  yt-dlp.exe", h + "  other.exe"} {
		if _, ok := ParseSums(bad, "yt-dlp.exe"); ok {
			t.Errorf("accepted %q", bad)
		}
	}
}

func TestUpdateHostsAndTags(t *testing.T) {
	for raw, want := range map[string]bool{
		"https://github.com/yt-dlp/yt-dlp/releases/download/2026.10.26/yt-dlp.exe": true,
		"https://api.github.com/repos/yt-dlp/yt-dlp/releases/latest":               true,
		"https://objects.githubusercontent.com/x":                                  true,
		"https://release-assets.githubusercontent.com/x":                           true,
		"http://github.com/x": false, "https://evilgithub.com/x": false, "https://github.com.evil.com/x": false,
		"https://user@github.com/x": false, "https://github.com:8443/x": false, "https://githubusercontent.com/x": false,
		"https://127.0.0.1/x": false,
	} {
		if got := updateHostOK(raw); got != want {
			t.Errorf("%s: %v want %v", raw, got, want)
		}
	}
	for tag, want := range map[string]bool{"2026.10.26": true, "2026.10.26.1": true, "v1": false, "2026.10.26/../x": false, "": false} {
		if ytdlpTagRe.MatchString(tag) != want {
			t.Errorf("tag %q", tag)
		}
	}
}
