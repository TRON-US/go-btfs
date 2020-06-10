package storage

import (
	cmds "github.com/TRON-US/go-btfs-cmds"
	"github.com/TRON-US/go-btfs/core/commands/storage/files"
)

var StorageFilesCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Interact with file storage services on BTFS..",
		ShortDescription: `
File storage services currently include renter retrieve uploaded file information and other file related operation.`,
	},
	Subcommands: map[string]*cmds.Command{
		"list": files.FileListCmd,
	},
}
