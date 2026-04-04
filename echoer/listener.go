package echoer

import (
	"context"
	"echoer_bot/lib/videokit"
	"echoer_bot/matrix_bot"
	"echoer_bot/system"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type telChannelEchoerImp struct {
	telBot        *tgbotapi.BotAPI
	tellChannelId int64
	matrixSender  matrix_bot.RoomAutoSender
	disableVideos bool

	mu            sync.Mutex
	albumBuffer   map[string][]*tgbotapi.Message
	albumDeadline map[string]*time.Timer
}

func NewService(telBot *tgbotapi.BotAPI, tellChannelId int64, matrixSender matrix_bot.RoomAutoSender, disableVideos bool) system.Echoer {
	return &telChannelEchoerImp{
		telBot:        telBot,
		tellChannelId: tellChannelId,
		matrixSender:  matrixSender,
		disableVideos: disableVideos,
		albumBuffer:   map[string][]*tgbotapi.Message{},
		albumDeadline: map[string]*time.Timer{},
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

		// Media (photos/videos) + albums (MediaGroupID)
		if msg.MediaGroupID != "" && (len(msg.Photo) > 0 || msg.Video != nil) {
			t.enqueueAlbum(msg.MediaGroupID, msg)
			continue
		}

		if len(msg.Photo) > 0 {
			if err := t.handlePhoto(msg, msg.Caption); err != nil {
				log.Printf("failed to handle photo: %v", err)
			}
		}

		if msg.Video != nil {
			if t.disableVideos {
				log.Printf("video forwarding disabled; skipping message %d", msg.MessageID)
				continue
			}
			if err := t.handleVideo(msg, msg.Caption); err != nil {
				log.Printf("failed to handle video: %v", err)
			}
		}
	}
}

func (t *telChannelEchoerImp) enqueueAlbum(groupID string, msg *tgbotapi.Message) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.albumBuffer[groupID] = append(t.albumBuffer[groupID], msg)
	if _, ok := t.albumDeadline[groupID]; ok {
		return
	}

	t.albumDeadline[groupID] = time.AfterFunc(1200*time.Millisecond, func() {
		t.flushAlbum(groupID)
	})
}

func (t *telChannelEchoerImp) flushAlbum(groupID string) {
	t.mu.Lock()
	msgs := t.albumBuffer[groupID]
	timer := t.albumDeadline[groupID]
	delete(t.albumBuffer, groupID)
	delete(t.albumDeadline, groupID)
	t.mu.Unlock()

	if timer != nil {
		timer.Stop()
	}
	if len(msgs) == 0 {
		return
	}

	sort.Slice(msgs, func(i, j int) bool { return msgs[i].MessageID < msgs[j].MessageID })

	albumCaption := ""
	for _, m := range msgs {
		if m.Caption != "" {
			albumCaption = m.Caption
			break
		}
	}

	captionOnMessageID := 0
	for _, m := range msgs {
		if len(m.Photo) > 0 {
			captionOnMessageID = m.MessageID
		}
	}
	if captionOnMessageID == 0 {
		for _, m := range msgs {
			if m.Video != nil {
				captionOnMessageID = m.MessageID
			}
		}
	}

	for _, m := range msgs {
		caption := ""
		if m.MessageID == captionOnMessageID {
			caption = albumCaption
		}
		if len(m.Photo) > 0 {
			if err := t.handlePhoto(m, caption); err != nil {
				log.Printf("failed to handle album photo: %v", err)
			}
			continue
		}
		if m.Video != nil {
			if t.disableVideos {
				continue
			}
			if err := t.handleVideo(m, caption); err != nil {
				log.Printf("failed to handle album video: %v", err)
			}
			continue
		}
	}
}

func (t *telChannelEchoerImp) handlePhoto(msg *tgbotapi.Message, caption string) error {
	if len(msg.Photo) == 0 {
		return fmt.Errorf("no photo in message")
	}

	photo := msg.Photo[len(msg.Photo)-1] // best quality

	ctx := context.Background()
	gf, err := t.telBot.GetFile(tgbotapi.FileConfig{FileID: photo.FileID})
	if err != nil {
		return fmt.Errorf("getFile: %w", err)
	}
	if gf.FileSize > 0 && int64(gf.FileSize) > videokit.MaxMatrixUploadBytes {
		if caption != "" {
			_ = t.matrixSender.SendTextSync(ctx, caption)
		}
		return nil
	}

	rc, contentLength, mimeType, filename, err := t.openFileStream(photo.FileID, "image/jpeg", "photo.jpg")
	if err != nil {
		return fmt.Errorf("open photo stream failed: %w", err)
	}
	defer rc.Close()

	if contentLength > 0 && contentLength > videokit.MaxMatrixUploadBytes {
		if caption != "" {
			_ = t.matrixSender.SendTextSync(ctx, caption)
		}
		return nil
	}

	return t.matrixSender.SendImageReaderSync(
		ctx,
		rc,
		contentLength,
		mimeType,
		filename,
		caption,
		photo.Width,
		photo.Height,
	)
}

func (t *telChannelEchoerImp) handleVideo(msg *tgbotapi.Message, caption string) error {
	if msg.Video == nil {
		return fmt.Errorf("no video in message")
	}

	video := msg.Video

	mimeType := video.MimeType
	if mimeType == "" {
		mimeType = "video/mp4"
	}

	filename := video.FileName
	if filename == "" {
		filename = "video.mp4"
	}

	ctx := context.Background()

	file, err := t.telBot.GetFile(tgbotapi.FileConfig{FileID: video.FileID})
	if err != nil {
		return fmt.Errorf("getFile: %w", err)
	}
	knownSize := int64(file.FileSize)

	durationMS := video.Duration * 1000

	if knownSize > 0 && knownSize <= videokit.MaxMatrixVideoBytes {
		rc, contentLength, mt, fn, err := t.openFileStream(video.FileID, mimeType, filename)
		if err != nil {
			return fmt.Errorf("open video stream failed: %w", err)
		}
		defer rc.Close()
		if contentLength > 0 && contentLength > videokit.MaxMatrixUploadBytes {
			if caption != "" {
				_ = t.matrixSender.SendTextSync(ctx, caption)
			}
			return nil
		}
		return t.matrixSender.SendVideoReaderSync(ctx, rc, contentLength, mt, fn, caption, video.Width, video.Height, durationMS)
	}

	tmpPath, _, cleanupTmp, err := t.downloadTelegramFileToTemp(video.FileID, "tgvid-*")
	if err != nil {
		return err
	}
	defer cleanupTmp()

	uploadPath, uploadSize, cleanupOut, ok, err := videokit.PrepareForMatrix(ctx, tmpPath)
	if err != nil {
		log.Printf("video compress failed, sending caption only: %v", err)
		if caption != "" {
			_ = t.matrixSender.SendTextSync(ctx, caption)
		}
		return nil
	}
	if !ok {
		if caption != "" {
			_ = t.matrixSender.SendTextSync(ctx, caption)
		}
		return nil
	}
	defer cleanupOut()

	if uploadSize > videokit.MaxMatrixUploadBytes {
		if caption != "" {
			_ = t.matrixSender.SendTextSync(ctx, caption)
		}
		return nil
	}

	f, err := os.Open(uploadPath)
	if err != nil {
		return err
	}
	defer f.Close()

	outName := filename
	if filepath.Ext(uploadPath) == ".mp4" {
		base := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
		if base == "" || base == "." {
			base = "video"
		}
		outName = base + ".mp4"
	}
	return t.matrixSender.SendVideoReaderSync(ctx, f, uploadSize, "video/mp4", outName, caption, video.Width, video.Height, durationMS)
}

// openFileStream resolves the file with getFile, then streams the body from Telegram (bot HTTP client).
func (t *telChannelEchoerImp) openFileStream(fileID, fallbackMimeType, fallbackFilename string) (io.ReadCloser, int64, string, string, error) {
	file, err := t.telBot.GetFile(tgbotapi.FileConfig{FileID: fileID})
	if err != nil {
		return nil, 0, "", "", fmt.Errorf("getFile: %w", err)
	}
	if file.FilePath == "" {
		return nil, 0, "", "", fmt.Errorf("getFile returned empty file_path")
	}

	req, err := http.NewRequest(http.MethodGet, file.Link(t.telBot.Token), nil)
	if err != nil {
		return nil, 0, "", "", err
	}

	resp, err := t.telBot.Client.Do(req)
	if err != nil {
		return nil, 0, "", "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		resp.Body.Close()
		return nil, 0, "", "", fmt.Errorf("telegram file GET: %s", resp.Status)
	}

	mimeType := resp.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = fallbackMimeType
	}

	filename := fallbackFilename
	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		_, params, parseErr := mime.ParseMediaType(cd)
		if parseErr == nil && params["filename"] != "" {
			filename = filepath.Base(params["filename"])
		}
	}
	if filename == "" {
		filename = fallbackFilename
	}

	contentLength := resp.ContentLength
	if contentLength <= 0 && file.FileSize > 0 {
		contentLength = int64(file.FileSize)
	}

	return resp.Body, contentLength, mimeType, filename, nil
}

func (t *telChannelEchoerImp) downloadTelegramFileToTemp(fileID, pattern string) (path string, size int64, cleanup func(), err error) {
	file, err := t.telBot.GetFile(tgbotapi.FileConfig{FileID: fileID})
	if err != nil {
		return "", 0, nil, err
	}
	if file.FilePath == "" {
		return "", 0, nil, fmt.Errorf("getFile returned empty file_path")
	}

	f, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", 0, nil, err
	}
	tmpPath := f.Name()
	cleanup = func() { _ = os.Remove(tmpPath) }

	req, err := http.NewRequest(http.MethodGet, file.Link(t.telBot.Token), nil)
	if err != nil {
		_ = f.Close()
		cleanup()
		return "", 0, nil, err
	}
	resp, err := t.telBot.Client.Do(req)
	if err != nil {
		_ = f.Close()
		cleanup()
		return "", 0, nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		resp.Body.Close()
		_ = f.Close()
		cleanup()
		return "", 0, nil, fmt.Errorf("telegram file GET: %s", resp.Status)
	}
	n, copyErr := io.Copy(f, resp.Body)
	resp.Body.Close()
	closeErr := f.Close()
	if copyErr != nil {
		cleanup()
		return "", 0, nil, copyErr
	}
	if closeErr != nil {
		cleanup()
		return "", 0, nil, closeErr
	}
	return tmpPath, n, cleanup, nil
}
