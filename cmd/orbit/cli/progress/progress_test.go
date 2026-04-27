package progress

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewEmitterPlainWritesStableStageLine(t *testing.T) {
	t.Parallel()

	var buffer bytes.Buffer
	emitter, err := NewEmitter(&buffer, "plain")
	require.NoError(t, err)

	require.NoError(t, emitter.Stage("building template preview"))
	require.Equal(t, "progress: building template preview\n", buffer.String())
}

func TestNewEmitterQuietSuppressesStageLines(t *testing.T) {
	t.Parallel()

	var buffer bytes.Buffer
	emitter, err := NewEmitter(&buffer, "quiet")
	require.NoError(t, err)

	require.NoError(t, emitter.Stage("template save complete"))
	require.Empty(t, buffer.String())
}

func TestNewEmitterRejectsInvalidMode(t *testing.T) {
	t.Parallel()

	_, err := NewEmitter(&bytes.Buffer{}, "verbose")
	require.ErrorContains(t, err, `invalid --progress value "verbose"`)
}
