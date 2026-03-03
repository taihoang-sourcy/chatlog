package lz4

import (
	"github.com/pierrec/lz4/v4"
)

func Decompress(src []byte) ([]byte, error) {
	// FIXME: lz4 compression ratio is expected to be less than 3, set to 4 for safety margin
	out := make([]byte, len(src)*4)

	n, err := lz4.UncompressBlock(src, out)
	if err != nil {
		return nil, err
	}
	return out[:n], nil
}
