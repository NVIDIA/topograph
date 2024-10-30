/*
 * Copyright (c) 2024, NVIDIA CORPORATION.  All rights reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package common

const (
	ProviderAWS  = "aws"
	ProviderOCI  = "oci"
	ProviderGCP  = "gcp"
	ProviderCW   = "cw"
	ProviderBM   = "baremetal"
	ProviderTest = "test"

	KeyEngine   = "engine"
	EngineSLURM = "slurm"
	EngineK8S   = "k8s"
	EngineTest  = "test"

	KeyUID                    = "uid"
	KeyTopoConfigPath         = "topology_config_path"
	KeyTopoConfigmapName      = "topology_configmap_name"
	KeyTopoConfigmapNamespace = "topology_configmap_namespace"
	KeyBlockSizes             = "block_sizes"
	KeySkipReload             = "skip_reload"
	KeyUseSimulation          = "use_simulation"
	KeyModelPath              = "model_path"

	KeyPlugin        = "plugin"
	ValTopologyTree  = "topology/tree"
	ValTopologyBlock = "topology/block"
)
