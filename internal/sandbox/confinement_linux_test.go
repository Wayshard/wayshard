//go:build linux

package sandbox

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func runConstrained(t *testing.T, c Constrainer, cmd *exec.Cmd, p Policy) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := c.Constrain(cmd, p); err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", err
	}
	clean, err := c.Attach(cmd, p)
	if err != nil {
		_ = cmd.Process.Kill()
		return "", err
	}
	defer clean()
	err = cmd.Wait()
	return buf.String(), err
}

// TestLandlockFilesystemConfinement proves real OS enforcement, not just a
// compiled policy: workspace writes work, while host files, /tmp and unrelated
// paths are denied for the process and its descendants.
func TestLandlockFilesystemConfinement(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux-only")
	}
	c := AsConstrainer(DefaultBackend())
	if !c.Report().Available {
		t.Skip("landlock unavailable")
	}
	ws := t.TempDir()
	tmp := t.TempDir()
	host := t.TempDir()
	hostCanary := filepath.Join(host, "canary.txt")
	if err := os.WriteFile(hostCanary, []byte("host-secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	pol := HarnessPolicy(ws, tmp)

	script := `
set -e
echo ok > "$WS/allowed.txt"
cat "$WS/allowed.txt" >/dev/null
if cat "$HOSTCANARY" >/dev/null 2>&1; then echo READ_HOST_OK; exit 11; fi
if echo pwn > /tmp/wayshard-host-escape 2>/dev/null; then echo WROTE_TMP; exit 12; fi
if echo pwn > "$HOST/escape.txt" 2>/dev/null; then echo WROTE_HOST; exit 13; fi
if sh -c 'cat "$HOSTCANARY" >/dev/null 2>&1'; then echo CHILD_READ_HOST_OK; exit 14; fi
if sh -c 'echo pwn > /tmp/wayshard-child-escape 2>/dev/null'; then echo CHILD_WROTE_TMP; exit 15; fi
exit 0
`
	cmd := exec.Command("sh", "-c", script)
	cmd.Env = append(os.Environ(), "WS="+ws, "HOST="+host, "HOSTCANARY="+hostCanary)
	out, err := runConstrained(t, c, cmd, pol)
	if err != nil {
		t.Fatalf("confinement test failed: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, "") {
		t.Fatal("unexpected")
	}
	if _, err := os.Stat(filepath.Join(ws, "allowed.txt")); err != nil {
		t.Fatal("workspace write did not succeed")
	}
	if _, err := os.Stat("/tmp/wayshard-host-escape"); err == nil {
		t.Fatal("sandbox wrote to host /tmp")
	}
	if _, err := os.Stat(filepath.Join(host, "escape.txt")); err == nil {
		t.Fatal("sandbox escaped to unrelated host path")
	}
}

// TestLandlockReadOnlyView denies writes to a read-only stage view.
func TestLandlockReadOnlyView(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux-only")
	}
	c := AsConstrainer(DefaultBackend())
	if !c.Report().Available {
		t.Skip("landlock unavailable")
	}
	view := t.TempDir()
	if err := os.WriteFile(filepath.Join(view, "readable.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	pol := ReadOnlyViewPolicy(view, tmp)
	cmd := exec.Command("sh", "-c", `cat "$VIEW/readable.txt" >/dev/null; if echo x > "$VIEW/write.txt" 2>/dev/null; then exit 21; fi; exit 0`)
	cmd.Env = append(os.Environ(), "VIEW="+view)
	out, err := runConstrained(t, c, cmd, pol)
	if err != nil {
		t.Fatalf("read-only view: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(view, "write.txt")); err == nil {
		t.Fatal("read-only view allowed a write")
	}
}

// TestEnvAllowlistDropsHostSecrets proves unrelated host secrets are absent.
func TestEnvAllowlistDropsHostSecrets(t *testing.T) {
	t.Setenv("TYPESAFE_API_KEY", "canary-jev")
	t.Setenv("WAYSHARD_VAULT_KEY", "canary-vault")
	t.Setenv("SOME_UNRELATED_SECRET", "canary-unrelated")
	t.Setenv("OPENROUTER_API_KEY", "canary-provider")
	env := HarnessEnv("/tmp/home", "/tmp/tmp", map[string]string{"EXPLICIT": "yes"})
	joined := strings.Join(env, "\n")
	for _, bad := range []string{"canary-jev", "canary-vault", "canary-unrelated", "canary-provider"} {
		if strings.Contains(joined, bad) {
			t.Fatalf("confined env leaked %q:\n%s", bad, joined)
		}
	}
	if !strings.Contains(joined, "EXPLICIT=yes") {
		t.Fatal("explicit env entry missing")
	}
	toolEnv := ToolEnv("/tmp/home", "/tmp/tmp", nil)
	if strings.Contains(strings.Join(toolEnv, "\n"), "canary-provider") {
		t.Fatal("tool env leaked provider credential")
	}
}
