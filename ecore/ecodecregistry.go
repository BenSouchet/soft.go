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

const (
	DEFAULT_EXTENSION = "*"
)

type ECodecRegistry interface {
	GetCodec(uri *URI) ECodec
	GetProtocolToCodecMap() map[string]ECodec
	GetExtensionToCodecMap() map[string]ECodec
}

var resourceCodecRegistryOnce sync.Once
var resourceCodecRegistryInstance ECodecRegistry

// GetCodecRegistry returns the process-wide default codec registry, initializing it once
// on first use. The one-time init is guarded by sync.Once (matching GetFactory /
// GetPackageRegistry) so concurrent first callers — e.g. the parallel per-resource
// renderers a multi-resource save fans out — cannot race on the lazy assignment.
func GetCodecRegistry() ECodecRegistry {
	resourceCodecRegistryOnce.Do(func() {
		resourceCodecRegistryInstance = NewECodecRegistryImpl()
		// initialize with default codecs
		extensionToCodecs := resourceCodecRegistryInstance.GetExtensionToCodecMap()
		extensionToCodecs["ecore"] = &XMICodec{}
		extensionToCodecs["xml"] = &XMLCodec{}
		extensionToCodecs["bin"] = &BinaryCodec{}
		extensionToCodecs["sqlite"] = &SQLCodec{}
		protocolToCodecs := resourceCodecRegistryInstance.GetProtocolToCodecMap()
		protocolToCodecs["memory"] = &NoCodec{}
	})
	return resourceCodecRegistryInstance
}
