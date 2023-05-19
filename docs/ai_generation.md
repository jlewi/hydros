# Natural Language API For Configuration Changes

Hydros provides experimental support for using natural language to describe
configuration changes. This feature lets you describe configuration changes
using natural language by adding annotations to your configuration files.
HydrosAI will then automatically generate the configuration changes for you.

**Important**: This feature relies on OpenAI's APIs and incurs charges 
related to inference.

There are two parts to configuring Hydros AI

1. Configuring the HydrosAI [KRM function](https://kpt.dev/book/02-concepts/03-functions)
   * This KRM function is responsible for generating the configuration changes
     from the natural language descriptions.
2. Attaching `ai.hydros.io/${TAG}` annotations to your configuration files.
   * These annotations are used by the HydrosAI KRM function to generate the
     configuration changes.

Hydros AI works very similar to how [OpenAI Plugins](https://platform.openai.com/docs/plugins/introduction)
work. In this case, plugins are KRM functions. AI is used to turn natural language descriptions of 
plugins into KRM function declarations.

## Motivation

A major challenge to using Cloud is creating levels of abstraction for configuration that reflect the separation
of concerns among teams [Tweet thread](https://twitter.com/kelseyhightower/status/1646538701818986501?s=20).
For example, many developers within an organization may need to spin up simple web apps accessible over the intranet. 
However, only a small number of developers may have a good understanding of networking 
(e.g. ISTIO, Gateway, Certificates, etc...).

KRM functions provide a mechanism for platform teams to create abstractions that hide low level details. A platform
team could create a KRM function that takes a declaration like the following in order to create a route on the
intranet:

```yaml
apiVersion: v1
kind: CorpRoute
metadata:
  name: foo
  namespace: foo
spec:
  prefix: "/foo"
  service: foo
```

The proliferation of KRM functions, however, could exacerbate the headache of remembering the precise syntax for 
lots of different resources. Hydros AI aims to solve this by letting developers use natural language to 
describe the infrastructure they want. Hydros AI then takes care of turning that description into a KRM function 
declaration or returning an error if there is no suitable KRM function.

For example, in the example rather above than directly defining `CorpRoute`, a developer could instead add an
`ai.hydros.io/1` annotation to their deployment.

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: foo
  namespace: foo
  annotations:
    ai.hydros.io/1: "Expose the foo service on the corporate intranet at /foo"
```

The developer can then run `hydros generate` to use AI to generate the KRM function declaration for the `CorpRoute`.

## Configuring the HydrosAI KRM function

Define a KRM function which specifies all the KRM functions you want Hydros to be aware of.
For each KRM function provide a `FilterSpec` which is an OpenAPI schema of the function.
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

As with OpenAI plugins, it is important to provide good descriptions of the function and its parameters as this is
what the AI relies on to turn natural language descriptions into KRM function declarations.

## Attaching `ai.hydros.io/${TAG}` annotations to your configuration files

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

When you run `hydrate generate`, hydros uses OpenAI's models to turn the natural language description into a
KRM function which it writes out to the configuration directory. For example, the above annotation would be turned into

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

## Running Hydros Generate

To generate the KRM functions run `hydros generate`

```bash
export OPENAI_API_KEY=<Your OpenAI API_KEY>
hydros generate <Directory containing YAML>
```

For each file containing a `ai.hydros.io/${TAG}` annotation, Hydros will generate a the file "$file_ai_generated.yaml"
containing the KRM function declarations generated from the annotation. 

**Important** The AI is not perfect and will sometimes generate invalid or incorrect KRM functions.

## References

[Blog on Cruise's Teams](https://medium.com/cruise/building-a-container-platform-at-cruise-part-1-507f3d561e6f) - A
useful reference point for the myriad ways organizations can be structured.
