// Based upon https://github.com/kubernetes/kubernetes/blob/58f2d96901b9bc0e90f5c6fb5bc808a7aa86a851/pkg/proxy/endpointslicecache.go

package main

import (
	"fmt"
	"iter"
	"reflect"
	"sync"

	discovery "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
)

type EndpointSliceCache struct {
	// lock protects trackerByServiceMap.
	lock sync.Mutex

	// trackerByServiceMap is the basis of this cache. It contains endpoint
	// slice trackers grouped by service name and endpoint slice name. The first
	// key represents a namespaced service name while the second key represents
	// an endpoint slice name. Since endpoints can move between slices, we
	// require slice specific caching to prevent endpoints being removed from
	// the cache when they may have just moved to a different slice.
	trackerByServiceMap map[types.NamespacedName]*endpointSliceTracker
}

// endpointSliceTracker keeps track of EndpointSlices as they are known for a
// specific service.
type endpointSliceTracker struct {
	slices endpointSliceDataByName // All known EndpointSlices for a service.
}

// endpointSliceDataByName groups endpointSliceData by the names of the
// corresponding EndpointSlices.
type endpointSliceDataByName map[string]*discovery.EndpointSlice

// NewEndpointSliceCache initializes an EndpointSliceCache.
func NewEndpointSliceCache() *EndpointSliceCache {
	return &EndpointSliceCache{
		trackerByServiceMap: map[types.NamespacedName]*endpointSliceTracker{},
	}
}

// newEndpointSliceTracker initializes an endpointSliceTracker.
func newEndpointSliceTracker() *endpointSliceTracker {
	return &endpointSliceTracker{
		slices: endpointSliceDataByName{},
	}
}

// Update updates a slice in the cache.
func (cache *EndpointSliceCache) Update(endpointSlice *discovery.EndpointSlice, remove bool) bool {
	serviceKey, sliceKey, err := endpointSliceCacheKeys(endpointSlice)
	if err != nil {
		klog.ErrorS(err, "Error getting endpoint slice cache keys")
		return false
	}

	cache.lock.Lock()
	defer cache.lock.Unlock()

	byServiceData, ok := cache.trackerByServiceMap[serviceKey]
	if !ok {
		byServiceData = newEndpointSliceTracker()
		cache.trackerByServiceMap[serviceKey] = byServiceData
	}

	if remove {
		if _, ok := byServiceData.slices[sliceKey]; ok {
			delete(byServiceData.slices, sliceKey)
			return true
		}
	} else {
		if cache.isEndpointSliceChanged(serviceKey, sliceKey, endpointSlice) {
			byServiceData.slices[sliceKey] = endpointSlice
			return true
		}
	}

	return false
}

func (cache *EndpointSliceCache) Services() iter.Seq2[types.NamespacedName, iter.Seq[*discovery.Endpoint]] {
	return func(yield func(types.NamespacedName, iter.Seq[*discovery.Endpoint]) bool) {
		cache.lock.Lock()
		defer cache.lock.Unlock()

		for serviceName, tracker := range cache.trackerByServiceMap {
			innerIterator := func(yield func(*discovery.Endpoint) bool) {
				for _, slice := range tracker.slices {
					for i := range slice.Endpoints {
						if !yield(&slice.Endpoints[i]) {
							return
						}
					}
				}
			}

			if !yield(serviceName, innerIterator) {
				return
			}
		}
	}
}

// isEndpointSliceChanged returns true if the endpointSlice parameter should be set as a new
// value in the cache.
func (cache *EndpointSliceCache) isEndpointSliceChanged(serviceKey types.NamespacedName, sliceKey string, endpointSlice *discovery.EndpointSlice) bool {
	if byServiceData, ok := cache.trackerByServiceMap[serviceKey]; ok {
		data, ok := byServiceData.slices[sliceKey]

		// If there's already a value, return whether or not this would
		// change that.
		if ok {
			return !reflect.DeepEqual(endpointSlice, data)
		}
	}

	// If not in the cache, it should be added.
	return true
}

// endpointSliceCacheKeys returns cache keys used for a given EndpointSlice.
func endpointSliceCacheKeys(endpointSlice *discovery.EndpointSlice) (types.NamespacedName, string, error) {
	var err error
	serviceName, ok := endpointSlice.Labels[discovery.LabelServiceName]
	if !ok || serviceName == "" {
		err = fmt.Errorf("no %s label set on endpoint slice: %s", discovery.LabelServiceName, endpointSlice.Name)
	} else if endpointSlice.Namespace == "" || endpointSlice.Name == "" {
		err = fmt.Errorf("expected EndpointSlice name and namespace to be set: %v", endpointSlice)
	}
	return types.NamespacedName{Namespace: endpointSlice.Namespace, Name: serviceName}, endpointSlice.Name, err
}
