# AWS Topology Provider

The AWS topology provider is based on the [DescribeInstanceTopology API](https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_DescribeInstanceTopology.html).
This API returns a list of EC2 instances, where each instance record includes an array of three network IDs.
These IDs describe a path through the three-tier network, from the leaf up to the root. Each network ID represents a group of
physical switches that share similar characteristics and connectivity patterns. Additionally, a record might include a
`CapacityBlockId`, which corresponds to the node’s NVLink domain.

Access to this API requires an IAM account with the `AmazonEC2ReadOnlyAccess` policy attached.
There are two main authentication methods:

* Providing IAM credentials directly
* Assigning an IAM role to the instance running Topograph

## Option 1: Using Explicit Credentials

AWS credentials consist of:
* `accessKeyId`
* `secretAccessKey`
* (Optional) `token`

You can provide credentials in several ways:

### Credentials via File

Store your credentials in a YAML file:

```yaml
accessKeyId: <ACCESS_KEY_ID>
secretAccessKey: <SECRET_ACCESS_KEY>
token: <OPTIONAL_TOKEN>
```

Then reference this file in your Topograph config:

```yaml
http:
  port: 49021
  ssl: false

provider: aws
engine: slurm

credentialsPath: /path/to/credentials.yaml
```

### Credentials via API Request Payload

Pass credentials directly in the topology request payload:

```json
{
  "provider": {
    "creds": {
      "accessKeyId": "<ACCESS_KEY_ID>",
      "secretAccessKey": "<SECRET_ACCESS_KEY>",
      "token": "<OPTIONAL_TOKEN>"
    }
  }
}
```

### Credentials via Environment Variables

Set IAM credentials as environment variables before starting the Topograph process:

```sh
export AWS_ACCESS_KEY_ID=<ACCESS_KEY_ID>
export AWS_SECRET_ACCESS_KEY=<SECRET_ACCESS_KEY>
export AWS_SESSION_TOKEN=<OPTIONAL_TOKEN>
```

## Option 2: Assigning IAM Role to an Instance

Alternatively, you can assign an IAM role to the compute node running Topograph. In this case, explicit credentials are not required, as AWS automatically provides the necessary permissions.
For more information, refer to the [documentation](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/attach-iam-role.html).
