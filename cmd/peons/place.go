package main

import "image"

// workArea est la zone utilisable de l'écran qui porte le plus de r (ou le plus proche
// si r est hors de tout écran) : l'écran moins la barre des tâches, où qu'elle soit
// et quelle que soit sa taille. ok est faux si l'OS ne répond pas.
func workArea(r image.Rectangle) (area image.Rectangle, ok bool) {
	if uiWnd.Load() == 0 { // interface pas montée (les tests) : pas d'écran à consulter
		return image.Rectangle{}, false
	}
	_, work, ok := monitorBounds(monitorFromRect(r, monitorNearest))
	if !ok || work.Empty() {
		return image.Rectangle{}, false
	}
	return work, true
}

// clampTo ramène le rectangle r dans area, sans le redimensionner. Si r est plus
// grand que area, son coin haut-gauche reste visible.
func clampTo(r, area image.Rectangle) image.Point {
	x := max(min(r.Min.X, area.Max.X-r.Dx()), area.Min.X)
	y := max(min(r.Min.Y, area.Max.Y-r.Dy()), area.Min.Y)
	return image.Pt(x, y)
}

// fit donne la position (coin haut-gauche) la plus proche de (x, y) où toute la
// fenêtre est visible : ni sous la barre des tâches ni hors de l'écran.
func (a *app) fit(x, y int) (int, int) {
	r := image.Rect(x, y, x+a.W, y+a.H)
	area, ok := workArea(r)
	if !ok {
		return x, y
	}
	p := clampTo(r, area)
	return p.X, p.Y
}
