package commands

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	cmds "github.com/TRON-US/go-btfs-cmds"
	config "github.com/TRON-US/go-btfs-config"
	"github.com/TRON-US/go-btfs/core"
	"github.com/TRON-US/go-btfs/core/commands/cmdenv"
	"github.com/TRON-US/go-btfs/core/commands/storage"
	"github.com/TRON-US/go-btfs/core/escrow"
	guard "github.com/TRON-US/go-btfs/core/guard"
	escrowpb "github.com/tron-us/go-btfs-common/protos/escrow"
	guardpb "github.com/tron-us/go-btfs-common/protos/guard"
	ledgerpb "github.com/tron-us/go-btfs-common/protos/ledger"

	"github.com/gogo/protobuf/proto"
	ic "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
)

const (
	BalanceOffSignOperation    = "balance"
	PayChannelOffSignOperation = "paychannel"
	PayRequestOffSignOperation = "payrequest"
	GuardOffSignOperation      = "guard"
)

func VerifySessionSignature(offSignRenterPid peer.ID, data string, sessionSigStr string) error {
	// get renter's public key
	pubKey, err := offSignRenterPid.ExtractPublicKey()
	if err != nil {
		return err
	}

	sigBytes, err := stringToBytes(sessionSigStr, Base64)
	if err != nil {
		return err
	}
	ok, err := pubKey.Verify([]byte(data), sigBytes)
	if !ok || err != nil {
		return fmt.Errorf("cannot verify session signature: %v", err)
	}
	return nil
}

func prepareSignedContractsForShardOffSign(param *paramsForPrepareContractsForShard,
	candidateHost *storage.HostNode, initialCall bool) error {
	shard := param.shard
	shard.CandidateHost = candidateHost
	ss := param.ss

	err := prepareSignedContractsForEscrowOffSign(param, !initialCall)
	if err != nil {
		return err
	}

	err = prepareSignedGuardContractForShardOffSign(param.ctx, ss, shard, param.n, !initialCall)
	if err != nil {
		return err
	}

	err = buildSignedGuardContractForShardOffSign(param.ctx, ss, shard, param.n, ss.RunMode,
		ss.OfflineCB.OfflinePeerID.Pretty(), !initialCall)
	if err != nil {
		return err
	}
	return nil
}

func BalanceWithOffSign(ctx context.Context, configuration *config.Config, ss *storage.FileContracts) (int64, error) {
	signedStr, err := PerformBalanceOffSign(ctx, ss)
	if err != nil {
		return 0, err
	}

	signedBytes, err := stringToBytes(signedStr, Base64)
	if err != nil {
		return 0, err
	}

	return escrow.BalanceHelper(ctx, configuration, true, signedBytes, nil)
}

type GetContractBatchRes struct {
	Contracts []*storage.Contract
}

const (
	Text = iota + 1
	Base64
)

var storageUploadGetContractBatchCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Get all the contracts from the upload session	(From BTFS SDK application's perspective).",
		ShortDescription: `
This command (on client) reads the unsigned contracts and returns 
the contracts to the caller.`,
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("session-id", true, false, "ID for the entire storage upload session."),
		cmds.StringArg("peer-id", true, false, "Offline signs needed for this particular client."),
		cmds.StringArg("nonce-timestamp", true, false, "Nonce timestamp string for this offline signing."),
		cmds.StringArg("upload-session-signature", true, false, "Private key-signed string of <peer-id>:<nonce-timstamp>"),
		cmds.StringArg("session-status", true, false, "Current upload session status."),
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		batchRes := &GetContractBatchRes{}
		cfg, err := cmdenv.GetConfig(env)
		if err != nil {
			return err
		}
		if !cfg.Experimental.StorageClientEnabled {
			return fmt.Errorf("storage client api not enabled")
		}

		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}

		ssID := req.Arguments[0]
		ss, err := storage.GlobalSession.GetSession(n, storage.RenterStoragePrefix, ssID)
		if err != nil {
			return err
		}

		status, _ := ss.GetStatus(ss.GetCurrentStatus())
		if req.Arguments[4] != status {
			return errors.New("unexpected session status from SDK during communication in offline signing")
		}

		// Get relevant contracts from ss and ss.ShardInfo
		contracts := make([]*storage.Contract, len(ss.ShardInfo))
		index := 0
		for k, shard := range ss.ShardInfo {
			c := &storage.Contract{
				Key: k,
			}
			// Set `c.Contract` according to the session status
			switch status {
			case storage.StdSessionStateFlow[storage.InitSignReadyForEscrowStatus].State:
				c.ContractData, err = MarshalAndStringifyForSign(shard.UnsignedEscrowContract)
				if err != nil {
					return err
				}
			case storage.StdSessionStateFlow[storage.InitSignReadyForGuardStatus].State:
				c.ContractData, err = MarshalAndStringifyForSign(shard.UnsignedGuardContract)
				if err != nil {
					return err
				}
			default:
				return fmt.Errorf("unexpected session status %s in renter node", status)
			}
			contracts[index] = c
			index++
		}

		batchRes.Contracts = contracts
		// Change the status to the next to prevent another call of this endponnt by SDK library
		ss.UpdateSessionStatus(ss.GetCurrentStatus(), true, nil)

		return res.Emit(batchRes)
	},
	Type: GetContractBatchRes{},
}

func MarshalAndStringifyForSign(message proto.Message) (string, error) {
	raw, err := proto.Marshal(message)
	if err != nil {
		return "", err
	}
	str, err := bytesToString(raw, Base64)
	if err != nil {
		return "", err
	}
	return str, nil
}

var storageUploadSignContractBatchCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Get the unsigned contracts from the upload session.",
		ShortDescription: `
This command reads all the unsigned contracts from the upload session 
(From BTFS SDK application's perspective) and returns the contracts to the caller.`,
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("session-id", true, false, "ID for the entire storage upload session."),
		cmds.StringArg("peer-id", true, false, "Offline signs needed for this particular client."),
		cmds.StringArg("nonce-timestamp", true, false, "Nonce timestamp string for this offline signing."),
		cmds.StringArg("upload-session-signature", true, false, "Private key-signed string of peer-id:nonce-timestamp"),
		cmds.StringArg("session-status", true, false, "current upload session status."),
		cmds.StringArg("signed-data-items", true, false, "signed data items."),
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		cfg, err := cmdenv.GetConfig(env)
		if err != nil {
			return err
		}
		if !cfg.Experimental.StorageClientEnabled {
			return fmt.Errorf("storage client api not enabled")
		}

		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}

		ssID := req.Arguments[0]
		ss, err := storage.GlobalSession.GetSession(n, storage.RenterStoragePrefix, ssID)
		if err != nil {
			return err
		}
		err = verifyReceivedMessage(req, ss)
		if err != nil {
			return err
		}

		// Set all the `shard.SignedBytes` from received signed data items.
		var signedContracts []storage.Contract
		signedContractsString := req.Arguments[5]
		err = json.Unmarshal([]byte(signedContractsString), &signedContracts)
		if err != nil {
			return err
		}
		if len(signedContracts) != len(ss.ShardInfo) {
			return fmt.Errorf("number of received signed data items %d does not match number of shards %d",
				len(signedContracts), len(ss.ShardInfo))
		}
		for i := 0; i < len(ss.ShardInfo); i++ {
			k := signedContracts[i].Key
			shard, found := ss.ShardInfo[k]
			if !found {
				return fmt.Errorf("can not find an entry for key %s from ShardInfo map", k)
			}
			by, err := stringToBytes(signedContracts[i].ContractData, Base64)
			if err != nil {
				return err
			}
			shard.SignedBytes = by
		}

		// Broadcast
		currentStatus := ss.GetCurrentStatus()
		switch currentStatus {
		case storage.InitSignProcessForEscrowStatus:
			close(ss.OfflineCB.OfflineSignEscrowChan)
		case storage.InitSignProcessForGuardStatus:
			close(ss.OfflineCB.OfflineSignGuardChan)
		default:
			return fmt.Errorf("unexpected session status %d", currentStatus)
		}

		return nil
	},
}

func verifyReceivedMessage(req *cmds.Request, ss *storage.FileContracts) error {
	offlinePeerID, err := peer.IDB58Decode(req.Arguments[1])
	if err != nil {
		return err
	}
	if ss.OfflineCB.OfflinePeerID != offlinePeerID {
		return errors.New("peerIDs do not match")
	}
	offlineNonceTimestamp, err := strconv.ParseUint(req.Arguments[2], 10, 64)
	if err != nil {
		return err
	}
	if ss.OfflineCB.OfflineNonceTimestamp != offlineNonceTimestamp {
		return errors.New("Nonce timestamps do not match")
	}
	offlineSessionSignature := req.Arguments[3]
	if ss.OfflineCB.OfflineSessionSignature != offlineSessionSignature {
		return errors.New("Session signature do not match")
	}
	return nil
}

var storageUploadGetUnsignedCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Get the input data for offline signing.",
		ShortDescription: `
This command obtains the offline signing input data for from the upload session 
(From BTFS SDK application's perspective) and returns to the caller.`,
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("session-id", true, false, "ID for the entire storage upload session."),
		cmds.StringArg("peer-id", true, false, "Offline signs needed for this particular client."),
		cmds.StringArg("nonce-timestamp", true, false, "Nonce timestamp string for this offline signing."),
		cmds.StringArg("upload-session-signature", true, false, "Private key-signed string of peer-id:nonce-timestamp"),
		cmds.StringArg("session-status", true, false, "current upload session status."),
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		cfg, err := cmdenv.GetConfig(env)
		if err != nil {
			return err
		}
		if !cfg.Experimental.StorageClientEnabled {
			return fmt.Errorf("storage client api not enabled")
		}

		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}

		ssID := req.Arguments[0]
		ss, err := storage.GlobalSession.GetSession(n, storage.RenterStoragePrefix, ssID)
		if err != nil {
			return err
		}
		err = verifyReceivedMessage(req, ss)
		if err != nil {
			return err
		}

		status, _ := ss.GetStatus(ss.GetCurrentStatus())
		if req.Arguments[4] != status {
			return errors.New("unexpected session status from SDK during communication in offline signing")
		}

		// Obtain relevant operation info from ss and ss.ShardInfo
		unsignedRes := ss.OfflineCB.OfflineUnsigned

		// Change the status to the next to prevent another call of this endponnt by SDK library
		currentSessionStatus := ss.GetCurrentStatus()
		ss.SendSessionStatusChanPerMode(currentSessionStatus, true, nil)

		return res.Emit(unsignedRes)
	},
	Type: storage.GetUnsignedRes{},
}

var storageUploadSignCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Return the signed data to the upload session.",
		ShortDescription: `
This command returns the signed data (From BTFS SDK application's perspective)
to the upload session.`,
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("session-id", true, false, "ID for the entire storage upload session."),
		cmds.StringArg("peer-id", true, false, "Offline signs needed for this particular client."),
		cmds.StringArg("nonce-timestamp", true, false, "Nonce timestamp string for this offline signing."),
		cmds.StringArg("upload-session-signature", true, false, "Private key-signed string of peer-id:nonce-timestamp"),
		cmds.StringArg("session-status", true, false, "current upload session status."),
		cmds.StringArg("signed", true, false, "signed json data."),
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		cfg, err := cmdenv.GetConfig(env)
		if err != nil {
			return err
		}
		if !cfg.Experimental.StorageClientEnabled {
			return fmt.Errorf("storage client api not enabled")
		}

		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}

		ssID := req.Arguments[0]
		ss, err := storage.GlobalSession.GetSession(n, storage.RenterStoragePrefix, ssID)
		if err != nil {
			return err
		}
		err = verifyReceivedMessage(req, ss)
		if err != nil {
			return err
		}

		// Set the received `signed`.
		if ss.OfflineCB == nil {
			return errors.New("offline control block is nil")
		}
		ss.OfflineCB.OfflineSigned = req.Arguments[4]

		// Broadcast
		close(ss.OfflineCB.OfflinePaySignChan)

		return nil
	},
}

// helpers for upload in offline signing

// prepareSignedContractsForEscrowOffSign gets a valid host and
// prepares unsigned escrow contract and unsigned quard contract.
// Moves the session status to `InitSignReadyForEscrowStatus`.
func prepareSignedContractsForEscrowOffSign(param *paramsForPrepareContractsForShard, retryCalling bool) error {
	ss := param.ss
	shard := param.shard

	escrowContract, guardContractMeta, err := buildContractsForShard(param, shard.CandidateHost)
	if err != nil {
		return err
	}

	// Output for this function is set here
	shard.UnsignedEscrowContract = escrowContract
	shard.UnsignedGuardContract = guardContractMeta

	if !retryCalling {
		// Change the session status to `InitSignReadyForEscrowStatus`.
		isLast, err := ss.IncrementAndCompareOffSignReadyShards(len(ss.ShardInfo))
		if err != nil {
			return err
		}
		if isLast {
			currentStatus := ss.GetCurrentStatus()
			if currentStatus != storage.UninitializedStatus {
				return fmt.Errorf("current status %d does not match expected UninitializedStatus", currentStatus)
			}
			// Reset variables for the next offline signing for the session
			ss.SetOffSignReadyShards(0)
			ss.UpdateSessionStatus(currentStatus, true, nil) // call this since the current session status is before "initStatus"
		}
	} else {
		// Build a Contract and offer to the OffSignQueue

		// Change shard status
		// TODO: steve Do at the next .. Change offlineSigningStatus to ready
	}

	return nil
}

func resetPaySignChannel(ss *storage.FileContracts) error {
	if ss.OfflineCB == nil {
		return errors.New("offline control block is nil")
	}
	ss.OfflineCB.OfflinePaySignChan = nil
	ss.OfflineCB.OfflinePaySignChan = make(chan string)

	return nil
}

// prepareSignedGuardContractForShardOffSign waits for broadcast signal and
// moves the session status to `InitSignReadyForGuardStatus`
func prepareSignedGuardContractForShardOffSign(ctx context.Context, ss *storage.FileContracts, shard *storage.Shard, n *core.IpfsNode, retryCalling bool) error {
	// "/storage/upload/getcontractbatch" and "/storage/upload/signedbatch" handlers perform responses
	// to SDK application's requests and sets each `shard.HalfSignedEscrowContract` with signed bytes.
	// The corresponding endpoint for `signedbatch` closes "ss.OfflineSignChan" to broadcast
	// Here we wait for the broadcast signal.
	select {
	case <-ss.OfflineCB.OfflineSignEscrowChan:
	case <-ctx.Done():
		return ctx.Err()
	}
	var err error
	shard.HalfSignedEscrowContract, err = escrow.SignContractAndMarshalOffSign(shard.UnsignedEscrowContract, shard.SignedBytes, nil, true)
	if err != nil {
		log.Error("sign escrow contract and maorshal failed ")
		return err
	}

	// Output for this function is set here
	//shard.HalfSignedEscrowContract = halfSignedEscrowContract
	isLast, err := ss.IncrementAndCompareOffSignReadyShards(len(ss.ShardInfo))
	if err != nil {
		return err
	}
	if isLast {
		ss.SetOffSignReadyShards(0)
		currentStatus := ss.GetCurrentStatus()
		if currentStatus != storage.InitSignProcessForEscrowStatus {
			return fmt.Errorf("current status %d does not match expected InitSignProcessForEscrowStatus", currentStatus)
		}
		ss.UpdateSessionStatus(currentStatus, true, nil) // call this instead of SendSessionStatusChan() as it is before "initStatus"
	}
	return nil
}

// buildSignedGuardContractForShardOffSign waits for broadcast signal,
// builds halfSignedEscrowContract and moves the session status to `InitSignStatus`
// moves the session status to `InitSignStatus`
func buildSignedGuardContractForShardOffSign(ctx context.Context, ss *storage.FileContracts,
	shard *storage.Shard, n *core.IpfsNode, runMode int, renterPid string, retryCalling bool) error {
	// Wait for the broadcast signal by "/storage/upload/signedbatch" handler.
	select {
	case <-ss.OfflineCB.OfflineSignGuardChan:
	case <-ctx.Done():
		return ctx.Err()
	}
	var err error
	shard.HalfSignedGuardContract, err =
		guard.SignedContractAndMarshal(shard.UnsignedGuardContract, shard.SignedBytes, nil, nil, true,
			runMode == storage.RepairMode, renterPid, n.Identity.Pretty())
	if err != nil {
		log.Error("sign guard contract and maorshal failed ")
		return err
	}

	// moves the session status to `InitSignStatus`
	isLast, err := ss.IncrementAndCompareOffSignReadyShards(len(ss.ShardInfo))
	if err != nil {
		return err
	}
	if isLast {
		ss.SetOffSignReadyShards(0)
		currentStatus := ss.GetCurrentStatus()
		if currentStatus != storage.InitSignProcessForGuardStatus {
			return fmt.Errorf("current status %d does not match expected InitSignProcessForGuardStatus", currentStatus)
		}
		ss.UpdateSessionStatus(currentStatus, true, nil) // call this instead of SendSessinStatusChan() as it is before "initStatus"
		close(ss.OfflineCB.OfflineInitSigDoneChan)
	} else {
		// Wait for the broadcast signal by the last goroutine
		select {
		case <-ss.OfflineCB.OfflineInitSigDoneChan:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

// PerformBalanceOffSign moves the session status to
// `BalanceSignReadyStatus` and get and return balance signed bytes
func PerformBalanceOffSign(ctx context.Context, ss *storage.FileContracts) (string, error) {
	ss.ResetOfflineUnsigned()
	ss.OfflineCB.OfflineUnsigned.Opcode = BalanceOffSignOperation
	currentStatus := ss.GetCurrentStatus()
	if currentStatus != storage.SubmitStatus {
		return "", fmt.Errorf("current status %d does not match expected SubmitStatus", currentStatus)
	}
	ss.SendSessionStatusChanPerMode(currentStatus, true, nil)
	// Wait for the signal that indicates signed bytes are received.
	select {
	case <-ss.OfflineCB.OfflinePaySignChan:
	case <-ctx.Done():
		return "", ctx.Err()
	}

	resetPaySignChannel(ss)
	return ss.OfflineCB.OfflineSigned, nil
}

// PerformPayChannelOffSign set up `ss.OfflineUnsigned`,
// moves the session status to `PayChannelSignReadyStatus`,
// and get and return pay channel signed bytes
func PerformPayChannelOffSign(ctx context.Context, ss *storage.FileContracts, escrowPubKey []byte, totalPrice int64) (string, error) {
	ss.ResetOfflineUnsigned()
	ss.OfflineCB.OfflineUnsigned.Opcode = PayChannelOffSignOperation
	var err error
	ss.OfflineCB.OfflineUnsigned.Unsigned, err = bytesToString(escrowPubKey, Base64)
	if err != nil {
		return "", err
	}
	ss.OfflineCB.OfflineUnsigned.Price = totalPrice
	currentStatus := ss.GetCurrentStatus()
	if currentStatus != storage.BalanceSignProcessStatus {
		return "", fmt.Errorf("current status %d does not match expected BalanceSignProcessStatus", currentStatus)
	}
	ss.SendSessionStatusChanPerMode(currentStatus, true, nil)
	// Wait for the signal that indicates signed bytes are received.
	select {
	case <-ss.OfflineCB.OfflinePaySignChan:
	case <-ctx.Done():
		return "", ctx.Err()
	}

	resetPaySignChannel(ss)
	return ss.OfflineCB.OfflineSigned, nil
}

func PerformPayinRequestOffSign(ctx context.Context, ss *storage.FileContracts, result *escrowpb.SignedSubmitContractResult) ([]byte, error) {
	ss.ResetOfflineUnsigned()
	ss.OfflineCB.OfflineUnsigned.Opcode = PayRequestOffSignOperation
	b, err := proto.Marshal(result)
	if err != nil {
		return nil, err
	}
	ss.OfflineCB.OfflineUnsigned.Unsigned, err = bytesToString(b, Base64)
	if err != nil {
		return nil, err
	}

	currentStatus := ss.GetCurrentStatus()
	if currentStatus != storage.PayChannelStatus {
		return nil, fmt.Errorf("current status %d does not match expected PayChannelStatus", currentStatus)
	}
	ss.SendSessionStatusChanPerMode(currentStatus, true, nil)

	return waitAndGetOfflineSigned(ss, ctx)
}

func PerformGuardFileMetaOffSign(ctx context.Context, ss *storage.FileContracts, meta *guardpb.FileStoreMeta) ([]byte, error) {
	ss.ResetOfflineUnsigned()
	ss.OfflineCB.OfflineUnsigned.Opcode = GuardOffSignOperation
	b, err := proto.Marshal(meta)
	if err != nil {
		return nil, err
	}
	ss.OfflineCB.OfflineUnsigned.Unsigned, err = bytesToString(b, Base64)
	if err != nil {
		return nil, err
	}

	currentStatus := ss.GetCurrentStatus()
	if currentStatus != storage.PayStatus {
		return nil, fmt.Errorf("current status %d does not match expected PayStatus", currentStatus)
	}
	ss.SendSessionStatusChanPerMode(currentStatus, true, nil)

	return waitAndGetOfflineSigned(ss, ctx)
}

func waitAndGetOfflineSigned(ss *storage.FileContracts, ctx context.Context) ([]byte, error) {
	// Wait for the signal that indicates signed bytes are received.
	select {
	case <-ss.OfflineCB.OfflinePaySignChan:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	resetPaySignChannel(ss)
	by, err := stringToBytes(ss.OfflineCB.OfflineSigned, Base64)
	if err != nil {
		return nil, err
	}
	return by, nil
}

func NewContractRequestOffSign(ctx context.Context, configuration *config.Config, ss *storage.FileContracts,
	signedContracts []*escrowpb.SignedEscrowContract, totalPrice int64) (*escrowpb.EscrowContractRequest, error) {
	// prepare channel commit
	var escrowPubKey ic.PubKey
	escrowPubKey, err := escrow.NewContractRequestHelper(configuration)
	if err != nil {
		return nil, err
	}
	pubK, err := ic.MarshalPublicKey(escrowPubKey)
	if err != nil {
		return nil, err
	}

	signedStr, err := PerformPayChannelOffSign(ctx, ss, pubK, totalPrice) // TODO: escrowAddress -> escrowPubKey
	if err != nil {
		return nil, err
	}
	signedBytes, err := stringToBytes(signedStr, Base64)
	if err != nil {
		return nil, err
	}
	var signedChannelCommit ledgerpb.SignedChannelCommit
	err = proto.Unmarshal(signedBytes, &signedChannelCommit)
	if err != nil {
		return nil, err
	}
	return &escrowpb.EscrowContractRequest{
		Contract:     signedContracts,
		BuyerChannel: &signedChannelCommit,
	}, nil
}

func NewPayinRequestOffSign(ctx context.Context, ss *storage.FileContracts,
	result *escrowpb.SignedSubmitContractResult) (*escrowpb.SignedPayinRequest, error) {
	signed, err := PerformPayinRequestOffSign(ctx, ss, result)
	if err != nil {
		return nil, err
	}

	signedPayInRequest := new(escrowpb.SignedPayinRequest)
	err = proto.Unmarshal(signed, signedPayInRequest)
	if err != nil {
		return nil, err
	}

	return signedPayInRequest, nil
}

func PrepAndUploadFileMetaOffSign(ctx context.Context, ss *storage.FileContracts,
	escrowResults *escrowpb.SignedSubmitContractResult, payinRes *escrowpb.SignedPayinResult,
	configuration *config.Config) (*guardpb.FileStoreStatus, error) {
	fileStatus, err := guard.PrepareFileMetaHelper(ss, payinRes, configuration)
	if err != nil {
		return nil, err
	}

	signed, err := PerformGuardFileMetaOffSign(ctx, ss, &fileStatus.FileStoreMeta)
	if err != nil {
		return nil, err
	}

	return guard.SubmitFileMetaHelper(ctx, configuration, fileStatus, signed)
}

func bytesToString(data []byte, encoding int) (string, error) {
	switch encoding {
	case Text:
		return string(data), nil
	case Base64:
		return base64.StdEncoding.EncodeToString(data), nil
	default:
		return "", fmt.Errorf(`unexpected parameter [%d] is given, either "text" or "base64" should be used`, encoding)
	}
}

func stringToBytes(str string, encoding int) ([]byte, error) {
	switch encoding {
	case Text:
		return []byte(str), nil
	case Base64:
		by, err := base64.StdEncoding.DecodeString(str)
		if err != nil {
			return nil, err
		}
		return by, nil
	default:
		return nil, fmt.Errorf(`unexpected encoding [%d], expected 1(Text) or 2(Base64)`, encoding)
	}
}
