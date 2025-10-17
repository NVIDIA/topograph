# Nebius Topology Provider

The Nebius topology provider uses the [Nebius AI Cloud SDK for Go](https://github.com/nebius/gosdk).
The `Services().Compute().V1().Instance().List()` method returns a list of compute instances for a specified project.
Each instance may include a `Status.InfinibandTopologyPath` field, which is an array of three network IDs. If present, these IDs describe the path through the three-tier network, from the root switch down to the leaf switch.

To use the API, you must provide authorization.
There are two ways to do this: using account credentials or an authorization token.

## Using credentials:

Nebius credentials consist of the following fields:
* `serviceAccountId`
* `publicKeyId`
* `privateKey`

You can provide credentials either in the Topology configuration file or directly in the topology request payload.

### Credentials via File

Store your credentials in a YAML file:

```yaml
serviceAccountId: <SERVICE-ACCOUNT-ID>
publicKeyId: <PUBLIC-KEY-ID>
privateKey: <PRIVATE-KEY>
```

Then reference this file in your Topograph config:

```yaml
http:
  port: 49021
  ssl: false

provider: nebius
engine: slurm

credentialsPath: /path/to/credentials.yaml
``` 

### Credentials via API Request Payload

Pass credentials directly in the topology request payload:

```json
{
  "provider": {
    "creds": {
      "serviceAccountId": "<SERVICE-ACCOUNT-ID>",
      "publicKeyId": "<PUBLIC-KEY-ID>",
      "privateKey": "<PRIVATE-KEY>"
    }
  }
}
```

## Using authorization token

You can provide an authorization token in one of two ways:
* Via the environment variable `IAM_TOKEN`
* By placing the token in the file `/mnt/cloud-metadata/token`
