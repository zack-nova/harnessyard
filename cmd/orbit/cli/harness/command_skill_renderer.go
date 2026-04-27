package harness

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type commandSkillFrontmatter struct {
	Name        string `yaml:"name,omitempty"`
	Description string `yaml:"description,omitempty"`
}

func compileCommandAsProjectSkill(repoRoot string, gitDir string, frameworkID string, routeOutput FrameworkRoutePlanOutput) (string, error) {
	namespace := strings.TrimSpace(routeOutput.OrbitID)
	if namespace == "" {
		namespace = "commands"
	}
	compiledRoot := filepath.Join(gitDir, "orbit", "state", "agents", "compiled", frameworkID, "commands", namespace, routeOutput.Artifact)
	if err := os.RemoveAll(compiledRoot); err != nil {
		return "", fmt.Errorf("clear compiled command skill %s: %w", compiledRoot, err)
	}
	if err := os.MkdirAll(compiledRoot, 0o750); err != nil {
		return "", fmt.Errorf("create compiled command skill %s: %w", compiledRoot, err)
	}
	rendered, err := renderCommandAsProjectSkillData(repoRoot, routeOutput.Source, routeOutput.Artifact)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(compiledRoot, "SKILL.md"), rendered, 0o600); err != nil {
		return "", fmt.Errorf("write compiled command skill: %w", err)
	}

	return compiledRoot, nil
}

func renderCommandAsProjectSkillData(repoRoot string, source string, artifact string) ([]byte, error) {
	sourcePath := filepath.Join(repoRoot, filepath.FromSlash(source))
	data, err := os.ReadFile(sourcePath) //nolint:gosec // Source path comes from resolved repo capability truth.
	if err != nil {
		return nil, fmt.Errorf("read command source %s: %w", source, err)
	}

	frontmatter, body, err := parseCommandSkillFrontmatter(data)
	if err != nil {
		return nil, fmt.Errorf("parse command source %s: %w", source, err)
	}
	name := strings.TrimSpace(frontmatter.Name)
	if name == "" {
		name = artifact
	}
	description := strings.TrimSpace(frontmatter.Description)
	if description == "" {
		description = fallbackCommandSkillDescription(body, artifact)
	}

	return renderCommandSkillMarkdown(name, description, body), nil
}

func parseCommandSkillFrontmatter(data []byte) (commandSkillFrontmatter, string, error) {
	normalized := bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	if !bytes.HasPrefix(normalized, []byte("---\n")) {
		return commandSkillFrontmatter{}, strings.TrimSpace(string(normalized)), nil
	}

	rest := bytes.TrimPrefix(normalized, []byte("---\n"))
	end := bytes.Index(rest, []byte("\n---\n"))
	if end < 0 {
		return commandSkillFrontmatter{}, "", fmt.Errorf("frontmatter must terminate with ---")
	}
	var frontmatter commandSkillFrontmatter
	if err := yaml.Unmarshal(rest[:end], &frontmatter); err != nil {
		return commandSkillFrontmatter{}, "", fmt.Errorf("frontmatter is invalid YAML: %w", err)
	}

	return frontmatter, strings.TrimSpace(string(rest[end+len("\n---\n"):])), nil
}

func fallbackCommandSkillDescription(body string, name string) string {
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		trimmed = strings.TrimLeft(trimmed, "# ")
		if len(trimmed) > 120 {
			return strings.TrimSpace(trimmed[:120])
		}
		return trimmed
	}

	return "Run the " + name + " command."
}

func renderCommandSkillMarkdown(name string, description string, body string) []byte {
	frontmatter, err := yaml.Marshal(commandSkillFrontmatter{
		Name:        name,
		Description: description,
	})
	if err != nil {
		frontmatter = []byte("name: " + name + "\ndescription: " + description + "\n")
	}
	content := strings.TrimSpace(body)
	if content == "" {
		content = "Run this command."
	}

	return []byte("---\n" + string(frontmatter) + "---\n\n" + content + "\n")
}
