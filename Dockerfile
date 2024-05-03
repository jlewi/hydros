ARG BUILD_IMAGE=golang:1.19
ARG RUNTIME_IMAGE=cgr.dev/chainguard/static:latest
FROM ${BUILD_IMAGE} as builder

# Build Args need to be after the FROM stage otherwise they don't get passed through to the RUN statment
ARG VERSION=unknown
ARG DATE=unknown
ARG COMMIT=unknown

WORKDIR /workspace/

COPY . /workspace

RUN wget -O kustomize.tar.gz https://github.com/kubernetes-sigs/kustomize/releases/download/kustomize%2Fv5.4.1/kustomize_v5.4.1_linux_amd64.tar.gz && \
    tar -xzvf kustomize.tar.gz \

## Build
# N.B Disabling CGO can potentially cause problems on MacOSX and darwin builds because some networking requires
# it https://github.com/golang/go/issues/16345. We use to build with CGO_ENABLED=- to disable it at Primer
# but I can't remember precisely why we needed to do it and therefore I don't know if it still applies.
# If we need to renable CGO we need to stop using chainguard's static image and use one with the appropriate
# environment.
#
# TODO(jeremy): We should be setting version information here
# The LDFLAG can't be specified multiple times so we use an environment variable to build it up over multiple lines
RUN LDFLAGS="-s -w -X github.com/jlewi/hydros/cmd/commands.version=${VERSION}" && \
    LDFLAGS="${LDFLAGS} -X github.com/jlewi/hydros/cmd/commands.commit=${COMMIT}" && \
    LDFLAGS="${LDFLAGS} -X github.com/jlewi/hydros/cmd/commands.date=${DATE}" && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on \
    go build \
    -ldflags "${LDFLAGS}" \
    -a -o hydros cmd/main.go

# TODO(jeremy): This won't be able to run Syncer until we update syncer to use GoGit and get rid of shelling
# out to other tools.
# FROM ${RUNTIME_IMAGE}
FROM alpine:3.14

RUN apk update && \
    apk add --no-cache git openssh-client

COPY --from=builder /workspace/hydros /
COPY kustomize /usr/local/bin/kustomize

ENTRYPOINT ["/hydros"]
