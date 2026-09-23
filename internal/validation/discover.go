package validation

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Wayshard/wayshard/internal/artifacts"
	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/process"
	"github.com/Wayshard/wayshard/internal/sandbox"
	"github.com/Wayshard/wayshard/internal/storage"
)

// Discover inspects project files passively. It never executes project code.
func Discover(root string) []artifacts.ValidationCheck {
	var checks []artifacts.ValidationCheck
	add := func(name, kind, cmd string, required bool) {
		checks = append(checks, artifacts.ValidationCheck{Name: name, Kind: kind, Command: cmd, Required: required, Status: string(domain.CheckNotVerified)})
	}
	if exists(root, "go.mod") {
		add("go-test", "test", "go test ./...", true)
		add("go-vet", "lint", "go vet ./...", false)
	}
	if exists(root, "package.json") {
		b, _ := os.ReadFile(filepath.Join(root, "package.json"))
		var pkg map[string]any
		_ = json.Unmarshal(b, &pkg)
		scripts, _ := pkg["scripts"].(map[string]any)
		if scripts != nil {
			if _, ok := scripts["test"]; ok {
				add("npm-test", "test", "npm test", true)
			}
			if _, ok := scripts["lint"]; ok {
				add("npm-lint", "lint", "npm run lint", false)
			}
			if _, ok := scripts["typecheck"]; ok {
				add("typecheck", "typecheck", "npm run typecheck", false)
			}
		}
	}
	if exists(root, "Cargo.toml") {
		add("cargo-test", "test", "cargo test", true)
	}
	if exists(root, "pyproject.toml") || exists(root, "pytest.ini") {
		add("pytest", "test", "pytest", false)
	}
	if exists(root, "Makefile") {
		b, _ := os.ReadFile(filepath.Join(root, "Makefile"))
		if bytes.Contains(b, []byte("\ntest:")) || bytes.Contains(b, []byte("\ntest :")) {
			add("make-test", "test", "make test", false)
		}
	}
	return checks
}

func exists(root, name string) bool {
	_, err := os.Stat(filepath.Join(root, name))
	return err == nil
}

type Runner struct {
	Store *storage.Store
	// DataDir roots synthetic HOME/TMP for confined validation. Optional.
	DataDir string
	// Network is the validation command network policy. Empty means NetworkNone:
	// project/tool commands are denied network by default.
	Network sandbox.NetworkMode
}

func (r *Runner) Run(ctx context.Context, workspace string, plan []artifacts.ValidationCheck, baseline map[string]string) artifacts.ValidationArtifact {
	art := artifacts.ValidationArtifact{Kind: "validation"}
	for _, c := range plan {
		c = r.execCheck(ctx, workspace, c)
		if base, ok := baseline[c.Name]; ok {
			c.Baseline = base
			art.BaselineCompared = true
			if base == string(domain.CheckFail) && c.Status == string(domain.CheckFail) {
				c.Summary = "pre-existing failure"
			} else if base != string(domain.CheckFail) && c.Status == string(domain.CheckFail) {
				c.Summary = "new regression"
				art.BlockingFailures = append(art.BlockingFailures, c.Name)
			}
		} else if c.Required && c.Status == string(domain.CheckFail) {
			art.BlockingFailures = append(art.BlockingFailures, c.Name)
		}
		if c.Status == string(domain.CheckWarn) {
			art.Warnings = append(art.Warnings, c.Name)
		}
		if c.Status == string(domain.CheckNotVerified) || c.Status == string(domain.CheckSkipped) {
			art.Unverified = append(art.Unverified, c.Name)
		}
		art.Checks = append(art.Checks, c)
	}
	return art
}

func (r *Runner) execCheck(ctx context.Context, dir string, c artifacts.ValidationCheck) artifacts.ValidationCheck {
	if strings.TrimSpace(c.Command) == "" {
		c.Status = string(domain.CheckSkipped)
		return c
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	start := time.Now()
	parts := strings.Fields(c.Command)
	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
	cmd.Dir = dir

	// Confine validation commands through the Tool Sandbox.
	home := r.syntheticHome(dir)
	net := r.Network
	if net == "" {
		// Tool/validation network is denied by default.
		net = sandbox.NetNone
	}
	pol := sandbox.ToolPolicy(dir, home, net)
	if exe, err := exec.LookPath(parts[0]); err == nil {
		pol.ReadOnlyRoots = append(pol.ReadOnlyRoots, filepath.Dir(exe))
		if parent := filepath.Dir(filepath.Dir(exe)); parent != "/" {
			pol.ReadOnlyRoots = append(pol.ReadOnlyRoots, parent)
		}
	}
	if root := os.Getenv("GOROOT"); root != "" {
		pol.ReadOnlyRoots = append(pol.ReadOnlyRoots, root)
	}
	if hm, err := os.UserHomeDir(); err == nil {
		pol.ReadOnlyRoots = append(pol.ReadOnlyRoots, filepath.Join(hm, ".local", "go"), filepath.Join(hm, "go"))
	}
	// A per-command ownership token lets a descendant that escaped the process
	// group (setsid/setpgid) be reconciled authoritatively after the command,
	// including after a timeout/cancellation.
	token, _ := process.NewToken()
	scoped := map[string]string{}
	if token != "" {
		scoped[process.TokenEnv] = token
	}
	cmd.Env = sandbox.ToolEnv(home, home, scoped)

	b := sandbox.DefaultBackend()
	con := sandbox.AsConstrainer(b)
	sandboxFailed := false
	if _, err := con.Compile(pol); err != nil {
		sandboxFailed = true
	} else if err := con.Constrain(cmd, pol); err != nil {
		sandboxFailed = true
	}
	if sandboxFailed {
		c.Status = string(domain.CheckBlocked)
		c.Dir = dir
		c.Summary = "tool sandbox could not be established"
		return c
	}
	// Kill the whole process group on cancellation/timeout, not just the
	// direct child, so validation cannot leave orphaned descendants.
	cmd.Cancel = func() error { return con.KillTree(cmd) }
	cmd.WaitDelay = 5 * time.Second

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Start()
	if err == nil {
		// Release the supervisor death pipe: Attach closes the parent's read end
		// and returns the cleanup that closes the write end. Always run the
		// returned cleanup so a required validation command does not retain
		// death-pipe FDs or linuxDeathPipes entries for the life of the server.
		cleanup, aerr := con.Attach(cmd, pol)
		if cleanup != nil {
			defer cleanup()
		}
		if aerr != nil {
			_ = con.KillTree(cmd)
			_ = cmd.Wait()
			err = aerr
		} else {
			err = cmd.Wait()
		}
	}
	// Reconcile any descendant that escaped the process group (setsid) by its
	// token, so a timeout/cancellation cannot leave an owned process behind.
	if token != "" && cmd.Process != nil {
		_, _, _, _ = process.ReconcileEnvToken(process.TokenEnv, process.HashToken(token), cmd.Process.Pid, 3*time.Second)
	}
	c.DurationMS = time.Since(start).Milliseconds()
	c.Dir = dir
	c.Fingerprint = "tool_sandbox:" + string(net)
	if r.Store != nil {
		h, _ := r.Store.Objects.Put(buf.Bytes())
		c.LogHash = h
	}
	if err != nil {
		if ctx.Err() != nil {
			c.Status = string(domain.CheckBlocked)
			c.Summary = ctx.Err().Error()
			return c
		}
		c.Status = string(domain.CheckFail)
		if cmd.ProcessState != nil {
			c.ExitCode = cmd.ProcessState.ExitCode()
		}
		c.Summary = err.Error()
		return c
	}
	c.Status = string(domain.CheckPass)
	c.ExitCode = 0
	c.Summary = "ok"
	return c
}

func (r *Runner) syntheticHome(workspace string) string {
	// Keep the tool HOME beside the disposable validation workspace so it is
	// discarded with it, rather than accumulating in a shared location.
	base := filepath.Dir(workspace)
	if base == "" {
		base = r.DataDir
	}
	if base == "" {
		base = os.TempDir()
	}
	h := filepath.Join(base, "toolhome")
	_ = os.MkdirAll(h, 0o700)
	return h
}
