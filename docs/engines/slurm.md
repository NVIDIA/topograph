# Topograph with SLURM

For the SLURM engine, topograph supports [tree](https://slurm.schedmd.com/topology.conf.html#SECTION_topology/tree) and [block](https://slurm.schedmd.com/topology.conf.html#SECTION_topology/block) topology configurations.

## Deriving block names from node names

For `topology/block`, the optional `blockName` engine parameter derives each block name from the names of its nodes. Both `nodeNameRegexp` and `format` are required when `blockName` is set.

```yaml
engine:
  name: slurm
  params:
    plugin: topology/block
    blockSizes: [8, 16]
    blockName:
      nodeNameRegexp: 'd([0-9]{2})-r([0-9]{2})'
      format: 'domain${1}_rack${2}'
```

For a block containing nodes such as `gpu-d05-r04-srv4`, this produces the name `domain05_rack04`. The expression uses Go regular-expression syntax and may match anywhere in the node name; use `^` or `$` when the site naming convention requires anchoring. The format uses Go regexp expansion syntax, including numeric captures such as `${1}` and named captures such as `${domain}`.

Every node in a non-empty block must match the expression and produce the same non-empty block name. Different blocks must produce unique names. Topograph rejects topology generation when any of these conditions is not met. Empty complemented blocks have no node name to evaluate and retain their generated default name.

The option can also be set on each `topologies` entry for per-partition output.

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
- **provider name** (`aws`, `gcp`, `oci`, `nebius`, `netq`, `nscale`, `lambdai`, or `infiniband-bm`)
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
