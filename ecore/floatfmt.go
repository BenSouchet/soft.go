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
	"math"
	"strconv"
)

// FormatFloatXSD renders a float in XSD canonical lexical form. The special
// values serialize as INF / -INF / NaN (W3C XSD), not Go's "+Inf"/"-Inf"/"NaN"
// — so they round-trip with the schema and other readers (ParseFloat accepts
// INF/-INF/NaN). Hand-maintained here (not in the generated factory) so the
// generated per-package factories — including custom EDataTypes — can share one
// impl that survives regeneration.
func FormatFloatXSD(v float64, bitSize int) string {
	switch {
	case math.IsInf(v, 1):
		return "INF"
	case math.IsInf(v, -1):
		return "-INF"
	case math.IsNaN(v):
		return "NaN"
	default:
		return strconv.FormatFloat(v, 'f', -1, bitSize)
	}
}
