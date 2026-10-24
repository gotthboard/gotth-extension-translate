package translate

import (
	"context"
	"crypto/subtle"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	translationv1 "github.com/gotthboard/gotth-extension-translate/proto/gotth/translation/v1"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type mockLocalTranslate struct {
	translationv1.UnimplementedTranslationServer
	translationv1.UnimplementedBackendIdentityServer
	token    string
	model    string
	oversize bool
}

func (m mockLocalTranslate) Prove(_ context.Context, in *translationv1.BackendProofRequest) (*translationv1.BackendProofResponse, error) {
	if len(in.GetChallenge()) != 32 {
		return nil, status.Error(codes.InvalidArgument, "invalid_backend_challenge")
	}
	return &translationv1.BackendProofResponse{Challenge: append([]byte(nil), in.GetChallenge()...), ProofHmacSha256: BackendProof(m.token, m.model, in.GetChallenge())}, nil
}

func (m mockLocalTranslate) Translate(ctx context.Context, in *translationv1.TranslateRequest) (*translationv1.TranslateResponse, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	tokens := md.Get("x-gotth-backend-token")
	callers := md.Get("x-gotth-caller-id")
	if len(tokens) != 1 || subtle.ConstantTimeCompare([]byte(tokens[0]), []byte(m.token)) != 1 || len(callers) != 1 || callers[0] != ExtensionID {
		return nil, status.Error(codes.Unauthenticated, "backend_identity_required")
	}
	if m.oversize {
		response := translated(in)
		response.Results[0].Text = strings.Repeat("x", MaxResponseEnvelopeBytes+1)
		return response, nil
	}
	return translated(in), nil
}

func listenUnix(path string) (net.Listener, error) { return net.Listen("unix", path) }

func connectedUnixFD(t *testing.T, path string) int {
	t.Helper()
	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	unixConn := conn.(*net.UnixConn)
	file, err := unixConn.File()
	if err != nil {
		t.Fatal(err)
	}
	fd, err := unix.Dup(int(file.Fd()))
	_ = file.Close()
	_ = conn.Close()
	if err != nil {
		t.Fatal(err)
	}
	return fd
}

func startMockBackend(t *testing.T, socket, token string) (*grpc.Server, net.Listener) {
	t.Helper()
	listener, err := listenUnix(socket)
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	mock := mockLocalTranslate{token: token, model: testModel}
	translationv1.RegisterTranslationServer(server, mock)
	translationv1.RegisterBackendIdentityServer(server, mock)
	go func() { _ = server.Serve(listener) }()
	return server, listener
}

func TestLocalBackendAuthenticatesConnectedUnixPeerBeforeToken(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(root, "translate.sock")
	server, listener := startMockBackend(t, socket, testBackendToken)
	defer server.Stop()
	defer listener.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	backend, err := NewLocalBackend(ctx, connectedUnixFD(t, socket), os.Getpid(), uint32(os.Geteuid()), uint32(os.Getegid()), testModel, testBackendToken)
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	response, err := backend.Translate(ctx, validRequest())
	if err != nil || response.GetResults()[0].GetModelIdentity() != testModel {
		t.Fatalf("response: %+v %v", response, err)
	}
}

func TestLocalBackendRejectsOversizedGRPCResponse(t *testing.T) {
	root := t.TempDir()
	socket := filepath.Join(root, "translate.sock")
	listener, err := listenUnix(socket)
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	mock := mockLocalTranslate{token: testBackendToken, model: testModel, oversize: true}
	translationv1.RegisterTranslationServer(server, mock)
	translationv1.RegisterBackendIdentityServer(server, mock)
	go func() { _ = server.Serve(listener) }()
	defer server.Stop()
	defer listener.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	backend, err := NewLocalBackend(ctx, connectedUnixFD(t, socket), os.Getpid(), uint32(os.Geteuid()), uint32(os.Getegid()), testModel, testBackendToken)
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	if _, err := backend.Translate(ctx, validRequest()); status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("oversized backend transport response: %v", err)
	}
}

func TestLocalBackendRejectsPeerAndProofMismatch(t *testing.T) {
	root := t.TempDir()
	socket := filepath.Join(root, "translate.sock")
	server, listener := startMockBackend(t, socket, testBackendToken)
	defer server.Stop()
	defer listener.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := NewLocalBackend(ctx, connectedUnixFD(t, socket), os.Getpid()+1, uint32(os.Geteuid()), uint32(os.Getegid()), testModel, testBackendToken); err == nil {
		t.Fatal("wrong peer PID accepted")
	}
	if _, err := NewLocalBackend(ctx, connectedUnixFD(t, socket), os.Getpid(), uint32(os.Geteuid()), uint32(os.Getegid()), testModel, "wrong-proof-token-012345678901234567890"); err == nil {
		t.Fatal("backend without configured proof token accepted")
	}
}

func TestLocalBackendNeverRedialsReplacedPath(t *testing.T) {
	root := t.TempDir()
	socket := filepath.Join(root, "translate.sock")
	server, listener := startMockBackend(t, socket, testBackendToken)
	fd := connectedUnixFD(t, socket)
	if err := os.Remove(socket); err != nil {
		t.Fatal(err)
	}
	impostor, impostorListener := startMockBackend(t, socket, "impostor-token-01234567890123456789012")
	defer impostor.Stop()
	defer impostorListener.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	backend, err := NewLocalBackend(ctx, fd, os.Getpid(), uint32(os.Geteuid()), uint32(os.Getegid()), testModel, testBackendToken)
	if err != nil {
		t.Fatalf("connected original rejected after pathname replacement: %v", err)
	}
	server.Stop()
	_ = listener.Close()
	callCtx, callCancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer callCancel()
	if _, err := backend.Translate(callCtx, validRequest()); err == nil {
		t.Fatal("dead authenticated connection silently redialed replacement")
	}
	_ = backend.Close()
}
