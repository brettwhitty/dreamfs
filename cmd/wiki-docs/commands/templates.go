package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v5"
	"gopkg.in/yaml.v3"
)

type TemplateItem struct {
	Name    string
	Content string
}

// LoadTemplates scans the .templates directory in the Wiki for markdown files.
func LoadTemplates(wikiDir string) ([]TemplateItem, error) {
	templatesDir := filepath.Join(wikiDir, ".templates")
	var templates []TemplateItem

	info, err := os.Stat(templatesDir)
	if os.IsNotExist(err) || !info.IsDir() {
		return templates, nil // No templates found, not an error
	}

	entries, err := os.ReadDir(templatesDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
			content, err := os.ReadFile(filepath.Join(templatesDir, entry.Name()))
			if err == nil {
				templates = append(templates, TemplateItem{
					Name:    strings.TrimSuffix(entry.Name(), ".md"),
					Content: string(content),
				})
			}
		}
	}

	return templates, nil
}

// ValidateFrontmatter validates the YAML frontmatter of a file against a JSON schema.
func ValidateFrontmatter(content string, schemaPath string) error {
	// 1. Check if schema exists
	if _, err := os.Stat(schemaPath); os.IsNotExist(err) {
		return nil // No schema defined, skip validation
	}

	// 2. Extract and Parse YAML Frontmatter
	fmMap, _ := parseFrontmatter(content)
	if len(fmMap) == 0 {
		// Should we fail if schema exists but no FM?
		// Maybe. Let's assume yes strict if schema exists.
		// But parseFrontmatter returns empty map if no FM.
		// Let's validate the empty map against schema.
	}

	// 3. Convert YAML Map to JSON (structure) for Validator
	// jsonschema library works on Go objects (interface{}), so map[string]interface{} is fine,
	// BUT it expects types compatible with JSON. YAML often unmarshals to map[interface{}]interface{} if not careful,
	// but gopkg.in/yaml.v3 with map[string]interface{} should be fine.
	// Actually, let's verify.
	// We might need to marshal to JSON and unmarshal back to ensure types are strictly JSON-compatible (e.g. integer vs float).

	jsonBytes, err := json.Marshal(fmMap)
	if err != nil {
		return fmt.Errorf("failed to convert frontmatter to JSON: %w", err)
	}

	var jsonObj interface{}
	if err := json.Unmarshal(jsonBytes, &jsonObj); err != nil {
		return fmt.Errorf("failed to parse JSON frontmatter: %w", err)
	}

	// 4. Compile Schema
	compiler := jsonschema.NewCompiler()
	// Allow loading schema from file
	// We need to handle Windows paths for URL?
	// jsonschema expects a URL. "file:///..."
	// simpler to just AddResource directly if we read it?

	schemaBytes, err := os.ReadFile(schemaPath)
	if err != nil {
		return fmt.Errorf("failed to read schema file: %w", err)
	}

	// Handle YAML Schemas (transform to JSON object first)
	if strings.HasSuffix(schemaPath, ".yaml") || strings.HasSuffix(schemaPath, ".yml") {
		var schemaObj interface{}
		if err := yaml.Unmarshal(schemaBytes, &schemaObj); err != nil {
			return fmt.Errorf("failed to parse YAML schema: %w", err)
		}
		jsonBytes, err := json.Marshal(schemaObj)
		if err != nil {
			return fmt.Errorf("failed to convert YAML schema to JSON: %w", err)
		}
		schemaBytes = jsonBytes
	}

	if err := compiler.AddResource("schema.json", strings.NewReader(string(schemaBytes))); err != nil {
		return fmt.Errorf("failed to compile schema: %w", err)
	}

	schema, err := compiler.Compile("schema.json")
	if err != nil {
		return fmt.Errorf("failed to compile schema: %w", err)
	}

	// 5. Validate
	if err := schema.Validate(jsonObj); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	return nil
}

// FindInheritedTemplate looks for an applicable template in the wiki based on the local file path.
// It searches upwards through the path hierarchy using the src-tmpl~ prefix and wildcard syntax.
func FindInheritedTemplate(relPath string, wikiDir string) (string, string) {
	// e.g. relPath: docs/agent/rules/GEMINI.md
	// Look for:
	// 1. src-tmpl~docs~agent~rules~GEMINI.md (Exact)
	// 2. src-tmpl~docs~agent~rules~*.md (Specific Dir Wildcard)
	// 3. src-tmpl~docs~agent~*.md (Parent Dir Wildcard)
	// 4. src-tmpl~docs~*.md
	// 5. src-tmpl.md (Global)

	ext := filepath.Ext(relPath)
	base := strings.TrimSuffix(relPath, ext)
	// Normalize path components for wiki naming (hyphens to underscores, / to ~)
	// Only the path part gets hyphen-to-underscore. The prefix is constant.
	base = strings.ReplaceAll(base, "-", "_")
	parts := strings.Split(base, "/")

	// 1. Check EXACT match first
	exactName := TemplatePrefixBase + strings.Join(parts, "~") + ".md"
	exactPath := filepath.Join(wikiDir, exactName)
	if _, err := os.Stat(exactPath); err == nil {
		content, _ := os.ReadFile(exactPath)
		return exactName, string(content)
	}

	// 2. Check DIRECTORY wildcards upwards
	for i := len(parts); i > 0; i-- {
		currentPath := strings.Join(parts[:i], "~")
		wikiFilename := TemplatePrefixBase + currentPath + "~*.md"
		wikiPath := filepath.Join(wikiDir, wikiFilename)

		if _, err := os.Stat(wikiPath); err == nil {
			content, _ := os.ReadFile(wikiPath)
			return wikiFilename, string(content)
		}
	}

	// 3. Check GLOBAL template
	// Prefix is "src-tmpl~", but global is "src-tmpl.md" (user specified)
	globalName := strings.TrimSuffix(TemplatePrefixBase, "~") + ".md" // "src-tmpl.md"
	globalPath := filepath.Join(wikiDir, globalName)
	if _, err := os.Stat(globalPath); err == nil {
		content, _ := os.ReadFile(globalPath)
		return globalName, string(content)
	}

	return "", ""
}
