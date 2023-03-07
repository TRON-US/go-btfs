module github.com/ipfs/go-ipfs/test/dependencies

go 1.13

require (
	github.com/Kubuxu/gocovmerge v0.0.0-20161216165753-7ecaa51963cd
	github.com/TRON-US/go-unixfs v0.6.0
	github.com/golangci/golangci-lint v1.26.0
	github.com/ipfs/go-blockservice v0.1.3
	github.com/ipfs/go-cid v0.3.2
	github.com/ipfs/go-cidutil v0.0.2
	github.com/ipfs/go-datastore v0.4.4
	github.com/ipfs/go-graphsync v0.1.1
	github.com/ipfs/go-ipfs-blockstore v1.0.0
	github.com/ipfs/go-ipfs-exchange-offline v0.0.1
	github.com/ipfs/go-log v1.0.4
	github.com/ipfs/go-merkledag v0.3.2
	github.com/ipfs/hang-fds v0.0.2
	github.com/ipfs/iptb v1.4.0
	github.com/ipfs/iptb-plugins v0.3.0
	github.com/ipld/go-ipld-prime v0.19.0
	github.com/jbenet/go-random v0.0.0-20190219211222-123a90aedc0c
	github.com/jbenet/go-random-files v0.0.0-20190219210431-31b3f20ebded
	github.com/libp2p/go-libp2p v0.9.6
	github.com/libp2p/go-libp2p-core v0.5.7
	github.com/multiformats/go-multiaddr v0.2.2
	github.com/multiformats/go-multiaddr-net v0.1.5
	github.com/multiformats/go-multihash v0.2.1
	gotest.tools/gotestsum v0.4.2
)

replace github.com/ipfs/go-cid => github.com/TRON-US/go-cid v0.2.0

replace github.com/libp2p/go-libp2p-core => github.com/TRON-US/go-libp2p-core v0.5.0

replace github.com/multiformats/go-multiaddr => github.com/TRON-US/go-multiaddr v0.3.0
