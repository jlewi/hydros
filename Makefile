ROOT := $(shell git rev-parse --show-toplevel)
PROJECT := chat-lewi

build-dir:
	mkdir -p .build

# TODO(jeremy): Get rid of this rule
build-go: build

build: build-dir
	CGO_ENABLED=0 go build -o .build/hydros github.com/jlewi/hydros/cmd

tidy-go:
	gofmt -s -w .
	goimports -w .
	
tidy: tidy-go

lint-go:
	# golangci-lint automatically searches up the root tree for configuration files.
	golangci-lint run

lint: lint-go

# TODO(jeremy): get rid of this rule
test-go: test

test:	
	go test -v ./...


# Build with ko
# This is much faster
# TODO(jeremy): We should add support to image.yaml to use ko
build-ko-image:
	KO_DOCKER_REPO=us-west1-docker.pkg.dev/dev-sailplane/images/hydros \
		ko build --bare github.com/jlewi/hydros/cmd  

# This builds the image using hydros which uses GCB
build-image:
	go run github.com/jlewi/hydros/cmd  build -f images.yaml


# N.B. The makefile commands to update the manifest were deleted as part of switching to hydros to build the images
# We should now be able to use hydros to update the manifest and pin the images

# hydrate uses the hydros takeover command to apply the sync configuration and push
# hydrated manifests to the manifests repository.
hydrate:
	hydros takeover \
		--file=$(ROOT)/manifests/manifestsync.yaml \
		--app-id=266158 \
		--work-dir=.build/run-hydros \
		--private-key="gcpSecretManager:///projects/chat-lewi/secrets/hydros-jlewi/versions/latest"

# Apply works by checking out the hydrated manifests and then applying them using a kubectl command.
apply:
	cd .build/run-hydros/hydros-dev-takeover/dest && git fetch origin && \
		git checkout origin/main
	kubectl --context=hydros apply \
		-R -f .build/run-hydros/hydros-dev-takeover/dest/hydros/lewi

update: update-image hydrate apply