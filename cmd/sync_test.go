package cmd

import (
	"encoding/base64"
	"strings"
	"testing"
)

// Cipi 5.18.0 / cipi/api 1.48: `cipi-cli sync` — the apps of a server sent
// to another one. What is refused before a server is asked, what goes in
// the request, and what the check's answers read like.

// syncTestKey builds a public key of the type named, as ssh-keygen writes it.
func syncTestKey(kind, comment string) string {
	blob := append([]byte{0, 0, 0, byte(len(kind))}, []byte(kind)...)
	blob = append(blob, []byte{0, 0, 0, 32}...)
	blob = append(blob, []byte("0123456789abcdef0123456789abcdef")...)
	key := kind + " " + base64.StdEncoding.EncodeToString(blob)
	if comment != "" {
		key += " " + comment
	}
	return key
}

func TestSyncTargetIsAnAddressAndAPort(t *testing.T) {
	for _, host := range []string{"203.0.113.9", "fd7a:115c:a1e0::6f01", "fra2.tail1234.ts.net", "web2", "a"} {
		if err := syncTarget(host, 22); err != nil {
			t.Errorf("%q refused: %v", host, err)
		}
	}
	for _, host := range []string{"", "a b", "-oProxyCommand=id", "host;id", "host$(id)", "host`id`", "user@host", "host/", ".host", "host.", strings.Repeat("a", 255)} {
		if err := syncTarget(host, 22); err == nil {
			t.Errorf("%q taken as a target", host)
		}
	}
	for _, port := range []int{0, -1, 65536} {
		if err := syncTarget("web2", port); err == nil {
			t.Errorf("port %d taken", port)
		}
	}
	if err := syncTarget("web2", 65535); err != nil {
		t.Errorf("port 65535 refused: %v", err)
	}
}

func TestSyncEndpointHostIsTheSuggestedAddress(t *testing.T) {
	cases := map[string]string{
		"https://api.staging.example.com":   "api.staging.example.com",
		"https://203.0.113.77:8443/":        "203.0.113.77",
		" https://staging.example.com/api ": "staging.example.com",
		"http://[2001:db8::7]:8080":         "2001:db8::7",
		"":                                  "",
		"not a url":                         "",
		"https://":                          "",
		"https://bad_host.example.com/":     "",
	}
	for endpoint, want := range cases {
		if got := syncEndpointHost(endpoint); got != want {
			t.Errorf("%q: got %q, want %q", endpoint, got, want)
		}
	}
}

func TestSyncKeyLineTakesOneKeyAndCleansIt(t *testing.T) {
	ed := syncTestKey("ssh-ed25519", "cipi@prod")
	for raw, want := range map[string]string{
		ed:                                      ed,
		"  " + ed + "  \n":                      ed,
		"# the key of prod\n" + ed:              ed,
		strings.Replace(ed, " ", "   ", 1):      ed,
		syncTestKey("ssh-ed25519", ""):          syncTestKey("ssh-ed25519", ""),
		syncTestKey("ssh-rsa", "root@x"):        syncTestKey("ssh-rsa", "root@x"),
		syncTestKey("ecdsa-sha2-nistp256", "a"): syncTestKey("ecdsa-sha2-nistp256", "a"),
		// A comment of several words: the first is kept.
		syncTestKey("ssh-ed25519", "andrea") + " laptop 2026": syncTestKey("ssh-ed25519", "andrea"),
	} {
		got, err := syncKeyLine(raw)
		if err != nil || got != want {
			t.Errorf("%q: got %q, %v", raw, got, err)
		}
	}
}

func TestSyncKeyLineRefusesWhatIsNotOnePublicKey(t *testing.T) {
	ed := syncTestKey("ssh-ed25519", "cipi@prod")
	rsaBlob := strings.Fields(syncTestKey("ssh-rsa", ""))[1]
	for name, raw := range map[string]string{
		"nothing":          "",
		"two keys":         ed + "\n" + ed,
		"a word":           "hello",
		"a private key":    "-----BEGIN OPENSSH PRIVATE KEY-----",
		"an option before": `command="id" ` + ed,
		"a type it is not": "ssh-ed25519 " + rsaBlob + " cipi@prod",
		"an unknown type":  strings.Replace(ed, "ssh-ed25519", "ssh-dss", 1),
		"a short blob":     "ssh-ed25519 AAAA cipi@prod",
		"a comment with ;": syncTestKey("ssh-ed25519", "x;id"),
		"not base64":       "ssh-ed25519 " + strings.Repeat("!", 60),
	} {
		if got, err := syncKeyLine(raw); err == nil {
			t.Errorf("%s: taken as %q", name, got)
		}
	}
	if _, err := syncKeyLine("-----BEGIN OPENSSH PRIVATE KEY-----"); err == nil || !strings.Contains(err.Error(), "private key") {
		t.Errorf("a private key is not said to be one: %v", err)
	}
}

func TestSyncPushBodyNamesTheTargetAndTheAppsAndNoPassphrase(t *testing.T) {
	body, err := syncPushBody("203.0.113.9", 2222, []string{"shop", "blog", "shop"}, true, false)
	if err != nil {
		t.Fatal(err)
	}
	apps, _ := body["apps"].([]string)
	if strings.Join(apps, ",") != "shop,blog" {
		t.Errorf("apps: %v", apps)
	}
	if body["host"] != "203.0.113.9" || body["port"] != 2222 || body["with_db"] != true || body["with_storage"] != false {
		t.Errorf("body: %v", body)
	}
	if len(body) != 5 {
		t.Errorf("the body has %d fields, 5 expected: %v", len(body), body)
	}
	for field := range body {
		if strings.Contains(field, "pass") || strings.Contains(field, "key") {
			t.Errorf("%q has no place in the request", field)
		}
	}

	// No app named: every app, as an empty list — not a missing field.
	all, err := syncPushBody("web2", 22, nil, false, true)
	if err != nil {
		t.Fatal(err)
	}
	if list, ok := all["apps"].([]string); !ok || list == nil || len(list) != 0 {
		t.Errorf("apps of 'every app': %#v", all["apps"])
	}

	for _, bad := range [][]string{{""}, {"shop blog"}, {"--force"}, {"shop;id"}, {"$(id)"}, {"a/b"}} {
		if _, err := syncPushBody("web2", 22, bad, false, false); err == nil {
			t.Errorf("%q taken as an app", bad)
		}
	}
	if _, err := syncPushBody("web 2", 22, nil, false, false); err == nil {
		t.Error("a bad host passed")
	}
	if _, err := syncPushBody("web2", 0, nil, false, false); err == nil {
		t.Error("a bad port passed")
	}
}

func TestSyncVerdictReadsTheCheck(t *testing.T) {
	ready, line := syncVerdict(map[string]interface{}{"ok": true, "auto_passphrase": true, "cipi": "5.18.0"}, false)
	if !ready || !strings.Contains(line, "accepts this server's key") || !strings.Contains(line, "5.18.0") {
		t.Errorf("ready: %v %q", ready, line)
	}
	// It answers, but cannot take the passphrase from the source: not ready.
	if ready, line := syncVerdict(map[string]interface{}{"ok": true, "auto_passphrase": false, "cipi": "5.17.2"}, false); ready || !strings.Contains(line, "too old") {
		t.Errorf("old target: %v %q", ready, line)
	}
	if ready, _ := syncVerdict(map[string]interface{}{"ok": true}, true); ready {
		t.Error("a target that says nothing of the passphrase is taken as ready")
	}
	for reason, want := range map[string]string{
		"not-trusted":      "does not trust this server's key yet",
		"unreachable":      "does not answer over SSH",
		"host-key-changed": "ssh-keygen -R",
		"no-cipi":          "Cipi does not",
		"no-key":           "no sync key",
	} {
		ready, line := syncVerdict(map[string]interface{}{"ok": false, "reason": reason}, false)
		if ready || !strings.Contains(line, want) {
			t.Errorf("%s: %v %q", reason, ready, line)
		}
	}
	// A target named by its profile is prepared from here: the advice is another.
	if _, line := syncVerdict(map[string]interface{}{"ok": false, "reason": "not-trusted"}, true); !strings.Contains(line, "sync-manage") || strings.Contains(line, "--to") {
		t.Errorf("not trusted, named: %q", line)
	}
	if _, line := syncVerdict(map[string]interface{}{"ok": false, "reason": "else", "message": " Odd. "}, false); line != "Odd." {
		t.Errorf("the server's own words: %q", line)
	}
	if ready, line := syncVerdict(map[string]interface{}{}, false); ready || line != "The target could not be checked." {
		t.Errorf("an empty answer: %v %q", ready, line)
	}
}

func TestSyncSummarySaysWhatGoesWhereAndWhatIsReplaced(t *testing.T) {
	all := syncSummary(nil, true, false, "staging (203.0.113.77)")
	for _, want := range []string{"every app of this server", "to staging (203.0.113.77)", "with their databases", "their database there is replaced", "Nothing is removed on this server."} {
		if !strings.Contains(all, want) {
			t.Errorf("%q not in %q", want, all)
		}
	}
	one := syncSummary([]string{"shop"}, false, false, "web2")
	if !strings.Contains(one, "the app shop to web2, without databases or uploaded files") || strings.Contains(one, "replaced") {
		t.Errorf("one app, no data: %q", one)
	}
	two := syncSummary([]string{"shop", "blog"}, true, true, "web2")
	if !strings.Contains(two, "2 apps (shop, blog)") || !strings.Contains(two, "their database and storage there are replaced") {
		t.Errorf("two apps, everything: %q", two)
	}
	if s := syncSummary([]string{"shop"}, false, true, "web2"); !strings.Contains(s, "their storage there is replaced") {
		t.Errorf("storage only: %q", s)
	}
}
