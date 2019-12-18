package guard

import (
	"context"
	"fmt"
	"github.com/TRON-US/go-btfs/core/escrow"
	"time"

	"github.com/TRON-US/go-btfs/core/commands/storage"
	"github.com/tron-us/go-btfs-common/crypto"
	escrowPb "github.com/tron-us/go-btfs-common/protos/escrow"
	guardPb "github.com/tron-us/go-btfs-common/protos/guard"

	config "github.com/TRON-US/go-btfs-config"
	"github.com/gogo/protobuf/proto"
	logging "github.com/ipfs/go-log"
	ic "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
)

var log = logging.Logger("core/guard")

func NewFileStatus(session *storage.FileContracts, contracts []*guardPb.Contract, configuration *config.Config) (*guardPb.FileStoreStatus, error) {
	guardPid, escrowPid, err := getGuardAndEscrowPid(configuration)
	if err != nil {
		return nil, err
	}
	var rentStart time.Time
	var rentEnd time.Time
	if len(contracts) > 0 {
		rentStart = contracts[0].RentStart
		rentEnd = contracts[0].RentEnd
	}

	fileStoreMeta := guardPb.FileStoreMeta{
		RenterPid:        session.Renter.Pretty(),
		FileHash:         session.FileHash.String(), //TODO need to check
		FileSize:         10000,                     //TODO need to revise later
		RentStart:        rentStart,                 //TODO need to revise later
		RentEnd:          rentEnd,                   //TODO need to revise later
		CheckFrequency:   0,
		GuardFee:         0,
		EscrowFee:        0,
		ShardCount:       int32(len(contracts)),
		MinimumShards:    0,
		RecoverThreshold: 0,
		EscrowPid:        escrowPid.Pretty(),
		GuardPid:         guardPid.Pretty(),
	}

	return &guardPb.FileStoreStatus{
		FileStoreMeta:     fileStoreMeta,
		State:             0,
		Contracts:         contracts,
		RenterSignature:   nil,
		GuardReceiveTime:  time.Time{},
		ChangeLog:         nil,
		CurrentTime:       time.Now(),
		GuardSignature:    nil,
		RentalState:       0,
		PreparerPid:       "",
		PreparerSignature: nil,
	}, nil
}

func NewContract(session *storage.FileContracts, configuration *config.Config, shardKey string, shardIndex int32) (*guardPb.ContractMeta, error) {
	shard := session.ShardInfo[shardKey]
	guardPid, escrowPid, err := getGuardAndEscrowPid(configuration)
	if err != nil {
		return nil, err
	}
	return &guardPb.ContractMeta{
		ContractId:    shard.ContractID,
		RenterPid:     session.Renter.Pretty(),
		HostPid:       shard.Receiver.Pretty(),
		ShardHash:     shard.ShardHash,
		ShardIndex:    shardIndex,
		ShardFileSize: shard.ShardSize,
		FileHash:      session.FileHash.String(),
		RentStart:     shard.StartTime,
		RentEnd:       shard.StartTime.Add(shard.ContractLength),
		GuardPid:      guardPid.Pretty(),
		EscrowPid:     escrowPid.Pretty(),
		Price:         shard.Price,
		Amount:        shard.TotalPay, // TODO: CHANGE and aLL other optional fields

	}, nil
}

func SignedContractAndMarshal(meta *guardPb.ContractMeta, cont *guardPb.Contract, privKey ic.PrivKey, isPayer bool) ([]byte, error) {
	sig, err := crypto.Sign(privKey, meta)
	if err != nil {
		return nil, err
	}
	if cont == nil {
		cont = &guardPb.Contract{
			ContractMeta:   *meta,
			LastModifyTime: time.Now(),
		}
	} else {
		cont.LastModifyTime = time.Now()
	}
	if isPayer {
		cont.RenterSignature = sig
	} else {
		cont.HostSignature = sig
	}
	return proto.Marshal(cont)
}

func UnmarshalGuardContract(marshaledBody []byte) (*guardPb.Contract, error) {
	signedContract := &guardPb.Contract{}
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

// TODO: modify or change it all
//func NewFileStoreStatus(session *storage.FileContracts, endTime time.Time, configuration *config.Config) (*guardPb.FileStoreStatus, error) {
//
//	escrowPid, err := pidFromString(configuration.Services.EscrowPubKeys[0])
//	if err != nil {
//		return nil, err
//	}
//	guardPid, err := pidFromString(configuration.Services.GuardPubKeys[0])
//	if err != nil {
//		return nil, err
//	}
//	fileStoreMeta := guardPb.FileStoreMeta{
//		RenterPid:        session.Renter.Pretty(),
//		FileHash:         session.FileHash.KeyString(),
//		FileSize:         2000000000, // default??
//		RentStart:        time.Now(),
//		RentEnd:          endTime,
//		CheckFrequency:   0,
//		GuardFee:         0,
//		EscrowFee:        0,
//		ShardCount:       int32(len(session.ShardInfo)),
//		MinimumShards:    10,
//		RecoverThreshold: 20,
//		EscrowPid:        escrowPid.Pretty(),
//		GuardPid:         guardPid.Pretty(),
//	}
//
//}

func pidFromString(key string) (peer.ID, error) {
	pubKey, err := escrow.ConvertPubKeyFromString(key)
	if err != nil {
		return "", err
	}
	return peer.IDFromPublicKey(pubKey)
}

func PrepAndUploadFileMeta(ctx context.Context, ss *storage.FileContracts,
	escrowResults *escrowPb.SignedSubmitContractResult, payinRes *escrowPb.SignedPayinResult,
	payerPriKey ic.PrivKey, configuration *config.Config) (*guardPb.FileStoreStatus, error) {
	// TODO: talk with Jin for doing signature for every contract
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

	fileStatus.RenterSignature, err = crypto.Sign(payerPriKey, &fileStatus.FileStoreMeta)
	if err != nil {
		return nil, err
	}

	err = submitFileStatus(ctx, configuration, fileStatus)
	if err != nil {
		return nil, err
	}

	return fileStatus, nil
}
