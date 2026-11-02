package profile

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

const MaxImageBytes = 10 * 1024 * 1024

var ErrImage = errors.New("invalid image")
var ErrTooLarge = errors.New("image too large")
var ErrImageBusy = errors.New("image validation busy")

type HEICValidator interface {
	Validate(context.Context, []byte, int64) error
}
type ImageValidator struct {
	HEIC         HEICValidator
	DecodeBudget int64
	slots        chan struct{}
}

func NewImageValidator(heic HEICValidator, budget int64) *ImageValidator {
	if budget <= 0 {
		budget = 64 << 20
	}
	return &ImageValidator{heic, budget, make(chan struct{}, 1)}
}

// ReadAndValidate bounds upload and decode concurrency before allocating buffers.
func (v *ImageValidator) ReadAndValidate(ctx context.Context, r io.Reader) ([]byte, string, error) {
	select {
	case v.slots <- struct{}{}:
	default:
		return nil, "", ErrImageBusy
	}
	defer func() { <-v.slots }()
	data, err := io.ReadAll(io.LimitReader(r, MaxImageBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(data) > MaxImageBytes {
		return nil, "", ErrTooLarge
	}
	if len(data) == 0 {
		return nil, "", ErrImage
	}
	if isHEIC(data) {
		if !validBoxes(data) {
			return nil, "", ErrImage
		}
		if v.HEIC == nil {
			return nil, "", ErrStorage
		}
		if err = v.HEIC.Validate(ctx, data, v.DecodeBudget); err != nil {
			return nil, "", err
		}
		return data, "image/heic", nil
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || format != "jpeg" && format != "png" {
		return nil, "", ErrImage
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width) > v.DecodeBudget/16/int64(cfg.Height) {
		return nil, "", ErrTooLarge
	}
	if format == "jpeg" && !completeJPEG(data) {
		return nil, "", ErrImage
	}
	if format == "png" && !completePNG(data) {
		return nil, "", ErrImage
	}
	if _, _, err = image.Decode(bytes.NewReader(data)); err != nil {
		return nil, "", ErrImage
	}
	if err = ctx.Err(); err != nil {
		return nil, "", err
	}
	return data, "image/" + format, nil
}

func completePNG(data []byte) bool {
	pos := 8
	for pos+12 <= len(data) {
		n := uint64(binary.BigEndian.Uint32(data[pos : pos+4]))
		end := uint64(pos) + 12 + n
		if end > uint64(len(data)) {
			return false
		}
		kind := string(data[pos+4 : pos+8])
		payloadEnd := int(end) - 4
		if crc32.ChecksumIEEE(data[pos+4:payloadEnd]) != binary.BigEndian.Uint32(data[payloadEnd:int(end)]) {
			return false
		}
		if kind == "IEND" {
			return n == 0 && int(end) == len(data)
		}
		pos = int(end)
	}
	return false
}
func completeJPEG(data []byte) bool {
	if len(data) < 4 || data[0] != 0xff || data[1] != 0xd8 {
		return false
	}
	pos := 2
	scan := false
	for pos < len(data) {
		if scan {
			for pos < len(data) && data[pos] != 0xff {
				pos++
			}
		}
		if pos >= len(data) || data[pos] != 0xff {
			return false
		}
		for pos < len(data) && data[pos] == 0xff {
			pos++
		}
		if pos >= len(data) {
			return false
		}
		marker := data[pos]
		pos++
		if marker == 0 {
			if !scan {
				return false
			}
			continue
		}
		if marker == 0xd9 {
			return pos == len(data)
		}
		if marker == 0xd8 {
			return false
		}
		if marker >= 0xd0 && marker <= 0xd7 {
			if !scan {
				return false
			}
			continue
		}
		if marker == 1 {
			continue
		}
		if pos+2 > len(data) {
			return false
		}
		size := int(binary.BigEndian.Uint16(data[pos : pos+2]))
		if size < 2 || size > len(data)-pos {
			return false
		}
		pos += size
		scan = marker == 0xda
	}
	return false
}
func validBoxes(data []byte) bool {
	pos := 0
	for pos < len(data) {
		if len(data)-pos < 8 {
			return false
		}
		size := uint64(binary.BigEndian.Uint32(data[pos : pos+4]))
		header := uint64(8)
		if size == 1 {
			if len(data)-pos < 16 {
				return false
			}
			size = binary.BigEndian.Uint64(data[pos+8 : pos+16])
			header = 16
		}
		if size == 0 {
			size = uint64(len(data) - pos)
		}
		if size < header || size > uint64(len(data)-pos) {
			return false
		}
		pos += int(size)
	}
	return pos == len(data)
}
func isHEIC(data []byte) bool {
	if len(data) < 24 || string(data[4:8]) != "ftyp" {
		return false
	}
	size := int(binary.BigEndian.Uint32(data[:4]))
	if size < 24 || size > len(data) || size%4 != 0 {
		return false
	}
	// HEVC-coded HEIF, not generic HEIF/AVIF/SVG with a renamed extension.
	for i := 8; i < size; i += 4 {
		if i == 12 {
			continue
		}
		brand := string(data[i : i+4])
		if brand == "heic" || brand == "heix" {
			return true
		}
	}
	return false
}

// CommandHEICValidator decodes in an isolated native subprocess. The converted
// scratch image is discarded; uploaded bytes are retained without transformation.
type CommandHEICValidator struct{}

func (CommandHEICValidator) Validate(ctx context.Context, data []byte, budget int64) error {
	if _, err := exec.LookPath("heif-convert"); err != nil {
		return ErrStorage
	}
	dir, err := os.MkdirTemp("", "tramas-heic-")
	if err != nil {
		return ErrStorage
	}
	defer os.RemoveAll(dir)
	input, output := filepath.Join(dir, "input.heic"), filepath.Join(dir, "decoded.png")
	if err = os.WriteFile(input, data, 0600); err != nil {
		return ErrStorage
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	// Limits are infrastructure safeguards, not product resolution/crop settings.
	cmd := exec.CommandContext(ctx, "sh", "-c", `ulimit -v 131072; ulimit -t 4; ulimit -f "$1"; exec heif-convert "$2" "$3"`, "image-validator", strconv.FormatInt(budget/512, 10), input, output)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + dir, "TMPDIR=" + dir}
	if err = cmd.Run(); err != nil {
		return ErrImage
	}
	f, err := os.Open(output)
	if err != nil {
		return ErrImage
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width) > budget/16/int64(cfg.Height) {
		return ErrTooLarge
	}
	return nil
}
