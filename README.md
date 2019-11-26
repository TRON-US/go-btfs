# go-btfs

## What is BTFS?

BitTorrent File System (BTFS) is a protocol forked from 
IPFS that utilizes the TRON network and the BitTorrent Ecosystem for integration with DApps and smart 
contracts. The <a href="https://docs.btfs.io/" target="_blank">API documentation</a> walks developers through BTFS setup, usage, and contains all the API references.  

## Table of Contents

- [Install](#install)
  - [System Requirements](#system-requirements)
- [Usage](#usage)
- [Getting Started](#getting-started)
  - [Some things to try](#some-things-to-try)
    - [Pay attention](#pay-attention)
    - [Step](#step)
- [Packages](#packages)
- [Soter](#soter)
- [License](#license)


## Install

The download and install instructions for BTFS are over at: https://docs.btfs.io/docs/install-btfs-1. 

### System Requirements

BTFS can run on most Linux, macOS, and Windows systems. We recommend 
running it on a machine with at least 2 GB of RAM (it’ll do fine with 
only one CPU core), but it should run fine with as little as 1 GB of 
RAM. On systems with less memory, it may not be completely stable.
Only support compiling from source for mac and unix-based system.

### Install Pre-Built Packages

We host pre-built binaries at https://github.com/TRON-US/btfs-binary-releases. 

#### Initialize a BTFS Daemon

```
$ btfs init
initializing BTFS node at /home/ubuntu/.btfs
generating 2048-bit keypair...done
peer identity: 16Uiu2HAmHmW9mHcE9c5UfUomy8caBuEgJVP99MDb4wFtzF8URgzE
to get started, enter:

        btfs cat /btfs/QmPbWqakofrBdDSm4mLUS5RE5QiPQi8JbnK73LgWwQNdbi/readme
```

#### Start the Daemon

Start the BTFS Daemon
```
$ btfs daemon
```

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

### Some things to try

Basic proof of 'btfs working' locally:

	echo "hello world" > hello
	btfs add hello
	# This should output a hash string that looks something like:
	# QmT78zSuBmuS4z925WZfrqQ1qHaJ56DQaTfyMUF7F8ff5o
	btfs cat <that hash>


## Development

Some places to get you started on the codebase:

- Main file: [./cmd/btfs/main.go](https://github.com/TRON-US/go-btfs/blob/master/cmd/btfs/main.go)
- CLI Commands: [./core/commands/](https://github.com/TRON-US/go-btfs/tree/master/core/commands)
- libp2p
  - libp2p: [libp2p](https://github.com/libp2p/go-libp2p)
  - DHT: [DHT](https://github.com/libp2p/go-libp2p-kad-dht)
  - PubSub: [PubSub](https://github.com/libp2p/go-libp2p-pubsub)


### Development Dependencies

If you make changes to the protocol buffers, you will need to install the [protoc compiler](https://github.com/google/protobuf).

### Soter

BTFS Soter is a charging service gateway based on the TRON Network and BTFS cluster. Users can access the BTFS network via the Soter gateway.

[Soter Interface Document](https://btfssoter.readme.io/docs/soter-interface-documentation)

### License

[MIT](./LICENSE)
