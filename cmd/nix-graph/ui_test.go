package main

import (
	"strings"
	"testing"
)

func copyTestGraph() *Graph {
	root := storePrefix + "0123456789abcdefghij0123456789ab-bash-5.2-p103"
	return &Graph{
		Root: root,
		info: map[string]*Info{root: {NarSize: 10}},
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

	// keys arriving in one read: y then h still copies, not collapse
	u.handle([]byte("yh"))
	if got != hash {
		t.Errorf("pasted yh = %q, want %q", got, hash)
	}
	if u.copyMode {
		t.Error("pasted yh should leave copy mode")
	}
}

func TestShellFlow(t *testing.T) {
	var dir string
	orig := spawnShell
	spawnShell = func(d string) error { dir = d; return nil }
	defer func() { spawnShell = orig }()

	u := NewUI(copyTestGraph())
	u.handle([]byte("s"))

	if dir != u.tree.Path {
		t.Errorf("shell dir = %q, want the selected path %q", dir, u.tree.Path)
	}
	if u.flash != "" {
		t.Errorf("flash = %q, want none on success", u.flash)
	}
	if u.keys == nil || u.keyStop == nil {
		t.Error("key reader must be restarted after the shell exits")
	}
}

func TestShellSpawnError(t *testing.T) {
	u := NewUI(copyTestGraph())
	u.sel.Path = storePrefix + "00000000000000000000000000000000-gone"
	u.handle([]byte("s"))

	if !strings.HasPrefix(u.flash, "shell: ") {
		t.Errorf("flash = %q, want the spawn error with shell: prefix", u.flash)
	}
}

func TestNarSortKey(t *testing.T) {
	u := NewUI(copyTestGraph())
	u.handle([]byte("z"))
	if u.sortKey != sortNar || u.sortDesc {
		t.Errorf("z state = %d/%v, want sortNar/asc", u.sortKey, u.sortDesc)
	}
	u.handle([]byte("z"))
	if u.sortKey != sortNone {
		t.Errorf("second z = %d, want sortNone", u.sortKey)
	}
	u.handle([]byte("z"))
	if u.sortKey != sortNar || !u.sortDesc {
		t.Errorf("third z = %d/%v, want sortNar/desc", u.sortKey, u.sortDesc)
	}
}

func TestStatusLineNarrowFlash(t *testing.T) {
	u := NewUI(testGraph())
	u.flash = "copied: hash"
	u.w = 50 // fits two cells, drops the flash
	line := u.statusLine()
	if strings.Contains(stripANSI(line), "copied") {
		t.Errorf("narrow status line must drop the flash: %q", stripANSI(line))
	}
	if strings.Contains(line, cyan+" closure") {
		t.Error("closure cell must not take the flash styling")
	}
}

func TestHelpMode(t *testing.T) {
	u := NewUI(copyTestGraph())
	u.handle([]byte("?"))
	if !u.helpMode {
		t.Fatal("? should open help")
	}
	u.handle([]byte("j"))
	if u.helpMode {
		t.Error("any key should close help")
	}
	if u.sel != u.tree {
		t.Error("closing key must not act on the tree")
	}
	if !u.handle([]byte("q")) {
		t.Error("q should quit on the open tree")
	}
	u.handle([]byte("?"))
	if !u.handle([]byte{3}) {
		t.Error("ctrl-c in help must quit")
	}
}

func TestOverlayKeepsWideValues(t *testing.T) {
	u := &UI{w: 80, h: 24}
	val := strings.Repeat("x", 30)
	o := stripANSI(u.overlay("T", [][2]string{{"a", val}, {"abcdefghijklmnop", "v"}}, "hint"))
	if !strings.Contains(o, val) {
		t.Errorf("overlay truncated the long first value: %q", o)
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
