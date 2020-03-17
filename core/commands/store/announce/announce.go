package announce

import (
	"fmt"
	cmds "github.com/TRON-US/go-btfs-cmds"
	"github.com/TRON-US/go-btfs/core/commands/cmdenv"
	"github.com/TRON-US/go-btfs/core/commands/storage"
	"github.com/alecthomas/units"
)

const (
	hostStoragePriceOptionName    = "host-storage-price"
	hostBandwidthPriceOptionName  = "host-bandwidth-price"
	hostCollateralPriceOptionName = "host-collateral-price"
	hostBandwidthLimitOptionName  = "host-bandwidth-limit"
	hostStorageTimeMinOptionName  = "host-storage-time-min"
	hostStorageMaxOptionName      = "host-storage-max"
	hostStorageEnableOptionName   = "enable-host-mode"

	bttTotalSupply uint64 = 990_000_000_000
)

var StorageAnnounceCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Update and announce storage host information.",
		ShortDescription: `
This command updates host information and broadcasts to the BTFS network. 

Examples

To set the min price per GiB to 1000000 JUST (1 BTT):
$ btfs storage announce --host-storage-price=1000000`,
	},
	Options: []cmds.Option{
		cmds.Uint64Option(hostStoragePriceOptionName, "s", "Min price per GiB of storage per day in JUST."),
		cmds.Uint64Option(hostBandwidthPriceOptionName, "b", "Min price per MiB of bandwidth in JUST."),
		cmds.Uint64Option(hostCollateralPriceOptionName, "cl", "Max collateral stake per hour per GiB in JUST."),
		cmds.FloatOption(hostBandwidthLimitOptionName, "l", "Max bandwidth limit per MB/s."),
		cmds.Uint64Option(hostStorageTimeMinOptionName, "d", "Min number of days for storage."),
		cmds.Uint64Option(hostStorageMaxOptionName, "m", "Max number of GB this host provides for storage."),
		cmds.BoolOption(hostStorageEnableOptionName, "hm", "Enable/disable host storage mode. By default no mode change is made. When specified, toggles between enable/disable host mode."),
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		cfg, err := cmdenv.GetConfig(env)
		if err != nil {
			return err
		}

		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}

		hm, hmFound := req.Options[hostStorageEnableOptionName].(bool)
		if hmFound {
			// New value is different from stored, then
			if hm != cfg.Experimental.StorageHostEnabled {
				cfg.Experimental.StorageHostEnabled = hm
				err = n.Repo.SetConfig(cfg)
				if err != nil {
					return err
				}
			}
			// turned off, do nothing
			if !hm {
				return nil
			}
		}

		if !cfg.Experimental.StorageHostEnabled {
			return fmt.Errorf("storage host api not enabled")
		}

		sp, spFound := req.Options[hostStoragePriceOptionName].(uint64)
		bp, bpFound := req.Options[hostBandwidthPriceOptionName].(uint64)
		cp, cpFound := req.Options[hostCollateralPriceOptionName].(uint64)
		bl, blFound := req.Options[hostBandwidthLimitOptionName].(float64)
		stm, stmFound := req.Options[hostStorageTimeMinOptionName].(uint64)
		sm, smFound := req.Options[hostStorageMaxOptionName].(uint64)

		if sp > bttTotalSupply || cp > bttTotalSupply || bp > bttTotalSupply {
			return fmt.Errorf("maximum price is %d", bttTotalSupply)
		}

		ns, err := storage.GetHostStorageConfig(req.Context, n)
		if err != nil {
			return err
		}

		// Update fields if set
		if spFound {
			ns.StoragePriceAsk = sp
		}
		if bpFound {
			ns.BandwidthPriceAsk = bp
		}
		if cpFound {
			ns.CollateralStake = cp
		}
		if blFound {
			ns.BandwidthLimit = bl
		}
		if stmFound {
			ns.StorageTimeMin = stm
		}
		// Storage size max is set in config instead of dynamic store
		if smFound {
			cfgRoot, err := cmdenv.GetConfigRoot(env)
			if err != nil {
				return err
			}
			sm = sm * uint64(units.GB)
			_, err = storage.CheckAndValidateHostStorageMax(cfgRoot, n.Repo, &sm, false)
			if err != nil {
				return err
			}
		}

		err = storage.PutHostStorageConfig(n, ns)
		if err != nil {
			return err
		}

		return nil
	},
}
