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

func destTestCmd(t *testing.T, flags map[string]string) *cobra.Command {
	t.Helper()
	c := &cobra.Command{Use: "add"}
	addS3Flags(c)
	c.Flags().String("use", "", "")
	for k, v := range flags {
		if err := c.Flags().Set(k, v); err != nil {
			t.Fatal(err)
		}
	}
	return c
}

func TestUseSendsProfilesThere(t *testing.T) {
	if p, err := useProfiles(" default, hourly-db ,"); err != nil || strings.Join(p, ",") != "default,hourly-db" {
		t.Errorf("--use: %v %v", p, err)
	}
	if _, err := useProfiles("default;rm"); err == nil {
		t.Error("--use took a bad name")
	}
	body := map[string]interface{}{}
	if err := addUse(destTestCmd(t, map[string]string{"use": "default"}), body); err != nil || len(body["profiles"].([]string)) != 1 {
		t.Errorf("addUse: %v %v", body, err)
	}
	empty := map[string]interface{}{}
	if err := addUse(destTestCmd(t, nil), empty); err != nil || len(empty) != 0 {
		t.Errorf("no --use, no profiles: %v", empty)
	}
	f, _ := os.CreateTemp(t.TempDir(), "secret")
	f.WriteString("sEcReT\nnext line\n")
	f.Close()
	c := destTestCmd(t, map[string]string{"secret-file": f.Name()})
	if sec, err := s3Secret(c, true); err != nil || sec != "sEcReT" {
		t.Errorf("the secret's first line from a file: %q %v", sec, err)
	}
}

func TestNoSecretFlag(t *testing.T) {
	for _, c := range []*cobra.Command{backupDestinationAddCmd, backupDestinationEditCmd} {
		if c.Flags().Lookup("secret") != nil {
			t.Errorf("%s has a --secret flag: it would stay in the shell's history", c.Name())
		}
		if c.Flags().Lookup("secret-file") == nil || c.Flags().Lookup("secret-stdin") == nil || c.Flags().Lookup("use") == nil {
			t.Errorf("%s: --secret-file, --secret-stdin and --use", c.Name())
		}
	}
	if backupConfigureCmd.Flags().Lookup("bucket") != nil || backupConfigureCmd.Flags().Lookup("type") != nil {
		t.Error("configure sets up the server only: no bucket, no type")
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
