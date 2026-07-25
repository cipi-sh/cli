package cmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/cipi-sh/cli/internal/config"
	"github.com/cipi-sh/cli/internal/output"
	"github.com/spf13/cobra"
)

var completionInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install shell completion for the current shell",
	Long: `Detect your shell, generate the completion script, and install it.

Supported shells: zsh, bash, fish.

For zsh/bash, the script is written under ~/.cipi/completions/ and a
source line is added to your shell rc file (idempotent).

For fish, the script is written to ~/.config/fish/completions/ (auto-loaded).

Restart your shell (or source the rc file) after installing.`,
	Example: `  cipi-cli completion install
  cipi-cli completion install --shell zsh
  cipi-cli completion install --shell bash`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		shell, _ := cmd.Flags().GetString("shell")
		if shell == "" {
			shell = detectShell()
		}
		shell = strings.ToLower(strings.TrimSpace(shell))

		switch shell {
		case "zsh", "bash", "fish":
		default:
			output.Error("Unsupported shell %q — use zsh, bash, or fish", shell)
			output.Info("Override with: cipi-cli completion install --shell zsh")
			return fmt.Errorf("unsupported shell: %s", shell)
		}

		fmt.Println()
		output.Info("Installing %s completion...", shell)

		path, err := installCompletion(shell)
		if err != nil {
			output.Error("%s", err)
			return err
		}

		output.Success("Completion installed → %s", path)
		fmt.Println()
		switch shell {
		case "fish":
			output.Info("Fish loads completions automatically. Open a new shell, then try:")
		default:
			output.Info("Reload your shell, then try:")
			output.Dim.Printf("  source %s\n", shellRCPath(shell))
		}
		output.Dim.Println("  cipi-cli <TAB>")
		fmt.Println()
		return nil
	},
}

func detectShell() string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		return ""
	}
	base := strings.ToLower(filepath.Base(shell))
	switch base {
	case "zsh", "bash", "fish":
		return base
	default:
		return base
	}
}

func installCompletion(shell string) (string, error) {
	var buf bytes.Buffer
	switch shell {
	case "zsh":
		if err := rootCmd.GenZshCompletion(&buf); err != nil {
			return "", fmt.Errorf("generating zsh completion: %w", err)
		}
	case "bash":
		if err := rootCmd.GenBashCompletionV2(&buf, true); err != nil {
			return "", fmt.Errorf("generating bash completion: %w", err)
		}
	case "fish":
		if err := rootCmd.GenFishCompletion(&buf, true); err != nil {
			return "", fmt.Errorf("generating fish completion: %w", err)
		}
	}

	if shell == "fish" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir := filepath.Join(home, ".config", "fish", "completions")
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", fmt.Errorf("creating fish completions dir: %w", err)
		}
		path := filepath.Join(dir, "cipi-cli.fish")
		if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
			return "", fmt.Errorf("writing completion file: %w", err)
		}
		return path, nil
	}

	dir := filepath.Join(config.Dir(), "completions")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("creating completions dir: %w", err)
	}

	filename := "cipi-cli." + shell
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		return "", fmt.Errorf("writing completion file: %w", err)
	}

	rc := shellRCPath(shell)
	marker := "# cipi-cli shell completion"
	sourceLine := completionSourceLine(shell, path)
	added, err := ensureRCSnippet(rc, marker, sourceLine)
	if err != nil {
		output.Warn("Could not update %s: %s", rc, err)
		output.Info("Add this line manually:")
		output.Dim.Printf("  %s\n", sourceLine)
		return path, nil
	}
	if added {
		output.Info("Hooked into %s", rc)
	} else {
		output.Info("Already hooked in %s", rc)
	}

	return path, nil
}

func shellRCPath(shell string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	switch shell {
	case "zsh":
		return filepath.Join(home, ".zshrc")
	case "bash":
		bashrc := filepath.Join(home, ".bashrc")
		if runtime.GOOS == "darwin" {
			bashProfile := filepath.Join(home, ".bash_profile")
			if _, err := os.Stat(bashrc); err != nil {
				return bashProfile
			}
			// Prefer .bash_profile on macOS when it already exists (login shells).
			if _, err := os.Stat(bashProfile); err == nil {
				return bashProfile
			}
		}
		return bashrc
	default:
		return ""
	}
}

func completionSourceLine(shell, path string) string {
	switch shell {
	case "zsh":
		return fmt.Sprintf("source %q", path)
	case "bash":
		return fmt.Sprintf("source %q", path)
	default:
		return ""
	}
}

func ensureRCSnippet(rcPath, marker, sourceLine string) (bool, error) {
	if rcPath == "" {
		return false, fmt.Errorf("could not determine shell rc path")
	}

	data, err := os.ReadFile(rcPath)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}

	content := string(data)
	if strings.Contains(content, marker) || strings.Contains(content, sourceLine) {
		return false, nil
	}

	snippet := "\n" + marker + "\n" + sourceLine + "\n"
	if !strings.HasSuffix(content, "\n") && len(content) > 0 {
		snippet = "\n" + snippet
	}

	f, err := os.OpenFile(rcPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return false, err
	}
	defer f.Close()

	if _, err = f.WriteString(snippet); err != nil {
		return false, err
	}
	return true, nil
}

func init() {
	rootCmd.InitDefaultCompletionCmd()
	completionInstallCmd.Flags().String("shell", "", "Shell to install for (zsh, bash, fish). Defaults to $SHELL")

	for _, c := range rootCmd.Commands() {
		if c.Name() == "completion" {
			c.AddCommand(completionInstallCmd)
			c.Short = "Generate or install shell completion scripts"
			c.Long = `Generate or install shell autocompletion for cipi-cli.

Quick setup (recommended):
  cipi-cli completion install

Or print a script for a specific shell:
  cipi-cli completion zsh
  cipi-cli completion bash
  cipi-cli completion fish`
			break
		}
	}
}
