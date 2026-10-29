// Package proc retrouve le processus Claude Code d'un hook et vérifie plus tard
// qu'il tourne encore : fermer un terminal tue Claude Code sans SessionEnd.
//
// Deux fonctions par système :
//
//	FindClaude() (pid, started, ok)  — appelée par le hook
//	Alive(pid, started) bool         — appelée par l'application
//
// started est une date de création propre à l'OS, opaque pour l'appelant ; elle
// protège d'un PID réutilisé.
package proc
