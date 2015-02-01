// Package fastlz implements FastLZ compression
/*

Currently only level 1 compression/decompression (fastest) is supported.

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

	hashLog  = 13
	hashSize = (1 << hashLog)
	hashMask = (hashSize - 1)
)

// ErrCorrupt indicates a corrupt input stream
var ErrCorrupt = errors.New("corrupt input")

// Compress compresses the input and returns the compressed byte stream
func Compress(input []byte) []byte {

	ip := input

	var ipIdx uint32
	var length = uint32(len(ip))

	var ipBoundIdx = length - 2
	var ipLimitIdx = length - 12
	var op = make([]byte, int(float64(len(ip)+50)*1.4))
	var opIdx uint32

	var htab []uint32
	var hslotIdx uint32
	var hval uint32

	var cpy uint32

	/* sanity check */
	if length < 4 {
		if length == 0 {
			return nil
		}

		/* create literal copy only */
		op[opIdx] = byte(length) - 1
		opIdx++
		ipBoundIdx++
		for ipIdx <= ipBoundIdx {
			op[opIdx] = ip[ipIdx]
			opIdx++
			ipIdx++
		}
		return op[:length+1]
	}

	/* initializes hash table */
	htab = make([]uint32, hashSize)

	/* we start with literal copy */
	cpy = 2
	op[opIdx] = maxCopy - 1
	opIdx++
	op[opIdx] = ip[ipIdx]
	opIdx++
	ipIdx++
	op[opIdx] = ip[ipIdx]
	opIdx++
	ipIdx++

	/* main loop */
	for ipIdx < ipLimitIdx {

		var refIdx uint32
		var distance uint32

		/* minimum match length */
		var ln uint32 = 3

		/* comparison starting-point */
		var anchorIdx = uint32(ipIdx)

		/* find potential match */
		hval = hash(ip, ipIdx)
		hslotIdx = hval
		refIdx = htab[hval]

		/* calculate distance to the match */
		distance = anchorIdx - refIdx

		/* update hash table */
		htab[hslotIdx] = anchorIdx

		/* is this a match? check the first 3 bytes */
		if distance == 0 ||
			(distance >= maxDistance) ||
			ip[refIdx] != ip[ipIdx] || ip[refIdx+1] != ip[ipIdx+1] || ip[refIdx+2] != ip[ipIdx+2] {
			/* goto literal: */
			op[opIdx] = ip[anchorIdx]
			opIdx++
			anchorIdx++
			ipIdx = anchorIdx
			cpy++
			if cpy == maxCopy {
				cpy = 0
				op[opIdx] = maxCopy - 1
				opIdx++
			}
			continue
		}

		/* last matched byte */
		refIdx += ln
		ipIdx = anchorIdx + ln

		/* distance is biased */
		distance--

		if distance == 0 {
			/* zero distance means a run */
			var x = ip[ipIdx-1]
			for ipIdx < ipBoundIdx {
				if ip[refIdx] != x {
					break
				} else {
					ipIdx++
				}
				refIdx++
			}
		} else {
			for ipIdx < ipBoundIdx && ip[refIdx] == ip[ipIdx] {
				refIdx++
				ipIdx++
			}
			if ipIdx < ipBoundIdx {
				ipIdx++
			}
		}

		/* if we have copied something, adjust the copy count */
		if cpy != 0 {
			/* copy is biased, '0' means 1 byte copy */
			op[opIdx-cpy-1] = byte(cpy) - 1
		} else {
			/* back, to overwrite the copy count */
			opIdx--
		}

		/* reset literal counter */
		cpy = 0

		/* length is biased, '1' means a match of 3 bytes */
		ipIdx -= 3
		ln = ipIdx - anchorIdx

		/* encode the match */
		for ln > maxLen-2 {
			op[opIdx] = (7 << 5) + byte(distance>>8)
			opIdx++
			op[opIdx] = (maxLen - 2 - 7 - 2)
			opIdx++
			op[opIdx] = byte(distance)
			opIdx++
			ln -= maxLen - 2
		}
		if ln < 7 {
			op[opIdx] = byte(ln<<5) + byte(distance>>8)
			opIdx++
			op[opIdx] = byte(distance)
			opIdx++
		} else {
			op[opIdx] = (7 << 5) + byte(distance>>8)
			opIdx++
			op[opIdx] = byte(ln - 7)
			opIdx++
			op[opIdx] = byte(distance)
			opIdx++
		}

		/* update the hash at match boundary */
		hval = hash(ip, ipIdx)
		htab[hval] = ipIdx
		ipIdx++
		hval = hash(ip, ipIdx)
		htab[hval] = ipIdx
		ipIdx++

		/* assuming literal copy */
		op[opIdx] = maxCopy - 1
		opIdx++
	}

	/* left-over as literal copy */
	ipBoundIdx++
	for ipIdx <= ipBoundIdx {
		op[opIdx] = ip[ipIdx]
		opIdx++
		ipIdx++
		cpy++
		if cpy == maxCopy {
			cpy = 0
			op[opIdx] = maxCopy - 1
			opIdx++
		}
	}

	/* if we have copied something, adjust the copy length */
	if cpy != 0 {
		op[opIdx-cpy-1] = byte(cpy) - 1
	} else {
		opIdx--
	}

	return op[:opIdx]
}

// Decompress decompresses input and returns the original data
func Decompress(input []byte, maxout int) ([]byte, error) {

	length := len(input)
	ip := input
	var ipIdx uint32

	ipLimitIdx := uint32(length)
	op := make([]byte, maxout)
	var opIdx uint32
	opLimitIdx := uint32(maxout)

	var ctrl = ip[ipIdx] & 31
	ipIdx++
	var loop = true

	for loop {
		var refIdx = opIdx
		var ln = uint32(ctrl >> 5)
		var ofs = uint32(ctrl&31) << 8

		if ctrl >= 32 {
			ln--
			refIdx -= ofs
			if ln == 7-1 {
				ln += uint32(ip[ipIdx])
				ipIdx++
			}
			refIdx -= uint32(ip[ipIdx])
			ipIdx++

			if opIdx+ln+3 > opLimitIdx {
				return nil, ErrCorrupt
			}

			if refIdx > opLimitIdx {
				// really want to check if  refIdx is <0, but unsigned makes it tricky
				return nil, ErrCorrupt
			}

			if ipIdx < ipLimitIdx {
				ctrl = byte(ip[ipIdx])
				ipIdx++
			} else {
				loop = false
			}

			if refIdx == opIdx {
				/* optimize copy for a run */
				var b = op[refIdx-1]
				op[opIdx] = b
				opIdx++
				op[opIdx] = b
				opIdx++
				op[opIdx] = b
				opIdx++
				for ; ln != 0; ln-- {
					op[opIdx] = b
					opIdx++
				}
			} else {
				/* copy from reference */
				refIdx--

				op[opIdx] = op[refIdx]
				opIdx++
				refIdx++

				op[opIdx] = op[refIdx]
				opIdx++
				refIdx++

				op[opIdx] = op[refIdx]
				opIdx++
				refIdx++

				for ; ln != 0; ln-- {
					op[opIdx] = op[refIdx]
					opIdx++
					refIdx++
				}
			}
		} else {
			ctrl++

			if opIdx+uint32(ctrl) > opLimitIdx {
				return nil, ErrCorrupt
			}

			if ipIdx+uint32(ctrl) > ipLimitIdx {
				return nil, ErrCorrupt
			}

			op[opIdx] = ip[ipIdx]
			opIdx++
			ipIdx++

			for ctrl--; ctrl > 0; ctrl-- {
				op[opIdx] = ip[ipIdx]
				opIdx++
				ipIdx++
			}

			loop = ipIdx < ipLimitIdx
			if loop {
				ctrl = ip[ipIdx]
				ipIdx++
			}
		}
	}

	return op[:opIdx], nil
}

func readu16(p []byte, i uint32) uint16 {
	return uint16(p[i]) + (uint16(p[i+1]) << 8)
}

func hash(p []byte, i uint32) uint32 {
	v := readu16(p, i)
	v ^= readu16(p, i+1) ^ (v >> (16 - hashLog))
	v &= hashMask
	return uint32(v)
}
