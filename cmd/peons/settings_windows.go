package main

import (
	"image"
	"time"
	"unsafe"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/sys/windows"
)

const (
	// WS_OVERLAPPEDWINDOW sans WS_MAXIMIZEBOX : agrandir ferait aussi varier la largeur
	wsOverlappedWindow = 0x00CE0000

	// résultats de WM_NCHITTEST sur les bords et les coins
	htBorder      = 5
	htLeft        = 10
	htRight       = 11
	htTop         = 12
	htTopLeft     = 13
	htTopRight    = 14
	htBottom      = 15
	htBottomLeft  = 16
	htBottomRight = 17

	wmGetMinMaxInfo = 0x0024
	wmKeyDown       = 0x0100
	wmMouseWheel    = 0x020A
	vkEscape        = 0x1B

	swShow    = 5
	swRestore = 9

	iconSmall = 0
	iconBig   = 1

	srcCopy = 0x00CC0020

	hwndNoTopmost = ^uintptr(1) // (HWND)-2
)

var settingsProc = windows.NewCallback(settingsWndProc)

// paintStruct : PAINTSTRUCT.
type paintStruct struct {
	hdc       uintptr
	erase     int32
	paint     winRect
	restore   int32
	incUpdate int32
	reserved  [32]byte
}

// minMaxInfo : MINMAXINFO.
type minMaxInfo struct {
	reserved, maxSize, maxPosition, minTrack, maxTrack winPoint
}

// openSettings ouvre la fenêtre, ou la ramène devant si elle l'est déjà.
func (a *app) openSettings() {
	if a.set != nil {
		a.set.raise()
		return
	}
	scale := a.ov.DisplayScale()
	s := &settings{a: a, scale: scale}
	regular, err := opentype.Parse(goregular.TTF)
	if err != nil {
		logf("réglages : police illisible : %v", err)
		return
	}
	bold, err := opentype.Parse(gobold.TTF)
	if err != nil {
		logf("réglages : police illisible : %v", err)
		return
	}
	for i := range s.faces {
		f := regular
		if styleBoldFace[i] {
			f = bold
		}
		s.faces[i], err = opentype.NewFace(f, &opentype.FaceOptions{Size: styleSize[i] * scale, DPI: 72, Hinting: font.HintingFull})
		if err != nil {
			logf("réglages : police : %v", err)
			return
		}
	}
	s.W = s.p(setW)
	// sous l'overlay, comme le panneau de l'ancienne app (ou là où il apparaîtra si caché)
	x, y := a.centerX-s.W/2, a.top+10
	if a.shown {
		ox, oy := a.ov.Position()
		x, y = ox+a.W/2-s.W/2, oy+a.H+10
	}
	// hauteur de départ : celle du contenu, plafonnée à la hauteur utile de l'écran ;
	// au-delà la fenêtre défile. Elle ne se redimensionne ensuite qu'en hauteur.
	s.refreshHook() // avant le build : la hauteur estimée dépend de l'état du hook
	s.H = s.build(time.Now())
	if _, work, ok := monitorBounds(monitorFromPoint(image.Pt(x, y), monitorNearest)); ok {
		s.H = min(s.H, work.Dy()-s.p(40))
		x = max(work.Min.X, min(x, work.Max.X-s.W))
		y = max(work.Min.Y, min(y, work.Max.Y-s.H))
	}
	// la taille voulue est celle de la zone cliente : la fenêtre est plus grande de ses bordures
	dpi := uintptr(96 * scale)
	frame := s.frame(s.W, s.H, dpi)
	inst, _, _ := procGetModuleHandle.Call(0)
	class, _ := windows.UTF16PtrFromString("peonsSettings")
	if !registerSettingsClass(inst, class) {
		logf("réglages : classe de fenêtre refusée")
		s.closeFaces()
		return
	}
	title, _ := windows.UTF16PtrFromString("peons — réglages")
	hwnd, _, err := procCreateWindow.Call(0, uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(title)), wsOverlappedWindow,
		uintptr(int32(x+frame.Min.X)), uintptr(int32(y+frame.Min.Y)), uintptr(frame.Dx()), uintptr(frame.Dy()), 0, 0, inst, 0)
	if hwnd == 0 {
		logf("réglages : fenêtre : %v", err)
		s.closeFaces()
		return
	}
	s.hwnd = hwnd
	a.set = s // avant l'affichage : la fenêtre reçoit tout de suite ses premiers messages
	if img, err := a.char.Compose("normal", 0, "neutral"); err == nil {
		if s.icon = iconFromImage(img); s.icon != 0 {
			procSendMessage.Call(hwnd, wmSetIcon, iconBig, s.icon)
			procSendMessage.Call(hwnd, wmSetIcon, iconSmall, s.icon)
		}
	}
	var rc winRect
	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
	s.W, s.H = int(rc.right), int(rc.bottom)
	s.raise()
}

var settingsClassDone bool

func registerSettingsClass(inst uintptr, name *uint16) bool {
	if settingsClassDone {
		return true
	}
	cursor, _, _ := procLoadCursor.Call(0, idcArrow)
	class := wndClassEx{wndProc: settingsProc, instance: inst, cursor: cursor, className: name}
	class.size = uint32(unsafe.Sizeof(class))
	if r, _, _ := procRegisterClass.Call(uintptr(unsafe.Pointer(&class))); r == 0 {
		return false
	}
	settingsClassDone = true
	return true
}

// frame donne le rectangle de la fenêtre entière, relatif au coin haut-gauche de sa
// zone cliente, pour une zone cliente de w×h.
func (s *settings) frame(w, h int, dpi uintptr) image.Rectangle {
	rc := winRect{0, 0, int32(w), int32(h)}
	procAdjustWindowRect.Call(uintptr(unsafe.Pointer(&rc)), wsOverlappedWindow, 0, 0, dpi)
	return rc.rect()
}

// raise montre la fenêtre devant les autres. Passer un instant « toujours au-dessus »
// la place devant les fenêtres ordinaires même si Windows refuse de lui donner le
// focus ; sinon elle peut s'ouvrir derrière l'application active.
func (s *settings) raise() {
	if r, _, _ := procIsIconic.Call(s.hwnd); r != 0 {
		procShowWindow.Call(s.hwnd, swRestore)
	} else {
		procShowWindow.Call(s.hwnd, swShow)
	}
	const flags = swpNoMove | swpNoSize
	procSetWindowPos.Call(s.hwnd, hwndTopmost, 0, 0, 0, 0, flags)
	procSetWindowPos.Call(s.hwnd, hwndNoTopmost, 0, 0, 0, 0, flags)
	procSetForegroundWindow.Call(s.hwnd)
}

// present recopie l'image dans la fenêtre.
func (s *settings) present() {
	if s.dib.memDC == 0 {
		return
	}
	dc, _, _ := procGetDC.Call(s.hwnd)
	if dc == 0 {
		return
	}
	procBitBlt.Call(dc, 0, 0, uintptr(s.dib.w), uintptr(s.dib.h), s.dib.memDC, 0, 0, srcCopy)
	procReleaseDC.Call(s.hwnd, dc)
}

func (s *settings) closeFaces() {
	for _, f := range s.faces {
		if f != nil {
			f.Close()
		}
	}
}

// closeSettings détruit la fenêtre et tout ce qu'elle avait alloué.
func (a *app) closeSettings() {
	s := a.set
	if s == nil {
		return
	}
	a.set = nil // les messages de la destruction ne sont plus pour elle
	procDestroyWindow.Call(s.hwnd)
	if s.icon != 0 {
		procDestroyIcon.Call(s.icon)
	}
	s.closeFaces()
	s.dib.free()
	s.canvas = nil
}

// settingsWndProc : les messages de la fenêtre de réglages. Tout tourne sur le fil
// principal, entre deux tours de la boucle, qui redessine ensuite.
func settingsWndProc(hwnd, msg, wparam, lparam uintptr) uintptr {
	var s *settings
	if a := uiApp; a != nil && a.set != nil && a.set.hwnd == hwnd {
		s = a.set
	}
	if s != nil {
		switch msg {
		case wmClose: // détruite par la boucle, pas depuis sa propre procédure
			pendingActions.Or(actCloseSettings)
			wake()
			return 0
		case wmKeyDown:
			if wparam == vkEscape {
				pendingActions.Or(actCloseSettings)
				wake()
			}
			return 0
		case wmEraseBkgnd:
			return 1
		case wmGetMinMaxInfo: // largeur figée, seule la hauteur varie
			info := (*minMaxInfo)(*(*unsafe.Pointer)(unsafe.Pointer(&lparam)))
			f := s.frame(s.p(setW), s.p(160), uintptr(96*s.scale))
			info.minTrack = winPoint{int32(f.Dx()), int32(f.Dy())}
			info.maxTrack.x = int32(f.Dx())
			return 0
		case wmNcHitTest: // pas de poignée latérale : plus de flèche sur les bords gauche et droit
			r, _, _ := procDefWindowProc.Call(hwnd, msg, wparam, lparam)
			switch r {
			case htLeft, htRight:
				return htBorder
			case htTopLeft, htTopRight:
				return htTop
			case htBottomLeft, htBottomRight:
				return htBottom
			}
			return r
		case wmSize:
			if w, h := int(lparam&0xFFFF), int(lparam>>16&0xFFFF); w > 0 && h > 0 {
				s.W, s.H = w, h
				s.draw(time.Now()) // la boucle est suspendue pendant qu'on tire sur un bord
			}
			return 0
		case wmPaint:
			var ps paintStruct
			procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
			s.present()
			procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
			return 0
		case wmMouseMove:
			s.pointerMoved(int(int16(lparam)), int(int16(lparam>>16)))
			if !s.tracking {
				s.tracking = true
				tme := trackMouseEvent{flags: tmeLeave, hwnd: hwnd}
				tme.size = uint32(unsafe.Sizeof(tme))
				procTrackMouseEvent.Call(uintptr(unsafe.Pointer(&tme)))
			}
			return 0
		case wmMouseLeave:
			s.hasMouse, s.tracking = false, false
			return 0
		case wmMouseWheel:
			s.wheel(float64(int16(wparam>>16)) / 120)
			return 0
		case wmLButtonUp:
			s.release(int(int16(lparam)), int(int16(lparam>>16)))
			return 0
		}
	}
	r, _, _ := procDefWindowProc.Call(hwnd, msg, wparam, lparam)
	return r
}
