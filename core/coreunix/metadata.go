package coreunix

import (
	"context"
	"encoding/json"
	"fmt"

	core "github.com/TRON-US/go-btfs/core"

	ft "github.com/TRON-US/go-unixfs"
	coreiface "github.com/TRON-US/interface-go-btfs-core"
	ipath "github.com/TRON-US/interface-go-btfs-core/path"
	cid "github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
	dag "github.com/ipfs/go-merkledag"
)

func AddMetadataTo(n *core.IpfsNode, skey string, m *ft.Metadata) (string, error) {
	c, err := cid.Decode(skey)
	if err != nil {
		return "", err
	}

	nd, err := n.DAG.Get(n.Context(), c)
	if err != nil {
		return "", err
	}

	mdnode := new(dag.ProtoNode)
	mdata, err := ft.BytesForMetadata(m)
	if err != nil {
		return "", err
	}

	mdnode.SetData(mdata)
	if err := mdnode.AddNodeLink("file", nd); err != nil {
		return "", err
	}

	err = n.DAG.Add(n.Context(), mdnode)
	if err != nil {
		return "", err
	}

	return mdnode.Cid().String(), nil
}

func Metadata(n *core.IpfsNode, skey string) (*ft.Metadata, error) {
	c, err := cid.Decode(skey)
	if err != nil {
		return nil, err
	}

	nd, err := n.DAG.Get(n.Context(), c)
	if err != nil {
		return nil, err
	}

	pbnd, ok := nd.(*dag.ProtoNode)
	if !ok {
		return nil, dag.ErrNotProtobuf
	}

	return ft.MetadataFromBytes(pbnd.Data())
}

// MetaDataMap takes a root node path and returns an unmarshalled metadata map in bytes
func MetaDataMap(ctx context.Context, api coreiface.CoreAPI, path ipath.Path) (map[string]interface{}, error) {
	metaData, err := MetaData(ctx, api, path)
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

// MetaData takes a root node path and returns the metadata in bytes
func MetaData(ctx context.Context, api coreiface.CoreAPI, path ipath.Path) ([]byte, error) {
	nd, err := api.ResolveNode(ctx, path)
	if err != nil {
		return nil, err
	}

	_, ok := nd.(*dag.ProtoNode)
	if !ok {
		return nil, dag.ErrNotProtobuf
	}

	ds := api.Dag()

	mn, err := getMetaData(ctx, nd, ds)
	if err != nil {
		return nil, err
	}

	fsNode, err := ft.FSNodeFromBytes(mn)
	if err != nil {
		return nil, err
	}

	if fsNode.Type() != ft.TTokenMeta {
		return nil, fmt.Errorf("invalid metadata fs node type: %d", fsNode.Type())
	}

	return fsNode.Data(), nil
}

// GetMetaData returns metadata under the given 'nd' node
// if 'nd' has metadata root node aa its first child.
func getMetaData(ctx context.Context, nd ipld.Node, ds ipld.DAGService) ([]byte, error) {
	_, metadata, err := GetDataForUserAndMeta(ctx, nd, ds)
	return metadata, err
}

// GetDataForUserAndMeta returns raw dag node user data, metadata in byte array, and error.
// Note that this function assumes, if token metadata exists inside
// the DAG topped by the given 'node', the 'node' has metadata root
// as first child, user data root as second child.
func GetDataForUserAndMeta(ctx context.Context, nd ipld.Node, ds ipld.DAGService) ([]byte, []byte, error) {
	n := nd.(*dag.ProtoNode)

	fsType, err := ft.GetFSType(n)
	if err != nil {
		return nil, nil, err
	}

	if ft.TTokenMeta == fsType {
		return nil, n.Data(), nil
	}

	// Return user data and metadata if first child is of type TTokenMeta.
	if nd.Links() != nil && len(nd.Links()) >= 2 {
		childen, err := ft.GetChildrenForDagWithMeta(ctx, nd, ds)
		if err != nil {
			return nil, nil, err
		}
		if childen == nil {
			return nil, n.Data(), nil
		}
		return childen.DataNode.(*dag.ProtoNode).Data(), childen.MetaNode.(*dag.ProtoNode).Data(), nil
	}

	return n.Data(), nil, nil
}
