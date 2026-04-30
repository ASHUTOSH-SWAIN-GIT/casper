package main

import "github.com/ASHUTOSH-SWAIN-GIT/casper/internal/runner"

// runnable is a CLI-side type alias of runner.Runnable so existing
// CLI code keeps its short name without changing every call site.
type runnable = runner.Runnable

// buildRunnable defers to runner.Build. The CLI and casperd share the
// same per-action dispatch — adding a new action lands in one place.
func buildRunnable(raw []byte) (*runnable, error) {
	return runner.Build(raw)
}
