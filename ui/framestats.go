package ui

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"runtime/metrics"
	"strconv"
	"strings"
	"time"
)

// Frame statistics: with MYGO_FRAME_STATS set, the engine logs each frame
// slower than a threshold, with how long each part took, which path drew
// it, what the process allocated meanwhile and whether the garbage
// collector ran. Unset, frames measure nothing.

// frameStatsThreshold is how long a frame takes before it is logged, from
// MYGO_FRAME_STATS: 1 (or true, on) logs those over 8 ms, another number
// those over that many milliseconds, and all every frame. frameStatsOn
// tells whether it is set.
var frameStatsThreshold, frameStatsOn = frameStatsSetting(os.Getenv("MYGO_FRAME_STATS"))

// MYGO_FRAME_CAPTURE writes bounded, in-memory frame samples when the window
// closes. Use one window per capture; it avoids logging in the measured loop.
var frameCapturePath = os.Getenv("MYGO_FRAME_CAPTURE")

type frameSample struct {
	StartNS     int64         `json:"start_ns"`
	WallNS      int64         `json:"wall_ns"`
	ThreadCPUNS int64         `json:"thread_cpu_ns"`
	PhasesNS    [phases]int64 `json:"phases_ns"`
	AllocBytes  uint64        `json:"alloc_bytes"`
	GC          uint64        `json:"gc"`
	Path        string        `json:"path"`
}

// frameLog writes the log of a frame; tests replace it.
var frameLog = log.Print

func frameStatsSetting(v string) (threshold time.Duration, on bool) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "0", "false", "off", "no":
		return 0, false
	case "1", "true", "on", "yes":
		return 8 * time.Millisecond, true
	case "all":
		return 0, true
	}
	ms, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
	if err != nil || ms < 0 {
		log.Printf("mygo: MYGO_FRAME_STATS=%q is not a number of milliseconds: logging frames over 8 ms", v)
		return 8 * time.Millisecond, true
	}
	return time.Duration(ms * float64(time.Millisecond)), true
}

// The parts of a frame.
const (
	phaseBuild = iota
	phaseLayout
	phasePaint
	phasePresent
	phases
)

// The runtime's counters a frame reads before and after.
const (
	statObjects = iota // allocations, but tiny ones
	statTiny           // tiny allocations, combined in blocks
	statBytes
	statCycles // GC cycles ended
	statAssist // CPU seconds goroutines helped the GC mark
	stats
)

// frameStats measures a frame for MYGO_FRAME_STATS.
type frameStats struct {
	capture     string
	rows        []frameSample
	dropped     int
	epoch       time.Time
	cpu         time.Duration
	size        [3]float32
	refresh     float32
	device      string
	threshold   time.Duration
	start, last time.Time
	laps        [phases]time.Duration
	passes      int
	samples     [stats]metrics.Sample
	before      [stats]float64
	layouts     uint64 // the text system's, before the frame
}

func newFrameStats(threshold time.Duration) *frameStats {
	f := &frameStats{threshold: threshold}
	if frameCapturePath != "" {
		f.capture = frameCapturePath
		f.rows = make([]frameSample, 0, 100000)
		f.epoch = time.Now()
	}
	for i, name := range [stats]string{
		"/gc/heap/allocs:objects",
		"/gc/heap/tiny/allocs:objects",
		"/gc/heap/allocs:bytes",
		"/gc/cycles/total:gc-cycles",
		"/cpu/classes/gc/mark/assist:cpu-seconds",
	} {
		f.samples[i].Name = name
	}
	return f
}

// read returns the runtime's counters.
func (f *frameStats) read() (v [stats]float64) {
	metrics.Read(f.samples[:])
	for i, s := range f.samples {
		switch s.Value.Kind() {
		case metrics.KindUint64:
			v[i] = float64(s.Value.Uint64())
		case metrics.KindFloat64:
			v[i] = s.Value.Float64()
		}
	}
	return v
}

// begin starts measuring a frame. The methods of a nil frameStats do
// nothing, as frames go while MYGO_FRAME_STATS is unset.
func (f *frameStats) begin(rt *engine) {
	if f == nil {
		return
	}
	f.before, f.layouts = f.read(), rt.text.LayoutsMade()
	f.laps, f.passes = [phases]time.Duration{}, 0
	f.start = time.Now()
	f.last = f.start
	if f.capture != "" {
		f.cpu = frameThreadCPU()
	}
}

// lap ends a part of the frame.
func (f *frameStats) lap(phase int) {
	if f == nil {
		return
	}
	now := time.Now()
	f.laps[phase] += now.Sub(f.last)
	f.last = now
}

// end logs the frame if it took longer than the threshold.
func (f *frameStats) end(rt *engine) {
	if f == nil {
		return
	}
	total := time.Since(f.start)
	if f.capture != "" {
		cpu := frameThreadCPU()
		f.size = rt.painted
		f.refresh = rt.host.refreshRate()
		if f.device == "" {
			if h, ok := rt.host.(*windowHost); ok {
				if g, ok := h.gpu.(interface{ Info() string }); ok {
					f.device = g.Info()
				}
			}
		}
		after := f.read()
		row := frameSample{StartNS: f.start.Sub(f.epoch).Nanoseconds(), WallNS: total.Nanoseconds(),
			AllocBytes: uint64(max(0, after[statBytes]-f.before[statBytes])), GC: uint64(max(0, after[statCycles]-f.before[statCycles]))}
		if cpu > 0 && f.cpu > 0 {
			row.ThreadCPUNS = (cpu - f.cpu).Nanoseconds()
		}
		for i, d := range f.laps {
			row.PhasesNS[i] = d.Nanoseconds()
		}
		if p, ok := rt.host.(interface{ framePath() string }); ok {
			row.Path = p.framePath()
		}
		if len(f.rows) < cap(f.rows) {
			f.rows = append(f.rows, row)
		} else {
			f.dropped++
		}
		return
	}
	if total < f.threshold {
		return
	}
	after := f.read()
	var d [stats]float64
	for i := range d {
		d[i] = after[i] - f.before[i]
	}
	ms := func(t time.Duration) string { return strconv.FormatFloat(t.Seconds()*1000, 'f', 1, 64) }
	other := total
	for _, t := range f.laps {
		other -= t
	}
	path := "not shown"
	if p, ok := rt.host.(interface{ framePath() string }); ok && p.framePath() != "" {
		path = p.framePath()
	}
	passes := ""
	switch {
	case f.passes == 0:
		passes = " (painted again)" // engine.repaintFrame
	case f.passes > 1:
		passes = fmt.Sprintf(" (%d passes)", f.passes)
	}
	gc := "no GC"
	if d[statCycles] > 0 || d[statAssist] > 0 {
		gc = fmt.Sprintf("GC: %.0f cycles ended, %s ms of assists", d[statCycles], ms(time.Duration(d[statAssist]*float64(time.Second))))
	}
	frameLog(fmt.Sprintf("mygo: frame %d took %s ms: build %s%s, layout %s, paint %s, present %s (%s), other %s; meanwhile the process made about %.0f allocations, %.1f KB, %d text layouts; %s",
		rt.frame, ms(total), ms(f.laps[phaseBuild]), passes, ms(f.laps[phaseLayout]), ms(f.laps[phasePaint]), ms(f.laps[phasePresent]), path, ms(other),
		d[statObjects]+d[statTiny], d[statBytes]/1024, rt.text.LayoutsMade()-f.layouts, gc))
}

func (f *frameStats) flush() {
	if f == nil || f.capture == "" {
		return
	}
	b, err := json.Marshal(struct {
		Frames  []frameSample `json:"frames"`
		Dropped int           `json:"dropped"`
		Size    [3]float32    `json:"size_dip_scale"`
		Refresh float32       `json:"refresh_hz"`
		Device  string        `json:"device"`
	}{f.rows, f.dropped, f.size, f.refresh, f.device})
	if err == nil {
		err = os.WriteFile(f.capture, b, 0600)
	}
	if err != nil {
		log.Printf("mygo: frame capture: %v", err)
	}
}
