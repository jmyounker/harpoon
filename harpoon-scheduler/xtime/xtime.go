// Package xtime acts as an intermediary for package time methods that we use
// to coördinate things in the scheduler. The idea is to allow us to swap
// (mock) out the implementations in tests.
package xtime

import (
	"time"
)

var (
	// Now wraps time.Now.
	Now = time.Now

	// After wraps time.After.
	After = time.After

	// Tick wraps time.Tick.
	Tick = time.Tick
)
