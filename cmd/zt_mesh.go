package cmd

import (
	"fmt"
	"regexp"
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
		return []ztRow{{"mesh", "off — cipi-cli zt join tailscale|netbird"}}
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
	// The last login sshd accepted over the mesh: what `zt lock ssh` asks for.
	if seen := ztMap(ssh["seen"]); seen != nil {
		last := ztOr(seen["at"], "?")
		if from := ztText(seen["from"]); from != "" {
			last += " from " + from
		}
		rows = append(rows, ztRow{"last ssh login", last})
	} else if _, said := ssh["seen"]; said {
		rows = append(rows, ztRow{"last ssh login", "nobody over the mesh yet"})
	}
	lock := ztMap(mesh["lock"])
	rows = append(rows,
		ztRow{"lock ssh", ztText(ztYes(lock["ssh"]))},
		ztRow{"lock http", ztText(ztYes(lock["http"]))},
	)
	// How this call reached the server, and what the server would refuse from here.
	if panel := ztMap(mesh["panel"]); panel != nil {
		via := ztOr(panel["via"], "unknown")
		if from := ztText(panel["from"]); from != "" {
			via += " (from " + from + ")"
		}
		rows = append(rows, ztRow{"this client", "reaches the server via " + via})
		blocked := ztMap(panel["blocked"])
		for _, b := range [][2]string{{"lock_ssh", "zt lock ssh"}, {"lock_http", "zt lock http"}, {"disable", "zt leave"}} {
			if why := ztText(blocked[b[0]]); why != "" {
				rows = append(rows, ztRow{"refused", b[1] + " — " + why})
			}
		}
	}
	if warnings, ok := mesh["warnings"].([]interface{}); ok {
		for _, w := range warnings {
			if s := ztText(w); s != "" {
				rows = append(rows, ztRow{"warning", s})
			}
		}
	}
	return rows
}

// ── changing the mesh (cipi/api 1.48, ability zt-manage) ──

var (
	ztKeyPattern      = regexp.MustCompile(`^[A-Za-z0-9._~:?&=%+/-]{8,512}$`)
	ztHostnamePattern = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$`)
	ztTagPattern      = regexp.MustCompile(`^(tag:)?[a-zA-Z][a-zA-Z0-9-]{0,62}$`)
	ztURLPattern      = regexp.MustCompile(`^https?://[A-Za-z0-9._-]+(:[0-9]{1,5})?(/[A-Za-z0-9._~/-]*)?$`)
)

// ztJoinOptions is what `zt join` was given.
type ztJoinOptions struct {
	Key, Hostname, ControlURL, Tags          string
	Direct, AcceptDNS, AcceptRoutes, SSH, Re bool
}

// ztProvider is the name the API takes: tailscale or netbird (ts and nb for short).
func ztProvider(arg string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(arg)) {
	case "tailscale", "ts":
		return "tailscale", nil
	case "netbird", "nb":
		return "netbird", nil
	}
	return "", fmt.Errorf("%q is not a mesh network: tailscale or netbird", arg)
}

// ztPort is the name the API takes for what a lock closes: ssh, or http (80 and 443).
func ztPort(arg string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(arg)) {
	case "ssh", "22":
		return "ssh", nil
	case "http", "https", "web", "80", "443":
		return "http", nil
	}
	return "", fmt.Errorf("%q is not a port a lock closes: ssh, or http (80 and 443)", arg)
}

// ztTags turns "cipi, tag:prod" into the list the API takes.
func ztTags(typed string) ([]string, error) {
	out := []string{}
	seen := map[string]bool{}
	for _, tag := range strings.Split(typed, ",") {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if !ztTagPattern.MatchString(tag) {
			return nil, fmt.Errorf("%q is not a tag: letters, digits and hyphens, such as cipi or tag:prod", tag)
		}
		if !strings.HasPrefix(tag, "tag:") {
			tag = "tag:" + tag
		}
		if !seen[tag] {
			seen[tag] = true
			out = append(out, tag)
		}
	}
	return out, nil
}

// ztJoinBody is the body of POST /api/zt/mesh: only what was given, only
// what the provider has. The rules are the server's, said before the call.
func ztJoinBody(provider string, o ztJoinOptions) (map[string]interface{}, error) {
	body := map[string]interface{}{"provider": provider}
	if key := strings.TrimSpace(o.Key); key != "" {
		if !ztKeyPattern.MatchString(key) {
			return nil, fmt.Errorf("that does not look like a key: 8 to 512 characters, no spaces or quotes")
		}
		body["key"] = key
	}
	if o.Hostname != "" {
		if !ztHostnamePattern.MatchString(o.Hostname) {
			return nil, fmt.Errorf("--hostname is letters, digits and hyphens, up to 63")
		}
		body["hostname"] = o.Hostname
	}
	if o.ControlURL != "" {
		if len(o.ControlURL) > 255 || !ztURLPattern.MatchString(o.ControlURL) {
			return nil, fmt.Errorf("--control-url is the coordination server: https://host or https://host:port")
		}
		body["control_url"] = o.ControlURL
	}
	if o.Direct {
		body["direct"] = true
	}
	if o.AcceptDNS {
		body["accept_dns"] = true
	}
	if o.AcceptRoutes {
		body["accept_routes"] = true
	}
	if provider != "tailscale" {
		if o.Tags != "" || o.SSH || o.Re {
			return nil, fmt.Errorf("--tags, --ssh and --reauth are Tailscale's: NetBird has groups on its setup key and OpenSSH for SSH")
		}
		return body, nil
	}
	tags, err := ztTags(o.Tags)
	if err != nil {
		return nil, err
	}
	if len(tags) > 0 {
		body["tags"] = tags
	}
	if o.SSH {
		body["ssh"] = true
	}
	if o.Re {
		if _, ok := body["key"]; !ok {
			return nil, fmt.Errorf("--reauth signs the machine in again: it needs a key")
		}
		body["reauth"] = true
	}
	return body, nil
}

// ztOnOff reads the argument of `zt ssh`.
func ztOnOff(arg string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(arg)) {
	case "on", "enable", "true", "yes":
		return true, nil
	case "off", "disable", "false", "no":
		return false, nil
	}
	return false, fmt.Errorf("%q: on or off", arg)
}

// ztLockQuestion is what `zt lock` asks before it closes a port.
func ztLockQuestion(port, label string) string {
	if port == "ssh" {
		return "Close SSH to the world? Port 22 will answer on the " + label + " network only."
	}
	return "Close 80/443 to the world? The sites, the panel and the API will answer on the " + label + " network only."
}

// ztMeshLabel is the name of the mesh the server is on, from a status or a change's answer; "mesh" when none.
func ztMeshLabel(data map[string]interface{}) string {
	mesh := ztMap(ztMap(data["details"])["mesh"])
	if l := ztText(mesh["label"]); l != "" {
		return l
	}
	switch ztText(mesh["provider"]) {
	case "tailscale":
		return "Tailscale"
	case "netbird":
		return "NetBird"
	}
	return "mesh"
}
