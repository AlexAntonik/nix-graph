package main

import (
	"strings"
	"testing"
)

func TestLoadingLine(t *testing.T) {
	line, lw := loadingLine("⠹", "1.2s", "building dependency tree of /nix/store/xyz-system", 40)
	plain := stripANSI(line)
	if !strings.HasPrefix(plain, "⠹ 1.2s building") {
		t.Errorf("loading line = %q, want spinner and elapsed before the message", plain)
	}
	if lw != 40 || runeLen(plain) != 40 {
		t.Errorf("loading line width = %d (plain %d), want 40", lw, runeLen(plain))
	}
	line, lw = loadingLine("⠋", "0.1s", "msg", 3)
	if lw != 1 || runeLen(stripANSI(line)) != 1 {
		t.Errorf("narrow line width = %d, want only the frame", lw)
	}
}

func TestCenterPos(t *testing.T) {
	row, col := centerPos(80, 24, 30)
	if row != 12 || col != 26 {
		t.Errorf("centerPos(80,24,30) = %d,%d, want 12,26", row, col)
	}
	row, col = centerPos(20, 24, 30)
	if row != 12 || col != 1 {
		t.Errorf("wide line clamps to col 1, got %d,%d", row, col)
	}
	row, col = centerPos(80, 1, 30)
	if row != 1 || col != 26 {
		t.Errorf("one-line terminal = %d,%d, want 1,26", row, col)
	}
}
