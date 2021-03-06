package pq

import (
	"errors"
	"fmt"
	"io"
	"os"
	"testing"
	"time"
)

var errNilNotification = errors.New("nil notification")

func expectNotification(t *testing.T, ch <-chan *Notification, relname string, extra string) error {
	select {
	case n := <-ch:
		if n == nil {
			return errNilNotification
		}
		if n.Channel != relname || n.Extra != extra {
			return fmt.Errorf("unexpected notification %v", n)
		}
		return nil
	case <-time.After(1500 * time.Millisecond):
		return fmt.Errorf("timeout")
	}
	panic("not reached")
}

func expectNoNotification(t *testing.T, ch <-chan *Notification) error {
	select {
	case n := <-ch:
		return fmt.Errorf("unexpected notification %v", n)
	case <-time.After(100 * time.Millisecond):
		return nil
	}
	panic("not reached")
}

func expectEvent(t *testing.T, eventch <-chan ListenerEventType, et ListenerEventType) error {
	select {
	case e := <-eventch:
		if e != et {
			return fmt.Errorf("unexpected event %v", e)
		}
		return nil
	case <-time.After(1500 * time.Millisecond):
		return fmt.Errorf("timeout")
	}
	panic("not reached")
}

func expectNoEvent(t *testing.T, eventch <-chan ListenerEventType) error {
	select {
	case e := <-eventch:
		return fmt.Errorf("unexpected event %v", e)
	case <-time.After(100 * time.Millisecond):
		return nil
	}
	panic("not reached")
}

func newTestListenerConn(t *testing.T) (*ListenerConn, <-chan *Notification) {
	datname := os.Getenv("PGDATABASE")
	sslmode := os.Getenv("PGSSLMODE")

	if datname == "" {
		os.Setenv("PGDATABASE", "pqgotest")
	}

	if sslmode == "" {
		os.Setenv("PGSSLMODE", "disable")
	}

	notificationChan := make(chan *Notification)
	l, err := NewListenerConn("", notificationChan)
	if err != nil {
		t.Fatal(err)
	}

	return l, notificationChan
}

func TestNewListenerConn(t *testing.T) {
	l, _ := newTestListenerConn(t)

	defer l.Close()
}

func TestConnListen(t *testing.T) {
	l, channel := newTestListenerConn(t)

	defer l.Close()

	db := openTestConn(t)
	defer db.Close()

	ok, err := l.Listen("notify_test")
	if !ok || err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec("NOTIFY notify_test")
	if err != nil {
		t.Fatal(err)
	}

	err = expectNotification(t, channel, "notify_test", "")
	if err != nil {
		t.Fatal(err)
	}
}

func TestConnUnlisten(t *testing.T) {
	l, channel := newTestListenerConn(t)

	defer l.Close()

	db := openTestConn(t)
	defer db.Close()

	ok, err := l.Listen("notify_test")
	if !ok || err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec("NOTIFY notify_test")

	err = expectNotification(t, channel, "notify_test", "")
	if err != nil {
		t.Fatal(err)
	}

	ok, err = l.Unlisten("notify_test")
	if !ok || err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec("NOTIFY notify_test")
	if err != nil {
		t.Fatal(err)
	}

	err = expectNoNotification(t, channel)
	if err != nil {
		t.Fatal(err)
	}
}

func TestConnUnlistenAll(t *testing.T) {
	l, channel := newTestListenerConn(t)

	defer l.Close()

	db := openTestConn(t)
	defer db.Close()

	ok, err := l.Listen("notify_test")
	if !ok || err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec("NOTIFY notify_test")

	err = expectNotification(t, channel, "notify_test", "")
	if err != nil {
		t.Fatal(err)
	}

	ok, err = l.UnlistenAll()
	if !ok || err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec("NOTIFY notify_test")
	if err != nil {
		t.Fatal(err)
	}

	err = expectNoNotification(t, channel)
	if err != nil {
		t.Fatal(err)
	}
}

func TestConnClose(t *testing.T) {
	l, _ := newTestListenerConn(t)
	defer l.Close()

	err := l.Close()
	if err != nil {
		t.Fatal(err)
	}
	err = l.Close()
	if err != errListenerConnClosed {
		t.Fatalf("expected errListenerConnClosed; got %v", err)
	}
}

func TestConnPing(t *testing.T) {
	l, _ := newTestListenerConn(t)
	defer l.Close()
	err := l.Ping()
	if err != nil {
		t.Fatal(err)
	}
	err = l.Close()
	if err != nil {
		t.Fatal(err)
	}
	err = l.Ping()
	if err != errListenerConnClosed {
		t.Fatalf("expected errListenerConnClosed; got %v", err)
	}
}

func TestNotifyExtra(t *testing.T) {
	db := openTestConn(t)
	defer db.Close()

	if getServerVersion(t, db) < 90000 {
		t.Log("skipping test due to old PG version")
		return
	}

	l, channel := newTestListenerConn(t)
	defer l.Close()

	ok, err := l.Listen("notify_test")
	if !ok || err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec("NOTIFY notify_test, 'something'")
	if err != nil {
		t.Fatal(err)
	}

	err = expectNotification(t, channel, "notify_test", "something")
	if err != nil {
		t.Fatal(err)
	}
}

// create a new test listener and also set the timeouts
func newTestListenerTimeout(t *testing.T, min time.Duration, max time.Duration) (*Listener, <-chan ListenerEventType) {
	datname := os.Getenv("PGDATABASE")
	sslmode := os.Getenv("PGSSLMODE")

	if datname == "" {
		os.Setenv("PGDATABASE", "pqgotest")
	}

	if sslmode == "" {
		os.Setenv("PGSSLMODE", "disable")
	}

	eventch := make(chan ListenerEventType, 16)
	l := NewListener("", min, max, func(t ListenerEventType, err error) { eventch <- t })
	err := expectEvent(t, eventch, ListenerEventConnected)
	if err != nil {
		t.Fatal(err)
	}
	return l, eventch
}

func newTestListener(t *testing.T) (*Listener, <-chan ListenerEventType) {
	return newTestListenerTimeout(t, time.Hour, time.Hour)
}

func TestListenerListen(t *testing.T) {
	l, _ := newTestListener(t)
	defer l.Close()

	db := openTestConn(t)
	defer db.Close()

	err := l.Listen("notify_listen_test")
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec("NOTIFY notify_listen_test")
	if err != nil {
		t.Fatal(err)
	}

	err = expectNotification(t, l.Notify, "notify_listen_test", "")
	if err != nil {
		t.Fatal(err)
	}
}

func TestListenerUnlisten(t *testing.T) {
	l, _ := newTestListener(t)
	defer l.Close()

	db := openTestConn(t)
	defer db.Close()

	err := l.Listen("notify_listen_test")
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec("NOTIFY notify_listen_test")
	if err != nil {
		t.Fatal(err)
	}

	err = l.Unlisten("notify_listen_test")
	if err != nil {
		t.Fatal(err)
	}

	err = expectNotification(t, l.Notify, "notify_listen_test", "")
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec("NOTIFY notify_listen_test")
	if err != nil {
		t.Fatal(err)
	}

	err = expectNoNotification(t, l.Notify)
	if err != nil {
		t.Fatal(err)
	}
}

func TestListenerUnlistenAll(t *testing.T) {
	l, _ := newTestListener(t)
	defer l.Close()

	db := openTestConn(t)
	defer db.Close()

	err := l.Listen("notify_listen_test")
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec("NOTIFY notify_listen_test")
	if err != nil {
		t.Fatal(err)
	}

	err = l.UnlistenAll()
	if err != nil {
		t.Fatal(err)
	}

	err = expectNotification(t, l.Notify, "notify_listen_test", "")
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec("NOTIFY notify_listen_test")
	if err != nil {
		t.Fatal(err)
	}

	err = expectNoNotification(t, l.Notify)
	if err != nil {
		t.Fatal(err)
	}
}

func TestListenerFailedQuery(t *testing.T) {
	l, eventch := newTestListener(t)
	defer l.Close()

	db := openTestConn(t)
	defer db.Close()

	err := l.Listen("notify_listen_test")
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec("NOTIFY notify_listen_test")
	if err != nil {
		t.Fatal(err)
	}

	err = expectNotification(t, l.Notify, "notify_listen_test", "")
	if err != nil {
		t.Fatal(err)
	}

	// shouldn't cause a disconnect
	ok, err := l.cn.ExecSimpleQuery("SELECT error")
	if !ok {
		t.Fatalf("could not send query to server: %v", err)
	}
	_, ok = err.(PGError)
	if !ok {
		t.Fatalf("unexpected error %v", err)
	}
	err = expectNoEvent(t, eventch)
	if err != nil {
		t.Fatal(err)
	}

	// should still work
	_, err = db.Exec("NOTIFY notify_listen_test")
	if err != nil {
		t.Fatal(err)
	}

	err = expectNotification(t, l.Notify, "notify_listen_test", "")
	if err != nil {
		t.Fatal(err)
	}
}

func TestListenerReconnect(t *testing.T) {
	l, eventch := newTestListenerTimeout(t, 20*time.Millisecond, time.Hour)
	defer l.Close()

	db := openTestConn(t)
	defer db.Close()

	err := l.Listen("notify_listen_test")
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec("NOTIFY notify_listen_test")
	if err != nil {
		t.Fatal(err)
	}

	err = expectNotification(t, l.Notify, "notify_listen_test", "")
	if err != nil {
		t.Fatal(err)
	}

	// kill the connection and make sure it comes back up
	ok, err := l.cn.ExecSimpleQuery("SELECT pg_terminate_backend(pg_backend_pid())")
	if ok {
		t.Fatalf("could not kill the connection: %v", err)
	}
	if err != io.EOF {
		t.Fatalf("unexpected error %v", err)
	}
	err = expectEvent(t, eventch, ListenerEventDisconnected)
	if err != nil {
		t.Fatal(err)
	}
	err = expectEvent(t, eventch, ListenerEventReconnected)
	if err != nil {
		t.Fatal(err)
	}

	// should still work
	_, err = db.Exec("NOTIFY notify_listen_test")
	if err != nil {
		t.Fatal(err)
	}

	// should get nil after Reconnected
	err = expectNotification(t, l.Notify, "", "")
	if err != errNilNotification {
		t.Fatal(err)
	}

	err = expectNotification(t, l.Notify, "notify_listen_test", "")
	if err != nil {
		t.Fatal(err)
	}
}

func TestListenerClose(t *testing.T) {
	l, _ := newTestListenerTimeout(t, 20*time.Millisecond, time.Hour)
	defer l.Close()

	err := l.Close()
	if err != nil {
		t.Fatal(err)
	}
	err = l.Close()
	if err != errListenerClosed {
		t.Fatalf("expected errListenerClosed; got %v", err)
	}
}

func TestListenerPing(t *testing.T) {
	l, _ := newTestListenerTimeout(t, 20*time.Millisecond, time.Hour)
	defer l.Close()

	err := l.Ping()
	if err != nil {
		t.Fatal(err)
	}

	err = l.Close()
	if err != nil {
		t.Fatal(err)
	}

	err = l.Ping()
	if err != errListenerClosed {
		t.Fatalf("expected errListenerClosed; got %v", err)
	}
}
