package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/cipi-sh/cli/internal/api"
	"github.com/cipi-sh/cli/internal/config"
	"github.com/cipi-sh/cli/internal/output"
	"github.com/spf13/cobra"
)

// `cipi-cli sync`: the apps of a server sent to another one. The server of
// the profile in use is the source — it exports, sends over SSH and has the
// target import. See sync_rules.go for what is checked before it is asked.

type syncKeyAnswer struct {
	Data struct {
		Key         string `json:"key"`
		Type        string `json:"type"`
		Fingerprint string `json:"fingerprint"`
		Comment     string `json:"comment"`
		Changed     bool   `json:"changed"`
	} `json:"data"`
	Message string `json:"message"`
}

type syncCheckAnswer struct {
	Data map[string]interface{} `json:"data"`
}

func syncFail(what string, err error) error {
	output.Error("%s: %s", what, featureErrorHint(err, syncAPI, "sync-manage"))
	return err
}

// syncServerKey asks a server for its sync key.
func syncServerKey(client *api.Client) (string, error) {
	var answer syncKeyAnswer
	if err := client.Get("/api/sync/key", &answer); err != nil {
		return "", err
	}
	return syncKeyLine(answer.Data.Key)
}

// syncGivenKey is the key of a trust or an untrust: the argument, the
// content of --key-file, the key of the server of --from, or what is pasted.
func syncGivenKey(cmd *cobra.Command, args []string) (string, error) {
	file, _ := cmd.Flags().GetString("key-file")
	from, _ := cmd.Flags().GetString("from")
	given := 0
	for _, on := range []bool{len(args) > 0, file != "", from != ""} {
		if on {
			given++
		}
	}
	if given > 1 {
		return "", fmt.Errorf("one of: the key as an argument, --key-file, --from")
	}
	switch {
	case from != "":
		source, err := api.NewClientForProfile(from)
		if err != nil {
			return "", err
		}
		key, err := syncServerKey(source)
		if err != nil {
			return "", fmt.Errorf("reading the key of %s: %s", from, featureErrorHint(err, syncAPI, "sync-manage"))
		}
		return key, nil
	case file != "":
		data, err := os.ReadFile(file)
		if err != nil {
			return "", fmt.Errorf("reading the key: %w", err)
		}
		return syncKeyLine(string(data))
	case len(args) > 0:
		return syncKeyLine(strings.Join(args, " "))
	}
	return syncKeyLine(output.ReadLine("Public key of the other server (cipi-cli sync key there)"))
}

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Send the apps of this server to another server",
	Long: `Copy apps from one Cipi server to another: a move to a bigger machine, a copy
of production on staging, a second server kept ready.

The server of the profile in use is the source. It exports the apps — their
settings, .env, deploy recipe, workers and crons, with databases and uploaded
files when asked — into one archive, encrypts it, sends it over SSH and has
the target import it. Apps that are not on the target are created there; apps
that are, updated. Nothing is removed or stopped on the source.

  cipi-cli prod sync push --to staging --with-db       both servers are profiles
  cipi-cli prod sync push shop --host 203.0.113.9      a server that is not

The target has to trust the source's key first. With --to the client does it
on the target and takes the key back out when the transfer ends; without, it
is "cipi sync trust" on the target, with the key of "cipi-cli sync key".

Encryption is the server's: the archive is encrypted with a passphrase the
source makes for that one transfer and hands to the target over SSH. It is
never shown and never passes through this client or the API.

Requires Cipi 5.18.0+, cipi/api 1.48+ and the sync-manage ability, on the
source — and on the target, for --to and for trust / untrust.

` + multiServerTip,
}

var syncKeyCmd = &cobra.Command{
	Use:   "key",
	Short: "This server's sync key: what a target has to trust",
	Long: `The public key this server signs in with when it sends apps to another one.

  cipi-cli prod sync key
  cipi-cli prod sync key --raw | ssh root@target cipi sync trust --key="$(cat)"

--raw prints the key and nothing else.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}
		var answer syncKeyAnswer
		if err := client.Get("/api/sync/key", &answer); err != nil {
			return syncFail("Failed to read the sync key", err)
		}
		if raw, _ := cmd.Flags().GetBool("raw"); raw {
			fmt.Println(answer.Data.Key)
			return nil
		}
		if jsonFlag {
			output.PrintJSON(answer)
			return nil
		}
		output.Header("Sync key of this server")
		output.KeyValue(nil, "key", answer.Data.Key)
		output.KeyValue(nil, "fingerprint", answer.Data.Fingerprint)
		fmt.Println()
		output.Info("On the server that receives: cipi sync trust (or cipi-cli <its profile> sync trust --from <this profile>)")
		fmt.Println()
		return nil
	},
}

var syncCheckCmd = &cobra.Command{
	Use:   "check <host>",
	Short: "Can this server send to that one",
	Long: `Ask this server whether the target accepts its key over SSH and runs a Cipi
that can take a transfer. Nothing is sent.

  cipi-cli prod sync check 203.0.113.9
  cipi-cli prod sync check fra2.tail1234.ts.net --port 2222

The address is the one this server reaches the target on — not this machine.
Exits with an error when the way is not open.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		port, _ := cmd.Flags().GetInt("port")
		if err := syncTarget(args[0], port); err != nil {
			output.Error("%s", err)
			return err
		}
		client, err := mustClient()
		if err != nil {
			return err
		}
		// The server waits for SSH to answer, or not to.
		client.HTTPClient.Timeout = 90 * time.Second
		var answer syncCheckAnswer
		if err := client.Post("/api/sync/check", map[string]interface{}{"host": args[0], "port": port}, &answer); err != nil {
			return syncFail("Failed to check the target", err)
		}
		if jsonFlag {
			output.PrintJSON(answer)
		}
		ready, line := syncVerdict(answer.Data, false)
		if !ready {
			if !jsonFlag {
				output.Error("%s", line)
			}
			return fmt.Errorf("%s", line)
		}
		if !jsonFlag {
			output.Success("%s", line)
			fmt.Println()
		}
		return nil
	},
}

func syncTrustRun(remove bool) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		key, err := syncGivenKey(cmd, args)
		if err != nil {
			output.Error("%s", err)
			return err
		}
		client, err := mustClient()
		if err != nil {
			return err
		}
		var answer syncKeyAnswer
		if remove {
			err = client.Delete("/api/sync/trust", map[string]string{"key": key}, &answer)
		} else {
			err = client.Post("/api/sync/trust", map[string]string{"key": key}, &answer)
		}
		if err != nil {
			return syncFail(map[bool]string{true: "Failed to remove the key", false: "Failed to trust the key"}[remove], err)
		}
		if jsonFlag {
			output.PrintJSON(answer)
			return nil
		}
		name := answer.Data.Fingerprint
		if answer.Data.Comment != "" {
			name = answer.Data.Comment + "  " + name
		}
		switch {
		case remove && answer.Data.Changed:
			output.Success("No longer trusted here: %s", name)
		case remove:
			output.Info("That key was not trusted here: %s", name)
		case answer.Data.Changed:
			output.Success("Trusted: its server can now send apps here. %s", name)
		default:
			output.Info("That key was trusted already: %s", name)
		}
		fmt.Println()
		return nil
	}
}

var syncTrustCmd = &cobra.Command{
	Use:   "trust [public key]",
	Short: "Let another server send apps to this one",
	Long: `Add the sync key of another server to this one: that server's cipi user can
then sign in here over SSH, which is what its "sync push" needs.

  cipi-cli staging sync trust --from prod            the key of another profile
  cipi-cli staging sync trust --key-file prod.pub
  cipi-cli staging sync trust "ssh-ed25519 AAAA… cipi@prod"
  cipi-cli staging sync trust                        paste it at the prompt

Trusting a key is giving its holder a shell as cipi on this server: only the
key of a server of yours. "sync untrust" takes it back.`,
	Args: cobra.ArbitraryArgs,
	RunE: syncTrustRun(false),
}

var syncUntrustCmd = &cobra.Command{
	Use:   "untrust [public key]",
	Short: "Take a server's sync key back out",
	Long: `Remove the sync key of another server from this one: it can no longer send
apps here, nor sign in.

  cipi-cli staging sync untrust --from prod
  cipi-cli staging sync untrust --key-file prod.pub`,
	Args: cobra.ArbitraryArgs,
	RunE: syncTrustRun(true),
}

var syncPushCmd = &cobra.Command{
	Use:   "push [app...]",
	Short: "Send apps to another server and import them there",
	Long: `Send the apps named — every app, when none is — to another Cipi server, and
import them there. Apps that are not on the target are created; apps that are,
updated. Nothing is removed or stopped on this server, and DNS is yours to
move.

  cipi-cli prod sync push --to staging --with-db
  cipi-cli prod sync push shop blog --to staging --with-db --with-storage
  cipi-cli prod sync push --host 203.0.113.9 --with-db

--to names the target by its profile: the client adds this server's key there,
checks the way, and takes the key back out when the transfer ends (--keep-trust
leaves it, for transfers that repeat). The address is the host of that
profile's endpoint unless --host says another — a private or mesh address, or
the real one of a server whose API is behind a proxy.

--host alone is for a server that is not a profile: it has to trust this
server's key already ("cipi-cli sync key" here, "cipi sync trust" there).

--with-db dumps the databases here and restores them there, replacing the
database of an app that is already on the target. --with-storage adds each
app's shared/storage, which can be large.

The archive is encrypted on this server with a passphrase made for the
transfer and handed to the target over SSH: there is nothing to type, and
nothing to keep. A transfer may take long; it is followed for two hours.

Prompts for confirmation unless -y / --yes is passed.

Requires Cipi 5.18.0+ on both servers, cipi/api 1.48+ and the sync-manage
ability.`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		host, _ := cmd.Flags().GetString("host")
		to, _ := cmd.Flags().GetString("to")
		port, _ := cmd.Flags().GetInt("port")
		withDB, _ := cmd.Flags().GetBool("with-db")
		withStorage, _ := cmd.Flags().GetBool("with-storage")
		keep, _ := cmd.Flags().GetBool("keep-trust")
		yes, _ := cmd.Flags().GetBool("yes")

		var target *api.Client
		if to != "" {
			profile, err := config.LoadNamed(to)
			if err != nil {
				output.Error("%s", err)
				return err
			}
			target = api.NewClientFromProfile(profile)
			if host == "" {
				if host = syncEndpointHost(profile.Endpoint); host == "" {
					err := fmt.Errorf("the endpoint of %s has no host to reach over SSH: say it with --host", to)
					output.Error("%s", err)
					return err
				}
			}
		}
		if host == "" {
			err := fmt.Errorf("where to: --to <profile>, or --host <address> for a server that is not a profile")
			output.Error("%s", err)
			return err
		}
		body, err := syncPushBody(host, port, args, withDB, withStorage)
		if err != nil {
			output.Error("%s", err)
			return err
		}
		apps, _ := body["apps"].([]string)

		source, err := mustClient()
		if err != nil {
			return err
		}

		// A target that is a profile is prepared from here: this server's key goes there.
		added, key := false, ""
		if target != nil {
			if key, err = syncServerKey(source); err != nil {
				return syncFail("Failed to read this server's sync key", err)
			}
			var trusted syncKeyAnswer
			if err := target.Post("/api/sync/trust", map[string]string{"key": key}, &trusted); err != nil {
				return syncFail("Failed to add this server's key on "+to, err)
			}
			added = trusted.Data.Changed
		}
		// Whatever happens next, a key added here does not outlive the transfer.
		forget := func() {
			if !added || keep {
				return
			}
			added = false
			if err := target.Delete("/api/sync/trust", map[string]string{"key": key}, nil); err != nil {
				output.Warn("This server's key is still trusted on %s — it could not be removed (%s). There: cipi-cli %s sync untrust --from <this profile>", to, err, to)
				return
			}
			output.Info("This server's key was removed from %s.", to)
		}
		defer forget()

		source.HTTPClient.Timeout = 90 * time.Second
		var check syncCheckAnswer
		if err := source.Post("/api/sync/check", map[string]interface{}{"host": host, "port": port}, &check); err != nil {
			return syncFail("Failed to check the target", err)
		}
		ready, line := syncVerdict(check.Data, target != nil)
		if !ready {
			output.Error("%s", line)
			return fmt.Errorf("%s", line)
		}
		output.Info("%s", line)

		name := host
		if to != "" {
			name = to + " (" + host + ")"
		}
		if !yes && !output.Confirm(syncSummary(apps, withDB, withStorage, name)) {
			output.Warn("Aborted")
			return nil
		}

		source.HTTPClient.Timeout = 30 * time.Second
		output.Info("Exporting, sending and importing — this can take a while...")
		job, err := source.DoAsyncAndWaitResultFor("POST", "/api/sync/push", body, api.DeployJobWait)
		if job != nil && job.Output != "" && !jsonFlag {
			printCLIOutput(job.Output)
		}
		if err != nil {
			return syncFail("Transfer failed", err)
		}
		if jsonFlag && job != nil {
			output.PrintJSON(job)
		}
		output.Success("Transfer to %s done. Point DNS at it when it is ready to take over.", name)
		fmt.Println()
		return nil
	},
}

func init() {
	syncKeyCmd.Flags().Bool("raw", false, "Print the key and nothing else")
	syncCheckCmd.Flags().Int("port", 22, "SSH port of the target")
	for _, c := range []*cobra.Command{syncTrustCmd, syncUntrustCmd} {
		c.Flags().String("key-file", "", "Read the public key from this file")
		c.Flags().String("from", "", "Take the key of the server of this profile")
	}
	syncPushCmd.Flags().String("to", "", "The target, by its profile: prepared and cleaned up from here")
	syncPushCmd.Flags().String("host", "", "The address this server reaches the target on over SSH")
	syncPushCmd.Flags().Int("port", 22, "SSH port of the target")
	syncPushCmd.Flags().Bool("with-db", false, "Send the databases too (replaces the database of an app already on the target)")
	syncPushCmd.Flags().Bool("with-storage", false, "Send each app's shared/storage too")
	syncPushCmd.Flags().Bool("keep-trust", false, "With --to: leave this server's key trusted on the target")
	syncPushCmd.Flags().BoolP("yes", "y", false, "Skip confirmation")

	syncCmd.AddCommand(syncKeyCmd, syncCheckCmd, syncTrustCmd, syncUntrustCmd, syncPushCmd)
	rootCmd.AddCommand(syncCmd)
}
