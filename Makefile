.PHONY: build debug run test clean install lint

build:
	go build -ldflags="-s -w" -o aicli ./cmd/aicli/

debug:
	go build -o aicli ./cmd/aicli/

run: build
	./aicli $(ARGS)

test:
	go test -v ./...

clean:
	rm -f aicli

install: build
	cp aicli /usr/local/bin/aicli

lint:
	go vet ./...
