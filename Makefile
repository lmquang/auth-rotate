APP_NAME := auth-rotate
BIN_DIR := bin

.PHONY: build test clean

build:
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/$(APP_NAME) .
	cp -f $(BIN_DIR)/$(APP_NAME) $(GOBIN)/$(APP_NAME)

test:
	go test ./...

clean:
	rm -rf $(BIN_DIR)
