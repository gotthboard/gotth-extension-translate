.PHONY: test vet build format binding generated metadata process artifact verify verify-heavy

test:
	go test -count=1 ./...

vet:
	go vet ./...

build:
	go build ./...

format:
	test -z "$$(gofmt -l cmd internal test proto/gotth/translation/v1/*.go)"

binding:
	./scripts/check-published-control.sh

generated:
	./scripts/verify-generated.sh

metadata:
	./scripts/verify-release-metadata.sh

process:
	@tmp=$$(mktemp -d); trap 'rm -rf "$$tmp"' EXIT HUP INT TERM; \
	go build -o "$$tmp/gotth-extension-translate" ./cmd/gotth-extension-translate; \
	GOTTH_TRANSLATE_EXTENSION_BINARY="$$tmp/gotth-extension-translate" go test -count=1 ./test/process

artifact:
	./scripts/test-reproducible-artifact.sh

verify: test vet build format binding generated metadata process artifact
	git diff --check

verify-heavy:
	go test -race -count=1 ./...
	go test -shuffle=on -count=20 ./internal/translate
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out
