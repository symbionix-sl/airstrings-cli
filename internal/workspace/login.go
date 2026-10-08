package workspace

import (
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/symbionix-sl/airstrings-cli/internal/client"
)

var (
	ErrLoginPending = errors.New("login not approved yet")
	ErrLoginExpired = errors.New("login code expired")
	ErrGrantLost    = errors.New("approval did not reach this terminal")
)

const PollBudget = 90 * time.Second

func PendingNextStep(url string) string {
	return "Approve in the browser at " + url + ", then run the same command again"
}

// StartLogin returns the unexpired pending login for baseURL, or starts a new
// one; fresh reports a new start. With loopback it listens on 127.0.0.1 for the
// approval redirect (l is nil when the API declines or the port is unavailable);
// a loopback pending is resumed only if its port can be bound again. The store
// is written before the API call so an unwritable store fails before any server
// state exists.
func StartLogin(baseURL, clientName string, loopback bool) (p *PendingLogin, fresh bool, l net.Listener, err error) {
	creds, err := LoadCreds()
	if err != nil {
		return nil, false, nil, err
	}
	if p := creds.PendingFor(baseURL); p != nil {
		if p.LoopbackPort == 0 {
			return p, false, nil, nil
		}
		if loopback {
			if l, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(p.LoopbackPort)); err == nil {
				return p, false, l, nil
			}
		}
	}
	if err := SaveCreds(creds); err != nil {
		return nil, false, nil, err
	}
	c := client.New("", baseURL, "", "")
	redirect := ""
	if loopback {
		if l, _ = net.Listen("tcp", "127.0.0.1:0"); l != nil {
			redirect = "http://" + l.Addr().String() + "/callback"
		}
	}
	s, err := c.StartCLIAuth(clientName, redirect)
	var apiErr *client.APIError
	if l != nil && (err == nil && !s.Loopback || errors.As(err, &apiErr) && apiErr.StatusCode/100 == 4) {
		l.Close()
		l = nil
		if err != nil {
			s, err = c.StartCLIAuth(clientName, "")
		}
	}
	if err != nil {
		if l != nil {
			l.Close()
		}
		return nil, false, nil, err
	}
	creds.Pending = &PendingLogin{
		BaseURL:                 baseURL,
		DeviceCode:              s.DeviceCode,
		UserCode:                s.UserCode,
		VerificationURIComplete: s.VerificationURIComplete,
		Interval:                s.Interval,
		ExpiresAt:               time.Now().Add(time.Duration(s.ExpiresIn) * time.Second),
	}
	if l != nil {
		creds.Pending.LoopbackPort = l.Addr().(*net.TCPAddr).Port
	}
	if err := SaveCreds(creds); err != nil {
		if l != nil {
			l.Close()
		}
		return nil, false, nil, err
	}
	return creds.Pending, true, l, nil
}

// ServeLoopback accepts one GET /callback?grant=… on l, redirects the browser to
// the approve page's done view and delivers the grant on the returned channel.
func ServeLoopback(l net.Listener, approveURL string) <-chan string {
	grants := make(chan string, 1)
	done := "/cli/approve?done=1"
	if u, err := url.Parse(approveURL); err == nil {
		done = u.Scheme + "://" + u.Host + done
	}
	var accepted atomic.Bool
	go http.Serve(l, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		grant := r.URL.Query().Get("grant")
		if r.Method != http.MethodGet || r.URL.Path != "/callback" || grant == "" || !accepted.CompareAndSwap(false, true) {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, done, http.StatusFound)
		w.(http.Flusher).Flush()
		grants <- grant
	}))
	return grants
}

// WaitForGrant sleeps d, returning early with a grant from grants (nil never fires).
func WaitForGrant(grants <-chan string) func(time.Duration) string {
	return func(d time.Duration) string {
		select {
		case g := <-grants:
			return g
		case <-time.After(d):
			return ""
		}
	}
}

// PollOnce redeems the device code (with grant, if any) once. On approval the org key is stored,
// made active (unless p.Org names another org), the pending login cleared and
// the previously stored key of the same org revoked (best effort).
func PollOnce(p *PendingLogin, grant string) (*OrgKey, error) {
	tok, err := client.New("", p.BaseURL, "", "").PollCLIAuth(p.DeviceCode, grant)
	if err != nil {
		if code := errorCode(err); code == "expired_token" || code == "access_denied" || code == "grant_required" || code == "invalid_grant" {
			if creds, lerr := LoadCreds(); lerr == nil {
				creds.Pending = nil
				SaveCreds(creds)
			}
		}
		return nil, err
	}
	creds, err := LoadCreds()
	if err != nil {
		return nil, err
	}
	if old := creds.OrgKey(p.BaseURL, tok.OrgID); old != nil && old.KeyID != tok.KeyID {
		client.New(old.APIKey, p.BaseURL, "", "").RevokeOrgKey(old.KeyID)
	}
	key := OrgKey{BaseURL: p.BaseURL, OrgID: tok.OrgID, OrgName: tok.OrgName, KeyID: tok.KeyID, APIKey: tok.APIKey, FullPower: tok.FullPower, CreatedAt: time.Now().UTC()}
	creds.SetOrgKey(key)
	if p.Org == "" || p.Org == key.OrgID {
		creds.UseOrg(p.BaseURL, key.OrgID)
	}
	creds.Pending = nil
	return &key, SaveCreds(creds)
}

// PollLogin polls every interval (plus 5 s per slow_down) until approval, a
// terminal error, or budget runs out (ErrLoginPending). sleep returns a
// loopback grant when one arrives, redeemed on the next poll.
func PollLogin(p *PendingLogin, budget time.Duration, sleep func(time.Duration) string) (*OrgKey, error) {
	interval := time.Duration(max(p.Interval, 1)) * time.Second
	var waited time.Duration
	grant := ""
	for {
		key, err := PollOnce(p, grant)
		if err == nil {
			return key, nil
		}
		switch errorCode(err) {
		case "authorization_pending":
		case "slow_down":
			interval += 5 * time.Second
		case "expired_token", "invalid_grant":
			return nil, ErrLoginExpired
		case "grant_required":
			return nil, ErrGrantLost
		default:
			return nil, err
		}
		if waited+interval > budget {
			return nil, ErrLoginPending
		}
		grant = sleep(interval)
		waited += interval
	}
}

func errorCode(err error) string {
	var apiErr *client.APIError
	if errors.As(err, &apiErr) {
		return apiErr.Body.Error.Code
	}
	return ""
}

func OpenBrowser(url string) bool {
	if os.Getenv("AIRSTRINGS_NO_BROWSER") != "" || client.ValidateBaseURL(url) != nil {
		return false
	}
	var cmd *exec.Cmd
	switch {
	case os.Getenv("BROWSER") != "":
		cmd = exec.Command(os.Getenv("BROWSER"), url)
	case runtime.GOOS == "darwin":
		cmd = exec.Command("open", url)
	case runtime.GOOS == "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start() == nil
}
