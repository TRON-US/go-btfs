#!/bin/bash
export PATH=$PATH:/usr/local/go/bin
export PATH=$PATH:$GOPATH/bin
export GO111MODULE=on

go clean -modcache
make install
btfs init

# cross-origin resource sharing
btfs config --json API.HTTPHeaders.Access-Control-Allow-Origin "[\"*\"]"
btfs config --json API.HTTPHeaders.Access-Control-Allow-Methods '["PUT", "GET", "POST", "OPTIONS"]'
btfs config --json API.HTTPHeaders.Access-Control-Allow-Credentials "[\"true\"]"
btfs config Addresses.API /ip4/0.0.0.0/tcp/5001
btfs config Addresses.Gateway /ip4/0.0.0.0/tcp/8080

# echo a message if btfs is installed successfully
var_btfs=$(which btfs)
if [ -n "$var_btfs" ]; then
    echo install successful, please run 'btfs daemon'
fi
