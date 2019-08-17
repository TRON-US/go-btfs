#!/usr/bin/env bash

# patching $GOPATH/pkg/mod/github.com/multiformats/go-multiaddr@v0.0.4
patchdir=$GOPATH/pkg/mod/github.com/multiformats/go-multiaddr@v0.0.4
chmod -R 777 $patchdir
if ! patch -R -s -f --dry-run $patchdir/protocols.go < ./patches/go-multiaddr-v0.0.4-protocols.go.patch 1>/dev/null; then
    patch $patchdir/protocols.go < ./patches/go-multiaddr-v0.0.4-protocols.go.patch;
fi

# patching $GOPATH/pkg/mod/github.com/ipfs/go-path@v0.0.4
patchdir=$GOPATH/pkg/mod/github.com/ipfs/go-path@v0.0.7
chmod -R 777 $patchdir
if ! patch -R -s -f --dry-run $patchdir/path.go < ./patches/go-path-v0.0.4-path.go.patch 1>/dev/null; then
    patch $patchdir/path.go < ./patches/go-path-v0.0.4-path.go.patch;
fi

# patching $GOPATH/pkg/mod/github.com/ipfs/interface-go-ipfs-core@v0.0.8
patchdir=$GOPATH/pkg/mod/github.com/ipfs/interface-go-ipfs-core@v0.1.0
chmod -R 777 $patchdir
# 1) path.go
if ! patch -R -s -f --dry-run $patchdir/path/path.go < ./patches/interface-go-ipfs-core-v0.0.8-path.go.patch 1>/dev/null; then
    patch $patchdir/path/path.go < ./patches/interface-go-ipfs-core-v0.0.8-path.go.patch;
fi
# 2) errors.go
if ! patch -R -s -f --dry-run $patchdir/errors.go < ./patches/interface-go-ipfs-core-v0.0.8-errors.go.patch 1>/dev/null; then
    patch $patchdir/errors.go < ./patches/interface-go-ipfs-core-v0.0.8-errors.go.patch;
fi

# patching $GOPATH/pkg/mod/github.com/libp2p/go-libp2p-kad-dht@v0.0.13
patchdir=$GOPATH/pkg/mod/github.com/libp2p/go-libp2p-kad-dht@v0.1.1
chmod -R 777 $patchdir
if ! patch -R -s -f --dry-run $patchdir/dht_bootstrap.go < ./patches/go-libp2p-kad-dht-v0.0.13-dht_bootstrap.go.patch 1>/dev/null; then
    patch $patchdir/dht_bootstrap.go < ./patches/go-libp2p-kad-dht-v0.0.13-dht_bootstrap.go.patch;
fi

# patching $GOPATH/pkg/mod/github.com/libp2p/go-libp2p-record@v0.0.1
patchdir=$GOPATH/pkg/mod/github.com/libp2p/go-libp2p-record@v0.1.0
chmod -R 777 $patchdir
if ! patch -R -s -f --dry-run $patchdir/validator.go < ./patches/go-libp2p-record-v0.0.1-validator.go.patch 1>/dev/null; then
    patch $patchdir/validator.go < ./patches/go-libp2p-record-v0.0.1-validator.go.patch;
fi

# patching $GOPATH/pkg/mod/github.com/ipfs/go-ipns@v0.0.1
patchdir=$GOPATH/pkg/mod/github.com/ipfs/go-ipns@v0.0.1
chmod -R 777 $patchdir
if ! patch -R -s -f --dry-run $patchdir/record.go < ./patches/go-ipns-v.0.0.1-record.go.patch 1>/dev/null; then
    patch $patchdir/record.go < ./patches/go-ipns-v.0.0.1-record.go.patch;
fi

# patching test
patchdir=$GOPATH/pkg/mod/github.com/ipfs/interface-go-ipfs-core@v0.0.8
cp ./patches/test/interface-go-ipfs-core.patch $patchdir
cd $patchdir
if ! patch -p1 -R -s -f --dry-run < interface-go-ipfs-core.patch 1>/dev/null; then
    patch -p1 < interface-go-ipfs-core.patch
fi
