// peons : un clawd par session Claude Code, dans une fenêtre transparente.
//
//	peons             lance l'application et ouvre les réglages (là, un bouton branche le hook)
//	peons run         idem ; options : voir « peons run -h »
//	peons hook        hook Claude Code : lit l'événement sur stdin et le transmet à l'application
//	peons install     branche le hook dans ~/.claude/settings.json (même chose que le bouton)
//	peons uninstall   le retire
//
// L'exécutable est construit pour le sous-système graphique (voir build.ps1) :
// un double-clic n'ouvre pas de console. Il ne dépend que des DLL de Windows.
package main

import (
	"fmt"
	"os"
	"runtime"
	"strings"
)

// Les fenêtres et la pompe de messages (waitMessages) doivent rester sur un seul
// fil du système.
func init() { runtime.LockOSThread() }

func main() {
	cmd := "run"
	if len(os.Args) > 1 && !strings.HasPrefix(os.Args[1], "-") {
		cmd = os.Args[1]
		os.Args = append(os.Args[:1], os.Args[2:]...)
	}
	// le hook n'écrit jamais ; « run » sans option vient d'un double-clic ou du hook
	if cmd != "hook" && (cmd != "run" || len(os.Args) > 1) {
		useConsole()
	}
	switch cmd {
	case "run":
		runApp()
	case "hook":
		runHook()
	case "install":
		runInstall(true)
	case "uninstall":
		runInstall(false)
	default:
		fmt.Fprintf(os.Stderr, "commande inconnue %q (run, hook, install, uninstall)\n", cmd)
		os.Exit(2)
	}
}
