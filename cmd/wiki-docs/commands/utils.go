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

// FileItem represents a file to be synced
type FileItem struct {
	WikiPath     string
	LocalPath    string
	RelPath      string
	WikiContent  string
	LocalContent string
	Status       string                 // "New", "Changed", "Same", "Runaway"
	ChangeType   string                 // "Content", "Meta", "Mixed", "New"
	Version      string                 // Version from frontmatter
	Approved     string                 // Approved versions from frontmatter
	HasValidYAML bool                   // Whether the file has compliant YAML frontmatter
	Meta         map[string]interface{} // Full parsed frontmatter
	ExpectedMeta []string               // Attributes expected from template
	MetaDiff     []string
	Selected     bool
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

// ScanAll discovers all files in Sources and Wiki and determines their sync status.
func ScanAll(cfg Config) ([]FileItem, error) {
	var items []FileItem

	// 1. Get List of Tracked Wiki Files (Definitive state)
	// We use 'git ls-files' to avoid being fooled by rebase artifacts or untracked debris.
	cmdWiki := exec.Command("git", "-C", cfg.WikiDir, "ls-files")
	outWiki, _ := cmdWiki.Output()
	wikiFiles := strings.Split(strings.TrimSpace(string(outWiki)), "\n")

	wikiMap := make(map[string]string) // Normalized Wiki Name -> Actual Wiki Path
	for _, wf := range wikiFiles {
		if wf == "" || filepath.Ext(wf) != ".md" {
			continue
		}
		// STRICT: Gitea Wikis are flat. Only files in the root of the wiki repo count.
		if strings.Contains(wf, "/") || strings.Contains(wf, "\\") {
			continue
		}

		// Only consider files that follow our naming conventions (to avoid repo debris)
		if !strings.HasPrefix(wf, WikiPrefixBase) && !strings.HasPrefix(wf, LegacyWikiPrefixBase) && !strings.HasPrefix(wf, TemplatePrefixBase) {
			continue
		}

		wikiMap[wf] = wf
	}

	// 2. Discover Local Files (Respecting .gitignore)
	localFiles := make(map[string]string) // RelPath -> WikiName
	for _, source := range cfg.Sources {
		absSourceDir := filepath.Join(cfg.RepoRoot, source)
		if _, err := os.Stat(absSourceDir); os.IsNotExist(err) {
			continue
		}

		err := filepath.Walk(absSourceDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				// Don't skip dirs yet, check-ignore handles it
				return nil
			}
			if filepath.Ext(path) != ".md" {
				return nil
			}

			relPathRaw, _ := filepath.Rel(cfg.RepoRoot, path)
			relPath := filepath.ToSlash(relPathRaw)

			// Check if ignored by git
			cmdIgnore := exec.Command("git", "check-ignore", "-q", relPath)
			if err := cmdIgnore.Run(); err == nil {
				// Exit code 0 means it IS ignored
				return nil
			}

			// Check if ignored by .geminiignore
			if isGeminiIgnored(cfg.RepoRoot, relPath) {
				return nil
			}

			// Calculate intended wiki name
			wikiName := ToWikiPath(relPath, WikiPrefixBase)
			localFiles[relPath] = wikiName

			// Load contents
			localContentBytes, _ := os.ReadFile(path)
			localContent := string(localContentBytes)

			status := "New"
			wikiContent := ""
			finalWikiPath := wikiName

			// 3. Matching logic (Wiki-First priority)
			// Combinations to check (in order of preference):
			// A. Primary: src-docs~ (with underscores)
			// B. Legacy:  repo-root~ (with underscores)
			// C. Legacy-Hyphen: src-docs~ (with hyphens)
			// D. Legacy-Hyphen: repo-root~ (with hyphens)

			status = "Untracked" // Default: Local-only, not yet in wiki
			wikiContent = ""
			finalWikiPath = ""
			found := false
			actualWikiFile := ""

			// A. Check Primary Match (src-docs~ + underscores)
			if wf, ok := wikiMap[wikiName]; ok {
				actualWikiFile = wf
				status = "Synced"
				found = true
			} else {
				// B. Check Legacy (repo-root~ + underscores)
				legacyName := ToWikiPath(relPath, LegacyWikiPrefixBase)
				if wf, ok := wikiMap[legacyName]; ok {
					actualWikiFile = wf
					status = "Legacy"
					found = true
				} else {
					// C. Check Legacy-Hyphen (src-docs~ + hyphens)
					hyphenatedPrimary := WikiPrefixBase + strings.ReplaceAll(strings.TrimSuffix(relPath, ".md"), "/", "~") + ".md"
					if wf, ok := wikiMap[hyphenatedPrimary]; ok {
						actualWikiFile = wf
						status = "Legacy"
						found = true
					} else {
						// D. Check Legacy-Hyphen (repo-root~ + hyphens)
						hyphenatedLegacy := LegacyWikiPrefixBase + strings.ReplaceAll(strings.TrimSuffix(relPath, ".md"), "/", "~") + ".md"
						if wf, ok := wikiMap[hyphenatedLegacy]; ok {
							actualWikiFile = wf
							status = "Legacy"
							found = true
						}
					}
				}
			}

			if found {
				finalWikiPath = actualWikiFile
				wikiPath := filepath.Join(cfg.WikiDir, actualWikiFile)
				bytesWiki, _ := os.ReadFile(wikiPath)
				wikiContent = string(bytesWiki)

				if CalculateChecksum(localContent) != CalculateChecksum(wikiContent) {
					status = "Changed"
				} else if status == "Synced" {
					status = "Same"
				}
			}

			// Extract version info from frontmatter
			fm, hasValidYAML := parseFrontmatter(localContent)
			version, _ := fm["version"].(string)
			approved := ""
			if v, ok := fm["approved_versions"]; ok {
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
				WikiPath:     finalWikiPath,
				LocalPath:    path,
				RelPath:      relPath,
				WikiContent:  wikiContent,
				LocalContent: localContent,
				Status:       status,
				ChangeType:   status,
				Version:      version,
				Approved:     approved,
				HasValidYAML: hasValidYAML,
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
			// Runaway discovery
			wikiPath := filepath.Join(cfg.WikiDir, actual)
			bytesWiki, _ := os.ReadFile(wikiPath)
			items = append(items, FileItem{
				WikiPath:    actual,
				RelPath:     actual, // No easy local match
				WikiContent: string(bytesWiki),
				Status:      "Orphan",
				ChangeType:  "Orphan",
			})
		}
	}

	return items, nil
}
