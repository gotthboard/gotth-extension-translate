package translate

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"unicode/utf8"

	extensioncore "github.com/gotthboard/gotth-extensions/pkg/extensions"
)

const maxRuntimeFileBytes = 64 << 10

type Config struct {
	Schema               string
	BackendPeerPID       int
	BackendPeerUID       uint32
	BackendPeerGID       uint32
	BackendModelIdentity string
	BackendQualityTier   string
	BackendTimeoutMS     int
	MaxConcurrent        int
}

type RuntimeConfig struct {
	Config       Config
	Manifest     extensioncore.Manifest
	Grant        extensioncore.Grant
	Session      extensioncore.Session
	ServiceToken string
	BackendToken string
	BackendFD    int
	SocketPath   string
	Version      string
}

func LoadRuntimeFromEnv() (RuntimeConfig, error) {
	paths := map[string]string{
		"configuration": os.Getenv("GOTTH_EXTENSION_CONFIG_FILE"),
		"manifest":      os.Getenv("GOTTH_EXTENSION_MANIFEST_FILE"),
		"grant":         os.Getenv("GOTTH_EXTENSION_GRANT_FILE"),
		"service token": os.Getenv("GOTTH_EXTENSION_SERVICE_TOKEN_FILE"),
		"backend token": os.Getenv("GOTTH_TRANSLATE_SERVICE_TOKEN_FILE"),
	}
	socket := os.Getenv("GOTTH_EXTENSION_SOCKET")
	backendFDText := os.Getenv("GOTTH_TRANSLATE_BACKEND_FD")
	backendFD64, backendFDErr := strconv.ParseUint(backendFDText, 10, 31)
	if _, ok := manifestVersion(BuildVersion); !ok || socket == "" || backendFDErr != nil || backendFD64 < 3 {
		return RuntimeConfig{}, errors.New("complete extension runtime configuration required")
	}
	for _, name := range []string{"configuration", "manifest", "grant", "service token", "backend token"} {
		if paths[name] == "" {
			return RuntimeConfig{}, fmt.Errorf("%s file required", name)
		}
	}
	configData, err := readPrivateFile(paths["configuration"], maxRuntimeFileBytes)
	if err != nil {
		return RuntimeConfig{}, fmt.Errorf("read configuration: %w", err)
	}
	manifestData, err := readPrivateFile(paths["manifest"], extensioncore.MaxDocumentBytes)
	if err != nil {
		return RuntimeConfig{}, fmt.Errorf("read manifest: %w", err)
	}
	grantData, err := readPrivateFile(paths["grant"], extensioncore.MaxDocumentBytes)
	if err != nil {
		return RuntimeConfig{}, fmt.Errorf("read grant: %w", err)
	}
	serviceToken, err := readPrivateToken(paths["service token"])
	if err != nil {
		return RuntimeConfig{}, errors.New("valid host service token required")
	}
	backendToken, err := readPrivateToken(paths["backend token"])
	if err != nil {
		return RuntimeConfig{}, errors.New("valid backend service token required")
	}
	config, err := decodeConfig(configData)
	if err != nil {
		return RuntimeConfig{}, err
	}
	manifest, err := extensioncore.ParseManifest(manifestData)
	if err != nil {
		return RuntimeConfig{}, errors.New("valid canonical extension manifest required")
	}
	expected, err := Manifest(BuildVersion)
	if err != nil || !reflectManifestEqual(manifest, expected) {
		return RuntimeConfig{}, errors.New("artifact manifest binding mismatch")
	}
	canonical, err := extensioncore.CanonicalManifest(manifest)
	if err != nil || !bytes.Equal(canonical, manifestData) {
		return RuntimeConfig{}, errors.New("canonical extension manifest required")
	}
	grant, err := extensioncore.ParseGrant(grantData)
	if err != nil {
		return RuntimeConfig{}, errors.New("valid extension grant required")
	}
	canonicalGrant, err := extensioncore.CanonicalGrant(grant)
	if err != nil || !bytes.Equal(canonicalGrant, grantData) {
		return RuntimeConfig{}, errors.New("canonical extension grant required")
	}
	session, err := negotiateExact(manifest, grant)
	if err != nil {
		return RuntimeConfig{}, errors.New("admitted translation session required")
	}
	if err := validateNewSocketPath(socket); err != nil {
		return RuntimeConfig{}, err
	}
	return RuntimeConfig{Config: config, Manifest: manifest, Grant: grant, Session: session, ServiceToken: serviceToken, BackendToken: backendToken, BackendFD: int(backendFD64), SocketPath: socket, Version: BuildVersion}, nil
}

func reflectManifestEqual(left, right extensioncore.Manifest) bool {
	lb, err1 := extensioncore.CanonicalManifest(left)
	rb, err2 := extensioncore.CanonicalManifest(right)
	return err1 == nil && err2 == nil && bytes.Equal(lb, rb)
}

func decodeConfig(data []byte) (Config, error) {
	fields, err := decodeUniqueObject(data)
	if err != nil || len(fields) != 8 {
		return Config{}, errors.New("invalid extension configuration")
	}
	var c Config
	if decodeField(fields, "schema", &c.Schema) != nil || c.Schema != ConfigSchema {
		return Config{}, errors.New("invalid configuration schema")
	}
	if decodeField(fields, "backend_peer_pid", &c.BackendPeerPID) != nil || c.BackendPeerPID < 1 {
		return Config{}, errors.New("valid backend peer pid required")
	}
	if decodeField(fields, "backend_peer_uid", &c.BackendPeerUID) != nil {
		return Config{}, errors.New("valid backend peer uid required")
	}
	if decodeField(fields, "backend_peer_gid", &c.BackendPeerGID) != nil {
		return Config{}, errors.New("valid backend peer gid required")
	}
	if decodeField(fields, "backend_model_identity", &c.BackendModelIdentity) != nil || !validDottedIdentity(c.BackendModelIdentity) {
		return Config{}, errors.New("valid backend model identity required")
	}
	if decodeField(fields, "backend_quality_tier", &c.BackendQualityTier) != nil || c.BackendQualityTier != "experimental" && c.BackendQualityTier != "evaluated" {
		return Config{}, errors.New("valid backend quality tier required")
	}
	if decodeField(fields, "backend_timeout_ms", &c.BackendTimeoutMS) != nil || c.BackendTimeoutMS < 100 || c.BackendTimeoutMS > 10_000 {
		return Config{}, errors.New("backend timeout must be 100..10000 milliseconds")
	}
	if decodeField(fields, "max_concurrent", &c.MaxConcurrent) != nil || c.MaxConcurrent < 1 || c.MaxConcurrent > 32 {
		return Config{}, errors.New("max concurrency must be 1..32")
	}
	return c, nil
}

func decodeUniqueObject(data []byte) (map[string]json.RawMessage, error) {
	if len(data) == 0 || len(data) > maxRuntimeFileBytes || !utf8.Valid(data) {
		return nil, errors.New("bounded UTF-8 JSON object required")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, errors.New("JSON object required")
	}
	fields := make(map[string]json.RawMessage)
	for decoder.More() {
		nameToken, err := decoder.Token()
		name, ok := nameToken.(string)
		if err != nil || !ok {
			return nil, errors.New("invalid JSON object")
		}
		if _, exists := fields[name]; exists {
			return nil, errors.New("duplicate JSON field")
		}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return nil, errors.New("invalid JSON field")
		}
		fields[name] = raw
	}
	if token, err = decoder.Token(); err != nil || token != json.Delim('}') {
		return nil, errors.New("invalid JSON object")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("trailing JSON input")
	}
	return fields, nil
}

func decodeField(fields map[string]json.RawMessage, name string, target any) error {
	raw, ok := fields[name]
	if !ok {
		return errors.New("missing field")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func readPrivateToken(path string) (string, error) {
	data, err := readPrivateFile(path, 256)
	if err != nil || len(data) < 32 || len(data) > 256 || !utf8.Valid(data) || !bytes.Equal(bytes.TrimSpace(data), data) {
		return "", errors.New("invalid token")
	}
	for _, b := range data {
		if b < 0x21 || b > 0x7e {
			return "", errors.New("invalid token")
		}
	}
	return string(data), nil
}

func readPrivateFile(path string, limit int) ([]byte, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || !pathHasNoSymlink(path) {
		return nil, errors.New("canonical absolute non-symlinked path required")
	}
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 || !ownedByEUID(info) {
		return nil, errors.New("private owned regular file required")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) || !opened.Mode().IsRegular() || opened.Mode().Perm()&0o077 != 0 || !ownedByEUID(opened) {
		return nil, errors.New("private file changed during open")
	}
	data, err := io.ReadAll(io.LimitReader(file, int64(limit)+1))
	if err != nil || len(data) > limit {
		return nil, errors.New("bounded private file required")
	}
	return data, nil
}

func ownedByEUID(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uint32(os.Geteuid())
}

func canonicalSocketPath(path string) bool {
	return filepath.IsAbs(path) && filepath.Clean(path) == path && len(path) <= 100
}

func validatePrivateDir(path string) error {
	if !pathHasNoSymlink(path) {
		return errors.New("non-symlinked socket directory required")
	}
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || info.Mode().Perm()&0o077 != 0 || !ownedByEUID(info) {
		return errors.New("private owned socket directory required")
	}
	return nil
}

func validateNewSocketPath(path string) error {
	if !canonicalSocketPath(path) {
		return errors.New("bounded canonical Unix socket path required")
	}
	if err := validatePrivateDir(filepath.Dir(path)); err != nil {
		return err
	}
	if _, err := os.Lstat(path); err == nil || !errors.Is(err, os.ErrNotExist) {
		return errors.New("extension socket path must not exist")
	}
	return nil
}

func pathHasNoSymlink(path string) bool {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return false
	}
	current := string(filepath.Separator)
	for _, part := range strings.Split(strings.TrimPrefix(path, current), current) {
		if part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return false
		}
	}
	return true
}

func validDottedIdentity(value string) bool {
	if len(value) < 3 || len(value) > 128 || value != strings.ToLower(value) {
		return false
	}
	parts := strings.Split(value, ".")
	if len(parts) < 3 {
		return false
	}
	for _, part := range parts {
		if part == "" || len(part) > 63 || part[0] < 'a' || part[0] > 'z' || part[len(part)-1] == '-' {
			return false
		}
		for _, c := range part {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
				return false
			}
		}
	}
	return true
}
