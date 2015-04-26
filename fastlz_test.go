package fastlz

import (
	"bytes"
	"io/ioutil"
	"math/rand"
	"testing"

	cfastlz "github.com/fromYukki/fastlz"
)

func testRoundtripCgo(t *testing.T, in []byte) {
	cz, _ := cfastlz.Compress(in)
	gz, _ := Encode(nil, in)

	// gz[4:] because we add a 4-byte length header
	if !bytes.Equal(cz, gz[4:]) {
		offs := dump(t, "gz", gz, "cz", cz)
		t.Fatalf("compression mismatch for length %d at offs %x", len(in), offs)
	}

	o, _ := Decode(nil, gz)
	if !bytes.Equal(in, o) {
		offs := dump(t, "o", o, "i", in)
		t.Fatalf("roundtrip mismatch for length %d at offs %x", len(in), offs)
	}
}

func testRoundtrip(t *testing.T, in []byte) {
	gz, _ := Encode(nil, in)
	o, _ := Decode(nil, gz)
	if !bytes.Equal(in, o) {
		offs := dump(t, "o", o, "i", in)
		t.Fatalf("roundtrip mismatch for length %d at offs %x", len(in), offs)
	}
}

func TestCompress(t *testing.T) {

	in, _ := ioutil.ReadFile("testdata/domains.txt")

	for i := 16; i < 10240; i++ {
		testRoundtrip(t, in[:i])
	}

	for i := 0; i < 10240; i++ {
		// limit the length.  The buffer must be >16 characters, and if
		// it's >64 then the C library switches to compression level 2,
		// which we haven't implemented.
		ln := rand.Intn(65536-16) + 16
		testRoundtripCgo(t, in[:ln])
	}
}

func TestRoundtrip(t *testing.T) {

	in, _ := ioutil.ReadFile("testdata/domains.txt")

	for i := 0; i < 10240; i++ {
		ln := rand.Intn(len(in)-16) + 16
		testRoundtrip(t, in[:ln])
	}
}

func dump(t *testing.T, s1 string, b1 []byte, s2 string, b2 []byte) int {
	/*
		t.Log("\n" + hex.Dump(b1[:256]))
		t.Log("\n" + hex.Dump(b2[:256]))
	*/
	for i, v := range b1 {
		if b2[i] != v {
			return i
		}
	}
	return -1
}
