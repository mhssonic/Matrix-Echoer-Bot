package matrix_bot

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"io"
	"log"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

type RoomAutoSender interface {
	SendTextSync(ctx context.Context, text string) error
	SendImageBytesWithHTMLCaptionSync(ctx context.Context, data []byte, mimeType, filename, rawCaption string) error
	SendImageReaderSync(ctx context.Context, content io.Reader, contentLength int64, mimeType, filename, rawCaption string, width, height int) error
	SendVideoReaderSync(ctx context.Context, content io.Reader, contentLength int64, mimeType, filename, rawCaption string, width, height, durationMS int) error
	SendImageBytesWithHTMLCaptionAsync(data []byte, mimeType, filename, rawCaption string)
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
	kind       event.MessageType
	data       []byte
	mimeType   string
	filename   string
	filePath   string
	rawCaption string
}

// SendTextSync Sends a simple text message to the room.
func (r *roomAutoSenderImp) SendTextSync(ctx context.Context, text string) error {
	// The library provides a convenient helper
	_, err := r.client.SendText(ctx, r.roomId, text)

	return err
}

func (r *roomAutoSenderImp) SendImageBytesWithHTMLCaptionSync(ctx context.Context, data []byte, mimeType, filename, rawCaption string) error {
	if mimeType == "" {
		mimeType = "image/jpeg"
	}
	if filename == "" {
		filename = "image"
	}

	mxc, err := r.client.UploadBytes(ctx, data, mimeType)
	if err != nil {
		return fmt.Errorf("upload image: %w", err)
	}

	img, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		img = image.Config{Width: 0, Height: 0}
	}

	content := &event.MessageEventContent{
		MsgType:  event.MsgImage,
		Body:     rawCaption,
		FileName: filename,
		URL:      mxc.ContentURI.CUString(),
		Format:   event.FormatHTML,
		Info: &event.FileInfo{
			Width:    img.Width,
			Height:   img.Height,
			MimeType: mimeType,
			Size:     len(data),
		},
	}

	if rawCaption != "" {
		content.FormattedBody = convertToMatrixHTML(rawCaption)
		content.Body = rawCaption
	} else {
		content.Body = filename
	}

	_, err = r.client.SendMessageEvent(ctx, r.roomId, event.EventMessage, content)
	if err != nil {
		return fmt.Errorf("send image message: %w", err)
	}
	return nil
}

func (r *roomAutoSenderImp) SendImageReaderSync(ctx context.Context, content io.Reader, contentLength int64, mimeType, filename, rawCaption string, width, height int) error {
	if mimeType == "" {
		mimeType = "image/jpeg"
	}
	if filename == "" {
		filename = "image"
	}

	//mxc, err := r.client.Upload(ctx, content, mimeType, contentLength)
	mxc, err := r.client.UploadMedia(ctx, mautrix.ReqUploadMedia{
		Content:       content,
		ContentLength: contentLength,
		ContentType:   mimeType,
	})
	if err != nil {
		return fmt.Errorf("upload image: %w", err)
	}

	contentEvt := &event.MessageEventContent{
		MsgType:  event.MsgImage,
		Body:     rawCaption,
		FileName: filename,
		URL:      mxc.ContentURI.CUString(),
		Format:   event.FormatHTML,
		Info: &event.FileInfo{
			Width:    width,
			Height:   height,
			MimeType: mimeType,
			Size:     int(contentLength),
		},
	}

	if rawCaption != "" {
		contentEvt.FormattedBody = convertToMatrixHTML(rawCaption)
		contentEvt.Body = rawCaption
	} else {
		contentEvt.Body = filename
	}

	_, err = r.client.SendMessageEvent(ctx, r.roomId, event.EventMessage, contentEvt)
	if err != nil {
		return fmt.Errorf("send image message: %w", err)
	}
	return nil
}

func (r *roomAutoSenderImp) SendVideoReaderSync(ctx context.Context, video io.Reader, videoLength int64, mimeType, filename, rawCaption string, width, height, durationMS int) error {
	if mimeType == "" {
		mimeType = "video/mp4"
	}
	if filename == "" {
		filename = "video"
	}

	mxc, err := r.client.Upload(ctx, video, mimeType, videoLength)
	if err != nil {
		return fmt.Errorf("upload video: %w", err)
	}

	content := &event.MessageEventContent{
		MsgType:  event.MsgVideo,
		Body:     rawCaption,
		FileName: filename,
		URL:      mxc.ContentURI.CUString(),
		Format:   event.FormatHTML,
		Info: &event.FileInfo{
			Width:    width,
			Height:   height,
			Duration: durationMS,
			MimeType: mimeType,
			Size:     int(videoLength),
		},
	}

	if rawCaption != "" {
		content.FormattedBody = convertToMatrixHTML(rawCaption)
		content.Body = rawCaption
	} else {
		content.Body = filename
	}

	_, err = r.client.SendMessageEvent(ctx, r.roomId, event.EventMessage, content)
	if err != nil {
		return fmt.Errorf("send video message: %w", err)
	}
	return nil
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

	return r.SendImageBytesWithHTMLCaptionSync(ctx, data, mimeType, filepath.Base(filePath), rawCaption)
}

// SendImageWithHTMLCaptionAsync queues work on the worker pool (blocks if the buffer is full).
func (r *roomAutoSenderImp) SendImageWithHTMLCaptionAsync(filePath, rawCaption string) {
	r.messageChannel <- &messageStruct{filePath: filePath, rawCaption: rawCaption}
}

func (r *roomAutoSenderImp) SendTextAsync(text string) {
	r.SendImageWithHTMLCaptionAsync("", text)
}

func (r *roomAutoSenderImp) SendImageBytesWithHTMLCaptionAsync(data []byte, mimeType, filename, rawCaption string) {
	cp := append([]byte(nil), data...)
	r.messageChannel <- &messageStruct{
		kind:       event.MsgImage,
		data:       cp,
		mimeType:   mimeType,
		filename:   filename,
		rawCaption: rawCaption,
	}
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
		if len(message.data) > 0 {
			switch message.kind {
			case event.MsgImage:
				err := r.SendImageBytesWithHTMLCaptionSync(ctx, message.data, message.mimeType, message.filename, message.rawCaption)
				if err != nil {
					log.Printf("Failed to send image: %v", err)
				}
			default:
				log.Printf("Unknown media kind: %s", message.kind)
			}
			continue
		}

		if message.filePath != "" {
			err := r.SendImageWithHTMLCaptionSync(ctx, message.filePath, message.rawCaption)
			if err != nil {
				log.Printf("Failed to send image: %v", err)
			}
			continue
		}

		err := r.SendTextSync(ctx, message.rawCaption)
		if err != nil {
			log.Printf("Failed to send text: %v", err)
		}
	}
}
