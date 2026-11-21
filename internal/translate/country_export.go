// SPDX-License-Identifier: GPL-2.0-or-later
package translate

// CountryCode resolves the controller's numeric country code using the
// upstream ISO table, preferring a per-radio setting over the global value.
// An unknown nonempty raw value returns an empty alpha-2 code.
func CountryCode(perRadio, global string) (alpha2, raw string) {
	return countryCode(perRadio, global)
}
