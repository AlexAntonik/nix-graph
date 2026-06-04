//go:build linux

package main

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

const (
	altEnter = "\x1b[?1049h"
	altExit  = "\x1b[?1049l"
	hideCur  = "\x1b[?25l"
	showCur  = "\x1b[?25h"
	clearScr = "\x1b[H\x1b[J"
	eraseEol = "\x1b[K"
	reset    = "\x1b[0m"

	dim  = "\x1b[90m"
	bold = "\x1b[1m"
	cyan = "\x1b[36m"
	blue = "\x1b[34m"
	yell = "\x1b[33m"
	rev  = "\x1b[7m"
)

const (
	sortNone = iota
	sortOwn
	sortDeps
	sortName
	sortClosure
)

type UI struct {
	g         *Graph
	tree      *Node
	sel       *Node
	rows      []Row
	offset    int
	w, h      int
	mu        sync.Mutex
	contents  map[string]Contents
	pending   map[string]bool
	redraw    chan struct{}
	clearNext bool
	sortKey   int
	sortDesc  bool
}

func NewUI(g *Graph) *UI {
	tree := NewTree(g)
	return &UI{
		g:         g,
		tree:      tree,
		sel:       tree,
		w:         80,
		h:         24,
		contents:  map[string]Contents{},
		pending:   map[string]bool{},
		redraw:    make(chan struct{}, 1),
		clearNext: true,
		sortKey:   sortOwn,
		sortDesc:  true,
	}
}

func setupTerminal() (func(), error) {
	old, err := makeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return nil, err
	}
	restore := func() {
		fmt.Print(reset + showCur + altExit)
		restoreTerm(int(os.Stdin.Fd()), old)
	}
	return restore, nil
}

func enterScreen() {
	fmt.Print(altEnter + hideCur + clearScr)
}

func (u *UI) loop() error {
	if w, h, err := termSize(); err == nil {
		u.w, u.h = w, h
	}
	keys := make(chan []byte, 8)
	go readKeys(keys)
	resize := make(chan os.Signal, 1)
	signal.Notify(resize, syscall.SIGWINCH)
	defer signal.Stop(resize)

	for {
		u.render()
		select {
		case buf, ok := <-keys:
			if !ok {
				return errors.New("terminal input closed")
			}
			if u.handle(buf) {
				return nil
			}
		case <-resize:
			if w, h, err := termSize(); err == nil {
				u.w, u.h = w, h
			}
			u.clearNext = true
		case <-u.redraw:
		}
	}
}

func readKeys(ch chan<- []byte) {
	defer close(ch)
	buf := make([]byte, 64)
	for {
		n, err := os.Stdin.Read(buf)
		if n > 0 {
			b := make([]byte, n)
			copy(b, buf[:n])
			ch <- b
		}
		if err != nil {
			return
		}
	}
}

func (u *UI) handle(buf []byte) bool {
	u.rows = u.tree.Visible()
	for i := 0; i < len(buf); i++ {
		switch b := buf[i]; {
		case b == 'q' || b == 'Q' || b == 3:
			return true
		case b == 0x1b:
			if i+2 < len(buf) && (buf[i+1] == '[' || buf[i+1] == 'O') {
				u.escape(buf[i+2])
				i += 2
			}
		case b == ' ' || b == '\r' || b == '\n':
			u.sel.Toggle(u.g)
			u.resort()
		case b == 'j':
			u.move(1)
		case b == 'k':
			u.move(-1)
		case b == 'l':
			u.drill()
		case b == 'h':
			u.up()
		case b == 'g':
			u.jump(0)
		case b == 'G':
			u.jump(len(u.rows) - 1)
		case b == 'o':
			u.setSort(sortOwn)
		case b == 'd':
			u.setSort(sortDeps)
		case b == 'n':
			u.setSort(sortName)
		case b == 'c':
			u.setSort(sortClosure)
		}
	}
	return false
}

func (u *UI) escape(c byte) {
	switch c {
	case 'A':
		u.move(-1)
	case 'B':
		u.move(1)
	case 'C':
		u.drill()
	case 'D':
		u.up()
	case 'H':
		u.jump(0)
	case 'F':
		u.jump(len(u.rows) - 1)
	case '1':
		u.jump(0)
	case '4':
		u.jump(len(u.rows) - 1)
	case '5':
		u.page(-1)
	case '6':
		u.page(1)
	}
}

func (u *UI) viewH() int {
	if h := u.h - 2; h > 0 {
		return h
	}
	return 1
}

func (u *UI) jump(idx int) {
	if len(u.rows) == 0 {
		return
	}
	if idx < 0 {
		idx = 0
	}
	if idx >= len(u.rows) {
		idx = len(u.rows) - 1
	}
	u.sel = u.rows[idx].Node
}

func (u *UI) move(d int) {
	if idx := u.indexOf(u.sel); idx >= 0 {
		u.jump(idx + d)
	}
}

func (u *UI) page(dir int) {
	u.move(dir * (u.viewH() - 1))
}

func (u *UI) drill() {
	if u.sel.isLeaf(u.g) {
		return
	}
	if !u.sel.Expanded {
		u.sel.Toggle(u.g)
		return
	}
	u.move(1)
}

func (u *UI) up() {
	if u.sel.Expanded && u.sel.Loaded && len(u.sel.Children) > 0 {
		u.sel.Expanded = false
		return
	}
	if u.sel.Parent != nil {
		u.sel = u.sel.Parent
	}
}

func (u *UI) indexOf(n *Node) int {
	for i, r := range u.rows {
		if r.Node == n {
			return i
		}
	}
	return -1
}

func (u *UI) setSort(k int) {
	idx := u.indexOf(u.sel)
	if u.sortKey != k {
		u.sortKey = k
		u.sortDesc = true
	} else if u.sortDesc {
		u.sortDesc = false
	} else {
		u.sortKey = sortNone
	}
	u.resort()
	u.rows = u.tree.Visible()
	if idx >= 0 && idx < len(u.rows) {
		u.sel = u.rows[idx].Node
	}
}

func (u *UI) resort() {
	var walk func(*Node)
	walk = func(n *Node) {
		if !n.Loaded {
			return
		}
		children := n.Children
		sort.SliceStable(children, func(i, j int) bool {
			return u.less(children[i], children[j])
		})
		for _, c := range children {
			walk(c)
		}
	}
	walk(u.tree)
}

func (u *UI) less(a, b *Node) bool {
	ia, ib := u.g.Get(a.Path), u.g.Get(b.Path)
	switch u.sortKey {
	case sortNone:
		sa, sb := infoSize(ia), infoSize(ib)
		if sa != sb {
			return sa > sb
		}
	case sortDeps:
		da, db := infoDirect(ia), infoDirect(ib)
		if da != db {
			return u.ord(da > db)
		}
	case sortOwn:
		sa, sb := infoSize(ia), infoSize(ib)
		if sa != sb {
			return u.ord(sa > sb)
		}
	case sortName:
		pa, pb := PkgName(a.Path), PkgName(b.Path)
		if pa != pb {
			return u.ord(pa > pb)
		}
	case sortClosure:
		ca, cb := u.g.Closure(a.Path).Bytes, u.g.Closure(b.Path).Bytes
		if ca != cb {
			return u.ord(ca > cb)
		}
	}
	return Name(a.Path) < Name(b.Path)
}

func (u *UI) ord(greater bool) bool {
	if u.sortDesc {
		return greater
	}
	return !greater
}

func infoDirect(i *Info) int {
	if i == nil {
		return 0
	}
	return i.Direct
}

func infoSize(i *Info) uint64 {
	if i == nil {
		return 0
	}
	return i.NarSize
}

func (u *UI) render() {
	u.rows = u.tree.Visible()
	if u.indexOf(u.sel) < 0 {
		u.sel = u.tree
	}
	rw := rightWidth(u.w)
	lw := u.w - rw
	if rw > 0 {
		lw--
	}
	selIdx := u.indexOf(u.sel)
	if selIdx < u.offset {
		u.offset = selIdx
	}
	if selIdx >= u.offset+u.viewH() {
		u.offset = selIdx - u.viewH() + 1
	}
	if u.offset < 0 {
		u.offset = 0
	}
	for u.offset < selIdx {
		st := u.sticky(u.offset)
		if len(st) > u.viewH()-1 || u.offset+u.viewH()-1-len(st) >= selIdx {
			break
		}
		u.offset++
	}

	sticky := u.sticky(u.offset)
	if max := u.viewH() - 1; len(sticky) > max {
		sticky = sticky[len(sticky)-max:]
	}
	right := u.rightPanel(rw, u.viewH())
	var b strings.Builder
	if u.clearNext {
		b.WriteString(clearScr)
		u.clearNext = false
	} else {
		b.WriteString("\x1b[H")
	}
	for r := 0; r < u.h-1; r++ {
		var left string
		switch {
		case r == 0:
			left = u.headerLine(lw)
		case r-1 < len(sticky):
			left = u.leftLine(sticky[r-1], lw)
		default:
			if idx := u.offset + r - 1 - len(sticky); idx < len(u.rows) {
				left = u.leftLine(u.rows[idx], lw)
			} else {
				left = padEnd("", lw)
			}
		}
		b.WriteString(left)
		b.WriteString(eraseEol)
		if rw > 0 {
			b.WriteString(dim + "│" + reset)
			if line := r - 1; line >= 0 && line < len(right) {
				b.WriteString(right[line])
			}
			b.WriteString(eraseEol)
		}
		if r < u.h-2 {
			b.WriteString("\r\n")
		}
	}
	b.WriteString("\r\n")
	b.WriteString(u.statusLine())
	os.Stdout.WriteString(b.String())
}

func rightWidth(w int) int {
	switch {
	case w < 70:
		return 0
	case w < 110:
		return 36
	default:
		return 48
	}
}

type headerLabel struct {
	s string
	w int
}

func (l headerLabel) pad(w int) string {
	if pad := w - l.w; pad > 0 {
		return l.s + strings.Repeat(" ", pad)
	}
	return l.s
}

func (u *UI) colLabel(text, letter string, key int) headerLabel {
	i := strings.Index(text, letter)
	s := text[:i] + cyan + letter + reset + bold + text[i+1:]
	w := runeLen(text)
	if u.sortKey == key {
		arrow := " ↑"
		if u.sortDesc {
			arrow = " ↓"
		}
		s += cyan + arrow + reset + bold
		w += 2
	}
	return headerLabel{s, w}
}

func (u *UI) headerLine(lw int) string {
	const metaW = 9 + 1 + 8 + 1 + 6
	cl := u.colLabel("CLOSURE", "C", sortClosure)
	own := u.colLabel("OWN", "O", sortOwn)
	deps := u.colLabel("DEPS", "D", sortDeps)
	name := u.colLabel(" NAME", "N", sortName)
	if lw < name.w+metaW+1 {
		return bold + padEnd(truncate("nixview", lw), lw) + reset
	}
	meta := cl.pad(9) + " " + own.pad(8) + " " + deps.pad(6)
	return bold + name.pad(lw-metaW) + meta + reset
}

func (u *UI) sticky(offset int) []Row {
	var chain []*Node
	for n := u.sel.Parent; n != nil; n = n.Parent {
		chain = append(chain, n)
	}
	var rows []Row
	for i := len(chain) - 1; i >= 0; i-- {
		idx := u.indexOf(chain[i])
		if idx < 0 || idx >= offset {
			break
		}
		rows = append(rows, u.rows[idx])
	}
	return rows
}

func (u *UI) leftLine(row Row, lw int) string {
	info := u.g.Get(row.Node.Path)
	if info == nil {
		return padEnd("", lw)
	}
	m := row.Node.Marker(u.g)
	if m != "" {
		m += " "
	}
	lead := row.Prefix + row.Conn
	name := lead + m + ShortName(row.Node.Path)
	cl, own := HumanSize(u.g.Closure(row.Node.Path).Bytes), HumanSize(info.NarSize)
	deps := strconv.Itoa(info.Direct)
	meta := fmt.Sprintf("%9s %8s %6s", cl, own, deps)
	nameW := lw - runeLen(meta)
	if nameW < 1 {
		return padEnd(truncate(name, lw), lw)
	}
	if row.Node == u.sel {
		return rev + padEnd(name, nameW) + meta + reset
	}
	markColor := dim
	switch m {
	case "[+] ":
		markColor = cyan
	case "[-] ":
		markColor = blue
	}
	body := padEnd(ShortName(row.Node.Path), nameW-runeLen(lead)-runeLen(m))
	return dim + lead + reset + markColor + m + reset + body +
		cyan + fmt.Sprintf("%9s", cl) + reset + " " +
		blue + fmt.Sprintf("%8s", own) + reset + " " +
		yell + fmt.Sprintf("%6s", deps) + reset
}

func (u *UI) statusLine() string {
	s := fmt.Sprintf(" %s │ %d paths │ closure %s │ j/k move · space toggle · h/l fold/drill · g/G ends · q quit",
		ShortName(u.g.Root), u.g.Size(), HumanSize(u.g.Closure(u.g.Root).Bytes))
	return rev + truncate(s, u.w) + reset
}

func (u *UI) rightPanel(w, maxRows int) []string {
	if w < 20 || maxRows < 1 {
		return nil
	}
	info := u.g.Get(u.sel.Path)
	if info == nil {
		return nil
	}
	path := u.sel.Path
	u.mu.Lock()
	c, have := u.contents[path]
	busy := u.pending[path]
	if !have && !busy {
		u.pending[path] = true
		busy = true
		go func() {
			res, err := Inspect(path)
			u.mu.Lock()
			if err == nil {
				u.contents[path] = res
			}
			delete(u.pending, path)
			u.mu.Unlock()
			select {
			case u.redraw <- struct{}{}:
			default:
			}
		}()
	}
	u.mu.Unlock()

	var out []string
	add := func(s string) {
		if len(out) < maxRows {
			out = append(out, s)
		}
	}
	kv := func(k, v string) {
		add(dim + " " + padEnd(k, 12) + reset + truncate(v, w-14))
	}
	add(bold + " " + truncate(ShortName(u.sel.Path), w-1) + reset)
	kv("name", Name(u.sel.Path))
	kv("hash", Hash(u.sel.Path))
	kv("path", u.sel.Path)
	add(dim + strings.Repeat("─", w) + reset)
	cl := u.g.Closure(u.sel.Path)
	kv("own size", HumanSize(info.NarSize))
	kv("closure", fmt.Sprintf("%s / %d paths", HumanSize(cl.Bytes), cl.Paths))
	kv("deps direct", strconv.Itoa(info.Direct))
	kv("deps total", strconv.Itoa(cl.Paths-1))
	if info.Deriver != "" {
		kv("deriver", Name(info.Deriver))
	}
	if info.RegistrationTime > 0 {
		kv("registered", time.Unix(info.RegistrationTime, 0).Format("2006-01-02"))
	}
	add(dim + strings.Repeat("─", w) + reset)
	switch {
	case have:
		add(bold + fmt.Sprintf(" contents: %d dirs %d files %d exec", c.Dirs, c.Files, c.Execs) + reset)
		for _, e := range c.Entries {
			if len(out) >= maxRows {
				break
			}
			nm := truncate(e.Name, w-13)
			if e.Dir {
				nm += "/"
			}
			style := reset
			switch {
			case e.Dir:
				style = cyan
			case e.Exec:
				style = bold
			}
			add(style + padEnd("  "+nm, w-9) + reset + dim + fmt.Sprintf("%8s", HumanSize(e.Size)) + reset)
		}
	case busy:
		add(dim + " inspecting contents…" + reset)
	}
	return out
}

func makeRaw(fd int) (*syscall.Termios, error) {
	var old syscall.Termios
	if err := ioctl(fd, syscall.TCGETS, &old); err != nil {
		return nil, err
	}
	raw := old
	raw.Iflag &^= syscall.IGNBRK | syscall.BRKINT | syscall.PARMRK | syscall.ISTRIP |
		syscall.INLCR | syscall.IGNCR | syscall.ICRNL | syscall.IXON
	raw.Oflag &^= syscall.OPOST
	raw.Lflag &^= syscall.ECHO | syscall.ECHONL | syscall.ICANON | syscall.ISIG | syscall.IEXTEN
	raw.Cflag &^= syscall.CSIZE | syscall.PARENB
	raw.Cflag |= syscall.CS8
	raw.Cc[syscall.VMIN] = 1
	raw.Cc[syscall.VTIME] = 0
	if err := ioctl(fd, syscall.TCSETS, &raw); err != nil {
		return nil, err
	}
	return &old, nil
}

func restoreTerm(fd int, old *syscall.Termios) {
	if old != nil {
		_ = ioctl(fd, syscall.TCSETS, old)
	}
}

func ioctl(fd int, req uint, t *syscall.Termios) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), uintptr(req), uintptr(unsafe.Pointer(t)))
	if errno != 0 {
		return errno
	}
	return nil
}

type winsize struct{ Row, Col, X, Y uint16 }

func termSize() (int, int, error) {
	var ws winsize
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, os.Stdout.Fd(),
		uintptr(syscall.TIOCGWINSZ), uintptr(unsafe.Pointer(&ws)))
	if errno != 0 {
		return 0, 0, errno
	}
	return int(ws.Col), int(ws.Row), nil
}
