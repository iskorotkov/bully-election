version = v1.1.0-sync-replicas-fetch.1
image = iskorotkov/bully-election
namespace = chaos-app

.PHONY: ci
ci: build test build-image push-image deploy

.PHONY: build
build:
	go build ./...

.PHONY: run
run:
	go run ./...

.PHONY: test
test:
	go test ./...

.PHONY: test-short
test-short:
	go test ./... -short

.PHONY: build-image
build-image:
	docker build -t $(image):$(version) -f build/bully-election.dockerfile .

.PHONY: push-image
push-image:
	docker push $(image):$(version)

.PHONY: deploy
deploy:
	kubectl apply -f deploy/bully-election.yml -n $(namespace)

.PHONY: undeploy
undeploy:
	kubectl delete -f deploy/bully-election.yml -n $(namespace)
