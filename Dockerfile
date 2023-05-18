ARG BUILD_IMAGE=golang:1.19
ARG RUNTIME_IMAGE=cgr.dev/chainguard/static:latest
FROM ${BUILD_IMAGE} as builder

WORKDIR /workspace/

COPY . /workspace


## Build
RUN CGO_ENABLED=- GOOS=linux GOARCH=amd64 GO111MODULE=on go build -a -o hydros cmd/main.go

# TODO(jeremy): This won't be able to run Syncer until we update syncer to use GoGit and get rid of shelling
# out to other tools.
FROM ${RUNTIME_IMAGE}

COPY --from=builder /workspace/hydros /

ENTRYPOINT ["/hydros"]
