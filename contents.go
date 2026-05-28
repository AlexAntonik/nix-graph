package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

type Entry struct {
	Name string
	Size uint64
	Dir  bool
	Exec bool
}

type Contents struct {
	Entries []Entry
	Dirs    int
	Files   int
	Execs   int
}

func Inspect(path string) (Contents, error) {
	var c Contents
	dirEntries, err := os.ReadDir(path)
	if err != nil {
		return c, err
	}
	for _, de := range dirEntries {
		e := Entry{Name: de.Name(), Dir: de.IsDir()}
		if e.Dir {
			e.Size = walkDir(filepath.Join(path, de.Name()), &c)
			c.Dirs++
		} else if fi, err := de.Info(); err == nil {
			e.Size = uint64(fi.Size())
			e.Exec = fi.Mode()&0o111 != 0
			c.Files++
			if e.Exec {
				c.Execs++
			}
		}
		c.Entries = append(c.Entries, e)
	}
	sort.Slice(c.Entries, func(i, j int) bool {
		if c.Entries[i].Size != c.Entries[j].Size {
			return c.Entries[i].Size > c.Entries[j].Size
		}
		return c.Entries[i].Name < c.Entries[j].Name
	})
	return c, nil
}

func walkDir(root string, c *Contents) uint64 {
	var total uint64
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if p != root {
				c.Dirs++
			}
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			return nil
		}
		total += uint64(fi.Size())
		c.Files++
		if fi.Mode()&0o111 != 0 {
			c.Execs++
		}
		return nil
	})
	return total
}
