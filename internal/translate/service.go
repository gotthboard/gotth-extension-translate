package translate

import (
	"context"
	"crypto/subtle"
	"errors"
	"strings"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"

	translationv1 "github.com/gotthboard/gotth-extension-translate/proto/gotth/translation/v1"
	extensioncore "github.com/gotthboard/gotth-extensions/pkg/extensions"
	extensionsv1 "github.com/gotthboard/gotth-sdk/gen/go/gotth/extensions/v1"
	"golang.org/x/text/language"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

const (
	MaxSegments              = 256
	MaxSegmentBytes          = 64 << 10
	MaxSourceBytes           = 256 << 10
	MaxRequestEnvelopeBytes  = 320 << 10
	MaxResponseEnvelopeBytes = 1 << 20
	MaxOutputExpansion       = 4
	MaxSegmentIDBytes        = 128
	MaxCorrelationIDBytes    = 36
)

const (
	serviceStarting int32 = iota
	serviceReady
	serviceStopping
	serviceFailed
)

type Service struct {
	extensionsv1.UnimplementedExtensionControlServer
	translationv1.UnimplementedTranslationServer
	runtime  RuntimeConfig
	backend  Backend
	recorder Recorder
	capacity chan struct{}
	state    atomic.Int32
}

func NewService(runtime RuntimeConfig, backend Backend, recorder Recorder) (*Service, error) {
	if backend == nil || runtime.ServiceToken == "" || runtime.Version == "" || runtime.Config.MaxConcurrent < 1 {
		return nil, errors.New("complete translation runtime required")
	}
	owned := cloneRuntime(runtime)
	fresh, err := negotiateExact(owned.Manifest, owned.Grant)
	if err != nil || !sameSession(fresh, owned.Session) {
		return nil, errSessionChanged
	}
	if recorder == nil {
		recorder = discardRecorder{}
	}
	s := &Service{runtime: owned, backend: backend, recorder: recorder, capacity: make(chan struct{}, owned.Config.MaxConcurrent)}
	s.state.Store(serviceStarting)
	return s, nil
}

func (s *Service) Close() error {
	backendErr := s.backend.Close()
	if closer, ok := s.recorder.(interface{ Close() error }); ok {
		if auditErr := closer.Close(); backendErr == nil {
			backendErr = auditErr
		}
	}
	return backendErr
}
func (s *Service) SetReady()    { s.state.Store(serviceReady) }
func (s *Service) SetStopping() { s.state.Store(serviceStopping) }
func (s *Service) SetFailed()   { s.state.Store(serviceFailed) }

// UnaryInterceptor enforces deadline and service-token admission before any
// host RPC application handler is invoked.
func (s *Service) UnaryInterceptor(ctx context.Context, request any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	if err := s.authorizeTransport(ctx); err != nil {
		return nil, err
	}
	return handler(ctx, request)
}

func (s *Service) authorizeTransport(ctx context.Context) error {
	if _, ok := ctx.Deadline(); !ok {
		return status.Error(codes.InvalidArgument, "deadline_required")
	}
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return status.Error(codes.Unauthenticated, "transport_identity_required")
	}
	values := md.Get("x-gotth-service-token")
	if len(values) != 1 || subtle.ConstantTimeCompare([]byte(values[0]), []byte(s.runtime.ServiceToken)) != 1 {
		return status.Error(codes.Unauthenticated, "invalid_transport_identity")
	}
	return nil
}

func (s *Service) authorizeRequest(ctx context.Context, correlationID string, requireSession bool) error {
	if err := s.authorizeTransport(ctx); err != nil {
		return err
	}
	if !validUUID(correlationID) {
		return status.Error(codes.InvalidArgument, "invalid_correlation_id")
	}
	fresh, err := negotiateExact(s.runtime.Manifest, s.runtime.Grant)
	if err != nil || !sameSession(fresh, s.runtime.Session) {
		return status.Error(codes.FailedPrecondition, "session_binding_changed")
	}
	if requireSession {
		md, _ := metadata.FromIncomingContext(ctx)
		sessions := md.Get("x-gotth-session-sha256")
		if len(sessions) != 1 || subtle.ConstantTimeCompare([]byte(sessions[0]), []byte(fresh.Fingerprint)) != 1 {
			return status.Error(codes.PermissionDenied, "session_binding_mismatch")
		}
	}
	return nil
}

func (s *Service) Handshake(ctx context.Context, request *extensionsv1.HandshakeRequest) (*extensionsv1.HandshakeResponse, error) {
	if request == nil || len(request.GetChallenge()) != 32 {
		return nil, status.Error(codes.InvalidArgument, "invalid_handshake")
	}
	if err := s.authorizeRequest(ctx, request.GetCorrelationId(), false); err != nil {
		return nil, err
	}
	binding := s.runtime.Session
	if request.GetExpectedExtensionId() != ExtensionID || request.GetExpectedManifestSha256() != binding.ManifestDigest || request.GetExpectedGrantSha256() != binding.GrantDigest || request.GetExpectedSessionSha256() != binding.Fingerprint {
		return nil, status.Error(codes.FailedPrecondition, "extension_binding_mismatch")
	}
	return &extensionsv1.HandshakeResponse{
		Challenge: append([]byte(nil), request.GetChallenge()...), ExtensionId: ExtensionID, ExtensionVersion: s.runtime.Version,
		ManifestSha256: binding.ManifestDigest, GrantSha256: binding.GrantDigest, SessionSha256: binding.Fingerprint,
		Control:      &extensionsv1.ProtocolVersion{Name: extensioncore.ControlName, Major: 1, Minor: 0},
		Interfaces:   []*extensionsv1.InterfaceVersion{{Name: InterfaceName, Major: 1, Minor: 0}},
		Capabilities: []string{TranslateCapability},
	}, nil
}

func (s *Service) Health(ctx context.Context, request *extensionsv1.HealthRequest) (*extensionsv1.HealthResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid_health_request")
	}
	if err := s.authorizeRequest(ctx, request.GetCorrelationId(), false); err != nil {
		return nil, err
	}
	if subtle.ConstantTimeCompare([]byte(request.GetSessionSha256()), []byte(s.runtime.Session.Fingerprint)) != 1 {
		return nil, status.Error(codes.FailedPrecondition, "session_binding_mismatch")
	}
	state, code := extensionsv1.HealthState_HEALTH_STATE_FAILED, "extension.failed"
	switch s.state.Load() {
	case serviceStarting:
		state, code = extensionsv1.HealthState_HEALTH_STATE_STARTING, "extension.starting"
	case serviceReady:
		state, code = extensionsv1.HealthState_HEALTH_STATE_READY, "extension.ready"
	case serviceStopping:
		state, code = extensionsv1.HealthState_HEALTH_STATE_STOPPING, "extension.stopping"
	}
	return &extensionsv1.HealthResponse{State: state, Code: code}, nil
}

func (s *Service) Translate(ctx context.Context, request *translationv1.TranslateRequest) (response *translationv1.TranslateResponse, returnErr error) {
	started := time.Now()
	code := "translated"
	segmentCount, sourceBytes, outputBytes := 0, 0, 0
	correlation := ""
	if request != nil {
		correlation = request.GetCorrelationId()
		segmentCount = len(request.GetSegments())
		sourceBytes = requestSourceBytes(request)
	}
	defer func() {
		if returnErr != nil {
			code = status.Convert(returnErr).Message()
		}
		if response != nil {
			outputBytes = responseTextBytes(response)
		}
		s.recorder.Record(Event{Name: "translation.request", Code: code, CorrelationDigest: digestCorrelation(correlation), SegmentCount: segmentCount, SourceBytes: sourceBytes, OutputBytes: outputBytes, DurationBucket: latencyBucket(time.Since(started))})
	}()
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid_request")
	}
	if err := s.authorizeRequest(ctx, request.GetCorrelationId(), true); err != nil {
		return nil, err
	}
	if reason := validateRequest(request); reason != "" {
		if reason == "request_too_large" {
			return nil, status.Error(codes.ResourceExhausted, reason)
		}
		return nil, status.Error(codes.InvalidArgument, reason)
	}
	select {
	case s.capacity <- struct{}{}:
		defer func() { <-s.capacity }()
	default:
		return nil, status.Error(codes.ResourceExhausted, "capacity_exhausted")
	}
	backendContext, cancel := context.WithTimeout(ctx, time.Duration(s.runtime.Config.BackendTimeoutMS)*time.Millisecond)
	defer cancel()
	backendResponse, err := s.backend.Translate(backendContext, request)
	if err != nil {
		switch {
		case status.Code(err) == codes.ResourceExhausted:
			return nil, status.Error(codes.ResourceExhausted, "output_too_large")
		case errors.Is(ctx.Err(), context.Canceled):
			return nil, status.Error(codes.Canceled, "request_cancelled")
		case errors.Is(ctx.Err(), context.DeadlineExceeded), errors.Is(backendContext.Err(), context.DeadlineExceeded):
			return nil, status.Error(codes.DeadlineExceeded, "backend_timeout")
		default:
			return nil, status.Error(codes.Unavailable, "backend_unavailable")
		}
	}
	if reason := validateResponse(request, backendResponse, s.runtime.Config.BackendModelIdentity, s.runtime.Config.BackendQualityTier); reason != "" {
		if reason == "output_too_large" {
			return nil, status.Error(codes.ResourceExhausted, reason)
		}
		return nil, status.Error(codes.DataLoss, reason)
	}
	return cloneResponse(backendResponse), nil
}

func validateRequest(request *translationv1.TranslateRequest) string {
	if len(request.ProtoReflect().GetUnknown()) != 0 {
		return "invalid_request"
	}
	if proto.Size(request) > MaxRequestEnvelopeBytes {
		return "request_too_large"
	}
	if !validUUID(request.GetCorrelationId()) {
		return "invalid_correlation_id"
	}
	if request.GetSourceLanguage() != "auto" && !canonicalLanguage(request.GetSourceLanguage()) {
		return "unsupported_language"
	}
	if !canonicalLanguage(request.GetTargetLanguage()) {
		return "unsupported_language"
	}
	switch request.GetContentKind() {
	case translationv1.ContentKind_CONTENT_KIND_PLAIN, translationv1.ContentKind_CONTENT_KIND_MAIL_VISIBLE_TEXT, translationv1.ContentKind_CONTENT_KIND_MARKDOWN_SEGMENT, translationv1.ContentKind_CONTENT_KIND_DOCUMENT_TEXT:
	default:
		return "unsupported_content_kind"
	}
	switch request.GetCacheClass() {
	case translationv1.CacheClass_CACHE_CLASS_NONE, translationv1.CacheClass_CACHE_CLASS_REQUEST:
	default:
		return "unsupported_retention"
	}
	if len(request.GetSegments()) == 0 {
		return "invalid_segments"
	}
	if len(request.GetSegments()) > MaxSegments {
		return "request_too_large"
	}
	seen := make(map[string]struct{}, len(request.GetSegments()))
	total := 0
	for _, segment := range request.GetSegments() {
		if segment == nil || len(segment.ProtoReflect().GetUnknown()) != 0 || !validOpaqueID(segment.GetId()) || !validText(segment.GetText()) {
			return "invalid_segments"
		}
		if _, exists := seen[segment.GetId()]; exists {
			return "duplicate_segment_id"
		}
		seen[segment.GetId()] = struct{}{}
		if len(segment.GetText()) > MaxSegmentBytes {
			return "request_too_large"
		}
		total += len(segment.GetText())
		if total > MaxSourceBytes {
			return "request_too_large"
		}
	}
	return ""
}

func validateResponse(request *translationv1.TranslateRequest, response *translationv1.TranslateResponse, modelIdentity, qualityTier string) string {
	if response == nil || len(response.ProtoReflect().GetUnknown()) != 0 {
		return "malformed_backend_response"
	}
	if proto.Size(response) > MaxResponseEnvelopeBytes {
		return "output_too_large"
	}
	if response.GetTargetLanguage() != request.GetTargetLanguage() || len(response.GetResults()) != len(request.GetSegments()) {
		return "malformed_backend_response"
	}
	totalSource, totalOutput := requestSourceBytes(request), 0
	for i, result := range response.GetResults() {
		source := request.GetSegments()[i]
		if result == nil || len(result.ProtoReflect().GetUnknown()) != 0 || result.GetId() != source.GetId() || !canonicalLanguage(result.GetDetectedSourceLanguage()) || !validText(result.GetText()) {
			return "malformed_backend_response"
		}
		if request.GetSourceLanguage() != "auto" && result.GetDetectedSourceLanguage() != request.GetSourceLanguage() {
			return "malformed_backend_response"
		}
		if result.GetModelIdentity() != modelIdentity || result.GetResultClass() != translationv1.ResultClass_RESULT_CLASS_TRANSLATED {
			return "malformed_backend_response"
		}
		expectedTier := translationv1.QualityTier_QUALITY_TIER_EXPERIMENTAL
		if qualityTier == "evaluated" {
			expectedTier = translationv1.QualityTier_QUALITY_TIER_EVALUATED
		}
		if result.GetQualityTier() != expectedTier {
			return "malformed_backend_response"
		}
		limit := len(source.GetText()) * MaxOutputExpansion
		if limit < 256 {
			limit = 256
		}
		if limit > MaxSourceBytes {
			limit = MaxSourceBytes
		}
		if len(result.GetText()) > limit {
			return "output_too_large"
		}
		totalOutput += len(result.GetText())
	}
	totalLimit := totalSource * MaxOutputExpansion
	if totalLimit < 1024 {
		totalLimit = 1024
	}
	if totalOutput > totalLimit || totalOutput > MaxResponseEnvelopeBytes {
		return "output_too_large"
	}
	return ""
}

func cloneResponse(in *translationv1.TranslateResponse) *translationv1.TranslateResponse {
	out := &translationv1.TranslateResponse{TargetLanguage: in.GetTargetLanguage(), Results: make([]*translationv1.SegmentResult, len(in.GetResults()))}
	for i, item := range in.GetResults() {
		out.Results[i] = &translationv1.SegmentResult{Id: item.GetId(), DetectedSourceLanguage: item.GetDetectedSourceLanguage(), Text: item.GetText(), QualityTier: item.GetQualityTier(), ModelIdentity: item.GetModelIdentity(), ResultClass: item.GetResultClass()}
	}
	return out
}

func canonicalLanguage(value string) bool {
	if len(value) == 0 || len(value) > 63 || !utf8.ValidString(value) {
		return false
	}
	tag, err := language.Parse(value)
	if err != nil || tag.String() != value {
		return false
	}
	base, _, _ := tag.Raw()
	return base.String() != "und"
}

func validOpaqueID(value string) bool {
	if len(value) == 0 || len(value) > MaxSegmentIDBytes || !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return false
		}
	}
	return true
}

func validText(value string) bool {
	return value != "" && utf8.ValidString(value) && !strings.ContainsRune(value, 0)
}

func validUUID(value string) bool {
	if len(value) != MaxCorrelationIDBytes || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	for i, c := range value {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			continue
		}
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func requestSourceBytes(request *translationv1.TranslateRequest) int {
	total := 0
	for _, segment := range request.GetSegments() {
		total += len(segment.GetText())
	}
	return total
}
func responseTextBytes(response *translationv1.TranslateResponse) int {
	total := 0
	for _, result := range response.GetResults() {
		total += len(result.GetText())
	}
	return total
}
