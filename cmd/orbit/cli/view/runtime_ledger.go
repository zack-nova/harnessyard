package view

import (
	"errors"
	"fmt"
	"strings"

	statepkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/state"
)

// RuntimeLedgerStaleness captures one detected runtime-ledger mismatch.
type RuntimeLedgerStaleness struct {
	Stale  bool
	Reason string
	Detail string
}

const runtimeLedgerStaleReasonPlanMismatch = "runtime_plan_mismatch"

func DetectCurrentRuntimeLedgerPlanStaleness(
	store statepkg.FSStore,
	orbitID string,
	planHash string,
) (RuntimeLedgerStaleness, error) {
	snapshot, err := store.ReadRuntimeStateSnapshot(orbitID)
	if err != nil {
		if errors.Is(err, statepkg.ErrRuntimeStateSnapshotNotFound) {
			return RuntimeLedgerStaleness{}, nil
		}

		return RuntimeLedgerStaleness{}, fmt.Errorf("read runtime state snapshot: %w", err)
	}

	if strings.TrimSpace(snapshot.PlanHash) == "" || strings.TrimSpace(planHash) == "" {
		return RuntimeLedgerStaleness{}, nil
	}
	if snapshot.PlanHash == planHash {
		return RuntimeLedgerStaleness{}, nil
	}

	return RuntimeLedgerStaleness{
		Stale:  true,
		Reason: runtimeLedgerStaleReasonPlanMismatch,
		Detail: fmt.Sprintf(
			"active projection no longer matches the current revision; rerun `orbit enter %s` to refresh it",
			orbitID,
		),
	}, nil
}

// ValidateCurrentRuntimeLedgerPlan ensures the stored runtime ledger still
// matches the currently resolved runtime plan before scoped commands proceed.
func ValidateCurrentRuntimeLedgerPlan(store statepkg.FSStore, orbitID string, planHash string) error {
	staleness, err := DetectCurrentRuntimeLedgerPlanStaleness(store, orbitID, planHash)
	if err != nil {
		return err
	}
	if !staleness.Stale {
		return nil
	}

	return fmt.Errorf(
		"current orbit %q ledger is stale: %s",
		orbitID,
		staleness.Detail,
	)
}
