// Package zran is a go implementation of zran by Mark Adler
// (https://github.com/madler/zlib/blob/master/examples/zran.c). Zran takes a
// compressed gzip file. This stream is decoded in its entirety, and an index is
// built with access points about every span bytes in the uncompressed output.
// The compressed file is left open, and can be read randomly, having to
// decompress on the average SPAN/2 uncompressed bytes before getting to the
// desired block of data.
package zran

import (
	"fmt"
	"io"
	"os"

	"github.com/coreos/pkg/zran/flate"
	"github.com/coreos/pkg/zran/gzip"
)

const (
	span  = 1 << 20 // 1M  -- desired distance between access points in uncompressed output
	chunk = 1 << 14 // 16k -- file input buffer size
)

// Point mimics flate.Decompressor for restoration of decoder state needed for
// random access. This probably saves more state then strictly necessary.
type point struct {
	roffset int64 // Export offset into input
	woffset int64 // Export offset into output

	// Input bits, in top of b.
	b  uint32
	nb uint
	// Huffman decoders for literal/length, distance.
	h1, h2 flate.HuffmanDecoder

	// Length arrays used to define Huffman codes.
	bits     *[flate.MaxLit + flate.MaxDist]int
	codebits *[flate.NumCodes]int

	// Output history, buffer.
	hist  *[flate.MaxHist]byte // Export history
	hp    int                  // current output position in buffer
	hw    int                  // have written hist[0:hw] already
	hfull bool                 // buffer has filled at least once

	// Temporary buffer (avoids repeated allocation).
	buf [4]byte

	final    bool
	copyLen  int
	copyDist int
}

// Index stores access points into compressed file. Span will determine the
// balance between the speed of random access against the memory requirements
// of the index. This may err on the side of replicating more of decompressor
// state then necessary.
type Index []point

func (i *Index) addPoint(rr *gzip.Reader, n int64) {
	// convert from io.ReadCloser to flate.Decompresser
	r := rr.Decompressor.(*flate.Decompressor)
	// Sanity check: does Decompresser Woffset equal accumulated n from readSpan()?
	if r.Woffset != n {
		panic("Inconsistant Decompressor state, Woffset doesn't represent accumulated readSpan() calls")
	}
	// Sanity check: Decompressor has nothing in toRead
	if len(r.ToRead) != 0 {
		panic("toRead not zero")
	}

	// save decompressor state
	pt := point{
		woffset: r.Woffset,
		roffset: r.Roffset,
		b:       r.B,
		nb:      r.Nb,
		h1: flate.HuffmanDecoder{
			Min:      r.H1.Min,
			Chunks:   r.H1.Chunks,
			LinkMask: r.H1.LinkMask,
			Links:    make([][]uint32, len(r.H1.Links)),
		},
		h2: flate.HuffmanDecoder{
			Min:      r.H2.Min,
			Chunks:   r.H2.Chunks,
			LinkMask: r.H2.LinkMask,
			Links:    make([][]uint32, len(r.H2.Links)),
		},
		bits:     new([flate.MaxLit + flate.MaxDist]int),
		codebits: new([flate.NumCodes]int),
		hist:     new([flate.MaxHist]byte),
		hp:       r.Hp,
		hw:       r.Hw,
		hfull:    r.Hfull,
		buf:      r.Buf,
		final:    r.Final,
		copyLen:  r.CopyLen,
		copyDist: r.CopyDist,
	}
	// deep copy h1.Links
	for i := range r.H2.Links {
		pt.h1.Links[i] = append(pt.h1.Links[i], r.H1.Links[i]...)
	}
	// deep copy h2.Links
	for i := range r.H2.Links {
		pt.h2.Links[i] = append(pt.h2.Links[i], r.H2.Links[i]...)
	}
	// deep copy hist buf
	copy(pt.hist[:], r.Hist[:])

	// add point to index
	*i = append(*i, pt)
}

// Restores decompressor to equivalent state as when index access point was taken
func inflateResume(r io.Reader, pt point) io.ReadCloser {
	f := flate.Decompressor{
		Woffset: pt.woffset,
		Roffset: pt.roffset,
		B:       pt.b,
		Nb:      pt.nb,
		H1: flate.HuffmanDecoder{
			Min:      pt.h1.Min,
			Chunks:   pt.h1.Chunks,
			LinkMask: pt.h1.LinkMask,
			Links:    make([][]uint32, len(pt.h1.Links)),
		},
		H2: flate.HuffmanDecoder{
			Min:      pt.h2.Min,
			Chunks:   pt.h2.Chunks,
			LinkMask: pt.h2.LinkMask,
			Links:    make([][]uint32, len(pt.h2.Links)),
		},
		Bits:     new([flate.MaxLit + flate.MaxDist]int),
		Codebits: new([flate.NumCodes]int),
		Hist:     new([flate.MaxHist]byte),
		Hp:       pt.hp,
		Hw:       pt.hw,
		Hfull:    pt.hfull,
		Buf:      pt.buf,
		Final:    pt.final,
		CopyLen:  pt.copyLen,
		CopyDist: pt.copyDist,
	} // deep copy h1.Links
	for i := range pt.h2.Links {
		f.H1.Links[i] = append(f.H1.Links[i], pt.h1.Links[i]...)
	}
	// deep copy h2.Links
	for i := range pt.h2.Links {
		f.H2.Links[i] = append(f.H2.Links[i], pt.h2.Links[i]...)
	}
	// deep copy hist buf
	copy(f.Hist[:], pt.hist[:])

	f.R = flate.MakeReader(r)
	f.Step = (*flate.Decompressor).NextBlock
	return &f
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
		if idx[i].woffset <= offset {
			pt = idx[i]
			break
		}
	}

	// set file to start of block
	_, err = in.Seek(pt.roffset, 0)
	if err != nil {
		return []byte{}, err
	}

	// restore decompressor state
	rr := inflateResume(in, pt)

	// inflate until out of input, offset - offsetOut + len(b) bytes have been
	// read, or end of file
	b := make([]byte, length+offset-pt.woffset)
	for {

		n, err := rr.Read(b)
		fmt.Printf("Read %v bytes", n)
		if err != nil || n == 0 {
			return b, err
		}
	}
}
