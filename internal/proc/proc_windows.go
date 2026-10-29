package proc

import (
	"os"
	"strings"
	"syscall"
	"unsafe"
)

const (
	processQueryLimitedInformation = 0x1000
	stillActive                    = 259
	errorAccessDenied              = syscall.Errno(5)
)

// FindClaude remonte les parents du processus courant jusqu'à Claude Code et
// donne son PID et sa date de création (FILETIME UTC, tel que GetProcessTimes
// la rend).
func FindClaude() (pid uint32, started int64, ok bool) {
	parents, names, err := table()
	if err != nil {
		return 0, 0, false
	}
	var nodeFallback uint32
	pid = uint32(os.Getpid())
	for i := 0; i < 8; i++ {
		parent, ok := parents[pid]
		if !ok || parent == 0 {
			break
		}
		pid = parent
		switch strings.ToLower(names[pid]) {
		case "claude.exe":
			t, ok := creationTime(pid)
			return pid, t, ok
		case "node.exe":
			// une installation npm lance Claude Code sous node : on garde le plus proche
			if nodeFallback == 0 {
				nodeFallback = pid
			}
		}
	}
	if nodeFallback != 0 {
		if t, ok := creationTime(nodeFallback); ok {
			return nodeFallback, t, true
		}
	}
	return 0, 0, false
}

// Alive dit si le processus existe encore. started (date de création) évite de
// confondre avec un autre processus qui aurait repris le PID ; 0 l'ignore.
func Alive(pid uint32, started int64) bool {
	h, err := syscall.OpenProcess(processQueryLimitedInformation, false, pid)
	if err != nil {
		// accès refusé : il existe mais appartient à quelqu'un d'autre
		return err == errorAccessDenied
	}
	defer syscall.CloseHandle(h)
	var code uint32
	if syscall.GetExitCodeProcess(h, &code) == nil && code != stillActive {
		return false
	}
	if started == 0 {
		return true
	}
	t, ok := creationOf(h)
	return !ok || t == started
}

func table() (parents map[uint32]uint32, names map[uint32]string, err error) {
	snap, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, nil, err
	}
	defer syscall.CloseHandle(snap)
	parents, names = map[uint32]uint32{}, map[uint32]string{}
	var e syscall.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = syscall.Process32First(snap, &e); err == nil; err = syscall.Process32Next(snap, &e) {
		parents[e.ProcessID] = e.ParentProcessID
		names[e.ProcessID] = syscall.UTF16ToString(e.ExeFile[:])
	}
	return parents, names, nil
}

func creationTime(pid uint32) (int64, bool) {
	h, err := syscall.OpenProcess(processQueryLimitedInformation, false, pid)
	if err != nil {
		return 0, false
	}
	defer syscall.CloseHandle(h)
	return creationOf(h)
}

func creationOf(h syscall.Handle) (int64, bool) {
	var creation, exit, kernel, user syscall.Filetime
	if syscall.GetProcessTimes(h, &creation, &exit, &kernel, &user) != nil {
		return 0, false
	}
	return int64(creation.HighDateTime)<<32 | int64(creation.LowDateTime), true
}

var (
	user32                  = syscall.NewLazyDLL("user32.dll")
	procEnumWindows         = user32.NewProc("EnumWindows")
	procWindowThreadProcess = user32.NewProc("GetWindowThreadProcessId")
	procIsWindowVisible     = user32.NewProc("IsWindowVisible")
	procGetWindow           = user32.NewProc("GetWindow")
	procGetWindowTextLength = user32.NewProc("GetWindowTextLengthW")
	procIsIconic            = user32.NewProc("IsIconic")
	procShowWindow          = user32.NewProc("ShowWindow")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procFlashWindowEx       = user32.NewProc("FlashWindowEx")
	procKeybdEvent          = user32.NewProc("keybd_event")
)

const (
	gwOwner     = 4
	swRestore   = 9
	vkMenu      = 0x12
	keyUp       = 2
	flashAll    = 3 // FLASHW_CAPTION | FLASHW_TRAY
	focusFlashN = 3
)

// Focus met au premier plan la fenêtre du terminal qui héberge le processus pid
// (Claude Code) et la fait clignoter : on remonte ses parents jusqu'au premier qui
// possède une fenêtre visible, sans propriétaire et titrée (Windows Terminal,
// VS Code…). Une console classique (conhost) n'est pas dans la chaîne des parents
// et n'est pas retrouvée. Faux si aucune fenêtre n'est trouvée.
func Focus(pid uint32) bool {
	parents, _, err := table()
	if err != nil {
		return false
	}
	for i := 0; i < 12 && pid != 0; i++ {
		if hwnd := windowOf(pid); hwnd != 0 {
			raise(hwnd)
			return true
		}
		pid = parents[pid]
	}
	return false
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
