/*

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

package designate

import (
	corev1 "k8s.io/api/core/v1"
)

// APIDetails information
type APIDetails struct {
	ContainerImage       string
	DatabaseHost         string
	DatabaseName         string
	OSPSecret            string
	TransportURLSecret   string
	UserPasswordSelector string
	VolumeMounts         []corev1.VolumeMount
	Privileged           bool
}

// InitContainerDetails contains configuration for init containers
type InitContainerDetails struct {
	ContainerImage string
	VolumeMounts   []corev1.VolumeMount
	EnvVars        []corev1.EnvVar
}

const (
	// InitContainerCommand -
	InitContainerCommand = "/usr/local/bin/container-scripts/init.sh"
)

// SimpleInitContainer creates a simple init container with the provided details
func SimpleInitContainer(init InitContainerDetails) corev1.Container {
	runAsUser := int64(0)

	args := []string{
		"-c",
		InitContainerCommand,
	}

	return corev1.Container{
		Name:  "init",
		Image: init.ContainerImage,
		SecurityContext: &corev1.SecurityContext{
			RunAsUser: &runAsUser,
		},
		Command: []string{
			"/bin/bash",
		},
		Args:         args,
		Env:          init.EnvVars,
		VolumeMounts: init.VolumeMounts,
	}
}

// InitContainer - init container for designate api pods
func InitContainer(init APIDetails) []corev1.Container {
	runAsUser := int64(0)

	args := []string{
		"-c",
		InitContainerCommand,
	}

	return []corev1.Container{
		{
			Name:  "init",
			Image: init.ContainerImage,
			SecurityContext: &corev1.SecurityContext{
				RunAsUser: &runAsUser,
			},
			Command: []string{
				"/bin/bash",
			},
			Args:         args,
			VolumeMounts: init.VolumeMounts,
		},
	}
}
