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
		// Decimal ('f') for the normal range — matches ADN, which does NOT exponent
		// mid/small values (a plain 'g' would render 1e-05 for 0.00001, diverging).
		// Scientific ('g', shortest round-trippable) only for very large magnitudes
		// (>= 1e21), where a plain decimal balloons to hundreds of digits — e.g. the
		// max double 1.79e308 became a 309-digit integer while ADN writes
		// 1.7976931348623157e+308. 1e21 is exactly where Go's 'g' switches to
		// exponent, so below it 'f' and 'g' agree; 'f' is kept there for clarity.
		if math.Abs(v) >= 1e21 {
			return strconv.FormatFloat(v, 'g', -1, bitSize)
		}
		return strconv.FormatFloat(v, 'f', -1, bitSize)
	}
}
