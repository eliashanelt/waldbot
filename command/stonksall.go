package command

import (
	"github.com/bwmarrin/discordgo"
)

func stonksallResponse(query Query) (string, *discordgo.File) {
	dates := getSortedDates()
	xValues, yValues := buildCumulativeData(dates, query.dateCondition, nil)
	return renderChart(xValues, yValues, "Stonks von allen Nutzern", "Stunden")
}
