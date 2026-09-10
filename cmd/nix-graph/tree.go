package main

type Node struct {
	Path     string
	Parent   *Node
	Expanded bool
	Loaded   bool
	Hidden   bool
	Children []*Node
}

func NewTree(g *Graph) *Node {
	return NewTreeAt(g, g.Root)
}

func NewTreeAt(g *Graph, path string) *Node {
	root := &Node{Path: path}
	root.Toggle(g)
	return root
}

// NewForest makes a hidden root holding every package in the graph (the P view).
func NewForest(g *Graph) *Node {
	root := &Node{Hidden: true, Expanded: true}
	paths := g.AllPaths()
	children := make([]*Node, 0, len(paths))
	for _, p := range paths {
		children = append(children, &Node{Path: p, Parent: root})
	}
	root.Children = children
	root.Loaded = true
	return root
}

// Toggle lazy-loads children on first use, then expands or collapses the node.
func (n *Node) Toggle(g *Graph) {
	if n.Hidden || n.isLeaf(g) {
		return
	}
	if !n.Loaded {
		n.load(g)
	}
	n.Expanded = !n.Expanded
}

// load fetches children of the node, dropping refs that already appear above 
func (n *Node) load(g *Graph) {
	refs := g.SortedRefs(n.Path)
	children := make([]*Node, 0, len(refs))
	for _, ref := range refs {
		if n.hasAncestor(ref) {
			continue
		}
		children = append(children, &Node{Path: ref, Parent: n})
	}
	n.Children = children
	n.Loaded = true
}

func (n *Node) hasAncestor(path string) bool {
	for a := n.Parent; a != nil; a = a.Parent {
		if a.Path == path {
			return true
		}
	}
	return false
}

func (n *Node) isLeaf(g *Graph) bool {
	if n.Loaded {
		return len(n.Children) == 0
	}
	info := g.Get(n.Path)
	if info == nil {
		return true
	}
	if g.Reverse {
		return info.Dependents == 0
	}
	return info.Direct == 0
}

func (n *Node) Marker(g *Graph) string {
	if n.isLeaf(g) {
		return ""
	}
	if n.Expanded {
		return "[-]"
	}
	return "[+]"
}

type Row struct {
	Node   *Node
	Prefix string
	Conn   string
}

// visibleRows flattens the expanded tree into display rows.
func (n *Node) visibleRows(match func(*Node) bool) []Row {
	rows := make([]Row, 0, 8)
	if !n.Hidden {
		rows = append(rows, Row{Node: n})
	}
	if !n.Expanded || !n.Loaded {
		return rows
	}
	var walk func(*Node, string, bool, bool)
	walk = func(node *Node, prefix string, last bool, first bool) {
		conn := " ├─ "
		if first {
			conn = " ┌─ "
		} else if last {
			conn = " └─ "
		}
		rows = append(rows, Row{node, prefix, conn})
		if !node.Expanded || !node.Loaded {
			return
		}
		kept := matchedChildren(node, match)
		for i, child := range kept {
			childPrefix := prefix + " │  "
			if last {
				childPrefix = prefix + "    "
			}
			walk(child, childPrefix, i == len(kept)-1, false)
		}
	}
	kept := matchedChildren(n, match)
	for i, child := range kept {
		walk(child, "", i == len(kept)-1, n.Hidden && i == 0)
	}
	return rows
}

func matchedChildren(n *Node, match func(*Node) bool) []*Node {
	if match == nil {
		return n.Children
	}
	kept := make([]*Node, 0, len(n.Children))
	for _, c := range n.Children {
		if match(c) || c.hasMatchDesc(match) {
			kept = append(kept, c)
		}
	}
	return kept
}

func (n *Node) hasMatchDesc(match func(*Node) bool) bool {
	if !n.Expanded || !n.Loaded {
		return false
	}
	for _, c := range n.Children {
		if match(c) || c.hasMatchDesc(match) {
			return true
		}
	}
	return false
}
