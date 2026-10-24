package translate

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"

	translationv1 "github.com/gotthboard/gotth-extension-translate/proto/gotth/translation/v1"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

const backendProofDomain = "gotth.extension.translate.backend-proof.v1\x00"

type Backend interface {
	Translate(context.Context, *translationv1.TranslateRequest) (*translationv1.TranslateResponse, error)
	Close() error
}

type localBackend struct {
	connection *grpc.ClientConn
	client     translationv1.TranslationClient
	token      string
}

// BackendProof computes the frozen application-readiness proof. It is exported
// so independently built gotth-translate backends can implement the protocol.
func BackendProof(token, modelIdentity string, challenge []byte) []byte {
	mac := hmac.New(sha256.New, []byte(token))
	_, _ = mac.Write([]byte(backendProofDomain))
	_, _ = mac.Write(challenge)
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(modelIdentity))
	return mac.Sum(nil)
}

// NewLocalBackend consumes one supervisor-provided connected Unix socket. It
// verifies kernel peer credentials, establishes gRPC on that exact connection,
// and proves token possession before the token can be sent by Translate. The
// dialer is single-use: a broken connection fails closed and never redials a
// mutable pathname.
func NewLocalBackend(ctx context.Context, fd int, peerPID int, peerUID, peerGID uint32, modelIdentity, token string) (Backend, error) {
	if ctx == nil || fd < 3 || peerPID < 1 || modelIdentity == "" || token == "" {
		return nil, errors.New("complete connected backend configuration required")
	}
	ucred, err := unix.GetsockoptUcred(fd, unix.SOL_SOCKET, unix.SO_PEERCRED)
	if err != nil || int(ucred.Pid) != peerPID || ucred.Uid != peerUID || ucred.Gid != peerGID {
		_ = unix.Close(fd)
		return nil, errors.New("backend peer identity mismatch")
	}
	file := os.NewFile(uintptr(fd), "gotth-translate-backend")
	if file == nil {
		_ = unix.Close(fd)
		return nil, errors.New("invalid connected backend descriptor")
	}
	connection, err := net.FileConn(file)
	_ = file.Close()
	if err != nil {
		return nil, fmt.Errorf("adopt connected backend descriptor: %w", err)
	}
	if connection.LocalAddr().Network() != "unix" || connection.RemoteAddr().Network() != "unix" {
		_ = connection.Close()
		return nil, errors.New("connected Unix backend descriptor required")
	}
	var mu sync.Mutex
	available := connection
	dialer := func(context.Context, string) (net.Conn, error) {
		mu.Lock()
		defer mu.Unlock()
		if available == nil {
			return nil, errors.New("backend reconnect forbidden")
		}
		conn := available
		available = nil
		return conn, nil
	}
	grpcConn, err := grpc.DialContext(
		ctx,
		"passthrough:///gotth-translate-connected",
		grpc.WithBlock(),
		grpc.WithDisableRetry(),
		grpc.WithIdleTimeout(0),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(dialer),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(MaxResponseEnvelopeBytes), grpc.MaxCallSendMsgSize(MaxRequestEnvelopeBytes)),
	)
	if err != nil {
		_ = connection.Close()
		return nil, fmt.Errorf("connect authenticated backend: %w", err)
	}
	challenge := make([]byte, 32)
	if _, err := rand.Read(challenge); err != nil {
		_ = grpcConn.Close()
		return nil, errors.New("create backend proof challenge")
	}
	proof, err := translationv1.NewBackendIdentityClient(grpcConn).Prove(ctx, &translationv1.BackendProofRequest{Challenge: challenge})
	if err != nil || proof == nil || !bytes.Equal(proof.GetChallenge(), challenge) || !hmac.Equal(proof.GetProofHmacSha256(), BackendProof(token, modelIdentity, challenge)) {
		_ = grpcConn.Close()
		return nil, errors.New("backend readiness proof failed")
	}
	return &localBackend{connection: grpcConn, client: translationv1.NewTranslationClient(grpcConn), token: token}, nil
}

func (b *localBackend) Translate(ctx context.Context, request *translationv1.TranslateRequest) (*translationv1.TranslateResponse, error) {
	ctx = metadata.AppendToOutgoingContext(ctx, "x-gotth-backend-token", b.token, "x-gotth-caller-id", ExtensionID)
	return b.client.Translate(ctx, request)
}

func (b *localBackend) Close() error { return b.connection.Close() }
