#!/usr/bin/env bash

# patching $GOPATH/pkg/mod/github.com/multiformats/go-multiaddr@v0.0.4
patchdir=$GOPATH/pkg/mod/github.com/multiformats/go-multiaddr@v0.0.4
chmod -R 777 $patchdir
if ! patch -R -s -f --dry-run $patchdir/protocols.go < ./patches/go-multiaddr-v0.0.4-protocols.go.patch 1>/dev/null; then
    patch $patchdir/protocols.go < ./patches/go-multiaddr-v0.0.4-protocols.go.patch;
fi

# patching $GOPATH/pkg/mod/github.com/ipfs/go-path@v0.0.4
patchdir=$GOPATH/pkg/mod/github.com/ipfs/go-path@v0.0.4
chmod -R 777 $patchdir
if ! patch -R -s -f --dry-run $patchdir/path.go < ./patches/go-path-v0.0.4-path.go.patch 1>/dev/null; then
    patch $patchdir/path.go < ./patches/go-path-v0.0.4-path.go.patch;
fi

# patching $GOPATH/pkg/mod/github.com/ipfs/interface-go-ipfs-core@v0.0.8
patchdir=$GOPATH/pkg/mod/github.com/ipfs/interface-go-ipfs-core@v0.0.8
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
patchdir=$GOPATH/pkg/mod/github.com/libp2p/go-libp2p-kad-dht@v0.0.13
chmod -R 777 $patchdir
if ! patch -R -s -f --dry-run $patchdir/dht_bootstrap.go < ./patches/go-libp2p-kad-dht-v0.0.13-dht_bootstrap.go.patch 1>/dev/null; then
    patch $patchdir/dht_bootstrap.go < ./patches/go-libp2p-kad-dht-v0.0.13-dht_bootstrap.go.patch;
fi

# patching $GOPATH/pkg/mod/github.com/ipfs/go-ipfs-cmds@v0.0.8
patchdir=$GOPATH/pkg/mod/github.com/ipfs/go-ipfs-cmds@v0.0.8
chmod -R 777 $patchdir
if ! patch -R -s -f --dry-run $patchdir/cli/parse.go < ./patches/go-ipfs-cmds-v0.0.8-cli-parse.go.patch 1>/dev/null; then
    patch $patchdir/cli/parse.go < ./patches/go-ipfs-cmds-v0.0.8-cli-parse.go.patch;
fi
if ! patch -R -s -f --dry-run $patchdir/http/client.go < ./patches/go-ipfs-cmds-http-client.go.patch 1>/dev/null; then
    patch $patchdir/http/client.go < ./patches/go-ipfs-cmds-http-client.go.patch;
fi

# patching $GOPATH/pkg/mod/github.com/ipfs/go-ipfs-config@v0.0.3
patchdir=$GOPATH/pkg/mod/github.com/ipfs/go-ipfs-config@v0.0.3
chmod -R 777 $patchdir
# 1) patching init.go
if ! patch -R -s -f --dry-run $patchdir/init.go < ./patches/go-ipfs-config-v0.0.3-init.go.patch 1>/dev/null; then
    patch $patchdir/init.go < ./patches/go-ipfs-config-v0.0.3-init.go.patch;
fi
# 2) patching config.go
if ! patch -R -s -f --dry-run $patchdir/config.go < ./patches/go-ipfs-config-v0.0.3-config.go.patch 1>/dev/null; then
    patch $patchdir/config.go < ./patches/go-ipfs-config-v0.0.3-config.go.patch;
fi
# 3) patching bootstrap_peers.go
if ! patch -R -s -f --dry-run $patchdir/bootstrap_peers.go < ./patches/go-ipfs-config-v0.0.3-bootstrap_peers.go.patch 1>/dev/null; then
    patch $patchdir/bootstrap_peers.go < ./patches/go-ipfs-config-v0.0.3-bootstrap_peers.go.patch;
fi
# 4) patching profile.go
if ! patch -R -s -f --dry-run $patchdir/profile.go < ./patches/go-ipfs-config-v0.0.3-profile.go.patch 1>/dev/null; then
    patch $patchdir/profile.go < ./patches/go-ipfs-config-v0.0.3-profile.go.patch
fi

# patching $GOPATH/pkg/mod/github.com/libp2p/go-libp2p-record@v0.0.1
patchdir=$GOPATH/pkg/mod/github.com/libp2p/go-libp2p-record@v0.0.1
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
