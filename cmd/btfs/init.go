package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strconv"
	"strings"

	assets "github.com/TRON-US/go-btfs/assets"
	oldcmds "github.com/TRON-US/go-btfs/commands"
	core "github.com/TRON-US/go-btfs/core"
	namesys "github.com/TRON-US/go-btfs/namesys"
	fsrepo "github.com/TRON-US/go-btfs/repo/fsrepo"

	cmds "github.com/TRON-US/go-btfs-cmds"
	config "github.com/TRON-US/go-btfs-config"
	files "github.com/TRON-US/go-btfs-files"
	"github.com/tyler-smith/go-bip32"
	"github.com/tyler-smith/go-bip39"
)

const (
	nBitsForKeypairDefault = 2048
	defaultEntropy         = 128
	mnemonicLength         = 12
	bitsOptionName         = "bits"
	emptyRepoOptionName    = "empty-repo"
	profileOptionName      = "profile"
	keyTypeOptionName      = "key"
	keyTypeDefault         = "Secp256k1"
	importKeyOptionName    = "import"
	rmOnUnpinOptionName    = "rm-on-unpin"
	seedOptionName         = "seed"
)

var initCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Initializes btfs config file.",
		ShortDescription: `
Initializes btfs configuration files and generates a new keypair.

If you are going to run BTFS in server environment, you may want to
initialize it using 'server' profile.

For the list of available profiles see 'btfs config profile --help'

btfs uses a repository in the local file system. By default, the repo is
located at ~/.btfs. To change the repo location, set the $BTFS_PATH
environment variable:

    export BTFS_PATH=/path/to/btfsrepo
`,
	},
	Arguments: []cmds.Argument{
		cmds.FileArg("default-config", false, false, "Initialize with the given configuration.").EnableStdin(),
	},
	Options: []cmds.Option{
		cmds.IntOption(bitsOptionName, "b", "Number of bits to use in the generated RSA private key.").WithDefault(nBitsForKeypairDefault),
		cmds.BoolOption(emptyRepoOptionName, "e", "Don't add and pin help files to the local storage."),
		cmds.StringOption(profileOptionName, "p", "Apply profile settings to config. Multiple profiles can be separated by ','"),
		cmds.StringOption(keyTypeOptionName, "k", "Key generation algorithm, e.g. RSA, Ed25519, Secp256k1, ECDSA, BIP39. By default is Secp256k1"),
		cmds.StringOption(importKeyOptionName, "i", "Import TRON private key to generate btfs PeerID."),
		cmds.BoolOption(rmOnUnpinOptionName, "r", "Remove unpinned files.").WithDefault(false),
		cmds.StringOption(seedOptionName, "s", "Import seed phrase"),

		// TODO need to decide whether to expose the override as a file or a
		// directory. That is: should we allow the user to also specify the
		// name of the file?
		// TODO cmds.StringOption("event-logs", "l", "Location for machine-readable event logs."),
	},
	PreRun: func(req *cmds.Request, env cmds.Environment) error {
		cctx := env.(*oldcmds.Context)
		daemonLocked, err := fsrepo.LockedByOtherProcess(cctx.ConfigRoot)
		if err != nil {
			return err
		}

		log.Info("checking if daemon is running...")
		if daemonLocked {
			log.Debug("btfs daemon is running")
			e := "btfs daemon is running. please stop it to run this command"
			return cmds.ClientError(e)
		}

		return nil
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		cctx := env.(*oldcmds.Context)
		empty, _ := req.Options[emptyRepoOptionName].(bool)
		nBitsForKeypair, _ := req.Options[bitsOptionName].(int)
		rmOnUnpin, _ := req.Options[rmOnUnpinOptionName].(bool)

		var conf *config.Config

		f := req.Files
		if f != nil {
			it := req.Files.Entries()
			if !it.Next() {
				if it.Err() != nil {
					return it.Err()
				}
				return fmt.Errorf("file argument was nil")
			}
			file := files.FileFromEntry(it)
			if file == nil {
				return fmt.Errorf("expected a regular file")
			}

			conf = &config.Config{}
			if err := json.NewDecoder(file).Decode(conf); err != nil {
				return err
			}
		}

		profile, _ := req.Options[profileOptionName].(string)

		var profiles []string
		if profile != "" {
			profiles = strings.Split(profile, ",")
		}

		importKey, _ := req.Options[importKeyOptionName].(string)
		keyType, _ := req.Options[keyTypeOptionName].(string)
		seedPhrase, _ := req.Options[seedOptionName].(string)

		finalImportKey, error := generateKey(importKey, keyType, seedPhrase)
		if error == nil {
			return doInit(os.Stdout, cctx.ConfigRoot, empty, nBitsForKeypair, profiles, conf, keyType, finalImportKey, rmOnUnpin)
		} else {
			return error
		}
	},
}

func generateKey(importKey string, keyType string, seedPhrase string) (string, error) {
	mnemonicLen := len(strings.Split(seedPhrase, ","))
	mnemonic := strings.ReplaceAll(seedPhrase, ",", " ")

	if importKey != "" && keyType != "" {
		return "", fmt.Errorf("cannot specify key type and import TRON private key at the same time")
	} else if seedPhrase != "" {
		if mnemonicLen != mnemonicLength {
			return "", fmt.Errorf("The seed phrase required to generate TRON private key needs to contain 12 words. Provided mnemonic has %v words.", mnemonicLen)
		}
		if err := !bip39.IsMnemonicValid(mnemonic); err {
			return "", fmt.Errorf("Entered seed phrase is not valid")
		}
		fmt.Println("Generating TRON key with BIP39 seed phrase...")
		importKey = generatePrivKeyUsingBIP39(mnemonic)
	}

	if keyType == "" {
		keyType = keyTypeDefault
	} else if keyType == "BIP39" {
		fmt.Println("Generating TRON key with BIP39 seed phrase...")
		importKey = generatePrivKeyUsingBIP39("")
	}
	return importKey, nil
}

var errRepoExists = errors.New(`btfs configuration file already exists!
Reinitializing would overwrite your keys.
`)

func generatePrivKeyUsingBIP39(mnemonic string) string {
	if mnemonic == "" {
		entropy, _ := bip39.NewEntropy(defaultEntropy)
		mnemonic, _ = bip39.NewMnemonic(entropy)
	}

	// Generate a Bip32 HD wallet for the mnemonic and a user supplied password
	seed := bip39.NewSeed(mnemonic, "")

	masterKey, _ := bip32.NewMasterKey(seed)
	publicKey := masterKey.PublicKey()

	childKey, _ := masterKey.NewChildKey(44 + bip32.FirstHardenedChild)
	childKey2, _ := childKey.NewChildKey(195 + bip32.FirstHardenedChild)
	childKey3, _ := childKey2.NewChildKey(0 + bip32.FirstHardenedChild)
	childKey4, _ := childKey3.NewChildKey(0)
	childKey5, _ := childKey4.NewChildKey(0)

	encoding := childKey5.Key
	importKey := hex.EncodeToString(encoding)

	// Display mnemonic and keys
	fmt.Println("Mnemonic: ", mnemonic)
	fmt.Println("Tron private key: ", importKey)
	fmt.Println("Master public key: ", publicKey)

	return importKey
}

func initWithDefaults(out io.Writer, repoRoot string, profile string) error {
	var profiles []string
	if profile != "" {
		profiles = strings.Split(profile, ",")
	}

	// the last argument (false) refers to the configuration variable Experimental.RemoveOnUnpin
	return doInit(out, repoRoot, false, nBitsForKeypairDefault, profiles, nil, keyTypeDefault, "", false)
}

func doInit(out io.Writer, repoRoot string, empty bool, nBitsForKeypair int, confProfiles []string, conf *config.Config,
	keyType string, importKey string, rmOnUnpin bool) error {
	if _, err := fmt.Fprintf(out, "initializing BTFS node at %s\n", repoRoot); err != nil {
		return err
	}

	if err := checkWritable(repoRoot); err != nil {
		return err
	}

	if fsrepo.IsInitialized(repoRoot) {
		return errRepoExists
	}

	if conf == nil {
		var err error
		conf, err = config.Init(out, nBitsForKeypair, keyType, importKey, rmOnUnpin)
		if err != nil {
			return err
		}

		if rmOnUnpin {
			raw := json.RawMessage(`{"rmOnUnpin":"` + strconv.FormatBool(rmOnUnpin) + `"}`)
			conf.Datastore.Params = &raw
		}
	}

	for _, profile := range confProfiles {
		transformer, ok := config.Profiles[profile]
		if !ok {
			return fmt.Errorf("invalid configuration profile: %s", profile)
		}

		if err := transformer.Transform(conf); err != nil {
			return err
		}
	}

	if err := fsrepo.Init(repoRoot, conf); err != nil {
		return err
	}

	if !empty {
		if err := addDefaultAssets(out, repoRoot); err != nil {
			return err
		}
	}

	return initializeIpnsKeyspace(repoRoot)
}

func checkWritable(dir string) error {
	_, err := os.Stat(dir)
	if err == nil {
		// dir exists, make sure we can write to it
		testfile := path.Join(dir, "test")
		fi, err := os.Create(testfile)
		if err != nil {
			if os.IsPermission(err) {
				return fmt.Errorf("%s is not writeable by the current user", dir)
			}
			return fmt.Errorf("unexpected error while checking writeablility of repo root: %s", err)
		}
		fi.Close()
		return os.Remove(testfile)
	}

	if os.IsNotExist(err) {
		// dir doesn't exist, check that we can create it
		return os.Mkdir(dir, 0775)
	}

	if os.IsPermission(err) {
		return fmt.Errorf("cannot write to %s, incorrect permissions", err)
	}

	return err
}

func addDefaultAssets(out io.Writer, repoRoot string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r, err := fsrepo.Open(repoRoot)
	if err != nil { // NB: repo is owned by the node
		return err
	}

	buildCfg := &core.BuildCfg{Repo: r}
	nd, err := core.NewNode(ctx, buildCfg)
	if err != nil {
		return err
	}
	defer nd.Close()

	// announce public ip by default for btfs nodes running on cloud vm
	// or local network with NAT
	err = buildCfg.AnnouncePublicIp()
	if err != nil {
		// don't quit here, user can manually add to config later
		fmt.Fprintf(out, "announce public ip failed.\n")
	}

	dkey, err := assets.SeedInitDocs(nd)
	if err != nil {
		return fmt.Errorf("init: seeding init docs failed: %s", err)
	}
	log.Debugf("init: seeded init docs %s", dkey)

	if _, err = fmt.Fprintf(out, "to get started, enter:\n"); err != nil {
		return err
	}

	_, err = fmt.Fprintf(out, "\n\tbtfs cat /btfs/%s/readme\n\n", dkey)
	return err
}

func initializeIpnsKeyspace(repoRoot string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r, err := fsrepo.Open(repoRoot)
	if err != nil { // NB: repo is owned by the node
		return err
	}

	nd, err := core.NewNode(ctx, &core.BuildCfg{Repo: r})
	if err != nil {
		return err
	}
	defer nd.Close()

	return namesys.InitializeKeyspace(ctx, nd.Namesys, nd.Pinning, nd.PrivateKey)
}
