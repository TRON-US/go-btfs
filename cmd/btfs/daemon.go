package main

import (
	"errors"
	_ "expvar"
	"fmt"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"

	version "github.com/TRON-US/go-btfs"
	utilmain "github.com/TRON-US/go-btfs/cmd/btfs/util"
	oldcmds "github.com/TRON-US/go-btfs/commands"
	"github.com/TRON-US/go-btfs/core"
	"github.com/TRON-US/go-btfs/core/commands"
	"github.com/TRON-US/go-btfs/core/coreapi"
	"github.com/TRON-US/go-btfs/core/corehttp"
	httpremote "github.com/TRON-US/go-btfs/core/corehttp/remote"
	"github.com/TRON-US/go-btfs/core/corerepo"
	"github.com/TRON-US/go-btfs/core/node/libp2p"
	nodeMount "github.com/TRON-US/go-btfs/fuse/node"
	"github.com/TRON-US/go-btfs/repo/fsrepo"
	migrate "github.com/TRON-US/go-btfs/repo/fsrepo/migrations"
	"github.com/TRON-US/go-btfs/spin"

	cmds "github.com/TRON-US/go-btfs-cmds"
	config "github.com/TRON-US/go-btfs-config"
	nodepb "github.com/tron-us/go-btfs-common/protos/node"

	"github.com/hashicorp/go-multierror"
	util "github.com/ipfs/go-ipfs-util"
	mprome "github.com/ipfs/go-metrics-prometheus"
	"github.com/jbenet/goprocess"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr-net"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	adjustFDLimitKwd          = "manage-fdlimit"
	enableGCKwd               = "enable-gc"
	initOptionKwd             = "init"
	initProfileOptionKwd      = "init-profile"
	ipfsMountKwd              = "mount-btfs"
	ipnsMountKwd              = "mount-btns"
	migrateKwd                = "migrate"
	mountKwd                  = "mount"
	offlineKwd                = "offline" // global option
	routingOptionKwd          = "routing"
	routingOptionSupernodeKwd = "supernode"
	routingOptionDHTClientKwd = "dhtclient"
	routingOptionDHTKwd       = "dht"
	routingOptionNoneKwd      = "none"
	routingOptionDefaultKwd   = "default"
	unencryptTransportKwd     = "disable-transport-encryption"
	unrestrictedApiAccessKwd  = "unrestricted-api"
	writableKwd               = "writable"
	enablePubSubKwd           = "enable-pubsub-experiment"
	enableIPNSPubSubKwd       = "enable-namesys-pubsub"
	enableMultiplexKwd        = "enable-mplex-experiment"
	hValueKwd                 = "hval"
	enableDataCollection      = "dc"
	enableStartupTest         = "enable-startup-test"
	swarmPortKwd              = "swarm-port"
	// apiAddrKwd    = "address-api"
	// swarmAddrKwd  = "address-swarm"
)

// BTFS daemon test exit error code
const (
	findBTFSBinaryFailed = 100
	getFileTestFailed    = 101
	addFileTestFailed    = 102
)

var daemonCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Run a network-connected BTFS node.",
		ShortDescription: `
'btfs daemon' runs a persistent btfs daemon that can serve commands
over the network. Most applications that use BTFS will do so by
communicating with a daemon over the HTTP API. While the daemon is
running, calls to 'btfs' commands will be sent over the network to
the daemon.
`,
		LongDescription: `
The daemon will start listening on ports on the network, which are
documented in (and can be modified through) 'btfs config Addresses'.
For example, to change the 'Gateway' port:

  btfs config Addresses.Gateway /ip4/127.0.0.1/tcp/8082

The API address can be changed the same way:

  btfs config Addresses.API /ip4/127.0.0.1/tcp/5002

Make sure to restart the daemon after changing addresses.

By default, the gateway is only accessible locally. To expose it to
other computers in the network, use 0.0.0.0 as the ip address:

  btfs config Addresses.Gateway /ip4/0.0.0.0/tcp/8080

Be careful if you expose the API. It is a security risk, as anyone could
control your node remotely. If you need to control the node remotely,
make sure to protect the port as you would other services or database
(firewall, authenticated proxy, etc).

HTTP Headers

btfs supports passing arbitrary headers to the API and Gateway. You can
do this by setting headers on the API.HTTPHeaders and Gateway.HTTPHeaders
keys:

  btfs config --json API.HTTPHeaders.X-Special-Header '["so special :)"]'
  btfs config --json Gateway.HTTPHeaders.X-Special-Header '["so special :)"]'

Note that the value of the keys is an _array_ of strings. This is because
headers can have more than one value, and it is convenient to pass through
to other libraries.

CORS Headers (for API)

You can setup CORS headers the same way:

  btfs config --json API.HTTPHeaders.Access-Control-Allow-Origin '["example.com"]'
  btfs config --json API.HTTPHeaders.Access-Control-Allow-Methods '["PUT", "GET", "POST"]'
  btfs config --json API.HTTPHeaders.Access-Control-Allow-Credentials '["true"]'

Shutdown

To shutdown the daemon, send a SIGINT signal to it (e.g. by pressing 'Ctrl-C')
or send a SIGTERM signal to it (e.g. with 'kill'). It may take a while for the
daemon to shutdown gracefully, but it can be killed forcibly by sending a
second signal.

BTFS_PATH environment variable

btfs uses a repository in the local file system. By default, the repo is
located at ~/.btfs. To change the repo location, set the $BTFS_PATH
environment variable:

  export BTFS_PATH=/path/to/btfsrepo

Routing

BTFS by default will use a DHT for content routing. There is a highly
experimental alternative that operates the DHT in a 'client only' mode that
can be enabled by running the daemon as:

  btfs daemon --routing=dhtclient

This will later be transitioned into a config option once it gets out of the
'experimental' stage.

DEPRECATION NOTICE

Previously, btfs used an environment variable as seen below:

  export API_ORIGIN="http://localhost:8888/"

This is deprecated. It is still honored in this version, but will be removed
in a future version, along with this notice. Please move to setting the HTTP
Headers.
`,
	},

	Options: []cmds.Option{
		cmds.BoolOption(initOptionKwd, "Initialize btfs with default settings if not already initialized"),
		cmds.StringOption(initProfileOptionKwd, "Configuration profiles to apply for --init. See btfs init --help for more"),
		cmds.StringOption(routingOptionKwd, "Overrides the routing option").WithDefault(routingOptionDefaultKwd),
		cmds.BoolOption(mountKwd, "Mounts BTFS to the filesystem"),
		cmds.BoolOption(writableKwd, "Enable writing objects (with POST, PUT and DELETE)"),
		cmds.StringOption(ipfsMountKwd, "Path to the mountpoint for BTFS (if using --mount). Defaults to config setting."),
		cmds.StringOption(ipnsMountKwd, "Path to the mountpoint for BTNS (if using --mount). Defaults to config setting."),
		cmds.BoolOption(unrestrictedApiAccessKwd, "Allow API access to unlisted hashes"),
		cmds.BoolOption(unencryptTransportKwd, "Disable transport encryption (for debugging protocols)"),
		cmds.BoolOption(enableGCKwd, "Enable automatic periodic repo garbage collection"),
		cmds.BoolOption(adjustFDLimitKwd, "Check and raise file descriptor limits if needed").WithDefault(true),
		cmds.BoolOption(migrateKwd, "If true, assume yes at the migrate prompt. If false, assume no."),
		cmds.BoolOption(enablePubSubKwd, "Instantiate the btfs daemon with the experimental pubsub feature enabled."),
		cmds.BoolOption(enableIPNSPubSubKwd, "Enable BTNS record distribution through pubsub; enables pubsub."),
		cmds.BoolOption(enableMultiplexKwd, "Add the experimental 'go-multiplex' stream muxer to libp2p on construction.").WithDefault(true),
		cmds.StringOption(hValueKwd, "H-value identifies the BitTorrent client this daemon is started by. None if not started by a BitTorrent client."),
		cmds.BoolOption(enableDataCollection, "Allow BTFS to collect and send out node statistics."),
		cmds.BoolOption(enableStartupTest, "Allow BTFS to perform start up test.").WithDefault(false),
		cmds.StringOption(swarmPortKwd, "Override existing announced swarm address with external port in the format of [WAN:LAN]."),

		// TODO: add way to override addresses. tricky part: updating the config if also --init.
		// cmds.StringOption(apiAddrKwd, "Address for the daemon rpc API (overrides config)"),
		// cmds.StringOption(swarmAddrKwd, "Address for the swarm socket (overrides config)"),
	},
	Subcommands: map[string]*cmds.Command{},
	Run:         daemonFunc,
}

// defaultMux tells mux to serve path using the default muxer. This is
// mostly useful to hook up things that register in the default muxer,
// and don't provide a convenient http.Handler entry point, such as
// expvar and http/pprof.
func defaultMux(path string) corehttp.ServeOption {
	return func(node *core.IpfsNode, _ net.Listener, mux *http.ServeMux) (*http.ServeMux, error) {
		mux.Handle(path, http.DefaultServeMux)
		return mux, nil
	}
}

func daemonFunc(req *cmds.Request, re cmds.ResponseEmitter, env cmds.Environment) (_err error) {
	// Inject metrics before we do anything
	err := mprome.Inject()
	if err != nil {
		log.Errorf("Injecting prometheus handler for metrics failed with message: %s\n", err.Error())
	}

	// let the user know we're going.
	fmt.Printf("Initializing daemon...\n")

	defer func() {
		if _err != nil {
			// Print an extra line before any errors. This could go
			// in the commands lib but doesn't really make sense for
			// all commands.
			fmt.Println()
		}
	}()

	// print the btfs version
	printVersion()

	managefd, _ := req.Options[adjustFDLimitKwd].(bool)
	if managefd {
		if _, _, err := utilmain.ManageFdLimit(); err != nil {
			log.Errorf("setting file descriptor limit: %s", err)
		}
	}

	cctx := env.(*oldcmds.Context)

	// check transport encryption flag.
	unencrypted, _ := req.Options[unencryptTransportKwd].(bool)
	if unencrypted {
		log.Warningf(`Running with --%s: All connections are UNENCRYPTED.
		You will not be able to connect to regular encrypted networks.`, unencryptTransportKwd)
	}

	// first, whether user has provided the initialization flag. we may be
	// running in an uninitialized state.
	initialize, _ := req.Options[initOptionKwd].(bool)
	inited := false
	if initialize {
		cfg := cctx.ConfigRoot
		if !fsrepo.IsInitialized(cfg) {
			profiles, _ := req.Options[initProfileOptionKwd].(string)

			err := initWithDefaults(os.Stdout, cfg, profiles)
			if err != nil {
				return err
			}
			inited = true
		}
	}

	// acquire the repo lock _before_ constructing a node. we need to make
	// sure we are permitted to access the resources (datastore, etc.)
	repo, err := fsrepo.Open(cctx.ConfigRoot)
	switch err {
	default:
		return err
	case fsrepo.ErrNeedMigration:
		domigrate, found := req.Options[migrateKwd].(bool)
		fmt.Println("Found outdated fs-repo, migrations need to be run.")

		if !found {
			domigrate = YesNoPrompt("Run migrations now? [y/N]")
		}

		if !domigrate {
			fmt.Println("Not running migrations of fs-repo now.")
			fmt.Println("Please get fs-repo-migrations from https://dist.ipfs.io")
			return fmt.Errorf("fs-repo requires migration")
		}

		err = migrate.RunMigration(fsrepo.RepoVersion)
		if err != nil {
			fmt.Println("The migrations of fs-repo failed:")
			fmt.Printf("  %s\n", err)
			fmt.Println("If you think this is a bug, please file an issue and include this whole log output.")
			fmt.Println("  https://github.com/ipfs/fs-repo-migrations")
			return err
		}

		repo, err = fsrepo.Open(cctx.ConfigRoot)
		if err != nil {
			return err
		}
	case nil:
		break
	}

	// The node will also close the repo but there are many places we could
	// fail before we get to that. It can't hurt to close it twice.
	defer repo.Close()

	cfg, err := cctx.GetConfig()
	if err != nil {
		return err
	}

	hValue, hasHval := req.Options[hValueKwd].(string)

	migrated := config.MigrateConfig(cfg, inited, hasHval)
	if migrated {
		// Flush changes if migrated
		err = repo.SetConfig(cfg)
		if err != nil {
			return err
		}
	}

	offline, _ := req.Options[offlineKwd].(bool)
	ipnsps, _ := req.Options[enableIPNSPubSubKwd].(bool)
	pubsub, _ := req.Options[enablePubSubKwd].(bool)
	mplex, _ := req.Options[enableMultiplexKwd].(bool)

	// Btfs auto update.
	url := fmt.Sprint(strings.Split(cfg.Addresses.API[0], "/")[2], ":", strings.Split(cfg.Addresses.API[0], "/")[4])

	if !cfg.Experimental.DisableAutoUpdate {
		go update(url, hValue)
	} else {
		fmt.Println("Auto-update was disabled as config Experimental.DisableAutoUpdate was set as True")
	}

	// Start assembling node config
	ncfg := &core.BuildCfg{
		Repo:                        repo,
		Permanent:                   true, // It is temporary way to signify that node is permanent
		Online:                      !offline,
		DisableEncryptedConnections: unencrypted,
		ExtraOpts: map[string]bool{
			"pubsub": pubsub,
			"ipnsps": ipnsps,
			"mplex":  mplex,
		},
		//TODO(Kubuxu): refactor Online vs Offline by adding Permanent vs Ephemeral
	}

	// Check if swarm port is overriden
	// Continue with default setup on error
	swarmPort, ok := req.Options[swarmPortKwd].(string)
	if ok {
		sp := strings.Split(swarmPort, ":")
		if len(sp) != 2 {
			log.Errorf("Invalid swarm-port: %v", swarmPort)
		} else {
			wanPort, err1 := strconv.Atoi(sp[0])
			lanPort, err2 := strconv.Atoi(sp[1])
			if err1 != nil || err2 != nil {
				log.Errorf("Invalid swarm-port: %v", swarmPort)
			} else {
				err = ncfg.AnnouncePublicIpWithPort(wanPort, lanPort)
				if err != nil {
					log.Errorf("Failed to announce new swarm address: %v", err)
				}
			}
		}
	}

	routingOption, _ := req.Options[routingOptionKwd].(string)
	if routingOption == routingOptionDefaultKwd {
		cfg, err := repo.Config()
		if err != nil {
			return err
		}

		routingOption = cfg.Routing.Type
		if routingOption == "" {
			routingOption = routingOptionDHTKwd
		}
	}
	switch routingOption {
	case routingOptionSupernodeKwd:
		return errors.New("supernode routing was never fully implemented and has been removed")
	case routingOptionDHTClientKwd:
		ncfg.Routing = libp2p.DHTClientOption
	case routingOptionDHTKwd:
		ncfg.Routing = libp2p.DHTOption
	case routingOptionNoneKwd:
		ncfg.Routing = libp2p.NilRouterOption
	default:
		return fmt.Errorf("unrecognized routing option: %s", routingOption)
	}

	node, err := core.NewNode(req.Context, ncfg)
	if err != nil {
		log.Error("error from node construction: ", err)
		return err
	}
	node.IsDaemon = true

	//Check if there is a swarm.key at btfs loc. This would still print fingerprint if they created a swarm.key with the same values
	spath := filepath.Join(cctx.ConfigRoot, "swarm.key")
	if node.PNetFingerprint != nil && util.FileExists(spath) {
		fmt.Println("Swarm is limited to private network of peers with the swarm key")
		fmt.Printf("Swarm key fingerprint: %x\n", node.PNetFingerprint)
	}

	printSwarmAddrs(node)

	defer func() {
		// We wait for the node to close first, as the node has children
		// that it will wait for before closing, such as the API server.
		node.Close()

		select {
		case <-req.Context.Done():
			log.Info("Gracefully shut down daemon")
		default:
		}
	}()

	cctx.ConstructNode = func() (*core.IpfsNode, error) {
		return node, nil
	}

	// Start "core" plugins. We want to do this *before* starting the HTTP
	// API as the user may be relying on these plugins.
	api, err := coreapi.NewCoreAPI(node)
	if err != nil {
		return err
	}
	err = cctx.Plugins.Start(api)
	if err != nil {
		return err
	}
	node.Process.AddChild(goprocess.WithTeardown(cctx.Plugins.Close))

	// construct api endpoint - every time
	apiErrc, err := serveHTTPApi(req, cctx)
	if err != nil {
		return err
	}

	// construct fuse mountpoints - if the user provided the --mount flag
	mount, _ := req.Options[mountKwd].(bool)
	if mount && offline {
		return cmds.Errorf(cmds.ErrClient, "mount is not currently supported in offline mode")
	}
	if mount {
		if err := mountFuse(req, cctx); err != nil {
			return err
		}
	}

	// repo blockstore GC - if --enable-gc flag is present
	gcErrc, err := maybeRunGC(req, node)
	if err != nil {
		return err
	}

	// construct http gateway - if it is set in the config
	var gwErrc <-chan error
	if len(cfg.Addresses.Gateway) > 0 {
		var err error
		gwErrc, err = serveHTTPGateway(req, cctx)
		if err != nil {
			return err
		}
	}

	// construct http remote api - if it is set in the config
	var rapiErrc <-chan error
	if len(cfg.Addresses.RemoteAPI) > 0 {
		var err error
		rapiErrc, err = serveHTTPRemoteApi(req, cctx)
		if err != nil {
			return err
		}
	}

	// initialize metrics collector
	prometheus.MustRegister(&corehttp.IpfsNodeCollector{Node: node})

	// The daemon is *finally* ready.
	fmt.Printf("Daemon is ready\n")

	runStartupTest, _ := req.Options[enableStartupTest].(bool)

	// BTFS functional test
	if runStartupTest {
		functest(cfg.Services.StatusServerDomain, cfg.Identity.PeerID, hValue)
	}

	// set Analytics flag if specified
	if dc, ok := req.Options[enableDataCollection]; ok == true {
		node.Repo.SetConfigKey("Experimental.Analytics", dc)
	}
	// Spin jobs in the background
	spin.Analytics(cctx.ConfigRoot, node, version.CurrentVersionNumber, hValue)
	spin.Hosts(node, env)
	spin.Contracts(node, req, env, nodepb.ContractStat_HOST.String())

	// Give the user some immediate feedback when they hit C-c
	go func() {
		<-req.Context.Done()
		fmt.Println("Received interrupt signal, shutting down...")
		fmt.Println("(Hit ctrl-c again to force-shutdown the daemon.)")
	}()

	// collect long-running errors and block for shutdown
	// TODO(cryptix): our fuse currently doesnt follow this pattern for graceful shutdown
	var errs error
	for err := range merge(apiErrc, gwErrc, rapiErrc, gcErrc) {
		if err != nil {
			errs = multierror.Append(errs, err)
		}
	}

	return errs
}

// serveHTTPApi collects options, creates listener, prints status message and starts serving requests
func serveHTTPApi(req *cmds.Request, cctx *oldcmds.Context) (<-chan error, error) {
	cfg, err := cctx.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("serveHTTPApi: GetConfig() failed: %s", err)
	}

	apiAddrs := make([]string, 0, 2)
	apiAddr, _ := req.Options[commands.ApiOption].(string)
	if apiAddr == "" {
		apiAddrs = cfg.Addresses.API
	} else {
		apiAddrs = append(apiAddrs, apiAddr)
	}

	listeners := make([]manet.Listener, 0, len(apiAddrs))
	for _, addr := range apiAddrs {
		apiMaddr, err := ma.NewMultiaddr(addr)
		if err != nil {
			return nil, fmt.Errorf("serveHTTPApi: invalid API address: %q (err: %s)", apiAddr, err)
		}

		apiLis, err := manet.Listen(apiMaddr)
		if err != nil {
			return nil, fmt.Errorf("serveHTTPApi: manet.Listen(%s) failed: %s", apiMaddr, err)
		}

		// we might have listened to /tcp/0 - lets see what we are listing on
		apiMaddr = apiLis.Multiaddr()
		fmt.Printf("API server listening on %s\n", apiMaddr)
		fmt.Printf("WebUI: http://%s/webui\n", apiLis.Addr())
		fmt.Printf("HostUI: http://%s/hostui\n", apiLis.Addr())
		listeners = append(listeners, apiLis)
	}

	// by default, we don't let you load arbitrary btfs objects through the api,
	// because this would open up the api to scripting vulnerabilities.
	// only the webui objects are allowed.
	// if you know what you're doing, go ahead and pass --unrestricted-api.
	unrestricted, _ := req.Options[unrestrictedApiAccessKwd].(bool)
	gatewayOpt := corehttp.GatewayOption(false, corehttp.WebUIPaths...)
	if unrestricted {
		gatewayOpt = corehttp.GatewayOption(true, "/btfs", "/btns")
	}

	var opts = []corehttp.ServeOption{
		corehttp.MetricsCollectionOption("api"),
		corehttp.CheckVersionOption(),
		corehttp.CommandsOption(*cctx),
		corehttp.WebUIOption,
		corehttp.HostUIOption,
		gatewayOpt,
		corehttp.VersionOption(),
		defaultMux("/debug/vars"),
		defaultMux("/debug/pprof/"),
		corehttp.MutexFractionOption("/debug/pprof-mutex/"),
		corehttp.MetricsScrapingOption("/debug/metrics/prometheus"),
		corehttp.LogOption(),
	}

	if len(cfg.Gateway.RootRedirect) > 0 {
		opts = append(opts, corehttp.RedirectOption("", cfg.Gateway.RootRedirect))
	}

	node, err := cctx.ConstructNode()
	if err != nil {
		return nil, fmt.Errorf("serveHTTPApi: ConstructNode() failed: %s", err)
	}

	if err := node.Repo.SetAPIAddr(listeners[0].Multiaddr()); err != nil {
		return nil, fmt.Errorf("serveHTTPApi: SetAPIAddr() failed: %s", err)
	}

	errc := make(chan error)
	var wg sync.WaitGroup
	for _, apiLis := range listeners {
		wg.Add(1)
		go func(lis manet.Listener) {
			defer wg.Done()
			errc <- corehttp.Serve(node, manet.NetListener(lis), opts...)
		}(apiLis)
	}

	go func() {
		wg.Wait()
		close(errc)
	}()

	return errc, nil
}

// printSwarmAddrs prints the addresses of the host
func printSwarmAddrs(node *core.IpfsNode) {
	if !node.IsOnline {
		fmt.Println("Swarm not listening, running in offline mode.")
		return
	}

	var lisAddrs []string
	ifaceAddrs, err := node.PeerHost.Network().InterfaceListenAddresses()
	if err != nil {
		log.Errorf("failed to read listening addresses: %s", err)
	}
	for _, addr := range ifaceAddrs {
		lisAddrs = append(lisAddrs, addr.String())
	}
	sort.Strings(lisAddrs)
	for _, addr := range lisAddrs {
		fmt.Printf("Swarm listening on %s\n", addr)
	}

	var addrs []string
	for _, addr := range node.PeerHost.Addrs() {
		addrs = append(addrs, addr.String())
	}
	sort.Strings(addrs)
	for _, addr := range addrs {
		fmt.Printf("Swarm announcing %s\n", addr)
	}
}

// serveHTTPGateway collects options, creates listener, prints status message and starts serving requests
func serveHTTPGateway(req *cmds.Request, cctx *oldcmds.Context) (<-chan error, error) {
	cfg, err := cctx.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("serveHTTPGateway: GetConfig() failed: %s", err)
	}

	writable, writableOptionFound := req.Options[writableKwd].(bool)
	if !writableOptionFound {
		writable = cfg.Gateway.Writable
	}

	gatewayAddrs := cfg.Addresses.Gateway
	listeners := make([]manet.Listener, 0, len(gatewayAddrs))
	for _, addr := range gatewayAddrs {
		gatewayMaddr, err := ma.NewMultiaddr(addr)
		if err != nil {
			return nil, fmt.Errorf("serveHTTPGateway: invalid gateway address: %q (err: %s)", addr, err)
		}

		gwLis, err := manet.Listen(gatewayMaddr)
		if err != nil {
			return nil, fmt.Errorf("serveHTTPGateway: manet.Listen(%s) failed: %s", gatewayMaddr, err)
		}
		// we might have listened to /tcp/0 - lets see what we are listing on
		gatewayMaddr = gwLis.Multiaddr()

		if writable {
			fmt.Printf("Gateway (writable) server listening on %s\n", gatewayMaddr)
		} else {
			fmt.Printf("Gateway (readonly) server listening on %s\n", gatewayMaddr)
		}

		listeners = append(listeners, gwLis)
	}

	cmdctx := *cctx
	cmdctx.Gateway = true

	var opts = []corehttp.ServeOption{
		corehttp.MetricsCollectionOption("gateway"),
		corehttp.IPNSHostnameOption(),
		corehttp.GatewayOption(writable, "/btfs", "/btns"),
		corehttp.VersionOption(),
		corehttp.CheckVersionOption(),
		corehttp.CommandsROOption(cmdctx),
	}

	if cfg.Experimental.P2pHttpProxy {
		opts = append(opts, corehttp.ProxyOption())
	}

	if len(cfg.Gateway.RootRedirect) > 0 {
		opts = append(opts, corehttp.RedirectOption("", cfg.Gateway.RootRedirect))
	}

	node, err := cctx.ConstructNode()
	if err != nil {
		return nil, fmt.Errorf("serveHTTPGateway: ConstructNode() failed: %s", err)
	}

	errc := make(chan error)
	var wg sync.WaitGroup
	for _, lis := range listeners {
		wg.Add(1)
		go func(lis manet.Listener) {
			defer wg.Done()
			errc <- corehttp.Serve(node, manet.NetListener(lis), opts...)
		}(lis)
	}

	go func() {
		wg.Wait()
		close(errc)
	}()

	return errc, nil
}

// serveHTTPRemoteApi collects options, creates listener, prints status message and starts serving requests
func serveHTTPRemoteApi(req *cmds.Request, cctx *oldcmds.Context) (<-chan error, error) {
	cfg, err := cctx.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("serveHTTPRemoteApi: GetConfig() failed: %s", err)
	}

	if !cfg.Experimental.Libp2pStreamMounting {
		return nil, fmt.Errorf("serveHTTPRemoteApi: libp2p stream mounting must be enabled")
	}

	rapiAddrs := cfg.Addresses.RemoteAPI
	listeners := make([]manet.Listener, 0, len(rapiAddrs))
	for _, addr := range rapiAddrs {
		rapiMaddr, err := ma.NewMultiaddr(addr)
		if err != nil {
			return nil, fmt.Errorf("serveHTTPRemoteApi: invalid remote api address: %q (err: %s)", addr, err)
		}

		rapiLis, err := manet.Listen(rapiMaddr)
		if err != nil {
			return nil, fmt.Errorf("serveHTTPRemoteApi: manet.Listen(%s) failed: %s", rapiMaddr, err)
		}
		// we might have listened to /tcp/0 - lets see what we are listing on
		rapiMaddr = rapiLis.Multiaddr()
		fmt.Printf("Remote API server listening on %s\n", rapiMaddr)

		listeners = append(listeners, rapiLis)
	}

	var opts = []corehttp.ServeOption{
		corehttp.MetricsCollectionOption("remote_api"),
		corehttp.VersionOption(),
		corehttp.CheckVersionOption(),
		corehttp.CommandsRemoteOption(*cctx),
	}

	node, err := cctx.ConstructNode()
	if err != nil {
		return nil, fmt.Errorf("serveHTTPRemoteApi: ConstructNode() failed: %s", err)
	}

	// set default listener to remote api endpoint
	if _, err := node.P2P.ForwardRemote(node.Context(),
		httpremote.P2PRemoteCallProto, listeners[0].Multiaddr(), false); err != nil {
		return nil, fmt.Errorf("serveHTTPRemoteApi: ForwardRemote() failed: %s", err)
	}

	errc := make(chan error)
	var wg sync.WaitGroup
	for _, lis := range listeners {
		wg.Add(1)
		go func(lis manet.Listener) {
			defer wg.Done()
			errc <- corehttp.Serve(node, manet.NetListener(lis), opts...)
		}(lis)
	}

	go func() {
		wg.Wait()
		close(errc)
	}()

	return errc, nil
}

//collects options and opens the fuse mountpoint
func mountFuse(req *cmds.Request, cctx *oldcmds.Context) error {
	cfg, err := cctx.GetConfig()
	if err != nil {
		return fmt.Errorf("mountFuse: GetConfig() failed: %s", err)
	}

	fsdir, found := req.Options[ipfsMountKwd].(string)
	if !found {
		fsdir = cfg.Mounts.IPFS
	}

	nsdir, found := req.Options[ipnsMountKwd].(string)
	if !found {
		nsdir = cfg.Mounts.IPNS
	}

	node, err := cctx.ConstructNode()
	if err != nil {
		return fmt.Errorf("mountFuse: ConstructNode() failed: %s", err)
	}

	err = nodeMount.Mount(node, fsdir, nsdir)
	if err != nil {
		return err
	}
	fmt.Printf("BTFS mounted at: %s\n", fsdir)
	fmt.Printf("BTNS mounted at: %s\n", nsdir)
	return nil
}

func maybeRunGC(req *cmds.Request, node *core.IpfsNode) (<-chan error, error) {
	enableGC, _ := req.Options[enableGCKwd].(bool)
	if !enableGC {
		return nil, nil
	}

	errc := make(chan error)
	go func() {
		errc <- corerepo.PeriodicGC(req.Context, node)
		close(errc)
	}()
	return errc, nil
}

// merge does fan-in of multiple read-only error channels
// taken from http://blog.golang.org/pipelines
func merge(cs ...<-chan error) <-chan error {
	var wg sync.WaitGroup
	out := make(chan error)

	// Start an output goroutine for each input channel in cs.  output
	// copies values from c to out until c is closed, then calls wg.Done.
	output := func(c <-chan error) {
		for n := range c {
			out <- n
		}
		wg.Done()
	}
	for _, c := range cs {
		if c != nil {
			wg.Add(1)
			go output(c)
		}
	}

	// Start a goroutine to close out once all the output goroutines are
	// done.  This must start after the wg.Add call.
	go func() {
		wg.Wait()
		close(out)
	}()
	return out
}

func YesNoPrompt(prompt string) bool {
	var s string
	for i := 0; i < 3; i++ {
		fmt.Printf("%s ", prompt)
		fmt.Scanf("%s", &s)
		switch s {
		case "y", "Y":
			return true
		case "n", "N":
			return false
		case "":
			return false
		}
		fmt.Println("Please press either 'y' or 'n'")
	}

	return false
}

func printVersion() {
	fmt.Printf("go-btfs version: %s-%s\n", version.CurrentVersionNumber, version.CurrentCommit)
	fmt.Printf("Repo version: %d\n", fsrepo.RepoVersion)
	fmt.Printf("System version: %s\n", runtime.GOARCH+"/"+runtime.GOOS)
	fmt.Printf("Golang version: %s\n", runtime.Version())
}

func getBtfsBinaryPath() (string, error) {
	defaultBtfsPath, err := getCurrentPath()
	if err != nil {
		log.Errorf("Get current program execution path error, reasons: [%v]", err)
		return "", err
	}

	ext := ""
	sep := "/"
	if runtime.GOOS == "windows" {
		ext = ".exe"
		sep = "\\"
	}
	latestBtfsBinary := "btfs" + ext
	latestBtfsBinaryPath := fmt.Sprint(defaultBtfsPath, sep, latestBtfsBinary)

	return latestBtfsBinaryPath, nil
}

func functest(statusServerDomain, peerId, hValue string) {
	btfsBinaryPath, err := getBtfsBinaryPath()
	if err != nil {
		fmt.Printf("Get btfs path failed, BTFS daemon test skipped\n")
		os.Exit(findBTFSBinaryFailed)
	}

	// prepare functional test before start btfs daemon
	ready_to_test := prepare_test(btfsBinaryPath, statusServerDomain, peerId, hValue)
	// start btfs functional test
	if ready_to_test {
		test_success := false
		// try up to two times
		for i := 0; i < 2; i++ {
			err := get_functest(btfsBinaryPath)
			if err != nil {
				fmt.Printf("BTFS daemon get file test failed! Reason: %v\n", err)
				SendError(err.Error(), statusServerDomain, peerId, hValue)
			} else {
				fmt.Printf("BTFS daemon get file test succeeded!\n")
				test_success = true
				break
			}
		}
		if !test_success {
			fmt.Printf("BTFS daemon get file test failed twice! exiting\n")
			os.Exit(getFileTestFailed)
		}
		test_success = false
		// try up to two times
		for i := 0; i < 2; i++ {
			if err := add_functest(btfsBinaryPath, peerId); err != nil {
				fmt.Printf("BTFS daemon add file test failed! Reason: %v\n", err)
				SendError(err.Error(), statusServerDomain, peerId, hValue)
			} else {
				fmt.Printf("BTFS daemon add file test succeeded!\n")
				test_success = true
				break
			}
		}
		if !test_success {
			fmt.Printf("BTFS daemon add file test failed twice! exiting\n")
			os.Exit(addFileTestFailed)
		}
	} else {
		fmt.Printf("BTFS daemon test skipped\n")
	}
}
