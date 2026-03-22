setup:
	./scripts/setup.sh

dev:
	./scripts/dev.sh

stop:
	./scripts/stop.sh

build:
	go build -o wiregate-server ./cmd/wiregate-server
