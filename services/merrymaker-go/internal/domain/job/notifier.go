package job

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/target/mmk-ui-api/internal/domain/model"
)

// ErrWaiterRequired indicates a notifier cannot be constructed without a waiter.
var ErrWaiterRequired = errors.New("notifier waiter is required")

// Waiter waits for job availability notifications.
type Waiter interface {
	WaitForNotification(ctx context.Context, jobType model.JobType) error
}

// Notifier manages subscriptions for job availability notifications.
type Notifier interface {
	Subscribe(jobType model.JobType) (func(), <-chan struct{})
	StopAll()
}

// NotifierOptions configure the behaviour of the default notifier implementation.
type NotifierOptions struct {
	Waiter     Waiter
	WaitWindow time.Duration
	Backoff    time.Duration
}

// DefaultNotifier is the default implementation of Notifier.
type DefaultNotifier struct {
	waiter     Waiter
	waitWindow time.Duration
	backoff    time.Duration

	mu        sync.Mutex
	subs      map[model.JobType]map[chan struct{}]struct{}
	listeners map[model.JobType]context.CancelFunc
}

// NewNotifier constructs the default notifier implementation.
func NewNotifier(opts NotifierOptions) (*DefaultNotifier, error) {
	if opts.Waiter == nil {
		return nil, ErrWaiterRequired
	}

	waitWindow := opts.WaitWindow
	if waitWindow <= 0 {
		waitWindow = time.Minute
	}

	backoff := opts.Backoff
	if backoff <= 0 {
		backoff = 250 * time.Millisecond
	}

	notifier := &DefaultNotifier{
		waiter:     opts.Waiter,
		waitWindow: waitWindow,
		backoff:    backoff,
		subs:       make(map[model.JobType]map[chan struct{}]struct{}),
		listeners:  make(map[model.JobType]context.CancelFunc),
	}
	return notifier, nil
}

func (n *DefaultNotifier) Subscribe(jobType model.JobType) (func(), <-chan struct{}) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if _, ok := n.listeners[jobType]; !ok {
		ctx, cancel := context.WithCancel(context.Background())
		n.listeners[jobType] = cancel
		go n.listenLoop(ctx, jobType)
	}

	ch := make(chan struct{}, 1)
	if n.subs[jobType] == nil {
		n.subs[jobType] = make(map[chan struct{}]struct{})
	}
	n.subs[jobType][ch] = struct{}{}

	unsub := func() {
		n.mu.Lock()
		defer n.mu.Unlock()
		subscribers := n.subs[jobType]
		if subscribers == nil {
			return
		}

		if _, ok := subscribers[ch]; !ok {
			return
		}
		delete(subscribers, ch)
		drainAndClose(ch)
		if len(subscribers) == 0 {
			n.stopListener(jobType)
			delete(n.subs, jobType)
		}
	}

	return unsub, ch
}

func (n *DefaultNotifier) StopAll() {
	n.mu.Lock()
	defer n.mu.Unlock()

	for jobType, cancel := range n.listeners {
		cancel()
		delete(n.listeners, jobType)
	}
	for jobType, subscribers := range n.subs {
		for ch := range subscribers {
			drainAndClose(ch)
		}
		delete(n.subs, jobType)
	}
}

func (n *DefaultNotifier) stopListener(jobType model.JobType) {
	cancel, ok := n.listeners[jobType]
	if !ok {
		return
	}
	cancel()
	delete(n.listeners, jobType)
}

func (n *DefaultNotifier) listenLoop(ctx context.Context, jobType model.JobType) {
	for ctx.Err() == nil {
		waitCtx, cancel := context.WithTimeout(ctx, n.waitWindow)
		err := n.waiter.WaitForNotification(waitCtx, jobType)
		cancel()

		n.broadcast(jobType)

		if err != nil && ctx.Err() == nil {
			timer := time.NewTimer(n.backoff)
			select {
			case <-ctx.Done():
				if !timer.Stop() {
					<-timer.C
				}
				return
			case <-timer.C:
			}
		}
	}
}

func (n *DefaultNotifier) broadcast(jobType model.JobType) {
	n.mu.Lock()
	defer n.mu.Unlock()

	subscribers := n.subs[jobType]
	for ch := range subscribers {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// drainAndClose removes any buffered notifications before closing the channel so
// receivers observe a closed channel immediately.
func drainAndClose(ch chan struct{}) {
	for {
		select {
		case <-ch:
		default:
			close(ch)
			return
		}
	}
}

var _ Notifier = (*DefaultNotifier)(nil)
