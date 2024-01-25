ARG BUILD_IMAGE=golang:1.19
ARG RUNTIME_IMAGE=cgr.dev/chainguard/static:latest
FROM ${BUILD_IMAGE} as builder

WORKDIR /workspace/

COPY . /workspace


## Build
# N.B Disabling CGO can potentially cause problems on MacOSX and darwin builds because some networking requires
# it https://github.com/golang/go/issues/16345. We use to build with CGO_ENABLED=- to disable it at Primer
# but I can't remember precisely why we needed to do it and therefore I don't know if it still applies.
# If we need to renable CGO we need to stop using chainguard's static image and use one with the appropriate
# environment.
#
# TODO(jeremy): We should be setting version information here
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -a -o hydros cmd/main.go

# TODO(jeremy): This won't be able to run Syncer until we update syncer to use GoGit and get rid of shelling
# out to other tools.
FROM ${RUNTIME_IMAGE}

COPY --from=builder /workspace/hydros /

ENTRYPOINT ["/hydros"]
