package command

import (
	"github.com/bwmarrin/discordgo"
)

func stonksslopeallResponse(query Query) (string, *discordgo.File) {
	if query.daywindow < 1 || query.daywindow > 100 {
		return "Daywindow muss zwischen 1 und 100 sein", nil
	}
	dates := getSortedDates()
	xValues, yValues := buildRollingAverageData(dates, query.dateCondition, nil, query.daywindow)
	return renderChart(xValues, yValues, "Stonkslope von allen Nutzern", "Stunden/Tag")
}
