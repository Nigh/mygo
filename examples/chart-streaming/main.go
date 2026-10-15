// Chart-streaming draws three rolling, 15,000-sample waveforms at 1 kHz each.
package main

import (
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"runtime/pprof"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"
)

const samples = 15000

type waveform struct {
	data             [3][samples]float32
	next             int64
	xs               []float32
	ys               [3][]float32
	bucketX, bucketY [3][]float32
	paths            [3]ui.Path
	points           [3][][2]float32
	start            time.Time
	duration         time.Duration
	width, height    int
	mode             string
	printed          bool
}

func value(i int64, s int) float32 {
	x := float64(i) / 1000
	// Deterministic noise and isolated spikes exercise extrema preservation.
	n := uint64(i)*6364136223846793005 + uint64(s+1)*1442695040888963407
	v := .22*math.Sin(x*float64(s+1)*7) + .08*(float64(n>>48)/65535-.5)
	if i%997 == int64(s*31) {
		v += .22
	}
	return float32(.2 + float64(s)*.3 + v*.55)
}

func newWaveform(w, h int, mode string, duration time.Duration) *waveform {
	s := &waveform{width: w, height: h, mode: mode, duration: duration, next: samples, xs: make([]float32, samples)}
	for k := range 3 {
		s.ys[k] = make([]float32, samples)
		for i := range samples {
			s.data[k][i] = value(int64(i), k)
		}
	}
	return s
}

func (s *waveform) update(now time.Time) {
	if s.start.IsZero() {
		s.start = now
	}
	elapsed := now.Sub(s.start)
	target := int64(samples) + elapsed.Nanoseconds()/int64(time.Millisecond)
	// A delayed frame generates only samples still in the visible window.
	for i := max(s.next, target-samples); i < target; i++ {
		for k := range 3 {
			s.data[k][i%samples] = value(i, k)
		}
	}
	s.next = target
	phase := float32(float64(elapsed.Nanoseconds()%int64(time.Millisecond)) / float64(time.Millisecond))
	for i := range samples {
		s.xs[i] = (float32(i) - phase) / float32(samples-1)
		for k := range 3 {
			s.ys[k][i] = s.data[k][(target-samples+int64(i))%samples]
		}
	}
}

func (s *waveform) view(c *ui.Context) {
	s.update(c.Now())
	if s.duration > 0 && c.Now().Sub(s.start) >= s.duration {
		mygo.App.Quit()
		return
	}
	c.AnimationFrame()
	t := c.Theme()
	x := chart.NewLinear(-15, 0)
	y := chart.NewLinear(0, 1)
	chart.Plot(c, chart.Axes{Left: y.Ticks(5), Bottom: x.Ticks(5)}, func(area ui.Element) {
		area.Size(float32(s.width), float32(s.height)).Label("Three live waveforms").Description("15 seconds at 1000 samples per second per series").Draw(func(p *ui.Painter, r ui.Rect) {
			if !s.printed {
				fmt.Printf("plot %.0fx%.0f DIP, scale %.2f, mode %s\n", r.W, r.H, p.Scale(), s.mode)
				s.printed = true
			}
			for _, tick := range y.Ticks(5) {
				p.Line(r.X, chart.Y(r, tick.At), r.X+r.W, chart.Y(r, tick.At), 1, t.Border)
			}
			colors := [3]ui.Color{ui.RGB(40, 140, 240), ui.RGB(240, 100, 70), ui.RGB(50, 190, 110)}
			p.Clip(r, 0, func() {
				for k := range 3 {
					xs, ys := s.xs, s.ys[k]
					if s.mode != "raw" {
						s.bucketX[k], s.bucketY[k] = chart.MinMax(s.bucketX[k], s.bucketY[k], xs, ys, int(math.Ceil(float64(r.W*p.Scale()))))
						xs, ys = s.bucketX[k], s.bucketY[k]
					}
					if s.mode == "gpu" {
						s.points[k] = s.points[k][:0]
						for i, x := range xs {
							s.points[k] = append(s.points[k], [2]float32{chart.X(r, x), chart.Y(r, ys[i])})
						}
						p.StrokePolyline(s.points[k], 2, colors[k])
					} else if s.mode == "reuse" {
						p.StrokePath(chart.LineInto(&s.paths[k], r, xs, ys, chart.CurveLinear), 2, colors[k])
					} else {
						p.StrokePath(chart.Line(r, xs, ys, chart.CurveLinear), 2, colors[k])
					}
				}
			})
		})
	}).Gap(8).Padding(8)
}

func main() {
	w := flag.Int("width", 1280, "plot width in DIPs")
	h := flag.Int("height", 720, "plot height in DIPs")
	x := flag.Int("x", 0, "window X position (zero with Y zero centers it)")
	y := flag.Int("y", 0, "window Y position")
	top := flag.Bool("always-on-top", false, "keep benchmark visible to prevent compositor throttling")
	d := flag.Duration("duration", 0, "quit after this duration (zero runs until closed)")
	mode := flag.String("mode", "gpu", "rendering strategy: raw, minmax, reuse or gpu")
	profile := flag.String("cpuprofile", "", "write a CPU profile; do not use during timing runs")
	flag.Parse()
	if *w <= 0 || *h <= 0 || *d < 0 || (*mode != "raw" && *mode != "minmax" && *mode != "reuse" && *mode != "gpu") {
		log.Fatal("invalid size, duration or mode")
	}
	if *profile != "" {
		f, err := os.Create(*profile)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		if err = pprof.StartCPUProfile(f); err != nil {
			log.Fatal(err)
		}
		defer pprof.StopCPUProfile()
	}
	s := newWaveform(*w, *h, *mode, *d)
	mygo.App.WhenReady(func() {
		mygo.NewWindow(mygo.WindowOptions{Title: "Streaming chart", Width: *w + 80, Height: *h + 80,
			MinWidth: *w + 80, MinHeight: *h + 80, X: *x, Y: *y, AlwaysOnTop: *top, UseContentSize: true, Content: ui.View(s.view)})
		// A stalled frame loop must still finish a timed measurement.
		if *d > 0 {
			time.AfterFunc(*d+time.Second, mygo.App.Quit)
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
