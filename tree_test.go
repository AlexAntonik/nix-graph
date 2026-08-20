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
	if rows := root.visibleRows(nil); len(rows) != 1 {
		t.Errorf("collapsed visible rows = %d, want 1", len(rows))
	}
}

func TestVisiblePrefixes(t *testing.T) {
	g := testGraph()
	root := NewTree(g)
	root.Children[0].Toggle(g)
	rows := root.visibleRows(nil)
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
	rows := root.visibleRows(nil)

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
	rows := root.visibleRows(nil)
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

	if u.sortKey != sortNar || !u.sortDesc {
		t.Fatalf("initial sort state = %d/%v, want sortNar/desc", u.sortKey, u.sortDesc)
	}
	natural := []string{"/s/big", "/s/zzz", "/s/mmm", "/s/aaa", "/s/small"}
	if got := paths(); !eqStrs(got, natural) {
		t.Errorf("initial order = %v, want %v", got, natural)
	}

	u.setSort(sortName)
	if got := paths(); !eqStrs(got, []string{"/s/zzz", "/s/small", "/s/mmm", "/s/big", "/s/aaa"}) {
		t.Errorf("name desc = %v", got)
	}
	u.setSort(sortName)
	if got := paths(); !eqStrs(got, []string{"/s/aaa", "/s/big", "/s/mmm", "/s/small", "/s/zzz"}) {
		t.Errorf("name asc = %v", got)
	}
	u.setSort(sortName)
	if u.sortKey != sortNone {
		t.Errorf("third press: sortKey = %d, want sortNone", u.sortKey)
	}
	if got := paths(); !eqStrs(got, natural) {
		t.Errorf("off order = %v, want %v", got, natural)
	}

	u.setSort(sortDeps)
	if got := paths(); !eqStrs(got, []string{"/s/big", "/s/aaa", "/s/mmm", "/s/small", "/s/zzz"}) {
		t.Errorf("deps desc = %v", got)
	}
	u.setSort(sortDeps)
	if got := paths(); !eqStrs(got, []string{"/s/mmm", "/s/small", "/s/zzz", "/s/aaa", "/s/big"}) {
		t.Errorf("deps asc = %v", got)
	}

	u.setSort(sortNar)
	if u.sortKey != sortNar || !u.sortDesc {
		t.Errorf("switching key resets to desc, got %d/%v", u.sortKey, u.sortDesc)
	}
	u.setSort(sortNar)
	if u.sortDesc || u.sortKey != sortNar {
		t.Errorf("nar cycle state = %d/%v, want sortNar/asc", u.sortKey, u.sortDesc)
	}

	u.setSort(sortClosure)
	if got := paths(); !eqStrs(got, []string{"/s/big", "/s/aaa", "/s/zzz", "/s/mmm", "/s/small"}) {
		t.Errorf("closure desc = %v", got)
	}
	u.setSort(sortClosure)
	if got := paths(); !eqStrs(got, []string{"/s/small", "/s/mmm", "/s/zzz", "/s/aaa", "/s/big"}) {
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
	if got := rowPaths(rows); !eqStrs(got, want) {
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
	u.rows = u.tree.visibleRows(u.matcher())
	if got := rowPaths(u.rows); !eqStrs(got, []string{"/s/root", "/s/small", "/s/small/kid"}) {
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
	if got := rowPaths(u.rows); !eqStrs(got, []string{"/s/root", "/s/small", "/s/small/kid"}) {
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
	u.rows = u.tree.visibleRows(u.matcher())
	if got := rowPaths(u.rows); !eqStrs(got, unfiltered) {
		t.Errorf("cleared rows = %v, want %v", got, unfiltered)
	}
}

func TestSortKeepsFilter(t *testing.T) {
	g := testGraph()
	g.info["/s/small/kid"] = &Info{NarSize: 3}
	g.info["/s/small/kid2"] = &Info{NarSize: 4}
	g.info["/s/small"].References = []string{"/s/small", "/s/small/kid", "/s/small/kid2"}
	g.countDirect()

	u := NewUI(g)
	u.tree.Children[1].Toggle(g)
	u.handle([]byte("f"))
	u.handle([]byte("kid"))
	u.handle([]byte{'\r'})
	u.rows = u.tree.visibleRows(u.matcher())
	u.handle([]byte("j"))
	u.handle([]byte("j"))
	u.handle([]byte("j"))
	if u.sel.Path != "/s/small/kid" {
		t.Fatalf("sel = %s, want /s/small/kid", u.sel.Path)
	}

	u.handle([]byte("n")) // sort by name while the filter is locked
	want := []string{"/s/root", "/s/small", "/s/small/kid2", "/s/small/kid"}
	if got := rowPaths(u.rows); !eqStrs(got, want) {
		t.Errorf("sorted rows = %v, want filtered rows kept %v", got, want)
	}
	if u.sel != u.rows[3].Node || u.sel.Path != "/s/small/kid" {
		t.Errorf("sel = %s, want cursor to stay on /s/small/kid", u.sel.Path)
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
	rows := u.tree.visibleRows(nil)
	const lw = 60
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
	h := stripANSI(u.headerLine(60))
	if !strings.HasSuffix(h, "  CLOSURE     ADDED  NAR-SIZE  DEPENDENCIES ") {
		t.Errorf("header = %q", h)
	}
	u.setSort(sortClosure)
	h = stripANSI(u.headerLine(60))
	if !strings.Contains(h, "↓CLOSURE") {
		t.Errorf("header with arrow = %q", h)
	}
	u.handle([]byte("a"))
	if u.sortKey != sortAdded || !u.sortDesc {
		t.Fatalf("a key sort state = %d/%v, want sortAdded/desc", u.sortKey, u.sortDesc)
	}
	h = stripANSI(u.headerLine(60))
	if !strings.Contains(h, "↓ADDED") {
		t.Errorf("header with added arrow = %q", h)
	}
	u.setSort(sortDeps)
	h = stripANSI(u.headerLine(60))
	if !strings.Contains(h, "↓DEPENDENCIES") {
		t.Errorf("header with deps arrow = %q", h)
	}
}

func TestAddedColumn(t *testing.T) {
	g := addedGraph()
	u := NewUI(g)
	u.rows = u.tree.visibleRows(nil)
	var row Row
	for _, r := range u.rows {
		if r.Node.Path == "/s/a" {
			row = r
		}
	}
	if row.Node == nil {
		t.Fatal("row /s/a not found")
	}
	// a shows closure 1105 (a+lib+only), added 105 (a+only), nar 100
	line := stripANSI(u.leftLine(row, 60))
	for _, want := range []string{"1.1K", "105B", "100B"} {
		if !strings.Contains(line, want) {
			t.Errorf("line = %q, want %q in it", line, want)
		}
	}
}

func TestToggleReverse(t *testing.T) {
	g := testGraph()
	u := NewUI(g)
	u.rows = u.tree.visibleRows(nil)
	u.handle([]byte("j"))
	u.handle([]byte("p"))
	if !u.reverse || !u.g.Reverse {
		t.Fatal("p should enable reverse mode")
	}
	if u.tree.Path != "/s/big" {
		t.Errorf("reverse tree root = %s, want /s/big", u.tree.Path)
	}
	if h := stripANSI(u.topBorder(false)); !strings.HasPrefix(h, "┌── Dependents graph") {
		t.Errorf("top border = %q, want Dependents graph prefix", h)
	}

	// second p at a new selection flips to the forward tree rooted there
	u.handle([]byte("j"))
	u.handle([]byte("p"))
	if !u.reverse || u.g.Reverse {
		t.Fatal("second p must flip to the forward tree, keeping the mode on")
	}
	if u.tree.Path != "/s/root" {
		t.Errorf("forward tree root = %s, want /s/root", u.tree.Path)
	}
	if u.sel != u.tree || u.sel.Path != "/s/root" {
		t.Errorf("sel = %v, want re-rooted node", u.sel)
	}
	if h := stripANSI(u.topBorder(false)); !strings.HasPrefix(h, "┌── Dependency graph") {
		t.Errorf("top border = %q, want Dependency graph prefix", h)
	}

	// one more press flips back to dependents
	u.handle([]byte("j"))
	u.handle([]byte("p"))
	if !u.reverse || !u.g.Reverse {
		t.Fatal("third p must flip back to dependents")
	}
	if u.tree.Path != "/s/big" {
		t.Errorf("re-inverted tree root = %s, want /s/big", u.tree.Path)
	}

	// P turns the mode off into the forward tree
	u.handle([]byte("P"))
	if u.reverse || u.g.Reverse {
		t.Fatal("P should exit reverse mode")
	}
	if u.tree.Path != g.Root || u.sel != u.tree {
		t.Errorf("forward tree root = %s sel = %v", u.tree.Path, u.sel)
	}
	if h := stripANSI(u.topBorder(false)); !strings.HasPrefix(h, "┌── Dependency graph") {
		t.Errorf("top border = %q, want Dependency graph prefix", h)
	}
	if strings.Contains(stripANSI(u.statusLine()), "p reverse") {
		t.Error("status must not contain p reverse cell")
	}

	// esc also turns the mode off
	u.handle([]byte("p"))
	u.handle([]byte{0x1b})
	if u.reverse || u.g.Reverse {
		t.Fatal("esc should exit reverse mode")
	}
	if u.tree.Path != g.Root {
		t.Errorf("tree after esc = %s, want root", u.tree.Path)
	}
}

func TestToggleReverseAll(t *testing.T) {
	g := testGraph()
	u := NewUI(g)
	u.rows = u.tree.visibleRows(nil)
	u.handle([]byte("P"))
	if !u.reverse || !u.g.Reverse {
		t.Fatal("P should enable reverse-all mode")
	}
	if !u.tree.Hidden {
		t.Fatal("reverse-all tree must have hidden root")
	}
	want := []string{"/s/big", "/s/small", "/s/root"}
	if got := rowPaths(u.rows); !eqStrs(got, want) {
		t.Errorf("top-level rows = %v, want %v", got, want)
	}
	if u.sel != u.rows[0].Node {
		t.Errorf("sel = %v, want first top-level row", u.sel)
	}
	if h := stripANSI(u.topBorder(false)); !strings.HasPrefix(h, "┌── Dependents graph") {
		t.Errorf("top border = %q, want Dependents graph prefix", h)
	}
	// p in the forest roots the inverted tree at the selection
	u.handle([]byte("p"))
	if !u.reverse || !u.g.Reverse || u.tree.Hidden {
		t.Fatal("p in forest should enter p-mode with an inverted tree")
	}
	if u.tree.Path != "/s/big" {
		t.Errorf("tree after p in forest = %s, want /s/big", u.tree.Path)
	}

	u.handle([]byte("P"))
	if u.reverse || u.g.Reverse {
		t.Fatal("P should exit reverse mode")
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
	u.rows = u.tree.visibleRows(nil)
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

func TestDepsColumn(t *testing.T) {
	g := &Graph{
		Root: "/s/root",
		info: map[string]*Info{
			"/s/root":  {NarSize: 10, References: []string{"/s/big", "/s/small"}},
			"/s/big":   {NarSize: 300, References: []string{"/s/root", "/s/mid"}},
			"/s/mid":   {NarSize: 60, References: []string{"/s/leaf"}},
			"/s/leaf":  {NarSize: 5},
			"/s/small": {NarSize: 50, References: []string{"/s/small"}},
		},
	}
	g.countDirect()

	u := NewUI(g)
	rows := u.tree.visibleRows(nil)

	// root directly needs big+small, transitively also mid+leaf
	fwd := stripANSI(u.leftLine(rows[0], 60))
	if !strings.HasSuffix(fwd, "     4 ") {
		t.Errorf("forward deps value = %q, want all deps of root = 4", fwd)
	}
	if w := runeLen(fwd); w != 60 {
		t.Errorf("forward selected row width = %d, want 60", w)
	}
	if w := runeLen(stripANSI(u.leftLine(rows[1], 60))); w != 60 {
		t.Errorf("forward non-selected row width = %d, want 60", w)
	}
	if h := stripANSI(u.headerLine(60)); !strings.HasSuffix(h, "NAR-SIZE  DEPENDENCIES ") {
		t.Errorf("forward header = %q, want DEPENDENCIES", h)
	}
	if w := runeLen(stripANSI(u.headerLine(60))); w != 60 {
		t.Errorf("forward header width = %d, want 60", w)
	}

	u.g.Reverse = true
	// small is directly referenced only by root, but big depends on it too
	if got := stripANSI(u.leftLine(rows[2], 60)); !strings.HasSuffix(got, "     2 ") {
		t.Errorf("reverse deps value = %q, want all dependents of small = 2", got)
	}
	if w := runeLen(stripANSI(u.leftLine(rows[1], 60))); w != 60 {
		t.Errorf("reverse non-selected row width = %d, want 60", w)
	}
	if h := stripANSI(u.headerLine(60)); !strings.Contains(h, "DEPENDENTS") {
		t.Errorf("reverse header = %q, want DEPENDENTS", h)
	}
	if w := runeLen(stripANSI(u.headerLine(60))); w != 60 {
		t.Errorf("reverse header width = %d, want 60", w)
	}

	u.setSort(sortDeps)
	if h := stripANSI(u.headerLine(60)); !strings.Contains(h, "↓DEPENDENTS") {
		t.Errorf("reverse sorted header = %q, want ↓DEPENDENTS", h)
	}
	if w := runeLen(stripANSI(u.headerLine(60))); w != 60 {
		t.Errorf("reverse sorted header width = %d, want 60", w)
	}
}

func TestDepsSortFollowsMode(t *testing.T) {
	g := &Graph{
		Root: "/s/root",
		info: map[string]*Info{
			"/s/root":  {NarSize: 10, References: []string{"/s/big", "/s/small", "/s/zzz", "/s/mmm", "/s/aaa"}},
			"/s/big":   {NarSize: 300, References: []string{"/s/root"}},
			"/s/small": {NarSize: 50, References: []string{"/s/small"}},
			"/s/zzz":   {NarSize: 40, References: []string{"/s/mmm"}},
			"/s/mmm":   {NarSize: 30},
			"/s/aaa":   {NarSize: 20, References: []string{"/s/mmm", "/s/zzz"}},
		},
	}
	g.countDirect()

	u := NewUI(g)
	u.setSort(sortDeps)
	paths := func() []string {
		out := make([]string, 0, len(u.tree.Children))
		for _, c := range u.tree.Children {
			out = append(out, c.Path)
		}
		return out
	}
	// transitively: big 5 (via root), aaa 2, zzz 1, mmm/small 0
	want := []string{"/s/big", "/s/aaa", "/s/zzz", "/s/mmm", "/s/small"}
	if got := paths(); !eqStrs(got, want) {
		t.Errorf("forward deps sort = %v, want %v", got, want)
	}
	// transitively: mmm 4, zzz 3, aaa/small 2, big 1
	u.g.Reverse = true
	u.resort()
	want = []string{"/s/mmm", "/s/zzz", "/s/aaa", "/s/small", "/s/big"}
	if got := paths(); !eqStrs(got, want) {
		t.Errorf("reverse deps sort = %v, want %v", got, want)
	}
}

func TestForestFirstRowCorner(t *testing.T) {
	g := testGraph()
	u := NewUI(g)
	u.rows = u.tree.visibleRows(nil)
	u.handle([]byte("P"))
	rows := u.tree.visibleRows(nil)
	if len(rows) < 2 {
		t.Fatalf("forest rows = %d, want at least 2", len(rows))
	}
	if rows[0].Conn != " ┌─ " {
		t.Errorf("forest first conn = %q, want corner", rows[0].Conn)
	}
	if rows[1].Conn != " ├─ " {
		t.Errorf("forest second conn = %q, want tee", rows[1].Conn)
	}

	// forward tree keeps plain tees: the root is visible
	u.handle([]byte("P"))
	u.rows = u.tree.visibleRows(nil)
	rows = u.tree.visibleRows(nil)
	if len(rows) < 3 {
		t.Fatalf("forward rows = %d, want at least 3", len(rows))
	}
	if rows[1].Conn != " ├─ " {
		t.Errorf("forward first child conn = %q, want tee", rows[1].Conn)
	}
}

func TestStatusTabConnectsAtDepth(t *testing.T) {
	g := testGraph()
	u := NewUI(g)
	u.h = 4
	u.offset = 0
	cases := []struct {
		row  Row
		want string
	}{
		{Row{Node: &Node{Path: "/s/a"}, Prefix: "    ", Conn: " ├─ "}, "──┘"},
		{Row{Node: &Node{Path: "/s/a"}, Prefix: " │   ", Conn: " └─ "}, "──┘"},
		{Row{Node: &Node{Path: "/s/a"}, Conn: " ├─ "}, "──┘"},
		{Row{Node: &Node{Path: "/s/a"}, Prefix: "    ", Conn: " └─ "}, "───"},
		{Row{Node: &Node{Path: "/s/a"}}, "───"},
	}
	for _, c := range cases {
		u.rows = []Row{c.row}
		if got := u.statusTab(); got != c.want {
			t.Errorf("statusTab(prefix=%q conn=%q) = %q, want %q", c.row.Prefix, c.row.Conn, got, c.want)
		}
	}
}

func TestTopHang(t *testing.T) {
	g := testGraph()
	u := NewUI(g)

	// forward tree at the top: root provides for its children
	u.rows = u.tree.visibleRows(nil)
	u.offset = 0
	if u.topHang(nil) {
		t.Error("root with children must not hang")
	}

	// depth-2 orphan below a bare sticky root: hangs
	orphan := Row{Node: &Node{Path: "/s/orphan"}, Prefix: " │  ", Conn: " └─ "}
	u.rows = []Row{{Node: u.tree}, orphan}
	u.offset = 1
	if !u.topHang([]Row{{Node: u.tree}}) {
		t.Error("depth-2 orphan must hang")
	}

	// same orphan with its parent row present: connected
	parent := Row{Node: &Node{Path: "/s/parent"}, Conn: " ├─ "}
	u.rows = []Row{parent, orphan}
	u.offset = 0
	if u.topHang(nil) {
		t.Error("orphan under a visible parent must not hang")
	}

	// forest rows hang whenever the first one is cut by scrolling
	u.handle([]byte("P"))
	u.rows = u.tree.visibleRows(nil)
	if len(u.rows) < 2 {
		t.Fatal("forest rows missing")
	}
	u.offset = 0
	if u.topHang(nil) {
		t.Error("forest first row is the corner, must not hang")
	}
	u.offset = 1
	if !u.topHang(nil) {
		t.Error("scrolled forest tee must hang")
	}
}

func TestTopBorderHangCorner(t *testing.T) {
	g := testGraph()
	u := NewUI(g)
	if h := stripANSI(u.topBorder(false)); !strings.HasPrefix(h, "┌── Dependency graph") {
		t.Errorf("plain border = %q", h)
	}
	if h := stripANSI(u.topBorder(true)); !strings.HasPrefix(h, "┌──┐Dependency graph") {
		t.Errorf("hanging border = %q, want ┌──┐ before the label", h)
	}
	if w := runeLen(stripANSI(u.topBorder(true))); w != 80 {
		t.Errorf("hanging border width = %d, want 80", w)
	}
	if w := runeLen(stripANSI(u.topBorder(false))); w != 80 {
		t.Errorf("plain border width = %d, want 80", w)
	}
}

func TestForestHeaderIndent(t *testing.T) {
	g := testGraph()
	u := NewUI(g)
	u.rows = u.tree.visibleRows(nil)
	u.handle([]byte("P"))
	if h := stripANSI(u.headerRow(60, false)); !strings.HasPrefix(h, "│     NAME") {
		t.Errorf("forest header = %q, want NAME at the [+] column", h)
	}
	if w := runeLen(stripANSI(u.headerRow(60, false))); w != 62 {
		t.Errorf("forest header width = %d, want 62", w)
	}
	if h := stripANSI(u.headerRow(60, true)); !strings.HasPrefix(h, "│  │  NAME") {
		t.Errorf("forest hanging header = %q, want │ then NAME at the [+] column", h)
	}

	// the forward tree has a top-level row: NAME sits one space from the frame
	u.handle([]byte("P"))
	if h := stripANSI(u.headerRow(60, false)); !strings.HasPrefix(h, "│ NAME") {
		t.Errorf("forward header = %q, want NAME one space from the frame", h)
	}
	if h := stripANSI(u.headerRow(60, true)); !strings.HasPrefix(h, "│  │NAME") {
		t.Errorf("forward hanging header = %q, want │ before NAME", h)
	}
}
