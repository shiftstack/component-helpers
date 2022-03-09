/*
Copyright 2022 The Kubernetes Authors.

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

package util

import (
	"fmt"
	"net"

	v1 "k8s.io/api/core/v1"
	netutils "k8s.io/utils/net"
)

// PreferNodeIP filters node addresses to prefer a specific node IP or address
// family.
//
// If nodeIP is either '0.0.0.0' or '::' it is taken to represent any address of
// that address family: IPv4 or IPv6. i.e. if nodeIP is '0.0.0.0' we will return
// node addresses sorted such that all IPv4 addresses are listed before IPv6
// addresses.
//
// If nodeIP is a specific IP, either IPv4 or IPv6, we will return node
// addresses filtered such that:
// * Any address matching nodeIP will be listed first.
// * If nodeIP matches an address of a particular type (internal or external),
//   that will be the *only* address of that type returned.
// * All remaining addresses are listed after.
func PreferNodeIP(nodeIP net.IP, cloudNodeAddresses []v1.NodeAddress) ([]v1.NodeAddress, error) {
	// If you are spelunking this function, it was originally copied with
	// minimal adaptations from the legacy cloud provider node address
	// re-ordering code in NodeAddress() in
	// kubernetes/kubernetes:pkg/kubelet/nodestatus/setters.go@a41f9e97

	// If nodeIP is unset, just use the addresses provided by the cloud provider as-is
	if nodeIP == nil {
		return cloudNodeAddresses, nil
	}

	// nodeIP is "0.0.0.0" or "::"; sort cloudNodeAddresses to
	// prefer addresses of the matching family
	if nodeIP.IsUnspecified() {
		preferIPv4 := nodeIP.To4() != nil
		isPreferredIPFamily := func(ip net.IP) bool { return (ip.To4() != nil) == preferIPv4 }

		sortedAddresses := make([]v1.NodeAddress, 0, len(cloudNodeAddresses))
		for _, nodeAddress := range cloudNodeAddresses {
			ip := netutils.ParseIPSloppy(nodeAddress.Address)
			if ip == nil || isPreferredIPFamily(ip) {
				sortedAddresses = append(sortedAddresses, nodeAddress)
			}
		}
		for _, nodeAddress := range cloudNodeAddresses {
			ip := netutils.ParseIPSloppy(nodeAddress.Address)
			if ip != nil && !isPreferredIPFamily(ip) {
				sortedAddresses = append(sortedAddresses, nodeAddress)
			}
		}
		return sortedAddresses, nil
	}

	// For every address supplied by the cloud provider that matches nodeIP, nodeIP is the enforced node address for
	// that address Type (like InternalIP and ExternalIP), meaning other addresses of the same Type are discarded.
	// See #61921 for more information: some cloud providers may supply secondary IPs, so nodeIP serves as a way to
	// ensure that the correct IPs show up on a Node object.
	enforcedNodeAddresses := []v1.NodeAddress{}

	nodeIPTypes := make(map[v1.NodeAddressType]bool)
	for _, nodeAddress := range cloudNodeAddresses {
		if netutils.ParseIPSloppy(nodeAddress.Address).Equal(nodeIP) {
			enforcedNodeAddresses = append(enforcedNodeAddresses, v1.NodeAddress{Type: nodeAddress.Type, Address: nodeAddress.Address})
			nodeIPTypes[nodeAddress.Type] = true
		}
	}

	// nodeIP must be among the addresses supplied by the cloud provider
	if len(enforcedNodeAddresses) == 0 {
		return nil, fmt.Errorf("failed to get node address from cloud provider that matches ip: %v", nodeIP)
	}

	// nodeIP was found, now use all other addresses supplied by the cloud provider NOT of the same Type as nodeIP.
	for _, nodeAddress := range cloudNodeAddresses {
		if !nodeIPTypes[nodeAddress.Type] {
			enforcedNodeAddresses = append(enforcedNodeAddresses, v1.NodeAddress{Type: nodeAddress.Type, Address: nodeAddress.Address})
		}
	}

	return enforcedNodeAddresses, nil
}
