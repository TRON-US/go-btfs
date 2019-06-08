package unixfs

import (
	cmds "github.com/ipfs/go-ipfs-cmds"
)

var UnixFSCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Interact with BTFS objects representing Unix filesystems.",
		ShortDescription: `
'btfs file' provides a familiar interface to file systems represented
by BTFS objects, which hides btfs implementation details like layout
objects (e.g. fanout and chunking).
`,
		LongDescription: `
'btfs file' provides a familiar interface to file systems represented
by BTFS objects, which hides btfs implementation details like layout
objects (e.g. fanout and chunking).
`,
	},

	Subcommands: map[string]*cmds.Command{
		"ls": LsCmd,
	},
}
