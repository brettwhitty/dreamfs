package commands

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
)

var addCmd = &cobra.Command{
	Use:   "add [file]",
	Short: "Add new files to wiki",
	Long: `Adds new files to the wiki. 
Only files that do not exist in the wiki will be processed.
Enforces branch protection and manual review.`,
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

		fmt.Println(styleInfo.Render("Scanning for new files..."))

		items, err := ScanAll(cfg)
		if err != nil {
			fmt.Println(styleErr.Render("Discovery failed: " + err.Error()))
			os.Exit(1)
		}

		// Filter for NEW files only
		var newFiles []FileItem
		for _, item := range items {
			// If target specified, strict filter
			if targetFile != "" {
				normTarget := filepath.ToSlash(targetFile)
				if item.RelPath != normTarget && !strings.HasSuffix(item.RelPath, normTarget) {
					continue
				}
			}

			if item.Status == "New" {
				newFiles = append(newFiles, item)
			} else if targetFile != "" {
				fmt.Println(styleInfo.Render(fmt.Sprintf("File '%s' already exists in wiki (as %s). Use 'wiki-sync push' to update.", item.RelPath, item.WikiPath)))
			}
		}

		// Handle Template Creation for non-existent target
		if len(newFiles) == 0 && targetFile != "" {
			// Check if it really doesn't exist locally
			if _, err := os.Stat(targetFile); os.IsNotExist(err) {
				// It's a brand new file request.
				// 1. Load Templates
				templates, err := LoadTemplates(cfg.WikiDir)
				if err != nil {
					fmt.Println(styleErr.Render("Failed to load templates: " + err.Error()))
				}

				if len(templates) > 0 {
					var selectedTemplate string
					var content string

					// Try inherited template first
					tName, tContent := FindInheritedTemplate(targetFile, cfg.WikiDir)
					if tName != "" {
						selectedTemplate = tName
						content = tContent
						fmt.Printf(styleInfo.Render("Found inherited template: %s")+"\n", tName)
					} else {
						var options []huh.Option[string]
						for _, t := range templates {
							options = append(options, huh.NewOption(t.Name, t.Name))
						}
						options = append(options, huh.NewOption("None (Empty)", ""))

						err := huh.NewSelect[string]().
							Title("Select a template for " + targetFile).
							Options(options...).
							Value(&selectedTemplate).
							Run()

						if err == nil && selectedTemplate != "" {
							for _, t := range templates {
								if t.Name == selectedTemplate {
									content = t.Content
									break
								}
							}
						}
					}

					if selectedTemplate != "" {
						// Write to local file (create dirs if needed)
						if err := os.MkdirAll(filepath.Dir(targetFile), 0755); err == nil {
							os.WriteFile(targetFile, []byte(content), 0644)
							fmt.Println(styleSuccess.Render("Created " + targetFile + " from template " + selectedTemplate))

							// Re-scan to pick it up
							items, _ = ScanAll(cfg)
							for _, item := range items {
								normTarget := filepath.ToSlash(targetFile)
								if (item.RelPath == normTarget || strings.HasSuffix(item.RelPath, normTarget)) && item.Status == "New" {
									newFiles = append(newFiles, item)
								}
							}
						}
					}
				}
			}
		}

		if len(newFiles) == 0 {
			fmt.Println(styleSuccess.Render("No new files to add."))
			return
		}

		// 3. Selection
		var selected []FileItem
		if targetFile != "" {
			selected = newFiles
		} else {
			selected = runInteractive(newFiles)
		}

		if len(selected) == 0 {
			fmt.Println("No files selected.")
			return
		}

		// 4. Processing
		for _, item := range selected {
			fmt.Println(strings.Repeat("=", 60))
			fmt.Printf("Adding: %s\n", styleNew.Render(item.RelPath))

			// Double check existence (Race condition)
			if _, err := os.Stat(filepath.Join(cfg.WikiDir, item.WikiPath)); err == nil {
				fmt.Println(styleErr.Render("⛔ ERROR: File already exists in wiki! Use 'wiki-sync push'."))
				continue
			}

			localContent := item.LocalContent

			// Check for Missing or Invalid Frontmatter
			_, hasFM := parseFrontmatter(localContent)
			if !hasFM {
				fmt.Println(styleInfo.Render("⚠️  Missing or invalid YAML frontmatter detected."))

				confirmInject := false

				// Try inherited template
				tName, tContent := FindInheritedTemplate(item.RelPath, cfg.WikiDir)
				if tName != "" {
					fmt.Printf(styleInfo.Render("Found inherited template: %s")+"\n", tName)
					huh.NewConfirm().
						Title("Inject inherited template?").
						Value(&confirmInject).
						Run()
					if confirmInject {
						localContent = tContent + "\n" + localContent
					}
				} else {
					huh.NewConfirm().
						Title("Inject frontmatter from template?").
						Value(&confirmInject).
						Run()

					if confirmInject {
						// Load Templates
						templates, _ := LoadTemplates(cfg.WikiDir)
						var selectedTemplate string

						if len(templates) > 0 {
							var options []huh.Option[string]
							for _, t := range templates {
								options = append(options, huh.NewOption(t.Name, t.Name))
							}

							huh.NewSelect[string]().
								Title("Select template").
								Options(options...).
								Value(&selectedTemplate).
								Run()
						}

						var itemsToInject string
						if selectedTemplate != "" {
							for _, t := range templates {
								if t.Name == selectedTemplate {
									itemsToInject = t.Content
									break
								}
							}
						} else {
							// Generic default
							itemsToInject = "---\ntitle: " + filepath.Base(item.RelPath) + "\n---\n\n"
						}

						// Prepend
						localContent = itemsToInject + "\n" + localContent
					}
				}
			}

			// Validation
			schemaPath := filepath.Join(cfg.WikiDir, ".schemas", "frontmatter.yaml")
			// Fallback to json if yaml not found? Or config?
			// For now, let's try both or just check existence.
			if _, err := os.Stat(schemaPath); os.IsNotExist(err) {
				schemaPath = filepath.Join(cfg.WikiDir, ".schemas", "frontmatter.json")
			}

			if err := ValidateFrontmatter(localContent, schemaPath); err != nil {
				printFatal("Schema Validation Failed", err, "Correct the frontmatter to match the schema defined in .schemas/frontmatter.yaml")
			}

			// B. Editor Review
			tmpFile, err := os.CreateTemp("", "wiki-add-*.md")
			if err != nil {
				fmt.Println(styleErr.Render("Temp file error: " + err.Error()))
				continue
			}
			tmpPath := tmpFile.Name()
			if err := os.WriteFile(tmpPath, []byte(localContent), 0644); err != nil {
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
				Title(fmt.Sprintf("Add %s to wiki?", item.RelPath)).
				Value(&confirm).
				Run()

			if err != nil || !confirm {
				fmt.Println("Skipped.")
				continue
			}

			// D. Write
			destPath := filepath.Join(cfg.WikiDir, item.WikiPath)
			if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
				fmt.Println(styleErr.Render("Mkdir failed: " + err.Error()))
				continue
			}

			if err := os.WriteFile(destPath, []byte(editedContent), 0644); err != nil {
				fmt.Println(styleErr.Render("Write failed: " + err.Error()))
			} else {
				fmt.Println(styleSuccess.Render("✓ Added"))
				// Note: We do NOT update state.go here because we don't have a revision/commit yet.
				// The next 'pull' will capture the state after the user commits and pushes the wiki repo.
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(addCmd)
}
