package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/cipi-sh/cli/internal/output"
	"github.com/spf13/cobra"
)

// The mesh network of Zero Trust — Tailscale or NetBird — changed from here
// (Cipi 5.18.0, cipi/api 1.48, ability zt-manage): join a network, leave it,
// close and reopen the SSH port and 80/443 to the world, Tailscale SSH.
// Synchronous on the server: each call answers with Zero Trust after it.

const ztManageAPI = "1.48"

// ztChange is what every change of the mesh answers.
type ztChange struct {
	Data    map[string]interface{} `json:"data"`
	Message string                 `json:"message"`
}

// ztPrintChange shows what the server said and the mesh as it is now.
func ztPrintChange(result ztChange, done string) {
	if jsonFlag {
		output.PrintJSON(result)
		return
	}
	for _, line := range strings.Split(result.Message, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			output.Info("%s", line)
		}
	}
	output.Success("%s", done)
	if rows := ztMeshRows(result.Data); rows != nil {
		output.Header("Zero Trust — mesh network")
		for _, row := range rows {
			if row.Label == "warning" || row.Label == "refused" {
				output.Warn("%s", row.Value)
				continue
			}
			output.KeyValue(nil, row.Label, row.Value)
		}
	}
	fmt.Println()
}

// ztJoinKey is the key of a join: --key, the first line of --key-file, or
// typed at the prompt. Empty with --adopt.
func ztJoinKey(cmd *cobra.Command, provider string) (string, error) {
	adopt, _ := cmd.Flags().GetBool("adopt")
	key, _ := cmd.Flags().GetString("key")
	file, _ := cmd.Flags().GetString("key-file")
	given := 0
	for _, on := range []bool{adopt, key != "", file != ""} {
		if on {
			given++
		}
	}
	if given > 1 {
		return "", fmt.Errorf("one of --key, --key-file and --adopt")
	}
	switch {
	case adopt:
		return "", nil
	case key != "":
		return key, nil
	case file != "":
		data, err := os.ReadFile(file)
		if err != nil {
			return "", fmt.Errorf("reading the key: %w", err)
		}
		line, _, _ := strings.Cut(strings.TrimSpace(string(data)), "\n")
		if line = strings.TrimSpace(line); line == "" {
			return "", fmt.Errorf("%s is empty", file)
		}
		return line, nil
	}
	name := "Auth key"
	if provider == "netbird" {
		name = "Setup key"
	}
	typed := strings.TrimSpace(output.ReadSecret(name))
	if typed == "" {
		return "", fmt.Errorf("no key given: --key, --key-file, or --adopt to take a client that is already connected on the server")
	}
	return typed, nil
}

var ztJoinCmd = &cobra.Command{
	Use:   "join <tailscale|netbird>",
	Short: "Put the server on a mesh network",
	Long: `Join a Tailscale or NetBird network: the server installs the client from its
vendor's package repository, signs in with the key and lets the mesh through
its firewall. Nothing is closed — SSH and 80/443 answer as before, until
"zt lock".

The key is a Tailscale auth key (admin console → Settings → Keys) or a NetBird
setup key. It is asked for at a prompt; --key-file reads it from a file,
--key takes it on the command line (it then stays in the shell's history).
The server uses it once and keeps it nowhere. --adopt takes a client that is
already installed and connected on the server as it is.

  cipi-cli zt join tailscale --tags cipi
  cipi-cli zt join tailscale --key-file ~/ts.key --hostname web1 --tags cipi,prod
  cipi-cli zt join netbird --control-url https://netbird.example.com
  cipi-cli zt join tailscale --adopt

A Tailscale machine joined with a tagged key has a node key that never
expires — which "zt lock ssh" asks for. One mesh per server; Cloudflare
Tunnel may be on beside it. It takes a minute or two.

Requires Cipi 5.18.0+, cipi/api 1.48+ and the zt-manage ability.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		provider, err := ztProvider(args[0])
		if err != nil {
			output.Error("%s", err)
			return err
		}
		opts := ztJoinOptions{}
		opts.Hostname, _ = cmd.Flags().GetString("hostname")
		opts.ControlURL, _ = cmd.Flags().GetString("control-url")
		opts.Tags, _ = cmd.Flags().GetString("tags")
		opts.Direct, _ = cmd.Flags().GetBool("direct")
		opts.AcceptDNS, _ = cmd.Flags().GetBool("accept-dns")
		opts.AcceptRoutes, _ = cmd.Flags().GetBool("accept-routes")
		opts.SSH, _ = cmd.Flags().GetBool("ssh")
		opts.Re, _ = cmd.Flags().GetBool("reauth")
		// What is wrong with the flags is said before a key is asked for.
		if _, err := ztJoinBody(provider, ztJoinOptions{Hostname: opts.Hostname, ControlURL: opts.ControlURL, Tags: opts.Tags, Direct: opts.Direct, AcceptDNS: opts.AcceptDNS, AcceptRoutes: opts.AcceptRoutes, SSH: opts.SSH}); err != nil {
			output.Error("%s", err)
			return err
		}
		if opts.Key, err = ztJoinKey(cmd, provider); err != nil {
			output.Error("%s", err)
			return err
		}
		body, err := ztJoinBody(provider, opts)
		if err != nil {
			output.Error("%s", err)
			return err
		}

		client, err := mustClient()
		if err != nil {
			return err
		}
		// The server installs a package and signs in.
		client.HTTPClient.Timeout = 5 * time.Minute

		label := map[string]string{"tailscale": "Tailscale", "netbird": "NetBird"}[provider]
		if !jsonFlag {
			output.Info("Joining %s — the server installs the client and signs in, a minute or two...", label)
		}
		var result ztChange
		if err := client.Post("/api/zt/mesh", body, &result); err != nil {
			output.Error("Failed to join %s: %s", label, featureErrorHint(err, ztManageAPI, "zt-manage"))
			return err
		}
		ztPrintChange(result, "This server is on the "+label+" network. Nothing was closed.")
		return nil
	},
}

var ztLeaveCmd = &cobra.Command{
	Use:   "leave",
	Short: "Take the server off its mesh network",
	Long: `Leave the mesh network: what its locks closed is reopened to the world first,
then the client is removed from the server. The machine stays listed in the
provider's console until it is deleted there.

The server refuses when this call itself reaches it over the mesh: it would
cut itself off. Do that at the server's shell (cipi zt tailscale|netbird
disable).

Prompts for confirmation unless -y / --yes is passed.

Requires Cipi 5.18.0+, cipi/api 1.48+ and the zt-manage ability.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		yes, _ := cmd.Flags().GetBool("yes")
		if !yes && !output.Confirm("Leave the mesh network? What its locks closed is reopened to the world, then the client is removed.") {
			output.Warn("Aborted")
			return nil
		}
		client, err := mustClient()
		if err != nil {
			return err
		}
		client.HTTPClient.Timeout = 2 * time.Minute

		var result ztChange
		if err := client.Delete("/api/zt/mesh", nil, &result); err != nil {
			output.Error("Failed to leave the network: %s", featureErrorHint(err, ztManageAPI, "zt-manage"))
			return err
		}
		ztPrintChange(result, "This server left the network.")
		return nil
	},
}

// ztLockRun is `zt lock` and `zt unlock`.
func ztLockRun(cmd *cobra.Command, arg string, lock bool) error {
	port, err := ztPort(arg)
	if err != nil {
		output.Error("%s", err)
		return err
	}
	client, err := mustClient()
	if err != nil {
		return err
	}
	if lock {
		yes, _ := cmd.Flags().GetBool("yes")
		if !yes && !output.Confirm(ztLockQuestion(port, "mesh")) {
			output.Warn("Aborted")
			return nil
		}
	}

	var result ztChange
	body := map[string]interface{}{"port": port}
	what := map[string]string{"ssh": "SSH", "http": "80/443"}[port]
	if lock {
		err = client.Post("/api/zt/mesh/lock", body, &result)
	} else {
		err = client.Delete("/api/zt/mesh/lock", body, &result)
	}
	if err != nil {
		verb := map[bool]string{true: "close", false: "reopen"}[lock]
		output.Error("Failed to %s %s: %s", verb, what, featureErrorHint(err, ztManageAPI, "zt-manage"))
		return err
	}
	done := what + " is open to the world again."
	if lock {
		done = what + " is closed to the world: it answers on the " + ztMeshLabel(result.Data) + " network only."
	}
	ztPrintChange(result, done)
	return nil
}

var ztLockCmd = &cobra.Command{
	Use:   "lock <ssh|http>",
	Short: "Close SSH or 80/443 to the world, open on the mesh only",
	Long: `Close a port to the world: it stays open on the mesh network.

  cipi-cli zt lock ssh     # port 22: SSH from the devices of the network only
  cipi-cli zt lock http    # 80 and 443: the sites, the panel and the API too

The server refuses what would lock people out, and says why:

  ssh   until sshd has accepted a login that came over the mesh (sign in once:
        ssh cipi@<the server's address on the network>), while the node key
        can expire, with the firewall off, or with Tailscale SSH on
  http  while an app renews its certificate over HTTP-01, and when this very
        call reaches the API on the public address the lock would close —
        point the profile's endpoint at the server's mesh address or name,
        or at the hostname on its Cloudflare tunnel

"cipi-cli zt" shows it beforehand, on its "refused" lines.

Prompts for confirmation unless -y / --yes is passed.

Requires Cipi 5.18.0+, cipi/api 1.48+ and the zt-manage ability.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return ztLockRun(cmd, args[0], true)
	},
}

var ztUnlockCmd = &cobra.Command{
	Use:   "unlock <ssh|http>",
	Short: "Reopen SSH or 80/443 to the world",
	Long: `Reopen a port the mesh network's lock closed.

  cipi-cli zt unlock ssh
  cipi-cli zt unlock http

A port Cloudflare's own lock also holds stays closed, and the server says so:
that one is reopened at the server's shell (cipi zt ssh unlock, cipi zt unlock
http).

Requires Cipi 5.18.0+, cipi/api 1.48+ and the zt-manage ability.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return ztLockRun(cmd, args[0], false)
	},
}

var ztSSHCmd = &cobra.Command{
	Use:   "ssh <on|off>",
	Short: "Turn Tailscale SSH on or off",
	Long: `Tailscale SSH: on the tailnet address, port 22 is answered by Tailscale, and
the tailnet's policy says who signs in and as which user — not the keys on the
server. Off, SSH over the network is OpenSSH, with the server's keys.

  cipi-cli zt ssh on
  cipi-cli zt ssh off

Tailscale only: over NetBird, SSH is OpenSSH. With Tailscale SSH on, SSH is
closed to the world at the server's shell, not from here.

Requires Cipi 5.18.0+, cipi/api 1.48+ and the zt-manage ability.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		on, err := ztOnOff(args[0])
		if err != nil {
			output.Error("%s", err)
			return err
		}
		client, err := mustClient()
		if err != nil {
			return err
		}
		var result ztChange
		if err := client.Put("/api/zt/mesh/ssh", map[string]interface{}{"enabled": on}, &result); err != nil {
			output.Error("Failed to change Tailscale SSH: %s", featureErrorHint(err, ztManageAPI, "zt-manage"))
			return err
		}
		ztPrintChange(result, "Tailscale SSH is "+map[bool]string{true: "on", false: "off"}[on]+".")
		return nil
	},
}

func init() {
	ztJoinCmd.Flags().String("key", "", "The auth key or setup key (stays in the shell's history: prefer the prompt or --key-file)")
	ztJoinCmd.Flags().String("key-file", "", "Read the key from the first line of this file")
	ztJoinCmd.Flags().Bool("adopt", false, "No key: take a client that is already installed and connected on the server")
	ztJoinCmd.Flags().String("hostname", "", "The machine's name on the network (default: the server's hostname)")
	ztJoinCmd.Flags().String("control-url", "", "The coordination server when it is not the vendor's: a Headscale, a self-hosted NetBird")
	ztJoinCmd.Flags().String("tags", "", "Tailscale: tags of the machine, separated by commas (cipi,prod)")
	ztJoinCmd.Flags().Bool("direct", false, "Also open the client's UDP port, for direct connections instead of a relay")
	ztJoinCmd.Flags().Bool("accept-dns", false, "Let the network set the server's DNS")
	ztJoinCmd.Flags().Bool("accept-routes", false, "Accept the routes other machines of the network advertise")
	ztJoinCmd.Flags().Bool("ssh", false, "Tailscale: turn Tailscale SSH on right away")
	ztJoinCmd.Flags().Bool("reauth", false, "Tailscale: sign in again a machine that is already registered (needs a key)")
	ztLeaveCmd.Flags().BoolP("yes", "y", false, "Skip confirmation")
	ztLockCmd.Flags().BoolP("yes", "y", false, "Skip confirmation")

	ztCmd.AddCommand(ztJoinCmd, ztLeaveCmd, ztLockCmd, ztUnlockCmd, ztSSHCmd)
}
