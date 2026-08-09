package ecore

import (
	"sync"
)

type resourcesList struct {
	BasicENotifyingList
	resourceSet *EResourceSetImpl
}

func newResourcesList(resourceSet *EResourceSetImpl) *resourcesList {
	l := new(resourcesList)
	l.interfaces = l
	l.data = []any{}
	l.isUnique = true
	l.resourceSet = resourceSet
	return l
}

func (l *resourcesList) GetNotifier() ENotifier {
	return l.resourceSet.AsENotifier()
}

func (l *resourcesList) GetFeatureID() int {
	return RESOURCE_SET__RESOURCES
}

func (l *resourcesList) inverseAdd(object any, notifications ENotificationChain) ENotificationChain {
	if eResource, _ := object.(EResourceInternal); eResource != nil {
		return eResource.BasicSetResourceSet(l.resourceSet.AsEResourceSet(), notifications)
	}
	return notifications
}

func (l *resourcesList) inverseRemove(object any, notifications ENotificationChain) ENotificationChain {
	if eResource, _ := object.(EResourceInternal); eResource != nil {
		return eResource.BasicSetResourceSet(nil, notifications)
	}
	return notifications
}

// EResourceSetImpl ...
//
// The three registries below are built on first use. That first use is reached from
// proxy resolution -- GetEObject -> GetResource -> GetPackageRegistry -- and
// Host.ReadStable resolves proxies while holding only a SHARED lock, so "first use"
// can be several readers at once. Unguarded, each of them built its own converter or
// registry and published it over the others', which is both a lost write and, for the
// loser, a registry it goes on reading while nothing else in the process shares it.
// lazyMutex makes each of them built once and published once. It guards only these
// fields: it is deliberately NOT held across resource.Load(), because parallel
// section loading depends on those running concurrently.
type EResourceSetImpl struct {
	ENotifierImpl
	lazyMutex             sync.Mutex
	resources             EList
	uriConverter          EURIConverter
	uriResourceMap        map[*URI]EResource
	resourceCodecRegistry ECodecRegistry
	packageRegistry       EPackageRegistry
	defaultLoadOptions    map[string]any
}

func NewEResourceSetImpl() *EResourceSetImpl {
	rs := new(EResourceSetImpl)
	rs.SetInterfaces(rs)
	rs.Initialize()
	return rs
}

func (r *EResourceSetImpl) Initialize() {
	r.ENotifierImpl.Initialize()
	r.resources = newResourcesList(r)
}

func (r *EResourceSetImpl) AsEResourceSet() EResourceSet {
	return r.interfaces.(EResourceSet)
}

func (r *EResourceSetImpl) GetResources() EList {
	return r.resources
}

func (r *EResourceSetImpl) GetResource(uri *URI, loadOnDemand bool) EResource {
	if r.uriResourceMap != nil {
		resource := r.uriResourceMap[uri]
		if resource != nil {
			if loadOnDemand && !resource.IsLoaded() {
				resource.Load()
			}
			return resource
		}
	}

	ePackage := r.GetPackageRegistry().GetPackage(uri.String())
	if ePackage != nil {
		resource := ePackage.EResource()
		if resource != nil {
			if r.uriResourceMap != nil {
				r.uriResourceMap[uri] = resource
			}
			return resource
		}
	}

	normalizedURI := r.GetURIConverter().Normalize(uri)
	for it := r.resources.Iterator(); it.HasNext(); {
		resource := it.Next().(EResource)
		resourceURI := r.GetURIConverter().Normalize(resource.GetURI())
		if resourceURI.Equals(normalizedURI) {
			if loadOnDemand && !resource.IsLoaded() {
				resource.Load()
			}
			if r.uriResourceMap != nil {
				r.uriResourceMap[uri] = resource
			}
			return resource
		}
	}

	if loadOnDemand {
		resource := r.AsEResourceSet().CreateResource(uri)
		if resource != nil {
			resource.Load()
			if r.uriResourceMap != nil {
				r.uriResourceMap[uri] = resource
			}
		}
		return resource
	}

	return nil
}

func (r *EResourceSetImpl) CreateResource(uri *URI) EResource {
	resource := NewEResourceImpl()
	resource.SetURI(uri)
	r.resources.Add(resource)
	return resource
}

func (r *EResourceSetImpl) GetEObject(uri *URI, loadOnDemand bool) EObject {
	trim := uri.TrimFragment()
	resource := r.GetResource(trim, loadOnDemand)
	if resource != nil {
		return resource.GetEObject(uri.Fragment())
	}
	return nil
}

func (r *EResourceSetImpl) GetURIConverter() EURIConverter {
	r.lazyMutex.Lock()
	defer r.lazyMutex.Unlock()
	if r.uriConverter == nil {
		r.uriConverter = NewEURIConverterImpl()
	}
	return r.uriConverter
}

func (r *EResourceSetImpl) SetURIConverter(uriConverter EURIConverter) {
	r.lazyMutex.Lock()
	defer r.lazyMutex.Unlock()
	r.uriConverter = uriConverter
}

func (r *EResourceSetImpl) GetDefaultLoadOptions() map[string]any {
	return r.defaultLoadOptions
}

func (r *EResourceSetImpl) SetDefaultLoadOptions(options map[string]any) {
	r.defaultLoadOptions = options
}

func (r *EResourceSetImpl) GetPackageRegistry() EPackageRegistry {
	r.lazyMutex.Lock()
	defer r.lazyMutex.Unlock()
	if r.packageRegistry == nil {
		r.packageRegistry = NewEPackageRegistryImplWithDelegate(GetPackageRegistry())
	}
	return r.packageRegistry
}

func (r *EResourceSetImpl) SetPackageRegistry(packageRegistry EPackageRegistry) {
	r.lazyMutex.Lock()
	defer r.lazyMutex.Unlock()
	r.packageRegistry = packageRegistry
}

func (r *EResourceSetImpl) GetCodecRegistry() ECodecRegistry {
	r.lazyMutex.Lock()
	defer r.lazyMutex.Unlock()
	if r.resourceCodecRegistry == nil {
		r.resourceCodecRegistry = NewECodecRegistryImplWithDelegate(GetCodecRegistry())
	}
	return r.resourceCodecRegistry
}

func (r *EResourceSetImpl) SetCodecRegistry(resourceCodecRegistry ECodecRegistry) {
	r.lazyMutex.Lock()
	defer r.lazyMutex.Unlock()
	r.resourceCodecRegistry = resourceCodecRegistry
}

func (r *EResourceSetImpl) SetURIResourceMap(uriResourceMap map[*URI]EResource) {
	r.uriResourceMap = uriResourceMap
}

func (r *EResourceSetImpl) GetURIResourceMap() map[*URI]EResource {
	return r.uriResourceMap
}
