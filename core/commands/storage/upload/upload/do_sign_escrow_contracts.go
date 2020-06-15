package upload

import (
	"fmt"
	"github.com/TRON-US/go-btfs/core/commands/storage/helper"
	"github.com/TRON-US/go-btfs/core/commands/storage/upload/escrow"
	uh "github.com/TRON-US/go-btfs/core/commands/storage/upload/helper"
	"github.com/TRON-US/go-btfs/core/commands/storage/upload/sessions"

	"github.com/tron-us/go-btfs-common/crypto"
	"github.com/tron-us/go-btfs-common/ledger"
	escrowpb "github.com/tron-us/go-btfs-common/protos/escrow"
	"github.com/tron-us/protobuf/proto"

	ic "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
)

func renterSignEscrowContract(rss *sessions.RenterSession, shardHash string, shardIndex int, host string, totalPay int64, contingentAmount int64,
	offlineSigning bool, isRenewContract bool, offSignPid peer.ID,
	contractId string) ([]byte, error) {
	var hostPid peer.ID
	if !isRenewContract {
		if id, err := peer.IDB58Decode(host); err == nil {
			hostPid = id
		} else {
			return nil, err
		}
	}
	escrowContract, err := newContract(rss, hostPid, totalPay, contingentAmount, false, isRenewContract, 0, offSignPid, contractId)
	if err != nil {
		return nil, fmt.Errorf("create escrow contract failed: [%v] ", err)
	}
	bc := make(chan []byte)
	shardId := sessions.GetShardId(rss.SsId, shardHash, shardIndex)
	uh.EscrowChanMaps.Set(shardId, bc)
	bytes, err := proto.Marshal(escrowContract)
	if err != nil {
		return nil, err
	}
	uh.EscrowContractMaps.Set(shardId, bytes)
	if !offlineSigning {
		errChan := make(chan error)
		go func() {
			sign, err := crypto.Sign(rss.CtxParams.N.PrivateKey, escrowContract)
			if err != nil {
				errChan <- err
				return
			}
			errChan <- nil
			bc <- sign
		}()
		err = <-errChan
		if err != nil {
			return nil, err
		}
	}
	renterSignBytes := <-bc
	uh.EscrowChanMaps.Remove(shardId)
	uh.EscrowContractMaps.Remove(shardId)
	renterSignedEscrowContract, err := signContractAndMarshalOffSign(escrowContract, renterSignBytes, nil)
	if err != nil {
		return nil, err
	}
	return renterSignedEscrowContract, nil
}

func newContract(rss *sessions.RenterSession, hostPid peer.ID, totalPay int64, contingentAmount int64, customizedSchedule bool, isRenewContract bool, period int,
	pid peer.ID, contractId string) (*escrowpb.EscrowContract, error) {
	var err error
	payerPubKey, err := pid.ExtractPublicKey()
	if err != nil {
		return nil, err
	}
	var hostPubKey ic.PubKey
	if hostPid != "" {
		if pubKey, err := hostPid.ExtractPublicKey(); err == nil {
			hostPubKey = pubKey
		} else {
			return nil, err
		}
	}
	if len(rss.CtxParams.Cfg.Services.GuardPubKeys) == 0 {
		return nil, fmt.Errorf("No Services.GuardPubKeys are set in config")
	}
	authPubKey, err := helper.ConvertToPubKey(rss.CtxParams.Cfg.Services.GuardPubKeys[0])
	if err != nil {
		return nil, err
	}
	ps := escrowpb.Schedule_MONTHLY
	p := 0
	if customizedSchedule {
		ps = escrowpb.Schedule_CUSTOMIZED
		p = period
	}
	contrType := escrowpb.ContractType_REGULAR
	if isRenewContract {
		contrType = escrowpb.ContractType_PLAN
	}
	return ledger.NewEscrowContract(contractId,
		payerPubKey, hostPubKey, authPubKey, totalPay, ps, int32(p), contrType, contingentAmount)
}

func signContractAndMarshalOffSign(unsignedContract *escrowpb.EscrowContract, signedBytes []byte,
	signedContract *escrowpb.SignedEscrowContract) ([]byte, error) {

	if signedContract == nil {
		signedContract = escrow.NewSignedContract(unsignedContract)
	}
	signedContract.BuyerSignature = signedBytes
	result, err := proto.Marshal(signedContract)
	if err != nil {
		return nil, err
	}
	return result, nil
}
