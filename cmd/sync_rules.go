package cmd

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// Apps sent from one server to another (`cipi sync`) through the API — Cipi
// 5.18.0, cipi/api 1.48, ability sync-manage. What is checked here before a
// server is asked, and what its answers mean in words. Standard library
// only: the tests run without the client.

const syncAPI = "1.48"

var (
	// An address or a hostname, as the server takes it for a target.
	syncHostRe = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9.:-]{0,252}[A-Za-z0-9])?$`)
	// One public key: type, key, and a comment of one word.
	syncKeyRe = regexp.MustCompile(`^(ssh-ed25519|ssh-rsa|ecdsa-sha2-nistp(?:256|384|521)) ([A-Za-z0-9+/]{40,}={0,2})(?: ([A-Za-z0-9@._:+-]{1,128}))?$`)
)

// syncTarget checks the address and the port of a target.
func syncTarget(host string, port int) error {
	if !syncHostRe.MatchString(host) {
		return fmt.Errorf("the target is an IP address or a hostname the other server answers SSH on (got %q)", host)
	}
	if port < 1 || port > 65535 {
		return fmt.Errorf("--port is a number from 1 to 65535")
	}
	return nil
}

// syncEndpointHost is the host of a profile's API endpoint: the address
// suggested for a target named by its profile. Empty when there is none.
func syncEndpointHost(endpoint string) string {
	u, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || u.Hostname() == "" || !syncHostRe.MatchString(u.Hostname()) {
		return ""
	}
	return u.Hostname()
}

// syncKeyLine makes one clean line of a public key — as pasted, or as the
// content of an id_ed25519.pub — and refuses what is not one.
func syncKeyLine(raw string) (string, error) {
	var lines []string
	for _, line := range strings.Split(raw, "\n") {
		if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") {
			lines = append(lines, line)
		}
	}
	if len(lines) > 0 && strings.Contains(lines[0], "PRIVATE KEY") {
		return "", fmt.Errorf("that is a private key: the public one is asked for (cipi-cli sync key, or the .pub file)")
	}
	if len(lines) != 1 {
		return "", fmt.Errorf("one public key, on one line (got %d lines)", len(lines))
	}
	fields := strings.Fields(lines[0])
	if len(fields) > 3 {
		// A comment of several words: the first one is kept, it is only a label.
		fields = fields[:3]
	}
	key := strings.Join(fields, " ")
	m := syncKeyRe.FindStringSubmatch(key)
	if m == nil || len(m[2]) > 1026 {
		return "", fmt.Errorf("not an SSH public key: it reads like \"ssh-ed25519 AAAA… cipi@server\"")
	}
	// The key says its own type inside: "ssh-ed25519 <an rsa key>" is not a key.
	blob, err := base64.StdEncoding.DecodeString(m[2])
	if err != nil || len(blob) < 4+len(m[1]) || string(blob[4:4+len(m[1])]) != m[1] {
		return "", fmt.Errorf("not an SSH public key: what follows %q is not a key of that type", m[1])
	}
	return key, nil
}

// syncApps is the list of apps of a transfer, each once, in the order given.
func syncApps(names []string) ([]string, error) {
	apps := []string{}
	seen := map[string]bool{}
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" || strings.ContainsAny(name, " \t/\\'\"`$;&|<>") || strings.HasPrefix(name, "-") {
			return nil, fmt.Errorf("%q is not the name of an app", name)
		}
		if !seen[name] {
			seen[name] = true
			apps = append(apps, name)
		}
	}
	return apps, nil
}

// syncPushBody is the body of POST /api/sync/push. No apps: every app of
// the server. There is no passphrase in it: the source makes one for the
// transfer and hands it to the target over SSH.
func syncPushBody(host string, port int, apps []string, withDB, withStorage bool) (map[string]interface{}, error) {
	if err := syncTarget(host, port); err != nil {
		return nil, err
	}
	list, err := syncApps(apps)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"host":         host,
		"port":         port,
		"apps":         list,
		"with_db":      withDB,
		"with_storage": withStorage,
	}, nil
}

// syncVerdict reads the answer of POST /api/sync/check: is the way open,
// and one sentence about it. named: the target is a profile of this client,
// which it can prepare itself.
func syncVerdict(check map[string]interface{}, named bool) (bool, string) {
	ok, _ := check["ok"].(bool)
	auto, _ := check["auto_passphrase"].(bool)
	version, _ := check["cipi"].(string)
	if ok && auto {
		line := "The target accepts this server's key and Cipi answers there"
		if version != "" {
			line += " (" + version + ")"
		}
		return true, line + "."
	}
	if ok {
		return false, "The target answers, but its Cipi is too old to take a transfer started from here: update it there (cipi self-update), then try again."
	}
	reason, _ := check["reason"].(string)
	switch reason {
	case "not-trusted":
		if named {
			return false, "The target does not accept this server's key, and it could not be added there: look at the abilities of that profile's token (sync-manage)."
		}
		return false, "The target does not trust this server's key yet. On it: cipi sync trust, with the key of cipi-cli sync key — or name the target's profile with --to and it is done for you."
	case "unreachable":
		return false, "The target does not answer over SSH at that address and port — from the server, not from this machine. A private or mesh address may be the one to use (--host)."
	case "host-key-changed":
		return false, "The host key of the target is not the one this server knows. If the target was rebuilt: ssh-keygen -R <address> as root on this server, then try again."
	case "no-cipi":
		return false, "SSH answers at that address, but Cipi does not: it is not a Cipi server, or its cipi user cannot run it."
	case "no-key":
		return false, "This server has no sync key (/home/cipi/.ssh/id_ed25519): make one on it with ssh-keygen as the cipi user."
	}
	if msg, _ := check["message"].(string); strings.TrimSpace(msg) != "" {
		return false, strings.TrimSpace(msg)
	}
	return false, "The target could not be checked."
}

// syncSummary is the question asked before a transfer starts.
func syncSummary(apps []string, withDB, withStorage bool, target string) string {
	what := "every app of this server"
	switch {
	case len(apps) == 1:
		what = "the app " + apps[0]
	case len(apps) > 1:
		what = fmt.Sprintf("%d apps (%s)", len(apps), strings.Join(apps, ", "))
	}
	with := "without databases or uploaded files"
	replaced := ""
	switch {
	case withDB && withStorage:
		with, replaced = "with their databases and uploaded files", " (their database and storage there are replaced)"
	case withDB:
		with, replaced = "with their databases", " (their database there is replaced)"
	case withStorage:
		with, replaced = "with their uploaded files", " (their storage there is replaced)"
	}
	return fmt.Sprintf("Send %s to %s, %s? Apps already there are updated%s, the others are created. Nothing is removed on this server.", what, target, with, replaced)
}
