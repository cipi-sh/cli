package cmd

import (
	"encoding/json"
	"strings"
	"testing"
)

// Cipi 5.18.0 / cipi/api 1.48: the mesh network a server is on — Tailscale or
// NetBird — in `cipi-cli zt`, next to Cloudflare's tunnel.

func ztData(t *testing.T, raw string) map[string]interface{} {
	t.Helper()
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	return data
}

func ztLines(rows []ztRow) string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Label+"="+r.Value)
	}
	return strings.Join(out, "\n")
}

func TestZtShowsTheMeshNextToTheTunnel(t *testing.T) {
	data := ztData(t, `{"enabled":true,"cloudflared":"running","tunnel":"fra1-prod","real_ip":"CF-Connecting-IP","lock_http":"true (cf-ips)","lock_ssh":"false","ssh_hostname":null,
		"mesh":"tailscale","mesh_state":"connected","mesh_ip":"100.101.102.103",
		"details":{"enabled":true,"tunnel":{"status":"healthy"},"hostnames":[{"app":"shop"},{"app":"blog"}],"missing":["shopapi"],
			"mesh":{"enabled":true,"provider":"tailscale","label":"Tailscale","connected":true,"state":"Running","ip":"100.101.102.103","fqdn":"fra1.tail1234.ts.net",
				"network":"example.com","control_url":null,"key_expiry":null,"key_expires_in_days":null,"peers":{"total":6,"online":4},
				"ssh":{"provider_ssh":false,"command":"ssh cipi@100.101.102.103"},"lock":{"ssh":true,"http":false},"warnings":[]}}}`)
	if !ztHasDetails(data) {
		t.Fatal("details not seen")
	}
	cf := ztLines(ztCloudflareRows(data))
	for _, want := range []string{"tunnel=fra1-prod", "cloudflared=running — Cloudflare says healthy", "lock http=true (cf-ips)", "ssh hostname=—", "hostnames=2", "not on tunnel=shopapi"} {
		if !strings.Contains(cf, want) {
			t.Errorf("cloudflare rows lack %q:\n%s", want, cf)
		}
	}
	mesh := ztLines(ztMeshRows(data))
	want := "mesh=Tailscale — connected (Running)\naddress=100.101.102.103   fra1.tail1234.ts.net\nnetwork=example.com\npeers=4/6 online\nnode key=does not expire\nssh=ssh cipi@100.101.102.103\nlock ssh=yes\nlock http=no"
	if mesh != want {
		t.Errorf("mesh rows:\n%s\nwant:\n%s", mesh, want)
	}
}

func TestZtMeshWithoutATunnelAndItsWarnings(t *testing.T) {
	data := ztData(t, `{"enabled":false,"tunnel":null,"details":{"enabled":false,
		"mesh":{"enabled":true,"provider":"tailscale","label":"Tailscale","connected":true,"state":"Running","ip":"100.101.102.103",
			"key_expiry":"2027-01-06T10:00:00Z","key_expires_in_days":40,"peers":{"total":1,"online":1},
			"ssh":{"provider_ssh":true,"command":"ssh cipi@100.101.102.103"},"lock":{"ssh":false,"http":false},
			"warnings":["The node key expires in 40 days: disable key expiry for this machine, or join with a tagged key"]}}}`)
	if cf := ztLines(ztCloudflareRows(data)); !strings.HasPrefix(cf, "tunnel=off") {
		t.Errorf("a server with no tunnel: %s", cf)
	}
	mesh := ztLines(ztMeshRows(data))
	for _, want := range []string{"node key=expires 2027-01-06 (40 days)", "ssh=ssh cipi@100.101.102.103   (Tailscale SSH)", "warning=The node key expires in 40 days"} {
		if !strings.Contains(mesh, want) {
			t.Errorf("mesh rows lack %q:\n%s", want, mesh)
		}
	}
}

func TestZtNetBirdDownHasNoNodeKeyLine(t *testing.T) {
	data := ztData(t, `{"enabled":false,"details":{"mesh":{"enabled":true,"provider":"netbird","label":"NetBird","connected":false,"state":"NeedsLogin",
		"ip":null,"control_url":"https://nb.example.com:443","peers":{"total":0,"online":0},"ssh":{"provider_ssh":false,"command":null},"lock":{"ssh":true,"http":false},
		"warnings":["SSH is locked to the mesh and the mesh is down: the provider's console is the way in"]}}}`)
	mesh := ztLines(ztMeshRows(data))
	want := "mesh=NetBird — disconnected (NeedsLogin)\naddress=none\nnetwork=—  via https://nb.example.com:443\npeers=0/0 online\nlock ssh=yes\nlock http=no\nwarning=SSH is locked to the mesh and the mesh is down: the provider's console is the way in"
	if mesh != want {
		t.Errorf("mesh rows:\n%s\nwant:\n%s", mesh, want)
	}
}

func TestZtNoMeshAndAnOlderServer(t *testing.T) {
	off := ztData(t, `{"enabled":true,"details":{"enabled":true,"mesh":{"enabled":false,"provider":null,"active":null}}}`)
	rows := ztMeshRows(off)
	if len(rows) != 1 || !strings.HasPrefix(rows[0].Value, "off") || !strings.Contains(rows[0].Value, "cipi zt tailscale enable") {
		t.Errorf("no mesh on: %v", rows)
	}
	// Cipi 5.16/5.17: details, and no word of a mesh.
	if rows := ztMeshRows(ztData(t, `{"enabled":true,"details":{"enabled":true}}`)); rows != nil {
		t.Errorf("a server that says nothing of a mesh got a mesh section: %v", rows)
	}
	// Older still: no details — the command prints what it always printed.
	old := ztData(t, `{"enabled":true,"cloudflared":"running","raw":"Cloudflare Zero Trust","details":null}`)
	if ztHasDetails(old) || ztMeshRows(old) != nil {
		t.Error("a server without details was read as one with them")
	}
}
