.PHONY: test vet fmt-check release-build

test:
	go test -count=1 ./...

vet:
	go vet ./...

fmt-check:
	test -z "$$(gofmt -l .)"

release-build:
	bash scripts/release-build.sh
