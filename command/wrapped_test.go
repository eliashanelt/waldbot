package command

import (
	"bytes"
	"fmt"
	"image/gif"
	"image/png"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"waldbot/data"
	"waldbot/date"

	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
)

// loadTestFonts populates data.NormalFont/BoldFont so the test can exercise
// the rendering pipeline. Tries the bundled fonts first, then falls back to
// the Go stdlib fonts from golang.org/x/image so the test works in CI without
// the production assets.
func loadTestFonts(t *testing.T) {
	t.Helper()
	for _, p := range []string{"./data/fonts/whitneylight.ttf", "../data/fonts/whitneylight.ttf"} {
		if b, err := ioutil.ReadFile(p); err == nil {
			if f, err := truetype.Parse(b); err == nil {
				data.NormalFont = f
				break
			}
		}
	}
	for _, p := range []string{"./data/fonts/whitneybold.ttf", "../data/fonts/whitneybold.ttf"} {
		if b, err := ioutil.ReadFile(p); err == nil {
			if f, err := truetype.Parse(b); err == nil {
				data.BoldFont = f
				break
			}
		}
	}
	if data.NormalFont == nil {
		f, err := truetype.Parse(goregular.TTF)
		if err != nil {
			t.Fatalf("parse goregular: %v", err)
		}
		data.NormalFont = f
	}
	if data.BoldFont == nil {
		f, err := truetype.Parse(gobold.TTF)
		if err != nil {
			t.Fatalf("parse gobold: %v", err)
		}
		data.BoldFont = f
	}
}

func TestRenderWrappedGif(t *testing.T) {
	loadTestFonts(t)

	stats := wrappedStats{
		year:               2025,
		displayName:        "Testuser",
		totalMinutes:       45_000, // 750h
		activeDays:         210,
		longestSession:     420,
		longestSessionDate: date.New(14, 6, 2025),
		biggestDay:         date.New(31, 12, 2025),
		biggestDayMinutes:  720,
		peakHour:           21,
		peakHourMinutes:    8_400,
		topChannelName:     "Hauptkanal",
		topChannelMin:      18_000,
		topMateName:        "Bestie",
		topMateMin:         12_500,
	}

	buf := renderWrappedGif(stats)
	if buf == nil {
		t.Fatal("renderWrappedGif returned nil")
	}
	t.Logf("gif size: %.1f KB", float64(len(buf))/1024)

	// decode to confirm it's well-formed
	g, err := gif.DecodeAll(bytes.NewReader(buf))
	if err != nil {
		t.Fatalf("decoded gif invalid: %v", err)
	}
	if len(g.Image) < 24 {
		t.Fatalf("expected many frames, got %d", len(g.Image))
	}
	t.Logf("frames: %d", len(g.Image))

	// optionally dump to a file when DUMP_WRAPPED is set to a path
	if out := os.Getenv("DUMP_WRAPPED"); out != "" {
		if out == "1" {
			out = filepath.Join(os.TempDir(), "wrapped_preview.gif")
		}
		if err := ioutil.WriteFile(out, buf, 0644); err != nil {
			t.Errorf("dump: %v", err)
		} else {
			t.Logf("wrote %s", out)
		}
	}

	// dump representative frames as PNGs for quick inspection
	if os.Getenv("DUMP_FRAMES") != "" {
		dir := os.Getenv("DUMP_FRAMES")
		// sample one frame from each scene boundary
		samples := []int{0, 20, 50, 90, 120, 150, 180, 210, 240}
		for _, idx := range samples {
			if idx >= len(g.Image) {
				continue
			}
			path := filepath.Join(dir, fmt.Sprintf("frame_%03d.png", idx))
			f, err := os.Create(path)
			if err != nil {
				t.Errorf("create %s: %v", path, err)
				continue
			}
			if err := png.Encode(f, g.Image[idx]); err != nil {
				t.Errorf("encode %s: %v", path, err)
			}
			f.Close()
		}
		t.Logf("dumped %d frames to %s", len(samples), dir)
	}
}
