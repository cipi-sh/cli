package cmd

import (
	"os"
	"strings"

	"github.com/cipi-sh/cli/internal/config"
)

var reservedRootCommands = map[string]struct{}{
	"configure":  {},
	"profiles":   {},
	"profile":    {},
	"servers":    {},
	"server":     {},
	"version":    {},
	"update":     {},
	"help":       {},
	"completion": {},
}

func stripProfileArg() {
	if len(os.Args) < 2 {
		return
	}

	arg := os.Args[1]
	if strings.HasPrefix(arg, "-") {
		return
	}
	if _, reserved := reservedRootCommands[arg]; reserved {
		return
	}
	if isKnownRootCommand(arg) {
		return
	}
	if !config.ProfileExists(arg) {
		return
	}

	config.SetActiveProfile(arg)
	os.Args = append([]string{os.Args[0]}, os.Args[2:]...)
}

func isKnownRootCommand(name string) bool {
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == name {
			return true
		}
		for _, alias := range cmd.Aliases {
			if alias == name {
				return true
			}
		}
	}
	return false
}
