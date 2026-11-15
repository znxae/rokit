package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

// Themes the page can ask for; "auto" follows the system and is resolved by the page.
var themeNames = []string{"auto", "apple", "dark", "material", "material-dark", "oled", "glass"}

// themeBackground is the window color shown until the page paints (matches --bg in themes.css).
var themeBackground = map[string]string{
	"auto": "#F5F5F7", "apple": "#F5F5F7", "dark": "#1C1C1E", "material": "#F4FBF8", "material-dark": "#0E1513", "oled": "#000000", "glass": "#D6E9F8",
}

type settings struct {
	Theme string `json:"theme"`
	// LoginNoticeSeen: the one-time Roblox login explanation was accepted. Not a secret.
	LoginNoticeSeen bool `json:"loginNoticeSeen,omitempty"`
	// SpeedMode is the last chosen audio speed mode: "tempo" (Normal, default) or "pitch" (Roblox).
	SpeedMode string `json:"speedMode,omitempty"`
}

// settingsPath is a variable so tests can point it at a temp dir.
var settingsPath = func() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "Rokit", "settings.json"), nil
}

func loadSettings() settings {
	s := settings{Theme: "auto", SpeedMode: modeTempo}
	p, err := settingsPath()
	if err != nil {
		return s
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return s
	}
	var got settings
	if json.Unmarshal(b, &got) == nil && slices.Contains(themeNames, got.Theme) {
		s.Theme = got.Theme
	}
	s.LoginNoticeSeen = got.LoginNoticeSeen
	if got.SpeedMode == modePitch {
		s.SpeedMode = modePitch
	}
	return s // a missing or corrupt file just means the default
}

func saveSettings(s settings) error {
	p, err := settingsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	b, _ := json.Marshal(s)
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p) // never leaves a half-written file
}

// Theme returns the saved theme name ("auto" by default).
func (r *Rokit) Theme() string { return loadSettings().Theme }

// SetTheme saves the theme; unknown names are rejected.
func (r *Rokit) SetTheme(name string) error {
	if !slices.Contains(themeNames, name) {
		return fmt.Errorf("tema %q tidak dikenal", name)
	}
	if name == "" {
		return errors.New("tema kosong")
	}
	s := loadSettings()
	s.Theme = name
	return saveSettings(s)
}

// SpeedMode returns the saved audio speed mode ("tempo" by default).
func (r *Rokit) SpeedMode() string { return loadSettings().SpeedMode }

// SetSpeedMode saves the audio speed mode; unknown values are rejected.
func (r *Rokit) SetSpeedMode(mode string) error {
	if mode != modeTempo && mode != modePitch {
		return fmt.Errorf("mode kecepatan %q tidak dikenal", mode)
	}
	s := loadSettings()
	s.SpeedMode = mode
	return saveSettings(s)
}
