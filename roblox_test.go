package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
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
	for raw, want := range map[string]bool{
		"https://fts.rbxcdn.com/v2/sc1/abc?x=1": true, "https://c0.rbxcdn.com/x": true, "https://a.rbxcdn.com/x": true,
		"https://rbxcdn.com/x": true, "https://A.RBXCDN.COM/x": true, "https://fts.rbxcdn.com:443/x": true, "https://fts.rbxcdn.com./x": true,
		"http://fts.rbxcdn.com/x": false, "https://evilrbxcdn.com/x": false, "https://rbxcdn.com.evil.com/x": false,
		"https://fts.rbxcdn.com.evil.com/x": false, "https://user@rbxcdn.com/x": false, "https://fts.rbxcdn.com@evil.com/x": false,
		"https://rbxcdn.com:8443/x": false, "https://x.rbxcdn.com:444/x": false, "https://127.0.0.1/x": false, "https://[::1]/x": false,
		"https://1.2.3.4:443/x": false, "ftp://x.rbxcdn.com/x": false, "": false, "//x.rbxcdn.com/x": false, "https://.rbxcdn.com/x": false,
		"https://contentdelivery.roblox.com/asset/?id=1": true, "https://CONTENTDELIVERY.roblox.com:443/x": true,
		"http://contentdelivery.roblox.com/x": false, "https://evilcontentdelivery.roblox.com/x": false,
		"https://contentdelivery.roblox.com.evil.com/x": false, "https://a.contentdelivery.roblox.com/x": false,
		"https://contentdelivery.roblox.com:8443/x": false, "https://user@contentdelivery.roblox.com/x": false,
		"https://roblox.com/x": false, "https://apis.roblox.com/x": false,
		"https://www.roblox.com/x": false, "https://assetdelivery.roblox.com/x": false, "https://-rbxcdn.com/x": false,
	} {
		if got := ValidCDNURL(raw); got != want {
			t.Errorf("ValidCDNURL(%q) = %v, want %v", raw, got, want)
		}
	}
}

func TestPickLocationAndSafeHost(t *testing.T) {
	// several entries: skip the bad ones, take the first that passes
	if got, err := pickLocation([]string{"", "http://x.rbxcdn.com/a", "https://evil.example/b", "https://c1.rbxcdn.com/c?token=SECRET"}); err != nil || got != "https://c1.rbxcdn.com/c?token=SECRET" {
		t.Errorf("%q %v", got, err)
	}
	// none valid: the message names scheme+host only, never the query or userinfo
	_, err := pickLocation([]string{"http://user:pw@cdn.example.com:8080/path/secret?token=SECRET#frag"})
	if err == nil || !strings.Contains(err.Error(), "(host: http://cdn.example.com)") || strings.Contains(err.Error(), "SECRET") || strings.Contains(err.Error(), "pw") || strings.Contains(err.Error(), "secret") {
		t.Errorf("%v", err)
	}
	if _, err := pickLocation(nil); err == nil || err.Error() != msgRobloxGone {
		t.Errorf("%v", err)
	}
	if SafeHost("::bad") != "(tidak terbaca)" {
		t.Error(SafeHost("::bad"))
	}
	// the same selection applies through the real parsers
	loc, _, err := ParseDelivery([]byte(`{"locations":[{"location":"https://evil.example/x"},{"assetFormat":"source","location":"https://fts.rbxcdn.com/ok"}],"assetTypeId":4}`))
	if err != nil || loc != "https://fts.rbxcdn.com/ok" {
		t.Errorf("%q %v", loc, err)
	}
	if _, _, err := ParseDelivery([]byte(`{"locations":[{"location":"https://evil.example/x?t=SECRET"}]}`)); err == nil || !strings.Contains(err.Error(), "(host: https://evil.example)") || strings.Contains(err.Error(), "SECRET") {
		t.Errorf("%v", err)
	}
	b, _ := ParseBatch([]byte(`[{"requestId":"7","assetTypeId":3,"locations":[{"location":"http://x.rbxcdn.com/a"},{"location":"https://c0.rbxcdn.com/b"}]},{"requestId":"8","locations":[{"location":"https://evil.example/q"}]}]`))
	if b[7].Err != nil || b[7].Loc != "https://c0.rbxcdn.com/b" || b[8].Err == nil || !strings.Contains(b[8].Err.Error(), "(host: https://evil.example)") {
		t.Errorf("%+v", b)
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
		{9, "zz", "rbxl", true},
		{99, "zz", "bin", true},
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
	s, err := r.RobloxDownload(t.Context(), "1837879082", RobloxOpts{})
	if err != nil || s.Ext != "ogg" || !s.IsAudio {
		t.Fatalf("download: %+v %v", s, err)
	}
	if b, _ := os.ReadFile(s.Path); len(b) < 1000 || string(b[:4]) != "OggS" {
		t.Errorf("not an Ogg file (%d bytes)", len(b))
	}
	if _, err := r.RobloxDownload(t.Context(), "1818", RobloxOpts{}); err == nil || err.Error() != msgRobloxPriv {
		t.Errorf("private asset: %v", err)
	}
}

func TestParseWrapperImage(t *testing.T) {
	decal := `<roblox xmlns:xmime="x"><Item class="Decal" referent="RBX0"><Properties><Content name="Texture"><url>http://www.roblox.com/asset/?id=157042876</url></Content></Properties></Item></roblox>`
	shirt := `<roblox><Item class="Shirt"><Properties><Content name="ShirtTemplate"><url>http://www.roblox.com/asset/?id=144076357</url></Content><string name="Name">S</string></Properties></Item></roblox>`
	pants := `<roblox><Item class="Pants"><Properties><Content name="PantsTemplate"><url>rbxassetid://99</url></Content></Properties></Item></roblox>`
	hat := `<roblox><Content name="MeshId"><url>http://www.roblox.com/asset/?id=1</url></Content><Content name="TextureId"><url>http://www.roblox.com/asset/?id=2</url></Content></roblox>`
	for _, c := range []struct {
		ty   int
		xml  string
		want int64
		ok   bool
	}{
		{13, decal, 157042876, true}, {18, decal, 157042876, true}, {11, shirt, 144076357, true}, {12, pants, 99, true},
		{11, decal, 0, false},               // a Shirt looks for ShirtTemplate, not Texture
		{13, `<roblox></roblox>`, 0, false}, // no reference
		{13, `<roblox><Content name="Texture"><url>http://evil.com/x</url></Content></roblox>`, 0, false},
		{10, hat, 0, false}, // models are not wrappers
		{13, `<roblox><Content name="Texture"><url>http://www.roblox.com/asset/?id=abc</url></Content></roblox>`, 0, false},
	} {
		got, ok := ParseWrapperImage(c.ty, []byte(c.xml))
		if ok != c.ok || got != c.want {
			t.Errorf("type %d: got %d,%v want %d,%v", c.ty, got, ok, c.want, c.ok)
		}
	}
}

func TestStatusError429AndFriends(t *testing.T) {
	for code, want := range map[int]string{429: msgRobloxRate, 401: msgRobloxPriv, 403: msgRobloxPriv, 404: msgRobloxGone, 500: "Roblox menjawab HTTP 500", 302: "Roblox menjawab HTTP 302"} {
		if err := StatusError(code); err == nil || err.Error() != want {
			t.Errorf("StatusError(%d) = %v, want %q", code, err, want)
		}
	}
	if StatusError(200) != nil {
		t.Error("200 must pass")
	}
}

// A real HTTP exchange against a local server: 429 shows the rate-limit message, with no credentials sent.
func TestRobloxGetHandlesStatuses(t *testing.T) {
	var gotUA, gotAuth, gotCookie string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA, gotAuth, gotCookie = r.Header.Get("User-Agent"), r.Header.Get("Authorization"), r.Header.Get("Cookie")
		switch r.URL.Path {
		case "/ok":
			w.Write([]byte(`{"a":1}`))
		case "/rate":
			w.Header().Set("Retry-After", "30")
			w.WriteHeader(http.StatusTooManyRequests)
		case "/auth":
			w.WriteHeader(http.StatusUnauthorized)
		case "/redir":
			http.Redirect(w, r, "https://evil.example/x", http.StatusFound)
		}
	}))
	defer srv.Close()
	ctx := t.Context()
	if b, err := robloxGet(ctx, srv.URL+"/ok"); err != nil || string(b) != `{"a":1}` {
		t.Errorf("ok: %q %v", b, err)
	}
	if gotUA != robloxUA || gotAuth != "" || gotCookie != "" {
		t.Errorf("headers: UA=%q auth=%q cookie=%q", gotUA, gotAuth, gotCookie)
	}
	for path, want := range map[string]string{"/rate": msgRobloxRate, "/auth": msgRobloxPriv, "/redir": "Roblox menjawab HTTP 302"} {
		if _, err := robloxGet(ctx, srv.URL+path); err == nil || err.Error() != want {
			t.Errorf("%s: %v, want %q", path, err, want)
		}
	}
}

func TestPacerSpacesRequests(t *testing.T) {
	ctx := withPace(t.Context(), 120*time.Millisecond)
	start := time.Now()
	for i := 0; i < 3; i++ {
		if err := paceWait(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if d := time.Since(start); d < 230*time.Millisecond {
		t.Errorf("3 requests took %v, want at least 2 gaps", d)
	}
	start = time.Now()
	for i := 0; i < 3; i++ {
		_ = paceWait(t.Context()) // no pacer: never waits
	}
	if time.Since(start) > 50*time.Millisecond {
		t.Error("unpaced requests waited")
	}
	cctx, cancel := context.WithCancel(withPace(t.Context(), time.Hour))
	_ = paceWait(cctx)
	cancel()
	if err := paceWait(cctx); err == nil {
		t.Error("a cancelled wait must return")
	}
}

func TestSplitAssetList(t *testing.T) {
	got, err := SplitAssetList("  1837879082 \r\n\r\nhttps://www.roblox.com/library/5/x\n\n rbxassetid://7 ")
	if err != nil || len(got) != 3 || got[0] != "1837879082" || got[2] != "rbxassetid://7" {
		t.Errorf("got %q %v", got, err)
	}
	if _, err := SplitAssetList("  \n \n"); err == nil {
		t.Error("empty list accepted")
	}
	fifty := strings.TrimSpace(strings.Repeat("1\n", MaxBatch))
	if got, err := SplitAssetList(fifty); err != nil || len(got) != MaxBatch {
		t.Errorf("50 lines: %d %v", len(got), err)
	}
	if _, err := SplitAssetList(fifty + "\n2"); err == nil || !strings.Contains(err.Error(), "maksimal 50") {
		t.Errorf("51 lines: %v", err)
	}
}

// Live per-type check; prints a table. Run: ROKIT_LIVE=1 go test -run TestLiveTypes -v
func TestLiveTypes(t *testing.T) {
	if os.Getenv("ROKIT_LIVE") == "" {
		t.Skip("set ROKIT_LIVE=1 to call the real Roblox endpoints")
	}
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	ctx := withPace(t.Context(), 700*time.Millisecond)
	for _, c := range []struct {
		label string
		id    int64
	}{
		{"Audio 1837879082", 1837879082}, {"Image 7229442422", 7229442422}, {"Decal 157042877 (public)", 157042877},
		{"Decal 176217906 (login)", 176217906}, {"Mesh 1028604", 1028604}, {"Shirt 144076358", 144076358},
		{"Hat 1028606", 1028606}, {"Place 1818", 1818}, {"Missing 130", 130},
	} {
		s, err := robloxFetch(ctx, c.id, 0, RobloxOpts{}, nil)
		if err != nil {
			t.Logf("%-26s -> %v", c.label, err)
			continue
		}
		fi, _ := os.Stat(s.Path)
		t.Logf("%-26s -> OK .%s %d bytes raw=%v via=%q", c.label, s.Ext, fi.Size(), s.Raw, s.Via)
	}
}
