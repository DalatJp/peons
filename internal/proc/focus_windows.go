package proc

import (
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode"
	"unsafe"
)

var (
	user32                  = syscall.NewLazyDLL("user32.dll")
	kernel32                = syscall.NewLazyDLL("kernel32.dll")
	procEnumWindows         = user32.NewProc("EnumWindows")
	procWindowThreadProcess = user32.NewProc("GetWindowThreadProcessId")
	procIsWindowVisible     = user32.NewProc("IsWindowVisible")
	procGetWindow           = user32.NewProc("GetWindow")
	procGetAncestor         = user32.NewProc("GetAncestor")
	procGetWindowTextLength = user32.NewProc("GetWindowTextLengthW")
	procGetWindowText       = user32.NewProc("GetWindowTextW")
	procGetClassName        = user32.NewProc("GetClassNameW")
	procGetForegroundWindow = user32.NewProc("GetForegroundWindow")
	procIsIconic            = user32.NewProc("IsIconic")
	procShowWindow          = user32.NewProc("ShowWindow")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procFlashWindowEx       = user32.NewProc("FlashWindowEx")
	procKeybdEvent          = user32.NewProc("keybd_event")
	procFreeConsole         = kernel32.NewProc("FreeConsole")
	procAttachConsole       = kernel32.NewProc("AttachConsole")
	procGetConsoleWindow    = kernel32.NewProc("GetConsoleWindow")
)

const (
	gwOwner       = 4
	gaRootOwner   = 3
	swRestore     = 9
	vkControl     = 0x11
	vkMenu        = 0x12
	keyUp         = 2
	flashAll      = 3 // FLASHW_CAPTION | FLASHW_TRAY
	focusFlashN   = 3
	terminalClass = "CASCADIA_HOSTING_WINDOW_CLASS" // Windows Terminal
	maxTabs       = 9                               // Ctrl+Alt+1 à 9
	tabSettle     = 150 * time.Millisecond          // le titre de la fenêtre suit l'onglet avec un léger retard
)

// focusMu : AttachConsole vaut pour tout le processus, deux Focus ne s'y mêlent pas.
var focusMu sync.Mutex

// Focus met au premier plan le terminal de la session dont Claude Code est le
// processus pid, et fait clignoter sa fenêtre. title est le titre de la session
// (celui de son onglet) : dans Windows Terminal, qui abrite plusieurs fenêtres et
// onglets dans un seul processus, il sert à sélectionner l'onglet.
//
// La fenêtre se retrouve par la console de Claude Code : sous ConPTY, la fenêtre
// de pseudo-console a pour propriétaire la fenêtre du terminal. À défaut (VS Code…),
// on remonte les parents de pid jusqu'à un processus qui possède une fenêtre visible.
// Faux si aucune fenêtre n'est trouvée.
func Focus(pid uint32, title string) bool {
	focusMu.Lock()
	defer focusMu.Unlock()
	hwnd := consoleOwner(pid)
	if hwnd == 0 {
		hwnd = ancestorWindow(pid)
	}
	if hwnd == 0 {
		return false
	}
	raise(hwnd)
	if className(hwnd) == terminalClass && title != "" {
		selectTab(hwnd, title)
	}
	return true
}

// consoleOwner donne la fenêtre de premier niveau qui héberge la console de pid, ou 0.
func consoleOwner(pid uint32) uintptr {
	procFreeConsole.Call() // une seule console par processus ; l'application n'en a normalement pas
	if r, _, _ := procAttachConsole.Call(uintptr(pid)); r == 0 {
		return 0
	}
	cw, _, _ := procGetConsoleWindow.Call()
	procFreeConsole.Call()
	if cw == 0 {
		return 0
	}
	if top, _, _ := procGetAncestor.Call(cw, gaRootOwner); top != 0 && top != cw {
		return top
	}
	if v, _, _ := procIsWindowVisible.Call(cw); v != 0 { // console classique : la fenêtre elle-même
		return cw
	}
	return 0
}

// ancestorWindow remonte les parents de pid jusqu'au premier qui possède une
// fenêtre visible, sans propriétaire et titrée.
func ancestorWindow(pid uint32) uintptr {
	parents, _, err := table()
	if err != nil {
		return 0
	}
	for i := 0; i < 12 && pid != 0; i++ {
		if hwnd := windowOf(pid); hwnd != 0 {
			return hwnd
		}
		pid = parents[pid]
	}
	return 0
}

// windowOf donne une fenêtre de premier niveau visible du processus pid, ou 0.
func windowOf(pid uint32) (found uintptr) {
	cb := syscall.NewCallback(func(hwnd, _ uintptr) uintptr {
		var owner uint32
		procWindowThreadProcess.Call(hwnd, uintptr(unsafe.Pointer(&owner)))
		if owner != pid {
			return 1
		}
		if v, _, _ := procIsWindowVisible.Call(hwnd); v == 0 {
			return 1
		}
		if o, _, _ := procGetWindow.Call(hwnd, gwOwner); o != 0 {
			return 1
		}
		if n, _, _ := procGetWindowTextLength.Call(hwnd); n == 0 {
			return 1
		}
		found = hwnd
		return 0
	})
	procEnumWindows.Call(cb, 0)
	return found
}

func raise(hwnd uintptr) {
	if m, _, _ := procIsIconic.Call(hwnd); m != 0 {
		procShowWindow.Call(hwnd, swRestore)
	}
	// une touche Alt factice lève la restriction de Windows sur le vol de focus
	procKeybdEvent.Call(vkMenu, 0, 0, 0)
	procKeybdEvent.Call(vkMenu, 0, keyUp, 0)
	procSetForegroundWindow.Call(hwnd)
	fi := struct {
		size    uint32
		hwnd    uintptr
		flags   uint32
		count   uint32
		timeout uint32
	}{hwnd: hwnd, flags: flashAll, count: focusFlashN}
	fi.size = uint32(unsafe.Sizeof(fi))
	procFlashWindowEx.Call(uintptr(unsafe.Pointer(&fi)))
}

func className(hwnd uintptr) string { return text(procGetClassName, hwnd) }

func windowTitle(hwnd uintptr) string { return text(procGetWindowText, hwnd) }

func text(proc *syscall.LazyProc, hwnd uintptr) string {
	buf := make([]uint16, 256)
	proc.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return syscall.UTF16ToString(buf)
}

// selectTab amène l'onglet de titre title au premier plan dans la fenêtre hwnd de
// Windows Terminal, déjà au premier plan : le titre de la fenêtre est celui de
// l'onglet actif, on parcourt donc les onglets avec Ctrl+Alt+1 à 9 (raccourcis par
// défaut) jusqu'à le retrouver. Sans correspondance, on remet l'onglet d'origine.
func selectTab(hwnd uintptr, title string) {
	if SameTitle(windowTitle(hwnd), title) {
		return
	}
	time.Sleep(tabSettle)
	origin := windowTitle(hwnd)
	originTab := 0
	for n := 1; n <= maxTabs; n++ {
		if fg, _, _ := procGetForegroundWindow.Call(); fg != hwnd {
			return // le focus a été pris ailleurs : on n'envoie pas de touches à une autre fenêtre
		}
		chord(byte('0' + n))
		time.Sleep(tabSettle)
		got := windowTitle(hwnd)
		if SameTitle(got, title) {
			return
		}
		if originTab == 0 && got == origin {
			originTab = n
		}
	}
	if originTab != 0 {
		chord(byte('0' + originTab))
	}
}

// chord envoie Ctrl+Alt+touche.
func chord(key byte) {
	for _, k := range []byte{vkControl, vkMenu, key} {
		procKeybdEvent.Call(uintptr(k), 0, 0, 0)
	}
	for _, k := range []byte{key, vkMenu, vkControl} {
		procKeybdEvent.Call(uintptr(k), 0, keyUp, 0)
	}
}

// SameTitle dit si le titre d'un onglet (préfixé d'un symbole d'état : ✳, ◐…, et
// éventuellement tronqué) est celui de la session.
func SameTitle(tab, session string) bool {
	a, b := bareTitle(tab), bareTitle(session)
	return a != "" && b != "" && (strings.HasPrefix(a, b) || strings.HasPrefix(b, a))
}

func bareTitle(s string) string {
	s = strings.TrimLeftFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	return strings.ToLower(strings.TrimSpace(s))
}
