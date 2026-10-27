package cmd

import (
	"strings"
	"testing"
)

func TestValidateEnvLabel(t *testing.T) {
	valid := []string{"production", "staging", "a", "0", "qa-1", "a_b", "pre-prod_2", strings.Repeat("x", 32)}
	for _, label := range valid {
		if err := validateEnvLabel(label); err != nil {
			t.Errorf("%q: unexpected error %v", label, err)
		}
	}

	invalid := []string{"", "Staging", "-x", "_x", "st aging", "stag.ing", "été", strings.Repeat("x", 33)}
	for _, label := range invalid {
		err := validateEnvLabel(label)
		if err == nil {
			t.Errorf("%q: expected an error", label)
			continue
		}
		if !strings.Contains(err.Error(), "invalid environment label") || !strings.Contains(err.Error(), "staging") {
			t.Errorf("%q: unhelpful error %q", label, err)
		}
	}
}

func TestAppEnv(t *testing.T) {
	cases := []struct {
		name string
		app  map[string]interface{}
		want string
	}{
		{"missing on an old server", map[string]interface{}{"app": "shop"}, "production"},
		{"empty", map[string]interface{}{"env": ""}, "production"},
		{"null", map[string]interface{}{"env": nil}, "production"},
		{"not a string", map[string]interface{}{"env": float64(1)}, "production"},
		{"label", map[string]interface{}{"env": "staging"}, "staging"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := appEnv(tc.app); got != tc.want {
				t.Fatalf("appEnv = %q, want %q", got, tc.want)
			}
		})
	}
}
