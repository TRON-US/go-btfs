package commands

import (
	"errors"
	"fmt"

	"github.com/TRON-US/go-btfs/core/commands/storage/upload/upload"

	cmds "github.com/TRON-US/go-btfs-cmds"
)

const storageLength = 365 * 10

var AddAndUploadCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline:          "add a file and upload to btfs",
		ShortDescription: "add a file and upload to btfs",
	},
	Arguments: []cmds.Argument{
		cmds.FileArg("path", true, true, "The path to a file to be added to btfs.").EnableRecursive().EnableStdin(),
	},
	Options: []cmds.Option{
		cmds.OptionRecursivePath, // a builtin option that allows recursive paths (-r, --recursive)
		cmds.OptionDerefArgs,     // a builtin option that resolves passed in filesystem links (--dereference-args)
		cmds.OptionStdinName,     // a builtin option that optionally allows wrapping stdin into a named file
		cmds.OptionHidden,
		cmds.OptionIgnore,
		cmds.OptionIgnoreRules,
		cmds.BoolOption(quietOptionName, "q", "Write minimal output."),
		cmds.BoolOption(quieterOptionName, "Q", "Write only final hash."),
		cmds.BoolOption(silentOptionName, "Write no output."),
		cmds.BoolOption(progressOptionName, "p", "Stream progress data."),
		cmds.BoolOption(trickleOptionName, "t", "Use trickle-dag format for dag generation."),
		cmds.BoolOption(onlyHashOptionName, "n", "Only chunk and hash - do not write to disk."),
		cmds.BoolOption(wrapOptionName, "w", "Wrap files with a directory object."),
		cmds.StringOption(chunkerOptionName, "s", "Chunking algorithm, size-[bytes], rabin-[min]-[avg]-[max], buzhash or reed-solomon-[#data]-[#parity]-[size]").WithDefault("size-262144"),
		cmds.BoolOption(pinOptionName, "Pin this object when adding.").WithDefault(true),
		cmds.BoolOption(rawLeavesOptionName, "Use raw blocks for leaf nodes. (experimental)"),
		cmds.BoolOption(noCopyOptionName, "Add the file using filestore. Implies raw-leaves. (experimental)"),
		cmds.BoolOption(fstoreCacheOptionName, "Check the filestore for pre-existing blocks. (experimental)"),
		cmds.IntOption(cidVersionOptionName, "CID version. Defaults to 0 unless an option that depends on CIDv1 is passed. (experimental)"),
		cmds.StringOption(hashOptionName, "Hash function to use. Implies CIDv1 if not sha2-256. (experimental)").WithDefault("sha2-256"),
		cmds.BoolOption(inlineOptionName, "Inline small blocks into CIDs. (experimental)"),
		cmds.IntOption(inlineLimitOptionName, "Maximum block size to inline. (experimental)").WithDefault(32),
		cmds.StringOption(tokenMetaOptionName, "m", "Token metadata in JSON string"),
		cmds.BoolOption(encryptName, "Encrypt the file."),
		cmds.StringOption(pubkeyName, "The public key to encrypt the file."),
		cmds.StringOption(peerIdName, "The peer id to encrypt the file."),
		cmds.IntOption(pinDurationCountOptionName, "d", "Duration for which the object is pinned in days.").WithDefault(0),
	},
	Run: func(request *cmds.Request, emitter cmds.ResponseEmitter, environment cmds.Environment) error {
		addReq := &*request
		addReq.Options[chunkerOptionName] = "reed-solomon"
		if err := AddCmd.Run(addReq, &nullEmitter{}, environment); err != nil {
			return err
		}
		hash := addReq.Context.Value(AddedFileHashKey)
		if hash == nil {
			return errors.New("add file to btfs failed")
		}

		uploadReq := &*request
		uploadReq.Arguments = []string{hash.(string)}
		uploadReq.Options = cmds.OptMap{}
		for _, opt := range upload.StorageUploadCmd.Options {
			uploadReq.Options[opt.Name()] = opt.Default()
		}
		uploadReq.Options[upload.StorageLengthOptionName] = storageLength
		if err := upload.StorageUploadCmd.Run(uploadReq, &nullEmitter{}, environment); err != nil {
			return err
		}
		sessionId := uploadReq.Context.Value(upload.UploadedSessionId)
		if sessionId == nil {
			return fmt.Errorf("upload failed, file-hash: %s", hash)
		}

		return cmds.EmitOnce(emitter, &AddUpload{
			Hash:      hash.(string),
			SessionId: sessionId.(string),
		})
	},
}

type AddUpload struct {
	Hash      string
	SessionId string
}

type nullEmitter struct{}

func (s *nullEmitter) Close() error                   { return nil }
func (s *nullEmitter) SetLength(_ uint64)             {}
func (s *nullEmitter) CloseWithError(err error) error { return nil }
func (s *nullEmitter) Emit(value interface{}) error   { return nil }
func (s *nullEmitter) RecordEvent(str string)         { return }
func (s *nullEmitter) ShowEventReport() string        { return "" }
