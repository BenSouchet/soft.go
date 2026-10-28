package ecore

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// A list asks its owner for adapters before each change. An owner without adapters keeps none: a
// read that created the list would write into an object the reader does not own.
func TestBasicNotifyingListLeavesAnOwnerWithoutAdaptersWithout(t *testing.T) {
	factory := GetFactory()
	child := factory.CreateEClass()
	child.SetName("Child")
	children := factory.CreateEReference()
	children.SetName("children")
	children.SetEType(child)
	children.SetContainment(true)
	children.SetUpperBound(UNBOUNDED_MULTIPLICITY)
	parent := factory.CreateEClass()
	parent.SetName("Parent")
	parent.GetEStructuralFeatures().Add(children)
	ePackage := factory.CreateEPackage()
	ePackage.SetName("adapters")
	ePackage.SetNsURI("http://adapters")
	ePackage.GetEClassifiers().AddAll(NewImmutableEList([]any{parent, child}))

	owner := NewDynamicEObjectImpl()
	owner.SetEClass(parent)
	element := NewDynamicEObjectImpl()
	element.SetEClass(child)
	owner.EGet(children).(EList).Add(element)

	assert.False(t, owner.EBasicHasAdapters(), "adding to its list created the adapters of the owner")
	assert.Nil(t, owner.EBasicAdapters())

	// An owner with an adapter is still notified.
	adapter := NewMockEAdapter(t)
	adapter.EXPECT().SetTarget(owner).Once()
	owner.EAdapters().Add(adapter)
	notified := 0
	adapter.EXPECT().NotifyChanged(mock.Anything).Run(func(ENotification) { notified++ }).Once()
	other := NewDynamicEObjectImpl()
	other.SetEClass(child)
	owner.EGet(children).(EList).Add(other)
	require.Equal(t, 1, notified)
}
