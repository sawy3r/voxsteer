.PHONY: test build-desktop build-browser build-macos-browser build-windows-amd64 build-windows-arm64 build-windows

test:
	go test ./...

build-desktop:
	mkdir -p bin
	go build -buildvcs=false -tags desktop -trimpath -o bin/voxsteer ./cmd/voxsteer

build-browser:
	mkdir -p bin
	CGO_ENABLED=0 go build -buildvcs=false -trimpath -o bin/voxsteer-browser ./cmd/voxsteer-browser

build-macos-browser:
	bash scripts/build-macos-browser.sh

build-windows: build-windows-amd64 build-windows-arm64
	cp config.example.json bin/config.example.json

build-windows-amd64:
	mkdir -p bin
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -buildvcs=false -tags desktop \
		-trimpath -ldflags="-H=windowsgui -s -w" \
		-o bin/voxsteer-windows-amd64.exe ./cmd/voxsteer

build-windows-arm64:
	mkdir -p bin
	CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build -buildvcs=false -tags desktop \
		-trimpath -ldflags="-H=windowsgui -s -w" \
		-o bin/voxsteer-windows-arm64.exe ./cmd/voxsteer
