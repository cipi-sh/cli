package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/cipi-sh/cli/internal/output"
	"github.com/spf13/cobra"
)

const (
	githubRepo  = "cipi-sh/cli"
	releasesAPI = "https://api.github.com/repos/" + githubRepo + "/releases/latest"
)

type ghAsset struct {
	Name        string `json:"name"`
	DownloadURL string `json:"browser_download_url"`
}

type ghRelease struct {
	TagName string    `json:"tag_name"`
	HTMLURL string    `json:"html_url"`
	Assets  []ghAsset `json:"assets"`
}

var updateCmd = &cobra.Command{
	Use:     "update",
	Aliases: []string{"self-update", "upgrade"},
	Short:   "Update the CLI to the latest release",
	RunE: func(cmd *cobra.Command, args []string) error {
		force, _ := cmd.Flags().GetBool("force")

		output.Info("Checking for the latest release...")
		release, err := fetchLatestRelease()
		if err != nil {
			output.Error("Failed to check for updates: %s", err)
			return err
		}

		latest := release.TagName
		if latest == "" {
			output.Error("Could not determine the latest version")
			return fmt.Errorf("empty tag_name in release response")
		}

		if !force && sameVersion(Version, latest) {
			output.Success("Already up to date (%s)", Version)
			fmt.Println()
			return nil
		}

		assetName := fmt.Sprintf("cipi-cli-%s-%s", runtime.GOOS, runtime.GOARCH)
		assetURL := findAsset(release.Assets, assetName)
		if assetURL == "" {
			output.Error("No prebuilt binary for %s/%s in release %s", runtime.GOOS, runtime.GOARCH, latest)
			if names := assetNames(release.Assets); len(names) == 0 {
				output.Info("Release %s was published without binaries — the CI build may not have run.", latest)
				output.Info("Maintainers: re-run the Release workflow for this tag, or publish a new tag (e.g. v1.0.1).")
			} else {
				output.Info("Available assets in %s: %s", latest, strings.Join(names, ", "))
			}
			return fmt.Errorf("asset %q not found in release %s", assetName, latest)
		}

		exePath, err := os.Executable()
		if err != nil {
			output.Error("Cannot locate the running binary: %s", err)
			return err
		}
		if resolved, err := filepath.EvalSymlinks(exePath); err == nil {
			exePath = resolved
		}

		output.Info("Downloading %s (%s)...", latest, assetName)
		binData, err := download(assetURL)
		if err != nil {
			output.Error("Download failed: %s", err)
			return err
		}

		if checksumURL := findAsset(release.Assets, "checksums.txt"); checksumURL != "" {
			if err := verifyChecksum(binData, assetName, checksumURL); err != nil {
				output.Error("Checksum verification failed: %s", err)
				return err
			}
			output.Success("Checksum verified")
		} else {
			output.Warn("No checksums.txt in release — skipping verification")
		}

		if err := replaceBinary(exePath, binData); err != nil {
			if errors.Is(err, os.ErrPermission) {
				output.Error("Permission denied writing to %s", exePath)
				output.Info("Re-run with elevated privileges: sudo cipi-cli update")
				return err
			}
			output.Error("Failed to install update: %s", err)
			return err
		}

		output.Success("Updated %s → %s", Version, latest)
		output.Dim.Printf("  Installed at %s\n\n", exePath)
		return nil
	},
}

func fetchLatestRelease() (*ghRelease, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("GET", releasesAPI, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "cipi-cli")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned HTTP %d", resp.StatusCode)
	}

	var release ghRelease
	if err := json.Unmarshal(body, &release); err != nil {
		return nil, fmt.Errorf("parsing release response: %w", err)
	}
	return &release, nil
}

func findAsset(assets []ghAsset, name string) string {
	for _, a := range assets {
		if a.Name == name {
			return a.DownloadURL
		}
	}
	return ""
}

func assetNames(assets []ghAsset) []string {
	names := make([]string, 0, len(assets))
	for _, a := range assets {
		if a.Name != "" {
			names = append(names, a.Name)
		}
	}
	return names
}

func download(url string) ([]byte, error) {
	client := &http.Client{Timeout: 5 * time.Minute}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "cipi-cli")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download returned HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func verifyChecksum(data []byte, assetName, checksumURL string) error {
	raw, err := download(checksumURL)
	if err != nil {
		return fmt.Errorf("downloading checksums: %w", err)
	}

	var expected string
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		// checksums.txt entries look like "<sha256>  <filename>"
		if filepath.Base(fields[1]) == assetName {
			expected = strings.ToLower(fields[0])
			break
		}
	}
	if expected == "" {
		return fmt.Errorf("no checksum entry for %s", assetName)
	}

	sum := sha256.Sum256(data)
	actual := hex.EncodeToString(sum[:])
	if actual != expected {
		return fmt.Errorf("expected %s, got %s", expected, actual)
	}
	return nil
}

func replaceBinary(exePath string, data []byte) error {
	dir := filepath.Dir(exePath)
	tmp, err := os.CreateTemp(dir, ".cipi-cli-update-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0755); err != nil {
		return err
	}
	return os.Rename(tmpName, exePath)
}

// sameVersion reports whether the running build matches the latest tag,
// tolerating a missing "v" prefix on either side.
func sameVersion(current, latest string) bool {
	norm := func(s string) string {
		return strings.TrimPrefix(strings.TrimSpace(s), "v")
	}
	if current == "" || current == "dev" {
		return false
	}
	return norm(current) == norm(latest)
}

func init() {
	updateCmd.Flags().Bool("force", false, "Reinstall even if already on the latest version")
	rootCmd.AddCommand(updateCmd)
}
