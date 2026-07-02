package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
)

const hashLen = 32

type Info struct {
	NarSize    uint64   `json:"narSize"`
	References []string `json:"references"`

	Direct     int `json:"-"`
	Dependents int `json:"-"`
}

type Closure struct {
	Paths int
	Bytes uint64
}

type Graph struct {
	Root       string
	Reverse    bool
	info       map[string]*Info
	closure    map[string]Closure
	dependents map[string][]string
	depClosure map[string]int
	added      map[string]uint64
}

func Load(root string) (*Graph, error) {
	data, err := nixPathInfo(root)
	if err != nil {
		return nil, err
	}
	g, err := parseInfoJSON(data)
	if err != nil {
		return nil, err
	}
	g.Root = root
	if _, ok := g.info[root]; !ok {
		return nil, fmt.Errorf("%s not found in the store", root)
	}
	return g, nil
}

func parseInfoJSON(data []byte) (*Graph, error) {
	var raw map[string]*Info
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse nix output: %w", err)
	}
	if len(raw) == 0 {
		return nil, errors.New("nix returned no paths")
	}
	g := &Graph{
		info:    make(map[string]*Info, len(raw)),
		closure: make(map[string]Closure, len(raw)),
	}
	for path, info := range raw {
		if info == nil {
			continue
		}
		g.info[path] = info
	}
	g.countDirect()
	return g, nil
}

func nixPathInfo(root string) ([]byte, error) {
	if out, err := runNix(root, true); err == nil {
		return out, nil
	}
	return runNix(root, false)
}

func runNix(root string, jsonFormat bool) ([]byte, error) {
	args := []string{"path-info", "-r"}
	if jsonFormat {
		args = append(args, "--json-format", "1")
	}
	args = append(args, "--json", root)
	cmd := exec.Command("nix", args...)
	cmd.Env = append(os.Environ(), "NIX_CONFIG=experimental-features = nix-command")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("nix path-info: %s", lastLine(stderr.String()))
	}
	return out, nil
}

func lastLine(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "unknown error"
	}
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		s = s[i+1:]
	}
	return strings.TrimSpace(s)
}

func (g *Graph) countDirect() {
	if g.dependents == nil {
		g.dependents = make(map[string][]string, len(g.info))
	}
	for path, info := range g.info {
		for _, ref := range info.References {
			if ref != path {
				if dep, ok := g.info[ref]; ok {
					info.Direct++
					dep.Dependents++
					g.dependents[ref] = append(g.dependents[ref], path)
				}
			}
		}
	}
}

func (g *Graph) Get(path string) *Info { return g.info[path] }

func (g *Graph) Size() int { return len(g.info) }

func (g *Graph) AllPaths() []string {
	paths := make([]string, 0, len(g.info))
	for p := range g.info {
		paths = append(paths, p)
	}
	sort.Slice(paths, func(i, j int) bool {
		a, b := g.info[paths[i]].NarSize, g.info[paths[j]].NarSize
		if a != b {
			return a > b
		}
		return paths[i] < paths[j]
	})
	return paths
}

func (g *Graph) SortedRefs(path string) []string {
	var src []string
	if g.Reverse {
		src = g.dependents[path]
	} else {
		info := g.info[path]
		if info == nil {
			return nil
		}
		for _, ref := range info.References {
			if ref != path {
				if _, ok := g.info[ref]; ok {
					src = append(src, ref)
				}
			}
		}
	}
	refs := make([]string, len(src))
	copy(refs, src)
	sort.Slice(refs, func(i, j int) bool {
		a, b := g.info[refs[i]].NarSize, g.info[refs[j]].NarSize
		if a != b {
			return a > b
		}
		return refs[i] < refs[j]
	})
	return refs
}

func (g *Graph) Closure(path string) Closure {
	if c, ok := g.closure[path]; ok {
		return c
	}
	c := g.closureOf(path)
	g.closure[path] = c
	return c
}

func (g *Graph) closureOf(path string) Closure {
	var c Closure
	seen := map[string]bool{path: true}
	stack := []string{path}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		info := g.info[cur]
		if info == nil {
			continue
		}
		c.Paths++
		c.Bytes += info.NarSize
		for _, ref := range info.References {
			if !seen[ref] && g.info[ref] != nil {
				seen[ref] = true
				stack = append(stack, ref)
			}
		}
	}
	return c
}

// Added returns the added size of path: the size of the path itself plus
// the sizes of all transitive dependencies that are reachable only through
func (g *Graph) Added(path string) uint64 {
	if g.added == nil {
		g.buildAdded()
	}
	if n, ok := g.added[path]; ok {
		return n
	}
	if i := g.info[path]; i != nil {
		return i.NarSize
	}
	return 0
}

func (g *Graph) buildAdded() {
	g.added = make(map[string]uint64, len(g.info))
	if _, ok := g.info[g.Root]; !ok {
		return
	}

	// Depth-first search over forward references, collecting nodes in
	// postorder.
	seen := map[string]bool{g.Root: true}
	type frame struct {
		path string
		next int
	}
	var post []string
	stack := []frame{{g.Root, 0}}
	for len(stack) > 0 {
		f := &stack[len(stack)-1]
		info := g.info[f.path]
		moved := false
		for info != nil && f.next < len(info.References) {
			ref := info.References[f.next]
			f.next++
			if ref == f.path || seen[ref] || g.info[ref] == nil {
				continue
			}
			seen[ref] = true
			stack = append(stack, frame{ref, 0})
			moved = true
			break
		}
		if !moved {
			post = append(post, f.path)
			stack = stack[:len(stack)-1]
		}
	}

	n := len(post)
	rpo := make([]string, n)
	idx := make(map[string]int, n)
	for i, p := range post {
		rpo[n-1-i] = p
	}
	for i, p := range rpo {
		idx[p] = i
	}

	preds := make([][]int, n)
	for i, p := range rpo {
		info := g.info[p]
		for _, ref := range info.References {
			if ref == p {
				continue
			}
			if j, ok := idx[ref]; ok {
				preds[j] = append(preds[j], i)
			}
		}
	}

	idom := make([]int, n)
	for i := range idom {
		idom[i] = -1
	}
	idom[0] = 0
	intersect := func(a, b int) int {
		for a != b {
			for a > b {
				a = idom[a]
			}
			for b > a {
				b = idom[b]
			}
		}
		return a
	}
	for changed := true; changed; {
		changed = false
		for w := 1; w < n; w++ {
			newIDom := -1
			for _, p := range preds[w] {
				if idom[p] == -1 {
					continue
				}
				if newIDom == -1 {
					newIDom = p
				} else {
					newIDom = intersect(newIDom, p)
				}
			}
			if newIDom != -1 && idom[w] != newIDom {
				idom[w] = newIDom
				changed = true
			}
		}
	}

	kids := make([][]int, n)
	for w := 1; w < n; w++ {
		if d := idom[w]; d >= 0 && d != w {
			kids[d] = append(kids[d], w)
		}
	}
	for w := n - 1; w >= 0; w-- {
		sum := g.info[rpo[w]].NarSize
		for _, c := range kids[w] {
			sum += g.added[rpo[c]]
		}
		g.added[rpo[w]] = sum
	}
}

// DependentsClosure returns how many paths transitively depend on path,
// counting path itself.
func (g *Graph) DependentsClosure(path string) int {
	if g.depClosure == nil {
		g.depClosure = make(map[string]int, len(g.info))
	}
	if n, ok := g.depClosure[path]; ok {
		return n
	}
	seen := map[string]bool{path: true}
	stack := []string{path}
	n := 0
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		n++
		for _, d := range g.dependents[cur] {
			if !seen[d] {
				seen[d] = true
				stack = append(stack, d)
			}
		}
	}
	g.depClosure[path] = n
	return n
}
