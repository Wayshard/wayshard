package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Wayshard/wayshard/internal/version"
)

func main() {
	fs := flag.NewFlagSet("wayshard", flag.ExitOnError)
	base := fs.String("server", envOr("WAYSHARD_SERVER", "http://127.0.0.1:7420"), "Wayshard server URL")
	token := fs.String("token", os.Getenv("WAYSHARD_TOKEN"), "device credential")
	_ = fs.Parse(os.Args[1:])
	args := fs.Args()
	if len(args) == 0 {
		launchTUI(*base, *token)
		return
	}
	c := &client{base: strings.TrimRight(*base, "/"), token: *token, http: &http.Client{Timeout: 30 * time.Second}}
	switch args[0] {
	case "version":
		fmt.Printf("Wayshard CLI %s (%s)\n", version.Version, version.Commit)
	case "pair":
		pf := flag.NewFlagSet("pair", flag.ExitOnError)
		sid := pf.String("server-id", "", "expected server id from the pairing invitation")
		fp := pf.String("fingerprint", "", "expected server fingerprint from the pairing invitation")
		inv := pf.String("invitation", "", "pairing invitation text or JSON")
		_ = pf.Parse(args[1:])
		rest := pf.Args()
		expectedID, expectedFP, inviteCode, inviteURL := *sid, *fp, "", ""
		if *inv != "" {
			parsed, perr := parseInvitation(*inv)
			if perr != nil {
				fatal(perr)
			}
			if parsed.ServerID != "" {
				expectedID = parsed.ServerID
			}
			if parsed.Fingerprint != "" {
				expectedFP = parsed.Fingerprint
			}
			inviteCode = parsed.Code
			if parsed.AdvertisedURL != "" {
				inviteURL = parsed.AdvertisedURL
			}
		}
		if len(rest) > 0 {
			inviteCode = rest[0]
		}
		if inviteURL != "" {
			c.base = strings.TrimRight(inviteURL, "/")
		}
		if inviteCode == "" {
			fatal("usage: wayshard pair --server-id <id> --fingerprint <fp> <code>  (or --invitation <text/json>)")
		}
		if expectedID == "" || expectedFP == "" {
			fatal("verified pairing requires the expected server id and fingerprint from the trusted invitation (pass --invitation, or --server-id and --fingerprint)")
		}
		if err := verifyServerIdentity(c, expectedID, expectedFP); err != nil {
			fatal(err)
		}
		name, _ := os.Hostname()
		body, err := c.post("/v1/pairing/complete", map[string]string{
			"code": inviteCode, "deviceName": name, "deviceKind": "cli",
			"expectedServerId": expectedID, "expectedFingerprint": expectedFP,
		})
		if err != nil {
			fatal(err)
		}
		var res struct {
			Credential  string `json:"credential"`
			ServerID    string `json:"serverId"`
			Fingerprint string `json:"fingerprint"`
		}
		_ = json.Unmarshal(body, &res)
		if res.Credential == "" {
			fatal("server did not return a credential: " + string(body))
		}
		if res.ServerID != "" && res.ServerID != expectedID {
			fatal("paired server id does not match the invited identity; credential not saved")
		}
		if res.Fingerprint != "" && res.Fingerprint != expectedFP {
			fatal("paired server fingerprint does not match the invited identity; credential not saved")
		}
		if err := saveToken(res.Credential); err != nil {
			fatal(err)
		}
		fmt.Println("paired", res.ServerID)
	case "projects":
		b, err := c.get("/v1/projects")
		if err != nil {
			fatal(err)
		}
		fmt.Println(string(b))
	case "open":
		if len(args) < 2 {
			fatal("usage: wayshard open <path>")
		}
		p, _ := filepath.Abs(args[1])
		b, err := c.post("/v1/projects/open", map[string]string{"path": p})
		if err != nil {
			fatal(err)
		}
		fmt.Println(string(b))
	case "invite":
		b, err := c.post("/v1/pairing/invitations", map[string]string{"advertisedUrl": *base})
		if err != nil {
			fatal(err)
		}
		fmt.Println(string(b))
	case "status":
		b, err := c.get("/v1/meta")
		if err != nil {
			fatal(err)
		}
		fmt.Println(string(b))
	case "devices":
		printGet(c, "/v1/devices")
	case "revoke":
		need(args, 2, "wayshard revoke <device-id>")
		printPost(c, "/v1/devices/"+args[1]+"/revoke", map[string]any{})
	case "sessions":
		need(args, 2, "wayshard sessions <project-id>")
		printGet(c, "/v1/projects/"+args[1]+"/conversations")
	case "send":
		need(args, 3, "wayshard send <conversation-id> <text>")
		printPost(c, "/v1/conversations/"+args[1]+"/messages", map[string]any{"text": strings.Join(args[2:], " ")})
	case "run":
		need(args, 2, "wayshard run <run-id>")
		printGet(c, "/v1/runs/"+args[1])
	case "stages":
		need(args, 2, "wayshard stages <run-id>")
		printGet(c, "/v1/runs/"+args[1]+"/stages")
	case "artifacts":
		need(args, 2, "wayshard artifacts <run-id>")
		printGet(c, "/v1/runs/"+args[1]+"/artifacts")
	case "changes":
		need(args, 2, "wayshard changes <run-id>")
		printGet(c, "/v1/runs/"+args[1]+"/changes")
	case "cancel":
		need(args, 2, "wayshard cancel <run-id>")
		printPost(c, "/v1/runs/"+args[1]+"/cancel", map[string]any{})
	case "retry":
		need(args, 2, "wayshard retry <run-id>")
		printPost(c, "/v1/runs/"+args[1]+"/retry", map[string]any{})
	case "integrate":
		need(args, 2, "wayshard integrate <run-id>")
		printPost(c, "/v1/runs/"+args[1]+"/integrate", map[string]any{})
	case "approvals":
		printGet(c, "/v1/approvals")
	case "allow":
		need(args, 2, "wayshard allow <approval-id>")
		printPost(c, "/v1/approvals/"+args[1]+"/resolve", map[string]any{"status": "allowed"})
	case "deny":
		need(args, 2, "wayshard deny <approval-id>")
		printPost(c, "/v1/approvals/"+args[1]+"/resolve", map[string]any{"status": "denied"})
	case "notifications":
		printGet(c, "/v1/notifications")
	case "harnesses":
		printGet(c, "/v1/harnesses")
	case "storage":
		printGet(c, "/v1/storage")
	case "knowledge":
		need(args, 2, "wayshard knowledge <project-id>")
		printGet(c, "/v1/projects/"+args[1]+"/knowledge")
	case "settings":
		printGet(c, "/v1/settings")
	case "help", "-h", "--help":
		fmt.Print(`wayshard — Wayshard CLI/TUI

  wayshard                         interactive client (launches the packaged Wayshard TUI)
  wayshard status|projects|devices|harnesses|approvals|notifications|storage|settings
  wayshard pair --invitation '<pairing card or json>'   verified identity pairing
  wayshard pair --server-id <id> --fingerprint <fp> <code>
  wayshard invite | open <path>
  wayshard send <conversation> <text>
  wayshard run|stages|artifacts|changes|cancel|retry|integrate <run-id>
  wayshard allow|deny <approval-id>
  wayshard knowledge <project-id>
  wayshard revoke <device-id>

Pairing refuses a bare code: verified pairing requires the expected server id and
fingerprint from the trusted invitation (--invitation, or --server-id/--fingerprint).
The device credential is stored in platform-secure storage (macOS Keychain,
Windows Credential Manager, Linux Secret Service); when that is unavailable, or
when WAYSHARD_HEADLESS=1 is set, it falls back to a 0600 file under the user
config directory. WAYSHARD_TOKEN supplies the credential explicitly and takes
precedence over stored material.
Environment: WAYSHARD_SERVER, WAYSHARD_TOKEN, WAYSHARD_HEADLESS
`)
	default:
		fatal("unknown command " + args[0])
	}
}

type client struct {
	base, token string
	http        *http.Client
}

func (c *client) do(method, path string, body any) ([]byte, error) {
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(context.Background(), method, c.base+path, rdr)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	} else if t, err := loadToken(); err == nil {
		req.Header.Set("Authorization", "Bearer "+t)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode >= 400 {
		return b, fmt.Errorf("%s: %s", resp.Status, b)
	}
	return b, nil
}

func (c *client) get(path string) ([]byte, error) { return c.do(http.MethodGet, path, nil) }
func (c *client) post(path string, body any) ([]byte, error) {
	return c.do(http.MethodPost, path, body)
}

func need(args []string, n int, usage string) {
	if len(args) < n {
		fatal(usage)
	}
}

func printGet(c *client, path string) {
	b, err := c.get(path)
	if err != nil {
		fatal(err)
	}
	fmt.Println(string(b))
}

func printPost(c *client, path string, body any) {
	b, err := c.post(path, body)
	if err != nil {
		fatal(err)
	}
	fmt.Println(string(b))
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func fatal(v any) {
	fmt.Fprintln(os.Stderr, v)
	os.Exit(1)
}
