package main

import "testing"

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
