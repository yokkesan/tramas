package profile_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"tramas/backend/internal/profile"
)

func imageBytes(t *testing.T, format string) []byte {
	t.Helper()
	var b bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	var err error
	if format == "png" {
		err = png.Encode(&b, img)
	} else {
		err = jpeg.Encode(&b, img, nil)
	}
	if err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func TestImageValidation(t *testing.T) {
	pngBytes := imageBytes(t, "png")
	jpegBytes := imageBytes(t, "jpeg")
	bomb := append([]byte(nil), pngBytes...)
	binary.BigEndian.PutUint32(bomb[16:20], 1<<20)
	binary.BigEndian.PutUint32(bomb[20:24], 1<<20)
	binary.BigEndian.PutUint32(bomb[29:33], crc32.ChecksumIEEE(bomb[12:29]))
	corrupt := append([]byte(nil), pngBytes...)
	corrupt[len(corrupt)-13] ^= 0xff
	for _, tc := range []struct {
		name  string
		data  []byte
		valid bool
	}{
		{"png", pngBytes, true}, {"jpeg", jpegBytes, true}, {"empty", nil, false}, {"svg", []byte(`<svg onload="alert(1)"></svg>`), false},
		{"gif", []byte("GIF89a payload"), false}, {"truncated jpeg", jpegBytes[:len(jpegBytes)-1], false}, {"truncated png", pngBytes[:len(pngBytes)-2], false},
		{"appended payload", append(append([]byte(nil), pngBytes...), []byte("<script>bad</script>")...), false}, {"corrupt png", corrupt, false}, {"decode memory bomb", bomb, false},
		{"duplicate png ending", append(append(append([]byte(nil), pngBytes...), []byte("payload")...), pngBytes[len(pngBytes)-12:]...), false},
		{"duplicate jpeg ending", append(append(append([]byte(nil), jpegBytes...), []byte("payload")...), 0xff, 0xd9), false},
		{"oversize", []byte(strings.Repeat("x", profile.MaxImageBytes+1)), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := profile.NewImageValidator(nil, 64<<20)
			out, contentType, err := v.ReadAndValidate(context.Background(), bytes.NewReader(tc.data))
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v want=%v", err == nil, tc.valid)
			}
			if tc.valid && (!bytes.Equal(out, tc.data) || contentType != "image/"+tc.name) {
				t.Fatal("image transformed or wrong content type")
			}
		})
	}
}
func TestFileStorageIsolation(t *testing.T) {
	root := t.TempDir()
	store, err := profile.NewFileStorage(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	key, err := store.Put(context.Background(), []byte("image bytes"))
	if err != nil {
		t.Fatal(err)
	}
	f, err := store.Open(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(f)
	f.Close()
	if string(data) != "image bytes" {
		t.Fatal("roundtrip failed")
	}
	for _, bad := range []string{"../secret", "/etc/passwd", key + "/../secret"} {
		if _, err = store.Open(context.Background(), bad); err == nil {
			t.Fatal("path traversal")
		}
	}
	target := filepath.Join(t.TempDir(), "private")
	if err = os.WriteFile(target, []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	symlink := strings.Repeat("a", 64)
	if err = os.Symlink(target, filepath.Join(root, symlink)); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Open(context.Background(), symlink); err == nil {
		t.Fatal("symlink escaped storage root")
	}
	if err = store.Delete(context.Background(), key); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Open(context.Background(), key); err == nil {
		t.Fatal("deletion failed")
	}
}
func TestHEICNativeValidation(t *testing.T) {
	if _, err := exec.LookPath("heif-enc"); err != nil {
		t.Skip("native HEIC tools run in compose.test.yaml")
	}
	dir := t.TempDir()
	input, output := filepath.Join(dir, "test.png"), filepath.Join(dir, "test.heic")
	if err := os.WriteFile(input, imageBytes(t, "png"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("heif-enc", "-q", "50", "-p", "x265:pools=1", input, "-o", output)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		t.Fatal("HEIC fixture generation failed")
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	validator := profile.NewImageValidator(profile.CommandHEICValidator{}, 64<<20)
	out, contentType, err := validator.ReadAndValidate(context.Background(), bytes.NewReader(data))
	if err != nil || contentType != "image/heic" || !bytes.Equal(out, data) {
		t.Fatal("valid HEIC rejected", err)
	}
	if _, _, err = validator.ReadAndValidate(context.Background(), bytes.NewReader(data[:len(data)/2])); err == nil {
		t.Fatal("truncated HEIC accepted")
	}
}
