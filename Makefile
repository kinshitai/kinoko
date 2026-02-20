.PHONY: check install-hooks setup

setup: install-hooks
	@echo "Setup complete"

check: install-hooks
	go build ./...
	go vet ./...
	golangci-lint run
	go test -race -count=1 ./...
	go test -tags integration -race -count=1 ./tests/integration/ -timeout 120s
	go test -tags integration -race -count=1 ./tests/e2e/ -timeout 120s

install-hooks:
	@git config core.hooksPath scripts
