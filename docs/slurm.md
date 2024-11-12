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

#### Using Toposim
To test the service on a simulated cluster, first add the following lines to `/etc/topograph/topograph-config.yaml` so that topograph knows to run topology in simulation and to forward any topology requests to toposim.
```bash
provider: "test"
engine: "test"
forward_service_url: dns:localhost:49025
```
Then run the topograph service as normal.

You must then start the toposim service as such, setting the path to the test model that you want to use in simulation:
```bash
/usr/local/bin/toposim -m /usr/local/bin/tests/models/<cluster-model>.yaml
```

You can then verify the topology results via simulation by querying topograph, and specifying the test model path as a parameter to the provider.
If you want to view the tree topology, then use the command:
```bash
id=$(curl -s -X POST -H "Content-Type: application/json" -d '{"provider":{"params":{"model_path":"/usr/local/bin/tests/models/<cluster-model>.yaml"}}}' http://localhost:49021/v1/generate)
```

And if you want to view the block topology (with specified block sizes), use the command:
```bash
id=$(curl -s -X POST -H "Content-Type: application/json" -d '{"provider":{"params":{"model_path":"/usr/local/bin/tests/models/<cluster-model>.yaml"}},"engine":{"params":{"plugin":"topology/block", "block_sizes": "4,8"}}}' http://localhost:49021/v1/generate)
```

You can query the results of either topology request with:
```bash
curl -s "http://localhost:49021/v1/topology?uid=$id"
```
Note the path specified in the topograph query should point to the same model as provided to toposim. 

#### Automated Solution for SLURM

The Cluster Topology Generator enables a fully automated solution when combined with SLURM's `strigger` command. You can set up a trigger that runs whenever a node goes down or comes up:

```bash
strigger --set --node --down --up --flags=perm --program=<script>
```

In this setup, the `<script>` would contain the curl command to call the endpoint:

```bash
curl -s -X POST -H "Content-Type: application/json" -d @payload.json http://localhost:49021/v1/generate
```

We provide the [create-topology-update-script.sh](../scripts/create-topology-update-script.sh) script, which performs the steps outlined above: it creates the topology update script and registers it with the strigger.

The script accepts the following parameters:
- **provider name** (aws, oci, gcp, cw, baremetal)
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
