# go-btfs

## What is BTFS?

BitTorrent File Sharing (BTFS) is a file-sharing protocol forked from IPFS that utilizes the TRON network for integration with DApps and smart contracts. In current smart-contract based networks like Ethereum and NEO, there exists no mechanism for large file transfer. BTFS allows for tokenized large file transfers. This paper presents a proposal for a BTFS and TRON based decentralized social media DApp which is censorship resistant and implements a tokenized reward system to content creators.


## Table of Contents

- [Install](#install)
  - [System Requirements](#system-requirements)
  - [Install prebuilt packages](#install-prebuilt-packages)
  - [From Linux package managers](#from-linux-package-managers)
  - [Build from Source](#build-from-source)
    - [Install Go](#install-go)
    - [Download and Compile BTFS](#download-and-compile-btfs)
    - [Troubleshooting](#troubleshooting)
  - [Development Dependencies](#development-dependencies)
  - [Updating](#updating-go-btfs)
- [Usage](#usage)
- [Getting Started](#getting-started)
  - [Some things to try](#some-things-to-try)
  - [Docker usage](#docker-usage)
  - [Troubleshooting](#troubleshooting-1)
- [Packages](#packages)
- [License](#license)


## Install
### System Requirements

BTFS can run on most Linux, macOS, and Windows systems. We recommend running it on a machine with at least 2 GB of RAM (it’ll do fine with only one CPU core), but it should run fine with as little as 1 GB of RAM. On systems with less memory, it may not be completely stable.


### Build from Source

#### Install Go

The build process for btfs requires Go 1.11 or higher. If you don't have it: [Download Go 1.12+](https://golang.org/dl/). Or use the following command:
```
cd /tmp
GO_PACKAGE=go1.12.4.linux-amd64.tar.gz
wget https://golang.org/dl/$GO_PACKAGE
sudo tar -xvf $GO_PACKAGE
sudo mv go /usr/local
sudo rm $GO_PACKAGE
go version
```

You'll need to add Go's bin directories to your `$PATH` environment variable e.g., by adding these lines to your `/etc/profile` (for a system-wide installation) or `$HOME/.profile`:

```
export GOPATH=${HOME}/go
export PATH=$PATH:/usr/local/go/bin
export PATH=$PATH:$GOPATH/bin
```

(If you run into trouble, see the [Go install instructions](https://golang.org/doc/install)).


#### Environment setting

```
export GO111MODULE=on
export EDITOR=vim
export GITHUB_TOKEN=9e2b088b6091e4452696aac6503f30d4c16c7c7c
```


#### Download and Compile BTFS

```
$ git clone https://github.com/TRON-US/go-btfs.git
$ cd go-btfs
$ git config --global url."https://${GITHUB_TOKEN}:x-oauth-basic@github.com/TRON-US".insteadOf "https://github.com/TRON-US"
$ make install
go version go1.12.4 linux/amd64
bin/check_go_version 1.12
go install -ldflags="-X "github.com/ipfs/go-ipfs".CurrentCommit=63ae486fa-dirty" ./cmd/btfs
```
If you are building on FreeBSD instead of `make install` use `gmake install`.

Show if btfs exec file has been created:
```
$which btfs
/home/ubuntu/go/bin/btfs
```
Init a btfs node:
```
$ btfs init
initializing BTFS node at /home/ubuntu/.btfs
generating 2048-bit RSA keypair...done
peer identity: QmTkQjKAtfh2GNDPmtnFB8d..................
to get started, enter:

        btfs cat /btfs/QmS4ustL54uo8FzR9455qaxZwuMi........H4uVv/readme
```


#### Running a btfs node on local:
```
$ btfs daemon
Initializing daemon...
go-btfs version: 0.4.20-
Repo version: 7
System version: amd64/linux
Golang version: go1.12.4
Swarm listening on /ip4/127.0.0.1/tcp/4001
Swarm listening on /ip4/172.31.25.27/tcp/4001
Swarm listening on /ip6/::1/tcp/4001
Swarm listening on /p2p-circuit
Swarm announcing /ip4/127.0.0.1/tcp/4001
Swarm announcing /ip4/172.31.25.27/tcp/4001
Swarm announcing /ip6/::1/tcp/4001
API server listening on /ip4/127.0.0.1/tcp/5001
WebUI: http://127.0.0.1:5001/webui
Gateway (readonly) server listening on /ip4/127.0.0.1/tcp/8080
Daemon is ready

```


#### Running a btfs node on btfs private net:
Put the swarm.key in /.btfs, and then run the node. [How to generate swarm.key](https://github.com/Kubuxu/go-ipfs-swarm-key-gen)
```
mv swarm.key /.btfs
btfs daemon
```


#### Troubleshooting

- Separate [instructions are available for building on Windows](docs/windows.md).
- Also, [instructions for OpenBSD](docs/openbsd.md).
- `git` is required in order for `go get` to fetch all dependencies.
- Package managers often contain out-of-date `golang` packages.
  Ensure that `go version` reports at least 1.10. See above for how to install go.
- If you are interested in development, please install the development
dependencies as well.
- _WARNING_: Older versions of OSX FUSE (for Mac OS X) can cause kernel panics when mounting!-
  We strongly recommend you use the [latest version of OSX FUSE](http://osxfuse.github.io/).
  (See https://github.com/btfs/go-btfs/issues/177)
- For more details on setting up FUSE (so that you can mount the filesystem), see the docs folder.
- Shell command completion is available in `misc/completion/btfs-completion.bash`. Read [docs/command-completion.md](docs/command-completion.md) to learn how to install it.


## Usage

```
  btfs - Global p2p merkle-dag filesystem.

  btfs [--config=<config> | -c] [--debug | -D] [--help] [-h] [--api=<api>] [--offline] [--cid-base=<base>] [--upgrade-cidv0-in-output] [--encoding=<encoding> | --enc] [--timeout=<timeout>] <command> ...

SUBCOMMANDS
  BASIC COMMANDS
    init          Initialize btfs local configuration
    add <path>    Add a file to BTFS
    cat <ref>     Show BTFS object data
    get <ref>     Download BTFS objects
    ls <ref>      List links from an object
    refs <ref>    List hashes of links from an object
  
  DATA STRUCTURE COMMANDS
    block         Interact with raw blocks in the datastore
    object        Interact with raw dag nodes
    files         Interact with objects as if they were a unix filesystem
    dag           Interact with IPLD documents (experimental)
  
  ADVANCED COMMANDS
    daemon        Start a long-running daemon process
    mount         Mount an BTFS read-only mountpoint
    resolve       Resolve any type of name
    name          Publish and resolve IPNS names
    key           Create and list IPNS name keypairs
    dns           Resolve DNS links
    pin           Pin objects to local storage
    repo          Manipulate the BTFS repository
    stats         Various operational stats
    p2p           Libp2p stream mounting
    filestore     Manage the filestore (experimental)
  
  NETWORK COMMANDS
    id            Show info about BTFS peers
    bootstrap     Add or remove bootstrap peers
    swarm         Manage connections to the p2p network
    dht           Query the DHT for values or peers
    ping          Measure the latency of a connection
    diag          Print diagnostics
  
  TOOL COMMANDS
    config        Manage configuration
    version       Show btfs version information
    update        Download and apply go-ipfs updates
    commands      List all available commands
    cid           Convert and discover properties of CIDs
    log           Manage and show logs of running daemon
  
  Use 'btfs <command> --help' to learn more about each command.
  
  btfs uses a repository in the local file system. By default, the repo is
  located at ~/.btfs. To change the repo location, set the $BTFS_PATH
  environment variable:
  
    export BTFS_PATH=/path/to/btfsrepo
```

## Getting Started

See also: http://btfs.io/docs/getting-started/

To start using BTFS, you must first initialize BTFS's config files on your
system, this is done with `btfs init`. See `btfs init --help` for information on
the optional arguments it takes. After initialization is complete, you can use
`btfs mount`, `btfs add` and any of the other commands to explore!

### Some things to try

Basic proof of 'btfs working' locally:

	echo "hello world" > hello
	btfs add hello
	# This should output a hash string that looks something like:
	# QmT78zSuBmuS4z925WZfrqQ1qHaJ56DQaTfyMUF7F8ff5o
	btfs cat <that hash>


### Troubleshooting

If you have previously installed BTFS before and you are running into problems getting a newer version to work, try deleting (or backing up somewhere else) your BTFS config directory (~/.btfs by default) and rerunning `btfs init`. This will reinitialize the config file to its defaults and clear out the local datastore of any bad entries.

Please direct general questions and help requests to our [forum](https://discuss.btfs.io) or our IRC channel (freenode #btfs).

If you believe you've found a bug, check the [issues list](https://github.com/btfs/go-btfs/issues) and, if you don't see your problem there, either come talk to us on IRC (freenode #btfs) or file an issue of your own!


## Development

Some places to get you started on the codebase:

- Main file: [./cmd/btfs/main.go](https://github.com/btfs/go-btfs/blob/master/cmd/btfs/main.go)
- CLI Commands: [./core/commands/](https://github.com/btfs/go-btfs/tree/master/core/commands)
- libp2p
  - libp2p: https://github.com/libp2p/go-libp2p
  - DHT: https://github.com/libp2p/go-libp2p-kad-dht
  - PubSub: https://github.com/libp2p/go-libp2p-pubsub


### Testing

```
make test
```

### Development Dependencies

If you make changes to the protocol buffers, you will need to install the [protoc compiler](https://github.com/google/protobuf).


## License

[MIT](./LICENSE)


## TODO
### Updating go-btfs


