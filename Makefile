# Tuck is a Windows app, but every target here works from any OS: the build
# is a plain Go cross-compile with no C toolchain involved.

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -H=windowsgui -s -w -X main.version=$(VERSION)
GOFLAGS := -trimpath

.PHONY: all build build-arm64 test vet lint preview resources clean

all: vet test build

## build: compile bin/tuck.exe for 64-bit Intel/AMD Windows
build:
	GOOS=windows GOARCH=amd64 go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o bin/tuck.exe ./cmd/tuck

## build-arm64: compile bin/tuck-arm64.exe for Windows on ARM
build-arm64:
	GOOS=windows GOARCH=arm64 go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o bin/tuck-arm64.exe ./cmd/tuck

## test: run the portable tests (the Windows-only ones run on Windows and in CI)
test:
	go test ./...

## vet: run go vet for both the host and the Windows build
vet:
	go vet ./...
	GOOS=windows GOARCH=amd64 go vet ./...

## lint: check formatting and run staticcheck for the Windows build
lint:
	@test -z "$$(gofmt -l .)" || (echo "gofmt needed:"; gofmt -l .; exit 1)
	go install honnef.co/go/tools/cmd/staticcheck@latest
	GOOS=windows GOARCH=amd64 "$$(go env GOPATH)/bin/staticcheck" ./...

## preview: render the custom-drawn UI to PNG files in ./preview
preview:
	go run ./cmd/tuck-preview -out preview

## resources: regenerate the icon, manifest and version info linked into the exe
resources:
	go run ./cmd/tuck-preview -icons cmd/tuck/winres
	cd cmd/tuck && go run github.com/tc-hib/go-winres@v0.3.3 make --in winres/winres.json --arch amd64,arm64

clean:
	rm -rf bin dist preview
