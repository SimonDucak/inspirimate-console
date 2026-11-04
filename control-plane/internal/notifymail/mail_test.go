package notifymail

import (
	"bufio"
	"context"
	"net"
	"strings"
	"testing"
	"time"
)

func TestTaskNoticeOmitsSecretsAndIncludesTheProject(t *testing.T) {
	subject, body := TaskNotice("Nakopávač", "succeeded", "add a login screen token=sbp_secret", "http://console.localhost:8080/#/app/abc")
	if subject != "Coding finished: Nakopávač" {
		t.Fatalf("subject %q", subject)
	}
	if !strings.Contains(body, "add a login screen token=sbp_secret") || !strings.Contains(body, "http://console.localhost:8080/#/app/abc") {
		t.Fatalf("body %q", body)
	}
	_, failed := TaskNotice("App", "failed", "", "")
	if !strings.Contains(failed, "stopped") || !strings.Contains(failed, "Status: failed") {
		t.Fatalf("failed body %q", failed)
	}
}

func TestSendUsesTheServerAndKeepsThePasswordOutOfTheMessage(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	got := make(chan string, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		r := bufio.NewReader(conn)
		var transcript strings.Builder
		_, _ = conn.Write([]byte("220 localhost\r\n"))
		for {
			line, err := r.ReadString('\n')
			transcript.WriteString(line)
			if err != nil {
				break
			}
			cmd := strings.ToUpper(strings.TrimSpace(line))
			switch {
			case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
				_, _ = conn.Write([]byte("250-localhost\r\n250 AUTH PLAIN\r\n"))
			case strings.HasPrefix(cmd, "AUTH"):
				_, _ = conn.Write([]byte("235 ok\r\n"))
			case strings.HasPrefix(cmd, "DATA"):
				_, _ = conn.Write([]byte("354 go\r\n"))
				for {
					data, err := r.ReadString('\n')
					transcript.WriteString(data)
					if err != nil || data == ".\r\n" {
						break
					}
				}
				_, _ = conn.Write([]byte("250 ok\r\n"))
			case strings.HasPrefix(cmd, "QUIT"):
				_, _ = conn.Write([]byte("221 bye\r\n"))
				got <- transcript.String()
				return
			default:
				_, _ = conn.Write([]byte("250 ok\r\n"))
			}
		}
	}()

	cfg := &Config{Host: "127.0.0.1", Port: portOf(ln), Username: "info@example.com", Password: "smtp-secret", From: "info@example.com"}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := cfg.Send(ctx, []string{"a@example.com"}, "Coding finished: App", "The coding agent finished."); err != nil {
		t.Fatal(err)
	}
	select {
	case transcript := <-got:
		data := transcript[strings.Index(transcript, "DATA"):]
		if strings.Contains(data, "smtp-secret") {
			t.Fatal("password was written into the message")
		}
		if !strings.Contains(transcript, "RCPT TO:<a@example.com>") || !strings.Contains(data, "Coding finished: App") {
			t.Fatalf("transcript %q", transcript)
		}
	case <-ctx.Done():
		t.Fatal("smtp server did not finish")
	}
}

func portOf(ln net.Listener) int {
	return ln.Addr().(*net.TCPAddr).Port
}
