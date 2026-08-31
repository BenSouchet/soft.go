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
	"bytes"
	"encoding/xml"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A comment has to survive BOTH halves of a round trip, and the two halves are written apart: the
// decoder decides where a comment WAS, the encoder decides where it GOES. Neither can be tested
// alone — a decoder that mislabels a position and an encoder that mislabels it the same way would
// still put the comment somewhere wrong — so what is pinned here is the pair, on a document that
// carries a comment in every position one can occupy.
const commentedLibrary = `<?xml version="1.0" encoding="UTF-8"?>
<!-- a note before the root -->
<Library xmlns="http:///org/eclipse/emf/examples/library/library.ecore/1.0.0" name="My Library">
  <!-- before the first child -->
  <employees firstName="Ada" lastName="Lovelace"/>
  <!-- between two siblings -->
  <employees firstName="Alan" lastName="Turing"/>
  <branches name="Annex">
    <!-- inside a nested element -->
    <employees firstName="Grace" lastName="Hopper"/>
  </branches>
  <!-- after the last child -->
</Library>`

// documentEvents reduces a document to the sequence a comment's position is defined against:
// element names in document order, and comment bodies where they stand between them. Comparing two
// documents' event sequences answers "did every comment come back among the same siblings, in the
// same order" and nothing else — indentation, attribute order and line endings cannot affect it.
func documentEvents(t *testing.T, data []byte) []string {
	t.Helper()
	var out []string
	dec := xml.NewDecoder(bytes.NewReader(data))
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		switch tk := tok.(type) {
		case xml.StartElement:
			out = append(out, "<"+tk.Name.Local+">")
		case xml.EndElement:
			out = append(out, "</"+tk.Name.Local+">")
		case xml.Comment:
			out = append(out, "#"+strings.TrimSpace(string(tk)))
		}
	}
	return out
}

func loadCommentedLibrary(t *testing.T) (*XMLProcessor, EResource) {
	t.Helper()
	ePackage := loadPackage("library.noroot.ecore")
	require.NotNil(t, ePackage)
	processor := NewXMLProcessor(XMLProcessorPackages([]EPackage{ePackage}))
	resource := processor.LoadWithReader(strings.NewReader(commentedLibrary), nil)
	require.NotNil(t, resource)
	require.True(t, resource.GetErrors().Empty(), diagnosticError(resource.GetErrors()))
	return processor, resource
}

func TestXMLCommentsSurviveARoundTrip(t *testing.T) {
	processor, resource := loadCommentedLibrary(t)

	var buffer bytes.Buffer
	processor.SaveWithWriter(&buffer, resource, map[string]any{XML_OPTION_KEEP_COMMENTS: true})
	require.True(t, resource.GetErrors().Empty(), diagnosticError(resource.GetErrors()))

	assert.Equal(t, documentEvents(t, []byte(commentedLibrary)), documentEvents(t, buffer.Bytes()))
	assert.True(t, resource.GetWarnings().Empty(), diagnosticError(resource.GetWarnings()))
}

// Without the option the encoder writes what it always wrote. This is the guard on blast radius:
// every caller that does not ask for comments — every physical document in a SWORD database, every
// other model this codec serves — keeps its output byte for byte, even though the decoder now
// collects comments for all of them.
func TestXMLCommentsAreNotWrittenUnlessAsked(t *testing.T) {
	processor, resource := loadCommentedLibrary(t)

	var buffer bytes.Buffer
	processor.SaveWithWriter(&buffer, resource, nil)
	require.True(t, resource.GetErrors().Empty(), diagnosticError(resource.GetErrors()))

	assert.NotContains(t, buffer.String(), "<!--")
	assert.Contains(t, buffer.String(), "Lovelace") // the document itself was still written
}

// Collecting comments must not reach the object graph: they are not objects, not features and not
// text, and a decode that let them leak into any of the three would corrupt the model to preserve a
// note about it.
func TestXMLCommentsDoNotEnterTheGraph(t *testing.T) {
	_, resource := loadCommentedLibrary(t)

	library := resource.GetContents().Get(0).(EObject)
	employees := library.EGet(library.EClass().GetEStructuralFeatureFromName("employees")).(EList)
	assert.Equal(t, 2, employees.Size())
	assert.Equal(t, "My Library", library.EGet(library.EClass().GetEStructuralFeatureFromName("name")))
}

// This is the mechanism's limit, stated as a test rather than left to be discovered.
//
// A comment is addressed by the position it held — inside which element, after how many of its
// children — not by the object it was written about. So an edit does not carry a comment with its
// meaning: deleting <branches> takes away both the comment INSIDE it and the position of the
// comment that stood AFTER it, because Library now has two children where the trailing comment was
// pinned after three.
//
// A load and a write-back never move a position, which is why the round trip above is exact. An
// editor built on this needs re-anchoring, and until it has one, a comment that cannot be placed is
// said out loud: it becomes a warning rather than vanishing, and the rest of the document is still
// written, because refusing the save would be the worse failure.
func TestXMLCommentsAreAnchoredToAPositionNotAnObject(t *testing.T) {
	processor, resource := loadCommentedLibrary(t)

	library := resource.GetContents().Get(0).(EObject)
	branches := library.EGet(library.EClass().GetEStructuralFeatureFromName("branches")).(EList)
	branches.Clear()

	var buffer bytes.Buffer
	processor.SaveWithWriter(&buffer, resource, map[string]any{XML_OPTION_KEEP_COMMENTS: true})

	require.True(t, resource.GetErrors().Empty(), diagnosticError(resource.GetErrors()))
	require.Equal(t, 1, resource.GetWarnings().Size())
	assert.Contains(t, resource.GetWarnings().Get(0).(EDiagnostic).GetMessage(), "2 of 5 XML comments")
	assert.NotContains(t, buffer.String(), "inside a nested element")
	assert.NotContains(t, buffer.String(), "after the last child")
	// The comments whose position the edit did not disturb are written as they always were.
	assert.Contains(t, buffer.String(), "a note before the root")
	assert.Contains(t, buffer.String(), "before the first child")
	assert.Contains(t, buffer.String(), "between two siblings")
}
