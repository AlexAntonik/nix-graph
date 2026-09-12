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
	"unicode/utf8"
	"unsafe"
)

const (
	altEnter = "\x1b[?1049h"
	altExit  = "\x1b[?1049l"
	hideCur  = "\x1b[?25l"
	showCur  = "\x1b[?25h"
	clearScr = "\x1b[H\x1b[J"
	reset    = "\x1b[0m"

	dim    = "\x1b[90m"
	bold   = "\x1b[1m"
	cyan   = "\x1b[36m"
	blue   = "\x1b[34m"
	green  = "\x1b[32m"
	yell   = "\x1b[33m"
	rev    = "\x1b[7m"
	white  = "\x1b[97m"
	orange = "\x1b[38;5;208m"
)

const (
	sortNone = iota
	sortNar
	sortDeps
	sortName
	sortClosure
	sortAdded
)

type UI struct {
	g          *Graph
	tree       *Node
	sel        *Node
	rows       []Row
	offset     int
	w, h       int
	clearNext  bool
	sortKey    int
	sortDesc   bool
	filter     string
	filterMode bool
	helpMode   bool
	copyMode   bool
	flash      string
	reverse    bool
	forest     bool
	keys       chan []byte
	keyStop    chan struct{}
}

func NewUI(g *Graph) *UI {
	tree := NewTree(g)
	u := &UI{
		g:         g,
		tree:      tree,
		sel:       tree,
		w:         80,
		h:         24,
		clearNext: true,
		sortKey:   sortNar,
		sortDesc:  true,
		keys:      make(chan []byte, 8),
		keyStop:   make(chan struct{}),
	}
	go readKeys(u.keys, u.keyStop)
	return u
}

// savedTerm is the cooked terminal state captured at startup; suspend
// hands it back to child programs
var savedTerm *syscall.Termios

// leaveScreen exits the alt screen and shows the cursor.
func leaveScreen() {
	fmt.Print(reset + showCur + altExit)
}

// suspend returns the terminal to the state it had before the tui started.
func suspend() {
	leaveScreen()
	restoreTerm(int(os.Stdin.Fd()), savedTerm)
}

// resume puts the terminal back into raw mode.
func resume() {
	_, _ = makeRaw(int(os.Stdin.Fd()))
}

func setupTerminal() (func(), error) {
	old, err := makeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return nil, err
	}
	savedTerm = old
	var once sync.Once
	restore := func() {
		once.Do(func() {
			leaveScreen()
			restoreTerm(int(os.Stdin.Fd()), old)
		})
	}
	// ISIG stays on, so Ctrl+C raises SIGINT at the kernel level even
	// when the event loop is stuck
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		restore()
		os.Exit(130)
	}()
	return restore, nil
}

func enterScreen() {
	fmt.Print(altEnter + hideCur + clearScr)
}

// loop is the main event loop
func (u *UI) loop() error {
	u.w, u.h = termSizeOr(u.w, u.h)
	resize := make(chan os.Signal, 1)
	signal.Notify(resize, syscall.SIGWINCH)
	defer signal.Stop(resize)

	for {
		u.render()
		select {
		case buf, ok := <-u.keys:
			if !ok {
				return errors.New("terminal input closed")
			}
			if u.handle(buf) {
				return nil
			}
		case <-resize:
			u.w, u.h = termSizeOr(u.w, u.h)
			u.clearNext = true
		}
	}
}

func readKeys(ch chan<- []byte, stop <-chan struct{}) {
	defer close(ch)
	fd := int(os.Stdin.Fd())
	epfd, err := syscall.EpollCreate1(syscall.EPOLL_CLOEXEC)
	if err != nil {
		return
	}
	defer syscall.Close(epfd)
	ev := syscall.EpollEvent{Events: syscall.EPOLLIN}
	if err := syscall.EpollCtl(epfd, syscall.EPOLL_CTL_ADD, fd, &ev); err != nil {
		return
	}
	buf := make([]byte, 64)
	events := make([]syscall.EpollEvent, 1)
	for {
		n, err := syscall.EpollWait(epfd, events, 100)
		if err != nil && err != syscall.EINTR {
			return
		}
		select {
		case <-stop:
			return
		default:
		}
		if n == 0 {
			continue
		}
		nn, err := syscall.Read(fd, buf)
		if nn > 0 {
			b := make([]byte, nn)
			copy(b, buf[:nn])
			select {
			case ch <- b:
			case <-stop:
				return
			}
		}
		if err != nil {
			if err == syscall.EINTR || err == syscall.EAGAIN {
				continue
			}
			return
		}
	}
}

func (u *UI) matcher() func(*Node) bool {
	if u.filter == "" {
		return nil
	}
	q := strings.ToLower(u.filter)
	return func(n *Node) bool {
		return strings.Contains(strings.ToLower(PkgName(n.Path)), q)
	}
}

// handle dispatches raw input depending on the active mode; returns true for exit.
func (u *UI) handle(buf []byte) bool {
	if !u.helpMode {
		u.flash = ""
	}
	u.rows = u.tree.visibleRows(u.matcher())
	for i := 0; i < len(buf); i++ {
		switch {
		case u.helpMode:
			u.helpMode = false
			if buf[i] == 3 {
				return true
			}
		case u.copyMode:
			u.copyMode = false
			switch buf[i] {
			case 3:
				return true
			case 'h':
				u.copySel("hash", Hash(u.sel.Path))
			case 'p':
				u.copySel("path", u.sel.Path)
			case 'n':
				u.copySel("name", PkgName(u.sel.Path))
			}
		case u.filterMode:
			switch b := buf[i]; {
			case b == 3:
				return true
			case b == 0x1b:
				if i+2 < len(buf) && (buf[i+1] == '[' || buf[i+1] == 'O') {
					i += 2
				} else {
					u.filter, u.filterMode = "", false
				}
			case b == '\r' || b == '\n':
				u.filterMode = false
			case b == 0x7f || b == 8:
				if r := []rune(u.filter); len(r) > 0 {
					u.filter = string(r[:len(r)-1])
				}
			default:
				if rn, sz := utf8.DecodeRune(buf[i:]); (rn != utf8.RuneError || sz > 1) && rn >= 0x20 {
					u.filter += string(rn)
					i += sz - 1
				}
			}
		default:
			switch b := buf[i]; {
			case b == 'q' || b == 'Q' || b == 3:
				return true
			case b == 0x1b:
				if i+2 < len(buf) && (buf[i+1] == '[' || buf[i+1] == 'O') {
					u.escape(buf[i+2])
					i += 2
				} else if u.reverse {
					u.setMode(false, false)
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
			case b == 'e':
				u.expandAll()
			case b == 'E':
				u.collapseAll()
			case b == 'g':
				u.jump(0)
			case b == 'G':
				u.jump(len(u.rows) - 1)
			case b == 'z':
				u.setSort(sortNar)
			case b == 'd':
				u.setSort(sortDeps)
			case b == 'n':
				u.setSort(sortName)
			case b == 'c':
				u.setSort(sortClosure)
			case b == 'a':
				u.setSort(sortAdded)
			case b == 'f' || b == 'F':
				u.filterMode = true
			case b == 's':
				u.shell()
			case b == 'y':
				u.copyMode = true
			case b == 'p':
				if u.reverse && !u.forest {
					u.flip()
				} else {
					u.setMode(true, false)
				}
			case b == 'P':
				if u.reverse {
					u.setMode(false, false)
				} else {
					u.setMode(true, true)
				}
			case b == '?':
				u.helpMode = true
			}
		}
	}
	return false
}

func (u *UI) copySel(what, text string) {
	if text == "" {
		return
	}
	copyClipboard(text)
	u.flash = "copied: " + what
}

// setMode switches between the forward tree (reverse=false), the p-mode
// tree rooted at the selected node (p) and the all-packages forest (P).
func (u *UI) setMode(reverse, forest bool) {
	u.reverse, u.forest, u.g.Reverse = reverse, forest, reverse
	switch {
	case !reverse:
		u.tree = NewTree(u.g)
	case forest:
		u.tree = NewForest(u.g)
	default:
		u.tree = NewTreeAt(u.g, u.sel.Path)
	}
	u.rebuild()
}

// flip rebuilds the tree in the opposite direction at the current
// selection: dependents become dependencies and back.
func (u *UI) flip() {
	u.g.Reverse = !u.g.Reverse
	u.tree = NewTreeAt(u.g, u.sel.Path)
	u.rebuild()
}

// expandAll unfolds the view breadth-first, capped so a huge closure
// cannot exhaust memory.
func (u *UI) expandAll() {
	if u.tree.ExpandAll(u.g, expandLimit) {
		u.flash = fmt.Sprintf("expand all: stopped at %d nodes", expandLimit)
	}
	u.resort()
	u.rows = u.tree.visibleRows(u.matcher())
}

// collapseAll folds the view back to its top-level rows.
func (u *UI) collapseAll() {
	u.tree.CollapseAll()
	u.rows = u.tree.visibleRows(u.matcher())
	if len(u.rows) > 0 {
		u.sel = u.rows[0].Node
	}
}

func (u *UI) rebuild() {
	u.offset = 0
	u.rows = u.tree.visibleRows(u.matcher())
	if len(u.rows) > 0 {
		u.sel = u.rows[0].Node
	}
	u.resort()
}

// maps CSI sequences to navigation actions.
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
	if h := u.h - 3; h > 0 {
		return h
	}
	return 1
}

func (u *UI) frameColor() string {
	if u.reverse {
		return orange
	}
	return dim
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
	if u.sel.Hidden {
		return
	}
	if u.sel.Expanded && u.sel.Loaded && len(u.sel.Children) > 0 {
		u.sel.Expanded = false
		return
	}
	if u.sel.Parent != nil && !u.sel.Parent.Hidden {
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
	u.rows = u.tree.visibleRows(u.matcher())
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
		da, db := u.depsCount(a.Path), u.depsCount(b.Path)
		if da != db {
			return u.ord(da > db)
		}
	case sortNar:
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
	case sortAdded:
		aa, ab := u.g.Added(a.Path), u.g.Added(b.Path)
		if aa != ab {
			return u.ord(aa > ab)
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

func (u *UI) depsColumn() (string, int) {
	if u.g.Reverse {
		return "DEPENDENTS", runeLen("DEPENDENTS") + 1
	}
	return "DEPENDENCIES", runeLen("DEPENDENCIES") + 1
}

// depsCount is the number in the deps column: all transitive dependencies
// in the forward graph, all transitive dependents in the inverted one.
func (u *UI) depsCount(path string) int {
	if u.g.Get(path) == nil {
		return 0
	}
	if u.g.Reverse {
		return u.g.DependentsClosure(path) - 1
	}
	return u.g.Closure(path).Paths - 1
}

// redraws the whole screen.
func (u *UI) render() {
	u.rows = u.tree.visibleRows(u.matcher())
	selIdx := u.indexOf(u.sel)
	if selIdx < 0 {
		u.sel = u.tree
		if len(u.rows) > 0 {
			u.sel = u.rows[0].Node
			selIdx = u.indexOf(u.sel)
		}
	}
	lw := u.w - 2
	if lw < 1 {
		lw = 1
	}
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
	hang := u.topHang(sticky)
	if hang {
		for i := range sticky {
			switch rowCol2(sticky[i]) {
			case '│', '├', '┌':
			default:
				sticky[i].Prefix = " │  " + sticky[i].Prefix
			}
		}
	}
	var b strings.Builder
	if u.clearNext {
		b.WriteString(clearScr)
		u.clearNext = false
	} else {
		b.WriteString("\x1b[H")
	}
	fc := u.frameColor()
	for r := 0; r < u.h; r++ {
		var left string
		switch {
		case r == 0:
			left = u.topBorder(hang)
		case r == 1:
			left = u.headerRow(lw, hang)
		case r == u.h-1:
			left = u.statusLine()
		default:
			var line string
			switch rowIdx := r - 2; {
			case rowIdx < len(sticky):
				line = u.rowLine(sticky[rowIdx], lw-1)
			default:
				if idx := u.offset + rowIdx - len(sticky); idx < len(u.rows) {
					line = u.rowLine(u.rows[idx], lw-1)
				} else {
					line = padEnd("", lw-1)
				}
			}
			left = fc + "│ " + reset + line + fc + "│" + reset
		}
		b.WriteString(left)
		if r < u.h-1 {
			b.WriteString("\r\n")
		}
	}
	if u.helpMode {
		b.WriteString(u.helpOverlay())
	}
	if u.copyMode {
		b.WriteString(u.copyOverlay())
	}
	os.Stdout.WriteString(b.String())
}

func (u *UI) topBorder(hang bool) string {
	title := "Dependency graph"
	if u.g.Reverse {
		title = "Dependents graph"
	}
	color := white
	if u.reverse {
		color = orange
	}
	sep := " "
	if hang {
		sep = "┐"
	}
	pad := u.w - runeLen(title) - 6
	if pad < 0 {
		title = truncate(title, u.w-6)
		pad = 0
	}
	var tp string
	if i := strings.Index(title, "p"); i >= 0 {
		tp = color + title[:i] + cyan + "p" + color + title[i+1:]
	} else {
		tp = color + title
	}
	return u.frameColor() + "┌──" + sep + reset + tp + reset + u.frameColor() + " " +
		strings.Repeat("─", pad) + "┐" + reset
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

func (l headerLabel) padStart(w int) string {
	if pad := w - l.w; pad > 0 {
		return strings.Repeat(" ", pad) + l.s
	}
	return l.s
}

func (u *UI) colLabel(text, letter string, key int) headerLabel {
	i := strings.Index(text, letter)
	s := text[:i] + cyan + letter + reset + bold + text[i+1:]
	w := runeLen(text)
	if u.sortKey == key {
		arrow := "↑"
		if u.sortDesc {
			arrow = "↓"
		}
		s = cyan + arrow + reset + bold + s
		w++
	}
	return headerLabel{s, w}
}

func (u *UI) headerLine(lw int) string {
	depsLbl, depsW := u.depsColumn()
	metaW := 9 + 1 + 9 + 1 + 9 + 1 + depsW + 1
	cl := u.colLabel("CLOSURE", "C", sortClosure)
	added := u.colLabel("ADDED", "A", sortAdded)
	nar := u.colLabel("NAR-SIZE", "Z", sortNar)
	deps := u.colLabel(depsLbl, "D", sortDeps)
	name := u.colLabel("NAME", "N", sortName)
	if lw < name.w+metaW+1 {
		return bold + padEnd("nix-graph", lw) + reset
	}
	fw, fs := 0, ""
	if u.filterMode || u.filter != "" {
		const lbl = " filter:"
		cur := ""
		if u.filterMode {
			cur = "▌"
		}
		// only draw the filter when it fits; squeezing it in would push
		// the line past lw
		if avail := lw - metaW - name.w - runeLen(lbl) - 1; avail >= 1 {
			q := truncate(u.filter, avail)
			fs = cyan + lbl + reset + bold + q
			if cur != "" {
				fs += cyan + cur + reset + bold
			}
			fw = runeLen(lbl) + runeLen(q) + runeLen(cur)
		}
	}
	meta := cl.padStart(9) + " " + added.padStart(9) + " " + nar.padStart(9) + " " + deps.padStart(depsW) + " "
	if fw == 0 {
		return bold + name.pad(lw-metaW) + meta + reset
	}
	pad := lw - metaW - fw - name.w
	return bold + name.s + fs + strings.Repeat(" ", pad) + meta + reset
}

// headerRow is the header line between the frame bars
func (u *UI) headerRow(lw int, hang bool) string {
	indent := 1
	if u.tree.Hidden {
		indent = 5
	}
	var hdr string
	if hang {
		pad := indent - 3
		if pad < 0 {
			pad = 0
		}
		hdr = u.frameColor() + "  │" + reset + strings.Repeat(" ", pad) + u.headerLine(lw-3-pad)
	} else {
		hdr = strings.Repeat(" ", indent) + u.headerLine(lw-indent)
	}
	return u.frameColor() + "│" + reset + hdr + u.frameColor() + "│" + reset
}

// sticky returns the ancestor rows of the selection that sit above the
// viewport offset and must remain visible.
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

// rowCol2 is the tree glyph a row draws in the continuation column next
// to the frame: ├/└ for children, │ for pass-through prefixes, or a
// blank for rows outside any branch (the tree root).
func rowCol2(r Row) rune {
	lead := []rune(r.Prefix + r.Conn)
	if len(lead) > 1 {
		return lead[1]
	}
	return ' '
}

// topHang reports whether the topmost tree line in the viewport is cut
// off from its parents and must be re-attached to the top frame. That
// happens when the line glyph of a row receives from above but nothing
// above it provides one: rows deeper than the visible root chain and
// all rows of the rootless forest.
func (u *UI) topHang(sticky []Row) bool {
	provided := false
	for rowIdx := 0; rowIdx < u.viewH(); rowIdx++ {
		var row Row
		if rowIdx < len(sticky) {
			row = sticky[rowIdx]
		} else if idx := u.offset + rowIdx - len(sticky); idx < len(u.rows) {
			row = u.rows[idx]
		} else {
			return false
		}
		switch c := rowCol2(row); c {
		case '│', '├', '└':
			if !provided && (u.tree.Hidden || runeLen(row.Prefix)/4+1 > 1) {
				return true
			}
			provided = c != '└'
		case '┌':
			provided = true
		default:
			provided = false
		}
	}
	return false
}

// rowLine renders a single tree row.
func (u *UI) rowLine(row Row, lw int) string {
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
	cl := HumanSize(u.g.Closure(row.Node.Path).Bytes)
	added := HumanSize(u.g.Added(row.Node.Path))
	nar := HumanSize(info.NarSize)
	_, depsW := u.depsColumn()
	deps := strconv.Itoa(u.depsCount(row.Node.Path))
	meta := fmt.Sprintf("%9s %9s %9s %*s ", cl, added, nar, depsW, deps)
	nameW := lw - runeLen(meta)
	if nameW < 1 {
		return padEnd(truncate(name, lw), lw)
	}
	if row.Node == u.sel {
		return rev + padEnd(name, nameW) + meta + reset
	}
	structCol := dim
	markColor := structCol
	switch m {
	case "[+] ":
		markColor = cyan
	case "[-] ":
		markColor = blue
	}
	if u.reverse {
		structCol = orange
		markColor = orange
	}
	body := padEnd(ShortName(row.Node.Path), nameW-runeLen(lead)-runeLen(m))
	return structCol + lead + reset + markColor + m + reset + body +
		cyan + fmt.Sprintf("%9s", cl) + reset + " " +
		green + fmt.Sprintf("%9s", added) + reset + " " +
		blue + fmt.Sprintf("%9s", nar) + reset + " " +
		yell + fmt.Sprintf("%*s", depsW, deps) + reset + " "
}

// statusTab draws the ┘ tail of the status line to line up it with the last row.
func (u *UI) statusTab() string {
	rowIdx := u.viewH() - 1
	if rowIdx < 0 {
		return "───"
	}
	st := u.sticky(u.offset)
	var row *Row
	if rowIdx < len(st) {
		row = &st[rowIdx]
	} else if idx := u.offset + rowIdx - len(st); idx >= 0 && idx < len(u.rows) {
		row = &u.rows[idx]
	}
	if row == nil {
		return "───"
	}
	if strings.ContainsRune(row.Conn, '├') || strings.ContainsRune(row.Prefix, '│') {
		return "──┘"
	}
	return "───"
}

func (u *UI) statusLine() string {
	fc := u.frameColor()
	wt := bold + white
	tab := fc + "└" + u.statusTab() + reset
	dashes := func(n int) string {
		if n < 1 {
			return ""
		}
		return fc + strings.Repeat("─", n) + reset
	}
	helpW := " ? Help "
	help := bold + " " + cyan + "?" + white + " Help " + reset
	cells := []string{
		" " + fmt.Sprintf("%d paths", u.g.Size()) + " ",
		" closure " + HumanSize(u.g.Closure(u.g.Root).Bytes) + " ",
	}
	if u.flash != "" {
		cells = append(cells, " "+u.flash+" ")
	}
	for n := len(cells); n > 0; n-- {
		cs := cells[:n]
		sum := 0
		for _, c := range cs {
			sum += runeLen(c)
		}
		fill := u.w - sum - runeLen(helpW) - 4*n - 5
		if fill < 2 {
			continue
		}
		line := tab + wt + cs[0] + reset
		for i, c := range cs[1:] {
			// the last cell is the flash only when it made the cut
			if u.flash != "" && n == len(cells) && i == len(cs)-2 {
				line += dashes(4) + bold + cyan + c + reset
				continue
			}
			line += dashes(4) + wt + c + reset
		}
		return line + dashes(fill) + help + dashes(4) + fc + "┘" + reset
	}
	if fill := u.w - runeLen(helpW) - 9; fill >= 2 {
		return tab + dashes(fill) + help + dashes(4) + fc + "┘" + reset
	}
	return fc + "└" + reset + padEnd(helpW, u.w-2) + fc + "┘" + reset
}

func (u *UI) helpOverlay() string {
	rows := [][2]string{
		{"j/k, ↑/↓", "move selection"},
		{"space, enter", "expand/collapse"},
		{"h/l , ←/→", "collapse/drill down"},
		{"e/E", "expand/collapse all"},
		{"g/G", "jump to top/bottom"},
		{"pgup/pgdn", "scroll by page"},
		{"c/a/z/d/n", "sort mode switch"},
		{"f", "filter by name"},
		{"y", "copy hash/path/name"},
		{"s", "shell in selected path"},
		{"p", "flip tree at selected node"},
		{"P", "all packages with dependents"},
		{"esc", "exit inverted view/filter"},
		{"?", "toggle this help"},
		{"q", "quit"},
	}
	return u.overlay("Help", rows, "press any key to close")
}

func (u *UI) copyOverlay() string {
	p := u.sel.Path
	rows := [][2]string{
		{"h  hash", Hash(p)},
		{"p  path", p},
		{"n  name", PkgName(p)},
	}
	return u.overlay("Copy", rows, "h/p/n copies, any key closes")
}

func (u *UI) overlay(title string, rows [][2]string, hint string) string {
	kw, maxw := 0, 0
	for _, r := range rows {
		if w := runeLen(r[0]); w > kw {
			kw = w
		}
	}
	for _, r := range rows {
		if w := kw + 2 + runeLen(r[1]); w > maxw {
			maxw = w
		}
	}
	if w := runeLen(hint); w > maxw {
		maxw = w
	}
	if w := runeLen(title) + 6; w > maxw {
		maxw = w
	}
	if avail := u.w - 4; maxw > avail {
		maxw = avail
	}
	bw := maxw + 4
	bh := len(rows) + 4
	row := (u.h - bh) / 2
	col := (u.w - bw) / 2
	if row < 1 {
		row = 1
	}
	if col < 1 {
		col = 1
	}
	dash := func(n int) string {
		if n < 0 {
			n = 0
		}
		return strings.Repeat("─", n)
	}
	var b strings.Builder
	line := func(r int, s string) {
		fmt.Fprintf(&b, "\x1b[%d;%dH%s%s", row+r, col, s, reset)
	}
	tw := runeLen(title) + 2
	side := (bw - 2 - tw) / 2
	if side < 0 {
		side = 0
	}
	top := dim + "┌" + dash(side) + reset + " " + bold + title + reset + " " +
		dim + dash(bw-2-tw-side) + "┐" + reset
	line(0, top)
	for i, r := range rows {
		line(i+1, dim+"│ "+reset+cyan+padEnd(r[0], kw)+reset+"  "+
			padEnd(r[1], maxw-kw-2)+dim+" │"+reset)
	}
	blank := dim + "│ " + reset + strings.Repeat(" ", maxw) + dim + " │" + reset
	line(len(rows)+1, blank)
	pad := (maxw - runeLen(hint)) / 2
	if pad < 0 {
		pad = 0
	}
	h := strings.Repeat(" ", pad) + hint + strings.Repeat(" ", maxw-pad-runeLen(hint))
	line(len(rows)+2, dim+"│ "+reset+dim+h+reset+dim+" │"+reset)
	line(len(rows)+3, dim+"└"+dash(bw-2)+"┘"+reset)
	return b.String()
}

func makeRaw(fd int) (*syscall.Termios, error) {
	var old syscall.Termios
	if err := ioctl(fd, syscall.TCGETS, unsafe.Pointer(&old)); err != nil {
		return nil, err
	}
	raw := old
	raw.Iflag &^= syscall.IGNBRK | syscall.BRKINT | syscall.PARMRK | syscall.ISTRIP |
		syscall.INLCR | syscall.IGNCR | syscall.ICRNL | syscall.IXON
	raw.Oflag &^= syscall.OPOST
	raw.Lflag &^= syscall.ECHO | syscall.ECHONL | syscall.ICANON | syscall.IEXTEN
	raw.Cflag &^= syscall.CSIZE | syscall.PARENB
	raw.Cflag |= syscall.CS8
	raw.Cc[syscall.VMIN] = 1
	raw.Cc[syscall.VTIME] = 0
	if err := ioctl(fd, syscall.TCSETS, unsafe.Pointer(&raw)); err != nil {
		return nil, err
	}
	return &old, nil
}

func restoreTerm(fd int, old *syscall.Termios) {
	if old != nil {
		_ = ioctl(fd, syscall.TCSETS, unsafe.Pointer(old))
	}
}

func ioctl(fd int, req uint, arg unsafe.Pointer) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), uintptr(req), uintptr(arg))
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

// termSizeOr returns the terminal size, falling back to w and h when the
// size is unavailable.
func termSizeOr(w, h int) (int, int) {
	if tw, th, err := termSize(); err == nil && tw > 0 && th > 0 {
		return tw, th
	}
	return w, h
}
