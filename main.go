package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"syscall"

	"github.com/disgoorg/disgo"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
)

var UrlRegex = regexp.MustCompile(`https?:\/\/\S+\.+[a-z]{2,6}/[^>\s]*`)
var BotToken string
var YTDLPPath string

var BotCommands = []discord.ApplicationCommandCreate{
	discord.MessageCommandCreate{
		Name: "Download media",
		Contexts: []discord.InteractionContextType{
			discord.InteractionContextTypeGuild,
			discord.InteractionContextTypeBotDM,
			discord.InteractionContextTypePrivateChannel,
		},
		IntegrationTypes: []discord.ApplicationIntegrationType{
			discord.ApplicationIntegrationTypeGuildInstall,
			discord.ApplicationIntegrationTypeUserInstall,
		},
	},
}

func main() {
	if len(os.Args) > 2 {
		config, err := ReadConfig(os.Args[1])

		if err != nil {
			panic(err)
		}

		BotToken = config.Token
		YTDLPPath = config.YtDlp
	} else {
		BotToken = os.Getenv("DISCORD_BOT_TOKEN")
		YTDLPPath = os.Getenv("YTDLP_PATH")
	}

	client, err := disgo.New(BotToken,
		bot.WithDefaultGateway(),
		bot.WithEventListenerFunc(commandListener),
	)
	if err != nil {
		slog.Error("error while building disgo instance", slog.Any("err", err))
		return
	}

	defer client.Close(context.TODO())

	if _, err = client.Rest().SetGlobalCommands(client.ApplicationID(), BotCommands); err != nil {
		slog.Error("error while registering commands", slog.Any("err", err))
	}

	if err = client.OpenGateway(context.TODO()); err != nil {
		slog.Error("error while connecting to gateway", slog.Any("err", err))
	}

	slog.Info("Discord bot is running. Press CTRL-C to exit.")
	s := make(chan os.Signal, 1)
	signal.Notify(s, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-s
}

func commandListener(event *events.ApplicationCommandInteractionCreate) {
	data := event.MessageCommandInteractionData()
	if data.CommandName() == "Download media" {
		matches := UrlRegex.FindString(data.TargetMessage().Content)

		if matches == "" {
			event.CreateMessage(discord.MessageCreate{
				Content: "No media found",
				Flags:   discord.MessageFlagEphemeral,
			})
			return
		}

		event.DeferCreateMessage(false)

		var mediaInfo MediaInfo

		if rawMediaInfo, err := GetInfo(matches); err != nil {
			CreateFollowupMessage(event, discord.MessageCreate{Content: "Error while reading media info\n" + err.Error()})
			return
		} else {

			if rawMediaInfo.Ext == "opus" {
				rawMediaInfo.Ext = "ogg"
			}

			mediaInfo = *rawMediaInfo
		}

		cmd := exec.Cmd{
			Path: YTDLPPath,
			Args: []string{YTDLPPath, "-o", "-", "-S", "res:720,filesize~20M", matches},
		}

		pipe, err := cmd.StdoutPipe()

		if err != nil {
			CreateFollowupMessage(event, discord.MessageCreate{Content: "Error while downloading media\n" + err.Error()})
			return
		}

		stderr, err := cmd.StderrPipe()

		if err != nil {
			CreateFollowupMessage(event, discord.MessageCreate{Content: "Error while downloading media\n" + err.Error()})
			return
		}

		go io.Copy(os.Stderr, stderr)

		if err = cmd.Start(); err != nil {
			CreateFollowupMessage(event, discord.MessageCreate{Content: "Error while downloading media\n" + err.Error()})
			return
		}

		event.Client().Rest().CreateFollowupMessage(event.ApplicationID(), event.Token(),
			discord.MessageCreate{
				Files: []*discord.File{
					{
						Name:   fmt.Sprintf("%s.%s", mediaInfo.DisplayID, mediaInfo.Ext),
						Reader: pipe,
					},
				},
			})
	}
}

type MediaInfo struct {
	Ext       string
	DisplayID string `json:"display_id"`
}

func GetInfo(url string) (*MediaInfo, error) {

	cmd := exec.Cmd{
		Path: YTDLPPath,
		Args: []string{YTDLPPath, "-J", "-S", "res:720,filesize~20M", url},
	}

	var info MediaInfo

	data, err := cmd.Output()

	if err != nil {
		return nil, err
	}

	json.Unmarshal(data, &info)

	return &info, nil
}

func CreateFollowupMessage(event *events.ApplicationCommandInteractionCreate, message discord.MessageCreate) error {
	_, err := event.Client().Rest().CreateFollowupMessage(event.ApplicationID(), event.Token(), message)

	if err != nil {
		return err
	}

	return nil
}

type Config struct {
	Token string `json:"DISCORD_BOT_TOKEN"`
	YtDlp string `json:"YTDLP_PATH"`
}

func ReadConfig(path string) (*Config, error) {
	var config Config

	configFile := os.Args[1]

	data, err := os.ReadFile(configFile)

	if err != nil {
		slog.Error("error while reading config file", slog.Any("err", err))
		return nil, err
	}

	if err = json.Unmarshal(data, &config); err != nil {
		slog.Error("error while unmarshalling config file", slog.Any("err", err))
		return nil, err
	}

	return &config, nil
}
