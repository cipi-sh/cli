package cmd

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/cipi-sh/cli/internal/api"
)

// Cipi 5.14.0 / cipi/api 1.44: an app's own nginx directives through the API.

func TestNormalizeNginxText(t *testing.T) {
	cases := map[string]string{
		"expires 7d;":                            "expires 7d;\n",
		"expires 7d;\r\nlocation /x { }\r\n\r\n": "expires 7d;\nlocation /x { }\n",
		"":                                       "",
		"  \n\t\n":                               "",
		"# only a comment\n\n   # another\n":     "",
		"# kept with what follows\nexpires 1d;":  "# kept with what follows\nexpires 1d;\n",
	}
	for in, want := range cases {
		if got := normalizeNginxText(in); got != want {
			t.Errorf("normalizeNginxText(%q) = %q, want %q", in, got, want)
		}
	}
	// The template of an empty app is nothing to send.
	if got := normalizeNginxText(nginxTemplate); got != "" {
		t.Errorf("the template alone must mean no directives, got %q", got)
	}
}

func TestValidateNginxText(t *testing.T) {
	if err := validateNginxText("location /x {\n\texpires 1d;\n}\n"); err != nil {
		t.Errorf("tabs and newlines are fine: %v", err)
	}
	if err := validateNginxText("expires 1d;\x01"); err == nil {
		t.Error("a control character was accepted")
	}
	if err := validateNginxText(strings.Repeat("#", nginxMaxBytes+1)); err == nil {
		t.Error("a text over the limit was accepted")
	}
	if err := validateNginxText(strings.Repeat("#", nginxMaxBytes)); err != nil {
		t.Errorf("a text at the limit was refused: %v", err)
	}
}

func TestNginxPathEscapesTheApp(t *testing.T) {
	if got := nginxPath("shop"); got != "/api/apps/shop/nginx" {
		t.Errorf("got %s", got)
	}
	if got := nginxPath("../x"); strings.Contains(got, "/../") {
		t.Errorf("the app is one path segment: %s", got)
	}
}

func TestNginxRefusalReadsTheReasons(t *testing.T) {
	var apiErr api.APIError
	body := `{"error":"nginx refused the directives — nothing changed","reasons":["nginx refused the directives — nothing changed","line 2: unknown directive \"boom\""]}`
	if err := json.Unmarshal([]byte(body), &apiErr); err != nil {
		t.Fatal(err)
	}
	apiErr.Status = 422
	reasons := nginxRefusal(&apiErr)
	if len(reasons) != 2 || reasons[1] != `line 2: unknown directive "boom"` {
		t.Fatalf("reasons: %v", reasons)
	}

	// A 422 without reasons (Laravel validation): its message.
	plain := &api.APIError{ErrorText: "The directives are too long (32 KB at most).", Status: 422}
	if got := nginxRefusal(plain); len(got) != 1 || !strings.Contains(got[0], "too long") {
		t.Fatalf("plain 422: %v", got)
	}

	// Anything else is not about the text.
	if nginxRefusal(&api.APIError{ErrorText: "nginx configuration is already invalid", Status: 503}) != nil {
		t.Error("a 503 is not a refused text")
	}
	if nginxRefusal(errors.New("connection refused")) != nil {
		t.Error("a transport error is not a refused text")
	}
}

func TestNginxCommandIsRegistered(t *testing.T) {
	found := map[string]bool{}
	for _, c := range nginxCmd.Commands() {
		found[c.Name()] = true
	}
	for _, name := range []string{"show", "edit", "set", "reset"} {
		if !found[name] {
			t.Errorf("nginx %s is missing", name)
		}
	}
	if nginxSetCmd.Flags().Lookup("file") == nil || nginxResetCmd.Flags().Lookup("yes") == nil {
		t.Error("flags: set --file, reset --yes")
	}
}
