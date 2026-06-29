package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// copyClipboard is indirected for tests.
var copyClipboard = copyText

// copyText places s on the clipboard: it emits an OSC 52 escape sequence,
// which works in most modern terminals and through ssh, and, best effort,
// writes through a local clipboard tool for terminals without OSC 52
// support.
func copyText(s string) {
	fmt.Printf("\x1b]52;c;%s\x07", base64.StdEncoding.EncodeToString([]byte(s)))
	localCopy(s)
}

func localCopy(s string) {
	var name string
	var args []string
	switch {
	case os.Getenv("WAYLAND_DISPLAY") != "":
		if p, err := exec.LookPath("wl-copy"); err == nil {
			name = p
		}
	case os.Getenv("DISPLAY") != "":
		if p, err := exec.LookPath("xclip"); err == nil {
			name, args = p, []string{"-selection", "clipboard"}
		} else if p, err := exec.LookPath("xsel"); err == nil {
			name, args = p, []string{"--clipboard", "--input"}
		}
	}
	if name == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = strings.NewReader(s)
	_ = cmd.Run()
}
