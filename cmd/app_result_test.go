package cmd

import (
	"strings"
	"testing"
)

func TestDescribeRepositoryAccess(t *testing.T) {
	t.Run("ok with branch", func(t *testing.T) {
		r := describeRepositoryAccess(map[string]interface{}{"status": "ok", "branch": "main"}, "first")
		if !r.OK || r.Message != "Repository: reachable (branch main)" || len(r.Details) != 0 {
			t.Fatalf("unexpected report: %+v", r)
		}
	})

	t.Run("ok without branch", func(t *testing.T) {
		r := describeRepositoryAccess(map[string]interface{}{"status": "ok"}, "first")
		if !r.OK || r.Message != "Repository: reachable" {
			t.Fatalf("unexpected report: %+v", r)
		}
	})

	t.Run("nobranch", func(t *testing.T) {
		r := describeRepositoryAccess(map[string]interface{}{
			"status": "nobranch",
			"branch": "develop",
			"hint":   "cipi app edit myapp --branch=<name>",
		}, "next")
		if r.OK || !strings.Contains(r.Message, "no branch 'develop'") {
			t.Fatalf("unexpected report: %+v", r)
		}
		if len(r.Details) != 1 || r.Details[0] != "Fix on the server: cipi app edit myapp --branch=<name>" {
			t.Fatalf("unexpected details: %#v", r.Details)
		}
	})

	t.Run("denied with error and hint", func(t *testing.T) {
		r := describeRepositoryAccess(map[string]interface{}{
			"status": "denied",
			"error":  "ERROR: Repository not found.",
			"hint":   "cipi git github-token <token> && cipi git refresh myapp",
		}, "first")
		if r.OK || !strings.Contains(r.Message, "the first deploy will fail") {
			t.Fatalf("unexpected report: %+v", r)
		}
		want := []string{
			"git: ERROR: Repository not found.",
			"Fix on the server: cipi git github-token <token> && cipi git refresh myapp",
		}
		if strings.Join(r.Details, "|") != strings.Join(want, "|") {
			t.Fatalf("details = %#v, want %#v", r.Details, want)
		}
	})

	t.Run("denied without hint", func(t *testing.T) {
		r := describeRepositoryAccess(map[string]interface{}{"status": "denied"}, "next")
		if r.OK || !strings.Contains(r.Message, "the next deploy will fail") || len(r.Details) != 1 {
			t.Fatalf("unexpected report: %+v", r)
		}
	})

	t.Run("unknown status", func(t *testing.T) {
		r := describeRepositoryAccess(map[string]interface{}{"status": "weird"}, "first")
		if r.OK || r.Message != "Repository: weird" {
			t.Fatalf("unexpected report: %+v", r)
		}
	})
}
