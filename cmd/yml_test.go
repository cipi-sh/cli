package cmd

import (
	"errors"
	"strings"
	"testing"

	"github.com/cipi-sh/cli/internal/api"
)

func TestYmlErrorHint(t *testing.T) {
	cases := []struct {
		name    string
		err     error
		ability string
		want    string // substring expected in the hint
		version bool   // the version hint is expected
	}{
		{
			name:    "403 names the ability",
			err:     &api.APIError{Message: "Invalid ability provided.", Status: 403},
			ability: "yml-manage",
			want:    "lacks the yml-manage ability",
		},
		{
			name:    "missing route on an old cipi/api",
			err:     &api.APIError{Message: "The route api/apps/shop/yml could not be found.", Status: 404},
			ability: "yml-view",
			version: true,
		},
		{
			name:    "503 from an old Cipi",
			err:     &api.APIError{ErrorText: "cipi yml plan failed", Status: 503},
			ability: "yml-view",
			version: true,
		},
		{
			name:    "unknown app is not a version problem",
			err:     &api.APIError{ErrorText: "App 'nope' not found", Status: 404},
			ability: "yml-view",
			want:    "App 'nope' not found",
		},
		{
			name:    "non-JSON 404",
			err:     errors.New("API error (HTTP 404): <html>Not Found</html>"),
			ability: "yml-view",
			version: true,
		},
		{
			name:    "non-JSON 403",
			err:     errors.New("API error (HTTP 403): Forbidden"),
			ability: "yml-view",
			want:    "lacks the yml-view ability",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ymlErrorHint(tc.err, tc.ability)
			if tc.want != "" && !strings.Contains(got, tc.want) {
				t.Fatalf("hint %q does not contain %q", got, tc.want)
			}
			if has := strings.Contains(got, "cipi/api 1.35+ and Cipi 5.6.4+"); has != tc.version {
				t.Fatalf("hint %q: version hint present=%v, want %v", got, has, tc.version)
			}
			if !strings.HasPrefix(got, tc.err.Error()) {
				t.Fatalf("hint %q does not start with the error %q", got, tc.err.Error())
			}
		})
	}
}

func TestYmlPlanProblem(t *testing.T) {
	file := "/home/shop/current/cipi.yml"
	yes, no := true, false

	cases := []struct {
		name string
		plan ymlPlanData
		want string // "" = no problem
	}{
		{"no file", ymlPlanData{App: "shop"}, "no cipi.yml"},
		{"invalid", ymlPlanData{App: "shop", File: &file, Valid: &no}, "not valid"},
		{"blocked", ymlPlanData{App: "shop", File: &file, Valid: &yes, InSync: &no, Blockers: []string{"PHP 8.5 is not installed"}}, "1 item(s) block"},
		{"changes", ymlPlanData{App: "shop", File: &file, Valid: &yes, InSync: &no, Changes: []ymlChange{{Kind: "php", Summary: "PHP 8.3 → 8.4"}}}, ""},
		{"in sync", ymlPlanData{App: "shop", File: &file, Valid: &yes, InSync: &yes}, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ymlPlanProblem(&tc.plan)
			switch {
			case tc.want == "" && err != nil:
				t.Fatalf("unexpected problem: %s", err)
			case tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)):
				t.Fatalf("problem = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestYmlFileCandidates(t *testing.T) {
	cases := map[string]string{
		"":           "cipi.yml",
		"production": "cipi-production.yml or cipi.yml",
		"staging":    "cipi-staging.yml or cipi.yml",
	}
	for env, want := range cases {
		if got := ymlFileCandidates(env); got != want {
			t.Errorf("ymlFileCandidates(%q) = %q, want %q", env, got, want)
		}
	}
}

func TestYmlFileNote(t *testing.T) {
	cases := []struct {
		name string
		env  string
		file string
		want string // "" = no note, else a substring of it
	}{
		{"old server without labels", "", "/home/shop/current/cipi.yml", ""},
		{"production on the shared file", "production", "/home/shop/current/cipi.yml", ""},
		{"staging on its own file", "staging", "/home/shop/current/cipi-staging.yml", ""},
		{"staging on the shared file", "staging", "/home/shop/current/cipi.yml", "cipi-staging.yml"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ymlFileNote(tc.env, tc.file)
			switch {
			case tc.want == "" && got != "":
				t.Fatalf("unexpected note %q", got)
			case tc.want != "" && !strings.Contains(got, tc.want):
				t.Fatalf("note %q does not mention %q", got, tc.want)
			}
		})
	}
}
