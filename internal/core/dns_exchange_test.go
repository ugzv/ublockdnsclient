package core

import (
	"errors"
	"testing"
)

func TestExchangeDNSQuery(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		resp, err := ExchangeDNSQuery(0x1234, "example.com", func(payload, buf []byte) (int, error) {
			copy(buf, payload)
			return len(payload), nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(resp) == 0 {
			t.Fatal("expected response bytes")
		}
	})

	t.Run("short response", func(t *testing.T) {
		_, err := ExchangeDNSQuery(0x1234, "example.com", func(payload, buf []byte) (int, error) {
			buf[0] = 0x12
			return 1, nil
		})
		if err == nil || err.Error() != "short DNS response" {
			t.Fatalf("expected short response error, got %v", err)
		}
	})

	t.Run("mismatched id", func(t *testing.T) {
		_, err := ExchangeDNSQuery(0x1234, "example.com", func(payload, buf []byte) (int, error) {
			copy(buf, payload)
			buf[0], buf[1] = 0x12, 0x35
			return len(payload), nil
		})
		if err == nil || err.Error() != "mismatched DNS transaction id" {
			t.Fatalf("expected mismatched id error, got %v", err)
		}
	})

	t.Run("transport error", func(t *testing.T) {
		want := errors.New("boom")
		_, err := ExchangeDNSQuery(0x1234, "example.com", func(payload, buf []byte) (int, error) {
			return 0, want
		})
		if !errors.Is(err, want) {
			t.Fatalf("expected wrapped transport error, got %v", err)
		}
	})
}
