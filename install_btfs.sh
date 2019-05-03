#!/bin/bash
export GOPATH=${HOME}/go
export PATH=$PATH:/usr/local/go/bin
export PATH=$PATH:$GOPATH/bin
export GO111MODULE=on
export EDITOR=vim
export GITHUB_TOKEN=9e2b088b6091e4452696aac6503f30d4c16c7c7c
git config --global url."https://${GITHUB_TOKEN}:x-oauth-basic@github.com/TRON-US".insteadOf "https://github.com/TRON-US"
make install
btfs init
# swarm key
cp swarm.key ~/.btfs/swarm.key
# bootstrap node
btfs bootstrap rm --all
btfs bootstrap add /ip4/3.18.120.107/tcp/4001/ipfs/QmcmRdAHQYTtpbs9Ud5rNx6WzHmU9WcYCrBneCSyKhMr7H
btfs bootstrap add /ip4/3.14.203.8/tcp/4001/ipfs/QmTPN8WRQc7vgB9VdgqVqjinwXC2jiSMjG9oT2Um2EVFHe
btfs bootstrap add /ip4/3.14.238.171/tcp/4001/ipfs/QmRb1Vi7JeNMVE2QVvCuWFU2J2qt6rn4pLf31CHyjt9GbB
# cross-origin resource sharing
btfs config --json API.HTTPHeaders.Access-Control-Allow-Origin "[\"*\"]"
btfs config --json API.HTTPHeaders.Access-Control-Allow-Methods '["PUT", "GET", "POST", "OPTIONS"]'
btfs config --json API.HTTPHeaders.Access-Control-Allow-Credentials "[\"true\"]"
btfs config Addresses.API /ip4/0.0.0.0/tcp/5001
btfs config Addresses.Gateway /ip4/0.0.0.0/tcp/8080
echo install successful, please run 'btfs daemon'
