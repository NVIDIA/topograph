# GCP Topology Provider

The GCP topology provider relies on the [Google Cloud Compute Engine API](https://cloud.google.com/compute/docs/reference/rest/v1/instances/list)
to retrieve a list of VM instances. Each instance record may include the
`physicalHostTopology` field, which describes the network topology of the
underlying compute node.

Access to the Compute Engine API must be authorized.

## Authentication Using a Service Account (ADC)

When running Topograph outside of GCP, one supported authentication method is to use
a **Google Cloud service account** with **Application Default Credentials (ADC)**.

### 1. Create a service account

```bash
gcloud iam service-accounts create compute-client \
  --display-name="Topograph API client"
```

### 2. Grant minimum required permissions

Grant the service account read-only access to Compute Engine resources:

```bash
gcloud projects add-iam-policy-binding YOUR_PROJECT_ID \
  --member="serviceAccount:compute-client@YOUR_PROJECT_ID.iam.gserviceaccount.com" \
  --role="roles/compute.viewer"
```

### 3. Create a service account key

```bash
gcloud iam service-accounts keys create key.json \
  --iam-account=compute-client@YOUR_PROJECT_ID.iam.gserviceaccount.com
```

### 4. Create a Kubernetes Secret from the key

```bash
kubectl create secret generic <secret-name> --from-file=credentials.yaml=<path/to/key.json>
```

### 5. Configure Helm values

In your Helm values file, set `config.credentialsSecretName` to the name of the
created secret. This instructs the Helm chart to set the
`GOOGLE_APPLICATION_CREDENTIALS` environment variable for Topograph.

Example:

```yaml
config:
  credentialsSecretName: gcp-compute-client-key
```

## Authentication When Running on GCP

If Topograph is running on a **GCP compute service**, you can authenticate without service account keys.

Attach or use a service account that grants the
`compute.instances.list` permission on the target project and zone.
A common example is the `roles/compute.viewer` role.

For more information about IAM roles and how to grant permissions, refer to the following documentation:

* [Roles overview](https://cloud.google.com/iam/docs/roles-overview)
* [Manage access to projects, folders, and organizations](https://cloud.google.com/iam/docs/granting-changing-revoking-access)
* [Grant a role using the Google Cloud console](https://cloud.google.com/iam/docs/grant-role-console)
* [Grant a role using gcloud](https://cloud.google.com/sdk/gcloud/reference/projects/add-iam-policy-binding)

## Setting Project ID

When calling the GCP API, the project ID is provided to specify the scope of the request.
By default, when using ADC, the project ID is fetched from the service account key.
When running on a GCP compute node, the project ID is extracted from the node metadata.

You can override the project ID by setting `project_id` provider parameter in the topology request payload:
```
{
  "provider": {
    "name": "gcp",
    "params": {
      "project_id": "your-project-id"
    }
  }
}
```
