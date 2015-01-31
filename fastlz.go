// Package fastlz implements the FastLZ compression method
/*

Currently only level 1 compression (fastest) is supported.

This code translated from

    https://github.com/ariya/FastLZ/blob/master/fastlz.c
    https://raw.githubusercontent.com/kbatten/fastlz.js/master/compressor.js

and is licensed under the MIT License like the originals.

FastLZ home page: http://fastlz.org/

*/
package fastlz

import "errors"

// TODO(dgryski): make compression API match snappy and friends
// TODO(dgryski): add level 2?
// TODO(dgryski): clean up code

const (
	maxCopy     = 32
	maxLen      = 264 /* 256 + 8 */
	maxDistance = 8192
)

const (
	hashLog  = 13
	hashSize = (1 << hashLog)
	hashMask = (hashSize - 1)
)

var ErrCorrupt = errors.New("corrupt input")

func readu16(p []byte, i uint32) uint16 {
	return uint16(p[i]) + (uint16(p[i+1]) << 8)
}

func hash(p []byte, i uint32) uint32 {
	v := readu16(p, i)
	v ^= readu16(p, i+1) ^ (v >> (16 - hashLog))
	v &= hashMask
	return uint32(v)
}

func Compress(input []byte) []byte {

	ip := input

	var ip_index uint32
	var length = uint32(len(ip))

	var ip_bound_index uint32 = length - 2
	var ip_limit_index uint32 = length - 12
	var op = make([]byte, int(float64(len(ip)+50)*1.4))
	var op_index uint32 = 0

	var htab []uint32
	var hslot_index uint32 = 0
	var hval uint32 = 0

	var cpy uint32

	/* sanity check */
	if length < 4 {
		if length == 0 {
			return nil
		}

		/* create literal copy only */
		op[op_index] = byte(length) - 1
		op_index++
		ip_bound_index++
		for ip_index <= ip_bound_index {
			op[op_index] = ip[ip_index]
			op_index++
			ip_index++
		}
		return op[:length+1]
	}

	/* initializes hash table */
	htab = make([]uint32, hashSize)

	/* we start with literal copy */
	cpy = 2
	op[op_index] = maxCopy - 1
	op_index++
	op[op_index] = ip[ip_index]
	op_index++
	ip_index++
	op[op_index] = ip[ip_index]
	op_index++
	ip_index++

	/* main loop */
	for ip_index < ip_limit_index {

		var ref_index uint32 = 0
		var distance uint32 = 0

		/* minimum match length */
		var ln uint32 = 3

		/* comparison starting-point */
		var anchor_index = uint32(ip_index)

		/* find potential match */
		hval = hash(ip, ip_index)
		hslot_index = hval
		ref_index = htab[hval]

		/* calculate distance to the match */
		distance = anchor_index - ref_index

		/* update hash table */
		htab[hslot_index] = anchor_index

		/* is this a match? check the first 3 bytes */
		if distance == 0 ||
			(distance >= maxDistance) ||
			ip[ref_index] != ip[ip_index] || ip[ref_index+1] != ip[ip_index+1] || ip[ref_index+2] != ip[ip_index+2] {
			/* goto literal: */
			op[op_index] = ip[anchor_index]
			op_index++
			anchor_index++
			ip_index = anchor_index
			cpy++
			if cpy == maxCopy {
				cpy = 0
				op[op_index] = maxCopy - 1
				op_index++
			}
			continue
		}

		/* last matched byte */
		ref_index += ln
		ip_index = anchor_index + ln

		/* distance is biased */
		distance--

		if distance == 0 {
			/* zero distance means a run */
			var x = ip[ip_index-1]
			for ip_index < ip_bound_index {
				if ip[ref_index] != x {
					break
				} else {
					ip_index++
				}
				ref_index++
			}
		} else {
			for ip_index < ip_bound_index && ip[ref_index] == ip[ip_index] {
				ref_index++
				ip_index++
			}
			if ip_index < ip_bound_index {
				ip_index++
			}
		}

		/* if we have copied something, adjust the copy count */
		if cpy != 0 {
			/* copy is biased, '0' means 1 byte copy */
			op[op_index-cpy-1] = byte(cpy) - 1
		} else {
			/* back, to overwrite the copy count */
			op_index--
		}

		/* reset literal counter */
		cpy = 0

		/* length is biased, '1' means a match of 3 bytes */
		ip_index -= 3
		ln = ip_index - anchor_index

		/* encode the match */
		for ln > maxLen-2 {
			op[op_index] = (7 << 5) + byte(distance>>8)
			op_index++
			op[op_index] = (maxLen - 2 - 7 - 2)
			op_index++
			op[op_index] = byte(distance)
			op_index++
			ln -= maxLen - 2
		}
		if ln < 7 {
			op[op_index] = byte(ln<<5) + byte(distance>>8)
			op_index++
			op[op_index] = byte(distance)
			op_index++
		} else {
			op[op_index] = (7 << 5) + byte(distance>>8)
			op_index++
			op[op_index] = byte(ln - 7)
			op_index++
			op[op_index] = byte(distance)
			op_index++
		}

		/* update the hash at match boundary */
		hval = hash(ip, ip_index)
		htab[hval] = ip_index
		ip_index++
		hval = hash(ip, ip_index)
		htab[hval] = ip_index
		ip_index++

		/* assuming literal copy */
		op[op_index] = maxCopy - 1
		op_index++
	}

	/* left-over as literal copy */
	ip_bound_index++
	for ip_index <= ip_bound_index {
		op[op_index] = ip[ip_index]
		op_index++
		ip_index++
		cpy++
		if cpy == maxCopy {
			cpy = 0
			op[op_index] = maxCopy - 1
			op_index++
		}
	}

	/* if we have copied something, adjust the copy length */
	if cpy != 0 {
		op[op_index-cpy-1] = byte(cpy) - 1
	} else {
		op_index--
	}

	return op[:op_index]
}

func Decompress(input []byte, maxout int) ([]byte, error) {

	length := len(input)
	ip := input
	var ip_index uint32

	ip_limit_index := uint32(length)
	op := make([]byte, maxout)
	var op_index uint32
	op_limit_index := uint32(maxout)

	var ctrl = ip[ip_index] & 31
	ip_index++
	var loop = true

	for loop {
		var ref_index = op_index
		var ln uint32 = uint32(ctrl >> 5)
		var ofs uint32 = uint32(ctrl&31) << 8

		if ctrl >= 32 {
			ln--
			ref_index -= ofs
			if ln == 7-1 {
				ln += uint32(ip[ip_index])
				ip_index++
			}
			ref_index -= uint32(ip[ip_index])
			ip_index++

			if op_index+ln+3 > op_limit_index {
				return nil, ErrCorrupt
			}

			if ref_index > op_limit_index {
				// really want to check if  ref_index is <0, but unsigned makes it tricky
				return nil, ErrCorrupt
			}

			if ip_index < ip_limit_index {
				ctrl = byte(ip[ip_index])
				ip_index++
			} else {
				loop = false
			}

			if ref_index == op_index {
				/* optimize copy for a run */
				var b uint8 = op[ref_index-1]
				op[op_index] = b
				op_index++
				op[op_index] = b
				op_index++
				op[op_index] = b
				op_index++
				for ; ln != 0; ln-- {
					op[op_index] = b
					op_index++
				}
			} else {
				/* copy from reference */
				ref_index--

				op[op_index] = op[ref_index]
				op_index++
				ref_index++

				op[op_index] = op[ref_index]
				op_index++
				ref_index++

				op[op_index] = op[ref_index]
				op_index++
				ref_index++

				for ; ln != 0; ln-- {
					op[op_index] = op[ref_index]
					op_index++
					ref_index++
				}
			}
		} else {
			ctrl++

			if op_index+uint32(ctrl) > op_limit_index {
				return nil, ErrCorrupt
			}

			if ip_index+uint32(ctrl) > ip_limit_index {
				return nil, ErrCorrupt
			}

			op[op_index] = ip[ip_index]
			op_index++
			ip_index++

			for ctrl--; ctrl > 0; ctrl-- {
				op[op_index] = ip[ip_index]
				op_index++
				ip_index++
			}

			loop = ip_index < ip_limit_index
			if loop {
				ctrl = ip[ip_index]
				ip_index++
			}
		}
	}

	return op[:op_index], nil
}
