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
	"strings"
	"strconv"
	"math"
)

type Stride struct {
	start uint32
	end uint32
	s1 int
	s2 int
	prefix string
	offset uint32
}

var fullAlphabet string = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"  // 26 chars

// handles 3-letter suffixes assigned with a regular pattern
//
// start: first hexid of range
// s1: major stride (interval between different first letters)
// s2: minor stride (interval between different second letters)
// prefix: the registration prefix
// first: the suffix to use at the start of the range (default: AAA)
// last: the last valid suffix in the range (default: ZZZ)
func NewStride(start uint32, s1 int, s2 int, prefix string, startToEnd ...string) Stride {
	mapping := Stride {
		start: start,
		s1: s1,
		s2: s2,
		prefix: prefix,
	}

	var first = "AAA"
	if len(startToEnd) > 0 {
		first = startToEnd[0]
	}
	c1 := strings.Index(fullAlphabet, string(first[0]))
	c2 := strings.Index(fullAlphabet, string(first[1]))
	c3 := strings.Index(fullAlphabet, string(first[2]))
	mapping.offset = uint32(c1 * mapping.s1 + c2 * mapping.s2 + c3)
	
	var last = "ZZZ"
	if len(startToEnd) > 1 {
		last = startToEnd[1]
	}
	c1 = strings.Index(fullAlphabet, string(last[0]))
	c2 = strings.Index(fullAlphabet, string(last[1]))
	c3 = strings.Index(fullAlphabet, string(last[2]))
	mapping.end = mapping.start - mapping.offset + uint32(c1 * mapping.s1 + c2 * mapping.s2 + c3)

	return mapping
}


var stride_mappings = []Stride {
	NewStride(0x008011, 26*26, 26, "ZS-"),
	NewStride(0x390000, 1024, 32, "F-G"),
	NewStride(0x398000, 1024, 32, "F-H"),

	NewStride(0x3C4421, 1024,  32, "D-A", "AAA", "OZZ"),
	NewStride(0x3C0001, 26*26, 26, "D-A", "PAA", "ZZZ"),
	NewStride(0x3C8421, 1024,  32, "D-B", "AAA", "OZZ"),
	NewStride(0x3C2001, 26*26, 26, "D-B", "PAA", "ZZZ"),
	NewStride(0x3CC000, 26*26, 26, "D-C"),
	NewStride(0x3D04A8, 26*26, 26, "D-E"),
	NewStride(0x3D4950, 26*26, 26, "D-F"),
	NewStride(0x3D8DF8, 26*26, 26, "D-G"),
	NewStride(0x3DD2A0, 26*26, 26, "D-H"),
	NewStride(0x3E1748, 26*26, 26, "D-I"),

	NewStride(0x448421, 1024,  32, "OO-"),
	NewStride(0x458421, 1024,  32, "OY-"),
	NewStride(0x460000, 26*26, 26, "OH-"),
	NewStride(0x468421, 1024,  32, "SX-"),
	NewStride(0x490421, 1024,  32, "CS-"),
	NewStride(0x4A0421, 1024,  32, "YR-"),
	NewStride(0x4B8421, 1024,  32, "TC-"),
	NewStride(0x740421, 1024,  32, "JY-"),
	NewStride(0x760421, 1024,  32, "AP-"),
	NewStride(0x768421, 1024,  32, "9V-"),
	NewStride(0x778421, 1024,  32, "YK-"),
	NewStride(0x7C0000, 36*36, 36, "VH-"),
	NewStride(0xC00001, 26*26, 26, "C-F"),
	NewStride(0xC044A9, 26*26, 26, "C-G"),
	NewStride(0xE01041, 4096,  64, "LV-"),
}

type Numeric struct {
	start uint32
	end uint32
	first uint32
	template string
}

// numeric registrations
//  start: start hexid in range
//  first: first numeric registration
//  count: number of numeric registrations
//  template: registration template, trailing characters are replaced with the numeric registration

func NewNumeric(start uint32, first uint32, count int, template string) Numeric {
	return Numeric {
		start: start,
		end: start + uint32(count - 1),
		first: first,
		template: template,
	}
}

var numeric_mappings = []Numeric {
	NewNumeric(0x140000, 0, 100000, "RA-00000"),
	NewNumeric(0x0B03E8, 1000, 1000, "CU-T0000"),
}

/*
	icao2reg() : Converts 24-bit Mode S addresses to aircraft registration numbers such as N-numbers and C-numbers.

			Input: uint32 representing the Mode S address. Valid range for
				translation is 0xA00001 - 0xADF7C7, inclusive.

				Values between ADF7C8 - AFFFFF are allocated to the United States,
				but are not used for aicraft on the civil registry. These could be
				military, other public aircraft, or future use.

				For other supported countries mapping tables are used.


			Output:
				string: String containing the decoded tail number (if decoding succeeded),
					"NON-NA" (for non-US / non Canada allocation), and "US-MIL" or "CA-MIL" for non-civil US / Canada allocation.

				bool: True if the Mode S address successfully translated to a valid registration
					number. False for all other conditions.
*/
func icao2reg(icao_addr uint32) (string, bool) {
	
	// Determine nationality
	if (icao_addr >= 0xA00001) && (icao_addr <= 0xAFFFFF) {
		return n_reg(icao_addr)
	} else if (icao_addr >= 0xC0CDF8) && (icao_addr <= 0xC3FFFF) {
		// Discard Canadian addresses that are not assigned to aircraft on the civil registry.
		return "CA-MIL", false
	} else {
		// other countries
		tail, success := numeric_reg(icao_addr)
		if success {
			return tail, success
		}

		return stride_reg(icao_addr)
	}
}

func stride_reg(icao_addr uint32) (string, bool) {
	for _, mapping := range stride_mappings {
			
		if icao_addr >= mapping.start && icao_addr <= mapping.end {
			var offset = int(icao_addr - mapping.start + mapping.offset)

			var i1 = int(math.Floor(float64(offset) / float64(mapping.s1)))
			offset = offset % mapping.s1
			var i2 = int(math.Floor(float64(offset) / float64(mapping.s2)))
			offset = offset % mapping.s2
			var i3 = offset

			if i1 < 0 || i1 > 25 || i2 < 0 || i2 > 25 || i3 < 0 || i3 > 25 {
				return "OTHER", false
			}		

			tail := fmt.Sprintf("%s%c%c%c", mapping.prefix, fullAlphabet[i1], fullAlphabet[i2], fullAlphabet[i3])
			return tail, true
		}
	}

	return "OTHER", false
}

// try the mappings in numeric_mappings
func numeric_reg(icao_addr uint32) (string, bool) {
	for _, mapping := range numeric_mappings {
		if icao_addr >= mapping.start && icao_addr <= mapping.end {
			var reg = strconv.Itoa(int(icao_addr - mapping.start + mapping.first))
			return mapping.template[:len(mapping.template)-len(reg)] + reg, true
		}
	}
	return "OTHER", false
}

func n_reg(icao_addr uint32) (string, bool) {
	// Initialize local variables
	base34alphabet := string("ABCDEFGHJKLMNPQRSTUVWXYZ0123456789")
	nationalOffset := uint32(0xA00001) // default is US

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

	return "N" + a_char + b_char + c_char + d_char + e_char, true
}