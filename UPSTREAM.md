# Upstream Attribution

Ce projet a été inspiré par de [sk-ruban/notchi](https://github.com/sk-ruban/notchi) (macOS) et de son portage Windows en Python.

Repris de l'upstream :

- le concept de l'application ;
- les noms des états et des émotions.

Les sprites de clawd sont dessinés par l'auteur du repo. Ceux de la pioche et du piston sont tirés/inspirés de de Minecraft : voir `LICENSE`.

Aucune bibliothèque tierce côté interface : l'application parle directement à Windows (`user32`, `gdi32`, `shell32`) par `golang.org/x/sys/windows`, sous-dépôt du projet Go.

Police des étiquettes : **PxPlus IBM EGA 8x14**, tirée de l'*Ultimate Oldschool PC Font Pack* de VileR (int10h.org), publiée sous licence CC BY-SA 4.0 (attribution et partage dans les mêmes conditions : à vérifier sur la page du pack avant toute diffusion).
