package main

import (
	"flag"
	"fmt"
	"os"

	"peons/internal/claudecfg"
)

// runInstall branche (ou débranche) le hook dans la configuration de Claude Code.
func runInstall(install bool) {
	dir := flag.String("config", "", "dossier de configuration de Claude Code (défaut : ~/.claude ou $CLAUDE_CONFIG_DIR)")
	flag.Parse()
	if *dir == "" {
		d, err := claudecfg.Dir()
		if err != nil {
			fail(err)
		}
		*dir = d
	}
	var changed bool
	var err error
	if install {
		exe, e := os.Executable()
		if e != nil {
			fail(e)
		}
		changed, err = claudecfg.Install(*dir, claudecfg.Command(exe))
	} else {
		changed, err = claudecfg.Uninstall(*dir)
	}
	if err != nil {
		fail(err)
	}
	switch {
	case !changed:
		fmt.Println("rien à changer")
	case install:
		fmt.Println("hook installé dans", *dir)
	default:
		fmt.Println("hook retiré de", *dir)
	}
}

// guiErrors : lancé par un double-clic (pas de terminal), une erreur s'affiche dans une boîte de message.
var guiErrors bool

func fail(err error) {
	logf("erreur : %v", err)
	if guiErrors {
		showError(err.Error())
	}
	fmt.Fprintln(os.Stderr, "peons :", err)
	os.Exit(1)
}
