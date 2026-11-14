package main

import (
	"encoding/json"
	"errors"
	"fmt"
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

// ValidCDNURL: the only place a download may come from is https://*.rbxcdn.com.
func ValidCDNURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return false
	}
	h := strings.ToLower(u.Hostname())
	return strings.HasSuffix(h, ".rbxcdn.com") && len(h) > len(".rbxcdn.com")
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
	if len(d.Locations) == 0 || d.Locations[0].Location == "" {
		return "", 0, errors.New(msgRobloxGone)
	}
	return d.Locations[0].Location, d.AssetTypeID, nil
}

// DeliveryError maps a Roblox error code to the message the page shows.
func DeliveryError(code int, msg string) error {
	switch code {
	case 401, 403, 409:
		return errors.New(msgRobloxPriv)
	case 404:
		return errors.New(msgRobloxGone)
	case 429:
		return errors.New(msgRobloxRate)
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
