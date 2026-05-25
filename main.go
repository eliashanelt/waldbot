package main

import (
	"fmt"
	"log"
	"math"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"waldbot/command"
	"waldbot/data"
	"waldbot/date"
	"waldbot/oauth"

	"github.com/bwmarrin/discordgo"
)

var (
	targetWidth int
	guild *discordgo.Guild
)

func getEnv(name string) string {
	env := os.Getenv(name)
	if env == "" {
		fmt.Println("Please set the environment variable '" + name + "'!")
		os.Exit(-1)
	}
	return env
}

func main() {
	fmt.Println("Starting Waldbot...")
	data.LoadData()
	loadConfig()
	data.LoadFonts()
	targetWidth = data.StringWidth(data.BoldFont, "__Osabama gone Oase") + data.StringWidth(data.NormalFont, ": 10000:00h")


	fmt.Println("NOWEB:", os.Getenv("NOWEB"))
	web := os.Getenv("NOWEB") != "true"
	if !web {
		fmt.Println("No web mode enabled!") 
	} else {
		oauth.ClientId = getEnv("WALDBOT_CLIENTID")
		oauth.ClientSecret = getEnv("WALDBOT_CLIENTSECRET")
	}
	token := getEnv("WALDBOT_TOKEN")

	data.ReadDays("./data/days/")
	if token == "" {
		fmt.Println("Please set the environment variable 'WALDBOT_TOKEN'")
	}

	if token == "" {
		fmt.Println("Could not find the bot token, please set the environment variable 'WALDBOT_TOKEN' to the application token!")
		return
	}
    dc, err := discordgo.New("Bot " + token)
	if err != nil {
		fmt.Println("Error while starting discord session, ", err)
        return
	}
    data.Dc = dc

	// handlers
	dc.AddHandler(command.InteractionHandler)
    dc.AddHandler(readyHandler)
    dc.AddHandler(messageReceiveHandler)
	dc.Identify.Intents = discordgo.IntentsAll
	dc.StateEnabled = true

	// Open a websocket connection to Discord and begin listening.
	err = dc.Open()
	if err != nil {
		fmt.Println("error opening connection,", err)
		return
	}

	guild, err = dc.Guild(config.GuildId)
	if err != nil {
		log.Fatal("Could not find guild specified in config!")
	}

	if web { addWebHandlers() }

	fmt.Println("Bot is now running!")

	date.CurrentDay = date.FromTime(time.Now())
	
	updateRankings()
	cleanUpRoles()

	mainLoop()
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc
	fmt.Println("Stopping...")
	dc.Close()
	data.FinishSessions()
	saveFiles()
    command.UnregisterCommands(dc)
}

func mainLoop() {
	currentTime := time.Now()
	currentTime.Second()
	quit := make(chan struct{})
	go func() {
		time.Sleep(time.Duration(60 - currentTime.Second()) * time.Second)
        minute()
		ticker := time.NewTicker(60 * time.Second)
    	for {
			select {
			case <- ticker.C:
                minute()
        		case <- quit:
            		ticker.Stop()
            		return
        	}
    	}
 	}()
}

func readyHandler(s *discordgo.Session, ready *discordgo.Ready) {
    fmt.Println("Bot is ready!")
    command.RegisterCommands(s, config.GuildId)
}

func messageReceiveHandler(s *discordgo.Session, msg *discordgo.MessageCreate) {
    content := msg.Content
    isTryingToRunCommand := false
    if strings.HasPrefix(content, "!help") {
        isTryingToRunCommand = true
    } else if strings.HasPrefix(content, "!") {
        cmd := content[1:]
        for _, slashCommand := range command.SlashCommands {
            if strings.HasPrefix(cmd, slashCommand.Name) {
                isTryingToRunCommand = true
                break
            }
        }
    }
    if isTryingToRunCommand {
        s.ChannelMessageSend(msg.ChannelID,
            "Der Waldläufer-Bot benutzt jetzt Slash Commands. " +
            "Probier Mal, ein Slash: **/** einzutippen und sieh dir die Vorschläge an.",
        )
    }
}

func minute() {
    data.MinuteUpdate(config.GuildId, saveFiles)
    cleanUpRoles()
    updateRankings()
    command.CheckAutoPostServerWrapped(config.GuildId, config.StatsChannelId)
}

func updateRankings() {
	daily, _ := data.CalculateUserMinutes(date.DailyCondition, true)
	data.Dc.ChannelMessageEdit(config.StatsChannelId, fmt.Sprint(data.GuildData.ServerdatenDailyMessageId),
        createRankingsMessage("**Heutige Sprachchatzeiten**", daily, 1))

	weekly, weeklyDays := data.CalculateUserMinutes(date.WeeklyCondition, true)
	data.Dc.ChannelMessageEdit(config.StatsChannelId, fmt.Sprint(data.GuildData.ServerdatenWeeklyMessageId),
        createRankingsMessage("**Wöchentliche Sprachchatzeiten**", weekly, weeklyDays))

	allTime, allTimeDays := data.CalculateUserMinutes(date.AllTimeCondition, true)
	data.Dc.ChannelMessageEdit(config.StatsChannelId, fmt.Sprint(data.GuildData.ServerdatenMessageId),
        createRankingsMessage("**Gesamte Sprachchatzeiten**", allTime, allTimeDays))

	// api stats
	apiStats = ApiStats {
		Ranking: make([]ApiUserStats, len(allTime)),
	}
	for i, user := range allTime {
		name := ""
		if member, err := data.Dc.State.Member(config.GuildId, data.LongUserId(user.UserId)); err == nil {
			name = data.EffectiveName(member)
		}

		apiStats.Ranking[i] = ApiUserStats {
			Username: name,
			Minutes: user.Minutes,
		}
	}
}

func createRankingsMessage(header string, stats []data.UserStats, dayCount int) string {
	maxHeaderLength := len(header) + 30
	allMinutes := uint32(0)
	rankings := ""
	for n, entry := range stats {
		rank := n + 1
		allMinutes += entry.Minutes
		if rank <= 15 {
			linePrefix := ""
			if rank < 10 {
				linePrefix += ":zero:" + data.DigitEmote(rank)
			} else {
				linePrefix += ":one:" + data.DigitEmote(rank - 10)
			}
			linePrefix += ": "
			member, err := data.Dc.State.Member(config.GuildId, data.LongUserId(entry.UserId))
			var name string
			if err != nil {
				name = "[Unbekannt]"
			} else {
				name = data.EffectiveName(member)
			}
			if len(name) > 18 {
				name = name[:15] + "..."
			}
			time := data.FormatTime(entry.Minutes)
			line := linePrefix + "**" + name + "**: " + time
			width := data.StringWidth(data.BoldFont, name) + data.StringWidth(data.NormalFont, ": " + time)
			spaceLength := data.CharWidth(data.NormalFont, ' ')
			spacesToAdd := float64(targetWidth - width) / float64(spaceLength)
			for i := 0; i < int(math.Round(spacesToAdd)); i++ {
				line += " "
			}
			// Topkanal
			var topChannel int16
			topMinutes := uint32(0)
			for channelID, minutes := range entry.Channels {
				if minutes > topMinutes {
					topChannel = channelID
					topMinutes = minutes
				}
			}
			channelName := "[Gelöschter Kanal]"
			if ch, err := data.Dc.State.Channel(data.LongChannelId(topChannel)); err == nil {
				channelName = ch.Name
			}
			line += "Topkanal: " + channelName + " (" + data.FormatTime(topMinutes) + ")\n"
			if len(rankings) + len(line) + maxHeaderLength >= 2000 {
				continue
			}
			rankings += line
		}
	}
	if dayCount > 1 {
		header += " (" + fmt.Sprint(dayCount) + " Tage, " + data.FormatTime(allMinutes) + ")"
	} else {
		header += " (" + data.FormatTime(allMinutes) + ")"
	}
	return header + "\n\n" + rankings + "\n" + "‎" // <--- there is a left-to-right mark here
}

func saveFiles() {
	data.SaveData()
	if currentDayData, ok := data.DayData[date.CurrentDay]; ok {
		file, err := os.Create(fmt.Sprint("./data/days/",
            date.CurrentDay.Day, "-", date.CurrentDay.Month, "-", date.CurrentDay.Year, ".day"))
		defer file.Close()
		if err != nil {
			log.Fatal("Could not save day data to file: ", err)
		}
		currentDayData.Save(file)
	}
}

const HELIUM_ID = "386512906549985300"

var ROLES = [...]string {
	"386512906549985300", // helium
	"386513014733406230", // ...
	"386513346381479936",
	"386513402878623747",
	"386513476337795081",
	"386513575587610624", // radon
}

func contains(s []string, e string) bool {
    for _, a := range s {
        if a == e {
            return true
        }
    }
    return false
}

func cleanUpRoles() {
	for _, m := range guild.Members {
		maxRole := -1
		for i, role := range ROLES {
			if contains(m.Roles, role) {
				maxRole = i
			}
		}
		for i := 0; i < maxRole; i++ {
			roleIndex := -1
			for j, role := range m.Roles {
				if role == ROLES[i] {
					roleIndex = j
				}
			}
			if roleIndex == -1 { continue }

			m.Roles = append(m.Roles[:roleIndex], m.Roles[roleIndex+1:]...)
			_, err := data.Dc.State.Guild(guild.ID)
			if err != nil {
				fmt.Println("Failed to update guild state", err)
            }
			
            _, err = data.Dc.GuildMemberEdit(m.GuildID, m.User.ID, &discordgo.GuildMemberParams{Roles: &m.Roles})
			if err == nil {
				fmt.Println("Removed role ", ROLES[i], "from", m.User.Username)
			} else {
				fmt.Println("Failed to remove role ", ROLES[i], "from", m.User.Username)
			}
		}
		if maxRole == -1 {
			m.Roles = append(m.Roles, HELIUM_ID)
			_, err := data.Dc.State.Guild(guild.ID)	
			if err != nil {
				fmt.Println("Failed to update guild state", err)
			}
            _, err = data.Dc.GuildMemberEdit(m.GuildID, m.User.ID, &discordgo.GuildMemberParams{Roles: &m.Roles})
			if err == nil {
				fmt.Println("Added Helium role to", m.User.Username)
			} else {
				fmt.Println("Failed to remove helium role from", m.User.Username)
			}
		}
	}
}

