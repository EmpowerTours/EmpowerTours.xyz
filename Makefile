.PHONY: build run test clean

build:
	go build -o bin/server ./cmd/server/

run: build
	./bin/server

test:
	go test ./...

clean:
	rm -rf bin/
	rm -f empowertours.db
