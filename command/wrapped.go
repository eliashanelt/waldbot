package command

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"log"
	"math"
	"math/rand"
	"time"

	"waldbot/data"
	"waldbot/date"

	"github.com/bwmarrin/discordgo"
	"github.com/golang/freetype"
	"github.com/golang/freetype/truetype"
)

// ─── public entry ────────────────────────────────────────────────────────────

func wrappedResponse(query Query) (string, *discordgo.File) {
	year := query.year
	if year == 0 {
		year = int64(time.Now().Year())
	}
	stats := computeWrappedStats(query.member, int(year))
	if stats.totalMinutes == 0 {
		return fmt.Sprintf("Keine Sprachchatdaten für %v in %d gefunden.",
			data.EffectiveName(query.member), year), nil
	}
	buf := renderScenes(buildPersonalScenes(stats))
	if buf == nil {
		return "Wrapped konnte nicht erstellt werden.", nil
	}
	return "", &discordgo.File{
		Name:        fmt.Sprintf("wrapped-%d.gif", year),
		ContentType: "image/gif",
		Reader:      bytes.NewReader(buf),
	}
}

// ─── stat aggregation ────────────────────────────────────────────────────────

type wrappedStats struct {
	year               int
	displayName        string
	totalMinutes       uint32
	activeDays         int
	longestSession     int16
	longestSessionDate date.Date
	biggestDay         date.Date
	biggestDayMinutes  uint32
	peakHour           int
	peakHourMinutes    uint32
	topChannelName     string
	topChannelMin      uint32
	topMateName        string
	topMateMin         uint32
	hourBuckets        [24]uint32
	weekdayMinutes     [7]uint32
}

func computeWrappedStats(member *discordgo.Member, year int) wrappedStats {
	userID := data.ShortUserId(member.User.ID)
	s := wrappedStats{year: year, displayName: data.EffectiveName(member)}

	channelMin := map[int16]uint32{}

	for d, day := range data.DayData {
		if int(d.Year) != year {
			continue
		}
		var dayMin uint32
		hasDay := false

		for ch, sessions := range day.Channels {
			for _, sess := range sessions {
				if sess.UserID != userID {
					continue
				}
				hasDay = true
				dayMin += uint32(sess.Minutes)
				channelMin[ch] += uint32(sess.Minutes)

				if sess.Minutes > s.longestSession {
					s.longestSession = sess.Minutes
					s.longestSessionDate = d
				}

				cur := int(sess.DayMinute)
				remaining := int(sess.Minutes)
				for remaining > 0 {
					h := cur / 60
					if h >= 24 {
						h = 23
					}
					room := 60 - cur%60
					if room > remaining {
						room = remaining
					}
					s.hourBuckets[h] += uint32(room)
					cur += room
					remaining -= room
				}
			}
		}
		if hasDay {
			s.activeDays++
			s.totalMinutes += dayMin
			if dayMin > s.biggestDayMinutes {
				s.biggestDayMinutes = dayMin
				s.biggestDay = d
			}
			t := time.Date(int(d.Year), time.Month(d.Month), int(d.Day), 0, 0, 0, 0, time.Local)
			s.weekdayMinutes[int(t.Weekday())] += dayMin
		}
	}

	for h, m := range s.hourBuckets {
		if m > s.peakHourMinutes {
			s.peakHourMinutes = m
			s.peakHour = h
		}
	}

	var topCh int16
	for ch, m := range channelMin {
		if m > s.topChannelMin {
			s.topChannelMin = m
			topCh = ch
		}
	}
	if s.topChannelMin > 0 {
		if c, err := data.Dc.Channel(data.LongChannelId(topCh)); err == nil {
			s.topChannelName = c.Name
		} else {
			s.topChannelName = "[Gelöschter Kanal]"
		}
	}

	yearCond := func(d date.Date) bool { return int(d.Year) == year }
	mates, _ := data.TimeWithMates(userID, yearCond)
	if len(mates) > 0 {
		s.topMateMin = mates[0].Minutes
		m, _ := data.Dc.State.Member(member.GuildID, data.LongUserId(mates[0].UserId))
		s.topMateName = data.EffectiveName(m)
	}
	return s
}

// ─── render constants ────────────────────────────────────────────────────────

const (
	wrappedW   = 480
	wrappedH   = 480
	wrappedFps = 12
)

// ─── color helpers ───────────────────────────────────────────────────────────

func hsv(h, s, v float64) color.RGBA {
	h = math.Mod(h, 1)
	if h < 0 {
		h += 1
	}
	c := v * s
	x := c * (1 - math.Abs(math.Mod(h*6, 2)-1))
	m := v - c
	var r, g, b float64
	switch int(h * 6) {
	case 0:
		r, g, b = c, x, 0
	case 1:
		r, g, b = x, c, 0
	case 2:
		r, g, b = 0, c, x
	case 3:
		r, g, b = 0, x, c
	case 4:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}
	return color.RGBA{
		R: clampByte((r + m) * 255),
		G: clampByte((g + m) * 255),
		B: clampByte((b + m) * 255),
		A: 255,
	}
}

func clampByte(f float64) uint8 {
	if f < 0 {
		return 0
	}
	if f > 255 {
		return 255
	}
	return uint8(f)
}

func lerp(a, b, t float64) float64 { return a + (b-a)*t }

func lerpColor(c1, c2 color.RGBA, t float64) color.RGBA {
	return color.RGBA{
		R: clampByte(lerp(float64(c1.R), float64(c2.R), t)),
		G: clampByte(lerp(float64(c1.G), float64(c2.G), t)),
		B: clampByte(lerp(float64(c1.B), float64(c2.B), t)),
		A: 255,
	}
}

func darken(c color.RGBA, f float64) color.RGBA {
	return color.RGBA{
		R: clampByte(float64(c.R) * f),
		G: clampByte(float64(c.G) * f),
		B: clampByte(float64(c.B) * f),
		A: 255,
	}
}

// ─── easing ──────────────────────────────────────────────────────────────────

func clamp01(t float64) float64 {
	if t < 0 {
		return 0
	}
	if t > 1 {
		return 1
	}
	return t
}

func easeOutCubic(t float64) float64 {
	t = clamp01(t)
	inv := 1 - t
	return 1 - inv*inv*inv
}

func easeOutBack(t float64) float64 {
	t = clamp01(t)
	const c1 = 1.70158
	const c3 = c1 + 1
	x := t - 1
	return 1 + c3*x*x*x + c1*x*x
}

// ─── drawing primitives ─────────────────────────────────────────────────────

// fillBackground paints a vertical gradient. Writes directly into Pix for ~5x
// speedup over SetRGBA.
func fillBackground(img *image.RGBA, top, bottom color.RGBA) {
	h := img.Bounds().Dy()
	w := img.Bounds().Dx()
	stride := img.Stride
	for y := 0; y < h; y++ {
		f := float64(y) / float64(h)
		c := lerpColor(top, bottom, f)
		row := img.Pix[y*stride : y*stride+w*4]
		for x := 0; x < w; x++ {
			i := x * 4
			row[i] = c.R
			row[i+1] = c.G
			row[i+2] = c.B
			row[i+3] = 255
		}
	}
}

func fillRect(img *image.RGBA, x, y, w, h int, c color.RGBA) {
	r := image.Rect(x, y, x+w, y+h)
	draw.Draw(img, r, &image.Uniform{c}, image.Point{}, draw.Src)
}

func fillCircle(img *image.RGBA, cx, cy int, r float64, c color.RGBA) {
	rr := int(math.Ceil(r))
	r2 := r * r
	for dy := -rr; dy <= rr; dy++ {
		for dx := -rr; dx <= rr; dx++ {
			if float64(dx*dx+dy*dy) <= r2 {
				x, y := cx+dx, cy+dy
				if x >= 0 && x < wrappedW && y >= 0 && y < wrappedH {
					img.SetRGBA(x, y, c)
				}
			}
		}
	}
}

func drawLine(img *image.RGBA, x0, y0, x1, y1 int, thickness float64, c color.RGBA) {
	dx, dy := x1-x0, y1-y0
	steps := int(math.Max(math.Abs(float64(dx)), math.Abs(float64(dy)))) + 1
	for i := 0; i <= steps; i++ {
		f := float64(i) / float64(steps)
		px := x0 + int(float64(dx)*f)
		py := y0 + int(float64(dy)*f)
		fillCircle(img, px, py, thickness/2, c)
	}
}

// ─── text ────────────────────────────────────────────────────────────────────

type textAlign int

const (
	alignLeft textAlign = iota
	alignCenter
	alignRight
)

func textWidth(fnt *truetype.Font, size float64, s string) int {
	face := truetype.NewFace(fnt, &truetype.Options{Size: size, DPI: 72})
	defer face.Close()
	total := 0
	for _, r := range s {
		if adv, ok := face.GlyphAdvance(r); ok {
			total += adv.Round()
		}
	}
	return total
}

func drawTextRaw(img *image.RGBA, fnt *truetype.Font, size float64, x, y int, c color.RGBA, s string) {
	if s == "" || size < 1 {
		return
	}
	ctx := freetype.NewContext()
	ctx.SetDPI(72)
	ctx.SetFont(fnt)
	ctx.SetFontSize(size)
	ctx.SetClip(img.Bounds())
	ctx.SetDst(img)
	ctx.SetSrc(image.NewUniform(c))
	if _, err := ctx.DrawString(s, freetype.Pt(x, y)); err != nil {
		log.Println("wrapped: drawText error:", err)
	}
}

// drawText draws text with a soft drop shadow for readability over any
// background. Always uses fully-opaque color (alpha quirks in palette
// quantization make semi-transparent text look ugly).
func drawText(img *image.RGBA, fnt *truetype.Font, size float64, x, y int, c color.RGBA, s string, a textAlign) {
	if s == "" || size < 1 {
		return
	}
	w := textWidth(fnt, size, s)
	switch a {
	case alignCenter:
		x -= w / 2
	case alignRight:
		x -= w
	}
	// shadow offset scales with size
	sh := int(math.Max(2, size*0.04))
	shadow := color.RGBA{0, 0, 0, 255}
	drawTextRaw(img, fnt, size, x+sh, y+sh, shadow, s)
	c.A = 255
	drawTextRaw(img, fnt, size, x, y, c, s)
}

// truncate a string to roughly fit a max pixel width
func fitText(fnt *truetype.Font, size float64, s string, maxW int) string {
	if textWidth(fnt, size, s) <= maxW {
		return s
	}
	for len(s) > 3 {
		s = s[:len(s)-1]
		candidate := s + "…"
		if textWidth(fnt, size, candidate) <= maxW {
			return candidate
		}
	}
	return s
}

// ─── particles (confetti) ───────────────────────────────────────────────────

type particle struct {
	x, y   float64
	vx, vy float64
	size   float64
	hue    float64
	phase  float64
}

func makeParticles(n int, seed int64) []particle {
	r := rand.New(rand.NewSource(seed))
	ps := make([]particle, n)
	for i := range ps {
		ps[i] = particle{
			x:     r.Float64() * wrappedW,
			y:     r.Float64() * wrappedH,
			vx:    (r.Float64() - 0.5) * 60,
			vy:    20 + r.Float64()*40,
			size:  2 + r.Float64()*3,
			hue:   r.Float64(),
			phase: r.Float64() * math.Pi * 2,
		}
	}
	return ps
}

// drawParticles draws confetti dots cycling through hues. Hues are quantized
// to 12 buckets so a small fixed palette of confetti colors suffices.
func drawParticles(img *image.RGBA, ps []particle, t float64) {
	for _, p := range ps {
		x := math.Mod(p.x+p.vx*t, wrappedW)
		if x < 0 {
			x += wrappedW
		}
		y := math.Mod(p.y+p.vy*t, wrappedH)
		if y < 0 {
			y += wrappedH
		}
		hueBucket := math.Floor((p.hue+t*0.15)*12) / 12
		c := hsv(hueBucket, 0.75, 1)
		twinkle := 0.5 + 0.5*math.Sin(t*5+p.phase)
		fillCircle(img, int(x), int(y), p.size*(0.6+0.6*twinkle), c)
	}
}

// ─── per-scene palette ──────────────────────────────────────────────────────

var confettiHues []color.RGBA

func init() {
	confettiHues = make([]color.RGBA, 12)
	for i := 0; i < 12; i++ {
		confettiHues[i] = hsv(float64(i)/12, 0.75, 1)
	}
}

// buildScenePalette builds a 256-color palette tuned to a scene's gradient.
// Heavy weight on the gradient ramp (so it stays smooth) plus UI colors,
// shadow ramps, and the 12 confetti hues. Per-scene palettes mean each GIF
// frame compresses tightly while keeping visual fidelity.
func buildScenePalette(top, bottom color.RGBA, accent color.RGBA) color.Palette {
	pal := make(color.Palette, 0, 256)

	// 128-step gradient ramp top→bottom
	const ramp = 128
	for i := 0; i < ramp; i++ {
		f := float64(i) / float64(ramp-1)
		pal = append(pal, lerpColor(top, bottom, f))
	}
	// 32-step darker version of the gradient (handles text shadow regions)
	for i := 0; i < 32; i++ {
		f := float64(i) / 31
		pal = append(pal, darken(lerpColor(top, bottom, f), 0.5))
	}
	// 16-step very dark for hard shadows / clock face
	for i := 0; i < 16; i++ {
		f := float64(i) / 15
		pal = append(pal, darken(lerpColor(top, bottom, f), 0.2))
	}
	// confetti hues
	for _, c := range confettiHues {
		pal = append(pal, c)
	}
	// UI colors
	pal = append(pal,
		color.RGBA{255, 255, 255, 255}, // white
		color.RGBA{240, 240, 240, 255},
		color.RGBA{200, 200, 200, 255},
		color.RGBA{255, 240, 180, 255}, // soft yellow
		color.RGBA{255, 220, 100, 255}, // gold
		color.RGBA{0, 0, 0, 255},
		color.RGBA{30, 30, 50, 255},
		accent,
	)
	// pad
	for len(pal) < 256 {
		pal = append(pal, color.RGBA{0, 0, 0, 255})
	}
	return pal[:256]
}

func quantize(src *image.RGBA, pal color.Palette) *image.Paletted {
	dst := image.NewPaletted(src.Bounds(), pal)
	draw.Draw(dst, dst.Bounds(), src, src.Bounds().Min, draw.Src)
	return dst
}

// ─── scene engine ────────────────────────────────────────────────────────────

type wrappedScene struct {
	duration float64 // seconds
	bgTop    color.RGBA
	bgBot    color.RGBA
	accent   color.RGBA
	draw     func(img *image.RGBA, t float64, sceneOffset float64)
}

func buildPersonalScenes(s wrappedStats) []wrappedScene {
	confetti := makeParticles(60, 7)

	white := color.RGBA{255, 255, 255, 255}
	gold := color.RGBA{255, 220, 100, 255}
	softYellow := color.RGBA{255, 240, 180, 255}

	return []wrappedScene{
		// 1. intro
		{
			duration: 2.2,
			bgTop:    color.RGBA{72, 110, 220, 255},
			bgBot:    color.RGBA{120, 50, 180, 255},
			accent:   gold,
			draw: func(img *image.RGBA, t, off float64) {
				drawParticles(img, confetti, off+t)

				lp := easeOutBack(clamp01(t / 0.6))
				drawText(img, data.NormalFont, 18+8*lp, wrappedW/2, 150, white,
					"WALDBOT PRÄSENTIERT", alignCenter)

				titleP := easeOutBack(clamp01((t - 0.3) / 0.9))
				drawText(img, data.BoldFont, 30+50*titleP, wrappedW/2, 250, white,
					"WRAPPED", alignCenter)

				yearP := easeOutCubic(clamp01((t - 0.9) / 0.7))
				drawText(img, data.BoldFont, 30+30*yearP, wrappedW/2, 320, softYellow,
					fmt.Sprintf("%d", s.year), alignCenter)

				nameP := easeOutCubic(clamp01((t - 1.4) / 0.6))
				if nameP > 0 {
					drawText(img, data.NormalFont, 22*nameP, wrappedW/2, 390, white,
						"für "+fitText(data.NormalFont, 22, s.displayName, wrappedW-40), alignCenter)
				}
			},
		},

		// 2. total hours counter
		{
			duration: 3.2,
			bgTop:    color.RGBA{220, 60, 120, 255},
			bgBot:    color.RGBA{80, 20, 80, 255},
			accent:   gold,
			draw: func(img *image.RGBA, t, off float64) {
				drawParticles(img, confetti, off+t)

				drawText(img, data.NormalFont, 26, wrappedW/2, 110, white,
					"Du warst online für", alignCenter)

				progress := easeOutCubic(clamp01((t - 0.2) / 2.2))
				hours := uint32(float64(s.totalMinutes) * progress / 60)
				big := fmt.Sprintf("%d", hours)
				pulse := 1.0
				if t > 2.4 {
					pulse = 1 + 0.08*math.Sin((t-2.4)*math.Pi*6)
				}
				drawText(img, data.BoldFont, 150*pulse, wrappedW/2, 280, white,
					big, alignCenter)
				drawText(img, data.NormalFont, 34, wrappedW/2, 330, softYellow,
					"Stunden", alignCenter)
				drawText(img, data.NormalFont, 22, wrappedW/2, 410, white,
					fmt.Sprintf("an %d Tagen", s.activeDays), alignCenter)
			},
		},

		// 3. peak hour clock
		{
			duration: 3.0,
			bgTop:    color.RGBA{255, 165, 80, 255},
			bgBot:    color.RGBA{180, 40, 80, 255},
			accent:   gold,
			draw: func(img *image.RGBA, t, off float64) {
				drawParticles(img, confetti, off+t)

				drawText(img, data.NormalFont, 26, wrappedW/2, 90, white,
					"Deine Peak-Zeit:", alignCenter)

				cx, cy := wrappedW/2, 280
				radius := 110.0
				fillCircle(img, cx, cy, radius+8, white)
				fillCircle(img, cx, cy, radius, color.RGBA{30, 30, 50, 255})
				for h := 0; h < 12; h++ {
					ang := float64(h)/12*2*math.Pi - math.Pi/2
					x1 := cx + int(math.Cos(ang)*(radius-6))
					y1 := cy + int(math.Sin(ang)*(radius-6))
					x2 := cx + int(math.Cos(ang)*radius)
					y2 := cy + int(math.Sin(ang)*radius)
					drawLine(img, x1, y1, x2, y2, 3, white)
				}

				sweepT := easeOutCubic(clamp01(t / 1.8))
				angle := sweepT*2*math.Pi*1.5 + float64(s.peakHour%12)/12*2*math.Pi - math.Pi/2
				if t > 1.8 {
					angle = float64(s.peakHour%12)/12*2*math.Pi - math.Pi/2
				}
				hx := cx + int(math.Cos(angle)*(radius-20))
				hy := cy + int(math.Sin(angle)*(radius-20))
				drawLine(img, cx, cy, hx, hy, 6, gold)
				fillCircle(img, cx, cy, 8, white)

				if t > 1.6 {
					p := easeOutBack(clamp01((t - 1.6) / 0.7))
					drawText(img, data.BoldFont, 50*p, wrappedW/2, 440, softYellow,
						fmt.Sprintf("%02d:00 Uhr", s.peakHour), alignCenter)
				}
			},
		},

		// 4. top channel
		{
			duration: 2.6,
			bgTop:    color.RGBA{60, 200, 130, 255},
			bgBot:    color.RGBA{20, 100, 120, 255},
			accent:   gold,
			draw: func(img *image.RGBA, t, off float64) {
				drawParticles(img, confetti, off+t)

				lp := easeOutCubic(clamp01(t / 0.5))
				lx := -200 + int(float64(wrappedW/2+200)*lp)
				drawText(img, data.NormalFont, 26, lx, 160, white, "Dein Lieblingskanal", alignCenter)

				namep := easeOutBack(clamp01((t - 0.4) / 0.9))
				nameSize := 20 + 40*namep
				name := fitText(data.BoldFont, nameSize, "#"+s.topChannelName, wrappedW-40)
				drawText(img, data.BoldFont, nameSize, wrappedW/2, 270, white, name, alignCenter)

				if t > 1.0 {
					p := easeOutCubic(clamp01((t - 1.0) / 0.8))
					drawText(img, data.NormalFont, 28*p, wrappedW/2, 360, softYellow,
						data.FormatTime(s.topChannelMin), alignCenter)
				}
			},
		},

		// 5. top mate
		{
			duration: 2.6,
			bgTop:    color.RGBA{255, 80, 160, 255},
			bgBot:    color.RGBA{120, 30, 100, 255},
			accent:   gold,
			draw: func(img *image.RGBA, t, off float64) {
				drawParticles(img, confetti, off+t)

				lp := easeOutCubic(clamp01(t / 0.5))
				lx := wrappedW + 200 - int(float64(wrappedW/2+200)*lp)
				drawText(img, data.NormalFont, 26, lx, 160, white, "Dein Top-Mate", alignCenter)

				if s.topMateName == "" {
					drawText(img, data.BoldFont, 36, wrappedW/2, 270, white,
						"— Niemand —", alignCenter)
					drawText(img, data.NormalFont, 22, wrappedW/2, 320, softYellow,
						"Einsamer Wolf :(", alignCenter)
					return
				}

				namep := easeOutBack(clamp01((t - 0.4) / 0.9))
				nameSize := 20 + 40*namep
				name := fitText(data.BoldFont, nameSize, s.topMateName, wrappedW-40)
				drawText(img, data.BoldFont, nameSize, wrappedW/2, 270, white, name, alignCenter)

				if t > 1.0 {
					p := easeOutCubic(clamp01((t - 1.0) / 0.8))
					drawText(img, data.NormalFont, 26*p, wrappedW/2, 360, softYellow,
						"gemeinsam "+data.FormatTime(s.topMateMin), alignCenter)
				}
			},
		},

		// 6. longest session
		{
			duration: 2.4,
			bgTop:    color.RGBA{120, 80, 220, 255},
			bgBot:    color.RGBA{40, 30, 100, 255},
			accent:   gold,
			draw: func(img *image.RGBA, t, off float64) {
				drawParticles(img, confetti, off+t)

				drawText(img, data.NormalFont, 26, wrappedW/2, 140, white,
					"Längste Session", alignCenter)

				progress := easeOutCubic(clamp01((t - 0.2) / 1.4))
				cur := uint32(float64(s.longestSession) * progress)
				drawText(img, data.BoldFont, 110, wrappedW/2, 280, white,
					data.FormatTime(cur), alignCenter)

				if t > 1.2 {
					p := easeOutCubic(clamp01((t - 1.2) / 0.8))
					d := s.longestSessionDate
					drawText(img, data.NormalFont, 22*p, wrappedW/2, 360, softYellow,
						fmt.Sprintf("am %d.%d.%d", d.Day, d.Month, d.Year), alignCenter)
				}
			},
		},

		// 7. biggest day
		{
			duration: 2.4,
			bgTop:    color.RGBA{255, 200, 80, 255},
			bgBot:    color.RGBA{200, 60, 40, 255},
			accent:   white,
			draw: func(img *image.RGBA, t, off float64) {
				drawParticles(img, confetti, off+t)

				drawText(img, data.NormalFont, 26, wrappedW/2, 140, white,
					"Größter Tag", alignCenter)

				progress := easeOutCubic(clamp01((t - 0.2) / 1.4))
				cur := uint32(float64(s.biggestDayMinutes) * progress)
				drawText(img, data.BoldFont, 100, wrappedW/2, 280, white,
					data.FormatTime(cur), alignCenter)

				if t > 1.2 {
					p := easeOutCubic(clamp01((t - 1.2) / 0.8))
					d := s.biggestDay
					wd := []string{"So", "Mo", "Di", "Mi", "Do", "Fr", "Sa"}[
						int(time.Date(int(d.Year), time.Month(d.Month), int(d.Day), 0, 0, 0, 0, time.Local).Weekday())]
					drawText(img, data.NormalFont, 22*p, wrappedW/2, 360, white,
						fmt.Sprintf("%s, %d.%d.%d", wd, d.Day, d.Month, d.Year), alignCenter)
				}
			},
		},

		// 8. outro
		{
			duration: 2.8,
			bgTop:    color.RGBA{60, 180, 220, 255},
			bgBot:    color.RGBA{120, 50, 200, 255},
			accent:   gold,
			draw: func(img *image.RGBA, t, off float64) {
				// double-density confetti burst
				drawParticles(img, confetti, off+t*2)

				p := easeOutBack(clamp01(t / 0.8))
				drawText(img, data.BoldFont, 56*p, wrappedW/2, 220, white,
					"Danke!", alignCenter)
				if t > 0.6 {
					p2 := easeOutCubic(clamp01((t - 0.6) / 0.8))
					drawText(img, data.NormalFont, 26*p2, wrappedW/2, 290, white,
						"Bis nächstes Jahr im Wald!", alignCenter)
				}
				if t > 1.2 {
					p3 := easeOutCubic(clamp01((t - 1.2) / 0.8))
					drawText(img, data.NormalFont, 18*p3, wrappedW/2, 410, softYellow,
						fmt.Sprintf("WALD WRAPPED %d", s.year), alignCenter)
				}
			},
		},
	}
}

// renderScenes encodes a list of scenes as an animated GIF. Each scene gets
// its own 256-color palette so multi-scene mood shifts compress tightly and
// gradients stay smooth.
func renderScenes(scenes []wrappedScene) []byte {
	g := &gif.GIF{LoopCount: 0}
	delayCs := 100 / wrappedFps
	sceneOffset := 0.0

	for _, sc := range scenes {
		pal := buildScenePalette(sc.bgTop, sc.bgBot, sc.accent)
		nFrames := int(sc.duration*float64(wrappedFps) + 0.5)
		if nFrames < 1 {
			nFrames = 1
		}
		for i := 0; i < nFrames; i++ {
			localT := float64(i) / float64(wrappedFps)
			img := image.NewRGBA(image.Rect(0, 0, wrappedW, wrappedH))
			fillBackground(img, sc.bgTop, sc.bgBot)
			sc.draw(img, localT, sceneOffset)
			g.Image = append(g.Image, quantize(img, pal))
			g.Delay = append(g.Delay, delayCs)
		}
		sceneOffset += sc.duration
	}

	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, g); err != nil {
		log.Println("wrapped: gif encode error:", err)
		return nil
	}
	return buf.Bytes()
}
