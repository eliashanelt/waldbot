package command

import (
	"waldbot/data"

	"github.com/bwmarrin/discordgo"
)

func stonksResponse(query Query) (string, *discordgo.File) {
	memberId := data.ShortUserId(query.member.User.ID)
	dates := getSortedDates()
	xValues, yValues := buildCumulativeData(dates, query.dateCondition, &memberId)
	return renderChart(xValues, yValues, "Stonks von "+data.EffectiveName(query.member), "Stunden")
}
