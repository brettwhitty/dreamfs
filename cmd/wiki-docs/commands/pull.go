package commands

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	pullForce     bool
	pullCheck     bool
	pullDryRun    bool
	pullURL       string
	targetVersion string
	keepAttrs     []string
	docStyle      = lipgloss.NewStyle().Margin(1, 2)
)

var pullCmd = &cobra.Command{
	Use:   "pull",
	Short: "Sync wiki to local docs (Wiki -> Repo)",
	Long: `Pulls changes from the wiki back to the local docs folder.
Supports local wiki clone (default) or HTTP fetching via --url or auto-detected git remote.`,
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := getConfig(cmd)
		if err != nil {
			fmt.Println(styleErr.Render("Error getting config: " + err.Error()))
			os.Exit(1)
		}

		// 0. Resolve Mode (Local vs URL)
		useURL := pullURL != ""
		if !useURL && cfg.WikiDir == "" {
			// Logic to auto-detect provided by Environment or default
		}

		_, errWiki := os.Stat(cfg.WikiDir)
		if os.IsNotExist(errWiki) || pullURL != "" {
			if pullURL == "" {
				remote, err := getGitRemoteURL(cfg.RepoRoot)
				if err == nil && remote != "" {
					pullURL = deriveWikiURLFromRemote(remote)
					fmt.Println(styleInfo.Render("Auto-detected Wiki URL: " + pullURL))
					useURL = true
				}
			} else {
				useURL = true
			}
		}

		if !useURL {
			if err := validateWikiDir(cfg.WikiDir); err != nil {
				fmt.Println(styleErr.Render(err.Error()))
				os.Exit(1)
			}
		}

		// 1. Discovery Phase
		var items []FileItem
		if useURL {
			fmt.Println(styleInfo.Render("Fetching from " + pullURL))
			items, err = discoverFilesURL(cfg, pullURL)
		} else {
			items, err = discoverFilesLocal(cfg)
		}

		if err != nil {
			fmt.Println(styleErr.Render("Discovery failed: " + err.Error()))
			os.Exit(1)
		}

		if len(items) == 0 {
			fmt.Println(styleInfo.Render("No relevant files found."))
			return
		}

		// 2. Filter out "Same" files AND check Version
		var changedItems []FileItem
		for _, item := range items {
			// Version Filtering Logic
			if targetVersion != "" {
				fm, _ := parseFrontmatter(item.WikiContent)
				if approved, ok := fm["approved_versions"]; ok {
					approvedStr, isStr := approved.(string)
					if !isStr {
						continue
					}
					match := false
					if approvedStr == "*" {
						match = true
					} else if approvedStr == targetVersion {
						match = true
					} else if strings.HasSuffix(approvedStr, "*") {
						prefix := strings.TrimSuffix(approvedStr, "*")
						if strings.HasPrefix(targetVersion, prefix) {
							match = true
						}
					}
					if !match {
						continue
					}
				} else {
					continue
				}
			}

			if item.Status != "Same" {
				changedItems = append(changedItems, item)
			}
		}

		if len(changedItems) == 0 {
			fmt.Println(styleSuccess.Render("Everything is up to date (after filtering)!"))
			return
		}

		// CHECK MODE
		if pullCheck {
			fmt.Println(styleErr.Render(fmt.Sprintf("Found %d changed files.", len(changedItems))))
			os.Exit(1)
		}

		// DRY RUN
		if pullDryRun {
			// Print Table
			fmt.Println(docStyle.Render(fmt.Sprintf("Found %d changed files:", len(changedItems))))

			// Headers
			fmt.Printf("%s  %-50s  %s\n", "STAT", "FILE", "DETAILS")
			fmt.Println(strings.Repeat("-", 80))

			for _, item := range changedItems {
				var st lipgloss.Style
				icon := " "

				switch item.ChangeType {
				case "New":
					st = styleNew
					icon = "+"
				case "Content":
					st = styleContent
					icon = "C"
				case "Meta":
					st = styleMeta
					icon = "M"
				case "Mixed":
					st = styleMixed
					icon = "*"
				default:
					st = lipgloss.NewStyle()
				}

				metaDetails := ""
				if len(item.MetaDiff) > 0 {
					metaDetails = fmt.Sprintf("[%s]", strings.Join(item.MetaDiff, ", "))
				}

				// RelPath truncation if needed
				path := item.RelPath
				if len(path) > 48 {
					path = "..." + path[len(path)-45:]
				}

				fmt.Printf("%s  %-50s  %s %s\n",
					st.Render(icon),
					path,
					st.Render(item.ChangeType),
					styleMeta.Render(metaDetails))
			}
			return
		}

		// 3. Interaction
		var selected []FileItem
		if pullForce {
			selected = changedItems
		} else {
			selected = runInteractive(changedItems)
		}

		// 4. Execution
		if len(selected) > 0 {
			fmt.Println(styleInfo.Render("Updating files..."))
			for _, item := range selected {
				// Reconstruct Content
				cleanBody := stripFrontmatter(item.WikiContent)
				finalContent := cleanBody

				if len(keepAttrs) > 0 {
					fm, _ := parseFrontmatter(item.WikiContent)
					newFM := make(map[string]interface{})
					for _, key := range keepAttrs {
						if val, ok := fm[key]; ok {
							newFM[key] = val
						}
					}
					// Check if effectiveDate is in keepAttrs
					found := false
					for _, attr := range keepAttrs {
						if attr == "effectiveDate" {
							found = true
							break
						}
					}
					if found {
						// Use current date
						newFM["effectiveDate"] = time.Now().Format("2006-01-02")
					}

					// Update State (Sync Metadata)
					// We calculate checksum of the CLEAN body we are about to save.
					checksum := CalculateChecksum(cleanBody)

					// Load State (SAFE: loading inside loop for now to ensure correctness)
					state, _ := LoadState()
					if state != nil && cfg.WikiDir != "" {
						// Try to get SHA from local Wiki Repo
						sha, _ := getFileGitRevision(cfg.WikiDir, item.WikiPath)
						if sha != "" {
							state.Update(item.RelPath, sha, checksum)
							if err := state.Save(); err != nil {
								// Log error but don't stop sync?
								fmt.Printf("Warning: Failed to save state: %v\n", err)
							}
						}
					}

					if len(newFM) > 0 {
						yamlBytes, err := yaml.Marshal(newFM)
						if err == nil {
							finalContent = fmt.Sprintf("---\n%s---\n\n%s", string(yamlBytes), cleanBody)
						}
					}
				}

				// Ensure dir
				if err := os.MkdirAll(filepath.Dir(item.LocalPath), 0755); err != nil {
					fmt.Printf("  %s %s: %v\n", styleErr.Render("X"), item.RelPath, err)
					continue
				}

				if err := os.WriteFile(item.LocalPath, []byte(finalContent), 0644); err != nil {
					fmt.Printf("  %s %s: %v\n", styleErr.Render("X"), item.RelPath, err)
				} else {
					fmt.Printf("  %s %s\n", styleSuccess.Render("✓"), item.RelPath)
				}
			}
		} else {
			fmt.Println("No files updated.")
		}
	},
}

func init() {
	pullCmd.Flags().BoolVarP(&pullForce, "force", "f", false, "Confirm all changes without prompt")
	pullCmd.Flags().BoolVar(&pullCheck, "check", false, "Exit with code 1 if changes are detected")
	pullCmd.Flags().BoolVar(&pullDryRun, "dry-run", false, "Print changes without applying them")
	pullCmd.Flags().StringVar(&pullURL, "url", os.Getenv("WIKI_URL"), "Git Wiki URL to fetch from (env: WIKI_URL)")
	pullCmd.Flags().StringVar(&targetVersion, "target-version", "", "Filter files by 'approved_versions' frontmatter")

	// Support comma-separated env var for default
	defaultKeep := []string{
		// GitHub Standard
		"title", "shortTitle", "intro", "versions", "redirect_from", "permissions", "product", "layout",
		"children", "childGroups", "featuredLinks", "showMiniToc", "changelog", "learningTracks", "type", "topics",
		"effectiveDate", "communityRedirect",
		// Internal
		"release_path", "approved_versions", "review_status", "authority", "generated_on", "origin_persona", "origin_session", "intent",
		// Integrity
		"readonly",
	}
	if envKeep := os.Getenv("WIKI_KEEP_ATTRS"); envKeep != "" {
		defaultKeep = strings.Split(envKeep, ",")
	}
	pullCmd.Flags().StringSliceVar(&keepAttrs, "keep-attrs", defaultKeep, "List of frontmatter attributes to preserve")

	rootCmd.AddCommand(pullCmd)
}

func discoverFilesLocal(cfg Config) ([]FileItem, error) {
	var items []FileItem
	files, err := os.ReadDir(cfg.WikiDir)
	if err != nil {
		return nil, err
	}

	for _, f := range files {
		if f.IsDir() || !strings.HasPrefix(f.Name(), WikiPrefixBase) || filepath.Ext(f.Name()) != ".md" {
			continue
		}

		wikiPath := filepath.Join(cfg.WikiDir, f.Name())
		contentBytes, err := os.ReadFile(wikiPath)
		if err != nil {
			continue
		}
		content := string(contentBytes)

		fm, _ := parseFrontmatter(content)

		rev, _ := getFileGitRevision(cfg.WikiDir, f.Name())
		if rev != "" {
			fm["wiki_revision"] = rev
		}

		// Reverse Map
		trimmed := strings.TrimPrefix(f.Name(), WikiPrefixBase)
		relPath := strings.ReplaceAll(trimmed, "~", string(filepath.Separator))
		localPath := filepath.Join(cfg.RepoRoot, relPath)

		// Filter by Sources
		allowed := false
		for _, source := range cfg.Sources {
			// Check if relPath starts with source
			// Clean paths to be safe
			cleanSource := filepath.Clean(source)
			cleanRel := filepath.Clean(relPath)
			if strings.HasPrefix(cleanRel, cleanSource) {
				allowed = true
				break
			}
		}

		if !allowed {
			// Skip files not in configured sources
			continue
		}

		status := "Same"
		changeType := ""
		localContent := ""
		var metaDiff []string

		if info, err := os.Stat(localPath); os.IsNotExist(err) {
			status = "New"
			changeType = "New"
		} else if !info.IsDir() {
			bytesLocal, _ := os.ReadFile(localPath)
			localContent = string(bytesLocal)

			cleanWiki := stripFrontmatter(content)
			cleanLocal := stripFrontmatter(localContent)

			bodyChanged := cleanWiki != cleanLocal

			localFM, _ := parseFrontmatter(localContent)
			expectedFM := make(map[string]interface{})
			if len(keepAttrs) > 0 {
				for _, key := range keepAttrs {
					if val, ok := fm[key]; ok {
						expectedFM[key] = val
					}
				}
				expectedFM["effectiveDate"] = time.Now().Format("2006-01-02")
			}

			metaChanged := false
			for k, v := range expectedFM {
				localV, ok := localFM[k]
				if !ok || fmt.Sprintf("%v", v) != fmt.Sprintf("%v", localV) {
					metaChanged = true
					metaDiff = append(metaDiff, k)
				}
			}

			if bodyChanged && metaChanged {
				status = "Changed"
				changeType = "Mixed"
			} else if bodyChanged {
				status = "Changed"
				changeType = "Content"
			} else if metaChanged {
				status = "Changed"
				changeType = "Meta"
			}
		}

		items = append(items, FileItem{
			WikiPath:     f.Name(),
			LocalPath:    localPath,
			RelPath:      relPath,
			WikiContent:  content,
			LocalContent: localContent,
			Status:       status,
			ChangeType:   changeType,
			MetaDiff:     metaDiff,
		})
	}
	return items, nil
}

func discoverFilesURL(cfg Config, baseURL string) ([]FileItem, error) {
	if !strings.HasSuffix(baseURL, "/") {
		baseURL += "/"
	}

	var items []FileItem

	for _, source := range cfg.Sources {
		absSourceDir := filepath.Join(cfg.RepoRoot, source)
		if _, err := os.Stat(absSourceDir); os.IsNotExist(err) {
			continue
		}

		err := filepath.Walk(absSourceDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || filepath.Ext(path) != ".md" {
				return nil
			}

			relPath, _ := filepath.Rel(cfg.RepoRoot, path)
			relPath = filepath.ToSlash(relPath)

			flattened := strings.ReplaceAll(relPath, "/", "~")
			wikiFilename := WikiPrefixBase + flattened
			url := baseURL + wikiFilename

			resp, err := http.Get(url)
			if err != nil {
				return fmt.Errorf("http error: %w", err)
			}
			defer resp.Body.Close()

			localContentBytes, _ := os.ReadFile(path)
			localContent := string(localContentBytes)

			if resp.StatusCode == 404 {
				return nil
			} else if resp.StatusCode != 200 {
				return fmt.Errorf("unexpected status code %d for %s", resp.StatusCode, url)
			}

			bodyBytes, _ := io.ReadAll(resp.Body)
			wikiContent := string(bodyBytes)

			status := "Same"
			changeType := ""

			cleanWiki := stripFrontmatter(wikiContent)
			cleanLocal := stripFrontmatter(localContent)

			bodyChanged := cleanWiki != cleanLocal

			localFM, _ := parseFrontmatter(localContent)
			wikiFM, _ := parseFrontmatter(wikiContent)

			expectedFM := make(map[string]interface{})
			if len(keepAttrs) > 0 {
				for _, key := range keepAttrs {
					if val, ok := wikiFM[key]; ok {
						expectedFM[key] = val
					}
				}
				expectedFM["effectiveDate"] = time.Now().Format("2006-01-02")
			}

			metaChanged := false
			var metaDiff []string

			for k, v := range expectedFM {
				localV, ok := localFM[k]
				if !ok || fmt.Sprintf("%v", v) != fmt.Sprintf("%v", localV) {
					metaChanged = true
					metaDiff = append(metaDiff, k)
				}
			}

			if bodyChanged && metaChanged {
				status = "Changed"
				changeType = "Mixed"
			} else if bodyChanged {
				status = "Changed"
				changeType = "Content"
			} else if metaChanged {
				status = "Changed"
				changeType = "Meta"
			}

			items = append(items, FileItem{
				WikiPath:     url,
				LocalPath:    path,
				RelPath:      relPath,
				WikiContent:  wikiContent,
				LocalContent: localContent,
				Status:       status,
				ChangeType:   changeType,
				MetaDiff:     metaDiff,
			})

			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	return items, nil
}

// Interactive Runner with HUH
func runInteractive(files []FileItem) []FileItem {
	var selectedPaths []string

	options := make([]huh.Option[string], len(files))
	for i, f := range files {
		options[i] = huh.NewOption(fmt.Sprintf("%s (%s)", f.RelPath, f.Status), f.RelPath).Selected(f.Selected)
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Select files to sync:").
				Description("Press space to select/deselect, enter to confirm.").
				Options(options...).
				Value(&selectedPaths),
		),
	).WithTheme(huh.ThemeBase())

	if err := form.Run(); err != nil {
		fmt.Println(styleErr.Render("Interactive selection cancelled: " + err.Error()))
		return nil
	}

	selectedItems := make([]FileItem, 0, len(selectedPaths))
	selectedPathMap := make(map[string]struct{})
	for _, p := range selectedPaths {
		selectedPathMap[p] = struct{}{}
	}

	for _, f := range files {
		if _, ok := selectedPathMap[f.RelPath]; ok {
			selectedItems = append(selectedItems, f)
		}
	}

	return selectedItems
}
