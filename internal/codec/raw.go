package codec

func init() {
	Register(Raw())
}

// Raw returns a codec that performs no compression (pass-through copy).
// Encode and Decode simply copy src into dst. This is the fastest codec
// and is used when compression overhead is not justified (small frames,
// already-compressed data, or LAN with abundant bandwidth).
func Raw() Codec {
	return rawCodec{}
}

type rawCodec struct{}

// Encode copies src into dst unchanged. Returns a sub-slice of dst containing
// the copied data. If dst is nil or too small, allocates a new buffer.
func (rawCodec) Encode(dst, src []byte) ([]byte, error) {
	if len(src) == 0 {
		return dst[:0], nil
	}

	if cap(dst) < len(src) {
		dst = make([]byte, len(src))
	} else {
		dst = dst[:len(src)]
	}

	copy(dst, src)
	return dst, nil
}

// Decode copies src into dst unchanged. Returns a sub-slice of dst containing
// the copied data. If dst is nil or too small, allocates a new buffer.
func (rawCodec) Decode(dst, src []byte) ([]byte, error) {
	if len(src) == 0 {
		return dst[:0], nil
	}

	if cap(dst) < len(src) {
		dst = make([]byte, len(src))
	} else {
		dst = dst[:len(src)]
	}

	copy(dst, src)
	return dst, nil
}

// ID returns the raw codec protocol identifier (0x00).
func (rawCodec) ID() byte {
	return IDRaw
}

// MaxEncodedSize returns srcLen since raw encoding has no overhead.
func (rawCodec) MaxEncodedSize(srcLen int) int {
	return srcLen
}
