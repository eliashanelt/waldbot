package command

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"log"
	"math"
	"time"

	"waldbot/data"
	"waldbot/date"

	"github.com/bwmarrin/discordgo"
)

// ─── public entry ────────────────────────────────────────────────────────────

func serverwrappedResponse(query Query) (string, *discordgo.File) {
	year := query.year
	if year == 0 {
		year = int64(time.Now().Year())
	}
	if query.member == nil || query.member.GuildID == "" {
		return "Server-ID nicht verfügbar.", nil
	}
	stats := computeServerStats(query.member.GuildID, int(year))
	if stats.totalMinutes == 0 {
		return fmt.Sprintf("Keine Sprachchatdaten für %d gefunden.", year), nil
	}
	buf := renderScenes(buildServerScenes(stats))
	if buf == nil {
		return "Server-Wrapped konnte nicht erstellt werden.", nil
	}
	return "", &discordgo.File{
		Name:        fmt.Sprintf("server-wrapped-%d.gif", year),
		ContentType: "image/gif",
		Reader:      bytes.NewReader(buf),
	}
}

// ─── stat aggregation ────────────────────────────────────────────────────────

type serverStats struct {
	year               int
	totalMinutes       uint32
	totalActiveUsers   int
	totalSessions      int
	biggestDay         date.Date
	biggestDayMinutes  uint32
	topUserName        string
	topUserMinutes     uint32
	topChannelName     string
	topChannelMinutes  uint32
	longestSession     int16
	longestSessionUser string
	longestSessionDate date.Date
	peakHour           int
	peakHourMinutes    uint32
	topMatePairA       string
	topMatePairB       string
	topMatePairMin     uint32
	hourBuckets        [24]uint32
}

func computeServerStats(guildID string, year int) serverStats {
	s := serverStats{year: year}

	channelMin := map[int16]uint32{}
	userMin := map[int16]uint32{}
	pairMin := map[[2]int16]uint32{}
	activeUsers := map[int16]bool{}

	var longestSessUserID int16

	for d, day := range data.DayData {
		if int(d.Year) != year {
			continue
		}
		var dayTotal uint32

		for ch, sessions := range day.Channels {
			for i, sess := range sessions {
				dayTotal += uint32(sess.Minutes)
				channelMin[ch] += uint32(sess.Minutes)
				userMin[sess.UserID] += uint32(sess.Minutes)
				activeUsers[sess.UserID] = true
				s.totalSessions++

				if sess.Minutes > s.longestSession {
					s.longestSession = sess.Minutes
					longestSessUserID = sess.UserID
					s.longestSessionDate = d
				}

				cur := int(sess.DayMinute)
				rem := int(sess.Minutes)
				for rem > 0 {
					h := cur / 60
					if h >= 24 {
						h = 23
					}
					room := 60 - cur%60
					if room > rem {
						room = rem
					}
					s.hourBuckets[h] += uint32(room)
					cur += room
					rem -= room
				}

				// pair overlaps only with later sessions in the same channel,
				// so each pair is counted once.
				for j := i + 1; j < len(sessions); j++ {
					other := sessions[j]
					if other.UserID == sess.UserID {
						continue
					}
					ov := sessionOverlap(sess, other)
					if ov > 0 {
						a, b := sess.UserID, other.UserID
						if a > b {
							a, b = b, a
						}
						pairMin[[2]int16{a, b}] += uint32(ov)
					}
				}
			}
		}
		s.totalMinutes += dayTotal
		if dayTotal > s.biggestDayMinutes {
			s.biggestDayMinutes = dayTotal
			s.biggestDay = d
		}
	}

	s.totalActiveUsers = len(activeUsers)

	var topUID int16
	for u, m := range userMin {
		if m > s.topUserMinutes {
			s.topUserMinutes = m
			topUID = u
		}
	}
	if s.topUserMinutes > 0 {
		s.topUserName = resolveMemberName(guildID, topUID)
	}

	var topCh int16
	for ch, m := range channelMin {
		if m > s.topChannelMinutes {
			s.topChannelMinutes = m
			topCh = ch
		}
	}
	if s.topChannelMinutes > 0 {
		if c, err := data.Dc.Channel(data.LongChannelId(topCh)); err == nil {
			s.topChannelName = c.Name
		} else {
			s.topChannelName = "[Gelöschter Kanal]"
		}
	}

	var topPair [2]int16
	for k, m := range pairMin {
		if m > s.topMatePairMin {
			s.topMatePairMin = m
			topPair = k
		}
	}
	if s.topMatePairMin > 0 {
		s.topMatePairA = resolveMemberName(guildID, topPair[0])
		s.topMatePairB = resolveMemberName(guildID, topPair[1])
	}

	if s.longestSession > 0 {
		s.longestSessionUser = resolveMemberName(guildID, longestSessUserID)
	}

	for h, m := range s.hourBuckets {
		if m > s.peakHourMinutes {
			s.peakHourMinutes = m
			s.peakHour = h
		}
	}

	return s
}

func sessionOverlap(a, b data.VoiceSession) int32 {
	aStart := int32(a.DayMinute)
	aEnd := aStart + int32(a.Minutes)
	bStart := int32(b.DayMinute)
	bEnd := bStart + int32(b.Minutes)
	start := aStart
	if bStart > start {
		start = bStart
	}
	end := aEnd
	if bEnd < end {
		end = bEnd
	}
	return end - start
}

func resolveMemberName(guildID string, userID int16) string {
	if m, err := data.Dc.State.Member(guildID, data.LongUserId(userID)); err == nil {
		return data.EffectiveName(m)
	}
	return "[Unbekannt]"
}

// ─── scenes ──────────────────────────────────────────────────────────────────

func buildServerScenes(s serverStats) []wrappedScene {
	confetti := makeParticles(60, 13)

	white := color.RGBA{255, 255, 255, 255}
	gold := color.RGBA{255, 220, 100, 255}
	softYellow := color.RGBA{255, 240, 180, 255}

	return []wrappedScene{
		// 1. intro
		{
			duration: 2.4,
			bgTop:    color.RGBA{40, 80, 200, 255},
			bgBot:    color.RGBA{140, 30, 220, 255},
			accent:   gold,
			draw: func(img *image.RGBA, t, off float64) {
				drawParticles(img, confetti, off+t)

				lp := easeOutBack(clamp01(t / 0.6))
				drawText(img, data.NormalFont, 18+8*lp, wrappedW/2, 130, white,
					"WALDBOT", alignCenter)

				titleP := easeOutBack(clamp01((t - 0.3) / 0.9))
				drawText(img, data.BoldFont, 22+30*titleP, wrappedW/2, 220, white,
					"SERVER WRAPPED", alignCenter)

				yearP := easeOutCubic(clamp01((t - 0.9) / 0.7))
				drawText(img, data.BoldFont, 30+30*yearP, wrappedW/2, 300, softYellow,
					fmt.Sprintf("%d", s.year), alignCenter)

				if t > 1.4 {
					p := easeOutCubic(clamp01((t - 1.4) / 0.7))
					drawText(img, data.NormalFont, 22*p, wrappedW/2, 390, white,
						fmt.Sprintf("mit %d Nutzern", s.totalActiveUsers), alignCenter)
				}
			},
		},

		// 2. total server hours
		{
			duration: 3.2,
			bgTop:    color.RGBA{255, 100, 80, 255},
			bgBot:    color.RGBA{100, 20, 60, 255},
			accent:   gold,
			draw: func(img *image.RGBA, t, off float64) {
				drawParticles(img, confetti, off+t)

				drawText(img, data.NormalFont, 26, wrappedW/2, 110, white,
					"Insgesamt", alignCenter)

				progress := easeOutCubic(clamp01((t - 0.2) / 2.2))
				hours := uint32(float64(s.totalMinutes) * progress / 60)
				big := fmt.Sprintf("%d", hours)
				pulse := 1.0
				if t > 2.4 {
					pulse = 1 + 0.08*math.Sin((t-2.4)*math.Pi*6)
				}
				drawText(img, data.BoldFont, 130*pulse, wrappedW/2, 280, white,
					big, alignCenter)
				drawText(img, data.NormalFont, 32, wrappedW/2, 330, softYellow,
					"Stunden im Sprachchat", alignCenter)
				drawText(img, data.NormalFont, 22, wrappedW/2, 410, white,
					fmt.Sprintf("über %d Sessions", s.totalSessions), alignCenter)
			},
		},

		// 3. peak hour clock
		{
			duration: 3.0,
			bgTop:    color.RGBA{60, 200, 140, 255},
			bgBot:    color.RGBA{20, 90, 120, 255},
			accent:   gold,
			draw: func(img *image.RGBA, t, off float64) {
				drawParticles(img, confetti, off+t)

				drawText(img, data.NormalFont, 26, wrappedW/2, 90, white,
					"Peak-Zeit des Servers:", alignCenter)

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

		// 4. top user
		{
			duration: 2.8,
			bgTop:    color.RGBA{255, 180, 60, 255},
			bgBot:    color.RGBA{200, 70, 20, 255},
			accent:   white,
			draw: func(img *image.RGBA, t, off float64) {
				drawParticles(img, confetti, off+t)

				lp := easeOutCubic(clamp01(t / 0.5))
				lx := -200 + int(float64(wrappedW/2+200)*lp)
				drawText(img, data.NormalFont, 26, lx, 160, white,
					"Most Active", alignCenter)

				namep := easeOutBack(clamp01((t - 0.4) / 0.9))
				nameSize := 20 + 40*namep
				name := fitText(data.BoldFont, nameSize, s.topUserName, wrappedW-40)
				drawText(img, data.BoldFont, nameSize, wrappedW/2, 270, white,
					name, alignCenter)

				if t > 1.0 {
					p := easeOutCubic(clamp01((t - 1.0) / 0.8))
					drawText(img, data.NormalFont, 28*p, wrappedW/2, 360, softYellow,
						data.FormatTime(s.topUserMinutes), alignCenter)
				}
			},
		},

		// 5. top channel
		{
			duration: 2.6,
			bgTop:    color.RGBA{60, 200, 200, 255},
			bgBot:    color.RGBA{20, 80, 150, 255},
			accent:   gold,
			draw: func(img *image.RGBA, t, off float64) {
				drawParticles(img, confetti, off+t)

				lp := easeOutCubic(clamp01(t / 0.5))
				lx := wrappedW + 200 - int(float64(wrappedW/2+200)*lp)
				drawText(img, data.NormalFont, 26, lx, 160, white,
					"Lieblings-Kanal", alignCenter)

				namep := easeOutBack(clamp01((t - 0.4) / 0.9))
				nameSize := 20 + 40*namep
				name := fitText(data.BoldFont, nameSize, "#"+s.topChannelName, wrappedW-40)
				drawText(img, data.BoldFont, nameSize, wrappedW/2, 270, white,
					name, alignCenter)

				if t > 1.0 {
					p := easeOutCubic(clamp01((t - 1.0) / 0.8))
					drawText(img, data.NormalFont, 28*p, wrappedW/2, 360, softYellow,
						data.FormatTime(s.topChannelMinutes), alignCenter)
				}
			},
		},

		// 6. top mate pair
		{
			duration: 3.0,
			bgTop:    color.RGBA{255, 80, 180, 255},
			bgBot:    color.RGBA{110, 30, 130, 255},
			accent:   gold,
			draw: func(img *image.RGBA, t, off float64) {
				drawParticles(img, confetti, off+t)

				drawText(img, data.NormalFont, 26, wrappedW/2, 100, white,
					"Power-Duo", alignCenter)

				if s.topMatePairA == "" {
					drawText(img, data.BoldFont, 36, wrappedW/2, 270, white,
						"— niemand —", alignCenter)
					return
				}

				p1 := easeOutBack(clamp01((t - 0.2) / 0.7))
				if p1 > 0 {
					name := fitText(data.BoldFont, 38, s.topMatePairA, wrappedW-40)
					drawText(img, data.BoldFont, 38*p1, wrappedW/2, 200, white,
						name, alignCenter)
				}
				if t > 0.5 {
					pAmp := easeOutBack(clamp01((t - 0.5) / 0.4))
					drawText(img, data.BoldFont, 28*pAmp, wrappedW/2, 250, softYellow,
						"&", alignCenter)
				}
				if t > 0.7 {
					p2 := easeOutBack(clamp01((t - 0.7) / 0.7))
					name := fitText(data.BoldFont, 38, s.topMatePairB, wrappedW-40)
					drawText(img, data.BoldFont, 38*p2, wrappedW/2, 310, white,
						name, alignCenter)
				}
				if t > 1.4 {
					p := easeOutCubic(clamp01((t - 1.4) / 0.8))
					drawText(img, data.NormalFont, 24*p, wrappedW/2, 400, softYellow,
						"gemeinsam "+data.FormatTime(s.topMatePairMin), alignCenter)
				}
			},
		},

		// 7. biggest day
		{
			duration: 2.4,
			bgTop:    color.RGBA{255, 200, 80, 255},
			bgBot:    color.RGBA{180, 60, 30, 255},
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

		// 8. longest session
		{
			duration: 2.8,
			bgTop:    color.RGBA{120, 80, 220, 255},
			bgBot:    color.RGBA{30, 20, 90, 255},
			accent:   gold,
			draw: func(img *image.RGBA, t, off float64) {
				drawParticles(img, confetti, off+t)

				drawText(img, data.NormalFont, 26, wrappedW/2, 130, white,
					"Längste Session", alignCenter)

				progress := easeOutCubic(clamp01((t - 0.2) / 1.4))
				cur := uint32(float64(s.longestSession) * progress)
				drawText(img, data.BoldFont, 100, wrappedW/2, 260, white,
					data.FormatTime(cur), alignCenter)

				if t > 1.2 {
					p := easeOutCubic(clamp01((t - 1.2) / 0.8))
					name := fitText(data.NormalFont, 24, s.longestSessionUser, wrappedW-40)
					drawText(img, data.NormalFont, 24*p, wrappedW/2, 330, softYellow,
						"von "+name, alignCenter)
				}
				if t > 1.5 {
					p := easeOutCubic(clamp01((t - 1.5) / 0.8))
					d := s.longestSessionDate
					drawText(img, data.NormalFont, 20*p, wrappedW/2, 390, white,
						fmt.Sprintf("am %d.%d.%d", d.Day, d.Month, d.Year), alignCenter)
				}
			},
		},

		// 9. outro
		{
			duration: 2.8,
			bgTop:    color.RGBA{60, 180, 220, 255},
			bgBot:    color.RGBA{140, 50, 200, 255},
			accent:   gold,
			draw: func(img *image.RGBA, t, off float64) {
				drawParticles(img, confetti, off+t*2)

				p := easeOutBack(clamp01(t / 0.8))
				drawText(img, data.BoldFont, 56*p, wrappedW/2, 210, white,
					"Danke!", alignCenter)
				if t > 0.6 {
					p2 := easeOutCubic(clamp01((t - 0.6) / 0.8))
					drawText(img, data.NormalFont, 26*p2, wrappedW/2, 280, white,
						"Bis nächstes Jahr im Wald!", alignCenter)
				}
				if t > 1.2 {
					p3 := easeOutCubic(clamp01((t - 1.2) / 0.8))
					drawText(img, data.NormalFont, 18*p3, wrappedW/2, 410, softYellow,
						fmt.Sprintf("WALD SERVER WRAPPED %d", s.year), alignCenter)
				}
			},
		},
	}
}

// ─── auto-post ───────────────────────────────────────────────────────────────

// CheckAutoPostServerWrapped posts the server-wrapped GIF for the most-recent
// fully-elapsed year if it hasn't been posted yet. Safe to call every minute;
// it early-exits in the common case and persists progress in GuildData so a
// restart doesn't double-post.
//
// First-deploy behavior: on a fresh install the cursor starts at 0, so the
// most recent completed year will be posted once when the bot first runs.
func CheckAutoPostServerWrapped(guildID, channelID string) {
	if guildID == "" || channelID == "" {
		return
	}
	prevYear := time.Now().Year() - 1
	if int(data.GuildData.LastServerWrappedYear) >= prevYear {
		return
	}

	stats := computeServerStats(guildID, prevYear)
	if stats.totalMinutes == 0 {
		// No data for that year — bump cursor so we don't recompute every
		// minute forever.
		data.GuildData.LastServerWrappedYear = int16(prevYear)
		data.SaveData()
		return
	}

	buf := renderScenes(buildServerScenes(stats))
	if buf == nil {
		log.Println("auto-post server wrapped: render returned nil")
		return
	}

	_, err := data.Dc.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Content: fmt.Sprintf("**Server Wrapped %d** ist da! :tada:", prevYear),
		Files: []*discordgo.File{{
			Name:        fmt.Sprintf("server-wrapped-%d.gif", prevYear),
			ContentType: "image/gif",
			Reader:      bytes.NewReader(buf),
		}},
	})
	if err != nil {
		log.Println("auto-post server wrapped:", err)
		return
	}
	data.GuildData.LastServerWrappedYear = int16(prevYear)
	data.SaveData()
}
