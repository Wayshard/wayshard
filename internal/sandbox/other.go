package sandbox

// Process-group helpers used by tests on unix. KillTree on Linux/Darwin uses
// negative pgid. This file is intentionally empty of Apply implementations so
// platform files remain the sole compilers of SandboxPolicy.
