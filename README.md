<p align="center"><a href="https://github.com/NVIDIA/topograph" target="_blank"><img src="docs/assets/topograph-logo.png" width="100" alt="Logo"></a></p>

# Topograph

![Build Status](https://github.com/NVIDIA/topograph/actions/workflows/go.yml/badge.svg)
![Codecov](https://codecov.io/gh/NVIDIA/topograph/branch/main/graph/badge.svg)
![Static Badge](https://img.shields.io/badge/license-Apache_2.0-green)

Topograph is a component that discovers the physical network topology of a cluster and exposes it to schedulers, enabling topology-aware scheduling decisions. It abstracts multiple topology sources and translates them into the format required by each scheduler.

## Motivation and Problem Statement

At scale, workload placement becomes as important as resource allocation. Where a job runs can have a significant impact on its performance. For example, a distributed training job that spans nodes on opposite sides of a data center fabric incurs additional latency during every gradient synchronization. Similarly, a disaggregated inference pipeline that ignores NVLink locality may fail to utilize the full interconnect bandwidth available. In both cases, overlooking the underlying network topology leads to inefficient execution.

The physical network topology of a cluster plays a critical role in determining application performance. Modern GPU clusters are typically built on multi-tier network fabrics, where communication costs vary depending on node placement. Nodes connected to the same leaf switch can communicate with lower latency and higher bandwidth than nodes separated by multiple network hops. On advanced systems such as GB200/GB300 NVL72, groups of nodes share a high-speed NVLink fabric, forming a locality domain that significantly outperforms even the fastest Ethernet or InfiniBand connections.

To make optimal placement decisions, schedulers must be aware of this topology. However, topology information is often fragmented across multiple sources, including cloud provider APIs, fabric management systems, and low-level system tools. Each source exposes data through different interfaces and formats. At the same time, workload managers, such as Slurm, Kubernetes, or Slurm-on-Kubernetes deployments like Slinky, require this information in their own specific formats.

Topograph addresses this challenge. It discovers the physical network topology of a cluster and exposes it to schedulers in a form they can consume. By abstracting over diverse topology sources and translating them into the required output formats, Topograph transforms what would otherwise be a manual, environment-specific process into a unified and extensible pipeline.

## Architecture

Topograph consists of five major components:

1. **API Server**
2. **Node Observer**
3. **Node Data Broker**
4. **Provider**
5. **Engine**

<p align="center"><img src="docs/assets/design.png" width="600" alt="Design"></p>

### 1. API Server

The API Server receives topology generation requests and returns results asynchronously. Requests are aggregated over a configurable delay window so that a burst of node changes (common during cluster scaling events) produces a single topology update rather than a storm.

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
# Valid options include "aws", "oci", "gcp", "nebius", "netq", "dra", "infiniband-k8s", "infiniband-bm" or "test".
# Can be overridden if the provider is specified in a topology request to topograph
provider: test

# engine: the engine that topograph will use (optional)
# Valid options include "slurm", "k8s", or "slinky".
# Can be overridden if the engine is specified in a topology request to topograph
engine: slurm

# requestAggregationDelay: defines the delay before processing a request (required).
# Topograph aggregates multiple sequential requests within this delay into a single request,
# processing only if no new requests arrive during the specified duration.
requestAggregationDelay: 15s

# forwardServiceUrl: specifies the URL of an external gRPC service
# to which requests are forwarded (optional).
# This can be useful for testing or integration with external systems.
# See protos/topology.proto for details.
# forwardServiceUrl:

# pageSize: sets the page size for topology requests against a CSP API (optional).
pageSize: 100

# ssl: specifies the paths to the TLS certificate, private key,
# and CA certificate (required if `http.ssl=true`).
ssl:
  cert: /etc/topograph/ssl/server-cert.pem
  key: /etc/topograph/ssl/server-key.pem
  ca_cert: /etc/topograph/ssl/ca-cert.pem
# credentialsPath: specifies the path to a YAML file containing API credentials (optional).
# When using credentials in Kubernetes-based engines ("k8s" or "slinky"),
# the secret file must be named `credentials.yaml`. For example:
# `kubectl create secret generic <secret-name> --from-file=credentials.yaml=<path to credentials>`
# For more details about credential configuration, refer to the docs/providers section.
# credentialsPath:

# env: environment variable names and values to inject into Topograph's shell (optional).
# The `PATH` variable, if provided, will append the specified value to the existing `PATH`.
# env:
#  SLURM_CONF: /etc/slurm/slurm.conf
#  PATH:
```

## Supported Environments

Topograph operates with two primary concepts: `provider` and `engine`. A `provider` represents a CSP or a similar environment, while an `engine` refers to a scheduling system like SLURM or Kubernetes.

Currently supported providers:

- [AWS](./docs/providers/aws.md)
- [OCI](./docs/providers/oci.md)
- [GCP](./docs/providers/gcp.md)
- [Nebius](./docs/providers/nebius.md)
- [NetQ](./docs/providers/netq.md)
- [DRA](./docs/providers/dra.md) — reads `nvidia.com/gpu.clique` labels set by the NVIDIA GPU operator DRA driver
- [InfiniBand (bare-metal)](./docs/providers/infiniband.md#infiniband-bm-bare-metal)
- [InfiniBand (Kubernetes)](./docs/providers/infiniband.md#infiniband-k8s-kubernetes)

Currently supported engines:

- [SLURM](./docs/engines/slurm.md)
- [Kubernetes](./docs/engines/k8s.md)
- [SLURM-on-Kubernetes (Slinky)](./docs/engines/slinky.md)

### Choosing a Provider

| Scenario | Recommended provider |
|---|---|
| Cloud cluster (AWS, GCP, OCI, Nebius) | Use the matching CSP provider |
| Spectrum-X fabric | [NetQ](./docs/providers/netq.md) |
| Multi-Node NVLink (MNNVL), infrastructure visibility | [NetQ](./docs/providers/netq.md) |
| MNNVL on Kubernetes (scheduling) | [DRA](./docs/providers/dra.md) |
| InfiniBand fabric, NetQ deployed | [NetQ](./docs/providers/netq.md) |
| InfiniBand fabric, no NetQ, bare-metal / Slurm | [InfiniBand (bare-metal)](./docs/providers/infiniband.md) |
| InfiniBand fabric, no NetQ, Kubernetes | [InfiniBand (Kubernetes)](./docs/providers/infiniband.md) |

For MNNVL environments, NetQ and DRA operate at different layers and can coexist: NetQ provides infrastructure-level visibility into the NVLink fabric while DRA feeds topology directly to Kubernetes schedulers via `nvidia.com/gpu.clique` node labels.

## Using Topograph

Topograph offers three endpoints for interacting with the service. Below are the details of each endpoint:

### 1. Health Endpoint

- **URL:** `http://<server>:<port>/healthz`
- **Description:** This endpoint verifies the service status. It returns a "200 OK" HTTP response if the service is operational.

### 2. Topology Request Endpoint

- **URL:** `http://<server>:<port>/v1/generate`
- **Description:** This endpoint is used to request a new cluster topology.
- **Payload:** The payload is a JSON object that includes the following fields:

  - **provider name**: (optional) A string specifying the Service Provider, such as `aws`, `oci`, `gcp`, `nebius`, `netq`, `dra`, `infiniband-k8s`, `infiniband-bm` or `test`. This parameter will be override the provider set in the topograph config.
  - **provider credentials**: (optional) A key-value map with provider-specific parameters for authentication.
  - **provider parameters**: (optional) A key-value map with parameters that are used for provider simulation with toposim.
    - **generateResponseCode**: (optional) An integer parameter that specifies the response code for the generate request. Supported by Providers = [test]. Valid values [202,4xx-6xx]. Default value = 202.
    - **topologyResponseCode**: (optional) An integer parameter that specifies the response code for the topology request. Supported by Providers = [test]. Valid values [200,202,4xx-6xx]. Default value = 200.  
    - **modelFileName**: (optional) A string parameter that specifies the name of the model file to use for simulating topology. Supported by Providers = [test].
    - **errorMessage**: (optional) A string parameter that specifies the message to be returned with error responses. Supported by Providers = [test].  
  - **engine name**: (optional) A string specifying the topology output, either `slurm`, `k8s`, or `slinky`. This parameter will override the engine set in the topograph config.
  - **engine parameters**: (optional) A key-value map with engine-specific parameters.
    - **slurm parameters**:
      - **topologyConfigPath**: (optional) A string specifying the file path for the topology configuration. If omitted, the topology config content is returned in the HTTP response.
      - **plugin**: (optional) A string specifying topology plugin: `topology/tree` (default) or `topology/block`.
      - **block_sizes**: (optional) A string specifying block size for `topology/block` plugin.
      - **reconfigure**: (optional) If `true`, invoke `scontrol reconfigure` after topology config is generated. Default `false`
    - **slinky parameters**:
      - **namespace**: A string specifying namespace where SLURM cluster is running.
      - **podSelector**: A standard Kubernetes label selector for pods running SLURM nodes.
      - **plugin**: (optional) A string specifying topology plugin: `topology/tree` (default) or `topology/block`.
      - **block_sizes**: (optional) A string specifying block size for `topology/block` plugin.
      - **topologyConfigPath**: A string specifying the key for the topology config in the ConfigMap.
      - **topologyConfigmapName**: A string specifying the name of the ConfigMap containing the topology config.
  - **nodes**: (optional) An array of regions mapping instance IDs to node names.

  Example:

```json
{
  "provider": {
    "name": "aws",
    "creds": {
      "accessKeyId": "id",
      "secretAccessKey": "secret"
    },
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
  - "200 OK" - The request has completed successfully.
  - "202 Accepted" - The request is still in progress and has not completed yet.
  - "404 Not Found" - The specified request ID does not exist.
  - Other error responses encountered by Topograph during request execution.

Example usage:

```bash
id=$(curl -s -X POST -H "Content-Type: application/json" -d @payload.json http://localhost:49021/v1/generate)

curl -s "http://localhost:49021/v1/topology?uid=$id"
```
