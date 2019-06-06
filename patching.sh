#!/usr/bin/env bash

# patching $GOPATH/pkg/mod/github.com/multiformats/go-multiaddr@v0.0.1/protocols.go
patchdir=$GOPATH/pkg/mod/github.com/multiformats/go-multiaddr@v0.0.1
chmod -R 777 $patchdir
if ! patch -R -p0 -s -f --dry-run  $patchdir/protocols.go < ./patches/protocols.go.patch; then
    patch -p0 $patchdir/protocols.go < ./patches/protocols.go.patch;
fi