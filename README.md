# go-btfs

## What is BTFS?

BitTorrent File System (BTFS) is a protocol forked from 
IPFS that utilizes the TRON network and the BitTorrent Ecosystem for integration with DApps and smart 
contracts. 

* The <a href="https://docs.btfs.io/" target="_blank">API documentation</a> walks developers through BTFS setup, usage, and API references. 
* Please join the BTFS community at https://discord.gg/PQWfzWS.   

## Table of Contents

- [Install](#install)
  - [System Requirements](#system-requirements)
- [Build from Source](#build-from-source)
  - [MacOS](#macos)
  - [Linux VM](#linux-vm)
  - [Docker](#docker)
- [Getting Started](#getting-started)
  - [Some things to try](#some-things-to-try)
  - [Usage](#usage)
- [Development](#development)
  - [Development Dependencies](#development-dependencies)
  - [BTFS Gateway](#btfs-gateway)
- [License](#license)


## Install

The download and install instructions for BTFS are over at: https://docs.btfs.io/docs/btfs-demo. 

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

## Build from Source

### MacOS

Clone the go-btfs repository
```
$ git clone https://github.com/TRON-US/go-btfs
```

Navigate to the go-btfs directory and run `make install`.
```
$ cd go-btfs
$ make install
```

A successful make install outputs something like:
```
$ make install
go: downloading github.com/tron-us/go-btfs-common v0.2.28
go: extracting github.com/tron-us/go-btfs-common v0.2.28
go: finding github.com/tron-us/go-btfs-common v0.2.28
go version go1.14.1 darwin/amd64
bin/check_go_version 1.14
go install  "-asmflags=all='-trimpath='" "-gcflags=all='-trimpath='" -ldflags="-X "github.com/TRON-US/go-btfs".CurrentCommit=e4848946d" ./cmd/btfs
```

Afterwards, run `btfs init` and `btfs daemon` to initialize and start the daemon. 

### Linux VM

Developers wishing to run a BTFS daemon on a Linux VM should first set up the environment. On an AWS EC2 Linux machine for example, it would be helpful to first install the following tools and dependencies:
```
$ sudo yum update          // Installs general updates for Linux
$ sudo yum install git     // Lets you git clone the go-btfs repository
$ sudo yum install patch   // Required for building from source
$ sudo yum install gcc     // Required for building from source
```

Building BTFS from source requires Go 1.14 or higher. To install from the terminal:
```
$ cd /tmp
$ GO_PACKAGE=go1.14.linux-amd64.tar.gz
$ wget https://golang.org/dl/$GO_PACKAGE
$ sudo tar -xvf $GO_PACKAGE
$ sudo mv go /usr/local
$ sudo rm $GO_PACKAGE
```

Navigate back to root directory and set the Go Path in the environment variables: 
```
$ export GOPATH=${HOME}/go
$ export PATH=$PATH:/usr/local/go/bin
$ export PATH=$PATH:$GOPATH/bin
$ export GO111MODULE=on
```

Verify the Go version is 1.14 or higher:
```
$ go version
```

Navigate to the go-btfs directory and run `make install`.
```
$ cd go-btfs
$ make install
```

Afterwards, run `btfs init` and `btfs daemon` to initialize and start the daemon. To re-initialize a new pair of keys, you can shut down the daemon first via `btfs shutdown`. Then run `rm -r .btfs` and `btfs init` again. 

### Docker

Developers also have the option to build a BTFS daemon within a Docker container. After cloning the go-btfs repository, navigate into the go-btfs directory. This is where the Dockerfile is located. Build the docker image:
```
$ cd go-btfs
$ docker image build -t btfs_docker .   // Builds the docker image and tags "btfs_docker" as the name 
```

A successful build should have an output like:
```
Sending build context to Docker daemon  2.789MB
Step 1/37 : FROM golang:1.14-stretch
 ---> 4fe257ac564c
Step 2/37 : MAINTAINER TRON-US <support@tron.network>
 ---> Using cache
 ---> 02409001f528

...

Step 37/37 : CMD ["daemon", "--migrate=true"]
 ---> Running in 3660f91dce94
Removing intermediate container 3660f91dce94
 ---> b4e1523cf264
Successfully built b4e1523cf264
Successfully tagged btfs_docker:latest
```

Start the container based on the new image. Starting the container also initializes and starts the BTFS daemon.
```
$ docker container run --publish 8080:5001 --detach --name btfs1 btfs_docker
```

The CLI flags are as such:

* `--publish` asks Docker to forward traffic incoming on the host’s port 8080, to the container’s port 5001. 
* `--detach` asks Docker to run this container in the background.
* `--name` specifies a name with which you can refer to your container in subsequent commands, in this case btfs1.

Configure cross-origin(CORS)
You need to configure cross-origin (CORS) to access the container from the host.
```
(host) docker exec -it btfs1 /bin/sh // Enter the container's shell
```

Then configure cross-origin(CORS) with btfs

```
(container) btfs config --json API.HTTPHeaders.Access-Control-Allow-Origin '["http://$IP:$PORT"]'
(container) btfs config --json API.HTTPHeaders.Access-Control-Allow-Methods '["PUT", "GET", "POST"]'
```

E.g:
```
(container) btfs config --json API.HTTPHeaders.Access-Control-Allow-Origin '["http://localhost:8080"]'
(container) btfs config --json API.HTTPHeaders.Access-Control-Allow-Methods '["PUT", "GET", "POST"]'
```

Exit the container and restart the container
```
(container) exit
(host) docker restart btfs1
```

You can access the container from the host with http://localhost:8080/hostui .

Execute commands within the docker container:
```
docker exec CONTAINER btfs add --chunker=reed-solomon FILE
```

## Getting Started

### Some things to try

Basic proof of 'btfs working' locally:

    echo "hello world" > hello
    btfs add --chunker=reed-solomon hello
    # This should output a hash string that looks something like:
    # QmaN4MmXMduZe7Y7XoMKFPuDFunvEZU6DWtBPg3L8kkAuS
    btfs cat <that hash>

### Usage

```
  btfs  - Global p2p merkle-dag filesystem.

  btfs [--config=<config> | -c] [--debug | -D] [--help] [-h] [--api=<api>] [--offline] [--cid-base=<base>] [--upgrade-cidv0-in-output] [--encoding=<encoding> | --enc] [--timeout=<timeout>] <command> ...

SUBCOMMANDS
  BASIC COMMANDS
    init          Initialize btfs local configuration
    add <path>    Add a file to BTFS
    cat <ref>     Show BTFS object data
    get <ref>     Download BTFS objects
    ls <ref>      List links from an object
    refs <ref>    List hashes of links from an object

  BTFS COMMANDS
    storage       Manage client and host storage features
    rm            Clean up locally stored files and objects

  DATA STRUCTURE COMMANDS
    block         Interact with raw blocks in the datastore
    object        Interact with raw dag nodes
    files         Interact with objects as if they were a unix filesystem
    dag           Interact with IPLD documents (experimental)
    metadata      Interact with metadata for BTFS files

  ADVANCED COMMANDS
    daemon        Start a long-running daemon process
    mount         Mount an BTFS read-only mount point
    resolve       Resolve any type of name
    name          Publish and resolve BTNS names
    key           Create and list BTNS name keypairs
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

### BTFS Gateway

BTFS Gateway is a free service that allows you to retrieve files from the BTFS network in your browser directly.

[How to use BTFS Gateway](https://docs.btfs.io/docs/btfs-gateway-user-guide)

## License

[MIT](./LICENSE)
