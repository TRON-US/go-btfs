module github.com/TRON-US/go-btfs

require (
	bazil.org/fuse v0.0.0-20200117225306-7b5117fecadc
	github.com/AndreasBriese/bbloom v0.0.0-20190825152654-46b345b51c96 // indirect
	github.com/FactomProject/basen v0.0.0-20150613233007-fe3947df716e // indirect
	github.com/FactomProject/btcutilecc v0.0.0-20130527213604-d3a63a5752ec // indirect
	github.com/TRON-US/go-btfs-api v0.3.0
	github.com/TRON-US/go-btfs-chunker v0.3.0
	github.com/TRON-US/go-btfs-cmds v0.2.3
	github.com/TRON-US/go-btfs-config v0.6.8
	github.com/TRON-US/go-btfs-files v0.2.0
	github.com/TRON-US/go-btfs-pinner v0.1.1
	github.com/TRON-US/go-btns v0.1.0
	github.com/TRON-US/go-eccrypto v0.0.1
	github.com/TRON-US/go-mfs v0.3.0
	github.com/TRON-US/go-unixfs v0.6.0
	github.com/TRON-US/interface-go-btfs-core v0.6.1
	github.com/Workiva/go-datastructures v1.0.52
	github.com/alecthomas/units v0.0.0-20190717042225-c3de453c63f4
	github.com/blang/semver v3.5.1+incompatible
	github.com/bren2010/proquint v0.0.0-20160323162903-38337c27106d
	github.com/btcsuite/btcutil v1.0.2
	github.com/cenkalti/backoff/v4 v4.0.2
	github.com/cmars/basen v0.0.0-20150613233007-fe3947df716e // indirect
	github.com/coreos/go-systemd/v22 v22.0.0
	github.com/dustin/go-humanize v1.0.0
	github.com/elgris/jsondiff v0.0.0-20160530203242-765b5c24c302
	github.com/ethereum/go-ethereum v1.9.17
	github.com/fatih/color v1.9.0 // indirect
	github.com/fsnotify/fsnotify v1.4.9
	github.com/go-bindata/go-bindata/v3 v3.1.3
	github.com/gogo/protobuf v1.3.1
	github.com/golang/mock v1.4.3 // indirect
	github.com/golang/protobuf v1.3.3
	github.com/google/uuid v1.1.1
	github.com/hashicorp/go-multierror v1.1.0
	github.com/hashicorp/golang-lru v0.5.4
	github.com/ipfs/go-bitswap v0.2.13
	github.com/ipfs/go-block-format v0.0.2
	github.com/ipfs/go-blockservice v0.1.3
	github.com/ipfs/go-cid v0.0.5
	github.com/ipfs/go-cidutil v0.0.2
	github.com/ipfs/go-datastore v0.4.4
	github.com/ipfs/go-detect-race v0.0.1
	github.com/ipfs/go-ds-badger v0.2.4
	github.com/ipfs/go-ds-flatfs v0.4.4
	github.com/ipfs/go-ds-leveldb v0.4.2
	github.com/ipfs/go-ds-measure v0.1.0
	github.com/ipfs/go-filestore v0.0.3
	github.com/ipfs/go-fs-lock v0.0.4
	github.com/ipfs/go-graphsync v0.0.5
	github.com/ipfs/go-ipfs-blockstore v0.1.4
	github.com/ipfs/go-ipfs-ds-help v0.1.1
	github.com/ipfs/go-ipfs-exchange-interface v0.0.1
	github.com/ipfs/go-ipfs-exchange-offline v0.0.1
	github.com/ipfs/go-ipfs-posinfo v0.0.1
	github.com/ipfs/go-ipfs-provider v0.4.3
	github.com/ipfs/go-ipfs-routing v0.1.0
	github.com/ipfs/go-ipfs-util v0.0.1
	github.com/ipfs/go-ipld-cbor v0.0.4
	github.com/ipfs/go-ipld-format v0.2.0
	github.com/ipfs/go-ipld-git v0.0.3
	github.com/ipfs/go-log v1.0.4
	github.com/ipfs/go-merkledag v0.3.2
	github.com/ipfs/go-metrics-interface v0.0.1
	github.com/ipfs/go-metrics-prometheus v0.0.2
	github.com/ipfs/go-path v0.0.7
	github.com/ipfs/go-verifcid v0.0.1
	github.com/ipld/go-car v0.1.0
	github.com/jbenet/go-is-domain v1.0.3
	github.com/jbenet/go-random v0.0.0-20190219211222-123a90aedc0c
	github.com/jbenet/go-temp-err-catcher v0.1.0
	github.com/jbenet/goprocess v0.1.4
	github.com/klauspost/reedsolomon v1.9.9
	github.com/libp2p/go-libp2p v0.8.3
	github.com/libp2p/go-libp2p-circuit v0.2.2
	github.com/libp2p/go-libp2p-connmgr v0.2.1
	github.com/libp2p/go-libp2p-core v0.5.6
	github.com/libp2p/go-libp2p-crypto v0.1.0
	github.com/libp2p/go-libp2p-discovery v0.4.0
	github.com/libp2p/go-libp2p-http v0.1.5
	github.com/libp2p/go-libp2p-kad-dht v0.7.11
	github.com/libp2p/go-libp2p-kbucket v0.4.1
	github.com/libp2p/go-libp2p-loggables v0.1.0
	github.com/libp2p/go-libp2p-mplex v0.2.3
	github.com/libp2p/go-libp2p-peerstore v0.2.3
	github.com/libp2p/go-libp2p-pubsub v0.2.7
	github.com/libp2p/go-libp2p-pubsub-router v0.2.1
	github.com/libp2p/go-libp2p-quic-transport v0.3.7
	github.com/libp2p/go-libp2p-record v0.1.2
	github.com/libp2p/go-libp2p-routing-helpers v0.2.3
	github.com/libp2p/go-libp2p-secio v0.2.2
	github.com/libp2p/go-libp2p-swarm v0.2.3
	github.com/libp2p/go-libp2p-testing v0.1.1
	github.com/libp2p/go-libp2p-tls v0.1.3
	github.com/libp2p/go-libp2p-yamux v0.2.7
	github.com/libp2p/go-maddr-filter v0.0.5
	github.com/libp2p/go-sockaddr v0.1.0 // indirect
	github.com/libp2p/go-socket-activation v0.0.2
	github.com/libp2p/go-testutil v0.1.0
	github.com/looplab/fsm v0.1.0
	github.com/markbates/pkger v0.17.0
	github.com/mattn/go-runewidth v0.0.8 // indirect
	github.com/mgutz/ansi v0.0.0-20170206155736-9520e82c474b // indirect
	github.com/mholt/archiver/v3 v3.3.0
	github.com/mitchellh/go-homedir v1.1.0
	github.com/mmcloughlin/avo v0.0.0-20200523190732-4439b6b2c061 // indirect
	github.com/mr-tron/base58 v1.2.0
	github.com/multiformats/go-multiaddr v0.2.2
	github.com/multiformats/go-multiaddr-dns v0.2.0
	github.com/multiformats/go-multiaddr-net v0.1.5
	github.com/multiformats/go-multibase v0.0.2
	github.com/multiformats/go-multihash v0.0.13
	github.com/natefinch/lumberjack v2.0.0+incompatible
	github.com/opentracing/opentracing-go v1.1.0
	github.com/orcaman/concurrent-map v0.0.0-20190826125027-8c72a8bb44f6
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.5.1
	github.com/shirou/gopsutil v2.20.5+incompatible
	github.com/status-im/keycard-go v0.0.0-20190316090335-8537d3370df4
	github.com/stretchr/testify v1.6.1
	github.com/syndtr/goleveldb v1.0.1-0.20190923125748-758128399b1d
	github.com/tron-us/go-btfs-common v0.5.5
	github.com/tron-us/go-common/v2 v2.1.1
	github.com/tron-us/protobuf v1.3.4
	github.com/tyler-smith/go-bip32 v0.0.0-20170922074101-2c9cfd177564
	github.com/tyler-smith/go-bip39 v1.0.2
	github.com/whyrusleeping/base32 v0.0.0-20170828182744-c30ac30633cc
	github.com/whyrusleeping/go-sysinfo v0.0.0-20190219211824-4a357d4b90b1
	github.com/whyrusleeping/multiaddr-filter v0.0.0-20160516205228-e903e4adabd7
	github.com/whyrusleeping/tar-utils v0.0.0-20180509141711-8c6c8ba81d5c
	go.uber.org/fx v1.12.0
	go.uber.org/zap v1.14.1
	go4.org v0.0.0-20200104003542-c7e774b10ea0
	golang.org/x/crypto v0.0.0-20200622213623-75b288015ac9
	golang.org/x/sys v0.0.0-20200420163511-1957bb5e6d1f
	gopkg.in/cheggaaa/pb.v1 v1.0.28
	gopkg.in/natefinch/lumberjack.v2 v2.0.0 // indirect
	gopkg.in/yaml.v2 v2.2.8
	launchpad.net/gocheck v0.0.0-20140225173054-000000000087 // indirect
)

replace github.com/ipfs/go-ipld-format => github.com/TRON-US/go-ipld-format v0.2.0

replace github.com/ipfs/go-cid => github.com/TRON-US/go-cid v0.2.0

replace github.com/libp2p/go-libp2p-core => github.com/TRON-US/go-libp2p-core v0.5.0

replace github.com/libp2p/go-libp2p-kad-dht => github.com/TRON-US/go-libp2p-kad-dht v0.9.2

replace github.com/multiformats/go-multiaddr => github.com/TRON-US/go-multiaddr v0.3.0

replace github.com/ipfs/go-path => github.com/TRON-US/go-path v0.1.0

replace github.com/ipfs/go-graphsync => github.com/TRON-US/go-graphsync v0.1.2

replace github.com/ipld/go-car => github.com/TRON-US/go-car v0.2.1

replace github.com/ipld/go-ipld-prime-proto => github.com/TRON-US/go-ipld-prime-proto v0.0.2

go 1.14
