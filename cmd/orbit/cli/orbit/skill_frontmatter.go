package orbit

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type skillFrontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

func loadSkillFrontmatter(repoRoot string, skillMDPath string) (skillFrontmatter, error) {
	filename := filepath.Join(repoRoot, filepath.FromSlash(skillMDPath))
	data, err := os.ReadFile(filename) //nolint:gosec // The path is repo-local and derived from a validated repo-relative path.
	if err != nil {
		return skillFrontmatter{}, fmt.Errorf("read SKILL.md: %w", err)
	}

	return parseSkillFrontmatter(data)
}

// LoadSkillFrontmatterName returns the validated skill name from one repo-relative SKILL.md.
func LoadSkillFrontmatterName(repoRoot string, skillMDPath string) (string, error) {
	frontmatter, err := loadSkillFrontmatter(repoRoot, skillMDPath)
	if err != nil {
		return "", err
	}

	return frontmatter.Name, nil
}

func parseSkillFrontmatter(data []byte) (skillFrontmatter, error) {
	content := strings.ReplaceAll(string(data), "\r\n", "\n")
	if !strings.HasPrefix(content, "---\n") {
		return skillFrontmatter{}, fmt.Errorf("SKILL.md must start with YAML frontmatter")
	}

	rest := strings.TrimPrefix(content, "---\n")
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		return skillFrontmatter{}, fmt.Errorf("SKILL.md frontmatter must terminate with ---")
	}

	frontmatterContent := rest[:end]
	var frontmatter skillFrontmatter
	if err := yaml.Unmarshal([]byte(frontmatterContent), &frontmatter); err != nil {
		return skillFrontmatter{}, fmt.Errorf("SKILL.md frontmatter is invalid YAML: %w", err)
	}
	if strings.TrimSpace(frontmatter.Name) == "" {
		return skillFrontmatter{}, fmt.Errorf("SKILL.md frontmatter must define non-empty name")
	}
	if strings.TrimSpace(frontmatter.Description) == "" {
		return skillFrontmatter{}, fmt.Errorf("SKILL.md frontmatter must define non-empty description")
	}

	frontmatter.Name = strings.TrimSpace(frontmatter.Name)
	frontmatter.Description = strings.TrimSpace(frontmatter.Description)

	return frontmatter, nil
}
