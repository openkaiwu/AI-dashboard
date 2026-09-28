.PHONY: run run-demo web-dev web-install web-build test gate build
run:
	cd server && go run ./cmd/aihub
run-demo:
	cd server && go run ./cmd/aihub -demo
web-dev:
	cd apps/web && npm run dev
web-install:
	npm ci --prefix apps/web
web-build:
	npm run build --prefix apps/web
	mkdir -p server/webassets/dist
	cp -R apps/web/dist/. server/webassets/dist/
test:
	cd server && go test ./...
gate:
	bash scripts/gate.sh
build:
	bash scripts/build.sh
