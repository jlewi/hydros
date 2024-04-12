# hydros

Hydros is a tools for continuous delivery. Hydros brings a declarative mindset to continuous delivery.
Rather than use a DAG to imperatively define a delivery pipeline, hyrdros consists of a collection of resources.
Each resource is a declarative definition of one or more delivery artifacts. Here are some examples of the semantics,
 
* [Image](docs/image_build.md) resource -  **Ensure there is an image built from the latest commit of the source repository**.
* [ManifestSync](docs/hydrating_manifests.md) resource -  **Ensure hydrated manifests for the latest commit of the source repository are checked into the hydrated repository**.
* [GitHubReleaser](docs/github_releaser.md) resource - **Ensure a GitHub release exists for the latest commit in the repository**.

Each resource is backed by a controller baked into the hydros binary. A resource is reconciled by applying it using
the CLI

```bash
hydros apply  <resource.yaml>
```

For more information see the [docs](docs)

# Open Source Project Status

* Code: experimental
* Issues: appreciated but unfortunately we don't have the bandwidth to respond in a timely fashion
* PRs: appreciated but unfortunately we don't have the bandwidth to respond in a timely fashion


# Documentation

Documentation is in [docs](docs)

# Binaries

There are two binaries currently being built out of this repository

* hydros - This is the main binary in this repository
* sanitizer - This is a utility to help sanitize internal code before publishing it as public open source. It was
   initially developed to aid in open sourcing hydros. It was inspired by a similar tool used at Google.

# Releasing

