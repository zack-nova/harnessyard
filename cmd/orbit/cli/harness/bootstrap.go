package harness

import (
	"fmt"
	"os"
	"time"
)

// BootstrapResult captures the repo-local files created by runtime bootstrap entrypoints.
type BootstrapResult struct {
	ManifestPath     string
	OrbitsDir        string
	ManifestCreated  bool
	OrbitsDirCreated bool
	Manifest         ManifestFile
	Runtime          RuntimeFile
}

// BootstrapRuntimeControlPlane initializes the runtime manifest and hosted orbit directory.
func BootstrapRuntimeControlPlane(repoRoot string, now time.Time) (BootstrapResult, error) {
	manifestFile, err := DefaultRuntimeManifestFile(repoRoot, now)
	if err != nil {
		return BootstrapResult{}, fmt.Errorf("build default harness manifest: %w", err)
	}

	runtimeFile, err := RuntimeFileFromManifestFile(manifestFile)
	if err != nil {
		return BootstrapResult{}, fmt.Errorf("build default harness runtime: %w", err)
	}

	result := BootstrapResult{
		ManifestPath: ManifestPath(repoRoot),
		OrbitsDir:    OrbitSpecsDirPath(repoRoot),
		Manifest:     manifestFile,
		Runtime:      runtimeFile,
	}

	if _, err := os.Stat(result.ManifestPath); err == nil {
		result.ManifestCreated = false
	} else if errorsIsNotExist(err) {
		result.ManifestCreated = true
	} else {
		return BootstrapResult{}, fmt.Errorf("stat %s: %w", result.ManifestPath, err)
	}

	if info, err := os.Stat(result.OrbitsDir); err == nil {
		if !info.IsDir() {
			return BootstrapResult{}, fmt.Errorf("%s exists but is not a directory", result.OrbitsDir)
		}
		result.OrbitsDirCreated = false
	} else if errorsIsNotExist(err) {
		result.OrbitsDirCreated = true
		if err := os.MkdirAll(result.OrbitsDir, 0o750); err != nil {
			return BootstrapResult{}, fmt.Errorf("create %s: %w", result.OrbitsDir, err)
		}
	} else {
		return BootstrapResult{}, fmt.Errorf("stat %s: %w", result.OrbitsDir, err)
	}

	if _, err := WriteManifestFile(repoRoot, manifestFile); err != nil {
		return BootstrapResult{}, fmt.Errorf("write harness manifest: %w", err)
	}

	return result, nil
}

func errorsIsNotExist(err error) bool {
	return err != nil && os.IsNotExist(err)
}
