package cmd

import (
	"encoding/json"
	"strings"
	"testing"
)

// Cipi 5.13.0 / cipi/api 1.43: .env order, paused commit, basic auth read
// right, MySQL on port 3306, paths left out of the backups.

func TestEnvVarsInOrderKeepsTheFileOrder(t *testing.T) {
	keys, vals, err := envVarsInOrder(json.RawMessage(`{"app":"shop","vars":{"ZED":"1","APP_NAME":"Shop","DB_PORT":3306,"EMPTY":null}}`))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(keys, ",") != "ZED,APP_NAME,DB_PORT,EMPTY" {
		t.Fatalf("keys = %v", keys)
	}
	if vals["APP_NAME"] != "Shop" || vals["DB_PORT"] != "3306" || vals["EMPTY"] != "" {
		t.Fatalf("vals = %v", vals)
	}

	// An older answer under "variables", and an empty file.
	if keys, _, _ := envVarsInOrder(json.RawMessage(`{"variables":{"B":"2","A":"1"}}`)); strings.Join(keys, ",") != "B,A" {
		t.Fatalf("variables: %v", keys)
	}
	for _, empty := range []string{`{"vars":{}}`, `{"vars":[]}`, `{"vars":null}`, `{}`} {
		if keys, _, err := envVarsInOrder(json.RawMessage(empty)); err != nil || len(keys) != 0 {
			t.Fatalf("%s: %v %v", empty, keys, err)
		}
	}
}

func TestEnvMoveKeys(t *testing.T) {
	file := []string{"APP_NAME", "APP_ENV", "APP_URL", "DB_HOST", "DB_PORT", "MAIL_HOST"}
	cases := []struct {
		move          []string
		after, before string
		want          string
	}{
		{[]string{"DB_HOST", "DB_PORT"}, "", "", "DB_HOST,DB_PORT,APP_NAME,APP_ENV,APP_URL,MAIL_HOST"},
		{[]string{"MAIL_HOST"}, "APP_URL", "", "APP_NAME,APP_ENV,APP_URL,MAIL_HOST,DB_HOST,DB_PORT"},
		{[]string{"APP_NAME"}, "", "MAIL_HOST", "APP_ENV,APP_URL,DB_HOST,DB_PORT,APP_NAME,MAIL_HOST"},
		{[]string{"DB_PORT", "DB_HOST"}, "MAIL_HOST", "", "APP_NAME,APP_ENV,APP_URL,MAIL_HOST,DB_PORT,DB_HOST"},
	}
	for _, c := range cases {
		got, err := envMoveKeys(file, c.move, c.after, c.before)
		if err != nil || strings.Join(got, ",") != c.want {
			t.Errorf("%v after=%q before=%q: got %v (%v), want %s", c.move, c.after, c.before, got, err, c.want)
		}
	}
	for _, bad := range []struct {
		move          []string
		after, before string
	}{
		{[]string{"NOPE"}, "", ""},
		{[]string{"DB_HOST", "DB_HOST"}, "", ""},
		{[]string{"DB_HOST"}, "NOPE", ""},
		{[]string{"DB_HOST"}, "DB_HOST", ""},
	} {
		if _, err := envMoveKeys(file, bad.move, bad.after, bad.before); err == nil {
			t.Errorf("%v after=%q: accepted", bad.move, bad.after)
		}
	}
	if firstDuplicate([]string{"A", "B", "A"}) != "A" || firstDuplicate([]string{"A", "B"}) != "" {
		t.Error("firstDuplicate")
	}
}

func TestBasicAuthStateFromTheOutputOfAnOlderAPI(t *testing.T) {
	enabled, users, ok := basicAuthStateFromText("API error (500): \x1b[1mBasic auth — shop\x1b[0m\n━━━━━━━━━━\n  Status     \x1b[2mdisabled\x1b[0m\n")
	if !ok || enabled || len(users) != 0 {
		t.Fatalf("disabled: %v %v %v", enabled, users, ok)
	}
	enabled, users, ok = basicAuthStateFromText("[1mBasic auth — shop[0m ━━━ Status [0;32menabled[0m Users [0;36madmin,ops[0m")
	if !ok || !enabled || strings.Join(users, ",") != "admin,ops" {
		t.Fatalf("enabled: %v %v %v", enabled, users, ok)
	}
	if _, _, ok := basicAuthStateFromText("App 'shop' not found"); ok {
		t.Fatal("another error read as a state")
	}
	if got := plainCLIText("\x1b[0;31mApp 'shop' not found\x1b[0m [1mx[0m ━━"); got != "App 'shop' not found x" {
		t.Fatalf("plainCLIText = %q", got)
	}
}

func TestDBEnginesDecodeTheServerOf3306(t *testing.T) {
	var data dbEnginesData
	if err := json.Unmarshal([]byte(`{"default":"mariadb","engines":[{"engine":"mariadb","status":"running","port":3306,"default":true,"server":"mysql","label":"MySQL"},{"engine":"pgsql","status":"not installed","port":null,"default":false}]}`), &data); err != nil {
		t.Fatal(err)
	}
	if data.Engines[0].Server != "mysql" || data.Engines[0].Label != "MySQL" || data.Engines[1].Port != nil {
		t.Fatalf("%+v", data)
	}
}

func TestBackupProfileExcludePaths(t *testing.T) {
	c := backupProfileEditCmd
	if err := c.Flags().Set("exclude-paths", "storage/app/uploads,*.mp4,shop:public/media files"); err != nil {
		t.Fatal(err)
	}
	body := backupProfileBody(c)
	got, _ := body["exclude_paths"].([]string)
	if strings.Join(got, "|") != "storage/app/uploads|*.mp4|shop:public/media files" {
		t.Fatalf("exclude_paths = %#v", body["exclude_paths"])
	}

	var p backupProfile
	if err := json.Unmarshal([]byte(`{"name":"nightly","exclude_paths":["storage/app/uploads"]}`), &p); err != nil || len(p.ExcludePaths) != 1 {
		t.Fatalf("decode: %+v %v", p, err)
	}
}

func TestDeployStatusPausedCommit(t *testing.T) {
	var d deployStatusData
	if err := json.Unmarshal([]byte(`{"app":"shop","state":"idle","paused_commit":"a1b2c3d4e5f60718"}`), &d); err != nil {
		t.Fatal(err)
	}
	if d.PausedCommit == nil || *d.PausedCommit != "a1b2c3d4e5f60718" {
		t.Fatalf("%+v", d)
	}
	if err := json.Unmarshal([]byte(`{"app":"shop","state":"idle"}`), &d); err != nil {
		t.Fatal(err)
	}
}
