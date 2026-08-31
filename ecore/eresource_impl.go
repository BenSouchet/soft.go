package ecore

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
)

type EResourceInternal interface {
	EResource

	DoLoad(decoder EDecoder)
	DoSave(encoder EEncoder)
	DoUnload()

	IsAttachedDetachedRequired() bool
	DoAttached(o EObject)
	DoDetached(o EObject)

	BasicSetLoaded(bool, ENotificationChain) ENotificationChain
	BasicSetResourceSet(EResourceSet, ENotificationChain) ENotificationChain
}

type resourceNotification struct {
	AbstractNotification
	notifier  ENotifier
	featureID int
}

func (n *resourceNotification) GetNotifier() ENotifier {
	return n.notifier
}

func (n *resourceNotification) GetFeature() EStructuralFeature {
	return nil
}

func (n *resourceNotification) GetFeatureID() int {
	return n.featureID
}

func newResourceNotification(
	notifier ENotifier,
	featureID int,
	eventType EventType,
	oldValue any,
	newValue any,
	position int) *resourceNotification {
	n := new(resourceNotification)
	n.Initialize(n, eventType, oldValue, newValue, position)
	n.notifier = notifier
	n.featureID = featureID
	return n
}

type resourceContents struct {
	BasicENotifyingList
	resource *EResourceImpl
}

func newResourceContents(resource *EResourceImpl) *resourceContents {
	rc := new(resourceContents)
	rc.interfaces = rc
	rc.data = []any{}
	rc.isUnique = true
	rc.resource = resource
	return rc
}

func (rc *resourceContents) GetNotifier() ENotifier {
	return rc.resource.AsENotifier()
}

func (rc *resourceContents) GetFeatureID() int {
	return RESOURCE__CONTENTS
}

func (rc *resourceContents) inverseAdd(object any, notifications ENotificationChain) ENotificationChain {
	n := notifications
	if eObject, _ := object.(EObjectInternal); eObject != nil {
		eResource := rc.resource.AsEResource()
		n = eObject.ESetResource(eResource, n)
		eResource.Attached(eObject)
	}
	return n
}

func (rc *resourceContents) inverseRemove(object any, notifications ENotificationChain) ENotificationChain {
	n := notifications
	if eObject, _ := object.(EObjectInternal); eObject != nil {
		eResource := rc.resource.AsEResource()
		eResource.Detached(eObject)
		n = eObject.ESetResource(nil, n)
	}
	return n
}

func (rc *resourceContents) DidAdd(index int, elem any) {
	rc.BasicENotifyingList.DidAdd(index, elem)
	if index == rc.Size()-1 {
		rc.loaded()
	}
}

func (rc *resourceContents) DidRemove(index int, old any) {
	rc.BasicENotifyingList.DidRemove(index, old)
	if rc.Size() == 0 {
		rc.unloaded()
	}
}

func (rc *resourceContents) DidClear(oldObjects []any) {
	rc.BasicENotifyingList.DidClear(oldObjects)
	rc.unloaded()
}

func (rc *resourceContents) loaded() {
	if !rc.resource.IsLoaded() {
		n := rc.resource.BasicSetLoaded(true, nil)
		if n != nil {
			n.Dispatch()
		}
	}
}

func (rc *resourceContents) unloaded() {
	if rc.resource.IsLoaded() {
		n := rc.resource.BasicSetLoaded(false, nil)
		if n != nil {
			n.Dispatch()
		}
	}
}

type resourceDiagnostics struct {
	BasicENotifyingList
	resource  *EResourceImpl
	featureID int
}

func newResourceDiagnostics(resource *EResourceImpl, featureID int) *resourceDiagnostics {
	rd := new(resourceDiagnostics)
	rd.interfaces = rd
	rd.data = []any{}
	rd.isUnique = true
	rd.resource = resource
	rd.featureID = featureID
	return rd
}

func (rd *resourceDiagnostics) GetNotifier() ENotifier {
	return rd.resource.AsENotifier()
}

func (rd *resourceDiagnostics) GetFeatureID() int {
	return rd.featureID
}

// EResource ...
type EResourceImpl struct {
	ENotifierImpl
	resourceSet     EResourceSet
	objectIDManager EObjectIDManager
	uri             *URI
	contents        EList
	errors          EList
	warnings        EList
	isLoaded        bool
	isLoading       bool
	listeners       EList
	// idCache indexes intrinsic ids -> object for getObjectByID when no
	// EObjectIDManager is set. Built lazily by one tree scan, then kept O(1) and
	// maintained incrementally on attach/detach. Without it, getObjectByID is O(n)
	// per call -> O(n²) over a full ResolveAll of a large multi-document database.
	//
	// The lazy build is a read that writes: getObjectByID is reached from proxy
	// resolution, and Host.ReadStable resolves proxies under a SHARED lock, so any
	// number of readers can arrive here at once. idMutex is what makes the build
	// happen once and be published once.
	idCache map[string]EObject
	idMutex sync.Mutex
	// xmlComments holds the XML comments the document carried, which no part of the object
	// graph can represent. The XML decoder puts them here on load and the XML encoder reads
	// them back on save; see XMLComments. Nil for a resource decoded from a document with no
	// comments, and for every resource that was never XML at all.
	xmlComments *XMLComments
}

// NewBasicEObject is BasicEObject constructor
func NewEResourceImpl() *EResourceImpl {
	r := new(EResourceImpl)
	r.SetInterfaces(r)
	r.Initialize()
	return r
}

func (r *EResourceImpl) AsEResource() EResource {
	return r.GetInterfaces().(EResource)
}

func (r *EResourceImpl) AsEResourceInternal() EResourceInternal {
	return r.GetInterfaces().(EResourceInternal)
}

func (r *EResourceImpl) GetResourceSet() EResourceSet {
	return r.resourceSet
}

func (r *EResourceImpl) GetURI() *URI {
	return r.uri
}

func (r *EResourceImpl) SetURI(uri *URI) {
	oldURI := r.uri
	r.uri = uri
	if r.ENotificationRequired() {
		r.ENotify(newResourceNotification(r.AsENotifier(), RESOURCE__URI, SET, oldURI, uri, -1))
	}
}

func (r *EResourceImpl) GetContents() EList {
	if r.contents == nil {
		r.contents = newResourceContents(r)
	}
	return r.contents
}

func (r *EResourceImpl) GetAllContents() EIterator {
	return r.getAllContentsResolve(r.GetInterfaces(), true)
}

func (r *EResourceImpl) GetResourceListeners() EList {
	if r.listeners == nil {
		r.listeners = NewEmptyBasicEList()
	}
	return r.listeners
}

func (r *EResourceImpl) getAllContentsResolve(root any, resolve bool) EIterator {
	return newTreeIterator(root, false, func(o any) EIterator {
		if o == r.GetInterfaces() {
			return o.(EResource).GetContents().Iterator()
		}
		contents := o.(EObject).EContents()
		if !resolve {
			contents = contents.(EObjectList).GetUnResolvedList()
		}
		return contents.Iterator()
	})
}

func (r *EResourceImpl) GetEObject(uriFragment string) EObject {
	id := uriFragment
	size := len(uriFragment)
	if size > 0 {
		if uriFragment[0] == '/' {
			path := strings.Split(uriFragment, "/")
			path = path[1:]
			return r.getObjectByPath(path)
		} else if uriFragment[size-1] == '?' {
			if index := strings.LastIndex(uriFragment[:size-2], "?"); index != -1 {
				id = uriFragment[:index]
			}
		}
	}
	return r.getObjectByID(id)
}

func (r *EResourceImpl) GetURIFragment(eObject EObject) string {
	id := GetEObjectID(eObject)
	if len(id) > 0 {
		return id
	} else {
		internalEObject := eObject.(EObjectInternal)
		if internalEObject.EInternalResource() == r.interfaces {
			if id = r.getIDForObject(eObject); len(id) > 0 {
				return id
			} else {
				return "/" + r.getURIFragmentRootSegment(eObject)
			}
		} else {
			fragmentPath := []string{}
			isContained := false
			for eContainer, _ := internalEObject.EInternalContainer().(EObjectInternal); eContainer != nil; eContainer, _ = internalEObject.EInternalContainer().(EObjectInternal) {
				if id = r.getIDForObject(eObject); len(id) == 0 {
					fragmentPath = append([]string{eContainer.EURIFragmentSegment(internalEObject.EContainingFeature(), internalEObject)}, fragmentPath...)
				}
				internalEObject = eContainer
				if eContainer.EInternalResource() == r.interfaces {
					isContained = true
					break
				}
			}
			if !isContained {
				return "/-1"
			}
			if len(id) == 0 {
				fragmentPath = append([]string{r.getURIFragmentRootSegment(internalEObject)}, fragmentPath...)
				fragmentPath = append([]string{""}, fragmentPath...)
				return strings.Join(fragmentPath, "/")
			} else {
				return id
			}
		}
	}
}

func (r *EResourceImpl) getURIFragmentRootSegment(eObject EObject) string {
	contents := r.GetContents()
	if contents.Size() > 1 {
		return strconv.Itoa(contents.IndexOf(eObject))
	} else {
		return ""
	}
}

func (r *EResourceImpl) getIDForObject(eObject EObject) string {
	if r.objectIDManager != nil {
		if id := r.objectIDManager.GetID(eObject); id != nil {
			return fmt.Sprintf("%v", id)
		}
	}
	return GetEObjectID(eObject)
}

func (r *EResourceImpl) getObjectByID(id string) EObject {
	if r.objectIDManager != nil {
		return r.objectIDManager.GetEObject(id)
	}
	// Build the intrinsic-id index once (unresolved walk — no proxy resolution, so no
	// re-entrancy), then serve O(1). Keeps first-in-tree-order on collisions, matching
	// the previous linear scan's result. Held under idMutex: concurrent readers reach
	// this through proxy resolution, and the walk is safe to run under the lock
	// precisely because it does not resolve.
	r.idMutex.Lock()
	defer r.idMutex.Unlock()
	if r.idCache == nil {
		cache := make(map[string]EObject)
		for it := r.getAllContentsResolve(r.GetInterfaces(), false); it.HasNext(); {
			eObject := it.Next().(EObject)
			if objectID := GetEObjectID(eObject); objectID != "" {
				if _, exists := cache[objectID]; !exists {
					cache[objectID] = eObject
				}
			}
		}
		r.idCache = cache
	}
	return r.idCache[id]
}

func (r *EResourceImpl) getObjectByPath(uriFragmentPath []string) EObject {
	var eObject EObject
	if len(uriFragmentPath) == 0 {
		eObject = r.getObjectForRootSegment("")
	} else {
		eObject = r.getObjectForRootSegment(uriFragmentPath[0])
	}
	for i := 1; i < len(uriFragmentPath) && eObject != nil; i++ {
		eObject = eObject.(EObjectInternal).EObjectForFragmentSegment(uriFragmentPath[i])
	}
	return eObject
}

func (r *EResourceImpl) getObjectForRootSegment(rootSegment string) EObject {
	position := 0
	if len(rootSegment) > 0 {
		if rootSegment[0] == '?' {
			return r.getObjectByID(rootSegment[1:])
		} else {
			position, _ = strconv.Atoi(rootSegment)
		}
	}
	if position >= 0 && position < r.GetContents().Size() {
		return r.GetContents().Get(position).(EObject)
	}
	return nil
}

func (r *EResourceImpl) IsAttachedDetachedRequired() bool {
	return r.objectIDManager != nil || r.hasIDCache() || (r.listeners != nil && !r.listeners.Empty())
}

func (r *EResourceImpl) hasIDCache() bool {
	r.idMutex.Lock()
	defer r.idMutex.Unlock()
	return r.idCache != nil
}

// putIDCache / dropIDCache keep the index consistent with the tree. They take the same
// lock as the lazy build so a mutation can never land in a map a reader is still
// filling in.
func (r *EResourceImpl) putIDCache(object EObject) {
	r.idMutex.Lock()
	defer r.idMutex.Unlock()
	if r.idCache != nil {
		if id := GetEObjectID(object); id != "" {
			r.idCache[id] = object
		}
	}
}

func (r *EResourceImpl) dropIDCache(object EObject) {
	r.idMutex.Lock()
	defer r.idMutex.Unlock()
	if r.idCache != nil {
		if id := GetEObjectID(object); id != "" {
			delete(r.idCache, id)
		}
	}
}

func (r *EResourceImpl) clearIDCache() {
	r.idMutex.Lock()
	defer r.idMutex.Unlock()
	r.idCache = nil
}

func (r *EResourceImpl) Attached(object EObject) {
	if resourceInternal := r.AsEResourceInternal(); resourceInternal.IsAttachedDetachedRequired() {
		resourceInternal.DoAttached(object)
		for it := r.getAllContentsResolve(object, false); it.HasNext(); {
			if o, _ := it.Next().(EObject); o != nil {
				resourceInternal.DoAttached(o)
			}
		}
	}
}

func (r *EResourceImpl) DoAttached(object EObject) {
	if r.objectIDManager != nil {
		r.objectIDManager.Register(object)
	}
	r.putIDCache(object)
	if r.listeners != nil {
		for itListener := r.listeners.Iterator(); itListener.HasNext(); {
			listener := itListener.Next().(EResourceListener)
			listener.Attached(object)
		}
	}
}

func (r *EResourceImpl) Detached(object EObject) {
	if resourceInternal := r.AsEResourceInternal(); resourceInternal.IsAttachedDetachedRequired() {
		resourceInternal.DoDetached(object)
		for it := r.getAllContentsResolve(object, false); it.HasNext(); {
			if o, _ := it.Next().(EObject); o != nil {
				resourceInternal.DoDetached(o)
			}
		}
	}
}

func (r *EResourceImpl) DoDetached(object EObject) {
	if r.objectIDManager != nil {
		r.objectIDManager.UnRegister(object)
	}
	r.dropIDCache(object)
	if r.listeners != nil {
		for itListener := r.listeners.Iterator(); itListener.HasNext(); {
			listener := itListener.Next().(EResourceListener)
			listener.Detached(object)
		}
	}
}

var defaultURIConverter EURIConverter = NewEURIConverterImpl()

func (r *EResourceImpl) getURIConverter() EURIConverter {
	if r.resourceSet != nil {
		return r.resourceSet.GetURIConverter()
	}
	return defaultURIConverter
}

func (r *EResourceImpl) GetCodecRegistry() ECodecRegistry {
	if r.resourceSet != nil {
		return r.resourceSet.GetCodecRegistry()
	}
	return GetCodecRegistry()
}

func (r *EResourceImpl) Load() {
	r.LoadWithOptions(nil)
}

func (r *EResourceImpl) LoadWithOptions(options map[string]any) {
	if !r.isLoaded {
		// Merge the resource set's default load options so lazily-resolved
		// resources (loaded with no explicit options, e.g. during proxy
		// resolution) still get the set-wide decode options. A resource's own
		// options take precedence.
		if r.resourceSet != nil {
			if defaults := r.resourceSet.GetDefaultLoadOptions(); len(defaults) > 0 {
				merged := make(map[string]any, len(defaults)+len(options))
				for k, v := range defaults {
					merged[k] = v
				}
				for k, v := range options {
					merged[k] = v
				}
				options = merged
			}
		}
		uriConverter := r.getURIConverter()
		if uriConverter != nil && r.uri != nil {
			rd, err := uriConverter.CreateReader(r.uri)
			if err != nil {
				errors := r.GetErrors()
				errors.Clear()
				errors.Add(NewEDiagnosticImpl("Unable to create reader for '"+r.uri.String()+"' :"+err.Error(), r.uri.String(), 0, 0))
			} else if rd != nil {
				r.LoadWithReader(rd, options)
				rd.Close()
			}
		}
	}
}

func (r *EResourceImpl) LoadWithReader(rd io.Reader, options map[string]any) {
	if !r.isLoaded {
		codecs := r.GetCodecRegistry()
		if codec := codecs.GetCodec(r.uri); codec == nil {
			errors := r.GetErrors()
			errors.Clear()
			errors.Add(NewEDiagnosticImpl("Unable to find codec for '"+r.uri.String()+"'", r.uri.String(), 0, 0))
		} else if decoder := codec.NewDecoder(r.AsEResource(), rd, options); decoder == nil {
			errors := r.GetErrors()
			errors.Clear()
			errors.Add(NewEDiagnosticImpl("Unable to create decoder for '"+r.uri.String()+"'", r.uri.String(), 0, 0))
		} else {
			r.isLoading = true
			ri := r.AsEResourceInternal()
			n := ri.BasicSetLoaded(true, nil)
			if r.errors != nil {
				r.errors.Clear()
			}
			if r.warnings != nil {
				r.warnings.Clear()
			}
			ri.DoLoad(decoder)
			if n != nil {
				n.Dispatch()
			}
			r.isLoading = false
		}
	}
}

func (r *EResourceImpl) DoLoad(decoder EDecoder) {
	decoder.DecodeResource()
}

func (r *EResourceImpl) Unload() {
	if r.isLoaded {
		n := r.BasicSetLoaded(false, nil)
		r.GetInterfaces().(EResourceInternal).DoUnload()
		if n != nil {
			n.Dispatch()
		}
	}
}

func (r *EResourceImpl) setXMLComments(c *XMLComments) {
	r.xmlComments = c
}

func (r *EResourceImpl) getXMLComments() *XMLComments {
	return r.xmlComments
}

func (r *EResourceImpl) DoUnload() {
	r.contents = nil
	r.errors = nil
	r.warnings = nil
	r.xmlComments = nil
	r.clearIDCache()
	if r.objectIDManager != nil {
		r.objectIDManager.Clear()
	}
}

func (r *EResourceImpl) IsLoaded() bool {
	return r.isLoaded
}

func (r *EResourceImpl) IsLoading() bool {
	return r.isLoading
}

func (r *EResourceImpl) SetLoading(isLoading bool) {
	r.isLoading = isLoading
}

func (r *EResourceImpl) Save() {
	r.SaveWithOptions(nil)
}

func (r *EResourceImpl) SaveWithOptions(options map[string]any) {
	uriConverter := r.getURIConverter()
	if uriConverter != nil && r.uri != nil {
		w, err := uriConverter.CreateWriter(r.uri)
		if err != nil {
			errors := r.GetErrors()
			errors.Clear()
			errors.Add(NewEDiagnosticImpl("Unable to create writer for '"+r.uri.String()+"' :"+err.Error(), r.uri.String(), 0, 0))
		} else if w != nil {
			r.SaveWithWriter(w, options)
			w.Close()
		}
	}
}

func (r *EResourceImpl) SaveWithWriter(w io.Writer, options map[string]any) {
	codecs := r.GetCodecRegistry()
	if codec := codecs.GetCodec(r.uri); codec == nil {
		errors := r.GetErrors()
		errors.Clear()
		errors.Add(NewEDiagnosticImpl("Unable to find codec for '"+r.uri.String()+"'", r.uri.String(), 0, 0))
	} else if encoder := codec.NewEncoder(r.AsEResource(), w, options); encoder == nil {
		errors := r.GetErrors()
		errors.Clear()
		errors.Add(NewEDiagnosticImpl("Unable to create encoder for '"+r.uri.String()+"'", r.uri.String(), 0, 0))
	} else {
		if r.errors != nil {
			r.errors.Clear()
		}
		if r.warnings != nil {
			r.warnings.Clear()
		}
		r.GetInterfaces().(EResourceInternal).DoSave(encoder)
	}
}

func (r *EResourceImpl) DoSave(encoder EEncoder) {
	encoder.EncodeResource()
}

func (r *EResourceImpl) GetErrors() EList {
	if r.errors == nil {
		r.errors = newResourceDiagnostics(r, RESOURCE__ERRORS)
	}
	return r.errors
}

func (r *EResourceImpl) GetWarnings() EList {
	if r.warnings == nil {
		r.warnings = newResourceDiagnostics(r, RESOURCE__WARNINGS)
	}
	return r.warnings
}

func (r *EResourceImpl) BasicSetLoaded(isLoaded bool, msgs ENotificationChain) ENotificationChain {
	notifications := msgs
	oldLoaded := r.isLoaded
	r.isLoaded = isLoaded
	if r.ENotificationRequired() {
		if notifications == nil {
			notifications = NewNotificationChain()
		}
		notifications.Add(newResourceNotification(r.GetInterfaces().(ENotifier), RESOURCE__IS_LOADED, SET, oldLoaded, r.isLoaded, -1))
	}
	return notifications
}

func (r *EResourceImpl) BasicSetResourceSet(resourceSet EResourceSet, msgs ENotificationChain) ENotificationChain {
	notifications := msgs
	oldAbstractResourceSet := r.resourceSet
	if oldAbstractResourceSet != nil {
		l := oldAbstractResourceSet.GetResources().(ENotifyingList)
		notifications = l.RemoveWithNotification(r.GetInterfaces().(ENotifier), notifications)
	}
	r.resourceSet = resourceSet
	if r.ENotificationRequired() {
		if notifications == nil {
			notifications = NewNotificationChain()
		}
		notifications.Add(newResourceNotification(r.GetInterfaces().(ENotifier), RESOURCE__RESOURCE_SET, SET, oldAbstractResourceSet, resourceSet, -1))
	}
	return notifications
}

func (r *EResourceImpl) SetObjectIDManager(objectIDManager EObjectIDManager) {
	r.objectIDManager = objectIDManager
}

func (r *EResourceImpl) GetObjectIDManager() EObjectIDManager {
	return r.objectIDManager
}
