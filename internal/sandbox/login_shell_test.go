//go:build !windows

package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestSanitizeLoginPATH(t *testing.T) {
	sep := string(os.PathListSeparator)
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{name: "plain", in: "/usr/bin" + sep + "/bin", want: "/usr/bin" + sep + "/bin"},
		{name: "dedupe", in: "/usr/bin" + sep + "/usr/bin", want: "/usr/bin"},
		{name: "drop relative and cwd", in: "bin" + sep + "." + sep + ".." + sep + "/usr/bin", want: "/usr/bin"},
		{name: "drop empty", in: sep + "/usr/bin" + sep, want: "/usr/bin"},
		{name: "clean traversal", in: "/usr/../usr/bin", want: "/usr/bin"},
		{name: "tilde not expanded dropped", in: "~/bin" + sep + "/bin", want: "/bin"},
		{name: "newline rejected", in: "/usr/bin\n/bin", wantErr: true},
		{name: "nul rejected", in: "/usr/bin\x00", wantErr: true},
		{name: "no absolute rejected", in: "bin:../x:."},
		{name: "empty rejected", in: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SanitizeLoginPATH(tt.in)
			if tt.wantErr || tt.want == "" {
				if err == nil {
					t.Fatalf("SanitizeLoginPATH(%q) = %q, want error", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("SanitizeLoginPATH(%q): %v", tt.in, err)
			}
			if got != tt.want {
				t.Fatalf("SanitizeLoginPATH(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestLoginShellStartupFilesSelection(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	etc := filepath.Join(root, "etc")
	if err := os.MkdirAll(filepath.Join(etc, "profile.d"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{
		filepath.Join(etc, "profile"),
		filepath.Join(etc, "profile.d", "10-lang.sh"),
		filepath.Join(etc, "bash.bashrc"),
		filepath.Join(home, ".bash_profile"),
		filepath.Join(home, ".profile"),
		filepath.Join(home, ".zprofile"),
		filepath.Join(home, ".zshrc"),
	} {
		if err := os.WriteFile(p, []byte("# startup\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	bash := loginShellStartupFiles("/bin/bash", home, etc)
	assertContains(t, bash, filepath.Join(etc, "profile"))
	assertContains(t, bash, filepath.Join(etc, "profile.d", "10-lang.sh"))
	assertContains(t, bash, filepath.Join(etc, "bash.bashrc"))
	assertContains(t, bash, filepath.Join(home, ".bash_profile"))
	assertContains(t, bash, filepath.Join(home, ".profile"))
	assertNotContains(t, bash, filepath.Join(home, ".zprofile"))

	zsh := loginShellStartupFiles("/bin/zsh", home, etc)
	assertContains(t, zsh, filepath.Join(home, ".zprofile"))
	assertContains(t, zsh, filepath.Join(home, ".zshrc"))
	assertNotContains(t, zsh, filepath.Join(home, ".bash_profile"))

	sh := loginShellStartupFiles("/bin/sh", home, etc)
	assertContains(t, sh, filepath.Join(etc, "profile"))
	assertContains(t, sh, filepath.Join(home, ".profile"))
	assertNotContains(t, sh, filepath.Join(home, ".bash_profile"))
	assertNotContains(t, sh, filepath.Join(home, ".zshrc"))
}

// TestLoginShellPolicyDeniesHomeSecrets proves the login-shell PATH probe can
// source the startup files it needs but cannot read unrelated private files, so
// a malicious profile cannot exfiltrate credentials through PATH.
func TestLoginShellPolicyDeniesHomeSecrets(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux-only")
	}
	if !AsConstrainer(DefaultBackend()).Report().Available {
		t.Skip("native sandbox unavailable")
	}
	shell := "/bin/sh"
	if _, err := os.Stat(shell); err != nil {
		t.Skip("no /bin/sh")
	}
	root := t.TempDir()
	home := filepath.Join(root, "home")
	etc := filepath.Join(root, "etc")
	shome := filepath.Join(root, "synthetic-home")
	stmp := filepath.Join(root, "synthetic-tmp")
	for _, d := range []string{home, filepath.Join(home, ".ssh"), filepath.Join(etc, "profile.d"), shome, stmp} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// A required HOME startup file that legitimately sets PATH. The real shell
	// always reads the absolute /etc/profile, so the "required startup" proof
	// uses the home profile that LoginShellPolicy grants.
	marker := "/opt/wayshard-login-marker"
	// An unrelated canary in the same (fake) /etc tree that must stay
	// unreadable.
	if err := os.WriteFile(filepath.Join(etc, "canary-secret"), []byte("ETC-CANARY-XYZ"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Unrelated private HOME files.
	secrets := map[string]string{
		".aws-credentials": "AWS-CANARY-111",
		".ssh/id_test":     "SSH-CANARY-222",
		".git-credentials": "GIT-CANARY-333",
		".provider-secret": "PROVIDER-CANARY-444",
	}
	for name, val := range secrets {
		if err := os.WriteFile(filepath.Join(home, name), []byte(val), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// Malicious startup profile: tries to read every secret plus a real /etc
	// file (/etc/passwd) that is deliberately not in the read-only system root
	// set, then encodes anything it reads into PATH and a leak file in the
	// writable synthetic temp. The required marker proves the startup file ran.
	profile := `#!/bin/sh
LEAK=""
for f in .aws-credentials .ssh/id_test .git-credentials .provider-secret; do
  v=$(cat "$HOME/$f" 2>/dev/null)
  [ -n "$v" ] && LEAK="$LEAK:$v"
done
for f in "` + filepath.Join(etc, "canary-secret") + `" /etc/passwd; do
  v=$(cat "$f" 2>/dev/null)
  [ -n "$v" ] && LEAK="$LEAK:$v"
done
printf '%s' "$LEAK" > "$TMPDIR/leak" 2>/dev/null
PATH="$PATH:` + marker + `:$LEAK"
`
	if err := os.WriteFile(filepath.Join(home, ".profile"), []byte(profile), 0o644); err != nil {
		t.Fatal(err)
	}

	pol := LoginShellPolicy(shell, home, etc, shome, stmp)
	env := HarnessEnv(home, stmp, nil)
	out, err := RunConstrainedOutput(context.Background(), pol, 5*time.Second, 64<<10, shell, []string{"-lc", `printf '%s' "$PATH"`}, env)
	if err != nil {
		t.Fatalf("login-shell probe: %v\n%s", err, out)
	}
	path := string(out)
	// The required startup file must still have been sourced.
	if !strings.Contains(path, marker) {
		t.Fatalf("required startup file was not sourced; PATH=%q", path)
	}
	for _, secret := range []string{"CANARY", "root:"} {
		if strings.Contains(path, secret) {
			t.Fatalf("login-shell PATH leaked %q: %q", secret, path)
		}
	}
	// The direct-read leak file must contain no secret: no private file was read.
	if b, rerr := os.ReadFile(filepath.Join(stmp, "leak")); rerr == nil {
		if strings.Contains(string(b), "CANARY") || strings.Contains(string(b), "root:") {
			t.Fatalf("login profile read a private file: %q", string(b))
		}
	}
}

func assertContains(t *testing.T, list []string, want string) {
	t.Helper()
	for _, v := range list {
		if v == want {
			return
		}
	}
	t.Fatalf("%q not found in %v", want, list)
}

func assertNotContains(t *testing.T, list []string, bad string) {
	t.Helper()
	for _, v := range list {
		if v == bad {
			t.Fatalf("unexpected %q in %v", bad, list)
		}
	}
}
