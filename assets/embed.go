// Package assets embarque le personnage (corps, yeux), les effets (fx), le terrain (l'île)
// et la police des étiquettes dans le binaire.
package assets

import "embed"

//go:embed characters/clawd fx terrain PxPlus_IBM_EGA_8x14.ttf
var FS embed.FS
