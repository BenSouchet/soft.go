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
	"fmt"
	"io"
	"sort"
	"strings"

	"golang.org/x/net/html"
)

type xmlSaveFeatureKind int

const (
	xsfkTransient xmlSaveFeatureKind = iota
	xsfkDataTypeSingle
	xsfkDataTypeElementSingle
	xsfkDataTypeContentSingle
	xsfkDataTypeSingleNillable
	xsfkDataTypeMany
	xsfkObjectContainSingle
	xsfkObjectContainMany
	xsfkObjectHrefSingle
	xsfkObjectHrefMany
	xsfkObjectContainSingleUnsettable
	xsfkObjectContainManyUnsettable
	xsfkObjectHrefSingleUnsettable
	xsfkObjectHrefManyUnsettable
	xsfkObjectElementSingle
	xsfkObjectElementSingleUnsettable
	xsfkObjectElementMany
	xsfkObjectElementIDRefSingle
	xsfkObjectElementIDRefSingleUnsettable
	xsfkObjectElementIDRefMany
	xsfkAttributeFeatureMap
	xsfkElementFeatureMap
	xsfkObjectAttributeSingle
	xsfkObjectAttributeMany
	xsfkObjectAttributeIDRefSingle
	xsfkObjectAttributeIDRefMany
	xsfkDataTypeAttributeMany
)

type xmlEncoderInternal interface {
	saveNamespaces()
}

type XMLEncoder struct {
	interfaces       any
	w                io.Writer
	resource         EResource
	roots            EList
	prefixesToURI    map[string]string
	uriToPrefixes    map[string][]string
	packages         map[EPackage]string
	featureKinds     map[EStructuralFeature]xmlSaveFeatureKind
	extendedMetaData *ExtendedMetaData
	str              *xmlString
	errorFn          func(diagnostic EDiagnostic)
	idAttributeName  string
	xmlVersion       string
	encoding         string
	keepDefaults     bool
	keepRequired     bool // write a required (lowerBound>=1) attribute even at the type default (dense form)
	denseDefaultField func(EObject, EStructuralFeature) bool // write this optional attribute even at the type default (caller-chosen, e.g. author-declared create-default fields); only with keepRequired
	// authoring (no-namespace) output controls, all opt-in via encode options:
	suppressDefaultNS    bool                                         // omit the default xmlns on the root
	noNSSchemaLocation   string                                       // emit xsi:noNamespaceSchemaLocation
	featureValueProvider func(EObject, EStructuralFeature) string     // kind="attribute" ref → verbatim value (name) instead of href/id-ref
}

func NewXMLEncoder(resource EResource, w io.Writer, options map[string]any) *XMLEncoder {
	s := new(XMLEncoder)
	s.interfaces = s
	s.resource = resource
	s.w = w
	s.xmlVersion = "1.0"
	s.encoding = "UTF-8"
	s.str = newXmlString()
	s.packages = make(map[EPackage]string)
	s.uriToPrefixes = make(map[string][]string)
	s.prefixesToURI = make(map[string]string)
	s.featureKinds = make(map[EStructuralFeature]xmlSaveFeatureKind)
	if options != nil {
		s.idAttributeName, _ = options[XML_OPTION_ID_ATTRIBUTE_NAME].(string)
		s.roots, _ = options[XML_OPTION_ROOT_OBJECTS].(EList)
		if extendedMetaData := options[XML_OPTION_EXTENDED_META_DATA]; extendedMetaData != nil {
			s.extendedMetaData = extendedMetaData.(*ExtendedMetaData)
		}
		s.suppressDefaultNS, _ = options[XML_OPTION_SUPPRESS_DEFAULT_NS].(bool)
		s.noNSSchemaLocation, _ = options[XML_OPTION_NO_NAMESPACE_SCHEMA_LOCATION].(string)
		s.featureValueProvider, _ = options[XML_OPTION_FEATURE_VALUE_PROVIDER].(func(EObject, EStructuralFeature) string)
		s.keepRequired, _ = options[XML_OPTION_KEEP_REQUIRED_DEFAULTS].(bool)
		s.denseDefaultField, _ = options[XML_OPTION_DENSE_DEFAULT_FIELDS].(func(EObject, EStructuralFeature) bool)
	}
	if s.extendedMetaData == nil {
		s.extendedMetaData = NewExtendedMetaData()
	}
	return s
}

func (s *XMLEncoder) SetEncoding(encoding string) {
	s.encoding = encoding
}

func (s *XMLEncoder) SetXMLVersion(xmlVersion string) {
	s.xmlVersion = xmlVersion
}

func (s *XMLEncoder) EncodeResource() {
	s.errorFn = func(diagnostic EDiagnostic) {
		s.resource.GetErrors().Add(diagnostic)
	}
	contents := s.roots
	if contents == nil {
		contents = s.resource.GetContents()
	}
	if contents.Empty() {
		return
	}
	s.encodeTopObject(contents.Get(0).(EObject))
}

func (s *XMLEncoder) EncodeObject(eObject EObject) (err error) {
	s.errorFn = func(diagnostic EDiagnostic) {
		if err == nil {
			err = diagnostic
		}
	}
	s.encodeTopObject(eObject)
	return
}

func (s *XMLEncoder) encodeTopObject(eObject EObject) {
	s.saveHeader()

	// initialize prefixes if any in top
	if s.extendedMetaData != nil {
		eClass := eObject.EClass()
		if ePrefixMapFeature := s.extendedMetaData.GetXMLNSPrefixMapFeature(eClass); ePrefixMapFeature != nil {
			m := eObject.EGet(ePrefixMapFeature).(EMap)
			s.setPrefixToNamespace(m)
		}
	}

	s.saveTopObject(eObject)

	// namespaces
	s.str.resetToFirstElementMark()
	s.interfaces.(xmlEncoderInternal).saveNamespaces()

	// write result
	if err := s.str.write(s.w); err != nil {
		s.error(NewEDiagnosticImpl(err.Error(), s.resource.GetURI().String(), 0, 0))
	}
}

func (s *XMLEncoder) saveHeader() {
	s.str.add(fmt.Sprintf("<?xml version=\"%v\" encoding=\"%v\"?>", s.xmlVersion, s.encoding))
	s.str.addLine()
}

func (s *XMLEncoder) saveTopObject(eObject EObject) {
	eClass := eObject.EClass()
	if s.extendedMetaData == nil || s.extendedMetaData.GetDocumentRoot(eClass.GetEPackage()) != eClass {
		var name string
		if rootFeature := s.getRootFeature(eClass); rootFeature != nil {
			name = s.getFeatureQName(rootFeature)
		} else {
			name = s.getClassQName(eClass)
		}
		s.str.startElement(name)
		s.saveElementID(eObject)
		s.saveFeatures(eObject, false)
	} else {
		s.str.startElement("")
		s.saveFeatures(eObject, false)
	}
}

func (s *XMLEncoder) getRootFeature(eClassifier EClassifier) EStructuralFeature {
	if s.extendedMetaData != nil {
		for eClassifier != nil {
			if eClass := s.extendedMetaData.GetDocumentRoot(eClassifier.GetEPackage()); eClass != nil {
				for it := eClass.GetEStructuralFeatures().Iterator(); it.HasNext(); {
					eFeature := it.Next().(EStructuralFeature)
					if eFeature.GetEType() == eClassifier && eFeature.IsChangeable() {
						return eFeature
					}
				}
			}
			if eClass, _ := eClassifier.(EClass); eClass != nil {
				eSuperTypes := eClass.GetESuperTypes()
				if eSuperTypes.Empty() {
					eClassifier = nil
				} else {
					eClassifier = eSuperTypes.Get(0).(EClass)
				}
			} else {
				eClassifier = nil
			}
		}
	}
	return nil
}

func (s *XMLEncoder) saveNamespaces() {
	prefixes := make([]string, 0, len(s.prefixesToURI))
	for prefix := range s.prefixesToURI {
		prefixes = append(prefixes, prefix)
	}
	sort.Strings(prefixes)
	for _, prefix := range prefixes {
		// No-namespace (authoring) output: omit the default-namespace declaration so
		// the root and its unqualified-package elements stay bare (<physical>, not
		// xmlns="…"). Prefixed namespaces (e.g. xsi) are still declared.
		if s.suppressDefaultNS && prefix == "" {
			continue
		}
		attribute := "xmlns"
		if len(prefix) > 0 {
			attribute += ":" + prefix
		}
		s.str.addAttribute(attribute, s.prefixesToURI[prefix])
	}
	// Authoring form declares the schema via xsi:noNamespaceSchemaLocation rather
	// than a default namespace; emit it (declaring xmlns:xsi if nothing else did).
	if s.noNSSchemaLocation != "" {
		if _, ok := s.prefixesToURI["xsi"]; !ok {
			s.str.addAttribute("xmlns:xsi", "http://www.w3.org/2001/XMLSchema-instance")
		}
		s.str.addAttribute("xsi:noNamespaceSchemaLocation", s.noNSSchemaLocation)
	}
}

func (s *XMLEncoder) saveElementID(eObject EObject) {
	if idManager := s.resource.GetObjectIDManager(); len(s.idAttributeName) > 0 && idManager != nil {
		if id := idManager.GetID(eObject); id != nil {
			s.str.addAttribute(s.idAttributeName, fmt.Sprintf("%v", id))
		}
	}
}

func (s *XMLEncoder) saveFeatures(eObject EObject, attributesOnly bool) bool {
	eClass := eObject.EClass()
	eAllFeatures := eClass.GetEAllStructuralFeatures()
	var elementFeatures []int
	elementCount := 0
	i := 0
	for it := eAllFeatures.Iterator(); it.HasNext(); i++ {
		// current feature
		eFeature := it.Next().(EStructuralFeature)
		// compute feature kind
		kind, ok := s.featureKinds[eFeature]
		if !ok {
			kind = s.getSaveFeatureKind(eFeature)
			s.featureKinds[eFeature] = kind
		}

		if kind != xsfkTransient && s.shouldSaveFeature(eObject, eFeature) {
			switch kind {
			case xsfkDataTypeSingle:
				s.saveDataTypeSingle(eObject, eFeature)
				continue
			case xsfkObjectAttributeSingle:
				// Authoring write-back: emit the target's name (provider value) verbatim
				// as the attribute value instead of an href/id-ref (e.g. protection="Aeronef").
				if s.saveProvidedFeatureValue(eObject, eFeature) {
					continue
				}
				// kind="attribute" reference written as an attribute: a same-resource
				// (or detached) target uses an intra-doc IDREF ("#frag"), a
				// cross-resource target uses a full href ("File.xml#frag") — both as
				// the attribute value. Mirrors the xsfkObjectHref dispatch so a
				// boundary/detached ref (e.g. subset extraction) is an IDREF, never a
				// dangling-HREF error.
				switch s.getSaveResourceKindSingle(eObject, eFeature) {
				case cross:
					s.saveEObjectSingle(eObject, eFeature)
				case same:
					s.saveIDRefSingle(eObject, eFeature)
				}
				continue
			case xsfkObjectAttributeMany:
				if s.saveProvidedFeatureValue(eObject, eFeature) {
					continue
				}
				switch s.getSaveResourceKindMany(eObject, eFeature) {
				case cross:
					s.saveEObjectMany(eObject, eFeature)
				case same:
					s.saveIDRefMany(eObject, eFeature)
				}
				continue
			case xsfkDataTypeSingleNillable:
				if !s.isNil(eObject, eFeature) {
					s.saveDataTypeSingle(eObject, eFeature)
					continue
				}
			case xsfkObjectContainManyUnsettable:
				fallthrough
			case xsfkDataTypeMany:
				if s.isEmpty(eObject, eFeature) {
					s.saveManyEmpty(eObject, eFeature)
					continue
				}
			case xsfkObjectContainSingleUnsettable:
			case xsfkObjectContainSingle:
			case xsfkObjectContainMany:
			case xsfkDataTypeContentSingle:
			case xsfkDataTypeElementSingle:
			case xsfkObjectHrefSingleUnsettable:
				if !s.isNil(eObject, eFeature) {
					// Authoring: a non-containment reference modeled without kind="attribute"
					// still references its target by name in authoring form, so emit the
					// provider's name as an attribute instead of an href element.
					if s.saveProvidedFeatureValue(eObject, eFeature) {
						continue
					}
					switch s.getSaveResourceKindSingle(eObject, eFeature) {
					case cross:
					case same:
						s.saveIDRefSingle(eObject, eFeature)
						continue
					default:
						continue
					}
				}
			case xsfkObjectHrefSingle:
				if s.saveProvidedFeatureValue(eObject, eFeature) {
					continue
				}
				switch s.getSaveResourceKindSingle(eObject, eFeature) {
				case cross:
				case same:
					s.saveIDRefSingle(eObject, eFeature)
					continue
				default:
					continue
				}
			case xsfkObjectHrefManyUnsettable:
				if s.isEmpty(eObject, eFeature) {
					s.saveManyEmpty(eObject, eFeature)
					continue
				} else {
					if s.saveProvidedFeatureValue(eObject, eFeature) {
						continue
					}
					switch s.getSaveResourceKindMany(eObject, eFeature) {
					case cross:
					case same:
						s.saveIDRefMany(eObject, eFeature)
						continue
					default:
						continue
					}
				}

			case xsfkObjectHrefMany:
				if s.saveProvidedFeatureValue(eObject, eFeature) {
					continue
				}
				switch s.getSaveResourceKindMany(eObject, eFeature) {
				case cross:
				case same:
					s.saveIDRefMany(eObject, eFeature)
					continue
				default:
					continue
				}
			default:
				continue
			}
			if attributesOnly {
				continue
			}
			if elementFeatures == nil {
				elementFeatures = make([]int, eAllFeatures.Size())
			}
			elementFeatures[elementCount] = i
			elementCount++
		}
	}
	if elementFeatures == nil {
		s.str.endEmptyElement()
		return false
	}
	for i := 0; i < elementCount; i++ {
		eFeature := eAllFeatures.Get(elementFeatures[i]).(EStructuralFeature)
		kind := s.featureKinds[eFeature]
		switch kind {
		case xsfkDataTypeSingleNillable:
			s.saveNil(eObject, eFeature)
		case xsfkDataTypeMany:
			s.saveDataTypeMany(eObject, eFeature)
		case xsfkObjectContainSingleUnsettable:
			if s.isNil(eObject, eFeature) {
				s.saveNil(eObject, eFeature)
			} else {
				s.saveContainedSingle(eObject, eFeature)
			}
		case xsfkObjectContainSingle:
			s.saveContainedSingle(eObject, eFeature)
		case xsfkDataTypeContentSingle:
			s.saveDataTypeContent(eObject, eFeature)
		case xsfkDataTypeElementSingle:
			s.saveDataTypeElementSingle(eObject, eFeature)
		case xsfkObjectContainManyUnsettable:
			fallthrough
		case xsfkObjectContainMany:
			s.saveContainedMany(eObject, eFeature)
		case xsfkObjectHrefSingleUnsettable:
			if s.isNil(eObject, eFeature) {
				s.saveNil(eObject, eFeature)
			} else {
				s.saveHRefSingle(eObject, eFeature)
			}
		case xsfkObjectHrefSingle:
			s.saveHRefSingle(eObject, eFeature)
		case xsfkObjectHrefManyUnsettable:
			fallthrough
		case xsfkObjectHrefMany:
			s.saveHRefMany(eObject, eFeature)
		}
	}

	s.str.endElement()
	return true
}

func (s *XMLEncoder) saveDataTypeSingle(eObject EObject, eFeature EStructuralFeature) {
	val := eObject.EGetResolve(eFeature, false)
	str, ok := s.getDataType(val, eFeature, true)
	if ok {
		s.str.addAttribute(s.getFeatureQName(eFeature), html.EscapeString(str))
	}
}

// saveDataTypeElementSingle writes a scalar attribute whose ExtendedMetaData kind
// is "element" as its own child element <name>value</name> (e.g.
// <equipment-category>ANT</equipment-category>) — the EMF encoding for a datatype
// feature declared as a child element. Counterpart of saveDataTypeSingle (the
// attribute form) and saveDataTypeMany (repeated child elements). Escapes the
// value like saveDataTypeSingle so a value with <, > or & stays valid XML.
func (s *XMLEncoder) saveDataTypeElementSingle(eObject EObject, eFeature EStructuralFeature) {
	val := eObject.EGetResolve(eFeature, false)
	str, ok := s.getDataType(val, eFeature, true)
	if ok {
		s.str.addContent(s.getFeatureQName(eFeature), html.EscapeString(str))
	}
}

// saveDataTypeContent writes a simple-content feature as the element's inline
// text body (XSD simpleContent), e.g. <comments>en g/m3</comments>, rather than
// an attribute. Counterpart of saveDataTypeSingle for kind="simple" features.
func (s *XMLEncoder) saveDataTypeContent(eObject EObject, eFeature EStructuralFeature) {
	val := eObject.EGetResolve(eFeature, false)
	str, ok := s.getDataType(val, eFeature, true)
	if ok {
		s.str.addContentValue(html.EscapeString(str))
	}
}

func (s *XMLEncoder) saveDataTypeMany(eObject EObject, eFeature EStructuralFeature) {
	l := eObject.EGetResolve(eFeature, false).(EList)
	d := eFeature.GetEType().(EDataType)
	p := d.GetEPackage()
	f := p.GetEFactoryInstance()
	name := s.getFeatureQName(eFeature)
	for it := l.Iterator(); it.HasNext(); {
		value := it.Next()
		if value == nil {
			s.str.startElement(name)
			s.str.addAttribute("xsi:nil", "true")
			s.str.endEmptyElement()
			s.uriToPrefixes[xsiURI] = []string{xsiNS}
			s.prefixesToURI[xsiNS] = xsiURI
		} else {
			str := f.ConvertToString(d, value)
			s.str.addContent(name, str)
		}
	}
}

func (s *XMLEncoder) saveManyEmpty(_ EObject, eFeature EStructuralFeature) {
	s.str.addAttribute(s.getFeatureQName(eFeature), "")
}

// saveProvidedFeatureValue, when a featureValueProvider is configured, writes the
// provider's string for this kind="attribute" reference verbatim as the attribute
// value (the authoring reference key — a target's name — for a single ref, or the
// caller-joined keys for a many ref) and returns true. A nil provider or an empty
// return falls through to the default href / id-ref encoding.
func (s *XMLEncoder) saveProvidedFeatureValue(eObject EObject, eFeature EStructuralFeature) bool {
	if s.featureValueProvider == nil {
		return false
	}
	v := s.featureValueProvider(eObject, eFeature)
	if v == "" {
		return false
	}
	// Escape like every other attribute value (addAttribute writes raw): a reference
	// name can contain & < > " (e.g. "R&D"); writing it raw produces invalid XML that
	// truncates the document on reload.
	s.str.addAttribute(s.getFeatureQName(eFeature), html.EscapeString(v))
	return true
}

func (s *XMLEncoder) saveEObjectSingle(eObject EObject, eFeature EStructuralFeature) {
	value, _ := eObject.EGetResolve(eFeature, false).(EObject)
	if value != nil {
		id := s.getHRef(value)
		s.str.addAttribute(s.getFeatureQName(eFeature), id)
	}
}

func (s *XMLEncoder) saveEObjectMany(eObject EObject, eFeature EStructuralFeature) {
	l := eObject.EGetResolve(eFeature, false).(EList)
	failure := false
	var buffer strings.Builder
	for it := l.Iterator(); ; {
		value, _ := it.Next().(EObject)
		if value != nil {
			id := s.getHRef(value)
			if id == "" {
				failure = true
				if !it.HasNext() {
					break
				}
			} else {
				buffer.WriteString(id)
				if it.HasNext() {
					buffer.WriteString(" ")
				} else {
					break
				}
			}
		}
	}
	if !failure && buffer.Len() > 0 {
		s.str.addAttribute(s.getFeatureQName(eFeature), buffer.String())
	}
}

func (s *XMLEncoder) saveNil(_ EObject, eFeature EStructuralFeature) {
	s.str.addNil(s.getFeatureQName(eFeature))
}

func (s *XMLEncoder) saveContainedSingle(eObject EObject, eFeature EStructuralFeature) {
	value, _ := eObject.EGetResolve(eFeature, false).(EObjectInternal)
	if value != nil {
		s.saveEObjectInternal(value, eFeature)
	}
}

func (s *XMLEncoder) saveContainedMany(eObject EObject, eFeature EStructuralFeature) {
	l := eObject.EGetResolve(eFeature, false).(EList)
	for it := l.Iterator(); it.HasNext(); {
		value, _ := it.Next().(EObjectInternal)
		if value != nil {
			s.saveEObjectInternal(value, eFeature)
		}
	}
}

func (s *XMLEncoder) saveEObjectInternal(o EObjectInternal, f EStructuralFeature) {
	if o.EInternalResource() != nil || o.EIsProxy() {
		s.saveHRef(o, f)
	} else {
		s.saveEObject(o, f)
	}
}

func (s *XMLEncoder) saveEObject(o EObject, f EStructuralFeature) {
	s.str.startElement(s.getFeatureQName(f))
	eClass := o.EClass()
	eType := f.GetEType()
	if eType != eClass && eType != GetPackage().GetEObject() {
		s.saveTypeAttribute(eClass)
	}
	s.saveElementID(o)
	s.saveFeatures(o, false)
}

func (s *XMLEncoder) saveTypeAttribute(eClass EClass) {
	s.str.addAttribute("xsi:type", s.getClassQName(eClass))
	s.uriToPrefixes[xsiURI] = []string{xsiNS}
	s.prefixesToURI[xsiNS] = xsiURI
}

func (s *XMLEncoder) saveHRefSingle(eObject EObject, eFeature EStructuralFeature) {
	value, _ := eObject.EGetResolve(eFeature, false).(EObject)
	if value != nil {
		s.saveHRef(value, eFeature)
	}
}

func (s *XMLEncoder) saveHRefMany(eObject EObject, eFeature EStructuralFeature) {
	l := eObject.EGetResolve(eFeature, false).(EList)
	for it := l.Iterator(); it.HasNext(); {
		value, _ := it.Next().(EObject)
		if value != nil {
			s.saveHRef(value, eFeature)
		}
	}
}

func (s *XMLEncoder) saveHRef(eObject EObject, eFeature EStructuralFeature) {
	href := s.getHRef(eObject)
	if href != "" {
		s.str.startElement(s.getFeatureQName(eFeature))
		eClass := eObject.EClass()
		eType, _ := eFeature.GetEType().(EClass)
		if eType != eClass && eType != nil && eType.IsAbstract() {
			s.saveTypeAttribute(eClass)
		}
		s.str.addAttribute("href", href)
		s.str.endEmptyElement()
	}
}

func (s *XMLEncoder) saveIDRefSingle(eObject EObject, eFeature EStructuralFeature) {
	value, _ := eObject.EGetResolve(eFeature, false).(EObject)
	if value != nil {
		id := s.getIDRef(value)
		if id != "" {
			s.str.addAttribute(s.getFeatureQName(eFeature), id)
		}
	}
}

func (s *XMLEncoder) saveIDRefMany(eObject EObject, eFeature EStructuralFeature) {
	l := eObject.EGetResolve(eFeature, false).(EList)
	failure := false
	var buffer strings.Builder
	for it := l.Iterator(); ; {
		value, _ := it.Next().(EObject)
		if value != nil {
			id := s.getIDRef(value)
			if id == "" {
				failure = true
				if !it.HasNext() {
					break
				}
			} else {
				buffer.WriteString(id)
				if it.HasNext() {
					buffer.WriteString(" ")
				} else {
					break
				}
			}
		}
	}
	if !failure && buffer.Len() > 0 {
		s.str.addAttribute(s.getFeatureQName(eFeature), buffer.String())
	}
}

func (s *XMLEncoder) isNil(eObject EObject, eFeature EStructuralFeature) bool {
	return eObject.EGetResolve(eFeature, false) == nil
}

func (s *XMLEncoder) isEmpty(eObject EObject, eFeature EStructuralFeature) bool {
	return eObject.EGetResolve(eFeature, false).(EList).Empty()
}

func (s *XMLEncoder) shouldSaveFeature(o EObject, f EStructuralFeature) bool {
	if o.EIsSet(f) || (s.keepDefaults && f.GetDefaultValueLiteral() != "") {
		return true
	}
	// Dense form: a required attribute is written even at the type default (int 0,
	// bool false, ...) — what ADN / an XSD-validating writer emits — so a created
	// entity carries its required scalar defaults explicitly. An optional attribute the
	// caller flags (denseDefaultField) is likewise written even at the default — used
	// for the schema's author-declared create-default fields ADN's ctor always emits
	// (soft.go has no isSet flag for value-typed unsettable scalars, so EIsSet can't see
	// a set-to-default). Attributes only: a required reference left unset must not be
	// forced to an empty href.
	if s.keepRequired {
		if _, isAttr := f.(EAttribute); isAttr &&
			(f.GetLowerBound() >= 1 || (s.denseDefaultField != nil && s.denseDefaultField(o, f))) {
			return true
		}
	}
	return false
}

func (s *XMLEncoder) getSaveFeatureKind(f EStructuralFeature) xmlSaveFeatureKind {
	if f.IsTransient() {
		return xsfkTransient
	}

	isMany := f.IsMany()
	isUnsettable := f.IsUnsettable()

	if r, _ := f.(EReference); r != nil {
		if r.IsContainment() {
			if isMany {
				if isUnsettable {
					return xsfkObjectContainManyUnsettable
				} else {
					return xsfkObjectContainMany
				}
			} else {
				if isUnsettable {
					return xsfkObjectContainSingleUnsettable
				} else {
					return xsfkObjectContainSingle
				}
			}
		}
		opposite := r.GetEOpposite()
		if opposite != nil && opposite.IsContainment() {
			return xsfkTransient
		}
		// A reference whose ExtendedMetaData declares kind="attribute" is an
		// IDREF-style attribute in the XSD (e.g. `protection="Protections.xml#…"`),
		// not a nested href element. The decoder already reads it from the
		// attribute; serialize it back the same way (via saveEObject*, which emits
		// the href as the attribute value) so it round-trips. Without this, such a
		// cross-resource reference is written as a `<protection href=…/>` child the
		// decoder cannot read back -> the reference is lost on reload.
		if s.extendedMetaData != nil && s.extendedMetaData.GetFeatureKind(f) == "attribute" {
			if isMany {
				return xsfkObjectAttributeMany
			}
			return xsfkObjectAttributeSingle
		}
		if isMany {
			if isUnsettable {
				return xsfkObjectHrefManyUnsettable
			} else {
				return xsfkObjectHrefMany
			}
		} else {
			if isUnsettable {
				return xsfkObjectHrefSingleUnsettable
			} else {
				return xsfkObjectHrefSingle
			}
		}
	} else {
		// Attribute
		d := f.GetEType().(EDataType)
		if !d.IsSerializable() {
			return xsfkTransient
		}
		// A feature whose ExtendedMetaData declares kind="simple" is the simple
		// content of a complex type — it serializes as the element's inline text
		// (e.g. <comments>en g/m3</comments>), not as an attribute. The decoder
		// already reads simple content from the element text; emit it the same way
		// so it round-trips (otherwise it is written as an attribute named after
		// the EMD name, often the synthetic ":0", producing invalid XML).
		if !isMany && s.extendedMetaData != nil && s.extendedMetaData.GetFeatureKind(f) == "simple" {
			return xsfkDataTypeContentSingle
		}
		// A scalar attribute whose ExtendedMetaData declares kind="element" is a
		// child element in the XSD (<equipment-category>ANT</equipment-category>),
		// NOT an attribute. The decoder reads it from the child element; without
		// this it falls through to xsfkDataTypeSingle and is (wrongly) written back
		// as an attribute — a round-trip form break vs the source/reference output.
		if !isMany && s.extendedMetaData != nil && s.extendedMetaData.GetFeatureKind(f) == "element" {
			return xsfkDataTypeElementSingle
		}
		if isMany {
			return xsfkDataTypeMany
		}
		if isUnsettable {
			return xsfkDataTypeSingleNillable
		}
		return xsfkDataTypeSingle

	}

}

const (
	skip  = iota
	same  = iota
	cross = iota
)

func (s *XMLEncoder) getSaveResourceKindSingle(eObject EObject, eFeature EStructuralFeature) int {
	value, _ := eObject.EGetResolve(eFeature, false).(EObjectInternal)
	if value == nil {
		return skip
	} else if value.EIsProxy() {
		return cross
	} else {
		resource := value.EResource()
		if resource == s.resource || resource == nil {
			return same
		}
		return cross
	}
}

func (s *XMLEncoder) getSaveResourceKindMany(eObject EObject, eFeature EStructuralFeature) int {
	list, _ := eObject.EGetResolve(eFeature, false).(EList)
	if list == nil || list.Empty() {
		return skip
	}
	for it := list.Iterator(); it.HasNext(); {
		o, _ := it.Next().(EObjectInternal)
		if o == nil {
			return skip
		} else if o.EIsProxy() {
			return cross
		} else {
			r := o.EResource()
			if r != nil && r != s.resource {
				return cross
			}
		}

	}
	return same
}

func (s *XMLEncoder) getClassQName(eClass EClass) string {
	return s.getElementQName(eClass.GetEPackage(), s.getXmlName(eClass), false)
}

func (s *XMLEncoder) getFeatureQName(eFeature EStructuralFeature) string {
	if s.extendedMetaData != nil {
		name := s.extendedMetaData.GetName(eFeature)
		namespace := s.extendedMetaData.GetNamespace(eFeature)
		ePackage := s.getPackageForSpace(namespace)
		if ePackage != nil {
			return s.getElementQName(ePackage, name, false)
		} else {
			return name
		}
	} else {
		return eFeature.GetName()
	}
}

func (s *XMLEncoder) getElementQName(ePackage EPackage, name string, mustHavePrefix bool) string {
	nsPrefix := s.getPrefix(ePackage, mustHavePrefix)
	if nsPrefix == "" {
		return name
	} else if len(name) == 0 {
		return nsPrefix
	} else {
		return nsPrefix + ":" + name
	}
}

func (s *XMLEncoder) getXmlName(eElement ENamedElement) string {
	if s.extendedMetaData != nil {
		return s.extendedMetaData.GetName(eElement)
	}
	return eElement.GetName()
}

func (s *XMLEncoder) getPrefix(ePackage EPackage, mustHavePrefix bool) string {
	nsPrefix, ok := s.packages[ePackage]
	if !ok {
		nsURI := ePackage.GetNsURI()
		found := false
		prefixes := s.uriToPrefixes[nsURI]
		for _, prefix := range prefixes {
			nsPrefix = prefix
			if !mustHavePrefix || len(nsPrefix) > 0 {
				found = true
				break
			}
		}
		if !found {
			nsPrefix = ePackage.GetNsPrefix()
			if len(nsPrefix) == 0 && mustHavePrefix {
				nsPrefix = "_"
			}

			if uri, exists := s.prefixesToURI[nsPrefix]; exists && uri != nsURI {
				index := 1
				for _, exists = s.prefixesToURI[nsPrefix+"_"+fmt.Sprintf("%d", index)]; exists; {
					index++
				}
				nsPrefix += "_" + fmt.Sprintf("%d", index)
			}
			s.prefixesToURI[nsPrefix] = nsURI
		}

		s.packages[ePackage] = nsPrefix
	}
	return nsPrefix
}

func (s *XMLEncoder) setPrefixToNamespace(prefixToNamespaceMap EMap) {
	for it := prefixToNamespaceMap.Iterator(); it.HasNext(); {
		entry := it.Next().(EMapEntry)
		prefix := entry.GetKey().(string)
		nsURI := entry.GetValue().(string)
		if ePackage := s.getPackageForSpace(nsURI); ePackage != nil {
			s.packages[ePackage] = prefix
		}
		s.prefixesToURI[prefix] = nsURI
		s.uriToPrefixes[nsURI] = append(s.uriToPrefixes[nsURI], prefix)
	}
}

func (s *XMLEncoder) getPackageForSpace(nsURI string) EPackage {
	packageRegistry := GetPackageRegistry()
	if s.resource.GetResourceSet() != nil {
		packageRegistry = s.resource.GetResourceSet().GetPackageRegistry()
	}
	return packageRegistry.GetPackage(nsURI)
}

func (s *XMLEncoder) getDataType(value any, f EStructuralFeature, _ bool) (string, bool) {
	if value == nil {
		return "", false
	} else {
		d := f.GetEType().(EDataType)
		p := d.GetEPackage()
		f := p.GetEFactoryInstance()
		s := f.ConvertToString(d, value)
		return s, true
	}
}

func (s *XMLEncoder) getHRef(eObject EObject) string {
	eInternal, _ := eObject.(EObjectInternal)
	if eInternal != nil {
		objectURI := eInternal.EProxyURI()
		if objectURI == nil {
			eOtherResource := eObject.EResource()
			if eOtherResource == nil {
				if s.resource != nil && s.resource.GetObjectIDManager() != nil && s.resource.GetObjectIDManager().GetID(eObject) != nil {
					objectURI = s.getResourceHRef(s.resource, eObject)
				} else {
					s.handleDanglingHREF(eObject)
					return ""
				}
			} else {
				objectURI = s.getResourceHRef(eOtherResource, eObject)
			}
		}
		objectURI = s.resource.GetURI().Relativize(objectURI)
		return objectURI.String()
	}
	return ""
}

func (s *XMLEncoder) getResourceHRef(resource EResource, object EObject) *URI {
	return NewURIBuilder(resource.GetURI()).SetFragment(resource.GetURIFragment(object)).URI()
}

func (s *XMLEncoder) getIDRef(eObject EObject) string {
	if s.resource == nil {
		return ""
	} else {
		return "#" + s.resource.GetURIFragment(eObject)
	}
}

func (s *XMLEncoder) handleDanglingHREF(eObject EObject) {
	s.error(NewEDiagnosticImpl(fmt.Sprintf("Object '%p' is not contained in a resource.", eObject), s.resource.GetURI().String(), 0, 0))
}

func (s *XMLEncoder) error(diagnostic EDiagnostic) {
	s.errorFn(diagnostic)
}
