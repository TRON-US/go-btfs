package ledger

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"os"
	"time"

	ledgerPb "github.com/TRON-US/go-btfs/core/ledger/pb"

	ethereum "github.com/ethereum/go-ethereum/crypto"
	"github.com/gogo/protobuf/proto"
	logging "github.com/ipfs/go-log"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

var log = logging.Logger("ledger")
var (
	ledgerAddr     = "ledger.bt.co:443"
	credentialPath = "/core/ledger/credential/public_key.pem"
)

func LedgerConnection() (*grpc.ClientConn, error) {
	wd, err := os.Getwd()
	credential, err := credentials.NewClientTLSFromFile(wd+credentialPath, "")
	if err != nil {
		log.Error("fail to load credential: ", err)
		return nil, err
	}
	conn, err := grpc.Dial(ledgerAddr, grpc.WithTransportCredentials(credential))
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func CloseConnection(conn *grpc.ClientConn) {
	if conn != nil {
		if err := conn.Close(); err != nil {
			log.Error("Failed to close connection: %v", err)
		}
	}
}

func NewClient(conn *grpc.ClientConn) ledgerPb.ChannelsClient {
	return ledgerPb.NewChannelsClient(conn)
}

func NewAccount(addr []byte, amount int64) *ledgerPb.Account {
	return &ledgerPb.Account{
		Address: &ledgerPb.PublicKey{Key: addr},
		Balance: amount,
	}
}

func NewChannelCommit(fromAddr []byte, toAddr []byte, amount int64) *ledgerPb.ChannelCommit {
	return &ledgerPb.ChannelCommit{
		Payer:    &ledgerPb.PublicKey{Key: fromAddr},
		Receiver: &ledgerPb.PublicKey{Key: toAddr},
		Amount:   amount,
		PayerId:  time.Now().UnixNano(),
	}
}

func NewChannelState(id *ledgerPb.ChannelID, sequence int64, fromAccount *ledgerPb.Account, toAccount *ledgerPb.Account) *ledgerPb.ChannelState {
	return &ledgerPb.ChannelState{
		Id:       id,
		Sequence: sequence,
		From:     fromAccount,
		To:       toAccount,
	}
}

func NewSignedChannelState(channelState *ledgerPb.ChannelState, fromSig []byte, toSig []byte) *ledgerPb.SignedChannelState {
	return &ledgerPb.SignedChannelState{
		Channel:       channelState,
		FromSignature: fromSig,
		ToSignature:   toSig,
	}
}

func CreateAccount(ctx context.Context, ledgerClient ledgerPb.ChannelsClient) (*ecdsa.PrivateKey, *ledgerPb.Account, error) {
	privKey, err := ethereum.GenerateKey()
	if err != nil {
		log.Error("fail to generate private key: ", err)
		return nil, nil, err
	}
	pubKeyBytes := ethereum.FromECDSAPub(&privKey.PublicKey)
	res, err := ledgerClient.CreateAccount(ctx, &ledgerPb.PublicKey{Key: pubKeyBytes})
	if err != nil {
		log.Error("fail to create account: ", err)
		return nil, nil, err
	}
	return privKey, res.GetAccount(), nil
}

func CreateChannel(ctx context.Context, ledgerClient ledgerPb.ChannelsClient, channelCommit *ledgerPb.ChannelCommit, sig []byte) (*ledgerPb.ChannelID, error) {
	return ledgerClient.CreateChannel(ctx, &ledgerPb.SignedChannelCommit{
		Channel:   channelCommit,
		Signature: sig,
	})
}

func CloseChannel(ctx context.Context, ledgerClient ledgerPb.ChannelsClient, signedChannelState *ledgerPb.SignedChannelState) error {
	closed, err := ledgerClient.CloseChannel(ctx, signedChannelState)
	if err != nil {
		log.Error("channel fail to close:", closed.GetState().Channel)
		return err
	}
	return nil
}

func Sign(key *ecdsa.PrivateKey, channelMessage proto.Message) ([]byte, error) {
	raw, err := proto.Marshal(channelMessage)
	if err != nil {
		log.Error("fail to marshal pb message: ", err)
		return nil, err
	}
	hash, err := hash(raw)
	if err != nil {
		log.Error("fail to hash pb message: ", err)
		return nil, err
	}
	return key.Sign(rand.Reader, hash, crypto.SHA256)
}

func hash(s []byte) ([]byte, error) {
	h := sha256.New()
	_, err := h.Write(s)
	if err != nil {
		return nil, err
	}
	bs := h.Sum(nil)
	return bs, nil
}
