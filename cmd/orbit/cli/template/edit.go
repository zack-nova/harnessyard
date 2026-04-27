package orbittemplate

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
)

const templateEditDirPattern = "orbit-template-edit-*"

// Editor edits a materialized template candidate tree in a temp directory.
type Editor interface {
	Edit(ctx context.Context, dir string) error
}

// CommandEditor runs one external editor command against the temp template directory.
type CommandEditor struct {
	Command string
	Argv    []string
}

// NewEnvironmentEditor builds an editor from the current EDITOR environment variable.
func NewEnvironmentEditor() (Editor, error) {
	command := strings.TrimSpace(os.Getenv("EDITOR"))
	if command == "" {
		return nil, fmt.Errorf("EDITOR must be set when editor mode is enabled")
	}

	argv, err := parseEditorCommand(command)
	if err != nil {
		return nil, fmt.Errorf("parse EDITOR: %w", err)
	}

	return CommandEditor{Command: command, Argv: argv}, nil
}

// Edit executes the configured editor command with the temp template directory appended as the final argument.
func (editor CommandEditor) Edit(ctx context.Context, dir string) error {
	argv := append([]string(nil), editor.Argv...)
	if len(argv) == 0 {
		var err error
		argv, err = parseEditorCommand(editor.Command)
		if err != nil {
			return fmt.Errorf("parse editor command: %w", err)
		}
	}

	args := append(argv[1:], dir)
	//nolint:gosec // The editor command is an explicit user-provided executable path/argv.
	cmd := exec.CommandContext(ctx, argv[0], args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run editor %q: %w", editor.Command, err)
	}

	return nil
}

func parseEditorCommand(command string) ([]string, error) {
	trimmed := strings.TrimSpace(command)
	if trimmed == "" {
		return nil, fmt.Errorf("editor command must not be empty")
	}

	argv := make([]string, 0)
	var current strings.Builder
	tokenStarted := false
	inSingleQuotes := false
	inDoubleQuotes := false
	escaped := false

	flush := func() {
		if !tokenStarted {
			return
		}
		argv = append(argv, current.String())
		current.Reset()
		tokenStarted = false
	}

	for _, r := range trimmed {
		switch {
		case escaped:
			current.WriteRune(r)
			tokenStarted = true
			escaped = false
		case inSingleQuotes:
			if r == '\'' {
				inSingleQuotes = false
			} else {
				current.WriteRune(r)
			}
			tokenStarted = true
		case inDoubleQuotes:
			switch r {
			case '"':
				inDoubleQuotes = false
			case '\\':
				escaped = true
			default:
				current.WriteRune(r)
			}
			tokenStarted = true
		case unicode.IsSpace(r):
			flush()
		case r == '\'':
			inSingleQuotes = true
			tokenStarted = true
		case r == '"':
			inDoubleQuotes = true
			tokenStarted = true
		case r == '\\':
			escaped = true
			tokenStarted = true
		default:
			current.WriteRune(r)
			tokenStarted = true
		}
	}

	switch {
	case escaped:
		return nil, fmt.Errorf("unterminated escape in editor command")
	case inSingleQuotes || inDoubleQuotes:
		return nil, fmt.Errorf("unterminated quote in editor command")
	}

	flush()
	if len(argv) == 0 {
		return nil, fmt.Errorf("editor command must not be empty")
	}

	return argv, nil
}

func editTemplateFiles(ctx context.Context, orbitID string, files []CandidateFile, editor Editor) ([]CandidateFile, error) {
	editedFiles, err := EditCandidateFiles(ctx, files, editor)
	if err != nil {
		return nil, err
	}

	companionPath, _, err := templateCompanionPaths(orbitID)
	if err != nil {
		return nil, fmt.Errorf("build companion definition path: %w", err)
	}

	var companionContent []byte
	for _, file := range editedFiles {
		if file.Path == companionPath {
			companionContent = file.Content
			break
		}
	}
	if companionContent == nil {
		return nil, fmt.Errorf("edited template must keep companion definition %s", companionPath)
	}
	if _, err := orbit.ParseHostedOrbitSpecData(companionContent, companionPath); err != nil {
		return nil, fmt.Errorf("validate edited companion definition %s: %w", companionPath, err)
	}

	return editedFiles, nil
}

// EditCandidateFiles materializes one candidate file tree in a temp directory, runs the editor,
// and returns the edited file set without validating domain-specific template contracts.
func EditCandidateFiles(ctx context.Context, files []CandidateFile, editor Editor) ([]CandidateFile, error) {
	if editor == nil {
		return nil, fmt.Errorf("template editor must be configured when --edit-template is enabled")
	}

	tempDir, err := os.MkdirTemp("", templateEditDirPattern)
	if err != nil {
		return nil, fmt.Errorf("create template edit temp dir: %w", err)
	}
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	for _, file := range files {
		filename := filepath.Join(tempDir, filepath.FromSlash(file.Path))
		if err := os.MkdirAll(filepath.Dir(filename), 0o750); err != nil {
			return nil, fmt.Errorf("create template edit parent dir for %s: %w", file.Path, err)
		}
		perm, err := gitpkg.FilePermForMode(file.Mode)
		if err != nil {
			return nil, fmt.Errorf("resolve template edit file mode %s: %w", file.Path, err)
		}
		if err := os.WriteFile(filename, file.Content, perm); err != nil {
			return nil, fmt.Errorf("write template edit file %s: %w", file.Path, err)
		}
	}

	if err := editor.Edit(ctx, tempDir); err != nil {
		return nil, fmt.Errorf("edit template candidate dir: %w", err)
	}

	return readEditedTemplateFiles(tempDir)
}

func readEditedTemplateFiles(root string) ([]CandidateFile, error) {
	files := make([]CandidateFile, 0)

	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("edited template path %s must be a regular file", path)
		}

		relativePath, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("resolve edited template relative path for %s: %w", path, err)
		}

		normalizedPath, err := ids.NormalizeRepoRelativePath(filepath.ToSlash(relativePath))
		if err != nil {
			return fmt.Errorf("normalize edited template path %q: %w", relativePath, err)
		}
		if normalizedPath == manifestRelativePath {
			return fmt.Errorf("edited template must not write %s directly", manifestRelativePath)
		}

		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("stat edited template file %s: %w", normalizedPath, err)
		}

		//nolint:gosec // The walked path is confined to the temp directory created in this function.
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read edited template file %s: %w", normalizedPath, err)
		}

		files = append(files, CandidateFile{
			Path:    normalizedPath,
			Content: data,
			Mode:    editedTemplateFileMode(info.Mode()),
		})

		return nil
	}); err != nil {
		return nil, fmt.Errorf("read edited template tree: %w", err)
	}

	sort.Slice(files, func(left, right int) bool {
		return files[left].Path < files[right].Path
	})

	return files, nil
}

func editedTemplateFileMode(mode os.FileMode) string {
	if mode&0o111 != 0 {
		return gitpkg.FileModeExecutable
	}

	return gitpkg.FileModeRegular
}
