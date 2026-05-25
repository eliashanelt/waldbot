package command

import (
	"fmt"
	"log"
	"waldbot/data"
	"waldbot/date"

	"github.com/bwmarrin/discordgo"
)

type Query struct {
	member        *discordgo.Member
	dateCondition date.DateCondition
	selfUser      bool
	mate          *discordgo.Member
	daywindow     int64
	year          int64
}

var (
	ZEITRAUM_OPTION = discordgo.ApplicationCommandOption{
		Type:        discordgo.ApplicationCommandOptionString,
		Name:        "zeitraum",
		Description: "z.B. daily', 'weekly', 'yearly', '1.2.2021', '15.4.2020-17.6.2020'",
		// choices geht nicht, da dann andere Werte nicht mehr akzeptiert werden
		/*
		   Choices: []*discordgo.ApplicationCommandOptionChoice {
		       {
		           Name: "daily",
		           Value: "daily",
		       },
		       {
		           Name: "weekly",
		           Value: "weekly",
		       },
		       {
		           Name: "yearly",
		           Value: "yearly",
		       },
		       {
		           Name: "1.1.2023-9.1.2023",
		           Value: "1.1.2023-9.1.2023",
		       },
		   },
		*/
		Required: false,
	}

	NUTZER_OPTION = discordgo.ApplicationCommandOption{
		Type:        discordgo.ApplicationCommandOptionUser,
		Name:        "nutzer",
		Description: "Nutzer, dessen Zeit angezeigt werden soll",
		Required:    false,
	}

	MATE_OPTION = discordgo.ApplicationCommandOption{
		Type:        discordgo.ApplicationCommandOptionUser,
		Name:        "mate",
		Description: "Mate, mit dem die Zeit ausgewertet werden soll",
		Required:    true,
	}
	DAYWINDOW_OPTION = discordgo.ApplicationCommandOption{
		Type:        discordgo.ApplicationCommandOptionInteger,
		Name:        "daywindow",
		Description: "Zeitraum über den die Stundenzahl berechnet wird (default 30)",
		Required:    false,
	}
	YEAR_OPTION = discordgo.ApplicationCommandOption{
		Type:        discordgo.ApplicationCommandOptionInteger,
		Name:        "jahr",
		Description: "Jahr für die Wrapped-Übersicht (default aktuelles Jahr)",
		Required:    false,
	}
)

type Options struct {
	zeitraum  bool
	nutzer    bool
	mate      bool
	daywindow bool
	year      bool
}

type SlashCommand struct {
	Name        string
	description string
	response    func(Query) (string, *discordgo.File)
	options     Options
	// deferred sends a "Bot is thinking..." ack immediately and follows up
	// with the actual response once it's ready. Needed for any command whose
	// response can't be produced inside Discord's 3-second window.
	deferred bool
}

var (
	SlashCommands = []SlashCommand{
		{
			Name:        "time",
			description: "Zeigt Sprachchatzeit, Rang und Lieblingskanäle an",
			response:    timeResponse,
			options:     Options{zeitraum: true, nutzer: true},
		},
		{
			Name:        "hours",
			description: "Generiert ein Diagram mit der Sprachchat-Zeitverteilung über einen Zeitraum",
			response:    hoursResponse,
			options:     Options{zeitraum: true, nutzer: true},
		},
		{
			Name:        "channels",
			description: "Zeigt ein Tortendiagramm mit der Verteilung der genutzten Sprachkanäle",
			response:    channelsResponse,
			options:     Options{zeitraum: true, nutzer: true},
		},
		{
			Name:        "session",
			description: "Zeigt die aktuelle Voicechat-Session",
			response:    sessionResponse,
			options:     Options{nutzer: true},
		},
		{
			Name:        "mate",
			description: "Zeigt deine Zeit mit einem Nutzer an",
			response:    mateResponse,
			options:     Options{mate: true, zeitraum: true, nutzer: true},
		},
		{
			Name:        "mates",
			description: "Zeigt ein Tortendiagramm der Sprachchatzeit mit anderen Nutzern",
			response:    matesResponse,
			options:     Options{zeitraum: true, nutzer: true},
		},
		{
			Name:        "stonks",
			description: "Zeigt einen Grahpen der Sprachchatzeit über Zeit",
			response:    stonksResponse,
			options:     Options{zeitraum: true, nutzer: true},
		},
		{
			Name:        "usercount",
			description: "Zeigt die Besucher des Discords pro Tag an",
			response:    usercountResponse,
			options:     Options{zeitraum: true},
		},
		{
			Name:        "stonkslope",
			description: "Generiert ein Diagram mit der Sprachchat-Zeitverteilung über einen Zeitraum",
			response:    stonkslopeResponse,
			options:     Options{zeitraum: true, nutzer: true, daywindow: true},
		},
		{
			Name:        "stonksall",
			description: "Zeigt einen Graphen der Sprachchatzeit aller Nutzer über Zeit",
			response:    stonksallResponse,
			options:     Options{zeitraum: true},
		},
		{
			Name:        "stonksslopeall",
			description: "Generiert ein Diagram mit der Sprachchat-Zeitverteilung aller Nutzer über einen Zeitraum",
			response:    stonksslopeallResponse,
			options:     Options{zeitraum: true, daywindow: true},
		},
		{
			Name:        "wrapped",
			description: "Generiert ein animiertes GIF mit deinen Sprachchat-Highlights des Jahres",
			response:    wrappedResponse,
			options:     Options{nutzer: true, year: true},
			deferred:    true,
		},
		{
			Name:        "serverwrapped",
			description: "Generiert ein animiertes GIF mit den Server-Highlights des Jahres",
			response:    serverwrappedResponse,
			options:     Options{year: true},
			deferred:    true,
		},
	}
	registeredCommands = make([]*discordgo.ApplicationCommand, len(SlashCommands))
)

const FALSE bool = false

func RegisterCommands(dc *discordgo.Session, guildID string) {
	// clean up previous commands that might still be hanging around
	oldCommands, err := dc.ApplicationCommands(dc.State.User.ID, "")
	if err == nil {
		for _, command := range oldCommands {
			dc.ApplicationCommandDelete(dc.State.User.ID, "", command.ID)
		}
	} else {
		fmt.Println("Failed to retrieve old application commands:", err)
	}
	oldGuildCommands, err := dc.ApplicationCommands(dc.State.User.ID, guildID)
	if err == nil {
		for _, command := range oldGuildCommands {
			dc.ApplicationCommandDelete(dc.State.User.ID, guildID, command.ID)
		}
	} else {
		fmt.Println("Failed to retrieve old application commands:", err)
	}

	for i, cmd := range SlashCommands {
		options := make([]*discordgo.ApplicationCommandOption, 0)
		if cmd.options.nutzer {
			options = append(options, &NUTZER_OPTION)
		}
		if cmd.options.zeitraum {
			options = append(options, &ZEITRAUM_OPTION)
		}
		if cmd.options.daywindow {
			options = append(options, &DAYWINDOW_OPTION)
		}
		if cmd.options.year {
			options = append(options, &YEAR_OPTION)
		}
		fmt.Println("Adding command:", cmd.Name, "UserID: ", dc.State.User.ID)
		dmPermission := false
		command, err := dc.ApplicationCommandCreate(dc.State.User.ID, "", &discordgo.ApplicationCommand{
			Name:         cmd.Name,
			Description:  cmd.description,
			Options:      options,
			DMPermission: &dmPermission,
		})
		if err != nil {
			log.Printf("Cannot create '%v' command: %v", cmd.Name, err)
		}
		registeredCommands[i] = command
	}
}

func UnregisterCommands(dc *discordgo.Session) {
	for _, v := range registeredCommands {
		err := dc.ApplicationCommandDelete(dc.State.User.ID, "", v.ID)
		if err != nil {
			log.Panicf("Cannot delete '%v' command: %v", v.Name, err)
		}
	}
}

func memberValue(s *discordgo.Session, guildID string, o *discordgo.ApplicationCommandInteractionDataOption) *discordgo.Member {
	user := o.UserValue(nil)
	if user == nil {
		log.Println("User was nil")
		return nil
	}
	m, err := s.State.Member(guildID, user.ID)
	if err != nil {
		log.Println("User to member err: ", err)
		return nil
	}

	return m
}

func parseOptions(
	options []*discordgo.ApplicationCommandInteractionDataOption,
	member *discordgo.Member,
	guildID string,
	validOptions Options,
) (Query, string) {
	query := Query{
		member:        member,
		dateCondition: date.AllTimeCondition,
		selfUser:      true,
		daywindow:     30,
	}
	for _, option := range options {
		switch option.Name {
		case "mate":
			if !validOptions.mate {
				return query, "invalide Option 'mate'"
			}
			query.mate = memberValue(data.Dc, guildID, option)
		case "nutzer":
			if !validOptions.nutzer {
				return query, "invalide Option 'nutzer'"
			}
			query.member = memberValue(data.Dc, guildID, option)
			if query.member == nil {
				return query, "invalider Nutzer"
			}
			if query.member.User.ID != member.User.ID {
				query.selfUser = false
			}
		case "zeitraum":
			if !validOptions.zeitraum {
				return query, "invalide Option 'zeitraum'"
			}
			parsedDateCondition, status := data.ParseDateCondition(option.StringValue(), date.AllTimeCondition)
			if status != data.ParseSuccess {
				return query, "invalider Zeitraum"
			}
			query.dateCondition = parsedDateCondition
		case "daywindow":
			if !validOptions.daywindow {
				return query, "invalide Option 'daywindow'"
			}
			query.daywindow = option.IntValue()
		case "jahr":
			if !validOptions.year {
				return query, "invalide Option 'jahr'"
			}
			query.year = option.IntValue()
		default:
			return query, "invalides Argument " + option.Name
		}

	}

	return query, ""
}

func runResponse(cmd SlashCommand, query Query, parseErr string) (string, []*discordgo.File) {
	if parseErr != "" {
		return "Fehler: " + parseErr, nil
	}
	content, file := cmd.response(query)
	if file == nil {
		return content, nil
	}
	return content, []*discordgo.File{file}
}

func InteractionHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	for _, cmd := range SlashCommands {
		if cmd.Name != i.ApplicationCommandData().Name {
			continue
		}
		member, memberErr := s.GuildMember(i.GuildID, i.Member.User.ID)
		if memberErr != nil {
			log.Println("Couldn't get member in interaction handler: ", memberErr)
		}
		query, parseErr := parseOptions(i.ApplicationCommandData().Options, member, i.GuildID, cmd.options)

		if cmd.deferred {
			// Ack within 3s; we'll edit the response with real content below.
			if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
			}); err != nil {
				log.Println("Failed to send deferred ack:", err)
				return
			}
			content, files := runResponse(cmd, query, parseErr)
			edit := &discordgo.WebhookEdit{Content: &content, Files: files}
			if _, err := s.InteractionResponseEdit(i.Interaction, edit); err != nil {
				log.Println("Failed to edit deferred response:", err)
			}
			return
		}

		content, files := runResponse(cmd, query, parseErr)
		if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: content,
				Files:   files,
			},
		}); err != nil {
			log.Println("Failed to send interaction response:", err)
		}
		return
	}
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "Fehler: der Befehl '" + i.ApplicationCommandData().Name + "' existiert nicht :(",
		},
	})
}
