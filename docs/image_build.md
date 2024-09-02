# Image Building

Hydros provides a controller to ensure images exist for a set of sources. 
The semantics of the `Image` resource are 
"Ensure there is an image built from the latest commit of the source repository".

## Defining an image

You can define an image by using the Image resource

Here's an example

```yaml
kind: Image
apiVersion: hydros.dev/v1alpha1
metadata:
  name: hydros
  namespace: hydros
spec:
  image: us-west1-docker.pkg.dev/foyle-public/images/hydros/hydros
  source:
    - uri: https://github.com/jlewi/hydros.git
      mappings:
        - src: Dockerfile
        # Specify individual directories so we don't include hidden directories
        - src: "go.mod"
        - src: "go.sum"
        - src: "api/**/*.go"
        - src: "cmd/**/*.go"
        - src: "pkg/**/*.go"
        - src: "test/**/*.go"
  builder:
    gcb:
      project: YOUR-PROJECT
      bucket : builds-your-project
```

Currently only the GCB builder is supported.

### Context

The context for the image is defined by the source field. Each entry in the source field specifies files
that will be copied into the context. The source field is an array of objects with the following fields:

* uri: The URI of the source resource. This can be a git repository or docker image.
  * For git repositories the URI should be the URL of the repository e.g. `https://github.com/jlewi/hydros.git`
  * For docker images the URI should be the image name with the scheme `docker://` e.g. `docker://gcr.io/foyle-public/hydros:latest
* mappings: An array of mappings specifying files to be copied into the context.

* src: This is a glob expression matching files to be copied into the context. The glob expression is relative to the
  root of the resource (e.g. the repository or the docker image). The following glob expressions are supported:
  * Double star `**` can be used to match all subdirectories
  * You can use `..` to go up the directory tree to match files located in parent directories of the `.yaml` file
* dest: This is the destination directory for the files. 
* strip: This is a prefix to strip of the matched files when computing the location in the destination directory. 

The location of the files inside the produced context (tarball) is as follows

Typically the first source will be the git repository containing the source code.

### Dockerfile

By default Hydros assumes the Dockerfile to be named `Dockerfile` and located at the root of the context. However,
you can specify the path to the Dockerfile using the `dockerfile` field in the `gcb` section.

### Docker build args

The following build args are passed to kaniko and can be used in your Dockerfile

* `DATE` - A human readable timestap
* `COMMIT` - The full git commit
* `VERSION` - A version string of the form `v20240126T092312`

Here's an example of using them in your Dockerfile

```docker

FROM ${BUILD_IMAGE} as builder

# Build Args need to be after the FROM stage otherwise they don't get passed through to the RUN statment
ARG VERSION=unknown
ARG DATE=unknown
ARG COMMIT=unknown
...
```

### Tags

The build automatically tags the image with the following tags

* `latest` 
*  The full git commit of the source repository

## Building an image

To build an image you can use the `hydros build` command

```bash
hydros build -f ~/git_hydros/kubedr/images.yaml
```

* If an image already exists in the registry with the same tag as the current commit, the image will not be rebuilt.
* If the repository is dirty Hydros will commit the changes and then build the image
* Hydros will automatically detect if the file is located in a git repository that matches one of the sources and 
  use the commit hash as the tag.