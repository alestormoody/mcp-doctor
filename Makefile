.PHONY: build test race vet lint vuln check

build:
	go build ./...

test:
	go test ./...

race:
	go test -race ./...

vet:
	go vet ./...

lint:
	golangci-lint run

vuln:
	go run golang.org/x/vuln/cmd/govulncheck@v1.10.0 ./...

check: vet build test race lint vuln
