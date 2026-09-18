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
		runTUI(*base, *token)
		return
	}
	c := &client{base: strings.TrimRight(*base, "/"), token: *token, http: &http.Client{Timeout: 30 * time.Second}}
	switch args[0] {
	case "version":
		fmt.Printf("Wayshard CLI %s (%s)\n", version.Version, version.Commit)
	case "pair":
		if len(args) < 2 {
			fatal("usage: wayshard pair <code>")
		}
		name, _ := os.Hostname()
		body, err := c.post("/v1/pairing/complete", map[string]string{
			"code": args[1], "deviceName": name, "deviceKind": "cli",
		})
		if err != nil {
			fatal(err)
		}
		var res struct {
			Credential string `json:"credential"`
			ServerID   string `json:"serverId"`
		}
		_ = json.Unmarshal(body, &res)
		if res.Credential != "" {
			_ = saveToken(res.Credential)
			fmt.Println("paired", res.ServerID)
		} else {
			fmt.Println(string(body))
		}
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

  wayshard                         interactive client
  wayshard status|projects|devices|harnesses|approvals|notifications|storage|settings
  wayshard pair <code> | invite | open <path>
  wayshard send <conversation> <text>
  wayshard run|stages|artifacts|changes|cancel|retry|integrate <run-id>
  wayshard allow|deny <approval-id>
  wayshard knowledge <project-id>
  wayshard revoke <device-id>

Environment: WAYSHARD_SERVER, WAYSHARD_TOKEN
`)
	default:
		fatal("unknown command " + args[0])
	}
}

func runTUI(base, token string) {
	fmt.Printf("Wayshard %s  %s\n", version.Version, base)
	fmt.Println("Interactive TUI talks to the Wayshard Server over HTTP/JSON + WebSocket.")
	fmt.Println("Type a command: status | projects | help | quit")
	in := io.Reader(os.Stdin)
	buf := make([]byte, 0, 256)
	tmp := make([]byte, 1)
	for {
		fmt.Print("wayshard> ")
		line := ""
		for {
			n, err := in.Read(tmp)
			if n > 0 {
				if tmp[0] == '\n' {
					line = string(buf)
					buf = buf[:0]
					break
				}
				buf = append(buf, tmp[0])
			}
			if err != nil {
				return
			}
		}
		line = strings.TrimSpace(line)
		switch line {
		case "", "help":
			fmt.Println("status | projects | quit")
		case "quit", "exit":
			return
		case "status":
			os.Args = []string{"wayshard", "status"}
			main()
			return
		default:
			fmt.Println("use subcommands: wayshard", line)
		}
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

func tokenPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "wayshard", "device.token")
}

func saveToken(t string) error {
	p := tokenPath()
	_ = os.MkdirAll(filepath.Dir(p), 0o700)
	return os.WriteFile(p, []byte(t), 0o600)
}

func loadToken() (string, error) {
	b, err := os.ReadFile(tokenPath())
	return strings.TrimSpace(string(b)), err
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
