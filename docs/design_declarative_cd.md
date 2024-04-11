# Design Declarative CD

## Objectives

1. Create a design document to begin rationalizing future development with the goal of creating a cohesive story 
   for declarative CD. 

## Motivation

Hydros has evolved in an adhoc fashion. The original intent was pretty modest. The goal of hydros was to solve
a gap in the GitOps toolchain by creating a tool that would continuously hydrate a set of manifests and check them
into another repository. Those manifests were then applied via GitOps tools (i.e. flux).

The original version of hydros worked by loading the `ManifestSync` via a ConfigMap and then periodically
applying the resources.

This had the disadvantage that setting up or making changes to the hydros configurations for an application was
a pain. They were stored in a separate repository so a developer had to go to that repository, make the changes, and
then wait for the changes to be applied.

This had several disadvantages
* ManifestSync objects weren't co-located with the application they described 
* Hydros reconciliations weren't triggered by push events

Overtime hydros evolved other features

* Support for continuous delivery of images via Skaffold
* Support for pinning images as part of hydration
* Support for running kustomize functions as part of hydration
* Support for letting users takeover an environment and push local changes

There are a number of other pain points

* Monitoring / Observability 
  * The only way to know the current status is to dig through logs
* Speed
  * Hydros' reconciliation loop currently only responds to periodic events
  * In particular we don't immediately consume GitHub push events

## Proposal

I think the path forward is a set of K8s controllers that handle various continuous delivery tasks. Based on
the current state the two central controllers would be 

* Image - declaratively deliver images into an artifact registry
* ManifestSync - declaratively deliver manifests into a git repository

Moving to K8s controllers would solve a number of problems

* Monitoring / Observability
  * We can use the standard K8s tooling and patterns to monitor the controllers
  * `kubectl describe images` or `kubectl describe manifestsyncs` would give us the current status of the delivery
* We can reuse the existing GitOps toolchain to continuously update the resources based on the resources in a 
  a repository 

### Local

I think its important to continue to support a local mode. This is useful for development and testing. I think
this can be achieved by continuing to support `hydros apply` and `hydros takeover`.

## Alternatives

### GitHub App

Makeing hydros a GitHub App is something we'd previously explored. Some of the code is in [../pkg/app](../pkg/ghapp).

There were a bunch of different problems we were potentially trying to solve by making it a GitHub App

* Reactivity - using GitHub events to trigger reconciliation
* Configuration - using GitHub App machinery to load configurations (i.e. resource definitions) directly from the GitHub repository
* Monitoring - using GitHub check runs to report status

I don't think this will work well because fundamentally we want a declarative, reconciler based architecture and this doesn't
map well onto the idea of a GitHub App. For example, I don't think it makes sense for GitHub CheckRuns to be the centerpiece of monitoring.
I think the semantics around checkruns are **run to completion** for a given commit. Consider the following,

* Commit 12ab is pushed 
* Hydros successfully deploys commit 12ab  
* Developer uses `hydros takeover` to push local commits
* Hydros pauses normal reconciliation for main during takeover
* Dev takeover expires
* Hydros rehydrates using commit 12ab to correct the drift

If `ManifestSync` is a K8s controller then the status of the resource can easily report the status during all of these phases.

If we rely on CheckRuns to report the status there is an impedence mismatch because CheckRuns are mapped to specific commits but the status
of the ManifestSync resource transcends individual commits.

If we rely on a GitHub App then I think we end up reinventing the wheel when it comes to synchronizing and applying the latest resources checked into git.

# References

* [Block on Reconcilers & GitOps](https://blog.kubeflow.org/mlops/)