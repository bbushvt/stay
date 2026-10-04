.PHONY: all web build run test check clean

all: build

# Build the frontend into web/dist (embedded into the binary by go:embed).
web:
	cd web && npm install && npm run build
	touch web/dist/.gitkeep

build: web
	go build -o stay .

run: build
	./stay

test:
	go test -race ./...

check: test
	cd web && npm run typecheck
	gofmt -l . | (! grep .)
	go vet ./...

clean:
	rm -f stay
	find web/dist -mindepth 1 ! -name .gitkeep -delete
