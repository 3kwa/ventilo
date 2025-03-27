.PHONY: help
help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

run: ## run ventilo
	go run ventilo.go

test: ## run tests suite
	go test

build: ## build docker image for amd64
	docker buildx build --platform linux/amd64 -t 3kwa/ventilo .

push: ## push docker image to dockerhub
	docker push 3kwa/ventilo

