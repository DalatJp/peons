package main

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// L'icône de la zone de notification et son menu : Réglages, Toujours afficher
// (case cochée selon les préférences), Quitter. Le menu s'ouvre au clic, gauche
// ou droit, sur l'icône.

const (
	nimAdd    = 0
	nimDelete = 2

	nifMessage = 0x1
	nifIcon    = 0x2
	nifTip     = 0x4

	wmLButtonUpTray = 0x0202
	wmRButtonUpTray = 0x0205

	mfString  = 0x0000
	mfChecked = 0x0008

	tpmRightButton = 0x0002
	tpmReturnCmd   = 0x0100

	cmdOpen   = 1
	cmdAlways = 2
	cmdQuit   = 3
)

// notifyIconData : NOTIFYICONDATAW (version Vista et suivantes).
type notifyIconData struct {
	size             uint32
	hwnd             uintptr
	id, flags        uint32
	callbackMessage  uint32
	icon             uintptr
	tip              [128]uint16
	state, stateMask uint32
	info             [256]uint16
	timeout          uint32
	infoTitle        [64]uint16
	infoFlags        uint32
	guid             [16]byte
	balloonIcon      uintptr
}

type tray struct {
	a    *app
	data notifyIconData
}

// newTray crée l'icône (corps normal, yeux neutres) ; nil si elle ne peut pas l'être.
func (a *app) newTray() *tray {
	img, err := a.char.Compose("normal", 0, "neutral")
	if err != nil {
		return nil
	}
	icon := iconFromImage(img)
	if icon == 0 {
		return nil
	}
	t := &tray{a: a}
	t.data = notifyIconData{hwnd: uiWnd.Load(), id: 1, flags: nifMessage | nifIcon | nifTip, callbackMessage: wmTray, icon: icon}
	t.data.size = uint32(unsafe.Sizeof(t.data))
	copy(t.data.tip[:], windows.StringToUTF16("peons"))
	if !t.add() {
		logf("icône de notification refusée par Windows") // l'Explorateur n'est peut-être pas prêt : TaskbarCreated la reposera
	}
	return t
}

// add (re)pose l'icône ; refait après un redémarrage de l'Explorateur.
func (t *tray) add() bool {
	r, _, _ := procShellNotifyIcon.Call(nimAdd, uintptr(unsafe.Pointer(&t.data)))
	return r != 0
}

func (t *tray) Destroy() {
	procShellNotifyIcon.Call(nimDelete, uintptr(unsafe.Pointer(&t.data)))
	procDestroyIcon.Call(t.data.icon)
}

// onMessage traite un message de l'icône (lparam : le message de souris).
func (t *tray) onMessage(lparam uintptr) {
	switch lparam & 0xFFFF {
	case wmLButtonUpTray, wmRButtonUpTray:
		t.showMenu()
	}
}

// showMenu ouvre le menu sous la souris. TrackPopupMenu garde la main jusqu'au
// choix : la commande est notée pour la boucle (runTrayActions).
func (t *tray) showMenu() {
	menu, _, _ := procCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer procDestroyMenu.Call(menu)
	add := func(flags, id uintptr, text string) {
		p, _ := windows.UTF16PtrFromString(text)
		procAppendMenu.Call(menu, flags, id, uintptr(unsafe.Pointer(p)))
	}
	always := uintptr(mfString)
	if t.a.prefs.AlwaysShow {
		always |= mfChecked
	}
	add(mfString, cmdOpen, "Réglages")
	add(always, cmdAlways, "Toujours afficher")
	add(mfString, cmdQuit, "Quitter")

	var pt winPoint
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	hwnd := t.data.hwnd
	procSetForegroundWindow.Call(hwnd) // sans cela, le menu ne se ferme pas au clic ailleurs
	cmd, _, _ := procTrackPopupMenu.Call(menu, tpmRightButton|tpmReturnCmd, uintptr(pt.x), uintptr(pt.y), 0, hwnd, 0)
	procPostMessage.Call(hwnd, wmNull, 0, 0)
	switch cmd {
	case cmdOpen:
		pendingActions.Or(actOpenSettings)
	case cmdAlways:
		pendingActions.Or(actToggleAlways)
	case cmdQuit:
		quit.Store(true)
	default:
		return
	}
	wake()
}
