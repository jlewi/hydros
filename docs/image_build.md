# Image Building

Hydros provides a controller to ensure images exist for a set of sources. 
The semantics of the `Image` resource are 
"Ensure there is an image built from the latest commit of the source repository".

## Defining an image

You can define an image by using the Image resource

Here's an example

```yaml
kind: Image
apiVersion: hydros.sailplane.ai/v1alpha1
metadata:
  name: kp
  namespace: kubepilot
spec:
  image: us-west1-docker.pkg.dev/dev-sailplane/images/hydros/hydros
  source:
  - uri: https://github.com/sailplaneai/code.git
    mappings:
      # Leave it at the root of the context because that's what hydros will look for.
      - src: /gilfoyle/Dockerfile
        strip: gilfoyle
      - src: "/gilfoyle/*.sh"
        strip: gilfoyle
    - uri: docker://us-west1-docker.pkg.dev/dev-sailplane/images/kubepilot
      mappings:
        - src: /kp
  builder:
    gcb:
      project: dev-sailplane
      bucket : builds-sailplane
```

Currently only the GCB builder is supported.

### Context

The context for the image is defined by the source field. Each entry in the source field specifies files
that will be copied into the context. The source field is an array of objects with the following fields:

* src: This is a glob expression matching files to be copied into the context. The glob expression is relative to the
  directory containing the image definition file.
  * Double star `**` can be used to match all subdirectories
  * You can use `..` to go up the directory tree to match files located in parent directories of the `.yaml` file
* dest: This is the destination directory for the files. 
* strip: This is a prefix to strip of the matched files when computing the location in the destination directory. 

The location of the files inside the produced context (tarball) is as follows

```
basePath = dir(imageDefinitionFile)
rPath = path of matched file relative to basePath
strippedRPath = strip rPath of prefix 
destPath = dest + stripPipedRpath
```

In the case where `src` begins with `..` basePath is adjusted to be the parent directory.

### Dockerfile

Hydros currently requires the Dockerfile to be named `Dockerfile` and located at the root of the context.

### Docker build args

The following build args are passed to kaniko and can be used in your Dockerfile

* `DATE` - A human readable timestap
* `COMMIT` - The full git commit
* `Version` - A version string of the form `vv20240126T092312`

### Tags

The build automatically tags the image with the following tags

* `latest` 
*  The full git commit of the source repository

## Building an image

To build an image you can use the `hydros build` command

```bash
hydros build ~/git_roboweb/kubedr/image.yaml
```

If an image already exists in the registry with the same tag as the current commit, the image will not be rebuilt.