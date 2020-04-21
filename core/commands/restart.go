package commands

import (
	"os/exec"
	"time"

	"github.com/TRON-US/go-btfs-cmds"

	"github.com/TRON-US/go-btfs/core/commands/store/path"

	"github.com/cenkalti/backoff/v3"
)

var daemonStartup = func() *backoff.ExponentialBackOff {
	bo := backoff.NewExponentialBackOff()
	bo.InitialInterval = 2 * time.Second
	bo.MaxElapsedTime = 300 * time.Second
	bo.Multiplier = 1
	bo.MaxInterval = 2 * time.Second
	return bo
}()

var restartCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Restart the daemon.",
		ShortDescription: `
Shutdown the runnning daemon and start a new daemon process.
And if specified a new btfs path, it will be applied.
`,
	},

	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		shutdownCmd := exec.Command("btfs", "shutdown")
		if err := shutdownCmd.Run(); err != nil {
			return err
		}

		if path.StorePath != "" && path.OriginPath != "" {
			if err := path.MoveFolder(); err != nil {
				return err
			}

			if err := path.WriteProperties(); err != nil {
				return err
			}
		}

		err := backoff.Retry(func() error {
			daemonCmd := exec.Command("btfs", "daemon")
			if err := daemonCmd.Run(); err != nil {
				return err
			}
			return nil
		}, daemonStartup)

		if err != nil {
			return err
		}
		return nil
	},
}
