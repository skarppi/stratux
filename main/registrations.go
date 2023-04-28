/*
	Copyright (c) 2015-2016 Christopher Young
	Distributable under the terms of The "BSD New" License
	that can be found in the LICENSE file, herein included
	as part of this header.

	registrations.go: Map icao transponder codes to registrations.
*/

package main

import (
	"fmt"
)


/*
	icao2reg() : Converts 24-bit Mode S addresses to N-numbers and C-numbers.

			Input: uint32 representing the Mode S address. Valid range for
				translation is 0xA00001 - 0xADF7C7, inclusive.

				Values outside the range A000001-AFFFFFF or C00001-C3FFFF
				are flagged as foreign.

				Values between ADF7C8 - AFFFFF are allocated to the United States,
				but are not used for aicraft on the civil registry. These could be
				military, other public aircraft, or future use.

				Values between C0CDF9 - C3FFFF are allocated to Canada,
				but are not used for aicraft on the civil registry. These could be
				military, other public aircraft, or future use.

				Values between 7C0000 - 7FFFFF are allocated to Australia.


			Output:
				string: String containing the decoded tail number (if decoding succeeded),
					"NON-NA" (for non-US / non Canada allocation), and "US-MIL" or "CA-MIL" for non-civil US / Canada allocation.

				bool: True if the Mode S address successfully translated to an
					N number. False for all other conditions.
*/
func icao2reg(icao_addr uint32) (string, bool) {
	// Initialize local variables
	base34alphabet := string("ABCDEFGHJKLMNPQRSTUVWXYZ0123456789")
	nationalOffset := uint32(0xA00001) // default is US
	tail := ""
	nation := ""

	// Determine nationality
	if (icao_addr >= 0xA00001) && (icao_addr <= 0xAFFFFF) {
		nation = "US"
	} else if (icao_addr >= 0xC00001) && (icao_addr <= 0xC3FFFF) {
		nation = "CA"
	} else if (icao_addr >= 0x7C0000) && (icao_addr <= 0x7FFFFF) {
		nation = "AU"
	} else {
		//TODO: future national decoding.
		return "OTHER", false
	}

	if nation == "CA" { // Canada decoding
		// First, discard addresses that are not assigned to aircraft on the civil registry
		if icao_addr > 0xC0CDF8 {
			//fmt.Printf("%X is a Canada aircraft, but not a CF-, CG-, or CI- registration.\n", icao_addr)
			return "CA-MIL", false
		}

		nationalOffset := uint32(0xC00001)
		serial := int32(icao_addr - nationalOffset)

		// Fifth letter
		e := serial % 26

		// Fourth letter
		d := (serial / 26) % 26

		// Third letter
		c := (serial / 676) % 26 // 676 == 26*26

		// Second letter
		b := (serial / 17576) % 26 // 17576 == 26*26*26

		b_str := "FGI"

		//fmt.Printf("B = %d, C = %d, D = %d, E = %d\n",b,c,d,e)
		tail = fmt.Sprintf("C-%c%c%c%c", b_str[b], c+65, d+65, e+65)
	}

	if nation == "AU" { // Australia decoding

		nationalOffset := uint32(0x7C0000)
		offset := (icao_addr - nationalOffset)
		i1 := offset / 1296
		offset2 := offset % 1296
		i2 := offset2 / 36
		offset3 := offset2 % 36
		i3 := offset3

		var a_char, b_char, c_char string

		a_char = fmt.Sprintf("%c", i1+65)
		b_char = fmt.Sprintf("%c", i2+65)
		c_char = fmt.Sprintf("%c", i3+65)

		if i1 < 0 || i1 > 25 || i2 < 0 || i2 > 25 || i3 < 0 || i3 > 25 {
			return "OTHER", false
		}

		tail = "VH-" + a_char + b_char + c_char
	}

	if nation == "US" { // FAA decoding
		// First, discard addresses that are not assigned to aircraft on the civil registry
		if icao_addr > 0xADF7C7 {
			//fmt.Printf("%X is a US aircraft, but not on the civil registry.\n", icao_addr)
			return "US-MIL", false
		}

		serial := int32(icao_addr - nationalOffset)
		// First digit
		a := (serial / 101711) + 1

		// Second digit
		a_remainder := serial % 101711
		b := ((a_remainder + 9510) / 10111) - 1

		// Third digit
		b_remainder := (a_remainder + 9510) % 10111
		c := ((b_remainder + 350) / 951) - 1

		// This next bit is more convoluted. First, figure out if we're using the "short" method of
		// decoding the last two digits (two letters, one letter and one blank, or two blanks).
		// This will be the case if digit "B" or "C" are calculated as negative, or if c_remainder
		// is less than 601.

		c_remainder := (b_remainder + 350) % 951
		var d, e int32

		if (b >= 0) && (c >= 0) && (c_remainder > 600) { // alphanumeric decoding method
			d = 24 + (c_remainder-601)/35
			e = (c_remainder - 601) % 35

		} else { // two-letter decoding method
			if (b < 0) || (c < 0) {
				c_remainder -= 350 // otherwise "  " == 350, "A " == 351, "AA" == 352, etc.
			}

			d = (c_remainder - 1) / 25
			e = (c_remainder - 1) % 25

			if e < 0 {
				d -= 1
				e += 25
			}
		}

		a_char := fmt.Sprintf("%d", a)
		var b_char, c_char, d_char, e_char string

		if b >= 0 {
			b_char = fmt.Sprintf("%d", b)
		}

		if b >= 0 && c >= 0 {
			c_char = fmt.Sprintf("%d", c)
		}

		if d > -1 {
			d_char = string(base34alphabet[d])
			if e > 0 {
				e_char = string(base34alphabet[e-1])
			}
		}

		tail = "N" + a_char + b_char + c_char + d_char + e_char

	}

	return tail, true
}