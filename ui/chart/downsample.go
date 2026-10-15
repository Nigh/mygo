package chart

import (
	"math"
	"sort"
)

// MinMax reduces a line to the minimum and maximum point in each physical
// pixel column, in their original order, retaining each run's endpoints.
// xs and ys are plot fractions; pixelWidth is ceil(plot.Width * painter.Scale()).
// Values outside [0,1] are clipped, retaining a neighboring point at either
// edge. Non-finite
// values break the line. Valid xs must be nondecreasing, even across breaks;
// otherwise MinMax panics. Mismatched inputs use their common length.
//
// Output replaces dstX and dstY, reusing their storage. They must not overlap
// each other or the inputs. A nonpositive pixelWidth returns empty output.
// Keep the original points for selection and readings.
func MinMax(dstX, dstY, xs, ys []float32, pixelWidth int) ([]float32, []float32) {
	dstX, dstY = dstX[:0], dstY[:0]
	if pixelWidth <= 0 {
		return dstX, dstY
	}
	n := min(len(xs), len(ys))
	finite := func(v float32) bool { return !math.IsNaN(float64(v)) && !math.IsInf(float64(v), 0) }
	previous := float32(math.Inf(-1))
	for _, x := range xs[:n] {
		if !finite(x) {
			continue
		}
		if x < previous {
			panic("chart.MinMax: xs must be nondecreasing")
		}
		previous = x
	}
	appendPoint := func(i int) { dstX = append(dstX, xs[i]); dstY = append(dstY, ys[i]) }
	runs(n, func(i int) bool { return finite(xs[i]) && finite(ys[i]) }, func(from, to int) {
		// Binary search leaves at most one point beyond either visible edge.
		lo := from + sort.Search(to-from, func(i int) bool { return xs[from+i] >= 0 })
		if lo == to {
			return
		}
		hi := from + sort.Search(to-from, func(i int) bool { return xs[from+i] > 1 })
		if lo == hi && (lo == from || xs[lo] > 1 && xs[lo-1] > 1) {
			return
		}
		if lo > from {
			lo--
		}
		if hi < to {
			hi++
		}
		if len(dstX) > 0 {
			dstX = append(dstX, float32(math.NaN()))
			dstY = append(dstY, float32(math.NaN()))
		}
		last := -1
		emit := func(i int) {
			if i != last {
				appendPoint(i)
				last = i
			}
		}
		emit(lo)
		for i := lo; i < hi; {
			bucket := int(math.Floor(float64(xs[i]) * float64(pixelWidth)))
			low, high := i, i
			i++
			for i < hi && xs[i] >= 0 && xs[i] <= 1 && bucket == int(math.Floor(float64(xs[i])*float64(pixelWidth))) {
				if ys[i] < ys[low] {
					low = i
				}
				if ys[i] > ys[high] {
					high = i
				}
				i++
			}
			emit(min(low, high))
			emit(max(low, high))
		}
		emit(hi - 1)
	})
	return dstX, dstY
}
