# GCP Topology Provider

The GCP topology provider relies on the [Google Cloud Compute Engine API](https://cloud.google.com/compute/docs/reference/rest/v1/instances/list).
This API returns a list of VM instances. Each instance record may include a `physicalHostTopology` field, which describes the network topology of the compute node.

To call this API, you must have an IAM role that grants the `compute.instances.list` permission on the target project and zone. A common example is the `roles/compute.viewer` role.

For more information about IAM roles and how to grant permissions, refer to the following documentation:

* [Roles overview](https://cloud.google.com/iam/docs/roles-overview)
* [Manage access to projects, folders, and organizations](https://cloud.google.com/iam/docs/granting-changing-revoking-access)
* [Grant a role using the Google Cloud console](https://cloud.google.com/iam/docs/grant-role-console)
* [Grant a role using gcloud](https://cloud.google.com/sdk/gcloud/reference/projects/add-iam-policy-binding)
