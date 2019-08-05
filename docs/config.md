# The go-ipfs config file

The go-ipfs config file is a JSON document located at `$IPFS_PATH/config`. It
is read once at node instantiation, either for an offline command, or when
starting the daemon. Commands that execute on a running daemon do not read the
config file at runtime.

#### Profiles

Configuration profiles allow to tweak configuration quickly. Profiles can be
applied with `--profile` flag to `ipfs init` or with the `ipfs config profile
apply` command. When a profile is applied a backup of the configuration file
will be created in `$IPFS_PATH`.

Available profiles:

- `server`

  Recommended for nodes with public IPv4 address (servers, VPSes, etc.),
  disables host and content discovery in local networks.

- `local-discovery`

  Sets default values to fields affected by `server` profile, enables
  discovery in local networks.

- `test`

  Reduces external interference, useful for running ipfs in test environments.
  Note that with these settings node won't be able to talk to the rest of the
  network without manual bootstrap.

- `default-networking`

  Restores default network settings. Inverse profile of the `test` profile.

- `announce-public`

  Announce public IP when running on cloud VM or local network.

- `badgerds`

  Replaces default datastore configuration with experimental badger datastore.
  If you apply this profile after `ipfs init`, you will need to convert your
  datastore to the new configuration. You can do this using [ipfs-ds-convert](https://github.com/ipfs/ipfs-ds-convert)

  WARNING: badger datastore is experimental. Make sure to backup your data
  frequently.

- `default-datastore`

  Restores default datastore configuration.

- `lowpower`

  Reduces daemon overhead on the system. May affect node functionality,
  performance of content discovery and data fetching may be degraded.

- `randomports`

  Generate random port for swarm.

## Table of Contents

- [`Addresses`](#addresses)
- [`API`](#api)
- [`Bootstrap`](#bootstrap)
- [`Datastore`](#datastore)
- [`Discovery`](#discovery)
- [`Routing`](#routing)
- [`Gateway`](#gateway)
- [`Identity`](#identity)
- [`Ipns`](#ipns)
- [`Mounts`](#mounts)
- [`Reprovider`](#reprovider)
- [`Swarm`](#swarm)

## `Addresses`
Contains information about various listener addresses to be used by this node.

- `API`
Multiaddr describing the address to serve the local HTTP API on.

Default: `/ip4/127.0.0.1/tcp/5001`

- `Gateway`
Multiaddr describing the address to serve the local gateway on.

Default: `/ip4/127.0.0.1/tcp/8080`

- `Swarm`
Array of multiaddrs describing which addresses to listen on for p2p swarm connections.

Default:
```json
[
  "/ip4/0.0.0.0/tcp/4001",
  "/ip6/::/tcp/4001"
]
```

- `Announce`
If non-empty, this array specifies the swarm addresses to announce to the network. If empty, the daemon will announce inferred swarm addresses.

Default: `[]`

- `NoAnnounce`
Array of swarm addresses not to announce to the network.

Default: `[]`

## `API`
Contains information used by the API gateway.

- `HTTPHeaders`
Map of HTTP headers to set on responses from the API HTTP server.

Example:
```json
{
	"Foo": ["bar"]
}
```

Default: `null`

## `Bootstrap`
Bootstrap is an array of multiaddrs of trusted nodes to connect to in order to
initiate a connection to the network.

Default: The ipfs.io bootstrap nodes

## `Datastore`
Contains information related to the construction and operation of the on-disk
storage system.

- `StorageMax`
A soft upper limit for the size of the ipfs repository's datastore. With `StorageGCWatermark`,
is used to calculate whether to trigger a gc run (only if `--enable-gc` flag is set).

Default: `10GB`

- `StorageGCWatermark`
The percentage of the `StorageMax` value at which a garbage collection will be
triggered automatically if the daemon was run with automatic gc enabled (that
option defaults to false currently).

Default: `90`

- `GCPeriod`
A time duration specifying how frequently to run a garbage collection. Only used
if automatic gc is enabled.

Default: `1h`

- `HashOnRead`
A boolean value. If set to true, all block reads from disk will be hashed and
verified. This will cause increased CPU utilization.

- `BloomFilterSize`
A number representing the size in bytes of the blockstore's [bloom filter](https://en.wikipedia.org/wiki/Bloom_filter). A value of zero represents the feature being disabled.

This site generates useful graphs for various bloom filter values: <https://hur.st/bloomfilter/?n=1e6&p=0.01&m=&k=7>
You may use it to find a preferred optimal value, where `m` is `BloomFilterSize` in bits. Remember to convert the value `m` from bits, into bytes for use as `BloomFilterSize` in the config file.
For example, for 1,000,000 blocks, expecting a 1% false positive rate, you'd end up with a filter size of 9592955 bits, so for `BloomFilterSize` we'd want to use 1199120 bytes.
As of writing, [7 hash functions](https://github.com/ipfs/go-ipfs-blockstore/blob/547442836ade055cc114b562a3cc193d4e57c884/caching.go#L22) are used, so the constant `k` is 7 in the formula.


Default: `0`

- `Spec`
Spec defines the structure of the ipfs datastore. It is a composable structure, where each datastore is represented by a json object. Datastores can wrap other datastores to provide extra functionality (eg metrics, logging, or caching).

This can be changed manually, however, if you make any changes that require a different on-disk structure, you will need to run the [ipfs-ds-convert tool](https://github.com/ipfs/ipfs-ds-convert) to migrate data into the new structures.

For more information on possible values for this configuration option, see docs/datastores.md

Default:
```
{
  "mounts": [
	{
	  "child": {
		"path": "blocks",
		"shardFunc": "/repo/flatfs/shard/v1/next-to-last/2",
		"sync": true,
		"type": "flatfs"
	  },
	  "mountpoint": "/blocks",
	  "prefix": "flatfs.datastore",
	  "type": "measure"
	},
	{
	  "child": {
		"compression": "none",
		"path": "datastore",
		"type": "levelds"
	  },
	  "mountpoint": "/",
	  "prefix": "leveldb.datastore",
	  "type": "measure"
	}
  ],
  "type": "mount"
}
```

## `Discovery`
Contains options for configuring ipfs node discovery mechanisms.

- `MDNS`
Options for multicast dns peer discovery.

  - `Enabled`
A boolean value for whether or not mdns should be active.

Default: `true`

  -  `Interval`
A number of seconds to wait between discovery checks.


## `Routing`
Contains options for content routing mechanisms.

- `Type`
Content routing mode. Can be overridden with daemon `--routing` flag. When set to `dhtclient`, the node won't join the DHT but can still use it to find content.
Valid modes are:
  - `dht` (default)
  - `dhtclient`
  - `none`
  
**Example:**

```json
{
  "Routing": {
    "Type": "dhtclient"
  }
}
```  
  

## `Gateway`
Options for the HTTP gateway.

- `NoFetch`
When set to true, the gateway will only serve content already in the local repo
and will not fetch files from the network.

Default: `false`

- `HTTPHeaders`
Headers to set on gateway responses.

Default:
```json
{
	"Access-Control-Allow-Headers": [
		"X-Requested-With"
	],
	"Access-Control-Allow-Methods": [
		"GET"
	],
	"Access-Control-Allow-Origin": [
		"*"
	]
}
```

- `RootRedirect`
A url to redirect requests for `/` to.

Default: `""`

- `Writable`
A boolean to configure whether the gateway is writeable or not.

Default: `false`

- `PathPrefixes`
TODO

Default: `[]`

## `Identity`

- `PeerID`
The unique PKI identity label for this configs peer. Set on init and never read,
its merely here for convenience. Ipfs will always generate the peerID from its
keypair at runtime.

- `PrivKey`
The base64 encoded protobuf describing (and containing) the nodes private key.

## `Ipns`

- `RepublishPeriod`
A time duration specifying how frequently to republish ipns records to ensure
they stay fresh on the network. If unset, we default to 4 hours.

- `RecordLifetime`
A time duration specifying the value to set on ipns records for their validity
lifetime.
If unset, we default to 24 hours.

- `ResolveCacheSize`
The number of entries to store in an LRU cache of resolved ipns entries. Entries
will be kept cached until their lifetime is expired.

Default: `128`

## `Mounts`
FUSE mount point configuration options.

- `IPFS`
Mountpoint for `/ipfs/`.

- `IPNS`
Mountpoint for `/ipns/`.

- `FuseAllowOther`
Sets the FUSE allow other option on the mountpoint.

## `Reprovider`

- `Interval`
Sets the time between rounds of reproviding local content to the routing
system. If unset, it defaults to 12 hours. If set to the value `"0"` it will
disable content reproviding.

Note: disabling content reproviding will result in other nodes on the network
not being able to discover that you have the objects that you have. If you want
to have this disabled and keep the network aware of what you have, you must
manually announce your content periodically.

- `Strategy`
Tells reprovider what should be announced. Valid strategies are:
  - "all" (default) - announce all stored data
  - "pinned" - only announce pinned data
  - "roots" - only announce directly pinned keys and root keys of recursive pins

## `Swarm`

Options for configuring the swarm.

- `AddrFilters`
An array of address filters (multiaddr netmasks) to filter dials to.
See [this issue](https://github.com/ipfs/go-ipfs/issues/1226#issuecomment-120494604) for more
information.

- `DisableBandwidthMetrics`
A boolean value that when set to true, will cause ipfs to not keep track of
bandwidth metrics. Disabling bandwidth metrics can lead to a slight performance
improvement, as well as a reduction in memory usage.

- `DisableNatPortMap`
Disable NAT discovery.

- `DisableRelay`
Disables the p2p-circuit relay transport.

- `EnableRelayHop`
Enables HOP relay for the node. If this is enabled, the node will act as
an intermediate (Hop Relay) node in relay circuits for connected peers.

- `EnableAutoRelay`
Enables automatic relay for this node.
If the node is a HOP relay (`EnableRelayHop` is true) then it will advertise itself as a relay through the DHT.
Otherwise, the node will test its own NAT situation (dialability) using passively discovered AutoNAT services.
If the node is not publicly reachable, then it will seek HOP relays advertised through the DHT and override its public address(es) with relay addresses.

- `EnableAutoNATService`
Enables the AutoNAT service for this node.
The service allows peers to discover their NAT situation by requesting dial backs to their public addresses.
This should only be enabled on publicly reachable nodes.

### `ConnMgr`

The connection manager determines which and how many connections to keep and can be configured to keep.

- `Type`
Sets the type of connection manager to use, options are: `"none"` (no connection management) and `"basic"`.

#### Basic Connection Manager

- `LowWater`
LowWater is the minimum number of connections to maintain.

- `HighWater`
HighWater is the number of connections that, when exceeded, will trigger a connection GC operation.

- `GracePeriod`
GracePeriod is a time duration that new connections are immune from being closed by the connection manager.

The "basic" connection manager tries to keep between `LowWater` and `HighWater` connections. It works by:

1. Keeping all connections until `HighWater` connections is reached.
2. Once `HighWater` is reached, it closes connections until `LowWater` is reached.
3. To prevent thrashing, it never closes connections established within the `GracePeriod`.

**Example:**


```json
{
  "Swarm": {
    "ConnMgr": {
      "Type": "basic",
      "LowWater": 100,
      "HighWater": 200,
      "GracePeriod": "30s"
    }
  }
}
```
