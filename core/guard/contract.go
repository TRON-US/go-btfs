package guard

import (
	"context"
	"fmt"
	"time"

	"github.com/TRON-US/go-btfs/core"
	"github.com/TRON-US/go-btfs/core/commands/storage"
	"github.com/TRON-US/go-btfs/core/escrow"

	"github.com/tron-us/go-btfs-common/crypto"
	escrowpb "github.com/tron-us/go-btfs-common/protos/escrow"
	guardpb "github.com/tron-us/go-btfs-common/protos/guard"

	config "github.com/TRON-US/go-btfs-config"
	"github.com/gogo/protobuf/proto"
	logging "github.com/ipfs/go-log"
	ic "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
)

var log = logging.Logger("core/guard")

func NewFileStatus(session *storage.FileContracts, contracts []*guardpb.Contract, configuration *config.Config) (*guardpb.FileStoreStatus, error) {
	guardPid, escrowPid, err := getGuardAndEscrowPid(configuration)
	if err != nil {
		return nil, err
	}
	var (
		rentStart   time.Time
		rentEnd     time.Time
		preparerPid = session.Renter.Pretty()
		renterPid   = session.Renter.Pretty()
		rentalState = guardpb.FileStoreStatus_NEW
	)
	if len(contracts) > 0 {
		rentStart = contracts[0].RentStart
		rentEnd = contracts[0].RentEnd
		preparerPid = contracts[0].PreparerPid
		renterPid = contracts[0].RenterPid
		if contracts[0].PreparerPid != contracts[0].RenterPid {
			rentalState = guardpb.FileStoreStatus_PARTIAL_NEW
		}
	}

	fileStoreMeta := guardpb.FileStoreMeta{
		RenterPid:        renterPid,
		FileHash:         session.FileHash.String(),
		FileSize:         session.GetFileSize(),
		RentStart:        rentStart,
		RentEnd:          rentEnd,
		CheckFrequency:   0,
		GuardFee:         0,
		EscrowFee:        0,
		ShardCount:       int32(len(contracts)),
		MinimumShards:    0,
		RecoverThreshold: 0,
		EscrowPid:        escrowPid.Pretty(),
		GuardPid:         guardPid.Pretty(),
	}

	return &guardpb.FileStoreStatus{
		FileStoreMeta:     fileStoreMeta,
		State:             0,
		Contracts:         contracts,
		RenterSignature:   nil,
		GuardReceiveTime:  time.Time{},
		ChangeLog:         nil,
		CurrentTime:       time.Now(),
		GuardSignature:    nil,
		RentalState:       rentalState,
		PreparerPid:       preparerPid,
		PreparerSignature: nil,
	}, nil
}

func NewContract(session *storage.FileContracts, configuration *config.Config, shardKey string, shardIndex int32,
	renterPid string) (*guardpb.ContractMeta, error) {
	shard := session.ShardInfo[shardKey]
	guardPid, escrowPid, err := getGuardAndEscrowPid(configuration)
	if err != nil {
		return nil, err
	}
	return &guardpb.ContractMeta{
		ContractId:    shard.ContractID,
		RenterPid:     renterPid,
		HostPid:       shard.Receiver.Pretty(),
		ShardHash:     shard.ShardHash.String(),
		ShardIndex:    shardIndex,
		ShardFileSize: shard.ShardSize,
		FileHash:      session.FileHash.String(),
		RentStart:     shard.StartTime,
		RentEnd:       shard.StartTime.Add(shard.ContractLength),
		GuardPid:      guardPid.Pretty(),
		EscrowPid:     escrowPid.Pretty(),
		Price:         shard.Price,
		Amount:        shard.TotalPay,
	}, nil
}

func SignedContractAndMarshal(meta *guardpb.ContractMeta, offlineSignedBytes []byte, cont *guardpb.Contract, privKey ic.PrivKey,
	isPayer bool, isRepair bool, renterPid string, nodePid string) ([]byte, error) {
	var signedBytes []byte
	var err error
	if offlineSignedBytes == nil {
		signedBytes, err = crypto.Sign(privKey, meta)
		if err != nil {
			return nil, err
		}
	} else {
		signedBytes = offlineSignedBytes
	}

	if cont == nil {
		cont = &guardpb.Contract{
			ContractMeta:   *meta,
			LastModifyTime: time.Now(),
		}
	} else {
		cont.LastModifyTime = time.Now()
	}
	if isPayer {
		cont.RenterPid = renterPid
		cont.PreparerPid = nodePid
		if isRepair {
			cont.PreparerSignature = signedBytes
		} else {
			cont.RenterSignature = signedBytes
		}
	} else {
		cont.HostSignature = signedBytes
	}
	return proto.Marshal(cont)
}

func UnmarshalGuardContract(marshaledBody []byte) (*guardpb.Contract, error) {
	signedContract := &guardpb.Contract{}
	err := proto.Unmarshal(marshaledBody, signedContract)
	if err != nil {
		return nil, err
	}
	return signedContract, nil
}

func getGuardAndEscrowPid(configuration *config.Config) (peer.ID, peer.ID, error) {
	escrowPubKeys := configuration.Services.EscrowPubKeys
	if len(escrowPubKeys) == 0 {
		return "", "", fmt.Errorf("missing escrow public key in config")
	}
	guardPubKeys := configuration.Services.GuardPubKeys
	if len(guardPubKeys) == 0 {
		return "", "", fmt.Errorf("missing guard public key in config")
	}
	escrowPid, err := pidFromString(escrowPubKeys[0])
	if err != nil {
		log.Error("parse escrow config failed", escrowPubKeys[0])
		return "", "", err
	}
	guardPid, err := pidFromString(guardPubKeys[0])
	if err != nil {
		log.Error("parse guard config failed", guardPubKeys[1])
		return "", "", err
	}
	return guardPid, escrowPid, err
}

func pidFromString(key string) (peer.ID, error) {
	pubKey, err := escrow.ConvertPubKeyFromString(key)
	if err != nil {
		return "", err
	}
	return peer.IDFromPublicKey(pubKey)
}

func PrepAndUploadFileMeta(ctx context.Context, ss *storage.FileContracts,
	escrowResults *escrowpb.SignedSubmitContractResult, payinRes *escrowpb.SignedPayinResult,
	payerPriKey ic.PrivKey, configuration *config.Config) (*guardpb.FileStoreStatus, error) {
	// TODO: talk with Jin for doing signature for every contract
	fileStatus, err := PrepareFileMetaHelper(ss, payinRes, configuration)
	if err != nil {
		return nil, err
	}

	sign, err := crypto.Sign(payerPriKey, &fileStatus.FileStoreMeta)
	if err != nil {
		return nil, err
	}

	return SubmitFileMetaHelper(ctx, configuration, fileStatus, sign)
}

func PrepareFileMetaHelper(ss *storage.FileContracts,
	payinRes *escrowpb.SignedPayinResult, configuration *config.Config) (*guardpb.FileStoreStatus, error) {
	// get escrow sig, add them to guard
	contracts := ss.GetGuardContracts()
	sig := payinRes.EscrowSignature
	for _, guardContract := range contracts {
		guardContract.EscrowSignature = sig
		guardContract.EscrowSignedTime = payinRes.Result.EscrowSignedTime
		guardContract.LastModifyTime = time.Now()
	}

	fileStatus, err := NewFileStatus(ss, contracts, configuration)
	if err != nil {
		return nil, err
	}
	return fileStatus, nil
}

func SubmitFileMetaHelper(ctx context.Context, configuration *config.Config,
	fileStatus *guardpb.FileStoreStatus, sign []byte) (*guardpb.FileStoreStatus, error) {
	if fileStatus.PreparerPid == fileStatus.RenterPid {
		fileStatus.RenterSignature = sign
	} else {
		fileStatus.RenterSignature = sign
		fileStatus.PreparerSignature = sign
	}

	err := SubmitFileStatus(ctx, configuration, fileStatus)
	if err != nil {
		return nil, err
	}

	return fileStatus, nil
}

// GetUpdatedGuardContracts retrieves updated guard contracts from remote based on latest timestamp
// and returns the list updated
func GetUpdatedGuardContracts(ctx context.Context, n *core.IpfsNode,
	lastUpdatedTime time.Time) ([]*guardpb.Contract, error) {
	now := time.Now()
	req := &guardpb.ListHostContractsRequest{
		HostPid:             n.Identity.Pretty(),
		RequesterPid:        n.Identity.Pretty(),
		RequestPageSize:     0, // FIXME Does it return all?
		RequestPageIndex:    0,
		LastModifyTimeSince: &lastUpdatedTime,
		State:               guardpb.ListHostContractsRequest_ALL,
		RequestTime:         &now,
	}
	signedReq, err := crypto.Sign(n.PrivateKey, req)
	if err != nil {
		return nil, err
	}
	req.Signature = signedReq

	cfg, err := n.Repo.Config()
	if err != nil {
		return nil, err
	}

	return ListHostContracts(ctx, cfg, req)
}
