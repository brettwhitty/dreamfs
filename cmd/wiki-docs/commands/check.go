package commands

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	checkTargetVersion string
)

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Interactive Dashboard (Explorer)",
	Long:  `Unified dashboard to explore sync status and trigger actions (Pull, Push, Diff, Add).`,
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := getConfig(cmd)
		if err != nil {
			fmt.Println(styleErr.Render("Error: " + err.Error()))
			os.Exit(1)
		}

		if err := validateWikiDir(cfg.WikiDir); err != nil {
			fmt.Println(styleErr.Render(err.Error()))
			os.Exit(1)
		}

		fmt.Println(styleInfo.Render("Scanning workspace..."))
		items, err := ScanAll(cfg)
		if err != nil {
			fmt.Println(styleErr.Render("Scan failed: " + err.Error()))
			os.Exit(1)
		}

		// Main Loop
		for {
			// Build List for Selection
			var options []huh.Option[*FileItem]
			for i := range items {
				item := &items[i]
				// Filter logic (target version)
				label := fmt.Sprintf("%-10s %s", item.Status, item.RelPath)

				// Version Check Highlight
				if checkTargetVersion != "" && item.Status == "Synced" {
					// Check if version is present
					var fmMap map[string]interface{}
					if err := yaml.Unmarshal([]byte(item.LocalContent), &fmMap); err != nil {
						// Handle error or ignore if not valid YAML?
						// For dashboard, maybe just skipping is fine or logging debug?
					}
					hasVersion := false
					if versions, ok := fmMap["approved_versions"].([]interface{}); ok {
						for _, v := range versions {
							if fmt.Sprintf("%v", v) == checkTargetVersion {
								hasVersion = true
								break
							}
						}
					}
					if !hasVersion {
						label = fmt.Sprintf("%-10s %s (Missing v%s)", "⚠️ Synced", item.RelPath, checkTargetVersion)
					}
				}

				options = append(options, huh.NewOption(label, item))
			}

			if len(options) == 0 {
				fmt.Println("No files found.")
				break
			}

			var selected *FileItem
			form := huh.NewForm(
				huh.NewGroup(
					huh.NewSelect[*FileItem]().
						Title("Wiki-Sync Explorer").
						Description("Select a file to act on (Ctrl+C to exit)").
						Options(options...).
						Value(&selected).
						Height(20), // List height
				),
			)

			err := form.Run()
			if err != nil {
				// User quit
				break
			}

			// Action Menu for Selected File
			handleSelection(cfg, *selected)

			// Re-Scan after action?
			// Ideally yes, to update status.
			newItems, err := ScanAll(cfg)
			if err == nil {
				items = newItems
			}
		}
	},
}

func handleSelection(cfg Config, item FileItem) {
	// Determine valid actions based on Status
	var actions []huh.Option[string]

	// Common Actions
	actions = append(actions, huh.NewOption("View Local Content", "view_local"))

	switch item.Status {
	case "New":
		actions = append(actions, huh.NewOption("Add to Wiki", "add"))
	case "Ahead":
		actions = append(actions, huh.NewOption("Push to Wiki", "push"))
		actions = append(actions, huh.NewOption("View Diff", "diff"))
	case "Behind":
		actions = append(actions, huh.NewOption("Pull from Wiki", "pull"))
		actions = append(actions, huh.NewOption("View Diff", "diff"))
	case "Conflict":
		actions = append(actions, huh.NewOption("View Diff", "diff"))
		actions = append(actions, huh.NewOption("Force Pull (Overwrite Local)", "pull_force"))
		actions = append(actions, huh.NewOption("Force Push (Overwrite Wiki)", "push_force"))
	case "Synced":
		actions = append(actions, huh.NewOption("View Wiki Content", "view_wiki"))
		// Version Promote
		if checkTargetVersion != "" {
			actions = append(actions, huh.NewOption(fmt.Sprintf("Promote to v%s", checkTargetVersion), "promote"))
		}
	case "Runaway":
		actions = append(actions, huh.NewOption("Import to Docs", "pull"))
	}

	actions = append(actions, huh.NewOption("Cancel", "cancel"))

	var action string
	if err := huh.NewSelect[string]().
		Title("Action: " + item.RelPath).
		Options(actions...).
		Value(&action).
		Run(); err != nil {
		return
	}

	switch action {
	case "view_local":
		fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("250")).Render(item.LocalContent))
		waitForKey()
	case "diff":
		// Show diff
		fmt.Println("Diff not implemented yet in dashboard view.")
		waitForKey()
	case "add":
		fmt.Println("Triggering Add...")
		fmt.Printf("Run: wiki-sync add %s\n", item.LocalPath)
		waitForKey()
	case "push":
		fmt.Println("Triggering Push...")
		fmt.Printf("Run: wiki-sync push %s\n", item.LocalPath)
		waitForKey()
	case "pull":
		fmt.Println("Triggering Pull...")
		fmt.Printf("Run: wiki-sync pull %s\n", item.RelPath)
		waitForKey()
	case "promote":
		promoted, err := addVersion(item.LocalContent, checkTargetVersion)
		if err != nil {
			fmt.Println(styleErr.Render("Failed to promote: " + err.Error()))
		} else {
			if err := os.WriteFile(item.LocalPath, []byte(promoted), 0644); err != nil {
				fmt.Println(styleErr.Render("Failed to write file: " + err.Error()))
			} else {
				fmt.Println(styleSuccess.Render("Promoted to " + checkTargetVersion))
			}
		}
		waitForKey()
	}
}

func addVersion(content, version string) (string, error) {
	// 1. Identify FM block
	re := regexp.MustCompile(`(?s)^---\r?\n(.*?)\r?\n---`)
	match := re.FindStringSubmatchIndex(content)

	var fmMap map[string]interface{}
	var body string

	if match == nil {
		// No FM, create new
		fmMap = make(map[string]interface{})
		body = content
	} else {
		// Parse existing
		fmStr := content[match[2]:match[3]]
		if err := yaml.Unmarshal([]byte(fmStr), &fmMap); err != nil {
			return "", err
		}
		body = content[match[1]:] // content after ---
		if strings.HasPrefix(body, "\n") {
			body = strings.TrimPrefix(body, "\n")
		} else if strings.HasPrefix(body, "\r\n") {
			body = strings.TrimPrefix(body, "\r\n")
		}
	}

	// 2. Update approved_versions
	var versions []interface{}
	if v, ok := fmMap["approved_versions"]; ok {
		if vList, ok := v.([]interface{}); ok {
			versions = vList
		}
	}

	// Check if exists
	exists := false
	for _, v := range versions {
		if fmt.Sprintf("%v", v) == version {
			exists = true
			break
		}
	}

	if !exists {
		versions = append(versions, version)
		fmMap["approved_versions"] = versions

		// 3. Update effectiveDate
		fmMap["effectiveDate"] = time.Now().Format("2006-01-02")
	} else {
		return content, nil // No change
	}

	// 4. Marshal
	newFMBytes, err := yaml.Marshal(fmMap)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("---\n%s---\n\n%s", string(newFMBytes), body), nil
}

func waitForKey() {
	fmt.Println("Press Enter to continue...")
	_, _ = bufio.NewReader(os.Stdin).ReadBytes('\n')
}

func init() {
	checkCmd.Flags().StringVar(&checkTargetVersion, "target-version", "", "Highlight files missing this version")
	rootCmd.AddCommand(checkCmd)
}
