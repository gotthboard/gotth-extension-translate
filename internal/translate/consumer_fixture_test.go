package translate

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	translationv1 "github.com/gotthboard/gotth-extension-translate/proto/gotth/translation/v1"
	"google.golang.org/protobuf/proto"
)

func TestConsumerFixturesMinimizeProductData(t *testing.T) {
	kinds := map[string]translationv1.ContentKind{"plain": translationv1.ContentKind_CONTENT_KIND_PLAIN, "mail-visible-text": translationv1.ContentKind_CONTENT_KIND_MAIL_VISIBLE_TEXT, "markdown-segment": translationv1.ContentKind_CONTENT_KIND_MARKDOWN_SEGMENT, "document-text": translationv1.ContentKind_CONTENT_KIND_DOCUMENT_TEXT}
	caches := map[string]translationv1.CacheClass{"none": translationv1.CacheClass_CACHE_CLASS_NONE, "request": translationv1.CacheClass_CACHE_CLASS_REQUEST}
	expected := map[string]struct{ product, kind string }{
		"mail.json":  {product: "mail", kind: "mail-visible-text"},
		"bb.json":    {product: "bb", kind: "markdown-segment"},
		"cloud.json": {product: "cloud", kind: "document-text"},
	}
	for _, name := range []string{"mail.json", "bb.json", "cloud.json"} {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "consumers", name))
			if err != nil {
				t.Fatal(err)
			}
			var raw map[string]json.RawMessage
			if err := json.Unmarshal(data, &raw); err != nil {
				t.Fatal(err)
			}
			if len(raw) != 3 || raw["product"] == nil || raw["protected"] == nil || raw["request"] == nil {
				t.Fatal("fixture top-level shape drift")
			}
			var product string
			if err := json.Unmarshal(raw["product"], &product); err != nil || product != expected[name].product {
				t.Fatalf("unexpected product %q: %v", product, err)
			}
			var protected []string
			if err := json.Unmarshal(raw["protected"], &protected); err != nil || len(protected) == 0 {
				t.Fatalf("protected fixture data required: %v", err)
			}
			var requestRaw struct {
				CorrelationID  string `json:"correlation_id"`
				SourceLanguage string `json:"source_language"`
				TargetLanguage string `json:"target_language"`
				ContentKind    string `json:"content_kind"`
				CacheClass     string `json:"cache_class"`
				Segments       []struct {
					ID   string `json:"id"`
					Text string `json:"text"`
				} `json:"segments"`
			}
			decoder := json.NewDecoder(bytes.NewReader(raw["request"]))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&requestRaw); err != nil {
				t.Fatal(err)
			}
			if requestRaw.ContentKind != expected[name].kind {
				t.Fatalf("content kind %q, want %q", requestRaw.ContentKind, expected[name].kind)
			}
			request := &translationv1.TranslateRequest{CorrelationId: requestRaw.CorrelationID, SourceLanguage: requestRaw.SourceLanguage, TargetLanguage: requestRaw.TargetLanguage, ContentKind: kinds[requestRaw.ContentKind], CacheClass: caches[requestRaw.CacheClass]}
			for _, segment := range requestRaw.Segments {
				request.Segments = append(request.Segments, &translationv1.Segment{Id: segment.ID, Text: segment.Text})
			}
			if reason := validateRequest(request); reason != "" {
				t.Fatalf("fixture rejected: %s", reason)
			}
			wire, err := proto.MarshalOptions{Deterministic: true}.Marshal(request)
			if err != nil {
				t.Fatal(err)
			}
			for _, secret := range protected {
				if bytes.Contains(wire, []byte(secret)) {
					t.Fatalf("protected product data crossed wire: %q", secret)
				}
			}
		})
	}
}
