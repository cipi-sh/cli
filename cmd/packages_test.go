package cmd

import (
	"encoding/json"
	"strings"
	"testing"
)

// Cipi 5.14.0 / cipi/api 1.44: optional packages installed and removed
// through the API; the list says what each entry brings and what is missing.

func TestDecodeHostPackagesReadsBothShapes(t *testing.T) {
	wrapped := `{"packages":[
		{"id":"browsershot","packages":["libnss3","libgbm1"],"description":"Libraries for Chrome","kind":"libraries","binaries":[],"missing":["libgbm1"],"installed":false,"partial":true},
		{"id":"ffmpeg","packages":["ffmpeg"],"description":"Transcoding","kind":"tools","binaries":["ffmpeg","ffprobe"],"missing":[],"installed":true,"partial":false}
	]}`
	pkgs, err := decodeHostPackages(json.RawMessage(wrapped))
	if err != nil || len(pkgs) != 2 {
		t.Fatalf("wrapped: %v %v", pkgs, err)
	}
	if pkgs[0].status() != "partial" || pkgs[0].provides() != "libraries" || strings.Join(pkgs[0].Missing, " ") != "libgbm1" {
		t.Fatalf("browsershot: %+v", pkgs[0])
	}
	if pkgs[1].status() != "installed" || pkgs[1].provides() != "ffmpeg, ffprobe" {
		t.Fatalf("ffmpeg: %+v", pkgs[1])
	}

	// An older server: a bare list, no kind, binaries or missing.
	old, err := decodeHostPackages(json.RawMessage(`[{"id":"poppler-utils","packages":["poppler-utils"],"installed":false}]`))
	if err != nil || len(old) != 1 || old[0].status() != "not installed" || old[0].provides() != "—" {
		t.Fatalf("bare list: %+v %v", old, err)
	}

	if _, err := decodeHostPackages(json.RawMessage(`"nope"`)); err == nil {
		t.Fatal("a string is not a catalog")
	}
}

func TestValidatePackageID(t *testing.T) {
	for _, ok := range []string{"browsershot", "image-optimizers", "fonts-cjk", "poppler-utils", "avif"} {
		if err := validatePackageID(ok); err != nil {
			t.Errorf("%s: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "-ffmpeg", "FFmpeg", "ffmpeg;reboot", "../etc", "lib nss3", "ffmpeg\n", strings.Repeat("a", 41)} {
		if err := validatePackageID(bad); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
}

func TestStringListKeepsOnlyStrings(t *testing.T) {
	got := stringList([]interface{}{"libnss3", 3, "", "libgbm1", nil})
	if strings.Join(got, " ") != "libnss3 libgbm1" {
		t.Fatalf("got %v", got)
	}
	if stringList("libnss3") != nil || stringList(nil) != nil {
		t.Fatal("not a list")
	}
}
