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
	NarSize          uint64   `json:"narSize"`
	Deriver          string   `json:"deriver"`
	RegistrationTime int64    `json:"registrationTime"`
	References       []string `json:"references"`

	Direct int `json:"-"`
}

type Closure struct {
	Paths int
	Bytes uint64
}

type Graph struct {
	Root    string
	info    map[string]*Info
	closure map[string]Closure
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
	for path, info := range g.info {
		for _, ref := range info.References {
			if ref != path {
				if _, ok := g.info[ref]; ok {
					info.Direct++
				}
			}
		}
	}
}

func (g *Graph) Get(path string) *Info { return g.info[path] }

func (g *Graph) Size() int { return len(g.info) }

func (g *Graph) SortedRefs(path string) []string {
	info := g.info[path]
	if info == nil {
		return nil
	}
	refs := make([]string, 0, len(info.References))
	for _, ref := range info.References {
		if ref != path {
			if _, ok := g.info[ref]; ok {
				refs = append(refs, ref)
			}
		}
	}
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
