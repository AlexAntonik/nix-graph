package main

import (
	"strings"
	"testing"
)

func copyTestGraph() *Graph {
	root := storePrefix + "0123456789abcdefghij0123456789ab-bash-5.2-p103"
	return &Graph{
		Root:    root,
		info:    map[string]*Info{root: {NarSize: 10}},
		closure: map[string]Closure{},
	}
}

func TestCopyFlow(t *testing.T) {
	var got string
	orig := copyClipboard
	copyClipboard = func(s string) { got = s }
	defer func() { copyClipboard = orig }()

	const hash = "0123456789abcdefghij0123456789ab"
	g := copyTestGraph()
	u := NewUI(g)

	u.handle([]byte("y"))
	if !u.copyMode {
		t.Fatal("y should enter copy mode")
	}
	if got != "" {
		t.Errorf("copy before choice = %q, want none", got)
	}

	u.handle([]byte("h"))
	if u.copyMode {
		t.Error("choice should leave copy mode")
	}
	if got != hash {
		t.Errorf("copied hash = %q, want %q", got, hash)
	}
	if u.flash != "copied: hash" {
		t.Errorf("flash = %q, want copied: hash", u.flash)
	}
	if line := stripANSI(u.statusLine()); !strings.Contains(line, "copied: hash") {
		t.Errorf("statusline = %q, want flash cell", line)
	}

	u.handle([]byte("j"))
	if u.flash != "" {
		t.Errorf("flash after next keypress = %q, want cleared", u.flash)
	}

	u.handle([]byte("y"))
	u.handle([]byte("p"))
	if got != storePrefix+hash+"-bash-5.2-p103" {
		t.Errorf("copied path = %q, want the full store path", got)
	}

	u.handle([]byte("y"))
	u.handle([]byte("n"))
	if got != "bash-5.2-p103" {
		t.Errorf("copied name = %q, want bash-5.2-p103", got)
	}

	u.handle([]byte("y"))
	u.handle([]byte("x"))
	if got != "bash-5.2-p103" || u.copyMode {
		t.Errorf("other key = %q mode=%v, want no copy and closed mode", got, u.copyMode)
	}

	u.handle([]byte("y"))
	if !u.handle([]byte{3}) {
		t.Error("ctrl-c in copy mode must quit")
	}
}

func TestCopyOverlayContent(t *testing.T) {
	g := copyTestGraph()
	u := NewUI(g)
	const hash = "0123456789abcdefghij0123456789ab"
	o := stripANSI(u.copyOverlay())
	for _, want := range []string{"Copy", "h  hash", "p  path", "n  name", hash, "bash-5.2-p103"} {
		if !strings.Contains(o, want) {
			t.Errorf("copy overlay missing %q:\n%s", want, o)
		}
	}
}
