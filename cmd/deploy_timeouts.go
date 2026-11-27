package cmd

import "fmt"

// The two clocks of a deploy, in seconds, as the server takes them (Cipi
// 5.18.0+): how long the frontend build may run — the node_build of a Laravel
// app, the install and the build of a Node app — and how long every other
// command may. The server keeps 1800 and 300 for an app that says nothing.
type deployClock struct {
	flag     string // --build-timeout
	field    string // build_timeout
	min, max int
	def      int
	what     string
}

var deployClocks = []deployClock{
	{flag: "build-timeout", field: "build_timeout", min: 60, max: 3600, def: 1800, what: "the frontend build"},
	{flag: "step-timeout", field: "step_timeout", min: 60, max: 1800, def: 300, what: "every other command of the deploy"},
}

// deployClockSeconds checks a number of seconds against the bounds of its clock.
func deployClockSeconds(c deployClock, seconds int) (int, error) {
	if seconds < c.min || seconds > c.max {
		return 0, fmt.Errorf("--%s is a number of seconds from %d to %d (%d unless set): how long %s may run", c.flag, c.min, c.max, c.def, c.what)
	}
	return seconds, nil
}
