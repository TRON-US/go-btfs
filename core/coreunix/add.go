package coreunix

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	gopath "path"
	"strconv"

	chunker "github.com/TRON-US/go-btfs-chunker"
	"github.com/TRON-US/go-btfs-files"
	"github.com/TRON-US/go-btfs/pin"
	"github.com/TRON-US/go-mfs"
	"github.com/TRON-US/go-unixfs"
	"github.com/TRON-US/go-unixfs/importer/balanced"
	ihelper "github.com/TRON-US/go-unixfs/importer/helpers"
	"github.com/TRON-US/go-unixfs/importer/trickle"
	uio "github.com/TRON-US/go-unixfs/io"
	ftutil "github.com/TRON-US/go-unixfs/util"
	coreiface "github.com/TRON-US/interface-go-btfs-core"
	"github.com/TRON-US/interface-go-btfs-core/path"
	"github.com/ipfs/go-cid"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	"github.com/ipfs/go-ipfs-posinfo"
	ipld "github.com/ipfs/go-ipld-format"
	logging "github.com/ipfs/go-log"
	dag "github.com/ipfs/go-merkledag"
)

var log = logging.Logger("coreunix")

// how many bytes of progress to wait before sending a progress update message
const progressReaderIncrement = 1024 * 256

var liveCacheSize = uint64(256 << 10)

type Link struct {
	Name, Hash string
	Size       uint64
}

// NewAdder Returns a new Adder used for a file add operation.
func NewAdder(ctx context.Context, p pin.Pinner, bs bstore.GCLocker, ds ipld.DAGService) (*Adder, error) {
	bufferedDS := ipld.NewBufferedDAG(ctx, ds)

	return &Adder{
		ctx:              ctx,
		pinning:          p,
		gcLocker:         bs,
		dagService:       ds,
		bufferedDS:       bufferedDS,
		Progress:         false,
		Pin:              true,
		Trickle:          false,
		MetaForDirectory: false,
		MetaDagToAdd:     false,
		MetadataDag:      nil,
		db:               nil,
		Chunker:          "",
		TokenMetadata:    "",
		PinDuration:      0,
	}, nil
}

// Adder holds the switches passed to the `add` command.
type Adder struct {
	ctx              context.Context
	pinning          pin.Pinner
	gcLocker         bstore.GCLocker
	dagService       ipld.DAGService
	bufferedDS       *ipld.BufferedDAG
	Out              chan<- interface{}
	Progress         bool
	Pin              bool
	Trickle          bool
	RawLeaves        bool
	Silent           bool
	NoCopy           bool
	MetaForDirectory bool
	MetaDagToAdd     bool
	MetadataDag      ipld.Node
	db               *ihelper.DagBuilderHelper
	Chunker          string
	mroot            *mfs.Root
	unlocker         bstore.Unlocker
	tempRoot         cid.Cid
	CidBuilder       cid.Builder
	liveNodes        uint64
	TokenMetadata    string
	PinDuration      int64
}

func (adder *Adder) GcLocker() bstore.GCLocker {
	return adder.gcLocker
}

func (adder *Adder) mfsRoot() (*mfs.Root, error) {
	if adder.mroot != nil {
		return adder.mroot, nil
	}
	rnode := unixfs.EmptyDirNode()
	rnode.SetCidBuilder(adder.CidBuilder)
	mr, err := mfs.NewRoot(adder.ctx, adder.dagService, rnode, nil)
	if err != nil {
		return nil, err
	}
	adder.mroot = mr
	return adder.mroot, nil
}

// SetMfsRoot sets `r` as the root for Adder.
func (adder *Adder) SetMfsRoot(r *mfs.Root) {
	adder.mroot = r
}

// Constructs a node from reader's data, and adds it. Doesn't pin.
func (adder *Adder) add(reader io.Reader, dirTreeBytes []byte) (ipld.Node, error) {
	chnk, err := chunker.FromString(reader, adder.Chunker)
	if err != nil {
		return nil, err
	}
	if dirTreeBytes != nil {
		chnk.SetIsDir(true)
	}

	var metaBytes []byte
	if adder.TokenMetadata != "" {
		metaBytes, err = adder.convertMetadataToBytes(true)
		if err != nil {
			return nil, err
		}
	}
	// This `if conditional statement` makes sure this block is
	// executed only one time for directory addition use case.
	if adder.MetadataDag == nil {
		// Only append metadata if it's available
		if md := chnk.MetaData(); md != nil {
			metaBytes, err = adder.appendMetadataObject(metaBytes, md)
			if err != nil {
				return nil, err
			}
		}
	}

	// Add SuperMeta if metaBytes is not nil
	chunkSize := chnk.ChunkSize()
	if chunkSize == 0 {
		chunkSize = uint64(ihelper.DefaultLinksPerBlock)
	}
	if metaBytes != nil {
		superMeta := &ihelper.SuperMeta{
			ChunkSize:     chnk.ChunkSize(),
			MaxLinks:      uint64(ihelper.DefaultLinksPerBlock),
			TrickleFormat: adder.Trickle,
		}
		metaBytes, err = adder.appendMetadataObject(metaBytes, superMeta)
		if err != nil {
			return nil, err
		}
	}

	// Merge regular metadata and encoded directory tree.
	metaBytes = ftutil.CreateMetadataList(metaBytes, dirTreeBytes)

	params := ihelper.DagBuilderParams{
		Dagserv:       adder.bufferedDS,
		RawLeaves:     adder.RawLeaves,
		Maxlinks:      ihelper.DefaultLinksPerBlock,
		NoCopy:        adder.NoCopy,
		CidBuilder:    adder.CidBuilder,
		TokenMetadata: metaBytes,
		ChunkSize:     chunkSize,
	}

	db, err := params.New(chnk)
	if err != nil {
		return nil, err
	}
	var nd ipld.Node
	if adder.Trickle {
		nd, err = trickle.Layout(db)
	} else {
		if adder.metaDagToBuild(db) {
			metaDag, err := balanced.BuildMetadataDag(db)
			if err != nil {
				return nil, err
			}
			db.SetMetaDagBuilt(true)
			if adder.MetaForDirectory {
				adder.MetadataDag = metaDag
				adder.db = db
				adder.MetaDagToAdd = true
				adder.TokenMetadata = ""
				// The next code line sets nil to `db.metaDb`.
				// The newly created metadada DAG `metaDag` will be
				// attached to the directory root instead.
				if dirTreeBytes == nil {
					db.SetMetaDb(nil)
				}
			}
		}
		nd, err = balanced.Layout(db)
	}
	if err != nil {
		return nil, err
	}

	return nd, adder.bufferedDS.Commit()
}

func (adder *Adder) metaDagToBuild(db *ihelper.DagBuilderHelper) bool {
	if !adder.MetaForDirectory {
		return db.IsThereMetaData() && !db.IsMetaDagBuilt()
	} else {
		return adder.MetadataDag == nil
	}
}

func (adder *Adder) metaDagToAddToDirectory() bool {
	if !adder.MetaForDirectory {
		return false
	} else {
		return adder.MetadataDag != nil && adder.MetaDagToAdd
	}
}

// RootNode returns the mfs root node
func (adder *Adder) curRootNode() (ipld.Node, error) {
	mr, err := adder.mfsRoot()
	if err != nil {
		return nil, err
	}
	root, err := mr.GetDirectory().GetNode()
	if err != nil {
		return nil, err
	}

	// if one root file, use that hash as root.
	if len(root.Links()) == 1 {
		nd, err := root.Links()[0].GetNode(adder.ctx, adder.dagService)
		if err != nil {
			return nil, err
		}

		root = nd
	}

	return root, err
}

// Recursively pins the root node of Adder and
// writes the pin state to the backing datastore.
func (adder *Adder) PinRoot(root ipld.Node) error {
	if !adder.Pin {
		return nil
	}

	rnk := root.Cid()

	err := adder.dagService.Add(adder.ctx, root)
	if err != nil {
		return err
	}

	if adder.tempRoot.Defined() {
		err := adder.pinning.Unpin(adder.ctx, adder.tempRoot, true)
		if err != nil {
			return err
		}
		adder.tempRoot = rnk
	}

	dur, err := pin.ExpiresAtWithUnitAndCount(pin.DefaultDurationUnit, adder.PinDuration)
	if err != nil {
		return err
	}
	adder.pinning.PinWithMode(rnk, dur, pin.Recursive)
	return adder.pinning.Flush()
}

// outputDirs outputs directory dagnodes in a postorder DFS pattern.
func (adder *Adder) outputDirs(path string, fsn mfs.FSNode) error {
	switch fsn := fsn.(type) {
	case *mfs.File:
		return nil
	case *mfs.Directory:
		names, err := fsn.ListNames(adder.ctx)
		if err != nil {
			return err
		}

		for _, name := range names {
			child, err := fsn.Child(name)
			if err != nil {
				return err
			}

			childpath := gopath.Join(path, name)
			err = adder.outputDirs(childpath, child)
			if err != nil {
				return err
			}

			fsn.Uncache(name)
		}
		nd, err := fsn.GetNode()
		if err != nil {
			return err
		}

		return outputDagnode(adder.Out, path, nd)
	default:
		return fmt.Errorf("unrecognized fsn type: %#v", fsn)
	}
}

func (adder *Adder) addNode(node ipld.Node, path string) error {
	// patch it into the root
	if path == "" {
		path = node.Cid().String()
	}

	if pi, ok := node.(*posinfo.FilestoreNode); ok {
		node = pi.Node
	}

	mr, err := adder.mfsRoot()
	if err != nil {
		return err
	}
	dir := gopath.Dir(path)
	if dir != "." { // No wrap case.
		opts := mfs.MkdirOpts{
			Mkparents:  true,
			Flush:      false,
			CidBuilder: adder.CidBuilder,
		}
		if err := mfs.Mkdir(mr, dir, opts); err != nil {
			return err
		}
	}

	// TODO: verify adder is single-threaded.
	if adder.metaDagToAddToDirectory() {
		// Add metadata DAG as the first child of the root.
		// Use uio.SmallestString since directory node links are
		// sorted lexicographically by their names when cid of the directory node is generated.
		// This will make sure metadata DAG is always the first child.
		if err := mfs.PutNode(mr, uio.SmallestString, adder.MetadataDag); err != nil {
			return err
		}
		adder.MetaDagToAdd = false
	}
	if err := mfs.PutNode(mr, path, node); err != nil {
		return err
	}

	if !adder.Silent {
		return outputDagnode(adder.Out, path, node)
	}
	return nil
}

// AddAllAndPin adds the given request's files and pin them.
func (adder *Adder) AddAllAndPin(file files.Node) (ipld.Node, error) {
	if adder.Pin {
		adder.unlocker = adder.gcLocker.PinLock()
	}
	defer func() {
		if adder.unlocker != nil {
			adder.unlocker.Unlock()
		}
	}()

	if err := adder.addFileNode("", file, true); err != nil {
		return nil, err
	}

	nd, err := adder.addToMfs(file)
	if err != nil {
		return nil, err
	}

	if !adder.Pin {
		return nd, nil
	}
	return nd, adder.PinRoot(nd)
}

// addToMfs adds the given file(s) to MFS and return the root.
func (adder *Adder) addToMfs(file files.Node) (ipld.Node, error) {

	// get root
	mr, err := adder.mfsRoot()
	if err != nil {
		return nil, err
	}
	var root mfs.FSNode
	rootdir := mr.GetDirectory()
	root = rootdir

	err = root.Flush()
	if err != nil {
		return nil, err
	}

	// if adding a file without wrapping, swap the root to it (when adding a
	// directory, mfs root is the directory)
	_, dir := file.(files.Directory)
	var name string
	if !dir {
		children, err := rootdir.ListNames(adder.ctx)
		if err != nil {
			return nil, err
		}

		if len(children) == 0 {
			return nil, fmt.Errorf("expected at least one child dir, got none")
		}

		// Replace root with the first child
		name = children[0]
		root, err = rootdir.Child(name)
		if err != nil {
			return nil, err
		}
	}

	err = mr.Close()
	if err != nil {
		return nil, err
	}

	nd, err := root.GetNode()
	if err != nil {
		return nil, err
	}

	// output directory events
	err = adder.outputDirs(name, root)
	if err != nil {
		return nil, err
	}

	return nd, nil
}

func (adder *Adder) addFileNode(path string, file files.Node, toplevel bool) error {
	defer file.Close()

	err := adder.maybePauseForGC()
	if err != nil {
		return err
	}

	if adder.liveNodes >= liveCacheSize {
		// TODO: A smarter cache that uses some sort of lru cache with an eviction handler
		mr, err := adder.mfsRoot()
		if err != nil {
			return err
		}
		if err := mr.FlushMemFree(adder.ctx); err != nil {
			return err
		}

		adder.liveNodes = 0
	}
	adder.liveNodes++

	switch f := file.(type) {
	case files.Directory:
		return adder.addDir(path, f, toplevel)
	case *files.Symlink:
		return adder.addSymlink(path, f)
	case files.File:
		return adder.addFile(path, f)
	default:
		return errors.New("unknown file type")
	}
}

func (adder *Adder) addSymlink(path string, l *files.Symlink) error {
	sdata, err := unixfs.SymlinkData(l.Target)
	if err != nil {
		return err
	}

	dagnode := dag.NodeWithData(sdata)
	dagnode.SetCidBuilder(adder.CidBuilder)
	err = adder.dagService.Add(adder.ctx, dagnode)
	if err != nil {
		return err
	}

	return adder.addNode(dagnode, path)
}

func (adder *Adder) addFile(path string, file files.File) error {
	// if the progress flag was specified, wrap the file so that we can send
	// progress updates to the client (over the output channel)
	var reader io.Reader = file
	if adder.Progress {
		rdr := &progressReader{file: reader, path: path, out: adder.Out}
		if fi, ok := file.(files.FileInfo); ok {
			reader = &progressReader2{rdr, fi}
		} else {
			reader = rdr
		}
	}

	dagnode, err := adder.add(reader, nil)
	if err != nil {
		return err
	}

	// patch it into the root
	return adder.addNode(dagnode, path)
}

func (adder *Adder) addDir(path string, dir files.Directory, toplevel bool) error {
	log.Infof("adding directory: %s", path)

	if !(toplevel && path == "") { // !toplevel || path != ''
		mr, err := adder.mfsRoot()
		if err != nil {
			return err
		}
		err = mfs.Mkdir(mr, path, mfs.MkdirOpts{
			Mkparents:  true,
			Flush:      false,
			CidBuilder: adder.CidBuilder,
		})
		if err != nil {
			return err
		}
	}

	it := dir.Entries()
	for it.Next() {
		fpath := gopath.Join(path, it.Name())
		err := adder.addFileNode(fpath, it.Node(), false)
		if err != nil {
			return err
		}
	}

	return it.Err()
}

// convertMetadataToBytes converts token metadata in JSON string to
// byte array in JSON encoding.
func (adder *Adder) convertMetadataToBytes(checkString bool) ([]byte, error) {
	s := adder.TokenMetadata
	b := []byte(s)
	// Optionally check if the input data is in JSON string.
	// If not, return zero and error.
	if checkString {
		var a interface{}
		var err error
		err = json.Unmarshal(b, &a)
		if err != nil {
			return nil, err
		}
	}

	return b, nil
}

// appendMetadataObject appends a new unmarshalled object to the existing
// bytes for metadata and returns the new encoding.
func (adder *Adder) appendMetadataObject(metadata []byte, o interface{}) ([]byte, error) {
	// Nothing to append, return original
	if o == nil {
		return metadata, nil
	}
	b, err := json.Marshal(o)
	if err != nil {
		return nil, err
	}
	// Nothing from existing, return new JSON
	if len(metadata) == 0 {
		return b, nil
	}
	// Now splice last } of existing and the { of the new JSON into a new ,
	return append(append(metadata[:len(metadata)-1], ','), b[1:]...), nil
}

func (adder *Adder) maybePauseForGC() error {
	if adder.unlocker != nil && adder.gcLocker.GCRequested() {
		rn, err := adder.curRootNode()
		if err != nil {
			return err
		}

		err = adder.PinRoot(rn)
		if err != nil {
			return err
		}

		adder.unlocker.Unlock()
		adder.unlocker = adder.gcLocker.PinLock()
	}
	return nil
}

// outputDagnode sends dagnode info over the output channel
func outputDagnode(out chan<- interface{}, name string, dn ipld.Node) error {
	if out == nil {
		return nil
	}

	o, err := getOutput(dn)
	if err != nil {
		return err
	}

	out <- &coreiface.AddEvent{
		Path: o.Path,
		Name: name,
		Size: o.Size,
	}

	return nil
}

// from core/commands/object.go
func getOutput(dagnode ipld.Node) (*coreiface.AddEvent, error) {
	c := dagnode.Cid()
	s, err := dagnode.Size()
	if err != nil {
		return nil, err
	}

	output := &coreiface.AddEvent{
		Path: path.IpfsPath(c),
		Size: strconv.FormatUint(s, 10),
	}

	return output, nil
}

type progressReader struct {
	file         io.Reader
	path         string
	out          chan<- interface{}
	bytes        int64
	lastProgress int64
}

func (i *progressReader) Read(p []byte) (int, error) {
	n, err := i.file.Read(p)

	i.bytes += int64(n)
	if i.bytes-i.lastProgress >= progressReaderIncrement || err == io.EOF {
		i.lastProgress = i.bytes
		i.out <- &coreiface.AddEvent{
			Name:  i.path,
			Bytes: i.bytes,
		}
	}

	return n, err
}

type progressReader2 struct {
	*progressReader
	files.FileInfo
}

func (i *progressReader2) Read(p []byte) (int, error) {
	return i.progressReader.Read(p)
}

type progressReader3 struct {
	*progressReader
	files.Node
}

func (i *progressReader3) Read(p []byte) (int, error) {
	return i.progressReader.Read(p)
}
