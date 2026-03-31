package main

import (
	"log"
	"news_bot/configures"
	"news_bot/echoer"
	"news_bot/matrix_bot"
	"news_bot/system"
	"news_bot/tel_bot"
	"news_bot/tel_client_echoer"
	"sync"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"
)

func main() {
	conf, err := configures.LoadConfig()
	if err != nil {
		log.Fatal(err)
	}

	client, err := mautrix.NewClient(conf.Homeserver, id.UserID(conf.UserID), conf.MatrixAccessToken)
	if err != nil {
		log.Fatalf("Failed to create Matrix client: %v", err)
	}

	matrixRoomAutoSender := matrix_bot.NewRoomAutoSender(client, conf.RoomID)
	matrixRoomReader := matrix_bot.NewCodeReader(client, conf.CodeReaderRoomId)

	telBot, err := tel_bot.NewTelegramBotWithProxy(conf.BotToken, conf.ProxyURL)
	if err != nil {
		log.Fatalf("Failed to create Telegram bot: %v", err)
	}

	matrixRoomAutoSender.Start(4)
	echoerServices := []system.Echoer{
		echoer.NewService(telBot, conf.ChannelBotChatId, matrixRoomAutoSender),
		tel_client_echoer.NewService(conf.TelClientChannelChatIds, matrixRoomAutoSender, matrixRoomReader, conf.TelClientConfig),
	}

	//fmt.Print(matrixRoomReader.ReadCode(context.Background(), nil))

	wg := sync.WaitGroup{}
	for _, service := range echoerServices {
		//for _, _ = range echoerServices {
		wg.Add(1)
		go func() {
			defer wg.Done()
			service.Start()
		}()
	}
	wg.Wait()
}
