package cmd

import (
	"fmt"

	"github.com/cipi-sh/cli/internal/output"
)

// printAppCreateResult prints the parsed `app-create` job result: credentials,
// deploy key, webhook, asset build and repository access. Absent fields are
// skipped, so older servers simply show less.
func printAppCreateResult(result map[string]interface{}) {
	if len(result) == 0 {
		return
	}
	fmt.Println()
	field := func(label, key string) {
		if v, ok := result[key]; ok && v != nil && v != "" {
			output.KeyValue(nil, label, formatFieldValue(v))
		}
	}
	pair := func(label, key string) {
		m, ok := result[key].(map[string]interface{})
		if !ok {
			return
		}
		output.KeyValue(nil, label, fmt.Sprintf("%s / %s", str(m, "user"), str(m, "password")))
	}
	field("Domain", "domain")
	field("PHP", "php")
	field("Home", "home")
	pair("SSH", "ssh")
	pair("Database", "database")
	field("Webhook", "webhook")
	field("Webhook token", "webhook_token")
	field("Build", "build")
	if key, ok := result["deploy_key"].(string); ok && key != "" {
		fmt.Println()
		output.Dim.Println("  Deploy key (add it to the repository with read access)")
		fmt.Printf("    %s\n", key)
	}
	if ra, ok := result["repository_access"].(map[string]interface{}); ok {
		fmt.Println()
		printRepositoryAccess(ra, "first")
	}
}

// repoAccessReport is the human form of a `repository_access` job result field.
type repoAccessReport struct {
	OK      bool     // true: print as success, false: print as warning
	Message string   // headline
	Details []string // extra lines (git error, fix command)
}

// describeRepositoryAccess turns `repository_access` (Cipi 5.6.0+ / API 1.34+:
// status ok|nobranch|denied, optional branch, error, hint) into a report.
// which is "first" after app create, "next" after app edit.
func describeRepositoryAccess(ra map[string]interface{}, which string) repoAccessReport {
	status, _ := ra["status"].(string)
	branch, _ := ra["branch"].(string)
	gitErr, _ := ra["error"].(string)
	hint, _ := ra["hint"].(string)

	var r repoAccessReport
	switch status {
	case "ok":
		r.OK = true
		r.Message = "Repository: reachable"
		if branch != "" {
			r.Message += fmt.Sprintf(" (branch %s)", branch)
		}
		return r
	case "nobranch":
		if branch != "" {
			r.Message = fmt.Sprintf("Repository: reachable, but it has no branch '%s' — deploys will fail until the branch exists or the app points to another one", branch)
		} else {
			r.Message = "Repository: reachable, but the configured branch does not exist — deploys will fail until it is fixed"
		}
	case "denied":
		r.Message = fmt.Sprintf("Repository: not readable with the deploy key — the %s deploy will fail until this is fixed", which)
	default:
		r.Message = fmt.Sprintf("Repository: %s", formatFieldValue(ra["status"]))
	}
	if gitErr != "" {
		r.Details = append(r.Details, "git: "+gitErr)
	}
	if hint != "" {
		r.Details = append(r.Details, "Fix on the server: "+hint)
	} else if status == "denied" {
		r.Details = append(r.Details, "Add the app's deploy key to the repository (read access), then deploy")
	}
	return r
}

func printRepositoryAccess(ra map[string]interface{}, which string) {
	r := describeRepositoryAccess(ra, which)
	if r.OK {
		output.Success("%s", r.Message)
	} else {
		output.Warn("%s", r.Message)
	}
	for _, d := range r.Details {
		fmt.Printf("    %s\n", d)
	}
}

// ymlApplyReport is the human form of an `app-yml-apply` job result.
type ymlApplyReport struct {
	OK      bool     // true: applied or unchanged
	Message string   // headline
	Details []string // failed changes, blockers or validation errors
	Hint    string   // what to do next, if anything
}

// ymlApplySucceeded is true for the outcomes that end the job as completed.
func ymlApplySucceeded(result map[string]interface{}) bool {
	outcome, _ := result["outcome"].(string)
	return outcome == "applied" || outcome == "unchanged"
}

// describeYmlApplyResult turns the `app-yml-apply` result (Cipi 5.6.4+ / API
// 1.35+: outcome applied|unchanged|partial|blocked|invalid|no-file, applied and
// failed counts, failures, blockers, errors) into a report. app is the name
// asked for, used when the result does not carry one.
func describeYmlApplyResult(result map[string]interface{}, app string) ymlApplyReport {
	if name, ok := result["app"].(string); ok && name != "" {
		app = name
	}
	outcome, _ := result["outcome"].(string)
	applied := ymlCount(result["applied"])
	failed := ymlCount(result["failed"])
	failures := ymlStrings(result["failures"])
	blockers := ymlStrings(result["blockers"])
	errs := ymlStrings(result["errors"])

	var r ymlApplyReport
	switch outcome {
	case "applied":
		r.OK = true
		if applied > 0 {
			r.Message = fmt.Sprintf("Applied %d change(s) from cipi.yml to '%s'", applied, app)
		} else {
			r.Message = fmt.Sprintf("cipi.yml applied to '%s'", app)
		}
	case "unchanged":
		r.OK = true
		r.Message = fmt.Sprintf("Nothing to do — '%s' already matches cipi.yml", app)
	case "partial":
		r.Message = fmt.Sprintf("Applied %d change(s) to '%s', %d failed", applied, app, failed)
		r.Details = append(r.Details, failures...)
		r.Hint = fmt.Sprintf("See what is left: cipi-cli yml plan %s", app)
	case "blocked":
		r.Message = fmt.Sprintf("Nothing was applied to '%s' — resolve the blocking items first", app)
		r.Details = append(r.Details, blockers...)
	case "invalid":
		r.Message = fmt.Sprintf("The cipi.yml of '%s' is not valid — nothing was applied", app)
	case "no-file":
		r.Message = fmt.Sprintf("No cipi.yml in the live release of '%s' — nothing to apply", app)
		r.Hint = "Commit a cipi.yml at the root of the repository, deploy, then apply"
	case "":
		r.Message = fmt.Sprintf("cipi.yml apply on '%s' reported no outcome", app)
	default:
		r.Message = fmt.Sprintf("cipi.yml apply on '%s' ended as %s", app, outcome)
	}
	// errors carries the validation messages (invalid) or any other error text.
	r.Details = append(r.Details, errs...)
	return r
}

func ymlCount(v interface{}) int {
	if f, ok := v.(float64); ok {
		return int(f)
	}
	return 0
}

func ymlStrings(v interface{}) []string {
	items, ok := v.([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if s := formatFieldValue(item); s != "" {
			out = append(out, s)
		}
	}
	return out
}
