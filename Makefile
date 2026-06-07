BINARY=syncviva
VERSION?=dev

.PHONY: build docker clean run

build:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o $(BINARY) .

docker:
	docker build -t syncviva:$(VERSION) .

clean:
	rm -f $(BINARY)

run:
	go run .
