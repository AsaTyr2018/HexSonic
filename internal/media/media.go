package media

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type Probe struct {
	Duration float64
	Codec    string
	Bitrate  int
	Channels int
	Rate     int
	Title    string
	Artist   string
	Album    string
	Genre    string
	HasCover bool
	TrackNo  int
}

type ffprobeResult struct {
	Streams []struct {
		CodecName   string `json:"codec_name"`
		CodecType   string `json:"codec_type"`
		Channels    int    `json:"channels"`
		SampleRate  string `json:"sample_rate"`
		BitRate     string `json:"bit_rate"`
		Disposition struct {
			AttachedPic int `json:"attached_pic"`
		} `json:"disposition"`
	} `json:"streams"`
	Format struct {
		Duration string            `json:"duration"`
		Tags     map[string]string `json:"tags"`
	} `json:"format"`
}

func ProbeFile(ctx context.Context, ffprobeBin, path string) (Probe, error) {
	cmd := exec.CommandContext(ctx, ffprobeBin,
		"-v", "error",
		"-print_format", "json",
		"-show_streams",
		"-show_format",
		path,
	)
	out, err := cmd.Output()
	if err != nil {
		return Probe{}, fmt.Errorf("ffprobe: %w", err)
	}
	var res ffprobeResult
	if err := json.Unmarshal(out, &res); err != nil {
		return Probe{}, fmt.Errorf("parse ffprobe json: %w", err)
	}

	p := Probe{}
	foundAudio := false
	for _, s := range res.Streams {
		if s.CodecType != "audio" {
			continue
		}
		foundAudio = true
		p.Codec = s.CodecName
		fmt.Sscanf(s.BitRate, "%d", &p.Bitrate)
		fmt.Sscanf(s.SampleRate, "%d", &p.Rate)
		p.Channels = s.Channels
		break
	}
	if !foundAudio {
		return Probe{}, fmt.Errorf("no audio stream found")
	}
	for _, s := range res.Streams {
		if s.Disposition.AttachedPic == 1 {
			p.HasCover = true
			break
		}
	}
	fmt.Sscanf(res.Format.Duration, "%f", &p.Duration)
	p.Title = firstNonEmpty(
		res.Format.Tags["title"],
		res.Format.Tags["TITLE"],
	)
	p.Artist = firstNonEmpty(
		res.Format.Tags["artist"],
		res.Format.Tags["ARTIST"],
		res.Format.Tags["album_artist"],
		res.Format.Tags["ALBUM_ARTIST"],
	)
	p.Album = firstNonEmpty(
		res.Format.Tags["album"],
		res.Format.Tags["ALBUM"],
	)
	p.Genre = firstNonEmpty(
		res.Format.Tags["genre"],
		res.Format.Tags["GENRE"],
	)
	p.TrackNo = parseTrackNumber(firstNonEmpty(
		res.Format.Tags["track"],
		res.Format.Tags["TRACK"],
		res.Format.Tags["tracknumber"],
		res.Format.Tags["TRACKNUMBER"],
		res.Format.Tags["trck"],
		res.Format.Tags["TRCK"],
	))
	return p, nil
}

func ExtractCoverJPEG(ctx context.Context, ffmpegBin, source, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, ffmpegBin,
		"-y",
		"-i", source,
		"-map", "0:v:0",
		"-frames:v", "1",
		"-q:v", "2",
		target,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("extract cover failed: %w (%s)", err, string(out))
	}
	return nil
}

func TranscodeMP3(ctx context.Context, ffmpegBin, source, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, ffmpegBin,
		"-y",
		"-i", source,
		"-vn",
		"-codec:a", "libmp3lame",
		"-b:a", "320k",
		target,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("mp3 transcode failed: %w (%s)", err, string(out))
	}
	return nil
}

func TranscodeOpus(ctx context.Context, ffmpegBin, source, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, ffmpegBin,
		"-y",
		"-i", source,
		"-vn",
		"-codec:a", "libopus",
		"-b:a", "160k",
		target,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("opus transcode failed: %w (%s)", err, string(out))
	}
	return nil
}

func TranscodeAAC(ctx context.Context, ffmpegBin, source, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, ffmpegBin,
		"-y",
		"-i", source,
		"-vn",
		"-codec:a", "aac",
		"-profile:a", "aac_low",
		"-b:a", "160k",
		"-movflags", "+faststart",
		target,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("aac transcode failed: %w (%s)", err, string(out))
	}
	return nil
}

func BuildWaveform(ctx context.Context, ffmpegBin, source, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, ffmpegBin,
		"-v", "error",
		"-i", source,
		"-ac", "1",
		"-ar", "1000",
		"-f", "f32le",
		"-",
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start ffmpeg waveform: %w", err)
	}
	data, err := io.ReadAll(stdout)
	if err != nil {
		return fmt.Errorf("read waveform stream: %w", err)
	}
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("wait ffmpeg waveform: %w", err)
	}

	if len(data)%4 != 0 {
		data = data[:len(data)-(len(data)%4)]
	}
	sampleCount := len(data) / 4
	if sampleCount == 0 {
		return fmt.Errorf("waveform sample count is zero")
	}

	samples := make([]float32, sampleCount)
	if err := binary.Read(bytes.NewReader(data), binary.LittleEndian, &samples); err != nil {
		return fmt.Errorf("decode samples: %w", err)
	}

	const bins = 512
	binVals := make([]float64, bins)
	step := float64(sampleCount) / float64(bins)
	for i := 0; i < bins; i++ {
		start := int(math.Floor(float64(i) * step))
		end := int(math.Floor(float64(i+1) * step))
		if end <= start {
			end = start + 1
		}
		if end > sampleCount {
			end = sampleCount
		}
		var max float64
		for _, s := range samples[start:end] {
			v := math.Abs(float64(s))
			if v > max {
				max = v
			}
		}
		if max > 1 {
			max = 1
		}
		binVals[i] = max
	}

	payload := struct {
		Bins []float64 `json:"bins"`
	}{Bins: binVals}
	f, err := os.Create(target)
	if err != nil {
		return fmt.Errorf("create waveform target: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(payload); err != nil {
		return fmt.Errorf("encode waveform json: %w", err)
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func parseTrackNumber(raw string) int {
	v := strings.TrimSpace(raw)
	if v == "" {
		return 0
	}
	if i := strings.Index(v, "/"); i >= 0 {
		v = strings.TrimSpace(v[:i])
	}
	if i, err := strconv.Atoi(v); err == nil && i > 0 {
		return i
	}
	return 0
}
