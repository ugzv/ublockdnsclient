package core

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"strings"
	"time"
)

func CheckLocalDNSProxy(hostname string) error {
	resp, err := queryDNSUDP("127.0.0.1:53", hostname)
	if err != nil {
		return fmt.Errorf("dns query failed via local proxy: %w", err)
	}

	if len(resp) < 4 {
		return errors.New("short DNS response from local proxy")
	}

	flags := binary.BigEndian.Uint16(resp[2:4])
	rcode := flags & 0x000F
	// Treat NXDOMAIN as healthy transport path (proxy is responding).
	if rcode != 0 && rcode != 3 {
		return fmt.Errorf("local proxy returned DNS rcode=%d", rcode)
	}

	return nil
}

func queryDNSUDP(serverAddr, hostname string) ([]byte, error) {
	id := uint16(rand.New(rand.NewSource(time.Now().UnixNano())).Intn(65535))

	conn, err := net.DialTimeout("udp", serverAddr, 2*time.Second)
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }()

	if err := conn.SetDeadline(time.Now().Add(4 * time.Second)); err != nil {
		return nil, err
	}

	return ExchangeDNSQuery(id, hostname, func(payload, buf []byte) (int, error) {
		if _, err := conn.Write(payload); err != nil {
			return 0, err
		}
		return conn.Read(buf)
	})
}

func buildDNSQuery(id uint16, hostname string) []byte {
	q := make([]byte, 12)
	binary.BigEndian.PutUint16(q[0:2], id)
	binary.BigEndian.PutUint16(q[2:4], 0x0100) // recursion desired
	binary.BigEndian.PutUint16(q[4:6], 1)      // QDCOUNT

	for _, label := range strings.Split(hostname, ".") {
		if label == "" {
			continue
		}
		q = append(q, byte(len(label)))
		q = append(q, label...)
	}
	q = append(q, 0x00)                   // end of QNAME
	q = append(q, 0x00, 0x01, 0x00, 0x01) // QTYPE=A, QCLASS=IN
	return q
}
