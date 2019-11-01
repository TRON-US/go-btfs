package ledger

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"time"

	ledgerPb "github.com/TRON-US/go-btfs/core/ledger/pb"

	"github.com/gogo/protobuf/proto"
	logging "github.com/ipfs/go-log"
	ic "github.com/libp2p/go-libp2p-core/crypto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

var log = logging.Logger("ledger")
var (
	ledgerAddr = "ledger-nginx.bt.co:443"
	certFile   = `-----BEGIN CERTIFICATE-----
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
	var grpcOption grpc.DialOption
	if certFile == "" {
		grpcOption = grpc.WithInsecure()
	} else {
		b := []byte(certFile)
		cp := x509.NewCertPool()
		if !cp.AppendCertsFromPEM(b) {
			return nil, fmt.Errorf("credentials: failed to append certificates")
		}
		credential := credentials.NewTLS(&tls.Config{RootCAs: cp})
		grpcOption = grpc.WithTransportCredentials(credential)
	}
	conn, err := grpc.Dial(ledgerAddr, grpcOption)
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

func NewAccount(pubKey ic.PubKey, amount int64) (*ledgerPb.Account, error) {
	addr, err := pubKey.Raw()
	if err != nil {
		return nil, err
	}
	return &ledgerPb.Account{
		Address: &ledgerPb.PublicKey{Key: addr},
		Balance: amount,
	}, nil
}

func NewChannelCommit(fromKey ic.PubKey, toKey ic.PubKey, amount int64) (*ledgerPb.ChannelCommit, error) {
	fromAddr, err := fromKey.Raw()
	if err != nil {
		return nil, err
	}
	toAddr, err := toKey.Raw()
	if err != nil {
		return nil, err
	}
	return &ledgerPb.ChannelCommit{
		Payer:     &ledgerPb.PublicKey{Key: fromAddr},
		Recipient: &ledgerPb.PublicKey{Key: toAddr},
		Amount:    amount,
		PayerId:   time.Now().UnixNano(),
	}, err
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

func ImportAccount(ctx context.Context, pubKey ic.PubKey, ledgerClient ledgerPb.ChannelsClient) (*ledgerPb.Account, error) {
	keyBytes, err := pubKey.Raw()
	if err != nil {
		return nil, err
	}
	res, err := ledgerClient.CreateAccount(ctx, &ledgerPb.PublicKey{Key: keyBytes})
	if err != nil {
		return nil, err
	}
	return res.GetAccount(), nil
}

func ImportSignedAccount(ctx context.Context, privKey ic.PrivKey, pubKey ic.PubKey, ledgerClient ledgerPb.ChannelsClient) (*ledgerPb.SignedCreateAccountResult, error) {
	pubKeyBytes, err := pubKey.Raw()
	if err != nil {
		return nil, err
	}
	singedPubKey := &ledgerPb.PublicKey{Key: pubKeyBytes}
	sigBytes, err := proto.Marshal(singedPubKey)
	signature, err := privKey.Sign(sigBytes)
	if err != nil {
		return nil, err
	}
	signedPubkey := &ledgerPb.SignedPublicKey{Key: singedPubKey, Signature: signature}
	return ledgerClient.SignedCreateAccount(ctx, signedPubkey)
}

func CreateChannel(ctx context.Context, ledgerClient ledgerPb.ChannelsClient, channelCommit *ledgerPb.ChannelCommit, sig []byte) (*ledgerPb.ChannelID, error) {
	return ledgerClient.CreateChannel(ctx, &ledgerPb.SignedChannelCommit{
		Channel:   channelCommit,
		Signature: sig,
	})
}

func CloseChannel(ctx context.Context, signedChannelState *ledgerPb.SignedChannelState) error {
	clientConn, err := LedgerConnection()
	defer CloseConnection(clientConn)
	if err != nil {
		return err
	}
	ledgerClient := NewClient(clientConn)
	_, err = ledgerClient.CloseChannel(ctx, signedChannelState)
	if err != nil {
		return err
	}
	return nil
}

func Sign(key ic.PrivKey, channelMessage proto.Message) ([]byte, error) {
	raw, err := proto.Marshal(channelMessage)
	if err != nil {
		return nil, err
	}
	return key.Sign(raw)
}

func Verify(key ic.PubKey, channelMessage proto.Message, sig []byte) (bool, error) {
	raw, err := proto.Marshal(channelMessage)
	if err != nil {
		return false, err
	}
	return key.Verify(raw, sig)
}
