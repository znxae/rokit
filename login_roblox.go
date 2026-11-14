package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
)

// Optional Roblox login. The password is typed on roblox.com in a separate window with its own
// in-memory web session; this app only reads the resulting session cookie, keeps it in the Windows
// Credential Manager, and sends it solely to the three Roblox API hosts below (never to the CDN, never logged).
// It opens only what that account itself may open: no access control is bypassed.

const (
	secretCookie   = "roblox-session"
	secretAccount  = "roblox-account"
	cookieName     = ".ROBLOSECURITY"
	msgRobloxPrivA = "Akun Roblox yang masuk juga tidak punya akses ke aset ini"
	loginTimeout   = 10 * time.Minute
)

// authHosts are the only hosts that ever receive the cookie.
var authHosts = map[string]bool{"assetdelivery.roblox.com": true, "economy.roblox.com": true, "users.roblox.com": true}

// RobloxAccount is who is logged in (LoggedIn false when nobody).
type RobloxAccount struct {
	LoggedIn bool   `json:"loggedIn"`
	Name     string `json:"name"`
	ID       int64  `json:"id"`
}

var (
	authMu     sync.Mutex
	authCookie string
	authLoaded bool
	loginMu    sync.Mutex
)

// ValidSessionCookie checks the shape only: printable ASCII, no separators that could inject a header.
func ValidSessionCookie(v string) bool {
	if len(v) < 100 || len(v) > 2000 {
		return false
	}
	for _, r := range v {
		if r <= ' ' || r > '~' || r == ';' || r == ',' || r == '"' || r == 0x5c { // 0x5c is a backslash
			return false
		}
	}
	return true
}

// ParseAuthUser reads users.roblox.com/v1/users/authenticated.
func ParseAuthUser(b []byte) (RobloxAccount, error) {
	var u struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	if json.Unmarshal(b, &u) != nil || u.ID <= 0 || u.Name == "" {
		return RobloxAccount{}, errors.New("login Roblox tidak valid")
	}
	return RobloxAccount{LoggedIn: true, Name: u.Name, ID: u.ID}, nil
}

func storedCookie() string {
	authMu.Lock()
	defer authMu.Unlock()
	if !authLoaded {
		authLoaded = true
		if b, err := mygo.Secrets.Get(secretCookie); err == nil && ValidSessionCookie(string(b)) {
			authCookie = string(b)
		}
	}
	return authCookie
}

func setStoredCookie(v string) {
	authMu.Lock()
	authCookie, authLoaded = v, true
	authMu.Unlock()
}

// attachAuth adds the cookie to a request, only for https requests to authHosts.
func attachAuth(req *http.Request) {
	if req.URL.Scheme != "https" || !authHosts[strings.ToLower(req.URL.Hostname())] {
		return
	}
	if c := storedCookie(); c != "" {
		req.Header.Set("Cookie", cookieName+"="+c)
	}
}

func loggedIn() bool { return storedCookie() != "" }

// RobloxAccountInfo says who is logged in, from what is stored (no network).
func (r *Rokit) RobloxAccountInfo() RobloxAccount {
	if !loggedIn() {
		return RobloxAccount{}
	}
	var a RobloxAccount
	if b, err := mygo.Secrets.Get(secretAccount); err == nil {
		_ = json.Unmarshal(b, &a)
	}
	a.LoggedIn = true
	return a
}

// RobloxLogout forgets the stored login. The session on roblox.com itself stays valid until you sign out there.
func (r *Rokit) RobloxLogout() error {
	setStoredCookie("")
	var first error
	for _, k := range []string{secretCookie, secretAccount} {
		if err := mygo.Secrets.Delete(k); err != nil && !errors.Is(err, mygo.ErrSecretNotFound) && first == nil {
			first = err
		}
	}
	return first
}

// RobloxLogin opens the official Roblox login page in its own window and waits for the login to finish.
func (r *Rokit) RobloxLogin(ctx context.Context) (RobloxAccount, error) {
	if !loginMu.TryLock() {
		return RobloxAccount{}, errors.New("jendela login Roblox sudah terbuka")
	}
	defer loginMu.Unlock()

	sess := mygo.NewEphemeralSession() // in memory, apart from the app's own pages; the page cannot call Go
	w := mygo.NewWindow(mygo.WindowOptions{Title: "Masuk Roblox", URL: "https://www.roblox.com/login", Width: 520, Height: 740, Page: mygo.PageOptions{Session: sess}})
	closed := make(chan struct{})
	w.OnClosed(func() { close(closed) })
	defer w.Close()

	deadline := time.After(loginTimeout)
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return RobloxAccount{}, ctx.Err()
		case <-closed:
			return RobloxAccount{}, errors.New("jendela login ditutup sebelum selesai masuk")
		case <-deadline:
			return RobloxAccount{}, errors.New("login Roblox kedaluwarsa (10 menit); coba lagi")
		case <-tick.C:
		}
		list, err := sess.Cookies.List(mygo.CookieFilter{Name: cookieName})
		if err != nil {
			continue
		}
		for _, c := range list {
			if !strings.HasSuffix(strings.TrimPrefix(c.Domain, "."), "roblox.com") || !ValidSessionCookie(c.Value) {
				continue
			}
			acc, err := verifyCookie(ctx, c.Value)
			if err != nil {
				return RobloxAccount{}, err
			}
			ab, _ := json.Marshal(acc)
			if err := mygo.Secrets.Set(secretCookie, []byte(c.Value)); err != nil {
				return RobloxAccount{}, fmt.Errorf("tidak bisa menyimpan login ke Credential Manager: %w", err)
			}
			_ = mygo.Secrets.Set(secretAccount, ab)
			setStoredCookie(c.Value)
			return acc, nil
		}
	}
}

// verifyCookie asks Roblox who the cookie belongs to.
func verifyCookie(ctx context.Context, cookie string) (RobloxAccount, error) {
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, "https://users.roblox.com/v1/users/authenticated", nil)
	if err != nil {
		return RobloxAccount{}, err
	}
	req.Header.Set("User-Agent", robloxUA)
	req.Header.Set("Cookie", cookieName+"="+cookie)
	resp, err := robloxAPI.Do(req)
	if err != nil {
		return RobloxAccount{}, fmt.Errorf("tidak bisa menghubungi Roblox: %w", err)
	}
	defer resp.Body.Close()
	var b [4096]byte
	n, _ := resp.Body.Read(b[:])
	if resp.StatusCode != 200 {
		return RobloxAccount{}, errors.New("login Roblox tidak valid")
	}
	return ParseAuthUser(b[:n])
}
