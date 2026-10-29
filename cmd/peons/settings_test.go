package main

import (
	"strings"
	"testing"
	"time"
)

func TestStepSpan(t *testing.T) {
	tests := []struct {
		from time.Duration
		dir  int
		want time.Duration
	}{
		{2 * time.Minute, +1, 3 * time.Minute},
		{2 * time.Minute, -1, 1 * time.Minute},
		{1 * time.Minute, -1, 1 * time.Minute}, // plancher
		{9 * time.Minute, +1, 10 * time.Minute},
		{10 * time.Minute, +1, 15 * time.Minute}, // pas de 5 au-delà de 10
		{10 * time.Minute, -1, 9 * time.Minute},
		{15 * time.Minute, -1, 10 * time.Minute},
		{120 * time.Minute, +1, 120 * time.Minute}, // plafond
		{20 * time.Second, +1, 1 * time.Minute},    // valeur venue d'un drapeau : on revient à la minute
	}
	for _, tt := range tests {
		if got := stepSpan(tt.from, tt.dir); got != tt.want {
			t.Errorf("stepSpan(%v, %+d) = %v, attendu %v", tt.from, tt.dir, got, tt.want)
		}
	}
}

func TestClampScale(t *testing.T) {
	for in, want := range map[int]int{-3: minScale, 0: minScale, 1: 1, 3: 3, 8: 8, 9: maxScale, 100: maxScale} {
		if got := clampScale(in); got != want {
			t.Errorf("clampScale(%d) = %d, attendu %d", in, got, want)
		}
	}
	if minScale != 1 {
		t.Error("1× doit rester possible : un pixel d'écran par pixel du sprite")
	}
}

func TestFormatting(t *testing.T) {
	for d, want := range map[time.Duration]string{
		-time.Second:                          "0 s",
		12 * time.Second:                      "12 s",
		185 * time.Second:                     "3 min 05 s",
		time.Hour + 2*time.Minute:             "1 h 02 min",
		26*time.Hour + 30*time.Minute:         "26 h 30 min",
		59*time.Second + 600*time.Millisecond: "1 min 00 s",
	} {
		if got := fmtDur(d); got != want {
			t.Errorf("fmtDur(%v) = %q, attendu %q", d, got, want)
		}
	}
	if got := fmtSpan(2 * time.Minute); got != "2 min" {
		t.Errorf("fmtSpan = %q", got)
	}
	if got := fmtSpan(20 * time.Second); got != "20 s" {
		t.Errorf("fmtSpan = %q", got)
	}
	if got := ellipsize("abcdef", 4); got != "abc…" {
		t.Errorf("ellipsize = %q", got)
	}
	if got := ellipsize("éé", 4); got != "éé" {
		t.Errorf("ellipsize ne doit pas couper un texte court : %q", got)
	}
	lines := wrap("un deux trois quatre cinq", 9)
	if len(lines) != 4 || lines[0] != "un deux" || lines[1] != "trois" || lines[3] != "cinq" {
		t.Errorf("wrap = %q", lines)
	}
}

func TestTruncate(t *testing.T) {
	long := strings.Repeat("é", 60)
	for in, want := range map[string]string{"court": "court", strings.Repeat("a", 50): strings.Repeat("a", 50), strings.Repeat("a", 51): strings.Repeat("a", 50) + "…", long: strings.Repeat("é", 50) + "…"} {
		if got := truncate(in, maxNameLen); got != want {
			t.Errorf("truncate(%.12q…) = %.12q…, attendu %.12q…", in, got, want)
		}
	}
}
