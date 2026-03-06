package core

import (
	"encoding/binary"
	"errors"
)

func ExchangeDNSQuery(queryID uint16, hostname string, exchange func(payload, buf []byte) (int, error)) ([]byte, error) {
	payload := buildDNSQuery(queryID, hostname)
	buf := make([]byte, 2048)

	n, err := exchange(payload, buf)
	if err != nil {
		return nil, err
	}
	resp := buf[:n]
	if len(resp) < 2 {
		return nil, errors.New("short DNS response")
	}
	if binary.BigEndian.Uint16(resp[:2]) != queryID {
		return nil, errors.New("mismatched DNS transaction id")
	}
	return resp, nil
}
