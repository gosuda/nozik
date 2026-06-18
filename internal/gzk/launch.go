package gzk

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/gg582/nozik/internal/config"
)

const defaultGozikAddr = "127.0.0.1:50051"

// FindGozikBinary searches for a Gozik executable in common locations.
// Resolution order:
//  1. NOZIK_GOZIK_BIN environment variable
//  2. Directory containing the current executable
//  3. Sibling ../gozik/gozik relative to the current executable
//  4. PATH lookup
func FindGozikBinary() (string, error) {
	if env := os.Getenv("NOZIK_GOZIK_BIN"); env != "" {
		if exists(env) {
			return env, nil
		}
		return "", fmt.Errorf("NOZIK_GOZIK_BIN points to missing file: %s", env)
	}

	exe, err := os.Executable()
	if err == nil {
		exeDir := filepath.Dir(exe)
		candidates := []string{
			filepath.Join(exeDir, gozikExeName()),
			filepath.Join(exeDir, "gozik"),
			filepath.Join(exeDir, "..", "gozik", gozikExeName()),
			filepath.Join(exeDir, "..", "gozik", "gozik"),
		}
		for _, c := range candidates {
			if exists(c) {
				return c, nil
			}
		}
	}

	if path, err := exec.LookPath("gozik"); err == nil {
		return path, nil
	}

	return "", fmt.Errorf("gozik binary not found; set NOZIK_GOZIK_BIN or add gozik to PATH")
}

func gozikExeName() string {
	if runtime.GOOS == "windows" {
		return "gozik.exe"
	}
	return "gozik"
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// isAddrReachable reports whether a TCP connection to addr succeeds within timeout.
func isAddrReachable(addr string, timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// gozikLogPath returns the path where Gozik stdout/stderr are redirected.
func gozikLogPath() string {
	return filepath.Join(config.AppDir(), "gozik.log")
}

// LaunchGozik starts a Gozik process listening at addr and returns immediately.
// The process is launched independently of Nozik in a new session and process
// group, with detached I/O, so it keeps running after Nozik exits. Nozik does
// not manage or wait for the process.
// If addr is empty it defaults to 127.0.0.1:50051.
func LaunchGozik(addr, binaryPath string) error {
	if addr == "" {
		addr = defaultGozikAddr
	}

	// Don't start a second instance if the port is already in use.
	if isAddrReachable(addr, 300*time.Millisecond) {
		return nil
	}

	if binaryPath == "" {
		var err error
		binaryPath, err = FindGozikBinary()
		if err != nil {
			return err
		}
	}

	if err := config.EnsureAppDir(); err != nil {
		return fmt.Errorf("ensure app dir: %w", err)
	}
	logFile, err := os.OpenFile(gozikLogPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open gozik log: %w", err)
	}
	defer logFile.Close()

	cmdEnv := append(os.Environ(), "GOZIK_GRPC_ADDR="+addr)

	attr := &os.ProcAttr{
		Env:   cmdEnv,
		Files: []*os.File{nil, logFile, logFile}, // stdin /dev/null, stdout+stderr -> log
		Sys: &syscall.SysProcAttr{
			Setsid: true, // new session & process group; independent of Nozik
		},
	}

	process, err := os.StartProcess(binaryPath, []string{gozikExeName()}, attr)
	if err != nil {
		return fmt.Errorf("start gozik %s: %w", binaryPath, err)
	}

	// Release immediately: Nozik does not own or wait for the Gozik process.
	_ = process.Release()
	return nil
}

// LaunchAndDial starts Gozik if it is not already running and then dials it.
// It returns the client and any error encountered while waiting for Gozik to
// become ready. The launched process runs independently and is not managed by
// the caller.
func LaunchAndDial(ctx context.Context, addr string) (*Client, error) {
	if addr == "" {
		addr = defaultGozikAddr
	}

	if err := LaunchGozik(addr, ""); err != nil {
		return nil, err
	}

	return WaitForGozik(ctx, addr)
}

// WaitForGozik repeatedly tries to dial Gozik until it succeeds or ctx is cancelled.
func WaitForGozik(ctx context.Context, addr string) (*Client, error) {
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()

	for {
		if client, err := Dial(addr); err == nil {
			return client, nil
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("timed out waiting for Gozik at %s: %w", addr, ctx.Err())
		case <-ticker.C:
		}
	}
}
