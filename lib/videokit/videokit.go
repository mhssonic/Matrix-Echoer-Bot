package videokit

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// MaxMatrixVideoBytes is the maximum video size allowed for upload (after optional compression).
const MaxMatrixVideoBytes int64 = 5 << 20

// FFmpegExecutable returns FFMPEG_PATH if set, otherwise "ffmpeg".
func FFmpegExecutable() string {
	if p := os.Getenv("FFMPEG_PATH"); p != "" {
		return p
	}
	return "ffmpeg"
}

// CompressH264AAC re-encodes video to H.264/AAC MP4 for smaller size (best-effort).
func CompressH264AAC(ctx context.Context, inPath, outPath string) error {
	inPath = filepath.Clean(inPath)
	outPath = filepath.Clean(outPath)
	ff := FFmpegExecutable()
	// scale: cap width 1280, keep aspect; faststart for web playback
	args := []string{
		"-y",
		"-i", inPath,
		"-c:v", "libx264", "-crf", "28", "-preset", "fast",
		"-vf", "scale='min(1280,iw)':-2",
		"-c:a", "aac", "-b:a", "96k",
		"-movflags", "+faststart",
		outPath,
	}
	cmd := exec.CommandContext(ctx, ff, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, string(out))
	}
	return nil
}

// PrepareForMatrix returns a path to upload, its size, and a cleanup function.
// If the file is still larger than MaxMatrixVideoBytes after compression, uploadOK is false
// (caller should skip upload and e.g. send caption as text). cleanup must always be called.
func PrepareForMatrix(ctx context.Context, inputPath string) (uploadPath string, size int64, cleanup func(), uploadOK bool, err error) {
	st, err := os.Stat(inputPath)
	if err != nil {
		return "", 0, nil, false, err
	}
	sz := st.Size()
	noop := func() {}

	if sz <= MaxMatrixVideoBytes {
		return inputPath, sz, noop, true, nil
	}

	out, err := os.CreateTemp(filepath.Dir(inputPath), "vid-compressed-*.mp4")
	if err != nil {
		return "", 0, nil, false, err
	}
	outPath := out.Name()
	_ = out.Close()

	rmOut := func() { _ = os.Remove(outPath) }

	if err := CompressH264AAC(ctx, inputPath, outPath); err != nil {
		rmOut()
		return "", 0, noop, false, err
	}

	st2, err := os.Stat(outPath)
	if err != nil {
		rmOut()
		return "", 0, noop, false, err
	}
	if st2.Size() > MaxMatrixVideoBytes {
		rmOut()
		return "", 0, noop, false, nil
	}

	return outPath, st2.Size(), rmOut, true, nil
}
