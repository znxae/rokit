package main

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strconv"
	"strings"
)

// VideoInfo is what ffprobe says about a media file.
type VideoInfo struct {
	Duration float64 `json:"duration"` // seconds, 0 if unknown
	FPS      float64 `json:"fps"`      // average frame rate of the video stream
	Width    int     `json:"width"`
	Height   int     `json:"height"`
	HasVideo bool    `json:"hasVideo"`
	HasAudio bool    `json:"hasAudio"`
}

const noAudioMsg = "Video ini tidak punya audio"

// ParseFraction reads "30000/1001" or "25" (0 when invalid or "0/0").
func ParseFraction(s string) float64 {
	n, d, found := strings.Cut(strings.TrimSpace(s), "/")
	a, err := strconv.ParseFloat(n, 64)
	if err != nil {
		return 0
	}
	if !found {
		return a
	}
	b, err := strconv.ParseFloat(d, 64)
	if err != nil || b == 0 {
		return 0
	}
	return a / b
}

// ParseProbe turns `ffprobe -of json` output into VideoInfo.
func ParseProbe(b []byte) (VideoInfo, error) {
	var raw struct {
		Streams []struct {
			Type string `json:"codec_type"`
			Avg  string `json:"avg_frame_rate"`
			R    string `json:"r_frame_rate"`
			W    int    `json:"width"`
			H    int    `json:"height"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return VideoInfo{}, err
	}
	var v VideoInfo
	v.Duration, _ = strconv.ParseFloat(raw.Format.Duration, 64)
	for _, s := range raw.Streams {
		switch s.Type {
		case "audio":
			v.HasAudio = true
		case "video":
			if v.HasVideo {
				continue // first video stream only
			}
			v.HasVideo = true
			v.Width, v.Height = s.W, s.H
			if v.FPS = ParseFraction(s.Avg); v.FPS == 0 {
				v.FPS = ParseFraction(s.R)
			}
		}
	}
	return v, nil
}

// NoAudioStderr recognises ffmpeg's complaint about an output without any stream.
func NoAudioStderr(stderr string) bool {
	return strings.Contains(stderr, "does not contain any stream") ||
		strings.Contains(stderr, "Output file #0 does not contain any stream")
}

func probeFile(ctx context.Context, path string) (VideoInfo, error) {
	cmd := exec.CommandContext(ctx, "ffprobe", "-v", "error",
		"-show_entries", "stream=codec_type,avg_frame_rate,r_frame_rate,width,height:format=duration",
		"-of", "json", path)
	hideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return VideoInfo{}, errors.New(ffmpegMsg)
		}
		return VideoInfo{}, err
	}
	return ParseProbe(out)
}
