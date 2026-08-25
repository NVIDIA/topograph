/*
 * Copyright 2026 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package crusoe

import (
	"strings"

	"k8s.io/klog/v2"
)

const (
	// Crusoe Cloud publishes the InfiniBand topology of every VM as Kubernetes
	// Node labels. The control plane owns them, so a guest VM does not need
	// fabric access to learn where it sits.
	labelPartitionID = "crusoe.ai/ib.partition.id" // InfiniBand partition, the outer fabric tier
	labelPodID       = "crusoe.ai/pod.id"          // rail-optimized pod, the inner fabric tier

	// labelGPUClique is published by NVIDIA GPU Feature Discovery on MNNVL
	// systems such as GB200 and GB300 NVL72. Its value is
	// "<clusterUUID>.<cliqueID>" and names the NVLink domain: the set of nodes
	// that share an NVLink fabric and can form one IMEX domain. Every node in a
	// rack carries the same value, and a clique never spans racks.
	labelGPUClique = "nvidia.com/gpu.clique"

	// nvlDomainPrefix marks a domain as NVLink-derived, so the domain a block
	// came from is readable in the accelerator topology.
	nvlDomainPrefix = "nvl-"

	// rootTier names the synthetic tier above every partition. Slurm needs one
	// common root so a job can span partitions and reach nodes that have no
	// InfiniBand at all.
	rootTier = "crusoe"

	// Fabric tier names used for nodes that carry no InfiniBand labels.
	cpuPartition = "cpu-partition"
	cpuPod       = "cpu-pod"
)

// fabricLabels holds the InfiniBand locality read from one Node.
type fabricLabels struct {
	PartitionID string
	PodID       string
}

// readFabricLabels extracts the InfiniBand locality of a Node. It reports false
// when either label is absent, which is the normal state of a CPU-only node and
// of a GPU SKU that ships without InfiniBand.
func readFabricLabels(labels map[string]string) (fabricLabels, bool) {
	partitionID := strings.TrimSpace(labels[labelPartitionID])
	podID := strings.TrimSpace(labels[labelPodID])

	if partitionID == "" || podID == "" {
		klog.V(4).Infof("No InfiniBand labels (%s=%q %s=%q)",
			labelPartitionID, partitionID, labelPodID, podID)
		return fabricLabels{}, false
	}

	return fabricLabels{PartitionID: partitionID, PodID: podID}, true
}

// readNVLinkDomain returns the NVLink domain of a Node, or "" when the node is
// not on an NVLink fabric.
func readNVLinkDomain(labels map[string]string) string {
	clique := strings.TrimSpace(labels[labelGPUClique])
	if clique == "" {
		return ""
	}

	klog.V(4).Infof("NVLink domain from %s=%q", labelGPUClique, clique)
	return nvlDomainPrefix + clique
}
