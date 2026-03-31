package matrix_bot

import (
	"context"
	"errors"
	"sync"

	"github.com/gotd/td/tg"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

func NewCodeReader(client *mautrix.Client, roomID string) CodeReader {
	return &codeReaderImp{
		roomId: id.RoomID(roomID),
		client: client,
	}
}

type codeReaderImp struct {
	roomId id.RoomID
	client *mautrix.Client
}

type CodeReader interface {
	ReadCode(ctx context.Context, sentCode *tg.AuthSentCode) (string, error)
}

// ReadCode waits for the next message event in the given room
// after the "waiting" message we send. It ignores everything that was
// already in the timeline before this call.
func (r *codeReaderImp) ReadCode(ctx context.Context, sentCode *tg.AuthSentCode) (string, error) {
	done := make(chan string, 1)
	errChan := make(chan error, 1)

	syncer := r.client.Syncer.(*mautrix.DefaultSyncer)

	// Register DontProcessOldEvents once (ideally do this at client init, not every call)
	syncer.OnSync(r.client.DontProcessOldEvents)

	// Send the waiting marker first
	waitingEvt, err := r.client.SendText(ctx, r.roomId, "🔄 Waiting for verification code...")
	if err != nil {
		return "", err
	}

	// Create the handler
	handler := func(ctx context.Context, evt *event.Event) {
		if evt.RoomID != r.roomId {
			return
		}

		//Skip our own "waiting" message
		if evt.ID == waitingEvt.EventID {
			return
		}

		// Skip anything that looks too old
		if evt.Unsigned.Age > 60000 { // 60 seconds should be plenty
			return
		}

		msg := evt.Content.AsMessage()
		if msg == nil || msg.Body == "" {
			return
		}
		// TODO fix memory leaks of it
		done <- msg.Body
	}

	// Register handler AFTER sending the waiting message
	syncer.OnEventType(event.EventMessage, handler)
	//defer syncer.RemoveEventHandler(event.EventMessage, handler) // cleanup

	// Start sync in background
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		syncErr := r.client.SyncWithContext(ctx)
		if syncErr != nil && !errors.Is(syncErr, context.Canceled) && !errors.Is(syncErr, context.DeadlineExceeded) {
			errChan <- syncErr
		}
	}()

	// Wait for the code or error
	select {
	case code := <-done:
		return code, nil
	case err := <-errChan:
		return "", err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}
