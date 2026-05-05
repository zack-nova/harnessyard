package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
)

// RuntimeView identifies the repository-local runtime presentation intent.
type RuntimeView string

const (
	RuntimeViewRun    RuntimeView = "run"
	RuntimeViewAuthor RuntimeView = "author"
)

// RuntimeViewSelection captures the persisted view selection. Persisted is
// derived when reading and is not written to disk.
type RuntimeViewSelection struct {
	View       RuntimeView `json:"view"`
	SelectedAt time.Time   `json:"selected_at,omitempty"`
	Persisted  bool        `json:"-"`
}

// WriteRuntimeViewSelection writes runtime_view_selection.json atomically.
func (store FSStore) WriteRuntimeViewSelection(selection RuntimeViewSelection) error {
	if err := validateRuntimeView(selection.View); err != nil {
		return err
	}

	data, err := json.MarshalIndent(selection, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal runtime view selection: %w", err)
	}

	data = append(data, '\n')

	if err := atomicWriteFile(store.runtimeViewSelectionPath(), data); err != nil {
		return fmt.Errorf("write runtime view selection: %w", err)
	}

	return nil
}

// ReadRuntimeViewSelection reads the repository-local selection. Missing state
// resolves to Run View without creating a file.
func (store FSStore) ReadRuntimeViewSelection() (RuntimeViewSelection, error) {
	data, err := os.ReadFile(store.runtimeViewSelectionPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return RuntimeViewSelection{View: RuntimeViewRun}, nil
		}
		return RuntimeViewSelection{}, fmt.Errorf("read runtime view selection: %w", err)
	}

	var selection RuntimeViewSelection
	if err := json.Unmarshal(data, &selection); err != nil {
		return RuntimeViewSelection{}, fmt.Errorf("unmarshal runtime view selection: %w", err)
	}
	if err := validateRuntimeView(selection.View); err != nil {
		return RuntimeViewSelection{}, err
	}
	selection.Persisted = true

	return selection, nil
}

func validateRuntimeView(view RuntimeView) error {
	switch view {
	case RuntimeViewRun, RuntimeViewAuthor:
		return nil
	default:
		return fmt.Errorf(`runtime view selection view must be %q or %q`, RuntimeViewRun, RuntimeViewAuthor)
	}
}
