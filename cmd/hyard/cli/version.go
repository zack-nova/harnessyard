package cli

import "fmt"

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
	builtBy = "unknown"
)

// Version returns the human-readable build metadata used by `hyard --version`.
func Version() string {
	return fmt.Sprintf("hyard %s\ncommit: %s\ndate: %s\nbuilt by: %s", version, commit, date, builtBy)
}
