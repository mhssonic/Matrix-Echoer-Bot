package configures

import (
	"echoer_bot/tel_client_echoer"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds all application configuration
type Config struct {
	// Matrix configurations
	Homeserver        string
	UserID            string
	MatrixAccessToken string
	RoomID            string

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

	// Matrix configurations
	cfg.Homeserver = getEnv("MATRIX_HOMESERVER")
	cfg.UserID = getEnv("MATRIX_USER_ID")
	cfg.MatrixAccessToken = getEnv("MATRIX_ACCESS_TOKEN")
	cfg.RoomID = getEnv("MATRIX_ROOM_ID")

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

// getEnv reads an environment variable. Returns empty string if not set.
func getEnv(key string) string {
	return strings.TrimSpace(os.Getenv(key))
}

// validate checks that all required fields are present
func (c *Config) validate() error {
	if c.Homeserver == "" {
		return fmt.Errorf("MATRIX_HOMESERVER environment variable is required")
	}
	if c.UserID == "" {
		return fmt.Errorf("MATRIX_USER_ID environment variable is required")
	}
	if c.MatrixAccessToken == "" {
		return fmt.Errorf("MATRIX_ACCESS_TOKEN environment variable is required")
	}
	if c.RoomID == "" {
		return fmt.Errorf("MATRIX_ROOM_ID environment variable is required")
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
