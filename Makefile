.PHONY: check install-hooks

check:
	go build ./...
	go vet ./...
	golangci-lint run
	go test -race -count=1 ./...
	go test -tags integration -race -count=1 ./tests/integration/ -timeout 120s

install-hooks:
	cp scripts/pre-commit .git/hooks/pre-commit
	chmod +x .git/hooks/pre-commit
	@echo "Pre-commit hook installed"
