package configures

import (
	"echoer_bot/tel_client_echoer"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// MatrixDestination is one Matrix account + target room (may be on a different server than others).
type MatrixDestination struct {
	Homeserver        string
	UserID            string
	MatrixAccessToken string
	RoomID            string
}

// Config holds all application configuration
type Config struct {
	MatrixDestinations []MatrixDestination

	// Telegram bot configurations
	BotToken                string
	ChannelBotChatId        int64
	TelClientChannelChatIds []int64
	DisableVideos           bool

	// System configurations
	ProxyURL string

	// Telegram client (echoer) configurations
	TelClientConfig  tel_client_echoer.Config
	CodeReaderRoomId string
}

// LoadConfig loads all configuration from environment variables
func LoadConfig() (Config, error) {
	cfg := Config{}

	if err := cfg.loadMatrixDestinations(); err != nil {
		return cfg, err
	}

	cfg.CodeReaderRoomId = getEnv("MATRIX_CODE_READER_ROOM_ID")

	// Telegram bot configurations
	cfg.BotToken = getEnv("TELEGRAM_BOT_TOKEN")

	if chatIDStr := getEnv("TELEGRAM_BOT_CHANNEL_CHAT_ID"); chatIDStr != "" {
		if id, err := strconv.ParseInt(chatIDStr, 10, 64); err == nil {
			cfg.ChannelBotChatId = id
		} else {
			return cfg, fmt.Errorf("invalid TELEGRAM_BOT_CHANNEL_CHAT_ID: %w", err)
		}
	}

	if chatIDsStr := getEnv("TELEGRAM_CLIENT_CHANNEL_CHAT_IDS"); chatIDsStr != "" {
		ids := strings.Split(chatIDsStr, ",")
		for _, idStr := range ids {
			idStr = strings.TrimSpace(idStr)
			if idStr == "" {
				continue
			}
			if id, err := strconv.ParseInt(idStr, 10, 64); err == nil {
				cfg.TelClientChannelChatIds = append(cfg.TelClientChannelChatIds, id)
			} else {
				return cfg, fmt.Errorf("invalid chat ID in TELEGRAM_CLIENT_CHANNEL_CHAT_IDS: %s", idStr)
			}
		}
	}

	// If only single ID was provided, make sure the slice is populated
	if len(cfg.TelClientChannelChatIds) == 0 && cfg.ChannelBotChatId != 0 {
		cfg.TelClientChannelChatIds = []int64{cfg.ChannelBotChatId}
	}

	// System configurations
	cfg.ProxyURL = getEnv("PROXY_URL")

	// Telegram client configurations
	cfg.TelClientConfig = tel_client_echoer.Config{
		ApiCode:     0,
		ApiHash:     getEnv("TELEGRAM_API_HASH"),
		PhoneNumber: getEnv("TELEGRAM_PHONE_NUMBER"),
		Password:    getEnv("TELEGRAM_PASSWORD"),
		Proxy:       getEnv("PROXY_URL"),
	}

	if v := strings.ToLower(getEnv("DISABLE_VIDEOS")); v != "" {
		cfg.DisableVideos = v == "1" || v == "true" || v == "yes" || v == "on"
	}

	// Optional: TELEGRAM_API_ID
	if apiCodeStr := getEnv("TELEGRAM_API_ID"); apiCodeStr != "" {
		if code, err := strconv.Atoi(apiCodeStr); err == nil {
			cfg.TelClientConfig.ApiCode = code
		} else {
			return cfg, fmt.Errorf("invalid TELEGRAM_API_ID: %w", err)
		}
	}

	// Validate required fields
	if err := cfg.validate(); err != nil {
		return cfg, err
	}

	return cfg, nil
}

func (c *Config) loadMatrixDestinations() error {
	multiHS := splitCommaList(getEnv("MATRIX_HOMESERVERS"))
	multiUID := splitCommaList(getEnv("MATRIX_USER_IDS"))
	multiTok := splitCommaList(getEnv("MATRIX_ACCESS_TOKENS"))
	multiRoom := splitCommaList(getEnv("MATRIX_ROOM_IDS"))

	useMulti := len(multiHS) > 0 || len(multiUID) > 0 || len(multiTok) > 0 || len(multiRoom) > 0
	if useMulti {
		n := len(multiHS)
		if n == 0 || n != len(multiUID) || n != len(multiTok) || n != len(multiRoom) {
			return fmt.Errorf("MATRIX_HOMESERVERS, MATRIX_USER_IDS, MATRIX_ACCESS_TOKENS, MATRIX_ROOM_IDS must all be non-empty comma-separated lists of the same length")
		}
		for i := 0; i < n; i++ {
			c.MatrixDestinations = append(c.MatrixDestinations, MatrixDestination{
				Homeserver:        multiHS[i],
				UserID:            multiUID[i],
				MatrixAccessToken: multiTok[i],
				RoomID:            multiRoom[i],
			})
		}
		return nil
	}

	hs := getEnv("MATRIX_HOMESERVER")
	uid := getEnv("MATRIX_USER_ID")
	tok := getEnv("MATRIX_ACCESS_TOKEN")
	room := getEnv("MATRIX_ROOM_ID")
	if hs == "" || uid == "" || tok == "" || room == "" {
		return fmt.Errorf("set either legacy MATRIX_HOMESERVER, MATRIX_USER_ID, MATRIX_ACCESS_TOKEN, MATRIX_ROOM_ID or multi MATRIX_HOMESERVERS, MATRIX_USER_IDS, MATRIX_ACCESS_TOKENS, MATRIX_ROOM_IDS")
	}
	c.MatrixDestinations = []MatrixDestination{{
		Homeserver:        hs,
		UserID:            uid,
		MatrixAccessToken: tok,
		RoomID:            room,
	}}
	return nil
}

func splitCommaList(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// getEnv reads an environment variable. Returns empty string if not set.
func getEnv(key string) string {
	return strings.TrimSpace(os.Getenv(key))
}

// validate checks that all required fields are present
func (c *Config) validate() error {
	if len(c.MatrixDestinations) == 0 {
		return fmt.Errorf("no Matrix destinations configured")
	}

	botEnabled := c.BotToken != ""
	clientEnabled := c.TelClientConfig.ApiCode != 0 && c.TelClientConfig.ApiHash != "" && c.TelClientConfig.PhoneNumber != ""

	if !botEnabled && !clientEnabled {
		return fmt.Errorf("no sources enabled: provide TELEGRAM_BOT_TOKEN and/or TELEGRAM_API_ID + TELEGRAM_API_HASH + TELEGRAM_PHONE_NUMBER")
	}

	if botEnabled && c.ChannelBotChatId == 0 {
		return fmt.Errorf("TELEGRAM_BOT_CHANNEL_CHAT_ID is required when TELEGRAM_BOT_TOKEN is set")
	}

	if clientEnabled {
		if c.CodeReaderRoomId == "" {
			return fmt.Errorf("MATRIX_CODE_READER_ROOM_ID is required when Telegram client is enabled")
		}
		if len(c.TelClientChannelChatIds) == 0 {
			return fmt.Errorf("TELEGRAM_CLIENT_CHANNEL_CHAT_IDS is required when Telegram client is enabled")
		}
	}

	return nil
}
