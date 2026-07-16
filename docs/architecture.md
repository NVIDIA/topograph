# Architecture

Topograph consists of five major components:

1. **API Server**
2. **Node Observer**
3. **Node Data Broker**
4. **Provider**
5. **Engine**

<p align="center"><img src="assets/design.png" width="600" alt="Design" /></p>

## Components

### 1. API Server

The API Server receives topology generation requests and returns results asynchronously. Requests are aggregated over a configurable delay window so that a burst of node changes (common during cluster scaling events) produces a single topology update rather than a storm.

### 2. Node Observer

The Node Observer is used in Kubernetes deployments. It monitors configured node and pod changes, watches the Topograph API pod, and can optionally schedule topology generation at a fixed interval. All of these events enter the same single-slot queue, so bursts are deduplicated and HTTP requests are executed serially. When the node-data-broker is enabled, every queued request waits for the broker DaemonSet's ready replica count to match its desired count, then uses the same HTTP retry path.

Periodic generation is disabled by default. When enabled, the first periodic event occurs after one complete interval; stopping the Controller or cancelling its context stops the ticker. The interval must be significantly greater than the API Server's request aggregation delay. Otherwise, repeated identical requests can continually reset the trailing aggregation timer and prevent generation from starting.

### 3. Node Data Broker

The Node Data Broker is also used when Topograph is deployed in a Kubernetes cluster. It collects relevant node attributes and stores them as node annotations.

### 4. Provider

The Provider interfaces with CSPs or on-premises tools to retrieve topology-related data from the cluster and converts it into an internal representation.

### 5. Engine

The Engine translates this internal representation into the format expected by the workload manager.

## Workflow

- The API Server listens on the port and notifies the Provider about incoming requests. In Kubernetes, the incoming requests are sent by the Node Observer in response to selected node/pod status, API-server readiness, or an optional periodic interval.
- The Provider receives notifications and invokes CSP API to retrieve topology-related information.
- The Engine converts the topology information into the format expected by the user cluster (e.g., SLURM or Kubernetes).
