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

// The XML declaration of a document, <?xml version="1.0" encoding="UTF-8" standalone="no" ?>, is
// not a value of its model either: its writer chose it, and two writers of one format do not choose
// alike. Like the comments (XMLComments), the decoder keeps it on the resource as it was written,
// and the encoder writes it back when asked (OPTION_KEEP_XML_DECLARATION).

// declarationKey is the key of a document's XML declaration in its resource's codec data.
type declarationKey struct{}

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
