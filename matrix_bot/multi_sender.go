package matrix_bot

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
)

// MultiRoomAutoSender fans out every RoomAutoSender call to multiple underlying senders
// (e.g. different Matrix homeservers, users, tokens, and rooms).
type MultiRoomAutoSender struct {
	senders []RoomAutoSender
}

// NewMultiRoomAutoSender returns a fan-out sender. senders must be non-empty.
func NewMultiRoomAutoSender(senders []RoomAutoSender) (RoomAutoSender, error) {
	if len(senders) == 0 {
		return nil, fmt.Errorf("NewMultiRoomAutoSender: need at least one RoomAutoSender")
	}
	out := make([]RoomAutoSender, len(senders))
	copy(out, senders)
	return &MultiRoomAutoSender{senders: out}, nil
}

func (m *MultiRoomAutoSender) SendTextSync(ctx context.Context, text string) error {
	var errs []error
	for _, s := range m.senders {
		if err := s.SendTextSync(ctx, text); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (m *MultiRoomAutoSender) SendImageBytesWithHTMLCaptionSync(ctx context.Context, data []byte, mimeType, filename, rawCaption string) error {
	var errs []error
	for _, s := range m.senders {
		if err := s.SendImageBytesWithHTMLCaptionSync(ctx, data, mimeType, filename, rawCaption); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (m *MultiRoomAutoSender) SendImageReaderSync(ctx context.Context, content io.Reader, contentLength int64, mimeType, filename, rawCaption string, width, height int) error {
	body, err := io.ReadAll(content)
	if err != nil {
		return fmt.Errorf("read image for multi-room upload: %w", err)
	}
	var errs []error
	for _, s := range m.senders {
		if err := s.SendImageReaderSync(ctx, bytes.NewReader(body), int64(len(body)), mimeType, filename, rawCaption, width, height); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (m *MultiRoomAutoSender) SendVideoReaderSync(ctx context.Context, video io.Reader, videoLength int64, mimeType, filename, rawCaption string, width, height, durationMS int) error {
	body, err := io.ReadAll(video)
	if err != nil {
		return fmt.Errorf("read video for multi-room upload: %w", err)
	}
	var errs []error
	for _, s := range m.senders {
		if err := s.SendVideoReaderSync(ctx, bytes.NewReader(body), int64(len(body)), mimeType, filename, rawCaption, width, height, durationMS); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (m *MultiRoomAutoSender) SendImageBytesWithHTMLCaptionAsync(data []byte, mimeType, filename, rawCaption string) {
	for _, s := range m.senders {
		s.SendImageBytesWithHTMLCaptionAsync(data, mimeType, filename, rawCaption)
	}
}

func (m *MultiRoomAutoSender) SimpleSendImageSync(ctx context.Context, filePath string, caption string) error {
	var errs []error
	for _, s := range m.senders {
		if err := s.SimpleSendImageSync(ctx, filePath, caption); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (m *MultiRoomAutoSender) SendImageWithHTMLCaptionSync(ctx context.Context, filePath, rawCaption string) error {
	var errs []error
	for _, s := range m.senders {
		if err := s.SendImageWithHTMLCaptionSync(ctx, filePath, rawCaption); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (m *MultiRoomAutoSender) SendImageWithHTMLCaptionAsync(filePath, rawCaption string) {
	for _, s := range m.senders {
		s.SendImageWithHTMLCaptionAsync(filePath, rawCaption)
	}
}

func (m *MultiRoomAutoSender) SendTextAsync(text string) {
	for _, s := range m.senders {
		s.SendTextAsync(text)
	}
}

func (m *MultiRoomAutoSender) Start(workerCount int) {
	for _, s := range m.senders {
		s.Start(workerCount)
	}
}

func (m *MultiRoomAutoSender) Stop() {
	for _, s := range m.senders {
		s.Stop()
	}
}
