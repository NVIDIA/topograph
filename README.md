<p align="center"><a href="https://github.com/NVIDIA/topograph" target="_blank"><img src="docs/assets/topograph-logo.png" width="100" alt="Logo"></a></p>

# Topograph

![Build Status](https://github.com/NVIDIA/topograph/actions/workflows/go.yml/badge.svg)
![Codecov](https://codecov.io/gh/NVIDIA/topograph/branch/main/graph/badge.svg)
![Static Badge](https://img.shields.io/badge/license-Apache_2.0-green)

Topograph is a component designed to expose the underlying physical network topology of a cluster to enable a workload manager make network-topology aware scheduling decisions.

Topograph consists of five major components:

1. **API Server**
2. **Node Observer**
3. **Node Data Broker**
4. **Provider**
5. **Engine**

<p align="center"><img src="docs/assets/design.png" width="600" alt="Design"></p>

## Components

### 1. API Server

The API Server handles and validates topology requests. It listens for network topology configuration requests on a specific port. When a request is received, the server triggers the Provider to initiate topology discovery.

### 2. Node Observer

The Node Observer is used in Kubernetes deployments. It monitors changes to cluster nodes. If a node goes down or comes online, the Node Observer sends a request to the API Server to generate a new topology configuration.

### 3. Node Data Broker

The Node Data Broker is also used when Topograph is deployed in a Kubernetes cluster. It collects relevant node attributes and stores them as node annotations.

### 4. Provider

The Provider interfaces with CSPs or on-premises tools to retrieve topology-related data from the cluster and converts it into an internal representation.

### 5. Engine

The Engine translates this internal representation into the format expected by the workload manager.

## Workflow

- The API Server listens on the port and notifies the Provider about incoming requests. In Kubernetes, the incoming requests are sent by the Node Observer, which watches changes in the node status.
- The Provider receives notifications and invokes CSP API to retrieve topology-related information.
- The Engine converts the topology information into the format expected by the user cluster (e.g., SLURM or Kubernetes).

## Configuration

Topograph accepts its configuration file path using the `-c` command-line parameter. The configuration file is a YAML document. A sample configuration file is located at [config/topograph-config.yaml](config/topograph-config.yaml).

The configuration file supports the following parameters:

```yaml
# serving topograph endpoint
http:
  # port: specifies the port on which the API server will listen (required).
  port: 49021
  # ssl: enables HTTPS protocol if set to `true` (optional).
  ssl: false

# provider: the provider that topograph will use (optional)
# Valid options include "aws", "oci", "gcp", "nebius", "netq", "baremetal" or "test".
# Can be overridden if the provider is specified in a topology request to topograph
provider: test

# engine: the engine that topograph will use (optional)
# Valid options include "slurm", "k8s", or "slinky".
# Can be overridden if the engine is specified in a topology request to topograph
engine: slurm

# request_aggregation_delay: defines the delay before processing a request (required).
# Topograph aggregates multiple sequential requests within this delay into a single request,
# processing only if no new requests arrive during the specified duration.
request_aggregation_delay: 15s

# forward_service_url: specifies the URL of an external gRPC service
# to which requests are forwarded (optional).
# This can be useful for testing or integration with external systems.
# See protos/topology.proto for details.
# forward_service_url:

# page_size: sets the page size for topology requests against a CSP API (optional).
page_size: 100

# ssl: specifies the paths to the TLS certificate, private key,
# and CA certificate (required if `http.ssl=true`).
ssl:
  cert: /etc/topograph/ssl/server-cert.pem
  key: /etc/topograph/ssl/server-key.pem
  ca_cert: /etc/topograph/ssl/ca-cert.pem
# credentials_path: specifies the path to a file containing CSP credentials (optional).
# credentials_path:

# env: environment variable names and values to inject into Topograph's shell (optional).
# The `PATH` variable, if provided, will append the specified value to the existing `PATH`.
# env:
#  SLURM_CONF: /etc/slurm/slurm.conf
#  PATH:
```

## Supported Environments

Topograph operates with two primary concepts: `provider` and `engine`. A `provider` represents a CSP or a similar environment, while an `engine` refers to a scheduling system like SLURM or Kubernetes.

Currently supported providers:

- AWS
- OCI
- GCP
- Nebius
- Bare metal

Currently supported engines:

- [SLURM](./docs/slurm.md)
- [Kubernetes](./docs/k8s.md)
- [SLURM-on-Kubernetes (Slinky)](./docs/slinky.md)

## Using Topograph

Topograph offers three endpoints for interacting with the service. Below are the details of each endpoint:

### 1. Health Endpoint

- **URL:** `http://<server>:<port>/healthz`
- **Description:** This endpoint verifies the service status. It returns a "200 OK" HTTP response if the service is operational.

### 2. Topology Request Endpoint

- **URL:** `http://<server>:<port>/v1/generate`
- **Description:** This endpoint is used to request a new cluster topology.
- **Payload:** The payload is a JSON object that includes the following fields:

  - **provider name**: (optional) A string specifying the Service Provider, such as `aws`, `oci`, `gcp`, `nebius`, `netq`, `baremetal` or `test`. This parameter will be override the provider set in the topograph config.
  - **provider credentials**: (optional) A key-value map with provider-specific parameters for authentication.
  - **provider parameters**: (optional) A key-value map with parameters that are used for provider simulation with toposim.
    - **model_path**: (optional) A string parameter that points to the model file to use for simulating topology.
  - **engine name**: (optional) A string specifying the topology output, either `slurm`, `k8s`, or `slinky`. This parameter will override the engine set in the topograph config.
  - **engine parameters**: (optional) A key-value map with engine-specific parameters.
    - **slurm parameters**:
      - **topology_config_path**: (optional) A string specifying the file path for the topology configuration. If omitted, the topology config content is returned in the HTTP response.
      - **plugin**: (optional) A string specifying topology plugin: `topology/tree` (default) or `topology/block`.
      - **block_sizes**: (optional) A string specifying block size for `topology/block` plugin.
      - **reconfigure**: (optional) If `true`, invoke `scontrol reconfigure` after topology config is generated. Default `false`
    - **slinky parameters**:
      - **namespace**: A string specifying namespace where SLURM cluster is running.
      - **podSelector**: A standard Kubernetes label selector for pods running SLURM nodes.
      - **plugin**: (optional) A string specifying topology plugin: `topology/tree` (default) or `topology/block`.
      - **block_sizes**: (optional) A string specifying block size for `topology/block` plugin.
      - **topology_config_path**: A string specifying the key for the topology config in the ConfigMap.
      - **topology_configmap_name**: A string specifying the name of the ConfigMap containing the topology config.
  - **nodes**: (optional) An array of regions mapping instance IDs to node names.

  Example:

```json
{
  "provider": {
    "name": "aws",
    "creds": {
      "access_key_id": "id",
      "secret_access_key": "secret"
    },
    "params": {
      "model_path": ""
    }
  },
  "engine": {
    "name": "slurm",
    "params": {
      "plugin": "topology/block",
      "block_sizes": "30,120"
    }
  },
  "nodes": [
    {
      "region": "region1",
      "instances": {
        "instance1": "node1",
        "instance2": "node2",
        "instance3": "node3"
      }
    },
    {
      "region": "region2",
      "instances": {
        "instance4": "node4",
        "instance5": "node5",
        "instance6": "node6"
      }
    }
  ]
}
```

- **Response:** This endpoint immediately returns a "202 Accepted" status with a unique request ID if the request is valid. If not, it returns an appropriate error code.

### 3. Topology Result Endpoint

- **URL:** `http://<server>:<port>/v1/topology`
- **Description:** This endpoint retrieves the result of a topology request.
- **URL Query Parameters:**
  - **uid**: Specifies the request ID returned by the topology request endpoint.
- **Response:** Depending on the request's execution stage, this endpoint can return:
  - "404 NotFound" if the configuration is not ready yet.
  - "200 OK" if the request has been completed successfully.
  - "500 InternalServerError" if there was an error during request execution.

Example usage:

```bash
id=$(curl -s -X POST -H "Content-Type: application/json" -d @payload.json http://localhost:49021/v1/generate)

curl -s "http://localhost:49021/v1/topology?uid=$id"
```
