# Design GitHub APP

## Objectives

1. Create a GitHub APP that loads and applies manifests in a repository.

## Motivation

The original version of hydros worked by loading the `ManifestSync` via a ConfigMap and then periodically
applying the resources.

This had the disadvantage that setting up or making changes to the hydros configurations for an application was
a pain. They were stored in separate repository so a developer had to go to that repository, make the changes, and
then wait for the changes to be applied. This also created a somewhat circular dependency

* A kustomize configmap was used to construct a configmap containing all the resources
* Hydros was used to hydrate the configmap and hydros into the hydrated repository
* Flux was used to deploy the manifests

This had several disadvantages
* ManifestSync objects weren't co-located with the application they described 
* Hydros reconciliations weren't triggered by push events

### Desired UX

* Hydros resources (`Image`, `ManifestSync`) are co-located with the application they describe
* Hydros reads the resources directly from the repository
  * Changes to the resources should be automatically applied
  * i.e. we should have semantics similar to most GitHub APPs where a user checks in a YAML configuration file
    and the APP automatically applies it

## Proposal

### Proposed UX

Each repository contains a `hydros.yaml` file in a well known location (root of the repository). This file
can contain one or more YAML resources. In most cases, the file will contain a `RepoConfig` resource. This
resource will contain the configuration for the repository.

```yaml
apiVersion: hydros.sailplane.ai/v1alpha1
kind: RepoConfig
metadata:
  name: hydros
  namespace: sailplaneai
spec:
  globs:
    - "**/*.yaml"
  selectors:
    - matchLabels:
        env: dev
```

The combination of `globs` and `selectors` will be used to determine which resources to apply. As follows

* Any files matching the globs will be treated as YAML files and the resources in them will be loaded
* The resources will be filtered by the selectors
  * Resources that don't match one of the selectors will be ignored 

The use of selectors is necessary to filter out resources related to other methods of deployment; e.g. doing
a manual takeover of the dev environment.

### Implementation

The GitHub APP will respond to events from the repository by loading the latest RepoConfig.

Independent controllers or separate controllers
* I think separate controllers. In the future we might create proper K8s controllers for the resources.   
  * but then why aren't we reusing the flux toolchain? 

## Advantage of making them K8s resources

Simplifies the monitoring of the resources. We can use the standard K8s tooling to monitor the resources.

## Alternate Design just use flux and K8s controllers

Using flux directly this might work as follows

1. You deploy the hydros controller in your cluster
1. You create a flux `GitRepository` that points to the repository containing the hydros resources
1. You create a flux `Kustomization` that points to the directory containing the `hydros.yaml` file in the repository

A limitation here is that the flux [Kustomize](https://fluxcd.io/flux/components/kustomize/api/v1/) doesn't give us
the semantics we want in terms of selecting resources to be applied. It only lets us set a single path which is a
directory containing a set of YAML files to be applied. We want to match a set of globs and selectors. 

