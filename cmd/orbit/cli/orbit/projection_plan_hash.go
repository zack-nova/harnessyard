package orbit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

func finalizeProjectionPlan(plan ProjectionPlan) (ProjectionPlan, error) {
	hash, err := projectionPlanHash(plan)
	if err != nil {
		return ProjectionPlan{}, err
	}

	plan.PlanHash = hash

	return plan, nil
}

func projectionPlanHash(plan ProjectionPlan) (string, error) {
	payload := struct {
		OrbitID            string   `json:"orbit_id"`
		ControlPaths       []string `json:"control_paths,omitempty"`
		MetaPaths          []string `json:"meta_paths,omitempty"`
		SubjectPaths       []string `json:"subject_paths,omitempty"`
		RulePaths          []string `json:"rule_paths,omitempty"`
		ProcessPaths       []string `json:"process_paths,omitempty"`
		CapabilityPaths    []string `json:"capability_paths,omitempty"`
		ProjectionPaths    []string `json:"projection_paths,omitempty"`
		OrbitWritePaths    []string `json:"orbit_write_paths,omitempty"`
		ExportPaths        []string `json:"export_paths,omitempty"`
		OrchestrationPaths []string `json:"orchestration_paths,omitempty"`
	}{
		OrbitID:            plan.OrbitID,
		ControlPaths:       plan.ControlPaths,
		MetaPaths:          plan.MetaPaths,
		SubjectPaths:       plan.SubjectPaths,
		RulePaths:          plan.RulePaths,
		ProcessPaths:       plan.ProcessPaths,
		CapabilityPaths:    plan.CapabilityPaths,
		ProjectionPaths:    plan.ProjectionPaths,
		OrbitWritePaths:    plan.OrbitWritePaths,
		ExportPaths:        plan.ExportPaths,
		OrchestrationPaths: plan.OrchestrationPaths,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal projection plan hash payload: %w", err)
	}

	sum := sha256.Sum256(data)

	return hex.EncodeToString(sum[:]), nil
}
