package cmd

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/cipi-sh/cli/internal/output"
	"github.com/spf13/cobra"
)

// Some pages of the cache, not all: POST /api/apps/{name}/cache/purge
// (cipi/api 1.48, Cipi 5.18.0).

const cachePurgeAPI = "1.48"

const cachePurgeMax = 500

var cachePurgePageRe = regexp.MustCompile(`^(https?://[A-Za-z0-9.-]{1,253}(:[0-9]{1,5})?)?/[A-Za-z0-9._~%:@!,;=+/-]*\*?$`)

// cachePurgePages makes the pages of the command line what the server takes:
// a query string or a fragment goes, a leading / is added, each page once.
func cachePurgePages(args []string) ([]string, error) {
	out := []string{}
	seen := map[string]bool{}
	for _, raw := range args {
		for _, page := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ' ' || r == '\n' || r == '\t' }) {
			prefix := strings.HasSuffix(page, "*")
			page = strings.TrimRight(page, "*")
			if i := strings.IndexAny(page, "?#"); i >= 0 {
				page = page[:i]
			}
			if !strings.HasPrefix(page, "/") && !strings.HasPrefix(page, "http://") && !strings.HasPrefix(page, "https://") {
				page = "/" + page
			}
			if prefix {
				page += "*"
			}
			if !cachePurgePageRe.MatchString(page) || len(page) > 2300 {
				return nil, fmt.Errorf("%q is not a page: a path (/blog/post/), every page under one (/blog/*), or a URL of the app — a * at the end only", raw)
			}
			if !seen[page] {
				seen[page] = true
				out = append(out, page)
			}
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("name at least one page: /blog/hello-world/, or '/blog/*' for every page under it (clear empties them all)")
	}
	if len(out) > cachePurgeMax {
		return nil, fmt.Errorf("at most %d pages at once (got %d) — one with a * covers many", cachePurgeMax, len(out))
	}
	return out, nil
}

type cachePurgeResponse struct {
	Data struct {
		App     string   `json:"app"`
		Enabled bool     `json:"enabled"`
		Purged  int      `json:"purged"`
		Targets []string `json:"targets"`
	} `json:"data"`
}

var cachePurgeCmd = &cobra.Command{
	Use:   "purge <app> <page>...",
	Short: "Delete some cached pages of the app, not all",
	Long: `Delete those pages from the cache and leave the rest warm.

  cipi-cli cache purge blog /blog/hello-world/
  cipi-cli cache purge blog '/category/news/*' /
  cipi-cli cache purge blog https://www.example.com/landing/

A path is that page on every domain and alias of the app, over http and https,
with its ?utm_… variants. A trailing * is every page under it — quote it, or
the shell expands it. A URL is that page on that domain only, and the domain
has to be one of the app's. Up to 500 pages; "cache clear" empties them all.

As the app's user on the server, without the API:

  sudo /usr/local/bin/cipi-cache-clear <app> /blog/hello-world/ '/blog/*'

WordPress: the Cipi Cache plugin (github.com/cipi-sh/cache-wp) purges the pages
a change touches. Any PHP app: composer require cipi/cache
(github.com/cipi-sh/cache-php).

Requires Cipi 5.18.0+, cipi/api 1.48+ and the nginx-manage ability.`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		pages, err := cachePurgePages(args[1:])
		if err != nil {
			output.Error("%s", err)
			return err
		}
		client, err := mustClient()
		if err != nil {
			return err
		}
		var result cachePurgeResponse
		if err := client.Post(cachePath(args[0], "/purge"), map[string]interface{}{"paths": pages}, &result); err != nil {
			output.Error("Failed to purge the pages: %s", featureErrorHint(err, cachePurgeAPI, "nginx-manage"))
			return err
		}
		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}
		if !result.Data.Enabled {
			output.Info("The page cache of %s is off — nothing to purge", args[0])
			fmt.Println()
			return nil
		}
		output.Success("Purged %d cached file(s) of %s for %d page(s) — the rest of the cache stays", result.Data.Purged, args[0], len(pages))
		fmt.Println()
		return nil
	},
}

func init() {
	cacheCmd.AddCommand(cachePurgeCmd)
}
