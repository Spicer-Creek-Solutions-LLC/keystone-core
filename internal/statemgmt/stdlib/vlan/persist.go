// SPDX-License-Identifier: Apache-2.0

package vlan

import "go.keystone-core.io/keystone-core/internal/statemgmt/stdlib/netpersist"

// resolveBackend turns `persist: auto` into a concrete backend.
func resolveBackend(p *params) (string, error) {
	if p.Persist == netpersist.Auto {
		return netpersist.DetectBackend()
	}
	return p.Persist, nil
}

// devicePersist builds the persistent footprint of this VLAN for the
// resolved backend: a `<vlan>.netdev` (`Kind=vlan` + `[VLAN] Id=`) plus a
// `[Network] VLAN=<vlan>` enslave drop-in under the parent's `.network.d/`
// (networkd), or a single `vlans:` document (netplan). The VLAN attaches
// to exactly one parent, so there is a single enslave entry.
func devicePersist(p *params) (netpersist.NetdevPersist, error) {
	backend, err := resolveBackend(p)
	if err != nil {
		return netpersist.NetdevPersist{}, err
	}
	d := netpersist.NetdevPersist{Backend: backend, Kind: netdevKind, Name: p.Name}
	switch backend {
	case netpersist.Networkd:
		d.NetdevBody = netpersist.RenderNetdev(p.Name, netdevKind, renderNetdevSection(p))
		if p.Parent != "" {
			d.Enslave = []netpersist.Enslave{{
				Iface: p.Parent,
				Body:  netpersist.RenderEnslave("VLAN", p.Name),
			}}
		}
	case netpersist.Netplan:
		d.NetplanBody = renderNetplan(p)
	}
	return d, nil
}
