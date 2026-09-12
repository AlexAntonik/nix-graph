//go:build linux

package main

import (
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"unsafe"
)

// shellBin is the shell to spawn: $SHELL or a portable fallback.
func shellBin() string {
	if sh := os.Getenv("SHELL"); sh != "" {
		return sh
	}
	return "/bin/sh"
}

// tcsetpgrp makes pgid the terminal's foreground process group.
func tcsetpgrp(fd, pgid int) {
	p := int32(pgid)
	_ = ioctl(fd, syscall.TIOCSPGRP, unsafe.Pointer(&p))
}

// spawnShell indirected for tests
var spawnShell = spawnShellRun

// spawnShellRun runs $SHELL in dir. 
func spawnShellRun(dir string) error {
	cmd := exec.Command(shellBin())
	cmd.Dir = dir
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// the child may own the terminal while nix-graph sits in the
	// background: without this, the tcsetpgrp below raises SIGTTOU and
	// stops nix-graph, handing the terminal to the parent shell
	signal.Ignore(syscall.SIGTTOU)
	defer signal.Reset(syscall.SIGTTOU)
	if err := cmd.Start(); err != nil {
		return err
	}
	fd := int(os.Stdin.Fd())
	tcsetpgrp(fd, cmd.Process.Pid)
	err := cmd.Wait()
	tcsetpgrp(fd, syscall.Getpgrp())
	return err
}

// shell suspends the tui, hands the terminal to $SHELL
func (u *UI) shell() {
	close(u.keyStop)
	for range u.keys { // wait for the key reader to release stdin
	}
	suspend()
	if err := spawnShell(u.sel.Path); err != nil {
		u.flash = "shell: " + err.Error()
	}
	resume()
	enterScreen()
	u.w, u.h = termSizeOr(u.w, u.h)
	u.clearNext = true
	u.keys, u.keyStop = make(chan []byte, 8), make(chan struct{})
	go readKeys(u.keys, u.keyStop)
}
