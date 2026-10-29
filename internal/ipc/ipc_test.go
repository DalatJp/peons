package ipc

import (
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"peons/internal/session"
)

// isolate envoie la socket dans un dossier de test.
func isolate(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)
	t.Setenv("LocalAppData", dir) // os.UserCacheDir sous Windows
}

func listen(t *testing.T) (*Server, chan session.Event) {
	t.Helper()
	got := make(chan session.Event, 64)
	s, err := Listen(func(ev session.Event) { got <- ev })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s, got
}

func TestSendReachesServer(t *testing.T) {
	isolate(t)
	_, got := listen(t)
	want := session.Event{Session: "s1", Name: "Stop", Cwd: `E:\Dev\peons`, PID: 42, Started: 7}
	if err := Send(want); err != nil {
		t.Fatal(err)
	}
	select {
	case ev := <-got:
		if ev != want {
			t.Errorf("reçu %+v, attendu %+v", ev, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("événement non reçu")
	}
}

func TestSendWithoutServerFails(t *testing.T) {
	isolate(t)
	if err := Send(session.Event{Session: "s", Name: "Stop"}); err == nil {
		t.Error("Send doit échouer quand personne n'écoute")
	}
	if Running() {
		t.Error("Running doit être faux")
	}
}

func TestSecondInstanceIsRefused(t *testing.T) {
	isolate(t)
	listen(t)
	if _, err := Listen(func(session.Event) {}); err != ErrRunning {
		t.Errorf("erreur %v, attendu ErrRunning", err)
	}
}

func TestStaleSocketIsReplaced(t *testing.T) {
	isolate(t)
	s, _ := listen(t)
	path, _ := Path()
	// simule un plantage : la socket reste sur le disque sans personne derrière
	s.cancel()
	s.ln.(*net.UnixListener).SetUnlinkOnClose(false)
	s.ln.Close()
	<-s.done
	if _, err := os.Stat(path); err != nil {
		t.Skip("cette plateforme a retiré le fichier :", err)
	}
	if _, got := listen(t); got == nil {
		t.Fatal("la socket périmée doit être reprise")
	}
}

func TestGarbageIsDropped(t *testing.T) {
	isolate(t)
	_, got := listen(t)
	path, _ := Path()
	for _, junk := range []string{
		"", "pas du json", `{"event":"Stop"}`, `{"session":"s"}`,
		`{"session":"` + strings.Repeat("x", 129) + `","event":"Stop"}`,
		`{"session":"s","event":"Stop","cwd":"` + strings.Repeat("x", maxMessage) + `"}`,
	} {
		c, err := net.Dial("unix", path)
		if err != nil {
			t.Fatal(err)
		}
		c.Write([]byte(junk))
		c.Close()
	}
	if err := Send(session.Event{Session: "ok", Name: "Stop"}); err != nil {
		t.Fatal(err)
	}
	select {
	case ev := <-got:
		if ev.Session != "ok" {
			t.Errorf("un message invalide est passé : %+v", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("le message valide n'est pas arrivé")
	}
	select {
	case ev := <-got:
		t.Errorf("message en trop : %+v", ev)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestSlowClientDoesNotBlockOthers(t *testing.T) {
	isolate(t)
	_, got := listen(t)
	path, _ := Path()
	// un client qui se connecte et ne dit rien occupe une place, pas le serveur
	idle, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer idle.Close()
	if err := Send(session.Event{Session: "fast", Name: "Stop"}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-got:
	case <-time.After(time.Second):
		t.Fatal("un client lent bloque les autres")
	}
}
