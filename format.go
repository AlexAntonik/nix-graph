package main

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const storePrefix = "/nix/store/"

func trimStore(path string) (string, bool) {
	if !strings.HasPrefix(path, storePrefix) {
		return path, false
	}
	rest := path[len(storePrefix):]
	if rest == "" {
		return "", false
	}
	return rest, true
}

func Name(path string) string {
	rest, ok := trimStore(path)
	if !ok {
		return path
	}
	return rest
}

func PkgName(path string) string {
	rest, ok := trimStore(path)
	if !ok || len(rest) <= hashLen {
		return rest
	}
	return rest[hashLen+1:]
}

func Hash(path string) string {
	rest, ok := trimStore(path)
	if !ok || len(rest) <= hashLen {
		return ""
	}
	return rest[:hashLen]
}

func ShortName(path string) string {
	rest, ok := trimStore(path)
	if !ok || len(rest) <= hashLen+1 {
		return rest
	}
	return rest[:4] + "…" + rest[hashLen-2:hashLen] + " " + rest[hashLen+1:]
}

func HumanSize(n uint64) string {
	const k, m, g, t = 1024.0, 1024.0 * 1024, 1024.0 * 1024 * 1024, 1024.0 * 1024 * 1024 * 1024
	switch f := float64(n); {
	case f < k:
		return fmt.Sprintf("%dB", n)
	case f < m:
		return fmt.Sprintf("%.1fK", f/k)
	case f < g:
		return fmt.Sprintf("%.1fM", f/m)
	case f < t:
		return fmt.Sprintf("%.1fG", f/g)
	default:
		return fmt.Sprintf("%.1fT", f/t)
	}
}

func runeLen(s string) int { return utf8.RuneCountInString(s) }

func truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if runeLen(s) <= w {
		return s
	}
	if w == 1 {
		return "…"
	}
	r := []rune(s)
	return string(r[:w-1]) + "…"
}

func padEnd(s string, w int) string {
	if pad := w - runeLen(s); pad > 0 {
		return s + strings.Repeat(" ", pad)
	}
	return truncate(s, w)
}

func splitAt(s string, n int) (string, string) {
	r := []rune(s)
	if n < 0 {
		n = 0
	}
	if n > len(r) {
		n = len(r)
	}
	return string(r[:n]), string(r[n:])
}
