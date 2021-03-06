/*
Copyright IBM Corporation 2020

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cnb

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"

	log "github.com/sirupsen/logrus"

	"github.com/konveyor/move2kube/internal/common"
)

type containerRuntimeProvider struct {
}

var (
	containerRuntime  = ""
	availableBuilders = []string{}
)

func (r *containerRuntimeProvider) getAllBuildpacks(builders []string) (map[string][]string, error) { //[Containerization target option value] buildpacks
	buildpacks := map[string][]string{}
	containerRuntime, available := r.getContainerRuntime()
	if !available {
		return buildpacks, errors.New("Container runtime not supported in this instance")
	}
	log.Debugf("Getting data of all builders %s", builders)
	for _, builder := range builders {
		inspectcmd := exec.Command(containerRuntime, "inspect", "--storage-driver=vfs", "--format", `{{ index .Config.Labels "`+orderLabel+`"}}`, builder)
		log.Debugf("Inspecting image %s", builder)
		output, err := inspectcmd.CombinedOutput()
		if err != nil {
			log.Debugf("Unable to inspect image %s : %s, %s", builder, err, output)
			continue
		}
		buildpacks[builder] = getBuildersFromLabel(string(output))
	}

	return buildpacks, nil
}

func (r *containerRuntimeProvider) getContainerRuntime() (runtime string, available bool) {
	if containerRuntime == "" {
		detectcmd := exec.Command("podman", "run", "--storage-driver=vfs", "--rm", "hello-world")
		output, err := detectcmd.CombinedOutput()
		if err != nil {
			log.Debugf("Podman not supported : %s : %s", err, output)
			containerRuntime = "none"
			return containerRuntime, false
		}
		containerRuntime = "podman"
		return containerRuntime, true
	} else if containerRuntime == "none" {
		return containerRuntime, false
	}
	return containerRuntime, true
}

func (r *containerRuntimeProvider) isBuilderAvailable(builder string) bool {
	containerRuntime, available := r.getContainerRuntime()
	if !available {
		return false
	}
	if common.IsStringPresent(availableBuilders, builder) {
		return true
	}
	// Check if the image exists locally
	existcmd := exec.Command(containerRuntime, "--storage-driver=vfs", "images", "-q", builder)
	log.Debugf("Checking if the image %s exists locally", builder)
	output, err := existcmd.Output()
	if err != nil {
		log.Warnf("Error while checking if the builder %s exists locally. Error: %q Output: %q", builder, err, output)
		return false
	}
	if len(output) > 0 {
		// Found the image in the local machine, no need to pull.
		availableBuilders = append(availableBuilders, builder)
		return true
	}

	pullcmd := exec.Command(containerRuntime, "pull", "--storage-driver=vfs", builder)
	log.Debugf("Pulling image %s", builder)
	output, err = pullcmd.CombinedOutput()
	if err != nil {
		log.Warnf("Error while pulling builder %s : %s : %s", builder, err, output)
		return false
	}
	availableBuilders = append(availableBuilders, builder)
	return true
}

func (r *containerRuntimeProvider) isBuilderSupported(path string, builder string) (bool, error) {
	if !r.isBuilderAvailable(builder) {
		return false, fmt.Errorf("Builder image not available : %s", builder)
	}
	containerRuntime, _ := r.getContainerRuntime()
	p, err := filepath.Abs(path)
	if err != nil {
		log.Warnf("Unable to resolve to absolute path : %s", err)
	}
	detectcmd := exec.Command(containerRuntime, "run", "--rm", "--storage-driver=vfs", "-v", p+":/workspace", builder, "/cnb/lifecycle/detector")
	log.Debugf("Running detect on image %s", builder)
	output, err := detectcmd.CombinedOutput()
	if err != nil {
		log.Debugf("Detect failed %s : %s : %s", builder, err, output)
		return false, nil
	}
	return true, nil
}
