.PHONY: setup 
setup:
	kind create cluster --config=./develop/kind.yaml --name beast

.PHONY: dev 
dev:
	skaffold dev --cleanup=false

.PHONY: destroy
destroy:
	kind delete cluster --name beast
