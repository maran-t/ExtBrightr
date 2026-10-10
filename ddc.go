//go:build windows

package main

import (
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// Brightness is VCP feature 0x10 in the VESA Monitor Control Command Set.
// It travels over DDC/CI, the I2C side channel in the DisplayPort/HDMI cable.
// Laptop panels are not driven this way, which is why the built-in Windows
// slider only ever moves the laptop screen.
const vcpBrightness = 0x10

type uiMon struct {
	name string
	pct  int
}

type physMon struct {
	name   string
	handle uintptr
	max    uint32
	pct    int
}

// ddc owns every monitor handle and does all DDC/CI I/O on its own goroutine.
// Each command takes tens of milliseconds, so the UI thread never waits on
// it: it records what it wants and the worker catches up, always writing the
// newest value and skipping the ones in between.
type ddc struct {
	notify func() // called after a new snapshot is published
	demo   bool

	mu          sync.Mutex
	gen         int
	snap        []uiMon
	want        map[int]int
	needRescan  bool
	needRefresh bool

	wake         chan struct{}
	mons         []physMon // worker only
	lastRecovery time.Time
}

func newDDC(notify func(), demo bool) *ddc {
	return &ddc{notify: notify, demo: demo, want: map[int]int{}, wake: make(chan struct{}, 1)}
}

func (d *ddc) poke() {
	select {
	case d.wake <- struct{}{}:
	default:
	}
}

// set asks for monitor idx (as numbered in generation gen) to go to pct.
func (d *ddc) set(gen, idx, pct int) {
	d.mu.Lock()
	if gen == d.gen {
		d.want[idx] = pct
	}
	d.mu.Unlock()
	d.poke()
}

// rescan re-enumerates monitors (after plugging, unplugging, waking).
func (d *ddc) rescan() {
	d.mu.Lock()
	d.needRescan = true
	d.mu.Unlock()
	d.poke()
}

// refresh re-reads current levels, in case they were changed on the monitor.
func (d *ddc) refresh() {
	d.mu.Lock()
	d.needRefresh = true
	d.mu.Unlock()
	d.poke()
}

func (d *ddc) snapshot() (int, []uiMon) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.gen, append([]uiMon(nil), d.snap...)
}

func (d *ddc) run() {
	for range d.wake {
		for {
			d.mu.Lock()
			rescan, refresh, want := d.needRescan, d.needRefresh, d.want
			d.needRescan, d.needRefresh, d.want = false, false, map[int]int{}
			d.mu.Unlock()

			if rescan {
				d.doRescan()
				continue
			}
			if !refresh && len(want) == 0 {
				break
			}
			failed := false
			for idx, pct := range want {
				if idx < len(d.mons) && !d.write(&d.mons[idx], pct) {
					failed = true
				}
			}
			if refresh {
				for i := range d.mons {
					if _, isPending := d.pending(i); isPending {
						continue // a newer write is queued; its value wins
					}
					if cur, max, ok := d.read(&d.mons[i]); ok {
						d.mons[i].max, d.mons[i].pct = max, toPct(cur, max)
					} else {
						failed = true
					}
				}
			}
			// A monitor that stopped answering has usually been unplugged
			// or re-enumerated by the driver; look again, but not in a loop.
			if failed && time.Since(d.lastRecovery) > 5*time.Second {
				d.lastRecovery = time.Now()
				d.doRescan()
				continue
			}
			if refresh {
				d.publish(false)
			}
		}
	}
}

func (d *ddc) pending(idx int) (int, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	v, ok := d.want[idx]
	return v, ok
}

func (d *ddc) publish(newGen bool) {
	snap := make([]uiMon, len(d.mons))
	for i, m := range d.mons {
		snap[i] = uiMon{m.name, m.pct}
	}
	d.mu.Lock()
	if newGen {
		d.gen++
		d.want = map[int]int{}
	}
	d.snap = snap
	d.mu.Unlock()
	d.notify()
}

func (d *ddc) doRescan() {
	if d.demo {
		if d.mons == nil {
			d.mons = []physMon{{name: "MAG 255F", max: 100, pct: 60}, {name: "DELL U2720Q", max: 100, pct: 35}}
		}
		d.publish(true)
		return
	}
	for _, m := range d.mons {
		pDestroyPhysicalMonitor.Call(m.handle)
	}
	d.mons = enumerateExternal()
	d.publish(true)
}

func toPct(cur, max uint32) int {
	if max == 0 {
		return 0
	}
	p := int((uint64(cur)*100 + uint64(max)/2) / uint64(max))
	if p > 100 {
		p = 100
	}
	return p
}

func (d *ddc) read(m *physMon) (cur, max uint32, ok bool) {
	if d.demo {
		return uint32(m.pct), 100, true
	}
	return getBrightness(m.handle)
}

func (d *ddc) write(m *physMon, pct int) bool {
	if d.demo {
		time.Sleep(30 * time.Millisecond)
		m.pct = pct
		return true
	}
	raw := (uint64(pct)*uint64(m.max) + 50) / 100
	for attempt := 0; attempt < 3; attempt++ {
		r, _, _ := pSetVCPFeature.Call(m.handle, vcpBrightness, uintptr(raw))
		// Monitors need a moment between DDC/CI commands.
		time.Sleep(40 * time.Millisecond)
		if r != 0 {
			m.pct = pct
			return true
		}
	}
	return false
}

func getBrightness(handle uintptr) (cur, max uint32, ok bool) {
	for attempt := 0; attempt < 3; attempt++ {
		r, _, _ := pGetVCPFeatureAndReply.Call(handle, vcpBrightness, 0,
			uintptr(unsafe.Pointer(&cur)), uintptr(unsafe.Pointer(&max)))
		if r != 0 && max > 0 {
			return cur, max, true
		}
		time.Sleep(40 * time.Millisecond)
	}
	return 0, 0, false
}

// --- enumeration ----------------------------------------------------------

type targetInfo struct {
	internal bool
	name     string
}

// displayTargets maps each GDI display name (\\.\DISPLAY1) to the physical
// outputs showing it, with the connector type and the EDID name. More than
// one output per display means the desktop is duplicated. ok is false if the
// display configuration could not be read at all.
func displayTargets() (targets map[string][]targetInfo, ok bool) {
	if !available(pQueryDisplayConfig) {
		return nil, false
	}
	for attempt := 0; attempt < 4; attempt++ {
		var numPaths, numModes uint32
		r, _, _ := pGetDisplayConfigBufferSizes.Call(qdcOnlyActivePaths,
			uintptr(unsafe.Pointer(&numPaths)), uintptr(unsafe.Pointer(&numModes)))
		if r != 0 {
			return nil, false
		}
		paths := make([]dcPathInfo, numPaths+1)
		modes := make([]dcModeInfo, numModes+1)
		r, _, _ = pQueryDisplayConfig.Call(qdcOnlyActivePaths,
			uintptr(unsafe.Pointer(&numPaths)), uintptr(unsafe.Pointer(&paths[0])),
			uintptr(unsafe.Pointer(&numModes)), uintptr(unsafe.Pointer(&modes[0])), 0)
		if r == errInsufficientBuffer {
			continue // the layout changed between the two calls
		}
		if r != 0 {
			return nil, false
		}
		targets = map[string][]targetInfo{}
		for i := range paths[:numPaths] {
			p := &paths[i]
			var src dcSourceName
			src.header = dcHeader{dcInfoGetSourceName, uint32(unsafe.Sizeof(src)), p.source.adapterID, p.source.id}
			if r, _, _ := pDisplayConfigGetDeviceInfo.Call(uintptr(unsafe.Pointer(&src))); r != 0 {
				continue
			}
			info := targetInfo{internal: isInternalTech(p.target.outputTechnology)}
			var tgt dcTargetName
			tgt.header = dcHeader{dcInfoGetTargetName, uint32(unsafe.Sizeof(tgt)), p.target.adapterID, p.target.id}
			if r, _, _ := pDisplayConfigGetDeviceInfo.Call(uintptr(unsafe.Pointer(&tgt))); r == 0 {
				info.internal = isInternalTech(tgt.outputTechnology)
				info.name = strings.TrimSpace(syscall.UTF16ToString(tgt.friendlyName[:]))
			}
			gdi := syscall.UTF16ToString(src.viewGdiDeviceName[:])
			targets[gdi] = append(targets[gdi], info)
		}
		return targets, true
	}
	return nil, false
}

var (
	enumResult   []uintptr
	enumCallback = syscall.NewCallback(func(hmon, hdc, rc, lparam uintptr) uintptr {
		enumResult = append(enumResult, hmon)
		return 1
	})
)

// enumerateExternal returns every external monitor that answers DDC/CI
// brightness queries. Called only from the worker goroutine.
func enumerateExternal() []physMon {
	targets, haveTargets := displayTargets()

	enumResult = enumResult[:0]
	pEnumDisplayMonitors.Call(0, 0, enumCallback, 0)

	var out []physMon
	for _, hmon := range enumResult {
		var mi monitorInfoEx
		mi.cbSize = uint32(unsafe.Sizeof(mi))
		if r, _, _ := pGetMonitorInfoW.Call(hmon, uintptr(unsafe.Pointer(&mi))); r == 0 {
			continue
		}
		var externalNames []string
		if haveTargets {
			tis := targets[syscall.UTF16ToString(mi.szDevice[:])]
			internalOnly := len(tis) > 0
			for _, ti := range tis {
				if !ti.internal {
					internalOnly = false
					externalNames = append(externalNames, ti.name)
				}
			}
			if internalOnly {
				continue // the laptop's own panel
			}
		}

		var n uint32
		if r, _, _ := pGetNumPhysicalMonitors.Call(hmon, uintptr(unsafe.Pointer(&n))); r == 0 || n == 0 {
			continue
		}
		pms := make([]physicalMonitor, n)
		if r, _, _ := pGetPhysicalMonitors.Call(hmon, uintptr(n), uintptr(unsafe.Pointer(&pms[0]))); r == 0 {
			continue
		}
		answered := 0
		for _, pm := range pms {
			cur, max, ok := getBrightness(pm.handle)
			if !ok {
				// No DDC/CI: a built-in panel in a duplicated desktop, a
				// monitor with DDC/CI switched off, or a dock that drops it.
				pDestroyPhysicalMonitor.Call(pm.handle)
				continue
			}
			name := ""
			if answered < len(externalNames) {
				name = externalNames[answered]
			}
			answered++
			out = append(out, physMon{name: name, handle: pm.handle, max: max, pct: toPct(cur, max)})
		}
	}
	nameMonitors(out)
	return out
}

// nameMonitors fills in missing names and tells identical models apart.
func nameMonitors(mons []physMon) {
	count := map[string]int{}
	for i := range mons {
		if mons[i].name == "" {
			mons[i].name = "External display"
		}
		count[mons[i].name]++
	}
	seen := map[string]int{}
	for i := range mons {
		if count[mons[i].name] > 1 {
			seen[mons[i].name]++
			mons[i].name += " " + strconv.Itoa(seen[mons[i].name])
		}
	}
}
