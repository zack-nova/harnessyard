package cli

import (
	"github.com/spf13/cobra"

	harnesscommands "github.com/zack-nova/harnessyard/cmd/harness/cli/commands"
	orbitcommands "github.com/zack-nova/harnessyard/cmd/orbit/cli/commands"
)

func newGuideCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "guide",
		Short: "Render, save, or sync guidance artifacts",
		Long: "Render, save, or sync guidance artifacts through the canonical hyard user surface.\n" +
			"Use `render` to build current orbit guidance artifacts, `save` to persist edited guidance\n" +
			"back into hosted truth, and `sync` to update runtime-wide root guidance artifacts.\n" +
			"`writeback` remains available as a compatibility alias for `save`.",
		Args: cobra.NoArgs,
	}

	cmd.AddCommand(
		newGuideRenderCommand(),
		newGuideSaveCommand(),
		newGuideWritebackCommand(),
		newGuideSyncCommand(),
	)

	return cmd
}

func newGuideRenderCommand() *cobra.Command {
	cmd := orbitcommands.NewGuidanceMaterializeCommandWithOptions(orbitcommands.GuidanceCommandOptions{
		DefaultAllOrbitsWhenOrbitOmitted: true,
	})
	cmd.Use = "render"
	cmd.Short = "Render authored guidance into editable root artifacts"
	cmd.Long = "Render authored guidance into editable root artifacts.\n" +
		"When --orbit is omitted, render all applicable orbits in the current guidance scope.\n" +
		"By default, --target all renders only applicable guidance targets and skips lanes\n" +
		"without authored truth. Use --orbit <id> to filter to one orbit, --seed-empty to\n" +
		"create first-draft empty blocks, or --strict to make --target all require every selected target."
	cmd.Example = "" +
		"  hyard guide render\n" +
		"  hyard guide render --target all\n" +
		"  hyard guide render --orbit docs\n" +
		"  hyard guide render --orbit docs --target all\n" +
		"  hyard guide render --orbit docs --target all --strict\n" +
		"  hyard guide render --orbit docs --target bootstrap\n" +
		"  hyard guide render --orbit docs --target humans --check\n" +
		"  hyard guide render --orbit docs --target agents --force\n"
	return cmd
}

func newGuideSaveCommand() *cobra.Command {
	cmd := orbitcommands.NewGuidanceBackfillCommandWithOptions(orbitcommands.GuidanceCommandOptions{
		DefaultAllOrbitsWhenOrbitOmitted: true,
	})
	cmd.Use = "save"
	cmd.Short = "Save edited root guidance back into hosted truth"
	cmd.Long = "Save edited root guidance back into hosted truth.\n" +
		"When --orbit is omitted, save all valid orbit blocks found in the selected root guidance artifacts.\n" +
		"With --target all, missing orbit blocks without hosted guidance are skipped; missing blocks with hosted guidance are blocked."
	cmd.Example = "" +
		"  hyard guide save\n" +
		"  hyard guide save --target all\n" +
		"  hyard guide save --orbit docs --target all\n" +
		"  hyard guide save --orbit docs --target humans --check\n" +
		"  hyard guide save --orbit docs --target bootstrap --json\n"
	return cmd
}

func newGuideWritebackCommand() *cobra.Command {
	cmd := orbitcommands.NewGuidanceBackfillCommandWithOptions(orbitcommands.GuidanceCommandOptions{
		DefaultAllOrbitsWhenOrbitOmitted: true,
	})
	cmd.Use = "writeback"
	cmd.Short = "Write edited root guidance back into hosted truth"
	cmd.Long = "Write edited root guidance back into hosted truth.\n" +
		"When --orbit is omitted, write back all valid orbit blocks found in the selected root guidance artifacts.\n" +
		"With --target all, missing orbit blocks without hosted guidance are skipped; missing blocks with hosted guidance are blocked."
	cmd.Example = "" +
		"  hyard guide writeback\n" +
		"  hyard guide writeback --target all\n" +
		"  hyard guide writeback --orbit docs --target all\n" +
		"  hyard guide writeback --orbit docs --target humans --check\n" +
		"  hyard guide writeback --orbit docs --target bootstrap --json\n"
	return cmd
}

func newGuideSyncCommand() *cobra.Command {
	cmd := harnesscommands.NewGuidanceComposeCommand()
	cmd.Use = "sync"
	cmd.Short = "Sync runtime-wide root guidance artifacts"
	cmd.Long = "Sync runtime-wide root guidance artifacts for agent, human, and bootstrap targets.\n" +
		"When --orbit is omitted, sync all current runtime orbit packages; use --orbit <id> to filter."
	cmd.Example = "" +
		"  hyard guide sync\n" +
		"  hyard guide sync --orbit docs --target all\n" +
		"  hyard guide sync --target humans --json\n" +
		"  hyard guide sync --target bootstrap --force\n"
	return cmd
}
