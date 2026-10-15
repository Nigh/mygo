//go:build linux && (amd64 || arm64)

package main

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/egoist/mygo/internal/gpu/gl"
	"github.com/egoist/mygo/internal/platform"
	"github.com/egoist/mygo/internal/surface"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
)

// benchSurface supplies frames directly, independent of the desktop compositor.
type benchSurface struct{ scale float64 }

func (s *benchSurface) Native() platform.SurfaceNative    { return platform.SurfaceNative{GLArea: 1} }
func (s *benchSurface) Size() (float64, float64, float64) { return 1360, 800, s.scale }
func (*benchSurface) RequestFrame()                       {}
func (*benchSurface) RefreshRate() float64                { return 60 }
func (*benchSurface) PresentPixels([]byte, int, int, int) {
	panic("offscreen benchmark fell back to CPU rendering")
}
func (*benchSurface) SetCursor(platform.Cursor)                 {}
func (*benchSurface) SetTextInput(platform.TextInputState)      {}
func (*benchSurface) UpdateAccessibility(*platform.AccessTree)  {}
func (*benchSurface) StartDataDrag(platform.DragRequest)        {}
func (*benchSurface) CancelDataDrag()                           {}
func (*benchSurface) SetDropFormats([]transfer.Format)          {}
func (*benchSurface) PlaceWebViews([]platform.WebViewPlacement) {}
func (*benchSurface) NewWebView(*platform.WindowOptions, platform.WindowHandler) (platform.WebView, error) {
	return nil, platform.ErrUnsupported
}

// BenchmarkStreamingGPU builds, lays out and submits the same native view into
// a real GL framebuffer at 1x/2x. It does not measure desktop frame pacing.
func BenchmarkStreamingGPU(b *testing.B) {
	for _, scale := range []float64{1, 2} {
		b.Run(fmt.Sprintf("%gx", scale), func(b *testing.B) {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			o, err := gl.NewOffscreen(false, int(1360*scale), int(800*scale))
			if err != nil {
				b.Fatal(err)
			}
			defer o.Release()
			b.Log(o.Info())
			s := newWaveform(1280, 720, "gpu", 0)
			s.printed = true
			conn := &surface.Conn{Surface: &benchSurface{scale: scale}, Invalidate: func() {}}
			ui.View(s.view).AttachContent(conn)
			defer conn.Detach()
			for range 20 {
				conn.Event(platform.SurfaceEvent{Kind: platform.SurfaceFrame})
			}
			b.ReportAllocs()
			for b.Loop() {
				conn.Event(platform.SurfaceEvent{Kind: platform.SurfaceFrame})
			}
			pix, err := gl.ReadFramebuffer(int(1360*scale), int(800*scale))
			if err != nil {
				b.Fatal(err)
			}
			var ink [3]int
			for i := 0; i+3 < len(pix); i += 4 {
				if pix[i] > 180 && pix[i+2] < 120 {
					ink[0]++
				}
				if pix[i+2] > 180 && pix[i] < 120 {
					ink[1]++
				}
				if pix[i+1] > 160 && pix[i+2] < 120 && pix[i] < 160 {
					ink[2]++
				}
			}
			for series, n := range ink {
				if n < 1000 {
					b.Fatal("GPU waveform missing", series, n)
				}
			}
		})
	}
}
