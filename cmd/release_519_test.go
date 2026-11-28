package cmd

import (
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestS3BodyChecksBeforeAnythingIsSent(t *testing.T) {
	b, err := s3Body("acme-offsite", "AKIAX", "sEcR3t/+=", "auto", "https://abc.r2.cloudflarestorage.com", true, true)
	if err != nil || b["bucket"] != "acme-offsite" || b["key"] != "AKIAX" || b["secret"] != "sEcR3t/+=" || b["region"] != "auto" || b["endpoint"] != "https://abc.r2.cloudflarestorage.com" {
		t.Fatalf("create body: %v %v", b, err)
	}
	if _, err := s3Body("acme", "", "", "", "", false, true); err == nil {
		t.Error("a new bucket without its keys taken")
	}
	e, err := s3Body("", "", "", "eu-west-1", "", true, false)
	if err != nil || len(e) != 2 || e["region"] != "eu-west-1" || e["endpoint"] != "none" {
		t.Errorf("edit: only what is given, an emptied endpoint is none: %v %v", e, err)
	}
	for _, c := range [][]string{{"Bad Bucket", "k", "s", "", ""}, {"b1", "AK IA", "s", "", ""}, {"b1", "k", "x'y", "", ""}, {"b1", "k", `s\`, "", ""}, {"b1", "k", "s", "eu central", ""},
		{"b1", "k", "s", "", "ftp://x"}, {"b1", "k", "s", "", "https://<account-id>.r2.cloudflarestorage.com"}, {"b1", "AK\nIA", "s", "", ""}} {
		if _, err := s3Body(c[0], c[1], c[2], c[3], c[4], c[4] != "", true); err == nil {
			t.Errorf("taken: %q", c)
		}
	}
}

func setupTestCmd(t *testing.T, flags map[string]string) *cobra.Command {
	t.Helper()
	c := &cobra.Command{Use: "configure"}
	addConfigureFlags(c)
	for k, v := range flags {
		if err := c.Flags().Set(k, v); err != nil {
			t.Fatal(err)
		}
	}
	return c
}

func TestSetupBodySaysWhereBackupsGo(t *testing.T) {
	b, err := setupBody(setupTestCmd(t, map[string]string{"type": "local"}))
	if err != nil || len(b) != 1 || b["type"] != "local" {
		t.Errorf("local: %v %v", b, err)
	}
	if b, err = setupBody(setupTestCmd(t, map[string]string{"local-only": "true"})); err != nil || b["type"] != "local" {
		t.Errorf("--local-only: %v %v", b, err)
	}
	b, err = setupBody(setupTestCmd(t, map[string]string{"host": "u1.your-storagebox.de", "port": "23", "user": "u1", "path": "cipi/"}))
	if err != nil || b["type"] != "sftp" || b["name"] != "sftp" || b["port"] != 23 || b["path"] != "cipi" {
		t.Errorf("sftp, said by its fields: %v %v", b, err)
	}
	f, _ := os.CreateTemp(t.TempDir(), "secret")
	f.WriteString("sEcReT\nnext line\n")
	f.Close()
	b, err = setupBody(setupTestCmd(t, map[string]string{"bucket": "acme-backups", "key": "AKIAX", "secret-file": f.Name(), "endpoint": "https://fsn1.your-objectstorage.com"}))
	if err != nil || b["type"] != "s3" || b["secret"] != "sEcReT" || b["endpoint"] != "https://fsn1.your-objectstorage.com" {
		t.Errorf("s3, the secret's first line from a file: %v %v", b, err)
	}
	if b, err = setupBody(setupTestCmd(t, map[string]string{"type": "s3", "bucket": "acme-backups", "region": "eu-west-1"})); err != nil || b["secret"] != nil || b["key"] != nil || b["region"] != "eu-west-1" {
		t.Errorf("s3, the region alone: the keys kept: %v %v", b, err)
	}
	for _, bad := range []map[string]string{{}, {"type": "nas"}, {"type": "s3"}, {"type": "sftp", "host": "h"}, {"type": "sftp", "host": "h", "user": "u", "name": "s3"}, {"local-only": "true", "type": "s3"}} {
		if _, err := setupBody(setupTestCmd(t, bad)); err == nil {
			t.Errorf("taken: %v", bad)
		}
	}
}

func TestNoSecretFlag(t *testing.T) {
	for _, c := range []*cobra.Command{backupConfigureCmd, backupDestinationAddCmd, backupDestinationEditCmd} {
		if c.Flags().Lookup("secret") != nil {
			t.Errorf("%s has a --secret flag: it would stay in the shell's history", c.Name())
		}
		if c.Flags().Lookup("secret-file") == nil || c.Flags().Lookup("secret-stdin") == nil {
			t.Errorf("%s: --secret-file and --secret-stdin", c.Name())
		}
	}
	for _, name := range []string{"configure", "key", "prune"} {
		found := false
		for _, c := range backupCmd.Commands() {
			found = found || c.Name() == name
		}
		if !found {
			t.Errorf("backup %s missing", name)
		}
	}
	if !strings.Contains(updatesCmd.Long, "signed releases are not available yet") {
		t.Error("updates: signed releases not available yet")
	}
}
