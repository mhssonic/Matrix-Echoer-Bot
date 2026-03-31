package matrix_bot

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"log"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

type RoomAutoSender interface {
	SendTextSync(ctx context.Context, text string) error
	SimpleSendImageSync(ctx context.Context, filePath string, caption string) error
	SendImageWithHTMLCaptionSync(ctx context.Context, filePath, rawCaption string) error
	SendImageWithHTMLCaptionAsync(filePath, rawCaption string)
	SendTextAsync(text string)
	Start(workerCount int)
	Stop()
}

func NewRoomAutoSender(client *mautrix.Client, roomID string) RoomAutoSender {
	return &roomAutoSenderImp{
		messageChannel: make(chan *messageStruct, 1024),
		roomId:         id.RoomID(roomID),
		client:         client,
	}
}

type roomAutoSenderImp struct {
	messageChannel chan *messageStruct
	roomId         id.RoomID
	client         *mautrix.Client
	//throwOldOutMessage bool
}

type messageStruct struct {
	filePath   string
	rawCaption string
}

// SendTextSync Sends a simple text message to the room.
func (r *roomAutoSenderImp) SendTextSync(ctx context.Context, text string) error {
	// The library provides a convenient helper
	_, err := r.client.SendText(ctx, r.roomId, text)

	return err
}

// SimpleSendImageSync uploads a photo and sends it as an m.image message.
func (r *roomAutoSenderImp) SimpleSendImageSync(ctx context.Context, filePath string, caption string) error {
	// 1. Read the file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	// 2. Detect MIME type (or default to image/jpeg)
	ext := filepath.Ext(filePath)
	mimeType := mime.TypeByExtension(ext)
	if mimeType == "" {
		mimeType = "image/jpeg"
	}

	// 3. Upload to Matrix media repository
	// UploadBytes returns an MXC URI (mxc://...)
	mxc, err := r.client.UploadBytes(ctx, data, mimeType)
	if err != nil {
		return fmt.Errorf("upload media: %w", err)
	}

	// 4. Build the image message content
	content := event.MessageEventContent{
		MsgType:  event.MsgImage,
		Body:     filepath.Base(filePath), // filename shown in clients
		FileName: filepath.Base(filePath),
		URL:      mxc.ContentURI.CUString(), // the mxc:// URI
		Info: &event.FileInfo{
			MimeType: mimeType,
			Size:     len(data),
		},
	}

	if caption != "" {
		content.Body = caption // This becomes the main caption
		content.FormattedBody = caption
		content.Format = "org.matrix.custom.html" // Optional but recommended
	}

	// 5. Send the message event
	_, err = r.client.SendMessageEvent(ctx, r.roomId, event.EventMessage, &content)
	if err != nil {
		return fmt.Errorf("send image event: %w", err)
	}

	return nil
}

// SendImageWithHTMLCaptionSync sends an image exactly like the official client
func (r *roomAutoSenderImp) SendImageWithHTMLCaptionSync(ctx context.Context, filePath, rawCaption string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	// Detect MIME type
	ext := strings.ToLower(filepath.Ext(filePath))
	mimeType := mime.TypeByExtension(ext)
	if mimeType == "" {
		mimeType = "image/jpeg"
	}

	// === 1. Upload the main image ===
	mxc, err := r.client.UploadBytes(ctx, data, mimeType)
	if err != nil {
		return fmt.Errorf("upload image: %w", err)
	}

	// === 2. Get image dimensions ===
	img, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		// fallback if decode fails
		img = image.Config{Width: 800, Height: 600}
	}

	// === 3. Build rich content (exactly like client) ===
	content := &event.MessageEventContent{
		MsgType:  event.MsgImage,
		Body:     rawCaption, // fallback text
		FileName: filepath.Base(filePath),
		URL:      mxc.ContentURI.CUString(), // the mxc:// URI
		Format:   "org.matrix.custom.html",
		Info: &event.FileInfo{
			Width:    img.Width,
			Height:   img.Height,
			MimeType: mimeType,
			Size:     len(data),
			// Thumbnail can be added later if you want to generate one
		},
	}

	// === 4. Convert caption to proper HTML (with <p> and <br />) ===
	if rawCaption != "" {
		htmlBody := convertToMatrixHTML(rawCaption)
		content.FormattedBody = htmlBody
		content.Body = rawCaption // plain text version
	}

	// Optional: Add blurhash (you can generate it with a library if needed)
	// content.Info.Blurhash = "K27nLI~qMx4TV@IA00j?ay"  // example from your request

	// === 5. Send the message ===
	_, err = r.client.SendMessageEvent(ctx, r.roomId, event.EventMessage, content)
	if err != nil {
		return fmt.Errorf("send message: %w", err)
	}

	return nil
}

// SendImageWithHTMLCaptionAsync sends an image exactly like the official client (non-blocking) advised version
func (r *roomAutoSenderImp) SendImageWithHTMLCaptionAsync(filePath, rawCaption string) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	select {
	case r.messageChannel <- &messageStruct{filePath, rawCaption}:
	case <-ticker.C:
	}
}

func (r *roomAutoSenderImp) SendTextAsync(text string) {
	r.SendImageWithHTMLCaptionAsync("", text)
}

func (r *roomAutoSenderImp) Start(workerCount int) {
	//ctx, _ := context.WithCancel(context.Background())
	for i := 0; i < workerCount; i++ {
		go r.sender(context.Background())
	}
}

func (r *roomAutoSenderImp) Stop() {
	close(r.messageChannel)
}

func (r *roomAutoSenderImp) sender(ctx context.Context) {
	for message := range r.messageChannel {
		if message.filePath == "" {
			err := r.SendTextSync(ctx, message.rawCaption)
			if err != nil {
				log.Printf("Failed to send text: %v", err)
			}
		} else if message.rawCaption != "" {
			err := r.SendImageWithHTMLCaptionSync(ctx, message.filePath, message.rawCaption)
			if err != nil {
				log.Printf("Failed to send image: %v", err)
			}
		}
	}
}
