# AWS Topology Provider

The AWS topology provider is based on the [DescribeInstanceTopology API](https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_DescribeInstanceTopology.html).
This API returns a list of EC2 instances, where each instance record includes an array of three network IDs.
These IDs describe a path through the three-tier network, from the leaf up to the root. Each network ID represents a group of
physical switches that share similar characteristics and connectivity patterns. Additionally, a record might include a
`CapacityBlockId`, which corresponds to the node’s NVLink domain.

Access to this API requires the following IAM permission:

```json
{
  "Effect": "Allow",
  "Action": "ec2:DescribeInstanceTopology",
  "Resource": "*"
}
```

There are two authentication modes:

1. Explicit credentials supplied through Topograph
2. AWS SDK default credential chain

## Option 1: Using Explicit Credentials

AWS credentials consist of:
* `accessKeyId`
* `secretAccessKey`
* (Optional) `token`

Credentials in an API request take precedence over credentials loaded from `credentialsPath`. Both take precedence over the AWS SDK default credential chain.

You can provide explicit credentials in two ways:

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

## Option 2: Using the AWS SDK Default Credential Chain

When explicit credentials are not provided, Topograph uses the [AWS SDK default credential chain](https://docs.aws.amazon.com/sdk-for-go/v2/developer-guide/configure-gosdk.html). This supports environment variables, shared AWS configuration, EKS Pod Identity, IAM Roles for Service Accounts (IRSA), and the IAM role assigned to the instance running Topograph.

### EKS Pod Identity

Install the [EKS Pod Identity Agent](https://docs.aws.amazon.com/eks/latest/userguide/pod-id-agent-setup.html), then [associate an IAM role](https://docs.aws.amazon.com/eks/latest/userguide/pod-id-association.html) with the ServiceAccount used by the Topograph API server. EKS injects the container credentials endpoint and authorization token into the pod; the AWS SDK discovers and refreshes those temporary credentials automatically.

The SDK may select environment or shared-profile credentials before EKS Pod Identity or an EC2 instance role. If either role should be authoritative, make sure higher-priority sources are not configured, including `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY`, `AWS_PROFILE`, `~/.aws/config`, and `~/.aws/credentials`.

### Assigning IAM Role to an Instance

Alternatively, you can assign an IAM role to the compute node running Topograph. In this case, explicit credentials are not required, as AWS automatically provides the necessary permissions.
For more information, refer to the [documentation](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/attach-iam-role.html).
