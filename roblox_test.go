package main

import (
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestParseAssetRef(t *testing.T) {
	ok := []struct {
		in string
		id int64
	}{
		{"1837879082", 1837879082},
		{"  42  ", 42},
		{"rbxassetid://1837879082", 1837879082},
		{"RBXASSETID://7", 7},
		{"https://www.roblox.com/library/1837879082/Paradise-Falls", 1837879082},
		{"https://roblox.com/library/12345", 12345},
		{"https://www.roblox.com/catalog/63690008/Pal-Hair", 63690008},
		{"https://www.roblox.com/marketplace/asset/99", 99},
		{"https://create.roblox.com/store/asset/1837879082/Paradise-Falls", 1837879082},
		{"https://create.roblox.com/marketplace/asset/5", 5},
		{"https://www.roblox.com/asset/?id=1837879082", 1837879082},
		{"https://www.roblox.com/asset?id=77&version=2", 77},
		{"https://assetdelivery.roblox.com/v1/asset/?id=1818", 1818},
		{"https://assetdelivery.roblox.com/v2/assetId/1818", 1818},
		{"http://www.roblox.com/library/9/x", 9},
		{"HTTPS://WWW.ROBLOX.COM/library/10/x", 10},
	}
	for _, c := range ok {
		got, err := ParseAssetRef(c.in)
		if err != nil || got != c.id {
			t.Errorf("ParseAssetRef(%q) = %d, %v; want %d", c.in, got, err, c.id)
		}
	}
	bad := []string{
		"", "abc", "12abc", "-5", "0", "00", "1.5", "12345678901234567", // 17 digits
		"rbxassetid://", "rbxassetid://abc", "rbxassetid://0", "rbxasset://5",
		"https://evil.com/library/123", "https://roblox.com.evil.com/library/123", "https://notroblox.com/library/1",
		"https://www.roblox.com@evil.com/library/123", "https://user:pw@www.roblox.com/library/1",
		"https://www.roblox.com:8080/library/1", "ftp://www.roblox.com/library/1", "file:///library/1", "javascript:alert(1)",
		"https://www.roblox.com/library/abc", "https://www.roblox.com/users/123/profile", "https://www.roblox.com/asset/?id=abc",
		"https://www.roblox.com/asset/", "https://apis.roblox.com/library/1", "https://www.roblox.com/library/1x",
	}
	for _, in := range bad {
		if id, err := ParseAssetRef(in); err == nil {
			t.Errorf("ParseAssetRef(%q) = %d, want an error", in, id)
		}
	}
}

func TestValidCDNURL(t *testing.T) {
	good := []string{"https://fts.rbxcdn.com/v2/sc1/abc?x=1", "https://c0.rbxcdn.com/x", "https://A.RBXCDN.COM/x"}
	bad := []string{"http://fts.rbxcdn.com/x", "https://rbxcdn.com/x", "https://evilrbxcdn.com/x", "https://fts.rbxcdn.com.evil.com/x",
		"https://fts.rbxcdn.com@evil.com/x", "https://x.rbxcdn.com:444/x", "ftp://x.rbxcdn.com/x", "", "//x.rbxcdn.com/x"}
	for _, u := range good {
		if !ValidCDNURL(u) {
			t.Errorf("ValidCDNURL(%q) = false", u)
		}
	}
	for _, u := range bad {
		if ValidCDNURL(u) {
			t.Errorf("ValidCDNURL(%q) = true", u)
		}
	}
}

func TestParseDetails(t *testing.T) {
	m, err := ParseDetails([]byte(`{"AssetId":1837879082,"Name":"Paradise Falls","AssetTypeId":3,"Created":"2018-05-26T02:21:50.427Z","Creator":{"Name":"APMOfficial"}}`))
	if err != nil || m.ID != 1837879082 || m.Name != "Paradise Falls" || m.Type != "Audio" || m.Creator != "APMOfficial" || m.Created != "2018-05-26" || !strings.Contains(m.Note, "tab Audio") {
		t.Errorf("got %+v, %v", m, err)
	}
	if _, err := ParseDetails([]byte(`{"errors":[{"code":20,"message":"No Product Info found associated with the AssetId."}]}`)); err == nil || err.Error() != msgRobloxGone {
		t.Errorf("not found: %v", err)
	}
	if _, err := ParseDetails([]byte(`<html>`)); err == nil {
		t.Error("garbage accepted")
	}
}

func TestParseDelivery(t *testing.T) {
	loc, ty, err := ParseDelivery([]byte(`{"locations":[{"assetFormat":"source","location":"https://fts.rbxcdn.com/v2/x"}],"assetTypeId":3}`))
	if err != nil || loc != "https://fts.rbxcdn.com/v2/x" || ty != 3 {
		t.Errorf("got %q %d %v", loc, ty, err)
	}
	for code, want := range map[int]string{401: msgRobloxPriv, 403: msgRobloxPriv, 409: msgRobloxPriv, 404: msgRobloxGone, 429: msgRobloxRate} {
		_, _, err := ParseDelivery([]byte(`{"errors":[{"code":` + itoa(code) + `,"message":"m"}]}`))
		if err == nil || err.Error() != want {
			t.Errorf("code %d: %v, want %q", code, err, want)
		}
	}
	if _, _, err := ParseDelivery([]byte(`{"errors":[{"code":500,"message":"boom"}]}`)); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("500: %v", err)
	}
	if _, _, err := ParseDelivery([]byte(`{"locations":[]}`)); err == nil {
		t.Error("empty locations accepted")
	}
}

func itoa(n int) string { return strconv.Itoa(n) }

func TestExtFor(t *testing.T) {
	for _, c := range []struct {
		ty   int
		head string
		ext  string
		raw  bool
	}{
		{1, "\x89PNG\r\n\x1a\n....", "png", false},
		{3, "OggS\x00\x02", "ogg", false},
		{3, "ID3\x04", "mp3", false},
		{3, "\xff\xfb\x90\x00", "mp3", false},
		{1, "\xff\xd8\xff\xe0", "jpg", false},
		{3, "RIFF\x00\x00\x00\x00WAVEfmt ", "wav", false},
		{13, "<roblox xmlns:xmime=", "rbxmx", true},
		{10, "<roblox!\x89\xff", "rbxm", true},
		{4, "version 2.00\n", "mesh", true},
		{3, "", "ogg", false}, // unknown bytes: fall back to the type
		{1, "zz", "png", false},
		{4, "zz", "mesh", true},
		{9, "zz", "bin", true},
	} {
		ext, raw := ExtFor(c.ty, []byte(c.head))
		if ext != c.ext || raw != c.raw {
			t.Errorf("ExtFor(%d, %q) = %s,%v; want %s,%v", c.ty, c.head, ext, raw, c.ext, c.raw)
		}
	}
}

func TestRobloxFileStemAndUnique(t *testing.T) {
	for in, want := range map[string]string{
		"Paradise Falls": "5-Paradise Falls", "a/b:c": "5-a_b_c", "": "5", "  ..  ": "5", "CON": "5-CON",
	} {
		if got := RobloxFileStem(5, in); got != want {
			t.Errorf("RobloxFileStem(%q) = %q, want %q", in, got, want)
		}
	}
	if got := []rune(RobloxFileStem(5, strings.Repeat("x", 300))); len(got) > 2+80 {
		t.Errorf("name not capped: %d", len(got))
	}
	dir := t.TempDir()
	p1 := UniquePath(dir, "5-x", "ogg")
	if err := os.WriteFile(p1, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if p2 := UniquePath(dir, "5-x", "ogg"); p2 == p1 || !strings.HasSuffix(p2, "5-x (1).ogg") {
		t.Errorf("not unique: %s", p2)
	}
}

func TestTypeNotes(t *testing.T) {
	if n := TypeNote(13); !strings.Contains(n, "bukan gambarnya") || strings.Contains(n, "PNG apa adanya") {
		t.Errorf("decal note: %q", n)
	}
	if AssetTypeName(3) != "Audio" || AssetTypeName(9999) != "Tipe 9999" {
		t.Error("type names")
	}
}

// Live: talks to Roblox's public endpoints. Run with ROKIT_LIVE=1 go test -run TestLiveRoblox -v
func TestLiveRoblox(t *testing.T) {
	if os.Getenv("ROKIT_LIVE") == "" {
		t.Skip("set ROKIT_LIVE=1 to call the real Roblox endpoints")
	}
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	r := &Rokit{}
	m, err := r.RobloxCheck(t.Context(), "https://create.roblox.com/store/asset/1837879082/Paradise-Falls")
	if err != nil || m.Type != "Audio" || m.Blocked != "" {
		t.Fatalf("check: %+v %v", m, err)
	}
	s, err := r.RobloxDownload(t.Context(), "1837879082")
	if err != nil || s.Ext != "ogg" || !s.IsAudio {
		t.Fatalf("download: %+v %v", s, err)
	}
	if b, _ := os.ReadFile(s.Path); len(b) < 1000 || string(b[:4]) != "OggS" {
		t.Errorf("not an Ogg file (%d bytes)", len(b))
	}
	if _, err := r.RobloxDownload(t.Context(), "1818"); err == nil || err.Error() != msgRobloxPriv {
		t.Errorf("private asset: %v", err)
	}
}
