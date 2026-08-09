// *****************************************************************************
// Copyright(c) 2021 MASA Group
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// *****************************************************************************

package ecore

type CompactEObjectContainer struct {
	CompactEObjectImpl
	container EObject
}

// ESetInternalContainer writes the container to its own field, but the feature id
// still lives in the high bits of the packed flags word that every accessor in
// CompactEObjectImpl reads. Take that type's mutex for it, or this write tears
// against a concurrent EIsProxy/EProxyURI/EClass.
func (o *CompactEObjectContainer) ESetInternalContainer(newContainer EObject, newContainerFeatureID int) {
	o.mutex.Lock()
	defer o.mutex.Unlock()
	o.container = newContainer
	o.flags = newContainerFeatureID<<16 | (o.flags & 0x00FF)
}

func (o *CompactEObjectContainer) EInternalContainer() EObject {
	o.mutex.Lock()
	defer o.mutex.Unlock()
	return o.container
}
