package cmd

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"

	"github.com/cipi-sh/cli/internal/output"
	"github.com/spf13/cobra"
)

// The nginx page cache of an app came with Cipi 5.17.0 and cipi/api 1.47
// (abilities nginx-view, nginx-manage — the ones of the nginx directives).
const cacheMinAPI = "1.47.0"

// pageCache is the data of GET /api/apps/{app}/cache.
type pageCache struct {
	App           string `json:"app"`
	Supported     bool   `json:"supported"`
	Kind          string `json:"kind"`
	Enabled       bool   `json:"enabled"`
	Active        bool   `json:"active"`
	TTL           string `json:"ttl"`
	TTLSeconds    int    `json:"ttl_seconds"`
	Size          string `json:"size"`
	ClearOnDeploy bool   `json:"clear_on_deploy"`
	Files         int64  `json:"files"`
	Bytes         int64  `json:"bytes"`
	Path          string `json:"path"`
	Suspended     bool   `json:"suspended"`
	ClearCommand  string `json:"clear_command"`
	Cleared       *int   `json:"cleared,omitempty"`
}

type pageCacheResponse struct {
	Data pageCache `json:"data"`
}

var (
	cacheTTLPattern  = regexp.MustCompile(`^([1-9][0-9]{0,6})(s|m|h|d)$`)
	cacheSizePattern = regexp.MustCompile(`^([1-9][0-9]{0,5})(m|g)$`)
)

func cachePath(app, suffix string) string {
	return "/api/apps/" + url.PathEscape(app) + "/cache" + suffix
}

// validCacheTTL is the server's own rule: a number and s, m, h or d, one
// second to thirty days.
func validCacheTTL(ttl string) bool {
	m := cacheTTLPattern.FindStringSubmatch(ttl)
	if m == nil {
		return false
	}
	n, _ := strconv.Atoi(m[1])
	n *= map[string]int{"s": 1, "m": 60, "h": 3600, "d": 86400}[m[2]]
	return n >= 1 && n <= 2592000
}

// validCacheSize: megabytes or gigabytes, 16m to 100g.
func validCacheSize(size string) bool {
	m := cacheSizePattern.FindStringSubmatch(size)
	if m == nil {
		return false
	}
	mb, _ := strconv.Atoi(m[1])
	if m[2] == "g" {
		mb *= 1024
	}
	return mb >= 16 && mb <= 102400
}

// cacheOnBody is what `cache on` sends: only what was given, so the rest
// stays as the server has it.
func cacheOnBody(ttl, size string, clearOnDeploy *bool) (map[string]interface{}, error) {
	body := map[string]interface{}{"enabled": true}
	if ttl != "" {
		if !validCacheTTL(ttl) {
			return nil, fmt.Errorf("invalid --ttl %q: a number and s, m, h or d (30s, 10m, 1h, 7d), thirty days at most", ttl)
		}
		body["ttl"] = ttl
	}
	if size != "" {
		if !validCacheSize(size) {
			return nil, fmt.Errorf("invalid --size %q: megabytes or gigabytes, 16m to 100g (256m, 2g)", size)
		}
		body["size"] = size
	}
	if clearOnDeploy != nil {
		body["clear_on_deploy"] = *clearOnDeploy
	}
	return body, nil
}

func (c pageCache) state() string {
	switch {
	case !c.Supported:
		return "n/a (Node)"
	case c.Enabled && !c.Active:
		return "on (not in the vhost: suspended?)"
	case c.Enabled:
		return "on"
	default:
		return "off"
	}
}

func printPageCache(c pageCache) {
	output.Header("Page cache — " + c.App)
	if !c.Supported {
		output.Warn("Not available for a Node app: its pages come from its own server")
		fmt.Println()
		return
	}
	state := c.state()
	if c.Enabled && c.Active {
		state = output.Green.Sprint(state)
	}
	output.KeyValue(nil, "Cache", state)
	if c.Kind != "" {
		kind := "fastcgi_cache (PHP-FPM)"
		if c.Kind == "proxy" {
			kind = "proxy_cache (Octane)"
		}
		output.KeyValue(nil, "Kind", kind)
	}
	output.KeyValue(nil, "Pages kept", c.TTL+"  (when the app's answer does not say)")
	output.KeyValue(nil, "Size on disk", fmt.Sprintf("%s of %s — %d page(s)", humanBytes(c.Bytes), c.Size, c.Files))
	if c.ClearOnDeploy {
		output.KeyValue(nil, "After a deploy", "emptied")
	} else {
		output.KeyValue(nil, "After a deploy", "kept (--deploy-clear to empty it)")
	}
	if c.ClearCommand != "" {
		output.KeyValue(nil, "On the server", c.ClearCommand)
	}
	fmt.Println()
}

func cacheFail(action string, err error, ability string) error {
	output.Error("%s: %s", action, featureErrorHint(err, cacheMinAPI, ability))
	return err
}

var cacheCmd = &cobra.Command{
	Use:   "cache",
	Short: "The nginx page cache of an app: on, off, ttl, clear",
	Long: `The page cache is nginx's: it keeps the page PHP made and answers the next
visitors with it, without running PHP at all (fastcgi_cache for an app on
PHP-FPM — Laravel, WordPress, any PHP app; proxy_cache for one on Octane).

It is not OPcache. OPcache is always on and keeps the compiled PHP files in
memory, so PHP starts faster — but it still runs at every request: the
framework boots, the database is queried, the page is rendered. With the page
cache none of that happens for a page that is kept. That is what makes
WordPress and content sites answer in a few milliseconds and hold a traffic
peak.

The cache respects what the app answers (Cache-Control, Expires, Set-Cookie:
a response that sets a cookie or says private / no-store is never kept) and
skips signed-in visitors, carts, admin and API paths and everything that is
not GET or HEAD. Every response says what happened in X-Cache-Status (HIT,
MISS, BYPASS, EXPIRED).

Cipi empties the cache after every deploy and rollback. Where content changes
without a deploy, "cipi-cli cache clear <app>" — or, on the server, as the
app's user: sudo /usr/local/bin/cipi-cache-clear <app>

Requires Cipi 5.17.0+, cipi/api 1.47+ and the nginx-view / nginx-manage
abilities.

  cipi-cli cache
  cipi-cli cache on blog --ttl 10m
  cipi-cli cache ttl blog 1h
  cipi-cli cache clear blog
  cipi-cli cache off blog

` + multiServerTip,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cacheListCmd.RunE(cmd, args)
	},
}

var cacheListCmd = &cobra.Command{
	Use:   "list",
	Short: "Every app with its page cache",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}
		var result struct {
			Data struct {
				Apps []pageCache `json:"apps"`
			} `json:"data"`
		}
		if err := client.Get("/api/cache", &result); err != nil {
			return cacheFail("Failed to read the page cache", err, "nginx-view")
		}
		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}
		output.Header("Page cache")
		if len(result.Data.Apps) == 0 {
			output.Warn("No apps")
			fmt.Println()
			return nil
		}
		t := output.NewTable("APP", "CACHE", "KEPT", "PAGES", "ON DISK")
		for _, c := range result.Data.Apps {
			if !c.Supported || !c.Enabled {
				t.Row(c.App, c.state(), "—", "—", "—")
				continue
			}
			t.Row(c.App, output.Green.Sprint(c.state()), c.TTL, strconv.FormatInt(c.Files, 10), humanBytes(c.Bytes))
		}
		t.Flush()
		fmt.Println()
		return nil
	},
}

var cacheShowCmd = &cobra.Command{
	Use:     "show <app>",
	Aliases: []string{"status"},
	Short:   "The page cache of one app",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}
		var result pageCacheResponse
		if err := client.Get(cachePath(args[0], ""), &result); err != nil {
			return cacheFail("Failed to read the page cache", err, "nginx-view")
		}
		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}
		printPageCache(result.Data)
		return nil
	},
}

var cacheOnCmd = &cobra.Command{
	Use:     "on <app>",
	Aliases: []string{"enable"},
	Short:   "Turn the page cache on, or change it",
	Long: `Turn the page cache on. What is not given stays as the server has it
(defaults: pages kept 10m, 256m on disk, emptied after every deploy). On an
app whose cache is on already it changes the settings.

nginx is tested before it reloads: a refusal changes nothing.

  cipi-cli cache on blog
  cipi-cli cache on blog --ttl 1h --size 1g
  cipi-cli cache on blog --deploy-clear=false`,
	Example: `  cipi-cli cache on blog --ttl 10m`,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ttl, _ := cmd.Flags().GetString("ttl")
		size, _ := cmd.Flags().GetString("size")
		var clearOnDeploy *bool
		if cmd.Flags().Changed("deploy-clear") {
			v, _ := cmd.Flags().GetBool("deploy-clear")
			clearOnDeploy = &v
		}
		body, err := cacheOnBody(ttl, size, clearOnDeploy)
		if err != nil {
			output.Error("%s", err)
			return err
		}
		client, err := mustClient()
		if err != nil {
			return err
		}
		var result pageCacheResponse
		if err := client.Put(cachePath(args[0], ""), body, &result); err != nil {
			return cacheFail("Failed to turn the page cache on", err, "nginx-manage")
		}
		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}
		output.Success("Page cache on for %s — pages kept %s, up to %s on disk", args[0], result.Data.TTL, result.Data.Size)
		fmt.Println()
		return nil
	},
}

var cacheOffCmd = &cobra.Command{
	Use:     "off <app>",
	Aliases: []string{"disable"},
	Short:   "Turn the page cache off and delete the cached pages",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}
		var result pageCacheResponse
		if err := client.Put(cachePath(args[0], ""), map[string]interface{}{"enabled": false}, &result); err != nil {
			return cacheFail("Failed to turn the page cache off", err, "nginx-manage")
		}
		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}
		output.Success("Page cache off for %s — every request runs PHP again", args[0])
		fmt.Println()
		return nil
	},
}

var cacheTTLCmd = &cobra.Command{
	Use:   "ttl <app> <time>",
	Short: "How long a page is kept: 30s, 10m, 1h, 7d",
	Long: `How long nginx keeps a page when the app's answer does not say (a
Cache-Control or Expires header of the app wins). The cache has to be on.

  cipi-cli cache ttl blog 1h`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if !validCacheTTL(args[1]) {
			err := fmt.Errorf("invalid time %q: a number and s, m, h or d (30s, 10m, 1h, 7d), thirty days at most", args[1])
			output.Error("%s", err)
			return err
		}
		client, err := mustClient()
		if err != nil {
			return err
		}
		var current pageCacheResponse
		if err := client.Get(cachePath(args[0], ""), &current); err != nil {
			return cacheFail("Failed to read the page cache", err, "nginx-view")
		}
		if !current.Data.Enabled {
			err := errors.New("the page cache of " + args[0] + " is off — turn it on with its time: cipi-cli cache on " + args[0] + " --ttl " + args[1])
			output.Error("%s", err)
			return err
		}
		var result pageCacheResponse
		if err := client.Put(cachePath(args[0], ""), map[string]interface{}{"enabled": true, "ttl": args[1]}, &result); err != nil {
			return cacheFail("Failed to set the time", err, "nginx-manage")
		}
		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}
		output.Success("Pages of %s are kept %s", args[0], result.Data.TTL)
		fmt.Println()
		return nil
	},
}

var cacheClearCmd = &cobra.Command{
	Use:     "clear <app>",
	Aliases: []string{"purge", "flush"},
	Short:   "Delete every cached page of the app",
	Long: `Delete every cached page of the app: the next visit to each page runs PHP
and keeps it again. Cipi does this by itself after every deploy and rollback.

In a deploy script or a cron on the server, as the app's user:

  sudo /usr/local/bin/cipi-cache-clear <app>`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}
		var result pageCacheResponse
		if err := client.Post(cachePath(args[0], "/clear"), nil, &result); err != nil {
			return cacheFail("Failed to empty the page cache", err, "nginx-manage")
		}
		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}
		if !result.Data.Enabled {
			output.Info("The page cache of %s is off — nothing to empty", args[0])
			fmt.Println()
			return nil
		}
		cleared := 0
		if result.Data.Cleared != nil {
			cleared = *result.Data.Cleared
		}
		output.Success("Page cache of %s emptied — %d page(s)", args[0], cleared)
		fmt.Println()
		return nil
	},
}

func init() {
	cacheOnCmd.Flags().String("ttl", "", "How long a page is kept: 30s, 10m, 1h, 7d (default 10m)")
	cacheOnCmd.Flags().String("size", "", "The most the cache takes on disk: 16m to 100g (default 256m)")
	cacheOnCmd.Flags().Bool("deploy-clear", true, "Empty the cache after every deploy and rollback")

	cacheCmd.AddCommand(cacheListCmd, cacheShowCmd, cacheOnCmd, cacheOffCmd, cacheTTLCmd, cacheClearCmd)
	rootCmd.AddCommand(cacheCmd)
}
