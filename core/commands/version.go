package commands

import (
	"errors"
	"fmt"
	"io"
	"runtime"
	"runtime/debug"

	version "github.com/ipfs/go-ipfs"
	fsrepo "github.com/TRON-US/go-btfs/repo/fsrepo"

	cmdkit "github.com/ipfs/go-ipfs-cmdkit"
	cmds "github.com/ipfs/go-ipfs-cmds"
)

type VersionOutput struct {
	Version string
	Commit  string
	Repo    string
	System  string
	Golang  string
}

const (
	versionNumberOptionName = "number"
	versionCommitOptionName = "commit"
	versionRepoOptionName   = "repo"
	versionAllOptionName    = "all"
)

var VersionCmd = &cmds.Command{
	Helptext: cmdkit.HelpText{
		Tagline:          "Show btfs version information.",
		ShortDescription: "Returns the current version of btfs and exits.",
	},
	Subcommands: map[string]*cmds.Command{
		"deps": depsVersionCommand,
	},

	Options: []cmdkit.Option{
		cmdkit.BoolOption(versionNumberOptionName, "n", "Only show the version number."),
		cmdkit.BoolOption(versionCommitOptionName, "Show the commit hash."),
		cmdkit.BoolOption(versionRepoOptionName, "Show repo version."),
		cmdkit.BoolOption(versionAllOptionName, "Show all version information"),
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		return cmds.EmitOnce(res, &VersionOutput{
			Version: version.CurrentVersionNumber,
			Commit:  version.CurrentCommit,
			Repo:    fmt.Sprint(fsrepo.RepoVersion),
			System:  runtime.GOARCH + "/" + runtime.GOOS, //TODO: Precise version here
			Golang:  runtime.Version(),
		})
	},
	Encoders: cmds.EncoderMap{
		cmds.Text: cmds.MakeTypedEncoder(func(req *cmds.Request, w io.Writer, version *VersionOutput) error {
			commit, _ := req.Options[versionCommitOptionName].(bool)
			commitTxt := ""
			if commit {
				commitTxt = "-" + version.Commit
			}

			all, _ := req.Options[versionAllOptionName].(bool)
			if all {
				out := fmt.Sprintf("go-btfs version: %s-%s\n"+
					"Repo version: %s\nSystem version: %s\nGolang version: %s\n",
					version.Version, version.Commit, version.Repo, version.System, version.Golang)
				fmt.Fprint(w, out)
				return nil
			}

			repo, _ := req.Options[versionRepoOptionName].(bool)
			if repo {
				fmt.Fprintln(w, version.Repo)
				return nil
			}

			number, _ := req.Options[versionNumberOptionName].(bool)
			if number {
				fmt.Fprintln(w, version.Version+commitTxt)
				return nil
			}

			fmt.Fprint(w, fmt.Sprintf("btfs version %s%s\n", version.Version, commitTxt))
			return nil
		}),
	},
	Type: VersionOutput{},
}

type Dependency struct {
	Path       string
	Version    string
	ReplacedBy string
	Sum        string
}

const pkgVersionFmt = "%s@%s"

var depsVersionCommand = &cmds.Command{
	Helptext: cmdkit.HelpText{
		Tagline: "Shows information about dependencies used for build",
		ShortDescription: `
Print out all dependencies and their versions.`,
	},
	Type: Dependency{},

	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		info, ok := debug.ReadBuildInfo()
		if !ok {
			return errors.New("no embedded dependency information")
		}
		toDependency := func(mod *debug.Module) (dep Dependency) {
			dep.Path = mod.Path
			dep.Version = mod.Version
			dep.Sum = mod.Sum
			if repl := mod.Replace; repl != nil {
				dep.ReplacedBy = fmt.Sprintf(pkgVersionFmt, repl.Path, repl.Version)
			}
			return
		}
		if err := res.Emit(toDependency(&info.Main)); err != nil {
			return err
		}
		for _, dep := range info.Deps {
			if err := res.Emit(toDependency(dep)); err != nil {
				return err
			}
		}
		return nil
	},
	Encoders: cmds.EncoderMap{
		cmds.Text: cmds.MakeTypedEncoder(func(req *cmds.Request, w io.Writer, dep Dependency) error {
			fmt.Fprintf(w, pkgVersionFmt, dep.Path, dep.Version)
			if dep.ReplacedBy != "" {
				fmt.Fprintf(w, " => %s", dep.ReplacedBy)
			}
			fmt.Fprintf(w, "\n")
			return nil
		}),
	},
}
