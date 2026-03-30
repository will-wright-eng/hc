.PHONY: build test clean install lint

build:
	go build -o hc ./cmd/hc

test:
	go test ./...

clean:
	rm -f hc

install:
	go install ./cmd/hc

lint:
	go vet ./...
