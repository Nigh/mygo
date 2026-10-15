import runpy
import unittest

stats = runpy.run_path("scripts/chart-stats.py")


class FrameStatsTest(unittest.TestCase):
    def test_percentiles_and_warmup(self):
        frames = [{"start_ns": i * 20_000_000, "wall_ns": 2_000_000,
                   "thread_cpu_ns": 1_000_000, "phases_ns": [500_000] * 4,
                   "alloc_bytes": 100, "gc": 0, "path": "GPU"} for i in range(100)]
        result = stats["summarize"]({"frames": frames, "dropped": 0}, .1, 1)
        self.assertEqual(result["frames"], 50)
        self.assertAlmostEqual(result["interval_ms"]["p99"], 20)
        self.assertEqual(result["cpu_ms"]["p99"], 1)
        self.assertEqual(result["over_16_67_pct"], 100)
        self.assertEqual(result["over_25_ms_pct"], 0)
        self.assertEqual(result["alloc_bytes_per_frame"], 100)

    def test_insufficient_capture(self):
        with self.assertRaises(ValueError):
            stats["summarize"]({"frames": [], "dropped": 0}, 0, 1)


if __name__ == "__main__":
    unittest.main()
