package tel_bot

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"golang.org/x/net/proxy"
)

const apiEndpoint = "https://api.telegram.org/bot%s/%s"

// NewTelegramBotWithProxy creates a Telegram bot that routes all traffic through a proxy
// Supported formats:
//   - HTTP proxy:   "http://127.0.0.1:8080"
//   - HTTP with auth: "http://user:pass@127.0.0.1:8080"
//   - SOCKS5:       "socks5://127.0.0.1:1080"
//   - SOCKS5 with auth: "socks5://user:pass@127.0.0.1:1080"
func NewTelegramBotWithProxy(token, proxyURL string) (*tgbotapi.BotAPI, error) {
	if proxyURL == "" {
		// No proxy - normal bot
		return tgbotapi.NewBotAPI(token)
	}

	// Parse the proxy URL
	proxyParsed, err := url.Parse(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("invalid proxy URL: %w", err)
	}

	// Create HTTP Transport with proxy
	transport := &http.Transport{}

	if proxyParsed.Scheme == "socks5" || proxyParsed.Scheme == "socks5h" {
		// SOCKS5 proxy (most common for Telegram)
		auth := proxy.Auth{}
		if proxyParsed.User != nil {
			auth.User = proxyParsed.User.Username()
			auth.Password, _ = proxyParsed.User.Password()
		}

		dialer, err := proxy.SOCKS5("tcp", proxyParsed.Host, &auth, proxy.Direct)
		if err != nil {
			return nil, fmt.Errorf("failed to create SOCKS5 dialer: %w", err)
		}

		transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.Dial(network, addr)
		}
	} else {
		// HTTP / HTTPS proxy
		transport.Proxy = http.ProxyURL(proxyParsed)
	}

	// Create custom HTTP client
	httpClient := &http.Client{
		Transport: transport,
		Timeout:   60 * time.Second,
	}

	// Create bot using custom client
	return tgbotapi.NewBotAPIWithClient(token, apiEndpoint, httpClient)
}
