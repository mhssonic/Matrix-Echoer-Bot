# Echoer_Bot

`Echoer_Bot` is a small Go service that **listens to Telegram sources** (a **Bot API** channel listener and/or a **user (MTProto) client**) and **echoes** new posts into a **Matrix** room as text and media messages.

Matrix sending is implemented with [mautrix](https://github.com/mautrix/go). Telegram uses [go-telegram-bot-api](https://github.com/go-telegram-bot-api/telegram-bot-api) for the bot and [gotd](https://github.com/gotd/td) for the user client.

## Features

- **Matrix → one room**: all echoed content goes to `MATRIX_ROOM_ID`.
- **Optional Telegram sources** (enable one or both):
  - **Bot**: set `TELEGRAM_BOT_TOKEN` (+ channel chat id). If the token is empty, the bot is not started.
  - **User client**: set `TELEGRAM_API_ID`, `TELEGRAM_API_HASH`, and `TELEGRAM_PHONE_NUMBER`. If those are missing, the client is not started.
- **Text, photos, and videos** from channel posts / channel messages (bot: `channel_post`; client: new channel messages).
- **Albums / grouped media**: Matrix has no native “album”; items are sent **in order** (sorted by Telegram message id). The **album caption** is attached to the **last photo** in the group (or the **last video** if there are no photos).
- **Videos**:
  - Optional **`DISABLE_VIDEOS`**: skip all video forwarding.
  - If a video is **larger than 5 MB**, the echoer tries **ffmpeg** to re-encode to a smaller H.264/AAC MP4 before upload.
  - If compression fails or the result is **still above 5 MB**, the **video is skipped** and **only the caption** is sent as a plain text message (when present).
- **30 MB cap** (enforced in the **echoer** layer only): if the **payload that would be uploaded to Matrix** is larger than **30 MB**, the echoer **does not upload** the file and sends **only the caption** as text (when present). The Matrix sender package does not enforce this limit.
- **Video metadata** on Matrix `m.video` events: **width**, **height**, and **duration** (milliseconds in `info`, derived from Telegram).
- **Telegram user login**: login codes can be read from a dedicated Matrix room via `MATRIX_CODE_READER_ROOM_ID` (required when the user client is enabled).
- **Proxy**: `PROXY_URL` is supported for the **bot** (HTTP/SOCKS5) and for the **user client** (**SOCKS5 / SOCKS5h only**).

## Requirements

- **Go** (see `go.mod`; currently **1.25**).
- **ffmpeg** on `PATH` (or set **`FFMPEG_PATH`**) if you want automatic shrinking of videos over **5 MB**. Without ffmpeg, oversized videos may fall back to **caption-only** behavior when compression is required.
- A **Matrix account** (often a bot) with an access token and membership in the target room.
- For the **user client**: Telegram **API ID** and **API hash** from [my.telegram.org](https://my.telegram.org).

The app reads **environment variables** only; it does **not** load a `.env` file by itself. Copy `.env.example` to `.env` and either export those variables in your shell or use a process manager / launcher that injects them.

## Configuration

| Variable | Required | Description |
|----------|----------|-------------|
| `MATRIX_HOMESERVER` | Yes | Homeserver base URL (e.g. `https://matrix.org`). |
| `MATRIX_USER_ID` | Yes | Matrix user id (e.g. `@bot:example.org`). |
| `MATRIX_ACCESS_TOKEN` | Yes | Access token for that user. |
| `MATRIX_ROOM_ID` | Yes | Room id where messages are sent. |
| `TELEGRAM_BOT_TOKEN` | No* | Bot token from BotFather. If unset, the bot source is disabled. |
| `TELEGRAM_BOT_CHANNEL_CHAT_ID` | With bot | Numeric channel id the bot listens to (`channel_post`). |
| `TELEGRAM_CLIENT_CHANNEL_CHAT_IDS` | With client | Comma-separated channel ids for the MTProto client. |
| `TELEGRAM_API_ID` | With client | Telegram API id (integer). |
| `TELEGRAM_API_HASH` | With client | Telegram API hash. |
| `TELEGRAM_PHONE_NUMBER` | With client | Phone number for login. |
| `TELEGRAM_PASSWORD` | No | 2FA password if enabled. |
| `MATRIX_CODE_READER_ROOM_ID` | With client | Room where the bot waits for the Telegram login code. |
| `PROXY_URL` | No | Proxy for Telegram (see above). |
| `DISABLE_VIDEOS` | No | If `true` / `1` / `yes` / `on`, videos are not forwarded. |
| `FFMPEG_PATH` | No | Full path to `ffmpeg` if it is not on `PATH`. |

\* At least one of **bot** or **user client** must be configured, or the app will refuse to start.

See `.env.example` for a filled-out template.

## Run

```bash
# From the repository root, with environment variables set:
go run .
```

Or build a binary:

```bash
go build -o echoer_bot .
./echoer_bot
```

On first run, the **user client** stores session data under `./data/telegram_session.json` (directory is created if needed).

## Project layout

| Path | Role |
|------|------|
| `main.go` | Wires config, Matrix client, and echoer goroutines. |
| `configures/` | Environment-based configuration and validation. |
| `echoer/` | Telegram **bot** channel listener → Matrix. |
| `tel_client_echoer/` | Telegram **user client** → Matrix. |
| `tel_bot/` | Bot API client factory (optional HTTP/SOCKS proxy). |
| `matrix_bot/` | Room sender (`m.room.message` text / image / video) and code reader for login. |
| `lib/videokit/` | ffmpeg compression and shared size constants (5 MB / 30 MB thresholds). |

## Behavior notes

- **Bot** downloads files via **`getFile`** then HTTP GET using the bot’s client (respects `PROXY_URL` when configured in `tel_bot`).
- **Images** are uploaded to the Matrix content repository and sent as `m.image` with dimensions when known.
- **Videos** are uploaded as `m.video` with `info` **w**, **h**, **duration** (ms), and **size** when known.
- Constants **`MaxMatrixVideoBytes`** (5 MB) and **`MaxMatrixUploadBytes`** (30 MB) live in `lib/videokit/videokit.go` if you need to tune them.

## License

If you add a license file, describe it here. Until then, all rights are reserved by the project author unless stated otherwise.
