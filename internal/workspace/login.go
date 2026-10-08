package workspace

import (
	"errors"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/symbionix-sl/airstrings-cli/internal/client"
)

var (
	ErrLoginPending = errors.New("login not approved yet")
	ErrLoginExpired = errors.New("login code expired")
)

const PollBudget = 90 * time.Second

func PendingNextStep(url string) string {
	return "Approve in the browser at " + url + ", then run the same command again"
}

// StartLogin returns the unexpired pending login for baseURL, or starts a new
// one; fresh reports a new start. The store is written before the API call so
// an unwritable store fails before any server state exists.
func StartLogin(baseURL, clientName string) (p *PendingLogin, fresh bool, err error) {
	creds, err := LoadCreds()
	if err != nil {
		return nil, false, err
	}
	if p := creds.PendingFor(baseURL); p != nil {
		return p, false, nil
	}
	if err := SaveCreds(creds); err != nil {
		return nil, false, err
	}
	s, err := client.New("", baseURL, "", "").StartCLIAuth(clientName)
	if err != nil {
		return nil, false, err
	}
	creds.Pending = &PendingLogin{
		BaseURL:                 baseURL,
		DeviceCode:              s.DeviceCode,
		UserCode:                s.UserCode,
		VerificationURIComplete: s.VerificationURIComplete,
		Interval:                s.Interval,
		ExpiresAt:               time.Now().Add(time.Duration(s.ExpiresIn) * time.Second),
	}
	return creds.Pending, true, SaveCreds(creds)
}

// PollOnce redeems the device code once. On approval the org key is stored,
// made active, the pending login cleared and the previously stored key of the
// same org revoked (best effort).
func PollOnce(p *PendingLogin) (*OrgKey, error) {
	tok, err := client.New("", p.BaseURL, "", "").PollCLIAuth(p.DeviceCode)
	if err != nil {
		if code := errorCode(err); code == "expired_token" || code == "access_denied" {
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
	creds.UseOrg(p.BaseURL, key.OrgID)
	creds.Pending = nil
	return &key, SaveCreds(creds)
}

// PollLogin polls every interval (plus 5 s per slow_down) until approval, a
// terminal error, or budget runs out (ErrLoginPending).
func PollLogin(p *PendingLogin, budget time.Duration, sleep func(time.Duration)) (*OrgKey, error) {
	interval := time.Duration(max(p.Interval, 1)) * time.Second
	var waited time.Duration
	for {
		key, err := PollOnce(p)
		if err == nil {
			return key, nil
		}
		switch errorCode(err) {
		case "authorization_pending":
		case "slow_down":
			interval += 5 * time.Second
		case "expired_token":
			return nil, ErrLoginExpired
		default:
			return nil, err
		}
		if waited+interval > budget {
			return nil, ErrLoginPending
		}
		sleep(interval)
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
