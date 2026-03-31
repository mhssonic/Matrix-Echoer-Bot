package echoer

import (
	"fmt"
	"log"
	"news_bot/matrix_bot"
	"news_bot/system"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type telChannelEchoerImp struct {
	telBot        *tgbotapi.BotAPI
	tellChannelId int64
	matrixSender  matrix_bot.RoomAutoSender
}

func NewService(telBot *tgbotapi.BotAPI, tellChannelId int64, matrixSender matrix_bot.RoomAutoSender) system.Echoer {
	return &telChannelEchoerImp{
		telBot:        telBot,
		tellChannelId: tellChannelId,
		matrixSender:  matrixSender,
	}
}

func (t *telChannelEchoerImp) Start() {
	t.telBot.Debug = false // set true to see raw JSON in console

	log.Printf("✅ Telegram channel listener started as @%s", t.telBot.Self.UserName)
	log.Printf("👀 Listening for new posts in channel ID: %d", t.tellChannelId)

	// Long polling configuration
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	u.AllowedUpdates = []string{"channel_post"} // only care about channel posts

	updates := t.telBot.GetUpdatesChan(u)

	// Main loop – runs forever
	for update := range updates {
		if update.ChannelPost == nil {
			continue // ignore normal chats / group messages
		}

		// Only process messages from our target channel
		if update.ChannelPost.Chat.ID != t.tellChannelId {
			continue
		}

		msg := update.ChannelPost

		fmt.Println("\n" + strings.Repeat("═", 80))
		fmt.Printf("📅 %s  |  Post ID: %d\n", msg.Time().Format("2006-01-02 15:04:05"), msg.MessageID)
		fmt.Println(strings.Repeat("─", 80))

		// TEXT
		if msg.Text != "" {
			t.matrixSender.SendTextAsync(msg.Text)
		}

		//// PHOTO
		//if len(msg.Photo) > 0 {
		//	// Get the largest photo (last one in the array is highest resolution)
		//	photo := msg.Photo[len(msg.Photo)-1]
		//
		//	fmt.Println("📸 PHOTO:")
		//	if msg.Caption != "" {
		//		fmt.Printf("Caption: %s\n", msg.Caption)
		//	}
		//	file, _ := bot.GetFile(tgbotapi.FileConfig{FileID: photo.FileID})
		//	fmt.Printf("File ID   : %s\n", photo.FileID)
		//	fmt.Printf("Width × Height : %d × %d\n", photo.Width, photo.Height)
		//	fmt.Printf("File Size : %d bytes\n", photo.FileSize)
		//	fmt.Printf("File Unique ID: %s\n", photo.FileUniqueID)
		//
		//	// Optional: download the photo right now
		//	// file, err := bot.GetFile(tgbotapi.FileConfig{FileID: photo.FileID})
		//	// if err == nil {
		//	//     fmt.Printf("Download URL: %s\n", file.Link(bot.Token))
		//	// }
		//}
		//
		//// VIDEO
		//if msg.Video != nil {
		//	video := msg.Video
		//	fmt.Println("🎥 VIDEO:")
		//	if msg.Caption != "" {
		//		fmt.Printf("Caption: %s\n", msg.Caption)
		//	}
		//	fmt.Printf("File ID   : %s\n", video.FileID)
		//	fmt.Printf("Duration  : %d seconds\n", video.Duration)
		//	fmt.Printf("Width × Height : %d × %d\n", video.Width, video.Height)
		//	fmt.Printf("File Size : %d bytes\n", video.FileSize)
		//	fmt.Printf("Mime Type : %s\n", video.MimeType)
		//}
		//
		//// You can add more types here later (Document, Audio, etc.)
	}
}
