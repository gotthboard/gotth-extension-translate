package translate

import (
	"errors"
	"reflect"

	extensioncore "github.com/gotthboard/gotth-extensions/pkg/extensions"
)

const (
	ExtensionID         = "gotth.extension.translate"
	InterfaceName       = "gotth.interface.translation"
	TranslateCapability = "translate.text"
	ConfigSchema        = "gotth.extension.translate.config.v1"
)

var BuildVersion = "dev"

func Manifest(version string) (extensioncore.Manifest, error) {
	manifestVersion, ok := manifestVersion(version)
	if !ok {
		return extensioncore.Manifest{}, extensioncore.ErrInvalidInput
	}
	m := extensioncore.Manifest{
		Schema:       extensioncore.ManifestSchema,
		ID:           ExtensionID,
		Name:         "GOTTH Translation",
		Version:      manifestVersion,
		Protocols:    []extensioncore.VersionRange{{Name: extensioncore.ControlName, Major: 1, MinMinor: 0, MaxMinor: 0}},
		Interfaces:   []extensioncore.VersionRange{{Name: InterfaceName, Major: 1, MinMinor: 0, MaxMinor: 0}},
		Capabilities: []string{TranslateCapability},
		Secrets:      []extensioncore.SecretRequirement{},
	}
	if err := extensioncore.ValidateManifest(m); err != nil {
		return extensioncore.Manifest{}, err
	}
	return m, nil
}

func hostProfile() extensioncore.HostProfile {
	return extensioncore.HostProfile{
		Protocols:  []extensioncore.VersionRange{{Name: extensioncore.ControlName, Major: 1, MinMinor: 0, MaxMinor: 0}},
		Interfaces: []extensioncore.VersionRange{{Name: InterfaceName, Major: 1, MinMinor: 0, MaxMinor: 0}},
	}
}

func negotiateExact(manifest extensioncore.Manifest, grant extensioncore.Grant) (extensioncore.Session, error) {
	session, err := extensioncore.Negotiate(manifest, grant, hostProfile())
	if err != nil {
		return extensioncore.Session{}, err
	}
	if len(session.Interfaces) != 1 || session.Interfaces[0] != (extensioncore.InterfaceGrant{Name: InterfaceName, Major: 1, Minor: 0}) {
		return extensioncore.Session{}, extensioncore.ErrUngrantedInterface
	}
	if len(session.Capabilities) != 1 || session.Capabilities[0] != TranslateCapability {
		return extensioncore.Session{}, extensioncore.ErrUngrantedCapability
	}
	if len(session.Secrets) != 0 {
		return extensioncore.Session{}, extensioncore.ErrUngrantedSecret
	}
	return session, nil
}

func sameSession(left, right extensioncore.Session) bool { return reflect.DeepEqual(left, right) }

func manifestVersion(version string) (string, bool) {
	if version == "dev" {
		return "0.0.0-dev", true
	}
	if version == "1.0.0" {
		return version, true
	}
	for _, prefix := range []string{"1.0.0-alpha.", "1.0.0-beta.", "1.0.0-rc."} {
		if len(version) > len(prefix) && version[:len(prefix)] == prefix {
			n := version[len(prefix):]
			if n[0] == '0' {
				return "", false
			}
			for _, c := range n {
				if c < '0' || c > '9' {
					return "", false
				}
			}
			return version, true
		}
	}
	return "", false
}

var errSessionChanged = errors.New("authoritative extension session changed")
