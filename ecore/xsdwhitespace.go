// *****************************************************************************
// Copyright(c) 2021 MASA Group
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// *****************************************************************************

package ecore

import "strings"

// xmlWhitespace is XML's S production — the characters XSD's whiteSpace facet acts on.
const xmlWhitespace = " \t\r\n"

// xsdCollapsesWhitespace reports whether a datatype's lexical space discards surrounding
// whitespace, i.e. whether XSD gives it whiteSpace="collapse".
//
// Every built-in XSD type carries that facet EXCEPT xs:string and the types derived from it,
// which carry whiteSpace="preserve" — so ` 10` is a perfectly valid xs:decimal denoting 10,
// while ` foo ` is an xs:string whose spaces are its own. Judged on the datatype's Go instance
// type rather than on an XSD annotation, because Ecore's own EDouble/EInt carry no XSD
// annotation at all and a generated per-package datatype (T_PositiveInteger, T_ZeroToOneDouble)
// carries a baseType detail this package cannot be made to depend on.
//
// An enumeration collapses too: its lexical space is a set of NMTOKENs, and a literal that
// arrived padded would otherwise be looked up with its padding and fail.
func xsdCollapsesWhitespace(eDataType EDataType) bool {
	if _, isEnum := eDataType.(EEnum); isEnum {
		return true
	}
	switch getInstanceTypeName(eDataType) {
	case "float64", "java.lang.Double", "double",
		"float32", "java.lang.Float", "float",
		"int", "java.lang.Integer",
		"uint64", "com.google.common.primitives.UnsignedLong",
		"int64", "java.lang.Long", "java.math.BigInteger", "long",
		"int32", "int16", "java.lang.Short", "short", "int8",
		"uint", "uint32", "uint16", "uint8",
		"byte", "java.lang.Byte",
		"bool", "java.lang.Boolean", "boolean",
		"java.math.BigDecimal",
		"time.Time", "*time.Time", "java.util.Date":
		return true
	}
	return false
}

// xsdCollapse applies that facet to one literal on its way into a factory.
//
// Without it a whitespace-padded number is not merely mis-parsed, it is silently zeroed: every
// createE<N>FromString in this package discards the strconv error and returns the zero value, so
// `y1=" 10"` loads as 0 and is written back as 0 — a value the document stated, replaced by one
// it did not, with nothing on stderr. Readers that own these files do collapse it: SWORD's ADN
// takes <line> coordinates through Xerces' XSD-typed conversion (xeumeuleu data.hpp to_float ->
// XMLFloat) against a schema that types them xs:decimal, and its SVG text reader parses x/y with
// a std::stringstream, which skips leading whitespace by default (svgl/Text.cpp).
//
// Only the lexical form is touched. A collapsing type has no whitespace in its value space, so
// this can never change what a document means — it can only stop the value being lost.
func xsdCollapse(eDataType EDataType, literalValue string) string {
	if !xsdCollapsesWhitespace(eDataType) {
		return literalValue
	}
	return strings.Trim(literalValue, xmlWhitespace)
}
