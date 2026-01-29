BINARY := vicostream

.PHONY: build clean

build:
	go build -o $(BINARY) ./cmd/vicostream

clean:
	rm -f $(BINARY)
