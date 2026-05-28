package main

type Node struct {
	Path     string
	Parent   *Node
	Expanded bool
	Loaded   bool
	Children []*Node
}

func NewTree(g *Graph) *Node {
	root := &Node{Path: g.Root}
	root.Toggle(g)
	return root
}

func (n *Node) Toggle(g *Graph) {
	if n.isLeaf(g) {
		return
	}
	if !n.Loaded {
		n.load(g)
	}
	n.Expanded = !n.Expanded
}

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
	return info == nil || info.Direct == 0
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

func (n *Node) Visible() []Row {
	rows := []Row{{Node: n}}
	if !n.Expanded || !n.Loaded {
		return rows
	}
	var walk func(*Node, string, int, bool)
	walk = func(node *Node, prefix string, depth int, last bool) {
		conn := " ├─ "
		if last {
			conn = " └─ "
		}
		rows = append(rows, Row{node, prefix, conn})
		if !node.Expanded || !node.Loaded {
			return
		}
		for i, child := range node.Children {
			lastChild := i == len(node.Children)-1
			childPrefix := prefix
			if depth > 0 {
				if last {
					childPrefix += "    "
				} else {
					childPrefix += " │  "
				}
			}
			walk(child, childPrefix, depth+1, lastChild)
		}
	}
	for i, child := range n.Children {
		walk(child, "", 1, i == len(n.Children)-1)
	}
	return rows
}
