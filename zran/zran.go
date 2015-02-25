// Package zran is a go implementation of zran by Mark Adler
// (https://github.com/madler/zlib/blob/master/examples/zran.c). Zran takes a
// compressed gzip file. This stream is decoded in its entirety, and an index
// is built with access points about every span bytes in the uncompressed
// output. The compressed file is left open, and can be read randomly, having
// to decompress on the average SPAN/2 uncompressed bytes before getting to the
// desired block of data.
package zran

import (
	"bufio"
	"io"
	"os"

	"github.com/coreos/pkg/zran/flate"
	"github.com/coreos/pkg/zran/gzip"
)

const (
	span = 1 << 20 // 1M -- min distance between access points in uncompressed output
)

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

// Index stores access points into compressed file. Span will determine the
// balance between the speed of random access against the memory requirements
// of the index.
type Index []*point

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

// Restores decompressor to equivalent state as when index access point was taken
//func inflateResume(r io.Reader, pt *point) io.ReadCloser {
func inflateResume(pt *point) *flate.Decompressor {
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
		Step:     pt.step,
		Final:    pt.final,
		Err:      pt.err,
		CopyLen:  pt.copyLen,
		CopyDist: pt.copyDist,
	}
	// restore hl and hd
	if pt.hl != nil {
		if pt.hl == &pt.h1 {
			f.Hl = &f.H1
		} else {
			f.Hl = &flate.HuffmanDecoder{
				Min:      pt.hl.Min,
				Chunks:   pt.hl.Chunks,
				LinkMask: pt.hl.LinkMask,
				Links:    make([][]uint32, len(pt.hl.Links)),
			}
			for i := range pt.hl.Links {
				f.Hl.Links[i] = append(f.Hl.Links[i], pt.hl.Links[i]...)
			}
		}
	}
	if pt.hd != nil {
		if pt.hd == &pt.h2 {
			f.Hd = &f.H2
		} else {
			f.Hd = &flate.HuffmanDecoder{
				Min:      pt.hd.Min,
				Chunks:   pt.hd.Chunks,
				LinkMask: pt.hd.LinkMask,
				Links:    make([][]uint32, len(pt.hd.Links)),
			}
			for i := range pt.hd.Links {
				f.Hd.Links[i] = append(f.Hd.Links[i], pt.hd.Links[i]...)
			}
		}
	}

	// restore HuffmanDecoders
	for i := range pt.h1.Links {
		f.H1.Links[i] = append(f.H1.Links[i], pt.h1.Links[i]...)
	}
	for i := range pt.h2.Links {
		f.H2.Links[i] = append(f.H2.Links[i], pt.h2.Links[i]...)
	}

	// restore hist buf, bits, and codebits
	copy(f.Hist[:], pt.hist[:])
	copy(f.Bits[:], pt.bits[:])
	copy(f.Codebits[:], pt.codebits[:])

	return &f
}

// ReadSpan reads blocks until span size or greater bytes have been read.
func readSpan(gr *gzip.Reader) ([]byte, error) {
	var err error
	var block, buf []byte

	for len(buf) < span {
		block, err = gr.ReadBlock()
		if err != nil {
			return block, err
		}
		buf = append(buf, block...)
	}
	return buf, nil
}

// BuildIndex decompresses given file and builds an index that records the
// state of the gzip decompresser every Span bytes into the uncompressed data.
// This index can be used to randomly read uncompressed data from the
// compressed file. Data after the end of the first gzip stream in the
// file is ignored and so concatenated gzip files are not supported.
func BuildIndex(file string) (Index, error) {
	in, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer in.Close()

	// Read Gzip Header and init our flate.Reader as well
	rr, err := gzip.NewReader(in)
	if err != nil {
		return nil, err
	}

	// Concatenated gzips not supported,
	rr.Multistream(false)

	// Access point before first block
	idx := new(Index)
	idx.addPoint(rr, 0)

	// Create access point in index about every span
	var totalRead int64
	var b []byte
	for {
		b = nil
		b, err = readSpan(rr)
		// Don't create index after last block
		if err == io.EOF {
			return *idx, nil
		} else if err != nil {
			return nil, err
		}

		totalRead += int64(len(b))
		idx.addPoint(rr, totalRead)
	}
}

// Extract uses an Index to read length bytes from offset into uncompressed
// data. If data is requested past the end of the uncompressed data, Extract
// will read less bytes then length and return io.EOF. Offset is zero indexed.
func Extract(filename string, idx Index, offset int64, length int64) ([]byte, error) {
	in, err := os.Open(filename)
	if err != nil {
		return []byte{}, err
	}
	defer in.Close()
	if length <= 0 || idx == nil {
		return []byte{}, nil
	}

	// find access point
	var pt *point
	for i := len(idx) - 1; i >= 0; i-- {
		if idx[i].woffset <= offset {
			pt = idx[i]
			break
		}
	}

	// Read gzip Header to find how many bytes are in the header
	gr, err := gzip.NewReader(in)
	if err != nil {
		return []byte{}, err
	}
	//gr.Close()

	// calculate header length
	headerBytes := 10 //Initial header length
	if gr.Flg&gzip.FlagName != 0 {
		headerBytes += len(gr.Header.Name) + 1 // +1 for null term
	}
	if gr.Flg&gzip.FlagExtra != 0 {
		headerBytes += len(gr.Header.Extra) + 2 // +2 for XLEN
	}
	if gr.Flg&gzip.FlagComment != 0 {
		headerBytes += len(gr.Header.Comment) + 1 // +1 for null term
	}
	if gr.Flg&gzip.FlagHdrCrc != 0 {
		headerBytes += 2
	}

	// set file to start of block (roffset + header length)
	_, err = in.Seek(pt.roffset+int64(headerBytes), 0)
	if err != nil {
		return []byte{}, err
	}

	// restore decompresser state
	fr := inflateResume(pt)
	fr.R = bufio.NewReader(in)

	// inflate until offset - woffset + bytes have been read, or end of file
	b := make([]byte, length+offset-pt.woffset)
	readWin := b
	var totalRead int
	for {
		n, err := fr.Read(readWin)
		totalRead += n
		readWin = readWin[n:]

		// finished file or read enough
		if n == 0 || totalRead == len(b) || err == io.EOF {
			return b[offset-pt.woffset:], err
		}
	}
}
