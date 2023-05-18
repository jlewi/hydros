ROOT := $(shell git rev-parse --show-toplevel)
PROJECT := chat-lewi

build-dir:
	mkdir -p .build

build-go: build-dir
	CGO_ENABLED=0 go build -o .build/hydros github.com/jlewi/hydros/cmd

tidy-go:
	gofmt -s -w .
	goimports -w .
	
tidy: tidy-go

lint-go:
	# golangci-lint automatically searches up the root tree for configuration files.
	golangci-lint run

lint: lint-go

test-go:
	go test -v ./...

build-image-submit:
	COMMIT=$$(git rev-parse HEAD) && \
					gcloud builds submit --project=$(PROJECT) --async --config=cloudbuild.yaml \
					--substitutions=COMMIT_SHA=local-$${COMMIT} \
					--format=yaml > .build/gcbjob.yaml

build-image-logs:
	JOBID=$$(yq e ".id" .build/gcbjob.yaml) && \
		gcloud --project=$(PROJECT) builds log --stream $${JOBID}

build-image: build-dir build-image-submit build-image-logs

# TODO(jeremy): This is really hacky. We should really be using hydros to do this in a declarative way.
# i.e. by setting the image to the local commit. Need to update hydros code to support GCR.
.PHONY: set-image
set-image:
	JOBID=$$(yq e ".id" .build/gcbjob.yaml) && \
	cd manifests/lewi && \
	kustomize edit set image hydros=us-west1-docker.pkg.dev/chat-lewi/hydros/hydros:$${JOBID}

update-image: build-image set-image

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

