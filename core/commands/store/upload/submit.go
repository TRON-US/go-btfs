package upload

import (
	"github.com/TRON-US/go-btfs/core/escrow"

	escrowpb "github.com/tron-us/go-btfs-common/protos/escrow"
	guardpb "github.com/tron-us/go-btfs-common/protos/guard"
	"github.com/tron-us/protobuf/proto"
)

func submit(rss *RenterSession, fileSize int64, offlineSigning bool) {
	rss.to(rssToSubmitEvent)
	res, err := doSubmit(rss, offlineSigning)
	if err != nil {
		//TODO: handle error
		return
	}
	//TODO
	pay(rss, res, fileSize, offlineSigning)
}

func doSubmit(rss *RenterSession, offlineSigning bool) (*escrowpb.SignedSubmitContractResult, error) {
	bs, t, err := prepareContracts(rss, rss.shardHashes)
	if err != nil {
		return nil, err
	}
	err = checkBalance(rss, offlineSigning, t)
	if err != nil {
		return nil, err
	}
	req, err := NewContractRequest(rss, bs, t, offlineSigning)
	if err != nil {
		return nil, err
	}
	var amount int64 = 0
	for _, c := range req.Contract {
		amount += c.Contract.Amount
	}
	submitContractRes, err := escrow.SubmitContractToEscrow(rss.ctx, rss.ctxParams.cfg, req)
	if err != nil {
		return nil, err
	}
	return submitContractRes, nil
}

func prepareContracts(rss *RenterSession, shardHashes []string) ([]*escrowpb.SignedEscrowContract, int64, error) {
	var signedContracts []*escrowpb.SignedEscrowContract
	var totalPrice int64
	for _, hash := range shardHashes {
		shard, err := GetRenterShard(rss.ctxParams, rss.ssId, hash)
		if err != nil {
			return nil, 0, err
		}
		c, err := shard.contracts()
		if err != nil {
			return nil, 0, err
		}
		escrowContract := &escrowpb.SignedEscrowContract{}
		err = proto.Unmarshal(c.SignedEscrowContract, escrowContract)
		if err != nil {
			return nil, 0, err
		}
		signedContracts = append(signedContracts, escrowContract)
		guardContract := &guardpb.Contract{}
		err = proto.Unmarshal(c.SignedGuardContract, guardContract)
		if err != nil {
			return nil, 0, err
		}
		totalPrice += guardContract.Amount
	}
	return signedContracts, totalPrice, nil
}
