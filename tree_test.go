package main

import (
	"strings"
	"testing"
)

func TestTreeExpandAndPrune(t *testing.T) {
	g := testGraph()
	root := NewTree(g)
	if !root.Expanded || !root.Loaded {
		t.Fatal("root should start expanded")
	}
	if len(root.Children) != 2 {
		t.Fatalf("root children = %d, want 2", len(root.Children))
	}
	if root.Children[0].Path != "/s/big" {
		t.Errorf("children not sorted by size: first = %s", root.Children[0].Path)
	}
	if root.Marker(g) != "[-]" {
		t.Errorf("expanded marker = %q, want [-]", root.Marker(g))
	}

	big := root.Children[0]
	big.Toggle(g)
	if len(big.Children) != 0 {
		t.Errorf("cyclic ref not pruned: %d children", len(big.Children))
	}
	if big.Marker(g) != "" {
		t.Errorf("leaf marker = %q, want empty", big.Marker(g))
	}
}

func TestTreeCollapse(t *testing.T) {
	g := testGraph()
	root := NewTree(g)
	root.Toggle(g)
	if root.Expanded {
		t.Fatal("root should be collapsed after toggle")
	}
	if root.Marker(g) != "[+]" {
		t.Errorf("collapsed marker = %q, want [+]", root.Marker(g))
	}
	if rows := root.Visible(); len(rows) != 1 {
		t.Errorf("collapsed visible rows = %d, want 1", len(rows))
	}
}

func TestVisiblePrefixes(t *testing.T) {
	g := testGraph()
	root := NewTree(g)
	root.Children[0].Toggle(g)
	rows := root.Visible()
	if len(rows) != 3 {
		t.Fatalf("visible rows = %d, want 3", len(rows))
	}
	if rows[0].Conn != "" || rows[0].Prefix != "" {
		t.Errorf("root row = %q/%q, want empty prefix and conn", rows[0].Prefix, rows[0].Conn)
	}
	if rows[1].Prefix != "" {
		t.Errorf("first child prefix = %q, want empty (no root bar)", rows[1].Prefix)
	}
	if rows[2].Prefix != "" {
		t.Errorf("last child prefix = %q, want empty (no root bar)", rows[2].Prefix)
	}
	last := len(root.Children) - 1
	for i := range root.Children {
		wantConn := " ├─ "
		if i == last {
			wantConn = " └─ "
		}
		if rows[i+1].Conn != wantConn {
			t.Errorf("conn = %q, want %q", rows[i+1].Conn, wantConn)
		}
	}
}

func TestBarUnderLastCorner(t *testing.T) {
	g := testGraph()
	g.info["/s/big/deep"] = &Info{NarSize: 1}
	g.info["/s/big/deep2"] = &Info{NarSize: 2}
	g.info["/s/small/kid"] = &Info{NarSize: 3}
	g.info["/s/small/kid/grand"] = &Info{NarSize: 4}
	g.info["/s/big"].References = []string{"/s/big/deep", "/s/big/deep2"}
	g.info["/s/small"].References = []string{"/s/small/kid"}
	g.info["/s/small/kid"].References = []string{"/s/small/kid/grand"}
	g.countDirect()

	root := NewTree(g)
	big, small := root.Children[0], root.Children[1]
	big.Toggle(g)
	small.Toggle(g)
	small.Children[0].Toggle(g)
	rows := root.Visible()

	prefixOf := func(n *Node) string {
		for _, r := range rows {
			if r.Node == n {
				return r.Prefix
			}
		}
		return "<missing>"
	}

	for _, c := range big.Children {
		if p := prefixOf(c); p != " │  " {
			t.Errorf("child of non-last node prefix = %q, want %q", p, " │  ")
		}
	}
	if p := prefixOf(small.Children[0]); p != "    " {
		t.Errorf("child of last node prefix = %q, want spaces", p)
	}
	if p := prefixOf(small.Children[0].Children[0]); p != "        " {
		t.Errorf("grandchild under last node prefix = %q, want spaces", p)
	}
}

func TestStickyPath(t *testing.T) {
	g := testGraph()
	g.info["/s/small/kid"] = &Info{NarSize: 3}
	g.info["/s/small/kid/grand"] = &Info{NarSize: 4}
	g.info["/s/small"].References = []string{"/s/small", "/s/small/kid"}
	g.info["/s/small/kid"].References = []string{"/s/small/kid", "/s/small/kid/grand"}
	g.countDirect()

	root := NewTree(g)
	small := root.Children[1]
	small.Toggle(g)
	small.Children[0].Toggle(g)
	rows := root.Visible()
	// order: root(0), big(1), small(2), kid(3), grand(4)
	u := &UI{g: g, tree: root, rows: rows, sel: rows[len(rows)-1].Node}

	if got := u.sticky(0); len(got) != 0 {
		t.Errorf("sticky(0) = %d rows, want 0", len(got))
	}
	if got := u.sticky(1); len(got) != 1 || got[0].Node != root {
		t.Errorf("sticky(1) = %d rows, want [root]", len(got))
	}
	if got := u.sticky(3); len(got) != 2 || got[0].Node != root || got[1].Node != small {
		t.Errorf("sticky(3) = %d rows, want [root small]", len(got))
	}
	if got := u.sticky(len(rows)); len(got) != 3 {
		t.Errorf("sticky(all) = %d rows, want [root small kid]", len(got))
	}
}

func TestUISort(t *testing.T) {
	g := testGraph()
	g.info["/s/zzz"] = &Info{NarSize: 300}
	g.info["/s/mmm"] = &Info{NarSize: 200}
	g.info["/s/aaa"] = &Info{NarSize: 100}
	g.info["/s/root"].References = []string{"/s/big", "/s/small", "/s/zzz", "/s/mmm", "/s/aaa"}
	g.info["/s/aaa"].References = []string{"/s/mmm", "/s/zzz"}
	g.info["/s/aaa"].Direct = 3
	g.countDirect()

	u := NewUI(g)
	paths := func() []string {
		out := make([]string, 0, len(u.tree.Children))
		for _, c := range u.tree.Children {
			out = append(out, c.Path)
		}
		return out
	}
	eq := func(got, want []string) bool {
		if len(got) != len(want) {
			return false
		}
		for i := range want {
			if got[i] != want[i] {
				return false
			}
		}
		return true
	}

	if u.sortKey != sortOwn || !u.sortDesc {
		t.Fatalf("initial sort state = %d/%v, want sortOwn/desc", u.sortKey, u.sortDesc)
	}
	natural := []string{"/s/big", "/s/zzz", "/s/mmm", "/s/aaa", "/s/small"}
	if got := paths(); !eq(got, natural) {
		t.Errorf("initial order = %v, want %v", got, natural)
	}

	u.setSort(sortName)
	if got := paths(); !eq(got, []string{"/s/zzz", "/s/small", "/s/mmm", "/s/big", "/s/aaa"}) {
		t.Errorf("name desc = %v", got)
	}
	u.setSort(sortName)
	if got := paths(); !eq(got, []string{"/s/aaa", "/s/big", "/s/mmm", "/s/small", "/s/zzz"}) {
		t.Errorf("name asc = %v", got)
	}
	u.setSort(sortName)
	if u.sortKey != sortNone {
		t.Errorf("third press: sortKey = %d, want sortNone", u.sortKey)
	}
	if got := paths(); !eq(got, natural) {
		t.Errorf("off order = %v, want %v", got, natural)
	}

	u.setSort(sortDeps)
	if got := paths(); !eq(got, []string{"/s/aaa", "/s/big", "/s/mmm", "/s/small", "/s/zzz"}) {
		t.Errorf("deps desc = %v", got)
	}
	u.setSort(sortDeps)
	if got := paths(); !eq(got, []string{"/s/mmm", "/s/small", "/s/zzz", "/s/big", "/s/aaa"}) {
		t.Errorf("deps asc = %v", got)
	}

	u.setSort(sortOwn)
	if u.sortKey != sortOwn || !u.sortDesc {
		t.Errorf("switching key resets to desc, got %d/%v", u.sortKey, u.sortDesc)
	}
	u.setSort(sortOwn)
	if u.sortDesc || u.sortKey != sortOwn {
		t.Errorf("own cycle state = %d/%v, want sortOwn/asc", u.sortKey, u.sortDesc)
	}

	u.setSort(sortClosure)
	if got := paths(); !eq(got, []string{"/s/big", "/s/aaa", "/s/zzz", "/s/mmm", "/s/small"}) {
		t.Errorf("closure desc = %v", got)
	}
	u.setSort(sortClosure)
	if got := paths(); !eq(got, []string{"/s/small", "/s/mmm", "/s/zzz", "/s/aaa", "/s/big"}) {
		t.Errorf("closure asc = %v", got)
	}
}

func rowPaths(rows []Row) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Node.Path)
	}
	return out
}

func eqPaths(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestFilterRows(t *testing.T) {
	g := testGraph()
	g.info["/s/small/kid"] = &Info{NarSize: 3}
	g.info["/s/small/kid2"] = &Info{NarSize: 4}
	g.info["/s/small"].References = []string{"/s/small", "/s/small/kid", "/s/small/kid2"}
	g.countDirect()

	root := NewTree(g)
	root.Children[1].Toggle(g)
	match := func(n *Node) bool { return strings.Contains(PkgName(n.Path), "kid") }

	rows := root.visibleRows(match)
	want := []string{"/s/root", "/s/small", "/s/small/kid2", "/s/small/kid"}
	if got := rowPaths(rows); !eqPaths(got, want) {
		t.Fatalf("filtered rows = %v, want %v", got, want)
	}
	if rows[3].Conn != " └─ " {
		t.Errorf("conn after filter = %q, want last-corner", rows[3].Conn)
	}
	if got := rowPaths(root.visibleRows(nil)); len(got) != 5 {
		t.Errorf("unfiltered rows = %v, want 5", got)
	}
}

func TestUIFilter(t *testing.T) {
	g := testGraph()
	g.info["/s/small/kid"] = &Info{NarSize: 3}
	g.info["/s/small"].References = []string{"/s/small", "/s/small/kid"}
	g.countDirect()

	u := NewUI(g)
	if u.matcher() != nil {
		t.Fatal("matcher should be nil without filter")
	}
	u.tree.Children[1].Toggle(g)
	unfiltered := []string{"/s/root", "/s/big", "/s/small", "/s/small/kid"}

	u.handle([]byte("f"))
	if !u.filterMode {
		t.Fatal("f should enter filter mode")
	}
	u.handle([]byte("kid"))
	if u.filter != "kid" || !u.filterMode {
		t.Fatalf("typing broken: %q mode=%v", u.filter, u.filterMode)
	}
	if got := rowPaths(u.rows); !eqPaths(got, []string{"/s/root", "/s/small", "/s/small/kid"}) {
		t.Errorf("live filtered rows = %v", got)
	}
	if h := stripANSI(u.headerLine(60)); !strings.Contains(h, "filter:kid▌") {
		t.Errorf("header typing = %q", h)
	}

	u.handle([]byte("q"))
	if u.filter != "kidq" {
		t.Errorf("q in filter mode = %q, want typed", u.filter)
	}
	u.handle([]byte{0x7f})
	if u.filter != "kid" {
		t.Errorf("backspace = %q, want kid", u.filter)
	}

	u.handle([]byte{'\r'})
	if u.filterMode || u.filter != "kid" {
		t.Errorf("enter lock state = mode=%v filter=%q", u.filterMode, u.filter)
	}
	if got := rowPaths(u.rows); !eqPaths(got, []string{"/s/root", "/s/small", "/s/small/kid"}) {
		t.Errorf("locked rows = %v", got)
	}
	if h := stripANSI(u.headerLine(60)); !strings.Contains(h, "filter:kid") || strings.Contains(h, "▌") {
		t.Errorf("header locked = %q", h)
	}

	u.handle([]byte("f"))
	if !u.filterMode || u.filter != "kid" {
		t.Errorf("re-enter = mode=%v filter=%q, want editing kept text", u.filterMode, u.filter)
	}
	if u.handle([]byte{0x1b}) {
		t.Error("esc in filter mode must not quit")
	}
	if u.filterMode || u.filter != "" {
		t.Errorf("esc clear = mode=%v filter=%q", u.filterMode, u.filter)
	}
	if got := rowPaths(u.rows); !eqPaths(got, unfiltered) {
		t.Errorf("cleared rows = %v, want %v", got, unfiltered)
	}
}

func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\x1b' {
			for i < len(s) && s[i] != 'm' {
				i++
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func TestLeftLineWidth(t *testing.T) {
	g := testGraph()
	u := NewUI(g)
	rows := u.tree.Visible()
	const lw = 43
	sel := runeLen(stripANSI(u.leftLine(rows[1], lw)))
	other := runeLen(stripANSI(u.leftLine(rows[2], lw)))
	if sel != lw || other != lw {
		t.Errorf("row widths sel=%d other=%d, want %d/%d", sel, other, lw, lw)
	}
	header := runeLen(stripANSI(u.headerLine(lw)))
	if header != lw {
		t.Errorf("header width = %d, want %d", header, lw)
	}
}

func TestHeaderLabels(t *testing.T) {
	g := testGraph()
	u := NewUI(g)
	u.sortKey = sortNone
	h := stripANSI(u.headerLine(43))
	if !strings.HasSuffix(h, "  CLOSURE      OWN   DEPS") {
		t.Errorf("header = %q", h)
	}
	u.setSort(sortClosure)
	h = stripANSI(u.headerLine(43))
	if !strings.Contains(h, "↓CLOSURE") {
		t.Errorf("header with arrow = %q", h)
	}
	u.setSort(sortDeps)
	h = stripANSI(u.headerLine(43))
	if !strings.Contains(h, "↓DEPS") {
		t.Errorf("header with deps arrow = %q", h)
	}
}

func TestSortKeepsCursor(t *testing.T) {
	g := testGraph()
	g.info["/s/zzz"] = &Info{NarSize: 300}
	g.info["/s/mmm"] = &Info{NarSize: 200}
	g.info["/s/aaa"] = &Info{NarSize: 100}
	g.info["/s/root"].References = []string{"/s/big", "/s/small", "/s/zzz", "/s/mmm", "/s/aaa"}
	g.countDirect()

	u := NewUI(g)
	u.rows = u.tree.Visible()
	u.sel = u.rows[5].Node // natural order row 5 = /s/small
	u.setSort(sortName)
	if u.rows[5].Node.Path != "/s/aaa" {
		t.Fatalf("row 5 after name sort = %s, want /s/aaa", u.rows[5].Node.Path)
	}
	if u.sel != u.rows[5].Node {
		t.Errorf("sel = %s, want cursor to stay on row 5 (%s)", u.sel.Path, u.rows[5].Node.Path)
	}
	if u.sel.Path != "/s/aaa" {
		t.Errorf("sel = %s, want /s/aaa", u.sel.Path)
	}
}
