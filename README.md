# go-btfs

![banner](https://btfs.io/btfs/QmVk7srrwahXLNmcDYvyUEJptyoxpndnRa57YJ11L4jV26/btfs.go.png)

[![](https://img.shields.io/badge/made%20by-Protocol%20Labs-blue.svg?style=flat-square)](http://ipn.io)
[![](https://img.shields.io/badge/project-BTFS-blue.svg?style=flat-square)](http://btfs.io/)
[![](https://img.shields.io/badge/freenode-%23btfs-blue.svg?style=flat-square)](http://webchat.freenode.net/?channels=%23btfs)
[![standard-readme compliant](https://img.shields.io/badge/standard--readme-OK-green.svg?style=flat-square)](https://github.com/RichardLitt/standard-readme)
[![GoDoc](https://godoc.org/github.com/btfs/go-btfs?status.svg)](https://godoc.org/github.com/btfs/go-btfs)
[![Build Status](https://travis-ci.com/btfs/go-btfs.svg?branch=master)](https://travis-ci.com/btfs/go-btfs)

## What is BTFS?

BTFS is a global, versioned, peer-to-peer filesystem. It combines good ideas from Git, BitTorrent, Kademlia, SFS, and the Web. It is like a single bittorrent swarm, exchanging git objects. BTFS provides an interface as simple as the HTTP web, but with permanence built in. You can also mount the world at /btfs.

For more info see: https://github.com/btfs/btfs.

Please put all issues regarding:
  - BTFS _design_ in the [btfs repo issues](https://github.com/btfs/btfs/issues).
  - Go BTFS _implementation_ in [this repo](https://github.com/btfs/go-btfs/issues).

## Table of Contents

- [Security Issues](#security-issues)
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
- [Development](#development)
- [Contributing](#contributing)
- [License](#license)

## Security Issues

The BTFS protocol and its implementations are still in heavy development. This means that there may be problems in our protocols, or there may be mistakes in our implementations. And -- though BTFS is not production-ready yet -- many people are already running nodes in their machines. So we take security vulnerabilities very seriously. If you discover a security issue, please bring it to our attention right away!

If you find a vulnerability that may affect live deployments -- for example, by exposing a remote execution exploit -- please send your report privately to security@btfs.io. Please DO NOT file a public issue. The GPG key for security@btfs.io is [4B9665FB 92636D17 7C7A86D3 50AAE8A9 59B13AF3](https://pgp.mit.edu/pks/lookup?op=get&search=0x50AAE8A959B13AF3).

If the issue is a protocol weakness that cannot be immediately exploited or something not yet deployed, just discuss it openly.

## Install

The canonical download instructions for BTFS are over at: https://docs.btfs.io/introduction/install/. It is **highly suggested** you follow those instructions if you are not interested in working on BTFS development.

### System Requirements

BTFS can run on most Linux, macOS, and Windows systems. We recommend running it on a machine with at least 2 GB of RAM (it’ll do fine with only one CPU core), but it should run fine with as little as 1 GB of RAM. On systems with less memory, it may not be completely stable.

### Install prebuilt packages

We host prebuilt binaries over at our [distributions page](https://btfs.io/ipns/dist.btfs.io#go-btfs).

From there:
- Click the blue "Download go-btfs" on the right side of the page.
- Open/extract the archive.
- Move `btfs` to your path (`install.sh` can do it for you).

You can also download go-btfs from this project's GitHub releases page if you are unable to access btfs.io.

### From Linux package managers

- [Arch Linux](#arch-linux)
- [Nix](#nix)
- [Snap](#snap)

#### Arch Linux

In Arch Linux go-btfs is available as
[go-btfs](https://www.archlinux.org/packages/community/x86_64/go-btfs/) package.

```
$ sudo pacman -S go-btfs
```

Development version of go-btfs is also on AUR under
[go-btfs-git](https://aur.archlinux.org/packages/go-btfs-git/).
You can install it using your favourite AUR Helper or manually from AUR.

#### Nix

For Linux and MacOSX you can use the purely functional package manager [Nix](https://nixos.org/nix/):

```
$ nix-env -i btfs
```

You can also install the Package by using it's attribute name, which is also `btfs`.

#### Guix

GNU's functional package manager, [Guix](https://www.gnu.org/software/guix/), also provides a go-btfs package:

```
$ guix package -i go-btfs
```

#### Snap

With snap, in any of the [supported Linux distributions](https://snapcraft.io/docs/core/install):

```
$ sudo snap install btfs
```

### Build from Source

#### Install Go

The build process for btfs requires Go 1.11 or higher. If you don't have it: [Download Go 1.11+](https://golang.org/dl/).

You'll need to add Go's bin directories to your `$PATH` environment variable e.g., by adding these lines to your `/etc/profile` (for a system-wide installation) or `$HOME/.profile`:

```
export PATH=$PATH:/usr/local/go/bin
export PATH=$PATH:$GOPATH/bin
```

(If you run into trouble, see the [Go install instructions](https://golang.org/doc/install)).

#### Download and Compile BTFS

```
$ git clone https://github.com/btfs/go-btfs.git

$ cd go-btfs
$ make install
```

If you are building on FreeBSD instead of `make install` use `gmake install`.

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
- See the [init examples](https://github.com/btfs/website/tree/master/static/docs/examples/init) for how to connect BTFS to systemd or whatever init system your distro uses.

### Updating go-btfs

#### Using btfs-update

BTFS has an updating tool that can be accessed through `btfs update`. The tool is
not installed alongside BTFS in order to keep that logic independent of the main
codebase. To install `btfs update`, [download it here](https://btfs.io/ipns/dist.btfs.io/#btfs-update).

#### Downloading BTFS builds using BTFS

List the available versions of go-btfs:

```
$ btfs cat /ipns/dist.btfs.io/go-btfs/versions
```

Then, to view available builds for a version from the previous command ($VERSION):

```
$ btfs ls /ipns/dist.btfs.io/go-btfs/$VERSION
```

To download a given build of a version:

```
$ btfs get /ipns/dist.btfs.io/go-btfs/$VERSION/go-btfs_$VERSION_darwin-386.tar.gz # darwin 32-bit build
$ btfs get /ipns/dist.btfs.io/go-btfs/$VERSION/go-btfs_$VERSION_darwin-amd64.tar.gz # darwin 64-bit build
$ btfs get /ipns/dist.btfs.io/go-btfs/$VERSION/go-btfs_$VERSION_freebsd-amd64.tar.gz # freebsd 64-bit build
$ btfs get /ipns/dist.btfs.io/go-btfs/$VERSION/go-btfs_$VERSION_linux-386.tar.gz # linux 32-bit build
$ btfs get /ipns/dist.btfs.io/go-btfs/$VERSION/go-btfs_$VERSION_linux-amd64.tar.gz # linux 64-bit build
$ btfs get /ipns/dist.btfs.io/go-btfs/$VERSION/go-btfs_$VERSION_linux-arm.tar.gz # linux arm build
$ btfs get /ipns/dist.btfs.io/go-btfs/$VERSION/go-btfs_$VERSION_windows-amd64.zip # windows 64-bit build
```

## Usage

```
  btfs - Global p2p merkle-dag filesystem.

  btfs [<flags>] <command> [<arg>] ...

SUBCOMMANDS
  BASIC COMMANDS
    init          Initialize btfs local configuration
    add <path>    Add a file to btfs
    cat <ref>     Show btfs object data
    get <ref>     Download btfs objects
    ls <ref>      List links from an object
    refs <ref>    List hashes of links from an object

  DATA STRUCTURE COMMANDS
    block         Interact with raw blocks in the datastore
    object        Interact with raw dag nodes
    files         Interact with objects as if they were a unix filesystem

  ADVANCED COMMANDS
    daemon        Start a long-running daemon process
    mount         Mount an btfs read-only mountpoint
    resolve       Resolve any type of name
    name          Publish or resolve IPNS names
    dns           Resolve DNS links
    pin           Pin objects to local storage
    repo          Manipulate an BTFS repository

  NETWORK COMMANDS
    id            Show info about btfs peers
    bootstrap     Add or remove bootstrap peers
    swarm         Manage connections to the p2p network
    dht           Query the DHT for values or peers
    ping          Measure the latency of a connection
    diag          Print diagnostics

  TOOL COMMANDS
    config        Manage configuration
    version       Show btfs version information
    update        Download and apply go-btfs updates
    commands      List all available commands

  Use 'btfs <command> --help' to learn more about each command.

  btfs uses a repository in the local file system. By default, the repo is located
  at ~/.btfs. To change the repo location, set the $BTFS_PATH environment variable:

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


### Docker usage

An BTFS docker image is hosted at [hub.docker.com/r/btfs/go-btfs](https://hub.docker.com/r/btfs/go-btfs/).
To make files visible inside the container you need to mount a host directory
with the `-v` option to docker. Choose a directory that you want to use to
import/export files from BTFS. You should also choose a directory to store
BTFS files that will persist when you restart the container.

    export btfs_staging=</absolute/path/to/somewhere/>
    export btfs_data=</absolute/path/to/somewhere_else/>

Start a container running btfs and expose ports 4001, 5001 and 8080:

    docker run -d --name btfs_host -v $btfs_staging:/export -v $btfs_data:/data/btfs -p 4001:4001 -p 127.0.0.1:8080:8080 -p 127.0.0.1:5001:5001 btfs/go-btfs:latest

Watch the btfs log:

    docker logs -f btfs_host

Wait for btfs to start. btfs is running when you see:

    Gateway (readonly) server
    listening on /ip4/0.0.0.0/tcp/8080

You can now stop watching the log.

Run btfs commands:

    docker exec btfs_host btfs <args...>

For example: connect to peers

    docker exec btfs_host btfs swarm peers

Add files:

    cp -r <something> $btfs_staging
    docker exec btfs_host btfs add -r /export/<something>

Stop the running container:

    docker stop btfs_host

When starting a container running btfs for the first time with an empty data directory, it will call `btfs init` to initialize configuration files and generate a new keypair. At this time, you can choose which profile to apply using the `BTFS_PROFILE` environment variable:

    docker run -d --name btfs_host -e BTFS_PROFILE=server -v $btfs_staging:/export -v $btfs_data:/data/btfs -p 4001:4001 -p 127.0.0.1:8080:8080 -p 127.0.0.1:5001:5001 btfs/go-btfs:latest

### Troubleshooting

If you have previously installed BTFS before and you are running into problems getting a newer version to work, try deleting (or backing up somewhere else) your BTFS config directory (~/.btfs by default) and rerunning `btfs init`. This will reinitialize the config file to its defaults and clear out the local datastore of any bad entries.

Please direct general questions and help requests to our [forum](https://discuss.btfs.io) or our IRC channel (freenode #btfs).

If you believe you've found a bug, check the [issues list](https://github.com/btfs/go-btfs/issues) and, if you don't see your problem there, either come talk to us on IRC (freenode #btfs) or file an issue of your own!

## Packages

> This table is generated using the module [`package-table`](https://github.com/btfs-shipyard/package-table) with `package-table --data=package-list.json`.

Listing of the main packages used in the BTFS ecosystem. There are also three specifications worth linking here:

| Name | CI/Travis | Coverage | Description |
| ---------|---------|---------|--------- |
| **Files** |
| [`go-unixfs`](//github.com/btfs/go-unixfs) | [![Travis CI](https://travis-ci.com/btfs/go-unixfs.svg?branch=master)](https://travis-ci.com/btfs/go-unixfs) | [![codecov](https://codecov.io/gh/btfs/go-unixfs/branch/master/graph/badge.svg)](https://codecov.io/gh/btfs/go-unixfs) | the core 'filesystem' logic |
| [`go-mfs`](//github.com/btfs/go-mfs) | [![Travis CI](https://travis-ci.com/btfs/go-mfs.svg?branch=master)](https://travis-ci.com/btfs/go-mfs) | [![codecov](https://codecov.io/gh/btfs/go-mfs/branch/master/graph/badge.svg)](https://codecov.io/gh/btfs/go-mfs) | a mutable filesystem editor for unixfs |
| [`go-btfs-posinfo`](//github.com/btfs/go-btfs-posinfo) | [![Travis CI](https://travis-ci.com/btfs/go-btfs-posinfo.svg?branch=master)](https://travis-ci.com/btfs/go-btfs-posinfo) | [![codecov](https://codecov.io/gh/btfs/go-btfs-posinfo/branch/master/graph/badge.svg)](https://codecov.io/gh/btfs/go-btfs-posinfo) | helper datatypes for the filestore |
| [`go-btfs-chunker`](//github.com/btfs/go-btfs-chunker) | [![Travis CI](https://travis-ci.com/btfs/go-btfs-chunker.svg?branch=master)](https://travis-ci.com/btfs/go-btfs-chunker) | [![codecov](https://codecov.io/gh/btfs/go-btfs-chunker/branch/master/graph/badge.svg)](https://codecov.io/gh/btfs/go-btfs-chunker) | file chunkers |
| **Exchange** |
| [`go-btfs-exchange-interface`](//github.com/btfs/go-btfs-exchange-interface) | [![Travis CI](https://travis-ci.com/btfs/go-btfs-exchange-interface.svg?branch=master)](https://travis-ci.com/btfs/go-btfs-exchange-interface) | [![codecov](https://codecov.io/gh/btfs/go-btfs-exchange-interface/branch/master/graph/badge.svg)](https://codecov.io/gh/btfs/go-btfs-exchange-interface) | exchange service interface |
| [`go-btfs-exchange-offline`](//github.com/btfs/go-btfs-exchange-offline) | [![Travis CI](https://travis-ci.com/btfs/go-btfs-exchange-offline.svg?branch=master)](https://travis-ci.com/btfs/go-btfs-exchange-offline) | [![codecov](https://codecov.io/gh/btfs/go-btfs-exchange-offline/branch/master/graph/badge.svg)](https://codecov.io/gh/btfs/go-btfs-exchange-offline) | (dummy) offline implementation of the exchange service |
| [`go-bitswap`](//github.com/btfs/go-bitswap) | [![Travis CI](https://travis-ci.com/btfs/go-bitswap.svg?branch=master)](https://travis-ci.com/btfs/go-bitswap) | [![codecov](https://codecov.io/gh/btfs/go-bitswap/branch/master/graph/badge.svg)](https://codecov.io/gh/btfs/go-bitswap) | bitswap protocol implementation |
| [`go-blockservice`](//github.com/btfs/go-blockservice) | [![Travis CI](https://travis-ci.com/btfs/go-blockservice.svg?branch=master)](https://travis-ci.com/btfs/go-blockservice) | [![codecov](https://codecov.io/gh/btfs/go-blockservice/branch/master/graph/badge.svg)](https://codecov.io/gh/btfs/go-blockservice) | service that plugs a blockstore and an exchange together |
| **Datastores** |
| [`go-datastore`](//github.com/btfs/go-datastore) | [![Travis CI](https://travis-ci.com/btfs/go-datastore.svg?branch=master)](https://travis-ci.com/btfs/go-datastore) | [![codecov](https://codecov.io/gh/btfs/go-datastore/branch/master/graph/badge.svg)](https://codecov.io/gh/btfs/go-datastore) | datastore interfaces, adapters, and basic implementations |
| [`go-btfs-ds-help`](//github.com/btfs/go-btfs-ds-help) | [![Travis CI](https://travis-ci.com/btfs/go-btfs-ds-help.svg?branch=master)](https://travis-ci.com/btfs/go-btfs-ds-help) | [![codecov](https://codecov.io/gh/btfs/go-btfs-ds-help/branch/master/graph/badge.svg)](https://codecov.io/gh/btfs/go-btfs-ds-help) | datastore utility functions |
| [`go-ds-flatfs`](//github.com/btfs/go-ds-flatfs) | [![Travis CI](https://travis-ci.com/btfs/go-ds-flatfs.svg?branch=master)](https://travis-ci.com/btfs/go-ds-flatfs) | [![codecov](https://codecov.io/gh/btfs/go-ds-flatfs/branch/master/graph/badge.svg)](https://codecov.io/gh/btfs/go-ds-flatfs) | a filesystem-based datastore |
| [`go-ds-measure`](//github.com/btfs/go-ds-measure) | [![Travis CI](https://travis-ci.com/btfs/go-ds-measure.svg?branch=master)](https://travis-ci.com/btfs/go-ds-measure) | [![codecov](https://codecov.io/gh/btfs/go-ds-measure/branch/master/graph/badge.svg)](https://codecov.io/gh/btfs/go-ds-measure) | a metric-collecting database adapter |
| [`go-ds-leveldb`](//github.com/btfs/go-ds-leveldb) | [![Travis CI](https://travis-ci.com/btfs/go-ds-leveldb.svg?branch=master)](https://travis-ci.com/btfs/go-ds-leveldb) | [![codecov](https://codecov.io/gh/btfs/go-ds-leveldb/branch/master/graph/badge.svg)](https://codecov.io/gh/btfs/go-ds-leveldb) | a leveldb based datastore |
| [`go-ds-badger`](//github.com/btfs/go-ds-badger) | [![Travis CI](https://travis-ci.com/btfs/go-ds-badger.svg?branch=master)](https://travis-ci.com/btfs/go-ds-badger) | [![codecov](https://codecov.io/gh/btfs/go-ds-badger/branch/master/graph/badge.svg)](https://codecov.io/gh/btfs/go-ds-badger) | a badgerdb based datastore |
| **Namesys** |
| [`go-ipns`](//github.com/btfs/go-ipns) | [![Travis CI](https://travis-ci.com/btfs/go-ipns.svg?branch=master)](https://travis-ci.com/btfs/go-ipns) | [![codecov](https://codecov.io/gh/btfs/go-ipns/branch/master/graph/badge.svg)](https://codecov.io/gh/btfs/go-ipns) | IPNS datastructures and validation logic |
| **Repo** |
| [`go-btfs-config`](//github.com/btfs/go-btfs-config) | [![Travis CI](https://travis-ci.com/btfs/go-btfs-config.svg?branch=master)](https://travis-ci.com/btfs/go-btfs-config) | [![codecov](https://codecov.io/gh/btfs/go-btfs-config/branch/master/graph/badge.svg)](https://codecov.io/gh/btfs/go-btfs-config) | go-btfs config file definitions |
| [`go-fs-lock`](//github.com/btfs/go-fs-lock) | [![Travis CI](https://travis-ci.com/btfs/go-fs-lock.svg?branch=master)](https://travis-ci.com/btfs/go-fs-lock) | [![codecov](https://codecov.io/gh/btfs/go-fs-lock/branch/master/graph/badge.svg)](https://codecov.io/gh/btfs/go-fs-lock) | lockfile management functions |
| [`fs-repo-migrations`](//github.com/btfs/fs-repo-migrations) | [![Travis CI](https://travis-ci.com/btfs/fs-repo-migrations.svg?branch=master)](https://travis-ci.com/btfs/fs-repo-migrations) | [![codecov](https://codecov.io/gh/btfs/fs-repo-migrations/branch/master/graph/badge.svg)](https://codecov.io/gh/btfs/fs-repo-migrations) | repo migrations |
| **Blocks** |
| [`go-block-format`](//github.com/btfs/go-block-format) | [![Travis CI](https://travis-ci.com/btfs/go-block-format.svg?branch=master)](https://travis-ci.com/btfs/go-block-format) | [![codecov](https://codecov.io/gh/btfs/go-block-format/branch/master/graph/badge.svg)](https://codecov.io/gh/btfs/go-block-format) | block interfaces and implementations |
| [`go-btfs-blockstore`](//github.com/btfs/go-btfs-blockstore) | [![Travis CI](https://travis-ci.com/btfs/go-btfs-blockstore.svg?branch=master)](https://travis-ci.com/btfs/go-btfs-blockstore) | [![codecov](https://codecov.io/gh/btfs/go-btfs-blockstore/branch/master/graph/badge.svg)](https://codecov.io/gh/btfs/go-btfs-blockstore) | blockstore interfaces and implementations |
| **Commands** |
| [`go-btfs-cmds`](//github.com/btfs/go-btfs-cmds) | [![Travis CI](https://travis-ci.com/btfs/go-btfs-cmds.svg?branch=master)](https://travis-ci.com/btfs/go-btfs-cmds) | [![codecov](https://codecov.io/gh/btfs/go-btfs-cmds/branch/master/graph/badge.svg)](https://codecov.io/gh/btfs/go-btfs-cmds) | CLI & HTTP commands library |
| [`go-btfs-cmdkit`](//github.com/btfs/go-btfs-cmdkit) | [![Travis CI](https://travis-ci.com/btfs/go-btfs-cmdkit.svg?branch=master)](https://travis-ci.com/btfs/go-btfs-cmdkit) | [![codecov](https://codecov.io/gh/btfs/go-btfs-cmdkit/branch/master/graph/badge.svg)](https://codecov.io/gh/btfs/go-btfs-cmdkit) | helper types for the commands library |
| [`go-btfs-api`](//github.com/btfs/go-btfs-api) | [![Travis CI](https://travis-ci.com/btfs/go-btfs-api.svg?branch=master)](https://travis-ci.com/btfs/go-btfs-api) | [![codecov](https://codecov.io/gh/btfs/go-btfs-api/branch/master/graph/badge.svg)](https://codecov.io/gh/btfs/go-btfs-api) | a shell for the BTFS HTTP API |
| **Metrics & Logging** |
| [`go-metrics-interface`](//github.com/btfs/go-metrics-interface) | [![Travis CI](https://travis-ci.com/btfs/go-metrics-interface.svg?branch=master)](https://travis-ci.com/btfs/go-metrics-interface) | [![codecov](https://codecov.io/gh/btfs/go-metrics-interface/branch/master/graph/badge.svg)](https://codecov.io/gh/btfs/go-metrics-interface) | metrics collection interfaces |
| [`go-metrics-prometheus`](//github.com/btfs/go-metrics-prometheus) | [![Travis CI](https://travis-ci.com/btfs/go-metrics-prometheus.svg?branch=master)](https://travis-ci.com/btfs/go-metrics-prometheus) | [![codecov](https://codecov.io/gh/btfs/go-metrics-prometheus/branch/master/graph/badge.svg)](https://codecov.io/gh/btfs/go-metrics-prometheus) | prometheus-backed metrics collector |
| [`go-log`](//github.com/btfs/go-log) | [![Travis CI](https://travis-ci.com/btfs/go-log.svg?branch=master)](https://travis-ci.com/btfs/go-log) | [![codecov](https://codecov.io/gh/btfs/go-log/branch/master/graph/badge.svg)](https://codecov.io/gh/btfs/go-log) | logging framework |
| **Generics/Utils** |
| [`go-btfs-routing`](//github.com/btfs/go-btfs-routing) | [![Travis CI](https://travis-ci.com/btfs/go-btfs-routing.svg?branch=master)](https://travis-ci.com/btfs/go-btfs-routing) | [![codecov](https://codecov.io/gh/btfs/go-btfs-routing/branch/master/graph/badge.svg)](https://codecov.io/gh/btfs/go-btfs-routing) | routing (content, peer, value) helpers |
| [`go-btfs-util`](//github.com/btfs/go-btfs-util) | [![Travis CI](https://travis-ci.com/btfs/go-btfs-util.svg?branch=master)](https://travis-ci.com/btfs/go-btfs-util) | [![codecov](https://codecov.io/gh/btfs/go-btfs-util/branch/master/graph/badge.svg)](https://codecov.io/gh/btfs/go-btfs-util) | the kitchen sink |
| [`go-btfs-addr`](//github.com/btfs/go-btfs-addr) | [![Travis CI](https://travis-ci.com/btfs/go-btfs-addr.svg?branch=master)](https://travis-ci.com/btfs/go-btfs-addr) | [![codecov](https://codecov.io/gh/btfs/go-btfs-addr/branch/master/graph/badge.svg)](https://codecov.io/gh/btfs/go-btfs-addr) | utility functions for parsing BTFS multiaddrs |

For brevity, we've omitted go-libp2p and go-ipld packages. These package tables can be found in their respective project's READMEs:

* [go-libp2p](https://github.com/libp2p/go-libp2p#packages)
* [go-ipld](https://github.com/ipld/go-ipld#packages)

## Development

Some places to get you started on the codebase:

- Main file: [./cmd/btfs/main.go](https://github.com/btfs/go-btfs/blob/master/cmd/btfs/main.go)
- CLI Commands: [./core/commands/](https://github.com/btfs/go-btfs/tree/master/core/commands)
- Bitswap (the data trading engine): [go-bitswap](https://github.com/btfs/go-bitswap)
- libp2p
  - libp2p: https://github.com/libp2p/go-libp2p
  - DHT: https://github.com/libp2p/go-libp2p-kad-dht
  - PubSub: https://github.com/libp2p/go-libp2p-pubsub
- [BTFS : The `Add` command demystified](https://github.com/btfs/go-btfs/tree/master/docs/add-code-flow.md)

### CLI, HTTP-API, Architecture Diagram

![](./docs/cli-http-api-core-diagram.png)

> [Origin](https://github.com/btfs/pm/pull/678#discussion_r210410924)

Description: Dotted means "likely going away". The "Legacy" parts are thin wrappers around some commands to translate between the new system and the old system. The grayed-out parts on the "daemon" diagram are there to show that the code is all the same, it's just that we turn some pieces on and some pieces off depending on whether we're running on the client or the server.

### Testing

```
make test
```

### Development Dependencies

If you make changes to the protocol buffers, you will need to install the [protoc compiler](https://github.com/google/protobuf).

### Developer Notes

Find more documentation for developers on [docs](./docs)

## Contributing

[![](https://cdn.rawgit.com/jbenet/contribute-btfs-gif/master/img/contribute.gif)](https://github.com/btfs/community/blob/master/CONTRIBUTING.md)

We ❤️ all [our contributors](docs/AUTHORS); this project wouldn’t be what it is without you! If you want to help out, please see [CONTRIBUTING.md](CONTRIBUTING.md).

This repository falls under the BTFS [Code of Conduct](https://github.com/btfs/community/blob/master/code-of-conduct.md).

You can contact us on the freenode #btfs-dev channel or attend one of our
[weekly calls](https://github.com/btfs/team-mgmt/issues/674).

## License

[MIT](./LICENSE)
