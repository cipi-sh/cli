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
