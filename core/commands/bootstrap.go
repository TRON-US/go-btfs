package commands

import (
	"errors"
	"io"
	"sort"

	cmdenv "github.com/TRON-US/go-btfs/core/commands/cmdenv"
	repo "github.com/TRON-US/go-btfs/repo"
	fsrepo "github.com/TRON-US/go-btfs/repo/fsrepo"

	cmds "github.com/ipfs/go-ipfs-cmds"
	config "github.com/ipfs/go-ipfs-config"
)

type BootstrapOutput struct {
	Peers []string
}

var peerOptionDesc = "A peer to add to the bootstrap list (in the format '<multiaddr>/<peerID>')"

var BootstrapCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Show or edit the list of bootstrap peers.",
		ShortDescription: `
Running 'btfs bootstrap' with no arguments will run 'btfs bootstrap list'.
` + bootstrapSecurityWarning,
	},

	Run:      bootstrapListCmd.Run,
	Encoders: bootstrapListCmd.Encoders,
	Type:     bootstrapListCmd.Type,

	Subcommands: map[string]*cmds.Command{
		"list": bootstrapListCmd,
		"add":  bootstrapAddCmd,
		"rm":   bootstrapRemoveCmd,
	},
}

const (
	defaultOptionName = "default"
)

var bootstrapAddCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Add peers to the bootstrap list.",
		ShortDescription: `Outputs a list of peers that were added (that weren't already
in the bootstrap list).
` + bootstrapSecurityWarning,
	},

	Arguments: []cmds.Argument{
		cmds.StringArg("peer", false, true, peerOptionDesc).EnableStdin(),
	},

	Options: []cmds.Option{
		cmds.BoolOption(defaultOptionName, "Add default bootstrap nodes. (Deprecated, use 'default' subcommand instead)"),
	},
	Subcommands: map[string]*cmds.Command{
		"default": bootstrapAddDefaultCmd,
	},

	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		deflt, _ := req.Options[defaultOptionName].(bool)

		var inputPeers []config.BootstrapPeer
		if deflt {
			// parse separately for meaningful, correct error.
			defltPeers, err := config.DefaultBootstrapPeers()
			if err != nil {
				return err
			}

			inputPeers = defltPeers
		} else {
			if err := req.ParseBodyArgs(); err != nil {
				return err
			}

			parsedPeers, err := config.ParseBootstrapPeers(req.Arguments)
			if err != nil {
				return err
			}

			inputPeers = parsedPeers
		}

		if len(inputPeers) == 0 {
			return errors.New("no bootstrap peers to add")
		}

		cfgRoot, err := cmdenv.GetConfigRoot(env)
		if err != nil {
			return err
		}

		r, err := fsrepo.Open(cfgRoot)
		if err != nil {
			return err
		}
		defer r.Close()
		cfg, err := r.Config()
		if err != nil {
			return err
		}

		added, err := bootstrapAdd(r, cfg, inputPeers)
		if err != nil {
			return err
		}

		return cmds.EmitOnce(res, &BootstrapOutput{config.BootstrapPeerStrings(added)})
	},
	Type: BootstrapOutput{},
	Encoders: cmds.EncoderMap{
		cmds.Text: cmds.MakeTypedEncoder(func(req *cmds.Request, w io.Writer, out *BootstrapOutput) error {
			return bootstrapWritePeers(w, "added ", out.Peers)
		}),
	},
}

var bootstrapAddDefaultCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Add default peers to the bootstrap list.",
		ShortDescription: `Outputs a list of peers that were added (that weren't already
in the bootstrap list).`,
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		defltPeers, err := config.DefaultBootstrapPeers()
		if err != nil {
			return err
		}

		cfgRoot, err := cmdenv.GetConfigRoot(env)
		if err != nil {
			return err
		}

		r, err := fsrepo.Open(cfgRoot)
		if err != nil {
			return err
		}

		defer r.Close()
		cfg, err := r.Config()
		if err != nil {
			return err
		}

		added, err := bootstrapAdd(r, cfg, defltPeers)
		if err != nil {
			return err
		}

		return cmds.EmitOnce(res, &BootstrapOutput{config.BootstrapPeerStrings(added)})
	},
	Type: BootstrapOutput{},
	Encoders: cmds.EncoderMap{
		cmds.Text: cmds.MakeTypedEncoder(func(req *cmds.Request, w io.Writer, out *BootstrapOutput) error {
			return bootstrapWritePeers(w, "added ", out.Peers)
		}),
	},
}

const (
	bootstrapAllOptionName = "all"
)

var bootstrapRemoveCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Remove peers from the bootstrap list.",
		ShortDescription: `Outputs the list of peers that were removed.
` + bootstrapSecurityWarning,
	},

	Arguments: []cmds.Argument{
		cmds.StringArg("peer", false, true, peerOptionDesc).EnableStdin(),
	},
	Options: []cmds.Option{
		cmds.BoolOption(bootstrapAllOptionName, "Remove all bootstrap peers. (Deprecated, use 'all' subcommand)"),
	},
	Subcommands: map[string]*cmds.Command{
		"all": bootstrapRemoveAllCmd,
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		all, _ := req.Options[bootstrapAllOptionName].(bool)

		cfgRoot, err := cmdenv.GetConfigRoot(env)
		if err != nil {
			return err
		}

		r, err := fsrepo.Open(cfgRoot)
		if err != nil {
			return err
		}
		defer r.Close()
		cfg, err := r.Config()
		if err != nil {
			return err
		}

		var removed []config.BootstrapPeer
		if all {
			removed, err = bootstrapRemoveAll(r, cfg)
		} else {
			if err := req.ParseBodyArgs(); err != nil {
				return err
			}

			input, perr := config.ParseBootstrapPeers(req.Arguments)
			if perr != nil {
				return perr
			}

			removed, err = bootstrapRemove(r, cfg, input)
		}
		if err != nil {
			return err
		}

		return cmds.EmitOnce(res, &BootstrapOutput{config.BootstrapPeerStrings(removed)})
	},
	Type: BootstrapOutput{},
	Encoders: cmds.EncoderMap{
		cmds.Text: cmds.MakeTypedEncoder(func(req *cmds.Request, w io.Writer, out *BootstrapOutput) error {
			return bootstrapWritePeers(w, "removed ", out.Peers)
		}),
	},
}

var bootstrapRemoveAllCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline:          "Remove all peers from the bootstrap list.",
		ShortDescription: `Outputs the list of peers that were removed.`,
	},

	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		cfgRoot, err := cmdenv.GetConfigRoot(env)
		if err != nil {
			return err
		}

		r, err := fsrepo.Open(cfgRoot)
		if err != nil {
			return err
		}
		defer r.Close()
		cfg, err := r.Config()
		if err != nil {
			return err
		}

		removed, err := bootstrapRemoveAll(r, cfg)
		if err != nil {
			return err
		}

		return cmds.EmitOnce(res, &BootstrapOutput{config.BootstrapPeerStrings(removed)})
	},
	Type: BootstrapOutput{},
	Encoders: cmds.EncoderMap{
		cmds.Text: cmds.MakeTypedEncoder(func(req *cmds.Request, w io.Writer, out *BootstrapOutput) error {
			return bootstrapWritePeers(w, "removed ", out.Peers)
		}),
	},
}

var bootstrapListCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline:          "Show peers in the bootstrap list.",
		ShortDescription: "Peers are output in the format '<multiaddr>/<peerID>'.",
	},

	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		cfgRoot, err := cmdenv.GetConfigRoot(env)
		if err != nil {
			return err
		}

		r, err := fsrepo.Open(cfgRoot)
		if err != nil {
			return err
		}
		defer r.Close()
		cfg, err := r.Config()
		if err != nil {
			return err
		}

		peers, err := cfg.BootstrapPeers()
		if err != nil {
			return err
		}

		return cmds.EmitOnce(res, &BootstrapOutput{config.BootstrapPeerStrings(peers)})
	},
	Type: BootstrapOutput{},
	Encoders: cmds.EncoderMap{
		cmds.Text: cmds.MakeTypedEncoder(func(req *cmds.Request, w io.Writer, out *BootstrapOutput) error {
			return bootstrapWritePeers(w, "", out.Peers)
		}),
	},
}

func bootstrapWritePeers(w io.Writer, prefix string, peers []string) error {
	sort.Stable(sort.StringSlice(peers))
	for _, peer := range peers {
		_, err := w.Write([]byte(prefix + peer + "\n"))
		if err != nil {
			return err
		}
	}
	return nil
}

func bootstrapAdd(r repo.Repo, cfg *config.Config, peers []config.BootstrapPeer) ([]config.BootstrapPeer, error) {
	addedMap := map[string]struct{}{}
	addedList := make([]config.BootstrapPeer, 0, len(peers))

	// re-add cfg bootstrap peers to rm dupes
	bpeers := cfg.Bootstrap
	cfg.Bootstrap = nil

	// add new peers
	for _, peer := range peers {
		s := peer.String()
		if _, found := addedMap[s]; found {
			continue
		}

		cfg.Bootstrap = append(cfg.Bootstrap, s)
		addedList = append(addedList, peer)
		addedMap[s] = struct{}{}
	}

	// add back original peers. in this order so that we output them.
	for _, s := range bpeers {
		if _, found := addedMap[s]; found {
			continue
		}

		cfg.Bootstrap = append(cfg.Bootstrap, s)
		addedMap[s] = struct{}{}
	}

	if err := r.SetConfig(cfg); err != nil {
		return nil, err
	}

	return addedList, nil
}

func bootstrapRemove(r repo.Repo, cfg *config.Config, toRemove []config.BootstrapPeer) ([]config.BootstrapPeer, error) {
	removed := make([]config.BootstrapPeer, 0, len(toRemove))
	keep := make([]config.BootstrapPeer, 0, len(cfg.Bootstrap))

	peers, err := cfg.BootstrapPeers()
	if err != nil {
		return nil, err
	}

	for _, peer := range peers {
		found := false
		for _, peer2 := range toRemove {
			if peer.Equal(peer2) {
				found = true
				removed = append(removed, peer)
				break
			}
		}

		if !found {
			keep = append(keep, peer)
		}
	}
	cfg.SetBootstrapPeers(keep)

	if err := r.SetConfig(cfg); err != nil {
		return nil, err
	}

	return removed, nil
}

func bootstrapRemoveAll(r repo.Repo, cfg *config.Config) ([]config.BootstrapPeer, error) {
	removed, err := cfg.BootstrapPeers()
	if err != nil {
		return nil, err
	}

	cfg.Bootstrap = nil
	if err := r.SetConfig(cfg); err != nil {
		return nil, err
	}

	return removed, nil
}

const bootstrapSecurityWarning = `
SECURITY WARNING:

The bootstrap command manipulates the "bootstrap list", which contains
the addresses of bootstrap nodes. These are the *trusted peers* from
which to learn about other peers in the network. Only edit this list
if you understand the risks of adding or removing nodes from this list.

`
