package tel_client_echoer

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/url"
	"news_bot/lib"
	"news_bot/lib/videokit"
	"news_bot/matrix_bot"
	"news_bot/system"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/telegram/dcs"
	"github.com/gotd/td/telegram/updates"
	"github.com/gotd/td/tg"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/net/proxy"
)

type Config struct {
	ApiCode     int
	ApiHash     string
	PhoneNumber string
	Password    string
	Proxy       string
}

type tellClientEchoer struct {
	tellChannelIds []int64
	matrixSender   matrix_bot.RoomAutoSender
	matrixReader   matrix_bot.CodeReader
	config         Config
	disableVideos  bool

	mu            sync.Mutex
	tgClient      *telegram.Client
	albumBuffer   map[int64][]*tg.Message
	albumDeadline map[int64]*time.Timer
}

func NewService(tellChannelId []int64, matrixSender matrix_bot.RoomAutoSender, reader matrix_bot.CodeReader, config Config, disableVideos bool) system.Echoer {
	return &tellClientEchoer{
		tellChannelIds: tellChannelId,
		matrixSender:   matrixSender,
		matrixReader:   reader,
		config:         config,
		disableVideos:  disableVideos,
		albumBuffer:    map[int64][]*tg.Message{},
		albumDeadline:  map[int64]*time.Timer{},
	}
}
func (t *tellClientEchoer) Start() {
	log, _ := zap.NewDevelopment(zap.IncreaseLevel(zapcore.InfoLevel))
	defer log.Sync()

	var resolver dcs.Resolver

	if t.config.Proxy != "" {
		log.Info("🔀 Using proxy for Telegram client", zap.String("url", t.config.Proxy))

		parsed, err := url.Parse(t.config.Proxy)
		if err != nil {
			log.Fatal("invalid proxy url", zap.Error(err))
		}

		if parsed.Scheme != "socks5" && parsed.Scheme != "socks5h" {
			log.Fatal("only socks5/socks5h supported", zap.String("scheme", parsed.Scheme))
		}

		var proxyAuth *proxy.Auth
		if parsed.User != nil {
			pw, _ := parsed.User.Password()
			proxyAuth = &proxy.Auth{
				User:     parsed.User.Username(),
				Password: pw,
			}
		}

		dialer, err := proxy.SOCKS5("tcp", parsed.Host, proxyAuth, proxy.Direct)
		if err != nil {
			log.Fatal("failed to create SOCKS5 dialer", zap.Error(err))
		}

		contextDialer := lib.NewContextDialerAdapter(dialer) // your existing adapter

		resolver = dcs.Plain(dcs.PlainOptions{
			Dial: contextDialer.DialContext,
		})
	}

	d := tg.NewUpdateDispatcher()
	gaps := updates.New(updates.Config{
		Handler: d,
		Logger:  log.Named("gaps"),
	})

	dataDir := "./data"
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		log.Fatal("Failed to create data directory", zap.Error(err))
	}

	sessionStorage := &session.FileStorage{
		Path: "./data/telegram_session.json",
	}

	client := telegram.NewClient(t.config.ApiCode, t.config.ApiHash, telegram.Options{
		Logger:         log,
		UpdateHandler:  gaps,
		Resolver:       resolver,
		SessionStorage: sessionStorage,
	})
	t.tgClient = client

	// Message handler
	d.OnNewChannelMessage(func(ctx context.Context, e tg.Entities, update *tg.UpdateNewChannelMessage) error {
		msg, ok := update.Message.(*tg.Message)
		if !ok {
			return nil
		}

		if peerCh, ok := msg.PeerID.(*tg.PeerChannel); ok {
			if len(t.tellChannelIds) == 0 || slices.Contains(t.tellChannelIds, peerCh.ChannelID) {
				if msg.GroupedID != 0 && msg.Media != nil {
					t.enqueueAlbum(msg.GroupedID, msg)
					return nil
				}
				return t.processMessage(ctx, client, msg)
			}
		}
		return nil
	})

	authenticator := auth.Constant(t.config.PhoneNumber, t.config.Password, auth.CodeAuthenticatorFunc(t.matrixReader.ReadCode))

	ctx := context.Background()

	for {
		err := client.Run(ctx, func(ctx context.Context) error {
			// Longer timeout for auth + self
			connectCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
			defer cancel()

			if err := client.Auth().IfNecessary(connectCtx, auth.NewFlow(authenticator, auth.SendCodeOptions{})); err != nil {
				return fmt.Errorf("auth failed: %w", err)
			}

			user, err := client.Self(ctx)
			if err != nil {
				return fmt.Errorf("self failed: %w", err)
			}

			fmt.Printf("✅ Logged in as @%s (ID: %d)\n", user.Username, user.ID)
			fmt.Println("👂 Listening for new messages... (Ctrl+C to stop)")

			// Start updates with gaps recovery
			return gaps.Run(ctx, client.API(), user.ID, updates.AuthOptions{
				OnStart: func(ctx context.Context) {
					log.Info("🚀 Updates engine started")
				},
			})
		})

		if err != nil {
			log.Warn("Client run failed, restarting in 5s...", zap.Error(err))
			// Optional: exponential backoff
			time.Sleep(5 * time.Second)
			continue
		}
		break // successful run (only exits on Ctrl+C)
	}
}

func (t *tellClientEchoer) enqueueAlbum(groupID int64, msg *tg.Message) {
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

func (t *tellClientEchoer) flushAlbum(groupID int64) {
	t.mu.Lock()
	msgs := t.albumBuffer[groupID]
	timer := t.albumDeadline[groupID]
	delete(t.albumBuffer, groupID)
	delete(t.albumDeadline, groupID)
	client := t.tgClient
	t.mu.Unlock()

	if timer != nil {
		timer.Stop()
	}
	if len(msgs) == 0 {
		return
	}

	sort.Slice(msgs, func(i, j int) bool { return msgs[i].ID < msgs[j].ID })

	albumCaption := ""
	for _, m := range msgs {
		if strings.TrimSpace(m.Message) != "" {
			albumCaption = m.Message
			break
		}
	}

	captionOnID := 0
	for _, m := range msgs {
		if _, ok := m.Media.(*tg.MessageMediaPhoto); ok {
			captionOnID = m.ID
		}
	}
	if captionOnID == 0 {
		for _, m := range msgs {
			if isAlbumVideoMessage(m) {
				captionOnID = m.ID
			}
		}
	}

	for _, m := range msgs {
		cap := ""
		if m.ID == captionOnID {
			cap = albumCaption
		}
		mCopy := *m
		mCopy.Message = cap
		_ = t.processMessage(context.Background(), client, &mCopy)
	}
}

func (t *tellClientEchoer) processMessage(ctx context.Context, client *telegram.Client, msg *tg.Message) error {
	if msg.Message != "" && msg.Media == nil {
		t.matrixSender.SendTextAsync(msg.Message)
		return nil
	}

	caption := msg.Message
	switch media := msg.Media.(type) {
	case *tg.MessageMediaPhoto:
		if client == nil || media.Photo == nil {
			return nil
		}
		photo, ok := media.Photo.(*tg.Photo)
		if !ok {
			return nil
		}
		sizeType, w, h, fileSize := pickLargestPhotoSizeMeta(photo.Sizes)
		if int64(fileSize) > videokit.MaxMatrixUploadBytes {
			if strings.TrimSpace(caption) != "" {
				_ = t.matrixSender.SendTextSync(ctx, caption)
			}
			return nil
		}
		loc := &tg.InputPhotoFileLocation{
			ID:            photo.ID,
			AccessHash:    photo.AccessHash,
			FileReference: photo.FileReference,
			ThumbSize:     sizeType,
		}
		pr, pw := io.Pipe()
		go func() {
			_, err := client.Download(loc).Stream(ctx, pw)
			_ = pw.CloseWithError(err)
		}()

		return t.matrixSender.SendImageReaderSync(ctx, pr, int64(fileSize), "image/jpeg", fmt.Sprintf("photo_%d.jpg", msg.ID), caption, w, h)
	case *tg.MessageMediaDocument:
		if client == nil || media.Document == nil {
			return nil
		}
		doc, ok := media.Document.(*tg.Document)
		if !ok {
			return nil
		}
		if t.disableVideos && isVideoDocument(doc) {
			return nil
		}
		if isVideoDocument(doc) {
			return t.handleTelegramClientVideo(ctx, client, doc, caption)
		}
		// for now: ignore non-video documents
		return nil
	default:
		// unsupported
	}

	return nil
}

func isAlbumVideoMessage(msg *tg.Message) bool {
	docMedia, ok := msg.Media.(*tg.MessageMediaDocument)
	if !ok || docMedia.Document == nil {
		return false
	}
	doc, ok := docMedia.Document.(*tg.Document)
	if !ok {
		return false
	}
	return isVideoDocument(doc)
}

func (t *tellClientEchoer) handleTelegramClientVideo(ctx context.Context, client *telegram.Client, doc *tg.Document, caption string) error {
	videoW, videoH, durationMS := documentVideoMeta(doc)

	loc := &tg.InputDocumentFileLocation{
		ID:            doc.ID,
		AccessHash:    doc.AccessHash,
		FileReference: doc.FileReference,
		ThumbSize:     "",
	}

	f, err := os.CreateTemp("", "tgcli-vid-*")
	if err != nil {
		return err
	}
	tmpName := f.Name()
	_ = f.Close()
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := client.Download(loc).ToPath(ctx, tmpName); err != nil {
		return err
	}

	uploadPath, uploadSz, cleanupOut, ok, err := videokit.PrepareForMatrix(ctx, tmpName)
	if err != nil {
		if strings.TrimSpace(caption) != "" {
			_ = t.matrixSender.SendTextSync(ctx, caption)
		}
		return nil
	}
	if !ok {
		if strings.TrimSpace(caption) != "" {
			_ = t.matrixSender.SendTextSync(ctx, caption)
		}
		return nil
	}
	defer cleanupOut()

	if uploadSz > videokit.MaxMatrixUploadBytes {
		if strings.TrimSpace(caption) != "" {
			_ = t.matrixSender.SendTextSync(ctx, caption)
		}
		return nil
	}

	rf, err := os.Open(uploadPath)
	if err != nil {
		return err
	}
	defer rf.Close()

	filename := documentFilename(doc)
	if filepath.Ext(filename) == "" {
		filename += ".mp4"
	}
	outName := filename
	if filepath.Ext(uploadPath) == ".mp4" {
		base := strings.TrimSuffix(filename, filepath.Ext(filename))
		if base == "" {
			base = "video"
		}
		outName = base + ".mp4"
	}
	mimeType := doc.MimeType
	if filepath.Ext(uploadPath) == ".mp4" {
		mimeType = "video/mp4"
	}
	if mimeType == "" {
		mimeType = "video/mp4"
	}
	return t.matrixSender.SendVideoReaderSync(ctx, rf, uploadSz, mimeType, outName, caption, videoW, videoH, durationMS)
}

// documentVideoMeta reads width, height, and duration for Matrix (duration in milliseconds).
func documentVideoMeta(doc *tg.Document) (width, height, durationMS int) {
	for _, attr := range doc.Attributes {
		if v, ok := attr.(*tg.DocumentAttributeVideo); ok {
			return v.W, v.H, int(math.Round(v.Duration * 1000))
		}
	}
	return 0, 0, 0
}

func pickLargestPhotoSizeMeta(sizes []tg.PhotoSizeClass) (typ string, w int, h int, size int) {
	bestType := "w"
	bestArea := 0
	bestW, bestH, bestSize := 0, 0, 0
	for _, s := range sizes {
		ps, ok := s.(*tg.PhotoSize)
		if !ok {
			continue
		}
		area := ps.W * ps.H
		if area > bestArea {
			bestArea = area
			bestType = ps.Type
			bestW, bestH, bestSize = ps.W, ps.H, ps.Size
		}
	}
	return bestType, bestW, bestH, bestSize
}

func isVideoDocument(doc *tg.Document) bool {
	for _, a := range doc.Attributes {
		if _, ok := a.(*tg.DocumentAttributeVideo); ok {
			return true
		}
	}
	return false
}

func documentFilename(doc *tg.Document) string {
	for _, a := range doc.Attributes {
		if f, ok := a.(*tg.DocumentAttributeFilename); ok && f.FileName != "" {
			return filepath.Base(f.FileName)
		}
	}
	return fmt.Sprintf("file_%d", doc.ID)
}
