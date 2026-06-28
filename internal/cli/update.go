package cli

import (
	"github.com/spf13/cobra"
	"github.com/wonderjl/no-mistakes/internal/update"
)

func newUpdateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "Report that built-in self-update is disabled",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return update.Run(cmd.OutOrStdout())
		},
	}
}
