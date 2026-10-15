# Streaming chart

Three linear waveforms, each retaining the latest 15,000 samples at 1,000
samples/second. A fixed ring keeps raw samples; the main thread consumes
elapsed samples in batches. X scrolls continuously with a fixed Y domain.
Each physical pixel column keeps its minimum/maximum sample in time order;
the endpoints of each continuous run are retained too. Axes, grid, layout
and GPU submission participate in timing.

```sh
CGO_ENABLED=0 go run ./examples/chart-streaming
```

The default `gpu` strategy uses `Painter.StrokePolyline`: instanced capsules
through the existing native renderer, without CPU path masks. `raw` retains
all points, `minmax` reduces points before constructing paths, and `reuse`
also reuses a path per series. All strategies reuse the input ring and arrays.
Opaque GPU segments can differ slightly in edge antialiasing from the CPU
outline. Translucent strokes and CPU fallback retain the original outline.

## Repeatable measurements

Build once, without the inspector, and run without a profiler or other builds
and tests in parallel. Each 20-second run warms up for 3 seconds and measures 17 seconds:

```sh
CGO_ENABLED=0 go build -tags mygo_noinspector -o /tmp/mygo-chart ./examples/chart-streaming
GDK_BACKEND=x11 GDK_SCALE=1 MYGO_FRAME_CAPTURE=/tmp/chart-1x.json \
  /tmp/mygo-chart -always-on-top -x=20 -y=20 -duration 20s
python3 scripts/chart-stats.py /tmp/chart-1x.json --warmup 3 --duration 17

# GTK on Wayland may ignore GDK_SCALE: check the printed scale and capture metadata.
# On this two-monitor test machine, keep the oversized 2x window on one monitor.
GDK_BACKEND=x11 GDK_SCALE=2 MYGO_FRAME_CAPTURE=/tmp/chart-2x.json \
  /tmp/mygo-chart -always-on-top -x=-100 -y=-100 -duration 20s
python3 scripts/chart-stats.py /tmp/chart-2x.json --warmup 3 --duration 17

# Three-minute stability run, including three seconds of warmup.
GDK_BACKEND=x11 GDK_SCALE=2 MYGO_FRAME_CAPTURE=/tmp/chart-long.json \
  /tmp/mygo-chart -always-on-top -x=-100 -y=-100 -duration 180s
python3 scripts/chart-stats.py /tmp/chart-long.json --warmup 3 --duration 177

# Profile separately: it changes timing and must not be used for acceptance.
/tmp/mygo-chart -mode reuse -duration 15s -cpuprofile /tmp/chart.pprof
go tool pprof -top /tmp/mygo-chart /tmp/chart.pprof

# Real GPU offscreen submission, usable with a locked desktop. It is not a
# compositor/frame-pacing test; each benchmark iteration supplies a frame.
CGO_ENABLED=0 MYGO_FRAME_CAPTURE=/tmp/chart-offscreen.json \
  go test -tags mygo_noinspector ./examples/chart-streaming -run '^$' \
  -bench 'BenchmarkStreamingGPU/2x' -benchtime 20s -count=1
python3 scripts/chart-stats.py /tmp/chart-offscreen.json --warmup 3 --duration 17
```

The plot is 1280x720 DIPs, 2-DIP strokes, with three seeded signals containing
smooth waves, high-frequency noise and isolated spikes. `-width` and `-height`
change the plot, not the window's outer dimensions. The minimum client size
includes axes and margins, so the window manager cannot shrink the measured
plot. A 2x window is larger than the available 2560x1440 monitor: some pixels
are offscreen, but the full 2720x1600 framebuffer is rendered. The position
above avoids spanning two monitors with different presentation phases.

`MYGO_FRAME_CAPTURE` is an opt-in, bounded collector (100,000 samples) that
writes JSON when the window closes; use one window per capture. It records
build/layout/paint/present wall time, main-thread CPU time on Linux,
process allocation/GC deltas, actual client size/scale, refresh rate and GL
device. Its CPU clock excludes GPU/compositor waits, but does not include CPU
work on other threads. Collection and final file writing are outside the
recorded frame duration. Disabled captures do no measurement work.

Frame-start and submission intervals are **not physical scanout timestamps**.
Results include P50/P95/P99/max, jitter standard deviation, strict 16.67ms
exceedance and intervals over 25ms. A 59.91Hz monitor's nominal interval is
16.69ms; strict exceedance therefore does not by itself mean a dropped frame.
Cross-monitor windows can submit at irregular rates even with the same render
cost. True displayed-frame acceptance needs compositor presentation feedback
or an external display measurement.

Keep the desktop unlocked during real-window tests: a locked compositor can
stop X11 frame callbacks entirely. The timed example has an independent exit
deadline; an incomplete capture is a failed run, not a low-cost frame. Performance captures and summaries belong in the ignored
`.mygo/chart-results/` directory and are used for PR descriptions, not committed.
For foreground acceptance, also pass `-always-on-top` to prevent a covered
window from being throttled, and temporarily inhibit automatic locking with
`gnome-session-inhibit --inhibit idle --reason 'MyGo chart benchmark'` before
the command. The inhibitor ends when the command exits.

## Validation

```sh
CGO_ENABLED=0 go test ./ui ./ui/chart ./examples/chart-streaming \
  -run 'TestStrokePolyline|TestMinMax|TestLineInto|TestPaths|TestFrameCapture|TestWaveformRing'
python3 scripts/chart-stats.test.py
MYGO_E2E=1 MYGO_GPU=1 go test ./internal/e2e -run TestContentPolylineFrames
```

The GPU image checks exercise desktop GL and OpenGL ES at 1x/2x against the
CPU renderer. Tests also cover spikes, ordering, viewport neighbors, missing
values, clipping, ring wraparound, delayed ingestion, reusable paths and
translucent fallback. Native GPU performance on Metal/Direct3D requires those
platforms' hardware; cross-compiling cannot establish it.
