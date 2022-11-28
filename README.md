# hydros

Hydros is a bot that hydrates manifests and opens a PR to check in hydrated manifests.

# Open Source Project Status

* Code: experimental
* Issues: appreciated but unfortunately we don't have the bandwidth to respond in a timely fashion
* PRs: appreciated but unfortunately we don't have the bandwidth to respond in a timely fashion

Primer is open sourcing hydros in order to foster discussions with the kustomize and GitOps
communities about potential areas of collaboration.

At this point in time, hydros is unlikely to work out of box for anyone outside of Primer
as it makes several decisions specific to how Primer does CI/CD.


# Binaries

There are three binaries currently being built out of this repository

* hydros - This is the main binary in this repository
* sanitizer - This is a utility to help sanitize internal code before publishing it as public open source. It was
   initially developed to aid in open sourcing hydros. It was inspired by a similar tool used at Google.

# Installing Hydros On Your Repositories

1. Create a GitHub App
   * Grant it the permissions
     * Contents - Read & Write
     * Pull Requests - Read & Write
2. Generate a private key for the github app
3. Install it on the repositories that will be used as the source and destination