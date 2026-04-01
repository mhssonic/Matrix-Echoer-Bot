package tel_client_echoer

import (
	"context"
	"fmt"
	"net/url"
	"news_bot/lib"
	"news_bot/matrix_bot"
	"news_bot/system"
	"os"
	"slices"
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
}

func NewService(tellChannelId []int64, matrixSender matrix_bot.RoomAutoSender, reader matrix_bot.CodeReader, config Config) system.Echoer {
	return &tellClientEchoer{
		tellChannelIds: tellChannelId,
		matrixSender:   matrixSender,
		matrixReader:   reader,
		config:         config,
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

	// Message handler
	d.OnNewChannelMessage(func(ctx context.Context, e tg.Entities, update *tg.UpdateNewChannelMessage) error {
		msg, ok := update.Message.(*tg.Message)
		if !ok || msg.Message == "" {
			return nil
		}

		if peerCh, ok := msg.PeerID.(*tg.PeerChannel); ok {
			if len(t.tellChannelIds) == 0 || slices.Contains(t.tellChannelIds, peerCh.ChannelID) {
				t.matrixSender.SendTextAsync(msg.Message)
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
