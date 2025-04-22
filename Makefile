.PHONY: lint fmt ci test devdeps
LINTER := golangci-lint
build:
	go build --o bin/afk .
ci: devdeps lint test
run:
	go run .

lint:
	@echo ">> Running linter ($(LINTER))"
	$(LINTER) run

fmt:
	@echo ">> Formatting code"
	gofmt -w .
	goimports -w .

test:
	@echo ">> Running tests"
	go test -v -cover ./...

devdeps:
	@echo ">> Installing development dependencies"
	which goimports > /dev/null || go install golang.org/x/tools/cmd/goimports@latest
	which golangci-lint > /dev/null || go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
