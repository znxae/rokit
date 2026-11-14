package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
)

// PickVideo opens the native file dialog for video files; "" means cancelled.
func (r *Rokit) PickVideo() (string, error) {
	sel, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Title: "Pilih file video",
		Filters: []mygo.FileFilter{
			{Name: "Video", Extensions: videoExts},
			{Name: "Semua file", Extensions: []string{"*"}},
		},
	})
	if err != nil || len(sel) == 0 {
		return "", err
	}
	return sel[0], nil
}

// PickFolder opens the native folder dialog; "" means cancelled.
func (r *Rokit) PickFolder() (string, error) {
	sel, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Title: "Pilih folder output", Directory: true, CreateDirectories: true})
	if err != nil || len(sel) == 0 {
		return "", err
	}
	return sel[0], nil
}

// OpenFolder shows a folder in the file manager.
func (r *Rokit) OpenFolder(path string) error { return mygo.Shell.OpenPath(path) }

// ProbeVideo reads duration, frame rate and streams of a file.
func (r *Rokit) ProbeVideo(ctx context.Context, path string) (VideoInfo, error) {
	return probeFile(ctx, path)
}

// SuggestFramesDir is Downloads/<video name>, made unique so nothing is overwritten.
func (r *Rokit) SuggestFramesDir(input string) string {
	stem := strings.TrimSuffix(filepath.Base(input), filepath.Ext(input))
	return UniqueDirPath(downloadsDir(), SanitizeFilename(stem))
}

// ExtractFrames writes frame_00001.png|jpg... into a fresh folder and streams progress.
// Closing the channel on the page cancels it and removes the partial folder.
func (r *Rokit) ExtractFrames(ctx context.Context, input, outDir string, o FrameOpts, progress *mygo.Channel[FrameProgress]) (FramesResult, error) {
	if err := o.Validate(); err != nil {
		return FramesResult{}, err
	}
	if st, err := os.Stat(input); err != nil || st.IsDir() {
		return FramesResult{}, fmt.Errorf("file video tidak ditemukan: %s", input)
	}
	if outDir == "" {
		return FramesResult{}, errors.New("pilih folder output dulu")
	}
	info, perr := probeFile(ctx, input)
	if perr == nil && !info.HasVideo {
		return FramesResult{}, errors.New("file ini tidak punya gambar (video)")
	}
	dir, err := ResolveOutDir(outDir)
	if err != nil {
		return FramesResult{}, err
	}
	total := EstimateFrames(info, o)

	cmd := exec.CommandContext(ctx, "ffmpeg", BuildFrameArgs(input, dir, o)...)
	hideWindow(cmd)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.StdoutPipe()
	if err == nil {
		err = cmd.Start()
	}
	if err != nil {
		_ = os.RemoveAll(dir)
		if errors.Is(err, exec.ErrNotFound) {
			return FramesResult{}, errors.New(ffmpegMsg)
		}
		return FramesResult{}, err
	}
	scanLines(out, func(line string) {
		if n, ok := ParseFFmpegProgress(line); ok {
			_ = progress.Send(FrameProgress{Done: n, Total: total})
		}
	})
	werr := cmd.Wait()
	if ctx.Err() != nil {
		_ = os.RemoveAll(dir)
		return FramesResult{}, ctx.Err()
	}
	if werr != nil {
		_ = os.RemoveAll(dir)
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return FramesResult{}, errors.New(msg)
		}
		return FramesResult{}, werr
	}
	n := countFrames(dir, o.Format)
	if n == 0 {
		_ = os.RemoveAll(dir)
		return FramesResult{}, errNoFrames
	}
	_ = progress.Send(FrameProgress{Done: n, Total: n})
	r.removeTemp(input)
	return FramesResult{Dir: dir, Count: n}, nil
}

func scanLines(rd io.Reader, fn func(string)) {
	sc := bufio.NewScanner(rd)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		fn(sc.Text())
	}
}

// Info fetches title, duration and thumbnail of one video ("Cek").
func (r *Rokit) Info(ctx context.Context, url string) (VideoMeta, error) {
	url = strings.TrimSpace(url)
	if !ValidURL(url) {
		return VideoMeta{}, errors.New("URL harus diawali http:// atau https://")
	}
	cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, "yt-dlp", "-J", "--no-playlist", "--no-warnings", "--", url)
	hideWindow(cmd)
	cmd.Env = append(os.Environ(), "PYTHONIOENCODING=utf-8")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return VideoMeta{}, ytdlpError(err, stderr.String())
	}
	meta, thumb, err := ParseMeta(stdout.Bytes())
	if err != nil {
		return VideoMeta{}, err
	}
	meta.Thumbnail = fetchThumbnail(cctx, thumb)
	return meta, nil
}

func ytdlpError(err error, stderr string) error {
	if errors.Is(err, exec.ErrNotFound) {
		return errors.New(ytdlpMsg)
	}
	if msg := strings.TrimSpace(stderr); msg != "" {
		return errors.New(msg)
	}
	return err
}

// fetchThumbnail returns a data URL ("" on any problem): https only, images only, max 3 MiB.
func fetchThumbnail(ctx context.Context, url string) string {
	if !strings.HasPrefix(url, "https://") {
		return ""
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return ""
	}
	resp, err := (&http.Client{Timeout: 8 * time.Second}).Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	ct := resp.Header.Get("Content-Type")
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(ct, "image/") {
		return ""
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 3<<20))
	if err != nil || len(b) == 0 {
		return ""
	}
	return "data:" + ct + ";base64," + base64.StdEncoding.EncodeToString(b)
}

// DownloadVideo downloads one video into a temp folder, then moves it to Downloads
// under its sanitized title, never overwriting. Closing the channel on the page
// cancels it, kills yt-dlp (and its ffmpeg) and deletes the temp folder.
func (r *Rokit) DownloadVideo(ctx context.Context, url string, o DLOpts, progress *mygo.Channel[DLProgress]) (Saved, error) {
	dir, err := os.MkdirTemp("", "rokit_vid_")
	if err != nil {
		return Saved{}, err
	}
	defer os.RemoveAll(dir) // success moves the file out first
	args, err := BuildDownloadArgs(url, dir, o)
	if err != nil {
		return Saved{}, err
	}
	cmd := exec.CommandContext(ctx, "yt-dlp", args...)
	hideWindow(cmd)
	cmd.Env = append(os.Environ(), "PYTHONIOENCODING=utf-8")
	stdout, err1 := cmd.StdoutPipe()
	stderr, err2 := cmd.StderrPipe()
	if err := errors.Join(err1, err2); err != nil {
		return Saved{}, err
	}
	if err := cmd.Start(); err != nil {
		return Saved{}, ytdlpError(err, "")
	}

	var mu sync.Mutex
	var lines, errTail []string
	handle := func(isErr bool) func(string) {
		return func(line string) {
			if p, ok := ParseProgress(line); ok {
				mu.Lock()
				_ = progress.Send(p)
				mu.Unlock()
				return
			}
			mu.Lock()
			if isErr {
				errTail = append(errTail, line)
				if len(errTail) > 20 {
					errTail = errTail[1:]
				}
			} else if strings.TrimSpace(line) != "" {
				lines = append(lines, strings.TrimSpace(line))
			}
			mu.Unlock()
		}
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); scanLines(stdout, handle(false)) }()
	go func() { defer wg.Done(); scanLines(stderr, handle(true)) }()
	wg.Wait() // before Wait: Wait closes the pipes
	werr := cmd.Wait()
	if ctx.Err() != nil {
		return Saved{}, ctx.Err()
	}
	if werr != nil {
		return Saved{}, ytdlpError(werr, strings.Join(errTail, "\n"))
	}

	src := ""
	if len(lines) > 0 {
		src = lines[len(lines)-1]
	}
	if st, err := os.Stat(src); err != nil || st.IsDir() {
		src = ""
		if es, _ := os.ReadDir(dir); len(es) > 0 {
			src = filepath.Join(dir, es[0].Name())
		}
	}
	if src == "" {
		return Saved{}, errors.New("yt-dlp selesai tetapi tidak menghasilkan file")
	}
	ext := strings.TrimPrefix(filepath.Ext(src), ".")
	stem := strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))
	dst := UniquePath(downloadsDir(), SanitizeFilename(stem), ext)
	if err := moveFile(src, dst); err != nil {
		return Saved{}, err
	}
	_ = progress.Send(DLProgress{Percent: 100})
	return Saved{Path: dst}, nil
}

func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	in, err := os.Open(src) // different volume: copy then delete
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		_ = os.Remove(dst)
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Remove(src)
}
