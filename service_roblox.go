package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// RobloxSaved is the finished download.
type RobloxSaved struct {
	Path    string `json:"path"`
	Ext     string `json:"ext"`
	Raw     bool   `json:"raw"`     // a Roblox-native format, not a common file
	IsAudio bool   `json:"isAudio"` // can be opened in the Audio tab
	Note    string `json:"note"`
}

// The API client never follows redirects: the endpoints answer directly, so a redirect is a surprise.
var robloxAPI = &http.Client{
	Timeout:       20 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

// The CDN client follows redirects only to https://*.rbxcdn.com, at most 3 times.
var robloxCDN = &http.Client{
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 3 || !ValidCDNURL(req.URL.String()) {
			return errors.New("Roblox mengalihkan unduhan ke alamat yang tidak diizinkan")
		}
		return nil
	},
}

// robloxGet reads a small API answer. No credentials are ever attached.
func robloxGet(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", robloxUA)
	req.Header.Set("Accept", "application/json")
	resp, err := robloxAPI.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tidak bisa menghubungi Roblox: %w", err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		return nil, errors.New(msgRobloxRate)
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, errors.New(msgRobloxPriv)
	case resp.StatusCode == http.StatusNotFound:
		return nil, errors.New(msgRobloxGone)
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("Roblox menjawab HTTP %d", resp.StatusCode)
	}
	return b, nil
}

func robloxDetails(ctx context.Context, id int64) (AssetMeta, error) {
	b, err := robloxGet(ctx, "https://economy.roblox.com/v2/assets/"+strconv.FormatInt(id, 10)+"/details")
	if err != nil {
		return AssetMeta{}, err
	}
	return ParseDetails(b)
}

func robloxLocation(ctx context.Context, id int64) (string, int, error) {
	b, err := robloxGet(ctx, "https://assetdelivery.roblox.com/v2/assetId/"+strconv.FormatInt(id, 10))
	if err != nil {
		return "", 0, err
	}
	return ParseDelivery(b)
}

// RobloxCheck ("Cek") parses the input and fetches name, type and creator. It also asks the delivery
// endpoint whether the asset can be fetched without a login, so the page can say so before "Unduh".
func (r *Rokit) RobloxCheck(ctx context.Context, input string) (AssetMeta, error) {
	id, err := ParseAssetRef(input)
	if err != nil {
		return AssetMeta{}, err
	}
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	m, err := robloxDetails(cctx, id)
	if err != nil {
		return AssetMeta{}, err
	}
	if _, _, derr := robloxLocation(cctx, id); derr != nil {
		if derr.Error() == msgRobloxRate {
			return AssetMeta{}, derr
		}
		m.Blocked = derr.Error()
	}
	return m, nil
}

// RobloxDownload saves the raw asset to Downloads/Rokit-Roblox/<id>-<name>.<ext>, never overwriting.
// Private, age-gated or removed assets stop with a message; nothing tries to get around that.
func (r *Rokit) RobloxDownload(ctx context.Context, input string) (RobloxSaved, error) {
	id, err := ParseAssetRef(input)
	if err != nil {
		return RobloxSaved{}, err
	}
	cctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	meta, err := robloxDetails(cctx, id)
	if err != nil {
		return RobloxSaved{}, err
	}
	loc, typeID, err := robloxLocation(cctx, id)
	if err != nil {
		return RobloxSaved{}, err
	}
	if !ValidCDNURL(loc) {
		return RobloxSaved{}, errors.New("Roblox memberi alamat unduhan yang tidak diizinkan (bukan *.rbxcdn.com lewat https)")
	}
	if typeID == 0 {
		typeID = meta.TypeID
	}

	req, err := http.NewRequestWithContext(cctx, http.MethodGet, loc, nil)
	if err != nil {
		return RobloxSaved{}, err
	}
	req.Header.Set("User-Agent", robloxUA)
	resp, err := robloxCDN.Do(req) // Go decodes the gzip the CDN serves, since we set no Accept-Encoding ourselves
	if err != nil {
		return RobloxSaved{}, fmt.Errorf("unduhan gagal: %w", err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		return RobloxSaved{}, errors.New(msgRobloxRate)
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return RobloxSaved{}, errors.New(msgRobloxPriv)
	case resp.StatusCode != http.StatusOK:
		return RobloxSaved{}, fmt.Errorf("unduhan gagal: HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > robloxMaxBytes {
		return RobloxSaved{}, fmt.Errorf("aset terlalu besar (batas %d MB)", robloxMaxBytes>>20)
	}

	dir := filepath.Join(downloadsDir(), robloxDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return RobloxSaved{}, err
	}
	tmp, err := os.CreateTemp(dir, ".rokit-*.part")
	if err != nil {
		return RobloxSaved{}, err
	}
	defer os.Remove(tmp.Name()) // a no-op after the rename
	n, err := io.Copy(tmp, io.LimitReader(resp.Body, robloxMaxBytes+1))
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return RobloxSaved{}, fmt.Errorf("unduhan terputus: %w", err)
	}
	if n > robloxMaxBytes {
		return RobloxSaved{}, fmt.Errorf("aset terlalu besar (batas %d MB)", robloxMaxBytes>>20)
	}
	if n == 0 {
		return RobloxSaved{}, errors.New("Roblox mengirim file kosong")
	}

	f, err := os.Open(tmp.Name())
	if err != nil {
		return RobloxSaved{}, err
	}
	head := make([]byte, 16)
	k, _ := io.ReadFull(f, head)
	f.Close()
	ext, raw := ExtFor(typeID, head[:k])
	dst := UniquePath(dir, RobloxFileStem(id, meta.Name), ext)
	if err := moveFile(tmp.Name(), dst); err != nil {
		return RobloxSaved{}, err
	}
	return RobloxSaved{Path: dst, Ext: ext, Raw: raw, IsAudio: typeID == 3 && !raw, Note: TypeNote(typeID)}, nil
}
