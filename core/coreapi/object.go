package coreapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/ioutil"

	"github.com/TRON-US/go-btfs/dagutils"
	"github.com/TRON-US/go-btfs/pin"

	ft "github.com/TRON-US/go-unixfs"
	pb "github.com/TRON-US/go-unixfs/pb"
	coreiface "github.com/TRON-US/interface-go-btfs-core"
	caopts "github.com/TRON-US/interface-go-btfs-core/options"
	ipath "github.com/TRON-US/interface-go-btfs-core/path"
	cid "github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
	dag "github.com/ipfs/go-merkledag"
)

const inputLimit = 2 << 20

type ObjectAPI CoreAPI

type Link struct {
	Name, Hash string
	Size       uint64
}

type Node struct {
	Links []Link
	Data  string
}

func (api *ObjectAPI) New(ctx context.Context, opts ...caopts.ObjectNewOption) (ipld.Node, error) {
	options, err := caopts.ObjectNewOptions(opts...)
	if err != nil {
		return nil, err
	}

	var n ipld.Node
	switch options.Type {
	case "empty":
		n = new(dag.ProtoNode)
	case "unixfs-dir":
		n = ft.EmptyDirNode()
	}

	err = api.dag.Add(ctx, n)
	if err != nil {
		return nil, err
	}
	return n, nil
}

func (api *ObjectAPI) Put(ctx context.Context, src io.Reader, opts ...caopts.ObjectPutOption) (ipath.Resolved, error) {
	options, err := caopts.ObjectPutOptions(opts...)
	if err != nil {
		return nil, err
	}

	data, err := ioutil.ReadAll(io.LimitReader(src, inputLimit+10))
	if err != nil {
		return nil, err
	}

	var dagnode *dag.ProtoNode
	switch options.InputEnc {
	case "json":
		node := new(Node)
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		err = decoder.Decode(node)
		if err != nil {
			return nil, err
		}

		dagnode, err = deserializeNode(node, options.DataType)
		if err != nil {
			return nil, err
		}

	case "protobuf":
		dagnode, err = dag.DecodeProtobuf(data)

	case "xml":
		node := new(Node)
		err = xml.Unmarshal(data, node)
		if err != nil {
			return nil, err
		}

		dagnode, err = deserializeNode(node, options.DataType)
		if err != nil {
			return nil, err
		}

	default:
		return nil, errors.New("unknown object encoding")
	}

	if err != nil {
		return nil, err
	}

	if options.Pin {
		defer api.blockstore.PinLock().Unlock()
	}

	err = api.dag.Add(ctx, dagnode)
	if err != nil {
		return nil, err
	}

	if options.Pin {
		api.pinning.PinWithMode(dagnode.Cid(), pin.Recursive)
		err = api.pinning.Flush()
		if err != nil {
			return nil, err
		}
	}

	return ipath.IpfsPath(dagnode.Cid()), nil
}

func (api *ObjectAPI) Get(ctx context.Context, path ipath.Path, meta bool) (ipld.Node, error) {
	return api.core().ResolveNode(ctx, path)
}

func (api *ObjectAPI) Data(ctx context.Context, path ipath.Path, unixfs bool, meta bool) (io.Reader, io.Reader, error) {
	nd, err := api.core().ResolveNode(ctx, path)
	if err != nil {
		return nil, nil, err
	}

	pbnd, ok := nd.(*dag.ProtoNode)
	if !ok {
		return nil, nil, dag.ErrNotProtobuf
	}

	if unixfs {
		ds := api.core().Dag()

		pData, metaData, err := getDataForUserAndMeta(pbnd, ds)
		if err != nil {
			return nil, nil, err
		}

		if meta && metaData != nil {
			return bytes.NewReader(pData), bytes.NewReader(metaData), nil
		} else {
			return bytes.NewReader(pData), nil, nil
		}
	} else {
		return bytes.NewReader(pbnd.Data()), nil, nil
	}
}

// Return user data, metadata in byte array, and error.
// Note that this function assumes, if token metadata exists inside
// the DAG topped by the given 'node', the 'node' has metadata root
// as first child, user data root as second child.
func getDataForUserAndMeta(nd ipld.Node, ds ipld.DAGService) ([]byte, []byte, error) {
	n := nd.(*dag.ProtoNode)

	fsType, err := getFSType(n)
	if err != nil {
		return nil, nil, err
	}

	if ft.TTokenMeta == fsType {
		return nil, n.Data(), nil
	}

	// Return user data and metadata if first child is of type TTokenMeta.
	if nd.Links() != nil && len(nd.Links()) >= 2 {
		childen, err := getChildrenForDagWithMeta(nd, ds)
		if err != nil {
			return nil, nil, err
		}
		if childen == nil {
			return nil, n.Data(), nil
		}
		return childen[1].(*dag.ProtoNode).Data(), childen[0].(*dag.ProtoNode).Data(), nil
	}

	return n.Data(), nil, nil
}

// Skips metadata if exists from the DAG topped by the given 'nd'.
// Case #1: if 'nd' is dummy root with metadata root node and user data root node being children
//    return the second child node that is the root of user data sub-DAG.
// Case #2: if 'nd' is metadata, return none.
// Case #3: if 'nd' is user data, return 'nd'.
func skipMetadataIfExists(nd ipld.Node, ds ipld.DAGService) (ipld.Node, error) {
	//var retNd ipld.Node = nil
	n := nd.(*dag.ProtoNode)

	fsType, err := getFSType(n)
	if err != nil {
		return nil, err
	}

	if ft.TTokenMeta == fsType {
		return nil, fmt.Errorf("Token metadata can not be accessed by default")
	}

	// Return user data and metadata if first child is of type TTokenMeta.
	if nd.Links() != nil && len(nd.Links()) >= 2 {
		childen, err := getChildrenForDagWithMeta(nd, ds)
		if err != nil {
			return nil, err
		}
		if childen == nil {
			return nd, nil
		}
		return childen[1], nil
	}

	return nd, nil
}

// Returns ipld.Node slice of size 2 if the given 'nd' is top of
// the DAG with token metadata.
func getChildrenForDagWithMeta(nd ipld.Node, ds ipld.DAGService) ([]ipld.Node, error) {
	var nodes = make([]ipld.Node, 2)
	for i := 0; i < 2; i++ {
		lnk := nd.Links()[i]
		c := lnk.Cid
		child, err := ds.Get(context.Background(), c)
		if err != nil {
			return nil, err
		}
		childNode, ok := child.(*dag.ProtoNode)
		if !ok {
			return nil, err
		}
		if i == 0 {
			// Make sure first child is of TTokenMeta.
			// If not, return nil.
			fsType, err := getFSType(childNode)
			if err != nil {
				return nil, err
			}
			if ft.TTokenMeta != fsType {
				return nil, nil
			}
		}
		nodes[i] = child
	}

	return nodes, nil
}

func getFSType(n *dag.ProtoNode) (pb.Data_DataType, error) {
	d, err := ft.FSNodeFromBytes(n.Data())
	if err != nil {
		return 0, err
	}

	return d.Type(), nil
}

func (api *ObjectAPI) MetaDataMap(ctx context.Context, path ipath.Path) (map[string]interface{}, error) {
	mr, err := api.MetaData(ctx, path)
	if err != nil {
		return nil, err
	}

	metaData, err := ioutil.ReadAll(mr)
	if err != nil {
		return nil, err
	}

	jmap := make(map[string]interface{})
	err = json.Unmarshal(metaData, &jmap)
	if err != nil {
		return nil, err
	}

	return jmap, nil
}

func (api *ObjectAPI) MetaData(ctx context.Context, path ipath.Path) (io.Reader, error) {
	nd, err := api.core().ResolveNode(ctx, path)
	if err != nil {
		return nil, err
	}

	_, ok := nd.(*dag.ProtoNode)
	if !ok {
		return nil, dag.ErrNotProtobuf
	}

	ds := api.core().Dag()

	metaData, err := getMetaData(nd, ds)
	if err != nil {
		return nil, err
	}

	return bytes.NewReader(metaData), nil
}

// Return metadata under the given 'nd' node
// if 'nd' has metadata root node aa its first child.
func getMetaData(nd ipld.Node, ds ipld.DAGService) ([]byte, error) {
	n := nd.(*dag.ProtoNode)

	fsType, err := getFSType(n)
	if err != nil {
		return nil, err
	}

	if ft.TTokenMeta == fsType {
		return n.Data(), nil
	}

	// Return metadata if first child is of type TTokenMeta.
	var metadata []byte = nil
	if nd.Links() != nil && len(nd.Links()) >= 2 {
		children, err := getChildrenForDagWithMeta(nd, ds)
		if err != nil {
			return nil, err
		}
		if children == nil {
			return nil, nil
		}
		return children[0].(*dag.ProtoNode).Data(), nil
	}

	return metadata, nil
}

func (api *ObjectAPI) Links(ctx context.Context, path ipath.Path) ([]*ipld.Link, error) {
	nd, err := api.core().ResolveNode(ctx, path)
	if err != nil {
		return nil, err
	}

	links := nd.Links()
	out := make([]*ipld.Link, len(links))
	for n, l := range links {
		out[n] = (*ipld.Link)(l)
	}

	return out, nil
}

func (api *ObjectAPI) Stat(ctx context.Context, path ipath.Path) (*coreiface.ObjectStat, error) {
	nd, err := api.core().ResolveNode(ctx, path)
	if err != nil {
		return nil, err
	}

	stat, err := nd.Stat()
	if err != nil {
		return nil, err
	}

	out := &coreiface.ObjectStat{
		Cid:            nd.Cid(),
		NumLinks:       stat.NumLinks,
		BlockSize:      stat.BlockSize,
		LinksSize:      stat.LinksSize,
		DataSize:       stat.DataSize,
		CumulativeSize: stat.CumulativeSize,
	}

	return out, nil
}

func (api *ObjectAPI) AddLink(ctx context.Context, base ipath.Path, name string, child ipath.Path, opts ...caopts.ObjectAddLinkOption) (ipath.Resolved, error) {
	options, err := caopts.ObjectAddLinkOptions(opts...)
	if err != nil {
		return nil, err
	}

	baseNd, err := api.core().ResolveNode(ctx, base)
	if err != nil {
		return nil, err
	}

	childNd, err := api.core().ResolveNode(ctx, child)
	if err != nil {
		return nil, err
	}

	basePb, ok := baseNd.(*dag.ProtoNode)
	if !ok {
		return nil, dag.ErrNotProtobuf
	}

	var createfunc func() *dag.ProtoNode
	if options.Create {
		createfunc = ft.EmptyDirNode
	}

	e := dagutils.NewDagEditor(basePb, api.dag)

	err = e.InsertNodeAtPath(ctx, name, childNd, createfunc)
	if err != nil {
		return nil, err
	}

	nnode, err := e.Finalize(ctx, api.dag)
	if err != nil {
		return nil, err
	}

	return ipath.IpfsPath(nnode.Cid()), nil
}

func (api *ObjectAPI) RmLink(ctx context.Context, base ipath.Path, link string) (ipath.Resolved, error) {
	baseNd, err := api.core().ResolveNode(ctx, base)
	if err != nil {
		return nil, err
	}

	basePb, ok := baseNd.(*dag.ProtoNode)
	if !ok {
		return nil, dag.ErrNotProtobuf
	}

	e := dagutils.NewDagEditor(basePb, api.dag)

	err = e.RmLink(ctx, link)
	if err != nil {
		return nil, err
	}

	nnode, err := e.Finalize(ctx, api.dag)
	if err != nil {
		return nil, err
	}

	return ipath.IpfsPath(nnode.Cid()), nil
}

func (api *ObjectAPI) AppendData(ctx context.Context, path ipath.Path, r io.Reader) (ipath.Resolved, error) {
	return api.patchData(ctx, path, r, true)
}

func (api *ObjectAPI) SetData(ctx context.Context, path ipath.Path, r io.Reader) (ipath.Resolved, error) {
	return api.patchData(ctx, path, r, false)
}

func (api *ObjectAPI) patchData(ctx context.Context, path ipath.Path, r io.Reader, appendData bool) (ipath.Resolved, error) {
	nd, err := api.core().ResolveNode(ctx, path)
	if err != nil {
		return nil, err
	}

	pbnd, ok := nd.(*dag.ProtoNode)
	if !ok {
		return nil, dag.ErrNotProtobuf
	}

	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}

	if appendData {
		data = append(pbnd.Data(), data...)
	}
	pbnd.SetData(data)

	err = api.dag.Add(ctx, pbnd)
	if err != nil {
		return nil, err
	}

	return ipath.IpfsPath(pbnd.Cid()), nil
}

func (api *ObjectAPI) Diff(ctx context.Context, before ipath.Path, after ipath.Path) ([]coreiface.ObjectChange, error) {
	beforeNd, err := api.core().ResolveNode(ctx, before)
	if err != nil {
		return nil, err
	}

	afterNd, err := api.core().ResolveNode(ctx, after)
	if err != nil {
		return nil, err
	}

	changes, err := dagutils.Diff(ctx, api.dag, beforeNd, afterNd)
	if err != nil {
		return nil, err
	}

	out := make([]coreiface.ObjectChange, len(changes))
	for i, change := range changes {
		out[i] = coreiface.ObjectChange{
			Type: change.Type,
			Path: change.Path,
		}

		if change.Before.Defined() {
			out[i].Before = ipath.IpfsPath(change.Before)
		}

		if change.After.Defined() {
			out[i].After = ipath.IpfsPath(change.After)
		}
	}

	return out, nil
}

func (api *ObjectAPI) core() coreiface.CoreAPI {
	return (*CoreAPI)(api)
}

func deserializeNode(nd *Node, dataFieldEncoding string) (*dag.ProtoNode, error) {
	dagnode := new(dag.ProtoNode)
	switch dataFieldEncoding {
	case "text":
		dagnode.SetData([]byte(nd.Data))
	case "base64":
		data, err := base64.StdEncoding.DecodeString(nd.Data)
		if err != nil {
			return nil, err
		}
		dagnode.SetData(data)
	default:
		return nil, fmt.Errorf("unkown data field encoding")
	}

	links := make([]*ipld.Link, len(nd.Links))
	for i, link := range nd.Links {
		c, err := cid.Decode(link.Hash)
		if err != nil {
			return nil, err
		}
		links[i] = &ipld.Link{
			Name: link.Name,
			Size: link.Size,
			Cid:  c,
		}
	}
	dagnode.SetLinks(links)

	return dagnode, nil
}
