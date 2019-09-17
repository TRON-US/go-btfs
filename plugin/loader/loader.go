package loader

import (
	"fmt"
	"os"
	"strings"

	coredag "github.com/TRON-US/go-btfs/core/coredag"
	plugin "github.com/TRON-US/go-btfs/plugin"
	fsrepo "github.com/TRON-US/go-btfs/repo/fsrepo"

	ipld "github.com/ipfs/go-ipld-format"
	logging "github.com/ipfs/go-log"
	coreiface "github.com/TRON-US/interface-go-btfs-core"
	opentracing "github.com/opentracing/opentracing-go"
)

var log = logging.Logger("plugin/loader")

var loadPluginsFunc = func(string) ([]plugin.Plugin, error) {
	return nil, nil
}

type loaderState int

const (
	loaderLoading loaderState = iota
	loaderInitializing
	loaderInitialized
	loaderInjecting
	loaderInjected
	loaderStarting
	loaderStarted
	loaderClosing
	loaderClosed
	loaderFailed
)

func (ls loaderState) String() string {
	switch ls {
	case loaderLoading:
		return "Loading"
	case loaderInitializing:
		return "Initializing"
	case loaderInitialized:
		return "Initialized"
	case loaderInjecting:
		return "Injecting"
	case loaderInjected:
		return "Injected"
	case loaderStarting:
		return "Starting"
	case loaderStarted:
		return "Started"
	case loaderClosing:
		return "Closing"
	case loaderClosed:
		return "Closed"
	case loaderFailed:
		return "Failed"
	default:
		return "Unknown"
	}
}

// PluginLoader keeps track of loaded plugins.
//
// To use:
// 1. Load any desired plugins with Load and LoadDirectory. Preloaded plugins
//    will automatically be loaded.
// 2. Call Initialize to run all initialization logic.
// 3. Call Inject to register the plugins.
// 4. Optionally call Start to start plugins.
// 5. Call Close to close all plugins.
type PluginLoader struct {
	state   loaderState
	plugins map[string]plugin.Plugin
	started []plugin.Plugin
}

// NewPluginLoader creates new plugin loader
func NewPluginLoader() (*PluginLoader, error) {
	loader := &PluginLoader{plugins: make(map[string]plugin.Plugin, len(preloadPlugins))}
	for _, v := range preloadPlugins {
		if err := loader.Load(v); err != nil {
			return nil, err
		}
	}
	return loader, nil
}

func (loader *PluginLoader) assertState(state loaderState) error {
	if loader.state != state {
		return fmt.Errorf("loader state must be %s, was %s", state, loader.state)
	}
	return nil
}

func (loader *PluginLoader) transition(from, to loaderState) error {
	if err := loader.assertState(from); err != nil {
		return err
	}
	loader.state = to
	return nil
}

// Load loads a plugin into the plugin loader.
func (loader *PluginLoader) Load(pl plugin.Plugin) error {
	if err := loader.assertState(loaderLoading); err != nil {
		return err
	}

	name := pl.Name()
	if ppl, ok := loader.plugins[name]; ok {
		// plugin is already loaded
		return fmt.Errorf(
			"plugin: %s, is duplicated in version: %s, "+
				"while trying to load dynamically: %s",
			name, ppl.Version(), pl.Version())
	}
	loader.plugins[name] = pl
	return nil
}

// LoadDirectory loads a directory of plugins into the plugin loader.
func (loader *PluginLoader) LoadDirectory(pluginDir string) error {
	if err := loader.assertState(loaderLoading); err != nil {
		return err
	}
	newPls, err := loadDynamicPlugins(pluginDir)
	if err != nil {
		return err
	}

	for _, pl := range newPls {
		if err := loader.Load(pl); err != nil {
			return err
		}
	}
	return nil
}

func loadDynamicPlugins(pluginDir string) ([]plugin.Plugin, error) {
	_, err := os.Stat(pluginDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return loadPluginsFunc(pluginDir)
}

// Initialize initializes all loaded plugins
func (loader *PluginLoader) Initialize() error {
	if err := loader.transition(loaderLoading, loaderInitializing); err != nil {
		return err
	}
	for _, p := range loader.plugins {
		err := p.Init()
		if err != nil {
			loader.state = loaderFailed
			return err
		}
	}

	return loader.transition(loaderInitializing, loaderInitialized)
}

// Inject hooks all the plugins into the appropriate subsystems.
func (loader *PluginLoader) Inject() error {
	if err := loader.transition(loaderInitialized, loaderInjecting); err != nil {
		return err
	}

	for _, pl := range loader.plugins {
		if pl, ok := pl.(plugin.PluginIPLD); ok {
			err := injectIPLDPlugin(pl)
			if err != nil {
				loader.state = loaderFailed
				return err
			}
		}
		if pl, ok := pl.(plugin.PluginTracer); ok {
			err := injectTracerPlugin(pl)
			if err != nil {
				loader.state = loaderFailed
				return err
			}
		}
		if pl, ok := pl.(plugin.PluginDatastore); ok {
			err := injectDatastorePlugin(pl)
			if err != nil {
				loader.state = loaderFailed
				return err
			}
		}
	}

	return loader.transition(loaderInjecting, loaderInjected)
}

// Start starts all long-running plugins.
func (loader *PluginLoader) Start(iface coreiface.CoreAPI) error {
	if err := loader.transition(loaderInjected, loaderStarting); err != nil {
		return err
	}
	for _, pl := range loader.plugins {
		if pl, ok := pl.(plugin.PluginDaemon); ok {
			err := pl.Start(iface)
			if err != nil {
				_ = loader.Close()
				return err
			}
			loader.started = append(loader.started, pl)
		}
	}

	return loader.transition(loaderStarting, loaderStarted)
}

// StopDaemon stops all long-running plugins.
func (loader *PluginLoader) Close() error {
	switch loader.state {
	case loaderClosing, loaderFailed, loaderClosed:
		// nothing to do.
		return nil
	}
	loader.state = loaderClosing

	var errs []string
	started := loader.started
	loader.started = nil
	for _, pl := range started {
		if pl, ok := pl.(plugin.PluginDaemon); ok {
			err := pl.Close()
			if err != nil {
				errs = append(errs, fmt.Sprintf(
					"error closing plugin %s: %s",
					pl.Name(),
					err.Error(),
				))
			}
		}
	}
	if errs != nil {
		loader.state = loaderFailed
		return fmt.Errorf(strings.Join(errs, "\n"))
	}
	loader.state = loaderClosed
	return nil
}

func injectDatastorePlugin(pl plugin.PluginDatastore) error {
	return fsrepo.AddDatastoreConfigHandler(pl.DatastoreTypeName(), pl.DatastoreConfigParser())
}

func injectIPLDPlugin(pl plugin.PluginIPLD) error {
	err := pl.RegisterBlockDecoders(ipld.DefaultBlockDecoder)
	if err != nil {
		return err
	}
	return pl.RegisterInputEncParsers(coredag.DefaultInputEncParsers)
}

func injectTracerPlugin(pl plugin.PluginTracer) error {
	tracer, err := pl.InitTracer()
	if err != nil {
		return err
	}
	opentracing.SetGlobalTracer(tracer)
	return nil
}
