package command

import (
	"waldbot/data"

	"github.com/bwmarrin/discordgo"
)

func stonkslopeResponse(query Query) (string, *discordgo.File) {
	if query.daywindow < 1 || query.daywindow > 100 {
		return "Daywindow muss zwischen 1 und 100 sein", nil
	}
	memberId := data.ShortUserId(query.member.User.ID)
	dates := getSortedDates()
	xValues, yValues := buildRollingAverageData(dates, query.dateCondition, &memberId, query.daywindow)
	return renderChart(xValues, yValues, "Stonkslope von "+data.EffectiveName(query.member), "Stunden/Tag")
}
