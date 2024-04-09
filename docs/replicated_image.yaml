# Replicated image

The replicated image resource can be use to copy images (and their tags) from one repository to one or more
other repositories. This is useful when you want to replicate images from one repository to another, for example
when you want to replicate images from a private repository to a public repository.

The current semantics are that on each reconcile the image in the source repository with the tag `latest` is
copied to the target repositories. 

Here is a sample resource

```
apiVersion: hydros.dev/v1alpha1
kind: ReplicatedImage
metadata:
  name: replicated-image-sample
spec:
  source:
    repository: "us-west1-docker.pkg.dev/foyle-public/images/foyle-vscode-ext"
  destinations:
    - "ghcr.io/jlewi/foyle-vscode-ext"
```

To apply the resource run

```shell
hydros apply -f path/to/replicated_image.yaml
```