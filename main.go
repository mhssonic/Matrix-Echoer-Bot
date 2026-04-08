package main

import (
	"log"
	"sync"

	"echoer_bot/configures"
	"echoer_bot/echoer"
	"echoer_bot/matrix_bot"
	"echoer_bot/system"
	"echoer_bot/tel_bot"
	"echoer_bot/tel_client_echoer"
	"log"
	"sync"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"
)

func main() {
	conf, err := configures.LoadConfig()
	if err != nil {
		log.Fatal(err)
	}

	clients := make([]*mautrix.Client, 0, len(conf.MatrixDestinations))
	for _, d := range conf.MatrixDestinations {
		c, err := mautrix.NewClient(d.Homeserver, id.UserID(d.UserID), d.MatrixAccessToken)
		if err != nil {
			log.Fatalf("Failed to create Matrix client: %v", err)
		}
		clients = append(clients, c)
	}

	senders := make([]matrix_bot.RoomAutoSender, 0, len(conf.MatrixDestinations))
	for i, d := range conf.MatrixDestinations {
		senders = append(senders, matrix_bot.NewRoomAutoSender(clients[i], d.RoomID))
	}

	matrixRoomAutoSender, err := matrix_bot.NewMultiRoomAutoSender(senders)
	if err != nil {
		log.Fatal(err)
	}

	var matrixRoomReader matrix_bot.CodeReader
	if conf.CodeReaderRoomId != "" {
		matrixRoomReader = matrix_bot.NewCodeReader(clients[0], conf.CodeReaderRoomId)
	}

	matrixRoomAutoSender.Start(4)
	echoerServices := []system.Echoer{}

	if conf.BotToken != "" {
		telBot, err := tel_bot.NewTelegramBotWithProxy(conf.BotToken, conf.ProxyURL)
		if err != nil {
			log.Fatalf("Failed to create Telegram bot: %v", err)
		}
		echoerServices = append(echoerServices, echoer.NewService(telBot, conf.ChannelBotChatId, matrixRoomAutoSender, conf.DisableVideos))
	}

	if conf.TelClientConfig.ApiCode != 0 && conf.TelClientConfig.ApiHash != "" && conf.TelClientConfig.PhoneNumber != "" {
		echoerServices = append(echoerServices, tel_client_echoer.NewService(conf.TelClientChannelChatIds, matrixRoomAutoSender, matrixRoomReader, conf.TelClientConfig, conf.DisableVideos))
	}

	wg := sync.WaitGroup{}
	for _, service := range echoerServices {
		wg.Add(1)
		go func() {
			defer wg.Done()
			service.Start()
		}()
	}
	wg.Wait()
}
