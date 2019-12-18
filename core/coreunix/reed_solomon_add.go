package coreunix

import (
	"encoding/json"
	"errors"
	"io"
	gopath "path"
	"strings"

	"github.com/TRON-US/go-btfs-files"
	"github.com/TRON-US/go-unixfs"
	uio "github.com/TRON-US/go-unixfs/io"
	ipld "github.com/ipfs/go-ipld-format"
	dag "github.com/ipfs/go-merkledag"
)

type ReedSolomonAdder struct {
	Adder
	InfileReaders         []io.Reader
	IsDir                 bool
	DirTreeSeparatorCount int
	CurrentOffset         uint64
}

// NewReedSolomonAdder returns a new ReedSolomonAdder used for a file/directory add operation.
func NewReedSolomonAdder(adder *Adder) (*ReedSolomonAdder, error) {
	rsa := &ReedSolomonAdder{
		InfileReaders: nil,
		IsDir:         false,
		CurrentOffset: 0,
	}
	rsa.Adder = *adder

	return rsa, nil
}

// AddAllAndPin adds the given request's files and pin them.
func (rsadder *ReedSolomonAdder) AddAllAndPin(file files.Node) (ipld.Node, error) {
	if rsadder.Pin {
		rsadder.unlocker = rsadder.gcLocker.PinLock()
	}
	defer func() {
		if rsadder.unlocker != nil {
			rsadder.unlocker.Unlock()
		}
	}()

	// Create a directory tree from the input `file` directory.
	n, err := rsadder.addFileNode("", file, true)
	if err != nil {
		return nil, err
	}
	var byts []byte
	switch node := n.(type) {
	case *uio.DirNode, *uio.SymlinkNode, *uio.FileNode:
		byts, err = json.Marshal(node)
		if err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("unexpected node type")
	}
	rsadder.DirTreeSeparatorCount = strings.Count(string(byts), "},{")

	// Create a DAG with the above directory tree as metadata and
	// the data from the given `file` directory or file.
	nd, err := rsadder.add(io.MultiReader(rsadder.InfileReaders...), byts)
	if err != nil {
		return nil, err
	}

	// output directory and file events
	err = rsadder.outputDirs("", nd)
	if err != nil {
		return nil, err
	}

	// Pin the newly created DAG.
	if !rsadder.Pin {
		return nd, nil
	}

	if err := rsadder.PinRoot(nd); err != nil {
		return nil, err
	}

	return nd, nil
}

// addFileNode traverses the directory tree under
// the given `file` in a bottom up DFS way.
func (rsadder *ReedSolomonAdder) addFileNode(path string, file files.Node, toplevel bool) (uio.Node, error) {
	//defer file.Close()

	err := rsadder.maybePauseForGC()
	if err != nil {
		return nil, err
	}

	if rsadder.liveNodes >= liveCacheSize {
		// TODO: flush free memory rsadder.mfsRoot()'s FlushMemFree() to flush free memory
		log.Info("rsadder liveNodes >= liveCacheSize, needs flushing")
	}
	rsadder.liveNodes++

	switch f := file.(type) {
	case files.Directory:
		return rsadder.addDir(path, f, toplevel)
	case *files.Symlink:
		return rsadder.addSymlink(path, f)
	case files.File:
		return rsadder.addFile(path, f)
	default:
		return nil, errors.New("unknown file type")
	}
}

func (rsadder *ReedSolomonAdder) addDir(path string, dir files.Directory, toplevel bool) (uio.Node, error) {
	log.Infof("adding directory: %s", path)

	_, dstName := gopath.Split(path)
	// fdir, err := rsadder.NewUfsDirectory()
	//if err != nil {
	//      return nil, err
	//}
	node := &uio.DirNode{
		BaseNode: uio.BaseNode{
			NodeType: uio.DirNodeType,
			NodePath: path,
			NodeName: dstName},
		//Directory: dir,
	}

	it := dir.Entries()
	for it.Next() {
		fpath := gopath.Join(path, it.Name())
		child, err := rsadder.addFileNode(fpath, it.Node(), false)
		if err != nil {
			return nil, err
		}
		node.Links = append(node.Links, child)
	}

	return node, it.Err()
}

func (rsadder *ReedSolomonAdder) addSymlink(path string, l *files.Symlink) (uio.Node, error) {
	_, dstName := gopath.Split(path)
	return &uio.SymlinkNode{
		BaseNode: uio.BaseNode{
			NodeType: uio.SymlinkNodeType,
			NodePath: path,
			NodeName: dstName},
		Data: l.Target,
		//Symlink:   *l,
	}, nil
}

func (rsadder *ReedSolomonAdder) addFile(path string, file files.File) (uio.Node, error) {
	_, dstName := gopath.Split(path)

	var reader io.Reader = file
	rsadder.InfileReaders = append(rsadder.InfileReaders, reader)
	// TODO: make sure file.Size() is accurate. This should never be stale.
	// So ..
	size, err := file.Size()
	if err != nil {
		return nil, err
	}
	currentOffset := rsadder.CurrentOffset
	rsadder.CurrentOffset += uint64(size)
	node := &uio.FileNode{
		BaseNode: uio.BaseNode{
			NodeType:    uio.FileNodeType,
			NodePath:    path,
			NodeName:    dstName,
			Siz:         uint64(size),
			StartOffset: currentOffset,
		},
		//File: file,
	}
	return node, nil
}

// outputDirs outputs file and directory dagnodes in a postorder DFS fashion.
func (rsadder *ReedSolomonAdder) outputDirs(path string, n ipld.Node) error {
	dn, ok := n.(*dag.ProtoNode)
	if !ok {
		return errors.New("expected protobuf node type")
	}
	fsn, err := unixfs.FSNodeFromBytes(dn.Data())
	if err != nil {
		return err
	}

	if fsn.IsDir() {
		for _, link := range dn.Links() {
			child, err := link.GetNode(rsadder.ctx, rsadder.dagService)
			if err != nil {
				return err
			}
			childpath := gopath.Join(path, link.Name)
			err = rsadder.outputDirs(childpath, child)
			if err != nil {
				return err
			}
		}
	} else {
		return outputDagnode(rsadder.Out, path, n)
	}
	return nil
}
