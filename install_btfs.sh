#!/bin/bash
export PATH=$PATH:/usr/local/go/bin
export PATH=$PATH:$GOPATH/bin
export GO111MODULE=on

go clean -modcache
make install
btfs init

# echo a message if btfs is installed successfully
var_btfs=$(which btfs)
if [ -n "$var_btfs" ]; then
    echo install successful, please run 'btfs daemon'
fi
