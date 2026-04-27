package orbittemplate

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseEditorCommandSupportsQuotedExecutableAndArguments(t *testing.T) {
	t.Parallel()

	argv, err := parseEditorCommand(`"/tmp/tools with spaces/editor.sh" --mode "template edit"`)
	require.NoError(t, err)
	require.Equal(t, []string{
		"/tmp/tools with spaces/editor.sh",
		"--mode",
		"template edit",
	}, argv)
}

func TestParseEditorCommandRejectsMalformedQuotes(t *testing.T) {
	t.Parallel()

	_, err := parseEditorCommand(`"/tmp/editor.sh --mode`)
	require.Error(t, err)
	require.ErrorContains(t, err, "unterminated")
}

func TestNewEnvironmentEditorRejectsMalformedEditorCommand(t *testing.T) {
	t.Setenv("EDITOR", `"/tmp/editor.sh --mode`)

	_, err := NewEnvironmentEditor()
	require.Error(t, err)
	require.ErrorContains(t, err, "parse EDITOR")
}
