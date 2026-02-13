package k8s

import (
	"context"
	"fmt"
	"time"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
)

// InformerManager sets up and manages dynamic shared informers for CRD resources.
type InformerManager struct {
	factory dynamicinformer.DynamicSharedInformerFactory
	stopCh  chan struct{}
	synced  bool
}

// NewInformerManager creates an InformerManager that watches CRDs in the given namespace.
// Pass empty string for namespace to watch all namespaces.
func NewInformerManager(client dynamic.Interface, namespace string, resyncPeriod time.Duration) *InformerManager {
	var factory dynamicinformer.DynamicSharedInformerFactory
	if namespace != "" {
		factory = dynamicinformer.NewFilteredDynamicSharedInformerFactory(
			client, resyncPeriod, namespace, nil,
		)
	} else {
		factory = dynamicinformer.NewDynamicSharedInformerFactory(client, resyncPeriod)
	}

	return &InformerManager{
		factory: factory,
		stopCh:  make(chan struct{}),
	}
}

// SetRemoteExecutionHandler registers event handlers for RemoteExecution resources.
func (im *InformerManager) SetRemoteExecutionHandler(handler cache.ResourceEventHandler) {
	informer := im.factory.ForResource(RemoteExecutionGVR).Informer()
	informer.AddEventHandler(handler) //nolint:errcheck // AddEventHandler returns a registration handle we don't need
}

// SetStateConfigHandler registers event handlers for StateConfig resources.
func (im *InformerManager) SetStateConfigHandler(handler cache.ResourceEventHandler) {
	informer := im.factory.ForResource(StateConfigGVR).Informer()
	informer.AddEventHandler(handler) //nolint:errcheck // AddEventHandler returns a registration handle we don't need
}

// Start starts all registered informers and waits for initial cache sync.
func (im *InformerManager) Start(ctx context.Context) error {
	im.factory.Start(im.stopCh)

	syncCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Wait for caches to sync
	syncs := im.factory.WaitForCacheSync(syncCtx.Done())
	for gvr, synced := range syncs {
		if !synced {
			return fmt.Errorf("informer cache sync failed for %s", gvr.String())
		}
	}

	im.synced = true
	return nil
}

// Stop shuts down all informers.
func (im *InformerManager) Stop() {
	close(im.stopCh)
}

// HasSynced returns true if all informer caches have synced.
func (im *InformerManager) HasSynced() bool {
	return im.synced
}

// KeyFromObject extracts a namespace/name key from a runtime object.
func KeyFromObject(obj interface{}) (string, error) {
	return cache.MetaNamespaceKeyFunc(obj)
}
