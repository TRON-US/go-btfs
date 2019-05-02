#!/bin/bash
cd $GOPATH/src/github.com/TRON-US/go-btfs
make install
btfs init
# swarm key
cp $GOPATH/src/github.com/TRON-US/go-btfs/swarm.key ~/.btfs/swarm.key
# bootstrap node
btfs bootstrap rm --all
btfs bootstrap add /ip4/3.18.120.107/tcp/4001/ipfs/QmcmRdAHQYTtpbs9Ud5rNx6WzHmU9WcYCrBneCSyKhMr7H
# cross-origin resource sharing
btfs config --json API.HTTPHeaders.Access-Control-Allow-Origin "[\"*\"]"
btfs config --json API.HTTPHeaders.Access-Control-Allow-Methods '["PUT", "GET", "POST", "OPTIONS"]'
btfs config --json API.HTTPHeaders.Access-Control-Allow-Credentials "[\"true\"]"
btfs config Addresses.API /ip4/0.0.0.0/tcp/5001
btfs config Addresses.Gateway /ip4/0.0.0.0/tcp/8080
# start the daemon
nohup btfs daemon </dev/null > /dev/null 2>&1 &
