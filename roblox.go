package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Roblox asset downloads use public, anonymous endpoints only: no cookie, no login, no bypass.

const (
	robloxUA        = "Rokit-Asset-Downloader/1.0"
	robloxMaxBytes  = 200 << 20 // cap on the downloaded asset
	robloxDirName   = "Rokit-Roblox"
	msgRobloxPriv   = "Aset ini privat atau butuh login Roblox; tidak bisa diunduh tanpa akses"
	msgRobloxGone   = "Aset tidak ditemukan atau sudah dihapus."
	msgRobloxRate   = "Terlalu banyak permintaan, coba lagi nanti"
	msgRobloxCDN    = "Roblox memberi alamat unduhan yang tidak diizinkan (bukan rbxcdn.com lewat https)"
	msgRobloxFormat = "Masukkan ID aset (angka) atau URL dari roblox.com, create.roblox.com, atau rbxassetid://ID."
)

var errRobloxRef = errors.New(msgRobloxFormat)

var (
	digitsOnly  = regexp.MustCompile(`^[0-9]{1,16}$`)
	assetPathRe = regexp.MustCompile(`^/(?:library|catalog|marketplace/asset|store/asset|v[12]/assetId)/([0-9]{1,16})(?:/|$)`)
	robloxHosts = map[string]bool{"roblox.com": true, "www.roblox.com": true, "create.roblox.com": true, "assetdelivery.roblox.com": true}
)

// ParseAssetRef turns what the user pasted into an asset ID: a bare number, rbxassetid://ID, or a
// roblox.com / create.roblox.com / assetdelivery.roblox.com URL. Anything else is an error.
func ParseAssetRef(s string) (int64, error) {
	s = strings.TrimSpace(s)
	switch {
	case digitsOnly.MatchString(s):
		return positiveID(s)
	case len(s) > 13 && strings.EqualFold(s[:13], "rbxassetid://"):
		return positiveID(s[13:])
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.Port() != "" {
		return 0, errRobloxRef
	}
	if !robloxHosts[strings.ToLower(u.Hostname())] {
		return 0, errRobloxRef
	}
	if m := assetPathRe.FindStringSubmatch(u.Path); m != nil {
		return positiveID(m[1])
	}
	switch strings.TrimSuffix(u.Path, "/") {
	case "/asset", "/v1/asset", "/marketplace/asset", "/library":
		if id := u.Query().Get("id"); digitsOnly.MatchString(id) {
			return positiveID(id)
		}
	}
	return 0, errRobloxRef
}

func positiveID(s string) (int64, error) {
	if !digitsOnly.MatchString(s) {
		return 0, errRobloxRef
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 {
		return 0, errRobloxRef
	}
	return n, nil
}

// ValidCDNURL: the only place a download may come from is https://rbxcdn.com or https://*.rbxcdn.com
// (whole-label match, so evilrbxcdn.com and rbxcdn.com.evil.com fail). No userinfo, no port but 443, no IP literal.
func ValidCDNURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || (u.Port() != "" && u.Port() != "443") {
		return false
	}
	h := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".") // "fts.rbxcdn.com." is the same host
	if _, err := netip.ParseAddr(h); err == nil {
		return false
	}
	return h == "rbxcdn.com" || (strings.HasSuffix(h, ".rbxcdn.com") && !strings.HasPrefix(h, ".") && !strings.Contains(h, ".."))
}

// SafeHost is "scheme://host" of a URL for error messages: no userinfo, path, query or token.
func SafeHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "(tidak terbaca)"
	}
	return u.Scheme + "://" + u.Hostname()
}

// pickLocation returns the first location that passes ValidCDNURL. When none does, the error names the
// scheme and host of the first one so the problem can be diagnosed without leaking a signed URL.
func pickLocation(locs []string) (string, error) {
	first := ""
	for _, l := range locs {
		if l == "" {
			continue
		}
		if first == "" {
			first = l
		}
		if ValidCDNURL(l) {
			return l, nil
		}
	}
	if first == "" {
		return "", errors.New(msgRobloxGone)
	}
	return "", fmt.Errorf("%s (host: %s)", msgRobloxCDN, SafeHost(first))
}

// AssetMeta is what "Cek" shows.
type AssetMeta struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	TypeID  int    `json:"typeId"`
	Type    string `json:"type"`
	Creator string `json:"creator"`
	Created string `json:"created"` // yyyy-mm-dd, "" if unknown
	Note    string `json:"note"`    // what the file will be, in Indonesian
	Blocked string `json:"blocked"` // non-empty: why it cannot be downloaded without a login
}

var assetTypes = map[int]string{
	1: "Image", 2: "T-Shirt", 3: "Audio", 4: "Mesh", 5: "Lua", 8: "Hat", 9: "Place", 10: "Model", 11: "Shirt", 12: "Pants",
	13: "Decal", 17: "Head", 18: "Face", 19: "Gear", 21: "Badge", 24: "Animation", 32: "Package", 34: "Game Pass",
	38: "Plugin", 40: "MeshPart", 41: "Hair Accessory", 42: "Face Accessory", 43: "Neck Accessory", 44: "Shoulder Accessory",
	45: "Front Accessory", 46: "Back Accessory", 47: "Waist Accessory", 61: "Emote Animation", 62: "Video",
}

// AssetTypeName names a Roblox AssetTypeId ("Tipe N" when unknown).
func AssetTypeName(id int) string {
	if n, ok := assetTypes[id]; ok {
		return n
	}
	return fmt.Sprintf("Tipe %d", id)
}

// ParseDetails reads the economy asset-details response.
func ParseDetails(b []byte) (AssetMeta, error) {
	var d struct {
		AssetID     int64  `json:"AssetId"`
		Name        string `json:"Name"`
		AssetTypeID int    `json:"AssetTypeId"`
		Created     string `json:"Created"`
		Creator     struct {
			Name string `json:"Name"`
		} `json:"Creator"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(b, &d); err != nil {
		return AssetMeta{}, errors.New("jawaban Roblox tidak bisa dibaca")
	}
	if len(d.Errors) > 0 || d.AssetID == 0 {
		return AssetMeta{}, errors.New(msgRobloxGone)
	}
	m := AssetMeta{ID: d.AssetID, Name: d.Name, TypeID: d.AssetTypeID, Type: AssetTypeName(d.AssetTypeID), Creator: d.Creator.Name}
	if t, err := time.Parse(time.RFC3339, d.Created); err == nil {
		m.Created = t.Format("2006-01-02")
	}
	m.Note = TypeNote(m.TypeID)
	return m, nil
}

// ParseDelivery reads the asset delivery response: the CDN location and type, or a mapped error.
// Roblox answers errors with HTTP 200 and an "errors" list, so the code inside decides.
func ParseDelivery(b []byte) (loc string, typeID int, err error) {
	var d struct {
		Locations []struct {
			Location string `json:"location"`
		} `json:"locations"`
		Errors []struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"errors"`
		AssetTypeID int `json:"assetTypeId"`
	}
	if err := json.Unmarshal(b, &d); err != nil {
		return "", 0, errors.New("jawaban Roblox tidak bisa dibaca")
	}
	if len(d.Errors) > 0 {
		return "", 0, DeliveryError(d.Errors[0].Code, d.Errors[0].Message)
	}
	locs := make([]string, len(d.Locations))
	for i, l := range d.Locations {
		locs[i] = l.Location
	}
	loc, err = pickLocation(locs)
	return loc, d.AssetTypeID, err
}

// DeliveryError maps a Roblox error code to the message the page shows.
func DeliveryError(code int, msg string) error {
	switch code {
	case 401, 403, 409:
		return errors.New(msgRobloxPriv)
	case 404:
		return errors.New(msgRobloxGone)
	case 429:
		return &RateError{}
	}
	return fmt.Errorf("Roblox menolak permintaan (%d): %s", code, msg)
}

// TypeNote says what the downloaded file is, so a Decal is never passed off as a PNG.
func TypeNote(typeID int) string {
	switch typeID {
	case 1:
		return "Image — gambar PNG apa adanya."
	case 3:
		return "Audio — disimpan apa adanya (.ogg atau .mp3). Bisa dibuka di tab Audio untuk diubah format, bitrate, atau kecepatannya."
	case 13:
		return "Decal — ini bukan gambarnya: ID Decal berbeda dari ID Image. File disimpan mentah (.rbxmx) berisi rujukan ke Image-nya."
	case 4, 40:
		return "Mesh — format mentah Roblox (.mesh), bukan OBJ/FBX."
	case 2, 8, 10, 11, 12, 17, 18, 19, 32, 41, 42, 43, 44, 45, 46, 47:
		return AssetTypeName(typeID) + " — model Roblox, disimpan mentah (.rbxmx); buka dengan Roblox Studio."
	}
	return AssetTypeName(typeID) + " — disimpan mentah dalam format Roblox."
}

// ExtFor picks the file extension from the real bytes first (PNG, Ogg, MP3, JPEG, WAV, Roblox XML or
// binary), then from the asset type. raw says the result is a Roblox-native format, not a common file.
func ExtFor(typeID int, h []byte) (ext string, raw bool) {
	has := func(off int, s string) bool { return len(h) >= off+len(s) && string(h[off:off+len(s)]) == s }
	switch {
	case has(0, "\x89PNG\r\n\x1a\n"):
		return "png", false
	case has(0, "OggS"):
		return "ogg", false
	case has(0, "ID3"), len(h) >= 2 && h[0] == 0xFF && h[1]&0xE0 == 0xE0:
		return "mp3", false
	case len(h) >= 3 && h[0] == 0xFF && h[1] == 0xD8 && h[2] == 0xFF:
		return "jpg", false
	case has(0, "RIFF") && has(8, "WAVE"):
		return "wav", false
	case has(0, "<roblox!"):
		return "rbxm", true
	case has(0, "<roblox"):
		return "rbxmx", true
	case has(0, "version "):
		return "mesh", true
	}
	switch typeID {
	case 1:
		return "png", false
	case 3:
		return "ogg", false
	case 4, 40:
		return "mesh", true
	case 9:
		return "rbxl", true
	case 10, 24, 38:
		return "rbxm", true
	}
	return "bin", true
}

// RobloxFileStem is "<id>-<name>" with the name made safe and short (just the ID when there is no name).
func RobloxFileStem(id int64, name string) string {
	if strings.Trim(name, " .	") == "" {
		return strconv.FormatInt(id, 10)
	}
	n := []rune(SanitizeFilename(name))
	if len(n) > 80 {
		n = n[:80]
	}
	return fmt.Sprintf("%d-%s", id, strings.TrimSpace(string(n)))
}

var (
	contentRe   = regexp.MustCompile(`<Content name="([A-Za-z]+)">\s*<url>([^<]*)</url>`)
	contentIDRe = regexp.MustCompile(`(?:[?&]id=|rbxassetid://)([0-9]{1,16})`)
)

// wrapperField is the property of a wrapper asset's XML that points at its image.
var wrapperField = map[int]string{13: "Texture", 18: "Texture", 11: "ShirtTemplate", 12: "PantsTemplate", 2: "Graphic"}

// IsWrapperType says whether an asset type is a thin XML wrapper around an image (Decal, Face, Shirt, Pants, T-Shirt).
func IsWrapperType(typeID int) bool { _, ok := wrapperField[typeID]; return ok }

// ParseWrapperImage finds the image asset ID a Decal, Face, Shirt, Pants or T-Shirt points at,
// reading only its public asset-delivery XML.
func ParseWrapperImage(typeID int, xml []byte) (int64, bool) {
	want, ok := wrapperField[typeID]
	if !ok {
		return 0, false
	}
	for _, m := range contentRe.FindAllSubmatch(xml, -1) {
		if string(m[1]) != want {
			continue
		}
		if id := contentIDRe.FindSubmatch(m[2]); id != nil {
			if n, err := positiveID(string(id[1])); err == nil {
				return n, true
			}
		}
	}
	return 0, false
}

// StatusError maps an HTTP status from a Roblox host to the message the page shows (nil for 200).
func StatusError(code int) error {
	switch {
	case code == 200:
		return nil
	case code == 429:
		return &RateError{}
	case code == 401 || code == 403:
		return errors.New(msgRobloxPriv)
	case code == 404:
		return errors.New(msgRobloxGone)
	}
	return fmt.Errorf("Roblox menjawab HTTP %d", code)
}

// RateError is HTTP 429. Wait is Roblox's Retry-After (0 when absent or unreadable).
type RateError struct{ Wait time.Duration }

func (*RateError) Error() string { return msgRobloxRate }

// ParseRetryAfter reads a Retry-After header: whole seconds or an HTTP date. Bad or past values give 0.
func ParseRetryAfter(v string, now time.Time) time.Duration {
	v = strings.TrimSpace(v)
	if n, err := strconv.Atoi(v); err == nil {
		return time.Duration(max(n, 0)) * time.Second
	}
	if t, err := time.Parse(time.RFC1123, v); err == nil && t.After(now) {
		return t.Sub(now)
	}
	return 0
}

// RobloxOpts are the optional download choices. Version and Avatar cannot be combined.
type RobloxOpts struct {
	Version int  `json:"version"` // 0 = latest
	Avatar  bool `json:"avatar"`  // the avatar-ready model of an accessory or head
	OBJ     bool `json:"obj"`     // a Mesh as .obj instead of raw .mesh
}

func (o RobloxOpts) Validate() error {
	switch {
	case o.Version < 0 || o.Version > 1_000_000:
		return errors.New("versi harus angka 1 atau lebih")
	case o.Version > 0 && o.Avatar:
		return errors.New("versi dan model avatar tidak bisa dipakai bersamaan")
	}
	return nil
}

// AvatarFormatFor is the Roblox-AssetFormat header value for a type ("" when it has none).
func AvatarFormatFor(typeID int) string {
	switch typeID {
	case 8, 41, 42, 43, 44, 45, 46, 47:
		return "avatar_meshpart_accessory"
	case 17, 79:
		return "avatar_meshpart_head"
	}
	return ""
}

// DeliveryURL is the public delivery endpoint for an asset, optionally a given version.
func DeliveryURL(id int64, o RobloxOpts) string {
	s := strconv.FormatInt(id, 10)
	switch {
	case o.Avatar:
		return "https://assetdelivery.roblox.com/v2/asset/?id=" + s
	case o.Version > 0:
		return "https://assetdelivery.roblox.com/v2/assetId/" + s + "/version/" + strconv.Itoa(o.Version)
	}
	return "https://assetdelivery.roblox.com/v2/assetId/" + s
}

// Delivered is one asset's answer from the batch endpoint.
type Delivered struct {
	Loc    string
	TypeID int
	Err    error
}

// BatchBody is the request body for the batch endpoint (requestId is the asset ID).
func BatchBody(ids []int64) []byte {
	type it struct {
		RequestID string `json:"requestId"`
		AssetID   int64  `json:"assetId"`
	}
	out := make([]it, len(ids))
	for i, id := range ids {
		out[i] = it{strconv.FormatInt(id, 10), id}
	}
	b, _ := json.Marshal(out)
	return b
}

// ParseBatch reads the batch endpoint's answer into a map keyed by asset ID.
func ParseBatch(b []byte) (map[int64]Delivered, error) {
	var d []struct {
		RequestID   string `json:"requestId"`
		AssetTypeID int    `json:"assetTypeId"`
		Locations   []struct {
			Location string `json:"location"`
		} `json:"locations"`
		Errors []struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, errors.New("jawaban Roblox tidak bisa dibaca")
	}
	out := map[int64]Delivered{}
	for _, x := range d {
		id, err := positiveID(x.RequestID)
		if err != nil {
			continue
		}
		switch {
		case len(x.Errors) > 0:
			out[id] = Delivered{Err: DeliveryError(x.Errors[0].Code, x.Errors[0].Message)}
		default:
			locs := make([]string, len(x.Locations))
			for i, l := range x.Locations {
				locs[i] = l.Location
			}
			loc, err := pickLocation(locs)
			out[id] = Delivered{Loc: loc, TypeID: x.AssetTypeID, Err: err}
		}
	}
	return out, nil
}

// MaxBatch is the most assets one list may hold.
const MaxBatch = 50

// SplitAssetList turns the pasted text (one ID or URL per line) into trimmed, non-empty lines.
func SplitAssetList(text string) ([]string, error) {
	var out []string
	for _, l := range strings.FieldsFunc(text, func(r rune) bool { return r == '\n' }) {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("daftar kosong: tulis satu ID atau URL per baris")
	}
	if len(out) > MaxBatch {
		return nil, fmt.Errorf("terlalu banyak: %d baris, maksimal %d", len(out), MaxBatch)
	}
	return out, nil
}

// BatchItem is the state of one line of "Unduh semua".
type BatchItem struct {
	Index   int    `json:"index"`
	Input   string `json:"input"`
	Status  string `json:"status"` // menunggu | jalan | selesai | gagal | dilewati
	Message string `json:"message"`
	Path    string `json:"path"`
}
