package gion

import (
	"encoding/binary"
	"errors"
	"io"
)

// ErrTooLong reports a sample slice whose WAV encoding would overflow the
// format's 32-bit sizes.
var ErrTooLong = errors.New("gion: too many samples for a WAV file")

// wavHeader is the canonical 44-byte RIFF header for mono 16-bit PCM.
type wavHeader struct {
	RIFF     [4]byte
	Size     uint32 // file size minus the first 8 bytes
	WAVE     [4]byte
	Fmt      [4]byte
	FmtSize  uint32
	Format   uint16 // 1 = PCM
	Channels uint16
	Rate     uint32
	ByteRate uint32
	Align    uint16
	Bits     uint16
	Data     [4]byte
	DataSize uint32
}

// WriteWAV writes the samples as a mono 16-bit PCM WAV stream at the given
// rate (DefaultRate when rate <= 0).
func WriteWAV(w io.Writer, rate int, samples []int16) error {
	if rate <= 0 {
		rate = DefaultRate
	}
	if len(samples) > (1<<31-44)/2 {
		return ErrTooLong
	}
	dataSize := uint32(len(samples) * 2) // #nosec G115 -- bounded above
	h := wavHeader{
		RIFF:     [4]byte{'R', 'I', 'F', 'F'},
		Size:     36 + dataSize,
		WAVE:     [4]byte{'W', 'A', 'V', 'E'},
		Fmt:      [4]byte{'f', 'm', 't', ' '},
		FmtSize:  16,
		Format:   1,
		Channels: 1,
		Rate:     uint32(rate),     // #nosec G115 -- sample rates are small
		ByteRate: uint32(rate * 2), // #nosec G115
		Align:    2,
		Bits:     16,
		Data:     [4]byte{'d', 'a', 't', 'a'},
		DataSize: dataSize,
	}
	err := binary.Write(w, binary.LittleEndian, h)
	if err != nil {
		return err
	}
	return binary.Write(w, binary.LittleEndian, samples)
}
