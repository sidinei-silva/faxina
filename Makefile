TAGS := webkit2_41

.PHONY: dev build test vet check scan dry-run

dev:
	wails dev -tags $(TAGS)

build:
	wails build -tags $(TAGS) -trimpath

test:
	go test -tags $(TAGS) ./...

vet:
	go vet -tags $(TAGS) ./...

check: vet test
	cd frontend && npm run check --silent

scan: ## varredura sem UI (somente leitura)
	go run -tags $(TAGS) . --scan-json

dry-run: ## prévia do Clean padrão sem executar nada
	go run -tags $(TAGS) . --clean-dry-run
