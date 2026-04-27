package commands

import (
	"io"

	progresspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/progress"
)

type installProgressEmitter = progresspkg.Emitter

func newInstallProgressEmitter(writer io.Writer, rawMode string) (installProgressEmitter, error) {
	return newProgressEmitter(writer, rawMode)
}
