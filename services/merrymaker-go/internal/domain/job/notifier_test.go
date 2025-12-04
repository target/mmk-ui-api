package job

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubWaiter struct {
	calls chan model.JobType
	err   error
	sleep time.Duration
}

func (s *stubWaiter) WaitForNotification(ctx context.Context, jobType model.JobType) error {
	select {
	case s.calls <- jobType:
	default:
	}

	if s.sleep > 0 {
		timer := time.NewTimer(s.sleep)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}

	if s.err != nil {
		return s.err
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	return nil
}

func TestNewNotifierRequiresWaiter(t *testing.T) {
	notifier, err := NewNotifier(NotifierOptions{})
	require.ErrorIs(t, err, ErrWaiterRequired)
	assert.Nil(t, notifier)
}

func TestNotifier_SubscribeReceivesNotifications(t *testing.T) {
	waiter := &stubWaiter{
		calls: make(chan model.JobType, 4),
	}
	notifier, err := NewNotifier(NotifierOptions{
		Waiter: waiter,
	})
	require.NoError(t, err)

	unsub, ch := notifier.Subscribe(model.JobTypeRules)
	defer unsub()

	select {
	case <-waiter.calls:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected waiter to be invoked")
	}

	select {
	case <-ch:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected notification to be delivered")
	}
}

func TestNotifier_UnsubscribeClosesChannel(t *testing.T) {
	waiter := &stubWaiter{
		calls: make(chan model.JobType, 1),
	}
	notifier, err := NewNotifier(NotifierOptions{
		Waiter: waiter,
	})
	require.NoError(t, err)

	unsub, ch := notifier.Subscribe(model.JobTypeAlert)

	// Allow goroutine to start
	select {
	case <-waiter.calls:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected waiter to be invoked")
	}

	unsub()

	select {
	case _, ok := <-ch:
		assert.False(t, ok, "channel should be closed after unsubscribe")
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected channel to close after unsubscribe")
	}
}

func TestNotifier_StopAllClosesChannels(t *testing.T) {
	waiter := &stubWaiter{
		calls: make(chan model.JobType, 2),
		err:   errors.New("boom"),
	}
	notifier, err := NewNotifier(NotifierOptions{
		Waiter: waiter,
	})
	require.NoError(t, err)

	unsubRules, chRules := notifier.Subscribe(model.JobTypeRules)
	unsubAlert, chAlert := notifier.Subscribe(model.JobTypeAlert)

	// Ensure listeners have started.
	for range 2 {
		select {
		case <-waiter.calls:
		case <-time.After(200 * time.Millisecond):
			t.Fatal("expected waiter to be invoked")
		}
	}

	notifier.StopAll()

	for _, ch := range []<-chan struct{}{chRules, chAlert} {
		select {
		case _, ok := <-ch:
			assert.False(t, ok, "channels should be closed after StopAll")
		case <-time.After(200 * time.Millisecond):
			t.Fatal("expected channel to close after StopAll")
		}
	}

	// Unsubscribes should remain safe post-stop.
	unsubRules()
	unsubAlert()
}
