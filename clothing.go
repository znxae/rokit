package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/png"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
)

// Classic Shirt / Pants maker: PNG validation and saving, plus the public avatar lookup for the 3D preview.
// The UV layout of the template lives in src/clothing_uv.ts (measured from Roblox's published template).

const (
	ClothingW        = 585
	ClothingH        = 559
	MaxClothingBytes = 4 << 20
	maxPreviewBytes  = 20 << 20
	clothingDirName  = "Rokit-Pakaian"
	uploadPageURL    = "https://create.roblox.com/dashboard/creations"
)

var pngSig = []byte("\x89PNG\r\n\x1a\n")

// ValidateClothingPNG checks a classic clothing image: a PNG of exactly 585x559, at most 4 MB.
// The messages are what the page shows.
func ValidateClothingPNG(b []byte) (w, h int, err error) {
	if len(b) > MaxClothingBytes {
		return 0, 0, fmt.Errorf("file terlalu besar (%.1f MB); batas %d MB", float64(len(b))/(1<<20), MaxClothingBytes>>20)
	}
	if !bytes.HasPrefix(b, pngSig) {
		return 0, 0, errors.New("file harus berformat PNG")
	}
	cfg, format, derr := image.DecodeConfig(bytes.NewReader(b))
	if derr != nil || format != "png" {
		return 0, 0, errors.New("file PNG rusak atau tidak bisa dibaca")
	}
	if cfg.Width != ClothingW || cfg.Height != ClothingH {
		return cfg.Width, cfg.Height, fmt.Errorf("ukuran harus %dx%d, gambar Anda %dx%d", ClothingW, ClothingH, cfg.Width, cfg.Height)
	}
	return cfg.Width, cfg.Height, nil
}

// ClothingFile is a validated image handed to the page (data is base64).
type ClothingFile struct {
	Name   string `json:"name"`
	Data   string `json:"data"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// PickClothingImage opens the file dialog for a PNG and validates it. An empty Name means cancelled.
func (r *Rokit) PickClothingImage() (ClothingFile, error) {
	sel, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Title: "Pilih gambar pakaian (PNG 585x559)", Filters: []mygo.FileFilter{{Name: "PNG", Extensions: []string{"png"}}}})
	if err != nil || len(sel) == 0 {
		return ClothingFile{}, err
	}
	st, err := os.Stat(sel[0])
	if err != nil {
		return ClothingFile{}, err
	}
	if st.Size() > MaxClothingBytes {
		return ClothingFile{}, fmt.Errorf("file terlalu besar (%.1f MB); batas %d MB", float64(st.Size())/(1<<20), MaxClothingBytes>>20)
	}
	b, err := os.ReadFile(sel[0])
	if err != nil {
		return ClothingFile{}, err
	}
	w, h, err := ValidateClothingPNG(b)
	if err != nil {
		return ClothingFile{}, err
	}
	return ClothingFile{Name: filepath.Base(sel[0]), Data: base64.StdEncoding.EncodeToString(b), Width: w, Height: h}, nil
}

var clothingKinds = map[string]string{"kemeja": "kemeja", "celana": "celana", "panduan": "template-panduan", "preview": "preview"}

// SaveClothingImage writes a PNG to Downloads\Rokit-Pakaian, never overwriting. kind is kemeja, celana, panduan
// or preview; the first three must be exactly 585x559, a preview is any PNG up to 20 MB.
func (r *Rokit) SaveClothingImage(kind, dataB64 string) (string, error) {
	stem, ok := clothingKinds[kind]
	if !ok {
		return "", errors.New("jenis gambar tidak dikenal")
	}
	if len(dataB64) > (maxPreviewBytes*4)/3+16 {
		return "", errors.New("gambar terlalu besar")
	}
	b, err := base64.StdEncoding.DecodeString(dataB64)
	if err != nil {
		return "", errors.New("data gambar tidak valid")
	}
	if kind == "preview" {
		if len(b) > maxPreviewBytes || !bytes.HasPrefix(b, pngSig) {
			return "", errors.New("gambar preview harus PNG dan maksimal 20 MB")
		}
	} else if _, _, err := ValidateClothingPNG(b); err != nil {
		return "", err
	}
	dir := filepath.Join(downloadsDir(), clothingDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	dst := UniquePath(dir, stem, "png")
	if err := os.WriteFile(dst, b, 0o644); err != nil {
		return "", err
	}
	return dst, nil
}

// OpenUploadPage opens Roblox's creator dashboard in the default browser. Uploading itself is not done here.
func (r *Rokit) OpenUploadPage() error { return mygo.Shell.OpenExternal(uploadPageURL) }

// ---- avatar lookup (public endpoints, no login) ----

// AvatarLook is the player's body colors for the preview. Colors are #RRGGBB.
type AvatarLook struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	AvatarType string `json:"avatarType"` // R6 or R15 (the preview is R6 either way)
	Head       string `json:"head"`
	Torso      string `json:"torso"`
	LeftArm    string `json:"leftArm"`
	RightArm   string `json:"rightArm"`
	LeftLeg    string `json:"leftLeg"`
	RightLeg   string `json:"rightLeg"`
}

const fallbackBodyColor = "#A3A2A5" // Medium stone grey

var usernameRe = regexp.MustCompile(`^[A-Za-z0-9_]{3,20}$`)

// ParseWho tells an ID from a username: digits only (up to 16) is an ID, anything else must look like a username.
func ParseWho(s string) (id int64, name string, err error) {
	s = strings.TrimSpace(s)
	switch {
	case digitsOnly.MatchString(s):
		n, perr := strconv.ParseInt(s, 10, 64)
		if perr == nil && n > 0 {
			return n, "", nil
		}
	case usernameRe.MatchString(s):
		return 0, s, nil
	}
	return 0, "", errors.New("tulis username Roblox (3-20 huruf, angka, _) atau User ID (angka)")
}

// ParseUserLookup reads POST users/v1/usernames/users.
func ParseUserLookup(b []byte) (int64, string, error) {
	var d struct {
		Data []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if json.Unmarshal(b, &d) != nil || len(d.Data) == 0 || d.Data[0].ID <= 0 {
		return 0, "", errors.New("pengguna Roblox tidak ditemukan")
	}
	return d.Data[0].ID, d.Data[0].Name, nil
}

// ParseUserName reads GET users/v1/users/ID.
func ParseUserName(b []byte) (string, error) {
	var d struct {
		Name string `json:"name"`
	}
	if json.Unmarshal(b, &d) != nil || d.Name == "" {
		return "", errors.New("pengguna Roblox tidak ditemukan")
	}
	return d.Name, nil
}

// ParsePalette reads bodyColorsPalette from avatar-rules into brickColorId -> #RRGGBB.
func ParsePalette(b []byte) (map[int]string, error) {
	var d struct {
		Palette []struct {
			ID  int    `json:"brickColorId"`
			Hex string `json:"hexColor"`
		} `json:"bodyColorsPalette"`
	}
	if json.Unmarshal(b, &d) != nil || len(d.Palette) == 0 {
		return nil, errors.New("palet warna Roblox tidak bisa dibaca")
	}
	out := make(map[int]string, len(d.Palette))
	for _, p := range d.Palette {
		out[p.ID] = p.Hex
	}
	return out, nil
}

var hexColorRe = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

// ParseAvatar reads GET avatar/v1/users/ID/avatar and resolves the body colors through the palette.
func ParseAvatar(b []byte, palette map[int]string) (AvatarLook, error) {
	var raw struct {
		Type   string `json:"playerAvatarType"`
		Colors struct {
			Head     int `json:"headColorId"`
			Torso    int `json:"torsoColorId"`
			RightArm int `json:"rightArmColorId"`
			LeftArm  int `json:"leftArmColorId"`
			RightLeg int `json:"rightLegColorId"`
			LeftLeg  int `json:"leftLegColorId"`
		} `json:"bodyColors"`
	}
	if json.Unmarshal(b, &raw) != nil || raw.Colors.Torso == 0 {
		return AvatarLook{}, errors.New("avatar Roblox tidak bisa dibaca")
	}
	hex := func(id int) string {
		if h, ok := palette[id]; ok && hexColorRe.MatchString(h) {
			return strings.ToUpper(h)
		}
		return fallbackBodyColor
	}
	return AvatarLook{AvatarType: raw.Type, Head: hex(raw.Colors.Head), Torso: hex(raw.Colors.Torso), LeftArm: hex(raw.Colors.LeftArm),
		RightArm: hex(raw.Colors.RightArm), LeftLeg: hex(raw.Colors.LeftLeg), RightLeg: hex(raw.Colors.RightLeg)}, nil
}

// RobloxAvatarLook finds a player by username or User ID and returns their body colors. Public endpoints only.
func (r *Rokit) RobloxAvatarLook(ctx context.Context, who string) (AvatarLook, error) {
	id, name, err := ParseWho(who)
	if err != nil {
		return AvatarLook{}, err
	}
	return avatarLook(ctx, id, name)
}

// RobloxMyAvatar is the "Avatar saya" shortcut: the logged-in account's avatar, looked up by its public ID.
func (r *Rokit) RobloxMyAvatar(ctx context.Context) (AvatarLook, error) {
	a := r.RobloxAccountInfo()
	if !a.LoggedIn || a.ID <= 0 {
		return AvatarLook{}, errors.New("masuk Roblox dulu (tombol Masuk Roblox di tab Aset Roblox)")
	}
	return avatarLook(ctx, a.ID, a.Name)
}

func avatarLook(ctx context.Context, id int64, name string) (AvatarLook, error) {
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if id == 0 {
		body, _ := json.Marshal(map[string]any{"usernames": []string{name}, "excludeBannedUsers": true})
		b, err := robloxDo(cctx, "POST", "https://users.roblox.com/v1/usernames/users", body, nil)
		if err != nil {
			return AvatarLook{}, err
		}
		if id, name, err = ParseUserLookup(b); err != nil {
			return AvatarLook{}, err
		}
	} else if name == "" {
		b, err := robloxGet(cctx, "https://users.roblox.com/v1/users/"+strconv.FormatInt(id, 10))
		if err != nil {
			return AvatarLook{}, err
		}
		if name, err = ParseUserName(b); err != nil {
			return AvatarLook{}, err
		}
	}
	pb, err := robloxGet(cctx, "https://avatar.roblox.com/v1/avatar-rules")
	if err != nil {
		return AvatarLook{}, err
	}
	palette, err := ParsePalette(pb)
	if err != nil {
		return AvatarLook{}, err
	}
	ab, err := robloxGet(cctx, "https://avatar.roblox.com/v1/users/"+strconv.FormatInt(id, 10)+"/avatar")
	if err != nil {
		return AvatarLook{}, err
	}
	look, err := ParseAvatar(ab, palette)
	if err != nil {
		return AvatarLook{}, err
	}
	look.ID, look.Name = id, name
	return look, nil
}
