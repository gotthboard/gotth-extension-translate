package translate

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	translationv1 "github.com/gotthboard/gotth-extension-translate/proto/gotth/translation/v1"
	extensionsv1 "github.com/gotthboard/gotth-sdk/gen/go/gotth/extensions/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

func TestTranslationDescriptorIsNarrow(t *testing.T) {
	file := translationv1.File_proto_gotth_translation_v1_translation_proto
	if string(file.Package()) != "gotth.translation.v1" || file.Services().Len() != 2 || file.Services().Get(0).Methods().Len() != 1 || file.Services().Get(1).Methods().Len() != 1 {
		t.Fatalf("unexpected service descriptor: %s", file.FullName())
	}
	forbidden := []string{"mime", "attachment", "header", "credential", "token", "secret", "url", "path", "object_key", "database", "callback", "html", "metadata", "payload"}
	var scan func(protoreflect.MessageDescriptors)
	scan = func(messages protoreflect.MessageDescriptors) {
		for i := 0; i < messages.Len(); i++ {
			message := messages.Get(i)
			fields := message.Fields()
			for j := 0; j < fields.Len(); j++ {
				field := fields.Get(j)
				name := strings.ToLower(string(field.Name()))
				proofBytes := message.FullName() == "gotth.translation.v1.BackendProofRequest" || message.FullName() == "gotth.translation.v1.BackendProofResponse"
				if field.IsMap() || field.Kind() == protoreflect.BytesKind && !proofBytes {
					t.Errorf("generic field %s.%s", message.FullName(), field.Name())
				}
				for _, word := range forbidden {
					if strings.Contains(name, word) {
						t.Errorf("forbidden field %s.%s", message.FullName(), field.Name())
					}
				}
			}
			scan(message.Messages())
		}
	}
	scan(file.Messages())
}

func TestTranslationDescriptorDigest(t *testing.T) {
	file := protodesc.ToFileDescriptorProto(translationv1.File_proto_gotth_translation_v1_translation_proto)
	set := &descriptorpb.FileDescriptorSet{File: []*descriptorpb.FileDescriptorProto{file}}
	encoded, err := proto.MarshalOptions{Deterministic: true}.Marshal(set)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(encoded)
	if got := hex.EncodeToString(digest[:]); got != "e676719d3f5ab11d699e89e1734990f79a1f8aff2c2a1e1d1756828a2599a1fb" {
		t.Fatalf("translation descriptor drift: %s", got)
	}
}

func TestPublishedControlDescriptorBinding(t *testing.T) {
	file := protodesc.ToFileDescriptorProto(extensionsv1.File_proto_gotth_extensions_v1_control_proto)
	set := &descriptorpb.FileDescriptorSet{File: []*descriptorpb.FileDescriptorProto{file}}
	encoded, err := proto.MarshalOptions{Deterministic: true}.Marshal(set)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(encoded)
	if got := hex.EncodeToString(digest[:]); got != "62a549b29a030f0decb46b2b1c2f382bbbc75430ba0b3d7c729bd21528b505cf" {
		t.Fatalf("control descriptor drift: %s", got)
	}
	if extensionsv1.ExtensionControl_Handshake_FullMethodName != "/gotth.extensions.v1.ExtensionControl/Handshake" || extensionsv1.ExtensionControl_Health_FullMethodName != "/gotth.extensions.v1.ExtensionControl/Health" {
		t.Fatal("control RPC paths drifted")
	}
}
