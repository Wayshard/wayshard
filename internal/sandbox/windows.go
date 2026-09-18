//go:build windows

package sandbox

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

func (WindowsBackend) Compile(p Policy) (Compiled, error) {
	if err := compileCommon(p); err != nil {
		return Compiled{Backend: "windows"}, err
	}
	c := Compiled{Backend: "windows", Features: []string{"job_object", "process_tree", "env_filter"}}
	if p.SyntheticHome != "" || p.SyntheticTemp != "" {
		c.Features = append(c.Features, "synthetic_home")
	}
	c.Unavailable = append(c.Unavailable, "appcontainer")
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
		if p.Required {
			return nil, fmt.Errorf("%w: CreateJobObject: %v", ErrRequiredIsolation, err)
		}
		return nil, fmt.Errorf("%w: CreateJobObject: %v", ErrRequiredIsolation, err)
	}
	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE | windows.JOB_OBJECT_LIMIT_BREAKAWAY_OK | windows.JOB_OBJECT_LIMIT_DIE_ON_UNHANDLED_EXCEPTION
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
	return func() {
		_ = windows.TerminateJobObject(job, 1)
		_ = windows.CloseHandle(job)
	}, nil
}

func (WindowsBackend) KillTree(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}

func (WindowsBackend) Report() IsolationReport {
	return IsolationReport{
		Backend:   "windows",
		Available: true,
		Mode:      "job_object",
		Features:  []string{"job_object", "process_tree", "env_filter"},
		Missing:   []string{"appcontainer"},
		Detail:    "Windows Job Objects for process-tree and resource limits; AppContainer filesystem isolation not claimed",
	}
}
