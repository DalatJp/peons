package proc

import (
	"os"
	"os/exec"
	"testing"
)

func TestAlive(t *testing.T) {
	self := uint32(os.Getpid())
	started, ok := creationTime(self)
	if !ok {
		t.Fatal("date de création du processus courant illisible")
	}
	if !Alive(self, started) || !Alive(self, 0) {
		t.Error("le processus courant est vivant")
	}
	if Alive(self, started+1) {
		t.Error("une date de création différente désigne un autre processus (PID réutilisé)")
	}

	cmd := exec.Command("cmd", "/c", "ping -n 30 127.0.0.1 >nul")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pid := uint32(cmd.Process.Pid)
	born, _ := creationTime(pid)
	if !Alive(pid, born) {
		t.Error("le processus lancé doit être vivant")
	}
	cmd.Process.Kill()
	cmd.Wait()
	if Alive(pid, born) {
		t.Error("le processus tué ne doit plus être vivant")
	}
}

// Sous go test, aucun parent n'est Claude Code : la recherche échoue proprement.
func TestFindClaudeAbsent(t *testing.T) {
	if _, _, ok := FindClaude(); ok {
		t.Skip("lancé sous Claude Code : la recherche aboutit, c'est normal")
	}
}
