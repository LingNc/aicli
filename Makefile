.PHONY: build run test clean install lint

build:
	go build -ldflags="-s -w" -o ai ./cmd/ai/

run: build
	./ai $(ARGS)

test:
	go test -v ./...

clean:
	rm -f ai

install: build
	cp ai /usr/local/bin/ai

lint:
	go vet ./...
