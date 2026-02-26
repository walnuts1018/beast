.PHONY: generate
generate:
	cd apiserver && go generate ./...

.PHONY:lint
lint:
	cd apiserver && go tool golangci-lint run

.PHONY: setup
setup:
	kind create cluster --config=./develop/kind.yaml --name beast

.PHONY: dev 
dev:
	skaffold dev -m apps --cleanup=false

.PHONY: destroy
destroy:
	kind delete cluster --name beast
