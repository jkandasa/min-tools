BINARY := min-stats

.PHONY: build clean

build:
	go build -trimpath -ldflags="-s -w" -o $(BINARY) .

clean:
	rm -f $(BINARY)
