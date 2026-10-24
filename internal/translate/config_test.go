package translate

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	extensioncore "github.com/gotthboard/gotth-extensions/pkg/extensions"
)

func TestLoadRuntimeStrictCanonicalAdmission(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	extensionPath := filepath.Join(root, "extension.sock")
	manifest, err := Manifest("dev")
	if err != nil {
		t.Fatal(err)
	}
	manifestBytes, err := extensioncore.CanonicalManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := extensioncore.ManifestDigest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	grant := extensioncore.Grant{Schema: extensioncore.GrantSchema, InstanceID: "11111111-1111-1111-1111-111111111111", ExtensionID: ExtensionID, ManifestDigest: digest, Capabilities: []string{TranslateCapability}, Interfaces: []extensioncore.InterfaceGrant{{Name: InterfaceName, Major: 1, Minor: 0}}, Secrets: []string{}}
	grantBytes, err := extensioncore.CanonicalGrant(grant)
	if err != nil {
		t.Fatal(err)
	}
	config := []byte(`{"schema":"gotth.extension.translate.config.v1","backend_peer_pid":` + fmt.Sprint(os.Getpid()) + `,"backend_peer_uid":` + fmt.Sprint(os.Geteuid()) + `,"backend_peer_gid":` + fmt.Sprint(os.Getegid()) + `,"backend_model_identity":"gotth.translate.mock.v1","backend_quality_tier":"experimental","backend_timeout_ms":1000,"max_concurrent":4}`)
	writePrivate := func(name string, data []byte) string {
		t.Helper()
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	paths := map[string]string{"GOTTH_EXTENSION_CONFIG_FILE": writePrivate("config.json", config), "GOTTH_EXTENSION_MANIFEST_FILE": writePrivate("manifest.json", manifestBytes), "GOTTH_EXTENSION_GRANT_FILE": writePrivate("grant.json", grantBytes), "GOTTH_EXTENSION_SERVICE_TOKEN_FILE": writePrivate("host.token", []byte(testHostToken)), "GOTTH_TRANSLATE_SERVICE_TOKEN_FILE": writePrivate("backend.token", []byte(testBackendToken)), "GOTTH_EXTENSION_SOCKET": extensionPath, "GOTTH_TRANSLATE_BACKEND_FD": "3"}
	for key, value := range paths {
		t.Setenv(key, value)
	}
	runtime, err := LoadRuntimeFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if runtime.Session.Fingerprint == "" || runtime.Config.BackendPeerPID != os.Getpid() || runtime.BackendFD != 3 {
		t.Fatalf("bad runtime: %+v", runtime)
	}
	original := paths["GOTTH_EXTENSION_CONFIG_FILE"]
	if err := os.WriteFile(original, []byte(`{"schema":"gotth.extension.translate.config.v1","schema":"gotth.extension.translate.config.v1","backend_peer_pid":1,"backend_peer_uid":0,"backend_peer_gid":0,"backend_model_identity":"gotth.translate.mock.v1","backend_quality_tier":"experimental","backend_timeout_ms":1000,"max_concurrent":4}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRuntimeFromEnv(); err == nil {
		t.Fatal("duplicate configuration field accepted")
	}
}

func TestPrivateTokenRequiresPrintableASCII(t *testing.T) {
	root := t.TempDir()
	for name, value := range map[string]string{
		"unicode": "token-01234567890123456789012345-å",
		"space":   "token-01234567890123456789012345 x",
		"delete":  "token-01234567890123456789012345\x7f",
	} {
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := readPrivateToken(path); err == nil {
			t.Errorf("accepted %s token", name)
		}
	}
	path := filepath.Join(root, "ascii")
	if err := os.WriteFile(path, []byte(testHostToken), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := readPrivateToken(path); err != nil || got != testHostToken {
		t.Fatalf("printable token rejected: %v", err)
	}
}

func TestPrivateFileRejectsSymlinkAndOpenPermissions(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.WriteFile(target, []byte(testHostToken), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := readPrivateFile(link, 256); err == nil {
		t.Fatal("symlink accepted")
	}
	if err := os.Chmod(target, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readPrivateFile(target, 256); err == nil {
		t.Fatal("group/world readable file accepted")
	}
	private := filepath.Join(root, "private")
	if err := os.Mkdir(private, 0o700); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(private, "token")
	if err := os.WriteFile(secret, []byte(testHostToken), 0o600); err != nil {
		t.Fatal(err)
	}
	parentLink := filepath.Join(root, "parent-link")
	if err := os.Symlink(private, parentLink); err != nil {
		t.Fatal(err)
	}
	if _, err := readPrivateFile(filepath.Join(parentLink, "token"), 256); err == nil {
		t.Fatal("symlinked parent path accepted")
	}
}
