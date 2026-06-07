// SPDX-License-Identifier: Apache-2.0

package route

import "go.keystone-core.io/keystone-core/internal/statemgmt/stdlib/netpersist"

// persistPresentDrift reports whether the persistent config for a
// `present` route is missing or stale. For networkd it also treats an
// absent interface base unit as drift, since the route's drop-in needs
// a `.network` to merge into.
func persistPresentDrift(p *params) (bool, error) {
	backend, err := resolveBackend(p)
	if err != nil {
		return false, err
	}
	desired, err := renderRoute(backend, p)
	if err != nil {
		return false, err
	}
	path, err := routePersistPath(backend, p)
	if err != nil {
		return false, err
	}
	current, exists, err := netpersist.Read(path)
	if err != nil {
		return false, err
	}
	if !exists || current != desired {
		return true, nil
	}
	if backend == netpersist.Networkd {
		_, baseExists, err := netpersist.Read(netpersist.NetworkPath(p.Interface))
		if err != nil {
			return false, err
		}
		if !baseExists {
			return true, nil
		}
	}
	return false, nil
}

// persistAbsentDrift reports whether a leftover persistent file exists
// for an `absent` route (it should be removed).
func persistAbsentDrift(p *params) (bool, error) {
	backend, err := resolveBackend(p)
	if err != nil {
		return false, err
	}
	path, err := routePersistPath(backend, p)
	if err != nil {
		return false, err
	}
	_, exists, err := netpersist.Read(path)
	return exists, err
}

// writePersist renders and writes the persistent config for a present
// route. For networkd it first creates the interface base unit if none
// exists (create-if-absent: a minimal `[Match]`-only unit that the
// `network` module's fuller base may later supersede), so a route-only
// interface has something for the drop-in to merge into.
func writePersist(p *params) error {
	backend, err := resolveBackend(p)
	if err != nil {
		return err
	}
	if backend == netpersist.Networkd {
		basePath := netpersist.NetworkPath(p.Interface)
		_, exists, err := netpersist.Read(basePath)
		if err != nil {
			return err
		}
		if !exists {
			if err := netpersist.Write(basePath, minimalBase(p.Interface)); err != nil {
				return err
			}
		}
	}
	desired, err := renderRoute(backend, p)
	if err != nil {
		return err
	}
	path, err := routePersistPath(backend, p)
	if err != nil {
		return err
	}
	return netpersist.Write(path, desired)
}

// removePersist deletes the persistent config for an absent route. It
// leaves any interface base unit alone — other routes or the `network`
// module's address config may still depend on it.
func removePersist(p *params) error {
	backend, err := resolveBackend(p)
	if err != nil {
		return err
	}
	path, err := routePersistPath(backend, p)
	if err != nil {
		return err
	}
	return netpersist.Remove(path)
}
