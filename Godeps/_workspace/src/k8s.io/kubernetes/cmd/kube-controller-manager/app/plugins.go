/*
Copyright 2014 The Kubernetes Authors All rights reserved.

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

package app

import (
	// This file exists to force the desired plugin implementations to be linked.
	// This should probably be part of some configuration fed into the build for a
	// given binary target.

	//Cloud providers
	_ "k8s.io/kubernetes/pkg/cloudprovider/aws"
	_ "k8s.io/kubernetes/pkg/cloudprovider/gce"
	_ "k8s.io/kubernetes/pkg/cloudprovider/mesos"
	_ "k8s.io/kubernetes/pkg/cloudprovider/openstack"
	_ "k8s.io/kubernetes/pkg/cloudprovider/ovirt"
	_ "k8s.io/kubernetes/pkg/cloudprovider/rackspace"
	_ "k8s.io/kubernetes/pkg/cloudprovider/vagrant"

	// Volume plugins
	"k8s.io/kubernetes/pkg/volume"
	"k8s.io/kubernetes/pkg/volume/host_path"
	"k8s.io/kubernetes/pkg/volume/nfs"
)

// ProbeRecyclableVolumePlugins collects all persistent volume plugins into an easy to use list.
func ProbeRecyclableVolumePlugins(flags VolumeConfigFlags) []volume.VolumePlugin {
	allPlugins := []volume.VolumePlugin{}

	// The list of plugins to probe is decided by this binary, not
	// by dynamic linking or other "magic".  Plugins will be analyzed and
	// initialized later.

	// Each plugin can make use of VolumeConfig.  The single arg to this func contains *all* enumerated
	// CLI flags meant to configure volume plugins.  From that single config, create an instance of volume.VolumeConfig
	// for a specific plugin and pass that instance to the plugin's ProbeVolumePlugins(config) func.
	hostPathConfig := volume.VolumeConfig{
		RecyclerMinimumTimeout:   flags.PersistentVolumeRecyclerMinimumTimeoutHostPath,
		RecyclerTimeoutIncrement: flags.PersistentVolumeRecyclerIncrementTimeoutHostPath,
		RecyclerDefaultPod:       flags.PersistentVolumeRecyclerDefaultPod,
	}
	nfsConfig := volume.VolumeConfig{
		RecyclerMinimumTimeout:   flags.PersistentVolumeRecyclerMinimumTimeoutNFS,
		RecyclerTimeoutIncrement: flags.PersistentVolumeRecyclerIncrementTimeoutNFS,
		RecyclerDefaultPod:       flags.PersistentVolumeRecyclerDefaultPod,
	}

	allPlugins = append(allPlugins, host_path.ProbeVolumePlugins(hostPathConfig)...)
	allPlugins = append(allPlugins, nfs.ProbeVolumePlugins(nfsConfig)...)
	return allPlugins
}
