package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestKindOfFile(t *testing.T) {
	for name, want := range map[string]string{"a.mp3": "audio", "A.FLAC": "audio", "v.mp4": "video", "v.MKV": "video", "s.png": "image", "n.txt": "other", "noext": "other", "x.mp3.txt": "other"} {
		if got := KindOfFile(name); got != want {
			t.Errorf("%s: %s want %s", name, got, want)
		}
	}
}

func TestClassifyDrop(t *testing.T) {
	d := t.TempDir()
	f := filepath.Join(d, "a.mp3")
	os.WriteFile(f, []byte("x"), 0o600)
	got := (&Rokit{}).ClassifyDrop([]string{f, d, filepath.Join(d, "gone.mp4")})
	if got[0].Kind != "audio" || got[0].Name != "a.mp3" || got[1].Kind != "folder" || got[2].Kind != "missing" {
		t.Fatalf("%+v", got)
	}
}
