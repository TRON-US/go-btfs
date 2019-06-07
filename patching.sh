#!/usr/bin/env bash

# patching $GOPATH/pkg/mod/github.com/multiformats/go-multiaddr@v0.0.1
patchdir=$GOPATH/pkg/mod/github.com/multiformats/go-multiaddr@v0.0.1
chmod -R 777 $patchdir
if ! patch -R -p0 -s -f --dry-run  $patchdir/protocols.go < ./patches/go-multiaddr-v0.0.1-protocols.go.patch; then
    patch -p0 $patchdir/protocols.go < ./patches/go-multiaddr-v0.0.1-protocols.go.patch;
fi

# patching $GOPATH/pkg/mod/github.com/ipfs/go-path@v0.0.3
patchdir=$GOPATH/pkg/mod/github.com/ipfs/go-path@v0.0.3
chmod -R 777 $patchdir
if ! patch -R -p0 -s -f --dry-run  $patchdir/path.go < ./patches/go-path-v0.0.3-path.go.patch; then
    patch -p0 $patchdir/path.go < ./patches/go-path-v0.0.3-path.go.patch;
fi

# patching $GOPATH/pkg/mod/github.com/ipfs/interface-go-ipfs-core@v0.0.6
patchdir=$GOPATH/pkg/mod/github.com/ipfs/interface-go-ipfs-core@v0.0.6
chmod -R 777 $patchdir
if ! patch -R -p0 -s -f --dry-run  $patchdir/path.go < ./patches/interface-go-ipfs-core-v0.0.6-path.go.patch; then
    patch -p0 $patchdir/path.go < ./patches/interface-go-ipfs-core-v0.0.6-path.go.patch;
fi

# patching $GOPATH/pkg/mod/github.com/libp2p/go-libp2p-kad-dht@v0.0.7
patchdir=$GOPATH/pkg/mod/github.com/libp2p/go-libp2p-kad-dht@v0.0.7
chmod -R 777 $patchdir
if ! patch -R -p0 -s -f --dry-run  $patchdir/dht_bootstrap.go < ./patches/go-libp2p-kad-dht-v0.0.7-dht_bootstrap.go.patch; then
    patch -p0 $patchdir/dht_bootstrap.go < ./patches/go-libp2p-kad-dht-v0.0.7-dht_bootstrap.go.patch;
fi

# patching $GOPATH/pkg/mod/github.com/ipfs/go-ipfs-cmds@v0.0.5
patchdir=$GOPATH/pkg/mod/github.com/ipfs/go-ipfs-cmds@v0.0.5
chmod -R 777 $patchdir
if ! patch -R -p0 -s -f --dry-run  $patchdir/cli/parse.go < ./patches/go-ipfs-cmds-v0.0.5-cli-parse.go.patch; then
    patch -p0 $patchdir/cli/parse.go < ./patches/go-ipfs-cmds-v0.0.5-cli-parse.go.patch;
fi

# patching $GOPATH/pkg/mod/github.com/ipfs/go-ipfs-config@v0.0.1
patchdir=$GOPATH/pkg/mod/github.com/ipfs/go-ipfs-config@v0.0.1
chmod -R 777 $patchdir
# 1) patching init.go
if ! patch -R -p0 -s -f --dry-run  $patchdir/init.go < ./patches/go-ipfs-config-v0.0.1-init.go.patch; then
    patch -p0 $patchdir/init.go < ./patches/go-ipfs-config-v0.0.1-init.go.patch;
fi
# 2) patching config.go
if ! patch -R -p0 -s -f --dry-run  $patchdir/config.go < ./patches/go-ipfs-config-v0.0.1-config.go.patch; then
    patch -p0 $patchdir/config.go < ./patches/go-ipfs-config-v0.0.1-config.go.patch;
fi
# 3) patching bootstrap_peers.go
if ! patch -R -p0 -s -f --dry-run  $patchdir/bootstrap_peers.go < ./patches/go-ipfs-config-v0.0.1-bootstrap_peers.go.patch; then
    patch -p0 $patchdir/bootstrap_peers.go < ./patches/go-ipfs-config-v0.0.1-bootstrap_peers.go.patch;
fi
