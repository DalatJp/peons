package main

import (
	"image"
	"testing"
)

func TestClampTo(t *testing.T) {
	// zone utilisable : un écran 1920×1080 dont la barre des tâches occupe 48 px à gauche
	// et 40 px en haut (elle n'est pas forcément en bas)
	area := image.Rect(48, 40, 1920, 1080)
	tests := []struct {
		name string
		x, y int
		want image.Point
	}{
		{"déjà visible", 500, 300, image.Pt(500, 300)},
		{"sous la barre à gauche", 10, 300, image.Pt(48, 300)},
		{"sous la barre en haut", 500, 0, image.Pt(500, 40)},
		{"trop à droite", 1900, 300, image.Pt(1920-200, 300)},
		{"trop bas", 500, 1070, image.Pt(500, 1080-100)},
		{"hors écran, coin", -500, -500, image.Pt(48, 40)},
		{"collé au bord", 48, 40, image.Pt(48, 40)},
		{"au bord opposé", 1720, 980, image.Pt(1720, 980)},
	}
	for _, tt := range tests {
		if got := clampTo(image.Rect(tt.x, tt.y, tt.x+200, tt.y+100), area); got != tt.want {
			t.Errorf("%s : %v, attendu %v", tt.name, got, tt.want)
		}
	}
	// plus grande que la zone : le coin haut-gauche reste visible
	if got := clampTo(image.Rect(900, 900, 900+3000, 900+2000), area); got != area.Min {
		t.Errorf("fenêtre trop grande : %v, attendu %v", got, area.Min)
	}
}
