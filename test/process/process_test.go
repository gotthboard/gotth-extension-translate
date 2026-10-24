package process_test

import (
	"bytes"
	"context"
	"crypto/subtle"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/gotthboard/gotth-extension-translate/internal/translate"
	translationv1 "github.com/gotthboard/gotth-extension-translate/proto/gotth/translation/v1"
	extensioncore "github.com/gotthboard/gotth-extensions/pkg/extensions"
	extensionsv1 "github.com/gotthboard/gotth-sdk/gen/go/gotth/extensions/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const hostToken = "host-token-012345678901234567890123456789"
const backendToken = "backend-token-01234567890123456789012345"
const modelIdentity = "gotth.translate.mock.v1"

type backendServer struct {
	translationv1.UnimplementedTranslationServer
	translationv1.UnimplementedBackendIdentityServer
}

func (backendServer) Prove(_ context.Context, in *translationv1.BackendProofRequest) (*translationv1.BackendProofResponse, error) {
	if len(in.GetChallenge()) != 32 {
		return nil, status.Error(codes.InvalidArgument, "invalid_backend_challenge")
	}
	return &translationv1.BackendProofResponse{Challenge: append([]byte(nil), in.GetChallenge()...), ProofHmacSha256: translate.BackendProof(backendToken, modelIdentity, in.GetChallenge())}, nil
}

func (backendServer) Translate(ctx context.Context, in *translationv1.TranslateRequest) (*translationv1.TranslateResponse, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	tokens := md.Get("x-gotth-backend-token")
	callers := md.Get("x-gotth-caller-id")
	if len(tokens) != 1 || subtle.ConstantTimeCompare([]byte(tokens[0]), []byte(backendToken)) != 1 || len(callers) != 1 || callers[0] != translate.ExtensionID {
		return nil, status.Error(codes.Unauthenticated, "backend_identity_required")
	}
	results := make([]*translationv1.SegmentResult, len(in.GetSegments()))
	for i, item := range in.GetSegments() {
		results[i] = &translationv1.SegmentResult{Id: item.GetId(), DetectedSourceLanguage: "en", Text: "hola mundo", QualityTier: translationv1.QualityTier_QUALITY_TIER_EXPERIMENTAL, ModelIdentity: modelIdentity, ResultClass: translationv1.ResultClass_RESULT_CLASS_TRANSLATED}
	}
	return &translationv1.TranslateResponse{TargetLanguage: in.GetTargetLanguage(), Results: results}, nil
}

type fixture struct {
	binary        string
	backendSocket string
	extensionPath string
	paths         map[string]string
	session       extensioncore.Session
}

func newFixture(t *testing.T, binary string) (*fixture, func()) {
	t.Helper()
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	backendSocket := filepath.Join(root, "backend.sock")
	backendListener, err := net.Listen("unix", backendSocket)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(backendSocket, 0o600); err != nil {
		t.Fatal(err)
	}
	backendGRPC := grpc.NewServer()
	backend := backendServer{}
	translationv1.RegisterTranslationServer(backendGRPC, backend)
	translationv1.RegisterBackendIdentityServer(backendGRPC, backend)
	go func() { _ = backendGRPC.Serve(backendListener) }()
	manifest, err := translate.Manifest("dev")
	if err != nil {
		t.Fatal(err)
	}
	manifestBytes, err := extensioncore.CanonicalManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifestDigest, err := extensioncore.ManifestDigest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	grant := extensioncore.Grant{Schema: extensioncore.GrantSchema, InstanceID: "11111111-1111-1111-1111-111111111111", ExtensionID: translate.ExtensionID, ManifestDigest: manifestDigest, Capabilities: []string{translate.TranslateCapability}, Interfaces: []extensioncore.InterfaceGrant{{Name: translate.InterfaceName, Major: 1, Minor: 0}}, Secrets: []string{}}
	grantBytes, err := extensioncore.CanonicalGrant(grant)
	if err != nil {
		t.Fatal(err)
	}
	session, err := extensioncore.Negotiate(manifest, grant, extensioncore.HostProfile{Protocols: []extensioncore.VersionRange{{Name: extensioncore.ControlName, Major: 1, MinMinor: 0, MaxMinor: 0}}, Interfaces: []extensioncore.VersionRange{{Name: translate.InterfaceName, Major: 1, MinMinor: 0, MaxMinor: 0}}})
	if err != nil {
		t.Fatal(err)
	}
	writePrivate := func(name string, data []byte) string {
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	config := []byte(fmt.Sprintf("{\"schema\":\"gotth.extension.translate.config.v1\",\"backend_peer_pid\":%d,\"backend_peer_uid\":%d,\"backend_peer_gid\":%d,\"backend_model_identity\":\"gotth.translate.mock.v1\",\"backend_quality_tier\":\"experimental\",\"backend_timeout_ms\":1000,\"max_concurrent\":2}", os.Getpid(), os.Geteuid(), os.Getegid()))
	extensionPath := filepath.Join(root, "extension.sock")
	paths := map[string]string{"GOTTH_EXTENSION_CONFIG_FILE": writePrivate("config.json", config), "GOTTH_EXTENSION_MANIFEST_FILE": writePrivate("manifest.json", manifestBytes), "GOTTH_EXTENSION_GRANT_FILE": writePrivate("grant.json", grantBytes), "GOTTH_EXTENSION_SERVICE_TOKEN_FILE": writePrivate("host.token", []byte(hostToken)), "GOTTH_TRANSLATE_SERVICE_TOKEN_FILE": writePrivate("backend.token", []byte(backendToken)), "GOTTH_EXTENSION_SOCKET": extensionPath, "GOTTH_TRANSLATE_BACKEND_FD": "3"}
	cleanup := func() { backendGRPC.Stop(); _ = backendListener.Close() }
	return &fixture{binary: binary, backendSocket: backendSocket, extensionPath: extensionPath, paths: paths, session: session}, cleanup
}

func (f *fixture) start(t *testing.T, stderr *bytes.Buffer) *exec.Cmd {
	t.Helper()
	conn, err := net.Dial("unix", f.backendSocket)
	if err != nil {
		t.Fatal(err)
	}
	file, err := conn.(*net.UnixConn).File()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(f.binary)
	cmd.Env = os.Environ()
	for key, value := range f.paths {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	cmd.ExtraFiles = []*os.File{file}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	_ = file.Close()
	_ = conn.Close()
	return cmd
}

func waitForExit(t *testing.T, cmd *exec.Cmd, limit time.Duration) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return err
	case <-time.After(limit):
		_ = cmd.Process.Kill()
		<-done
		t.Fatal("process did not terminate")
		return nil
	}
}

func waitForSocket(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if info, err := os.Lstat(path); err == nil {
			if info.Mode().Perm() != 0o600 {
				t.Fatalf("socket mode %o", info.Mode().Perm())
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("extension socket absent")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func waitForSocketRemoval(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(deadline) {
		if _, err := os.Lstat(path); os.IsNotExist(err) {
			return
		}
		time.Sleep(time.Millisecond)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("socket not removed: %v", err)
	}
}

func TestImmediateTerminateRestartAndAuthenticatedLifecycle(t *testing.T) {
	binary := os.Getenv("GOTTH_TRANSLATE_EXTENSION_BINARY")
	if binary == "" {
		t.Skip("GOTTH_TRANSLATE_EXTENSION_BINARY required")
	}
	fixture, cleanup := newFixture(t, binary)
	defer cleanup()
	for i := 0; i < 20; i++ {
		var stderr bytes.Buffer
		cmd := fixture.start(t, &stderr)
		if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
			t.Fatal(err)
		}
		_ = waitForExit(t, cmd, 3*time.Second)
		waitForSocketRemoval(t, fixture.extensionPath)
	}

	var stderr bytes.Buffer
	cmd := fixture.start(t, &stderr)
	waited := false
	defer func() {
		if !waited {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()
	waitForSocket(t, fixture.extensionPath)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	connection, err := grpc.NewClient("passthrough:///extension", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", fixture.extensionPath)
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	control := extensionsv1.NewExtensionControlClient(connection)
	request := &translationv1.TranslateRequest{CorrelationId: "22222222-2222-2222-2222-222222222222", SourceLanguage: "en", TargetLanguage: "es", ContentKind: translationv1.ContentKind_CONTENT_KIND_MAIL_VISIBLE_TEXT, CacheClass: translationv1.CacheClass_CACHE_CLASS_NONE, Segments: []*translationv1.Segment{{Id: "visible-body", Text: "SECRET PROCESS CONTENT"}}}
	if _, err := translationv1.NewTranslationClient(connection).Translate(ctx, request); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("unauthenticated call: %v", err)
	}
	if _, err := control.Handshake(ctx, &extensionsv1.HandshakeRequest{}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("malformed unauthenticated handshake reached handler: %v", err)
	}
	authed := metadata.NewOutgoingContext(ctx, metadata.Pairs("x-gotth-service-token", hostToken, "x-gotth-session-sha256", fixture.session.Fingerprint))
	handshake, err := control.Handshake(authed, &extensionsv1.HandshakeRequest{CorrelationId: request.CorrelationId, Challenge: bytes.Repeat([]byte{1}, 32), ExpectedExtensionId: translate.ExtensionID, ExpectedManifestSha256: fixture.session.ManifestDigest, ExpectedGrantSha256: fixture.session.GrantDigest, ExpectedSessionSha256: fixture.session.Fingerprint})
	if err != nil || handshake.GetSessionSha256() != fixture.session.Fingerprint {
		t.Fatalf("handshake: %+v %v", handshake, err)
	}
	response, err := translationv1.NewTranslationClient(connection).Translate(authed, request)
	if err != nil || response.GetResults()[0].GetText() != "hola mundo" {
		t.Fatalf("translation: %+v %v", response, err)
	}
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	if err := waitForExit(t, cmd, 3*time.Second); err != nil {
		t.Fatal(err)
	}
	waited = true
	waitForSocketRemoval(t, fixture.extensionPath)
	if strings.Contains(stderr.String(), "SECRET PROCESS CONTENT") || strings.Contains(stderr.String(), "hola mundo") || strings.Contains(stderr.String(), request.CorrelationId) {
		t.Fatalf("content leaked to stderr: %s", stderr.String())
	}
}
