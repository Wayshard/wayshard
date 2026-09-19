package validation

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Wayshard/wayshard/internal/artifacts"
	"github.com/Wayshard/wayshard/internal/domain"
)

// These tests exercise real OS confinement and use POSIX shell scripts; the
// forensic sandbox failure was demonstrated on Linux. macOS/Windows runtime
// enforcement remains unverified (see CONFORMANCE.md).
func requireLinux(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("native sandbox enforcement is verified on Linux only")
	}
}

// TestValidationRunsInToolSandbox proves repository-controlled validation
// commands cannot read host files, write outside the workspace, or see ambient
// secrets, while ordinary commands still work.
func TestValidationRunsInToolSandbox(t *testing.T) {
	requireLinux(t)
	host := t.TempDir()
	hostCanary := filepath.Join(host, "host-canary.txt")
	if err := os.WriteFile(hostCanary, []byte("host-secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	t.Setenv("VALIDATION_CANARY", "canary-ambient-secret")

	ws := t.TempDir()
	script := "#!/bin/sh\n" +
		"fail=0\n" +
		"if cat " + hostCanary + " >/dev/null 2>&1; then echo read-host >> escapes.txt; fail=1; fi\n" +
		"if echo pwn > " + outside + "/escape.txt 2>/dev/null; then echo wrote-outside >> escapes.txt; fail=1; fi\n" +
		"echo \"SECRET=${VALIDATION_CANARY:-}\" > env.txt\n" +
		"exit $fail\n"
	if err := os.WriteFile(filepath.Join(ws, "check.sh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	r := &Runner{DataDir: t.TempDir()}
	art := r.Run(context.Background(), ws, []artifacts.ValidationCheck{
		{Name: "sandbox-probe", Command: "./check.sh", Required: true, Status: string(domain.CheckNotVerified)},
	}, nil)
	if len(art.Checks) != 1 {
		t.Fatalf("checks: %+v", art.Checks)
	}
	if art.Checks[0].Status != string(domain.CheckPass) {
		t.Fatalf("validation escaped the tool sandbox: %+v", art.Checks[0])
	}
	if _, err := os.Stat(filepath.Join(outside, "escape.txt")); err == nil {
		t.Fatal("validation wrote outside the workspace")
	}
	if b, err := os.ReadFile(filepath.Join(ws, "escapes.txt")); err == nil {
		t.Fatalf("validation escaped: %s", b)
	}
	if b, err := os.ReadFile(filepath.Join(ws, "env.txt")); err != nil {
		t.Fatal(err)
	} else if string(b) != "SECRET=\n" {
		t.Fatalf("ambient secret leaked into validation: %q", b)
	}
}

// TestValidationOrdinaryCommandStillWorks ensures benign commands pass.
func TestValidationOrdinaryCommandStillWorks(t *testing.T) {
	requireLinux(t)
	ws := t.TempDir()
	r := &Runner{DataDir: t.TempDir()}
	art := r.Run(context.Background(), ws, []artifacts.ValidationCheck{
		{Name: "true", Command: "true", Required: true, Status: string(domain.CheckNotVerified)},
	}, nil)
	if art.Checks[0].Status != string(domain.CheckPass) {
		t.Fatalf("benign validation failed: %+v", art.Checks[0])
	}
}

// TestValidationBaselineClassification distinguishes pre-existing failures from
// new regressions.
func TestValidationBaselineClassification(t *testing.T) {
	requireLinux(t)
	ws := t.TempDir()
	script := "#!/bin/sh\nexit 1\n"
	if err := os.WriteFile(filepath.Join(ws, "fail.sh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	r := &Runner{DataDir: t.TempDir()}
	check := artifacts.ValidationCheck{Name: "gate", Command: "./fail.sh", Required: true, Status: string(domain.CheckNotVerified)}

	pre := r.Run(context.Background(), ws, []artifacts.ValidationCheck{check}, map[string]string{"gate": string(domain.CheckFail)})
	if !pre.BaselineCompared {
		t.Fatal("baseline was not compared")
	}
	if len(pre.BlockingFailures) != 0 {
		t.Fatalf("pre-existing failure treated as regression: %+v", pre.BlockingFailures)
	}
	if pre.Checks[0].Summary != "pre-existing failure" {
		t.Fatalf("summary: %q", pre.Checks[0].Summary)
	}

	reg := r.Run(context.Background(), ws, []artifacts.ValidationCheck{check}, map[string]string{"gate": string(domain.CheckPass)})
	if len(reg.BlockingFailures) != 1 {
		t.Fatalf("new regression not flagged: %+v", reg.BlockingFailures)
	}
}
