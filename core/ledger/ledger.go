package ledger

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"strings"
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
	certFile = `-----BEGIN CERTIFICATE-----
MIIEADCCAuigAwIBAgIBADANBgkqhkiG9w0BAQUFADBjMQswCQYDVQQGEwJVUzEh
MB8GA1UEChMYVGhlIEdvIERhZGR5IEdyb3VwLCBJbmMuMTEwLwYDVQQLEyhHbyBE
YWRkeSBDbGFzcyAyIENlcnRpZmljYXRpb24gQXV0aG9yaXR5MB4XDTA0MDYyOTE3
MDYyMFoXDTM0MDYyOTE3MDYyMFowYzELMAkGA1UEBhMCVVMxITAfBgNVBAoTGFRo
ZSBHbyBEYWRkeSBHcm91cCwgSW5jLjExMC8GA1UECxMoR28gRGFkZHkgQ2xhc3Mg
MiBDZXJ0aWZpY2F0aW9uIEF1dGhvcml0eTCCASAwDQYJKoZIhvcNAQEBBQADggEN
ADCCAQgCggEBAN6d1+pXGEmhW+vXX0iG6r7d/+TvZxz0ZWizV3GgXne77ZtJ6XCA
PVYYYwhv2vLM0D9/AlQiVBDYsoHUwHU9S3/Hd8M+eKsaA7Ugay9qK7HFiH7Eux6w
wdhFJ2+qN1j3hybX2C32qRe3H3I2TqYXP2WYktsqbl2i/ojgC95/5Y0V4evLOtXi
EqITLdiOr18SPaAIBQi2XKVlOARFmR6jYGB0xUGlcmIbYsUfb18aQr4CUWWoriMY
avx4A6lNf4DD+qta/KFApMoZFv6yyO9ecw3ud72a9nmYvLEHZ6IVDd2gWMZEewo+
YihfukEHU1jPEX44dMX4/7VpkI+EdOqXG68CAQOjgcAwgb0wHQYDVR0OBBYEFNLE
sNKR1EwRcbNhyz2h/t2oatTjMIGNBgNVHSMEgYUwgYKAFNLEsNKR1EwRcbNhyz2h
/t2oatTjoWekZTBjMQswCQYDVQQGEwJVUzEhMB8GA1UEChMYVGhlIEdvIERhZGR5
IEdyb3VwLCBJbmMuMTEwLwYDVQQLEyhHbyBEYWRkeSBDbGFzcyAyIENlcnRpZmlj
YXRpb24gQXV0aG9yaXR5ggEAMAwGA1UdEwQFMAMBAf8wDQYJKoZIhvcNAQEFBQAD
ggEBADJL87LKPpH8EsahB4yOd6AzBhRckB4Y9wimPQoZ+YeAEW5p5JYXMP80kWNy
OO7MHAGjHZQopDH2esRU1/blMVgDoszOYtuURXO1v0XJJLXVggKtI3lpjbi2Tc7P
TMozI+gciKqdi0FuFskg5YmezTvacPd+mSYgFFQlq25zheabIZ0KbIIOqPjCDPoQ
HmyW74cNxA9hi63ugyuV+I6ShHI56yDqg+2DzZduCLzrTia2cyvk0/ZM/iZx4mER
dEr/VxqHD3VILs9RaRegAhJhldXRQLIQTO7ErBBDpqWeCtWVYpoNz4iCxTIM5Cuf
ReYNnyicsbkqWletNw+vHX/bvZ8=
-----END CERTIFICATE-----`
)

func LedgerConnection() (*grpc.ClientConn, error) {
	b, err := ioutil.ReadAll(strings.NewReader(certFile))
	if err != nil {
		return nil, err
	}
	cp := x509.NewCertPool()
	if !cp.AppendCertsFromPEM(b) {
		return nil, fmt.Errorf("credentials: failed to append certificates")
	}
	credential := credentials.NewTLS(&tls.Config{RootCAs: cp})
	conn, err := grpc.Dial(ledgerAddr, grpc.WithTransportCredentials(credential))
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func CloseConnection(conn *grpc.ClientConn) {
	if conn != nil {
		if err := conn.Close(); err != nil {
			log.Error("Failed to close connection: ", err)
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
		log.Error("channel fail to close: ", closed.GetState().Channel)
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
