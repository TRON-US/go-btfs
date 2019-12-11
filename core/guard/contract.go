package guard

import (
	"github.com/gogo/protobuf/proto"
	ic "github.com/libp2p/go-libp2p-core/crypto"
	"time"

	config "github.com/TRON-US/go-btfs-config"
	"github.com/TRON-US/go-btfs/core/commands/storage"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/tron-us/go-btfs-common/crypto"
	guardPb "github.com/tron-us/go-btfs-common/protos/guard"
)

func NewFileStatus(session *storage.FileContracts, contracts []*guardPb.Contract, configuration *config.Config) (*guardPb.FileStoreStatus, error) {
	guardPid, escrowPid, err := getGuardAndEscrowPid(configuration)
	if err != nil {
		return nil, err
	}

	fileStoreMeta := guardPb.FileStoreMeta{
		RenterPid:        session.Renter.Pretty(),
		FileHash:         session.FileHash.String(), //TODO need to check
		FileSize:         10000,                     //TODO need to revise later
		RentStart:        time.Time{},               //TODO need to revise later
		RentEnd:          time.Time{},               //TODO need to revise later
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

func NewContract(session *storage.FileContracts, configuration *config.Config, chunkHash string, chunkIndex int32) (*guardPb.ContractMeta, error) {
	shard := session.ShardInfo[chunkHash]
	guardPid, escrowPid, err := getGuardAndEscrowPid(configuration)
	if err != nil {
		return nil, err
	}
	return &guardPb.ContractMeta{

		ContractId:    shard.ContractID,
		RenterPid:     session.Renter.Pretty(),
		HostPid:       shard.Receiver.Pretty(),
		ShardHash:     chunkHash,
		ShardIndex:    chunkIndex,
		ShardFileSize: int64(shard.Size),
		FileHash:      session.FileHash.KeyString(),
		RentStart:     shard.StartTime,
		RentEnd:       shard.StartTime.Add(shard.Length),
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
		return "", "", err
	}
	guardPid, err := pidFromString(guardPubKeys[0])
	if err != nil {
		return "", "", err
	}
	return guardPid, escrowPid, err
}

// Todo: modify or change it all
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
	pubKey, err := crypto.ToPubKey(key)
	if err != nil {
		return "", err
	}
	return peer.IDFromPublicKey(pubKey)
}

func SubmitFileStatus(configuration *config.Config, fileStatus guardPb.FileStoreStatus) error {
	return grpc.GuardClient(configuration.Services.GuardDomain).WithContext(context.Background(), func(ctx context.Context,
		c guardPb.GuardServiceClient) error {

		fileStatusResponse, err := c.SubmitFileStoreMeta(context.Background(), &fileStatus)
		if err != nil {
			return err
		} else if fileStatusResponse.Code != guardPb.ResponseCode_SUCCESS {
			return fmt.Errorf("failed to execute submit file status to gurad %s", fileStatusResponse.Code.String())
		}
		return nil
	})
}
