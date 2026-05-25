package data

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"time"

	"waldbot/date"
	"waldbot/oauth"

	"github.com/bwmarrin/discordgo"
)

var (
	Dc *discordgo.Session
	DayData map[date.Date]Day = map[date.Date]Day {}
	Sessions map[UserId]ActiveSession = map[UserId]ActiveSession {}
	GuildData Data
)

type ActiveSession struct {
	Session VoiceSession
	ChannelID ChannelId
}

type Data struct {
	ServerdatenMessageId       int64
	ServerdatenWeeklyMessageId int64
	ServerdatenDailyMessageId  int64

	ShortUserIds map[string]int16
	NextId       int16

	ShortChannelId map[string]int16
	NextChannelId  int16

	DynamicChannels map[string][]string

	OAuthLogins map[string]oauth.Login

	// LastServerWrappedYear is the most recent year for which the bot has
	// auto-posted a server wrapped. Used to avoid double-posting.
	LastServerWrappedYear int16
}

func LoadData() {
	fmt.Println("Loading data file...")
	file, err := os.Open("./data/data.json")
	if err != nil {
		log.Fatal("could not find data.json: ", err)
	}
	defer file.Close()
	bytes, _ := ioutil.ReadAll(file)
	err = json.Unmarshal(bytes, &GuildData)
	if GuildData.OAuthLogins == nil {
		GuildData.OAuthLogins = map[string]oauth.Login {}
	}
}

func SaveData() {
	fmt.Println("Saving data file...")
	file, _ := os.Create("./data/data.json")
	bytes, err := json.Marshal(&GuildData)
	if err != nil {
		log.Fatal("Json Marshal error:", err)
	}
	_, err = file.Write(bytes)
	if err != nil {
		log.Fatal("Could not save json: ", err)
	}
	file.Close()
}

func ShortUserId(id string) int16 {
	if val, ok := GuildData.ShortUserIds[id]; ok {
		return val
	}
	shortId := GuildData.NextId
	GuildData.NextId++
	GuildData.ShortUserIds[fmt.Sprint(id)] = shortId
	return shortId
}

func ShortChannelId(id string) int16 {
	if val, ok := GuildData.ShortChannelId[id]; ok {
		return val
	}
	shortId := GuildData.NextChannelId
	GuildData.NextChannelId++
	GuildData.ShortChannelId[fmt.Sprint(id)] = shortId
	return shortId
}

func LongChannelId(id int16) string {
	for longId, shortId := range GuildData.ShortChannelId {
		if shortId == id {
			return longId
		}
	}
	panic("Could not find long channel id by short id: " + fmt.Sprint(id))
}

func LongUserId(id int16) string {
	for longId, shortId := range GuildData.ShortUserIds {
		if shortId == id {
			return longId
		}
	}
	panic("Could not find long user id by short id: " + fmt.Sprint(id))
}

func AddOAuthSession(access string, refresh string, expiresIn uint64) string {
	found := true
	var token string
	// search for unique token
	for found {
		token = randomBase64String(32)
		_, found = GuildData.OAuthLogins[token]
	}
	now := time.Now()
	GuildData.OAuthLogins[token] = oauth.Login {
		AccessToken: access,
		RefreshToken: refresh,
		AccessExpires: uint64(now.Add(time.Duration(expiresIn) * time.Second).Unix()),
		Expires: uint64(now.Add(24 * time.Hour * 30).Unix()), // session expires after 30 days
	}
	return token
}

func GetOAuthSession(token string) *oauth.Login {
	if session, ok := GuildData.OAuthLogins[token]; ok {
		now := time.Now()
		// update expiry
		session.Expires = uint64(now.Add(24 * time.Hour * 90).Unix())

		if session.AccessExpires < uint64(now.Unix()) + 60 {
			fmt.Println("Refreshing OAuth2 access token")
			access, err := oauth.RefreshToken(session.RefreshToken)
			if err != nil {
				fmt.Println("Error while refreshing token:", err)
				return nil
			}
			session.AccessToken = access.AccessToken
			session.AccessExpires = uint64(now.Add(time.Duration(access.ExpiresIn) * time.Second).Unix())
		}

		GuildData.OAuthLogins[token] = session
		// return found session
		return &session
	} else {
		return nil
	}
}

func RemoveOAuthSession(token string) bool {
	if _, ok := GuildData.OAuthLogins[token]; ok {
		delete(GuildData.OAuthLogins, token)
		return true
	}
	return false
}

func randomBase64String(n int) string {
	var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789+-")
	b := make([]rune, n)
    for i := range b {
        b[i] = letterRunes[rand.Intn(len(letterRunes))]
    }
    return string(b)
}
