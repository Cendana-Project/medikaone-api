package email

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

func TestSMTPSenderDisabledReturnsError(t *testing.T) {
	sender := NewSMTPSender(&Config{Enabled: false})
	if err := sender.Send("recipient@example.com", "subject", "body"); !errors.Is(err, ErrDeliveryDisabled) {
		t.Fatalf("Send() error = %v, want ErrDeliveryDisabled", err)
	}
}

func TestSMTPSenderRejectsWhenGlobalDeliverySlotsAreBusy(t *testing.T) {
	for i := 0; i < cap(smtpDeliverySlots); i++ {
		smtpDeliverySlots <- struct{}{}
	}
	t.Cleanup(func() {
		for i := 0; i < cap(smtpDeliverySlots); i++ {
			<-smtpDeliverySlots
		}
	})
	sender := NewSMTPSender(&Config{
		Enabled: true, Host: "smtp.example.test", Port: 587,
		Username: "apikey", Password: "secret", FromEmail: "sender@example.test",
		Timeout: time.Second, UseSTARTTLS: true,
	})
	if err := sender.SendWithContext(context.Background(), "recipient@example.test", "subject", "body"); !errors.Is(err, ErrDeliveryBusy) {
		t.Fatalf("SendWithContext() error = %v, want ErrDeliveryBusy", err)
	}
}

func TestSMTPSenderRejectsServerWithoutSTARTTLS(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer conn.Close()
		_, _ = fmt.Fprint(conn, "220 localhost ESMTP\r\n")
		reader := bufio.NewReader(conn)
		if _, readErr := reader.ReadString('\n'); readErr != nil {
			return
		}
		_, _ = fmt.Fprint(conn, "250-localhost\r\n250 AUTH PLAIN\r\n")
	}()

	host, port := splitListenerAddress(t, listener.Addr().String())
	sender := NewSMTPSender(&Config{
		Enabled:     true,
		Host:        host,
		Port:        port,
		Username:    "apikey",
		Password:    "secret",
		FromEmail:   "sender@example.com",
		Timeout:     time.Second,
		UseSTARTTLS: true,
	})
	err = sender.SendWithContext(context.Background(), "recipient@example.com", "subject", "body")
	if err == nil || !strings.Contains(err.Error(), "does not advertise required STARTTLS") {
		t.Fatalf("SendWithContext() error = %v, want required STARTTLS error", err)
	}
	<-serverDone
}

func TestSMTPSenderAppliesDeadlineToGreeting(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer conn.Close()
		_, _ = bufio.NewReader(conn).ReadByte()
	}()

	host, port := splitListenerAddress(t, listener.Addr().String())
	sender := NewSMTPSender(&Config{
		Enabled:     true,
		Host:        host,
		Port:        port,
		Username:    "apikey",
		Password:    "secret",
		FromEmail:   "sender@example.com",
		Timeout:     75 * time.Millisecond,
		UseSTARTTLS: true,
	})
	started := time.Now()
	err = sender.SendWithContext(context.Background(), "recipient@example.com", "subject", "body")
	if err == nil {
		t.Fatal("SendWithContext() expected a deadline error")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("SMTP greeting was not bounded by timeout; elapsed=%s", elapsed)
	}
}

func TestSMTPSenderRejectsHeaderInjection(t *testing.T) {
	sender := NewSMTPSender(&Config{
		Enabled:     true,
		Timeout:     time.Second,
		UseSTARTTLS: true,
		FromEmail:   "sender@example.com",
	})
	err := sender.SendWithContext(context.Background(), "recipient@example.com", "hello\r\nBcc: victim@example.com", "body")
	if err == nil || !strings.Contains(err.Error(), "invalid newline") {
		t.Fatalf("SendWithContext() error = %v, want header validation error", err)
	}
}

func splitListenerAddress(t *testing.T, address string) (string, int) {
	t.Helper()
	host, portString, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatal(err)
	}
	var port int
	if _, err := fmt.Sscanf(portString, "%d", &port); err != nil {
		t.Fatal(err)
	}
	return host, port
}
