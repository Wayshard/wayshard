//go:build linux

package harness

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Wayshard/wayshard/internal/testutil"
)

func requirePython(t *testing.T) string {
	t.Helper()
	p, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not installed")
	}
	return p
}

func writeExecutable(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

func findInstallation(t *testing.T, insts []Installation, exe string) Installation {
	t.Helper()
	for _, in := range insts {
		if in.Executable == exe {
			return in
		}
	}
	t.Fatalf("installation for %s not found in %+v", exe, insts)
	return Installation{}
}

// TestProbePolicyConfinesMaliciousVersionProbe runs a real discovery of an
// explicit malicious executable and proves the ProbePolicy denies host,
// project, Wayshard-data, secret and network access.
func TestProbePolicyConfinesMaliciousVersionProbe(t *testing.T) {
	py := requirePython(t)
	root := t.TempDir()
	probeDir := filepath.Join(root, "probe")
	if err := os.MkdirAll(probeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	hostCanary := filepath.Join(root, "host-secret.txt")
	_ = os.WriteFile(hostCanary, []byte("HOST-SECRET-XYZ"), 0o644)
	wsData := filepath.Join(root, "wayshard-runtime", "app.db")
	_ = os.MkdirAll(filepath.Dir(wsData), 0o755)
	_ = os.WriteFile(wsData, []byte("WS-DB-SECRET"), 0o644)
	projectFile := filepath.Join(root, "project", "source.go")
	_ = os.MkdirAll(filepath.Dir(projectFile), 0o755)
	_ = os.WriteFile(projectFile, []byte("PROJECT-SECRET"), 0o644)

	t.Setenv("AWS_ACCESS_KEY_ID", "AWS-CANARY")
	t.Setenv("GITHUB_TOKEN", "GH-CANARY")
	t.Setenv("SSH_AUTH_SOCK", "/tmp/agent.sock")
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "unix:path=/run/dbus")
	t.Setenv("DISPLAY", ":0")
	t.Setenv("DOCKER_HOST", "unix:///var/run/docker.sock")

	script := `#!/bin/sh
H=$(cat "` + hostCanary + `" 2>/dev/null)
W=$(cat "` + wsData + `" 2>/dev/null)
P=$(cat "` + projectFile + `" 2>/dev/null)
ENVV="${AWS_ACCESS_KEY_ID:-}${GITHUB_TOKEN:-}${SSH_AUTH_SOCK:-}${DISPLAY:-}"
NET=$(` + py + ` - <<'PY'
import socket
res=[]
def t(name, fam, typ):
    try:
        s=socket.socket(fam, typ); s.close(); res.append(name+"=OK")
    except OSError as e:
        res.append(name+"=ERR%d"%e.errno)
t("tcp", socket.AF_INET, socket.SOCK_STREAM)
t("udp", socket.AF_INET, socket.SOCK_DGRAM)
t("unixfs", socket.AF_UNIX, socket.SOCK_STREAM)
t("unixabs", socket.AF_UNIX, socket.SOCK_STREAM)
print(",".join(res))
PY
)
echo "v9.9.9|h=$H|w=$W|p=$P|env=$ENVV|net=$NET"
echo pwned > "` + probeDir + `/pwned" 2>/dev/null || true
`
	exe := filepath.Join(probeDir, "malicious-harness")
	writeExecutable(t, exe, script)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	insts, err := Discover(ctx, DiscoverOptions{
		PATH: probeDir, Home: probeDir, ExtraPaths: []string{exe},
		Probe: true, IncludeLoginPATH: false, WellKnownDirs: false,
		ProbeTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	inst := findInstallation(t, insts, exe)
	v := inst.Version
	t.Logf("malicious probe version line: %q", v)
	if !strings.HasPrefix(v, "v9.9.9") {
		t.Fatalf("malicious executable did not run under ProbePolicy: %q", v)
	}
	for _, secret := range []string{"HOST-SECRET-XYZ", "WS-DB-SECRET", "PROJECT-SECRET", "AWS-CANARY", "GH-CANARY", "/tmp/agent.sock", ":0"} {
		if strings.Contains(v, secret) {
			t.Fatalf("probe leaked %q: %q", secret, v)
		}
	}
	for _, net := range []string{"tcp=OK", "udp=OK", "unixfs=OK", "unixabs=OK"} {
		if strings.Contains(v, net) {
			t.Fatalf("probe network not denied (%s): %q", net, v)
		}
	}
	if _, err := os.Stat(filepath.Join(probeDir, "pwned")); err == nil {
		t.Fatal("probe wrote into its own (read-only) directory")
	}
	if b, _ := os.ReadFile(hostCanary); string(b) != "HOST-SECRET-XYZ" {
		t.Fatal("host canary modified by probe")
	}
}

// TestProbePolicyConfinesACPInitialize proves the ACP initialize probe runs
// under ProbePolicy and cannot read a host canary.
func TestProbePolicyConfinesACPInitialize(t *testing.T) {
	fake := testutil.BuildFakeACP(t)
	canary := filepath.Join(t.TempDir(), "host-secret.txt")
	_ = os.WriteFile(canary, []byte("INIT-CANARY"), 0o644)
	t.Setenv("WAYSHARD_FAKE_INIT_CANARY", canary)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	insts, err := Discover(ctx, DiscoverOptions{
		PATH: filepath.Dir(fake), Home: t.TempDir(), ExtraPaths: []string{fake},
		Probe: true, IncludeLoginPATH: false, WellKnownDirs: false,
		ProbeTimeout: 8 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	inst := findInstallation(t, insts, fake)
	if strings.Contains(inst.AgentInfo.Version, "INIT-CANARY") {
		t.Fatalf("ACP initialize read the host canary: %q", inst.AgentInfo.Version)
	}
	if inst.AgentInfo.Version != "confined" {
		t.Fatalf("initialize canary probe did not run confined: %q", inst.AgentInfo.Version)
	}
}

// TestProbeTimeoutKillsDescendants proves a hanging probe is killed with its
// descendants and leaves no orphan.
func TestProbeTimeoutKillsDescendants(t *testing.T) {
	if _, err := exec.LookPath("pgrep"); err != nil {
		t.Skip("pgrep not installed")
	}
	marker := "WS_PROBE_ORPHAN_" + strings.ReplaceAll(t.Name(), "/", "_")
	probeDir := t.TempDir()
	exe := filepath.Join(probeDir, "hanging-harness")
	writeExecutable(t, exe, "#!/bin/sh\n/bin/sh -c 'sleep 300 # "+marker+"' &\nsleep 300\n")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err := Discover(ctx, DiscoverOptions{
		PATH: probeDir, Home: probeDir, ExtraPaths: []string{exe},
		Probe: true, IncludeLoginPATH: false, WellKnownDirs: false,
		ProbeTimeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		out, _ := exec.Command("pgrep", "-f", marker).Output()
		if strings.TrimSpace(string(out)) == "" {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	out, _ := exec.Command("pgrep", "-af", marker).Output()
	t.Fatalf("probe descendant survived timeout: %s", out)
}

// TestProbeOutputBounded proves probe output capture is bounded.
func TestProbeOutputBounded(t *testing.T) {
	probeDir := t.TempDir()
	exe := filepath.Join(probeDir, "noisy-harness")
	writeExecutable(t, exe, "#!/bin/sh\nhead -c 4000000 /dev/zero | tr '\\0' 'A'\necho\necho v1.0.0\n")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	insts, err := Discover(ctx, DiscoverOptions{
		PATH: probeDir, Home: probeDir, ExtraPaths: []string{exe},
		Probe: true, IncludeLoginPATH: false, WellKnownDirs: false,
		ProbeTimeout: 8 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	inst := findInstallation(t, insts, exe)
	if len(inst.Version) > 256<<10+64 {
		t.Fatalf("probe output not bounded: %d bytes", len(inst.Version))
	}
}
