package main

import "testing"

func TestTipScaleIsIndependentOfClawdSize(t *testing.T) {
	for _, c := range []struct {
		size    int
		display float64
		want    int
	}{
		{1, 1, 1},
		{1, 1.5, 2}, // taille par défaut, écran à 150 % : comme avant
		{1, 2, 2},
		{2, 1, 2},
		{4, 1, 4},
		{2, 1.5, 3},
		{0, 1, 1}, // jamais sous 1
	} {
		if got := tipScale(c.size, c.display); got != c.want {
			t.Errorf("tipScale(%d, %.2f) = %d, attendu %d", c.size, c.display, got, c.want)
		}
	}
}

func TestClampTipSize(t *testing.T) {
	for in, want := range map[int]int{-2: minTipSize, 0: defaultTipSize, 1: 1, 3: 3, 4: 4, 5: maxTipSize, 99: maxTipSize} {
		if got := clampTipSize(in); got != want {
			t.Errorf("clampTipSize(%d) = %d, attendu %d", in, got, want)
		}
	}
}
