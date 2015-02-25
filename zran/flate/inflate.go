// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:generate go run gen.go -output fixedhuff.go

// Package flate implements the DEFLATE compressed data format, described in
// RFC 1951.  The gzip and zlib packages implement access to DEFLATE-based file
// formats.
package flate

import (
	"bufio"
	"io"
	"strconv"
)

const (
	maxCodeLen = 16    // max length of Huffman code
	MaxHist    = 32768 // max history required
	// The next three numbers come from the RFC, section 3.2.7.
	MaxLit   = 286
	MaxDist  = 32
	NumCodes = 19 // number of codes in Huffman meta-code
)

// A CorruptInputError reports the presence of corrupt input at a given offset.
type CorruptInputError int64

func (e CorruptInputError) Error() string {
	return "flate: corrupt input before offset " + strconv.FormatInt(int64(e), 10)
}

// An InternalError reports an error in the flate code itself.
type InternalError string

func (e InternalError) Error() string { return "flate: internal error: " + string(e) }

// A ReadError reports an error encountered while reading input.
type ReadError struct {
	Offset int64 // byte offset where error occurred
	Err    error // error returned by underlying Read
}

func (e *ReadError) Error() string {
	return "flate: read error at offset " + strconv.FormatInt(e.Offset, 10) + ": " + e.Err.Error()
}

// A WriteError reports an error encountered while writing output.
type WriteError struct {
	Offset int64 // byte offset where error occurred
	Err    error // error returned by underlying Write
}

func (e *WriteError) Error() string {
	return "flate: write error at offset " + strconv.FormatInt(e.Offset, 10) + ": " + e.Err.Error()
}

// Resetter resets a ReadCloser returned by NewReader or NewReaderDict to
// to switch to a new underlying Reader. This permits reusing a ReadCloser
// instead of allocating a new one.
type Resetter interface {
	// Reset discards any buffered data and resets the Resetter as if it was
	// newly initialized with the given reader.
	Reset(r io.Reader, dict []byte) error
}

// Note that much of the implementation of huffmanDecoder is also copied
// into gen.go (in package main) for the purpose of precomputing the
// fixed huffman tables so they can be included statically.

// The data structure for decoding Huffman tables is based on that of
// zlib. There is a lookup table of a fixed bit width (huffmanChunkBits),
// For codes smaller than the table width, there are multiple entries
// (each combination of trailing bits has the same value). For codes
// larger than the table width, the table contains a link to an overflow
// table. The width of each entry in the link table is the maximum code
// size minus the chunk width.

// Note that you can do a lookup in the table even without all bits
// filled. Since the extra bits are zero, and the DEFLATE Huffman codes
// have the property that shorter codes come before longer ones, the
// bit length estimate in the result is a lower bound on the actual
// number of bits.

// chunk & 15 is number of bits
// chunk >> 4 is value, including table link

const (
	huffmanChunkBits  = 9
	huffmanNumChunks  = 1 << huffmanChunkBits
	huffmanCountMask  = 15
	huffmanValueShift = 4
)

type HuffmanDecoder struct {
	Min      int                      // the minimum code length
	Chunks   [huffmanNumChunks]uint32 // chunks as described above
	Links    [][]uint32               // overflow links
	LinkMask uint32                   // mask the width of the link table
}

// Initialize Huffman decoding tables from array of code lengths.
func (h *HuffmanDecoder) init(bits []int) bool {
	if h.Min != 0 {
		*h = HuffmanDecoder{}
	}

	// Count number of codes of each length,
	// compute min and max length.
	var count [maxCodeLen]int
	var min, max int
	for _, n := range bits {
		if n == 0 {
			continue
		}
		if min == 0 || n < min {
			min = n
		}
		if n > max {
			max = n
		}
		count[n]++
	}
	if max == 0 {
		return false
	}

	h.Min = min
	var linkBits uint
	var numLinks int
	if max > huffmanChunkBits {
		linkBits = uint(max) - huffmanChunkBits
		numLinks = 1 << linkBits
		h.LinkMask = uint32(numLinks - 1)
	}
	code := 0
	var nextcode [maxCodeLen]int
	for i := min; i <= max; i++ {
		if i == huffmanChunkBits+1 {
			// create link tables
			link := code >> 1
			if huffmanNumChunks < link {
				return false
			}
			h.Links = make([][]uint32, huffmanNumChunks-link)
			for j := uint(link); j < huffmanNumChunks; j++ {
				reverse := int(reverseByte[j>>8]) | int(reverseByte[j&0xff])<<8
				reverse >>= uint(16 - huffmanChunkBits)
				off := j - uint(link)
				h.Chunks[reverse] = uint32(off<<huffmanValueShift + uint(i))
				h.Links[off] = make([]uint32, 1<<linkBits)
			}
		}
		n := count[i]
		nextcode[i] = code
		code += n
		code <<= 1
	}

	for i, n := range bits {
		if n == 0 {
			continue
		}
		code := nextcode[n]
		nextcode[n]++
		chunk := uint32(i<<huffmanValueShift | n)
		reverse := int(reverseByte[code>>8]) | int(reverseByte[code&0xff])<<8
		reverse >>= uint(16 - n)
		if n <= huffmanChunkBits {
			for off := reverse; off < huffmanNumChunks; off += 1 << uint(n) {
				h.Chunks[off] = chunk
			}
		} else {
			value := h.Chunks[reverse&(huffmanNumChunks-1)] >> huffmanValueShift
			if value >= uint32(len(h.Links)) {
				return false
			}
			linktab := h.Links[value]
			reverse >>= huffmanChunkBits
			for off := reverse; off < numLinks; off += 1 << uint(n-huffmanChunkBits) {
				linktab[off] = chunk
			}
		}
	}
	return true
}

// The actual read interface needed by NewReader.
// If the passed in io.Reader does not also have ReadByte,
// the NewReader will introduce its own buffering.
type Reader interface {
	io.Reader
	io.ByteReader
}

// Decompress state.
type Decompressor struct {
	// Input source.
	R       Reader
	Roffset int64 // Export offset into input
	Woffset int64 // Export offset into output

	// Input bits, in top of b.
	B  uint32
	Nb uint

	// Huffman decoders for literal/length, distance.
	H1, H2 HuffmanDecoder

	// Length arrays used to define Huffman codes.
	Bits     *[MaxLit + MaxDist]int
	Codebits *[NumCodes]int

	// Output history, buffer.
	Hist  *[MaxHist]byte // Export history
	Hp    int            // current output position in buffer
	Hw    int            // have written hist[0:hw] already
	Hfull bool           // buffer has filled at least once

	// Temporary buffer (avoids repeated allocation).
	Buf [4]byte

	// Next step in the decompression,
	// and decompression state.
	Step     func(*Decompressor)
	Final    bool
	Err      error
	ToRead   []byte
	Hl, Hd   *HuffmanDecoder
	CopyLen  int
	CopyDist int
}

func (f *Decompressor) NextBlock() {
	if f.Final {
		if f.Hw != f.Hp {
			f.flush((*Decompressor).NextBlock)
			return
		}
		f.Err = io.EOF
		return
	}
	for f.Nb < 1+2 {
		if f.Err = f.moreBits(); f.Err != nil {
			return
		}
	}
	f.Final = f.B&1 == 1
	f.B >>= 1
	typ := f.B & 3
	f.B >>= 2
	f.Nb -= 1 + 2
	switch typ {
	case 0:
		f.dataBlock()
	case 1:
		// compressed, fixed Huffman tables
		f.Hl = &fixedHuffmanDecoder
		f.Hd = nil
		f.huffmanBlock()
	case 2:
		// compressed, dynamic Huffman tables
		if f.Err = f.readHuffman(); f.Err != nil {
			break
		}
		f.Hl = &f.H1
		f.Hd = &f.H2
		f.huffmanBlock()
	default:
		// 3 is reserved.
		f.Err = CorruptInputError(f.Roffset)
	}
}

// ReadBlock() is a modified Read() that always fully returns the last block
// read. If b isn't big enough to hold a single uncompressed block, we increase
// size of b.
func (f *Decompressor) ReadBlock() ([]byte, error) {
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

func (f *Decompressor) Read(b []byte) (int, error) {
	for {
		if len(f.ToRead) > 0 {
			n := copy(b, f.ToRead)
			f.ToRead = f.ToRead[n:]
			return n, nil
		}
		if f.Err != nil {
			return 0, f.Err
		}
		f.Step(f)
	}
}

func (f *Decompressor) Close() error {
	if f.Err == io.EOF {
		return nil
	}
	return f.Err
}

// RFC 1951 section 3.2.7.
// Compression with dynamic Huffman codes

var codeOrder = [...]int{16, 17, 18, 0, 8, 7, 9, 6, 10, 5, 11, 4, 12, 3, 13, 2, 14, 1, 15}

func (f *Decompressor) readHuffman() error {
	// HLIT[5], HDIST[5], HCLEN[4].
	for f.Nb < 5+5+4 {
		if err := f.moreBits(); err != nil {
			return err
		}
	}
	nlit := int(f.B&0x1F) + 257
	if nlit > MaxLit {
		return CorruptInputError(f.Roffset)
	}
	f.B >>= 5
	ndist := int(f.B&0x1F) + 1
	// maxDist is 32, so ndist is always valid.
	f.B >>= 5
	nclen := int(f.B&0xF) + 4
	// numCodes is 19, so nclen is always valid.
	f.B >>= 4
	f.Nb -= 5 + 5 + 4

	// (HCLEN+4)*3 bits: code lengths in the magic codeOrder order.
	for i := 0; i < nclen; i++ {
		for f.Nb < 3 {
			if err := f.moreBits(); err != nil {
				return err
			}
		}
		f.Codebits[codeOrder[i]] = int(f.B & 0x7)
		f.B >>= 3
		f.Nb -= 3
	}
	for i := nclen; i < len(codeOrder); i++ {
		f.Codebits[codeOrder[i]] = 0
	}
	if !f.H1.init(f.Codebits[0:]) {
		return CorruptInputError(f.Roffset)
	}

	// HLIT + 257 code lengths, HDIST + 1 code lengths,
	// using the code length Huffman code.
	for i, n := 0, nlit+ndist; i < n; {
		x, err := f.huffSym(&f.H1)
		if err != nil {
			return err
		}
		if x < 16 {
			// Actual length.
			f.Bits[i] = x
			i++
			continue
		}
		// Repeat previous length or zero.
		var rep int
		var nb uint
		var b int
		switch x {
		default:
			return InternalError("unexpected length code")
		case 16:
			rep = 3
			nb = 2
			if i == 0 {
				return CorruptInputError(f.Roffset)
			}
			b = f.Bits[i-1]
		case 17:
			rep = 3
			nb = 3
			b = 0
		case 18:
			rep = 11
			nb = 7
			b = 0
		}
		for f.Nb < nb {
			if err := f.moreBits(); err != nil {
				return err
			}
		}
		rep += int(f.B & uint32(1<<nb-1))
		f.B >>= nb
		f.Nb -= nb
		if i+rep > n {
			return CorruptInputError(f.Roffset)
		}
		for j := 0; j < rep; j++ {
			f.Bits[i] = b
			i++
		}
	}

	if !f.H1.init(f.Bits[0:nlit]) || !f.H2.init(f.Bits[nlit:nlit+ndist]) {
		return CorruptInputError(f.Roffset)
	}

	return nil
}

// Decode a single Huffman block from f.
// hl and hd are the Huffman states for the lit/length values
// and the distance values, respectively.  If hd == nil, using the
// fixed distance encoding associated with fixed Huffman blocks.
func (f *Decompressor) huffmanBlock() {
	for {
		v, err := f.huffSym(f.Hl)
		if err != nil {
			f.Err = err
			return
		}
		var n uint // number of bits extra
		var length int
		switch {
		case v < 256:
			f.Hist[f.Hp] = byte(v)
			f.Hp++
			if f.Hp == len(f.Hist) {
				// After the flush, continue this loop.
				f.flush((*Decompressor).huffmanBlock)
				return
			}
			continue
		case v == 256:
			// Done with huffman block; read next block.
			f.Step = (*Decompressor).NextBlock
			return
		// otherwise, reference to older data
		case v < 265:
			length = v - (257 - 3)
			n = 0
		case v < 269:
			length = v*2 - (265*2 - 11)
			n = 1
		case v < 273:
			length = v*4 - (269*4 - 19)
			n = 2
		case v < 277:
			length = v*8 - (273*8 - 35)
			n = 3
		case v < 281:
			length = v*16 - (277*16 - 67)
			n = 4
		case v < 285:
			length = v*32 - (281*32 - 131)
			n = 5
		default:
			length = 258
			n = 0
		}
		if n > 0 {
			for f.Nb < n {
				if err = f.moreBits(); err != nil {
					f.Err = err
					return
				}
			}
			length += int(f.B & uint32(1<<n-1))
			f.B >>= n
			f.Nb -= n
		}

		var dist int
		if f.Hd == nil {
			for f.Nb < 5 {
				if err = f.moreBits(); err != nil {
					f.Err = err
					return
				}
			}
			dist = int(reverseByte[(f.B&0x1F)<<3])
			f.B >>= 5
			f.Nb -= 5
		} else {
			if dist, err = f.huffSym(f.Hd); err != nil {
				f.Err = err
				return
			}
		}

		switch {
		case dist < 4:
			dist++
		case dist >= 30:
			f.Err = CorruptInputError(f.Roffset)
			return
		default:
			nb := uint(dist-2) >> 1
			// have 1 bit in bottom of dist, need nb more.
			extra := (dist & 1) << nb
			for f.Nb < nb {
				if err = f.moreBits(); err != nil {
					f.Err = err
					return
				}
			}
			extra |= int(f.B & uint32(1<<nb-1))
			f.B >>= nb
			f.Nb -= nb
			dist = 1<<(nb+1) + 1 + extra
		}

		// Copy history[-dist:-dist+length] into output.
		if dist > len(f.Hist) {
			f.Err = InternalError("bad history distance")
			return
		}

		// No check on length; encoding can be prescient.
		if !f.Hfull && dist > f.Hp {
			f.Err = CorruptInputError(f.Roffset)
			return
		}

		f.CopyLen, f.CopyDist = length, dist
		if f.copyHist() {
			return
		}
	}
}

// copyHist copies f.copyLen bytes from f.hist (f.copyDist bytes ago) to itself.
// It reports whether the f.hist buffer is full.
func (f *Decompressor) copyHist() bool {
	p := f.Hp - f.CopyDist
	if p < 0 {
		p += len(f.Hist)
	}
	for f.CopyLen > 0 {
		n := f.CopyLen
		if x := len(f.Hist) - f.Hp; n > x {
			n = x
		}
		if x := len(f.Hist) - p; n > x {
			n = x
		}
		forwardCopy(f.Hist[:], f.Hp, p, n)
		p += n
		f.Hp += n
		f.CopyLen -= n
		if f.Hp == len(f.Hist) {
			// After flush continue copying out of history.
			f.flush((*Decompressor).copyHuff)
			return true
		}
		if p == len(f.Hist) {
			p = 0
		}
	}
	return false
}

func (f *Decompressor) copyHuff() {
	if f.copyHist() {
		return
	}

	f.huffmanBlock()
}

// Copy a single uncompressed data block from input to output.
func (f *Decompressor) dataBlock() {
	// Uncompressed.
	// Discard current half-byte.
	f.Nb = 0
	f.B = 0

	// Length then ones-complement of length.
	nr, err := io.ReadFull(f.R, f.Buf[0:4])
	f.Roffset += int64(nr)
	if err != nil {
		f.Err = &ReadError{f.Roffset, err}
		return
	}
	n := int(f.Buf[0]) | int(f.Buf[1])<<8
	nn := int(f.Buf[2]) | int(f.Buf[3])<<8
	if uint16(nn) != uint16(^n) {
		f.Err = CorruptInputError(f.Roffset)
		return
	}

	if n == 0 {
		// 0-length block means sync
		f.flush((*Decompressor).NextBlock)
		return
	}

	f.CopyLen = n
	f.copyData()
}

// copyData copies f.copyLen bytes from the underlying reader into f.hist.
// It pauses for reads when f.hist is full.
func (f *Decompressor) copyData() {
	n := f.CopyLen
	for n > 0 {
		m := len(f.Hist) - f.Hp
		if m > n {
			m = n
		}
		m, err := io.ReadFull(f.R, f.Hist[f.Hp:f.Hp+m])
		f.Roffset += int64(m)
		if err != nil {
			f.Err = &ReadError{f.Roffset, err}
			return
		}
		n -= m
		f.Hp += m
		if f.Hp == len(f.Hist) {
			f.CopyLen = n
			f.flush((*Decompressor).copyData)
			return
		}
	}
	f.Step = (*Decompressor).NextBlock
}

func (f *Decompressor) setDict(dict []byte) {
	if len(dict) > len(f.Hist) {
		// Will only remember the tail.
		dict = dict[len(dict)-len(f.Hist):]
	}

	f.Hp = copy(f.Hist[:], dict)
	if f.Hp == len(f.Hist) {
		f.Hp = 0
		f.Hfull = true
	}
	f.Hw = f.Hp
}

func (f *Decompressor) moreBits() error {
	c, err := f.R.ReadByte()
	if err != nil {
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
		return err
	}
	f.Roffset++
	f.B |= uint32(c) << f.Nb
	f.Nb += 8
	return nil
}

// Read the next Huffman-encoded symbol from f according to h.
func (f *Decompressor) huffSym(h *HuffmanDecoder) (int, error) {
	n := uint(h.Min)
	for {
		for f.Nb < n {
			if err := f.moreBits(); err != nil {
				return 0, err
			}
		}
		chunk := h.Chunks[f.B&(huffmanNumChunks-1)]
		n = uint(chunk & huffmanCountMask)
		if n > huffmanChunkBits {
			chunk = h.Links[chunk>>huffmanValueShift][(f.B>>huffmanChunkBits)&h.LinkMask]
			n = uint(chunk & huffmanCountMask)
			if n == 0 {
				f.Err = CorruptInputError(f.Roffset)
				return 0, f.Err
			}
		}
		if n <= f.Nb {
			f.B >>= n
			f.Nb -= n
			return int(chunk >> huffmanValueShift), nil
		}
	}
}

// Flush any buffered output to the underlying writer.
func (f *Decompressor) flush(step func(*Decompressor)) {
	f.ToRead = f.Hist[f.Hw:f.Hp]
	f.Woffset += int64(f.Hp - f.Hw)
	f.Hw = f.Hp
	if f.Hp == len(f.Hist) {
		f.Hp = 0
		f.Hw = 0
		f.Hfull = true
	}
	f.Step = step
}

func MakeReader(r io.Reader) Reader {
	if rr, ok := r.(Reader); ok {
		return rr
	}
	return bufio.NewReader(r)
}

func (f *Decompressor) Reset(r io.Reader, dict []byte) error {
	*f = Decompressor{
		R:        MakeReader(r),
		Bits:     f.Bits,
		Codebits: f.Codebits,
		Hist:     f.Hist,
		Step:     (*Decompressor).NextBlock,
	}
	if dict != nil {
		f.setDict(dict)
	}
	return nil
}

// NewReader returns a new ReadCloser that can be used
// to read the uncompressed version of r.
// If r does not also implement io.ByteReader,
// the decompressor may read more data than necessary from r.
// It is the caller's responsibility to call Close on the ReadCloser
// when finished reading.
//
// The ReadCloser returned by NewReader also implements Resetter.
func NewReader(r io.Reader) io.ReadCloser {
	var f Decompressor
	f.Bits = new([MaxLit + MaxDist]int)
	f.Codebits = new([NumCodes]int)
	f.R = MakeReader(r)
	f.Hist = new([MaxHist]byte)
	f.Step = (*Decompressor).NextBlock
	return &f
}

// NewReaderDict is like NewReader but initializes the reader
// with a preset dictionary.  The returned Reader behaves as if
// the uncompressed data stream started with the given dictionary,
// which has already been read.  NewReaderDict is typically used
// to read data compressed by NewWriterDict.
//
// The ReadCloser returned by NewReader also implements Resetter.
func NewReaderDict(r io.Reader, dict []byte) io.ReadCloser {
	var f Decompressor
	f.R = MakeReader(r)
	f.Hist = new([MaxHist]byte)
	f.Bits = new([MaxLit + MaxDist]int)
	f.Codebits = new([NumCodes]int)
	f.Step = (*Decompressor).NextBlock
	f.setDict(dict)
	return &f
}
