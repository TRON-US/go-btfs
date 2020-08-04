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

	"github.com/libp2p/go-libp2p-core/peer"
)

func renterSignEscrowContract(rss *sessions.RenterSession, shardHash string, shardIndex int, host string, totalPay int64,
	offlineSigning bool, offSignPid peer.ID,
	contractId string) ([]byte, error) {
	hostPid, err := peer.IDB58Decode(host)
	if err != nil {
		return nil, err
	}
	escrowContract, err := newContract(rss, hostPid, totalPay, false, 0, offSignPid, contractId)
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

func newContract(rss *sessions.RenterSession, hostPid peer.ID, totalPay int64, customizedSchedule bool, period int,
	pid peer.ID, contractId string) (*escrowpb.EscrowContract, error) {
	var err error
	payerPubKey, err := pid.ExtractPublicKey()
	if err != nil {
		return nil, err
	}
	hostPubKey, err := hostPid.ExtractPublicKey()
	if err != nil {
		return nil, err
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
	return ledger.NewEscrowContract(contractId, payerPubKey, hostPubKey, authPubKey, totalPay, ps,
		int32(p), escrowpb.ContractType_REGULAR, 0)

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
