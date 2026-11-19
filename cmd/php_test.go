package cmd

import "testing"

func TestIsLegacyPHP(t *testing.T) {
	installable := []string{"8.3", "8.4", "8.5", "8.6"}

	for _, v := range []string{"7.4", "8.0", "8.1", "8.2"} {
		if !isLegacyPHP(v, installable) {
			t.Errorf("%s: installed and older than everything installable, expected legacy", v)
		}
	}
	for _, v := range []string{"8.3", "8.5", "8.6", "8.7", "9.0"} {
		if isLegacyPHP(v, installable) {
			t.Errorf("%s: not legacy", v)
		}
	}

	// A core older than 5.9.1 lists 8.3-8.5: a newer version is not legacy.
	if isLegacyPHP("8.6", []string{"8.3", "8.4", "8.5"}) {
		t.Error("8.6 is newer than the list, not legacy")
	}
	// No list from the server (API before 1.15), or not a version: nothing is called legacy.
	if isLegacyPHP("8.1", nil) || isLegacyPHP("", installable) || isLegacyPHP("dev", installable) {
		t.Error("without a list, or without a version, nothing is legacy")
	}
}
