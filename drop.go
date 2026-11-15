package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// DropItem is one dropped path and what it is: audio, video, image (PNG), folder, missing or other.
type DropItem struct {
	Path string `json:"path"`
	Name string `json:"name"`
	Ext  string `json:"ext"`
	Kind string `json:"kind"`
}

// KindOfFile classifies by extension only (same lists as the file pickers); "other" when unknown.
func KindOfFile(name string) string {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))
	switch {
	case slices.Contains(audioExts, ext):
		return "audio"
	case slices.Contains(videoExts, ext):
		return "video"
	case ext == "png":
		return "image"
	}
	return "other"
}

// ClassifyDrop describes dropped paths for the page; the page decides per tab what to do with them.
func (r *Rokit) ClassifyDrop(paths []string) []DropItem {
	out := make([]DropItem, 0, len(paths))
	for _, p := range paths {
		it := DropItem{Path: p, Name: filepath.Base(p), Ext: strings.ToLower(filepath.Ext(p)), Kind: KindOfFile(p)}
		if st, err := os.Stat(p); err != nil {
			it.Kind = "missing"
		} else if st.IsDir() {
			it.Kind = "folder"
		}
		out = append(out, it)
	}
	return out
}
