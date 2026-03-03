IMAGE_NAME  ?= stock-server-mcp
IMAGE_TAG   ?= latest
NAMESPACE   ?= agentgateway-system
PLATFORMS   ?= linux/amd64,linux/arm64

.PHONY: build deploy undeploy logs

build: ## Build multi-arch image (amd64 + arm64)
	docker buildx build --platform $(PLATFORMS) \
		--load -t $(IMAGE_NAME):$(IMAGE_TAG) server/

deploy: ## Deploy the stock MCP server to K8s
	kubectl apply -f config/serviceaccount.yaml
	kubectl apply -f config/service.yaml
	sed 's|image: stock-server-mcp:latest|image: $(IMAGE_NAME):$(IMAGE_TAG)|' \
		config/deployment.yaml | kubectl apply -f -
	kubectl rollout status deployment/stock-server-mcp -n $(NAMESPACE) --timeout=60s

undeploy: ## Remove the stock MCP server from K8s
	kubectl delete -f config/deployment.yaml --ignore-not-found
	kubectl delete -f config/service.yaml --ignore-not-found
	kubectl delete -f config/serviceaccount.yaml --ignore-not-found

logs: ## Tail the stock MCP server logs
	kubectl logs -n $(NAMESPACE) deploy/stock-server-mcp -f --tail=50
