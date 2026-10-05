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
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestXMLEncoderEscapesReferenceURIs(t *testing.T) {
	for _, kind := range []string{"element", "attribute"} {
		t.Run(kind, func(t *testing.T) {
			factory := GetFactory()
			pkg := factory.CreateEPackage()
			pkg.SetName("refs")
			pkg.SetNsPrefix("refs")
			pkg.SetNsURI("urn:refs")
			rootClass, targetClass := factory.CreateEClass(), factory.CreateEClass()
			rootClass.SetName("Root")
			targetClass.SetName("Target")
			pkg.GetEClassifiers().AddAll(NewImmutableEList([]any{rootClass, targetClass}))
			references := make([]EReference, 2)
			for i, name := range []string{"single", "many"} {
				ref := factory.CreateEReference()
				ref.SetName(name)
				ref.SetEType(targetClass)
				if i == 1 {
					ref.SetUpperBound(-1)
				}
				annotation := factory.CreateEAnnotation()
				annotation.SetSource("http:///org/eclipse/emf/ecore/util/ExtendedMetaData")
				annotation.GetDetails().Put("kind", kind)
				ref.GetEAnnotations().Add(annotation)
				rootClass.GetEStructuralFeatures().Add(ref)
				references[i] = ref
			}
			root := pkg.GetEFactoryInstance().Create(rootClass)
			proxy := func(fragment string) EObject {
				obj := pkg.GetEFactoryInstance().Create(targetClass)
				obj.(EObjectInternal).ESetProxyURI(NewURI("R&D.xml#" + fragment))
				return obj
			}
			root.ESet(references[0], proxy("first"))
			root.EGet(references[1]).(EList).AddAll(NewImmutableEList([]any{proxy("first"), proxy("second")}))
			processor := NewXMLProcessor(XMLProcessorPackages([]EPackage{pkg}))
			resource := processor.GetResourceSet().CreateResource(NewURI("root.xml"))
			resource.GetContents().Add(root)
			options := map[string]any{XML_OPTION_EXTENDED_META_DATA: NewExtendedMetaData()}
			encoded := processor.SaveToString(resource, options)
			require.True(t, resource.GetErrors().Empty(), diagnosticError(resource.GetErrors()))
			require.Contains(t, encoded, "R&amp;D.xml")
			require.NotContains(t, encoded, "&amp;amp;")
			decoder := xml.NewDecoder(strings.NewReader(encoded))
			var values []string
			for {
				token, err := decoder.Token()
				if err == io.EOF {
					break
				}
				require.NoError(t, err)
				if start, ok := token.(xml.StartElement); ok {
					for _, attr := range start.Attr {
						if attr.Name.Local == "href" || attr.Name.Local == "single" || attr.Name.Local == "many" {
							values = append(values, strings.Fields(attr.Value)...)
						}
					}
				}
			}
			require.Equal(t, []string{"R&D.xml#first", "R&D.xml#first", "R&D.xml#second"}, values)
			reloaded := processor.GetResourceSet().CreateResource(NewURI("roundtrip.xml"))
			reloaded.LoadWithReader(strings.NewReader(encoded), options)
			require.True(t, reloaded.GetErrors().Empty(), diagnosticError(reloaded.GetErrors()))
			require.Equal(t, 1, reloaded.GetContents().Size())
			object := reloaded.GetContents().Get(0).(EObject)
			one := object.EGetResolve(references[0], false).(EObjectInternal)
			require.Contains(t, one.EProxyURI().String(), "R&D.xml#first")
			many := object.EGetResolve(references[1], false).(EList)
			require.Equal(t, 2, many.Size())
			for i, fragment := range []string{"first", "second"} {
				require.Contains(t, many.Get(i).(EObjectInternal).EProxyURI().String(), "R&D.xml#"+fragment)
			}
		})
	}
}

func TestXMLEncoderLibraryNoRootWithOptions(t *testing.T) {
	// load package
	ePackage := loadPackage("library.noroot.ecore")
	assert.NotNil(t, ePackage)

	// load resource
	options := map[string]any{XML_OPTION_SUPPRESS_DOCUMENT_ROOT: true, XML_OPTION_EXTENDED_META_DATA: NewExtendedMetaData()}
	xmlProcessor := NewXMLProcessor(XMLProcessorPackages([]EPackage{ePackage}))
	eResource := xmlProcessor.LoadWithOptions(NewURI("testdata/library.noroot.xml"), options)
	require.NotNil(t, eResource)
	assert.True(t, eResource.IsLoaded())
	assert.True(t, eResource.GetErrors().Empty(), diagnosticError(eResource.GetErrors()))
	assert.True(t, eResource.GetWarnings().Empty(), diagnosticError(eResource.GetWarnings()))

	// save
	eResource.SetURI(NewURI("testdata/library.noroot.result.xml"))
	xmlProcessor.SaveWithOptions(eResource, options)

	// result
	src, err := os.ReadFile("testdata/library.noroot.xml")
	assert.Nil(t, err)

	result, err := os.ReadFile("testdata/library.noroot.xml")
	assert.Nil(t, err)
	assert.Equal(t, strings.ReplaceAll(string(src), "\r\n", "\n"), strings.ReplaceAll(string(result), "\r\n", "\n"))
}

func TestXMLEncoderLibraryNoRootWithReaderWriter(t *testing.T) {
	// load package
	ePackage := loadPackage("library.noroot.ecore")
	assert.NotNil(t, ePackage)

	// xml processor
	xmlProcessor := NewXMLProcessor(XMLProcessorPackages([]EPackage{ePackage}))
	options := map[string]any{XML_OPTION_SUPPRESS_DOCUMENT_ROOT: true, XML_OPTION_EXTENDED_META_DATA: NewExtendedMetaData()}

	// load resource
	reader, error := os.Open("testdata/library.noroot.xml")
	require.Nil(t, error)
	eResource := xmlProcessor.LoadWithReader(reader, options)
	require.NotNil(t, eResource)
	assert.True(t, eResource.IsLoaded())
	assert.True(t, eResource.GetErrors().Empty(), diagnosticError(eResource.GetErrors()))
	assert.True(t, eResource.GetWarnings().Empty(), diagnosticError(eResource.GetWarnings()))

	// save
	writer, error := os.Create("testdata/library.noroot.result.xml")
	require.Nil(t, error)
	xmlProcessor.SaveWithWriter(writer, eResource, options)

	// result
	src, err := os.ReadFile("testdata/library.noroot.xml")
	assert.Nil(t, err)

	result, err := os.ReadFile("testdata/library.noroot.xml")
	assert.Nil(t, err)
	assert.Equal(t, strings.ReplaceAll(string(src), "\r\n", "\n"), strings.ReplaceAll(string(result), "\r\n", "\n"))
}

func TestXMLEncoderLibraryComplex(t *testing.T) {
	// load package
	ePackage := loadPackage("library.complex.ecore")
	assert.NotNil(t, ePackage)

	// load resource
	xmlProcessor := NewXMLProcessor(XMLProcessorPackages([]EPackage{ePackage}))
	eResource := xmlProcessor.Load(NewURI("testdata/library.complex.xml"))
	require.NotNil(t, eResource)
	assert.True(t, eResource.IsLoaded())
	assert.True(t, eResource.GetErrors().Empty(), diagnosticError(eResource.GetErrors()))
	assert.True(t, eResource.GetWarnings().Empty(), diagnosticError(eResource.GetWarnings()))
	// save resource
	result := xmlProcessor.SaveToString(eResource, nil)

	bytes, err := os.ReadFile("testdata/library.complex.xml")
	assert.Nil(t, err)
	assert.Equal(t, strings.ReplaceAll(string(bytes), "\r\n", "\n"), strings.ReplaceAll(result, "\r\n", "\n"))
}

func TestXMLEncoderEMaps(t *testing.T) {
	// load package
	ePackage := loadPackage("emap.ecore")
	assert.NotNil(t, ePackage)

	// load resource
	xmlProcessor := NewXMLProcessor(XMLProcessorPackages([]EPackage{ePackage}))
	eResource := xmlProcessor.Load(NewURI("testdata/emap.xml"))
	require.NotNil(t, eResource)
	assert.True(t, eResource.IsLoaded())
	assert.True(t, eResource.GetErrors().Empty(), diagnosticError(eResource.GetErrors()))
	assert.True(t, eResource.GetWarnings().Empty(), diagnosticError(eResource.GetWarnings()))
	// save resource
	result := xmlProcessor.SaveToString(eResource, nil)

	bytes, err := os.ReadFile("testdata/emap.xml")
	assert.Nil(t, err)
	assert.Equal(t, strings.ReplaceAll(string(bytes), "\r\n", "\n"), strings.ReplaceAll(result, "\r\n", "\n"))
}

func TestXMLEncoderLibraryComplexSubElement(t *testing.T) {
	// load package
	ePackage := loadPackage("library.complex.ecore")
	assert.NotNil(t, ePackage)

	// load resource
	xmlProcessor := NewXMLProcessor(XMLProcessorPackages([]EPackage{ePackage}))
	eResource := xmlProcessor.Load(NewURI("testdata/library.complex.xml"))
	require.NotNil(t, eResource)
	assert.True(t, eResource.IsLoaded())
	assert.True(t, eResource.GetErrors().Empty(), diagnosticError(eResource.GetErrors()))
	assert.True(t, eResource.GetWarnings().Empty(), diagnosticError(eResource.GetWarnings()))

	eObject := eResource.GetEObject("//@library/@employees.0")
	require.NotNil(t, eObject)
	eContainer := eObject.EContainer()
	require.NotNil(t, eContainer)

	// create a new resource
	eNewResource := eResource.GetResourceSet().CreateResource(NewURI("testdata/library.complex.sub.xml"))
	// add object to new resource
	eNewResource.GetContents().Add(eObject)
	// save it
	result := xmlProcessor.SaveToString(eNewResource, nil)

	// check result
	bytes, err := os.ReadFile("testdata/library.complex.sub.xml")
	assert.Nil(t, err)
	assert.Equal(t, strings.ReplaceAll(string(bytes), "\r\n", "\n"), strings.ReplaceAll(result, "\r\n", "\n"))

	// attach to original resource
	eLibraryClass, _ := ePackage.GetEClassifier("Library").(EClass)
	require.NotNil(t, eLibraryClass)
	eLibraryEmployeesFeature := eLibraryClass.GetEStructuralFeatureFromName("employees")
	require.NotNil(t, eLibraryEmployeesFeature)
	eList := eContainer.EGet(eLibraryEmployeesFeature).(EList)
	eList.Add(eObject)
	assert.Equal(t, eResource, eObject.EResource())
}

func TestXMLEncoderLibraryComplexWithOptions(t *testing.T) {
	// load package
	ePackage := loadPackage("library.complex.ecore")
	assert.NotNil(t, ePackage)

	options := map[string]any{XML_OPTION_SUPPRESS_DOCUMENT_ROOT: true, XML_OPTION_EXTENDED_META_DATA: NewExtendedMetaData()}

	// load resource
	xmlProcessor := NewXMLProcessor(XMLProcessorPackages([]EPackage{ePackage}))
	eResource := xmlProcessor.LoadWithOptions(NewURI("testdata/library.complex.noroot.xml"), options)
	require.NotNil(t, eResource)
	assert.True(t, eResource.IsLoaded())
	assert.True(t, eResource.GetErrors().Empty(), diagnosticError(eResource.GetErrors()))
	assert.True(t, eResource.GetWarnings().Empty(), diagnosticError(eResource.GetWarnings()))

	// save resource
	result := xmlProcessor.SaveToString(eResource, options)

	bytes, err := os.ReadFile("testdata/library.complex.noroot.xml")
	assert.Nil(t, err)
	assert.Equal(t, strings.ReplaceAll(string(bytes), "\r\n", "\n"), strings.ReplaceAll(result, "\r\n", "\n"))
}

func TestXMLEncoderSimpleEscapeXML(t *testing.T) {
	// load libray simple ecore	package
	ePackage := loadPackage("library.simple.ecore")
	assert.NotNil(t, ePackage)

	// retrive library class & library name attribute
	eLibraryClass, _ := ePackage.GetEClassifier("Library").(EClass)
	assert.NotNil(t, eLibraryClass)
	eLibraryLocationAttribute, _ := eLibraryClass.GetEStructuralFeatureFromName("location").(EAttribute)
	assert.NotNil(t, eLibraryLocationAttribute)

	eFactory := ePackage.GetEFactoryInstance()
	eLibrary := eFactory.Create(eLibraryClass)
	eLibrary.ESet(eLibraryLocationAttribute, "a<b")

	xmlProcessor := NewXMLProcessor(XMLProcessorPackages([]EPackage{ePackage}))
	eResource := xmlProcessor.GetResourceSet().CreateResource(NewURI("testdata/library.simple.escape.output.xml"))
	eResource.GetContents().Add(eLibrary)
	result := xmlProcessor.SaveToString(eResource, nil)

	bytes, err := os.ReadFile("testdata/library.simple.escape.xml")
	assert.Nil(t, err)
	assert.Equal(t, strings.ReplaceAll(string(bytes), "\r\n", "\n"), strings.ReplaceAll(result, "\r\n", "\n"))
}

func TestXMLEncoderSimpleXMLWithIDs(t *testing.T) {

	// load libray simple ecore	package
	ePackage := loadPackage("library.simple.ecore")
	assert.NotNil(t, ePackage)

	eResourceSet := NewEResourceSetImpl()
	eResourceSet.GetPackageRegistry().RegisterPackage(ePackage)
	eResource := eResourceSet.CreateResource(NewURI("testdata/library.simple.xml"))
	require.NotNil(t, eResource)
	eResource.SetObjectIDManager(NewIncrementalIDManager())
	eResource.Load()
	assert.True(t, eResource.IsLoaded())
	assert.True(t, eResource.GetErrors().Empty(), diagnosticError(eResource.GetErrors()))

	var strbuff strings.Builder
	eResource.SaveWithWriter(&strbuff, map[string]any{XML_OPTION_ID_ATTRIBUTE_NAME: "id"})

	bytes, err := os.ReadFile("testdata/library.simple.ids.xml")
	assert.Nil(t, err)
	assert.Equal(t, strings.ReplaceAll(string(bytes), "\r\n", "\n"), strings.ReplaceAll(strbuff.String(), "\r\n", "\n"))
}

func TestXMLEncoderSimpleXMLWithEDataTypeList(t *testing.T) {

	// load libray simple ecore	package
	ePackage := loadPackage("library.datalist.ecore")
	assert.NotNil(t, ePackage)

	xmlProcessor := NewXMLProcessor(XMLProcessorPackages([]EPackage{ePackage}))
	eResource := xmlProcessor.Load(NewURI("testdata/library.datalist.xml"))
	require.NotNil(t, eResource)
	assert.True(t, eResource.IsLoaded())
	assert.True(t, eResource.GetErrors().Empty(), diagnosticError(eResource.GetErrors()))
	assert.True(t, eResource.GetWarnings().Empty(), diagnosticError(eResource.GetWarnings()))
	// save resource
	result := xmlProcessor.SaveToString(eResource, nil)

	bytes, err := os.ReadFile("testdata/library.datalist.xml")
	assert.Nil(t, err)
	assert.Equal(t, strings.ReplaceAll(string(bytes), "\r\n", "\n"), strings.ReplaceAll(result, "\r\n", "\n"))
}

func TestXMLEncoderSimpleXMLRootObjects(t *testing.T) {
	// load libray simple ecore	package
	ePackage := loadPackage("library.simple.ecore")
	assert.NotNil(t, ePackage)

	// load model file
	xmlProcessor := NewXMLProcessor(XMLProcessorPackages([]EPackage{ePackage}))
	eResource := xmlProcessor.Load(NewURI("testdata/library.simple.xml"))
	require.NotNil(t, eResource)
	assert.True(t, eResource.IsLoaded())
	assert.True(t, eResource.GetErrors().Empty(), diagnosticError(eResource.GetErrors()))

	// retrieve second book
	libraryClass, _ := ePackage.GetEClassifier("Library").(EClass)
	require.NotNil(t, libraryClass)
	libraryBooksFeature := libraryClass.GetEStructuralFeatureFromName("books")
	require.NotNil(t, libraryBooksFeature)

	require.Equal(t, 1, eResource.GetContents().Size())
	eLibrary, _ := eResource.GetContents().Get(0).(EObject)
	require.NotNil(t, eLibrary)

	eBooks, _ := eLibrary.EGet(libraryBooksFeature).(EList)
	require.NotNil(t, eBooks)
	require.Equal(t, 4, eBooks.Size())
	eBook := eBooks.Get(1).(EObject)

	// save it now
	var strbuff strings.Builder
	eResource.SaveWithWriter(&strbuff, map[string]any{XML_OPTION_ROOT_OBJECTS: NewImmutableEList([]any{eBook})})

	bytes, err := os.ReadFile("testdata/book.simple.xml")
	assert.Nil(t, err)
	assert.Equal(t, strings.ReplaceAll(string(bytes), "\r\n", "\n"), strings.ReplaceAll(strbuff.String(), "\r\n", "\n"))
}

func TestXMLEncoderSimpleObject(t *testing.T) {
	// load libray simple ecore	package
	ePackage := loadPackage("library.simple.ecore")
	assert.NotNil(t, ePackage)

	// load model file
	xmlProcessor := NewXMLProcessor(XMLProcessorPackages([]EPackage{ePackage}))
	eResource := xmlProcessor.Load(NewURI("testdata/library.simple.xml"))
	require.NotNil(t, eResource)
	assert.True(t, eResource.IsLoaded())
	assert.True(t, eResource.GetErrors().Empty(), diagnosticError(eResource.GetErrors()))

	// retrieve second book
	libraryClass, _ := ePackage.GetEClassifier("Library").(EClass)
	require.NotNil(t, libraryClass)
	libraryBooksFeature := libraryClass.GetEStructuralFeatureFromName("books")
	require.NotNil(t, libraryBooksFeature)

	require.Equal(t, 1, eResource.GetContents().Size())
	eLibrary, _ := eResource.GetContents().Get(0).(EObject)
	require.NotNil(t, eLibrary)

	eBooks, _ := eLibrary.EGet(libraryBooksFeature).(EList)
	require.NotNil(t, eBooks)
	require.Equal(t, 4, eBooks.Size())
	eBook := eBooks.Get(1).(EObject)

	var strbuff strings.Builder
	encoder := NewXMLEncoder(eResource, &strbuff, nil)
	err := encoder.EncodeObject(eBook)
	assert.Nil(t, err)

	bytes, err := os.ReadFile("testdata/book.simple.xml")
	assert.Nil(t, err)
	assert.Equal(t, strings.ReplaceAll(string(bytes), "\r\n", "\n"), strings.ReplaceAll(strbuff.String(), "\r\n", "\n"))
}

func BenchmarkXMLEncoderLibraryComplexBig(b *testing.B) {
	// load package
	ePackage := loadPackage("library.complex.ecore")
	require.NotNil(b, ePackage)
	xmlProcessor := NewXMLProcessor(XMLProcessorPackages([]EPackage{ePackage}))
	eResource := xmlProcessor.Load(NewURI("testdata/library.complex.big.xml"))
	require.NotNil(b, eResource)
	for i := 0; i < b.N; i++ {
		var strbuff strings.Builder
		eResource.SaveWithWriter(&strbuff, nil)
		assert.True(b, eResource.GetErrors().Empty(), diagnosticError(eResource.GetErrors()))
	}
}

func BenchmarkXMLEncoderAllTypes(b *testing.B) {
	ePackage := loadPackage("alltypes.ecore")
	require.NotNil(b, ePackage)

	eResourceSet := NewEResourceSetImpl()
	eResourceSet.GetPackageRegistry().RegisterPackage(ePackage)
	eResource := eResourceSet.CreateResource(NewURI("testdata/alltypes.xml"))
	require.NotNil(b, eResource)

	// retrive library class & library name attribute
	objectClass, _ := ePackage.GetEClassifier("Object").(EClass)
	require.NotNil(b, objectClass)

	enumType := ePackage.GetEClassifier("EnumCategory").(EEnum)
	require.NotNil(b, enumType)

	objectF32Attribute := objectClass.GetEStructuralFeatureFromName("f32")
	require.NotNil(b, objectF32Attribute)

	objectF64Attribute := objectClass.GetEStructuralFeatureFromName("f64")
	require.NotNil(b, objectF64Attribute)

	objectStringAttribute := objectClass.GetEStructuralFeatureFromName("str")
	require.NotNil(b, objectStringAttribute)

	objectI8Attribute := objectClass.GetEStructuralFeatureFromName("i8")
	require.NotNil(b, objectI8Attribute)

	objectI16Attribute := objectClass.GetEStructuralFeatureFromName("i16")
	require.NotNil(b, objectI16Attribute)

	objectI32Attribute := objectClass.GetEStructuralFeatureFromName("i32")
	require.NotNil(b, objectI32Attribute)

	objectI64Attribute := objectClass.GetEStructuralFeatureFromName("i64")
	require.NotNil(b, objectI64Attribute)

	objectIntAttribute := objectClass.GetEStructuralFeatureFromName("i")
	require.NotNil(b, objectIntAttribute)

	objectBytesAttribute := objectClass.GetEStructuralFeatureFromName("bytes")
	require.NotNil(b, objectBytesAttribute)

	objectBoolAttribute := objectClass.GetEStructuralFeatureFromName("b")
	require.NotNil(b, objectBoolAttribute)

	objectEnumAttribute := objectClass.GetEStructuralFeatureFromName("e")
	require.NotNil(b, objectEnumAttribute)

	eFactory := ePackage.GetEFactoryInstance()
	eObject := eFactory.Create(objectClass)
	eObject.ESet(objectF64Attribute, float64(4.0))
	eObject.ESet(objectF32Attribute, float32(3.0))
	eObject.ESet(objectStringAttribute, "str")
	eObject.ESet(objectI8Attribute, byte('b'))
	eObject.ESet(objectI16Attribute, int16(2))
	eObject.ESet(objectI32Attribute, int32(1))
	eObject.ESet(objectI64Attribute, int64(0))
	eObject.ESet(objectIntAttribute, int(-1))
	eObject.ESet(objectBytesAttribute, []byte("bytes"))
	eObject.ESet(objectBoolAttribute, true)
	eObject.ESet(objectEnumAttribute, enumType.GetDefaultValue())

	eResource.GetContents().Add(eObject)
	eResource.Save()
}
