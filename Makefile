APP_NAME := auth-rotate
BIN_DIR := bin

.PHONY: build build-linux test clean

build:
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/$(APP_NAME) .
	cp -f $(BIN_DIR)/$(APP_NAME) $(GOBIN)/$(APP_NAME)

build-linux:
	@mkdir -p $(BIN_DIR)
	GOOS=linux GOARCH=amd64 go build -o $(BIN_DIR)/$(APP_NAME)-linux-amd64 .
	GOOS=linux GOARCH=arm64 go build -o $(BIN_DIR)/$(APP_NAME)-linux-arm64 .

test:
	go test ./...

clean:
	rm -rf $(BIN_DIR)
