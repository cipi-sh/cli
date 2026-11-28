package cmd

import (
	"strings"
	"testing"
)

func TestSftpBodyChecksBeforeAnythingIsSent(t *testing.T) {
	b, err := sftpBody("box", "u1.your-storagebox.de", 23, "u1", "cipi/", true, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if b["name"] != "box" || b["type"] != "sftp" || b["port"] != 23 || b["path"] != "cipi" || b["host"] != "u1.your-storagebox.de" {
		t.Errorf("create body: %v", b)
	}
	e, err := sftpBody("box", "", 22, "", "backups", false, true, false)
	if err != nil || len(e) != 1 || e["path"] != "backups" {
		t.Errorf("edit body, only what is given: %v %v", e, err)
	}
	for _, c := range [][]string{{"s3", "h", "u", ""}, {"Box", "h", "u", ""}, {"box", "a;b", "u", ""}, {"box", "h", "a b", ""}, {"box", "h", "u", "../x"}, {"box", "", "u", ""}} {
		if _, err := sftpBody(c[0], c[1], 22, c[2], c[3], false, true, true); err == nil {
			t.Errorf("taken: %v", c)
		}
	}
	if _, err := sftpBody("box", "h", 70000, "u", "", true, false, true); err == nil {
		t.Error("port 70000 taken")
	}
	for k := range b {
		if strings.Contains(k, "pass") || strings.Contains(k, "secret") {
			t.Errorf("%s has no place in the request", k)
		}
	}
}

func TestSftpInstallHint(t *testing.T) {
	sb := strings.Join(sftpInstallHint("box", "u1.your-storagebox.de", 23, "u1", "ssh-ed25519 AAAA k"), "\n")
	if !strings.Contains(sb, "ssh-copy-id -p 23 -s -f -i cipi-backup-box.pub u1@u1.your-storagebox.de") {
		t.Errorf("storage box: %s", sb)
	}
	vps := strings.Join(sftpInstallHint("vps", "203.0.113.9", 22, "backup", "ssh-ed25519 AAAA k'x"), "\n")
	if !strings.Contains(vps, "echo 'ssh-ed25519 AAAA kx' >> ~/.ssh/authorized_keys") {
		t.Errorf("vps: %s", vps)
	}
}

func TestUpdatesDelayHours(t *testing.T) {
	for in, want := range map[string]int{"12h": 12, "12": 12, "2d": 48, "7d": 168, "off": 0, "0": 0, "on": 12, " 6H ": 6} {
		if got, err := updatesDelayHours(in); err != nil || got != want {
			t.Errorf("%q: %d %v, want %d", in, got, err, want)
		}
	}
	for _, bad := range []string{"8d", "169", "-1", "x", "1w"} {
		if _, err := updatesDelayHours(bad); err == nil {
			t.Errorf("%q taken", bad)
		}
	}
	if !fingerprintRe.MatchString("SHA256:"+strings.Repeat("A", 43)) || fingerprintRe.MatchString("SHA256:bad;id") {
		t.Error("fingerprint pattern")
	}
}
