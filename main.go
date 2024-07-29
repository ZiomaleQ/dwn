package main

import (
	"context"
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
	"github.com/lrstanley/go-ytdlp"
)

var UrlRegex = regexp.MustCompile(`https?:\/\/\S+\.+[a-z]{2,6}/[^>\s]*`)

var BotToken = os.Getenv("DISCORD_BOT_TOKEN")
var BotCommands = []discord.ApplicationCommandCreate{
	discord.MessageCommandCreate{
		Name: "Download media",
	},
}

func main() {
	ytdlp.MustInstall(context.TODO(), nil)

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

		cmd := exec.Cmd{
			// Path: os.Getenv("YTDL_PATH"),
			Path: `C:\PathExposed\yt-dlp.exe`,
			Args: []string{matches, "-o -"},
		}

		pipe, err := cmd.StdoutPipe()

		if err != nil {
			event.CreateMessage(discord.MessageCreate{
				Content: "Error while reading media file\n" + err.Error(),
			})
			return
		}

		stderr, err := cmd.StderrPipe()

		if err != nil {
			event.CreateMessage(discord.MessageCreate{
				Content: "Error while reading media file\n" + err.Error(),
			})
			return
		}

		err = cmd.Start()

		if err != nil {
			event.CreateMessage(discord.MessageCreate{
				Content: "Error while downloading media\n" + err.Error(),
				Flags:   discord.MessageFlagEphemeral,
			})
			return
		}

		go func() {
			buf := make([]byte, 1024)
			for {
				n, err := stderr.Read(buf)
				if err != nil {
					break
				}
				slog.Info(string(buf[:n]))
			}
		}()

		go func() {
			buf := make([]byte, 1024)
			for {
				n, err := pipe.Read(buf)
				if err != nil {
					break
				}
				slog.Info(string(buf[:n]))
			}
		}()

		event.CreateMessage(discord.MessageCreate{
			Content: "Downloaded media",
			// Files: []*discord.File{
			// 	{
			// 		Name:   "funny_video.mp4",
			// 		Reader: pipe,
			// 	},
			// },
		})
	}
}
