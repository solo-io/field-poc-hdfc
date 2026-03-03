IMAGE_NAME  ?= stock-agent
IMAGE_TAG   ?= latest
NAMESPACE   ?= agentgateway-system
PLATFORMS   ?= linux/amd64,linux/arm64

.PHONY: build deploy undeploy logs

build: ## Build multi-arch image (amd64 + arm64)
	docker buildx build --platform $(PLATFORMS) \
		--load -t $(IMAGE_NAME):$(IMAGE_TAG) server/

deploy: ## Deploy the stock agent to K8s
	kubectl apply -f config/serviceaccount.yaml
	kubectl apply -f config/service.yaml
	sed 's|image: stock-agent:latest|image: $(IMAGE_NAME):$(IMAGE_TAG)|' \
		config/deployment.yaml | kubectl apply -f -
	kubectl rollout status deployment/stock-agent -n $(NAMESPACE) --timeout=60s

undeploy: ## Remove the stock agent from K8s
	kubectl delete -f config/deployment.yaml --ignore-not-found
	kubectl delete -f config/service.yaml --ignore-not-found
	kubectl delete -f config/serviceaccount.yaml --ignore-not-found

logs: ## Tail the stock agent logs
	kubectl logs -n $(NAMESPACE) deploy/stock-agent -f --tail=50
