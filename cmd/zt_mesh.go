package cmd

import (
	"fmt"
	"strings"
)

// GET /api/zt in words, for the terminal: Cloudflare's lines and the mesh
// network's (Tailscale, NetBird — Cipi 5.18.0, cipi/api 1.48). `details`,
// the whole of `cipi zt status --json`, stays for --json: on one line it
// cannot be read. Standard library only: the tests run without the client.

// ztRow is one "label  value" line.
type ztRow struct{ Label, Value string }

func ztMap(v interface{}) map[string]interface{} {
	m, _ := v.(map[string]interface{})
	return m
}

func ztText(v interface{}) string {
	switch val := v.(type) {
	case nil:
		return ""
	case string:
		return val
	case bool:
		if val {
			return "yes"
		}
		return "no"
	case float64:
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d", int64(val))
		}
		return fmt.Sprintf("%g", val)
	default:
		return fmt.Sprintf("%v", val)
	}
}

func ztOr(v interface{}, fallback string) string {
	if s := ztText(v); s != "" {
		return s
	}
	return fallback
}

func ztYes(v interface{}) bool {
	b, _ := v.(bool)
	return b
}

// ztHasDetails says whether the server sent `details` (Cipi 5.16.0+): the
// readable summary is for those; an older one keeps the output it had.
func ztHasDetails(data map[string]interface{}) bool {
	return ztMap(data["details"]) != nil
}

// ztCloudflareRows are the tunnel's lines, in a fixed order.
func ztCloudflareRows(data map[string]interface{}) []ztRow {
	if !ztYes(data["enabled"]) {
		return []ztRow{{"tunnel", "off — cipi zt enable (on the server)"}}
	}
	details := ztMap(data["details"])
	tunnel := ztMap(details["tunnel"])
	state := ztOr(data["cloudflared"], "—")
	if s := ztText(tunnel["status"]); s != "" {
		state += " — Cloudflare says " + s
	}
	rows := []ztRow{
		{"tunnel", ztOr(data["tunnel"], "—")},
		{"cloudflared", state},
		{"real ip", ztOr(data["real_ip"], "—")},
		{"lock http", ztOr(data["lock_http"], "—")},
		{"lock ssh", ztOr(data["lock_ssh"], "—")},
		{"ssh hostname", ztOr(data["ssh_hostname"], "—")},
	}
	if hosts, ok := details["hostnames"].([]interface{}); ok {
		rows = append(rows, ztRow{"hostnames", fmt.Sprintf("%d", len(hosts))})
	}
	if missing, ok := details["missing"].([]interface{}); ok && len(missing) > 0 {
		names := make([]string, 0, len(missing))
		for _, m := range missing {
			names = append(names, ztText(m))
		}
		rows = append(rows, ztRow{"not on tunnel", strings.Join(names, ", ")})
	}
	return rows
}

// ztMeshRows are the mesh network's lines. Nil when the server says nothing
// of a mesh (Cipi < 5.18.0); one line when it is on none.
func ztMeshRows(data map[string]interface{}) []ztRow {
	mesh := ztMap(ztMap(data["details"])["mesh"])
	if mesh == nil {
		return nil
	}
	provider := ztText(mesh["provider"])
	if !ztYes(mesh["enabled"]) || provider == "" {
		return []ztRow{{"mesh", "off — cipi zt tailscale enable | cipi zt netbird enable (on the server)"}}
	}
	label := ztOr(mesh["label"], provider)
	state := "disconnected"
	if ztYes(mesh["connected"]) {
		state = "connected"
	}
	if s := ztText(mesh["state"]); s != "" && !strings.EqualFold(s, state) {
		state += " (" + s + ")"
	}
	address := ztOr(mesh["ip"], "none")
	if fqdn := ztText(mesh["fqdn"]); fqdn != "" {
		address += "   " + fqdn
	}
	network := ztOr(mesh["network"], "—")
	if url := ztText(mesh["control_url"]); url != "" {
		network += "  via " + url
	}
	peers := ztMap(mesh["peers"])
	rows := []ztRow{
		{"mesh", label + " — " + state},
		{"address", address},
		{"network", network},
		{"peers", ztOr(peers["online"], "0") + "/" + ztOr(peers["total"], "0") + " online"},
	}
	if provider == "tailscale" {
		key := "does not expire"
		if expiry := ztText(mesh["key_expiry"]); expiry != "" {
			day, _, _ := strings.Cut(expiry, "T")
			key = "expires " + day
			if days := ztText(mesh["key_expires_in_days"]); days != "" {
				key += " (" + days + " days)"
			}
		}
		rows = append(rows, ztRow{"node key", key})
	}
	ssh := ztMap(mesh["ssh"])
	if cmd := ztText(ssh["command"]); cmd != "" {
		if ztYes(ssh["provider_ssh"]) {
			cmd += "   (" + label + " SSH)"
		}
		rows = append(rows, ztRow{"ssh", cmd})
	}
	lock := ztMap(mesh["lock"])
	rows = append(rows,
		ztRow{"lock ssh", ztText(ztYes(lock["ssh"]))},
		ztRow{"lock http", ztText(ztYes(lock["http"]))},
	)
	if warnings, ok := mesh["warnings"].([]interface{}); ok {
		for _, w := range warnings {
			if s := ztText(w); s != "" {
				rows = append(rows, ztRow{"warning", s})
			}
		}
	}
	return rows
}
