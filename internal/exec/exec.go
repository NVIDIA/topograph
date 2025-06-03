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

package exec

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/NVIDIA/topograph/internal/cluset"
	"k8s.io/klog/v2"
)

func Exec(ctx context.Context, exe string, args []string, env map[string]string) (*bytes.Buffer, error) {
	klog.V(2).Infof("Execute command %s", strings.Join(append([]string{exe}, args...), " "))
	cmd := exec.CommandContext(ctx, exe, args...)

	cmd.Env = os.Environ()
	if len(env) != 0 {
		vars := make([]string, 0, len(env))
		for k, v := range env {
			vars = append(vars, fmt.Sprintf("%s=%s", k, v))
		}
		cmd.Env = append(cmd.Env, vars...)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		msg := stderr.String()
		klog.ErrorS(err, "failed to execute command", "stdout", stdout.String(), "stderr", msg)
		return nil, fmt.Errorf("%s failed: %s : %v", exe, msg, err)
	}
	return &stdout, nil
}

func Pdsh(ctx context.Context, cmd string, nodes []string) (*bytes.Buffer, error) {
	return Exec(ctx, "pdsh", []string{"-w", strings.Join(cluset.Compact(nodes), ","), cmd}, nil)
}
