# Topograph with SLURM

For the SLURM engine, topograph supports [tree](https://slurm.schedmd.com/topology.conf.html#SECTION_topology/tree) and [block](https://slurm.schedmd.com/topology.conf.html#SECTION_topology/block) topology configurations.

### Test Provider and Engine
There is a special *provider* and *engine* named `test`, which supports both SLURM and Kubernetes. This configuration returns static results and is primarily used for testing purposes.

## Installation and Configuration
Topograph can be installed using the `topograph` Debian or RPM package. This package sets up a service but does not start it automatically, allowing users to update the configuration before launch.

The configuration file and certificates created by the installer are located in the /etc/topograph directory.

#### Service Management
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

#### Verifying Health
To verify the service is healthy, you can use the following command:

```bash
curl http://localhost:49021/healthz
```

#### Automated Solution for SLURM

The Cluster Topology Generator enables a fully automated solution when combined with SLURM's `strigger` command. You can set up a trigger that runs whenever a node goes down or comes up:

```bash
strigger --set --node --down --up --flags=perm --program=<script>
```

In this setup, the `<script>` would contain the curl command to call the endpoint:

```bash
curl -s -X POST -H "Content-Type: application/json" -d @payload.json http://localhost:49021/v1/generate
```

We provide `scripts/create-topology-update-script.sh` in the repository, which performs the steps outlined above: it creates the topology update script and registers it with the strigger.

The script accepts the following parameters:
- **provider name** (aws, oci, gcp, nebius, netq, infiniband-bm)
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
