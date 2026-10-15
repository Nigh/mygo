#!/usr/bin/env python3
"""Summarize MYGO_FRAME_CAPTURE files; timestamps are submissions, not scanout."""
import argparse
import collections
import json
import math
import statistics


def quantile(values, q):
    values = sorted(values)
    return values[max(0, math.ceil(q * len(values)) - 1)] if values else None


def summarize(data, warmup, duration):
    rows = [r for r in data["frames"] if warmup <= r["start_ns"] / 1e9 < warmup + duration]
    if len(rows) < 2:
        raise ValueError("fewer than two measured frames")
    starts = [r["start_ns"] / 1e6 for r in rows]
    intervals = [b - a for a, b in zip(starts, starts[1:])]
    cpu = [r["thread_cpu_ns"] / 1e6 for r in rows if r["thread_cpu_ns"] > 0]
    wall = [r["wall_ns"] / 1e6 for r in rows]
    submit = [r["start_ns"] / 1e6 + sum(r["phases_ns"]) / 1e6 for r in rows]
    submits = [b - a for a, b in zip(submit, submit[1:])]
    def percentiles(v):
        return {name: quantile(v, q) for name, q in [("p50", .5), ("p95", .95), ("p99", .99), ("max", 1)]}
    return {
        "frames": len(rows), "seconds": (starts[-1] - starts[0]) / 1000,
        "cpu_ms": percentiles(cpu), "wall_ms": percentiles(wall),
        "interval_ms": percentiles(intervals), "submit_interval_ms": percentiles(submits),
        "jitter_stddev_ms": statistics.pstdev(intervals),
        "over_16_67_pct": 100 * sum(d > 16.67 for d in intervals) / len(intervals),
        "over_25_ms_pct": 100 * sum(d > 25 for d in intervals) / len(intervals),
        "phases_p99_ms": [quantile([r["phases_ns"][i] / 1e6 for r in rows], .99) for i in range(4)],
        "alloc_bytes_per_frame": statistics.mean(r["alloc_bytes"] for r in rows),
        "gc_cycles": sum(r["gc"] for r in rows), "paths": dict(collections.Counter(r["path"] for r in rows)),
        "dropped_samples": data["dropped"],
        "size_dip_scale": data.get("size_dip_scale"), "refresh_hz": data.get("refresh_hz"),
        "device": data.get("device"),
    }


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("files", nargs="+")
    parser.add_argument("--warmup", type=float, default=3)
    parser.add_argument("--duration", type=float, default=17)
    args = parser.parse_args()
    for path in args.files:
        with open(path) as f:
            result = summarize(json.load(f), args.warmup, args.duration)
        print(json.dumps({"file": path, **result}, ensure_ascii=False))
