# Dev TakeOver

During development it is often convenient to deploy to your dev cluster without being blocked on code review.
Hydros supports taking over dev using the CLI. This works as follows

1. Create a `ManifestSync` specific to the dev takeover
    * Change the branch in `sourceRepo` to be a branch that Hydros(and developers) can directly push without
      being blocked on code review (or potentially presubmits passing)
    * Change the branch in `forkRepo` to be a branch that hydros will use for dev takeover
        * This branch should be different from the branch used by other ManfiestSync resources

2. Use the CLI to push changes as necessary

   ```bash
    hydros takeover \
      --file=<path/to/your/manifestsync.yaml> \
      --app-id=<GitHubAppID for Hydros> \
      --work-dir=<Local directory for Hydros to clone repositories> \
      --private-key=<Path to the GitHubApp Private Key; use gcpSecretManager:/// to use a GCP Secrete Manager S>
   ```
    * This will commit any changes on your local branch
    * Push them to the branch specified by `sourceRepo`
    * Run Hydros to sync from the specified branch to the hydrated repository

**Important** If two developers try to takeover dev at the same time, they will end up overwriting each other's changes.

The advantages of Hydros' approach to dev takeover are

* You get the benefits of GitOps; i.e. all changes are checked into git providing a consistent log

## Pausing Reconciliation

When you run `hydros takeover` you pause normal reconciliation of the `ManifestSync` resource. 
This means that Hydros will not automatically restore the latest changes on `main` until an expiration time is reached. 
By default the pause is **2 hours** but you can use the flag `--pause` to set a longer duration. 

* Hydros will automatically restore the latest changes on main once an expiration time is reached

You can check if reconciliation is paused by checking the `status` field of the `ManifestSync` resource that is
checked into the hydrated repository.

```yaml 
apiVersion: mlp.primer.ai/v1alpha1
kind: ManifestSync
...
spec:
  ...
status:
  pausedUntil: "2024-01-30T20:46:27-08:00"
  ...
```

Here the `pausedUntil` field indicates the time at which reconciliation will be restored. If you want to manually
unpause reconciliation you can delete the `status` field from the `ManifestSync` resource in the hydrated repository.