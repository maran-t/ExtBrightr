//go:build windows

// ExtBrightness: a tray icon with a brightness slider for external monitors.
//
// Left-click the icon for a flyout with one slider per external monitor;
// right-click for the menu. The laptop's own panel is never listed.
package main

import (
	"math"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

const (
	msgTray     = wmApp + 1 // tray icon callback
	msgMonitors = wmApp + 2 // the DDC worker published a new snapshot
	msgSlide    = wmApp + 3 // next frame of the opening slide
	msgRedraw   = wmApp + 4 // draw the flyout (see invalidate)

	timerRescan = 1

	menuAutostart = 1
	menuRescan    = 2
	menuExit      = 3

	trayID = 1
)

type row struct {
	name     string
	pct      int
	lastUser uint64 // tick of the user's last change, to ignore stale read-backs
}

type app struct {
	hInst    uintptr
	hwndMain uintptr // hidden; owns the tray icon and receives broadcasts
	hwndFly  uintptr
	exe      string

	build   uint32
	dark    bool
	accent  rgb
	icon    uintptr
	nid     notifyIconData
	msgTBar uint32 // "TaskbarCreated": Explorer restarted, re-add the icon

	ddc  *ddc
	gen  int
	rows []row

	visible  bool
	lastHide uint64
	anchor   point // tray icon centre when the flyout was opened
	work     rect  // that monitor's work area and full bounds
	full     rect
	scale    float64
	hover    int
	drag     int
	focus    int
	keyboard bool // show the focus highlight only once the keyboard is used
	tracking bool
	wheelAcc int

	fontScale     float64
	fonts         [3]uintptr
	redrawPending bool

	// The opening slide: the flyout rises from the taskbar into place.
	sliding     bool
	slideGen    uintptr
	slideStop   *atomic.Bool
	slideT0     float64 // perfSeconds at the start
	slideX      int32   // final position
	slideY      int32
	slideDX     float64 // starting offset from it, in pixels
	slideDY     float64
	slideBehind uintptr // taskbar to stay under while rising, or 0
}

var theApp *app

func main() {
	runtime.LockOSThread()

	// Resolve system DLLs from System32 only, never from the folder the
	// program happens to be run from.
	if available(pSetDefaultDllDirectories) {
		pSetDefaultDllDirectories.Call(0x00000800)
	}

	demo, showNow := false, false
	for _, arg := range os.Args[1:] {
		switch arg {
		case "--demo": // two pretend monitors, for trying the UI without hardware
			demo = true
		case "--show":
			showNow = true
		}
	}

	if h, _, err := pCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(utf16(mutexName)))); h != 0 {
		if errno, ok := err.(syscall.Errno); ok && errno == errAlreadyExist {
			return // already running
		}
	}

	// The manifest already declares per-monitor DPI awareness; this covers
	// builds made without it.
	if available(pSetProcessDpiAwarenessContext) {
		pSetProcessDpiAwarenessContext.Call(^uintptr(3)) // PER_MONITOR_AWARE_V2
	} else {
		pSetProcessDPIAware.Call()
	}

	a := &app{hover: -1, drag: -1, focus: 0, scale: 1}
	theApp = a
	a.hInst, _, _ = pGetModuleHandleW.Call(0)
	a.exe, _ = os.Executable()
	a.build = windowsBuild()
	a.dark = taskbarIsDark()
	a.accent = accentFor(a.dark)
	allowDarkMenus(a.build)

	// If the program was moved since autostart was switched on, keep the
	// startup entry pointing at where it lives now.
	if cmd, on := autostartEnabled(); on && a.exe != "" && !strings.EqualFold(cmd, autostartCommand(a.exe)) {
		setAutostart(a.exe, true)
	}

	a.createWindows()
	a.addTrayIcon()

	a.ddc = newDDC(func() { pPostMessageW.Call(a.hwndMain, msgMonitors, 0, 0) }, demo)
	go a.ddc.run()
	a.ddc.rescan()

	if showNow {
		a.showFlyout()
	}

	var m msgT
	for {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func (a *app) createWindows() {
	cursor, _, _ := pLoadCursorW.Call(0, idcArrow)

	mainClass := wndClassEx{
		lpfnWndProc:   syscall.NewCallback(mainProc),
		hInstance:     a.hInst,
		lpszClassName: utf16(classMain),
	}
	mainClass.cbSize = uint32(unsafe.Sizeof(mainClass))
	pRegisterClassExW.Call(uintptr(unsafe.Pointer(&mainClass)))

	// No shadow of any kind: the class shadow (CS_DROPSHADOW) is a separate
	// window that does not follow the flyout while it slides, and DWM shadows
	// the windows it rounds, so the flyout is a layered window with its
	// corners cut out by redraw.
	flyClass := wndClassEx{
		lpfnWndProc:   syscall.NewCallback(flyProc),
		hInstance:     a.hInst,
		hCursor:       cursor,
		lpszClassName: utf16(classFlyout),
	}
	flyClass.cbSize = uint32(unsafe.Sizeof(flyClass))
	pRegisterClassExW.Call(uintptr(unsafe.Pointer(&flyClass)))

	// Not a message-only window: those do not get broadcasts such as
	// WM_DISPLAYCHANGE or TaskbarCreated.
	a.hwndMain, _, _ = pCreateWindowExW.Call(0,
		uintptr(unsafe.Pointer(utf16(classMain))), uintptr(unsafe.Pointer(utf16(appTitle))),
		wsPopup, 0, 0, 0, 0, 0, 0, a.hInst, 0)

	a.hwndFly, _, _ = pCreateWindowExW.Call(wsExLayered|wsExToolWindow|wsExTopmost,
		uintptr(unsafe.Pointer(utf16(classFlyout))), uintptr(unsafe.Pointer(utf16(appTitle))),
		wsPopup, 0, 0, 10, 10, a.hwndMain, 0, a.hInst, 0)

	name, _, _ := pRegisterWindowMessageW.Call(uintptr(unsafe.Pointer(utf16("TaskbarCreated"))))
	a.msgTBar = uint32(name)
}

// rounded: Windows 11 flyouts have rounded corners; Windows 10 keeps them square.
func (a *app) rounded() bool { return a.build >= 22000 }

// --- tray icon ------------------------------------------------------------

func makeSunIcon(dark bool) uintptr {
	size, _, _ := pGetSystemMetrics.Call(smCxSmIcon)
	n := int(size)
	if n < 16 {
		n = 16
	}
	col, alpha := trayInk(dark)
	pix := renderSunIcon(n, col, alpha)

	bi := bitmapInfoHeader{biWidth: int32(n), biHeight: -int32(n), biPlanes: 1, biBitCount: 32}
	bi.biSize = uint32(unsafe.Sizeof(bi))
	var bits unsafe.Pointer
	color, _, _ := pCreateDIBSection.Call(0, uintptr(unsafe.Pointer(&bi)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if color == 0 || bits == nil {
		return 0
	}
	copy(unsafe.Slice((*byte)(bits), len(pix)), pix)

	maskBits := make([]byte, ((n+15)/16)*2*n) // all zero: alpha decides
	mask, _, _ := pCreateBitmap.Call(uintptr(n), uintptr(n), 1, 1, uintptr(unsafe.Pointer(&maskBits[0])))

	info := iconInfo{fIcon: 1, hbmMask: mask, hbmColor: color}
	icon, _, _ := pCreateIconIndirect.Call(uintptr(unsafe.Pointer(&info)))
	pDeleteObject.Call(color)
	pDeleteObject.Call(mask)
	return icon
}

func (a *app) addTrayIcon() {
	if a.icon == 0 {
		a.icon = makeSunIcon(a.dark)
	}
	a.nid = notifyIconData{
		hWnd:             a.hwndMain,
		uID:              trayID,
		uFlags:           nifMessage | nifIcon | nifTip,
		uCallbackMessage: msgTray,
		hIcon:            a.icon,
	}
	a.nid.cbSize = uint32(unsafe.Sizeof(a.nid))
	a.setTip()
	pShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&a.nid)))
}

func (a *app) setTip() {
	lines := []string{"Brightness"}
	if len(a.rows) == 0 {
		lines = []string{"Brightness: no external monitor found"}
	}
	for _, r := range a.rows {
		lines = append(lines, r.name+": "+strconv.Itoa(r.pct)+"%")
	}
	tip, _ := syscall.UTF16FromString(strings.Join(lines, "\n"))
	if len(tip) > len(a.nid.szTip) {
		tip = append(tip[:len(a.nid.szTip)-1], 0)
	}
	a.nid.szTip = [128]uint16{}
	copy(a.nid.szTip[:], tip)
}

func (a *app) updateTray() {
	a.setTip()
	a.nid.hIcon = a.icon
	pShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(&a.nid)))
}

func (a *app) reloadTheme() {
	dark := taskbarIsDark()
	accent := accentFor(dark)
	if dark == a.dark && accent == a.accent {
		return
	}
	if dark != a.dark {
		old := a.icon
		a.dark = dark
		a.icon = makeSunIcon(a.dark)
		a.updateTray()
		if old != 0 {
			pDestroyIcon.Call(old)
		}
	}
	a.accent = accent
	a.invalidate()
}

func (a *app) showMenu() {
	menu, _, _ := pCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	flags := uintptr(mfString)
	_, on := autostartEnabled()
	if on {
		flags |= mfChecked
	}
	pAppendMenuW.Call(menu, flags, menuAutostart, uintptr(unsafe.Pointer(utf16("Start with Windows"))))
	pAppendMenuW.Call(menu, mfString, menuRescan, uintptr(unsafe.Pointer(utf16("Rescan monitors"))))
	pAppendMenuW.Call(menu, mfSeparator, 0, 0)
	pAppendMenuW.Call(menu, mfString, menuExit, uintptr(unsafe.Pointer(utf16("Exit"))))

	var pt point
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	// The menu only dismisses on an outside click if its owner is foreground.
	pSetForegroundWindow.Call(a.hwndMain)
	cmd, _, _ := pTrackPopupMenu.Call(menu, tpmReturnCmd|tpmNoNotify|tpmRightBtn|tpmBottomAlgn,
		uintptr(pt.x), uintptr(pt.y), 0, a.hwndMain, 0)
	pPostMessageW.Call(a.hwndMain, 0, 0, 0)
	pDestroyMenu.Call(menu)

	switch cmd {
	case menuAutostart:
		setAutostart(a.exe, !on)
	case menuRescan:
		a.ddc.rescan()
	case menuExit:
		pDestroyWindow.Call(a.hwndMain)
	}
}

func mainProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	a := theApp
	switch uint32(msg) {
	case msgTray:
		switch uint32(lParam) {
		case wmLButtonUp:
			a.toggleFlyout()
		case wmRButtonUp, wmContextMenu:
			a.showMenu()
		}
		return 0
	case msgMonitors:
		a.monitorsChanged()
		return 0
	case wmDisplayChange:
		// Plugging a monitor fires several of these; wait for it to settle.
		pSetTimer.Call(hwnd, timerRescan, 1500, 0)
		return 0
	case wmPowerBroadcast:
		if wParam == pbtResumeAuto {
			pSetTimer.Call(hwnd, timerRescan, 4000, 0)
		}
		return 1
	case wmTimer:
		if wParam == timerRescan {
			pKillTimer.Call(hwnd, timerRescan)
			a.ddc.rescan()
		}
		return 0
	case wmSettingChange:
		a.reloadTheme()
		return 0
	case wmDestroy:
		pShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&a.nid)))
		pPostQuitMessage.Call(0)
		return 0
	}
	if a != nil && a.msgTBar != 0 && uint32(msg) == a.msgTBar {
		a.addTrayIcon()
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, msg, wParam, lParam)
	return r
}

// monitorsChanged merges a snapshot from the DDC worker into the UI state.
func (a *app) monitorsChanged() {
	gen, snap := a.ddc.snapshot()
	now := tickCount()
	if gen != a.gen || len(snap) != len(a.rows) {
		a.gen = gen
		a.rows = make([]row, len(snap))
		for i, m := range snap {
			a.rows[i] = row{name: m.name, pct: m.pct}
		}
		a.endDrag()
		a.hover = -1
		if a.focus >= len(a.rows) {
			a.focus = 0
		}
		if a.visible {
			a.finishSlide()
			x, y, w, h := a.flyoutRect()
			pSetWindowPos.Call(a.hwndFly, 0, uintptr(x), uintptr(y), uintptr(w), uintptr(h), swpNoZOrder|swpNoActivate)
		}
	} else {
		for i, m := range snap {
			// A read-back that raced with the user's own drag is stale.
			if now-a.rows[i].lastUser > 1500 {
				a.rows[i].pct = m.pct
			}
		}
	}
	a.invalidate()
	a.updateTray()
}

// --- flyout ---------------------------------------------------------------

func (a *app) toggleFlyout() {
	if a.visible {
		a.hideFlyout()
		return
	}
	// Clicking the icon while the flyout is open first deactivates (and so
	// hides) it; that same click must not reopen it.
	if tickCount()-a.lastHide < 400 {
		return
	}
	a.showFlyout()
}

// findAnchor works out where the tray icon is, which monitor's taskbar it sits
// on, and that monitor's DPI scale. If Explorer will not say where the icon
// is (it can be tucked into the overflow area), the pointer stands in for it.
func (a *app) findAnchor() {
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&a.anchor)))
	id := notifyIconIdentifier{hWnd: a.hwndMain, uID: trayID}
	id.cbSize = uint32(unsafe.Sizeof(id))
	var r rect
	if hr, _, _ := pShellNotifyIconGetRect.Call(uintptr(unsafe.Pointer(&id)), uintptr(unsafe.Pointer(&r))); hr == 0 && r.right > r.left {
		a.anchor = point{(r.left + r.right) / 2, (r.top + r.bottom) / 2}
	}

	hmon, _, _ := pMonitorFromPoint.Call(uintptr(uint32(a.anchor.x))|uintptr(uint32(a.anchor.y))<<32, monitorNearest)
	var mi monitorInfoEx
	mi.cbSize = uint32(unsafe.Sizeof(mi))
	pGetMonitorInfoW.Call(hmon, uintptr(unsafe.Pointer(&mi)))
	a.work, a.full = mi.rcWork, mi.rcMonitor

	a.scale = 1
	if available(pGetDpiForMonitor) {
		var dx, dy uint32
		if hr, _, _ := pGetDpiForMonitor.Call(hmon, 0, uintptr(unsafe.Pointer(&dx)), uintptr(unsafe.Pointer(&dy))); hr == 0 && dx >= 96 {
			a.scale = float64(dx) / 96
		}
	}
}

func clampI(v, lo, hi int32) int32 {
	if hi < lo {
		hi = lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// flyoutRect sizes the flyout for the current rows and tucks it against
// whichever screen edge the taskbar is on, as close to the icon as fits.
func (a *app) flyoutRect() (x, y int32, w, h int) {
	w, h = flyoutSize(len(a.rows), a.scale)
	W, H := int32(w), int32(h)
	work := a.work
	margin := int32(12*a.scale + 0.5)
	x = clampI(a.anchor.x-W/2, work.left+margin, work.right-W-margin)
	y = work.bottom - H - margin
	switch a.taskbarEdge() {
	case edgeTop:
		y = work.top + margin
	case edgeLeft:
		x = work.left + margin
		y = clampI(a.anchor.y-H/2, work.top+margin, work.bottom-H-margin)
	case edgeRight:
		x = work.right - W - margin
		y = clampI(a.anchor.y-H/2, work.top+margin, work.bottom-H-margin)
	}
	return x, y, w, h
}

const (
	edgeBottom = iota
	edgeTop
	edgeLeft
	edgeRight
)

// taskbarEdge says which side of the flyout's monitor the taskbar takes up,
// judged by where the work area stops short of the screen.
func (a *app) taskbarEdge() int {
	switch {
	case a.work.top > a.full.top:
		return edgeTop
	case a.work.left > a.full.left:
		return edgeLeft
	case a.work.right < a.full.right:
		return edgeRight
	}
	return edgeBottom
}

// --- opening slide ----------------------------------------------------------

const (
	slideSeconds = 0.25
	slideDIP     = 40.0 // how far the flyout rises from behind the taskbar
)

// taskbarWindow returns the taskbar the tray icon sits on (the main one or a
// secondary monitor's), or 0 if the icon is somewhere else, such as the
// overflow area.
func (a *app) taskbarWindow() uintptr {
	h, _, _ := pWindowFromPoint.Call(uintptr(uint32(a.anchor.x)) | uintptr(uint32(a.anchor.y))<<32)
	if h == 0 {
		return 0
	}
	root, _, _ := pGetAncestor.Call(h, gaRoot)
	var cls [64]uint16
	pGetClassNameW.Call(root, uintptr(unsafe.Pointer(&cls[0])), uintptr(len(cls)))
	switch syscall.UTF16ToString(cls[:]) {
	case "Shell_TrayWnd", "Shell_SecondaryTrayWnd":
		return root
	}
	return 0
}

// slideFrom works out where the opening slide starts, as an offset from the
// final position towards the taskbar. When the taskbar window is known the
// flyout starts partly behind it and stays under it until it lands, the way
// the Windows 11 flyouts come up; otherwise it only crosses the gap above the
// taskbar, so it never covers it. All zero when animations are switched off.
func (a *app) slideFrom() (dx, dy float64, behind uintptr) {
	if !animationsOn() {
		return 0, 0, 0
	}
	dist := 12 * a.scale // the margin flyoutRect leaves
	if behind = a.taskbarWindow(); behind != 0 {
		dist = slideDIP * a.scale
	}
	switch a.taskbarEdge() {
	case edgeTop:
		dy = -dist
	case edgeLeft:
		dx = -dist
	case edgeRight:
		dx = dist
	default:
		dy = dist
	}
	return dx, dy, behind
}

// startSlide runs the slide from the flyout's current, offset position to
// (x, y). Frames are paced by DwmFlush, which returns once the compositor
// has shown the next frame, so the motion steps once per display refresh.
func (a *app) startSlide(x, y int32, dx, dy float64, behind uintptr) {
	a.stopSlide()
	a.sliding = true
	a.slideX, a.slideY, a.slideDX, a.slideDY, a.slideBehind = x, y, dx, dy, behind
	a.slideT0 = perfSeconds()
	a.slideGen++
	gen, hwnd := a.slideGen, a.hwndFly
	stop := new(atomic.Bool)
	a.slideStop = stop
	go func() {
		deadline := time.Now().Add(time.Duration(slideSeconds*float64(time.Second)) + 500*time.Millisecond)
		for !stop.Load() && time.Now().Before(deadline) {
			if r, _, _ := pDwmFlush.Call(); r != 0 {
				time.Sleep(10 * time.Millisecond)
			}
			pPostMessageW.Call(hwnd, msgSlide, gen, 0)
		}
		pPostMessageW.Call(hwnd, msgSlide, gen, 1) // land, however it went
	}()
}

// stepSlide moves the flyout to where it should be now.
func (a *app) stepSlide(gen uintptr, last bool) {
	if !a.sliding || gen != a.slideGen {
		return
	}
	t := (perfSeconds() - a.slideT0) / slideSeconds
	if t >= 1 || last {
		a.finishSlide()
		return
	}
	// Ease out: quick to start, gentle landing.
	k := math.Pow(1-t, 3)
	x := a.slideX + int32(math.Round(a.slideDX*k))
	y := a.slideY + int32(math.Round(a.slideDY*k))
	insert, flags := a.slideBehind, uintptr(swpNoSize|swpNoActivate)
	if insert == 0 {
		flags |= swpNoZOrder
	}
	pSetWindowPos.Call(a.hwndFly, insert, uintptr(x), uintptr(y), 0, 0, flags)
}

// finishSlide puts the flyout at its final place, back above the taskbar.
func (a *app) finishSlide() {
	if !a.sliding {
		return
	}
	a.stopSlide()
	pSetWindowPos.Call(a.hwndFly, hwndTopmost, uintptr(a.slideX), uintptr(a.slideY), 0, 0, swpNoSize|swpNoActivate)
}

func (a *app) stopSlide() {
	if a.sliding {
		a.sliding = false
		a.slideStop.Store(true)
	}
}

func (a *app) showFlyout() {
	a.findAnchor()
	x, y, w, h := a.flyoutRect()

	a.hover, a.keyboard, a.wheelAcc = -1, false, 0
	a.visible = true

	dx, dy, behind := a.slideFrom()
	sx, sy := x+int32(math.Round(dx)), y+int32(math.Round(dy))
	insert := hwndTopmost
	if behind != 0 {
		insert = behind
	}
	// A layered window shows whatever it was last given, so place it and
	// draw it while it is still hidden, then show it.
	pSetWindowPos.Call(a.hwndFly, insert, uintptr(sx), uintptr(sy), uintptr(w), uintptr(h), swpNoActivate)
	a.redraw()
	pSetWindowPos.Call(a.hwndFly, insert, 0, 0, 0, 0, swpNoMove|swpNoSize|swpShowWindow)
	pSetForegroundWindow.Call(a.hwndFly)
	pSetFocus.Call(a.hwndFly)
	if behind != 0 {
		// Activating brought it to the top; tuck it back under the taskbar.
		pSetWindowPos.Call(a.hwndFly, behind, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate)
	}
	if sx != x || sy != y {
		a.startSlide(x, y, dx, dy, behind)
	}

	if len(a.rows) == 0 {
		a.ddc.rescan() // maybe it was plugged in or switched on since
	} else {
		a.ddc.refresh() // pick up changes made with the monitor's own buttons
	}
}

func (a *app) hideFlyout() {
	if !a.visible {
		return
	}
	a.visible = false
	a.lastHide = tickCount()
	a.endDrag()
	a.stopSlide()
	pShowWindow.Call(a.hwndFly, swHide)
	a.updateTray()
}

func (a *app) endDrag() {
	if a.drag >= 0 {
		a.drag = -1
		pReleaseCapture.Call()
	}
}

func (a *app) setPct(i, pct int) {
	if i < 0 || i >= len(a.rows) {
		return
	}
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	a.rows[i].lastUser = tickCount()
	if a.rows[i].pct == pct {
		return
	}
	a.rows[i].pct = pct
	a.ddc.set(a.gen, i, pct)
	a.invalidate()
}

func (a *app) ensureFonts() {
	if a.fontScale == a.scale && a.fonts[0] != 0 {
		return
	}
	for i, f := range a.fonts {
		if f != 0 {
			pDeleteObject.Call(f)
			a.fonts[i] = 0
		}
	}
	a.fontScale = a.scale
	a.fonts[styleTitle] = a.makeFont(14, 600)
	a.fonts[styleBody] = a.makeFont(14, 400)
	a.fonts[styleSmall] = a.makeFont(12, 400)
}

// makeFont prefers the Windows 11 UI face and falls back to Segoe UI. GDI
// silently substitutes when a face is missing, so check what it really gave.
func (a *app) makeFont(sizeDip float64, weight uintptr) uintptr {
	height := int32(-(sizeDip*a.scale + 0.5))
	create := func(face string) uintptr {
		f, _, _ := pCreateFontW.Call(uintptr(height), 0, 0, 0, weight, 0, 0, 0, defaultCharset,
			0, 0, cleartypeQual, 0, uintptr(unsafe.Pointer(utf16(face))))
		return f
	}
	if a.rounded() {
		const face = "Segoe UI Variable Text"
		if f := create(face); f != 0 {
			dc, _, _ := pGetDC.Call(0)
			old, _, _ := pSelectObject.Call(dc, f)
			var got [64]uint16
			pGetTextFaceW.Call(dc, uintptr(len(got)), uintptr(unsafe.Pointer(&got[0])))
			pSelectObject.Call(dc, old)
			pReleaseDC.Call(0, dc)
			if strings.EqualFold(syscall.UTF16ToString(got[:]), face) {
				return f
			}
			pDeleteObject.Call(f)
		}
	}
	return create("Segoe UI")
}

// invalidate asks for a redraw once the current message is handled. Like
// InvalidateRect for an ordinary window, several requests in a row draw once.
func (a *app) invalidate() {
	if a.redrawPending {
		return
	}
	a.redrawPending = true
	pPostMessageW.Call(a.hwndFly, msgRedraw, 0, 0)
}

// redraw renders the flyout and hands it to the layered window, which keeps
// no other copy of its contents. The corners are cut out here (cutCorners)
// rather than rounded by DWM: DWM gives the windows it rounds a shadow of
// its own, which showed below the flyout once it landed above the taskbar.
func (a *app) redraw() {
	a.redrawPending = false
	if !a.visible {
		return
	}
	w, h := flyoutSize(len(a.rows), a.scale)
	screen, _, _ := pGetDC.Call(0)
	if screen == 0 {
		return
	}
	defer pReleaseDC.Call(0, screen)

	bi := bitmapInfoHeader{biWidth: int32(w), biHeight: -int32(h), biPlanes: 1, biBitCount: 32}
	bi.biSize = uint32(unsafe.Sizeof(bi))
	var bits unsafe.Pointer
	bmp, _, _ := pCreateDIBSection.Call(screen, uintptr(unsafe.Pointer(&bi)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if bmp == 0 || bits == nil {
		return
	}
	defer pDeleteObject.Call(bmp)
	mem, _, _ := pCreateCompatibleDC.Call(screen)
	defer pDeleteDC.Call(mem)
	oldBmp, _, _ := pSelectObject.Call(mem, bmp)
	defer pSelectObject.Call(mem, oldBmp)

	model := &flyModel{
		dark: a.dark, rounded: a.rounded(), accent: a.accent, scale: a.scale,
		rows: make([]rowModel, len(a.rows)), hover: a.hover, press: a.drag, focus: -1,
	}
	if a.keyboard {
		model.focus = a.focus
	}
	for i, r := range a.rows {
		model.rows[i] = rowModel{r.name, r.pct}
	}
	cv := &canvas{pix: unsafe.Slice((*byte)(bits), w*h*4), w: w, h: h}
	texts := renderFlyout(cv, model)

	a.ensureFonts()
	pGdiFlush.Call()
	pSetBkMode.Call(mem, bkTransparent)
	oldFont, _, _ := pSelectObject.Call(mem, a.fonts[styleBody])
	for i := range texts {
		t := &texts[i]
		pSelectObject.Call(mem, a.fonts[t.style])
		pSetTextColor.Call(mem, colorref(t.col))
		flags := uintptr(dtNoPrefix)
		switch t.align {
		case alignLeft:
			flags |= dtSingleLine | dtVCenter | dtEndEllipsis
		case alignRight:
			flags |= dtSingleLine | dtVCenter | dtRight
		case alignCenterWrap:
			flags |= dtCenter | dtWordBreak
		}
		pDrawTextW.Call(mem, uintptr(unsafe.Pointer(utf16(t.s))), ^uintptr(0), uintptr(unsafe.Pointer(&t.box)), flags)
	}
	pSelectObject.Call(mem, oldFont)
	pGdiFlush.Call()

	radius := 0.0
	if model.rounded {
		radius = cornerR * a.scale
	}
	cv.cutCorners(radius)

	size := [2]int32{int32(w), int32(h)} // SIZE
	var src point
	blend := [4]byte{0, 0, 255, 1} // AC_SRC_OVER, no flags, opaque, AC_SRC_ALPHA
	pUpdateLayeredWindow.Call(a.hwndFly, screen, 0, uintptr(unsafe.Pointer(&size)), mem,
		uintptr(unsafe.Pointer(&src)), 0, uintptr(unsafe.Pointer(&blend)), ulwAlpha)
}

func keyDown(vk uintptr) bool {
	s, _, _ := pGetKeyState.Call(vk)
	return int16(s) < 0
}

func flyProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	a := theApp
	switch uint32(msg) {
	case wmEraseBkgnd:
		return 1
	case wmPaint:
		// Nothing to paint: redraw feeds the layered window directly.
		var ps paintStruct
		pBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	case msgRedraw:
		a.redraw()
		return 0
	case msgSlide:
		a.stepSlide(wParam, lParam != 0)
		return 0

	case wmActivate:
		if loword(wParam) == 0 { // WA_INACTIVE: clicked elsewhere
			a.hideFlyout()
			return 0
		}
		// Fall through: DefWindowProc gives the window keyboard focus.

	case wmLButtonDown:
		x, y := lparamXY(lParam)
		i := rowAt(len(a.rows), a.scale, float64(y))
		if i >= 0 {
			g := rowGeometry(i, a.scale)
			if onSlider(g, a.scale, float64(x), float64(y)) {
				a.drag, a.focus, a.keyboard = i, i, false
				pSetCapture.Call(hwnd)
				a.setPct(i, pctFromX(g, float64(x)))
				a.invalidate()
			}
		}
		return 0

	case wmMouseMove:
		x, y := lparamXY(lParam)
		if a.drag >= 0 {
			a.setPct(a.drag, pctFromX(rowGeometry(a.drag, a.scale), float64(x)))
			return 0
		}
		hover := -1
		if i := rowAt(len(a.rows), a.scale, float64(y)); i >= 0 && onSlider(rowGeometry(i, a.scale), a.scale, float64(x), float64(y)) {
			hover = i
		}
		if hover != a.hover {
			a.hover = hover
			a.invalidate()
		}
		if !a.tracking {
			tme := trackMouseEventT{dwFlags: tmeLeave, hwndTrack: hwnd}
			tme.cbSize = uint32(unsafe.Sizeof(tme))
			pTrackMouseEvent.Call(uintptr(unsafe.Pointer(&tme)))
			a.tracking = true
		}
		return 0

	case wmMouseLeave:
		a.tracking = false
		if a.hover != -1 {
			a.hover = -1
			a.invalidate()
		}
		return 0

	case wmLButtonUp:
		a.endDrag()
		a.invalidate()
		return 0

	case wmCaptureChanged:
		if a.drag >= 0 {
			a.drag = -1
			a.invalidate()
		}
		return 0

	case wmMouseWheel:
		// Wheel coordinates are in screen space.
		sx, sy := lparamXY(lParam)
		pt := point{sx, sy}
		pScreenToClient.Call(hwnd, uintptr(unsafe.Pointer(&pt)))
		i := rowAt(len(a.rows), a.scale, float64(pt.y))
		if i < 0 {
			i = a.focus
		}
		// One wheel notch is 120 units and moves 5%; precision touchpads
		// send many small deltas, so carry the remainder.
		a.wheelAcc += int(int16(hiword(wParam)))
		steps := a.wheelAcc * 5 / 120
		a.wheelAcc -= steps * 120 / 5
		if steps != 0 && i >= 0 && i < len(a.rows) {
			a.focus = i
			a.setPct(i, a.rows[i].pct+steps)
		}
		return 0

	case wmKeyDown:
		n := len(a.rows)
		if wParam == vkEscape {
			a.hideFlyout()
			return 0
		}
		if n == 0 {
			return 0
		}
		if a.focus < 0 || a.focus >= n {
			a.focus = 0
		}
		step := 5
		if keyDown(vkShift) {
			step = 1 // fine adjustment
		}
		cur := a.rows[a.focus].pct
		switch wParam {
		case vkLeft:
			a.setPct(a.focus, cur-step)
		case vkRight:
			a.setPct(a.focus, cur+step)
		case vkNext:
			a.setPct(a.focus, cur-20)
		case vkPrior:
			a.setPct(a.focus, cur+20)
		case vkHome:
			a.setPct(a.focus, 0)
		case vkEnd:
			a.setPct(a.focus, 100)
		case vkUp:
			a.focus = (a.focus + n - 1) % n
		case vkDown, vkTab:
			a.focus = (a.focus + 1) % n
		default:
			return 0
		}
		a.keyboard = true
		a.invalidate()
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, msg, wParam, lParam)
	return r
}
