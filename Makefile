.PHONY: run dev build runtime dsh-src test check fmt tidy release

# Run the app from source. It uses resources/runtime when it exists, and
# otherwise the dsh on PATH.
run:
	go run .

# MyGo's development app: rebuild and restart on Go edits.
dev:
	go tool mygo dev

# Bundle dsh and Node into resources/runtime, which packaged apps ship.
runtime:
	scripts/prepare-runtime.sh

# Pull the dsh source at the tag the runtime bundles (third_party/, ignored).
dsh-src:
	scripts/pull-dsh.sh

# Package the app for this platform into dist/ (needs Go and make runtime).
build: runtime
	go tool mygo build

test:
	go test ./...

# Every Go file of the app; third_party/ is the dsh source, not ours.
GOFILES = $$(find . -name '*.go' -not -path './third_party/*' -not -path './.gocache/*' -not -path './dist/*')

fmt:
	gofmt -w $(GOFILES)

# Everything CI runs: formatting, vet, tests, build. No runtime needed.
check:
	@unformatted="$$(gofmt -l $(GOFILES))"; \
	if [ -n "$$unformatted" ]; then echo "gofmt needed:"; echo "$$unformatted"; exit 1; fi
	go vet ./...
	go test ./...
	go build ./...

tidy:
	go mod tidy

# Tag a release; the tag's push builds the packages in CI.
#   make release VERSION=0.1.1
VERSION ?=
release:
	@test -n "$(VERSION)" || { echo "usage: make release VERSION=x.y.z" >&2; exit 2; }
	@sed -i 's/"version": "[^"]*"/"version": "$(VERSION)"/' mygo.json
	git commit -am "release: v$(VERSION)" || true
	git tag -a "v$(VERSION)" -m "v$(VERSION)"
	@echo "pushed?  git push --follow-tags"
