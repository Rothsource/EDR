//go:build windows

package auth

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modWevtapi = windows.NewLazySystemDLL("wevtapi.dll")

	procEvtSubscribe      = modWevtapi.NewProc("EvtSubscribe")
	procEvtCreateBookmark = modWevtapi.NewProc("EvtCreateBookmark")
	procEvtUpdateBookmark = modWevtapi.NewProc("EvtUpdateBookmark")
	procEvtRender         = modWevtapi.NewProc("EvtRender")
	procEvtClose          = modWevtapi.NewProc("EvtClose")
)

const (
	evtSubscribeStartAfterBookmark = 3
	evtSubscribeToFutureEvents     = 1
	evtSubscribeActionError        = 0
	evtSubscribeActionDeliver      = 1
	evtRenderEventXml              = 1
	evtRenderBookmark              = 2
)

type subscription struct {
	handle   windows.Handle
	bookmark windows.Handle
	done     chan error  // receives one error when the subscription must be rebuilt
	failed   atomic.Bool // set after a failed push; later events are ignored until resubscribe
}

func (s *subscription) Close() {
	if s.handle != 0 {
		procEvtClose.Call(uintptr(s.handle))
	}
	if s.bookmark != 0 {
		procEvtClose.Call(uintptr(s.bookmark))
	}
}

// signal asks runWithBackoff to tear this subscription down. Never blocks.
func (s *subscription) signal(err error) {
	select {
	case s.done <- err:
	default:
	}
}

// subscribe opens a live subscription on the Security channel using a
// structured XML query, resuming after bookmarkXML if non-empty (else
// future events only).
//
// For every delivered record the callback:
//  1. renders it to XML and calls onEvent (which must Push it durably);
//  2. only if onEvent returned nil, advances the in-memory bookmark;
//  3. then hands the NEW bookmark XML to persist.
//
// If onEvent fails, the subscription is marked failed: it ignores every
// later event (so the bookmark can never move past the unsent one) and
// asks runWithBackoff to resubscribe from the last persisted bookmark.
func subscribe(xpath, bookmarkXML string, onEvent func(xmlEvent string) error, persist func(bookmarkXML string)) (*subscription, error) {
	sub := &subscription{done: make(chan error, 1)}

	var bookmarkHandle windows.Handle
	flags := uintptr(evtSubscribeToFutureEvents)

	if bookmarkXML != "" {
		bmPtr, err := syscall.UTF16PtrFromString(bookmarkXML)
		if err != nil {
			return nil, err
		}
		r, _, callErr := procEvtCreateBookmark.Call(uintptr(unsafe.Pointer(bmPtr)))
		if r == 0 {
			return nil, fmt.Errorf("EvtCreateBookmark failed: %w", callErr)
		}
		bookmarkHandle = windows.Handle(r)
		flags = evtSubscribeStartAfterBookmark
	}
	sub.bookmark = bookmarkHandle

	xpathPtr, err := syscall.UTF16PtrFromString(xpath)
	if err != nil {
		return nil, err
	}

	callback := syscall.NewCallback(func(action, userContext uintptr, eventHandle windows.Handle) uintptr {
		if action == evtSubscribeActionError {
			sub.signal(fmt.Errorf("subscription error, code=%d", eventHandle))
			return 0
		}
		if sub.failed.Load() {
			return 0 // waiting for resubscribe; don't let the bookmark skip ahead
		}

		xmlStr, err := renderEventXML(eventHandle)
		if err != nil {
			fmt.Fprintf(os.Stderr, "auth(windows): render failed: %v\n", err)
			return 0
		}
		if err := onEvent(xmlStr); err != nil {
			fmt.Fprintf(os.Stderr, "auth(windows): handler failed, resubscribing from last bookmark: %v\n", err)
			sub.failed.Store(true)
			sub.signal(err)
			return 0
		}

		// Push succeeded: advance the bookmark, then persist it.
		updateBookmark(&sub.bookmark, eventHandle)
		if persist != nil {
			if xmlBM, rerr := renderBookmarkXML(sub.bookmark); rerr == nil {
				persist(xmlBM)
			} else {
				fmt.Fprintf(os.Stderr, "auth(windows): rendering bookmark failed: %v\n", rerr)
			}
		}
		return 0
	})

	r, _, callErr := procEvtSubscribe.Call(
		0, 0,
		0, // channel path is NULL: the structured query names the channel
		uintptr(unsafe.Pointer(xpathPtr)),
		uintptr(bookmarkHandle),
		0,
		callback,
		flags,
	)
	if r == 0 {
		if bookmarkHandle != 0 {
			procEvtClose.Call(uintptr(bookmarkHandle))
		}
		return nil, fmt.Errorf("EvtSubscribe failed: %w", callErr)
	}
	sub.handle = windows.Handle(r)
	return sub, nil
}

func renderEventXML(h windows.Handle) (string, error) {
	var used, propCount uint32
	procEvtRender.Call(0, uintptr(h), evtRenderEventXml, 0, 0, uintptr(unsafe.Pointer(&used)), uintptr(unsafe.Pointer(&propCount)))
	buf := make([]uint16, used/2+1)
	r, _, callErr := procEvtRender.Call(0, uintptr(h), evtRenderEventXml,
		uintptr(len(buf)*2), uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&used)), uintptr(unsafe.Pointer(&propCount)))
	if r == 0 {
		return "", fmt.Errorf("EvtRender failed: %w", callErr)
	}
	return syscall.UTF16ToString(buf), nil
}

func updateBookmark(bookmark *windows.Handle, eventHandle windows.Handle) {
	if *bookmark == 0 {
		r, _, _ := procEvtCreateBookmark.Call(0)
		*bookmark = windows.Handle(r)
	}
	procEvtUpdateBookmark.Call(uintptr(*bookmark), uintptr(eventHandle))
}

func renderBookmarkXML(bookmark windows.Handle) (string, error) {
	var used, propCount uint32
	procEvtRender.Call(0, uintptr(bookmark), evtRenderBookmark, 0, 0, uintptr(unsafe.Pointer(&used)), uintptr(unsafe.Pointer(&propCount)))
	buf := make([]uint16, used/2+1)
	r, _, callErr := procEvtRender.Call(0, uintptr(bookmark), evtRenderBookmark,
		uintptr(len(buf)*2), uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&used)), uintptr(unsafe.Pointer(&propCount)))
	if r == 0 {
		return "", fmt.Errorf("EvtRender(bookmark) failed: %w", callErr)
	}
	return syscall.UTF16ToString(buf), nil
}

// loadBookmark/saveBookmarkXML persist bookmark XML (not a record-ID number).
func loadBookmark(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(b), nil
}

func saveBookmarkXML(path, xml string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(xml); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func parseWindowsTime(systemTime string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, systemTime)
	if err != nil {
		return time.Now().UTC()
	}
	return t.UTC()
}

// sleepCtx sleeps for d but returns false early if ctx is cancelled, so a
// config reload is never stuck behind a long backoff.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// runWithBackoff wraps subscribe() with resubscribe-on-error, per Step 4's
// "resubscribe with backoff if the Event Log service restarts."
func runWithBackoff(ctx context.Context, xpath, bookmarkFile string, onEvent func(xmlEvent string) error) error {
	backoff := time.Second
	const maxBackoff = 30 * time.Second

	for {
		if ctx.Err() != nil {
			return nil
		}

		bm, _ := loadBookmark(bookmarkFile)

		sub, err := subscribe(xpath, bm, onEvent, func(xmlBM string) {
			if err := saveBookmarkXML(bookmarkFile, xmlBM); err != nil {
				fmt.Fprintf(os.Stderr, "auth(windows): saving bookmark: %v\n", err)
			}
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "auth(windows): subscribe failed, retrying in %s: %v\n", backoff, err)
			if !sleepCtx(ctx, backoff) {
				return nil
			}
			backoff = min(backoff*2, maxBackoff)
			continue
		}
		backoff = time.Second

		select {
		case subErr := <-sub.done:
			fmt.Fprintf(os.Stderr, "auth(windows): subscription dropped, resubscribing: %v\n", subErr)
			sub.Close()
			if !sleepCtx(ctx, backoff) {
				return nil
			}
			backoff = min(backoff*2, maxBackoff)
		case <-ctx.Done():
			sub.Close()
			return nil
		}
	}
}
