.PHONY: build install dev-server dev-ui

build:
	cd web && npm run build
	go build ./cmd/onlyagents/

install:
	cd web && npm run build
	go install ./cmd/onlyagents/

# Dev: run Go (watches web/dist), Vite rebuilds in parallel
dev-server:
	go run ./cmd/onlyagents/ server start

dev-ui:
	cd web && npm run dev
