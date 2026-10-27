// *****************************************************************************
// Copyright(c) 2021 MASA Group
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// *****************************************************************************

package ecore

import "io"

const (
	XML_OPTION_EXTENDED_META_DATA            = "EXTENDED_META_DATA"            // ExtendedMetaData pointer
	XML_OPTION_SUPPRESS_DOCUMENT_ROOT        = "SUPPRESS_DOCUMENT_ROOT"        // if true , suppress document root if found
	XML_OPTION_DEFERRED_REFERENCE_RESOLUTION = "DEFERRED_REFERENCE_RESOLUTION" // if true , defer id ref resolution
	XML_OPTION_DEFERRED_ROOT_ATTACHMENT      = "DEFERRED_ROOT_ATTACHMENT"      // if true , defer id ref resolution
	XML_OPTION_ID_ATTRIBUTE_NAME             = "ID_ATTRIBUTE_NAME"             // value of the id attribute
	XML_OPTION_ROOT_OBJECTS                  = "ROOT_OBJECTS"                  // list of root objects to save
	XML_OPTION_ID_REFERENCE_AS_PROXY         = "ID_REFERENCE_AS_PROXY"         // if true, an id reference that resolves to nothing in the document becomes a proxy carrying the raw id (for external resolution) instead of an unresolved-reference error
	XML_OPTION_SUPPRESS_DEFAULT_NS           = "SUPPRESS_DEFAULT_NS"           // encode: if true, omit the default-namespace xmlns declaration on the root (no-namespace output)
	XML_OPTION_NO_NAMESPACE_SCHEMA_LOCATION  = "NO_NAMESPACE_SCHEMA_LOCATION"  // encode: string; if set, emit xsi:noNamespaceSchemaLocation="<value>" on the root (declaring xmlns:xsi if needed)
	XML_OPTION_SCHEMA_LOCATION               = "SCHEMA_LOCATION"               // encode: string; if set, emit xsi:schemaLocation="<value>" on the root, pairs of a namespace and a location, instead of the ones its document root records
	XML_OPTION_FEATURE_VALUE_PROVIDER        = "FEATURE_VALUE_PROVIDER"        // encode: func(EObject, EStructuralFeature) string; for a kind="attribute" reference, a non-empty return is written verbatim as the attribute value instead of an href/id-ref (used to emit a target's name)
	// encode: serialize a required (lowerBound>=1) attribute even at the type default,
	// which is the dense form ADN and XSD-validating writers emit.
	XML_OPTION_KEEP_REQUIRED_DEFAULTS = "KEEP_REQUIRED_DEFAULTS"
	// encode: func(EObject, EStructuralFeature) bool. An attribute it returns true for is
	// serialized even at the type default, for caller-chosen optional fields. Only
	// consulted when KEEP_REQUIRED_DEFAULTS is set.
	XML_OPTION_DENSE_DEFAULT_FIELDS = "DENSE_DEFAULT_FIELDS"
	// encode: write back the XML comments the resource's document carried, each among the
	// siblings it stood between (see XMLComments). Off by default, because it is the only
	// option here that can add nodes to a document a caller did not ask to change: a decode
	// always collects the comments, and this decides whether a write puts them back.
	XML_OPTION_KEEP_COMMENTS = "KEEP_COMMENTS"
)

type XMLCodec struct {
}

func (d *XMLCodec) NewEncoder(resource EResource, w io.Writer, options map[string]any) EEncoder {
	return NewXMLEncoder(resource, w, options)
}
func (d *XMLCodec) NewDecoder(resource EResource, r io.Reader, options map[string]any) EDecoder {
	return NewXMLDecoder(resource, r, options)
}
