package main

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"os"
	"strings"
	"testing"
)

func pngOf(w, h int) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.NRGBA{255, 0, 0, 255})
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

func TestValidateClothingPNG(t *testing.T) {
	if w, h, err := ValidateClothingPNG(pngOf(585, 559)); err != nil || w != 585 || h != 559 {
		t.Errorf("good image: %d %d %v", w, h, err)
	}
	if _, _, err := ValidateClothingPNG(pngOf(128, 128)); err == nil || err.Error() != "ukuran harus 585x559, gambar Anda 128x128" {
		t.Errorf("wrong size: %v", err)
	}
	if _, _, err := ValidateClothingPNG(pngOf(586, 559)); err == nil || !strings.Contains(err.Error(), "586x559") {
		t.Errorf("off by one: %v", err)
	}
	if _, _, err := ValidateClothingPNG([]byte("GIF89a....")); err == nil || err.Error() != "file harus berformat PNG" {
		t.Errorf("not png: %v", err)
	}
	if _, _, err := ValidateClothingPNG(append(append([]byte{}, pngSig...), "garbage"...)); err == nil || !strings.Contains(err.Error(), "rusak") {
		t.Errorf("corrupt: %v", err)
	}
	if _, _, err := ValidateClothingPNG(append(pngOf(585, 559), make([]byte, MaxClothingBytes)...)); err == nil || !strings.Contains(err.Error(), "terlalu besar") {
		t.Errorf("too big: %v", err)
	}
	if _, _, err := ValidateClothingPNG(nil); err == nil {
		t.Error("empty accepted")
	}
}

func TestSaveClothingImage(t *testing.T) {
	r := &Rokit{}
	if _, err := r.SaveClothingImage("nope", ""); err == nil {
		t.Error("unknown kind accepted")
	}
	if _, err := r.SaveClothingImage("kemeja", base64.StdEncoding.EncodeToString(pngOf(10, 10))); err == nil || !strings.Contains(err.Error(), "ukuran harus") {
		t.Errorf("wrong size saved: %v", err)
	}
	if _, err := r.SaveClothingImage("kemeja", "!!!not base64"); err == nil {
		t.Error("bad base64 accepted")
	}
	if _, err := r.SaveClothingImage("preview", base64.StdEncoding.EncodeToString([]byte("not a png"))); err == nil {
		t.Error("non-png preview accepted")
	}
}

func TestParseWho(t *testing.T) {
	if id, n, err := ParseWho(" 156 "); err != nil || id != 156 || n != "" {
		t.Errorf("%d %q %v", id, n, err)
	}
	if id, n, err := ParseWho("Builder_Man1"); err != nil || id != 0 || n != "Builder_Man1" {
		t.Errorf("%d %q %v", id, n, err)
	}
	for _, bad := range []string{"", "ab", "has space", "bad-name", strings.Repeat("a", 21), "0", "<script>", "a/b"} {
		if _, _, err := ParseWho(bad); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}

func TestParseAvatarAndPalette(t *testing.T) {
	pal, err := ParsePalette([]byte(`{"bodyColorsPalette":[{"brickColorId":194,"hexColor":"#a3a2a5","name":"x"},{"brickColorId":23,"hexColor":"#0D69AC"},{"brickColorId":5,"hexColor":"javascript:1"}]}`))
	if err != nil || len(pal) != 3 {
		t.Fatal(pal, err)
	}
	look, err := ParseAvatar([]byte(`{"playerAvatarType":"R15","bodyColors":{"headColorId":194,"torsoColorId":23,"rightArmColorId":194,"leftArmColorId":5,"rightLegColorId":999,"leftLegColorId":23}}`), pal)
	if err != nil {
		t.Fatal(err)
	}
	if look.AvatarType != "R15" || look.Head != "#A3A2A5" || look.Torso != "#0D69AC" || look.LeftArm != fallbackBodyColor || look.RightLeg != fallbackBodyColor || look.LeftLeg != "#0D69AC" {
		t.Errorf("%+v", look)
	}
	for _, bad := range []string{`{}`, `x`, `{"bodyColors":{}}`} {
		if _, err := ParseAvatar([]byte(bad), pal); err == nil {
			t.Errorf("accepted %s", bad)
		}
	}
	if _, err := ParsePalette([]byte(`{}`)); err == nil {
		t.Error("empty palette accepted")
	}
	if id, name, err := ParseUserLookup([]byte(`{"data":[{"id":1,"name":"Roblox"}]}`)); err != nil || id != 1 || name != "Roblox" {
		t.Errorf("%d %s %v", id, name, err)
	}
	if _, _, err := ParseUserLookup([]byte(`{"data":[]}`)); err == nil {
		t.Error("empty lookup accepted")
	}
	if n, err := ParseUserName([]byte(`{"name":"Roblox","id":1}`)); err != nil || n != "Roblox" {
		t.Errorf("%s %v", n, err)
	}
}

func TestLiveAvatarLook(t *testing.T) {
	if os.Getenv("ROKIT_LIVE") != "1" {
		t.Skip("set ROKIT_LIVE=1")
	}
	r := &Rokit{}
	for _, who := range []string{"Roblox", "1"} {
		l, err := r.RobloxAvatarLook(t.Context(), who)
		if err != nil {
			t.Errorf("%s: %v", who, err)
			continue
		}
		t.Logf("%s -> id=%d name=%s type=%s head=%s torso=%s la=%s ra=%s ll=%s rl=%s", who, l.ID, l.Name, l.AvatarType, l.Head, l.Torso, l.LeftArm, l.RightArm, l.LeftLeg, l.RightLeg)
	}
	if _, err := r.RobloxAvatarLook(t.Context(), "zz_no_such_user_zz_9x"); err == nil {
		t.Error("missing user accepted")
	}
}
