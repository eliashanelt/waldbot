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

func stonksallResponse(query Query) (string, *discordgo.File) {
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
	var allTimeMinutes uint32
	for _, date := range dates {
		if !query.dateCondition(date) {
			continue
		}
		var minutes uint32
		for _, channel := range data.DayData[date].Channels {
			for _, session := range channel {
				minutes += uint32(session.Minutes)
			}
		}
		if minutes > 0 || !starting {
			starting = false
			allTimeMinutes += minutes
			xValues = append(xValues, time.Date(int(date.Year), time.Month(date.Month), int(date.Day), 0, 0, 0, 0, time.Local))
			yValues = append(yValues, float64(allTimeMinutes))
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
		Title:        "Stonks von allen Nutzern",
		XAxis: chart.XAxis{
			Name:           "Datum",
			ValueFormatter: chart.TimeDateValueFormatter,
		},
		YAxis: chart.YAxis{
			Name: "Stunden",
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
