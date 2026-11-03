.PHONY: test test-opentofu test-fnox vet vet-opentofu fmt-check fmt-check-opentofu build build-opentofu release-build-opentofu

# Root convenience targets delegate to adapter-local toolchains and tests.
test: test-opentofu

test-opentofu:
	cd adapters/opentofu && go test -count=1 ./...

test-fnox:
	python3 adapters/fnox/tests/test_cli.py

vet: vet-opentofu

vet-opentofu:
	cd adapters/opentofu && go vet ./...

fmt-check: fmt-check-opentofu

fmt-check-opentofu:
	cd adapters/opentofu && test -z "$$(gofmt -l .)"

build: build-opentofu

build-opentofu:
	mkdir -p adapters/opentofu/dist
	cd adapters/opentofu && go build -trimpath -buildvcs=false -o dist/nexusq-keyprovider-opentofu .

release-build-opentofu:
	bash adapters/opentofu/scripts/release-build.sh
