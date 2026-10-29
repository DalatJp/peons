// Package ipc relie les hooks (un processus par événement) à l'application par
// une socket Unix, présente sur Windows 10+, Linux et macOS : pas de port TCP
// ouvert à tout le monde, et la socket est dans un dossier propre à l'utilisateur.
//
// L'application ne fait confiance à rien de ce qui arrive : taille, durée et
// nombre de connexions simultanées sont bornés, les champs sont validés.
package ipc

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"

	"peons/internal/session"
)

const (
	maxMessage = 16 << 10 // un événement fait quelques centaines d'octets
	maxConns   = 16       // connexions traitées en même temps
	connTime   = 2 * time.Second
	dialTime   = 300 * time.Millisecond
)

// ErrRunning : une autre instance écoute déjà.
var ErrRunning = errors.New("peons tourne déjà")

// Path donne la socket de l'utilisateur courant.
func Path() (string, error) {
	dir := os.Getenv("XDG_RUNTIME_DIR") // Linux : tmpfs privé, vidé à la déconnexion
	if dir == "" {
		var err error
		if dir, err = os.UserCacheDir(); err != nil {
			return "", err
		}
	}
	return filepath.Join(dir, "peons", "peons.sock"), nil
}

// Send livre un événement à l'application. Il rend une erreur si personne
// n'écoute : l'appelant (le hook) peut alors la démarrer.
func Send(ev session.Event) error {
	path, err := Path()
	if err != nil {
		return err
	}
	body, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	conn, err := net.DialTimeout("unix", path, dialTime)
	if err != nil {
		return err
	}
	defer conn.Close()
	conn.SetWriteDeadline(time.Now().Add(time.Second))
	_, err = conn.Write(body)
	return err
}

// OpenSettings est l'événement de contrôle qui demande à l'application déjà
// lancée d'ouvrir ses réglages (un second lancement de l'exécutable l'envoie).
const OpenSettings = "peons.settings"

// SendOpenSettings demande à l'application qui tourne d'ouvrir ses réglages.
func SendOpenSettings() error {
	return Send(session.Event{Session: "control", Name: OpenSettings})
}

// Running dit si une application écoute.
func Running() bool {
	path, err := Path()
	if err != nil {
		return false
	}
	conn, err := net.DialTimeout("unix", path, dialTime)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// Server reçoit les événements.
type Server struct {
	ln     net.Listener
	cancel context.CancelFunc
	done   chan struct{}
}

// Listen ouvre la socket et appelle handle pour chaque événement valide, depuis
// une goroutine du serveur (handle doit donc être sûr en concurrence). Il
// rend ErrRunning si une autre instance écoute déjà ; une socket laissée par un
// plantage est reprise.
func Listen(handle func(session.Event)) (*Server, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if Running() {
		return nil, ErrRunning
	}
	os.Remove(path) // reste d'un plantage : personne n'écoute dessus
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &Server{ln: ln, cancel: cancel, done: make(chan struct{})}
	go s.serve(ctx, handle)
	return s, nil
}

// Close arrête d'écouter, attend la fin du service et supprime la socket.
func (s *Server) Close() {
	s.cancel()
	s.ln.Close() // net supprime le fichier d'une socket qu'il a créée
	<-s.done
}

func (s *Server) serve(ctx context.Context, handle func(session.Event)) {
	defer close(s.done)
	slots := make(chan struct{}, maxConns)
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			time.Sleep(50 * time.Millisecond) // erreur transitoire : pas de boucle serrée
			continue
		}
		select {
		case slots <- struct{}{}:
			go func() {
				defer func() { <-slots }()
				serveConn(conn, handle)
			}()
		default:
			conn.Close() // saturé : on abandonne cet événement plutôt que d'accumuler
		}
	}
	for i := 0; i < maxConns; i++ { // laisse finir les connexions en cours
		slots <- struct{}{}
	}
}

func serveConn(conn net.Conn, handle func(session.Event)) {
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(connTime))
	raw, err := io.ReadAll(io.LimitReader(conn, maxMessage+1))
	if err != nil || len(raw) > maxMessage {
		return
	}
	var ev session.Event
	if json.Unmarshal(raw, &ev) != nil || !valid(ev) {
		return
	}
	handle(ev)
}

func valid(ev session.Event) bool {
	return ev.Session != "" && len(ev.Session) <= 128 && ev.Name != "" && len(ev.Name) <= 64 &&
		len(ev.Cwd) <= 4096 && len(ev.Tool) <= 128 && len(ev.Transcript) <= 4096 &&
		len(ev.Agent) <= 128 && len(ev.AgentType) <= 128
}
