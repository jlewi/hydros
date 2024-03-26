# Continuous Delivery

## What you'll learn

How to use hydros to continuously deliver a set of resources to a kubernetes cluster.

## Configuring continuous delivery

You configure continuous delivery for your application by defining resources corresponding to the artifacts
that need to be continuously delivered.

### Images

Images are defined using the `image` resource type.  

Refer to the [image build](image_build.md) document for more information on how to define images.

### Kubernetes Manifests

Kubernetes manifests are defined using the `ManifestSync` resource type.

Refer to the [manifest sync](manifest_sync.md) document for more information on how to define manifests.

### RepoConfig

A RepoConfig resource is used to define a collection of resources that need to be continuously delivered. 

Here's an example:

```yaml
apiVersion: hydros.dev/v1alpha1
kind: RepoConfig
metadata:
  name: repo
  namespace: sailplaneai
spec:
  repo: https://github.com/yourrepo/code.git
  gitHubAppConfig:
    appID: 384797
    privateKey: gcpsecretmanager:///projects/YOURPROJECT/secrets/hydros-ghapp-key/versions/latest
  globs:
    - "**/*.yaml"
  selectors:
    - matchLabels:
        env: dev
```

The main fields are

* **repo**: The git repository and branch to synchronize the changes from
* **globs**: A list of glob expressions matching files containing hydros resources
* **selectors**: A list of selectors to apply to the resources to determine which ones to synchronize
  * Only resources whose labels match at least one selector will be synchronized 

## Reconciliation

You can use the hydros CLI to reconcile one more resources; e.g. 

```bash
hydros apply --work-dir=/tmp/hydros --dev-logger=true /path/to/your/repo_config.yaml
```

Typically, you'll want to use a `RepoConfig` resource as there are most likely multiple resources that need to be
be built to deliver your application.

### Dependency Resolution

`hydros` currently makes no attempt to resolve dependencies between resources and ensure that resources are reconciled
in the correct order. For example, a `ManifestSync` can require several images to be built before it can be reconciled.
Likewise an `Image` can require other images to be built before it can be built.

When you invoke `hydros apply` on a `RepoConfig` resource, `hydros` will attempt to reconcile all the resources in 
parallel.

To deal with dependencies, you take advantage of the level based nature of the reconciliation process and continually
run reconciliation. Once all dependencies are satisfied, the resources will converge to their desired state. To
continuously run reconciliation, you can use the `--period` flag to specify an interval at which to run reconciliation.

```bash
hydros apply --work-dir=/tmp/hydros --dev-logger=true /path/to/your/repo_config.yaml --period=5m
```
