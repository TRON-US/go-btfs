package store

import (
	"github.com/TRON-US/go-btfs/core/commands/store/announce"
	"github.com/TRON-US/go-btfs/core/commands/store/challenge"
	"github.com/TRON-US/go-btfs/core/commands/store/contracts"
	"github.com/TRON-US/go-btfs/core/commands/store/hosts"
	"github.com/TRON-US/go-btfs/core/commands/store/info"
	"github.com/TRON-US/go-btfs/core/commands/store/stats"
	upload "github.com/TRON-US/go-btfs/core/commands/store/upload"

	cmds "github.com/TRON-US/go-btfs-cmds"
)

var StorageCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Interact with storage services on BTFS.",
		ShortDescription: `
Storage services include client upload operations, host storage operations,
host information sync/display operations, and BTT payment-related routines.`,
	},
	Subcommands: map[string]*cmds.Command{
		"upload":    upload.StorageUploadCmd,
		"hosts":     hosts.StorageHostsCmd,
		"info":      info.StorageInfoCmd,
		"announce":  announce.StorageAnnounceCmd,
		"challenge": challenge.StorageChallengeCmd,
		"stats":     stats.StorageStatsCmd,
		"contracts": contracts.StorageContractsCmd,
	},
}
