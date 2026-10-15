//go:build !linux

package ui

import "time"

// Thread CPU capture is currently available on Linux; zero means unavailable.
func frameThreadCPU() time.Duration { return 0 }
