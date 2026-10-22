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

func TestDescribeYmlApplyResult(t *testing.T) {
	list := func(items ...string) []interface{} {
		out := make([]interface{}, len(items))
		for i, s := range items {
			out[i] = s
		}
		return out
	}

	cases := []struct {
		name    string
		result  map[string]interface{}
		ok      bool
		message string
		details []string
		hint    bool
	}{
		{
			name:    "applied",
			result:  map[string]interface{}{"app": "shop", "outcome": "applied", "applied": float64(3), "failed": float64(0), "failures": list(), "blockers": list(), "errors": list()},
			ok:      true,
			message: "Applied 3 change(s) from cipi.yml to 'shop'",
		},
		{
			name:    "unchanged",
			result:  map[string]interface{}{"app": "shop", "outcome": "unchanged", "applied": float64(0)},
			ok:      true,
			message: "Nothing to do — 'shop' already matches cipi.yml",
		},
		{
			name:    "partial",
			result:  map[string]interface{}{"app": "shop", "outcome": "partial", "applied": float64(2), "failed": float64(1), "failures": list("add worker default")},
			message: "Applied 2 change(s) to 'shop', 1 failed",
			details: []string{"add worker default"},
			hint:    true,
		},
		{
			name:    "blocked",
			result:  map[string]interface{}{"app": "shop", "outcome": "blocked", "blockers": list("PHP 8.5 is not installed — run: cipi php install 8.5")},
			message: "Nothing was applied to 'shop' — resolve the blocking items first",
			details: []string{"PHP 8.5 is not installed — run: cipi php install 8.5"},
		},
		{
			name:    "invalid",
			result:  map[string]interface{}{"app": "shop", "outcome": "invalid", "errors": list("line 4: unknown key 'phpp'", "line 9: workers must be a list")},
			message: "The cipi.yml of 'shop' is not valid — nothing was applied",
			details: []string{"line 4: unknown key 'phpp'", "line 9: workers must be a list"},
		},
		{
			name:    "no-file uses the requested app name",
			result:  map[string]interface{}{"outcome": "no-file"},
			message: "No cipi.yml in the live release of 'myapp' — nothing to apply",
			hint:    true,
		},
		{
			name:    "missing outcome",
			result:  map[string]interface{}{"app": "shop"},
			message: "cipi.yml apply on 'shop' reported no outcome",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := describeYmlApplyResult(tc.result, "myapp")
			if r.OK != tc.ok || r.Message != tc.message {
				t.Fatalf("unexpected report: %+v", r)
			}
			if strings.Join(r.Details, "|") != strings.Join(tc.details, "|") {
				t.Fatalf("details = %#v, want %#v", r.Details, tc.details)
			}
			if (r.Hint != "") != tc.hint {
				t.Fatalf("hint = %q, want present=%v", r.Hint, tc.hint)
			}
			if got := ymlApplySucceeded(tc.result); got != tc.ok {
				t.Fatalf("ymlApplySucceeded = %v, want %v", got, tc.ok)
			}
		})
	}
}
