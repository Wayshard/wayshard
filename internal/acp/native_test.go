package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestNativeHandshakeTimeout proves a harness that never answers initialize is
// bounded by Limits.HandshakeTimeout on the host OS rather than hanging.
func TestNativeHandshakeTimeout(t *testing.T) {
	ctx := context.Background()
	d, err := Launch(ctx, Spec{
		Command: fakeACP,
		Env:     withFakeEnv("success", "plan", "WAYSHARD_FAKE_HANG_INITIALIZE", "1"),
	}, DefaultClientConfig(), Hooks{}, Limits{HandshakeTimeout: 1 * time.Second, ShutdownWait: 500 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })

	start := time.Now()
	_, err = d.Handshake(ctx)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected handshake timeout")
	}
	if !errors.Is(err, ErrHandshakeTimeout) && !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want a handshake timeout", err)
	}
	if elapsed > 10*time.Second {
		t.Fatalf("handshake did not time out promptly: %s", elapsed)
	}
}

type launchRecord struct {
	Cwd  string            `json:"cwd"`
	Args []string          `json:"args"`
	Env  map[string]string `json:"env"`
}

// TestNativeProcessEnvironmentAndQuoting proves the launched harness receives the
// exact working directory, argv and inherited environment Wayshard passed,
// including values containing spaces, quotes and separators. This is the native
// process-launch/quoting/environment contract on every platform.
func TestNativeProcessEnvironmentAndQuoting(t *testing.T) {
	record := filepath.Join(t.TempDir(), "launch.jsonl")
	cwd := t.TempDir()
	const canaryName = "WAYSHARD_NATIVE_QUOTE_CANARY"
	const canaryValue = `a "quoted" value with spaces, \\ backslash and = signs`
	const argValue = `--echo=weird "quoted" arg with spaces`

	env := append(os.Environ(),
		canaryName+"="+canaryValue,
		"WAYSHARD_FAKE_RECORD="+record,
		"WAYSHARD_FAKE_RECORD_KEYS="+canaryName,
	)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	d, err := Launch(ctx, Spec{Command: fakeACP, Args: []string{argValue}, Dir: cwd, Env: env},
		DefaultClientConfig(), Hooks{}, Limits{HandshakeTimeout: 5 * time.Second, ShutdownWait: 500 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if _, err := d.Handshake(ctx); err != nil {
		t.Fatal(err)
	}

	rec := readLaunchRecord(t, record)
	if filepath.Clean(rec.Cwd) != filepath.Clean(cwd) {
		t.Fatalf("harness cwd = %q, want %q", rec.Cwd, cwd)
	}
	if len(rec.Args) != 1 || rec.Args[0] != argValue {
		t.Fatalf("harness args = %q, want [%q]", rec.Args, argValue)
	}
	if got := rec.Env[canaryName]; got != canaryValue {
		t.Fatalf("harness env %s = %q, want %q", canaryName, got, canaryValue)
	}
}

func readLaunchRecord(t *testing.T, path string) launchRecord {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		f, err := os.Open(path)
		if err == nil {
			sc := bufio.NewScanner(f)
			var last launchRecord
			found := false
			for sc.Scan() {
				if len(sc.Bytes()) == 0 {
					continue
				}
				if err := json.Unmarshal(sc.Bytes(), &last); err == nil {
					found = true
				}
			}
			_ = f.Close()
			if found {
				return last
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("no launch record written to %s", path)
	return launchRecord{}
}
