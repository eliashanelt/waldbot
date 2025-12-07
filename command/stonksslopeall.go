package command

import (
	"bytes"
	"log"
	"sort"
	"time"
	"waldbot/data"
	"waldbot/date"

	"github.com/bwmarrin/discordgo"
	"github.com/wcharczuk/go-chart/v2"
)

func stonksslopeallResponse(query Query) (string, *discordgo.File) {
	if query.daywindow < 1 || query.daywindow > 100 {
		return "Daywindow muss zwischen 1 und 100 sein", nil
	}

	dates := make([]date.Date, 0, len(data.DayData))

	for date := range data.DayData {
		dates = append(dates, date)
	}

	sort.Slice(dates, func(i, j int) bool {
		return date.IsSmaller(dates[i], dates[j])
	})

	starting := true

	var xValues []time.Time
	var yValues []float64
	var nday uint = 0
	for j, date := range dates {
		nday++

		var minutes uint32
		var dayscounted uint32
		for i := 0; i < int(query.daywindow); i++ {
			if j+i >= len(dates) {
				break
			}
			if !query.dateCondition(dates[j+i]) {
				continue
			}
			dayscounted++
			for _, channel := range data.DayData[dates[j+i]].Channels {
				for _, session := range channel {
					minutes += uint32(session.Minutes)
				}
			}
		}

		if minutes > 0 || !starting {
			starting = false
			xValues = append(xValues, time.Date(int(date.Year), time.Month(date.Month), int(date.Day), 0, 0, 0, 0, time.Local))
			yValues = append(yValues, float64(minutes)/float64(dayscounted))
		}
	}
	if len(xValues) < 2 {
		return "Nicht genug Daten gefunden!", nil
	}
	graph := chart.Chart{
		Width:  1280,
		Height: 720,
		Background: chart.Style{
			Padding: chart.Box{
				Top:    80,
				Left:   10,
				Right:  10,
				Bottom: 10,
			},
		},
		ColorPalette: waldColorPalette,
		Title:        "Stonkslope von allen Nutzern",
		XAxis: chart.XAxis{
			Name:           "Datum",
			ValueFormatter: chart.TimeDateValueFormatter,
		},
		YAxis: chart.YAxis{
			Name: "Stunden/Tag",
			ValueFormatter: func(v interface{}) string {
				if typed, isTyped := v.(float64); isTyped {
					return data.FormatTime(uint32(typed))
				}
				return "error"
			},
		},
		Series: []chart.Series{
			chart.TimeSeries{
				XValues: xValues,
				YValues: yValues,
			},
		},
	}
	buffer := bytes.NewBuffer([]byte{})
	err := graph.Render(chart.PNG, buffer)
	if err != nil {
		log.Println("Error while creating diagram: ", err)
		return "Diagram error", nil
	}
	return "", &discordgo.File{
		Name:        "stonks.png",
		ContentType: "image/png",
		Reader:      bytes.NewReader(buffer.Bytes()),
	}
}
