# OCI Topology Provider

Topograph supports two variants of the OCI topology provider: one based on the Instance Metadata Service (IMDS) and another based on the OCI Core Services API.

## IMDS-based topology provider.
This variant can only run in SLURM clusters. Topograph collects topology information from each participating node by querying the node’s IMDS endpoint http://169.254.169.254/opc/v2/host/rdmaTopologyData.

No additional OCI authorization is required; access to the IMDS is local to the node. The only prerequisite is the ability to SSH into the worker nodes.

## API-based topology provider.
This is the default OCI topology provider. It uses the [ComputeClient.ListComputeHosts()](https://docs.oracle.com/en-us/iaas/tools/go/65.100.1/core/index.html#ComputeClient.ListComputeHosts) method from the OCI Core Services SDK, which calls the `ListDedicatedVmHosts` API.

This API returns information about compute hosts, including identifiers for HPC network domains, which correspond to the underlying network tiers used for placement and interconnects.

Access to this API requires authorization. Specifically, the caller must have a policy that permits either `inspect` or `use` actions on dedicated VM hosts within the relevant tenancy.
The minimum IAM verb required is `inspect dedicated-vm-hosts`. This allows listing and retrieving metadata but does not allow creating or deleting hosts.

Example IAM policy for a dynamic group:
```
Allow dynamic-group <dynamic-group> to inspect dedicated-vm-hosts in tenancy
```

There are two main methods to authenticate:

* Provide IAM credentials explicitly
* Assign an IAM role to the instance running Topograph

### Option 1: Using Explicit Credentials

OCI credentials consist of:
* `tenancyId`: OCI tenant OCID
* `userId`: OCI user OCID
* `region`: target region
* `fingerprint`: public key fingerprint
* `privateKey`: private key
* (Optional) `passphrase`: private key passphrase

You can provide credentials in two ways:

#### Credentials via File

Store your credentials in a YAML file:

```yaml
tenancyId: <OCI_TENANCY_ID>
userId: <OCI_USER_ID>
region: <REGION>
fingerprint: <FINGERPRINT>
privateKey: <PRIVATE_KEY>
passphrase: <OPTIONAL_PASSPHRASE>
```

Then reference this file in your Topograph config:

```yaml
http:
  port: 49021
  ssl: false

provider: oci
engine: slurm

credentials_path: /path/to/credentials.yaml
```

#### Credentials via API Request Payload

Pass credentials directly in the topology request payload:

```json
{
  "provider": {
    "creds": {
      "tenancyId": "<OCI_TENANCY_ID>",
      "userId": "<OCI_USER_ID>",
      "region": "<REGION>",
      "fingerprint": "<FINGERPRINT>",
      "privateKey": "<PRIVATE_KEY>",
      "passphrase": "<OPTIONAL_PASSPHRASE>"
    }
  }
}
```

### Option 2: Assigning IAM Role to an Instance

Instead of providing credentials, you can assign an IAM role to the compute instance running Topograph. OCI automatically injects temporary credentials, so explicit credentials are not needed.

For details, see [Calling Services from an Instance](https://docs.oracle.com/en-us/iaas/Content/Identity/Tasks/callingservicesfrominstances.htm).
