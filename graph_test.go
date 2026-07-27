package main

import "testing"

func testGraph() *Graph {
	g := &Graph{
		Root: "/s/root",
		info: map[string]*Info{
			"/s/root":  {NarSize: 10, References: []string{"/s/big", "/s/small", "/s/root", "/s/ghost"}},
			"/s/big":   {NarSize: 300, References: []string{"/s/root"}},
			"/s/small": {NarSize: 50, References: []string{"/s/small"}},
		},
	}
	g.countDirect()
	return g
}

func TestSortedRefs(t *testing.T) {
	g := testGraph()
	refs := g.SortedRefs("/s/root")
	want := []string{"/s/big", "/s/small"}
	if len(refs) != len(want) {
		t.Fatalf("SortedRefs = %v, want %v", refs, want)
	}
	for i := range want {
		if refs[i] != want[i] {
			t.Fatalf("SortedRefs = %v, want %v", refs, want)
		}
	}
}

func TestDirectCount(t *testing.T) {
	g := testGraph()
	if got := g.Get("/s/root").Direct; got != 2 {
		t.Errorf("Direct = %d, want 2", got)
	}
	if got := g.Get("/s/small").Direct; got != 0 {
		t.Errorf("Direct self-ref = %d, want 0", got)
	}
}

func TestClosure(t *testing.T) {
	g := testGraph()
	c := g.Closure("/s/root")
	if c.Paths != 3 || c.Bytes != 360 {
		t.Errorf("Closure(root) = %+v, want {3 360}", c)
	}
	c = g.Closure("/s/small")
	if c.Paths != 1 || c.Bytes != 50 {
		t.Errorf("Closure(small) self-ref = %+v, want {1 50}", c)
	}
}

func TestClosureCycle(t *testing.T) {
	g := testGraph()
	g.info["/s/loop"] = &Info{NarSize: 7, References: []string{"/s/loop2", "/s/root"}}
	g.info["/s/loop2"] = &Info{NarSize: 8, References: []string{"/s/loop"}}
	c := g.Closure("/s/loop")
	if c.Paths != 5 || c.Bytes != 375 {
		t.Errorf("Closure with cycle = %+v, want {5 375}", c)
	}
}

// addedGraph: lib is shared by root, a and b; only is reachable through a.
func addedGraph() *Graph {
	g := &Graph{
		Root: "/s/root",
		info: map[string]*Info{
			"/s/root": {NarSize: 10, References: []string{"/s/a", "/s/b", "/s/lib"}},
			"/s/a":    {NarSize: 100, References: []string{"/s/lib", "/s/only"}},
			"/s/only": {NarSize: 5},
			"/s/b":    {NarSize: 200, References: []string{"/s/lib"}},
			"/s/lib":  {NarSize: 1000, References: []string{"/s/lib"}},
		},
	}
	g.countDirect()
	return g
}

func TestAdded(t *testing.T) {
	g := addedGraph()
	want := map[string]uint64{
		"/s/root": 1315, // = closure size of the root
		"/s/a":    105,  // itself plus the unique dep
		"/s/b":    200,
		"/s/lib":  1000,
		"/s/only": 5,
	}
	for path, size := range want {
		if got := g.Added(path); got != size {
			t.Errorf("Added(%s) = %d, want %d", path, got, size)
		}
	}
}

func TestAddedDeepShared(t *testing.T) {
	g := &Graph{
		Root: "/s/root",
		info: map[string]*Info{
			"/s/root": {NarSize: 10, References: []string{"/s/a", "/s/b"}},
			"/s/a":    {NarSize: 100, References: []string{"/s/x", "/s/y"}},
			"/s/b":    {NarSize: 200, References: []string{"/s/x", "/s/z"}},
			"/s/x":    {NarSize: 1000},
			"/s/y":    {NarSize: 50, References: []string{"/s/z"}},
			"/s/z":    {NarSize: 20},
		},
	}
	g.countDirect()
	// x and z are also reachable through b, only y is unique to a
	want := map[string]uint64{
		"/s/root": 1380,
		"/s/a":    150,
		"/s/b":    200,
		"/s/x":    1000,
		"/s/y":    50,
		"/s/z":    20,
	}
	for path, size := range want {
		if got := g.Added(path); got != size {
			t.Errorf("Added(%s) = %d, want %d", path, got, size)
		}
	}
	if c := g.Closure("/s/root"); c.Bytes != 1380 {
		t.Errorf("Closure(root) = %d, want 1380", c.Bytes)
	}
}

func TestReverseGraph(t *testing.T) {
	g := testGraph()
	if got := g.Get("/s/root").Dependents; got != 1 {
		t.Errorf("Dependents(root) = %d, want 1", got)
	}
	if got := g.Get("/s/big").Dependents; got != 1 {
		t.Errorf("Dependents(big) = %d, want 1", got)
	}

	g.Reverse = true
	if refs := g.SortedRefs("/s/root"); !eqStrs(refs, []string{"/s/big"}) {
		t.Errorf("reverse refs of root = %v, want [/s/big]", refs)
	}
	if refs := g.SortedRefs("/s/small"); !eqStrs(refs, []string{"/s/root"}) {
		t.Errorf("reverse refs of small = %v, want [/s/root]", refs)
	}

	root := NewTreeAt(g, "/s/big")
	if !root.Loaded || len(root.Children) != 1 || root.Children[0].Path != "/s/root" {
		t.Fatalf("reverse tree of big = %+v, want [root]", root.Children)
	}
	if root.Marker(g) != "[-]" {
		t.Errorf("reverse marker = %q, want [-]", root.Marker(g))
	}
	kids := root.Children[0]
	kids.Toggle(g)
	if len(kids.Children) != 0 {
		t.Errorf("dependent big is ancestor, must be pruned: %d children", len(kids.Children))
	}

	g.Reverse = false
	if refs := g.SortedRefs("/s/root"); !eqStrs(refs, []string{"/s/big", "/s/small"}) {
		t.Errorf("forward refs of root = %v", refs)
	}
}

func eqStrs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range b {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestParseInfoJSON(t *testing.T) {
	data := []byte(`{
  "/nix/store/aaa-root": {
    "narSize": 100,
    "deriver": "/nix/store/d.drv",
    "registrationTime": 1700000000,
    "references": ["/nix/store/bbb-lib", "/nix/store/aaa-root"]
  },
  "/nix/store/bbb-lib": {
    "narSize": 200,
    "references": []
  }
}`)
	if _, err := parseInfoJSON(data); err != nil {
		t.Fatalf("parseInfoJSON: %v", err)
	}
}

func TestLastLine(t *testing.T) {
	if got := lastLine("warning: x\nerror: no such path\n"); got != "error: no such path" {
		t.Errorf("lastLine = %q, want the last line", got)
	}
	if got := lastLine("  \n"); got != "" {
		t.Errorf("lastLine(blank) = %q, want empty", got)
	}
}

func TestAddedCycle(t *testing.T) {
	g := &Graph{
		Root: "/s/root",
		info: map[string]*Info{
			"/s/root": {NarSize: 10, References: []string{"/s/a"}},
			"/s/a":    {NarSize: 100, References: []string{"/s/b"}},
			"/s/b":    {NarSize: 200, References: []string{"/s/a"}},
		},
	}
	g.countDirect()
	// b is reachable only through a, so a carries a and b
	want := map[string]uint64{"/s/root": 310, "/s/a": 300, "/s/b": 200}
	for path, size := range want {
		if got := g.Added(path); got != size {
			t.Errorf("Added(%s) = %d, want %d", path, got, size)
		}
	}
}
