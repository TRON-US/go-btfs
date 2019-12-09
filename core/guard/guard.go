package guard

import (
	"time"

	config "github.com/TRON-US/go-btfs-config"
	"github.com/TRON-US/go-btfs/core/commands/storage"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/tron-us/go-btfs-common/crypto"
	guardPb "github.com/tron-us/go-btfs-common/protos/guard"
)

func NewContractMeta(session *storage.FileContracts, configuration *config.Config, chunkHash string, chunkIndex int32) (*guardPb.ContractMeta, error) {
	shard := session.ShardInfo[chunkHash]
	guardPid, escrowPid, err := getGuardAndEscrowPid(configuration)
	if err != nil {
		return nil, err
	}
	return &guardPb.ContractMeta{
		ContractId:       shard.ContractID,
		RenterPid:        session.Renter.Pretty(),
		HostPid:          shard.Receiver.Pretty(),
		ShardHash:        chunkHash,
		ShardIndex:       chunkIndex,
		FileHash:         session.FileHash.KeyString(),
		RentStart:        shard.StartTime,
		RentEnd:          shard.StartTime.Add(shard.Length),
		GuardPid:         guardPid.Pretty(),
		EscrowPid:        escrowPid.Pretty(),
		Price:            shard.Price,
		Amount:           1000,
		CollateralAmount: 0,
		PayoutSchedule:   0,
		NumPayouts:       5,
	}, nil
}

func getGuardAndEscrowPid(configuration *config.Config) (peer.ID, peer.ID, error) {
	escrowPid, err := pidFromString(configuration.Services.EscrowPubKeys[0])
	if err != nil {
		return "", "", err
	}
	guardPid, err := pidFromString(configuration.Services.GuardPubKeys[0])
	if err != nil {
		return "", "", err
	}
	return guardPid, escrowPid, err
}

// Todo: modify or change it all
func NewFileStoreStatus(session *storage.FileContracts, endTime time.Time, configuration *config.Config) (*guardPb.FileStoreStatus, error) {

	escrowPid, err := pidFromString(configuration.Services.EscrowPubKeys[0])
	if err != nil {
		return nil, err
	}
	guardPid, err := pidFromString(configuration.Services.GuardPubKeys[0])
	if err != nil {
		return nil, err
	}
	fileStoreMeta := guardPb.FileStoreMeta{
		RenterPid:        session.Renter.Pretty(),
		FileHash:         session.FileHash.KeyString(),
		FileSize:         2000000000, // default??
		RentStart:        time.Now(),
		RentEnd:          endTime,
		CheckFrequency:   0,
		GuardFee:         0,
		EscrowFee:        0,
		ShardCount: int32(len(session.ShardInfo)),
		MinimumShards:    10,
		RecoverThreshold: 20,
		EscrowPid:        escrowPid.Pretty(),
		GuardPid:         guardPid.Pretty(),
	}

}

func pidFromString(key string) (peer.ID, error) {
	pubKey, err := crypto.ToPubKey(key)
	if err != nil {
		return "", err
	}
	return peer.IDFromPublicKey(pubKey)
}
