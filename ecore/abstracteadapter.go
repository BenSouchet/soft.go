// *****************************************************************************
// Copyright(c) 2021 MASA Group
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// *****************************************************************************

package ecore

import "sync"

// AbstractEAdapter is a abstract implementation of EAdapter interface
type AbstractEAdapter struct {
	// mutex guards target. One adapter instance is routinely attached to many
	// notifiers — an EContentAdapter installs itself on every object of a graph — and
	// each attach writes target through notifierAdapterList.DidAdd. Loading several
	// resources of one resource set concurrently therefore writes this field from
	// several goroutines at once, and ENotifier is an interface (two words), so an
	// unguarded write can be observed torn rather than merely stale.
	//
	// The lock covers the field only, never an attach walk: EContentAdapter.SetTarget
	// re-enters SetTarget for every child it attaches to, so a lock held across the
	// walk would deadlock on itself.
	mutex  sync.Mutex
	target ENotifier
}

func NewAbstractAdapter() *AbstractEAdapter {
	return &AbstractEAdapter{}
}

// GetTarget Returns the target from which the AbstractEAdapter receives notification.
func (a *AbstractEAdapter) GetTarget() ENotifier {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	return a.target
}

// SetTarget Sets the target from which the AbstractEAdapter will receive notification.
func (a *AbstractEAdapter) SetTarget(notifier ENotifier) {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	a.target = notifier
}

func (a *AbstractEAdapter) UnSetTarget(notifier ENotifier) {
	// Compare and clear under one acquisition. Calling SetTarget here instead would
	// re-enter the lock, and splitting the two would let another goroutine's attach
	// land between them.
	a.mutex.Lock()
	defer a.mutex.Unlock()
	if notifier == a.target {
		a.target = nil
	}
}
