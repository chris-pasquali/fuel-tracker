.PHONY: api web test tidy compose

tidy:
	cd api && go mod tidy

test:
	cd api && go test ./...

api:
	cd api && DATABASE_URL=postgres://fillwise:fillwise@127.0.0.1:5432/fillwise?sslmode=disable \
		REDIS_URL=redis://127.0.0.1:6379 \
		SNAPSHOT_PATH=../data/sydney_snapshot.json \
		go run ./cmd/server

web:
	cd web && npm install && npm run dev

compose:
	docker compose up --build
