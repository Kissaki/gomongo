// Copyright 2010 The 'gomongo' Authors.  All rights reserved.
// Use of this source code is governed by the New BSD License
// that can be found in the LICENSE file.

package mongo

import (
	"encoding/binary"
	"rand"
	crand "crypto/rand"
)


// Like BSON documents, all data in the mongo wire protocol is little-endian.
var pack = binary.LittleEndian

var (
	// Initialized to zero.
	_WORD32 = make([]byte, 4)
	_WORD64 = make([]byte, 8)
)

var lastRequestID int32


func init() {
	// Uses the 'urandom' device to get a seed which will be used by 'rand'.
	randombytes := make([]byte, 8)
	if _, err := crand.Read(randombytes); err != nil {
		panic("Pseudo-random source malfunction!")
	}

	random := binary.LittleEndian.Uint64(randombytes)
	// If you seed it with something predictable like the time, the risk is obvious.
	// rand.Seed(time.Nanoseconds())
	rand.Seed(int64(random))
}


// *** Utility functions
// ***

/* Gets a random request identifier different to the last one.
 */
func getRequestID() int32 {
	id := rand.Int31()

	if id == lastRequestID {
		return getRequestID()
	}
	lastRequestID = id

	return id
}

// *** Bits data

func setBit32(num *int32, position ...byte) {
	const MASK = 1

	for _, pos := range position {
		*num |= MASK << pos
	}
}
