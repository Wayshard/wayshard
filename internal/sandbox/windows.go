//go:build windows

package sandbox

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// winJobs tracks the Job Object assigned to a launch so KillTree can terminate
// the whole job (not just the root process). It is keyed by *exec.Cmd because
// Constrain runs before Start and Attach runs after Start.
var winJobs sync.Map // map[*exec.Cmd]windows.Handle

// Windows Job Objects are used for process/resource management only. They do NOT
// provide filesystem confinement, network denial, or race-free process-tree
// containment (a process can spawn a child in the window between CreateProcess
// and AssignProcessToJobObject). AppContainer is not implemented. A Required
// policy therefore fails closed on Windows rather than running with weaker
// containment than the policy promises.
func (WindowsBackend) Compile(p Policy) (Compiled, error) {
	if err := compileCommon(p, false); err != nil {
		return Compiled{Backend: "windows"}, err
	}
	c := Compiled{Backend: "windows", Features: []string{
		"env_filter", "job_object", string(FeatureResourceLimits),
	}}
	if p.SyntheticHome != "" || p.SyntheticTemp != "" {
		c.Features = append(c.Features, string(FeatureSyntheticEnv))
	}
	c.Unavailable = append(c.Unavailable,
		"appcontainer",
		string(FeatureFSRead), string(FeatureFSWrite),
		string(FeatureNetworkNone), string(FeatureProcessTree),
	)
	if err := validateRequiredFeatures(c, p); err != nil {
		return c, err
	}
	return c, nil
}

func (b WindowsBackend) Apply(ctx context.Context, p Policy) (Cleanup, error) {
	_ = ctx
	if _, err := b.Compile(p); err != nil {
		return nil, err
	}
	return func() {}, nil
}

func (WindowsBackend) Constrain(cmd *exec.Cmd, p Policy) error {
	if _, err := (WindowsBackend{}).Compile(p); err != nil {
		return err
	}
	cmd.Env = filterEnv(p, cmd.Env)
	if p.SyntheticHome != "" {
		_ = os.MkdirAll(p.SyntheticHome, 0o700)
		cmd.Env = append(cmd.Env, "HOME="+p.SyntheticHome, "USERPROFILE="+p.SyntheticHome)
	}
	if p.SyntheticTemp != "" {
		_ = os.MkdirAll(p.SyntheticTemp, 0o700)
		cmd.Env = append(cmd.Env, "TEMP="+p.SyntheticTemp, "TMP="+p.SyntheticTemp)
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= syscall.CREATE_NEW_PROCESS_GROUP
	return nil
}

func (WindowsBackend) Attach(cmd *exec.Cmd, p Policy) (Cleanup, error) {
	if cmd == nil || cmd.Process == nil {
		return nil, fmt.Errorf("%w: process not started", ErrRequiredIsolation)
	}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: CreateJobObject: %v", ErrRequiredIsolation, err)
	}
	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE | windows.JOB_OBJECT_LIMIT_DIE_ON_UNHANDLED_EXCEPTION
	if p.MaxProcesses > 0 {
		info.BasicLimitInformation.LimitFlags |= windows.JOB_OBJECT_LIMIT_ACTIVE_PROCESS
		info.BasicLimitInformation.ActiveProcessLimit = uint32(p.MaxProcesses)
	}
	if p.MemoryBytes > 0 {
		info.BasicLimitInformation.LimitFlags |= windows.JOB_OBJECT_LIMIT_JOB_MEMORY
		info.JobMemoryLimit = uintptr(p.MemoryBytes)
	}
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		_ = windows.CloseHandle(job)
		return nil, fmt.Errorf("%w: SetInformationJobObject: %v", ErrRequiredIsolation, err)
	}
	ph, err := windows.OpenProcess(windows.PROCESS_ALL_ACCESS, false, uint32(cmd.Process.Pid))
	if err != nil {
		_ = windows.CloseHandle(job)
		return nil, fmt.Errorf("%w: OpenProcess: %v", ErrRequiredIsolation, err)
	}
	if err := windows.AssignProcessToJobObject(job, ph); err != nil {
		_ = windows.CloseHandle(ph)
		_ = windows.CloseHandle(job)
		return nil, fmt.Errorf("%w: AssignProcessToJobObject: %v", ErrRequiredIsolation, err)
	}
	_ = windows.CloseHandle(ph)
	winJobs.Store(cmd, job)
	return func() {
		_ = windows.TerminateJobObject(job, 1)
		_ = windows.CloseHandle(job)
		winJobs.Delete(cmd)
	}, nil
}

// KillTree terminates the whole Job Object when one is assigned, so every
// process that was actually contained dies. Before assignment (or if assignment
// failed) it falls back to killing the root process. The Job Object assignment
// race means this cannot be presented as guaranteed tree containment, which is
// why process_tree is not a claimed feature and Required policies fail closed.
func (WindowsBackend) KillTree(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	if v, ok := winJobs.Load(cmd); ok {
		if job, ok := v.(windows.Handle); ok {
			return windows.TerminateJobObject(job, 1)
		}
	}
	return cmd.Process.Kill()
}

func (WindowsBackend) Report() IsolationReport {
	return IsolationReport{
		Backend:   "windows",
		Available: false,
		Mode:      "job_object_management",
		Features: []string{
			"env_filter", "job_object", string(FeatureResourceLimits), string(FeatureSyntheticEnv),
		},
		Missing: []string{
			"appcontainer",
			string(FeatureFSRead), string(FeatureFSWrite),
			string(FeatureNetworkNone), string(FeatureProcessTree),
		},
		Detail: "Windows Job Objects provide process/resource management only; filesystem confinement, network denial and race-free process-tree containment are unavailable, so required-isolation harness/tool/probe execution fails closed",
	}
}
