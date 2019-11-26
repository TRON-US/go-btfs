package test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"io"
	"io/ioutil"
	"testing"
	"time"

	ft "github.com/TRON-US/go-unixfs"
	importer "github.com/TRON-US/go-unixfs/importer"
	uio "github.com/TRON-US/go-unixfs/io"
	"github.com/TRON-US/go-unixfs/mod"
	bserv "github.com/ipfs/go-blockservice"
	merkledag "github.com/ipfs/go-merkledag"

	chunker "github.com/TRON-US/go-btfs-chunker"
	config "github.com/TRON-US/go-btfs-config"
	files "github.com/TRON-US/go-btfs-files"
	"github.com/TRON-US/go-btfs/core"
	"github.com/TRON-US/go-btfs/core/coreapi"
	"github.com/TRON-US/go-btfs/core/coreunix"
	"github.com/TRON-US/go-btfs/repo"
	coreiface "github.com/TRON-US/interface-go-btfs-core"
	ipath "github.com/TRON-US/interface-go-btfs-core/path"
	cid "github.com/ipfs/go-cid"
	datastore "github.com/ipfs/go-datastore"
	ds "github.com/ipfs/go-datastore"
	dssync "github.com/ipfs/go-datastore/sync"
	syncds "github.com/ipfs/go-datastore/sync"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	offline "github.com/ipfs/go-ipfs-exchange-offline"
	u "github.com/ipfs/go-ipfs-util"
	ipld "github.com/ipfs/go-ipld-format"
)

type MetaStruct struct {
	Price  float64
	Number int
	NodeId string
}

func getDagserv(t *testing.T) ipld.DAGService {
	db := dssync.MutexWrap(ds.NewMapDatastore())
	bs := bstore.NewBlockstore(db)
	blockserv := bserv.New(bs, offline.Exchange(bs))
	return merkledag.NewDAGService(blockserv)
}

func TestMetadata(t *testing.T) {
	ctx := context.Background()
	// Make some random node
	ds := getDagserv(t)
	data := make([]byte, 1000)
	_, err := io.ReadFull(u.NewTimeSeededRand(), data)
	if err != nil {
		t.Fatal(err)
	}
	r := bytes.NewReader(data)
	nd, err := importer.BuildDagFromReader(ds, chunker.DefaultSplitter(r))
	if err != nil {
		t.Fatal(err)
	}

	c := nd.Cid()

	m := new(ft.Metadata)
	m.MimeType = "THIS IS A TEST"

	// Such effort, many compromise
	ipfsnode := &core.IpfsNode{DAG: ds}

	mdk, err := coreunix.AddMetadataTo(ipfsnode, c.String(), m)
	if err != nil {
		t.Fatal(err)
	}

	rec, err := coreunix.Metadata(ipfsnode, mdk)
	if err != nil {
		t.Fatal(err)
	}
	if rec.MimeType != m.MimeType {
		t.Fatalf("something went wrong in conversion: '%s' != '%s'", rec.MimeType, m.MimeType)
	}

	cdk, err := cid.Decode(mdk)
	if err != nil {
		t.Fatal(err)
	}

	retnode, err := ds.Get(ctx, cdk)
	if err != nil {
		t.Fatal(err)
	}

	rtnpb, ok := retnode.(*merkledag.ProtoNode)
	if !ok {
		t.Fatal("expected protobuf node")
	}

	ndr, err := uio.NewDagReader(ctx, rtnpb, ds)
	if err != nil {
		t.Fatal(err)
	}

	out, err := ioutil.ReadAll(ndr)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(out, data) {
		t.Fatal("read incorrect data")
	}
}

func addFileWithMetadata(node *core.IpfsNode, data []byte, metadata string) (ipath.Path, error) {
	return addFileToBtfs(node, data, metadata)
}

func addFile(node *core.IpfsNode, data []byte) (ipath.Path, error) {
	return addFileToBtfs(node, data, "")
}

func addFileWithoutMetadata(node *core.IpfsNode, data []byte) (ipath.Path, error) {
	return addFileToBtfs(node, data, "")
}

func addFileToBtfs(node *core.IpfsNode, data []byte, metadata string) (ipath.Path, error) {
	// Create out chan and adder
	output := make(chan interface{})
	adder, err := coreunix.NewAdder(context.Background(), node.Pinning, node.Blockstore, node.DAG)
	if err != nil {
		return nil, err
	}
	adder.Out = output
	if metadata != "" {
		adder.TokenMetadata = metadata
	}

	// Create a file with goroutine
	file := files.NewBytesFile(data)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	go func() {
		defer close(output)
		_, err := adder.AddAllAndPin(file)
		if err != nil {
			output <- err
		}
	}()

	var addedFileHash cid.Cid
	select {
	case o := <-output:
		err, ok := o.(error)
		if ok {
			return nil, err
		}
		addedFileHash = o.(*coreiface.AddEvent).Path.Cid()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	return ipath.IpfsPath(addedFileHash), nil
}

func addMetadata(node *core.IpfsNode, path ipath.Path, meta string) (ipath.Path, error) {
	api, err := coreapi.NewCoreAPI(node)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	nd, err := api.ResolveNode(ctx, path)
	if err != nil {
		return nil, err
	}

	output := make(chan interface{})
	modifier, err := coreunix.NewMetaModifier(context.Background(), node.Pinning, node.Blockstore, node.DAG)
	if err != nil {
		return nil, err
	}
	modifier.Out = output
	modifier.TokenMetadata = meta

	go func() {
		defer close(output)
		n, _, err := modifier.AddMetaAndPin(nd)
		if err != nil {
			output <- err
		}
		modifier.Out <- n.Cid()
	}()

	var modifiedFileHash cid.Cid
	select {
	case o := <-output:
		err, ok := o.(error)
		if ok {
			return nil, err
		}
		modifiedFileHash = o.(cid.Cid)
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	return ipath.IpfsPath(modifiedFileHash), nil
}

func TestAppendMetadata(t *testing.T) {
	// Create repo.
	r := &repo.Mock{
		C: config.Config{
			Identity: config.Identity{
				PeerID: testPeerID, // required by offline node
			},
		},
		D: syncds.MutexWrap(datastore.NewMapDatastore()),
	}
	// Build Ipfs node.
	node, err := core.NewNode(context.Background(), &core.BuildCfg{Repo: r})
	if err != nil {
		t.Fatal(err)
	}

	// Add a file to BTFS network.
	data := []byte("existing file contents")
	path, err := addFileWithMetadata(node, data, `{"price":11.2}`)
	if err != nil {
		t.Fatal(err)
	}

	// Append token metadata to the BTFS file.
	p, err := addMetadata(node, path, fmt.Sprintf(`{"number":%d}`, 1234))
	if err != nil {
		t.Fatal(err)
	}

	// Verify modified file.
	verifyMetadataItems(t, node, p, &MetaStruct{Price: 11.2, Number: 1234})

	if p.String() != "/btfs/QmXDCec9RpJTBkUQyHFziVqWSdHfuBuKHNRTT31Hh5Wgtn" {
		// note: the exact number will depend on the size and the sharding algo. used
		t.Fatalf("expected %s, got %s", "/btfs/QmXDCec9RpJTBkUQyHFziVqWSdHfuBuKHNRTT31Hh5Wgtn", p.String())
	}
}

func TestAddMetadata(t *testing.T) {
	// Create repo.
	r := &repo.Mock{
		C: config.Config{
			Identity: config.Identity{
				PeerID: testPeerID, // required by offline node
			},
		},
		D: syncds.MutexWrap(datastore.NewMapDatastore()),
	}
	// Build Ipfs node.
	node, err := core.NewNode(context.Background(), &core.BuildCfg{Repo: r})
	if err != nil {
		t.Fatal(err)
	}

	// Add a file to BTFS network.
	path, err := addFile(node, []byte("existing file contents"))
	if err != nil {
		t.Fatal(err)
	}

	// Add token metadata to the BTFS file without existing metadata.
	expected := fmt.Sprintf(`{"number":%d}`, 2368)
	p, err := addMetadata(node, path, expected)
	if err != nil {
		t.Fatal(err)
	}

	// Verify modified file.
	metaSize, err := metaDataSize(node, p)
	if err != nil {
		t.Fatal(err)
	}
	if len(expected) != metaSize {
		t.Fatalf("expected %d, got %d", len(expected), int(metaSize))
	}
	verifyMetadataItems(t, node, p, &MetaStruct{Number: 2368})
	if p.String() != "/btfs/QmUjy2YN56NzJowxpY8419NdLXMwTfLPRAZPr55rxbYV1G" {
		// note: the exact number will depend on the size and the sharding algo. used
		t.Fatalf("expected %s, got %s", "/btfs/QmUjy2YN56NzJowxpY8419NdLXMwTfLPRAZPr55rxbYV1G", p.String())
	}
}

func TestUpdateMetadata(t *testing.T) {
	// Create repo.
	r := &repo.Mock{
		C: config.Config{
			Identity: config.Identity{
				PeerID: testPeerID, // required by offline node
			},
		},
		D: syncds.MutexWrap(datastore.NewMapDatastore()),
	}
	// Build Ipfs node.
	node, err := core.NewNode(context.Background(), &core.BuildCfg{Repo: r})
	if err != nil {
		t.Fatal(err)
	}

	// Add a file to BTFS network.
	data := []byte("existing file contents")
	path, err := addFileWithMetadata(node, data, `{"price":11.2}`)
	if err != nil {
		t.Fatal(err)
	}

	// Append token metadata to the BTFS file.
	p, err := addMetadata(node, path, `{"price":23.56,"number":4356,"NewItem":"justSave"}`)
	if err != nil {
		t.Fatal(err)
	}

	// Verify modified file.
	verifyMetadataItems(t, node, p, &MetaStruct{Price: 23.56, Number: 4356})

	if p.String() != "/btfs/QmXay6rSJnUS4PcwkvqWtLbHWhH77RD7knvqkrwtREEcJ7" {
		// note: the exact number will depend on the size and the sharding algo. used
		t.Fatalf("expected %s, got %s", "/btfs/QmXay6rSJnUS4PcwkvqWtLbHWhH77RD7knvqkrwtREEcJ7", p.String())
	}
}

// removeMetadata removes the metadata items for the key values from the given `meta` string.
// `meta` will have a format of key list. E.g., "price,nodeId".
func removeMetadata(node *core.IpfsNode, path ipath.Path, meta string) (ipath.Path, error) {
	api, err := coreapi.NewCoreAPI(node)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	nd, err := api.ResolveNode(ctx, path)
	if err != nil {
		return nil, err
	}

	output := make(chan interface{})
	modifier, err := coreunix.NewMetaModifier(context.Background(), node.Pinning, node.Blockstore, node.DAG)
	if err != nil {
		return nil, err
	}
	modifier.Out = output
	modifier.TokenMetadata = meta

	go func() {
		defer close(output)
		n, _, err := modifier.RemoveMetaAndPin(nd)
		if err != nil {
			output <- err
		}
		modifier.Out <- n.Cid()
	}()

	var modifiedFileHash cid.Cid
	select {
	case o := <-output:
		err, ok := o.(error)
		if ok {
			return nil, err
		}
		modifiedFileHash = o.(cid.Cid)
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	return ipath.IpfsPath(modifiedFileHash), nil
}
func TestRemoveMetadata(t *testing.T) {
	// Create repo.
	r := &repo.Mock{
		C: config.Config{
			Identity: config.Identity{
				PeerID: testPeerID, // required by offline node
			},
		},
		D: syncds.MutexWrap(datastore.NewMapDatastore()),
	}
	// Build Ipfs node.
	node, err := core.NewNode(context.Background(), &core.BuildCfg{Repo: r})
	if err != nil {
		t.Fatal(err)
	}

	// Add a file to BTFS network.
	data := []byte("existing file contents")
	path, err := addFileWithMetadata(node, data, `{"price":11.2,"number":1234}`)
	if err != nil {
		t.Fatal(err)
	}

	// Append token metadata to the BTFS file.
	p, err := removeMetadata(node, path, `price,id`)
	if err != nil {
		t.Fatal(err)
	}

	// Verify modified file.
	verifyMetadataItems(t, node, p, &MetaStruct{Number: 1234})

	if p.String() != "/btfs/QmWJzTx5BNVwwuj92CJ5xPhyKWmwQHGt2eTwUMp6WQG3K2" {
		// note: the exact number will depend on the size and the sharding algo. used
		t.Fatalf("expected %s, got %s", "/btfs/QmWJzTx5BNVwwuj92CJ5xPhyKWmwQHGt2eTwUMp6WQG3K2", p.String())
	}
}

// metaDataSize returns the size of the metadata sub-DAG of
// the DAG of the given `p`.
func metaDataSize(node *core.IpfsNode, p ipath.Path) (int, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	api, err := coreapi.NewCoreAPI(node)
	if err != nil {
		return 0, err
	}
	nd, err := api.ResolveNode(ctx, p)
	if err != nil {
		return 0, err
	}
	mnode, err := ft.GetMetaSubdagRoot(ctx, nd, node.DAG)
	if err != nil {
		return 0, err
	}
	fileSize, err := mod.FileSize(mnode)
	if err != nil {
		return 0, err
	}
	return int(fileSize), nil
}

func verifyMetadataItems(t *testing.T, node *core.IpfsNode, p ipath.Path, exp *MetaStruct) {

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	api, err := coreapi.NewCoreAPI(node)
	if err != nil {
		t.Fatal(err)
	}

	b, err := coreunix.GetMetaData(ctx, api, p)
	if err != nil {
		t.Fatal(err)
	}
	var meta MetaStruct
	if err := json.Unmarshal(b, &meta); err != nil {
		t.Fatal(err)
	}

	if meta.Price != exp.Price {
		t.Fatalf("expected %.2f, got %.2f", exp.Price, meta.Price)
	}
	if meta.Number != exp.Number {
		t.Fatalf("expected %d, got %d", exp.Number, meta.Number)
	}
	if meta.NodeId != exp.NodeId {
		t.Fatalf("expected %s, got %s", exp.NodeId, meta.NodeId)
	}
}
