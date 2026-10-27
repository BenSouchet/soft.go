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
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The xsi:schemaLocation of a document is recorded in its document root, and written back by a
// save, as EMF does: a document that names the schema of its namespace keeps naming it.
func TestXMLSchemaLocationRoundTrip(t *testing.T) {
	ePackage := loadPackage("library.complex.ecore")
	require.NotNil(t, ePackage)
	data, err := os.ReadFile("testdata/library.complex.xml")
	require.NoError(t, err)
	const namespace = "http:///org/eclipse/emf/examples/library/library.ecore/1.0.0"
	document := strings.Replace(strings.ReplaceAll(string(data), "\r\n", "\n"),
		`<library xmlns="`+namespace+`"`,
		`<library xmlns="`+namespace+`" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:schemaLocation="`+namespace+` schemas/1.0.0/library.xsd"`, 1)

	xmlProcessor := NewXMLProcessor(XMLProcessorPackages([]EPackage{ePackage}))
	eResource := xmlProcessor.GetResourceSet().CreateResource(NewURI("testdata/library.xml"))
	eResource.LoadWithReader(strings.NewReader(document), nil)
	require.True(t, eResource.GetErrors().Empty(), diagnosticError(eResource.GetErrors()))

	root := eResource.GetContents().Get(0).(EObject)
	feature := NewExtendedMetaData().GetXSISchemaLocationMapFeature(root.EClass())
	require.NotNil(t, feature)
	assert.Equal(t, "schemas/1.0.0/library.xsd", root.EGet(feature).(EMap).GetValue(namespace))

	assert.Equal(t, document, strings.ReplaceAll(xmlProcessor.SaveToString(eResource, nil), "\r\n", "\n"))
}

// XML_OPTION_SCHEMA_LOCATION names the schemas of a document a save writes, over the ones its
// document root records: a writer that knows the schema of a document states it.
func TestXMLSchemaLocationOption(t *testing.T) {
	ePackage := loadPackage("library.complex.ecore")
	require.NotNil(t, ePackage)
	const namespace = "http:///org/eclipse/emf/examples/library/library.ecore/1.0.0"
	xmlProcessor := NewXMLProcessor(XMLProcessorPackages([]EPackage{ePackage}))
	eResource := xmlProcessor.Load(NewURI("testdata/library.complex.xml"))
	require.True(t, eResource.GetErrors().Empty(), diagnosticError(eResource.GetErrors()))

	result := xmlProcessor.SaveToString(eResource, map[string]any{XML_OPTION_SCHEMA_LOCATION: namespace + " schemas/2.0.0/library.xsd"})
	assert.Contains(t, result, `<library xmlns="`+namespace+`" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:schemaLocation="`+namespace+` schemas/2.0.0/library.xsd"`)

	root := eResource.GetContents().Get(0).(EObject)
	feature := NewExtendedMetaData().GetXSISchemaLocationMapFeature(root.EClass())
	assert.False(t, root.EIsSet(feature), "the option leaves the document root as it is")
}
