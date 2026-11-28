package cmd

import (
	"strings"
	"testing"
)

func TestCachePurgePagesAreWhatTheServerTakes(t *testing.T) {
	got, err := cachePurgePages([]string{"/blog/hello/?utm_source=x#top", "category/news/*", "/a/,/b/", "https://www.example.com/x/", "/blog/hello/"})
	if err != nil {
		t.Fatal(err)
	}
	want := "/blog/hello/ /category/news/* /a/ /b/ https://www.example.com/x/"
	if strings.Join(got, " ") != want {
		t.Errorf("got %v, want %s", got, want)
	}
	for _, bad := range []string{"/a*b/", "/x'", "/$(id)", "https://bad host/x"} {
		if _, err := cachePurgePages([]string{bad}); err == nil {
			t.Errorf("%q taken as a page", bad)
		}
	}
	if _, err := cachePurgePages([]string{""}); err == nil {
		t.Error("no page taken")
	}
	many := make([]string, 501)
	for i := range many {
		many[i] = "/p" + strings.Repeat("x", i%7) + string(rune('a'+i%26)) + "/" + strings.Repeat("y", i/26)
	}
	if _, err := cachePurgePages(many); err == nil {
		t.Error("501 pages taken")
	}
}
