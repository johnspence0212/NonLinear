.PHONY: run build test clean dist

ADDR ?= :3333
DATA_DIR ?= ./data

run:
	go run ./cmd/nonlinear -addr $(ADDR) -data $(DATA_DIR)

build:
	go build -o nonlinear ./cmd/nonlinear

test:
	go test ./...

clean:
	rm -f nonlinear
	rm -rf dist

dist:
	mkdir -p dist
	GOOS=linux   GOARCH=amd64 go build -o dist/nonlinear-linux-amd64 ./cmd/nonlinear
	GOOS=darwin  GOARCH=amd64 go build -o dist/nonlinear-darwin-amd64 ./cmd/nonlinear
	GOOS=darwin  GOARCH=arm64 go build -o dist/nonlinear-darwin-arm64 ./cmd/nonlinear
	GOOS=windows GOARCH=amd64 go build -o dist/nonlinear-windows-amd64.exe ./cmd/nonlinear
