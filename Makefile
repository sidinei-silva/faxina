TAGS := webkit2_41

.PHONY: dev build test vet check scan dry-run

frontend/dist/index.html:
	cd frontend && npm ci --silent && npm run build --silent

dev:
	wails dev -tags $(TAGS)

build:
	wails build -tags $(TAGS) -trimpath

test: frontend/dist/index.html
	go test -tags $(TAGS) ./...

vet: frontend/dist/index.html
	go vet -tags $(TAGS) ./...

check: vet test
	cd frontend && npm run check --silent

scan: frontend/dist/index.html ## varredura sem UI (somente leitura)
	go run -tags $(TAGS) . --scan-json

dry-run: frontend/dist/index.html ## prévia do Clean padrão sem executar nada
	go run -tags $(TAGS) . --clean-dry-run
