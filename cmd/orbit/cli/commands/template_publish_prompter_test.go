package commands

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestBuildTemplateSourceBranchPushPrompterUsesPreservedInteractiveContext(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{}
	cmd.SetContext(WithTemplatePublishInteractive(context.Background()))
	cmd.SetIn(strings.NewReader(""))
	cmd.SetErr(&bytes.Buffer{})

	prompter := buildTemplateSourceBranchPushPrompter(cmd, false)
	require.NotNil(t, prompter)
}

func TestBuildTemplateSourceBranchPushPrompterStaysDisabledForJSON(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{}
	cmd.SetContext(WithTemplatePublishInteractive(context.Background()))
	cmd.SetIn(strings.NewReader(""))
	cmd.SetErr(&bytes.Buffer{})

	prompter := buildTemplateSourceBranchPushPrompter(cmd, true)
	require.Nil(t, prompter)
}
