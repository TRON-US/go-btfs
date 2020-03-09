package guard

import (
	"context"
	"fmt"
	config "github.com/TRON-US/go-btfs-config"
	"github.com/TRON-US/go-btfs/core/escrow"
	"github.com/ethereum/go-ethereum/log"
	ic "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/tron-us/go-btfs-common/crypto"
	escrowPb "github.com/tron-us/go-btfs-common/protos/escrow"
	guardPb "github.com/tron-us/go-btfs-common/protos/guard"
	"github.com/tron-us/go-btfs-common/protos/storage/shard"
	"github.com/tron-us/protobuf/proto"
	"time"
)

func PrepAndUploadFileMeta(ctx context.Context, contracts []*guardPb.Contract, payinRes *escrowPb.SignedPayinResult,
	payerPriKey ic.PrivKey, configuration *config.Config, renterId string, fileHash string) (*guardPb.FileStoreStatus,
	error) {
	sig := payinRes.EscrowSignature
	for _, guardContract := range contracts {
		guardContract.EscrowSignature = sig
		guardContract.EscrowSignedTime = payinRes.Result.EscrowSignedTime
		guardContract.LastModifyTime = time.Now()
	}

	fileStatus, err := NewFileStatus(contracts, configuration, renterId, fileHash)
	if err != nil {
		return nil, err
	}

	sign, err := crypto.Sign(payerPriKey, &fileStatus.FileStoreMeta)
	if err != nil {
		return nil, err
	}
	if fileStatus.PreparerPid == fileStatus.RenterPid {
		fileStatus.RenterSignature = sign
	} else {
		fileStatus.RenterSignature = sign
		fileStatus.PreparerSignature = sign
	}

	err = submitFileStatus(ctx, configuration, fileStatus)
	if err != nil {
		return nil, err
	}

	return fileStatus, nil
}

func NewFileStatus(contracts []*guardPb.Contract, configuration *config.Config,
	renterId string, fileHash string) (*guardPb.FileStoreStatus, error) {
	guardPid, escrowPid, err := getGuardAndEscrowPid(configuration)
	if err != nil {
		return nil, err
	}
	var (
		rentStart   time.Time
		rentEnd     time.Time
		preparerPid = renterId
		renterPid   = renterId
		rentalState = guardPb.FileStoreStatus_NEW
	)
	if len(contracts) > 0 {
		rentStart = contracts[0].RentStart
		rentEnd = contracts[0].RentEnd
		preparerPid = contracts[0].PreparerPid
		renterPid = contracts[0].RenterPid
		if contracts[0].PreparerPid != contracts[0].RenterPid {
			rentalState = guardPb.FileStoreStatus_PARTIAL_NEW
		}
	}

	fileStoreMeta := guardPb.FileStoreMeta{
		RenterPid:        renterPid,
		FileHash:         fileHash,  //TODO need to check
		FileSize:         10000,     //TODO need to revise later
		RentStart:        rentStart, //TODO need to revise later
		RentEnd:          rentEnd,   //TODO need to revise later
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
		RentalState:       rentalState,
		PreparerPid:       preparerPid,
		PreparerSignature: nil,
	}, nil
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

func SignedContractAndMarshal(meta *guardPb.ContractMeta, cont *guardPb.Contract, privKey ic.PrivKey,
	isPayer bool, isRepair bool, renterPid string, nodePid string) ([]byte, error) {
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
		cont.RenterPid = renterPid
		cont.PreparerPid = nodePid
		if isRepair {
			cont.PreparerSignature = sig
		} else {
			cont.RenterSignature = sig
		}
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

func NewContract(md *shard.Metadata, shardHash string, configuration *config.Config,
	renterPid string) (*guardPb.ContractMeta, error) {
	guardPid, escrowPid, err := getGuardAndEscrowPid(configuration)
	if err != nil {
		return nil, err
	}
	return &guardPb.ContractMeta{
		ContractId:    md.ContractId,
		RenterPid:     renterPid,
		HostPid:       md.Receiver,
		ShardHash:     shardHash,
		ShardIndex:    md.Index,
		ShardFileSize: md.ShardFileSize,
		FileHash:      md.FileHash,
		RentStart:     md.StartTime,
		RentEnd:       md.StartTime.Add(md.ContractLength),
		GuardPid:      guardPid.Pretty(),
		EscrowPid:     escrowPid.Pretty(),
		Price:         md.Price,
		Amount:        md.TotalPay, // TODO: CHANGE and aLL other optional fields
	}, nil
}
