dev:
	./scripts/dev.sh

stop:
	./scripts/stop.sh

setup:
	./scripts/setup.sh

build:
	go build -o wiregate-server ./cmd/wiregate-server

pg-up:
	docker compose -f deploy/compose/docker-compose.postgres.yml up -d

pg-down:
	docker compose -f deploy/compose/docker-compose.postgres.yml down

dev-pg: pg-up
	WIREGATE_POSTGRES_DSN="postgres://wiregate:wiregate@localhost:5432/wiregate?sslmode=disable" ./scripts/dev.sh

release:
	./scripts/release.sh deploy

release-deploy:
	./scripts/release.sh deploy

release-publish:
	./scripts/release.sh publish

release-upgrade:
	./scripts/release.sh upgrade

release-rollback:
	./scripts/release.sh rollback
