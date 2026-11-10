package xml

import (
	"bytes"
	"strings"
	"testing"

	"github.com/masagroup/soft.go/ecore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// escapePackage is a package of one class C with a single and a many string attribute, the
// attribute "single" and the elements "many".
func escapePackage() (ecore.EPackage, ecore.EClass, ecore.EAttribute, ecore.EAttribute) {
	f := ecore.GetFactory()
	ePackage := f.CreateEPackage()
	ePackage.SetName("p")
	ePackage.SetNsURI("http://p")
	ePackage.SetNsPrefix("p")
	ePackage.SetEFactoryInstance(f.CreateEFactory())
	eClass := f.CreateEClass()
	eClass.SetName("C")
	ePackage.GetEClassifiers().Add(eClass)
	single := f.CreateEAttribute()
	single.SetName("single")
	single.SetEType(ecore.GetPackage().GetEString())
	eClass.GetEStructuralFeatures().Add(single)
	many := f.CreateEAttribute()
	many.SetName("many")
	many.SetUpperBound(ecore.UNBOUNDED_MULTIPLICITY)
	many.SetEType(ecore.GetPackage().GetEString())
	eClass.GetEStructuralFeatures().Add(many)
	return ePackage, eClass, single, many
}

// The XML declaration a document was read with is written back as it was, under
// OPTION_KEEP_XML_DECLARATION: two writers of one format write different ones.
func TestXMLDeclaration_IsWrittenBackAsItWasRead(t *testing.T) {
	ePackage, _, _, _ := escapePackage()
	eResourceSet := ecore.NewEResourceSetImpl()
	eResourceSet.GetPackageRegistry().RegisterPackage(ePackage)
	for _, declaration := range []string{
		`<?xml version="1.0" encoding="UTF-8" standalone="no" ?>`,
		`<?xml version="1.0" encoding="UTF-8"?>`,
	} {
		document := declaration + "\n<p:C xmlns:p=\"http://p\" single=\"v\"/>\n"
		eResource := eResourceSet.CreateResource(ecore.NewURI("file:///declaration.xml"))
		NewDecoder(eResource, strings.NewReader(document), nil).DecodeResource()
		require.True(t, eResource.GetErrors().Empty())
		require.Equal(t, declaration, XMLDeclaration(eResource))

		var kept, plain bytes.Buffer
		NewEncoder(eResource, &kept, map[string]any{OPTION_KEEP_XML_DECLARATION: true}).EncodeResource()
		assert.True(t, strings.HasPrefix(kept.String(), declaration+"\n"), kept.String())
		NewEncoder(eResource, &plain, nil).EncodeResource()
		assert.True(t, strings.HasPrefix(plain.String(), `<?xml version="1.0" encoding="UTF-8"?>`+"\n"), "without the option, the encoder's own")
		eResourceSet.GetResources().Remove(eResource)
	}
}

// The document type declaration a document was read with, <!DOCTYPE TS> of a Qt Linguist file,
// is written back after its XML declaration.
func TestXMLDocumentType_IsWrittenBackAfterTheDeclaration(t *testing.T) {
	ePackage, _, _, _ := escapePackage()
	eResourceSet := ecore.NewEResourceSetImpl()
	eResourceSet.GetPackageRegistry().RegisterPackage(ePackage)
	prolog := `<?xml version="1.0" encoding="utf-8"?>` + "\n<!DOCTYPE TS>\n"
	eResource := eResourceSet.CreateResource(ecore.NewURI("file:///doctype.xml"))
	NewDecoder(eResource, strings.NewReader(prolog+"<p:C xmlns:p=\"http://p\" single=\"v\"/>\n"), nil).DecodeResource()
	require.True(t, eResource.GetErrors().Empty())
	require.Equal(t, "<!DOCTYPE TS>", XMLDocumentType(eResource))
	var kept bytes.Buffer
	NewEncoder(eResource, &kept, map[string]any{OPTION_KEEP_XML_DECLARATION: true}).EncodeResource()
	assert.True(t, strings.HasPrefix(kept.String(), prolog), kept.String())

	SetXMLDocumentType(eResource, "<?xml not a doctype?>")
	assert.Equal(t, "<!DOCTYPE TS>", XMLDocumentType(eResource))
}

// A document held elsewhere than in its file is given its declaration back; what is not a
// declaration is not taken for one.
func TestXMLDeclaration_IsGivenToADocumentNotReadFromXML(t *testing.T) {
	ePackage, eClass, single, _ := escapePackage()
	eResourceSet := ecore.NewEResourceSetImpl()
	eResourceSet.GetPackageRegistry().RegisterPackage(ePackage)
	eResource := eResourceSet.CreateResource(ecore.NewURI("file:///held.xml"))
	eObject := ePackage.GetEFactoryInstance().Create(eClass)
	eObject.ESet(single, "v")
	eResource.GetContents().Add(eObject)

	SetXMLDeclaration(eResource, "<!-- not a declaration -->")
	assert.Empty(t, XMLDeclaration(eResource))
	declaration := `<?xml version="1.0" encoding="UTF-8" standalone="no" ?>`
	SetXMLDeclaration(eResource, declaration)
	var buffer bytes.Buffer
	NewEncoder(eResource, &buffer, map[string]any{OPTION_KEEP_XML_DECLARATION: true}).EncodeResource()
	assert.True(t, strings.HasPrefix(buffer.String(), declaration+"\n"), buffer.String())
}

// A declaration naming another encoding than UTF-8 is written back with the document in it.
func TestXMLDeclaration_WritesTheDocumentInTheEncodingItNames(t *testing.T) {
	ePackage, _, _, _ := escapePackage()
	eResourceSet := ecore.NewEResourceSetImpl()
	eResourceSet.GetPackageRegistry().RegisterPackage(ePackage)
	declaration := `<?xml version="1.0" encoding="ISO-8859-1" standalone="no" ?>`
	document := declaration + "\n<p:C xmlns:p=\"http://p\" single=\"caf\xe9\"/>\n"
	eResource := eResourceSet.CreateResource(ecore.NewURI("file:///latin1.xml"))
	NewDecoder(eResource, strings.NewReader(document), nil).DecodeResource()
	require.True(t, eResource.GetErrors().Empty())
	var kept bytes.Buffer
	NewEncoder(eResource, &kept, map[string]any{OPTION_KEEP_XML_DECLARATION: true}).EncodeResource()
	require.True(t, eResource.GetErrors().Empty())
	assert.Equal(t, document, kept.String())
}
