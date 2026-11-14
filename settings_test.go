package main

import (
	"os"
	"path/filepath"
	"testing"
)

func tempSettings(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "cfg", "settings.json")
	old := settingsPath
	settingsPath = func() (string, error) { return p, nil }
	t.Cleanup(func() { settingsPath = old })
	return p
}

func TestThemeDefaultsToAuto(t *testing.T) {
	tempSettings(t)
	if got := (&Rokit{}).Theme(); got != "auto" {
		t.Fatal(got)
	}
}

func TestThemeRoundTrip(t *testing.T) {
	p := tempSettings(t)
	r := &Rokit{}
	for _, name := range themeNames {
		if err := r.SetTheme(name); err != nil {
			t.Fatal(name, err)
		}
		if got := r.Theme(); got != name {
			t.Errorf("saved %q, read %q", name, got)
		}
	}
	if _, err := os.Stat(p + ".tmp"); err == nil {
		t.Error("temp file left behind")
	}
}

func TestSetThemeRejectsUnknown(t *testing.T) {
	tempSettings(t)
	r := &Rokit{}
	for _, bad := range []string{"", "neon", "../x", "APPLE"} {
		if r.SetTheme(bad) == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	if r.Theme() != "auto" {
		t.Error("a rejected name must not be saved")
	}
}

func TestCorruptSettingsFallBack(t *testing.T) {
	p := tempSettings(t)
	os.MkdirAll(filepath.Dir(p), 0o755)
	for _, content := range []string{"not json", `{"theme":"neon"}`, ""} {
		os.WriteFile(p, []byte(content), 0o644)
		if got := (&Rokit{}).Theme(); got != "auto" {
			t.Errorf("%q -> %q", content, got)
		}
	}
}

func TestEveryThemeHasBackground(t *testing.T) {
	for _, n := range themeNames {
		if themeBackground[n] == "" {
			t.Errorf("no window background for %s", n)
		}
	}
}
