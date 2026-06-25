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
	yell   = "\x1b[33m"
	rev    = "\x1b[7m"
	white  = "\x1b[97m"
	orange = "\x1b[38;5;208m"
)

const (
	sortNone = iota
	sortOwn
	sortDeps
	sortName
	sortClosure
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
	reverse    bool
	forest     bool
}

func NewUI(g *Graph) *UI {
	tree := NewTree(g)
	return &UI{
		g:         g,
		tree:      tree,
		sel:       tree,
		w:         80,
		h:         24,
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

func (u *UI) matcher() func(*Node) bool {
	if u.filter == "" {
		return nil
	}
	q := strings.ToLower(u.filter)
	return func(n *Node) bool {
		return strings.Contains(strings.ToLower(PkgName(n.Path)), q)
	}
}

func (u *UI) handle(buf []byte) bool {
	if u.helpMode {
		u.helpMode = false
		for i := 0; i < len(buf); i++ {
			if buf[i] == 3 {
				return true
			}
		}
		return false
	}
	u.rows = u.tree.visibleRows(u.matcher())
	for i := 0; i < len(buf); i++ {
		if u.filterMode {
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
			continue
		}
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
		case b == 'f' || b == 'F':
			u.filterMode = true
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
	u.rows = u.tree.visibleRows(u.matcher())
	return false
}

// setMode switches between the forward tree (reverse=false), the p-mode
// tree rooted at the selected node (p) and the all-packages forest (P).
// In p-mode every p press flips the tree direction at the current
// selection (dependents <-> dependencies); P or esc turns the mode off.
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

func (u *UI) rebuild() {
	u.offset = 0
	u.rows = u.tree.visibleRows(u.matcher())
	if len(u.rows) > 0 {
		u.sel = u.rows[0].Node
	}
	u.resort()
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
	if h := u.h - 3; h > 0 {
		return h
	}
	return 1
}

func (u *UI) frameCol() string {
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
		da, db := u.depsCount(a.Path), u.depsCount(b.Path)
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

// depsCol is the last meta column: dependencies in the forward graph,
// dependents in the inverted one. The width leaves room for the sort arrow.
func (u *UI) depsCol() (string, int) {
	if u.g.Reverse {
		return "DEPENDENTS", 11
	}
	return "DEPENDENCIES", 13
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

func infoSize(i *Info) uint64 {
	if i == nil {
		return 0
	}
	return i.NarSize
}

func (u *UI) render() {
	u.rows = u.tree.visibleRows(u.matcher())
	if u.indexOf(u.sel) < 0 {
		if len(u.rows) > 0 {
			u.sel = u.rows[0].Node
		} else {
			u.sel = u.tree
		}
	}
	lw := u.w - 2
	if lw < 1 {
		lw = 1
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
	var b strings.Builder
	if u.clearNext {
		b.WriteString(clearScr)
		u.clearNext = false
	} else {
		b.WriteString("\x1b[H")
	}
	fc := u.frameCol()
	for r := 0; r < u.h; r++ {
		var left string
		switch {
		case r == 0:
			left = u.topBorder()
		case r == 1:
			left = fc + "│" + reset + u.headerLine(lw) + fc + "│" + reset
		case r == u.h-1:
			left = u.statusLine()
		default:
			var line string
			switch rowIdx := r - 2; {
			case rowIdx < len(sticky):
				line = u.leftLine(sticky[rowIdx], lw)
			default:
				if idx := u.offset + rowIdx - len(sticky); idx < len(u.rows) {
					line = u.leftLine(u.rows[idx], lw)
				} else {
					line = padEnd("", lw)
				}
			}
			left = fc + "│" + reset + line + fc + "│" + reset
		}
		b.WriteString(left)
		if r < u.h-1 {
			b.WriteString("\r\n")
		}
	}
	if u.helpMode {
		b.WriteString(u.helpOverlay())
	}
	os.Stdout.WriteString(b.String())
}

func (u *UI) topBorder() string {
	title := " Dependency graph"
	if u.g.Reverse {
		title = " Dependents graph"
	}
	color := white
	if u.reverse {
		color = orange
	}
	if pad := u.w - runeLen(title) - 4; pad < 0 {
		title = truncate(title, u.w-4)
	}
	pad := u.w - runeLen(title) - 4
	if pad < 0 {
		pad = 0
	}
	var tp string
	if i := strings.Index(title, "p"); i >= 0 {
		tp = color + title[:i] + cyan + "p" + color + title[i+1:]
	} else {
		tp = color + title
	}
	return u.frameCol() + "┌─" + reset + tp + reset + u.frameCol() + " " +
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
	depsLbl, depsW := u.depsCol()
	metaW := 9 + 1 + 8 + 1 + depsW
	cl := u.colLabel("CLOSURE", "C", sortClosure)
	own := u.colLabel("OWN", "O", sortOwn)
	deps := u.colLabel(depsLbl, "D", sortDeps)
	name := u.colLabel("NAME", "N", sortName)
	if lw < name.w+metaW+1 {
		return bold + padEnd(truncate("nixview", lw), lw) + reset
	}
	fw, fs := 0, ""
	if u.filterMode || u.filter != "" {
		const lbl = " filter:"
		cur := ""
		if u.filterMode {
			cur = "▌"
		}
		avail := lw - metaW - name.w - runeLen(lbl) - 1
		if avail < 1 {
			avail = 1
		}
		q := truncate(u.filter, avail)
		fs = cyan + lbl + reset + bold + q
		if cur != "" {
			fs += cyan + cur + reset + bold
		}
		fw = runeLen(lbl) + runeLen(q) + runeLen(cur)
	}
	meta := cl.padStart(9) + " " + own.padStart(8) + " " + deps.padStart(depsW)
	if fw == 0 {
		return bold + name.pad(lw-metaW) + meta + reset
	}
	pad := lw - metaW - fw - name.w
	if pad < 1 {
		pad = 1
	}
	return bold + name.s + fs + strings.Repeat(" ", pad) + meta + reset
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
	_, depsW := u.depsCol()
	deps := strconv.Itoa(u.depsCount(row.Node.Path))
	meta := fmt.Sprintf("%9s %8s %*s", cl, own, depsW, deps)
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
		blue + fmt.Sprintf("%8s", own) + reset + " " +
		yell + fmt.Sprintf("%*s", depsW, deps) + reset
}

func (u *UI) statusTab() string {
	rowIdx := u.viewH() - 1
	if rowIdx < 0 {
		return "──"
	}
	st := u.sticky(u.offset)
	var row *Row
	if rowIdx < len(st) {
		row = &st[rowIdx]
	} else if idx := u.offset + rowIdx - len(st); idx >= 0 && idx < len(u.rows) {
		row = &u.rows[idx]
	}
	if row == nil {
		return "──"
	}
	lead := []rune(row.Prefix + row.Conn)
	if len(lead) > 1 && (lead[1] == '├' || lead[1] == '│') {
		return "─┘"
	}
	return "──"
}

func (u *UI) statusLine() string {
	fc := u.frameCol()
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
	for n := len(cells); n > 0; n-- {
		cs := cells[:n]
		sum := 0
		for _, c := range cs {
			sum += runeLen(c)
		}
		fill := u.w - sum - runeLen(helpW) - 4*n - 4
		if fill < 2 {
			continue
		}
		line := tab + wt + cs[0] + reset
		for _, c := range cs[1:] {
			line += dashes(4) + wt + c + reset
		}
		return line + dashes(fill) + help + dashes(4) + fc + "┘" + reset
	}
	if fill := u.w - runeLen(helpW) - 8; fill >= 2 {
		return tab + dashes(fill) + help + dashes(4) + fc + "┘" + reset
	}
	return fc + "└" + reset + padEnd(truncate(helpW, u.w-2), u.w-2) + fc + "┘" + reset
}

func (u *UI) helpOverlay() string {
	rows := [][2]string{
		{"j/k, ↑/↓", "move selection"},
		{"space, enter", "expand / collapse"},
		{"h / l", "collapse / drill down"},
		{"g / G", "jump to top / bottom"},
		{"pgup / pgdn", "scroll by page"},
		{"o / d / n / c", "sort by own / deps / name / closure"},
		{"f", "filter by name"},
		{"p", "flip tree at selected node"},
		{"P", "all packages with dependents"},
		{"esc", "exit inverted view / filter"},
		{"?", "toggle this help"},
		{"q", "quit"},
	}
	const title = "Help"
	hint := "press any key to close"
	kw, maxw := 0, 0
	for _, r := range rows {
		if w := runeLen(r[0]); w > kw {
			kw = w
		}
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
	if cap := u.w - 4; maxw > cap {
		maxw = cap
	}
	if maxw < 1 {
		maxw = 1
	}
	bw := maxw + 4
	bh := len(rows) + 4
	row := (u.h - bh) / 2
	col := (u.w - bw) / 2
	if row > u.h-1-bh {
		row = u.h - 1 - bh
	}
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
