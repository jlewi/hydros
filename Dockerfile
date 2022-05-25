# Build the sidecar
ARG GOLANG_IMAGE=SET_IN_SKAFFOLD_YAML

# Actual runtime image should be set in skaffold.yaml as a build file.
# It should be pinned to a specific image
ARG RUNTIME_IMAGE=SET_IN_SKAFFOLD_YAML

FROM ${GOLANG_IMAGE} as builder


WORKDIR /workspace/

COPY . /workspace


## Build
RUN CGO_ENABLED=- GOOS=linux GOARCH=amd64 GO111MODULE=on go build -a -o hydros cmd/main.go

# Use the runtime image that contains all the code we need. Built from ~/hydros/runtime-image.
FROM ${RUNTIME_IMAGE}

COPY --from=builder /workspace/hydros /

ENTRYPOINT ["/hydros"]
