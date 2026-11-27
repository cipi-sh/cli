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
	if len(rows) != 1 || !strings.HasPrefix(rows[0].Value, "off") || !strings.Contains(rows[0].Value, "cipi-cli zt join tailscale|netbird") {
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

// The mesh managed from the client: `zt join|leave|lock|unlock|ssh`.

func TestZtJoinBodyHasOnlyWhatWasGivenAndWhatTheProviderHas(t *testing.T) {
	body, err := ztJoinBody("tailscale", ztJoinOptions{Key: " tskey-auth-kABC-0123456789 ", Hostname: "web1", Tags: "cipi, tag:prod,cipi", Direct: true, SSH: true})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := json.Marshal(body)
	want := `{"direct":true,"hostname":"web1","key":"tskey-auth-kABC-0123456789","provider":"tailscale","ssh":true,"tags":["tag:cipi","tag:prod"]}`
	if string(got) != want {
		t.Fatalf("tailscale body:\n got %s\nwant %s", got, want)
	}

	body, err = ztJoinBody("netbird", ztJoinOptions{Key: "A1B2C3D4-E5F6-7890", ControlURL: "https://netbird.example.com:443", AcceptRoutes: true})
	if err != nil {
		t.Fatal(err)
	}
	got, _ = json.Marshal(body)
	want = `{"accept_routes":true,"control_url":"https://netbird.example.com:443","key":"A1B2C3D4-E5F6-7890","provider":"netbird"}`
	if string(got) != want {
		t.Fatalf("netbird body:\n got %s\nwant %s", got, want)
	}

	// --adopt: no key at all.
	body, err = ztJoinBody("tailscale", ztJoinOptions{})
	if err != nil || len(body) != 1 || body["provider"] != "tailscale" {
		t.Fatalf("adopt: %v %v", body, err)
	}
}

func TestZtJoinRefusesWhatTheServerWouldRefuse(t *testing.T) {
	for name, o := range map[string]ztJoinOptions{
		"a key with a quote":    {Key: "tskey'; id #x"},
		"a key with a space":    {Key: "has a space key"},
		"a short key":           {Key: "short"},
		"a hostname":            {Key: "tskey-abcdef", Hostname: "web_1"},
		"a control url":         {Key: "tskey-abcdef", ControlURL: "ftp://x"},
		"a control url, quoted": {Key: "tskey-abcdef", ControlURL: "https://hs.test'; id"},
		"a tag":                 {Key: "tskey-abcdef", Tags: "ok,no way"},
		"reauth without a key":  {Re: true},
	} {
		if _, err := ztJoinBody("tailscale", o); err == nil {
			t.Errorf("%s: not refused", name)
		}
	}
	for name, o := range map[string]ztJoinOptions{
		"tags":   {Key: "abcdefgh", Tags: "cipi"},
		"ssh":    {Key: "abcdefgh", SSH: true},
		"reauth": {Key: "abcdefgh", Re: true},
	} {
		if _, err := ztJoinBody("netbird", o); err == nil || !strings.Contains(err.Error(), "Tailscale's") {
			t.Errorf("netbird %s: %v", name, err)
		}
	}
}

func TestZtNamesAreTheOnesTheAPITakes(t *testing.T) {
	for in, want := range map[string]string{"tailscale": "tailscale", "TS": "tailscale", " netbird ": "netbird", "nb": "netbird"} {
		if got, err := ztProvider(in); err != nil || got != want {
			t.Errorf("provider %q: %q %v", in, got, err)
		}
	}
	if _, err := ztProvider("zerotier"); err == nil {
		t.Error("zerotier is not a provider")
	}
	for in, want := range map[string]string{"ssh": "ssh", "22": "ssh", "http": "http", "HTTPS": "http", "443": "http", "web": "http"} {
		if got, err := ztPort(in); err != nil || got != want {
			t.Errorf("port %q: %q %v", in, got, err)
		}
	}
	for _, bad := range []string{"", "rdp", "3306", "22; id"} {
		if _, err := ztPort(bad); err == nil {
			t.Errorf("port %q: not refused", bad)
		}
	}
	for in, want := range map[string]bool{"on": true, "enable": true, "off": false, "Disable": false} {
		if got, err := ztOnOff(in); err != nil || got != want {
			t.Errorf("on/off %q: %v %v", in, got, err)
		}
	}
	if _, err := ztOnOff("maybe"); err == nil {
		t.Error("maybe is neither on nor off")
	}
	if tags, err := ztTags(""); err != nil || len(tags) != 0 {
		t.Errorf("no tags: %v %v", tags, err)
	}
}

func TestZtShowsWhatTheServerWouldRefuseFromHere(t *testing.T) {
	data := ztData(t, `{"enabled":false,"mesh":"tailscale","details":{"enabled":false,
		"mesh":{"enabled":true,"provider":"tailscale","label":"Tailscale","connected":true,"state":"Running","ip":"100.90.14.22","peers":{"total":4,"online":3},
			"ssh":{"provider_ssh":false,"command":"ssh cipi@100.90.14.22","seen":null},"lock":{"ssh":false,"http":false},"warnings":[],
			"panel":{"via":"public","from":"203.0.113.9","blocked":{"lock_ssh":"Nobody has signed in over SSH through Tailscale yet.","lock_http":"The panel reaches this server's API on its public address.","disable":null}}}}}`)
	lines := ztLines(ztMeshRows(data))
	for _, want := range []string{
		"last ssh login=nobody over the mesh yet",
		"this client=reaches the server via public (from 203.0.113.9)",
		"refused=zt lock ssh — Nobody has signed in over SSH through Tailscale yet.",
		"refused=zt lock http — The panel reaches this server's API on its public address.",
	} {
		if !strings.Contains(lines, want) {
			t.Errorf("missing %q in:\n%s", want, lines)
		}
	}
	if strings.Contains(lines, "zt leave") {
		t.Errorf("leave is not refused:\n%s", lines)
	}

	data = ztData(t, `{"enabled":false,"details":{"mesh":{"enabled":true,"provider":"netbird","connected":true,"ip":"100.76.3.41",
		"ssh":{"command":"ssh cipi@100.76.3.41","seen":{"at":"2026-11-27T12:00:01","from":"100.76.9.9"}},"lock":{"ssh":true,"http":false},
		"panel":{"via":"mesh","from":"100.76.9.9","blocked":{"lock_ssh":null,"lock_http":null,"disable":"The panel reaches this server over NetBird: leaving the network would cut it off."}}}}}`)
	lines = ztLines(ztMeshRows(data))
	for _, want := range []string{"last ssh login=2026-11-27T12:00:01 from 100.76.9.9", "this client=reaches the server via mesh (from 100.76.9.9)", "refused=zt leave — The panel reaches this server over NetBird"} {
		if !strings.Contains(lines, want) {
			t.Errorf("missing %q in:\n%s", want, lines)
		}
	}
	if ztMeshLabel(data) != "NetBird" || ztMeshLabel(map[string]interface{}{}) != "mesh" {
		t.Errorf("label: %q", ztMeshLabel(data))
	}

	// A server on no mesh says how to join one from here.
	off := ztLines(ztMeshRows(ztData(t, `{"details":{"mesh":{"enabled":false,"provider":null}}}`)))
	if off != "mesh=off — cipi-cli zt join tailscale|netbird" {
		t.Errorf("off: %s", off)
	}
}

func TestZtLockAsksInWords(t *testing.T) {
	if q := ztLockQuestion("ssh", "Tailscale"); !strings.Contains(q, "Port 22") || !strings.Contains(q, "Tailscale network only") {
		t.Errorf("ssh: %s", q)
	}
	if q := ztLockQuestion("http", "mesh"); !strings.Contains(q, "80/443") || !strings.Contains(q, "the panel and the API") {
		t.Errorf("http: %s", q)
	}
}
