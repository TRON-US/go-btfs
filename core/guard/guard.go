package guard

import (
	config "github.com/TRON-US/go-btfs-config"
	"github.com/TRON-US/go-btfs/core/commands/storage"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/tron-us/go-btfs-common/crypto"
	guardPb "github.com/tron-us/go-btfs-common/protos/guard"
	"time"
)

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
		FileHash:         session.FileHash,
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
