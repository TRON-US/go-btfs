package coreapi

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"reflect"

	"github.com/TRON-US/go-btfs/core"

	"github.com/TRON-US/go-btfs/core/coreunix"

	chunker "github.com/TRON-US/go-btfs-chunker"
	files "github.com/TRON-US/go-btfs-files"
	ecies "github.com/TRON-US/go-eccrypto"
	"github.com/TRON-US/go-mfs"
	ft "github.com/TRON-US/go-unixfs"
	unixfile "github.com/TRON-US/go-unixfs/file"
	"github.com/TRON-US/go-unixfs/importer/helpers"
	uio "github.com/TRON-US/go-unixfs/io"
	ftutil "github.com/TRON-US/go-unixfs/util"
	coreiface "github.com/TRON-US/interface-go-btfs-core"
	"github.com/TRON-US/interface-go-btfs-core/options"
	"github.com/TRON-US/interface-go-btfs-core/path"
	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-cidutil"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	ipld "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-merkledag"
	dag "github.com/ipfs/go-merkledag"
	dagtest "github.com/ipfs/go-merkledag/test"
	"github.com/libp2p/go-libp2p-core/peer"
)

type UnixfsAPI CoreAPI

// Add builds a merkledag node from a reader, adds it to the blockstore,
// and returns the key representing that node.
func (api *UnixfsAPI) Add(ctx context.Context, node files.Node, opts ...options.UnixfsAddOption) (path.Resolved, error) {
	settings, prefix, err := options.UnixfsAddOptions(opts...)
	if err != nil {
		return nil, err
	}

	cfg, err := api.repo.Config()
	if err != nil {
		return nil, err
	}

	// check if repo will exceed storage limit if added
	// TODO: this doesn't handle the case if the hashed file is already in blocks (deduplicated)
	// TODO: conditional GC is disabled due to it is somehow not possible to pass the size to the daemon
	//if err := corerepo.ConditionalGC(req.Context(), n, uint64(size)); err != nil {
	//	res.SetError(err, cmds.ErrNormal)
	//	return
	//}

	if settings.NoCopy && !(cfg.Experimental.FilestoreEnabled || cfg.Experimental.UrlstoreEnabled) {
		return nil, fmt.Errorf("either the filestore or the urlstore must be enabled to use nocopy, see: https://git.io/vNItf")
	}

	addblockstore := api.blockstore
	if !(settings.FsCache || settings.NoCopy) {
		addblockstore = bstore.NewGCBlockstore(api.baseBlocks, api.blockstore)
	}
	exch := api.exchange
	pinning := api.pinning

	if settings.OnlyHash {
		nilnode, err := core.NewNode(ctx, &core.BuildCfg{
			//TODO: need this to be true or all files
			// hashed will be stored in memory!
			NilRepo: true,
		})
		if err != nil {
			return nil, err
		}
		addblockstore = nilnode.Blockstore
		exch = nilnode.Exchange
		pinning = nilnode.Pinning
	}

	bserv := blockservice.New(addblockstore, exch) // hash security 001
	dserv := dag.NewDAGService(bserv)

	fileAdder, err := coreunix.NewAdder(ctx, pinning, addblockstore, dserv)
	if err != nil {
		return nil, err
	}

	fileAdder.Chunker = settings.Chunker
	if settings.Events != nil {
		fileAdder.Out = settings.Events
		fileAdder.Progress = settings.Progress
	}
	fileAdder.Pin = settings.Pin && !settings.OnlyHash
	fileAdder.Silent = settings.Silent
	fileAdder.RawLeaves = settings.RawLeaves
	fileAdder.NoCopy = settings.NoCopy
	fileAdder.CidBuilder = prefix

	switch settings.Layout {
	case options.BalancedLayout:
		// Default
	case options.TrickleLayout:
		fileAdder.Trickle = true
	default:
		return nil, fmt.Errorf("unknown layout: %d", settings.Layout)
	}

	if settings.Inline {
		fileAdder.CidBuilder = cidutil.InlineBuilder{
			Builder: fileAdder.CidBuilder,
			Limit:   settings.InlineLimit,
		}
	}

	if settings.OnlyHash {
		md := dagtest.Mock()
		emptyDirNode := ft.EmptyDirNode()
		// Use the same prefix for the "empty" MFS root as for the file adder.
		emptyDirNode.SetCidBuilder(fileAdder.CidBuilder)
		mr, err := mfs.NewRoot(ctx, md, emptyDirNode, nil)
		if err != nil {
			return nil, err
		}

		fileAdder.SetMfsRoot(mr)
	}

	if settings.Encrypt {
		pubKey := settings.Pubkey
		if pubKey == "" {
			peerId := settings.PeerId
			if peerId == "" {
				peerId = api.identity.Pretty()
			}
			pubKey, err = peerId2pubkey(peerId)
			if err != nil {
				return nil, err
			}
		}
		log.Infof("The file will be encrypted with pubkey: %s", settings.Pubkey)
		switch f := node.(type) {
		case files.File:
			bytes, err := ioutil.ReadAll(f)
			if err != nil {
				return nil, err
			}

			ciphertext, metadata, err := ecies.Encrypt(pubKey, bytes)
			if err != nil {
				return nil, err
			}
			m := make(map[string]interface{})
			m["Iv"] = metadata.Iv
			m["EphemPublicKey"] = metadata.EphemPublicKey
			m["Mac"] = metadata.Mac
			m["Mode"] = metadata.Mode
			if err != nil {
				return nil, err
			}

			settings.TokenMetadata, err = api.appendMetaMap(settings.TokenMetadata, m)
			if err != nil {
				return nil, err
			}
			node = files.NewBytesFile([]byte(ciphertext))
		default:
			return nil, notSupport(f)
		}
	}

	if settings.PinDuration != 0 {
		fileAdder.PinDuration = settings.PinDuration
	}
	// This block is intentionally placed here so that
	// any execution case can append metadata
	if settings.TokenMetadata != "" {
		fileAdder.TokenMetadata = settings.TokenMetadata
		if _, ok := node.(files.Directory); ok {
			fileAdder.MetaForDirectory = true
		}
	}

	// if chunker is reed-solomon and the given `node` is directory,
	// create ReedSolomonAdder. Otherwise use Adder.
	var nd ipld.Node
	_, ok := node.(files.Directory)
	if ok && chunker.IsReedSolomon(settings.Chunker) {
		rsfileAdder, err := coreunix.NewReedSolomonAdder(fileAdder)
		if err != nil {
			return nil, err
		}
		rsfileAdder.IsDir = true
		nd, err = rsfileAdder.AddAllAndPin(node)
	} else {
		nd, err = fileAdder.AddAllAndPin(node)
	}
	if err != nil {
		return nil, err
	}

	if err := api.provider.Provide(nd.Cid()); err != nil {
		return nil, err
	}

	return path.IpfsPath(nd.Cid()), nil
}

func notSupport(f interface{}) error {
	return fmt.Errorf("not support: %v", f)
}

func peerId2pubkey(peerId string) (string, error) {
	id, err := peer.IDB58Decode(peerId)
	if err != nil {
		return "", err
	}
	publicKey, err := id.ExtractPublicKey()
	if err != nil {
		return "", err
	}

	bytes, err := publicKey.Bytes()
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes[4:]), nil
}

func (api *UnixfsAPI) Get(ctx context.Context, p path.Path, opts ...options.UnixfsGetOption) (files.Node, error) {
	settings, err := options.UnixfsGetOptions(opts...)
	if err != nil {
		return nil, err
	}
	ses := api.core().getSession(ctx)

	nd, err := ses.ResolveNode(ctx, p)
	if err != nil {
		return nil, err
	}

	var node files.Node
	if !settings.Metadata && settings.Decrypt {
		node, err = unixfile.NewUnixfsFile(ctx, ses.dag, nd,
			unixfile.UnixfsFileOptions{RepairShards: settings.Repairs})
		switch f := node.(type) {
		case files.File:
			privKey, err := api.getPrivateKey(settings.PrivateKey)
			if err != nil {
				return nil, err
			}

			mbytes, err := api.GetMetadata(ctx, p)
			if err != nil {
				return nil, err
			}

			t := &ecies.EciesMetadata{}
			err = json.Unmarshal(mbytes, t)
			if err != nil {
				return nil, err
			}

			bytes, err := ioutil.ReadAll(f)
			if err != nil {
				return nil, err
			}

			s, err := ecies.Decrypt(privKey, string(bytes), t)
			if err != nil {
				return nil, err
			}
			node = files.NewBytesFile([]byte(s))
		default:
			return nil, notSupport(f)
		}
	} else {
		var ds ipld.DAGService
		// If needing repair, the dag has to be writable
		if settings.Repairs != nil {
			ds = api.dag
		} else {
			ds = ses.dag
		}
		node, err = unixfile.NewUnixfsFile(ctx, ds, nd,
			unixfile.UnixfsFileOptions{Meta: settings.Metadata, RepairShards: settings.Repairs})
		if settings.Metadata {
			f, ok := node.(files.File)
			if !ok {
				return nil, notSupport(f)
			}
			inb, err := ftutil.ReadMetadataElementFromDag(ctx, nd, api.dag, settings.Metadata)
			if err != nil {
				return nil, err
			}

			m := make(map[string]interface{})
			err = json.Unmarshal(inb, &m)
			if err != nil {
				return nil, err
			}

			// Delete SuperMeta entries from the map.
			var superMeta helpers.SuperMeta
			typ := reflect.TypeOf(superMeta)
			for i := 0; i < typ.NumField(); i++ {
				delete(m, typ.Field(i).Name)
			}

			outb, err := json.Marshal(m)
			if err != nil {
				return nil, err
			}
			node = files.NewBytesFile(outb)
		}
	}

	return node, err
}

// compatible with base64, 32bits-hex and 36bits-hex
func (api *UnixfsAPI) getPrivateKey(input string) (string, error) {
	privKey := input
	var bytes []byte
	var err error
	if privKey == "" {
		bytes, err = api.privateKey.Bytes()
		if err != nil {
			return "", err
		}
	} else if isPath(privKey) {
		bytes, err = ioutil.ReadFile(privKey)
		if err != nil {
			return "", err
		}
		bytes, err = decodePrivateKey(string(bytes))
		if err != nil {
			return "", err
		}
	} else {
		bytes, err = decodePrivateKey(privKey)
		if err != nil {
			return "", err
		}
	}
	if len(bytes) < 32 {
		return "", fmt.Errorf("invalid private key")
	}
	privKey = hex.EncodeToString(bytes[4:])
	return privKey, nil
}

func isPath(path string) bool {
	if path == "" {
		return false
	}
	fc := path[0]
	return fc == '.' || fc == '/' || fc == '~'
}

func decodePrivateKey(privKey string) ([]byte, error) {
	bytes, err := base64.StdEncoding.DecodeString(privKey)
	if err != nil {
		bytes, err = hex.DecodeString(privKey)
		if err != nil {
			return nil, err
		}
	}
	return bytes, nil
}

// Ls returns the contents of an BTFS or BTNS object(s) at path p, with the format:
// `<link base58 hash> <link size in bytes> <link name>`
func (api *UnixfsAPI) Ls(ctx context.Context, p path.Path, opts ...options.UnixfsLsOption) (<-chan coreiface.DirEntry, error) {
	settings, err := options.UnixfsLsOptions(opts...)
	if err != nil {
		return nil, err
	}

	ses := api.core().getSession(ctx)
	uses := (*UnixfsAPI)(ses)

	dagnode, err := ses.ResolveNode(ctx, p)
	if err != nil {
		return nil, err
	}

	dir, err := uio.NewDirectoryFromNode(ses.dag, dagnode)
	if err == uio.ErrNotADir {
		return uses.lsFromLinks(ctx, dagnode.Links(), settings)
	}
	if err != nil {
		return nil, err
	}

	return uses.lsFromLinksAsync(ctx, dir, settings)
}

func (api *UnixfsAPI) AddMetadata(ctx context.Context, p path.Path, m string, opts ...options.UnixfsAddMetaOption) (path.Resolved, error) {
	settings, err := options.UnixfsAddMetaOptions(opts...)
	if err != nil {
		return nil, err
	}

	addblockstore := api.blockstore
	exch := api.exchange
	pinning := api.pinning

	bserv := blockservice.New(addblockstore, exch) // hash security 001
	dserv := dag.NewDAGService(bserv)

	metaModifier, err := coreunix.NewMetaModifier(ctx, pinning, addblockstore, dserv)
	if err != nil {
		return nil, err
	}

	if m != "" {
		metaModifier.TokenMetadata = m
	}

	metaModifier.Pin = settings.Pin

	ses := api.core().getSession(ctx)

	nd, err := ses.ResolveNode(ctx, p)
	if err != nil {
		return nil, err
	}

	newNode, rp, err := metaModifier.AddMetaAndPin(nd)
	if err != nil {
		return nil, err
	}

	if err := api.provider.Provide(newNode.Cid()); err != nil {
		return nil, err
	}

	// Return a new /btfs resolved path from the CID.
	return rp, nil
}

func (api *UnixfsAPI) RemoveMetadata(ctx context.Context, p path.Path, m string, opts ...options.UnixfsRemoveMetaOption) (path.Resolved, error) {
	settings, err := options.UnixfsRemoveMetaOptions(opts...)
	if err != nil {
		return nil, err
	}

	addblockstore := api.blockstore
	exch := api.exchange
	pinning := api.pinning

	bserv := blockservice.New(addblockstore, exch) // hash security 001
	dserv := dag.NewDAGService(bserv)

	metaModifier, err := coreunix.NewMetaModifier(ctx, pinning, addblockstore, dserv)
	if err != nil {
		return nil, err
	}

	if m != "" {
		metaModifier.TokenMetadata = m
	}

	metaModifier.Pin = settings.Pin

	ses := api.core().getSession(ctx)

	nd, err := ses.ResolveNode(ctx, p)
	if err != nil {
		return nil, err
	}

	newNode, rp, err := metaModifier.RemoveMetaAndPin(nd)
	if err != nil {
		return nil, err
	}

	if err := api.provider.Provide(newNode.Cid()); err != nil {
		return nil, err
	}

	// Return a new /btfs resolved path from the CID.
	return rp, nil
}

func (api *UnixfsAPI) processLink(ctx context.Context, linkres ft.LinkResult, settings *options.UnixfsLsSettings) coreiface.DirEntry {
	lnk := coreiface.DirEntry{
		Name: linkres.Link.Name,
		Cid:  linkres.Link.Cid,
		Err:  linkres.Err,
	}
	if lnk.Err != nil {
		return lnk
	}

	switch lnk.Cid.Type() {
	case cid.Raw:
		// No need to check with raw leaves
		lnk.Type = coreiface.TFile
		lnk.Size = linkres.Link.Size
	case cid.DagProtobuf:
		if !settings.ResolveChildren {
			break
		}

		linkNode, err := linkres.Link.GetNode(ctx, api.dag)
		if err != nil {
			lnk.Err = err
			break
		}

		if pn, ok := linkNode.(*merkledag.ProtoNode); ok {
			d, err := ft.FSNodeFromBytes(pn.Data())
			if err != nil {
				lnk.Err = err
				break
			}
			switch d.Type() {
			case ft.TFile, ft.TRaw:
				lnk.Type = coreiface.TFile
			case ft.THAMTShard, ft.TDirectory, ft.TMetadata:
				lnk.Type = coreiface.TDirectory
			case ft.TSymlink:
				lnk.Type = coreiface.TSymlink
				lnk.Target = string(d.Data())
			}
			lnk.Size = d.FileSize()
		}
	}

	return lnk
}

func (api *UnixfsAPI) lsFromLinksAsync(ctx context.Context, dir uio.Directory, settings *options.UnixfsLsSettings) (<-chan coreiface.DirEntry, error) {
	out := make(chan coreiface.DirEntry)

	go func() {
		defer close(out)
		for l := range dir.EnumLinksAsync(ctx) {
			select {
			case out <- api.processLink(ctx, l, settings): //TODO: perf: processing can be done in background and in parallel
			case <-ctx.Done():
				return
			}
		}
	}()

	return out, nil
}

func (api *UnixfsAPI) lsFromLinks(ctx context.Context, ndlinks []*ipld.Link, settings *options.UnixfsLsSettings) (<-chan coreiface.DirEntry, error) {
	links := make(chan coreiface.DirEntry, len(ndlinks))
	for _, l := range ndlinks {
		lr := ft.LinkResult{Link: &ipld.Link{Name: l.Name, Size: l.Size, Cid: l.Cid}}

		links <- api.processLink(ctx, lr, settings) //TODO: can be parallel if settings.Async
	}
	close(links)
	return links, nil
}

func (api *UnixfsAPI) core() *CoreAPI {
	return (*CoreAPI)(api)
}

func (api *UnixfsAPI) GetMetadata(ctx context.Context, p path.Path) ([]byte, error) {
	f, err := api.core().Unixfs().Get(ctx, p, options.Unixfs.Metadata(true))
	if err != nil {
		return nil, err
	}
	switch f.(type) {
	case files.File:
		bytes, err := ioutil.ReadAll(f.(files.File))
		if err != nil {
			return nil, err
		}
		return bytes, nil
	default:
		return nil, notSupport(f)
	}
}

func (api *UnixfsAPI) appendMetaMap(tokenMetadata string, metaMap map[string]interface{}) (string, error) {
	if metaMap == nil {
		return "", nil
	}

	var b []byte = nil
	var err error = nil
	if tokenMetadata != "" {
		tmp := make(map[string]interface{})
		err = json.Unmarshal([]byte(tokenMetadata), &tmp)
		if err != nil {
			return "", err
		}
		for k, v := range metaMap {
			tmp[k] = v
		}
		b, err = json.Marshal(tmp)
		if err != nil {
			return "", err
		}
	} else {
		b, err = json.Marshal(metaMap)
		if err != nil {
			return "", err
		}
	}

	tokenMetadata = string(b)
	return tokenMetadata, nil
}
