// SPDX-License-Identifier: Apache-2.0

package bond

import "go.keystone-core.io/keystone-core/internal/statemgmt/stdlib/netpersist"

// resolveBackend turns `persist: auto` into a concrete backend.
func resolveBackend(p *params) (string, error) {
	if p.Persist == netpersist.Auto {
		return netpersist.DetectBackend()
	}
	return p.Persist, nil
}

// devicePersist builds the persistent footprint of this bond for the
// resolved backend. For networkd it renders the `.netdev` and one
// `Bond=` enslave drop-in per member; for netplan a single `bonds:`
// document. The absent path only needs Kind+Name (it globs the
// drop-ins), so member-less builds are fine there.
func devicePersist(p *params) (netpersist.NetdevPersist, error) {
	backend, err := resolveBackend(p)
	if err != nil {
		return netpersist.NetdevPersist{}, err
	}
	d := netpersist.NetdevPersist{Backend: backend, Kind: netdevKind, Name: p.Name}
	switch backend {
	case netpersist.Networkd:
		d.NetdevBody = netpersist.RenderNetdev(p.Name, netdevKind, renderNetdevSection(p))
		for _, m := range p.Members {
			d.Enslave = append(d.Enslave, netpersist.Enslave{
				Iface: m,
				Body:  netpersist.RenderEnslave("Bond", p.Name),
			})
		}
	case netpersist.Netplan:
		d.NetplanBody = renderNetplan(p)
	}
	return d, nil
}
