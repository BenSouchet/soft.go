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
// - so they round-trip with the schema and other readers (ParseFloat accepts
// INF/-INF/NaN). Hand-maintained here (not in the generated factory) so the
// generated per-package factories - including custom EDataTypes - can share one
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
		// Decimal ('f') for the normal range, matching ADN, which does not exponent
		// mid/small values. Scientific ('g') only from 1e21, where a decimal balloons
		// to hundreds of digits. 1e21 is where Go's 'g' switches to exponent anyway,
		// so below it the two agree and 'f' is kept for clarity.
		if math.Abs(v) >= 1e21 {
			return strconv.FormatFloat(v, 'g', -1, bitSize)
		}
		return strconv.FormatFloat(v, 'f', -1, bitSize)
	}
}
