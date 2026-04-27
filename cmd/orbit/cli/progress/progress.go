package progress

import (
	"fmt"
	"io"
	"os"
	"strings"
)

type mode string

const (
	modeAuto  mode = "auto"
	modePlain mode = "plain"
	modeQuiet mode = "quiet"
)

// Emitter writes stable CLI progress stages to one writer.
type Emitter struct {
	writer  io.Writer
	enabled bool
}

// NewEmitter builds one progress emitter from the shared auto/plain/quiet contract.
func NewEmitter(writer io.Writer, rawMode string) (Emitter, error) {
	parsed, err := parseMode(rawMode)
	if err != nil {
		return Emitter{}, err
	}

	switch parsed {
	case modePlain:
		return Emitter{writer: writer, enabled: true}, nil
	case modeQuiet:
		return Emitter{writer: writer, enabled: false}, nil
	case modeAuto:
		return Emitter{writer: writer, enabled: writerIsTerminal(writer)}, nil
	default:
		return Emitter{}, fmt.Errorf("unsupported progress mode %q", rawMode)
	}
}

func parseMode(rawMode string) (mode, error) {
	trimmed := strings.TrimSpace(rawMode)
	if trimmed == "" {
		return modeAuto, nil
	}

	switch mode(trimmed) {
	case modeAuto, modePlain, modeQuiet:
		return mode(trimmed), nil
	default:
		return "", fmt.Errorf("invalid --progress value %q (expected auto, plain, or quiet)", rawMode)
	}
}

func writerIsTerminal(writer io.Writer) bool {
	file, ok := writer.(*os.File)
	if !ok {
		return false
	}

	info, err := file.Stat()
	if err != nil {
		return false
	}

	return (info.Mode() & os.ModeCharDevice) != 0
}

// Stage emits one stable progress line to stderr when progress is enabled.
func (emitter Emitter) Stage(stage string) error {
	if !emitter.enabled {
		return nil
	}
	if _, err := fmt.Fprintf(emitter.writer, "progress: %s\n", strings.TrimSpace(stage)); err != nil {
		return fmt.Errorf("write progress: %w", err)
	}
	return nil
}
