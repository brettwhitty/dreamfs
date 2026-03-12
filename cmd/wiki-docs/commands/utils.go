package commands

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

const WikiPrefixBase = "src_docs~"
const LegacyWikiPrefixBase = "repo-root~"
const TemplatePrefixBase = "src_tmpl~"
const DefaultSource = "."

// ToWikiPath converts a local relative path to its flattened wiki filename.
// It replaces "/" with "~" and "-" with "_" for compatibility.
func ToWikiPath(relPath string, prefix string) string {
	ext := filepath.Ext(relPath)
	name := strings.TrimSuffix(relPath, ext)

	// Replace separators
	flattened := strings.ReplaceAll(name, "/", "~")
	// Replace hyphens with underscores
	flattened = strings.ReplaceAll(flattened, "-", "_")

	return prefix + flattened + ext
}

// Config holds the derived configuration
type Config struct {
	RepoRoot string
	Sources  []string // Relative paths from RepoRoot, e.g. ["docs", ".gemini/skills"]
	WikiDir  string
}

// FileStatus defines the synchronization status of a file
type FileStatus string

const (
	StatusCurrent   FileStatus = "Current"
	StatusModified  FileStatus = "Modified"
	StatusUntracked FileStatus = "Untracked"
	StatusObsoleted FileStatus = "Obsoleted" // Replaces Orphan: Source no longer approved/present
	StatusOutdated  FileStatus = "Outdated"  // Replaces Legacy: Staged Version < Approved Version
	StatusIgnored   FileStatus = "Ignored"
)

// FileItem represents a file to be synced
// FileItem represents a document in the staging pipeline.
// It bridges the gap between the authoritative Wiki source and the local Repository staging.
type FileItem struct {
	WikiPath     string // Absolute URL or path in Wiki repository
	LocalPath    string // Absolute path in local repository
	RelPath      string // Repository-relative path
	WikiContent  string // Raw content from Wiki
	LocalContent string // Raw content from Local Repository

	// Status flags for the Auditor
	Status     FileStatus // Current, Modified, Outdated, Obsoleted, etc.
	ChangeType string     // Classification: Content, Meta, Mixed

	// Metadata Shadow
	Version      string                 // Extracted version identifier
	Approved     string                 // Comma-separated approved versions
	HasValidYAML bool                   // Correctness of frontmatter structure
	Meta         map[string]interface{} // Full metadata map (Merged Shadow)

	// TUI Presentation
	Selected      bool
	Readonly      bool                   // "readonly: true" in frontmatter
	IsIgnored     bool                   // Matched by .gitignore or .geminiignore
	TemplateAttrs map[string]interface{} // Attributes from the inherited template
	InfoLayout    string                 // Custom display template from "## wiki-docs.display"
	MetaDiff      []string               // List of modified metadata fields
}

// Styles
var (
	styleInfo    = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))  // Blue
	styleSuccess = lipgloss.NewStyle().Foreground(lipgloss.Color("46"))  // Green
	styleErr     = lipgloss.NewStyle().Foreground(lipgloss.Color("196")) // Red
	styleNew     = lipgloss.NewStyle().Foreground(lipgloss.Color("226")) // Yellow
	styleContent = lipgloss.NewStyle().Foreground(lipgloss.Color("33"))  // Blueish
	styleMeta    = lipgloss.NewStyle().Foreground(lipgloss.Color("201")) // Magenta
	styleMixed   = lipgloss.NewStyle().Foreground(lipgloss.Color("208")) // Orange
)

func getGitRemoteURL(dir string) (string, error) {
	cmd := exec.Command("git", "-C", dir, "remote", "get-url", "origin")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func deriveWikiURLFromRemote(remote string) string {
	return strings.TrimSuffix(remote, ".git")
}

// GetWikiURL determines the Wiki URL from Env or Git Remote
func GetWikiURL(cfg Config) string {
	// 1. Environment Variable
	if url := os.Getenv("WIKI_URL"); url != "" {
		return strings.TrimRight(url, "/")
	}

	// 2. Git Remote (Fallback)
	remote, err := getGitRemoteURL(cfg.RepoRoot)
	if err == nil && remote != "" {
		return deriveWikiURLFromRemote(remote) + ".wiki"
	}

	return ""
}

func getConfig(cmd *cobra.Command) (Config, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return Config{}, err
	}

	wikiDir, _ := cmd.Flags().GetString("wiki-path")
	if !filepath.IsAbs(wikiDir) {
		wikiDir = filepath.Join(cwd, wikiDir)
	}

	// Default Config
	cfg := Config{
		RepoRoot: cwd,
		Sources:  []string{DefaultSource},
		WikiDir:  wikiDir,
	}

	// Try to load config file
	configPath := filepath.Join(cwd, ".config", "wiki-docs", "config.yaml")
	if _, err := os.Stat(configPath); err == nil {
		data, err := os.ReadFile(configPath)
		if err == nil {
			var parsed struct {
				Sources []string `yaml:"sources"`
			}
			if err := yaml.Unmarshal(data, &parsed); err == nil {
				if len(parsed.Sources) > 0 {
					cfg.Sources = parsed.Sources
				}
			}
		}
	}

	return cfg, nil
}

func validateWikiDir(path string) error {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return fmt.Errorf("wiki directory not found at '%s'.\nPlease clone the wiki repo to this location or specify --wiki-path", path)
	}
	if !info.IsDir() {
		return fmt.Errorf("path '%s' is not a directory", path)
	}
	return nil
}

// isGeminiIgnored checks if a path matches any pattern in .geminiignore
func isGeminiIgnored(repoRoot, relPath string) bool {
	ignorePath := filepath.Join(repoRoot, ".geminiignore")
	data, err := os.ReadFile(ignorePath)
	if err != nil {
		return false
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Simple prefix or contains check for now (mirrors .gitignore basic usage)
		if strings.HasPrefix(relPath, line) {
			return true
		}
	}
	return false
}

// printFatal prints a styled error message and exits with status 1
func printFatal(title string, err error, suggestions ...string) {
	fmt.Println()
	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("196")).
		Padding(1, 2).
		MarginBottom(1)

	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("196")).
		Bold(true).
		MarginBottom(1)

	body := titleStyle.Render("❌ " + title)
	if err != nil {
		body += "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Render(err.Error())
	}

	if len(suggestions) > 0 {
		body += "\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Italic(true).Render("Suggestions:")
		for _, s := range suggestions {
			body += "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render("• "+s)
		}
	}

	fmt.Println(border.Render(body))
	os.Exit(1)
}

func assertEditorSet() error {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		// Try to fallback to git editor config?
		// But explicit is better.
		return fmt.Errorf("EDITOR environment variable is missing")
	}
	return nil
}

func checkWikiBranch(wikiDir string) error {
	// Check if branch is main or master
	cmd := exec.Command("git", "-C", wikiDir, "branch", "--show-current")
	out, err := cmd.Output()
	if err != nil {
		// Possibly detached head or not a git repo (already validated dir exists)
		// Let's warn but maybe allow if strict mode not set?
		// For now, fail safe.
		return fmt.Errorf("failed to check wiki git branch: %w", err)
	}
	branch := strings.TrimSpace(string(out))
	if branch == "main" || branch == "master" {
		return fmt.Errorf("protected branch '%s' detected. Please checkout a feature branch in the wiki repo before pushing updates.", branch)
	}
	return nil
}

// Helper: Strip Frontmatter
func stripFrontmatter(content string) string {
	if !strings.HasPrefix(content, "---\n") && !strings.HasPrefix(content, "---\r\n") {
		return content
	}
	parts := strings.SplitN(content, "---", 3)
	if len(parts) == 3 {
		return strings.TrimSpace(parts[2])
	}
	return content
}

// Helper: Parse Frontmatter and validate YAML
func parseFrontmatter(content string) (map[string]interface{}, bool) {
	fm := make(map[string]interface{})
	if !strings.HasPrefix(content, "---\n") && !strings.HasPrefix(content, "---\r\n") {
		return fm, false // No frontmatter
	}
	parts := strings.SplitN(content, "---", 3)
	if len(parts) == 3 {
		if err := yaml.Unmarshal([]byte(parts[1]), &fm); err != nil {
			return nil, false // Invalid YAML
		}
		return fm, true // Valid YAML
	}
	return fm, false // Incomplete frontmatter
}

// Helper: Checksum
func CalculateChecksum(content string) string {
	hash := sha256.Sum256([]byte(content))
	return hex.EncodeToString(hash[:])
}

// Get Git Revision of a file in Wiki Repo
func getFileGitRevision(repoPath, relPath string) (string, error) {
	cmd := exec.Command("git", "-C", repoPath, "log", "-n", "1", "--pretty=format:%H", "--", relPath)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// ScanAll executes a comprehensive audit of the project workspace.
// It iterates through configured source directories and cross-references them
// with the Gitea Wiki, utilizing .config/wiki-docs/state.json for precise version attribution.
func ScanAll(cfg Config) ([]FileItem, error) {
	var items []FileItem

	// 1. Build Wiki Map: flattened filename -> actual relative filename (Definitive state)
	// We walk the wiki directory to discover all staged candidates, excluding internal metadata.
	wikiMap := make(map[string]string)
	filepath.Walk(cfg.WikiDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			name := info.Name()
			if name == ".git" || name == ".config" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(cfg.WikiDir, path)
		// Flat structure enforcement: Only files in the wiki root are candidates.
		if !strings.Contains(rel, "/") && !strings.Contains(rel, "\\") {
			wikiMap[rel] = rel
		}
		return nil
	})

	// Pre-load the staging metadata for the entire scan (Performance Optimization)
	state, _ := LoadState(cfg.RepoRoot)

	// Keep track of which local files map to which wiki entries for the "Runaway" check.
	localFiles := make(map[string]string)

	for _, source := range cfg.Sources {
		absSourceDir := filepath.Join(cfg.RepoRoot, source)
		if _, err := os.Stat(absSourceDir); os.IsNotExist(err) {
			continue
		}

		err := filepath.Walk(absSourceDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}

			// Repository Skip Logic: Exclude build artifacts and private directories
			if info.IsDir() {
				name := info.Name()
				if name == ".git" || name == ".config" || name == "node_modules" || name == "vendor" {
					return filepath.SkipDir
				}
				return nil
			}

			// Auditor Focus: Only markdown documents are considered for staging
			if filepath.Ext(path) != ".md" {
				return nil
			}

			relPathRaw, _ := filepath.Rel(cfg.RepoRoot, path)
			relPath := filepath.ToSlash(relPathRaw)

			// Git Integrity: Check if the file is ignored by repository-level configuration.
			// Ignored files are tracked but flagged for the TUI observer.
			cmdIgnore := exec.Command("git", "check-ignore", "-q", relPath)
			isIgnored := false
			if err := cmdIgnore.Run(); err == nil {
				isIgnored = true
			}

			// Check if ignored by .geminiignore (Tool-specific ignore)
			if isGeminiIgnored(cfg.RepoRoot, relPath) {
				return nil
			}

			// Calculate intended wiki name
			wikiName := ToWikiPath(relPath, WikiPrefixBase)
			localFiles[relPath] = wikiName

			// Load contents
			localContentBytes, _ := os.ReadFile(path)
			localContent := string(localContentBytes)

			var status FileStatus = StatusUntracked
			wikiContent := ""
			finalWikiPath := wikiName

			// 3. Matching logic (Wiki-First priority)
			// Combinations to check (in order of preference):
			// A. Primary: src-docs~ (with underscores)
			// B. Legacy Prfix:  repo-root~ (with underscores)
			// C. Primary-Hyphen: src-docs~ (with hyphens)
			// D. Legacy-Hyphen: repo-root~ (with hyphens)

			status = StatusUntracked // Default: Local-only, not yet in wiki
			wikiContent = ""
			finalWikiPath = ""
			found := false
			actualWikiFile := ""

			// Resolve Wiki Filename: Matches Wiki naming conventions
			// A. Check Primary Match (src-docs~ + underscores)
			if wf, ok := wikiMap[wikiName]; ok {
				actualWikiFile = wf
				status = StatusCurrent // Assume Current; verified later by hash/rev
				found = true
			} else {
				// B. Check Legacy (repo-root~ + underscores)
				// Documents staged using the older repo-root~ naming convention
				legacyName := ToWikiPath(relPath, LegacyWikiPrefixBase)
				if wf, ok := wikiMap[legacyName]; ok {
					actualWikiFile = wf
					status = StatusOutdated // Path-based deprecation is inherently Outdated
					found = true
				} else {
					// C. Check Legacy-Hyphen (src-docs~ + hyphens)
					// Handle documents using older hyphenated separators for path flattening
					hyphenatedPrimary := WikiPrefixBase + strings.ReplaceAll(strings.TrimSuffix(relPath, ".md"), "/", "~") + ".md"
					if wf, ok := wikiMap[hyphenatedPrimary]; ok {
						actualWikiFile = wf
						status = StatusOutdated
						found = true
					} else {
						// D. Check Legacy-Hyphen (repo-root~ + hyphens)
						hyphenatedLegacy := LegacyWikiPrefixBase + strings.ReplaceAll(strings.TrimSuffix(relPath, ".md"), "/", "~") + ".md"
						if wf, ok := wikiMap[hyphenatedLegacy]; ok {
							actualWikiFile = wf
							status = StatusOutdated
							found = true
						}
					}
				}
			}

			// 4. State & Integrity Check: Cross-reference local state with live Wiki
			stagedRev := ""
			stagedChecksum := ""
			if state != nil {
				if fs, ok := state.Get(relPath); ok {
					stagedRev = fs.LastRev
					stagedChecksum = fs.LastChecksum
				}
			}

			if found {
				finalWikiPath = actualWikiFile
				wikiFileAbsPath := filepath.Join(cfg.WikiDir, actualWikiFile)
				bytesWiki, _ := os.ReadFile(wikiFileAbsPath)
				wikiContent = string(bytesWiki)

				// BODY-ONLY HASH CHECK:
				// Extract the markdown body and discard YAML frontmatter to avoid
				// false-positives caused by staging-time metadata stripping.
				localBody := stripFrontmatter(localContent)
				wikiBody := stripFrontmatter(wikiContent)

				localChecksum := CalculateChecksum(localBody)
				wikiChecksum := CalculateChecksum(wikiBody)

				currentWikiRev, _ := getFileGitRevision(cfg.WikiDir, actualWikiFile)

				// Status Attribution Hierarchy:
				// Determines if the staged file is Current, Outdated (Wiki changed),
				// or Modified (Local changed).
				if localChecksum == wikiChecksum {
					// Local content matches live Wiki body perfectly.
					if stagedRev != "" && stagedRev != currentWikiRev {
						// Content matches but we are tracking a stale revision.
						status = StatusOutdated
					} else if status == StatusCurrent {
						status = StatusCurrent
					}
				} else if stagedChecksum != "" && localChecksum == stagedChecksum {
					// Local matches documented 'staged checksum' but differs from current Wiki.
					// This indicates the Wiki source has updated since the last pull.
					status = StatusOutdated
				} else if stagedChecksum != "" {
					// Local body differs from both current Wiki AND the documented staged hash.
					// This indicates local repository-level modifications.
					status = StatusModified
				} else {
					// No history, but it differs from current Wiki.
					status = StatusModified
				}
			}

			// Parse local metadata for auditing and TUI presentation.
			fm, hasValidYAML := parseFrontmatter(localContent)

			// Metadata Inheritance:
			// Populate the display map with Wiki metadata markers for transparency.
			// Values not present in local YAML are labels as "(inherited)" to signify source authority.
			displayFM := make(map[string]interface{})
			if wikiContent != "" {
				wfm, _ := parseFrontmatter(wikiContent)
				for k, v := range wfm {
					// Label as inherited if it's not in our local frontmatter
					if _, exists := fm[k]; !exists {
						displayFM[k] = fmt.Sprintf("%v (inherited)", v)
					} else {
						displayFM[k] = fm[k]
					}
				}
				// Also include any local-only metadata if they exist
				for k, v := range fm {
					if _, exists := displayFM[k]; !exists {
						displayFM[k] = v
					}
				}
			} else {
				displayFM = fm
			}

			// Check Readonly
			readonly, _ := displayFM["readonly"].(bool)

			// Get Template Attributes
			tmplName, tmplContent := FindInheritedTemplate(relPath, cfg.WikiDir)
			var tmplAttrs map[string]interface{}
			var infoLayout string

			if tmplName != "" {
				tmplAttrs, _ = parseFrontmatter(tmplContent)
				infoLayout = extractInfoLayout(tmplContent)
			}

			if isIgnored && status == StatusUntracked {
				status = StatusIgnored
			} else if isIgnored {
				// It is ignored but also found in Wiki (Current/Modified/etc).
				// We keep the sync status but mark IsIgnored = true
			}
			version, _ := displayFM["version"].(string)
			approved := ""
			if v, ok := displayFM["approved_versions"]; ok {
				switch t := v.(type) {
				case string:
					approved = t
				case []interface{}:
					var strs []string
					for _, s := range t {
						strs = append(strs, fmt.Sprint(s))
					}
					approved = strings.Join(strs, ",")
				}
			}

			items = append(items, FileItem{
				WikiPath:      finalWikiPath,
				LocalPath:     path,
				RelPath:       relPath,
				WikiContent:   wikiContent,
				LocalContent:  localContent,
				Status:        status,
				ChangeType:    string(status),
				Version:       version,
				Approved:      approved,
				HasValidYAML:  hasValidYAML,
				Meta:          displayFM,
				Readonly:      readonly,
				IsIgnored:     isIgnored,
				TemplateAttrs: tmplAttrs,
				InfoLayout:    infoLayout,
			})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	// 3. Scan Wiki for items NOT in local (Runaways)
	for base, actual := range wikiMap {
		// Does this wiki file map back to any of our identified local files?
		matched := false
		for rel, wikiName := range localFiles {
			// Check primary match
			if wikiName == base {
				matched = true
				break
			}
			// Check legacy matches
			if ToWikiPath(rel, LegacyWikiPrefixBase) == base {
				matched = true
				break
			}
			// Check hyphenated match (primary)
			hyphenatedPrimary := WikiPrefixBase + strings.ReplaceAll(strings.TrimSuffix(rel, ".md"), "/", "~") + ".md"
			if hyphenatedPrimary == base {
				matched = true
				break
			}
			// Check hyphenated match (legacy)
			hyphenatedLegacy := LegacyWikiPrefixBase + strings.ReplaceAll(strings.TrimSuffix(rel, ".md"), "/", "~") + ".md"
			if hyphenatedLegacy == base {
				matched = true
				break
			}
		}

		if !matched {
			// Runaway discovery (Wiki files not in local repo)
			wikiPath := filepath.Join(cfg.WikiDir, actual)
			bytesWiki, _ := os.ReadFile(wikiPath)
			items = append(items, FileItem{
				WikiPath:    actual,
				RelPath:     actual,
				WikiContent: string(bytesWiki),
				Status:      StatusObsoleted,
				ChangeType:  "Obsoleted",
			})
		}
	}

	return items, nil
}

// Helper: Extract info layout from "## wiki-docs.display" block
func extractInfoLayout(content string) string {
	// 1. Find the header
	header := "## wiki-docs.display"
	startIdx := strings.Index(content, header)
	if startIdx == -1 {
		return ""
	}

	// 2. Start after the header line
	bodyStart := startIdx + len(header)

	// Move past newline
	rest := content[bodyStart:]
	if idx := strings.Index(rest, "\n"); idx != -1 {
		rest = rest[idx+1:]
	} else {
		return "" // Header at end of file, empty body
	}

	// 3. Find next header (## ) to stop, or EOF
	// We want to stop at the next "## " that starts a line
	// Simple approach: split by lines
	lines := strings.Split(rest, "\n")
	var layoutLines []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			break // Found next section
		}
		layoutLines = append(layoutLines, line)
	}

	return strings.TrimSpace(strings.Join(layoutLines, "\n"))
}
