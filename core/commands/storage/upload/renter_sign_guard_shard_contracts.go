package upload

import (
	"fmt"
	"time"

	config "github.com/TRON-US/go-btfs-config"
	"github.com/tron-us/go-btfs-common/crypto"
	guardpb "github.com/tron-us/go-btfs-common/protos/guard"
	"github.com/tron-us/protobuf/proto"

	"github.com/libp2p/go-libp2p-core/peer"
	cmap "github.com/orcaman/concurrent-map"
)

var (
	guardChanMaps     = cmap.New()
	guardContractMaps = cmap.New()
)

type ContractParams struct {
	ContractId    string
	RenterPid     string
	HostPid       string
	ShardIndex    int32
	ShardHash     string
	ShardSize     int64
	FileHash      string
	StartTime     time.Time
	StorageLength int64
	Price         int64
	TotalPay      int64
}

func renterSignGuardContract(rss *RenterSession, params *ContractParams, offlineSigning bool) ([]byte, error) {
	guardPid, escrowPid, err := getGuardAndEscrowPid(rss.ctxParams.cfg)
	if err != nil {
		return nil, err
	}
	gm := &guardpb.ContractMeta{
		ContractId:    params.ContractId,
		RenterPid:     params.RenterPid,
		HostPid:       params.HostPid,
		ShardHash:     params.ShardHash,
		ShardIndex:    params.ShardIndex,
		ShardFileSize: params.ShardSize,
		FileHash:      params.FileHash,
		RentStart:     params.StartTime,
		RentEnd:       params.StartTime.Add(time.Duration(params.StorageLength*24) * time.Hour),
		GuardPid:      guardPid.Pretty(),
		EscrowPid:     escrowPid.Pretty(),
		Price:         params.Price,
		Amount:        params.TotalPay,
	}
	cont := &guardpb.Contract{
		ContractMeta:   *gm,
		LastModifyTime: time.Now(),
	}
	cont.RenterPid = rss.ctxParams.n.Identity.String()
	cont.PreparerPid = rss.ctxParams.n.Identity.String()
	bc := make(chan []byte)
	guardChanMaps.Set(getShardId(rss.ssId, gm.ShardHash, int(gm.ShardIndex)), bc)
	bytes, err := proto.Marshal(gm)
	if err != nil {
		return nil, err
	}
	guardContractMaps.Set(getShardId(rss.ssId, gm.ShardHash, int(gm.ShardIndex)), bytes)
	if !offlineSigning {
		go func() {
			sign, err := crypto.Sign(rss.ctxParams.n.PrivateKey, gm)
			if err != nil {
				rss.to(rssErrorStatus, err)
				return
			}
			bc <- sign
		}()
	}
	signedBytes := <-bc
	cont.RenterSignature = signedBytes
	return proto.Marshal(cont)
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
