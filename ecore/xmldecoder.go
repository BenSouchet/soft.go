// *****************************************************************************
// Copyright(c) 2021 MASA Group
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// *****************************************************************************

package ecore

import (
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"golang.org/x/net/html/charset"
)

const (
	href                            = "href"
	typeAttrib                      = "type"
	schemaLocationAttrib            = "schemaLocation"
	noNamespaceSchemaLocationAttrib = "noNamespaceSchemaLocation"
	xsiURI                          = "http://www.w3.org/2001/XMLSchema-instance"
	xsiNS                           = "xsi"
	xmlNS                           = "xmlns"
)

type xmlLoadFeatureKind int

const (
	xlfkSingle xmlLoadFeatureKind = iota
	xlfkMany
	xlfkManyAdd
	xlfkManyMove
	xlfkOther
)

const (
	load_object_type = "object"
	load_error_type  = "error"
)

type xmlReference struct {
	object  EObject
	feature EStructuralFeature
	id      string
	pos     int
}

type xmlDecoderInternal interface {
	getXSIType() string
	handleAttributes(object EObject)
}

type XMLDecoder struct {
	resource               EResource
	interfaces             any
	spacesToFactories      map[string]EFactory
	attachFn               func(object EObject)
	errorFn                func(diagnostic EDiagnostic)
	extendedMetaData       *ExtendedMetaData
	prefixesToURI          map[string]string
	decoder                *xml.Decoder
	textBuilder            *strings.Builder
	namespaces             *xmlNamespaces
	xmlVersion             string
	encoding               string
	idAttributeName        string
	references             []xmlReference
	attributes             []xml.Attr
	sameDocumentProxies    []EObject
	notFeatures            []xml.Name
	types                  []any
	objects                []EObject
	deferred               []EObject
	elements               []string
	simpleFeatures         map[EClass]EStructuralFeature
	isSuppressDocumentRoot bool
	isResolveDeferred      bool
	isIDReferenceAsProxy   bool
}

func NewXMLDecoder(resource EResource, r io.Reader, options map[string]any) *XMLDecoder {
	l := new(XMLDecoder)
	l.interfaces = l
	l.resource = resource
	l.decoder = xml.NewDecoder(r)
	l.decoder.CharsetReader = charset.NewReaderLabel
	l.namespaces = newXMLNamespaces()
	l.prefixesToURI = make(map[string]string)
	l.spacesToFactories = make(map[string]EFactory)
	l.simpleFeatures = make(map[EClass]EStructuralFeature)
	l.notFeatures = append(l.notFeatures, xml.Name{Space: xsiURI, Local: typeAttrib}, xml.Name{Space: xsiURI, Local: schemaLocationAttrib}, xml.Name{Space: xsiURI, Local: noNamespaceSchemaLocationAttrib})
	if options != nil {
		l.idAttributeName, _ = options[XML_OPTION_ID_ATTRIBUTE_NAME].(string)
		l.isResolveDeferred = options[XML_OPTION_DEFERRED_REFERENCE_RESOLUTION] == true
		l.isIDReferenceAsProxy = options[XML_OPTION_ID_REFERENCE_AS_PROXY] == true
		l.isSuppressDocumentRoot = options[XML_OPTION_SUPPRESS_DOCUMENT_ROOT] == true
		if extendedMetaData := options[XML_OPTION_EXTENDED_META_DATA]; extendedMetaData != nil {
			l.extendedMetaData = extendedMetaData.(*ExtendedMetaData)
		}
		if options[XML_OPTION_DEFERRED_ROOT_ATTACHMENT] == true {
			l.deferred = []EObject{}
		}
	}
	if l.extendedMetaData == nil {
		l.extendedMetaData = NewExtendedMetaData()
	}
	return l
}

func (l *XMLDecoder) GetXMLVersion() string {
	return l.xmlVersion
}

func (l *XMLDecoder) GetEncoding() string {
	return l.encoding
}

func (l *XMLDecoder) DecodeResource() {
	l.attachFn = func(object EObject) {
		l.resource.GetContents().Add(object)
	}
	l.errorFn = func(diagnostic EDiagnostic) {
		l.resource.GetErrors().Add(diagnostic)
	}
	l.decodeTopObject()
}

func (l *XMLDecoder) DecodeObject() (eObject EObject, err error) {
	l.attachFn = func(o EObject) {
		if eObject == nil {
			eObject = o
		}
	}
	l.errorFn = func(diagnostic EDiagnostic) {
		if err == nil {
			err = diagnostic
		}
	}
	l.decodeTopObject()
	return
}

func (l *XMLDecoder) decodeTopObject() {
	for {
		t, tokenErr := l.decoder.Token()
		if tokenErr != nil {
			if tokenErr != io.EOF {
				l.error(NewEDiagnosticImpl(tokenErr.Error(), l.resource.GetURI().String(), int(l.decoder.InputOffset()), 0))
			}
			break
		}
		switch t := t.(type) {
		case xml.StartElement:
			l.startElement(t)
		case xml.EndElement:
			l.endElement(t)
		case xml.CharData:
			l.text(string([]byte(t)))
		case xml.Comment:
			l.comment(string([]byte(t)))
		case xml.ProcInst:
			l.processingInstruction(t)
		case xml.Directive:
			l.directive(string([]byte(t)))
		}
	}
}

func (l *XMLDecoder) startElement(e xml.StartElement) {
	l.elements = append(l.elements, e.Name.Local)
	l.setAttributes(e.Attr)
	l.namespaces.pushContext()
	l.handlePrefixMapping()
	if len(l.objects) == 0 {
		l.handleSchemaLocation()
	}
	l.processElement(e.Name.Space, e.Name.Local)
}

func (l *XMLDecoder) endElement(_ xml.EndElement) {
	if len(l.elements) > 0 {
		l.elements = l.elements[:len(l.elements)-1]
	}

	// remove last object
	var eRoot EObject
	var eObject EObject
	if len(l.objects) > 0 {
		eRoot = l.objects[0]
		eObject = l.objects[len(l.objects)-1]
		l.objects = l.objects[:len(l.objects)-1]
	}

	// remove last type
	var eType any
	if len(l.types) > 0 {
		eType = l.types[len(l.types)-1]
		l.types = l.types[:len(l.types)-1]
	}
	if l.textBuilder != nil {
		if eType == load_object_type {
			if l.textBuilder.Len() > 0 {
				// Text directly inside an object element is either this class's
				// simple content (set it on the kind="simple" feature) or, failing
				// that, a proxy URI written as element text.
				if sf := l.getSimpleFeature(eObject.EClass()); sf != nil {
					l.setFeatureValue(eObject, sf, l.textBuilder.String(), -1)
				} else {
					l.handleProxy(eObject, l.textBuilder.String())
				}
			}
		} else if eType != load_error_type {
			if eObject == nil && len(l.objects) > 0 {
				eObject = l.objects[len(l.objects)-1]
			}
			l.setFeatureValue(eObject, eType.(EStructuralFeature), l.textBuilder.String(), -1)
		}
	}
	l.textBuilder = nil

	// end of the document
	if len(l.elements) == 0 {
		for _, deferred := range l.deferred {
			l.resource.GetContents().Add(deferred)
		}
		l.handleReferences()
		l.recordSchemaLocations(eRoot)
	}

	context := l.namespaces.popContext()
	for _, p := range context {
		delete(l.spacesToFactories, p[1].(string))
	}

}

func (l *XMLDecoder) setAttributes(attributes []xml.Attr) []xml.Attr {
	old := l.attributes
	l.attributes = attributes
	return old
}

func (l *XMLDecoder) getAttributeValue(uri string, local string) string {
	if l.attributes != nil {
		for _, attr := range l.attributes {
			if attr.Name.Space == uri && attr.Name.Local == local {
				return attr.Value
			}
		}
	}
	return ""
}

func (l *XMLDecoder) getXSIType() string {
	return l.getAttributeValue(xsiURI, typeAttrib)
}

func (l *XMLDecoder) handleSchemaLocation() {
	xsiSchemaLocation := l.getAttributeValue(xsiURI, schemaLocationAttrib)
	if len(xsiSchemaLocation) > 0 {
		l.handleXSISchemaLocation(xsiSchemaLocation)
	}

	xsiNoNamespaceSchemaLocation := l.getAttributeValue(xsiURI, noNamespaceSchemaLocationAttrib)
	if len(xsiNoNamespaceSchemaLocation) > 0 {
		l.handleXSINoNamespaceSchemaLocation(xsiNoNamespaceSchemaLocation)
	}
}

func (l *XMLDecoder) handleXSISchemaLocation(loc string) {
}

func (l *XMLDecoder) handleXSINoNamespaceSchemaLocation(loc string) {
}

func (l *XMLDecoder) handlePrefixMapping() {
	if l.attributes != nil {
		for _, attr := range l.attributes {
			if attr.Name.Space == xmlNS {
				l.startPrefixMapping(attr.Name.Local, attr.Value)
			} else if attr.Name.Space == "" && attr.Name.Local == xmlNS {
				l.startPrefixMapping("", attr.Value)
			}
		}
	}
}

func (l *XMLDecoder) startPrefixMapping(prefix string, uri string) {
	l.namespaces.declarePrefix(prefix, uri)
	if _, exists := l.prefixesToURI[prefix]; exists {
		index := 1
		for _, exists = l.prefixesToURI[prefix+"_"+fmt.Sprintf("%d", index)]; exists; {
			index++
		}
		prefix += "_" + fmt.Sprintf("%d", index)
	}
	l.prefixesToURI[prefix] = uri
	delete(l.spacesToFactories, uri)
}

func (l *XMLDecoder) processElement(space string, local string) {
	if len(l.objects) == 0 {
		eObject := l.createTopObject(space, local)
		if eObject != nil {
			if l.deferred != nil {
				l.deferred = append(l.deferred, eObject)
			} else {
				l.attachFn(eObject)
			}
		}
	} else {
		l.handleFeature(space, local)
	}
}

func (l *XMLDecoder) validateObject(eObject EObject, space, typeName string) {
	if eObject == nil {
		l.error(NewEDiagnosticImpl("Class {'"+space+"':'"+typeName+"'} not found", l.resource.GetURI().String(), int(l.decoder.InputOffset()), 0))
	}
}

func (l *XMLDecoder) processObject(eObject EObject) {
	if eObject != nil {
		l.objects = append(l.objects, eObject)
		l.types = append(l.types, load_object_type)
		// If this object's class carries simple content (a kind="simple"
		// attribute), capture the element's inline text so endElement can set it
		// on that feature, e.g. <comments>en g/m3</comments>.
		if l.getSimpleFeature(eObject.EClass()) != nil {
			l.textBuilder = &strings.Builder{}
		}
	} else {
		l.types = append(l.types, load_error_type)
	}
}

// getSimpleFeature returns the class's XSD simple-content feature (the
// kind="simple" attribute), or nil, cached per class.
func (l *XMLDecoder) getSimpleFeature(eClass EClass) EStructuralFeature {
	if f, ok := l.simpleFeatures[eClass]; ok {
		return f
	}
	var found EStructuralFeature
	if l.extendedMetaData != nil {
		for it := eClass.GetEAllStructuralFeatures().Iterator(); it.HasNext(); {
			f := it.Next().(EStructuralFeature)
			if _, isAttr := f.(EAttribute); isAttr && l.extendedMetaData.GetFeatureKind(f) == "simple" {
				found = f
				break
			}
		}
	}
	l.simpleFeatures[eClass] = found
	return found
}

func (l *XMLDecoder) createTopObject(space string, local string) EObject {
	eFactory := l.getFactoryForSpace(space)
	if eFactory != nil {
		ePackage := eFactory.GetEPackage()
		if l.extendedMetaData != nil && l.extendedMetaData.GetDocumentRoot(ePackage) != nil {
			eClass := l.extendedMetaData.GetDocumentRoot(ePackage)
			// add document root to object list & handle its features
			eObject := l.createObjectWithFactory(eFactory, eClass, false)
			l.processObject(eObject)
			l.handleFeature(space, local)
			if l.isSuppressDocumentRoot {
				// remove document root from object list
				l.objects = l.objects[1:]
				// remove type info from type list
				l.types = l.types[1:]

				// consider new child object as the future new root
				// remove it from document root
				if len(l.objects) > 0 {
					eObject = l.objects[0]
					// remove new object from its container ( document root )
					Remove(eObject)
				}
			}
			return eObject
		} else {
			eType := l.getType(ePackage, local)
			eObject := l.createObjectWithFactory(eFactory, eType, true)
			l.validateObject(eObject, space, local)
			l.processObject(eObject)
			return eObject
		}
	} else {
		prefix, _ := l.namespaces.getPrefix(space)
		l.handleUnknownPackage(prefix, space)
		return nil
	}
}

func (l *XMLDecoder) createObjectWithFactory(eFactory EFactory, eType EClassifier, handleAttributes bool) EObject {
	if eFactory != nil {
		eClass, _ := eType.(EClass)
		if eClass != nil && !eClass.IsAbstract() {
			eObject := eFactory.Create(eClass)
			if eObject != nil && handleAttributes {
				l.interfaces.(xmlDecoderInternal).handleAttributes(eObject)
			}
			return eObject
		}
	}
	return nil
}

func (l *XMLDecoder) createObjectFromFeatureType(eObject EObject, eFeature EStructuralFeature) EObject {
	var eResult EObject
	if eFeature != nil {
		if eType := eFeature.GetEType(); eType != nil {
			eFactory := eType.GetEPackage().GetEFactoryInstance()
			eResult = l.createObjectWithFactory(eFactory, eType, true)
		}
	}
	if eResult != nil {
		l.setFeatureValue(eObject, eFeature, eResult, -1)
	}
	l.processObject(eResult)
	return eResult
}

func (l *XMLDecoder) createObjectFromTypeName(eObject EObject, qname string, eFeature EStructuralFeature) EObject {
	prefix := ""
	local := qname
	if index := strings.Index(qname, ":"); index > 0 {
		prefix = qname[:index]
		local = qname[index+1:]
	}
	space, _ := l.namespaces.getURI(prefix)
	eFactory := l.getFactoryForSpace(space)
	if eFactory == nil {
		l.handleUnknownPackage(prefix, space)
		return nil
	}

	eType := l.getType(eFactory.GetEPackage(), local)
	eResult := l.createObjectWithFactory(eFactory, eType, true)
	l.validateObject(eResult, space, local)
	if eResult != nil {
		l.setFeatureValue(eObject, eFeature, eResult, -1)
	}
	l.processObject(eResult)
	return eResult
}

func (l *XMLDecoder) handleFeature(space string, local string) {
	var eObject EObject
	if len(l.objects) > 0 {
		eObject = l.objects[len(l.objects)-1]
	}
	if eObject != nil {
		eFeature := l.getFeature(eObject, space, local)
		if eFeature != nil {
			if featureKind := l.getLoadFeatureKind(eFeature); featureKind == xlfkSingle || featureKind == xlfkMany {
				l.textBuilder = &strings.Builder{}
				l.types = append(l.types, eFeature)
				l.objects = append(l.objects, nil)
			} else {
				xsiType := l.interfaces.(xmlDecoderInternal).getXSIType()
				if len(xsiType) > 0 {
					l.createObjectFromTypeName(eObject, xsiType, eFeature)
				} else {
					l.createObjectFromFeatureType(eObject, eFeature)
				}
			}
		} else {
			// Unknown element: push a nil-object placeholder so this element and its
			// whole subtree pop balanced in endElement; otherwise the matching pop
			// would remove a real ancestor object and silently drop later siblings.
			l.types = append(l.types, load_error_type)
			l.objects = append(l.objects, nil)
			l.handleUnknownFeature(local)
		}
	} else {
		// Already inside an unknown (placeholder) subtree: keep it balanced.
		l.types = append(l.types, load_error_type)
		l.objects = append(l.objects, nil)
		l.handleUnknownFeature(local)
	}

}

// xmlAppendUnique appends value to a many-valued feature list during load using
// the unchecked AddUnique fast path when the list supports it (every soft.go
// list, via AbstractEList), falling back to Add otherwise (e.g. a mock). This
// turns the O(n²) load of a large many-valued reference (Add's per-element
// uniqueness scan) into O(n); see AbstractEList.AddUnique.
func xmlAppendUnique(eList EList, value any) {
	if au, ok := eList.(interface{ AddUnique(any) }); ok {
		au.AddUnique(value)
	} else {
		eList.Add(value)
	}
}

func (l *XMLDecoder) setFeatureValue(eObject EObject,
	eFeature EStructuralFeature,
	value any,
	position int) {
	kind := l.getLoadFeatureKind(eFeature)
	switch kind {
	case xlfkSingle:
		eDataType := eFeature.GetEType().(EDataType)
		if value == nil {
			eObject.ESet(eFeature, nil)
		} else if eFactory := l.factoryFor(eFeature, eDataType); eFactory != nil {
			eObject.ESet(eFeature, eFactory.CreateFromString(eDataType, value.(string)))
		}

	case xlfkMany:
		eDataType := eFeature.GetEType().(EDataType)
		eList := eObject.EGetResolve(eFeature, false).(EList)
		if position == -2 {
		} else if value == nil {
			xmlAppendUnique(eList, nil)
		} else if eFactory := l.factoryFor(eFeature, eDataType); eFactory != nil {
			xmlAppendUnique(eList, eFactory.CreateFromString(eDataType, value.(string)))
		}
	case xlfkManyAdd:
		fallthrough
	case xlfkManyMove:
		eList := eObject.EGetResolve(eFeature, false).(EList)
		if position == -1 {
			xmlAppendUnique(eList, value)
		} else if position == -2 {
			eList.Clear()
		} else if eObject == value {
			index := eList.IndexOf(value)
			if index == -1 {
				eList.Insert(position, value)
			} else {
				eList.Move(position, index)
			}
		} else if kind == xlfkManyAdd {
			xmlAppendUnique(eList, value)
		} else {
			eList.MoveObject(position, value)
		}
	default:
		eObject.ESet(eFeature, value)
	}
}

// factoryFor is the factory that builds values of eDataType, or nil with the reason already
// reported as a load diagnostic.
//
// It exists because the chain it replaces — eDataType.GetEPackage().GetEFactoryInstance() —
// dereferenced both links unchecked, on the hottest path in the decoder. A data type whose
// package link is empty is not a fantasy: a classifier is only in a package while it is
// CONTAINED by one (EClassifierImpl.GetEPackage tests the container feature id), so a type that
// is detached, still being built, or reached through a proxy that resolved to something
// unattached returns nil there. Every one of those is a malformed metamodel, and every one of
// them used to kill the process from inside a decode — which in a server means one bad model
// takes down every session, with a stack that names the position argument and not the attribute,
// the element or the type. A decoder's answer to a metamodel it cannot use is a diagnostic
// naming what it met, and skipping the value: the resource then reports an error, which is what
// every other malformed-input path here already does.
func (l *XMLDecoder) factoryFor(eFeature EStructuralFeature, eDataType EDataType) EFactory {
	ePackage := eDataType.GetEPackage()
	if ePackage == nil {
		l.decodeError("Data type " + eDataType.GetName() + " of feature " + eFeature.GetName() + " belongs to no package")
		return nil
	}
	eFactory := ePackage.GetEFactoryInstance()
	if eFactory == nil {
		l.decodeError("Package " + ePackage.GetNsURI() + " has no factory to build " + eDataType.GetName() + " for feature " + eFeature.GetName())
	}
	return eFactory
}

func (l *XMLDecoder) decodeError(message string) {
	l.error(NewEDiagnosticImpl(message, l.resource.GetURI().String(), int(l.decoder.InputOffset()), 0))
}

func (l *XMLDecoder) getLoadFeatureKind(eFeature EStructuralFeature) xmlLoadFeatureKind {
	eClassifier := eFeature.GetEType()
	if eDataType, _ := eClassifier.(EDataType); eDataType != nil {
		if eFeature.IsMany() {
			return xlfkMany
		}
		return xlfkSingle
	} else if eFeature.IsMany() {
		eReference := eFeature.(EReference)
		eOpposite := eReference.GetEOpposite()
		if eOpposite == nil || eOpposite.IsTransient() || !eOpposite.IsMany() {
			return xlfkManyAdd
		}
		return xlfkManyMove
	}
	return xlfkOther
}

func (l *XMLDecoder) handleAttributes(eObject EObject) {
	if l.attributes != nil {
		for _, attr := range l.attributes {
			name := attr.Name.Local
			uri := attr.Name.Space
			value := attr.Value
			if name == l.idAttributeName {
				if idManager := l.resource.GetObjectIDManager(); idManager != nil {
					if err := idManager.SetID(eObject, value); err != nil {
						l.error(NewEDiagnosticImpl(err.Error(), l.resource.GetURI().String(), int(l.decoder.InputOffset()), 0))
					}
				}
			} else if name == href {
				l.handleProxy(eObject, value)
			} else if name != xmlNS && uri != xmlNS && l.isUserAttribute(attr.Name) {
				l.setAttributeValue(eObject, name, value)
			}
		}
	}
}

func (l *XMLDecoder) isUserAttribute(name xml.Name) bool {
	for _, notFeature := range l.notFeatures {
		if notFeature == name {
			return false
		}
	}
	return true
}

func (l *XMLDecoder) getFactoryForSpace(space string) EFactory {
	factory := l.spacesToFactories[space]
	if factory == nil {
		packageRegistry := GetPackageRegistry()
		if l.resource.GetResourceSet() != nil {
			packageRegistry = l.resource.GetResourceSet().GetPackageRegistry()
		}
		factory = packageRegistry.GetFactory(space)
		if factory != nil {
			l.spacesToFactories[space] = factory
		}
	}
	return factory
}

func (l *XMLDecoder) setAttributeValue(eObject EObject, qname string, value string) {
	local := qname
	prefix := ""
	if index := strings.Index(qname, ":"); index > 0 {
		local = qname[index+1:]
		prefix = qname[:index-1]
	}
	space, _ := l.namespaces.getURI(prefix)
	eFeature := l.getFeature(eObject, space, local)
	if eFeature != nil {
		kind := l.getLoadFeatureKind(eFeature)
		if kind == xlfkSingle || kind == xlfkMany {
			l.setFeatureValue(eObject, eFeature, value, -2)
		} else {
			l.setValueFromId(eObject, eFeature.(EReference), value)
		}
	} else {
		l.handleUnknownFeature(local)
	}
}

func (l *XMLDecoder) setValueFromId(eObject EObject, eReference EReference, ids string) {
	mustAdd := l.isResolveDeferred
	mustAddOrNotOppositeIsMany := false
	isFirstID := true
	position := 0
	references := []xmlReference{}
	// A reference attribute is, by the XML IDREFS convention, a space-separated
	// list of ids. That convention is incompatible with databases whose ids are
	// human names containing spaces ("Missile SA Stinger"): splitting shreds one
	// reference into several bogus ones. When ids are resolved by name as proxies
	// (XML_OPTION_ID_REFERENCE_AS_PROXY) the whole attribute value is a single id —
	// multi-valued references are expressed as repeated elements, not a space list.
	var tokens []string
	if l.isIDReferenceAsProxy {
		tokens = []string{ids}
	} else {
		tokens = strings.Split(ids, " ")
	}
	qName := ""
	for _, token := range tokens {
		id := token
		if index := strings.Index(id, "#"); index != -1 {
			if index == 0 {
				id = id[1:]
			} else {
				oldAttributes := l.setAttributes(nil)
				var eProxy EObject
				if len(qName) == 0 {
					eProxy = l.createObjectFromFeatureType(eObject, eReference)
				} else {
					eProxy = l.createObjectFromTypeName(eObject, qName, eReference)
				}
				l.setAttributes(oldAttributes)
				if eProxy != nil {
					l.handleProxy(eProxy, id)
				}
				l.objects = l.objects[:len(l.objects)-1]
				qName = ""
				position++
				continue
			}
		} else if index := strings.Index(id, ":"); index != -1 && !l.isIDReferenceAsProxy {
			// A ':' normally separates a cross-document proxy's xsi:type qName
			// (prefix:Type). But under id-reference-as-proxy the whole value is a
			// single name resolved later, and authoring names legitimately contain
			// ':' ("EMB - AGC : lunette chef d'engins HS") — same rationale as the
			// space-splitting note above. Treating it as a qName here drops the ref
			// (position stays 0 -> the feature is unset), so skip qName handling.
			qName = id
			continue
		}

		if !l.isResolveDeferred {
			if isFirstID {
				eOpposite := eReference.GetEOpposite()
				if eOpposite != nil {
					mustAdd = eOpposite.IsTransient() || eReference.IsMany()
					mustAddOrNotOppositeIsMany = mustAdd || !eOpposite.IsMany()
				} else {
					mustAdd = true
					mustAddOrNotOppositeIsMany = true
				}
				isFirstID = false
			}

			if mustAddOrNotOppositeIsMany {
				resolved := l.resource.GetEObject(id)
				if resolved != nil {
					l.setFeatureValue(eObject, eReference, resolved, -1)
					qName = ""
					position++
					continue
				}
			}
		}

		if mustAdd {
			references = append(references, xmlReference{object: eObject, feature: eReference, id: id, pos: position})
		}

		qName = ""
		position++
	}

	if position == 0 {
		l.setFeatureValue(eObject, eReference, nil, -2)
	} else {
		l.references = append(l.references, references...)
	}
}

func (l *XMLDecoder) handleProxy(eProxy EObject, id string) {
	resourceURI := l.resource.GetURI()
	uri, ok := ParseURI(id)
	if ok != nil || resourceURI == nil {
		return
	}
	// resolve reference uri
	if !uri.IsAbsolute() {
		uri = resourceURI.Resolve(uri)
	}

	// set object proxy uri
	eProxy.(EObjectInternal).ESetProxyURI(uri)

	if resourceURI.Equals(uri.TrimFragment()) {
		l.sameDocumentProxies = append(l.sameDocumentProxies, eProxy)
	}
}

func (l *XMLDecoder) handleReferences() {
	for _, eProxy := range l.sameDocumentProxies {
		for itRef := eProxy.EClass().GetEAllReferences().Iterator(); itRef.HasNext(); {
			eReference := itRef.Next().(EReference)
			eOpposite := eReference.GetEOpposite()
			if eOpposite != nil && eOpposite.IsChangeable() && eProxy.EIsSet(eReference) {
				resolvedObject := l.resource.GetEObject(eProxy.(EObjectInternal).EProxyURI().Fragment())
				if resolvedObject != nil {
					var proxyHolder EObject
					if eReference.IsMany() {
						value := eProxy.EGet(eReference)
						list := value.(EList)
						proxyHolder = list.Get(0).(EObject)
					} else {
						value := eProxy.EGet(eReference)
						proxyHolder = value.(EObject)
					}

					if eOpposite.IsMany() {
						value := proxyHolder.EGetResolve(eOpposite, false)
						holderContents := value.(EList)
						resolvedIndex := holderContents.IndexOf(resolvedObject)
						if resolvedIndex != -1 {
							proxyIndex := holderContents.IndexOf(eProxy)
							holderContents.Move(proxyIndex, resolvedIndex)
							if proxyIndex > resolvedIndex {
								holderContents.Remove(proxyIndex - 1)
							} else {
								holderContents.Remove(proxyIndex + 1)
							}
							break
						}
					}

					replace := false
					if eReference.IsMany() {
						value := resolvedObject.EGet(eReference)
						list := value.(EList)
						replace = !list.Contains(proxyHolder)
					} else {
						value := resolvedObject.EGet(eReference)
						object := value.(EObject)
						replace = object != proxyHolder
					}

					if replace {
						if eOpposite.IsMany() {
							value := proxyHolder.EGetResolve(eOpposite, false)
							list := value.(EList)
							ndx := list.IndexOf(eProxy)
							list.Set(ndx, resolvedObject)
						} else {
							proxyHolder.ESet(eOpposite, resolvedObject)
						}
					}
					break
				}
			}
		}
	}

	for _, reference := range l.references {
		// An empty id is not a reference (e.g. an optional ref attribute present but
		// empty). Skip it: do not resolve, do not proxy (an empty proxy URI would
		// later be loaded as a file with an empty path), and do not error.
		if reference.id == "" {
			continue
		}
		eObject := l.resource.GetEObject(reference.id)
		if eObject == nil && l.isIDReferenceAsProxy {
			// An id that resolves to nothing in THIS document is left as a proxy
			// carrying the raw id, for an external resolver (e.g. a name-based
			// cross-document index) to resolve after load — instead of a spurious
			// unresolved-reference error. This is what lets a database whose
			// references are document-local names spanning many files load cleanly,
			// and avoids appending one diagnostic per cross-file reference (the
			// errors list is unique → Add is O(n) → O(n²) over a large database).
			eObject = l.createIDReferenceProxy(reference.feature, reference.id)
		}
		if eObject != nil {
			l.setFeatureValue(reference.object, reference.feature, eObject, reference.pos)
		} else {
			l.error(NewEDiagnosticImpl(
				"Unresolved reference '"+reference.id+"'", l.resource.GetURI().String(), int(l.decoder.InputOffset()), 0))
		}
	}
}

// createIDReferenceProxy builds a standalone proxy EObject of the reference's
// target type whose proxy URI is the raw, unresolved id. It is not attached to
// any feature here; the caller places it via setFeatureValue so multiplicity and
// position are handled the same as a resolved object. Used only when
// XML_OPTION_ID_REFERENCE_AS_PROXY is set.
func (l *XMLDecoder) createIDReferenceProxy(eFeature EStructuralFeature, id string) EObject {
	if id == "" {
		return nil // an empty id is no reference; never build an empty-URI proxy
	}
	eReference, _ := eFeature.(EReference)
	if eReference == nil {
		return nil
	}
	eType := eReference.GetEType()
	eFactory := eType.GetEPackage().GetEFactoryInstance()
	eProxy := l.createObjectWithFactory(eFactory, eType, false)
	if eProxy == nil {
		// The reference target type is abstract (a polymorphic reference): the
		// factory cannot instantiate it, but the proxy only needs to carry the id
		// until an external resolver replaces it with the concrete object. A
		// dynamic EObject typed as the abstract class is a valid, type-compatible
		// placeholder for that.
		if eClass, _ := eType.(EClass); eClass != nil {
			dyn := NewDynamicEObjectImpl()
			dyn.SetEClass(eClass)
			eProxy = dyn
		}
	}
	if eProxy != nil {
		// Make the proxy an INTRA-DOCUMENT reference — the current resource's URI with
		// the unresolved id as fragment (#id) — not a bare-name URI. A bare name would
		// be taken for a resource URI by EResolveProxy/ResolveAll, which then creates
		// and loads a phantom file named after the id: thousands of failing file opens
		// during a full resolve, plus an empty resource leaked into the set that later
		// breaks SaveAs ("Unable to find codec"). As a fragment of the already-loaded
		// resource, resolution merely looks the id up there (nil for an unresolved
		// name) — no I/O, no leak. An external name resolver reads the id back from
		// EProxyURI().Fragment() (the same form a same-document href proxy uses).
		eProxy.(EObjectInternal).ESetProxyURI(NewURIBuilder(l.resource.GetURI()).SetFragment(id).URI())
	}
	return eProxy
}

func (l *XMLDecoder) recordSchemaLocations(eObject EObject) {
	if l.extendedMetaData != nil && eObject != nil {
		eClass := eObject.EClass()
		if xmlnsPrefixMapFeature := l.extendedMetaData.GetXMLNSPrefixMapFeature(eClass); xmlnsPrefixMapFeature != nil {
			m := eObject.EGet(xmlnsPrefixMapFeature).(EMap)
			for prefix, nsURI := range l.prefixesToURI {
				m.Put(prefix, nsURI)
			}
		}
	}
}

func (l *XMLDecoder) getFeature(eObject EObject, _, name string) EStructuralFeature {
	eClass := eObject.EClass()
	eFeature := eClass.GetEStructuralFeatureFromName(name)
	if eFeature == nil && l.extendedMetaData != nil {
		features := eClass.GetEAllStructuralFeatures()
		for it := features.Iterator(); it.HasNext(); {
			feature := it.Next().(EStructuralFeature)
			if name == l.extendedMetaData.GetName(feature) {
				return feature
			}
		}
	}
	return eFeature
}

func (l *XMLDecoder) getType(ePackage EPackage, name string) EClassifier {
	if l.extendedMetaData != nil {
		return l.extendedMetaData.GetType(ePackage, name)
	}
	return ePackage.GetEClassifier(name)
}

func (l *XMLDecoder) handleUnknownFeature(name string) {
	l.error(NewEDiagnosticImpl("Feature "+name+" not found", l.resource.GetURI().String(), int(l.decoder.InputOffset()), 0))
}

func (l *XMLDecoder) handleUnknownPackage(name, space string) {
	l.error(NewEDiagnosticImpl("Package {'"+name+"'='"+space+"'} not found", l.resource.GetURI().String(), int(l.decoder.InputOffset()), 0))
}

func (l *XMLDecoder) error(diagnostic EDiagnostic) {
	l.errorFn(diagnostic)
}

func (l *XMLDecoder) text(data string) {
	if l.textBuilder != nil {
		l.textBuilder.WriteString(data)
	}
}

func (l *XMLDecoder) comment(comment string) {
}

func (l *XMLDecoder) processingInstruction(procInst xml.ProcInst) {
	if procInst.Target == "xml" {
		content := string(procInst.Inst)
		if ver := l.procInst("version", content); ver != "" {
			l.xmlVersion = ver
		}
		if encoding := l.procInst("encoding", content); encoding != "" {
			l.encoding = encoding
		}
	}
}

func (l *XMLDecoder) procInst(param, s string) string {
	// TODO: this parsing is somewhat lame and not exact.
	// It works for all actual cases, though.
	param = param + "="
	idx := strings.Index(s, param)
	if idx == -1 {
		return ""
	}
	v := s[idx+len(param):]
	if v == "" {
		return ""
	}
	if v[0] != '\'' && v[0] != '"' {
		return ""
	}
	idx = strings.IndexRune(v[1:], rune(v[0]))
	if idx == -1 {
		return ""
	}
	return v[1 : idx+1]
}

func (l *XMLDecoder) directive(directive string) {
}
