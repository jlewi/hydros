# Generative AI with Hydros

Define a KPT function which specifies all the kustomize functions you want hydros to be aware of.
For each kustomize function provide a `FilterSpec` which is an OpenAPI schema of the function.
Here's an example

```yaml
name: Inflate hydros AI annotaions
kind: HydrosAI
metadata:
name: Inflate hydros AI annotaions
spec:
filterSpecs:
  - openapi: 3.0.0
    info:
      title: Workload Identity Generator
      version: 1.0.0
      description: A high level API for generating the resources needed to enable workload identity on a GKE cluster. The API takes care of creating the Kubernetes and Google service accounts and IAM bindings as needed.
    paths: {}
    components:
      schemas:
        WorkloadIdentity:
          type: object
          properties:
            spec:
              description: The spec provides the high level API for the workload resources.
              type: object
              properties:
                requirement:
                  type: string
                  description: This should be a natural language description of what this WorkloadIdentity is doing; for example "Create a KSA foo bound to GSA dev@acme.com with cloud storage permissions"
                ksa:
                  type: object
                  properties:
                    name:
                      type: string
                      description: The name of the kubernetes service account to bind to the Google service account
                    create:
                      type: boolean
                      description: Whether the kubernetes service account should be created if it doesn't exist
                gsa:
                  type: object
                  properties:
                    name:
                      type: string
                      description: The name of the google service account to bind to the kubernetes service account
                    create:
                      type: boolean
                      description: Whether the google service account should be created if it doesn't exist
                    iamBindings:
                      type: array
                      description: A list of the GCP iam roles that should be assigned to the GSA if they aren't already. For a list of roles refer to https://cloud.google.com/iam/docs/understanding-roles
                      items:
                        type: string
```

On your Kubernetes resources add annotations with the key `ai.hydros.io/${TAG}`. The value
should be a natural language description of the function you want to invoke. The ${TAG} is an arbitrary string
which makes the key unique; this allows for multiple annotations on a resource.  Here's an example

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: trainer
  annotations:
    ai.hydros.io/1: “Attach the GCP service account trainer with GCS and BigQuery edit permissions. Create the service account if it doesn’t exist”
  ...
```

When you run hydrate hydros will parse the annotations use OpenAI to turn the natural language description into a
kustomize function which it writes out to the source directory. For example, the above annotation would be turned into

```yaml
apiVersion: v1
kind: WorkloadIdentity
metadata:
  name: attach-trainer-gcp-sa
  annotations:
    owner.hydros.io/: "{\"hash\":\"f5faf45dba60ad4c4a91f2b3a6e592e59837d858d2f5f5ff7f9bb794290ad47c\",\"prompt\":\"“Attach the GCP service account trainer with GCS and BigQuery edit permissions. Create the service account if it doesn’t exist”\",\"response\":\"{\\\"id\\\":\\\"chatcmpl-7FrOtNfSrKRiZo35Qn7tpFtt3ntlV\\\",\\\"object\\\":\\\"chat.completion\\\",\\\"created\\\":1684014547,\\\"model\\\":\\\"gpt-3.5-turbo-0301\\\",\\\"choices\\\":[{\\\"index\\\":0,\\\"finish_reason\\\":\\\"stop\\\",\\\"message\\\":{\\\"role\\\":\\\"assistant\\\",\\\"content\\\":\\\"Here is the YAML definition of the corresponding custom resource based on the provided schema:\\\\n```\\\\napiVersion: v1\\\\nkind: WorkloadIdentity\\\\nmetadata:\\\\n  name: attach-trainer-gcp-sa\\\\nspec:\\\\n  requirement: \\\\\\\"Attach the GCP service account trainer with GCS and BigQuery edit permissions. Create the service account if it doesn’t exist\\\\\\\"\\\\n  ksa:\\\\n    create: true\\\\n    name: trainer\\\\n  gsa:\\\\n    create: true\\\\n    name: trainer@\\\\u003cPROJECT_ID\\\\u003e.iam.gserviceaccount.com\\\\n    iamBindings:\\\\n      - roles/storage.objectAdmin\\\\n      - roles/bigquery.dataEditor\\\\n```\\\\nMake sure to replace `\\\\u003cPROJECT_ID\\\\u003e` with the actual GCP project ID.\\\"}}],\\\"usage\\\":{\\\"prompt_tokens\\\":465,\\\"completion_tokens\\\":150,\\\"total_tokens\\\":615}}\"}"
spec:
  gsa:
    name: trainer@<PROJECT_ID>.iam.gserviceaccount.com
    create: true
    iamBindings:
    - roles/storage.objectAdmin
    - roles/bigquery.dataEditor
  ksa:
    name: trainer
    create: true
  requirement: "Attach the GCP service account trainer with GCS and BigQuery edit permissions. Create the service account if it doesn’t exist"
```

Note this requires access to OpenAI. You must provide an OpenAI API key in the `OPENAI_API_KEY` environment variable.