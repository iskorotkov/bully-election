image=iskorotkov/bully-election
version=v0.1.0-alpha.1

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
pish-image:
	docker push $(image):$(version)
