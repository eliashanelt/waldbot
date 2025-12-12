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

// getSortedDates returns all dates from DayData sorted chronologically
func getSortedDates() []date.Date {
	dates := make([]date.Date, 0, len(data.DayData))
	for date := range data.DayData {
		dates = append(dates, date)
	}
	sort.Slice(dates, func(i, j int) bool {
		return date.IsSmaller(dates[i], dates[j])
	})
	return dates
}

// aggregateMinutes sums up minutes for all sessions in a date's channels
// If userID is nil, counts sessions for all users. Otherwise, only counts sessions for the specified user.
func aggregateMinutes(date date.Date, userID *data.UserId) uint32 {
	var minutes uint32
	for _, channel := range data.DayData[date].Channels {
		for _, session := range channel {
			if userID == nil || session.UserID == *userID {
				minutes += uint32(session.Minutes)
			}
		}
	}
	return minutes
}

// buildCumulativeData creates time series data with cumulative minutes
func buildCumulativeData(dates []date.Date, dateCondition func(date.Date) bool, userID *data.UserId) ([]time.Time, []float64) {
	var xValues []time.Time
	var yValues []float64
	var allTimeMinutes uint32
	starting := true

	for _, date := range dates {
		if !dateCondition(date) {
			continue
		}
		minutes := aggregateMinutes(date, userID)
		if minutes > 0 || !starting {
			starting = false
			allTimeMinutes += minutes
			xValues = append(xValues, time.Date(int(date.Year), time.Month(date.Month), int(date.Day), 0, 0, 0, 0, time.Local))
			yValues = append(yValues, float64(allTimeMinutes))
		}
	}
	return xValues, yValues
}

// buildRollingAverageData creates time series data with rolling window average
func buildRollingAverageData(dates []date.Date, dateCondition func(date.Date) bool, userID *data.UserId, dayWindow int64) ([]time.Time, []float64) {
	var xValues []time.Time
	var yValues []float64
	starting := true

	for j, date := range dates {
		var minutes uint32
		var dayscounted uint32

		for i := 0; i < int(dayWindow); i++ {
			if j+i >= len(dates) {
				break
			}
			if !dateCondition(dates[j+i]) {
				continue
			}
			dayscounted++
			minutes += aggregateMinutes(dates[j+i], userID)
		}

		if minutes > 0 || !starting {
			starting = false
			xValues = append(xValues, time.Date(int(date.Year), time.Month(date.Month), int(date.Day), 0, 0, 0, 0, time.Local))
			yValues = append(yValues, float64(minutes)/float64(dayscounted))
		}
	}
	return xValues, yValues
}

// renderChart creates and renders a chart with the given parameters
func renderChart(xValues []time.Time, yValues []float64, title string, yAxisName string) (string, *discordgo.File) {
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
		Title:        title,
		XAxis: chart.XAxis{
			Name:           "Datum",
			ValueFormatter: chart.TimeDateValueFormatter,
		},
		YAxis: chart.YAxis{
			Name: yAxisName,
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
