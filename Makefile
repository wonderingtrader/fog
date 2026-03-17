BINARY=fog
BUILD_DIR=./build
CMD=./cmd/fog

.PHONY: all build install clean cross vet fmt

all: build

build:
	@mkdir -p $(BUILD_DIR)
	go build -ldflags="-s -w" -o $(BUILD_DIR)/$(BINARY) $(CMD)

install:
	go install $(CMD)

clean:
	rm -rf $(BUILD_DIR)

cross:
	@mkdir -p $(BUILD_DIR)
	GOOS=linux   GOARCH=amd64  go build -ldflags="-s -w" -o $(BUILD_DIR)/$(BINARY)-linux-amd64   $(CMD)
	GOOS=linux   GOARCH=arm64  go build -ldflags="-s -w" -o $(BUILD_DIR)/$(BINARY)-linux-arm64   $(CMD)
	GOOS=darwin  GOARCH=amd64  go build -ldflags="-s -w" -o $(BUILD_DIR)/$(BINARY)-darwin-amd64  $(CMD)
	GOOS=darwin  GOARCH=arm64  go build -ldflags="-s -w" -o $(BUILD_DIR)/$(BINARY)-darwin-arm64  $(CMD)
	GOOS=windows GOARCH=amd64  go build -ldflags="-s -w" -o $(BUILD_DIR)/$(BINARY)-windows-amd64.exe $(CMD)

vet:
	go vet ./...

fmt:
	gofmt -w .
