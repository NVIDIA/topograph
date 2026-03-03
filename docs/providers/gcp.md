# GCP Topology Provider

The GCP topology provider relies on the [Google Cloud Compute Engine API](https://cloud.google.com/compute/docs/reference/rest/v1/instances/list)
to retrieve a list of VM instances. Each instance record may include the
`physicalHostTopology` field, which describes the network topology of the
underlying compute node.

Access to the Compute Engine API must be authorized.

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

In your Helm values file, set `config.credentialsSecret` to the name of the
created secret. This instructs the Helm chart to set the
`GOOGLE_APPLICATION_CREDENTIALS` environment variable for Topograph.

Example:

```yaml
config:
  credentialsSecret: gcp-compute-client-key
```

## Authentication Using GCP Workload Identity Federation (EKS)

When running Topograph in Kubernetes cluster, one supported authentication method is to use
a **GCP Workload Identity Federation** with **Application Default Credentials (ADC)**.

### 1. Identify the values for the parameters

Identify the values specific to the setup, and replace the env variables with the corresponding values.

```bash
## EKS
export EKS_CLUSTER="<name of eks cluster>"
export AWS_REGION="<aws region>"
export OIDC_ISSUER=$(aws eks describe-cluster --name "$EKS_CLUSTER" --region "$AWS_REGION" --query "cluster.identity.oidc.issuer" --output text)

## GCP
export GCP_PROJECT="<name of the GCP project>"
export GCP_PROJECT_NUMBER="<id number of the GCP project>"
export WORKLOAD_POOL_ID="eks-pool"
export WORKLOAD_POOL_NAME="AWS EKS Workload Identity Pool"
export WORKLOAD_POOL_DESC="AWS EKS Workload Identity Pool"
export WORKLOAD_PROVIDER_ID="eks-workload-provider"
export ATTRIBUTE_MAPPING="google.subject=assertion.sub"
#assertion.sub contains system:serviceaccount:NAMESPACE:KSA_NAME 

## GCP Service Account (GSA) details
export GSA_NAME="compute-client"
export GSA_DESC="Topograph API client"
export GSA_PROJECT="<name of the GCP project where the GSA is created>" # could be same as $GCP_PROJECT or different
export GSA_ROLE="roles/compute.viewer"
export GSA_EMAIL=$GSA_NAME@$GSA_PROJECT.iam.gserviceaccount.com

## Kubernetes Service Account (KSA) details
export NAMESPACE="<namespace name where topograph will be deployed>"
export KSA_NAME="<kubernetes service account name for topograph>"
export CRED_CONFIG_MAP="<kubernetes config map name for GCP credentials>"
```

### 2. Create a GCP service account (GSA)
Create a GCP Service Account (if it doesn't exist already).

```bash
gcloud iam service-accounts create $GSA_NAME --project $GSA_PROJECT --display-name="$GSA_DESC"
```

### 3. Grant minimum required permissions

Grant the GSA read-only access to Compute Engine resources:

```bash
gcloud projects add-iam-policy-binding $GCP_PROJECT \
  --member="serviceAccount:$GSA_EMAIL" \
  --role=$GSA_ROLE
```

### 4. Create a GCP Workload Identity Pool

```bash
gcloud iam workload-identity-pools create $WORKLOAD_POOL_ID \
    --project=$GCP_PROJECT \
    --location="global" \
    --description="$WORKLOAD_POOL_DESC" \
    --display-name="$WORKLOAD_POOL_NAME" 
```

### 5. Create a GCP Workload Identity Provider
```bash
gcloud iam workload-identity-pools providers create-oidc $WORKLOAD_PROVIDER_ID \
 --project=$GCP_PROJECT \
 --location="global" \
 --workload-identity-pool="$WORKLOAD_POOL_ID" \
 --issuer-uri="$OIDC_ISSUER" \
 --attribute-mapping="$ATTRIBUTE_MAPPING"
```

### 6. Grant Kubernetes Service Account (KSA) permission to impersonate GCP Service Account (GSA)
```bash
gcloud iam service-accounts add-iam-policy-binding $GSA_EMAIL \
  --member="principal://iam.googleapis.com/projects/$GCP_PROJECT_NUMBER/locations/global/workloadIdentityPools/$WORKLOAD_POOL_ID/subject/system:serviceaccount:$NAMESPACE:$KSA_NAME" \
  --role=roles/iam.workloadIdentityUser
```

### 7. Create credential configuration file 
```bash
gcloud iam workload-identity-pools create-cred-config \
    projects/$GCP_PROJECT_NUMBER/locations/global/workloadIdentityPools/$WORKLOAD_POOL_ID/providers/$WORKLOAD_PROVIDER_ID \
    --service-account=$GSA_EMAIL \
    --credential-source-file=/var/run/service-account/token \
    --credential-source-type=text \
    --output-file=credentials-config.json
```

### 8. Create a Kubernetes Config Map 
Create a k8s config map from the output of the previous command.

```bash
kubectl create configmap $CRED_CONFIG_MAP --from-file=credentials-config.json
```

### 9. Configure Helm values

In the Helm values file for the deployment, set the following parameters :
* `global.provider.params.workloadIdentityFederation.credentialsConfigmap` to the name of the created config map in step 8.
* `global.provider.params.workloadIdentityFederation.audience` to the `audience` attribute in the `credentials-config.json` created in step 7.

This instructs the Helm chart to set the `GOOGLE_APPLICATION_CREDENTIALS` environment variable for Topograph.

Example:

```yaml
global:
  provider:
    params:
      workloadIdentityFederation:
        credentialsConfigmap: gcp-credentials-config
        audience: "//iam.googleapis.com/projects/123/locations/global/workloadIdentityPools/my-pool/providers/my-workload-provider"
```
For more information about setting Google Workload Identity Federation, refer to the following documentation:

* [GCP Workload Identity Federation](https://cloud.google.com/iam/docs/workload-identity-federation-with-kubernetes)

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
