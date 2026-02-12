// Sync files and directories to and from local and remote object stores
//
// Nick Craig-Wood <nick@craig-wood.com>
package main

import (
	"fmt"
	"os"

	_ "github.com/ebadenes/eclone/backend/all" // import all backends
	_ "github.com/ebadenes/eclone/cmd/all"     // import all commands
	versionCmd "github.com/ebadenes/eclone/cmd/version"
	"github.com/rclone/rclone/cmd"
	_ "github.com/rclone/rclone/lib/plugin" // import plugins
	"github.com/spf13/cobra"
)

func main() {
	// Brand the root command as eclone
	cmd.Root.Use = "eclone"
	cmd.Root.Short = "Show help for eclone commands, flags and backends."

	// Override Root.Run after cmd.Main's setupRootCommand has configured it,
	// so that --version prints "eclone" instead of "rclone".
	cobra.OnInitialize(func() {
		cmd.Root.Run = func(c *cobra.Command, args []string) {
			v, _ := c.Flags().GetBool("version")
			if v {
				versionCmd.ShowVersion()
				os.Exit(0)
			}
			_ = c.Usage()
			if len(args) > 0 {
				fmt.Fprintf(os.Stderr, "Command not found.\n")
				os.Exit(1)
			}
		}
	})

	cmd.Main()
}
