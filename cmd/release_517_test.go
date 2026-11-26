package cmd

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
)

// Cipi 5.17.0 / cipi/api 1.47: the page cache of an app, the PHP of an Octane
// app, Valkey per app, and how notifications read.

func TestCacheTTLAndSizeAreTheServersRules(t *testing.T) {
	for _, ok := range []string{"1s", "30s", "10m", "1h", "7d", "30d", "2592000s"} {
		if !validCacheTTL(ok) {
			t.Errorf("ttl %q was refused", ok)
		}
	}
	for _, bad := range []string{"", "0s", "10", "10 m", "10M", "31d", "2592001s", "1w", "-5m", "5m; rm"} {
		if validCacheTTL(bad) {
			t.Errorf("ttl %q was accepted", bad)
		}
	}
	for _, ok := range []string{"16m", "256m", "1g", "100g"} {
		if !validCacheSize(ok) {
			t.Errorf("size %q was refused", ok)
		}
	}
	for _, bad := range []string{"", "15m", "101g", "1G", "256", "256mb", "0g"} {
		if validCacheSize(bad) {
			t.Errorf("size %q was accepted", bad)
		}
	}
}

func TestCacheOnSendsOnlyWhatWasGiven(t *testing.T) {
	body, err := cacheOnBody("", "", nil, nil)
	if err != nil || len(body) != 1 || body["enabled"] != true {
		t.Fatalf("plain on: %v %v", body, err)
	}
	no := false
	body, err = cacheOnBody("1h", "2g", &no, nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(body)
	if string(raw) != `{"clear_on_deploy":false,"enabled":true,"size":"2g","ttl":"1h"}` {
		t.Errorf("body: %s", raw)
	}
	if _, err := cacheOnBody("ten minutes", "", nil, nil); err == nil {
		t.Error("a ttl that is not a time was sent")
	}
	if _, err := cacheOnBody("", "8m", nil, nil); err == nil {
		t.Error("a size under 16m was sent")
	}
	// The app's own paths: the whole list when given, an empty one for none.
	body, _ = cacheOnBody("", "", nil, []string{"/cart*", "/feed"})
	raw, _ = json.Marshal(body)
	if string(raw) != `{"enabled":true,"exclude":["/cart*","/feed"]}` {
		t.Errorf("body with paths: %s", raw)
	}
	body, _ = cacheOnBody("", "", nil, []string{})
	raw, _ = json.Marshal(body)
	if string(raw) != `{"enabled":true,"exclude":[]}` {
		t.Errorf("body with no path: %s", raw)
	}
}

func TestCacheExcludePaths(t *testing.T) {
	got, err := cacheExcludePaths([]string{"cart*", " /feed , /cart*", "/a**b", "/caff%C3%A8_1.0~x", "/"})
	if err != nil || strings.Join(got, " ") != "/cart* /feed /a*b /caff%C3%A8_1.0~x /" {
		t.Errorf("paths: %v %v", got, err)
	}
	if got, err := cacheExcludePaths([]string{"", " , "}); err != nil || len(got) != 0 {
		t.Errorf("nothing given is an empty list: %v %v", got, err)
	}
	for _, bad := range []string{"/a b", "/x';id", "/a;b", `/a"b`, "/a$b", "/a{b}", "/a|b", "/(a)", `/a\b`, "/a?b=1", "/a#b", "/a\nb", "/" + strings.Repeat("a", 128)} {
		if _, err := cacheExcludePaths([]string{bad}); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
	for _, all := range []string{"*", "/*", "/**"} {
		if _, err := cacheExcludePaths([]string{all}); err == nil || !strings.Contains(err.Error(), "every page") {
			t.Errorf("%q is every page: %v", all, err)
		}
	}
	many := make([]string, 51)
	for i := range many {
		many[i] = "/p" + strconv.Itoa(i)
	}
	if _, err := cacheExcludePaths(many); err == nil {
		t.Error("51 paths were accepted")
	}
	if _, err := cacheExcludePaths(many[:50]); err != nil {
		t.Errorf("50 paths: %v", err)
	}
}

func TestCacheExcludeAddAndRemove(t *testing.T) {
	now := []string{"/cart*", "/feed"}
	if got := cacheExcludeAdd(now, []string{"/feed", "/x", "/x"}); strings.Join(got, " ") != "/cart* /feed /x" {
		t.Errorf("add: %v", got)
	}
	if strings.Join(now, " ") != "/cart* /feed" {
		t.Errorf("add changed the list it was given: %v", now)
	}
	out, missing := cacheExcludeRemove(now, []string{"/feed", "/nope"})
	if strings.Join(out, " ") != "/cart*" || strings.Join(missing, " ") != "/nope" {
		t.Errorf("remove: %v %v", out, missing)
	}
	out, _ = cacheExcludeRemove(nil, []string{"/x"})
	if raw, _ := json.Marshal(out); string(raw) != "[]" {
		t.Errorf("an empty list is [], not null: %s", raw)
	}
	var c pageCacheResponse
	if err := json.Unmarshal([]byte(`{"data":{"app":"blog","enabled":true,"exclude":["/cart*"],"exclude_max":50}}`), &c); err != nil || len(c.Data.Exclude) != 1 || c.Data.ExcludeMax != 50 {
		t.Errorf("status: %+v %v", c.Data, err)
	}
	if got := cachePath("blog", "/exclude"); got != "/api/apps/blog/cache/exclude" {
		t.Errorf("got %s", got)
	}
}

func TestCachePathEscapesTheApp(t *testing.T) {
	if got := cachePath("blog", "/clear"); got != "/api/apps/blog/cache/clear" {
		t.Errorf("got %s", got)
	}
	if got := cachePath("../x", ""); strings.Contains(got, "/../") {
		t.Errorf("the app is one path segment: %s", got)
	}
	if got := octanePath("shop", "/php"); got != "/api/apps/shop/octane/php" {
		t.Errorf("got %s", got)
	}
}

func TestPageCacheState(t *testing.T) {
	cases := map[string]pageCache{
		"n/a (Node)":                        {Supported: false, Enabled: false},
		"off":                               {Supported: true},
		"on":                                {Supported: true, Enabled: true, Active: true},
		"on (not in the vhost: suspended?)": {Supported: true, Enabled: true},
	}
	for want, c := range cases {
		if got := c.state(); got != want {
			t.Errorf("state %+v = %q, want %q", c, got, want)
		}
	}
	// `cleared` is there after a clear, and 0 is a value.
	var r pageCacheResponse
	if err := json.Unmarshal([]byte(`{"data":{"app":"blog","enabled":true,"cleared":0}}`), &r); err != nil || r.Data.Cleared == nil || *r.Data.Cleared != 0 {
		t.Errorf("cleared: %+v %v", r.Data.Cleared, err)
	}
}

func TestOctaneFixIsThisClientsCommand(t *testing.T) {
	cases := map[string]string{
		"cipi frankenphp php shop 8.5": "cipi-cli octane php shop 8.5",
		"cipi php install 8.6":         "cipi-cli php install 8.6",
		"cipi frankenphp import x":     "cipi frankenphp import x",
	}
	for in, want := range cases {
		if got := octaneFixHere(in); got != want {
			t.Errorf("octaneFixHere(%q) = %q, want %q", in, got, want)
		}
	}
	if !octanePHPPattern.MatchString("8.5") || octanePHPPattern.MatchString("8.5.1") || octanePHPPattern.MatchString("latest") || octanePHPPattern.MatchString("8.5 --force") {
		t.Error("a PHP version is X.Y and nothing else")
	}
	var o struct {
		Data octanePHP `json:"data"`
	}
	raw := `{"data":{"app":"shop","php":"8.5","octane_php":"8.6.0","build":"1.14.0-php8.6.0-99137391","state":"mismatch","available_php":["8.5","8.6"],"fix":{"command":"cipi frankenphp php shop 8.5","why":"a build with PHP 8.5 is on this server"}}}`
	if err := json.Unmarshal([]byte(raw), &o); err != nil || o.Data.Fix == nil || o.Data.Fix.Command == "" || len(o.Data.AvailablePHP) != 2 {
		t.Errorf("octane: %+v %v", o.Data, err)
	}
	raw = `{"data":{"app":"shop","php":"8.5","octane_php":null,"build":null,"state":"unpinned","fix":null}}`
	var fresh struct {
		Data octanePHP `json:"data"`
	}
	if err := json.Unmarshal([]byte(raw), &fresh); err != nil || fresh.Data.OctanePHP != "" || fresh.Data.Build != "" || fresh.Data.Fix != nil {
		t.Errorf("octane with nulls: %+v %v", fresh.Data, err)
	}
}

func TestValkeyMoveBody(t *testing.T) {
	if _, err := valkeyMoveBody("", false, false); err == nil {
		t.Error("neither an app nor --all was accepted")
	}
	if _, err := valkeyMoveBody("shop", true, false); err == nil {
		t.Error("an app and --all together were accepted")
	}
	body, _ := valkeyMoveBody("shop", false, false)
	raw, _ := json.Marshal(body)
	if string(raw) != `{"app":"shop"}` {
		t.Errorf("one app: %s", raw)
	}
	body, _ = valkeyMoveBody("", true, true)
	raw, _ = json.Marshal(body)
	if string(raw) != `{"all":true,"copy":false}` {
		t.Errorf("--all --no-copy: %s", raw)
	}
	var r valkeyResponse
	list := `{"data":{"shared_port":6379,"apps":[{"app":"shop","uses":"own","port":6401,"instance":"running","memory":"2.31M","keys":412},{"app":"blog","uses":"shared","port":null,"instance":null,"memory":null,"keys":null}],"shared":["blog"],"isolated":1}}`
	if err := json.Unmarshal([]byte(list), &r); err != nil || len(r.Data.Apps) != 2 || r.Data.Apps[0].Port == nil || *r.Data.Apps[0].Port != 6401 || r.Data.Apps[1].Port != nil || r.Data.Apps[1].Keys != nil {
		t.Errorf("list: %+v %v", r.Data, err)
	}
}

func TestNotificationSettingsBodies(t *testing.T) {
	body, err := notifTimezoneBody("Europe/Rome")
	if err != nil || body["timezone"] != "Europe/Rome" {
		t.Errorf("timezone: %v %v", body, err)
	}
	for _, word := range []string{"server", "system", "default"} {
		body, err = notifTimezoneBody(word)
		raw, _ := json.Marshal(body)
		if err != nil || string(raw) != `{"timezone":null}` {
			t.Errorf("%s: %s %v", word, raw, err)
		}
	}
	for _, bad := range []string{"", "Europe/Rome; id", "../etc/passwd", "a/b/c/d", "Europe Rome"} {
		if _, err := notifTimezoneBody(bad); err == nil {
			t.Errorf("time zone %q was accepted", bad)
		}
	}
	if _, err := notifTimezoneBody("America/Argentina/Buenos_Aires"); err != nil {
		t.Errorf("a three-part zone was refused: %v", err)
	}

	body, err = notifServerNameBody("EU West 1.a_b-c", false)
	if err != nil || body["server_name"] != "EU West 1.a_b-c" {
		t.Errorf("name: %v %v", body, err)
	}
	body, err = notifServerNameBody("", true)
	raw, _ := json.Marshal(body)
	if err != nil || string(raw) != `{"server_name":null}` {
		t.Errorf("--clear: %s %v", raw, err)
	}
	for _, bad := range []string{"", "--clear", "prod\nSubject: x", "prod $(id)", strings.Repeat("a", 41), " prod", "prod "} {
		if _, err := notifServerNameBody(bad, false); err == nil {
			t.Errorf("name %q was accepted", bad)
		}
	}
	if _, err := notifServerNameBody("prod", true); err == nil {
		t.Error("a name and --clear together were accepted")
	}
	var s notifSettingsResponse
	if err := json.Unmarshal([]byte(`{"data":{"timezone":null,"server_timezone":"Etc/UTC","server_name":"production","hostname":"vmi12334","now":"2026-11-26 10:00:00 UTC"}}`), &s); err != nil || s.Data.Timezone != nil || s.Data.ServerName == nil || *s.Data.ServerName != "production" {
		t.Errorf("settings: %+v %v", s.Data, err)
	}
}

func TestTheNewCommandsAreRegistered(t *testing.T) {
	want := map[string][]string{
		"cache":         {"list", "show", "on", "off", "ttl", "exclude", "clear"},
		"octane":        {"show", "php", "restart", "builds", "upgrade"},
		"valkey":        {"list", "isolate", "share", "restart", "flush-shared"},
		"notifications": {"settings", "timezone", "server-name"},
	}
	for parent, subs := range want {
		p, _, err := rootCmd.Find([]string{parent})
		if err != nil || p == nil || p.Name() != parent {
			t.Errorf("%s is not a command", parent)
			continue
		}
		for _, sub := range subs {
			found := false
			for _, c := range p.Commands() {
				if c.Name() == sub {
					found = true
				}
			}
			if !found {
				t.Errorf("%s %s is not a command", parent, sub)
			}
		}
	}
}
