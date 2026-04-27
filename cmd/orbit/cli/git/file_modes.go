package git

import (
	"fmt"
	"os"
)

const (
	FileModeRegular    = "100644"
	FileModeExecutable = "100755"
)

func normalizeFileMode(mode string) (string, error) {
	switch mode {
	case "", FileModeRegular:
		return FileModeRegular, nil
	case FileModeExecutable:
		return FileModeExecutable, nil
	default:
		return "", fmt.Errorf("unsupported file mode %q", mode)
	}
}

func fileModeFromWorktreeInfo(info os.FileInfo) (string, error) {
	if info == nil {
		return "", fmt.Errorf("worktree file info must not be nil")
	}
	if info.IsDir() {
		return "", fmt.Errorf("tracked path %q must be a file", info.Name())
	}

	if info.Mode()&0o111 != 0 {
		return FileModeExecutable, nil
	}

	return FileModeRegular, nil
}

// FilePermForMode maps a Git tree mode to the worktree permissions Orbit should write.
func FilePermForMode(mode string) (os.FileMode, error) {
	normalizedMode, err := normalizeFileMode(mode)
	if err != nil {
		return 0, err
	}

	if normalizedMode == FileModeExecutable {
		return 0o755, nil
	}

	return 0o644, nil
}
