// Package gzran is a go implementation of zran by Mark Adler
// (https://github.com/madler/zlib/blob/master/examples/zran.c). Gzran takes a
// compressed gzip file. This stream is decoded in its entirety, and an index
// is built with access points about every span bytes in the uncompressed
// output. The compressed file is left open, and can be read randomly, having
// to decompress on the average SPAN/2 uncompressed bytes before getting to the
// desired block of data.
package gzran

import (
	"io"
	"os"

	"github.com/coreos/pkg/gzran/flate"
	"github.com/coreos/pkg/gzran/gzip"
)

const (
	span = 1 << 20 // 1M -- min distance between access points in uncompressed output
)

// Index stores access points into compressed file. Span will determine the
// balance between the speed of random access against the memory requirements
// of the index.
type Index []*point

// Point mimics flate.Decompressor for restoration of decoder state needed for
// random access. This probably saves more state then strictly necessary.
type point struct {
	roffset int64
	woffset int64

	// Input bits, in top of b.
	b  uint32
	nb uint
	// Huffman decoders for literal/length, distance.
	h1, h2 flate.HuffmanDecoder

	// Length arrays used to define Huffman codes.
	bits     *[flate.MaxLit + flate.MaxDist]int
	codebits *[flate.NumCodes]int

	// Output history, buffer.
	hist  *[flate.MaxHist]byte
	hp    int  // current output position in buffer
	hw    int  // have written hist[0:hw] already
	hfull bool // buffer has filled at least once

	// Temporary buffer (avoids repeated allocation).
	buf [4]byte

	step     func(*flate.Decompressor)
	final    bool
	err      error
	hl, hd   *flate.HuffmanDecoder
	copyLen  int
	copyDist int
}

func (idx *Index) addPoint(gr *gzip.Reader, n int64) {
	// convert from io.ReadCloser to flate.Decompresser
	r := gr.Decompressor.(*flate.Decompressor)

	// save decompressor state
	pt := &point{
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
		step:     r.Step,
		final:    r.Final,
		err:      r.Err,
		copyLen:  r.CopyLen,
		copyDist: r.CopyDist,
	}

	// save hl and hd
	if r.Hl != nil {
		if r.Hl == &r.H1 {
			pt.hl = &pt.h1
		} else {
			pt.hl = &flate.HuffmanDecoder{
				Min:      r.Hl.Min,
				Chunks:   r.Hl.Chunks,
				LinkMask: r.Hl.LinkMask,
				Links:    make([][]uint32, len(r.Hl.Links)),
			}
			for i := range r.Hl.Links {
				pt.hl.Links[i] = append(pt.hl.Links[i], r.Hl.Links[i]...)
			}
		}
	}
	if r.Hd != nil {
		if r.Hd == &r.H2 {
			pt.hd = &pt.h2
		} else {
			pt.hd = &flate.HuffmanDecoder{
				Min:      r.Hd.Min,
				Chunks:   r.Hd.Chunks,
				LinkMask: r.Hd.LinkMask,
				Links:    make([][]uint32, len(r.Hd.Links)),
			}
			for i := range r.Hd.Links {
				pt.hd.Links[i] = append(pt.hd.Links[i], r.Hd.Links[i]...)
			}
		}

	}
	// copy HuffmanDecoders
	if r.H1.Links != nil {
		for i := range r.H1.Links {
			pt.h1.Links[i] = append(pt.h1.Links[i], r.H1.Links[i]...)
		}
	} else {
		pt.h1.Links = nil
	}
	if r.H2.Links != nil {
		for i := range r.H2.Links {
			pt.h2.Links[i] = append(pt.h2.Links[i], r.H2.Links[i]...)
		}
	} else {
		pt.h2.Links = nil
	}
	// copy hist buf, bits, and codebits
	copy(pt.hist[:], r.Hist[:])
	copy(pt.bits[:], r.Bits[:])
	copy(pt.codebits[:], r.Codebits[:])

	// add point to index
	*idx = append(*idx, pt)
}

// replacement (and copy of) of flate.Read(): always allocates a buffer big
// enough to fully return the block of uncompressed data
func readBlock(f *flate.Decompressor) ([]byte, error) {
	for {
		if len(f.ToRead) > 0 {
			//allocate b to be same size as toRead
			b := make([]byte, len(f.ToRead))

			n := copy(b, f.ToRead)
			f.ToRead = f.ToRead[n:]
			return b, nil
		}
		if f.Err != nil {
			return []byte{}, f.Err
		}
		f.Step(f)
	}
}

// replace (and copy) gzip.Read() to use our own readBlock(). multistream concatenated
// gzips are ignored. mostly a copy of gzip.Read()
func gzReadBlock(z *gzip.Reader) ([]byte, error) {
	if z.Err != nil {
		return []byte{}, z.Err
	}

	zr := z.Decompressor.(*flate.Decompressor) // io.ReadCloser -> flate.Decompressor
	b, err := readBlock(zr)

	z.Digest.Write(b)
	z.Size += uint32(len(b))
	if len(b) != 0 || err != io.EOF {
		z.Err = err
		return b, err
	}

	// Finished file; check checksum + size.
	if _, err := io.ReadFull(z.R, z.Buf[0:8]); err != nil {
		z.Err = err
		return []byte{}, err
	}
	crc32, isize := gzip.Get4(z.Buf[0:4]), gzip.Get4(z.Buf[4:8])
	sum := z.Digest.Sum32()
	if sum != crc32 || isize != z.Size {
		z.Err = gzip.ErrChecksum
		return []byte{}, z.Err
	}

	// multistream not supported
	return []byte{}, io.EOF
}

// readSpan reads blocks until span size or greater bytes have been read.
func readSpan(gz *gzip.Reader) ([]byte, error) {
	var err error
	var block, buf []byte

	for len(buf) < span {
		block, err = gzReadBlock(gz)
		if err != nil {
			return block, err
		}
		buf = append(buf, block...)
	}
	return buf, nil
}

// BuildIndex decompresses given file and builds an index that records the
// state of the gzip decompresser every span bytes into the uncompressed data.
// This index can be used to randomly read uncompressed data from the
// compressed file. Data after the end of the first gzip stream in the
// file is ignored and so concatenated gzip files are not supported.
func BuildIndex(file string) (Index, error) {
	in, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer in.Close()

	// read Gzip Header and init our flate.Reader as well
	rr, err := gzip.NewReader(in)
	if err != nil {
		return nil, err
	}

	// concatenated gzips not supported,
	rr.Multistream(false)

	// access point before first block
	idx := new(Index)
	idx.addPoint(rr, 0)

	// create access point in index about every span
	var totalRead int64
	var b []byte
	for {
		b = nil
		b, err = readSpan(rr)
		// don't create index after last block
		if err == io.EOF {
			return *idx, nil
		} else if err != nil {
			return nil, err
		}

		totalRead += int64(len(b))
		idx.addPoint(rr, totalRead)
	}
}
