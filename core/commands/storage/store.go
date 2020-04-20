package storage

import (
	"github.com/TRON-US/go-btfs/core/commands/storage/challenge"
	"github.com/TRON-US/go-btfs/core/commands/storage/upload"

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
		"hosts":     StorageHostsCmd,
		"info":      StorageInfoCmd,
		"announce":  StorageAnnounceCmd,
		"challenge": challenge.StorageChallengeCmd,
		"stats":     StorageStatsCmd,
		"contracts": StorageContractsCmd,
	},
}
