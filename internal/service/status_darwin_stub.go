//go:build !darwin

package service

func dnsFromScutil() ([]string, error) {
	return nil, nil
}
