# go-btfs



## What is BTFS?

BitTorrent File Sharing (BTFS) is a file-sharing protocol forked from 
IPFS that utilizes the TRON network and the BitTorrent Ecosystem for integration with DApps and smart 
contracts. The <a href="https://docs.btfs.io/" target="_blank">API documentation</a> walks developers through BTFS setup, usage, and contains all the API references.  

## Table of Contents

- [Install](#install)
  - [System Requirements](#system-requirements)
  - [Build from Source](#build-from-source)
    - [Install Go](#step1-install-go)
    - [Access To Private Repo](#step2-access-to-private-repo)
    - [Download Source Code](#step3-download-source-code)
    - [Build And Join BTFS Private Net](#step4-build-and-join-btfs-private-net)
        - [Install script](#using-one-step-script)
        - [Instruction](#step-by-step-instruction)
    - [Run BTFS At The Backend](#step5-run-btfs-at-the-backend)
- [Usage](#usage)
- [Started by binary file](#started-by-binary-file)
  - [Automatically generate](#automatically-generate)
- [Getting Started](#getting-started)
  - [Some things to try](#some-things-to-try)
    - [Pay attention](#pay-attention)
    - [Step](#step)
- [Packages](#packages)
- [License](#license)


## Install

### System Requirements

BTFS can run on most Linux, macOS, and Windows systems. We recommend 
running it on a machine with at least 2 GB of RAM (it’ll do fine with 
only one CPU core), but it should run fine with as little as 1 GB of 
RAM. On systems with less memory, it may not be completely stable.
Only support compiling from source for mac and unix-based system.


### Build from Source

#### Step0 Install GCC
GCC is required to build btfs from source code. Approaches to install GCC may vary from operating system to operating system.
To install GCC on Debian and Ubuntu system, please run the following commands:
```bash
sudo apt-get update
sudo apt-get install build-essential manpages-dev
```
To verify that GCC has been successfully installed on your machine, please run this command:
```bash
gcc --version
```

#### Step1 Install Go

The build process for btfs requires Go 1.12 or higher. If you don't 
have it: [Download Go 1.12+](https://golang.org/dl/). Or use the 
following command:
```
cd /tmp
GO_PACKAGE=go1.12.4.linux-amd64.tar.gz
wget https://golang.org/dl/$GO_PACKAGE
sudo tar -xvf $GO_PACKAGE
sudo mv go /usr/local
sudo rm $GO_PACKAGE
go version
```

You'll need to add Go's bin directories to your `$PATH` environment 
variable e.g., by adding these lines to your `/etc/profile` (for a 
system-wide installation) or `$HOME/.profile`:

```
export GOPATH=${HOME}/go
export PATH=$PATH:/usr/local/go/bin
export PATH=$PATH:$GOPATH/bin
export GO111MODULE=on
```

(If you run into trouble, see the [Go install instructions](https://golang.org/doc/install)).


#### Step2 Download Source Code

```
$ git clone https://github.com/TRON-US/go-btfs.git
$ cd go-btfs
```


#### Step3 Build And Join BTFS Private Net

Your can choose to using on-step script to compile and build or just follow step by step.

##### Using One-step Script

`bash install_btfs.sh`


##### Step-by-step Instruction

###### 1. Compile and build:

```
$ go clean -modcache
$ make install
go version go1.12.4 linux/amd64
bin/check_go_version 1.12
go install -ldflags="-X "github.com/btfs/go-btfs".CurrentCommit=63ae486fa-dirty" ./cmd/btfs
```
If you are building on FreeBSD instead of `make install` use `gmake install`.

Show if btfs exec file has been created:
```
$which btfs
/home/ubuntu/go/bin/btfs
```

###### 2. Init a btfs node:

```
$ btfs init -k ECDSA(type of key generation, e.g. RSA, ECDSA ...)
initializing BTFS node at /home/ubuntu/.btfs
generating 2048-bit ECDSA keypair...done
peer identity: QmTkQjKAtfh2GNDPmtnFB8d..................
to get started, enter:

        btfs cat /btfs/QmS4ustL54uo8FzR9455qaxZwuMi........H4uVv/readme
```

###### 3. Auto Update Setting

Create a config.yaml file in the same path of your btfs binary path. The config.yaml file has the following context:
```
version: 0.0.4    # btfs version, order by version.go
md5: 034cf64b76f8bf5f506ce6aca9fa81c4    #btfs binary md5
autoupdateFlg: true     # is auto update
sleepTimeSeconds: 20        # how often to auto update (second）.
```

###### 4. Configuration

Enable Cross-Origin Resource Sharing:
```
$ btfs config --json API.HTTPHeaders.Access-Control-Allow-Origin "[\"*\"]"
$ btfs config --json API.HTTPHeaders.Access-Control-Allow-Methods '["PUT", "GET", "POST", "OPTIONS"]'
$ btfs config --json API.HTTPHeaders.Access-Control-Allow-Credentials "[\"true\"]"
```
Enable gateway, api port:
```
$ btfs config Addresses.API /ip4/0.0.0.0/tcp/5001
$ btfs config Addresses.Gateway /ip4/0.0.0.0/tcp/8080
```


#### Step4 Run BTFS At The Backend

```
you need to make sure there is no btfs node already running, using `ps -ef |grep "btfs daemon"` to check if there is btfs node running and then kill the node process if it is, then running the following command:
$ sudo nohup btfs daemon </dev/null >/dev/null 2>&1 &
```
Check if your node is connect to BTFS private net:
```
$ btfs swarm peers
/ip4/3.18.120.107/tcp/4001/btfs/QmcmRdAHQYTtpbs9Ud5rNx6WzHmU9WcYCrBneCSyKhMr7H
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

## Started by binary file

Please refer to the documentation for the btfs-binary-releases project.

[btfs-binary-releases](https://github.com/TRON-US/btfs-binary-releases/blob/master/README.md)

### Automatically generate

#### Pay attention

* This section is limited to maintenance personnel of btfs-binary-release.
* Make sure go-btfs and btfs-binary-release are in the same directory.

#### Step

1. Download go-btfs and btfs-binary-release repo.

```shell
git clone https://github.com/TRON-US/go-btfs.git
git clone https://github.com/TRON-US/btfs-binary-releases.git
```

2. Create a branch to upload in btfs-binary-release.

```shell
cd btfs-binary-releases
git checkout -b uploader
```

3. Execute automatic packaging script.

```shell
cd ../go-btfs
bash package.sh
```

Now you will generate all supported versions of the binary package. Push it to the remote to generate the pull request, we will review your pr and merge it into the master.

## Getting Started

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


## Development

Some places to get you started on the codebase:

- Main file: [./cmd/btfs/main.go](https://github.com/TRON-US/go-btfs/blob/master/cmd/btfs/main.go)
- CLI Commands: [./core/commands/](https://github.com/TRON-US/go-btfs/tree/master/core/commands)
- libp2p
  - libp2p: [libp2p](https://github.com/libp2p/go-libp2p)
  - DHT: [DHT](https://github.com/libp2p/go-libp2p-kad-dht)
  - PubSub: [PubSub](https://github.com/libp2p/go-libp2p-pubsub)


### Testing

```
make test
```

### Development Dependencies

If you make changes to the protocol buffers, you will need to install the [protoc compiler](https://github.com/google/protobuf).


### License

[MIT](./LICENSE)


## TODO
#### Troubleshooting
