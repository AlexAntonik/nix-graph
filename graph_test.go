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
		closure: map[string]Closure{},
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
