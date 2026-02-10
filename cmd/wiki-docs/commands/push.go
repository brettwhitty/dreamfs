package commands

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var pushCmd = &cobra.Command{
	Use:   "push [file]",
	Short: "Update existing files in wiki",
	Long: `Updates existing files in the wiki. Enforces branch protection and manual review.
For new files, use 'wiki-sync add'.`,
	Run: func(cmd *cobra.Command, args []string) {
		// 1. Checks
		if err := assertEditorSet(); err != nil {
			printFatal("Editor Not Configured", err,
				"Set EDITOR environment variable in your profile.",
				"PowerShell: $env:EDITOR='code -w'",
				"Cmd: setx EDITOR \"code -w\"",
				"Git Bash: export EDITOR='code -w'",
			)
		}

		cfg, err := getConfig(cmd)
		if err != nil {
			fmt.Println(styleErr.Render("Error getting config: " + err.Error()))
			os.Exit(1)
		}

		if err := validateWikiDir(cfg.WikiDir); err != nil {
			fmt.Println(styleErr.Render(err.Error()))
			os.Exit(1)
		}

		if err := checkWikiBranch(cfg.WikiDir); err != nil {
			fmt.Printf("⛔ %s\n", styleErr.Render(err.Error()))
			os.Exit(1)
		}

		// 2. Discovery
		targetFile := ""
		if len(args) > 0 {
			targetFile = args[0]
		}

		fmt.Println(styleInfo.Render("Scanning for updates..."))

		items, err := ScanAll(cfg)
		if err != nil {
			fmt.Println(styleErr.Render("Discovery failed: " + err.Error()))
			os.Exit(1)
		}

		// Filter for EXISTING/CHANGED files only (No "New")
		var updates []FileItem
		for _, item := range items {
			// If target specified, strict filter
			if targetFile != "" {
				normTarget := filepath.ToSlash(targetFile)
				if item.RelPath != normTarget && !strings.HasSuffix(item.RelPath, normTarget) {
					continue
				}
			}

			if item.Status == "Changed" || item.Status == "Same" || item.Status == "Legacy" {
				updates = append(updates, item)
			} else if item.Status == "New" {
				if targetFile != "" {
					fmt.Println(styleErr.Render(fmt.Sprintf("File '%s' does not exist in wiki. Use 'wiki-sync add' for new files.", item.RelPath)))
				}
			}
		}

		if len(updates) == 0 {
			fmt.Println(styleSuccess.Render("No existing files to update."))
			return
		}

		// 3. Selection
		var selected []FileItem
		if targetFile != "" {
			selected = updates
		} else {
			selected = runInteractive(updates)
		}

		if len(selected) == 0 {
			fmt.Println("No files selected.")
			return
		}

		// 4. Processing
		for _, item := range selected {
			fmt.Println(strings.Repeat("=", 60))
			fmt.Printf("Updating: %s\n", styleInfo.Render(item.RelPath))

			// A. Revision Check
			remoteSHA, err := getFileGitRevision(cfg.WikiDir, item.WikiPath)
			if err != nil {
				fmt.Println(styleErr.Render("Failed to get remote revision: " + err.Error()))
				continue
			}

			var fmMap map[string]interface{}
			if err := yaml.Unmarshal([]byte(item.LocalContent), &fmMap); err != nil {
				// Invalid YAML in local file, might not have keys we check.
				// Proceed but treat as empty map for checks?
			}

			// 1. ReadOnly Check
			if val, ok := fmMap["readonly"]; ok {
				if isRO, ok := val.(bool); ok && isRO {
					fmt.Println(styleErr.Render("⛔ SKIPPING: File is marked as 'readonly'"))
					continue
				}
			}

			// 2. Integrity Checks (State-based)
			state, _ := LoadState()
			var storedSum, storedRev string
			if state != nil {
				if fState, ok := state.Get(item.RelPath); ok {
					storedSum = fState.LastChecksum
					storedRev = fState.LastRev
				}
			}

			if storedSum != "" {
				localBody := stripFrontmatter(item.LocalContent)
				calcSum := CalculateChecksum(localBody)

				if storedSum != calcSum {
					fmt.Println(styleErr.Render("⛔ INTEGRITY ERROR: Local file modified outside of wiki-sync workflow."))
					fmt.Printf("  Stored Checksum: %s\n", storedSum)
					fmt.Printf("  Actual Checksum: %s\n", calcSum)
					fmt.Println(styleInfo.Render("This file is protected. Please revert local changes and edit via wiki or use 'wiki-sync pull'."))
					continue
				} else {
					fmt.Println(styleSuccess.Render("✓ Integrity verified"))
				}
			}

			// 3. Revision Check
			localRev := storedRev

			if remoteSHA != "" && localRev != "" && localRev != remoteSHA {
				fmt.Println(styleErr.Render("⛔ STOMP DETECTED: Wiki has changed since last pull."))
				fmt.Printf("  Local Revision:  %s\n", localRev)
				fmt.Printf("  Remote Revision: %s\n", remoteSHA)
				fmt.Println(styleInfo.Render("Please 'wiki-sync pull' to merge changes before pushing."))
				continue
			} else if remoteSHA != "" && localRev == "" {
				fmt.Println(styleInfo.Render("⚠️  No local state found. Proceeding with caution."))
			}

			fmt.Println(styleSuccess.Render(fmt.Sprintf("✓ Revision verified (%s)", remoteSHA)))

			// B. Editor Review
			tmpFile, err := os.CreateTemp("", "wiki-update-*.md")
			if err != nil {
				fmt.Println(styleErr.Render("Temp file error: " + err.Error()))
				continue
			}
			tmpPath := tmpFile.Name()
			if err := os.WriteFile(tmpPath, []byte(item.LocalContent), 0644); err != nil {
				fmt.Println(styleErr.Render("Error writing temp file: " + err.Error()))
				continue
			}
			tmpFile.Close()

			fmt.Println(styleInfo.Render("Launching $EDITOR..."))
			editor := os.Getenv("EDITOR")

			cmd := exec.Command(editor, tmpPath)
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				fmt.Println(styleErr.Render("Editor failed: " + err.Error()))
				os.Remove(tmpPath)
				continue
			}

			editedBytes, _ := os.ReadFile(tmpPath)
			editedContent := string(editedBytes)
			os.Remove(tmpPath)

			// C. Confirm
			confirm := false
			err = huh.NewConfirm().
				Title(fmt.Sprintf("Push updates to %s?", item.RelPath)).
				Value(&confirm).
				Run()

			if err != nil || !confirm {
				fmt.Println("Skipped.")
				continue
			}

			// D. Write
			destPath := filepath.Join(cfg.WikiDir, item.WikiPath)
			if err := os.WriteFile(destPath, []byte(editedContent), 0644); err != nil {
				fmt.Println(styleErr.Render("Write failed: " + err.Error()))
			} else {
				fmt.Println(styleSuccess.Render("✓ Updated"))
			}
		}
	},
}

// discoverFilesPush is deprecated, use ScanAll
func discoverFilesPush(cfg Config, target string) ([]FileItem, error) {
	return ScanAll(cfg)
}

func init() {
	rootCmd.AddCommand(pushCmd)
}
