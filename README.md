# topograph

Topograph is a component designed to expose the underlying physical network topology of a cluster to enable a workload manager make network-topology aware scheduling decisions. It consists of four major components:

1. **CSP Connector**
2. **API Server**
3. **Topology Generator**
4. **Node Observer**

## Components

### 1. CSP Connector

The CSP Connector is responsible for interfacing with various CSPs to retrieve cluster-related information. Currently, it supports AWS, with plans to add support for OCI, GCP, and Azure. The primary goal of the CSP Connector is to obtain the network topology configuration of a cluster, which may require several subsequent API calls. Once the information is obtained, the CSP Connector translates the network topology from CSP-specific formats to an internal format that can be utilized by the Topology Generator.

### 2. API Server

The API Server listens for network topology configuration requests on a specific port. When a request is received, the server triggers the Topology Generator to populate the configuration.

The API Server exposes two endpoints: one for synchronous requests and one for asynchronous requests.

- The synchronous endpoint responds to the HTTP request with the topology configuration, though this process may take some time.
- In the asynchronous mode, the API Server promptly returns a "202 Accepted" response to the HTTP request. It then begins generating and serializing the topology configuration.

### 3. Topology Generator

The Topology Generator is the central component that manages the overall network topology of the cluster. It performs the following functions:

- **Notification Handling:** Receives notifications from the API Server.
- **Topology Gathering:** Instructs the CSP Connector to fetch the current network topology from the CSP.
- **User Cluster Update:** Translates network topology from the internal format into a format expected by the user cluster, such as SLURM or Kubernetes.

### 4. Node Observer
The Node Observer is used when the Topology Generator is deployed in a Kubernetes cluster. It monitors changes in the cluster nodes.
If a node's status changes (e.g., a node goes down or comes up), the Node Observer sends a request to the API Server to generate a new topology configuration.

## Supported Environments

Topograph functions using the concepts of `provider` and `engine`. Here, a `provider` refers to a CSP, and an `engine` denotes a scheduling system such as SLURM or Kubernetes.

### SLURM Engine

For the SLURM engine, topograph supports the following CSPs:
- AWS
- OCI
- GCP

### Kubernetes Engine

Support for the Kubernetes engine is currently in the development stage.

### Test Provider and Engine

There is a special *provider* and *engine* named `test`, which supports both SLURM and Kubernetes. This configuration returns static results and is primarily used for testing purposes.

## Workflow

- The API Server listens on the port and notifies the Topology Generator about incoming requests.
- The Topology Generator receives the notification and attempts to gather the current network topology of the cluster.
- The Topology Generator instructs the CSP Connector to retrieve the network topology from the CSP.
- The CSP Connector fetches the topology and translates it from the CSP-specific format to an internal format.
- The Topology Generator converts the internal format into the format expected by the user cluster (e.g., SLURM or Kubernetes).
- The Topology Generator returns the network topology configuration to the API Server, which then relays it back to the requester.

## Topograph Installation and Configuration
Topograph can be installed using the `topograph` Debian or RPM package. This package sets up a service but does not start it automatically, allowing users to update the configuration before launch.

### Configuration
The default configuration file is located at [config/topograph-config.yaml](config/topograph-config.yaml). It includes settings for:
 - HTTP endpoint for the Topology Generator
 - SSL/TLS connection
 - environment variables

By default, SSL/TLS is enabled, and the server certificate and key are generated during package installation.

The configuration file also includes an optional section for environment variables. When specified, these variables are added to the shell environment. Note that the `PATH` variable, if provided, is appended to the existing `PATH`.

### Service Management
To enable and start the service, run the following commands:
```bash
systemctl enable topograph.service
systemctl start topograph.service
```

Upon starting, the service executes:
```bash
/usr/local/bin/topograph -c /etc/topograph/topograph-config.yaml
```

To disable and stop the service, run the following commands:
```bash
systemctl stop topograph.service
systemctl disable topograph.service
systemctl daemon-reload
```

### Testing the Service
To verify the service is running correctly, you can use the following commands:
```bash
curl http://localhost:49021/healthz

curl -X POST "http://localhost:49021/v1/generate?provider=test&engine=test"
```

### Using the Cluster Topology Generator

The Cluster Topology Generator offers three endpoints for interacting with the service. Below are the details of each endpoint:

#### 1. Health Endpoint

- **URL:** `http://<server>:<port>/healthz`
- **Description:** This endpoint verifies the service status. It returns a "200 OK" HTTP response if the service is operational.

#### 2. Topology Request Endpoint

- **URL:** `http://<server>:<port>/v1/generate`
- **Description:** This endpoint is used to request a new cluster topology.
- **URL Query Parameters:**
  - **provider**: (mandatory) Specifies the Cloud Service Provider (CSP) such as `aws`, `oci`, `gcp`, `azure`, or `test`.
  - **engine**: (mandatory) Specifies the configuration format, either `slurm` or `k8s`.
  - **topology_config_path**: (optional for `engine=slurm`, mandatory for `engine=k8s`) Specifies the file path for the topology config when `engine=slurm`, or the key for the topology config in the configmap when `engine=k8s`. If omitted for `slurm`, the content of the topology config is returned with the HTTP response.
  - **topology_configmap_name**: (mandatory for `engine=k8s`) Specifies the name of the configmap containing the topology config.
  - **topology_configmap_namespace**: (mandatory for `engine=k8s`) Specifies the namespace of the configmap containing the topology config.
  - **skip_reload**: (optional for `engine=slurm`) Omit cluster reconfiguration if present.
- **Response:** This endpoint immediately returns a "202 Accepted" status with a unique request ID if the request is valid. If not, it returns an appropriate error code.

#### 3. Topology Result Endpoint

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
id=$(curl -s -X POST "http://localhost:49021/v1/generate?provider=aws&engine=slurm&topology_config_path=/path/to/topology.conf")

curl -s http://localhost:49021/v1/topology?uid=$id
```

You can optionally skip the SLURM reconfiguration:

```bash
id=$(curl -X POST "http://localhost:49021/v1/generate?provider=oci&engine=slurm&topology_config_path=/path/to/topology.conf&skip_reload)

curl -s http://localhost:49021/v1/topology?uid=$id
```

### Automated Solution for SLURM

The Cluster Topology Generator enables a fully automated solution when combined with SLURM's `strigger` command. You can set up a trigger that runs whenever a node goes down or comes up:

```bash
strigger --set --node --down --up --flags=perm --program=<script>
```

In this setup, the `<script>` would contain the curl command to call the asynchronous endpoint:

```bash
curl -X POST "http://localhost:49021/v1/generate?provider=aws&engine=slurm&topology_config_path=/path/to/topology.conf"
```

We provide the [create-topology-update-script.sh](scripts/create-topology-update-script.sh) script, which performs the steps outlined above: it creates the topology update script and registers it with the strigger.

The script accepts the following parameters:
- **provider name** (aws, oci, gcp)
- **path to the generated topology update script**
- **path to the topology.conf file**

Usage:
```bash
create-topology-update-script.sh -p <provider name> -s <topology update script> -c <path to topology.conf>
```

Example:
```bash
create-topology-update-script.sh -p aws -s /etc/slurm/update-topology-config.sh -c /etc/slurm/topology.conf
```

This automation ensures that your cluster topology is updated and SLURM configuration is reloaded whenever there are changes in node status, maintaining an up-to-date cluster configuration.


### Optional: Instance ID to Node Name Mapping
You can provide a mapping between CSP VM instance IDs and SLURM cluster node names as part of the request payload:
```bash
cat payload.json
{
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

curl -X POST -H "Content-Type: application/json" -d @payload.json "http://localhost:49021/v1/generate?provider=aws&engine=slurm"
```
