.PHONY: run run-demo web-dev web-build test tidy

ADDR ?= :8080
DATABASE ?= $(PWD)/data/aihub.db

run:
	mkdir -p data
	cd server && go run ./cmd/aihub -addr $(ADDR) -database $(DATABASE)

run-demo:
	mkdir -p data
	cd server && go run ./cmd/aihub -addr $(ADDR) -database $(DATABASE) -demo

web-dev:
	cd apps/web && npm run dev

web-install:
	cd apps/web && npm install

web-build:
	cd apps/web && npm run build
	rm -rf server/webdist
	cp -r apps/web/dist server/webdist

test:
	cd server && go test ./...

tidy:
	cd server && go mod tidy
