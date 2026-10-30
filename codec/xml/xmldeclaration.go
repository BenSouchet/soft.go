// *****************************************************************************
// Copyright(c) 2021 MASA Group
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// *****************************************************************************

package xml

import (
	"strings"

	"github.com/masagroup/soft.go/ecore"
)

// The XML declaration of a document, <?xml version="1.0" encoding="UTF-8" standalone="no" ?>, and
// its document type declaration, <!DOCTYPE TS>, are not values of its model either: its writer
// chose them, and two writers of one format do not choose alike. Like the comments (XMLComments),
// the decoder keeps them on the resource as they were written, and the encoder writes them back
// when asked (OPTION_KEEP_XML_DECLARATION).

// declarationKey and doctypeKey are the keys of a document's XML declaration and document type
// declaration in its resource's codec data.
type (
	declarationKey struct{}
	doctypeKey     struct{}
)

// XMLDocumentType returns the document type declaration the document of resource was read with,
// <!DOCTYPE ...> as it was written, "" when it had none or was not read from XML.
func XMLDocumentType(resource ecore.EResource) string {
	carrier, _ := resource.(codecDataCarrier)
	if carrier == nil {
		return ""
	}
	doctype, _ := carrier.GetCodecData(doctypeKey{}).(string)
	return doctype
}

// SetXMLDocumentType gives resource the document type declaration its document is written with
// under OPTION_KEEP_XML_DECLARATION. A value that is not a document type declaration is ignored.
func SetXMLDocumentType(resource ecore.EResource, doctype string) {
	if !strings.HasPrefix(doctype, "<!DOCTYPE") || !strings.HasSuffix(doctype, ">") {
		return
	}
	if carrier, _ := resource.(codecDataCarrier); carrier != nil {
		carrier.SetCodecData(doctypeKey{}, doctype)
	}
}

// XMLDeclaration returns the XML declaration the document of resource was read with, as it was
// written, "" when it had none or was not read from XML.
func XMLDeclaration(resource ecore.EResource) string {
	carrier, _ := resource.(codecDataCarrier)
	if carrier == nil {
		return ""
	}
	declaration, _ := carrier.GetCodecData(declarationKey{}).(string)
	return declaration
}

// SetXMLDeclaration gives resource the XML declaration its document is written with under
// OPTION_KEEP_XML_DECLARATION: a document kept elsewhere than in its file keeps its declaration so.
// A value that is not an XML declaration is ignored.
func SetXMLDeclaration(resource ecore.EResource, declaration string) {
	if !strings.HasPrefix(declaration, "<?xml ") || !strings.HasSuffix(declaration, "?>") {
		return
	}
	if carrier, _ := resource.(codecDataCarrier); carrier != nil {
		carrier.SetCodecData(declarationKey{}, declaration)
	}
}

// minimalContentEscaper and minimalAttributeEscaper escape only what XML requires
// (OPTION_MINIMAL_ESCAPE): '&' and '<', '>' as the closing of "]]>" would need it, '"' in an
// attribute, which is written between double quotes, and the whitespaces a parser would normalize.
// An apostrophe stays an apostrophe.
var (
	minimalContentEscaper   = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\r", "&#13;")
	minimalAttributeEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;",
		"\r", "&#13;", "\n", "&#xA;", "\t", "&#x9;")
)
