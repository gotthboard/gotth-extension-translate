package translate

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	translationv1 "github.com/gotthboard/gotth-extension-translate/proto/gotth/translation/v1"
	extensioncore "github.com/gotthboard/gotth-extensions/pkg/extensions"
	extensionsv1 "github.com/gotthboard/gotth-sdk/gen/go/gotth/extensions/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const testHostToken = "host-token-012345678901234567890123456789"
const testBackendToken = "backend-token-01234567890123456789012345"
const testModel = "gotth.translate.mock.v1"

type fakeBackend struct {
	calls     atomic.Int32
	translate func(context.Context, *translationv1.TranslateRequest) (*translationv1.TranslateResponse, error)
}

func (f *fakeBackend) Translate(ctx context.Context, in *translationv1.TranslateRequest) (*translationv1.TranslateResponse, error) {
	f.calls.Add(1)
	return f.translate(ctx, in)
}
func (f *fakeBackend) Close() error { return nil }

type blockedWriter struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (w *blockedWriter) Write(data []byte) (int, error) {
	w.once.Do(func() { close(w.started) })
	<-w.release
	return len(data), nil
}

func admittedRuntime(t *testing.T) RuntimeConfig {
	t.Helper()
	manifest, err := Manifest("dev")
	if err != nil {
		t.Fatal(err)
	}
	digest, err := extensioncore.ManifestDigest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	grant := extensioncore.Grant{Schema: extensioncore.GrantSchema, InstanceID: "11111111-1111-1111-1111-111111111111", ExtensionID: ExtensionID, ManifestDigest: digest, Capabilities: []string{TranslateCapability}, Interfaces: []extensioncore.InterfaceGrant{{Name: InterfaceName, Major: 1, Minor: 0}}, Secrets: []string{}}
	session, err := negotiateExact(manifest, grant)
	if err != nil {
		t.Fatal(err)
	}
	return RuntimeConfig{Config: Config{Schema: ConfigSchema, BackendPeerPID: os.Getpid(), BackendPeerUID: uint32(os.Geteuid()), BackendPeerGID: uint32(os.Getegid()), BackendModelIdentity: testModel, BackendQualityTier: "experimental", BackendTimeoutMS: 100, MaxConcurrent: 2}, Manifest: manifest, Grant: grant, Session: session, ServiceToken: testHostToken, BackendToken: testBackendToken, SocketPath: "/tmp/extension.sock", Version: "dev"}
}

func validRequest() *translationv1.TranslateRequest {
	return &translationv1.TranslateRequest{CorrelationId: "22222222-2222-2222-2222-222222222222", SourceLanguage: "en", TargetLanguage: "es", ContentKind: translationv1.ContentKind_CONTENT_KIND_PLAIN, CacheClass: translationv1.CacheClass_CACHE_CLASS_NONE, Segments: []*translationv1.Segment{{Id: "s1", Text: "hello"}, {Id: "s2", Text: "world"}}}
}

func translated(request *translationv1.TranslateRequest) *translationv1.TranslateResponse {
	results := make([]*translationv1.SegmentResult, len(request.GetSegments()))
	for i, item := range request.GetSegments() {
		results[i] = &translationv1.SegmentResult{Id: item.GetId(), DetectedSourceLanguage: "en", Text: "translated-" + item.GetText(), QualityTier: translationv1.QualityTier_QUALITY_TIER_EXPERIMENTAL, ModelIdentity: testModel, ResultClass: translationv1.ResultClass_RESULT_CLASS_TRANSLATED}
	}
	return &translationv1.TranslateResponse{TargetLanguage: request.GetTargetLanguage(), Results: results}
}

func authorizedContext(t *testing.T, runtime RuntimeConfig) (context.Context, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	md := metadata.Pairs("x-gotth-service-token", testHostToken, "x-gotth-session-sha256", runtime.Session.Fingerprint)
	return metadata.NewIncomingContext(ctx, md), cancel
}

func newTestService(t *testing.T, runtime RuntimeConfig, backend *fakeBackend, recorder Recorder) *Service {
	t.Helper()
	service, err := NewService(runtime, backend, recorder)
	if err != nil {
		t.Fatal(err)
	}
	service.SetReady()
	return service
}

func TestManifestAndExactSession(t *testing.T) {
	runtime := admittedRuntime(t)
	if runtime.Manifest.ID != ExtensionID || runtime.Session.ManifestDigest == "" || runtime.Session.GrantDigest == "" || runtime.Session.Fingerprint == "" {
		t.Fatalf("incomplete binding: %+v", runtime.Session)
	}
	if len(runtime.Manifest.Secrets) != 0 || len(runtime.Session.Secrets) != 0 {
		t.Fatal("v1 must have no provider secret or remote egress grant")
	}
	missingCapability := runtime.Grant
	missingCapability.Capabilities = []string{}
	if _, err := negotiateExact(runtime.Manifest, missingCapability); !errors.Is(err, extensioncore.ErrUngrantedCapability) {
		t.Fatalf("missing translate capability accepted: %v", err)
	}
	remoteEgress := runtime.Grant
	remoteEgress.Capabilities = []string{TranslateCapability, "translate.remote-egress"}
	if _, err := negotiateExact(runtime.Manifest, remoteEgress); !errors.Is(err, extensioncore.ErrUngrantedCapability) {
		t.Fatalf("remote egress capability accepted: %v", err)
	}
	mutated := runtime
	mutated.Session.Capabilities[0] = "translate.other"
	backend := &fakeBackend{translate: func(context.Context, *translationv1.TranslateRequest) (*translationv1.TranslateResponse, error) {
		return nil, nil
	}}
	if _, err := NewService(mutated, backend, nil); !errors.Is(err, errSessionChanged) {
		t.Fatalf("mutated session accepted: %v", err)
	}
}

func TestServiceDetachesAuthoritativeRuntime(t *testing.T) {
	runtime := admittedRuntime(t)
	backend := &fakeBackend{translate: func(_ context.Context, in *translationv1.TranslateRequest) (*translationv1.TranslateResponse, error) {
		return translated(in), nil
	}}
	service := newTestService(t, runtime, backend, nil)
	runtime.Manifest.Capabilities[0] = "translate.mutated"
	runtime.Grant.Capabilities[0] = "translate.mutated"
	runtime.Session.Capabilities[0] = "translate.mutated"
	ctx, cancel := authorizedContext(t, service.runtime)
	defer cancel()
	if _, err := service.Translate(ctx, validRequest()); err != nil {
		t.Fatalf("caller mutation changed retained authority: %v", err)
	}
}

func TestControlHandshakeAndHealth(t *testing.T) {
	runtime := admittedRuntime(t)
	backend := &fakeBackend{translate: func(_ context.Context, in *translationv1.TranslateRequest) (*translationv1.TranslateResponse, error) {
		return translated(in), nil
	}}
	service := newTestService(t, runtime, backend, nil)
	ctx, cancel := authorizedContext(t, runtime)
	defer cancel()
	challenge := bytes.Repeat([]byte{7}, 32)
	handshake, err := service.Handshake(ctx, &extensionsv1.HandshakeRequest{CorrelationId: validRequest().CorrelationId, Challenge: challenge, ExpectedExtensionId: ExtensionID, ExpectedManifestSha256: runtime.Session.ManifestDigest, ExpectedGrantSha256: runtime.Session.GrantDigest, ExpectedSessionSha256: runtime.Session.Fingerprint})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(handshake.GetChallenge(), challenge) || handshake.GetSessionSha256() != runtime.Session.Fingerprint || len(handshake.GetInterfaces()) != 1 {
		t.Fatalf("bad handshake: %+v", handshake)
	}
	health, err := service.Health(ctx, &extensionsv1.HealthRequest{CorrelationId: validRequest().CorrelationId, SessionSha256: runtime.Session.Fingerprint})
	if err != nil || health.GetState() != extensionsv1.HealthState_HEALTH_STATE_READY {
		t.Fatalf("health: %+v %v", health, err)
	}
	_, err = service.Health(ctx, &extensionsv1.HealthRequest{CorrelationId: validRequest().CorrelationId, SessionSha256: strings.Repeat("0", 64)})
	if status.Code(err) != codes.FailedPrecondition || status.Convert(err).Message() != "session_binding_mismatch" {
		t.Fatalf("stale health: %v", err)
	}
}

func TestTransportDeadlineAndSessionAuthentication(t *testing.T) {
	runtime := admittedRuntime(t)
	backend := &fakeBackend{translate: func(_ context.Context, in *translationv1.TranslateRequest) (*translationv1.TranslateResponse, error) {
		return translated(in), nil
	}}
	service := newTestService(t, runtime, backend, nil)
	request := validRequest()
	if _, err := service.Translate(context.Background(), request); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("missing deadline: %v", err)
	}
	deadline, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := service.Translate(deadline, request); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("missing identity: %v", err)
	}
	wrongToken := metadata.NewIncomingContext(deadline, metadata.Pairs("x-gotth-service-token", strings.Repeat("x", len(testHostToken)), "x-gotth-session-sha256", runtime.Session.Fingerprint))
	if _, err := service.Translate(wrongToken, request); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("wrong identity: %v", err)
	}
	otherGrant := runtime.Grant
	otherGrant.InstanceID = "99999999-9999-9999-9999-999999999999"
	otherSession, err := negotiateExact(runtime.Manifest, otherGrant)
	if err != nil {
		t.Fatal(err)
	}
	wrongSession := metadata.NewIncomingContext(deadline, metadata.Pairs("x-gotth-service-token", testHostToken, "x-gotth-session-sha256", otherSession.Fingerprint))
	if _, err := service.Translate(wrongSession, request); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("cross-product session request: %v", err)
	}
	if backend.calls.Load() != 0 {
		t.Fatalf("backend called on authentication failure: %d", backend.calls.Load())
	}
}

func TestUnaryInterceptorRejectsEveryHostRPCBeforeHandler(t *testing.T) {
	runtime := admittedRuntime(t)
	backend := &fakeBackend{translate: func(_ context.Context, in *translationv1.TranslateRequest) (*translationv1.TranslateResponse, error) {
		return translated(in), nil
	}}
	service := newTestService(t, runtime, backend, nil)
	var invoked atomic.Int32
	handler := func(context.Context, any) (any, error) {
		invoked.Add(1)
		return nil, nil
	}
	for _, method := range []string{extensionsv1.ExtensionControl_Handshake_FullMethodName, extensionsv1.ExtensionControl_Health_FullMethodName, translationv1.Translation_Translate_FullMethodName} {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		_, err := service.UnaryInterceptor(ctx, nil, &grpc.UnaryServerInfo{FullMethod: method}, handler)
		cancel()
		if status.Code(err) != codes.Unauthenticated {
			t.Fatalf("%s: %v", method, err)
		}
	}
	withToken := metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-gotth-service-token", testHostToken))
	if _, err := service.UnaryInterceptor(withToken, nil, &grpc.UnaryServerInfo{FullMethod: extensionsv1.ExtensionControl_Health_FullMethodName}, handler); status.Code(err) != codes.InvalidArgument || status.Convert(err).Message() != "deadline_required" {
		t.Fatalf("missing deadline: %v", err)
	}
	if invoked.Load() != 0 || backend.calls.Load() != 0 {
		t.Fatalf("rejected RPC reached handler=%d backend=%d", invoked.Load(), backend.calls.Load())
	}
}

func TestStableHostErrorPairs(t *testing.T) {
	runtime := admittedRuntime(t)
	backend := &fakeBackend{translate: func(_ context.Context, in *translationv1.TranslateRequest) (*translationv1.TranslateResponse, error) {
		return translated(in), nil
	}}
	service := newTestService(t, runtime, backend, nil)
	requestCase := func(mutate func(*translationv1.TranslateRequest)) func(context.Context) error {
		return func(ctx context.Context) error {
			request := validRequest()
			mutate(request)
			_, err := service.Translate(ctx, request)
			return err
		}
	}
	cases := []struct {
		name    string
		code    codes.Code
		message string
		call    func(context.Context) error
	}{
		{"invalid handshake", codes.InvalidArgument, "invalid_handshake", func(ctx context.Context) error { _, err := service.Handshake(ctx, nil); return err }},
		{"binding mismatch", codes.FailedPrecondition, "extension_binding_mismatch", func(ctx context.Context) error {
			_, err := service.Handshake(ctx, &extensionsv1.HandshakeRequest{CorrelationId: validRequest().CorrelationId, Challenge: bytes.Repeat([]byte{1}, 32)})
			return err
		}},
		{"invalid health", codes.InvalidArgument, "invalid_health_request", func(ctx context.Context) error { _, err := service.Health(ctx, nil); return err }},
		{"health session", codes.FailedPrecondition, "session_binding_mismatch", func(ctx context.Context) error {
			_, err := service.Health(ctx, &extensionsv1.HealthRequest{CorrelationId: validRequest().CorrelationId, SessionSha256: strings.Repeat("0", 64)})
			return err
		}},
		{"nil translate", codes.InvalidArgument, "invalid_request", func(ctx context.Context) error { _, err := service.Translate(ctx, nil); return err }},
		{"request size", codes.ResourceExhausted, "request_too_large", requestCase(func(r *translationv1.TranslateRequest) { r.Segments[0].Text = strings.Repeat("x", MaxSegmentBytes+1) })},
		{"language", codes.InvalidArgument, "unsupported_language", requestCase(func(r *translationv1.TranslateRequest) { r.TargetLanguage = "auto" })},
		{"content", codes.InvalidArgument, "unsupported_content_kind", requestCase(func(r *translationv1.TranslateRequest) { r.ContentKind = 99 })},
		{"retention", codes.InvalidArgument, "unsupported_retention", requestCase(func(r *translationv1.TranslateRequest) { r.CacheClass = 99 })},
		{"segments", codes.InvalidArgument, "invalid_segments", requestCase(func(r *translationv1.TranslateRequest) { r.Segments[0].Text = "" })},
		{"duplicate", codes.InvalidArgument, "duplicate_segment_id", requestCase(func(r *translationv1.TranslateRequest) { r.Segments[1].Id = r.Segments[0].Id })},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := authorizedContext(t, runtime)
			defer cancel()
			err := tc.call(ctx)
			if status.Code(err) != tc.code || status.Convert(err).Message() != tc.message {
				t.Fatalf("got %v; want %s/%s", err, tc.code, tc.message)
			}
		})
	}
}

func TestRequestValidationRejectsBeforeBackend(t *testing.T) {
	runtime := admittedRuntime(t)
	backend := &fakeBackend{translate: func(_ context.Context, in *translationv1.TranslateRequest) (*translationv1.TranslateResponse, error) {
		return translated(in), nil
	}}
	service := newTestService(t, runtime, backend, nil)
	cases := []struct {
		name    string
		mutate  func(*translationv1.TranslateRequest)
		code    codes.Code
		message string
	}{
		{"noncanonical source", func(r *translationv1.TranslateRequest) { r.SourceLanguage = "EN-us" }, codes.InvalidArgument, "unsupported_language"},
		{"auto target", func(r *translationv1.TranslateRequest) { r.TargetLanguage = "auto" }, codes.InvalidArgument, "unsupported_language"},
		{"unsupported cache", func(r *translationv1.TranslateRequest) { r.CacheClass = 99 }, codes.InvalidArgument, "unsupported_retention"},
		{"content kind", func(r *translationv1.TranslateRequest) { r.ContentKind = 99 }, codes.InvalidArgument, "unsupported_content_kind"},
		{"unknown field", func(r *translationv1.TranslateRequest) { r.ProtoReflect().SetUnknown([]byte{0x38, 0x01}) }, codes.InvalidArgument, "invalid_request"},
		{"duplicate ID", func(r *translationv1.TranslateRequest) { r.Segments[1].Id = r.Segments[0].Id }, codes.InvalidArgument, "duplicate_segment_id"},
		{"empty text", func(r *translationv1.TranslateRequest) { r.Segments[0].Text = "" }, codes.InvalidArgument, "invalid_segments"},
		{"nul text", func(r *translationv1.TranslateRequest) { r.Segments[0].Text = "a" + string(rune(0)) + "b" }, codes.InvalidArgument, "invalid_segments"},
		{"large segment", func(r *translationv1.TranslateRequest) { r.Segments[0].Text = strings.Repeat("x", MaxSegmentBytes+1) }, codes.ResourceExhausted, "request_too_large"},
		{"too many", func(r *translationv1.TranslateRequest) {
			r.Segments = make([]*translationv1.Segment, MaxSegments+1)
			for i := range r.Segments {
				r.Segments[i] = &translationv1.Segment{Id: "s" + string(rune(0x100+i)), Text: "x"}
			}
		}, codes.ResourceExhausted, "request_too_large"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request := validRequest()
			tc.mutate(request)
			ctx, cancel := authorizedContext(t, runtime)
			defer cancel()
			_, err := service.Translate(ctx, request)
			if status.Code(err) != tc.code || status.Convert(err).Message() != tc.message {
				t.Fatalf("got %v", err)
			}
		})
	}
	if backend.calls.Load() != 0 {
		t.Fatalf("backend called for rejected input: %d", backend.calls.Load())
	}
}

func TestValidRequestPreservesOrderAndMetadata(t *testing.T) {
	runtime := admittedRuntime(t)
	backend := &fakeBackend{translate: func(_ context.Context, in *translationv1.TranslateRequest) (*translationv1.TranslateResponse, error) {
		return translated(in), nil
	}}
	service := newTestService(t, runtime, backend, nil)
	ctx, cancel := authorizedContext(t, runtime)
	defer cancel()
	response, err := service.Translate(ctx, validRequest())
	if err != nil {
		t.Fatal(err)
	}
	if response.GetTargetLanguage() != "es" || len(response.GetResults()) != 2 || response.GetResults()[0].GetId() != "s1" || response.GetResults()[1].GetId() != "s2" {
		t.Fatalf("bad response: %+v", response)
	}
}

func TestHostileBackendResponses(t *testing.T) {
	mutations := []struct {
		name, message string
		mutate        func(*translationv1.TranslateResponse)
	}{
		{"nil", "malformed_backend_response", func(r *translationv1.TranslateResponse) { *r = translationv1.TranslateResponse{} }},
		{"wrong target", "malformed_backend_response", func(r *translationv1.TranslateResponse) { r.TargetLanguage = "fr" }},
		{"missing", "malformed_backend_response", func(r *translationv1.TranslateResponse) { r.Results = r.Results[:1] }},
		{"reordered", "malformed_backend_response", func(r *translationv1.TranslateResponse) { r.Results[0], r.Results[1] = r.Results[1], r.Results[0] }},
		{"duplicate", "malformed_backend_response", func(r *translationv1.TranslateResponse) { r.Results[1].Id = r.Results[0].Id }},
		{"language", "malformed_backend_response", func(r *translationv1.TranslateResponse) { r.Results[0].DetectedSourceLanguage = "EN" }},
		{"ambiguous language", "malformed_backend_response", func(r *translationv1.TranslateResponse) { r.Results[0].DetectedSourceLanguage = "und" }},
		{"model", "malformed_backend_response", func(r *translationv1.TranslateResponse) { r.Results[0].ModelIdentity = "other.model.v1" }},
		{"quality", "malformed_backend_response", func(r *translationv1.TranslateResponse) { r.Results[0].QualityTier = 0 }},
		{"unadmitted quality", "malformed_backend_response", func(r *translationv1.TranslateResponse) {
			r.Results[0].QualityTier = translationv1.QualityTier_QUALITY_TIER_EVALUATED
		}},
		{"class", "malformed_backend_response", func(r *translationv1.TranslateResponse) { r.Results[0].ResultClass = 0 }},
		{"expansion", "output_too_large", func(r *translationv1.TranslateResponse) { r.Results[0].Text = strings.Repeat("x", 257) }},
	}
	for _, tc := range mutations {
		t.Run(tc.name, func(t *testing.T) {
			runtime := admittedRuntime(t)
			backend := &fakeBackend{translate: func(_ context.Context, in *translationv1.TranslateRequest) (*translationv1.TranslateResponse, error) {
				r := translated(in)
				tc.mutate(r)
				return r, nil
			}}
			service := newTestService(t, runtime, backend, nil)
			ctx, cancel := authorizedContext(t, runtime)
			defer cancel()
			_, err := service.Translate(ctx, validRequest())
			if status.Convert(err).Message() != tc.message {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestEvaluatedQualityRequiresExactAdmission(t *testing.T) {
	runtime := admittedRuntime(t)
	runtime.Config.BackendQualityTier = "evaluated"
	backend := &fakeBackend{translate: func(_ context.Context, in *translationv1.TranslateRequest) (*translationv1.TranslateResponse, error) {
		response := translated(in)
		for _, result := range response.Results {
			result.QualityTier = translationv1.QualityTier_QUALITY_TIER_EVALUATED
		}
		return response, nil
	}}
	service := newTestService(t, runtime, backend, nil)
	ctx, cancel := authorizedContext(t, runtime)
	defer cancel()
	if _, err := service.Translate(ctx, validRequest()); err != nil {
		t.Fatalf("evaluated tier rejected: %v", err)
	}
}

func TestTimeoutCancellationAndCapacity(t *testing.T) {
	runtime := admittedRuntime(t)
	runtime.Config.BackendTimeoutMS = 100
	runtime.Config.MaxConcurrent = 1
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	backend := &fakeBackend{translate: func(ctx context.Context, in *translationv1.TranslateRequest) (*translationv1.TranslateResponse, error) {
		once.Do(func() { close(entered) })
		select {
		case <-release:
			return translated(in), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}}
	service := newTestService(t, runtime, backend, nil)
	ctx1, cancel1 := authorizedContext(t, runtime)
	defer cancel1()
	done := make(chan error, 1)
	go func() { _, err := service.Translate(ctx1, validRequest()); done <- err }()
	<-entered
	ctx2, cancel2 := authorizedContext(t, runtime)
	defer cancel2()
	if _, err := service.Translate(ctx2, validRequest()); status.Code(err) != codes.ResourceExhausted || status.Convert(err).Message() != "capacity_exhausted" {
		t.Fatalf("capacity: %v", err)
	}
	if err := <-done; status.Code(err) != codes.DeadlineExceeded || status.Convert(err).Message() != "backend_timeout" {
		t.Fatalf("timeout: %v", err)
	}
	close(release)

	cancelBackend := &fakeBackend{translate: func(ctx context.Context, _ *translationv1.TranslateRequest) (*translationv1.TranslateResponse, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	service = newTestService(t, admittedRuntime(t), cancelBackend, nil)
	base, cancel := authorizedContext(t, admittedRuntime(t))
	cancel()
	if _, err := service.Translate(base, validRequest()); status.Code(err) != codes.Canceled {
		t.Fatalf("cancel: %v", err)
	}
}

func TestPrivacyRecorderNeverContainsContent(t *testing.T) {
	runtime := admittedRuntime(t)
	backend := &fakeBackend{translate: func(_ context.Context, in *translationv1.TranslateRequest) (*translationv1.TranslateResponse, error) {
		return translated(in), nil
	}}
	var output bytes.Buffer
	service := newTestService(t, runtime, backend, NewLogRecorder(&output))
	request := validRequest()
	request.Segments[0].Text = "TOP-SECRET-SOURCE"
	ctx, cancel := authorizedContext(t, runtime)
	defer cancel()
	response, err := service.Translate(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Close(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "TOP-SECRET-SOURCE") || strings.Contains(output.String(), response.Results[0].Text) || strings.Contains(output.String(), request.CorrelationId) {
		t.Fatalf("content leaked: %s", output.String())
	}
	if !strings.Contains(output.String(), digestCorrelation(request.CorrelationId)) {
		t.Fatalf("missing digest: %s", output.String())
	}
}

func TestBlockedAuditWriterCannotDelayRPCOrShutdown(t *testing.T) {
	runtime := admittedRuntime(t)
	backend := &fakeBackend{translate: func(_ context.Context, in *translationv1.TranslateRequest) (*translationv1.TranslateResponse, error) {
		return translated(in), nil
	}}
	writer := &blockedWriter{started: make(chan struct{}), release: make(chan struct{})}
	recorder := NewLogRecorder(writer)
	recorder.Record(Event{Name: "prime"})
	<-writer.started
	for i := 0; i < auditQueueCapacity+1; i++ {
		recorder.Record(Event{Name: "queued"})
	}
	if recorder.Stats().Dropped == 0 {
		t.Fatal("bounded audit overflow was not accounted")
	}
	service := newTestService(t, runtime, backend, recorder)
	ctx, cancel := authorizedContext(t, runtime)
	started := time.Now()
	_, err := service.Translate(ctx, validRequest())
	cancel()
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed > 100*time.Millisecond {
		t.Fatalf("blocked audit delayed RPC: %s", elapsed)
	}
	started = time.Now()
	if err := recorder.Close(); err == nil {
		t.Fatal("blocked audit close unexpectedly completed")
	}
	if elapsed := time.Since(started); elapsed > 250*time.Millisecond {
		t.Fatalf("blocked audit delayed shutdown: %s", elapsed)
	}
	close(writer.release)
}

func TestExactRequestBounds(t *testing.T) {
	request := validRequest()
	request.Segments = []*translationv1.Segment{
		{Id: "a", Text: strings.Repeat("a", MaxSegmentBytes)},
		{Id: "b", Text: strings.Repeat("b", MaxSegmentBytes)},
		{Id: "c", Text: strings.Repeat("c", MaxSegmentBytes)},
		{Id: "d", Text: strings.Repeat("d", MaxSegmentBytes-1)},
		{Id: strings.Repeat("e", MaxSegmentIDBytes), Text: "e"},
	}
	if got := validateRequest(request); got != "" {
		t.Fatalf("exact source/id bounds rejected: %s", got)
	}
	request.Segments[4].Text = "ee"
	if got := validateRequest(request); got != "request_too_large" {
		t.Fatalf("source overflow: %s", got)
	}
	request = validRequest()
	request.Segments[0].Id = strings.Repeat("i", MaxSegmentIDBytes+1)
	if got := validateRequest(request); got != "invalid_segments" {
		t.Fatalf("segment id overflow: %s", got)
	}
	request = validRequest()
	request.CorrelationId += "0"
	if got := validateRequest(request); got != "invalid_correlation_id" {
		t.Fatalf("correlation length overflow: %s", got)
	}
}

func TestBackendResourceExhaustedMapsToOutputTooLarge(t *testing.T) {
	runtime := admittedRuntime(t)
	backend := &fakeBackend{translate: func(context.Context, *translationv1.TranslateRequest) (*translationv1.TranslateResponse, error) {
		return nil, status.Error(codes.ResourceExhausted, "grpc transport receive limit")
	}}
	service := newTestService(t, runtime, backend, nil)
	ctx, cancel := authorizedContext(t, runtime)
	defer cancel()
	_, err := service.Translate(ctx, validRequest())
	if status.Code(err) != codes.ResourceExhausted || status.Convert(err).Message() != "output_too_large" {
		t.Fatalf("oversized backend mapping: %v", err)
	}
}

func TestCanonicalLanguageBoundary(t *testing.T) {
	cases := map[string]bool{
		"en": true, "en-US": true, "zh-Hant": true, "sr-Latn-RS": true,
		"": false, "auto": false, "und": false, "und-US": false, "und-Latn": false, "EN-us": false, "en-us": false, "iw": false, "not_a_tag": false,
	}
	for value, want := range cases {
		if got := canonicalLanguage(value); got != want {
			t.Errorf("canonicalLanguage(%q) = %v, want %v", value, got, want)
		}
	}
}

func TestValidationAllocationAndLatencyBounds(t *testing.T) {
	request := validRequest()
	allocations := testing.AllocsPerRun(1000, func() {
		if got := validateRequest(request); got != "" {
			panic(got)
		}
	})
	if allocations > 6 {
		t.Fatalf("validation allocations %.1f > 6", allocations)
	}
	start := time.Now()
	for i := 0; i < 10_000; i++ {
		_ = validateRequest(request)
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("validation latency %s", elapsed)
	}
}

func FuzzValidateRequest(f *testing.F) {
	f.Add("en", "es", "id", "hello")
	f.Fuzz(func(t *testing.T, source, target, id, text string) {
		request := validRequest()
		request.SourceLanguage = source
		request.TargetLanguage = target
		request.Segments = []*translationv1.Segment{{Id: id, Text: text}}
		_ = validateRequest(request)
	})
}

func BenchmarkValidateRequest(b *testing.B) {
	request := validRequest()
	b.ReportAllocs()
	for b.Loop() {
		_ = validateRequest(request)
	}
}
