package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/egoist/mygo"
)

// RobloxSaved is the finished download.
type RobloxSaved struct {
	Path    string `json:"path"`
	Ext     string `json:"ext"`
	Raw     bool   `json:"raw"`     // a Roblox-native format, not a common file
	IsAudio bool   `json:"isAudio"` // can be opened in the Audio tab
	Note    string `json:"note"`
	Via     string `json:"via"` // "Decal -> gambar ID 123" when a wrapper was resolved to its image
}

const (
	pageGap       = time.Second // batch pace: one request per second
	maxMeshBytes  = 64 << 20    // largest mesh converted to OBJ in memory
	maxRetryWait  = 60 * time.Second
	maxWrapperXML = 1 << 20
)

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

// pacer spaces requests at least gap apart. The zero gap (single downloads) never waits.
type pacer struct {
	mu   sync.Mutex
	gap  time.Duration
	next time.Time
}

type paceKey struct{}

func withPace(ctx context.Context, gap time.Duration) context.Context {
	return context.WithValue(ctx, paceKey{}, &pacer{gap: gap})
}

func paceWait(ctx context.Context) error {
	p, _ := ctx.Value(paceKey{}).(*pacer)
	if p == nil || p.gap == 0 {
		return nil
	}
	p.mu.Lock()
	start := time.Now()
	if p.next.After(start) {
		start = p.next
	}
	wait := time.Until(start)
	p.next = start.Add(p.gap)
	p.mu.Unlock()
	if wait <= 0 {
		return nil
	}
	select {
	case <-time.After(wait):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// robloxGet reads a small API answer. No credentials are ever attached.
func robloxGet(ctx context.Context, url string) ([]byte, error) {
	return robloxDo(ctx, http.MethodGet, url, nil, nil)
}

// robloxDo is robloxGet with a method, optional JSON body and extra headers; a 429 carries Retry-After.
func robloxDo(ctx context.Context, method, url string, body []byte, hdr map[string]string) ([]byte, error) {
	if err := paceWait(ctx); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", robloxUA)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := robloxAPI.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tidak bisa menghubungi Roblox: %w", err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if err := StatusError(resp.StatusCode); err != nil {
		var re *RateError
		if errors.As(err, &re) {
			re.Wait = ParseRetryAfter(resp.Header.Get("Retry-After"), time.Now())
		}
		return nil, err
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

func robloxLocation(ctx context.Context, id int64, o RobloxOpts, typeID int) (string, int, error) {
	var hdr map[string]string
	if o.Avatar {
		f := AvatarFormatFor(typeID)
		if f == "" {
			return "", 0, errors.New("model avatar hanya untuk aksesori dan kepala")
		}
		hdr = map[string]string{"Roblox-AssetFormat": f}
	}
	b, err := robloxDo(ctx, http.MethodGet, DeliveryURL(id, o), nil, hdr)
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
	if _, _, derr := robloxLocation(cctx, id, RobloxOpts{}, 0); derr != nil {
		var re *RateError
		if errors.As(derr, &re) {
			return AssetMeta{}, derr
		}
		m.Blocked = derr.Error()
	}
	if IsWrapperType(m.TypeID) && m.Blocked == "" {
		m.Note = TypeNote(m.TypeID) + " Akan diunduh sebagai gambarnya (PNG)."
	}
	return m, nil
}

// RobloxDownload saves the asset to Downloads/Rokit-Roblox/<id>-<name>.<ext>, never overwriting.
// A Decal, Face, Shirt, Pants or T-Shirt is a public XML wrapper: the image it points at is fetched instead.
// Private, age-gated or removed assets stop with a message; nothing tries to get around that.
// opts: a version number, the avatar-ready model, or OBJ for meshes (see RobloxOpts).
func (r *Rokit) RobloxDownload(ctx context.Context, input string, opts RobloxOpts) (RobloxSaved, error) {
	id, err := ParseAssetRef(input)
	if err != nil {
		return RobloxSaved{}, err
	}
	if err := opts.Validate(); err != nil {
		return RobloxSaved{}, err
	}
	return robloxFetch(ctx, id, 0, opts, nil)
}

// robloxFetch downloads one asset. pre is an already looked-up location (from the batch endpoint) or nil.
func robloxFetch(ctx context.Context, id int64, depth int, opts RobloxOpts, pre *Delivered) (RobloxSaved, error) {
	cctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	meta, err := robloxDetails(cctx, id)
	if err != nil {
		return RobloxSaved{}, err
	}
	var loc string
	var typeID int
	switch {
	case pre != nil && pre.Err != nil:
		return RobloxSaved{}, pre.Err
	case pre != nil:
		loc, typeID = pre.Loc, pre.TypeID
	default:
		if loc, typeID, err = robloxLocation(cctx, id, opts, meta.TypeID); err != nil {
			return RobloxSaved{}, err
		}
	}
	if !ValidCDNURL(loc) {
		return RobloxSaved{}, errors.New("Roblox memberi alamat unduhan yang tidak diizinkan (bukan *.rbxcdn.com lewat https)")
	}
	if typeID == 0 {
		typeID = meta.TypeID
	}
	if err := paceWait(cctx); err != nil {
		return RobloxSaved{}, err
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
	if err := StatusError(resp.StatusCode); err != nil {
		return RobloxSaved{}, err
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

	// Only the first bytes are needed, except for a small wrapper XML, which is read whole.
	f, err := os.Open(tmp.Name())
	if err != nil {
		return RobloxSaved{}, err
	}
	body, err := io.ReadAll(io.LimitReader(f, maxWrapperXML+1))
	f.Close()
	if err != nil {
		return RobloxSaved{}, err
	}
	ext, raw := ExtFor(typeID, body[:min(len(body), 16)])

	if IsWrapperType(typeID) && ext == "rbxmx" && n <= maxWrapperXML && depth == 0 {
		if imgID, ok := ParseWrapperImage(typeID, body); ok {
			saved, err := robloxFetch(ctx, imgID, depth+1, RobloxOpts{}, nil)
			via := fmt.Sprintf("%s -> gambar ID %d", AssetTypeName(typeID), imgID)
			if err != nil {
				return RobloxSaved{}, fmt.Errorf("%s: %w", via, err)
			}
			saved.Via = via
			saved.Note = via + ". " + saved.Note
			return saved, nil
		}
	}

	stem := RobloxFileStem(id, meta.Name)
	if opts.OBJ && (typeID == 4 || typeID == 40) && ext == "mesh" {
		if n > maxMeshBytes {
			return RobloxSaved{}, errors.New("mesh terlalu besar untuk dikonversi ke OBJ; unduh sebagai .mesh mentah")
		}
		data, err := os.ReadFile(tmp.Name())
		if err != nil {
			return RobloxSaved{}, err
		}
		obj, err := MeshToOBJ(data)
		if err != nil {
			return RobloxSaved{}, err
		}
		if err := os.WriteFile(tmp.Name(), obj, 0o644); err != nil {
			return RobloxSaved{}, err
		}
		dst := UniquePath(dir, stem, "obj")
		if err := moveFile(tmp.Name(), dst); err != nil {
			return RobloxSaved{}, err
		}
		return RobloxSaved{Path: dst, Ext: "obj", Note: "Mesh dikonversi ke OBJ (posisi, UV, normal)."}, nil
	}
	if opts.Version > 0 {
		stem += "-v" + strconv.Itoa(opts.Version)
	}
	dst := UniquePath(dir, stem, ext)
	if err := moveFile(tmp.Name(), dst); err != nil {
		return RobloxSaved{}, err
	}
	return RobloxSaved{Path: dst, Ext: ext, Raw: raw, IsAudio: typeID == 3 && !raw, Note: TypeNote(typeID)}, nil
}

// RobloxDownloadMany downloads a pasted list, one line at a time, one request per second, and streams
// each item's state. Items that need a login are marked "gagal" and the run goes on; a 429 stops it.
func (r *Rokit) RobloxDownloadMany(ctx context.Context, text string, progress *mygo.Channel[BatchItem]) ([]BatchItem, error) {
	lines, err := SplitAssetList(text)
	if err != nil {
		return nil, err
	}
	items := make([]BatchItem, len(lines))
	for i, l := range lines {
		items[i] = BatchItem{Index: i, Input: l, Status: "menunggu"}
		_ = progress.Send(items[i])
	}
	pctx := withPace(ctx, pageGap)
	pre := robloxBatchLookup(pctx, items)
	seen := map[int64]bool{}
	stopped := ""
	for i := range items {
		it := &items[i]
		if stopped != "" || ctx.Err() != nil {
			it.Status, it.Message = "dilewati", stopped
			if ctx.Err() != nil {
				it.Message = "dibatalkan"
			}
			_ = progress.Send(*it)
			continue
		}
		id, perr := ParseAssetRef(it.Input)
		switch {
		case perr != nil:
			it.Status, it.Message = "gagal", perr.Error()
		case seen[id]:
			it.Status, it.Message = "dilewati", "duplikat dalam daftar"
		default:
			seen[id] = true
			it.Status = "jalan"
			_ = progress.Send(*it)
			d := pre[id]
			s, ferr := robloxFetch(pctx, id, 0, RobloxOpts{}, d)
			var re *RateError
			if errors.As(ferr, &re) && re.Wait > 0 && re.Wait <= maxRetryWait {
				it.Message = fmt.Sprintf("menunggu %d dtk (Roblox membatasi)", int(re.Wait.Seconds()))
				_ = progress.Send(*it)
				select {
				case <-time.After(re.Wait):
					s, ferr = robloxFetch(pctx, id, 0, RobloxOpts{}, d)
				case <-ctx.Done():
					ferr = ctx.Err()
				}
			}
			if ferr != nil {
				it.Status, it.Message = "gagal", ferr.Error()
				if errors.As(ferr, &re) {
					stopped = "berhenti karena Roblox membatasi permintaan; coba lagi nanti"
				}
			} else {
				it.Status, it.Path, it.Message = "selesai", s.Path, s.Via
			}
		}
		_ = progress.Send(*it)
	}
	return items, nil
}

// robloxBatchLookup asks the batch endpoint for every distinct valid ID in one request, saving one request
// per asset. Any failure returns nil and each asset is then looked up on its own.
func robloxBatchLookup(ctx context.Context, items []BatchItem) map[int64]*Delivered {
	var ids []int64
	seen := map[int64]bool{}
	for _, it := range items {
		if id, err := ParseAssetRef(it.Input); err == nil && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	if len(ids) < 2 {
		return nil
	}
	b, err := robloxDo(ctx, http.MethodPost, "https://assetdelivery.roblox.com/v2/assets/batch", BatchBody(ids), nil)
	if err != nil {
		return nil
	}
	got, err := ParseBatch(b)
	if err != nil {
		return nil
	}
	out := map[int64]*Delivered{}
	for id, d := range got {
		d := d
		out[id] = &d
	}
	return out
}
