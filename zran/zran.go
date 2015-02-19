// Package zran is a go implementation of zran by Mark Adler
// (https://github.com/madler/zlib/blob/master/examples/zran.c). Zran takes a
// compressed gzip file. This stream is decoded in its entirety, and an index is
// built with access points about every span bytes in the uncompressed output.
// The compressed file is left open, and can be read randomly, having to
// decompress on the average SPAN/2 uncompressed bytes before getting to the
// desired block of data.
package zran

import (
	"io"
	"os"

	"github.com/coreos/pkg/zran/flate"
	"github.com/coreos/pkg/zran/gzip"
)

const (
	span    = 1 << 20 // 1M  -- desired distance between access points in uncompressed output
	winSize = 1 << 15 // 32K -- sliding window size, max history required to build a dictionary for flate
	chunk   = 1 << 14 // 16k -- file input buffer size
)

type point struct {
	offsetOut int64 // offset into uncompressed data
	offsetIn  int64 // offset into input of first full byte
	// bits      int            // number of bits (1-7) from byte at -1, or 0
	// TODO: Do we need to save nb and b?
	window *[winSize]byte // preceding 32K of uncompressed data
}

// Index stores access points into compressed file. Span will determine the
// balance between the speed of random access against the memory requirements
// of the index.
type Index []point

func (i *Index) addPoint(rr *gzip.Reader, n int64) {
	// convert from io.ReadCloser to flate.Decompresser
	r := rr.Decompressor.(*flate.Decompressor)
	// Sanity check: does Decompresser Woffset equal accumulated n from readSpan()?
	if r.Woffset != n {
		panic("Inconsistant Decompressor state, Woffset doesn't represent accumulated readSpan() calls")
	}
	pt := point{
		offsetOut: r.Woffset,
		offsetIn:  r.Roffset,
		window:    new([winSize]byte),
	}
	copy(pt.window[:], r.Hist[:])
	*i = append(*i, pt)
}

// ReadSpan reads blocks until span size or greater bytes have been read.
func readSpan(rr *gzip.Reader) ([]byte, error) {
	var err error
	var block, buf []byte

	for len(buf) < span {
		block, err = rr.ReadBlock()
		if err != nil {
			return block, err
		}
		buf = append(buf, block...)
	}
	return buf, nil
}

// BuildIndex works by decompressing the gzip stream a block at a time, and at
// the end of each block deciding if enough uncompressed data has gone by to
// justify the creation of a new access point. If so, that point is saved in
// the Index. Access points created about every span bytes of uncompressed
// output.
// NOTE: Data after the end of the first gzip stream in the file is ignored.
func BuildIndex(filename string) (Index, error) {
	in, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer in.Close()

	// Read Gzip Header and init our flate.Reader as well
	rr, err := gzip.NewReader(in)
	if err != nil {
		return nil, err
	}

	// NOTE: will ACI's ever concatenate gzipped files? Looking at the spec it
	// appears to be a single tar file compressed not a multistream gzip file.

	// Concatenated gzips not supported,
	rr.Multistream(false)

	// Access point before first block
	idx := new(Index)
	idx.addPoint(rr, 0)

	// Create access point in index about every span
	var totalRead int64
	var b []byte
	for {
		b, err = readSpan(rr)
		if err == io.EOF {
			return *idx, nil

		} else if err != nil {
			return nil, err
		}
		totalRead += int64(len(b))
		idx.addPoint(rr, totalRead)
	}
}

// Extract uses an Index to read len(b) bytes from uncompressed offset into b. If
// data is requested past the end of the uncompressed data Extract will read
// less bytes then n and return io.EOF
func Extract(filename string, idx Index, offset int64, length int64) ([]byte, error) {
	in, err := os.Open(filename)
	if err != nil {
		return []byte{}, err
	}
	defer in.Close()
	if length <= 0 {
		return []byte{}, nil
	}

	// find access point
	var pt point
	for i := range idx {
		if idx[i].offsetOut <= offset {
			break
			pt = idx[i]
		}
	}

	//TODO: Subtract byte from seek if we have bits in previous byte as part of
	//block.  This is to replicate inflateprime from zlib. I think this may
	//be easiest to achieve by replicating and restoring more aspects of the
	//decompressor's state at index creation points.
	_, err = in.Seek(pt.offsetIn, 0)
	if err != nil {
		return []byte{}, err
	}

	rr := flate.NewReaderDict(in, pt.window[:])

	// inflate until out of input or offset - offsetOut + len(b) bytes have been read or end of file
	b := make([]byte, length+offset-pt.offsetOut)
	for {
		n, err := rr.Read(b)
		if err != nil || n == 0 {
			return b, err
		}
	}
}
