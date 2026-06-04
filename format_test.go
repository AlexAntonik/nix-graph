package main

import "testing"

func TestHumanSize(t *testing.T) {
	tests := []struct {
		in   uint64
		want string
	}{
		{0, "0B"},
		{512, "512B"},
		{1024, "1.0K"},
		{1536, "1.5K"},
		{5 << 20, "5.0M"},
		{3 << 30, "3.0G"},
	}
	for _, tt := range tests {
		if got := HumanSize(tt.in); got != tt.want {
			t.Errorf("HumanSize(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestShortName(t *testing.T) {
	path := storePrefix + "0123456789abcdefghij0123456789ab-bash-5.2-p103"
	want := "0123…ab bash-5.2-p103"
	if got := ShortName(path); got != want {
		t.Errorf("ShortName = %q, want %q", got, want)
	}
	if got := ShortName("/etc/hosts"); got != "/etc/hosts" {
		t.Errorf("ShortName non-store = %q, want unchanged", got)
	}
}

func TestPkgName(t *testing.T) {
	path := storePrefix + "0123456789abcdefghij0123456789ab-bash-5.2-p103"
	if got := PkgName(path); got != "bash-5.2-p103" {
		t.Errorf("PkgName = %q, want %q", got, "bash-5.2-p103")
	}
	if got := PkgName("/etc/hosts"); got != "/etc/hosts" {
		t.Errorf("PkgName non-store = %q, want unchanged", got)
	}
}

func TestPadEndTruncate(t *testing.T) {
	tests := []struct {
		s    string
		w    int
		want string
	}{
		{"ab", 4, "ab  "},
		{"abcdef", 4, "abc…"},
		{"abc", 3, "abc"},
		{"abc", 0, ""},
	}
	for _, tt := range tests {
		if got := padEnd(tt.s, tt.w); got != tt.want {
			t.Errorf("padEnd(%q, %d) = %q, want %q", tt.s, tt.w, got, tt.want)
		}
	}
}
