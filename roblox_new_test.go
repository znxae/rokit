package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"github.com/egoist/mygo"
	"math"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func TestExtForFallbacks(t *testing.T) {
	for typ, want := range map[int]string{9: "rbxl", 10: "rbxm", 24: "rbxm", 38: "rbxm", 99: "bin"} {
		if got, raw := ExtFor(typ, nil); got != want || !raw {
			t.Errorf("type %d: %s %v", typ, got, raw)
		}
	}
}

func TestRetryAfter(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for in, want := range map[string]time.Duration{
		"5": 5 * time.Second, " 12 ": 12 * time.Second, "": 0, "abc": 0, "-3": 0,
		now.Add(30 * time.Second).Format(time.RFC1123): 30 * time.Second,
		now.Add(-time.Hour).Format(time.RFC1123):       0,
	} {
		if got := ParseRetryAfter(in, now); got != want {
			t.Errorf("%q: %v want %v", in, got, want)
		}
	}
	if (StatusError(429)).Error() != msgRobloxRate {
		t.Error("429 message")
	}
}

func TestOptsAndURLs(t *testing.T) {
	if (RobloxOpts{Version: 2, Avatar: true}).Validate() == nil || (RobloxOpts{Version: -1}).Validate() == nil {
		t.Error("invalid opts accepted")
	}
	if (RobloxOpts{Version: 3}).Validate() != nil || (RobloxOpts{Avatar: true, OBJ: true}).Validate() != nil {
		t.Error("valid opts rejected")
	}
	if got := DeliveryURL(7, RobloxOpts{}); got != "https://assetdelivery.roblox.com/v2/assetId/7" {
		t.Error(got)
	}
	if got := DeliveryURL(7, RobloxOpts{Version: 3}); got != "https://assetdelivery.roblox.com/v2/assetId/7/version/3" {
		t.Error(got)
	}
	if got := DeliveryURL(7, RobloxOpts{Avatar: true}); got != "https://assetdelivery.roblox.com/v2/asset/?id=7" {
		t.Error(got)
	}
	for typ, want := range map[int]string{8: "avatar_meshpart_accessory", 47: "avatar_meshpart_accessory", 17: "avatar_meshpart_head", 79: "avatar_meshpart_head", 3: ""} {
		if AvatarFormatFor(typ) != want {
			t.Errorf("type %d", typ)
		}
	}
}

func TestBatch(t *testing.T) {
	if got := string(BatchBody([]int64{5, 6})); got != `[{"requestId":"5","assetId":5},{"requestId":"6","assetId":6}]` {
		t.Error(got)
	}
	got, err := ParseBatch([]byte(`[{"locations":[{"location":"https://c0.rbxcdn.com/x"}],"requestId":"3","assetTypeId":3},
		{"errors":[{"code":409,"message":"no"}],"requestId":"1818"},{"errors":[{"code":404}],"requestId":"130"},
		{"requestId":"9"},{"requestId":"zz"}]`))
	if err != nil {
		t.Fatal(err)
	}
	if d := got[3]; d.Loc != "https://c0.rbxcdn.com/x" || d.TypeID != 3 || d.Err != nil {
		t.Errorf("%+v", d)
	}
	if got[1818].Err == nil || got[1818].Err.Error() != msgRobloxPriv || got[130].Err.Error() != msgRobloxGone || got[9].Err.Error() != msgRobloxGone || len(got) != 4 {
		t.Errorf("%+v", got)
	}
	if _, err := ParseBatch([]byte(`{`)); err == nil {
		t.Error("bad json accepted")
	}
}

func TestMeshV1(t *testing.T) {
	m := "version 1.00\r\n1\r\n[2,4,6][0,1,0][0.5,0.25,0][2,0,0][0,1,0][1,0,0][0,2,0][0,1,0][0,1,0]"
	out, err := MeshToOBJ([]byte(m))
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{"v 1 2 3\n", "vt 0.5 0.75\n", "vn 0 1 0\n", "f 1/1/1 2/2/2 3/3/3\n"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in\n%s", want, s)
		}
	}
	if _, err := MeshToOBJ([]byte("version 1.00\n2\n[1,1,1]")); err == nil {
		t.Error("short v1 accepted")
	}
}

// bin builds a v2 (vs 36 or 40) or v3 mesh with the given vertices (x,y,z) and faces.
func buildMesh(ver string, vs int, verts [][3]float32, faces [][3]uint32, lods []uint32) []byte {
	var b bytes.Buffer
	b.WriteString("version " + ver + "\n")
	w := func(v any) { binary.Write(&b, binary.LittleEndian, v) }
	if ver[0] == '2' {
		w(uint16(12))
		w(uint8(vs))
		w(uint8(12))
		w(uint32(len(verts)))
		w(uint32(len(faces)))
	} else {
		w(uint16(16))
		w(uint8(vs))
		w(uint8(12))
		w(uint16(4))
		w(uint16(len(lods)))
		w(uint32(len(verts)))
		w(uint32(len(faces)))
	}
	for _, v := range verts {
		row := make([]byte, vs)
		for i, f := range []float32{v[0], v[1], v[2], 0, 1, 0, 0.25, 0.75} {
			binary.LittleEndian.PutUint32(row[4*i:], math.Float32bits(f))
		}
		b.Write(row)
	}
	for _, f := range faces {
		w(f)
	}
	for _, l := range lods {
		w(l)
	}
	return b.Bytes()
}

func TestMeshBinary(t *testing.T) {
	verts := [][3]float32{{0, 0, 0}, {1, 0, 0}, {0, 1, 0}, {5, 5, 5}}
	faces := [][3]uint32{{0, 1, 2}, {1, 2, 3}}
	for _, vs := range []int{36, 40} {
		out, err := MeshToOBJ(buildMesh("2.00", vs, verts, faces, nil))
		if err != nil {
			t.Fatal(err)
		}
		s := string(out)
		if strings.Count(s, "\nv ") != 4 || !strings.Contains(s, "f 1/1/1 2/2/2 3/3/3") || !strings.Contains(s, "f 2/2/2 3/3/3 4/4/4") || !strings.Contains(s, "vt 0.25 0.25") {
			t.Errorf("v2 vs=%d:\n%s", vs, s)
		}
	}
	out, err := MeshToOBJ(buildMesh("3.00", 40, verts, faces, []uint32{0, 1, 2}))
	if err != nil {
		t.Fatal(err)
	}
	if s := string(out); strings.Count(s, "\nf ") != 1 {
		t.Errorf("v3 should keep only LOD0:\n%s", s)
	}
}

func TestMeshBad(t *testing.T) {
	good := buildMesh("2.00", 40, [][3]float32{{0, 0, 0}, {1, 0, 0}, {0, 1, 0}}, [][3]uint32{{0, 1, 2}}, nil)
	oob := buildMesh("2.00", 40, [][3]float32{{0, 0, 0}, {1, 0, 0}, {0, 1, 0}}, [][3]uint32{{0, 1, 9}}, nil)
	for name, in := range map[string][]byte{
		"empty": nil, "text": []byte("hello"), "truncated": good[:len(good)-5], "index out of range": oob,
		"header only": []byte("version 2.00\n"), "v5": []byte("version 5.00\n\x00\x00\x00\x00"),
	} {
		if _, err := MeshToOBJ(in); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if _, err := MeshToOBJ(good); err != nil {
		t.Error(err)
	}
}

func TestLiveOptions(t *testing.T) {
	if os.Getenv("ROKIT_LIVE") != "1" {
		t.Skip("set ROKIT_LIVE=1")
	}
	r := &Rokit{}
	cases := []struct {
		name, in string
		o        RobloxOpts
		ext      string
	}{
		{"mesh to obj", "1028604", RobloxOpts{OBJ: true}, "obj"},
		{"avatar hat", "1028606", RobloxOpts{Avatar: true}, "rbxm"},
		{"audio version 1", "1837879082", RobloxOpts{Version: 1}, "ogg"},
	}
	for _, c := range cases {
		s, err := r.RobloxDownload(t.Context(), c.in, c.o)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		t.Cleanup(func() { os.Remove(s.Path) })
		if s.Ext != c.ext {
			t.Errorf("%s: ext %s want %s (%s)", c.name, s.Ext, c.ext, s.Path)
		}
		t.Logf("%s -> %s", c.name, s.Path)
	}
	if _, err := r.RobloxDownload(t.Context(), "1837879082", RobloxOpts{Avatar: true}); err == nil {
		t.Error("avatar format on audio accepted")
	}
	pre := robloxBatchLookup(withPace(t.Context(), 0), []BatchItem{{Input: "1837879082"}, {Input: "1818"}, {Input: "130"}})
	if pre[1837879082] == nil || pre[1837879082].Err != nil || pre[1818].Err == nil || pre[130].Err == nil {
		t.Errorf("batch lookup: %+v", pre)
	}
}

func TestLoginHelpers(t *testing.T) {
	ok := "_|WARNING:-DO-NOT-SHARE-" + strings.Repeat("A1b2", 40) + "|_"
	if !ValidSessionCookie(ok) {
		t.Error("valid cookie rejected")
	}
	for name, v := range map[string]string{"short": "abc", "semicolon": ok + ";x=1", "space": ok + " x", "newline": ok + "\r\nX: y", "long": strings.Repeat("a", 2001), "unicode": ok + "é"} {
		if ValidSessionCookie(v) {
			t.Errorf("%s accepted", name)
		}
	}
	if a, err := ParseAuthUser([]byte(`{"id":5,"name":"x","displayName":"y"}`)); err != nil || !a.LoggedIn || a.ID != 5 || a.Name != "x" {
		t.Errorf("%+v %v", a, err)
	}
	for _, bad := range []string{`{}`, `{"id":0,"name":"x"}`, `x`, `{"errors":[{"code":0}]}`} {
		if _, err := ParseAuthUser([]byte(bad)); err == nil {
			t.Errorf("%s accepted", bad)
		}
	}
}

func TestCookieOnlyToRobloxAPIHosts(t *testing.T) {
	setStoredCookie("_|WARNING:-" + strings.Repeat("Z", 120))
	defer setStoredCookie("")
	for url, want := range map[string]bool{
		"https://assetdelivery.roblox.com/v2/assetId/1": true, "https://economy.roblox.com/x": true, "https://users.roblox.com/x": true,
		"http://assetdelivery.roblox.com/x": false, "https://c0.rbxcdn.com/x": false, "https://evil.example/x": false,
		"https://assetdelivery.roblox.com.evil.example/x": false, "https://www.roblox.com/x": false,
	} {
		req, _ := http.NewRequest("GET", url, nil)
		attachAuth(req)
		if got := req.Header.Get("Cookie") != ""; got != want {
			t.Errorf("%s: cookie sent=%v want %v", url, got, want)
		}
	}
}

// Keluar must really delete the Credential Manager entries. Uses dummy values and skips if a real login is stored here.
func TestLogoutDeletesStoredLogin(t *testing.T) {
	if _, err := mygo.Secrets.Get(secretCookie); err == nil {
		t.Skip("a login is stored under this service; not touching it")
	}
	if err := mygo.Secrets.Set(secretCookie, []byte("dummy-not-a-real-cookie")); err != nil {
		t.Skip("no credential store: ", err)
	}
	_ = mygo.Secrets.Set(secretAccount, []byte(`{"name":"dummy"}`))
	setStoredCookie("x")
	if err := (&Rokit{}).RobloxLogout(); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{secretCookie, secretAccount} {
		if _, err := mygo.Secrets.Get(k); !errors.Is(err, mygo.ErrSecretNotFound) {
			t.Errorf("%s still stored: %v", k, err)
		}
	}
	if loggedIn() {
		t.Error("still logged in after Keluar")
	}
}

func TestLoginNoticeFlagSurvivesThemeChange(t *testing.T) {
	dir := t.TempDir()
	old := settingsPath
	settingsPath = func() (string, error) { return dir + "/s.json", nil }
	defer func() { settingsPath = old }()
	if err := saveSettings(settings{Theme: "dark", LoginNoticeSeen: true}); err != nil {
		t.Fatal(err)
	}
	if err := (&Rokit{}).SetTheme("oled"); err != nil {
		t.Fatal(err)
	}
	if s := loadSettings(); s.Theme != "oled" || !s.LoginNoticeSeen {
		t.Errorf("%+v", s)
	}
	b, _ := os.ReadFile(dir + "/s.json")
	if strings.Contains(strings.ToLower(string(b)), "cookie") || strings.Contains(string(b), "ROBLOSECURITY") {
		t.Errorf("settings.json mentions a cookie: %s", b)
	}
}

func TestCookieNeverToDownloadHosts(t *testing.T) {
	setStoredCookie("_|WARNING:-" + strings.Repeat("Z", 120))
	defer setStoredCookie("")
	for _, u := range []string{"https://contentdelivery.roblox.com/asset/?id=1", "https://fts.rbxcdn.com/x", "https://c0.rbxcdn.com/x"} {
		req, _ := http.NewRequest("GET", u, nil)
		attachAuth(req)
		if req.Header.Get("Cookie") != "" {
			t.Errorf("cookie attached for %s", u)
		}
	}
}
