package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	cmdenv "github.com/ipfs/go-ipfs/core/commands/cmdenv"

	cid "github.com/ipfs/go-cid"
	cmdkit "github.com/ipfs/go-ipfs-cmdkit"
	cmds "github.com/ipfs/go-ipfs-cmds"
	ipld "github.com/ipfs/go-ipld-format"
	dag "github.com/ipfs/go-merkledag"
	path "github.com/ipfs/go-path"
	peer "github.com/libp2p/go-libp2p-peer"
	pstore "github.com/libp2p/go-libp2p-peerstore"
	routing "github.com/libp2p/go-libp2p-routing"
	notif "github.com/libp2p/go-libp2p-routing/notifications"
	b58 "github.com/mr-tron/base58/base58"
)

var ErrNotDHT = errors.New("routing service is not a DHT")

// TODO: Factor into `btfs dht` and `btfs routing`.
// Everything *except `query` goes into `btfs routing`.

var DhtCmd = &cmds.Command{
	Helptext: cmdkit.HelpText{
		Tagline:          "Issue commands directly through the DHT.",
		ShortDescription: ``,
	},

	Subcommands: map[string]*cmds.Command{
		"query":     queryDhtCmd,
		"findprovs": findProvidersDhtCmd,
		"findpeer":  findPeerDhtCmd,
		"get":       getValueDhtCmd,
		"put":       putValueDhtCmd,
		"provide":   provideRefDhtCmd,
	},
}

const (
	dhtVerboseOptionName = "verbose"
)

var queryDhtCmd = &cmds.Command{
	Helptext: cmdkit.HelpText{
		Tagline:          "Find the closest Peer IDs to a given Peer ID by querying the DHT.",
		ShortDescription: "Outputs a list of newline-delimited Peer IDs.",
	},

	Arguments: []cmdkit.Argument{
		cmdkit.StringArg("peerID", true, true, "The peerID to run the query against."),
	},
	Options: []cmdkit.Option{
		cmdkit.BoolOption(dhtVerboseOptionName, "v", "Print extra information."),
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		nd, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}

		if nd.DHT == nil {
			return ErrNotDHT
		}

		id, err := peer.IDB58Decode(req.Arguments[0])
		if err != nil {
			return cmds.ClientError("invalid peer ID")
		}

		ctx, cancel := context.WithCancel(req.Context)
		ctx, events := notif.RegisterForQueryEvents(ctx)

		closestPeers, err := nd.DHT.GetClosestPeers(ctx, string(id))
		if err != nil {
			cancel()
			return err
		}

		go func() {
			defer cancel()
			for p := range closestPeers {
				notif.PublishQueryEvent(ctx, &notif.QueryEvent{
					ID:   p,
					Type: notif.FinalPeer,
				})
			}
		}()

		for e := range events {
			if err := res.Emit(e); err != nil {
				return err
			}
		}

		return nil
	},
	Encoders: cmds.EncoderMap{
		cmds.Text: cmds.MakeTypedEncoder(func(req *cmds.Request, w io.Writer, out *notif.QueryEvent) error {
			pfm := pfuncMap{
				notif.PeerResponse: func(obj *notif.QueryEvent, out io.Writer, verbose bool) {
					for _, p := range obj.Responses {
						fmt.Fprintf(out, "%s\n", p.ID.Pretty())
					}
				},
			}
			verbose, _ := req.Options[dhtVerboseOptionName].(bool)
			printEvent(out, w, verbose, pfm)
			return nil
		}),
	},
	Type: notif.QueryEvent{},
}

const (
	numProvidersOptionName = "num-providers"
)

var findProvidersDhtCmd = &cmds.Command{
	Helptext: cmdkit.HelpText{
		Tagline:          "Find peers that can provide a specific value, given a key.",
		ShortDescription: "Outputs a list of newline-delimited provider Peer IDs.",
	},

	Arguments: []cmdkit.Argument{
		cmdkit.StringArg("key", true, true, "The key to find providers for."),
	},
	Options: []cmdkit.Option{
		cmdkit.BoolOption(dhtVerboseOptionName, "v", "Print extra information."),
		cmdkit.IntOption(numProvidersOptionName, "n", "The number of providers to find.").WithDefault(20),
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}

		if !n.IsOnline {
			return ErrNotOnline
		}

		numProviders, _ := req.Options[numProvidersOptionName].(int)
		if numProviders < 1 {
			return fmt.Errorf("number of providers must be greater than 0")
		}

		c, err := cid.Parse(req.Arguments[0])

		if err != nil {
			return err
		}

		ctx, cancel := context.WithCancel(req.Context)
		ctx, events := notif.RegisterForQueryEvents(ctx)

		pchan := n.Routing.FindProvidersAsync(ctx, c, numProviders)

		go func() {
			defer cancel()
			for p := range pchan {
				np := p
				notif.PublishQueryEvent(ctx, &notif.QueryEvent{
					Type:      notif.Provider,
					Responses: []*pstore.PeerInfo{&np},
				})
			}
		}()
		for e := range events {
			if err := res.Emit(e); err != nil {
				return err
			}
		}

		return nil
	},
	Encoders: cmds.EncoderMap{
		cmds.Text: cmds.MakeTypedEncoder(func(req *cmds.Request, w io.Writer, out *notif.QueryEvent) error {
			pfm := pfuncMap{
				notif.FinalPeer: func(obj *notif.QueryEvent, out io.Writer, verbose bool) {
					if verbose {
						fmt.Fprintf(out, "* closest peer %s\n", obj.ID)
					}
				},
				notif.Provider: func(obj *notif.QueryEvent, out io.Writer, verbose bool) {
					prov := obj.Responses[0]
					if verbose {
						fmt.Fprintf(out, "provider: ")
					}
					fmt.Fprintf(out, "%s\n", prov.ID.Pretty())
					if verbose {
						for _, a := range prov.Addrs {
							fmt.Fprintf(out, "\t%s\n", a)
						}
					}
				},
			}

			verbose, _ := req.Options[dhtVerboseOptionName].(bool)
			printEvent(out, w, verbose, pfm)

			return nil
		}),
	},
	Type: notif.QueryEvent{},
}

const (
	recursiveOptionName = "recursive"
)

var provideRefDhtCmd = &cmds.Command{
	Helptext: cmdkit.HelpText{
		Tagline: "Announce to the network that you are providing given values.",
	},

	Arguments: []cmdkit.Argument{
		cmdkit.StringArg("key", true, true, "The key[s] to send provide records for.").EnableStdin(),
	},
	Options: []cmdkit.Option{
		cmdkit.BoolOption(dhtVerboseOptionName, "v", "Print extra information."),
		cmdkit.BoolOption(recursiveOptionName, "r", "Recursively provide entire graph."),
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		nd, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}

		if !nd.IsOnline {
			return ErrNotOnline
		}

		if len(nd.PeerHost.Network().Conns()) == 0 {
			return errors.New("cannot provide, no connected peers")
		}

		rec, _ := req.Options[recursiveOptionName].(bool)

		var cids []cid.Cid
		for _, arg := range req.Arguments {
			c, err := cid.Decode(arg)
			if err != nil {
				return err
			}

			has, err := nd.Blockstore.Has(c)
			if err != nil {
				return err
			}

			if !has {
				return fmt.Errorf("block %s not found locally, cannot provide", c)
			}

			cids = append(cids, c)
		}

		ctx, cancel := context.WithCancel(req.Context)
		ctx, events := notif.RegisterForQueryEvents(ctx)

		var provideErr error
		go func() {
			defer cancel()
			if rec {
				provideErr = provideKeysRec(ctx, nd.Routing, nd.DAG, cids)
			} else {
				provideErr = provideKeys(ctx, nd.Routing, cids)
			}
			if provideErr != nil {
				notif.PublishQueryEvent(ctx, &notif.QueryEvent{
					Type:  notif.QueryError,
					Extra: provideErr.Error(),
				})
			}
		}()

		for e := range events {
			if err := res.Emit(e); err != nil {
				return err
			}
		}

		return provideErr
	},
	Encoders: cmds.EncoderMap{
		cmds.Text: cmds.MakeTypedEncoder(func(req *cmds.Request, w io.Writer, out *notif.QueryEvent) error {
			pfm := pfuncMap{
				notif.FinalPeer: func(obj *notif.QueryEvent, out io.Writer, verbose bool) {
					if verbose {
						fmt.Fprintf(out, "sending provider record to peer %s\n", obj.ID)
					}
				},
			}

			verbose, _ := req.Options[dhtVerboseOptionName].(bool)
			printEvent(out, w, verbose, pfm)

			return nil
		}),
	},
	Type: notif.QueryEvent{},
}

func provideKeys(ctx context.Context, r routing.IpfsRouting, cids []cid.Cid) error {
	for _, c := range cids {
		err := r.Provide(ctx, c, true)
		if err != nil {
			return err
		}
	}
	return nil
}

func provideKeysRec(ctx context.Context, r routing.IpfsRouting, dserv ipld.DAGService, cids []cid.Cid) error {
	provided := cid.NewSet()
	for _, c := range cids {
		kset := cid.NewSet()

		err := dag.EnumerateChildrenAsync(ctx, dag.GetLinksDirect(dserv), c, kset.Visit)
		if err != nil {
			return err
		}

		for _, k := range kset.Keys() {
			if provided.Has(k) {
				continue
			}

			err = r.Provide(ctx, k, true)
			if err != nil {
				return err
			}
			provided.Add(k)
		}
	}

	return nil
}

var findPeerDhtCmd = &cmds.Command{
	Helptext: cmdkit.HelpText{
		Tagline:          "Find the multiaddresses associated with a Peer ID.",
		ShortDescription: "Outputs a list of newline-delimited multiaddresses.",
	},

	Arguments: []cmdkit.Argument{
		cmdkit.StringArg("peerID", true, true, "The ID of the peer to search for."),
	},
	Options: []cmdkit.Option{
		cmdkit.BoolOption(dhtVerboseOptionName, "v", "Print extra information."),
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		nd, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}

		if !nd.IsOnline {
			return ErrNotOnline
		}

		pid, err := peer.IDB58Decode(req.Arguments[0])
		if err != nil {
			return err
		}

		ctx, cancel := context.WithCancel(req.Context)
		ctx, events := notif.RegisterForQueryEvents(ctx)

		var findPeerErr error
		go func() {
			defer cancel()
			var pi pstore.PeerInfo
			pi, findPeerErr = nd.Routing.FindPeer(ctx, pid)
			if findPeerErr != nil {
				notif.PublishQueryEvent(ctx, &notif.QueryEvent{
					Type:  notif.QueryError,
					Extra: findPeerErr.Error(),
				})
				return
			}

			notif.PublishQueryEvent(ctx, &notif.QueryEvent{
				Type:      notif.FinalPeer,
				Responses: []*pstore.PeerInfo{&pi},
			})
		}()

		for e := range events {
			if err := res.Emit(e); err != nil {
				return err
			}
		}

		return findPeerErr
	},
	Encoders: cmds.EncoderMap{
		cmds.Text: cmds.MakeTypedEncoder(func(req *cmds.Request, w io.Writer, out *notif.QueryEvent) error {
			pfm := pfuncMap{
				notif.FinalPeer: func(obj *notif.QueryEvent, out io.Writer, verbose bool) {
					pi := obj.Responses[0]
					for _, a := range pi.Addrs {
						fmt.Fprintf(out, "%s\n", a)
					}
				},
			}

			verbose, _ := req.Options[dhtVerboseOptionName].(bool)
			printEvent(out, w, verbose, pfm)
			return nil
		}),
	},
	Type: notif.QueryEvent{},
}

var getValueDhtCmd = &cmds.Command{
	Helptext: cmdkit.HelpText{
		Tagline: "Given a key, query the routing system for its best value.",
		ShortDescription: `
Outputs the best value for the given key.

There may be several different values for a given key stored in the routing
system; in this context 'best' means the record that is most desirable. There is
no one metric for 'best': it depends entirely on the key type. For IPNS, 'best'
is the record that is both valid and has the highest sequence number (freshest).
Different key types can specify other 'best' rules.
`,
	},

	Arguments: []cmdkit.Argument{
		cmdkit.StringArg("key", true, true, "The key to find a value for."),
	},
	Options: []cmdkit.Option{
		cmdkit.BoolOption(dhtVerboseOptionName, "v", "Print extra information."),
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		nd, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}

		if !nd.IsOnline {
			return ErrNotOnline
		}

		dhtkey, err := escapeDhtKey(req.Arguments[0])
		if err != nil {
			return err
		}

		ctx, cancel := context.WithCancel(req.Context)
		ctx, events := notif.RegisterForQueryEvents(ctx)

		var getErr error
		go func() {
			defer cancel()
			var val []byte
			val, getErr = nd.Routing.GetValue(ctx, dhtkey)
			if getErr != nil {
				notif.PublishQueryEvent(ctx, &notif.QueryEvent{
					Type:  notif.QueryError,
					Extra: getErr.Error(),
				})
			} else {
				notif.PublishQueryEvent(ctx, &notif.QueryEvent{
					Type:  notif.Value,
					Extra: string(val),
				})
			}
		}()

		for e := range events {
			if err := res.Emit(e); err != nil {
				return err
			}
		}

		return getErr
	},
	Encoders: cmds.EncoderMap{
		cmds.Text: cmds.MakeTypedEncoder(func(req *cmds.Request, w io.Writer, out *notif.QueryEvent) error {
			pfm := pfuncMap{
				notif.Value: func(obj *notif.QueryEvent, out io.Writer, verbose bool) {
					if verbose {
						fmt.Fprintf(out, "got value: '%s'\n", obj.Extra)
					} else {
						fmt.Fprintln(out, obj.Extra)
					}
				},
			}

			verbose, _ := req.Options[dhtVerboseOptionName].(bool)
			printEvent(out, w, verbose, pfm)

			return nil
		}),
	},
	Type: notif.QueryEvent{},
}

var putValueDhtCmd = &cmds.Command{
	Helptext: cmdkit.HelpText{
		Tagline: "Write a key/value pair to the routing system.",
		ShortDescription: `
Given a key of the form /foo/bar and a value of any form, this will write that
value to the routing system with that key.

Keys have two parts: a keytype (foo) and the key name (bar). IPNS uses the
/ipns keytype, and expects the key name to be a Peer ID. IPNS entries are
specifically formatted (protocol buffer).

You may only use keytypes that are supported in your btfs binary: currently
this is only /ipns. Unless you have a relatively deep understanding of the
go-ipfs routing internals, you likely want to be using 'btfs name publish' instead
of this.

Value is arbitrary text. Standard input can be used to provide value.

NOTE: A value may not exceed 2048 bytes.
`,
	},

	Arguments: []cmdkit.Argument{
		cmdkit.StringArg("key", true, false, "The key to store the value at."),
		cmdkit.StringArg("value", true, false, "The value to store.").EnableStdin(),
	},
	Options: []cmdkit.Option{
		cmdkit.BoolOption(dhtVerboseOptionName, "v", "Print extra information."),
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		nd, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}

		if !nd.IsOnline {
			return ErrNotOnline
		}

		key, err := escapeDhtKey(req.Arguments[0])
		if err != nil {
			return err
		}

		data := req.Arguments[1]

		ctx, cancel := context.WithCancel(req.Context)
		ctx, events := notif.RegisterForQueryEvents(ctx)

		var putErr error
		go func() {
			defer cancel()
			putErr = nd.Routing.PutValue(ctx, key, []byte(data))
			if putErr != nil {
				notif.PublishQueryEvent(ctx, &notif.QueryEvent{
					Type:  notif.QueryError,
					Extra: putErr.Error(),
				})
			}
		}()

		for e := range events {
			if err := res.Emit(e); err != nil {
				return err
			}
		}

		return putErr
	},
	Encoders: cmds.EncoderMap{
		cmds.Text: cmds.MakeTypedEncoder(func(req *cmds.Request, w io.Writer, out *notif.QueryEvent) error {
			pfm := pfuncMap{
				notif.FinalPeer: func(obj *notif.QueryEvent, out io.Writer, verbose bool) {
					if verbose {
						fmt.Fprintf(out, "* closest peer %s\n", obj.ID)
					}
				},
				notif.Value: func(obj *notif.QueryEvent, out io.Writer, verbose bool) {
					fmt.Fprintf(out, "%s\n", obj.ID.Pretty())
				},
			}

			verbose, _ := req.Options[dhtVerboseOptionName].(bool)

			printEvent(out, w, verbose, pfm)

			return nil
		}),
	},
	Type: notif.QueryEvent{},
}

type printFunc func(obj *notif.QueryEvent, out io.Writer, verbose bool)
type pfuncMap map[notif.QueryEventType]printFunc

func printEvent(obj *notif.QueryEvent, out io.Writer, verbose bool, override pfuncMap) {
	if verbose {
		fmt.Fprintf(out, "%s: ", time.Now().Format("15:04:05.000"))
	}

	if override != nil {
		if pf, ok := override[obj.Type]; ok {
			pf(obj, out, verbose)
			return
		}
	}

	switch obj.Type {
	case notif.SendingQuery:
		if verbose {
			fmt.Fprintf(out, "* querying %s\n", obj.ID)
		}
	case notif.Value:
		if verbose {
			fmt.Fprintf(out, "got value: '%s'\n", obj.Extra)
		} else {
			fmt.Fprint(out, obj.Extra)
		}
	case notif.PeerResponse:
		if verbose {
			fmt.Fprintf(out, "* %s says use ", obj.ID)
			for _, p := range obj.Responses {
				fmt.Fprintf(out, "%s ", p.ID)
			}
			fmt.Fprintln(out)
		}
	case notif.QueryError:
		if verbose {
			fmt.Fprintf(out, "error: %s\n", obj.Extra)
		}
	case notif.DialingPeer:
		if verbose {
			fmt.Fprintf(out, "dialing peer: %s\n", obj.ID)
		}
	case notif.AddingPeer:
		if verbose {
			fmt.Fprintf(out, "adding peer to query: %s\n", obj.ID)
		}
	case notif.FinalPeer:
	default:
		if verbose {
			fmt.Fprintf(out, "unrecognized event type: %d\n", obj.Type)
		}
	}
}

func escapeDhtKey(s string) (string, error) {
	parts := path.SplitList(s)
	switch len(parts) {
	case 1:
		k, err := b58.Decode(s)
		if err != nil {
			return "", err
		}
		return string(k), nil
	case 3:
		k, err := b58.Decode(parts[2])
		if err != nil {
			return "", err
		}
		return path.Join(append(parts[:2], string(k))), nil
	default:
		return "", errors.New("invalid key")
	}
}
